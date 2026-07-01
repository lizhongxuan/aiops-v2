import { describe, expect, it, vi } from "vitest";

import { fetchCapabilities } from "./capabilityManagementApi";
import { buildCapabilityManagementViewModel } from "./capabilityManagementViewModel";

describe("capabilityManagementApi", () => {
  it("fetches the unified capability list from GET /api/v1/capabilities", async () => {
    const fetchImpl = vi.fn(async () => new Response(JSON.stringify({ items: [] }), { status: 200, headers: { "Content-Type": "application/json" } }));

    await fetchCapabilities(fetchImpl);

    expect(fetchImpl).toHaveBeenCalledWith(
      "/api/v1/capabilities",
      expect.objectContaining({ method: "GET", credentials: "include" }),
    );
  });
});

describe("buildCapabilityManagementViewModel", () => {
  it("normalizes capability records and excludes connector management entries from the list", () => {
    const vm = buildCapabilityManagementViewModel({
      items: [
        {
          id: "skill.ops-triage",
          name: "Ops Triage",
          source: "skill",
          connection: { name: "builtin", status: "connected" },
          permissions: ["read_metrics"],
          risks: ["low"],
          runtime: { host: "web", mode: "local" },
          audit: { lastUsedAt: "2026-06-16T08:00:00+08:00", updatedBy: "system" },
        },
        { id: "connector.management", name: "Connector 管理", source: "connector" },
      ],
    });

    expect(vm.items).toHaveLength(1);
    expect(vm.items[0]).toMatchObject({
      id: "skill.ops-triage",
      name: "Ops Triage",
      sourceLabel: "skill",
      connectionSummary: "builtin · connected",
      permissionRiskSummary: "read_metrics / low",
      runtimeSummary: "host: web, mode: local",
      auditSummary: "lastUsedAt: 2026-06-16T08:00:00+08:00, updatedBy: system",
    });
  });
});
