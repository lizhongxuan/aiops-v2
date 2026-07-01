import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AppShellChromeProvider } from "@/app/AppShellChromeContext";
import { AppRouter } from "@/router";
import { shouldUseExperiencePackFixtureFallback } from "./ExperiencePacksPage";

const statePayload = {
  hosts: [{ id: "server-local", name: "server-local", status: "online" }],
};

const hostsPayload = {
  items: [
    {
      id: "host-prod-07",
      name: "web-07",
      address: "10.10.4.27",
      sshUser: "root",
      sshPort: 22,
      transport: "ssh_bootstrap",
      status: "online",
      terminalCapable: true,
      agentVersion: "1.8.4",
      lastHeartbeat: new Date().toISOString(),
      os: "linux",
      arch: "amd64",
      osRelease: "Ubuntu 24.04 LTS",
      kernelVersion: "6.8.0-31-generic",
      cpuCores: 8,
      memoryBytes: 34359738368,
      labels: { env: "prod", role: "web", cluster: "ops-k8s" },
    },
  ],
};

const hostProfilesPayload = {
  items: [
    {
      host_id: "host-prod-07",
      display_name: "web-07",
      status: "online",
      os: "linux",
      arch: "x86_64",
      labels: { env: "prod", role: "web" },
      agent_version: "1.8.4",
      agent_id: "agent-prod-07",
      runtime: {
        os_release: "Ubuntu 24.04",
        kernel: "6.8.0",
        request_body: "secret-business-payload",
      },
      service_runtime: { supervisor: "systemd", unit: "aiops-agent.service" },
      token: "secret-profile-token",
      password: "secret-profile-password",
      private_key: "-----BEGIN PRIVATE KEY-----",
      cookie: "sid=secret-cookie",
      authorization: "Bearer secret-authorization",
      last_heartbeat_at: "2026-05-12T09:20:00+08:00",
      profile_expires_at: "2026-05-12T09:50:00+08:00",
    },
    {
      host_id: "host-db-01",
      display_name: "pg-primary",
      status: "offline",
      os: "linux",
      arch: "x86_64",
      labels: { role: "db" },
      profile_expires_at: "2026-05-12T08:00:00+08:00",
    },
  ],
};

const hostLeasesPayload = {
  items: [
    {
      lease_id: "lease-prod-07",
      host_id: "host-prod-07",
      status: "active",
      mission_id: "case-debug-1",
      owner_session_id: "session-debug-1",
      acquired_at: "2026-05-12T09:10:00+08:00",
      expires_at: "2026-05-12T09:40:00+08:00",
    },
    {
      lease_id: "lease-db-conflict",
      host_id: "host-db-01",
      status: "conflict",
      mission_id: "case-pg-1",
      owner_session_id: "session-pg-1",
      expires_at: "2026-05-12T09:45:00+08:00",
    },
  ],
};

const hostReportHistoryPayload = {
  items: [
    {
      report_id: "report-web-07",
      host_id: "host-prod-07",
      status: "accepted",
      reported_at: "2026-05-12T09:20:00+08:00",
      summary: "CPU 8C / Memory 32GiB / Disk 400GiB",
    },
  ],
};

const experienceCandidatesPayload = {
  items: [
    {
      id: "candidate-pg-pool",
      pack_id: "pack-pg-pool",
      title: "PG 连接池修复候选经验包",
      summary: "从 case-pg-fix 提炼出的连接池耗尽处理经验，等待审核启用。",
      status: "candidate",
      match_reason: "中间件类型、错误模式和 HostProfile 标签一致",
      source_case_id: "case-pg-fix",
      experience_pack: {
        id: "pack-pg-pool",
        title: "PG 连接池修复经验包",
        summary: "诊断连接池耗尽，执行参数调整并验证恢复。",
        version: "v1.0",
        status: "enabled",
        review_status: "approved",
        enabled: true,
        workflow_binding: {
          workflow_id: "wf-pg-pool-fix",
          workflow_name: "PG Pool Fix",
          status: "draft",
          version: "v1",
        },
        retrieval_eval: {
          score: 0.91,
          matched_cases: 4,
          verdict: "pass",
          last_evaluated_at: "2026-05-12T09:30:00+08:00",
        },
        authorization_scopes: [
          {
            type: "environment",
            value: "prod",
            searchable: true,
            reason: "生产 PG 集群",
          },
        ],
      },
    },
    {
      id: "candidate-java-heap",
      pack_id: "pack-java-heap",
      title: "Java 堆内存排障经验包",
      summary: "已启用但还没有配置可检索范围。",
      status: "enabled",
      source_case_id: "case-java-oom",
      experience_pack: {
        id: "pack-java-heap",
        title: "Java 堆内存排障经验包",
        summary: "线程 dump 与堆转储排查流程。",
        version: "v2.1",
        status: "enabled",
        review_status: "approved",
        enabled: true,
        workflow_binding: {
          workflow_id: "wf-java-heap",
          workflow_name: "Java Heap RCA",
          status: "bound",
        },
        retrieval_eval: { score: 0.77, matched_cases: 2, verdict: "warn" },
        authorization_scopes: [],
      },
    },
  ],
};

const experienceReusePayload = {
  items: [
    {
      id: "reuse-pg-1",
      pack_id: "pack-pg-pool",
      case_id: "case-pg-repeat",
      result: "failed_rollback",
      summary: "连接池调整失败，已执行回滚并记录失败点。",
      reused_by: "主 Agent",
      reused_at: "2026-05-12T10:00:00+08:00",
    },
  ],
};

const opsManualsPayload = {
  items: [
    {
      id: "manual-redis-memory",
      title: "Redis 内存压力排障",
      status: "verified",
      workflow_ref: { workflow_id: "workflow-redis-memory" },
      operation: { target_type: "redis", action: "rca_or_repair" },
      applicability: {
        middleware: "redis",
        os: ["ubuntu"],
        platform: ["vm"],
        execution_surface: ["ssh"],
      },
      required_context: { required_inputs: ["target_instance"] },
      preconditions: ["确认目标实例"],
      validation: ["指标恢复"],
      cannot_use_when: ["无法确认实例"],
      run_record_summary: {
        success_count: 3,
        failure_count: 1,
        recent_result: "success",
      },
    },
  ],
};

