import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { AgentUiArtifactPart } from "./AgentUiArtifactPart";

describe("WorkflowResultArtifact compatibility", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
  });

  it("keeps workflow_result on the existing execution result renderer", async () => {
    await act(async () => {
      root.render(
        <AgentUiArtifactPart artifact={{
          id: "workflow-run-1",
          type: "workflow_result",
          payload: {
            hostLease: { hostId: "host-a", expiresAt: "2026-07-21T10:00:00Z" },
            failedStep: "健康检查",
            rollbackResult: "已回滚",
          },
        }} />,
      );
    });

    expect(container.textContent).toContain("主机租约");
    expect(container.textContent).toContain("失败步骤");
    expect(container.textContent).toContain("回滚结果");
    expect(container.querySelector('[data-testid="workflow-change-artifact"]')).toBeNull();
  });
});
