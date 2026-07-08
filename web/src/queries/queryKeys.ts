export const queryKeys = {
  hosts: {
    all: ["hosts"] as const,
    list: () => ["hosts", "list"] as const,
  },
  sessions: {
    all: ["sessions"] as const,
    list: () => ["sessions", "list"] as const,
    byKind: (kind: string) => ["sessions", "list", kind] as const,
  },
  terminalSessions: {
    all: ["terminalSessions"] as const,
    list: () => ["terminalSessions", "list"] as const,
  },
  llmConfig: () => ["settings", "llmConfig"] as const,
  assistantTransport: {
    all: ["assistantTransport"] as const,
    state: (scope: string, sessionId: string) =>
      ["assistantTransport", "state", scope, sessionId] as const,
  },
};