const opsManualCandidatesPayload = {
  items: [
    {
      id: "candidate-redis-memory",
      review_status: "pending",
      source_type: "workflow",
      proposed_manual: {
        id: "manual-redis-memory-draft",
        title: "Redis 内存压力候选",
        status: "draft",
        workflow_ref: { workflow_id: "workflow-redis-memory" },
        operation: { target_type: "redis", action: "rca_or_repair" },
      },
    },
  ],
};

const opsManualRunRecordsPayload = {
  items: [
    {
      id: "run-redis-1",
      manual_id: "manual-redis-memory",
      workflow_id: "workflow-redis-memory",
      execution_status: "success",
      validation_status: "passed",
    },
  ],
};

const llmPayload = {
  provider: "openai",
  model: "gpt-5.4",
  apiKeySet: true,
  apiKeyMasked: "sk-***",
  baseURL: "https://api.openai.com/v1",
  maxContextTokens: 131072,
  maxOutputTokens: 20000,
  requestTimeoutMs: 25000,
  reasoningEffort: "high",
  bifrostActive: true,
};

const runtimeSettingsPayload = {
  settings: {
    agentRuntime: {
      intentFrameRouting: "trace_only",
      diagnosticProtocol: true,
    },
    tooling: {
      readOnlyRetryEnabled: false,
      readOnlyRetryMaxPerCall: 1,
      readOnlyRetryMaxPerTurn: 3,
      readOnlyRetryBackoffBaseMs: 300,
      readOnlyRetryBackoffMaxMs: 2000,
    },
    workflow: {
      referenceGuardMode: "enforce",
      validationProvider: "static",
      validationImage: "python:3.12-slim",
    },
    opsManual: { autoRetrieval: false },
    debug: {
      modelInputTrace: true,
      finalState: false,
      transportProjection: false,
      transcriptProjection: false,
    },
    publicWeb: { enabled: true },
  },
  defaults: {},
  restartRequiredKeys: [],
};

const skillPayload = {
  items: [
    {
      id: "ops-triage",
      name: "Ops Triage",
      description: "Triage incidents",
      source: "builtin",
      defaultEnabled: true,
      defaultActivationMode: "default_enabled",
    },
  ],
};

const mcpPayload = {
  items: [
    {
      id: "metrics",
      name: "Metrics MCP",
      type: "http",
      source: "builtin",
      defaultEnabled: true,
      permission: "readonly",
    },
  ],
};

const profilesPayload = {
  items: [
    {
      id: "main-agent",
      name: "Main Agent",
      description: "Main runtime profile",
      systemPrompt: { content: "You are the main AIOps agent." },
      runtime: {
        model: "gpt-5.4",
        approvalMode: "on-request",
        sandboxMode: "workspace-write",
      },
      skills: skillPayload.items,
      mcps: mcpPayload.items,
    },
  ],
  skillCatalog: skillPayload.items,
  mcpCatalog: mcpPayload.items,
};

const agentProfilePreviewPayload = {
  profileId: "main-agent",
  capabilitySnapshot: {
    fingerprint: "sha256:preview",
    items: [
      {
        id: "coroot",
        kind: "mcp_server",
        enabled: true,
        source: "profile",
        sourceScope: "user",
        reason: "enabled by profile sre",
        runtimeStatus: "connected",
        risk: "low",
      },
      {
        id: "dangerous-shell",
        kind: "skill",
        enabled: false,
        source: "admin_policy",
        sourceScope: "managed",
        reason: "disabled by admin deny",
        risk: "high",
      },
      {
        id: "plugin-shell",
        kind: "mcp_server",
        enabled: false,
        source: "plugin:ops",
        sourceScope: "plugin",
        reason: "pending explicit approval",
        runtimeStatus: "pending_approval",
        risk: "high",
      },
    ],
  },
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
  if (url.endsWith("/api/v1/state")) return jsonResponse(statePayload);
  if (url.includes("/api/v1/host-profiles/") && url.endsWith("/report-history"))
    return jsonResponse(hostReportHistoryPayload);
  if (url.includes("/api/v1/host-profiles"))
    return jsonResponse(hostProfilesPayload);
  if (url.includes("/api/v1/host-leases"))
    return jsonResponse(hostLeasesPayload);
  if (
    url.includes("/api/v1/experience-packs/") &&
    url.includes("/reuse-records")
  )
    return jsonResponse(experienceReusePayload);
  if (url.includes("/api/v1/experience-packs/candidates"))
    return jsonResponse(experienceCandidatesPayload);
  if (
    url.includes("/api/v1/experience-packs/") &&
    url.includes("/authorization-scopes")
  )
    return jsonResponse({
      pack: experienceCandidatesPayload.items[0].experience_pack,
    });
  if (url.includes("/api/v1/experience-packs/") && url.includes("/enabled"))
    return jsonResponse({
      pack: experienceCandidatesPayload.items[0].experience_pack,
    });
  if (url.includes("/api/v1/ops-manuals/candidates"))
    return jsonResponse(opsManualCandidatesPayload);
  if (url.includes("/api/v1/ops-manuals/run-records"))
    return jsonResponse(opsManualRunRecordsPayload);
  if (url.includes("/api/v1/ops-manuals"))
    return jsonResponse(opsManualsPayload);
  if (url.endsWith("/api/v1/hosts")) {
    if (init?.method === "POST") return jsonResponse({ ok: true });
    return jsonResponse(hostsPayload);
  }
  if (url.endsWith("/api/v1/sessions"))
    return jsonResponse({
      activeSessionId: "sess-1",
      sessions: [
        {
          id: "sess-1",
          kind: "single_host",
          title: "Nginx chat",
          selectedHostId: "host-prod-07",
        },
      ],
    });
  if (url.endsWith("/api/v1/terminal/sessions"))
    return jsonResponse({ items: [{ id: "term-1", status: "running" }] });
  if (url.endsWith("/api/v1/llm-config"))
    return jsonResponse(
      init?.method === "PUT" ? { ok: true, message: "saved" } : llmPayload,
    );
  if (url.endsWith("/api/v1/runtime-settings"))
    return jsonResponse(
      init?.method === "PATCH"
        ? runtimeSettingsPayload
        : runtimeSettingsPayload,
    );
  if (url.endsWith("/api/v1/agent-skills")) return jsonResponse(skillPayload);
  if (url.includes("/api/v1/agent-skills/"))
    return jsonResponse({
      items: skillPayload.items,
      item: skillPayload.items[0],
    });
  if (url.endsWith("/api/v1/agent-mcps")) return jsonResponse(mcpPayload);
  if (url.includes("/api/v1/agent-mcps/"))
    return jsonResponse({ items: mcpPayload.items, item: mcpPayload.items[0] });
  if (url.endsWith("/api/v1/agent-profiles"))
    return jsonResponse(profilesPayload);
  if (url.includes("/api/v1/agent-profile/preview"))
    return jsonResponse(agentProfilePreviewPayload);
  if (url.endsWith("/api/v1/agent-profile"))
    return jsonResponse(
      init?.method === "PUT"
        ? profilesPayload.items[0]
        : profilesPayload.items[0],
    );
  if (url.endsWith("/api/v1/agent-profile/reset"))
    return jsonResponse(profilesPayload.items[0]);
  if (url.endsWith("/api/v1/agent-profiles/export"))
    return jsonResponse(profilesPayload);
  if (url.endsWith("/api/v1/agent-profiles/import"))
    return jsonResponse(profilesPayload);
  return jsonResponse({});
}

