import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AppShellChromeProvider } from "@/app/AppShellChromeContext";
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
  hypotheses: [
    {
      id: "hyp-1",
      title: "DB saturation",
      summary: "Connection pool is exhausted",
    },
  ],
  evidence: [
    {
      id: "ev-1",
      evidence_ref: "ev-coroot-latency",
      artifact_id: "coroot-checkout-latency-chart",
      title: "Coroot latency",
      summary: "p95 above threshold",
      source: "coroot",
    },
  ],
  host_profile_snapshots: [
    {
      host_id: "host-web-1",
      display_name: "web-1",
      labels: { env: "prod", role: "web" },
    },
  ],
  host_leases: [
    {
      lease_id: "lease-1",
      host_id: "host-web-1",
      status: "acquired",
      expires_at: "2026-05-12T10:00:00+08:00",
    },
  ],
  workflow_runs: [
    {
      run_id: "run-1",
      workflow_id: "wf-checkout-fix",
      status: "succeeded",
      verification_refs: ["verify-1"],
    },
  ],
  verifications: [
    {
      id: "verify-1",
      title: "Latency recovered",
      status: "passed",
      summary: "p95 back to baseline",
    },
  ],
  experience_candidates: [
    {
      pack_id: "pack-checkout-lock",
      title: "Checkout lock wait pack",
      status: "candidate",
    },
  ],
  postmortem: { summary: "Draft RCA" },
  pendingApprovals: [
    {
      id: "approval-1",
      command: "kubectl rollout restart deployment/checkout",
      decision: "pending",
    },
  ],
};

const slowButtonIncidentPayload = {
  id: "case-slow-button",
  title: "页面按钮很慢",
  status: "waiting_confirmation",
  severity: "medium",
  source: "debug_mode",
  environment: "prod",
  businessCapability: "checkout",
  entityId: "svc-checkout",
  host_profile_snapshots: [{ host_id: "host-web-1", display_name: "web-1" }],
  pendingActions: [
    {
      actionId: "confirm-slow-button",
      title: "确认慢按钮修复",
      status: "pending",
    },
  ],
  debug_event: {
    debug_event_id: "debug-secret-1",
    trace_id: "trace-slow-button",
    request_body: "card-number=4111111111111111",
    headers: {
      cookie: "sid=secret-debug-cookie",
      authorization: "Bearer secret-debug-authorization",
    },
    user_input: "用户输入原文 secret-debug-user-input",
  },
  token: "secret-debug-token",
  password: "secret-debug-password",
};

const pgFixIncidentPayload = {
  id: "case-pg-fix",
  title: "PG 锁冲突修复",
  status: "waiting_confirmation",
  severity: "high",
  source: "manual",
  environment: "prod",
  businessCapability: "database",
  entityId: "pg-primary",
  host_profile_snapshots: [{ host_id: "db-pg-1", display_name: "db-pg-1" }],
  host_leases: [
    { lease_id: "lease-pg-conflict", host_id: "db-pg-1", status: "conflict" },
  ],
  pendingActions: [
    { actionId: "confirm-pg-fix", title: "确认 PG 修复", status: "pending" },
  ],
  pendingApprovals: [
    {
      id: "approval-pg-conflict",
      command: "runner workflow run pg-pool-fix",
      decision: "pending",
    },
  ],
};

const corootWebhookIncidentPayload = {
  id: "case-coroot-webhook",
  title: "order-api SLO burn",
  status: "open",
  severity: "critical",
  source: "coroot",
  environment: "prod",
  affectedServices: ["order-api"],
  evidenceRefs: ["ev-coroot-webhook"],
  evidence: [
    {
      id: "ev-coroot-webhook",
      source: "coroot",
      rawRef: "coroot:webhook:abc123",
      summary: "Coroot alert · order-api SLO burn · service=order-api",
      confidence: "high",
      entityId: "order-api",
    },
  ],
};

const runbookPayload = {
  id: "checkout-restart",
  title: "Checkout safe restart",
  risk: "medium",
  scope: "prod",
  capabilities: ["checkout"],
  steps: [{ id: "step-1", title: "Drain traffic" }],
  verifications: [{ id: "verify-1", title: "Check p95" }],
  proposals: [
    {
      id: "proposal-1",
      title: "Restart checkout",
      command: "kubectl rollout restart deployment/checkout",
    },
  ],
};

const mcpPayload = {
  configPath: "mcp-servers.json",
  items: [
    {
      name: "docs",
      transport: "http",
      url: "http://127.0.0.1:9000/mcp",
      status: "connected",
      toolCount: 3,
      resourceCount: 2,
    },
  ],
};

