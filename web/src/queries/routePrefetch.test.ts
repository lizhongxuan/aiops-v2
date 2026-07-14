import { QueryClient } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";

import { prefetchRouteData } from "@/queries/routePrefetch";

function createClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
}

describe("routePrefetch", () => {
  it("prefetches chat route dependencies", () => {
    const queryClient = createClient();
    const spy = vi
      .spyOn(queryClient, "prefetchQuery")
      .mockResolvedValue(undefined);

    prefetchRouteData(queryClient, "/");

    expect(spy).toHaveBeenCalledTimes(3);
    expect(spy.mock.calls.map((call) => call[0].queryKey)).toEqual([
      ["sessions", "list"],
      ["hosts", "list"],
      ["settings", "llmConfig"],
    ]);
  });

  it("prefetches hosts route dependencies", () => {
    const queryClient = createClient();
    const spy = vi
      .spyOn(queryClient, "prefetchQuery")
      .mockResolvedValue(undefined);

    prefetchRouteData(queryClient, "/settings/hosts");

    expect(spy).toHaveBeenCalledTimes(3);
    expect(spy.mock.calls.map((call) => call[0].queryKey)).toEqual([
      ["hosts", "list"],
      ["sessions", "list"],
      ["terminalSessions", "list"],
    ]);
  });
});