async function flush() {
  await act(async () => {
    for (let index = 0; index < 5; index += 1) {
      await Promise.resolve();
    }
  });
}

function requestBodyFromCall(call: unknown[]) {
  const init = call[1] as RequestInit | undefined;
  return JSON.parse(String(init?.body || "{}")) as Record<string, unknown>;
}

function setInputValue(input: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(
    HTMLInputElement.prototype,
    "value",
  )?.set;
  setter?.call(input, value);
  input.dispatchEvent(new Event("input", { bubbles: true }));
}

function setSelectValue(select: HTMLSelectElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(
    HTMLSelectElement.prototype,
    "value",
  )?.set;
  setter?.call(select, value);
  select.dispatchEvent(new Event("change", { bubbles: true }));
}

function inputInField(root: Element | null | undefined, labelText: string) {
  const label = Array.from(root?.querySelectorAll("label") || []).find((item) =>
    item.textContent?.includes(labelText),
  );
  return label?.querySelector("input") as HTMLInputElement | null;
}

describe("React settings pages", () => {
  let container: HTMLDivElement;
  let root: Root;

  async function renderPath(path: string) {
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

  async function remountPath(path: string) {
    act(() => {
      root.unmount();
    });
    container.innerHTML = "";
    root = createRoot(container);
    await renderPath(path);
  }

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
    act(() => {
      root.unmount();
    });
    container.remove();
  });

  it.each([
    ["/settings", "设置"],
    ["/settings/llm", "LLM 配置"],
    ["/settings/runtime", "Agent Runtime"],
    ["/settings/hosts", "env=prod"],
    ["/settings/ops-manuals", "Redis 内存压力排障"],
    ["/settings/experience-packs", "旧入口已迁移到运维手册"],
    ["/settings/agent", "Agent Profile"],
    ["/capabilities", "集中查看 Skills、MCP Servers 与 Capability Bindings。"],
  ])("renders migrated settings route %s", async (path, expectedText) => {
    await renderPath(path);

    expect(container.textContent).toContain(expectedText);
    expect(container.textContent).not.toContain("Migration Placeholder");
  });

  it("keeps only unified capability management on the settings landing page", async () => {
    await renderPath("/settings");

    expect(container.textContent).toContain("能力管理");
    expect(container.textContent).toContain("运行时配置");
    expect(container.textContent).toContain(
      "Agent、工具、Workflow、Debug 的运行期参数",
    );
    expect(container.querySelector('a[href="/settings/runtime"]')).toBeTruthy();
    expect(container.querySelector('a[href="/capabilities"]')).toBeTruthy();
    expect(container.textContent).not.toContain("Skills");
    expect(container.textContent).not.toContain("MCP Catalog");
    expect(container.textContent).not.toContain("Capability Center");
    expect(container.querySelector('a[href="/settings/skills"]')).toBeNull();
    expect(container.querySelector('a[href="/settings/mcp"]')).toBeNull();
    expect(container.querySelector('a[href="/capability-center"]')).toBeNull();
  });

  it("renders Agent Profile Effective Capabilities preview with source and disabled reasons", async () => {
    await renderPath("/settings/agent");

    expect(container.textContent).toContain("Effective Capabilities");
    expect(container.textContent).toContain("sha256:preview");
    expect(container.textContent).toContain("coroot");
    expect(container.textContent).toContain("connected");
    expect(container.textContent).toContain("disabled by admin deny");
    expect(container.textContent).toContain("pending explicit approval");

    const enabledList = container.querySelector(
      '[data-testid="effective-capabilities-enabled"]',
    );
    expect(enabledList?.textContent).toContain("coroot");
    expect(enabledList?.textContent).not.toContain("dangerous-shell");
    expect(enabledList?.textContent).not.toContain("plugin-shell");
  });

  it("moves settings page actions into the app shell header", async () => {
    await renderPath("/settings/hosts");

    const hostsHeader = container.querySelector(
      '[data-testid="app-shell-header"]',
    );
    expect(hostsHeader?.textContent).toContain("主机列表");
    expect(hostsHeader?.textContent).not.toContain("刷新");
    expect(hostsHeader?.textContent).toContain("接入主机");
    expect(
      container.querySelector("main > div header")?.textContent || "",
    ).not.toContain("HostLease 锁状态");

    await remountPath("/settings/ops-manuals");
    const packsHeader = container.querySelector(
      '[data-testid="app-shell-header"]',
    );
    expect(packsHeader?.textContent).toContain("运维手册");
    expect(packsHeader?.textContent).not.toContain("刷新");
    expect(
      container.querySelector("main > div header")?.textContent || "",
    ).not.toContain("经验包入口已迁移");
  });

  it("renders only the host list with client-reported system basics", async () => {
    await renderPath("/settings/hosts");

    expect(container.textContent).toContain("主机列表");
    expect(container.textContent).toContain("主机 IP / 用户名");
    expect(container.textContent).toContain("10.10.4.27 / root");
    expect(container.textContent).not.toContain(
      "手动 1.8.4 · key 10.10.4.27:root",
    );
    expect(container.textContent).toContain("Ubuntu 24.04 LTS");
    expect(container.textContent).toContain("6.8.0-31-generic");
    expect(container.textContent).toContain("8 核");
    expect(container.textContent).toContain("32 GiB");
    expect(container.textContent).toContain("env=prod");
    expect(container.textContent).not.toContain("主机画像");
    expect(container.textContent).not.toContain("主机租约");
    expect(container.textContent).not.toContain("上报历史");
    expect(container.textContent).not.toContain("执行风险");
    expect(container.textContent).not.toContain("接入配置");
    expect(container.textContent).not.toContain("当前 HostLease");
    expect(container.textContent).not.toContain("暂无主机画像");
    expect(container.textContent).not.toMatch(
      /secret-profile-token|secret-profile-password|PRIVATE KEY|secret-cookie|secret-authorization|secret-business-payload/i,
    );
  });

  it("explains stale host heartbeat with the last reported timestamp", async () => {
    vi.mocked(globalThis.fetch).mockImplementation((input, init) => {
      const url = String(input);
      if (url.endsWith("/api/v1/hosts")) {
        return jsonResponse({
          items: [
            {
              ...hostsPayload.items[0],
              status: "online",
              agentStatus: "stale",
              runtimeReachability: "agent_stale",
              lastHeartbeat: "2026-06-14T16:33:10Z",
            },
          ],
        });
      }
      return mockFetch(input, init);
    });

    await renderPath("/settings/hosts");

    expect(container.textContent).toContain("超时");
    expect(container.textContent).toContain("最后心跳 2026-06-14 16:33 UTC");
    expect(container.textContent).not.toContain("Agent 未继续上报");
  });

  it("keeps host install step internals out of the heartbeat cell", async () => {
    vi.mocked(globalThis.fetch).mockImplementation((input, init) => {
      const url = String(input);
      if (url.endsWith("/api/v1/hosts")) {
        return jsonResponse({
          items: [
            {
              ...hostsPayload.items[0],
              status: "install_failed",
              installState: "failed",
              installStep: "start-service",
              installRunId: "direct-host-a-dc250ee27a30ef57",
              installWorkflowId: "wf-direct-host-a",
              lastError: "host-agent start failed",
            },
          ],
        });
      }
      return mockFetch(input, init);
    });

    await renderPath("/settings/hosts");

    const rows = Array.from(container.querySelectorAll("tbody tr"));
    const hostRow = rows.find((row) => row.textContent?.includes("10.10.4.27"));
    const heartbeatCell = hostRow?.querySelectorAll("td")[1];
    expect(heartbeatCell?.textContent).toContain("安装失败");
    expect(heartbeatCell?.textContent).toContain("查看错误");
    expect(heartbeatCell?.textContent).not.toContain("start-service");
    expect(heartbeatCell?.textContent).not.toContain(
      "direct-host-a-dc250ee27a30ef57",
    );
  });

  it("keeps long host errors out of the heartbeat cell and opens them in a scrollable dialog", async () => {
    const longHostError =
      'host-agent heartbeat: Post "http://172.18.13.11:8001/api/v1/host-agents/heartbeat": dial tcp 172.18.13.11:8001: connect: connection refused; rpc error: code = Unavailable desc = closing transport due to connection error: error reading from server: EOF';
    vi.mocked(globalThis.fetch).mockImplementation((input, init) => {
      const url = String(input);
      if (url.endsWith("/api/v1/hosts")) {
        return jsonResponse({
          items: [
            {
              ...hostsPayload.items[0],
              status: "stale",
              agentStatus: "stale",
              lastError: longHostError,
              lastHeartbeat: "2026-06-14T16:33:10Z",
            },
          ],
        });
      }
      return mockFetch(input, init);
    });

    await renderPath("/settings/hosts");

    const rows = Array.from(container.querySelectorAll("tbody tr"));
    const hostRow = rows.find((row) => row.textContent?.includes("10.10.4.27"));
    const heartbeatCell = hostRow?.querySelectorAll("td")[1];
    expect(heartbeatCell?.textContent).toContain("超时");
    expect(heartbeatCell?.textContent).toContain("查看错误");
    expect(heartbeatCell?.textContent).not.toContain(longHostError);
    expect(container.textContent).not.toContain(longHostError);

    const errorButton = Array.from(
      heartbeatCell?.querySelectorAll("button") || [],
    ).find((button) => button.textContent?.includes("查看错误"));
    expect(errorButton).toBeTruthy();
    await act(async () => errorButton?.click());
    await flush();

    const dialog = document.body.querySelector(
      '[data-testid="host-error-dialog"]',
    );
    expect(dialog?.textContent).toContain("主机错误详情");
    expect(dialog?.textContent).toContain(longHostError);
    expect(dialog?.className).toContain("max-h");
    expect(dialog?.className).toContain("overflow-hidden");
    expect(
      dialog?.querySelector('[data-testid="host-error-dialog-scroll"]')
      ?.className,
    ).toContain("overflow-y-auto");
  });

  it("opens an in-app confirmation dialog when deleting a host", async () => {
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(false);

    await renderPath("/settings/hosts");

    const deleteButton = container.querySelector(
      'button[aria-label="删除主机 host-prod-07"]',
    );
    expect(deleteButton).toBeTruthy();

    await act(async () => deleteButton?.click());
    await flush();

    expect(confirmSpy).not.toHaveBeenCalled();
    const dialog = document.body.querySelector('[data-slot="dialog-content"]');
    expect(dialog?.textContent).toContain("确认删除");
    expect(dialog?.textContent).toContain("确认删除主机 host-prod-07？");
    expect(dialog?.textContent).toContain("取消");
    expect(dialog?.textContent).toContain("删除");
  });

  it("shows host access save errors inside the host dialog layer", async () => {
    vi.mocked(globalThis.fetch).mockImplementation((input, init) => {
      const url = String(input);
      if (url.includes("/api/v1/hosts/") && init?.method === "PUT") {
        return Promise.resolve(
          new Response(
            JSON.stringify({ error: "ssh credential ref is required" }),
            {
              status: 400,
              headers: { "Content-Type": "application/json" },
            },
          ),
        );
      }
      return mockFetch(input, init);
    });

    await renderPath("/settings/hosts");

    const editButton = Array.from(container.querySelectorAll("button")).find(
      (button) => button.textContent?.includes("编辑"),
    );
    expect(editButton).toBeTruthy();
    await act(async () => editButton?.click());
    await flush();

    const dialog = document.body.querySelector('[data-slot="dialog-content"]');
    expect(dialog?.textContent).toContain("编辑主机");
    const saveButton = Array.from(
      dialog?.querySelectorAll("button") || [],
    ).find((button) => button.textContent?.includes("保存"));
    expect(saveButton).toBeTruthy();
    await act(async () => saveButton?.click());
    await flush();
    expect(
      vi
        .mocked(globalThis.fetch)
        .mock.calls.some(
          (call) =>
            String(call[0]).endsWith("/api/v1/hosts/host-prod-07") &&
            (call[1] as RequestInit | undefined)?.method === "PUT",
        ),
    ).toBe(true);

    const dialogAfterSave = document.body.querySelector(
      '[data-slot="dialog-content"]',
    );
    expect(dialogAfterSave?.textContent).toContain(
      "ssh credential ref is required",
    );
    expect(
      container.querySelector('[role="alert"]')?.textContent || "",
    ).not.toContain("ssh credential ref is required");
  });

  it("saves host access configuration without installing the agent", async () => {
    await renderPath("/settings/hosts");

    const createButton = Array.from(container.querySelectorAll("button")).find(
      (button) => button.textContent?.includes("接入主机"),
    );
    expect(createButton).toBeTruthy();
    await act(async () => createButton?.click());
    await flush();

    const dialog = document.body.querySelector('[data-slot="dialog-content"]');
    expect(dialog?.textContent).toContain("SSH 密码");
    expect(dialog?.textContent).toContain("留空保留已保存密码");
    expect(dialog?.textContent).not.toContain(
      "可选。保存后仅由服务端内部 secret 使用；编辑时留空表示保留已保存密码或使用默认 SSH 认证。",
    );
    expect(dialog?.textContent).not.toContain("SSH 凭据引用");
    expect(dialog?.textContent).not.toContain("Host ID");
    expect(dialog?.textContent).toContain("名称（可选）");
    expect(dialog?.textContent).not.toContain("接入方式");
    expect(dialog?.textContent).not.toContain("SSH 安装 Node");
    expect(dialog?.textContent).not.toContain("Agent 主动上报");
    expect(dialog?.textContent).not.toContain("服务本机");
    expect(dialog?.textContent).not.toContain("Transport");
    expect(dialog?.textContent).toContain(
      "SSH 用户必须是 root 或具备 sudo 权限",
    );
    expect(dialog?.textContent).toContain("连接方式");
    expect(dialog?.textContent).toContain("Node 连接地址");
    expect(dialog?.textContent).not.toContain("AI Server 回调地址");

    const addressInput = inputInField(dialog, "地址");
    const userInput = inputInField(dialog, "SSH 用户");
    const passwordInput = inputInField(dialog, "SSH 密码");
    expect(passwordInput?.type).toBe("password");
    await act(async () => {
      setInputValue(addressInput!, "172.18.13.13");
      setInputValue(userInput!, "kduser");
      setInputValue(passwordInput!, "ssh-user-password");
    });

    const saveButton = Array.from(
      dialog?.querySelectorAll("button") || [],
    ).find((button) => button.textContent?.includes("保存"));
    expect(saveButton).toBeTruthy();
    await act(async () => saveButton?.click());
    await flush();

    const createCall = vi
      .mocked(globalThis.fetch)
      .mock.calls.find(
        (call) =>
          String(call[0]).endsWith("/api/v1/hosts") &&
          (call[1] as RequestInit | undefined)?.method === "POST",
      );
    expect(createCall).toBeTruthy();
    const body = requestBodyFromCall(createCall!);
    expect(body).not.toHaveProperty("id");
    expect(body.name).toBe("");
    expect(body.transport).toBe("manual");
    expect(body.installViaSsh).toBe(false);
    expect(body.sshPassword).toBe("ssh-user-password");
    expect(body.connectionMode).toBe("aiops_pull");
    expect(body.agentUrl).toBe("http://172.18.13.13:7072");
    expect(body.agentServerUrl).toBe("");
    expect(body).not.toHaveProperty("sshCredentialRef");
    expect(
      vi
        .mocked(globalThis.fetch)
        .mock.calls.some(
          (call) =>
            String(call[0]).includes("/install") &&
            (call[1] as RequestInit | undefined)?.method === "POST",
        ),
    ).toBe(false);
    expect(container.textContent).toContain("主机 IP / 用户名");
    expect(container.textContent).toContain("10.10.4.27 / root");
    expect(container.textContent).toContain("安装 Node");
  });

  it("shows host save completion in a dialog instead of an inline page banner", async () => {
    await renderPath("/settings/hosts");

    const createButton = Array.from(container.querySelectorAll("button")).find(
      (button) => button.textContent?.includes("接入主机"),
    );
    expect(createButton).toBeTruthy();
    await act(async () => createButton?.click());
    await flush();

    const editDialog = document.body.querySelector(
      '[data-slot="dialog-content"]',
    );
    const addressInput = inputInField(editDialog, "地址");
    const userInput = inputInField(editDialog, "SSH 用户");
    await act(async () => {
      setInputValue(addressInput!, "172.18.13.13");
      setInputValue(userInput!, "kduser");
    });

    const saveButton = Array.from(
      editDialog?.querySelectorAll("button") || [],
    ).find((button) => button.textContent?.includes("保存"));
    await act(async () => saveButton?.click());
    await flush();

    expect(container.querySelector('[role="alert"]')).toBeNull();
    const completionDialog = document.body.querySelector(
      '[data-testid="host-operation-result-dialog"]',
    );
    expect(completionDialog?.textContent).toContain("操作完成");
    expect(completionDialog?.textContent).toContain("主机信息已保存");
  });

  it("switches host connection parameters between pull and node push modes", async () => {
    await renderPath("/settings/hosts");

    const editButton = Array.from(container.querySelectorAll("button")).find(
      (button) => button.textContent?.includes("编辑"),
    );
    expect(editButton).toBeTruthy();
    await act(async () => editButton?.click());
    await flush();

    const dialog = document.body.querySelector('[data-slot="dialog-content"]');
    expect(dialog?.textContent).toContain("连接方式");
    expect(dialog?.textContent).toContain("Node 连接地址");
    expect(dialog?.textContent).not.toContain("AI Server 回调地址");

    const modeSelect = dialog?.querySelector("select") as HTMLSelectElement;
    expect(modeSelect).toBeTruthy();
    await act(async () => {
      setSelectValue(modeSelect, "node_push_grpc");
    });
    await flush();

    const switchedDialog = document.body.querySelector(
      '[data-slot="dialog-content"]',
    );
    expect(switchedDialog?.textContent).toContain("AI Server 回调地址");
    expect(switchedDialog?.textContent).not.toContain("Node 连接地址");

    const callbackInput = inputInField(switchedDialog, "AI Server 回调地址");
    expect(callbackInput).toBeTruthy();
    await act(async () => {
      setInputValue(callbackInput!, "http://aiops.example.test:18080");
    });

    const saveButton = Array.from(
      switchedDialog?.querySelectorAll("button") || [],
    ).find((button) => button.textContent?.includes("保存"));
    expect(saveButton).toBeTruthy();
    await act(async () => saveButton?.click());
    await flush();

    const updateCall = vi
      .mocked(globalThis.fetch)
      .mock.calls.filter(
        (call) =>
          String(call[0]).endsWith("/api/v1/hosts/host-prod-07") &&
          (call[1] as RequestInit | undefined)?.method === "PUT",
      )
      .at(-1);
    expect(updateCall).toBeTruthy();
    const body = requestBodyFromCall(updateCall!);
    expect(body.connectionMode).toBe("node_push_grpc");
    expect(body.agentServerUrl).toBe("http://aiops.example.test:18080");
  });

  it("opens host access config when saved hosts have not reported profiles yet", async () => {
    vi.mocked(globalThis.fetch).mockImplementation((input, init) => {
      const url = String(input);
      if (url.includes("/api/v1/host-profiles"))
        return jsonResponse({ items: [] });
      if (url.includes("/api/v1/host-leases"))
        return jsonResponse({ items: [] });
      return mockFetch(input, init);
    });

    await renderPath("/settings/hosts");

    expect(container.textContent).toContain("主机 IP / 用户名");
    expect(container.textContent).toContain("10.10.4.27 / root");
    expect(container.textContent).toContain("安装 Node");
    expect(container.textContent).not.toContain("暂无主机画像");
  });

  it("installs host agent from the host access list action", async () => {
    await renderPath("/settings/hosts");

    const installButton = Array.from(container.querySelectorAll("button")).find(
      (button) => button.textContent?.includes("安装 Node"),
    );
    expect(installButton).toBeTruthy();
    await act(async () => installButton?.click());
    await flush();

    const installDialog = document.body.querySelector(
      '[data-testid="host-agent-install-dialog"]',
    );
    expect(installDialog?.textContent).toContain("Node 安装步骤");
    expect(installDialog?.className).toContain("max-h-[calc(100dvh-2rem)]");
    expect(installDialog?.className).toContain("overflow-hidden");
    const installDialogScroll = document.body.querySelector(
      '[data-testid="host-agent-install-dialog-scroll"]',
    );
    expect(installDialogScroll?.className).toContain("overflow-y-auto");
    expect(installDialog?.textContent).toContain("web-07");
    expect(installDialog?.textContent).toContain("校验输入");
    expect(installDialog?.textContent).toContain("连接 SSH");
    expect(installDialog?.textContent).toContain("启动服务");
    expect(installDialog?.textContent).toContain("验证本机健康检查");
    expect(installDialog?.textContent).toContain("完成安装");

    const installCall = vi
      .mocked(globalThis.fetch)
      .mock.calls.find(
        (call) =>
          String(call[0]).endsWith("/api/v1/hosts/host-prod-07/install") &&
          (call[1] as RequestInit | undefined)?.method === "POST",
      );
    expect(installCall).toBeTruthy();
    const body = requestBodyFromCall(installCall!);
    expect(body.agentVersion).toBe("1.8.4");
    expect(body.connectionMode).toBe("aiops_pull");
    expect(body.agentServerUrl || "").toBe("");
    expect(body.force).toBe(false);
  });

  it("does not reuse saved node endpoint as the install callback URL", async () => {
    vi.mocked(globalThis.fetch).mockImplementation((input, init) => {
      const url = String(input);
      if (
        url.endsWith("/api/v1/hosts") &&
        (!init || !init.method || init.method === "GET")
      ) {
        return jsonResponse({
          items: [
            {
              ...hostsPayload.items[0],
              agentServerUrl: "http://10.10.4.27:7072",
              connectionMode: "node_push_grpc",
            },
          ],
        });
      }
      return mockFetch(input, init);
    });

    await renderPath("/settings/hosts");

    const installButton = Array.from(container.querySelectorAll("button")).find(
      (button) => button.textContent?.includes("安装 Node"),
    );
    expect(installButton).toBeTruthy();
    await act(async () => installButton?.click());
    await flush();

    const installCall = vi
      .mocked(globalThis.fetch)
      .mock.calls.filter(
        (call) =>
          String(call[0]).endsWith("/api/v1/hosts/host-prod-07/install") &&
          (call[1] as RequestInit | undefined)?.method === "POST",
      )
      .at(-1);
    expect(installCall).toBeTruthy();
    const body = requestBodyFromCall(installCall!);
    expect(body.connectionMode).toBe("node_push_grpc");
    expect(body.agentServerUrl).toBe(window.location.origin);
    expect(body.agentServerUrl).not.toContain(":7072");
  });

  it("keeps host agent install failures inside the install dialog", async () => {
    vi.mocked(globalThis.fetch).mockImplementation((input, init) => {
      const url = String(input);
      if (url.endsWith("/api/v1/hosts/host-prod-07/install")) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              error: "ssh command failed: host-agent start failed",
            }),
            {
              status: 500,
              headers: { "Content-Type": "application/json" },
            },
          ),
        );
      }
      return mockFetch(input, init);
    });

    await renderPath("/settings/hosts");

    const installButton = Array.from(container.querySelectorAll("button")).find(
      (button) => button.textContent?.includes("安装 Node"),
    );
    expect(installButton).toBeTruthy();
    await act(async () => installButton?.click());
    await flush();

    const installDialog = document.body.querySelector(
      '[data-testid="host-agent-install-dialog"]',
    );
    expect(installDialog?.textContent).toContain("安装失败");
    expect(installDialog?.textContent).toContain(
      "ssh command failed: host-agent start failed",
    );
    expect(container.textContent).not.toContain("操作失败");
    expect(container.textContent).not.toContain(
      "ssh command failed: host-agent start failed",
    );
  });

  it("renders Ops Manual tabs, candidates, and run records in Chinese", async () => {
    await renderPath("/settings/ops-manuals");

    expect(container.textContent).toContain("运维手册");
    expect(container.textContent).toContain("已验证手册");
    expect(container.textContent).toContain("待审核手册");
    expect(container.textContent).toContain("执行记录");
    expect(container.textContent).toContain("Redis 内存压力排障");
    expect(container.textContent).toContain("workflow-redis-memory");
    expect(container.textContent).not.toContain("Experience Pack");

    const reviewTab = Array.from(container.querySelectorAll("button")).find(
      (button) => button.textContent?.includes("待审核手册"),
    );
    expect(reviewTab).toBeTruthy();
    await act(async () => reviewTab?.click());
    await flush();
    expect(container.textContent).toContain("Redis 内存压力候选");
    expect(container.textContent).toContain("通过");

    const recordsTab = Array.from(container.querySelectorAll("button")).find(
      (button) => button.textContent?.includes("执行记录"),
    );
    expect(recordsTab).toBeTruthy();
    await act(async () => recordsTab?.click());
    await flush();
    expect(container.textContent).toContain("run-redis-1");
    expect(container.textContent).toContain("成功");
  });

  it("shows an empty state when there are no verified ops manuals", async () => {
    vi.mocked(globalThis.fetch).mockImplementation((input, init) => {
      const url = String(input);
      if (
        url.includes("/api/v1/ops-manuals") &&
        !url.includes("/candidates") &&
        !url.includes("/run-records")
      ) {
        return jsonResponse({ items: [] });
      }
      return mockFetch(input, init);
    });

    await renderPath("/settings/ops-manuals");

    expect(container.textContent).toContain("暂无已验证手册");
    expect(container.textContent).toContain(
      "通过审核并绑定 Runner Workflow 后会出现在这里。",
    );
    expect(
      container.querySelector('[data-testid="ops-manual-side-detail"]'),
    ).toBeNull();
  });

  it("keeps Experience Pack fixture fallback out of production mode", () => {
    expect(
      shouldUseExperiencePackFixtureFallback({
        DEV: false,
        MODE: "production",
      }),
    ).toBe(false);
    expect(
      shouldUseExperiencePackFixtureFallback({
        DEV: true,
        MODE: "development",
      }),
    ).toBe(true);
    expect(
      shouldUseExperiencePackFixtureFallback({ DEV: false, MODE: "test" }),
    ).toBe(true);
  });

  it("supports refresh, save, delete, and import settings operations", async () => {
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);
    await renderPath("/settings/llm");

    expect(container.textContent).not.toContain("保存并重启 Runtime");
    const saveLlm = Array.from(container.querySelectorAll("button")).find(
      (button) => button.textContent?.includes("保存配置"),
    );
    expect(saveLlm).toBeTruthy();
    await act(async () => {
      saveLlm?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    await flush();
    expect(globalThis.fetch).toHaveBeenCalledWith(
      "/api/v1/llm-config",
      expect.objectContaining({ method: "PUT" }),
    );
    expect(container.textContent).toContain("配置已保存");

    await remountPath("/settings/agent");
    const importInput = container.querySelector(
      'input[type="file"]',
    ) as HTMLInputElement;
    const importFile = new File(
      [JSON.stringify(profilesPayload)],
      "profiles.json",
      { type: "application/json" },
    );
    Object.defineProperty(importInput, "files", {
      configurable: true,
      value: [importFile],
    });
    await act(async () => {
      importInput.dispatchEvent(new Event("change", { bubbles: true }));
    });
    await flush();
    expect(globalThis.fetch).toHaveBeenCalledWith(
      "/api/v1/agent-profiles/import",
      expect.objectContaining({ method: "POST" }),
    );
  });

  it("loads and saves the LLM context size", async () => {
    await renderPath("/settings/llm");

    const contextInput = container.querySelector(
      '[data-testid="llm-context-tokens-input"]',
    ) as HTMLInputElement;
    const requestTimeoutInput = container.querySelector(
      '[data-testid="llm-request-timeout-ms-input"]',
    ) as HTMLInputElement;
    expect(contextInput?.value).toBe("131072");
    expect(contextInput?.min).toBe("10000");
    expect(requestTimeoutInput?.value).toBe("25000");
    expect(requestTimeoutInput?.min).toBe("1");
    const reasoningSelect = container.querySelector(
      '[data-testid="llm-reasoning-effort-select"]',
    ) as HTMLSelectElement;
    expect(reasoningSelect?.value).toBe("high");

    const saveLlm = Array.from(container.querySelectorAll("button")).find(
      (button) => button.textContent?.includes("保存配置"),
    );
    await act(async () => {
      saveLlm?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    await flush();

    const llmPutCall = vi
      .mocked(globalThis.fetch)
      .mock.calls.find(
        (call) =>
          String(call[0]).endsWith("/api/v1/llm-config") &&
          (call[1] as RequestInit | undefined)?.method === "PUT",
      );
    expect(llmPutCall).toBeTruthy();
    const body = requestBodyFromCall(llmPutCall as unknown[]);
    expect(body.maxContextTokens).toBe(131072);
    expect(body.requestTimeoutMs).toBe(25000);
    expect(body.reasoningEffort).toBe("high");
  });

  it("does not show explanatory helper text on the LLM config page", async () => {
    await renderPath("/settings/llm");

    expect(container.textContent).not.toContain(
      "配置主模型接入、接口协议、模型名和 Base URL",
    );
    expect(container.textContent).not.toContain("模型接入与接口配置");
    expect(container.textContent).not.toContain("未填写时默认");
    expect(container.textContent).not.toContain("保存时最小");
    expect(container.textContent).not.toContain("已设置时留空会保持原密钥");
    expect(container.textContent).not.toContain("OpenAI 兼容接口可填网关地址");
  });

  it("keeps GLM models under the Zhipu provider instead of OpenAI", async () => {
    await renderPath("/settings/llm");

    const providerSelect = Array.from(
      container.querySelectorAll("select"),
    ).find(
      (select) => select.getAttribute("aria-label") === "Provider",
    ) as HTMLSelectElement;
    expect(providerSelect?.value).toBe("openai");

    const modelSelect = container.querySelector(
      '[data-testid="llm-model-select"]',
    ) as HTMLSelectElement;
    const options = Array.from(modelSelect.options).map(
      (option) => option.value,
    );
    expect(options).not.toContain("glm-4.7");

    await act(async () => {
      providerSelect.value = "zhipu";
      providerSelect.dispatchEvent(new Event("change", { bubbles: true }));
    });
    await flush();

    const zhipuOptions = Array.from(modelSelect.options).map(
      (option) => option.value,
    );
    expect(providerSelect.value).toBe("zhipu");
    expect(zhipuOptions).toContain("glm-4.7");
    expect(modelSelect.value).toMatch(/^glm-/);
    expect(container.textContent).toContain("智谱平台 API");
  });

  it("displays legacy OpenAI-compatible GLM config as Zhipu GLM", async () => {
    vi.mocked(globalThis.fetch).mockImplementation(
      async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        if (url.endsWith("/api/v1/llm-config") && init?.method !== "PUT") {
          return jsonResponse({
            ...llmPayload,
            provider: "openai",
            model: "glm-4.7",
            baseURL: "https://api.z.ai/api/paas/v4",
          });
        }
        return mockFetch(input, init);
      },
    );

    await renderPath("/settings/llm");

    const providerSelect = Array.from(
      container.querySelectorAll("select"),
    ).find(
      (select) => select.getAttribute("aria-label") === "Provider",
    ) as HTMLSelectElement;
    expect(providerSelect?.value).toBe("zhipu");
    expect(container.textContent).toContain("智谱 GLM");
    expect(container.textContent).not.toContain("PROVIDERopenai");
  });

  it("keeps the LLM reasoning effort when switching provider before saving", async () => {
    await renderPath("/settings/llm");

    const providerSelect = Array.from(
      container.querySelectorAll("select"),
    ).find(
      (select) => select.getAttribute("aria-label") === "Provider",
    ) as HTMLSelectElement;
    const reasoningSelect = container.querySelector(
      '[data-testid="llm-reasoning-effort-select"]',
    ) as HTMLSelectElement;
    expect(reasoningSelect?.value).toBe("high");
    await act(async () => {
      providerSelect.value = "anthropic";
      providerSelect.dispatchEvent(new Event("change", { bubbles: true }));
    });
    await flush();
    expect(reasoningSelect.value).toBe("high");

    const saveLlm = Array.from(container.querySelectorAll("button")).find(
      (button) => button.textContent?.includes("保存配置"),
    );
    await act(async () => {
      saveLlm?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    await flush();

    const llmPutCall = vi
      .mocked(globalThis.fetch)
      .mock.calls.find(
        (call) =>
          String(call[0]).endsWith("/api/v1/llm-config") &&
          (call[1] as RequestInit | undefined)?.method === "PUT",
      );
    expect(requestBodyFromCall(llmPutCall as unknown[]).reasoningEffort).toBe(
      "high",
    );
  });

  it("shows the default LLM context size when the API does not return one", async () => {
    vi.mocked(globalThis.fetch).mockImplementation((input, init) => {
      const url = String(input);
      if (url.endsWith("/api/v1/llm-config") && init?.method !== "PUT") {
        return jsonResponse({ ...llmPayload, maxContextTokens: undefined });
      }
      return mockFetch(input, init);
    });

    await renderPath("/settings/llm");

    const contextInput = container.querySelector(
      '[data-testid="llm-context-tokens-input"]',
    ) as HTMLInputElement;
    expect(contextInput?.value).toBe("200000");
  });
});
