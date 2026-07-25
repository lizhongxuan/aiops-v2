import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createWorkflowWorkspaceState, workflowWorkspaceReducer } from "./workspace/workflowWorkspaceReducer";
import type { WorkflowRunGateFacts } from "./workspace/workflowWorkspaceSelectors";
import { WorkflowRunPanel } from "./WorkflowRunPanel";

const key = { tenantId: "tenant-a", workflowId: "workflow-a" };
const revision = (value: string) => ({ resourceVersion: value, semanticHash: `semantic-${value}`, layoutHash: `layout-${value}` });
const basis = (value: string) => ({ resourceVersion: value, semanticHash: `semantic-${value}`, catalogVersion: "catalog-1", actionSchemaDigest: "actions-1", referenceSnapshotHash: "refs-1", environmentBindingVersion: "env-1" });

function loaded(value = "rv-1") {
  return workflowWorkspaceReducer(createWorkflowWorkspaceState(), {
    type: "snapshot.loaded",
    snapshot: { workflowKey: key, revision: revision(value), graph: {}, sequence: 1, loadedAt: "2026-07-21T00:00:00Z" },
  });
}

function facts(value = "rv-1", overrides: Partial<WorkflowRunGateFacts> = {}): WorkflowRunGateFacts {
  return {
    currentBasis: basis(value),
    environment: { name: "staging", bindingVersion: "env-1" },
    postCheck: { basis: basis(value), mode: "draft_non_production", status: "dry_run_passed", checkedAt: "2026-07-21T00:00:01Z" },
    ...overrides,
  };
}

