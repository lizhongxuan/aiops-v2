import { describe, expect, it, vi } from "vitest";

import {
  createExternalReferencesApi,
  normalizeExternalReferenceContent,
  verifyExternalReferenceDigest,
} from "./externalReferences";

function createRecordingHttpClient(payload: unknown = { ok: true }, error?: Error) {
  const calls: Array<{ method: string; path: string }> = [];
  return {
    calls,
    get: vi.fn((path: string) => {
      calls.push({ method: "GET", path });
      return error ? Promise.reject(error) : Promise.resolve(payload);
    }),
  };
}

describe("external references API", () => {
  it("reads and normalizes external references from the same-origin endpoint", async () => {
    const http = createRecordingHttpClient({
      id: "ref/blob 1",
      kind: "blob",
      content_type: "text/plain",
      summary: "原始日志",
      content: "raw evidence",
      bytes: "12",
      digest: "sha256:3bdaa3c452e349e0b3a07cbcf915a971518544204666e169d7166fd618eb96ae",
    });
    const api = createExternalReferencesApi(http);

    await expect(api.getExternalReference("ref/blob 1")).resolves.toMatchObject({
      id: "ref/blob 1",
      kind: "blob",
      contentType: "text/plain",
      summary: "原始日志",
      content: "raw evidence",
      bytes: 12,
    });
    expect(http.calls).toEqual([{ method: "GET", path: "/api/external-references/ref%2Fblob%201" }]);
  });

  it("surfaces API errors to the caller", async () => {
    const api = createExternalReferencesApi(createRecordingHttpClient(null, new Error("not found")));

    await expect(api.getExternalReference("ref-missing")).rejects.toThrow("not found");
  });

  it("falls back safely for unknown reference kinds", () => {
    expect(normalizeExternalReferenceContent({ id: "ref-unknown", kind: "sqlite", content: "" })).toMatchObject({
      id: "ref-unknown",
      kind: "unknown",
      contentType: "text/plain",
    });
  });

  it("detects digest matches and mismatches for sha256 references", async () => {
    await expect(
      verifyExternalReferenceDigest(normalizeExternalReferenceContent({
        id: "ref-ok",
        content: "raw evidence",
        digest: "sha256:3bdaa3c452e349e0b3a07cbcf915a971518544204666e169d7166fd618eb96ae",
      })),
    ).resolves.toBe(true);
    await expect(
      verifyExternalReferenceDigest(normalizeExternalReferenceContent({
        id: "ref-bad",
        content: "tampered",
        digest: "sha256:3bdaa3c452e349e0b3a07cbcf915a971518544204666e169d7166fd618eb96ae",
      })),
    ).resolves.toBe(false);
  });
});
