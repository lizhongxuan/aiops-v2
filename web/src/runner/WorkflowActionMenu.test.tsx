import { act } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it } from "vitest";

import { groupWorkflowActionMenuItems, WorkflowActionMenu } from "./WorkflowActionMenu";

describe("WorkflowActionMenu", () => {
  it("groups only file, assist and management capabilities and rejects lifecycle duplicates", () => {
    const groups = groupWorkflowActionMenuItems([
      { id: "import", label: "导入", group: "file", onSelect: () => undefined },
      { id: "export", label: "导出", group: "file", onSelect: () => undefined },
      { id: "manual", label: "生成运维手册", group: "assist", onSelect: () => undefined },
      { id: "delete", label: "删除工作流", group: "management", danger: true, onSelect: () => undefined },
      { id: "publish" as "import", label: "发布", group: "file", onSelect: () => undefined },
    ]);
    expect(groups.map((group) => group.label)).toEqual(["文件", "辅助", "管理"]);
    expect(groups.flatMap((group) => group.items.map((item) => item.id))).toEqual(["import", "export", "manual", "delete"]);
  });

  it("exposes one labelled Radix trigger for the keyboard menu", async () => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    const container = document.createElement("div");
    document.body.appendChild(container);
    const root = createRoot(container);
    await act(async () => root.render(
      <WorkflowActionMenu items={[
        { id: "export", label: "导出", group: "file", onSelect: () => undefined },
      ]} />,
    ));
    const trigger = container.querySelector('[data-testid="workflow-action-menu-trigger"]');
    expect(trigger?.getAttribute("aria-label")).toBe("更多工作流操作");
    await act(async () => root.unmount());
    container.remove();
  });
});