describe("WorkflowRunPanel", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
  });

  it("presents the exact non-production draft target without owning its lifecycle action", async () => {
    await act(async () => root.render(<WorkflowRunPanel state={loaded()} facts={facts()} />));

    const draftTarget = container.querySelector('[data-testid="workflow-target-draft"]') as HTMLElement;
    expect(draftTarget.textContent).toContain("当前草稿");
    expect(draftTarget.textContent).toContain("非生产");
    expect(draftTarget.textContent).not.toContain("rv-1");
    expect(draftTarget.textContent).toContain("staging");
    expect(draftTarget.textContent).toContain("可试运行");
    expectNoLifecycleCta(container);
  });

  it("does not duplicate Run published when the Command Bar owns run_published", async () => {
    const onRunPublished = vi.fn();
    let state = loaded("rv-1");
    state = workflowWorkspaceReducer(state, { type: "lifecycle.received", artifact: { artifactId: "published-1", basis: basis("rv-1"), status: "published", issuedAt: "2026-07-21T00:00:00Z" } });
    await act(async () => root.render(<WorkflowRunPanel state={state} facts={facts("rv-1")} onRunPublished={onRunPublished} />));

    const publishedTarget = container.querySelector('[data-testid="workflow-target-published"]') as HTMLElement;
    expect(publishedTarget.textContent).not.toContain("rv-1");
    expect(publishedTarget.textContent).toContain("可运行");
    expect(container.querySelector('[data-testid="workflow-run-published"]')).toBeNull();
    expect(onRunPublished).not.toHaveBeenCalled();
    expectNoDraftOrRecoveryCta(container);
  });

  it("offers one immutable published secondary action when a newer draft makes Publish changes primary", async () => {
    const onRunPublished = vi.fn();
    let state = loaded("rv-2");
    state = workflowWorkspaceReducer(state, { type: "lifecycle.received", artifact: { artifactId: "published-1", basis: basis("rv-1"), status: "published", issuedAt: "2026-07-21T00:00:00Z" } });
    await act(async () => root.render(<WorkflowRunPanel state={state} facts={facts("rv-2")} onRunPublished={onRunPublished} />));

    const publishedTarget = container.querySelector('[data-testid="workflow-target-published"]') as HTMLElement;
    const draftTarget = container.querySelector('[data-testid="workflow-target-draft"]') as HTMLElement;
    expect(publishedTarget.textContent).toContain("已发布版本");
    expect(publishedTarget.textContent).not.toContain("rv-1");
    expect(publishedTarget.textContent).toContain("可运行");
    expect(draftTarget.textContent).toContain("当前草稿");
    expect(draftTarget.textContent).not.toContain("rv-2");
    expect(publishedTarget.textContent).not.toContain("rv-2");
    const publishedActions = container.querySelectorAll<HTMLButtonElement>('[data-testid="workflow-run-published"]');
    expect(publishedActions).toHaveLength(1);
    expect(publishedActions[0].textContent).toContain("运行已发布版本");
    await act(async () => publishedActions[0].click());
    expect(onRunPublished).toHaveBeenCalledTimes(1);
    expect(onRunPublished).toHaveBeenCalledWith({
      kind: "published_revision",
      workflowKey: key,
      resourceVersion: "rv-1",
      semanticHash: "semantic-rv-1",
    });
    expectNoDraftOrRecoveryCta(container);
  });

  it("shows no disabled published CTA when an immutable published target does not exist", async () => {
    await act(async () => root.render(<WorkflowRunPanel state={loaded()} facts={facts()} />));

    expect(container.textContent).toContain("尚无已发布版本");
    expectNoLifecycleCta(container);
  });

  it("keeps full revision and basis identifiers out of the DOM until technical details are expanded", async () => {
    const fullRevision = `sha256:${"a".repeat(64)}`;
    const state = loaded(fullRevision);
    const runFacts = facts(fullRevision, {
      currentBasis: {
        ...basis(fullRevision),
        semanticHash: `sha256:${"b".repeat(64)}`,
      },
    });
    await act(async () => root.render(<WorkflowRunPanel state={state} facts={runFacts} />));

    expect(container.textContent).not.toContain(fullRevision);
    expect(container.textContent).not.toContain(`semantic-${fullRevision}`);
    expect(container.textContent).not.toContain("sha256:");
    const disclosure = container.querySelector('[data-testid="workflow-run-technical-details-toggle"]') as HTMLButtonElement;
    await act(async () => disclosure.click());
    expect(container.textContent).toContain(fullRevision);
    expect(container.textContent).toContain(`semantic-${fullRevision}`);
  });

  it("explains that a validated draft remains blocked when no trusted environment is bound", async () => {
    const state = workflowWorkspaceReducer(loaded(), {
      type: "validation.received",
      artifact: {
        artifactId: "valid-no-environment",
        basis: basis("rv-1"),
        status: "valid",
        diagnostics: [],
        issuedAt: "2026-07-21T00:00:01Z",
      },
    });
    const runFacts = facts("rv-1", {
      environment: { name: "", bindingVersion: "env-1" },
      postCheck: undefined,
    });
    await act(async () => root.render(<WorkflowRunPanel state={state} facts={runFacts} />));

    expect(container.textContent).toContain("当前没有可信运行环境，因此暂不能试运行");
    expect(container.textContent).not.toContain("可以在绑定环境中试运行当前草稿");
    expectNoLifecycleCta(container);
  });

  it("keeps session run records inside one inspector and exposes recovery only when it is wired", async () => {
    const onRetrySessionRun = vi.fn();
    await act(async () => root.render(
      <WorkflowRunPanel
        state={loaded()}
        facts={facts()}
        sessionRuns={[
          {
            id: "run-2",
            status: "failed",
            targetLabel: "当前草稿",
            startedAt: "2026-07-21T08:02:00Z",
            durationMs: 1200,
            failure: { action: "执行检查步骤", savedState: "草稿已保存", message: "远端连接已中断" },
            variables: [{ name: "region", value: "cn-south-1" }],
            path: ["Start", "检查步骤"],
            logs: ["连接远端执行器", "连接中断"],
          },
        ]}
        onRetrySessionRun={onRetrySessionRun}
      />,
    ));

    expect(container.querySelectorAll('[data-testid="workflow-run-inspector"]')).toHaveLength(1);
    expect(container.textContent).toContain("当前会话");
    expect(container.textContent).not.toContain("执行检查步骤失败");
    await act(async () => (container.querySelector('[data-testid="workflow-run-inspector"] ol button') as HTMLButtonElement).click());
    expect(container.textContent).toContain("执行检查步骤失败");
    expect(container.textContent).toContain("草稿已保存");
    await act(async () => (container.querySelector('[data-testid="workflow-run-retry"]') as HTMLButtonElement).click());
    expect(onRetrySessionRun).toHaveBeenCalledWith("run-2");
  });

  it("renders configuration_required with identifiers only and ignores values or unsafe names", async () => {
    const configuration = {
      secretRefs: ["secret://workflow/cloud-api", "api-token=do-not-render"],
      environments: ["staging"],
      secretValues: ["do-not-render"],
    };
    const configFacts = facts("rv-1", {
      postCheck: {
        basis: basis("rv-1"), mode: "draft_non_production", status: "configuration_required",
        checkedAt: "2026-07-21T00:00:01Z", configuration,
      },
    });
    await act(async () => root.render(<WorkflowRunPanel state={loaded()} facts={configFacts} />));

    expect(container.textContent).toContain("还需配置");
    expect(container.textContent).toContain("secret://workflow/cloud-api");
    expect(container.textContent).toContain("staging");
    expect(container.textContent).not.toContain("do-not-render");
    expectNoLifecycleCta(container);
  });

  it("shows saved_with_issues facts without raw diagnostics or duplicate recovery actions", async () => {
    let state = loaded("rv-2");
    state = workflowWorkspaceReducer(state, { type: "lifecycle.received", artifact: { artifactId: "published-1", basis: basis("rv-1"), status: "published", issuedAt: "2026-07-21T00:00:00Z" } });
    const issueFacts = facts("rv-2", {
      postCheck: {
        basis: basis("rv-2"), mode: "draft_non_production", status: "saved_with_issues",
        checkedAt: "2026-07-21T00:00:01Z",
        diagnostics: [{ code: "CONNECTOR_FAILED", severity: "error", message: "secret-value-do-not-render" }],
      },
    });
    await act(async () => root.render(<WorkflowRunPanel state={state} facts={issueFacts} />));

    expect(container.textContent).toContain("已保存但有问题");
    expect(container.textContent).toContain("发布与运行已关闭");
    expect(container.textContent).not.toContain("secret-value-do-not-render");
    expectNoLifecycleCta(container);
    expect(container.querySelector('[data-publish-allowed="false"]')).not.toBeNull();
  });

  it("fails closed when current basis facts are absent", async () => {
    await act(async () => root.render(<WorkflowRunPanel state={loaded()} />));
    expect(container.textContent).toContain("运行依据尚未同步");
    expectNoLifecycleCta(container);
  });
});

function expectNoLifecycleCta(container: HTMLElement) {
  expectNoDraftOrRecoveryCta(container);
  expect(container.querySelector('[data-testid="workflow-run-published"]')).toBeNull();
}

function expectNoDraftOrRecoveryCta(container: HTMLElement) {
  expect(container.querySelector('[data-testid="workflow-draft-dry-run"]')).toBeNull();
  expect([...container.querySelectorAll("button")].map((button) => button.textContent?.trim())).not.toEqual(
    expect.arrayContaining(["试运行", "配置测试环境", "查看全部问题"]),
  );
}
