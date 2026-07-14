import { describe, expect, it, vi } from "vitest";

import { hostListQuery } from "@/queries/hostQueries";
import { queryKeys } from "@/queries/queryKeys";
import { sessionListQuery } from "@/queries/sessionQueries";
import { llmConfigQuery } from "@/queries/settingsQueries";
import { terminalSessionListQuery } from "@/queries/terminalQueries";

describe("aiops query options", () => {
  it("uses stable query keys for shared server state", () => {
    expect(queryKeys.hosts.list()).toEqual(["hosts", "list"]);
    expect(queryKeys.sessions.list()).toEqual(["sessions", "list"]);
    expect(queryKeys.terminalSessions.list()).toEqual([
      "terminalSessions",
      "list",
    ]);
    expect(queryKeys.llmConfig()).toEqual(["settings", "llmConfig"]);
  });

  it("sets cache windows that keep pages warm across navigation", () => {
    expect(hostListQuery().staleTime).toBe(30_000);
    expect(hostListQuery().gcTime).toBe(10 * 60_000);
    expect(sessionListQuery().staleTime).toBe(15_000);
    expect(terminalSessionListQuery().staleTime).toBe(30_000);
    expect(llmConfigQuery().staleTime).toBe(5 * 60_000);
  });

  it("passes abort signals into query functions", async () => {
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ items: [] }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    const controller = new AbortController();

    await hostListQuery().queryFn({
      signal: controller.signal,
      queryKey: queryKeys.hosts.list(),
      meta: undefined,
    } as never);

    expect(fetchSpy).toHaveBeenCalledWith(
      "/api/v1/hosts",
      expect.objectContaining({ signal: controller.signal }),
    );
    fetchSpy.mockRestore();
  });
});
