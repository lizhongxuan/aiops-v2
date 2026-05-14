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
        category: "repair",
        usage_shape: "guided",
        status: "enabled",
        review_status: "approved",
        enabled: true,
        validation_gate: { status: "passed", validators: ["runner.readonly_probe"] },
        runner_bindings: [{ id: "binding-pg", workflow_id: "wf-pg-pool-fix", workflow_name: "PG Pool Fix", status: "ready" }],
        history: { success_count: 4, failure_count: 1, recent_result: "success" },
        advanced_refs: { gene_asset_id: "gene-pg-pool", capsule_asset_ids: ["capsule-pg-1"] },
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
        category: "optimize",
        usage_shape: "diagnostic",
        status: "enabled",
        review_status: "approved",
        enabled: true,
        validation_gate: { status: "blocked", reasons: ["缺少 rollback"] },
        history: { success_count: 2, failure_count: 0, recent_result: "success" },
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

const runnerCandidatePayload = {
  id: "runner-candidate-pg",
  pack_id: "pack-pg-pool",
  workflow_id: "wf-pg-pool-local-draft",
  workflow_name: "PG Pool Local Draft",
  status: "draft",
  studio_draft_link: "/runner/wf-pg-pool-local-draft",
  workflow: {
    id: "wf-pg-pool-local-draft",
    name: "wf-pg-pool-local-draft",
    title: "PG Pool Local Draft",
    graph: {
      workflow: { name: "wf-pg-pool-local-draft" },
      nodes: [{ id: "start", type: "start" }, { id: "validate", type: "action", step: { action: "shell.run", args: { script: "echo ok" } } }],
      edges: [{ id: "start-validate", source: "start", target: "validate", source_port: "next", target_port: "in" }],
    },
  },
};

