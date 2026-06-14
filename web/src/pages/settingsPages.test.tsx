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
      runtime: { os_release: "Ubuntu 24.04", kernel: "6.8.0", request_body: "secret-business-payload" },
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
        workflow_binding: { workflow_id: "wf-pg-pool-fix", workflow_name: "PG Pool Fix", status: "draft", version: "v1" },
        retrieval_eval: { score: 0.91, matched_cases: 4, verdict: "pass", last_evaluated_at: "2026-05-12T09:30:00+08:00" },
        authorization_scopes: [{ type: "environment", value: "prod", searchable: true, reason: "生产 PG 集群" }],
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
        workflow_binding: { workflow_id: "wf-java-heap", workflow_name: "Java Heap RCA", status: "bound" },
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
      applicability: { middleware: "redis", os: ["ubuntu"], platform: ["vm"], execution_surface: ["ssh"] },
      required_context: { required_inputs: ["target_instance"] },
      preconditions: ["确认目标实例"],
      validation: ["指标恢复"],
      cannot_use_when: ["无法确认实例"],
      run_record_summary: { success_count: 3, failure_count: 1, recent_result: "success" },
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
  reasoningEffort: "high",
  bifrostActive: true,
};

const skillPayload = {
  items: [{ id: "ops-triage", name: "Ops Triage", description: "Triage incidents", source: "builtin", defaultEnabled: true, defaultActivationMode: "default_enabled" }],
};

const mcpPayload = {
  items: [{ id: "metrics", name: "Metrics MCP", type: "http", source: "builtin", defaultEnabled: true, permission: "readonly" }],
};

const profilesPayload = {
  items: [
    {
      id: "main-agent",
      name: "Main Agent",
      description: "Main runtime profile",
      systemPrompt: { content: "You are the main AIOps agent." },
      runtime: { model: "gpt-5.4", approvalMode: "on-request", sandboxMode: "workspace-write" },
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
      { id: "coroot", kind: "mcp_server", enabled: true, source: "profile", sourceScope: "user", reason: "enabled by profile sre", runtimeStatus: "connected", risk: "low" },
      { id: "dangerous-shell", kind: "skill", enabled: false, source: "admin_policy", sourceScope: "managed", reason: "disabled by admin deny", risk: "high" },
      { id: "plugin-shell", kind: "mcp_server", enabled: false, source: "plugin:ops", sourceScope: "plugin", reason: "pending explicit approval", runtimeStatus: "pending_approval", risk: "high" },
    ],
  },
};

function jsonResponse(payload: unknown) {
  return Promise.resolve(new Response(JSON.stringify(payload), { status: 200, headers: { "Content-Type": "application/json" } }));
}