const auditsPayload = {
  items: [
    {
      id: "audit-1",
      createdAt: "2026-05-07T00:00:00Z",
      host: "host-prod-07",
      toolName: "shell",
      decision: "pending",
      command: "free -h",
    },
  ],
  stats: { todayTotal: 1, pending: 1, autoAccepted: 0, grantedCommands: 1 },
};

const grantsPayload = {
  items: [
    {
      id: "grant-1",
      hostId: "host-prod-07",
      command: "systemctl status nginx",
      status: "active",
    },
  ],
};

const opsGraphLookupPayload = {
  matches: [
    {
      id: "pg-primary",
      name: "PG 主库",
      type: "middleware",
      status: "warning",
    },
  ],
};

const opsGraphNeighborhoodPayload = {
  entity: {
    id: "pg-primary",
    name: "PG 主库",
    type: "middleware_cluster",
    status: "warning",
    health: "连接池耗尽",
    members: [
      { id: "pg-0", name: "pg-0", role: "primary", status: "warning" },
      { id: "pg-1", name: "pg-1", role: "replica", status: "healthy" },
    ],
    related_experience_packs: [
      { id: "manual-pg-pool", name: "PG 连接池修复运维手册" },
    ],
  },
  neighbors: [
    {
      id: "pg-0",
      name: "pg-0",
      type: "middleware_instance",
      relation: "contains",
      status: "warning",
    },
    {
      id: "svc-checkout",
      name: "Checkout 服务",
      type: "service",
      relation: "writes_to",
      status: "warning",
    },
    {
      id: "host-db-01",
      name: "db-01",
      type: "host",
      relation: "runs_on",
      status: "online",
      host_profile: {
        host_id: "host-db-01",
        display_name: "db-01",
        os: "Linux",
        arch: "x86_64",
        agent_version: "1.8.4",
        labels: { env: "prod", role: "db" },
      },
      host_lease: {
        lease_id: "lease-db-01",
        status: "conflict",
        mission_id: "case-pg-fix",
        expires_at: "2026-05-12T10:00:00+08:00",
      },
    },
  ],
  entities: [],
  relationships: [],
  depth: 2,
};

const opsGraphImpactPayload = {
  capabilities: [{ name: "checkout", impact: "写入排队" }],
  tenants: [{ name: "tenant-a", impact: "部分受影响" }],
};

function jsonResponse(payload: unknown) {
  return Promise.resolve(
    new Response(JSON.stringify(payload), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }),
  );
}

function mockFetch(input: RequestInfo | URL, init?: RequestInit) {
  const url = String(input);
  if (url.includes("/api/v1/incidents/case-coroot-webhook/start-chat")) {
    return jsonResponse({
      sessionId: "sess-coroot-webhook",
      turnId: "turn-coroot-webhook",
      startedRuntimeTurn: true,
    });
  }
  if (url.includes("/api/v1/incidents/case-coroot-webhook"))
    return jsonResponse(corootWebhookIncidentPayload);
  if (url.includes("/api/v1/incidents/incident-1"))
    return jsonResponse(incidentPayload);
  if (url.includes("/api/v1/incidents/case-slow-button"))
    return jsonResponse(slowButtonIncidentPayload);
  if (url.includes("/api/v1/incidents/case-pg-fix"))
    return jsonResponse(pgFixIncidentPayload);
  if (url.includes("/api/v1/incidents")) {
    const params = new URL(url, "http://localhost").searchParams;
    if (params.get("lock_conflict") === "true")
      return jsonResponse({ items: [pgFixIncidentPayload] });
    return jsonResponse({
      items: [incidentPayload, slowButtonIncidentPayload, pgFixIncidentPayload],
    });
  }
  if (url.includes("/api/v1/opsgraph/lookup"))
    return jsonResponse(opsGraphLookupPayload);
  if (url.includes("/api/v1/opsgraph/graphs/graph.default"))
    return jsonResponse({
      graph: { id: "graph.default", name: "默认图谱", nodes: [], edges: [] },
    });
  if (
    url.includes("/api/v1/opsgraph/entities/") &&
    url.includes("/business-impact")
  )
    return jsonResponse(opsGraphImpactPayload);
  if (url.includes("/api/v1/opsgraph/entities/"))
    return jsonResponse(opsGraphNeighborhoodPayload);
  if (url.includes("/api/v1/runbooks/match"))
    return jsonResponse({
      items: [{ id: "match-1", title: "Checkout safe restart", score: 0.92 }],
    });
  if (url.includes("/api/v1/runbooks/instances"))
    return jsonResponse({ items: [{ id: "inst-1", status: "pending" }] });
  if (url.includes("/api/v1/runbooks/checkout-restart"))
    return jsonResponse(runbookPayload);
  if (url.includes("/api/v1/runbooks"))
    return jsonResponse({ items: [runbookPayload] });
  if (url.includes("/api/v1/mcp/servers")) return jsonResponse(mcpPayload);
  if (url.includes("/api/v1/approval-audits"))
    return jsonResponse(auditsPayload);
  if (url.includes("/api/v1/approval-grants"))
    return jsonResponse(grantsPayload);
  if (url.includes("/api/v1/approvals/approval-1/decision"))
    return jsonResponse({ ok: true });
  if (url.includes("/api/v1/state")) return jsonResponse({});
  return jsonResponse({});
}

