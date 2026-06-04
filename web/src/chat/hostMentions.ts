export type HostMentionCandidate = {
  tokenId: string;
  raw: string;
  value: string;
  start: number;
  end: number;
  source: "ip_literal" | "hostname_literal";
};

const HOST_TOKEN_PATTERN = /@([A-Za-z0-9](?:[A-Za-z0-9.-]{0,251}[A-Za-z0-9])?)/g;
const IPV4_PATTERN = /^(?:25[0-5]|2[0-4]\d|1?\d?\d)(?:\.(?:25[0-5]|2[0-4]\d|1?\d?\d)){3}$/;

export function parseHostMentionCandidates(input: string): HostMentionCandidate[] {
  const candidates: HostMentionCandidate[] = [];
  for (const match of input.matchAll(HOST_TOKEN_PATTERN)) {
    const atIndex = match.index ?? 0;
    if (isEmailLikeMention(input, atIndex)) {
      continue;
    }
    const value = match[1];
    const raw = `@${value}`;
    candidates.push({
      tokenId: `hm-${atIndex}-${value.toLowerCase()}`,
      raw,
      value,
      start: atIndex,
      end: atIndex + raw.length,
      source: IPV4_PATTERN.test(value) ? "ip_literal" : "hostname_literal",
    });
  }
  return candidates;
}

export function uniqueHostMentionKeys(candidates: HostMentionCandidate[]): string[] {
  return Array.from(new Set(candidates.map((item) => item.value.toLowerCase())));
}

export function buildHostMentionMetadata(candidates: HostMentionCandidate[]): Record<string, string> {
  return {
    "aiops.hostops.mentions": JSON.stringify(candidates),
    "aiops.hostops.clientDetectedMultiHost": String(uniqueHostMentionKeys(candidates).length >= 2),
  };
}

function isEmailLikeMention(input: string, atIndex: number) {
  const previous = atIndex > 0 ? input[atIndex - 1] : "";
  return /[A-Za-z0-9._%+-]/.test(previous);
}
