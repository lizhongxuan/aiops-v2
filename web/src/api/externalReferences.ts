import httpClient from "./httpClient";

export type ExternalReferenceKind = "blob" | "card" | "file" | "mcp_resource" | "unknown";

export type ExternalReferenceContent = {
  id: string;
  kind: ExternalReferenceKind;
  contentType: string;
  summary: string;
  content: string;
  bytes: number;
  digest: string;
  title: string;
  uri: string;
  cardRef: string;
  filePath: string;
  raw: Record<string, unknown>;
};

type ExternalReferencesHttpClient = {
  get(path: string): Promise<unknown>;
};

const SUPPORTED_KINDS = new Set<ExternalReferenceKind>(["blob", "card", "file", "mcp_resource"]);

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

function text(value: unknown, fallback = "") {
  if (value === undefined || value === null) return fallback;
  const normalized = String(value).trim();
  return normalized || fallback;
}

function numberValue(value: unknown, fallback = 0) {
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value === "string" && value.trim()) {
    const parsed = Number(value);
    if (Number.isFinite(parsed)) return parsed;
  }
  return fallback;
}

function pick(source: Record<string, unknown>, ...keys: string[]) {
  for (const key of keys) {
    const value = source[key];
    if (value !== undefined && value !== null && value !== "") return value;
  }
  return "";
}

function normalizeKind(value: unknown): ExternalReferenceKind {
  const kind = text(value).toLowerCase();
  return SUPPORTED_KINDS.has(kind as ExternalReferenceKind) ? (kind as ExternalReferenceKind) : "unknown";
}

function normalizeContent(value: unknown) {
  if (typeof value === "string") return value;
  if (value === undefined || value === null) return "";
  return JSON.stringify(value, null, 2);
}

export function normalizeExternalReferenceContent(value: unknown): ExternalReferenceContent {
  const source = isRecord(value) ? value : {};
  const id = text(pick(source, "id", "referenceId", "reference_id"));
  const content = normalizeContent(pick(source, "content", "rawContent", "raw_content", "text"));
  return {
    id,
    kind: normalizeKind(pick(source, "kind", "type")),
    contentType: text(pick(source, "contentType", "content_type"), "text/plain"),
    summary: text(source.summary),
    content,
    bytes: numberValue(pick(source, "bytes", "size"), content.length),
    digest: text(source.digest),
    title: text(source.title),
    uri: text(source.uri),
    cardRef: text(pick(source, "cardRef", "card_ref")),
    filePath: text(pick(source, "filePath", "file_path")),
    raw: source,
  };
}

export async function verifyExternalReferenceDigest(reference: ExternalReferenceContent): Promise<boolean | null> {
  if (!reference.digest) return null;
  const match = /^sha256:([a-f0-9]{64})$/i.exec(reference.digest);
  if (!match || !globalThis.crypto?.subtle) return null;

  const bytes = new TextEncoder().encode(reference.content);
  const hash = await globalThis.crypto.subtle.digest("SHA-256", bytes);
  const hex = Array.from(new Uint8Array(hash))
    .map((byte) => byte.toString(16).padStart(2, "0"))
    .join("");
  return hex === match[1].toLowerCase();
}

export function createExternalReferencesApi(client: ExternalReferencesHttpClient = httpClient) {
  return {
    async getExternalReference(referenceId: string) {
      const payload = await client.get(`/api/external-references/${encodeURIComponent(referenceId)}`);
      return normalizeExternalReferenceContent(payload);
    },
  };
}

const externalReferencesApi = createExternalReferencesApi();

export const getExternalReference = externalReferencesApi.getExternalReference;