async function flush() {
  await act(async () => {
    for (let index = 0; index < 5; index += 1) await Promise.resolve();
  });
}

function changeSelect(select: HTMLSelectElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(
    HTMLSelectElement.prototype,
    "value",
  )?.set;
  setter?.call(select, value);
  select.dispatchEvent(new Event("change", { bubbles: true }));
}

function changeInput(input: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(
    HTMLInputElement.prototype,
    "value",
  )?.set;
  setter?.call(input, value);
  input.dispatchEvent(new Event("input", { bubbles: true }));
}

describe("React complex migration pages", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (
      globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
    ).IS_REACT_ACT_ENVIRONMENT = true;
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
      root.render(
        <AppShellChromeProvider>
          <MemoryRouter initialEntries={[path]}>
            <AppRouter />
          </MemoryRouter>
        </AppShellChromeProvider>,
      );
    });
    await flush();
  }

  it.each([
    ["/incidents", "Checkout latency spike"],
    ["/incidents/incident-1", "DB saturation"],
    ["/runbooks", "Runbook 已不作为当前方案主概念，请使用 Runner Workflow"],
    [
      "/runbooks/checkout-restart",
      "Runbook 已不作为当前方案主概念，请使用 Runner Workflow",
    ],
    ["/mcp", "docs"],
    ["/approval-management", "审批流水"],
  ])("renders migrated complex route %s", async (path, expected) => {
    await render(path);
    expect(container.textContent).toContain(expected);
    expect(container.textContent).not.toContain("Migration Placeholder");
  });

  it("moves complex page actions into the app shell header", async () => {
    await render("/incidents");

    const shellHeader = container.querySelector(
      '[data-testid="app-shell-header"]',
    );
    expect(shellHeader?.textContent).toContain("Case 工作台");
    expect(shellHeader?.textContent).not.toContain("刷新");
    expect(
      container.querySelector("main > div header")?.textContent || "",
    ).not.toContain("展示 Debug Mode");

    act(() => root.unmount());
    container.innerHTML = "";
    root = createRoot(container);
    await render("/mcp");
    const mcpHeader = container.querySelector(
      '[data-testid="app-shell-header"]',
    );
    expect(mcpHeader?.textContent).toContain("MCP Servers");
    expect(mcpHeader?.textContent).toContain("刷新全部");
    expect(mcpHeader?.textContent).toContain("添加 MCP");
    expect(
      container.querySelector("main > div header")?.textContent || "",
    ).not.toContain("MCP runtime server");
  });

  it.each(["/runbooks", "/runbooks/checkout-restart"])(
    "keeps %s as a Runner Workflow compatibility route",
    async (path) => {
      await render(path);

      expect(container.textContent).toContain(
        "Runbook 已不作为当前方案主概念，请使用 Runner Workflow",
      );
      const runnerLinks = Array.from(
        container.querySelectorAll('a[href="/runner"]'),
      );
      expect(
        runnerLinks.some((link) =>
          link.textContent?.includes("前往 Runner Workflow"),
        ),
      ).toBe(true);
      const runbookApiCalls = vi
        .mocked(globalThis.fetch)
        .mock.calls.filter(([input]) =>
          String(input).startsWith("/api/v1/runbooks"),
        );
      expect(runbookApiCalls).toHaveLength(0);
    },
  );

  it("submits incident approval decisions through existing approvals API", async () => {
    await render("/incidents/incident-1");
    const approve = Array.from(container.querySelectorAll("button")).find(
      (button) => button.textContent?.includes("批准"),
    );
    expect(approve).toBeTruthy();
    await act(async () =>
      approve?.dispatchEvent(new MouseEvent("click", { bubbles: true })),
    );
    await flush();
    expect(globalThis.fetch).toHaveBeenCalledWith(
      "/api/v1/approvals/approval-1/decision",
      expect.objectContaining({ method: "POST" }),
    );
  });

  it("renders Case workbench closure context from a legacy incident payload", async () => {
    await render("/incidents/incident-1");

    expect(container.textContent).toContain("Case 工作台");
    expect(container.textContent).toContain("ev-coroot-latency");
    expect(container.textContent).toContain("web-1");
    expect(container.textContent).toContain("lease-1");
    expect(container.textContent).toContain("wf-checkout-fix");
    expect(container.textContent).toContain("Latency recovered");
    expect(container.textContent).toContain("Checkout lock wait pack");
    expect(
      container.querySelector('a[href="/debug/prompts?case_id=incident-1"]')
        ?.textContent,
    ).toContain("Prompt Trace");
  });

  it("applies Case list filters for status, source, environment, host and blockers", async () => {
    await render("/incidents");

    const listCard = container.querySelector('[data-testid="case-list-card"]');
    const filtersPanel = container.querySelector(
      '[data-testid="case-list-filters"]',
    );
    expect(container.textContent).not.toContain("Case 筛选");
    expect(listCard).toBeTruthy();
    expect(filtersPanel).toBeTruthy();
    expect(listCard?.contains(filtersPanel)).toBe(true);
    expect(container.textContent).toContain("页面按钮很慢");
    expect(
      container.querySelector('a[href="/incidents/case-slow-button"]')
        ?.textContent,
    ).toContain("页面按钮很慢");
    expect(
      container.querySelector('a[href="/incidents/case-pg-fix"]')?.textContent,
    ).toContain("PG 锁冲突修复");

    const status = container.querySelector(
      '[aria-label="状态筛选"]',
    ) as HTMLSelectElement | null;
    const source = container.querySelector(
      '[aria-label="来源筛选"]',
    ) as HTMLSelectElement | null;
    const environment = container.querySelector(
      '[aria-label="环境筛选"]',
    ) as HTMLSelectElement | null;
    const host = container.querySelector(
      '[aria-label="主机筛选"]',
    ) as HTMLInputElement | null;
    const waitingConfirmation = container.querySelector(
      '[aria-label="待确认筛选"]',
    ) as HTMLSelectElement | null;
    const lockConflict = container.querySelector(
      '[aria-label="锁冲突筛选"]',
    ) as HTMLSelectElement | null;

    expect(status).toBeTruthy();
    expect(source).toBeTruthy();
    expect(environment).toBeTruthy();
    expect(host).toBeTruthy();
    expect(waitingConfirmation).toBeTruthy();
    expect(lockConflict).toBeTruthy();

    await act(async () => {
      changeSelect(status!, "waiting_confirmation");
      changeSelect(source!, "manual");
      changeSelect(environment!, "prod");
      changeInput(host!, "db-pg-1");
      changeSelect(waitingConfirmation!, "true");
      changeSelect(lockConflict!, "true");
    });

    const apply = Array.from(container.querySelectorAll("button")).find(
      (button) => button.textContent?.includes("应用筛选"),
    );
    expect(apply).toBeTruthy();
    await act(async () =>
      apply?.dispatchEvent(new MouseEvent("click", { bubbles: true })),
    );
    await flush();

    expect(globalThis.fetch).toHaveBeenCalledWith(
      "/api/v1/incidents?status=waiting_confirmation&source=manual&environment=prod&host_id=db-pg-1&waiting_confirmation=true&lock_conflict=true",
      expect.objectContaining({ credentials: "include" }),
    );
    expect(container.textContent).toContain("PG 锁冲突修复");
    expect(container.textContent).not.toContain("Runbook");
  });

  it("explains how Cases and OpsGraph relationships are created when data is empty", async () => {
    vi.mocked(globalThis.fetch).mockImplementation((input, init) => {
      const url = String(input);
      if (
        url.includes("/api/v1/incidents") &&
        !url.includes("/api/v1/incidents/")
      ) {
        return jsonResponse({ items: [] });
      }
      return mockFetch(input, init);
    });

    await render("/incidents");

    expect(container.textContent).toContain("AI 对话中发起修复");
    expect(container.textContent).toContain("Debug Mode");
    expect(container.textContent).toContain("Coroot webhook");

    act(() => root.unmount());
    container.innerHTML = "";
    root = createRoot(container);
    vi.mocked(globalThis.fetch).mockImplementation(mockFetch as typeof fetch);

    await render("/opsgraph");

    const guide = container.querySelector(
      '[data-testid="opsgraph-empty-guide"]',
    );
    expect(guide?.textContent).toContain("这个图谱现在是空的");
    expect(container.textContent).toContain("手工构建");
    expect(container.textContent).not.toContain("Coroot MCP");
    expect(container.textContent).not.toContain("主机上报");
  });

  it("renders a Case header and readable stage tabs on the workbench detail page", async () => {
    await render("/incidents/incident-1");

    const header = container.querySelector('[data-testid="case-header"]');
    const tabs = container.querySelector('[data-testid="case-stage-tabs"]');

    expect(header?.textContent).toContain("Case 总览");
    expect(header?.textContent).toContain("Checkout latency spike");
    expect(header?.textContent).toContain("处理中");
    expect(header?.textContent).toContain("SEV2");
    expect(header?.textContent).toContain("prod");
    expect(header?.textContent).toContain("checkout");
    expect(tabs?.textContent).toContain("概览");
    expect(tabs?.textContent).toContain("证据 1");
    expect(tabs?.textContent).toContain("主机环境 1");
    expect(tabs?.textContent).toContain("执行 1");
    expect(tabs?.textContent).toContain("验证 1");
    expect(tabs?.textContent).toContain("经验 1");
    expect(container.textContent).toContain("租户");
    expect(container.textContent).not.toContain("Capabilities");
  });

  it("renders OpsGraph manual authoring editor without a full CMDB editor", async () => {
    await render("/opsgraph");

    const shellHeader = container.querySelector(
      '[data-testid="app-shell-header"]',
    );
    expect(shellHeader?.textContent).toContain("OpsGraph");
    expect(shellHeader?.textContent).not.toContain("Case 工作台");
    expect(shellHeader?.textContent).not.toContain("主机与租约");

    expect(container.textContent).toContain("这个图谱现在是空的");
    expect(container.textContent).toContain("新建服务");
    expect(container.textContent).toContain("中间件集群");
    expect(container.textContent).toContain("手工构建");
    expect(container.textContent).not.toContain("CMDB 编辑");
  });

  it("does not allow approving workflow execution while HostLease is blocked", async () => {
    await render("/incidents/case-pg-fix");

    expect(container.textContent).toContain("HostLease 阻塞");
    expect(container.textContent).toContain("lease-pg-conflict");
    const approve = Array.from(container.querySelectorAll("button")).find(
      (button) => button.textContent?.includes("批准"),
    );
    const reject = Array.from(container.querySelectorAll("button")).find(
      (button) => button.textContent?.includes("拒绝"),
    );
    expect(approve).toBeTruthy();
    expect(reject).toBeTruthy();
    expect(approve?.disabled).toBe(true);
    expect(approve?.getAttribute("title")).toContain("HostLease");
    expect(reject?.disabled).toBe(false);
  });

  it("shows Coroot webhook raw evidence and starts Chat only from the user action", async () => {
    await render("/incidents/case-coroot-webhook");

    expect(container.textContent).toContain("Coroot");
    expect(container.textContent).toContain(
      "Coroot alert · order-api SLO burn",
    );
    expect(container.textContent).toContain("进入 Chat 排查");
    expect(container.textContent).not.toContain("自动修复");
    expect(globalThis.fetch).not.toHaveBeenCalledWith(
      "/api/v1/incidents/case-coroot-webhook/start-chat",
      expect.anything(),
    );

    const startChat = Array.from(container.querySelectorAll("button")).find(
      (button) => button.textContent?.includes("进入 Chat 排查"),
    );
    expect(startChat).toBeTruthy();
    await act(async () =>
      startChat?.dispatchEvent(new MouseEvent("click", { bubbles: true })),
    );
    await flush();

    expect(globalThis.fetch).toHaveBeenCalledWith(
      "/api/v1/incidents/case-coroot-webhook/start-chat",
      expect.objectContaining({ method: "POST" }),
    );
  });

  it("does not render DebugEvent request body, cookies, tokens, passwords, or original user input", async () => {
    await render("/incidents/case-slow-button");

    expect(container.textContent).toContain("页面按钮很慢");
    expect(container.textContent).toContain("Debug Mode");
    expect(container.innerHTML).not.toMatch(
      /card-number|secret-debug-cookie|secret-debug-authorization|secret-debug-user-input|secret-debug-token|secret-debug-password/i,
    );
  });

  it("runs mcp runtime server actions through mcp API", async () => {
    await render("/mcp");
    const close = Array.from(container.querySelectorAll("button")).find(
      (button) => button.textContent?.includes("关闭"),
    );
    expect(close).toBeTruthy();
    await act(async () =>
      close?.dispatchEvent(new MouseEvent("click", { bubbles: true })),
    );
    await flush();
    expect(globalThis.fetch).toHaveBeenCalledWith(
      "/api/v1/mcp/servers/docs/close",
      expect.objectContaining({ method: "POST" }),
    );
  });
});
