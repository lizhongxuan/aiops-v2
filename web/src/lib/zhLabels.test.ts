import { describe, expect, it } from "vitest";

import { navigationSections, routeInventory } from "@/app/navigation";
import {
  zhAgentUiTypeLabel,
  zhCaseStatusLabel,
  zhHostLeaseStatusLabel,
  zhNavigationTitle,
  zhPermissionStatusLabel,
  zhRedactionStatusLabel,
  zhRiskLevelLabel,
  zhWorkflowStatusLabel,
} from "@/lib/zhLabels";

describe("zhLabels", () => {
  it("maps known domain states to Chinese labels", () => {
    expect(zhCaseStatusLabel("open")).toBe("待处理");
    expect(zhCaseStatusLabel("collecting_evidence")).toBe("采集证据");
    expect(zhWorkflowStatusLabel("dry_run_passed")).toBe("试运行通过");
    expect(zhHostLeaseStatusLabel("acquired")).toBe("已锁定");
    expect(zhAgentUiTypeLabel("tool_call")).toBe("工具调用");
    expect(zhAgentUiTypeLabel("coroot_chart")).toBe("Coroot 图表");
    expect(zhAgentUiTypeLabel("ops_manual_preflight_result")).toBe("运维手册预检");
    expect(zhRedactionStatusLabel("redacted")).toBe("已脱敏");
    expect(zhPermissionStatusLabel("pending")).toBe("等待授权");
    expect(zhRiskLevelLabel("high")).toBe("高风险");
  });

  it("returns safe fallback labels for unknown or empty values", () => {
    expect(zhCaseStatusLabel("blocked_by_policy")).toBe("未知状态（blocked_by_policy）");
    expect(zhWorkflowStatusLabel("")).toBe("未知状态");
    expect(zhHostLeaseStatusLabel(undefined)).toBe("未知状态");
    expect(zhNavigationTitle("/unknown-route")).toBe("未知导航（/unknown-route）");
  });

  it("maps simplified navigation titles by route path", () => {
    expect(zhNavigationTitle("/")).toBe("AI 对话");
    expect(zhNavigationTitle("/incidents")).toBe("Case 工作台");
    expect(zhNavigationTitle("/opsgraph")).toBe("OpsGraph");
    expect(zhNavigationTitle("/opsgraph/graphs")).toBe("OpsGraph 列表");
    expect(zhNavigationTitle("/runner")).toBe("Runner Workflow");
    expect(zhNavigationTitle("/settings/llm")).toBe("LLM 配置");
    expect(zhNavigationTitle("/settings/coroot")).toBe("Coroot 监控");
    expect(zhNavigationTitle("/settings/hosts")).toBe("主机列表");
    expect(zhNavigationTitle("/settings/experience-packs")).toBe("运维手册");
    expect(zhNavigationTitle("/capabilities")).toBe("能力管理");
    expect(zhNavigationTitle("/coroot")).toBe("Coroot");
    expect(zhNavigationTitle("/coroot/config")).toBe("Coroot 配置");
    expect(zhNavigationTitle("/agent-ui")).toBe("Agent UI");
    expect(zhNavigationTitle("/debug/prompts")).toBe("Prompt Trace");
  });
});

describe("navigation convergence", () => {
  it("exposes only the simplified nav entry set", () => {
    const visiblePaths = navigationSections.flatMap((section) => section.items.filter((item) => item.nav).map((item) => item.path));

    expect(visiblePaths).toEqual([
      "/",
      "/incidents",
      "/coroot",
      "/opsgraph",
      "/runner",
      "/settings/hosts",
      "/settings/ops-manuals",
      "/capabilities",
      "/agent-ui",
      "/debug/prompts",
    ]);
  });

  it("keeps hidden legacy routes in routeInventory for router compatibility", () => {
    const inventoryPaths = routeInventory.map((item) => item.path);

    expect(inventoryPaths).toContain("/coroot");
    expect(inventoryPaths).toContain("/coroot/config");
    expect(inventoryPaths).toContain("/coroot/p/:projectId/:view?/:id?/:report?");
    expect(inventoryPaths).toContain("/settings/coroot");
    expect(inventoryPaths).toEqual(expect.arrayContaining([
      "/protocol",
      "/erp",
      "/runbooks",
      "/settings/skills",
      "/settings/mcp",
      "/mcp",
      "/capability-center",
      "/agent-ui",
      "/ui-cards",
      "/generator",
      "/lab",
    ]));
  });
});
