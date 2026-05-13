declare module "@/lib/hostListViewModel" {
  export function buildHostListViewModel(input?: unknown): any;
}

declare module "@/data/opsWorkspace" {
  export const experiencePacks: any[];
}

declare module "@/lib/mcpUiCardModel" {
  export function normalizeMcpUiCard(input?: unknown, defaults?: unknown): any;
}
