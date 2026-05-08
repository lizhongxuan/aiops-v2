import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AppRouter } from "@/router";

const incidentPayload = {
  id: "incident-1",
  title: "Checkout latency spike",
  status: "active",
  severity: "SEV2",
  environment: "prod",
  businessCapability: "checkout",
  summary: "Checkout latency increased after deploy.",
  entityId: "svc-checkout",
  hypotheses: [{ id: "hyp-1", title: "DB saturation", summary: "Connection pool is exhausted" }],
  evidence: [{ id: "ev-1", title: "Coroot latency", summary: "p95 above threshold", source: "coroot" }],
  postmortem: { summary: "Draft RCA" },
  pendingApprovals: [{ id: "approval-1", command: "kubectl rollout restart deployment/checkout", decision: "pending" }],
};

const runbookPayload = {
  id: "checkout-restart",
  title: "Checkout safe restart",
  risk: "medium",
  scope: "prod",
  capabilities: ["checkout"],
  steps: [{ id: "step-1", title: "Drain traffic" }],
  verifications: [{ id: "verify-1", title: "Check p95" }],
  proposals: [{ id: "proposal-1", title: "Restart checkout", command: "kubectl rollout restart deployment/checkout" }],
};

const mcpPayload = {
  configPath: "mcp-servers.json",
  items: [{ name: "docs", transport: "http", url: "http://127.0.0.1:9000/mcp", status: "connected", toolCount: 3, resourceCount: 2 }],
};

const auditsPayload = {
  items: [{ id: "audit-1", createdAt: "2026-05-07T00:00:00Z", host: "host-prod-07", toolName: "shell", decision: "pending", command: "free -h" }],
  stats: { todayTotal: 1, pending: 1, autoAccepted: 0, grantedCommands: 1 },
};

const grantsPayload = {
  items: [{ id: "grant-1", hostId: "host-prod-07", command: "systemctl status nginx", status: "active" }],
};

function jsonResponse(payload: unknown) {
  return Promise.resolve(new Response(JSON.stringify(payload), { status: 200, headers: { "Content-Type": "application/json" } }));
}

function mockFetch(input: RequestInfo | URL, init?: RequestInit) {
  const url = String(input);
  if (url.includes("/api/v1/incidents/incident-1")) return jsonResponse(incidentPayload);
  if (url.includes("/api/v1/incidents")) return jsonResponse({ items: [incidentPayload] });
  if (url.includes("/api/v1/opsgraph/entities/")) return jsonResponse({ neighbors: [{ id: "db", name: "primary-db", relation: "depends_on" }], capabilities: [{ name: "checkout", impact: "degraded" }], tenants: [{ name: "tenant-a", impact: "partial" }] });
  if (url.includes("/api/v1/runbooks/match")) return jsonResponse({ items: [{ id: "match-1", title: "Checkout safe restart", score: 0.92 }] });
  if (url.includes("/api/v1/runbooks/instances")) return jsonResponse({ items: [{ id: "inst-1", status: "pending" }] });
  if (url.includes("/api/v1/runbooks/checkout-restart")) return jsonResponse(runbookPayload);
  if (url.includes("/api/v1/runbooks")) return jsonResponse({ items: [runbookPayload] });
  if (url.includes("/api/v1/mcp/servers")) return jsonResponse(mcpPayload);
  if (url.includes("/api/v1/approval-audits")) return jsonResponse(auditsPayload);
  if (url.includes("/api/v1/approval-grants")) return jsonResponse(grantsPayload);
  if (url.includes("/api/v1/approvals/approval-1/decision")) return jsonResponse({ ok: true });
  if (url.includes("/api/v1/state")) return jsonResponse({});
  return jsonResponse({});
}

async function flush() {
  await act(async () => {
    for (let index = 0; index < 5; index += 1) await Promise.resolve();
  });
}

describe("React complex migration pages", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
    globalThis.ResizeObserver = class ResizeObserver {
      observe() {}
      unobserve() {}
      disconnect() {}
    };
    vi.spyOn(globalThis, "fetch").mockImplementation(mockFetch as typeof fetch);
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
  });

  async function render(path: string) {
    await act(async () => {
      root.render(<MemoryRouter initialEntries={[path]}><AppRouter /></MemoryRouter>);
    });
    await flush();
  }

  it.each([
    ["/incidents", "Checkout latency spike"],
    ["/incidents/incident-1", "DB saturation"],
    ["/runbooks", "Checkout safe restart"],
    ["/runbooks/checkout-restart", "Drain traffic"],
    ["/mcp", "docs"],
    ["/approval-management", "审批流水"],
  ])("renders migrated complex route %s", async (path, expected) => {
    await render(path);
    expect(container.textContent).toContain(expected);
    expect(container.textContent).not.toContain("Migration Placeholder");
  });

  it("submits incident approval decisions through existing approvals API", async () => {
    await render("/incidents/incident-1");
    const approve = Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("批准"));
    expect(approve).toBeTruthy();
    await act(async () => approve?.dispatchEvent(new MouseEvent("click", { bubbles: true })));
    await flush();
    expect(globalThis.fetch).toHaveBeenCalledWith(
      "/api/v1/approvals/approval-1/decision",
      expect.objectContaining({ method: "POST" }),
    );
  });

  it("runs mcp runtime server actions through mcp API", async () => {
    await render("/mcp");
    const close = Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("关闭"));
    expect(close).toBeTruthy();
    await act(async () => close?.dispatchEvent(new MouseEvent("click", { bubbles: true })));
    await flush();
    expect(globalThis.fetch).toHaveBeenCalledWith(
      "/api/v1/mcp/servers/docs/close",
      expect.objectContaining({ method: "POST" }),
    );
  });
});
