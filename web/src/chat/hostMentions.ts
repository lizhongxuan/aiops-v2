export type HostMentionCandidate = {
  tokenId: string;
  raw: string;
  value: string;
  start: number;
  end: number;
  source: "ip_literal" | "hostname_literal" | "local_alias";
  hostId?: string;
};

export type SpecialAiMentionCandidate = {
  tokenId: string;
  raw: string;
  value: "coroot" | "ops_graph" | "ops_manus" | "ops_manuals";
  start: number;
  end: number;
  source: "ai_tool";
};

const HOST_TOKEN_PATTERN = /@([A-Za-z0-9](?:[A-Za-z0-9.-]{0,251}[A-Za-z0-9])?)/g;
const SPECIAL_AI_TOKEN_PATTERN =
  /(^|[^\p{L}\p{N}_])@(coroot|ops_graph|ops_manus|ops_manuals)(?=$|[^\p{L}\p{N}_])/giu;
const IPV4_PATTERN = /^(?:25[0-5]|2[0-4]\d|1?\d?\d)(?:\.(?:25[0-5]|2[0-4]\d|1?\d?\d)){3}$/;
const SPECIAL_AI_TRIGGER_MENTIONS = new Set(["coroot", "ops_graph", "ops_manus", "ops_manuals"]);

export function parseHostMentionCandidates(input: string): HostMentionCandidate[] {
  const candidates: HostMentionCandidate[] = [];
  for (const match of input.matchAll(HOST_TOKEN_PATTERN)) {
    const atIndex = match.index ?? 0;
    if (isEmailLikeMention(input, atIndex)) {
      continue;
    }
    const value = match[1];
    if (input[atIndex + value.length + 1] === "_") {
      continue;
    }
    const normalizedValue = value.toLowerCase();
    if (SPECIAL_AI_TRIGGER_MENTIONS.has(normalizedValue)) {
      continue;
    }
    const raw = `@${value}`;
    const localAlias = normalizedValue === "local";
    candidates.push({
      tokenId: `hm-${atIndex}-${value.toLowerCase()}`,
      raw,
      value: localAlias ? "local" : value,
      start: atIndex,
      end: atIndex + raw.length,
      source: localAlias
        ? "local_alias"
        : IPV4_PATTERN.test(value)
          ? "ip_literal"
          : "hostname_literal",
      ...(localAlias ? { hostId: "server-local" } : {}),
    });
  }
  return candidates;
}

export function parseSpecialAiMentionCandidates(input: string): SpecialAiMentionCandidate[] {
  const candidates: SpecialAiMentionCandidate[] = [];
  for (const match of input.matchAll(SPECIAL_AI_TOKEN_PATTERN)) {
    const prefix = match[1] || "";
    const value = match[2].toLowerCase() as SpecialAiMentionCandidate["value"];
    const start = (match.index ?? 0) + prefix.length;
    const raw = `@${value}`;
    candidates.push({
      tokenId: `aim-${start}-${value}`,
      raw,
      value,
      start,
      end: start + raw.length,
      source: "ai_tool",
    });
  }
  return candidates;
}

export function uniqueHostMentionKeys(candidates: HostMentionCandidate[]): string[] {
  return Array.from(new Set(candidates.map((item) => item.value.toLowerCase())));
}

export function buildHostMentionMetadata(candidates: HostMentionCandidate[]): Record<string, string> {
  if (!candidates.length) {
    return {};
  }
  const serializedCandidates = candidates.map((candidate) =>
    candidate.source === "local_alias"
      ? { ...candidate, value: "server-local", hostId: "server-local" }
      : candidate,
  );
  return {
    "aiops.hostops.mentions": JSON.stringify(serializedCandidates),
    "aiops.hostops.clientDetectedMultiHost": String(uniqueHostMentionKeys(candidates).length >= 2),
  };
}

function isEmailLikeMention(input: string, atIndex: number) {
  const previous = atIndex > 0 ? input[atIndex - 1] : "";
  return /[A-Za-z0-9._%+-]/.test(previous);
}