const llmPayload = {
  provider: "openai",
  model: "gpt-5.4",
  apiKeySet: true,
  apiKeyMasked: "sk-***",
  baseURL: "https://api.openai.com/v1",
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
  if (url.endsWith("/api/v1/experience-packs/runner-candidates/confirm")) return jsonResponse(runnerCandidatePayload);
  if (url.endsWith("/api/v1/experience-packs/runner-candidates/prepare")) return jsonResponse(runnerCandidatePayload);
  if (url.includes("/api/v1/experience-packs/candidates")) return jsonResponse(experienceCandidatesPayload);
  if (url.includes("/api/v1/experience-packs/") && url.includes("/authorization-scopes")) return jsonResponse({ pack: experienceCandidatesPayload.items[0].experience_pack });
  if (url.includes("/api/v1/experience-packs/") && url.includes("/enabled")) return jsonResponse({ pack: experienceCandidatesPayload.items[0].experience_pack });
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
    window.localStorage.clear();
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
    ["/settings/experience-packs", "经验库"],
    ["/settings/agent", "Agent Profile"],
    ["/settings/skills", "Ops Triage"],
    ["/settings/mcp", "Metrics MCP"],
  ])("renders migrated settings route %s", async (path, expectedText) => {
    await renderPath(path);

    expect(container.textContent).toContain(expectedText);
    expect(container.textContent).not.toContain("Migration Placeholder");
  });

  it("moves settings page actions into the app shell header", async () => {
    await renderPath("/settings/hosts");

    const hostsHeader = container.querySelector('[data-testid="app-shell-header"]');
    expect(hostsHeader?.textContent).toContain("主机与租约");
    expect(hostsHeader?.textContent).not.toContain("刷新");
    expect(hostsHeader?.textContent).toContain("接入主机");
    expect(container.querySelector("main > div header")?.textContent || "").not.toContain("HostLease 锁状态");

    await remountPath("/settings/experience-packs");
    const packsHeader = container.querySelector('[data-testid="app-shell-header"]');
    expect(packsHeader?.textContent).toContain("经验包");
    expect(packsHeader?.textContent).not.toContain("刷新");
    expect(container.querySelector("main > div header")?.textContent || "").not.toContain("必须先审核启用");
  });

  it("sends an experience-pack review item to Runner Studio as a local draft", async () => {
    await renderPath("/settings/experience-packs");

    const reviewTab = Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("待审核经验"));
    expect(reviewTab).toBeTruthy();
    await act(async () => reviewTab?.dispatchEvent(new MouseEvent("click", { bubbles: true })));
    await flush();

    const sendButton = Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("发送到 Runner Studio"));
    expect(sendButton).toBeTruthy();
    await act(async () => sendButton?.dispatchEvent(new MouseEvent("click", { bubbles: true })));
    await flush();

    expect(container.textContent).toContain("已创建本地草稿");
    const link = Array.from(container.querySelectorAll("a")).find((anchor) => anchor.textContent?.includes("打开 Runner Studio"));
    expect(link?.getAttribute("href")).toBe("/runner/wf-pg-pool-local-draft");
    const drafts = JSON.parse(window.localStorage.getItem("runner.studio.localDrafts") || "{}");
    expect(drafts["wf-pg-pool-local-draft"]).toMatchObject({
      id: "wf-pg-pool-local-draft",
      local_draft: true,
      ai_generated_draft: true,
    });
    expect(drafts["wf-pg-pool-local-draft"].graph.nodes).toHaveLength(2);

    await remountPath("/runner/wf-pg-pool-local-draft");
    const draftsAfterNavigation = JSON.parse(window.localStorage.getItem("runner.studio.localDrafts") || "{}");
    expect(container.textContent).toContain("PG Pool Local Draft");
    expect(container.textContent).toContain("validate");
    expect(draftsAfterNavigation["wf-pg-pool-local-draft"].graph.nodes).toHaveLength(2);
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

  it("renders Experience Pack library, review queue, detail, validation, runner, and advanced sections in Chinese", async () => {
    await renderPath("/settings/experience-packs");

    expect(container.textContent).toContain("经验包");
    expect(container.textContent).toContain("经验库");
    expect(container.textContent).toContain("待审核经验");
    const packsHeader = container.querySelector('[data-testid="app-shell-header"]');
    const headerText = packsHeader?.textContent || "";
    expect((headerText.match(/经验包/g) || [])).toHaveLength(1);
    expect(headerText).toContain("经验库");
    expect(headerText).toContain("待审核经验");
    expect(headerText).not.toContain("经验详情");
    expect(packsHeader?.contains(container.querySelector('[role="tablist"][aria-label="经验包视图"]'))).toBe(true);
    expect(container.textContent).not.toContain("共同呈现 GEP Skill Bundle");
    expect(container.textContent).not.toContain("默认展示 Skill");
    expect(container.textContent).toContain("PG 连接池修复候选经验包");
    expect(container.textContent).toContain("Java 堆内存排障经验包");
    expect(container.textContent).toContain("不可检索");
    expect(container.textContent).toContain("上一页");
    expect(container.textContent).toContain("下一页");
    expect(container.querySelector('[data-testid="experience-pack-pagination"]')?.className).toContain("mt-auto");
    expect(container.textContent).not.toContain("历史成功");
    expect(container.textContent).not.toContain("Validation Gate passed");

    const reviewTab = Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("待审核经验"));
    expect(reviewTab).toBeTruthy();
    await act(async () => reviewTab?.click());
    await flush();
    expect(container.textContent).toContain("Skill.md 摘要");
    expect(container.textContent).toContain("必要文件完整性");
    expect(container.textContent).toContain("GEP schema");
    expect(container.textContent).toContain("asset_id");
    expect(container.textContent).toContain("Capsule 来源");
    expect(container.textContent).toContain("AVOID cue");
    expect(container.textContent).toContain("发送到 Runner Studio");

    const firstExperienceCard = Array.from(container.querySelectorAll('[data-slot="card"][data-size="sm"]')).find((card) => card.textContent?.includes("PG 连接池修复候选经验包"));
    expect(firstExperienceCard).toBeTruthy();
    await act(async () => firstExperienceCard?.dispatchEvent(new MouseEvent("click", { bubbles: true })));
    await flush();
    const detailDialog = container.querySelector('[role="dialog"][aria-modal="true"]');
    expect(detailDialog?.textContent).toContain("经验详情");
    expect(detailDialog?.parentElement?.className).toContain("z-[90]");
    expect(detailDialog?.textContent).toContain("Skill.md");
    expect(detailDialog?.textContent).toContain("Skill / Runner / GEP");
    expect(detailDialog?.textContent).toContain("Runner 负责怎么执行");
    expect(detailDialog?.textContent).toContain("PostgreSQL + pgvector");
    expect(detailDialog?.textContent).toContain("历史效果");
    expect(detailDialog?.textContent).toContain("高级区");
    expect(detailDialog?.textContent).toContain("Gene asset_id");
    expect(detailDialog?.textContent).toContain("capsule-pg-1");
    expect(detailDialog?.textContent).toContain("OS/environment variants");
    expect(detailDialog?.textContent).toContain("Runner Bindings");
  });

  it("lets an approved Experience Pack configure searchable scope from the detail page", async () => {
    await renderPath("/settings/experience-packs");

    const javaCard = Array.from(container.querySelectorAll('[data-slot="card"][data-size="sm"]')).find((card) => card.textContent?.includes("Java 堆内存排障经验包"));
    expect(javaCard).toBeTruthy();
    await act(async () => javaCard?.dispatchEvent(new MouseEvent("click", { bubbles: true })));
    await flush();

    const detailDialog = container.querySelector('[role="dialog"][aria-modal="true"]');
    expect(detailDialog?.textContent).toContain("Java 堆内存排障经验包");
    expect(detailDialog?.textContent).toContain("经验包尚未配置可检索授权范围");

    const configureButton = Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("配置可检索范围"));
    expect(configureButton).toBeTruthy();
    await act(async () => configureButton?.dispatchEvent(new MouseEvent("click", { bubbles: true })));
    await flush();

    expect(globalThis.fetch).toHaveBeenCalledWith(
      "/api/v1/experience-packs/pack-java-heap/authorization-scopes",
      expect.objectContaining({
        method: "PUT",
        body: JSON.stringify({ scopes: [{ type: "environment", value: "prod", searchable: true, reason: "默认生产环境授权" }] }),
      }),
    );
  });

  it("keeps the Experience Pack workbench full width when there are no candidates", async () => {
    vi.mocked(globalThis.fetch).mockImplementation((input, init) => {
      const url = String(input);
      if (url.includes("/api/v1/experience-packs/candidates")) {
        return jsonResponse({ items: [] });
      }
      return mockFetch(input, init);
    });

    await renderPath("/settings/experience-packs");

    const layout = container.querySelector('[data-testid="experience-pack-workbench-layout"]');
    expect(layout?.className).toContain("xl:grid-cols-1");
    expect(container.textContent).toContain("没有经验包");
    expect(container.textContent).toContain("从 AI 对话或 Case 详情提炼");
    expect(container.textContent).toContain("审核启用后再进入经验库");
  });

  it("keeps Experience Pack fixture fallback out of production mode", () => {
    expect(shouldUseExperiencePackFixtureFallback({ DEV: false, MODE: "production" })).toBe(false);
    expect(shouldUseExperiencePackFixtureFallback({ DEV: true, MODE: "development" })).toBe(true);
    expect(shouldUseExperiencePackFixtureFallback({ DEV: false, MODE: "test" })).toBe(true);
  });

  it("supports refresh, save, delete, and import settings operations", async () => {
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);
    await renderPath("/settings/llm");

    const saveLlm = Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("保存并重启 Runtime"));
    expect(saveLlm).toBeTruthy();
    await act(async () => {
      saveLlm?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    await flush();
    expect(globalThis.fetch).toHaveBeenCalledWith(
      "/api/v1/llm-config",
      expect.objectContaining({ method: "PUT" }),
    );

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
});
