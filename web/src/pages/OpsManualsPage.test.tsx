import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { OpsManualsPage } from "./OpsManualsPage";

const manualsPayload = {
  items: [
    {
      id: "manual-redis-memory",
      title: "Redis 内存压力排障",
      status: "verified",
      version: "v1",
      owner: "sre",
      workflow_ref: { workflow_id: "workflow-redis-memory", workflow_version: "v3" },
      operation: { target_type: "redis", action: "rca_or_repair", risk_level: "medium", stateful: true },
      applicability: { middleware: "redis", os: ["ubuntu"], platform: ["vm"], execution_surface: ["ssh"] },
      required_context: { required_inputs: ["target_instance"], required_evidence: ["used_memory_rss"] },
      parameter_rules: { target_instance: { source: "user_or_inventory", required: true } },
      preconditions: ["确认目标实例可连接"],
      validation: ["used_memory_rss 回落"],
      cannot_use_when: ["无法确认目标实例"],
      document_markdown: "用于 Redis 内存压力排障。",
      run_record_summary: { success_count: 7, failure_count: 1, recent_result: "success", last_run_at: "2026-05-14T09:00:00+08:00" },
    },
  ],
};

const candidatesPayload = {
  items: [
    {
      id: "candidate-pg-backup",
      source_type: "workflow",
      source_refs: ["workflow-postgres-backup"],
      review_status: "pending",
      proposed_manual: {
        id: "manual-pg-backup-draft",
        title: "PostgreSQL 备份候选",
        status: "draft",
        workflow_ref: { workflow_id: "workflow-postgres-backup" },
        operation: { target_type: "postgresql", action: "backup" },
        applicability: { middleware: "postgresql", os: ["centos"], platform: ["vm"], execution_surface: ["ssh"] },
        required_context: { required_inputs: ["host", "backup_path"] },
        preconditions: ["确认磁盘空间"],
        validation: ["备份文件可校验"],
        cannot_use_when: ["无法确认数据库版本"],
      },
      validation_report: ["缺少一次 Dry Run 记录"],
    },
  ],
};

const runRecordsPayload = {
  items: [
    {
      id: "run-redis-1",
      manual_id: "manual-redis-memory",
      workflow_id: "workflow-redis-memory",
      dry_run_status: "passed",
      execution_status: "success",
      validation_status: "passed",
      operator: "sre",
      completed_at: "2026-05-14T09:00:00+08:00",
    },
    {
      id: "run-redis-2",
      manual_id: "manual-redis-memory",
      workflow_id: "workflow-redis-memory",
      dry_run_status: "passed",
      execution_status: "failed",
      validation_status: "failed",
      failure_reason: "指标未恢复",
      operator: "sre",
      completed_at: "2026-05-13T09:00:00+08:00",
    },
  ],
};

function jsonResponse(payload: unknown) {
  return Promise.resolve(new Response(JSON.stringify(payload), { status: 200, headers: { "Content-Type": "application/json" } }));
}

function mockFetch(input: RequestInfo | URL) {
  const url = String(input);
  if (url.includes("/api/v1/ops-manuals/candidates")) return jsonResponse(candidatesPayload);
  if (url.includes("/api/v1/ops-manuals/run-records")) return jsonResponse(runRecordsPayload);
  if (url.includes("/api/v1/ops-manuals")) return jsonResponse(manualsPayload);
  return jsonResponse({});
}

async function flush() {
  await act(async () => {
    for (let index = 0; index < 5; index += 1) {
      await Promise.resolve();
    }
  });
}

describe("OpsManualsPage", () => {
  let container: HTMLDivElement;
  let root: Root;

  async function renderPath(path = "/settings/ops-manuals") {
    await act(async () => {
      root.render(
        <MemoryRouter initialEntries={[path]}>
          <OpsManualsPage />
        </MemoryRouter>,
      );
    });
    await flush();
  }

  beforeEach(() => {
    (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
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
    vi.restoreAllMocks();
  });

  it("renders verified manuals, review candidates, and run record tabs without internal scores", async () => {
    await renderPath();

    expect(container.textContent).toContain("运维手册");
    expect(container.textContent).toContain("已验证手册");
    expect(container.textContent).toContain("待审核手册");
    expect(container.textContent).toContain("执行记录");
    expect(container.textContent).toContain("Redis 内存压力排障");
    expect(container.textContent).toContain("redis / rca_or_repair");
    expect(container.textContent).toContain("ubuntu");
    expect(container.textContent).toContain("vm");
    expect(container.textContent).toContain("ssh");
    expect(container.textContent).toContain("workflow-redis-memory");
    expect(container.textContent).toContain("最近执行");
    expect(container.textContent).not.toContain("digest");
    expect(container.textContent).not.toContain("命中率");
    expect(container.textContent ?? "").not.toMatch(/Gene|Capsule|GEP|EvolutionEvent/);

    const reviewTab = Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("待审核手册"));
    await act(async () => reviewTab?.dispatchEvent(new MouseEvent("click", { bubbles: true })));
    expect(container.textContent).toContain("PostgreSQL 备份候选");
    expect(container.textContent).toContain("通过");
    expect(container.textContent).toContain("退回修改");
    expect(container.textContent).toContain("删除候选");
    expect(container.textContent).toContain("只读预览");
    expect(container.textContent ?? "").not.toMatch(/Gene|Capsule|GEP|EvolutionEvent/);

    const recordsTab = Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("执行记录"));
    await act(async () => recordsTab?.dispatchEvent(new MouseEvent("click", { bubbles: true })));
    expect(container.textContent).toContain("成功 1");
    expect(container.textContent).toContain("失败 1");
    expect(container.textContent).toContain("指标未恢复");
    expect(container.textContent ?? "").not.toMatch(/Gene|Capsule|GEP|EvolutionEvent/);
  });

  it("opens manual details in a modal instead of a side-by-side detail panel", async () => {
    await renderPath();

    expect(container.querySelector('[data-testid="ops-manual-side-detail"]')).toBeNull();
    const card = container.querySelector('[data-testid="ops-manual-card-manual-redis-memory"]') as HTMLButtonElement | null;
    expect(card).not.toBeNull();

    await act(async () => {
      card?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    const dialog = document.body.querySelector('[role="dialog"]');
    expect(dialog?.textContent).toContain("使用说明");
    expect(dialog?.textContent).toContain("适用环境");
    expect(dialog?.textContent).toContain("参数说明");
    expect(dialog?.textContent).toContain("前置检查");
    expect(dialog?.textContent).toContain("验证方式");
    expect(dialog?.textContent).toContain("不能使用条件");
    expect(dialog?.textContent).toContain("绑定 Workflow");
    expect(dialog?.textContent).toContain("执行记录");
  });

  it("shows a migration notice when entered from the old experience packs route", async () => {
    await renderPath("/settings/experience-packs");

    expect(container.textContent).toContain("旧入口已迁移到运维手册");
    expect(container.textContent).toContain("/settings/ops-manuals");
  });
});