function mockFetch(input: RequestInfo | URL, init?: RequestInit) {
  const url = String(input);
  if (url.endsWith("/api/v1/state")) return jsonResponse(statePayload);
  if (url.includes("/api/v1/host-profiles/") && url.endsWith("/report-history")) return jsonResponse(hostReportHistoryPayload);
  if (url.includes("/api/v1/host-profiles")) return jsonResponse(hostProfilesPayload);
  if (url.includes("/api/v1/host-leases")) return jsonResponse(hostLeasesPayload);
  if (url.includes("/api/v1/experience-packs/") && url.includes("/reuse-records")) return jsonResponse(experienceReusePayload);
  if (url.includes("/api/v1/experience-packs/candidates")) return jsonResponse(experienceCandidatesPayload);
  if (url.includes("/api/v1/experience-packs/") && url.includes("/authorization-scopes")) return jsonResponse({ pack: experienceCandidatesPayload.items[0].experience_pack });
  if (url.includes("/api/v1/experience-packs/") && url.includes("/enabled")) return jsonResponse({ pack: experienceCandidatesPayload.items[0].experience_pack });
  if (url.includes("/api/v1/ops-manuals/candidates")) return jsonResponse(opsManualCandidatesPayload);
  if (url.includes("/api/v1/ops-manuals/run-records")) return jsonResponse(opsManualRunRecordsPayload);
  if (url.includes("/api/v1/ops-manuals")) return jsonResponse(opsManualsPayload);
  if (url.endsWith("/api/v1/hosts")) {
    if (init?.method === "POST") return jsonResponse({ ok: true });
    return jsonResponse(hostsPayload);
  }
  if (url.endsWith("/api/v1/sessions")) return jsonResponse({ activeSessionId: "sess-1", sessions: [{ id: "sess-1", kind: "single_host", title: "Nginx chat", selectedHostId: "host-prod-07" }] });
  if (url.endsWith("/api/v1/terminal/sessions")) return jsonResponse({ items: [{ id: "term-1", status: "running" }] });
  if (url.endsWith("/api/v1/llm-config")) return jsonResponse(init?.method === "PUT" ? { ok: true, message: "saved" } : llmPayload);
  if (url.endsWith("/api/v1/agent-skills")) return jsonResponse(skillPayload);
  if (url.includes("/api/v1/agent-skills/")) return jsonResponse({ items: skillPayload.items, item: skillPayload.items[0] });
  if (url.endsWith("/api/v1/agent-mcps")) return jsonResponse(mcpPayload);
  if (url.includes("/api/v1/agent-mcps/")) return jsonResponse({ items: mcpPayload.items, item: mcpPayload.items[0] });
  if (url.endsWith("/api/v1/agent-profiles")) return jsonResponse(profilesPayload);
  if (url.includes("/api/v1/agent-profile/preview")) return jsonResponse(agentProfilePreviewPayload);
  if (url.endsWith("/api/v1/agent-profile")) return jsonResponse(init?.method === "PUT" ? profilesPayload.items[0] : profilesPayload.items[0]);
  if (url.endsWith("/api/v1/agent-profile/reset")) return jsonResponse(profilesPayload.items[0]);
  if (url.endsWith("/api/v1/agent-profiles/export")) return jsonResponse(profilesPayload);
  if (url.endsWith("/api/v1/agent-profiles/import")) return jsonResponse(profilesPayload);
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
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
  setter?.call(input, value);
  input.dispatchEvent(new Event("input", { bubbles: true }));
}

