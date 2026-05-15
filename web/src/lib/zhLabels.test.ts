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
    expect(zhNavigationTitle("/runner")).toBe("Runner Workflow");
    expect(zhNavigationTitle("/settings/llm")).toBe("LLM 配置");
    expect(zhNavigationTitle("/settings/hosts")).toBe("主机与租约");
    expect(zhNavigationTitle("/settings/experience-packs")).toBe("运维手册");
    expect(zhNavigationTitle("/mcp")).toBe("MCP 服务");
    expect(zhNavigationTitle("/coroot")).toBe("Coroot 观测");
    expect(zhNavigationTitle("/debug/prompts")).toBe("Prompt Trace");
  });
});

describe("navigation convergence", () => {
  it("exposes only the simplified nav entry set", () => {
    const visiblePaths = navigationSections.flatMap((section) => section.items.filter((item) => item.nav).map((item) => item.path));

    expect(visiblePaths).toEqual([
      "/",
      "/incidents",
      "/opsgraph",
      "/runner",
      "/settings/llm",
      "/settings/hosts",
      "/settings/ops-manuals",
      "/mcp",
      "/coroot",
      "/debug/prompts",
    ]);
  });

  it("keeps hidden legacy routes in routeInventory for router compatibility", () => {
    const inventoryPaths = routeInventory.map((item) => item.path);

    expect(inventoryPaths).toEqual(expect.arrayContaining([
      "/protocol",
      "/erp",
      "/runbooks",
      "/capability-center",
      "/ui-cards",
      "/generator",
      "/lab",
    ]));
  });
});