function inputInField(root: Element | null | undefined, labelText: string) {
  const label = Array.from(root?.querySelectorAll("label") || []).find((item) => item.textContent?.includes(labelText));
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
    act(() => {
      root.unmount();
    });
    container.remove();
  });

  it.each([
    ["/settings", "设置"],
    ["/settings/llm", "LLM 配置"],
    ["/settings/hosts", "env=prod"],
    ["/settings/ops-manuals", "Redis 内存压力排障"],
    ["/settings/experience-packs", "旧入口已迁移到运维手册"],
    ["/settings/agent", "Agent Profile"],
    ["/settings/skills", "Ops Triage"],
    ["/settings/mcp", "Metrics MCP"],
  ])("renders migrated settings route %s", async (path, expectedText) => {
    await renderPath(path);

    expect(container.textContent).toContain(expectedText);
    expect(container.textContent).not.toContain("Migration Placeholder");
  });

  it("renders Agent Profile Effective Capabilities preview with source and disabled reasons", async () => {
    await renderPath("/settings/agent");

    expect(container.textContent).toContain("Effective Capabilities");
    expect(container.textContent).toContain("sha256:preview");
    expect(container.textContent).toContain("coroot");
    expect(container.textContent).toContain("connected");
    expect(container.textContent).toContain("disabled by admin deny");
    expect(container.textContent).toContain("pending explicit approval");

    const enabledList = container.querySelector('[data-testid="effective-capabilities-enabled"]');
    expect(enabledList?.textContent).toContain("coroot");
    expect(enabledList?.textContent).not.toContain("dangerous-shell");
    expect(enabledList?.textContent).not.toContain("plugin-shell");
  });

  it("moves settings page actions into the app shell header", async () => {
    await renderPath("/settings/hosts");

    const hostsHeader = container.querySelector('[data-testid="app-shell-header"]');
    expect(hostsHeader?.textContent).toContain("主机与租约");
    expect(hostsHeader?.textContent).not.toContain("刷新");
    expect(hostsHeader?.textContent).toContain("接入主机");
    expect(container.querySelector("main > div header")?.textContent || "").not.toContain("HostLease 锁状态");

    await remountPath("/settings/ops-manuals");
    const packsHeader = container.querySelector('[data-testid="app-shell-header"]');
    expect(packsHeader?.textContent).toContain("运维手册");
    expect(packsHeader?.textContent).not.toContain("刷新");
    expect(container.querySelector("main > div header")?.textContent || "").not.toContain("经验包入口已迁移");
  });

  it("renders HostProfile, HostLease, report history and access config tabs in Chinese", async () => {
    await renderPath("/settings/hosts");

    expect(container.textContent).toContain("主机与租约");
    expect(container.textContent).toContain("主机画像");
    expect(container.textContent).toContain("主机租约");
    expect(container.textContent).toContain("上报历史");
    expect(container.textContent).toContain("接入配置");
    expect(container.textContent).toContain("web-07");
    expect(container.textContent).toContain("Linux");
    expect(container.textContent).toContain("x86_64");
    expect(container.textContent).toContain("env=prod");
    expect(container.textContent).toContain("基础信息");
    expect(container.textContent).toContain("运行环境");
    expect(container.textContent).toContain("已安装 Agent");
    expect(container.textContent).toContain("service runtime");
    expect(container.textContent).toContain("最近 Case");
    expect(container.textContent).toContain("当前 HostLease");
    expect(container.textContent).toContain("agent-prod-07");
    expect(container.textContent).toContain("aiops-agent.service");
    expect(container.textContent).toContain("case-debug-1");
    expect(container.textContent).toContain("2026-05-12T09:40:00+08:00");
    expect(container.textContent).not.toMatch(
      /secret-profile-token|secret-profile-password|PRIVATE KEY|secret-cookie|secret-authorization|secret-business-payload/i,
    );

    const leaseTab = Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("主机租约"));
    expect(leaseTab).toBeTruthy();
    await act(async () => leaseTab?.dispatchEvent(new MouseEvent("click", { bubbles: true })));
    await flush();
    expect(container.textContent).toContain("lease-prod-07");
    expect(container.textContent).toContain("case-debug-1");

    const reportTab = Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("上报历史"));
    expect(reportTab).toBeTruthy();
    await act(async () => reportTab?.dispatchEvent(new MouseEvent("click", { bubbles: true })));
    await flush();
    expect(container.textContent).toContain("report-web-07");

    const profileTab = Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("主机画像"));
    expect(profileTab).toBeTruthy();
    await act(async () => profileTab?.dispatchEvent(new MouseEvent("click", { bubbles: true })));
    await flush();
    expect(container.textContent).toContain("客户端离线");
    expect(container.textContent).toContain("环境标签缺失");
    expect(container.textContent).toContain("host-db-01");
    expect(container.textContent).toContain("case-pg-1");
    expect(container.textContent).toContain("2026-05-12T09:45:00+08:00");

    const accessTab = Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("接入配置"));
    expect(accessTab).toBeTruthy();
  });

  it("shows host access save errors inside the host dialog layer", async () => {
    vi.mocked(globalThis.fetch).mockImplementation((input, init) => {
      const url = String(input);
      if (url.includes("/api/v1/hosts/") && init?.method === "PUT") {
        return Promise.resolve(
          new Response(JSON.stringify({ error: "ssh credential ref is required" }), {
            status: 400,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      return mockFetch(input, init);
    });

    await renderPath("/settings/hosts");

    const accessTab = Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("接入配置"));
    expect(accessTab).toBeTruthy();
    await act(async () => accessTab?.click());
    await flush();

    const editButton = Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("编辑"));
    expect(editButton).toBeTruthy();
    await act(async () => editButton?.click());
    await flush();

    const dialog = document.body.querySelector('[data-slot="dialog-content"]');
    expect(dialog?.textContent).toContain("编辑主机");
    const saveButton = Array.from(dialog?.querySelectorAll("button") || []).find((button) => button.textContent?.includes("保存"));
    expect(saveButton).toBeTruthy();
    await act(async () => saveButton?.click());
    await flush();
    expect(
      vi.mocked(globalThis.fetch).mock.calls.some((call) => String(call[0]).endsWith("/api/v1/hosts/host-prod-07") && (call[1] as RequestInit | undefined)?.method === "PUT"),
    ).toBe(true);

    const dialogAfterSave = document.body.querySelector('[data-slot="dialog-content"]');
    expect(dialogAfterSave?.textContent).toContain("ssh credential ref is required");
    expect(container.querySelector('[role="alert"]')?.textContent || "").not.toContain("ssh credential ref is required");
  });

  it("saves host access configuration without installing the agent", async () => {
    await renderPath("/settings/hosts");

    const createButton = Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("接入主机"));
    expect(createButton).toBeTruthy();
    await act(async () => createButton?.click());
    await flush();

    const dialog = document.body.querySelector('[data-slot="dialog-content"]');
    expect(dialog?.textContent).toContain("SSH 密码");
    expect(dialog?.textContent).not.toContain("SSH 凭据引用");
    expect(dialog?.textContent).not.toContain("Host ID");
    expect(dialog?.textContent).toContain("名称（可选）");
    expect(dialog?.textContent).not.toContain("接入方式");
    expect(dialog?.textContent).not.toContain("SSH 安装 Agent");
    expect(dialog?.textContent).not.toContain("Agent 主动上报");
    expect(dialog?.textContent).not.toContain("服务本机");
    expect(dialog?.textContent).not.toContain("Transport");
    expect(dialog?.textContent).toContain("SSH 用户必须是 root 或具备 sudo 权限");

    const addressInput = inputInField(dialog, "地址");
    const userInput = inputInField(dialog, "SSH 用户");
    const passwordInput = inputInField(dialog, "SSH 密码");
    expect(passwordInput?.type).toBe("password");
    await act(async () => {
      setInputValue(addressInput!, "172.18.13.13");
      setInputValue(userInput!, "kduser");
      setInputValue(passwordInput!, "ssh-user-password");
    });

    const saveButton = Array.from(dialog?.querySelectorAll("button") || []).find((button) => button.textContent?.includes("保存"));
    expect(saveButton).toBeTruthy();
    await act(async () => saveButton?.click());
    await flush();

    const createCall = vi
      .mocked(globalThis.fetch)
      .mock.calls.find((call) => String(call[0]).endsWith("/api/v1/hosts") && (call[1] as RequestInit | undefined)?.method === "POST");
    expect(createCall).toBeTruthy();
    const body = requestBodyFromCall(createCall!);
    expect(body).not.toHaveProperty("id");
    expect(body.name).toBe("");
    expect(body.transport).toBe("manual");
    expect(body.installViaSsh).toBe(false);
    expect(body.sshPassword).toBe("ssh-user-password");
    expect(body).not.toHaveProperty("sshCredentialRef");
    expect(
      vi.mocked(globalThis.fetch).mock.calls.some((call) => String(call[0]).includes("/install") && (call[1] as RequestInit | undefined)?.method === "POST"),
    ).toBe(false);
    expect(container.textContent).toContain("主机 IP / 用户名");
    expect(container.textContent).toContain("10.10.4.27 / root");
    expect(container.textContent).toContain("安装 Agent");
  });

  it("opens host access config when saved hosts have not reported profiles yet", async () => {
    vi.mocked(globalThis.fetch).mockImplementation((input, init) => {
      const url = String(input);
      if (url.includes("/api/v1/host-profiles")) return jsonResponse({ items: [] });
      if (url.includes("/api/v1/host-leases")) return jsonResponse({ items: [] });
      return mockFetch(input, init);
    });

    await renderPath("/settings/hosts");

    expect(container.textContent).toContain("主机 IP / 用户名");
    expect(container.textContent).toContain("10.10.4.27 / root");
    expect(container.textContent).toContain("安装 Agent");
    expect(container.textContent).not.toContain("暂无主机画像");
  });

  it("installs host agent from the host access list action", async () => {
    await renderPath("/settings/hosts");

    const accessTab = Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("接入配置"));
    expect(accessTab).toBeTruthy();
    await act(async () => accessTab?.click());
    await flush();

    const installButton = Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("安装 Agent"));
    expect(installButton).toBeTruthy();
    await act(async () => installButton?.click());
    await flush();

    const installCall = vi
      .mocked(globalThis.fetch)
      .mock.calls.find((call) => String(call[0]).endsWith("/api/v1/hosts/host-prod-07/install") && (call[1] as RequestInit | undefined)?.method === "POST");
    expect(installCall).toBeTruthy();
    const body = requestBodyFromCall(installCall!);
    expect(body.agentVersion).toBe("1.8.4");
    expect(body.force).toBe(false);
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

    const reviewTab = Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("待审核手册"));
    expect(reviewTab).toBeTruthy();
    await act(async () => reviewTab?.click());
    await flush();
    expect(container.textContent).toContain("Redis 内存压力候选");
    expect(container.textContent).toContain("通过");

    const recordsTab = Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("执行记录"));
    expect(recordsTab).toBeTruthy();
    await act(async () => recordsTab?.click());
    await flush();
    expect(container.textContent).toContain("run-redis-1");
    expect(container.textContent).toContain("成功");
  });

  it("shows an empty state when there are no verified ops manuals", async () => {
    vi.mocked(globalThis.fetch).mockImplementation((input, init) => {
      const url = String(input);
      if (url.includes("/api/v1/ops-manuals") && !url.includes("/candidates") && !url.includes("/run-records")) {
        return jsonResponse({ items: [] });
      }
      return mockFetch(input, init);
    });

    await renderPath("/settings/ops-manuals");

    expect(container.textContent).toContain("暂无已验证手册");
    expect(container.textContent).toContain("通过审核并绑定 Runner Workflow 后会出现在这里。");
    expect(container.querySelector('[data-testid="ops-manual-side-detail"]')).toBeNull();
  });

  it("keeps Experience Pack fixture fallback out of production mode", () => {
    expect(shouldUseExperiencePackFixtureFallback({ DEV: false, MODE: "production" })).toBe(false);
    expect(shouldUseExperiencePackFixtureFallback({ DEV: true, MODE: "development" })).toBe(true);
    expect(shouldUseExperiencePackFixtureFallback({ DEV: false, MODE: "test" })).toBe(true);
  });

  it("supports refresh, save, delete, and import settings operations", async () => {
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);
    await renderPath("/settings/llm");

    expect(container.textContent).not.toContain("保存并重启 Runtime");
    const saveLlm = Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("保存配置"));
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

    await remountPath("/settings/skills");
    const deleteSkill = Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("删除"));
    expect(deleteSkill).toBeTruthy();
    await act(async () => {
      deleteSkill?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    await flush();
    expect(confirmSpy).toHaveBeenCalled();
    expect(globalThis.fetch).toHaveBeenCalledWith(
      "/api/v1/agent-skills/ops-triage",
      expect.objectContaining({ method: "DELETE" }),
    );

    await remountPath("/settings/agent");
    const importInput = container.querySelector('input[type="file"]') as HTMLInputElement;
    const importFile = new File([JSON.stringify(profilesPayload)], "profiles.json", { type: "application/json" });
    Object.defineProperty(importInput, "files", { configurable: true, value: [importFile] });
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

    const contextInput = container.querySelector('[data-testid="llm-context-tokens-input"]') as HTMLInputElement;
    expect(contextInput?.value).toBe("131072");
    expect(contextInput?.min).toBe("10000");
    const reasoningSelect = container.querySelector('[data-testid="llm-reasoning-effort-select"]') as HTMLSelectElement;
    expect(reasoningSelect?.value).toBe("high");

    const saveLlm = Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("保存配置"));
    await act(async () => {
      saveLlm?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    await flush();

    const llmPutCall = vi.mocked(globalThis.fetch).mock.calls.find((call) => String(call[0]).endsWith("/api/v1/llm-config") && (call[1] as RequestInit | undefined)?.method === "PUT");
    expect(llmPutCall).toBeTruthy();
    const body = requestBodyFromCall(llmPutCall as unknown[]);
    expect(body.maxContextTokens).toBe(131072);
    expect(body.reasoningEffort).toBe("high");
  });

  it("does not show explanatory helper text on the LLM config page", async () => {
    await renderPath("/settings/llm");

    expect(container.textContent).not.toContain("配置主模型接入、接口协议、模型名和 Base URL");
    expect(container.textContent).not.toContain("模型接入与接口配置");
    expect(container.textContent).not.toContain("未填写时默认");
    expect(container.textContent).not.toContain("保存时最小");
    expect(container.textContent).not.toContain("已设置时留空会保持原密钥");
    expect(container.textContent).not.toContain("OpenAI 兼容接口可填网关地址");
  });

  it("keeps GLM models under the Zhipu provider instead of OpenAI", async () => {
    await renderPath("/settings/llm");

    const providerSelect = Array.from(container.querySelectorAll("select")).find((select) => select.getAttribute("aria-label") === "Provider") as HTMLSelectElement;
    expect(providerSelect?.value).toBe("openai");

    const options = Array.from(container.querySelectorAll("#llm-model-presets option")).map((option) => option.getAttribute("value"));
    expect(options).not.toContain("glm-4.7");

    await act(async () => {
      providerSelect.value = "zhipu";
      providerSelect.dispatchEvent(new Event("change", { bubbles: true }));
    });
    await flush();

    const zhipuOptions = Array.from(container.querySelectorAll("#llm-model-presets option")).map((option) => option.getAttribute("value"));
    expect(providerSelect.value).toBe("zhipu");
    expect(zhipuOptions).toContain("glm-4.7");
    expect((container.querySelector('input[list="llm-model-presets"]') as HTMLInputElement)?.value).toBe("glm-4.7");
    expect(container.textContent).toContain("OpenAI 兼容接口");
  });

  it("displays legacy OpenAI-compatible GLM config as Zhipu GLM", async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
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
    });

    await renderPath("/settings/llm");

    const providerSelect = Array.from(container.querySelectorAll("select")).find((select) => select.getAttribute("aria-label") === "Provider") as HTMLSelectElement;
    expect(providerSelect?.value).toBe("zhipu");
    expect(container.textContent).toContain("智谱 GLM");
    expect(container.textContent).not.toContain("PROVIDERopenai");
  });

  it("keeps the LLM reasoning effort when switching provider before saving", async () => {
    await renderPath("/settings/llm");

    const providerSelect = Array.from(container.querySelectorAll("select")).find((select) => select.getAttribute("aria-label") === "Provider") as HTMLSelectElement;
    const reasoningSelect = container.querySelector('[data-testid="llm-reasoning-effort-select"]') as HTMLSelectElement;
    expect(reasoningSelect?.value).toBe("high");
    await act(async () => {
      providerSelect.value = "anthropic";
      providerSelect.dispatchEvent(new Event("change", { bubbles: true }));
    });
    await flush();
    expect(reasoningSelect.value).toBe("high");

    const saveLlm = Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("保存配置"));
    await act(async () => {
      saveLlm?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    await flush();

    const llmPutCall = vi
      .mocked(globalThis.fetch)
      .mock.calls.find((call) => String(call[0]).endsWith("/api/v1/llm-config") && (call[1] as RequestInit | undefined)?.method === "PUT");
    expect(requestBodyFromCall(llmPutCall as unknown[]).reasoningEffort).toBe("high");
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

    const contextInput = container.querySelector('[data-testid="llm-context-tokens-input"]') as HTMLInputElement;
    expect(contextInput?.value).toBe("200000");
  });
});
