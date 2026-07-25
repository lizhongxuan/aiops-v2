import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";

import { RunnerCanvas } from "@/components/runner/RunnerCanvas";
import {
  removeGraphNode,
  type RunnerEdge,
  type RunnerGraph,
  type RunnerNode,
} from "@/components/runner/canvasGraphAdapter";
import { tryProjectWorkflowCanvas } from "@/components/runner/workflowCanvasProjection";
import { requestWorkflowContextDockTab, WorkflowContextDock } from "@/runner/WorkflowContextDock";
import type { WorkflowActionMenuItem } from "@/runner/WorkflowActionMenu";
import { WorkflowChangeReview, type WorkflowReviewFieldDiff } from "@/runner/WorkflowChangeReview";
import { WorkflowCandidateActionBar } from "@/runner/WorkflowCandidateActionBar";
import { WorkflowCandidateWorkspaceActionBar } from "@/runner/WorkflowCandidateWorkspaceActionBar";
import { WorkflowDeleteConfirmation, deriveWorkflowNodeDeleteImpact } from "@/runner/WorkflowDeleteConfirmation";
import { WorkflowHistoryPanel, type WorkflowDurableHistoryEntry } from "@/runner/WorkflowHistoryPanel";
import {
  WorkflowLifecycleHeader,
  type WorkflowLifecycleAction,
  type WorkflowSavePresentation,
} from "@/runner/WorkflowLifecycleHeader";
import { WorkflowRunPanel } from "@/runner/WorkflowRunPanel";
import type { WorkflowRunSessionRecord } from "@/runner/WorkflowRunInspector";
import { WorkflowValidationPanel, type WorkflowValidationLocation } from "@/runner/WorkflowValidationPanel";
import {
  createWorkflowStudioActionRegistry,
  type WorkflowStudioActionCapabilities,
  type WorkflowStudioActionHandlers,
} from "@/runner/workflowStudioActionRegistry";
import {
  useWorkflowWorkspace,
  type WorkflowManualCommand,
  type WorkflowMutationOptions,
} from "@/runner/workspace/WorkflowWorkspaceProvider";
import type { WorkflowPrimaryActionTarget } from "@/runner/workspace/workflowInteractionContract";
import type { WorkflowChangeSetV2, WorkflowJsonValue } from "@/runner/workspace/workflowMcpContracts";
import {
  type WorkflowRunGateFacts,
} from "@/runner/workspace/workflowWorkspaceSelectors";
import {
  createWorkflowSurfaceFocusDetail,
  dispatchWorkflowSurfaceFocus,
  readWorkflowSurfaceFocusDetail,
  resolveWorkflowOperationIndex,
  WORKFLOW_SURFACE_FOCUS_EVENT,
  type WorkflowSurfaceFocusCandidate,
  type WorkflowSurfaceFocusDetail,
  type WorkflowSurfaceFocusTarget,
} from "@/runner/transport/workflowSurfaceFocus";

export type WorkflowEditorAction = {
  action?: string;
  name?: string;
  label?: string;
  title?: string;
  category?: string;
  description?: string;
};

export type WorkflowEditorInspectorContext = {
  graph: RunnerGraph;
  node: RunnerNode | null;
  readOnly: boolean;
  onApplyNode: (node: RunnerNode) => void;
  onDeleteNode: (nodeId: string) => void;
  onCloseNode: () => void;
};

export type WorkflowEditorCommandState =
  | { status: "pending"; message: string }
  | { status: "accepted"; message: string }
  | { status: "reconciled"; message: string }
  | { status: "failed"; message: string };

export type WorkflowEditorShellProps = {
  graph: RunnerGraph;
  actions: WorkflowEditorAction[];
  runState: {
    nodes?: Record<string, { status?: string; message?: string; error?: string; summary?: string; result?: unknown }>;
  };
  focusNodeId: string;
  selectedNodeId: string;
  aiHighlightedNodeIds: string[];
  fullscreen: boolean;
  workflowTitle?: string;
  workflowEnvironment?: string;
  saveState?: WorkflowSavePresentation;
  lifecycleHeaderTarget?: HTMLElement | null;
  runGateFacts?: WorkflowRunGateFacts;
  renderNodeInspector?: (context: WorkflowEditorInspectorContext) => ReactNode;
  aiTransportPanel?: ReactNode;
  candidateReviewPanel?: ReactNode;
  sessionRuns?: WorkflowRunSessionRecord[];
  durableHistoryEntries?: WorkflowDurableHistoryEntry[];
  commandMenuItems?: readonly WorkflowActionMenuItem[];
  onCanvasDeleteEdge?: (edgeId: string) => void;
  onCanvasUndo?: () => void;
  onCanvasRedo?: () => void;
  /**
   * A brand-new local draft has no server revision to bind a typed command to.
   * This callback is the one bootstrap exception: it edits the local draft
   * until the initial Graph POST establishes the authoritative revision.
   */
  onBootstrapGraphChange?: (graph: RunnerGraph) => void;
  onReviseCandidateGraph?: (previousCandidateGraph: RunnerGraph, nextCandidateGraph: RunnerGraph) => void | Promise<void>;
  onUndoHistory?: (transactionId: string) => void;
  onRedoHistory?: (transactionId: string) => void;
  onResolveHistoryConflict?: (transactionId: string, strategy: "review_three_way") => void;
  onCheckWorkflow?: (target: Extract<WorkflowPrimaryActionTarget, { kind: "saved_draft" }>) => void | Promise<void>;
  onDryRunWorkflow?: (target: Extract<WorkflowPrimaryActionTarget, { kind: "draft_non_production" }>) => void | Promise<void>;
  onPublishWorkflow?: (target: Extract<WorkflowPrimaryActionTarget, { kind: "saved_draft" }>) => void | Promise<void>;
  onRunPublishedWorkflow?: (target: Extract<WorkflowPrimaryActionTarget, { kind: "published_revision" }>) => void | Promise<void>;
  onRetrySessionRun?: (runId: string) => void;
  onCommandStateChange?: (state: WorkflowEditorCommandState) => void;
  onBack?: () => void;
  onSelectNode: (nodeId: string) => void;
  onOpenNodeConfig: (nodeId: string) => void;
  onToggleFullscreen: () => void;
};

const ignoreNodeAction = () => {};

/**
 * Canvas owner for server-authoritative manual edits. The input Graph is only
 * an initial/read-only rendering fallback; a committed Workspace projection
 * always wins and no command mutates that Graph optimistically.
 */
export function WorkflowEditorShell({
  graph,
  actions,
  runState,
  focusNodeId,
  selectedNodeId,
  aiHighlightedNodeIds,
  fullscreen,
  workflowTitle = "工作流",
  workflowEnvironment,
  saveState,
  lifecycleHeaderTarget,
  runGateFacts,
  renderNodeInspector,
  aiTransportPanel,
  candidateReviewPanel,
  sessionRuns = [],
  durableHistoryEntries = [],
  commandMenuItems = [],
  onCanvasDeleteEdge,
  onCanvasUndo,
  onCanvasRedo,
  onBootstrapGraphChange,
  onReviseCandidateGraph,
  onUndoHistory,
  onRedoHistory,
  onResolveHistoryConflict,
  onCheckWorkflow,
  onDryRunWorkflow,
  onPublishWorkflow,
  onRunPublishedWorkflow,
  onRetrySessionRun,
  onCommandStateChange,
  onBack,
  onSelectNode,
  onOpenNodeConfig,
  onToggleFullscreen,
}: WorkflowEditorShellProps) {
  const workspace = useWorkflowWorkspace();
  const observedPendingRef = useRef(false);
  const [surfaceFocus, setSurfaceFocus] = useState<WorkflowSurfaceFocusDetail>();
  const [reviewFocus, setReviewFocus] = useState<WorkflowSurfaceFocusDetail>();
  const [pendingCanvasDeleteNodeId, setPendingCanvasDeleteNodeId] = useState<string>();
  const committedGraph = runnerGraph(workspace.state.committed?.graph) || graph;
  const candidate = workspace.state.candidate;
  const candidateProjection = useMemo(
    () => tryProjectWorkflowCanvas(committedGraph, candidate, {
      committedResourceVersion: workspace.state.committed?.revision.resourceVersion,
    }),
    [candidate, committedGraph, workspace.state.committed?.revision.resourceVersion],
  );
  const candidateGraph = candidateProjection.graph;
  const candidateCanRevise = Boolean(
    candidate
    && candidateProjection.status === "candidate"
    && onReviseCandidateGraph,
  );
  const inspectorGraph = candidate ? candidateGraph : committedGraph;
  const reviewFieldDiffs = useMemo(
    () => candidate ? workflowReviewFieldDiffs(candidate.changeSet, committedGraph, candidateGraph) : [],
    [candidate, candidateGraph, committedGraph],
  );
  const pendingCanvasDelete = useMemo(() => {
    const node = (inspectorGraph.nodes || []).find((item) => item.id === pendingCanvasDeleteNodeId);
    return node ? deriveWorkflowNodeDeleteImpact(inspectorGraph, node) : undefined;
  }, [inspectorGraph, pendingCanvasDeleteNodeId]);
  const workspaceHasSelection = workspace.state.selection.nodeIds.length + workspace.state.selection.edgeIds.length > 0;
  const effectiveSelectedNodeId = surfaceFocus && "nodeId" in surfaceFocus.target
    ? surfaceFocus.target.nodeId
    : workspace.state.selection.nodeIds[0] || (workspaceHasSelection ? "" : selectedNodeId);
  const effectiveFocusNodeId = surfaceFocus && "nodeId" in surfaceFocus.target
    ? `${surfaceFocus.target.nodeId}:${surfaceFocus.requestId}`
    : focusNodeId;
  const selectedNode = inspectorGraph.nodes?.find((node) => node.id === effectiveSelectedNodeId) || null;

  const submitGraphChange = useCallback((nextGraph: RunnerGraph) => {
    if (!workspace.state.committed && onBootstrapGraphChange) {
      onBootstrapGraphChange(nextGraph);
      return;
    }
    let commands: WorkflowManualCommand[];
    try {
      commands = workflowCommandsForGraphChange(committedGraph, nextGraph);
    } catch (error) {
      onCommandStateChange?.({ status: "failed", message: workflowCommandErrorMessage(error) });
      return;
    }
    const options = commandOptions(commands);
    onCommandStateChange?.({ status: "pending", message: "正在保存到服务端" });
    void workspace.commands.executeBatch(commands, options)
      .then(() => onCommandStateChange?.({ status: "accepted", message: "已提交，等待服务端事件确认" }))
      .catch((error) => onCommandStateChange?.({ status: "failed", message: workflowCommandErrorMessage(error) }));
  }, [committedGraph, onBootstrapGraphChange, onCommandStateChange, workspace.commands, workspace.state.committed]);

  const reviseCandidateGraph = useCallback((previousGraph: RunnerGraph, nextGraph: RunnerGraph) => {
    if (!candidateCanRevise || !onReviseCandidateGraph) {
      onCommandStateChange?.({ status: "failed", message: "当前候选版本为只读，无法修改" });
      return;
    }
    try {
      const pending = onReviseCandidateGraph(previousGraph, nextGraph);
      if (pending && typeof pending.then === "function") {
        void pending.catch((error) => onCommandStateChange?.({ status: "failed", message: workflowCommandErrorMessage(error) }));
      }
    } catch (error) {
      onCommandStateChange?.({ status: "failed", message: workflowCommandErrorMessage(error) });
    }
  }, [candidateCanRevise, onCommandStateChange, onReviseCandidateGraph]);

  const submitInspectorGraphChange = useCallback((nextGraph: RunnerGraph) => {
    if (candidate) {
      reviseCandidateGraph(candidateGraph, nextGraph);
      return;
    }
    submitGraphChange(nextGraph);
  }, [candidate, candidateGraph, reviseCandidateGraph, submitGraphChange]);

  const deleteNode = useCallback((nodeId: string) => {
    const nextGraph = candidate
      ? {
          ...inspectorGraph,
          nodes: (inspectorGraph.nodes || []).filter((node) => node.id !== nodeId),
          edges: (inspectorGraph.edges || []).filter((edge) => edge.source !== nodeId && edge.target !== nodeId),
        }
      : removeGraphNode(inspectorGraph, nodeId, { bridgeSimpleChain: true });
    submitInspectorGraphChange(nextGraph);
  }, [candidate, inspectorGraph, submitInspectorGraphChange]);

  const requestCanvasDeleteNode = useCallback((nodeId: string) => {
    const node = (inspectorGraph.nodes || []).find((item) => item.id === nodeId);
    if (!node) return;
    const impact = deriveWorkflowNodeDeleteImpact(inspectorGraph, node);
    if (!impact.requiresConfirmation) {
      deleteNode(nodeId);
      return;
    }
    setPendingCanvasDeleteNodeId(impact.nodeId);
  }, [deleteNode, inspectorGraph]);

  const deleteEdge = useCallback((edgeId: string) => {
    const source = candidate ? candidateGraph : committedGraph;
    const nextGraph = {
      ...source,
      edges: (source.edges || []).filter((edge) => edgeIdentity(edge) !== edgeId),
    };
    if (candidate) {
      reviseCandidateGraph(candidateGraph, nextGraph);
      return;
    }
    submitGraphChange(nextGraph);
  }, [candidate, candidateGraph, committedGraph, reviseCandidateGraph, submitGraphChange]);

  const applyNode = useCallback((node: RunnerNode) => {
    submitInspectorGraphChange({
      ...inspectorGraph,
      nodes: (inspectorGraph.nodes || []).map((current) => current.id === node.id ? node : current),
    });
  }, [inspectorGraph, submitInspectorGraphChange]);

  const syncWorkspaceNodeSelection = useCallback((nodeId: string) => {
    const normalized = nodeId.trim();
    workspace.setSelection(normalized
      ? { nodeIds: [normalized], edgeIds: [], anchorId: normalized }
      : { nodeIds: [], edgeIds: [] });
  }, [workspace]);

  const selectNode = useCallback((nodeId: string) => {
    setSurfaceFocus(undefined);
    syncWorkspaceNodeSelection(nodeId);
    onSelectNode(nodeId);
  }, [onSelectNode, syncWorkspaceNodeSelection]);

  const openNodeConfiguration = useCallback((nodeId: string) => {
    setSurfaceFocus(undefined);
    syncWorkspaceNodeSelection(nodeId);
    onOpenNodeConfig(nodeId);
  }, [onOpenNodeConfig, syncWorkspaceNodeSelection]);

  const closeNodeConfiguration = useCallback(() => {
    const nodeId = effectiveSelectedNodeId;
    setSurfaceFocus(undefined);
    workspace.setSelection({ nodeIds: [], edgeIds: [] });
    onSelectNode("");
    window.requestAnimationFrame(() => {
      if (!nodeId) return;
      const target = [...document.querySelectorAll<HTMLElement>('[data-testid^="canvas-node-"]')]
        .find((element) => element.dataset.testid === `canvas-node-${nodeId}`);
      if (!target) return;
      if (target.tabIndex < 0) target.tabIndex = 0;
      target.focus({ preventScroll: true });
    });
  }, [effectiveSelectedNodeId, onSelectNode, workspace]);

  const focusSurfaceObject = useCallback((detail: WorkflowSurfaceFocusDetail, broadcast: boolean) => {
    const nodeId = "nodeId" in detail.target ? detail.target.nodeId : undefined;
    const edgeId = "edgeId" in detail.target ? detail.target.edgeId : undefined;
    if (nodeId && !(inspectorGraph.nodes || []).some((node) => node.id === nodeId)) return;
    if (edgeId && !(inspectorGraph.edges || []).some((edge) => edgeIdentity(edge) === edgeId)) return;
    if (nodeId) {
      syncWorkspaceNodeSelection(nodeId);
      onSelectNode(nodeId);
    } else if (edgeId) {
      workspace.setSelection({ nodeIds: [], edgeIds: [edgeId], anchorId: edgeId });
    }
    setSurfaceFocus(detail);
    if (detail.source !== "review") setReviewFocus(detail);
    if (detail.source !== "review") requestWorkflowContextDockTab("changes");
    if (broadcast) dispatchWorkflowSurfaceFocus(detail);
  }, [inspectorGraph.edges, inspectorGraph.nodes, onSelectNode, syncWorkspaceNodeSelection, workspace]);

  const focusCandidateObject = useCallback((
    target: WorkflowSurfaceFocusTarget,
    source: "review" | "canvas",
    preferredIndex?: number,
  ) => {
    if (!candidate) return false;
    const operationIndex = resolveWorkflowOperationIndex(candidate.changeSet.operations, target, preferredIndex);
    if (operationIndex === undefined) return false;
    focusSurfaceObject(createWorkflowSurfaceFocusDetail({
      source,
      workflowId: candidate.candidate.workflowKey.workflowId,
      target,
      candidate: workflowSurfaceCandidate(candidate),
      operationIndex,
    }), true);
    return true;
  }, [candidate, focusSurfaceObject]);

  const selectCanvasNode = useCallback((nodeId: string) => {
    const normalized = nodeId.trim();
    if (normalized && focusCandidateObject({ nodeId: normalized }, "canvas")) return;
    selectNode(normalized);
    if (normalized) requestWorkflowContextDockTab("configuration");
  }, [focusCandidateObject, selectNode]);

  useEffect(() => {
    const handleSurfaceFocus = (event: Event) => {
      const detail = readWorkflowSurfaceFocusDetail(event);
      const workflowId = workspace.state.committed?.workflowKey.workflowId || workspace.state.workflowKey?.workflowId;
      if (!detail || detail.source !== "chat" || !workflowId || detail.workflowId !== workflowId) return;
      if (detail.candidate) {
        if (!candidate || !workflowSurfaceCandidateMatches(workflowSurfaceCandidate(candidate), detail.candidate)) return;
        const operationIndex = resolveWorkflowOperationIndex(candidate.changeSet.operations, detail.target, detail.operationIndex);
        if (operationIndex === undefined) return;
        focusSurfaceObject({ ...detail, operationIndex }, false);
        return;
      }
      focusSurfaceObject(detail, false);
    };
    window.addEventListener(WORKFLOW_SURFACE_FOCUS_EVENT, handleSurfaceFocus);
    return () => window.removeEventListener(WORKFLOW_SURFACE_FOCUS_EVENT, handleSurfaceFocus);
  }, [candidate, focusSurfaceObject, workspace.state.committed?.workflowKey.workflowId, workspace.state.workflowKey?.workflowId]);

  useEffect(() => {
    if (workspace.pendingCommands.length > 0) {
      observedPendingRef.current = true;
      return;
    }
    if (!observedPendingRef.current || workspace.unsavedError) return;
    observedPendingRef.current = false;
    onCommandStateChange?.({ status: "reconciled", message: "已由服务端确认" });
  }, [onCommandStateChange, workspace.pendingCommands.length, workspace.unsavedError]);

  const nodeInspector = selectedNode && renderNodeInspector
    ? renderNodeInspector({
      graph: inspectorGraph,
      node: selectedNode,
      readOnly: Boolean(candidate && !candidateCanRevise),
      onApplyNode: applyNode,
      onDeleteNode: deleteNode,
      onCloseNode: closeNodeConfiguration,
    })
    : undefined;
  const candidateActivation = candidate ? {
    tab: "changes" as const,
    key: `${candidate.candidate.id}:${candidate.candidate.currentRevision}:${candidate.changeSet.previewHash}`,
  } : undefined;
  const focusReviewObject = useCallback((location: WorkflowValidationLocation) => {
    if (location.nodeId) {
      selectNode(location.nodeId);
      if (location.field) requestWorkflowContextDockTab("configuration");
      return;
    }
    if (location.edgeId) {
      workspace.setSelection({ nodeIds: [], edgeIds: [location.edgeId], anchorId: location.edgeId });
    }
  }, [selectNode, workspace]);
  const lifecycleActionHandlers = useMemo<WorkflowStudioActionHandlers>(() => ({
    ...(onCheckWorkflow ? {
      check: (target) => onCheckWorkflow(assertLifecycleTarget(target, "saved_draft")),
    } : {}),
    ...(onDryRunWorkflow ? {
      dry_run: (target) => onDryRunWorkflow(assertLifecycleTarget(target, "draft_non_production")),
    } : {}),
    ...(onPublishWorkflow ? {
      publish: (target) => onPublishWorkflow(assertLifecycleTarget(target, "saved_draft")),
      publish_changes: (target) => onPublishWorkflow(assertLifecycleTarget(target, "saved_draft")),
    } : {}),
    ...(onRunPublishedWorkflow ? {
      run_published: (target) => onRunPublishedWorkflow(assertLifecycleTarget(target, "published_revision")),
    } : {}),
    fix_issues: () => requestWorkflowContextDockTab("changes"),
  }), [onCheckWorkflow, onDryRunWorkflow, onPublishWorkflow, onRunPublishedWorkflow]);
  const lifecycleActionCapabilities = useMemo<WorkflowStudioActionCapabilities>(() => ({
    "workflow.validate": Boolean(onCheckWorkflow),
    "workflow.draft_dry_run": Boolean(onDryRunWorkflow),
    "workflow.publish": Boolean(onPublishWorkflow),
    "workflow.run_published": Boolean(onRunPublishedWorkflow),
    "workflow.review_issues": true,
    "workflow.configure_environment": false,
  }), [onCheckWorkflow, onDryRunWorkflow, onPublishWorkflow, onRunPublishedWorkflow]);
  const lifecycleActionRegistry = useMemo(
    () => createWorkflowStudioActionRegistry({
      capabilities: lifecycleActionCapabilities,
      handlers: lifecycleActionHandlers,
    }),
    [lifecycleActionCapabilities, lifecycleActionHandlers],
  );
  const canHandleLifecycleAction = useCallback(
    (action: WorkflowLifecycleAction) => lifecycleActionRegistry.canExecute(action),
    [lifecycleActionRegistry],
  );
  const handleLifecycleAction = useCallback((action: WorkflowLifecycleAction) => {
    void lifecycleActionRegistry.execute(action).catch((error) => {
      onCommandStateChange?.({ status: "failed", message: workflowCommandErrorMessage(error) });
    });
  }, [lifecycleActionRegistry, onCommandStateChange]);
  const savedChanges = (
    <WorkflowHistoryPanel
      entries={durableHistoryEntries}
      pending={workspace.historyPending}
      error={workspace.historyError}
      onUndo={onUndoHistory || workspace.undoHistory}
      onRedo={onRedoHistory || workspace.redoHistory}
      onResolveConflict={onResolveHistoryConflict}
      onDismissError={workspace.clearHistoryError}
    />
  );
  const newestUndoTransactionId = useMemo(
    () => durableHistoryEntries
      .filter((entry) => entry.source === "durable_transaction" && entry.reversible)
      .sort((left, right) => Date.parse(right.occurredAt) - Date.parse(left.occurredAt))[0]?.transactionId,
    [durableHistoryEntries],
  );
  const newestRedoTransactionId = useMemo(
    () => durableHistoryEntries
      .filter((entry) => entry.source === "durable_transaction" && entry.redoable)
      .sort((left, right) => Date.parse(right.occurredAt) - Date.parse(left.occurredAt))[0]?.transactionId,
    [durableHistoryEntries],
  );
  const review = (
    <div className="workflow-context-review-stack">
      {candidate ? (
        <WorkflowChangeReview
          key={`${candidate.candidate.id}:${candidate.candidate.currentRevision}:${reviewFocus?.requestId || "default"}`}
          changeSet={candidate.changeSet}
          fieldDiffs={reviewFieldDiffs}
          initialOperationIndex={reviewFocus?.operationIndex}
          savedChanges={savedChanges}
          onFocusNode={(nodeId) => focusCandidateObject({ nodeId }, "review")}
          onFocusEdge={(edgeId) => focusCandidateObject({ edgeId }, "review")}
        />
      ) : savedChanges}
      {candidateReviewPanel}
      {candidate ? aiTransportPanel ? (
        <WorkflowCandidateWorkspaceActionBar />
      ) : (
        <div data-testid="workflow-candidate-offline-actions">
          <p className="border-t border-amber-200 bg-amber-50 px-4 py-3 text-sm leading-6 text-amber-950" role="status">
            当前没有可验证的确认步骤；应用操作已安全禁用，你仍可放弃候选修改。
          </p>
          <WorkflowCandidateActionBar
            binding={{
              candidateId: candidate.candidate.id,
              changeSetId: candidate.changeSet.id,
              previewHash: candidate.changeSet.previewHash,
              expectedRevision: candidate.changeSet.base.resourceVersion,
            }}
            approval={{
              status: "ready",
              binding: {
                candidateId: candidate.candidate.id,
                changeSetId: candidate.changeSet.id,
                previewHash: candidate.changeSet.previewHash,
                expectedRevision: candidate.changeSet.base.resourceVersion,
              },
            }}
            changeCount={candidate.changeSet.operations.length}
            onDiscard={() => workspace.discardCandidate()}
          />
        </div>
      ) : null}
      {workspace.candidateActionError ? (
        <p className="border border-rose-200 bg-rose-50 px-4 py-3 text-sm leading-6 text-rose-950" role="alert" data-testid="workflow-candidate-action-error">
          {workspace.candidateActionError.message}。已保存的工作流未发生修改。
        </p>
      ) : null}
      <WorkflowValidationPanel state={workspace.state} onFocusIssue={focusReviewObject} />
    </div>
  );
  const canvas = (
    <section className="runner-studio-main">
      <section className="runner-studio-canvas" aria-label="工作流画布" data-testid="runner-studio-canvas">
        {pendingCanvasDelete ? (
          <WorkflowDeleteConfirmation
            impact={pendingCanvasDelete}
            onCancel={() => setPendingCanvasDeleteNodeId(undefined)}
            onConfirm={() => {
              const nodeId = pendingCanvasDelete.nodeId;
              setPendingCanvasDeleteNodeId(undefined);
              deleteNode(nodeId);
            }}
          />
        ) : null}
        <RunnerCanvas
          graph={committedGraph}
          candidate={candidate}
          committedResourceVersion={workspace.state.committed?.revision.resourceVersion}
          actions={actions}
          runState={runState}
          focusNodeId={effectiveFocusNodeId}
          selectedNodeId={effectiveSelectedNodeId}
          aiHighlightedNodeIds={aiHighlightedNodeIds}
          fullscreen={fullscreen}
          onUpdateGraph={submitGraphChange}
          onReviseCandidateGraph={candidateCanRevise ? reviseCandidateGraph : undefined}
          onSelectNode={selectCanvasNode}
          onOpenNodeConfig={openNodeConfiguration}
          onDeleteNode={requestCanvasDeleteNode}
          onDeleteEdge={onCanvasDeleteEdge || deleteEdge}
          onUndo={onCanvasUndo}
          onRedo={onCanvasRedo}
          onNodeAction={ignoreNodeAction}
          onToggleFullscreen={onToggleFullscreen}
        />
      </section>
    </section>
  );

  const lifecycleHeader = (
    <WorkflowLifecycleHeader
      placement={lifecycleHeaderTarget === undefined ? "workspace" : "topbar"}
      state={workspace.state}
      facts={runGateFacts}
      title={workflowTitle}
      environment={workflowEnvironment || runGateFacts?.environment.name}
      saveState={workspace.unsavedError ? "unsaved" : workspace.pendingCommands.length ? "applying" : saveState}
      canHandleAction={canHandleLifecycleAction}
      onAction={handleLifecycleAction}
      onBack={onBack}
      onUndo={newestUndoTransactionId
        ? () => void Promise.resolve((onUndoHistory || workspace.undoHistory)(newestUndoTransactionId)).catch(() => undefined)
        : onCanvasUndo}
      onRedo={newestRedoTransactionId
        ? () => void Promise.resolve((onRedoHistory || workspace.redoHistory)(newestRedoTransactionId)).catch(() => undefined)
        : onCanvasRedo}
      onOpenWorkbench={() => requestWorkflowContextDockTab(aiTransportPanel ? "ai" : "changes")}
      menuItems={commandMenuItems}
    />
  );

  return (
    <>
      {lifecycleHeaderTarget ? createPortal(lifecycleHeader, lifecycleHeaderTarget) : null}
      <div className={`runner-studio-workspace ${lifecycleHeaderTarget === undefined ? "with-lifecycle-header" : ""} ${nodeInspector ? "with-node-panel" : ""}`}>
      {lifecycleHeaderTarget === undefined ? lifecycleHeader : null}
      <WorkflowContextDock
        configurationFocusTargetId={effectiveSelectedNodeId}
        canvas={canvas}
        slots={{
          ai: aiTransportPanel,
          configuration: nodeInspector,
          changes: review,
          run: (
            <WorkflowRunPanel
              state={workspace.state}
              facts={runGateFacts}
              onRunPublished={onRunPublishedWorkflow}
              sessionRuns={sessionRuns}
              onRetrySessionRun={onRetrySessionRun}
            />
          ),
        }}
        activation={candidateActivation}
      />
      </div>
    </>
  );
}

/**
 * Derives readable field-level evidence from the same immutable operations used
 * to project the Candidate canvas. Values are selected only from the affected
 * object/field; the Review component performs the final recursive redaction.
 */
export function workflowReviewFieldDiffs(
  changeSet: WorkflowChangeSetV2,
  committed: RunnerGraph,
  projected: RunnerGraph,
): WorkflowReviewFieldDiff[] {
  const beforeNodes = new Map((committed.nodes || []).map((node) => [node.id, node]));
  const afterNodes = new Map((projected.nodes || []).map((node) => [node.id, node]));
  const beforeEdges = new Map((committed.edges || []).flatMap((edge) => edge.id ? [[edge.id, edge] as const] : []));
  const afterEdges = new Map((projected.edges || []).flatMap((edge) => edge.id ? [[edge.id, edge] as const] : []));
  const result: WorkflowReviewFieldDiff[] = [];
  const pushObject = (operationIndex: number, prefix: string, before: unknown, after: unknown) => {
    const keys = new Set([...objectKeys(before), ...objectKeys(after)]);
    for (const key of [...keys].sort()) {
      const previous = recordValue(before, key);
      const next = recordValue(after, key);
      if (stableReviewValue(previous) === stableReviewValue(next)) continue;
      result.push({
        operationIndex,
        path: `${prefix}/${escapeJsonPointer(key)}`,
        ...(toWorkflowJsonValue(previous) !== undefined ? { before: toWorkflowJsonValue(previous) } : {}),
        ...(toWorkflowJsonValue(next) !== undefined ? { after: toWorkflowJsonValue(next) } : {}),
      });
    }
  };

  changeSet.operations.forEach((operation, operationIndex) => {
    switch (operation.op) {
      case "add_node": {
        const id = operation.node.id || operation.clientRef;
        pushObject(operationIndex, `/nodes/${escapeJsonPointer(id)}`, undefined, afterNodes.get(id) || operation.node);
        break;
      }
      case "update_node":
        pushObject(
          operationIndex,
          `/nodes/${escapeJsonPointer(operation.nodeId)}`,
          pickReviewFields(beforeNodes.get(operation.nodeId), Object.keys(operation.set)),
          pickReviewFields(afterNodes.get(operation.nodeId) || operation.set, Object.keys(operation.set)),
        );
        break;
      case "delete_node":
        pushObject(operationIndex, `/nodes/${escapeJsonPointer(operation.nodeId)}`, beforeNodes.get(operation.nodeId), undefined);
        break;
      case "add_edge": {
        const id = operation.edge.id || operation.clientRef || `${operation.edge.source}-${operation.edge.target}`;
        pushObject(operationIndex, `/edges/${escapeJsonPointer(id)}`, undefined, (operation.edge.id ? afterEdges.get(operation.edge.id) : undefined) || operation.edge);
        break;
      }
      case "update_edge":
        pushObject(
          operationIndex,
          `/edges/${escapeJsonPointer(operation.edgeId)}`,
          pickReviewFields(beforeEdges.get(operation.edgeId), Object.keys(operation.set)),
          pickReviewFields(afterEdges.get(operation.edgeId) || operation.set, Object.keys(operation.set)),
        );
        break;
      case "delete_edge":
        pushObject(operationIndex, `/edges/${escapeJsonPointer(operation.edgeId)}`, beforeEdges.get(operation.edgeId), undefined);
        break;
      case "update_workflow":
        pushObject(operationIndex, "/workflow", pickReviewFields(committed.workflow, Object.keys(operation.set)), pickReviewFields(projected.workflow || operation.set, Object.keys(operation.set)));
        break;
      case "update_layout":
        operation.layout.nodes.forEach(({ nodeId, position }) => {
          pushObject(operationIndex, `/nodes/${escapeJsonPointer(nodeId)}/position`, beforeNodes.get(nodeId)?.position, afterNodes.get(nodeId)?.position || position);
        });
        if (operation.layout.viewport) pushObject(operationIndex, "/layout/viewport", recordValue(committed.layout, "viewport"), operation.layout.viewport);
        break;
      case "attach_manual_binding":
        result.push({ operationIndex, path: "/workflow/manualBinding", after: toWorkflowJsonValue(operation.manualRef) });
        break;
    }
  });
  return result.slice(0, 2_000);
}

function objectKeys(value: unknown): string[] {
  return value && typeof value === "object" && !Array.isArray(value) ? Object.keys(value) : [];
}

function recordValue(value: unknown, key: string): unknown {
  return value && typeof value === "object" && !Array.isArray(value) ? (value as Record<string, unknown>)[key] : undefined;
}

function pickReviewFields(value: unknown, keys: string[]): Record<string, unknown> {
  return Object.fromEntries(keys.map((key) => [key, recordValue(value, key)]));
}

function toWorkflowJsonValue(value: unknown): WorkflowJsonValue | undefined {
  if (value === undefined) return undefined;
  try {
    return JSON.parse(JSON.stringify(value)) as WorkflowJsonValue;
  } catch {
    return undefined;
  }
}

function stableReviewValue(value: unknown): string {
  const normalized = toWorkflowJsonValue(value);
  return normalized === undefined ? "__undefined__" : JSON.stringify(normalized);
}

function assertLifecycleTarget<Kind extends WorkflowPrimaryActionTarget["kind"]>(
  target: WorkflowPrimaryActionTarget,
  kind: Kind,
): Extract<WorkflowPrimaryActionTarget, { kind: Kind }> {
  if (target.kind !== kind) {
    throw new TypeError(`workflow action target must be ${kind}`);
  }
  return target as Extract<WorkflowPrimaryActionTarget, { kind: Kind }>;
}

function escapeJsonPointer(value: string): string {
  return value.replaceAll("~", "~0").replaceAll("/", "~1");
}

function workflowSurfaceCandidate(
  candidate: NonNullable<ReturnType<typeof useWorkflowWorkspace>["state"]["candidate"]>,
): WorkflowSurfaceFocusCandidate {
  return {
    candidateId: candidate.candidate.id,
    changeSetId: candidate.changeSet.id,
    previewHash: candidate.changeSet.previewHash,
    expectedRevision: candidate.changeSet.base.resourceVersion,
  };
}

function workflowSurfaceCandidateMatches(
  left: WorkflowSurfaceFocusCandidate,
  right: WorkflowSurfaceFocusCandidate,
) {
  return left.candidateId === right.candidateId
    && left.changeSetId === right.changeSetId
    && left.previewHash === right.previewHash
    && left.expectedRevision === right.expectedRevision;
}

export function workflowCommandForGraphChange(current: RunnerGraph, next: RunnerGraph): WorkflowManualCommand {
  const commands = workflowCommandsForGraphChange(current, next);
  if (commands.length !== 1) throw new Error("canvas change must map to a single typed workflow command");
  return commands[0];
}

export function workflowCommandsForGraphChange(current: RunnerGraph, next: RunnerGraph): WorkflowManualCommand[] {
  if (!jsonEqual(current.workflow || {}, next.workflow || {}) || !jsonEqual(current.layout, next.layout)) {
    throw new Error("canvas workflow metadata changes are not supported by manual commands");
  }
  const currentNodes = new Map((current.nodes || []).map((node) => [node.id, node]));
  const nextNodes = new Map((next.nodes || []).map((node) => [node.id, node]));
  const currentEdges = new Map((current.edges || []).map((edge) => [edgeIdentity(edge), edge]));
  const nextEdges = new Map((next.edges || []).map((edge) => [edgeIdentity(edge), edge]));
  const addedNodes = [...nextNodes].filter(([id]) => !currentNodes.has(id));
  const deletedNodes = [...currentNodes].filter(([id]) => !nextNodes.has(id));
  const addedEdges = [...nextEdges].filter(([id]) => !currentEdges.has(id));
  const deletedEdges = [...currentEdges].filter(([id]) => !nextEdges.has(id));
  const changedNodes = [...nextNodes].flatMap(([id, node]) => {
    const before = currentNodes.get(id);
    if (!before || jsonEqual(editableNode(before), editableNode(node))) return [];
    return [{ before, node }];
  });
  const changedEdges = [...nextEdges].flatMap(([id, edge]) => {
    const before = currentEdges.get(id);
    if (!before || jsonEqual(editableEdge(before), editableEdge(edge))) return [];
    return [{ before, edge }];
  });
  const commands: WorkflowManualCommand[] = [];
  const deletedNodeIds = new Set(deletedNodes.map(([id]) => id));

  for (const [edgeId, edge] of deletedEdges) {
    if (deletedNodeIds.has(String(edge.source || "")) || deletedNodeIds.has(String(edge.target || ""))) continue;
    commands.push({ kind: "disconnect_edge", edgeId });
  }
  for (const [nodeId] of deletedNodes) {
    commands.push({ kind: "delete_node", nodeId, cascadeEdges: true });
  }
  for (const [, node] of addedNodes) {
    commands.push({ kind: "add_node", node: editableNode(node) });
  }

  const moves: Array<{ nodeId: string; position: { x: number; y: number } }> = [];
  for (const { before, node } of changedNodes) {
    const beforeValue = editableNode(before);
    const nextValue = editableNode(node);
    const nodePatch = changedFields(beforeValue, nextValue, ["id", "position"]);
    if (Object.keys(nodePatch).length > 0) {
      commands.push({ kind: "update_node", nodeId: node.id, nodePatch });
    }
    if (!jsonEqual(beforeValue.position, nextValue.position)) {
      const position = nextValue.position as { x?: unknown; y?: unknown } | undefined;
      if (!position || !Number.isFinite(position.x) || !Number.isFinite(position.y)) {
        throw new Error(`canvas node ${node.id} move requires a finite position`);
      }
      moves.push({ nodeId: node.id, position: { x: Number(position.x), y: Number(position.y) } });
    }
  }
  for (const { before, edge } of changedEdges) {
    const edgePatch = changedFields(editableEdge(before), editableEdge(edge), ["id"]);
    if (Object.keys(edgePatch).length > 0) {
      commands.push({ kind: "update_edge", edgeId: edgeIdentity(edge), edgePatch });
    }
  }
  for (const [, edge] of addedEdges) {
    commands.push({ kind: "connect_edge", edge: editableEdge(edge) });
  }
  if (moves.length > 0) commands.push({ kind: "move", layout: { nodes: moves } });
  if (commands.length === 0) throw new Error("canvas change contains no supported workflow command");
  return commands;
}

function commandOptions(commands: readonly WorkflowManualCommand[]): WorkflowMutationOptions {
  const serialized = JSON.stringify(commands);
  const kind = commands.length === 1 ? commands[0].kind : "batch";
  let hash = 5381;
  for (let index = 0; index < serialized.length; index += 1) hash = ((hash << 5) + hash) ^ serialized.charCodeAt(index);
  return {
    reason: `manual canvas ${kind}`,
    actionId: `${kind}-${(hash >>> 0).toString(16)}`,
    ...(commands.some((command) => command.kind === "delete_node" || command.kind === "disconnect_edge") ? { confirmed: true } : {}),
  };
}

function editableNode(node: RunnerNode): Record<string, unknown> {
  return defined({
    id: node.id,
    type: node.type,
    position: node.position ? { x: Number(node.position.x), y: Number(node.position.y) } : undefined,
    stepName: node.stepName ?? node.step_name,
    stepId: node.stepId ?? node.step_id,
    step: node.step,
    handlerName: node.handlerName ?? node.handler_name,
    handler: node.handler,
    approval: node.approval,
    condition: node.condition,
    subflow: node.subflow,
    aggregator: node.aggregator,
    join: node.join,
    loop: node.loop,
    parentId: node.parentId ?? node.parent_id,
    label: node.label,
    collapsed: node.collapsed,
    ports: Array.isArray(node.ports) ? node.ports.map((port) => defined({ id: port.id, type: port.type, label: port.label })) : node.ports,
    inputs: node.inputs,
    outputs: node.outputs,
  });
}

function editableEdge(edge: RunnerEdge): Record<string, unknown> {
  return defined({
    id: edgeIdentity(edge),
    source: edge.source,
    sourcePort: edge.sourcePort ?? edge.source_port ?? edge.sourceHandle,
    target: edge.target,
    targetPort: edge.targetPort ?? edge.target_port ?? edge.targetHandle,
    kind: edge.kind,
    condition: edge.condition,
  });
}

function edgeIdentity(edge: RunnerEdge): string {
  return String(edge.id || `${edge.source}-${edge.target}-${edge.kind || edge.source_port || "next"}`);
}

function changedFields(before: Record<string, unknown>, next: Record<string, unknown>, ignored: string[]): Record<string, unknown> {
  const result: Record<string, unknown> = {};
  for (const key of new Set([...Object.keys(before), ...Object.keys(next)])) {
    if (ignored.includes(key) || jsonEqual(before[key], next[key])) continue;
    if (next[key] === undefined) throw new Error(`clearing ${key} is not supported by the typed workflow command`);
    result[key] = cloneJson(next[key]);
  }
  return result;
}

function defined(value: Record<string, unknown>): Record<string, unknown> {
  return Object.fromEntries(Object.entries(value).filter(([, item]) => item !== undefined));
}

function jsonEqual(left: unknown, right: unknown): boolean {
  return JSON.stringify(left) === JSON.stringify(right);
}

function cloneJson(value: unknown): unknown {
  return value === undefined ? undefined : JSON.parse(JSON.stringify(value));
}

function runnerGraph(value: unknown): RunnerGraph | null {
  if (!value || typeof value !== "object" || Array.isArray(value)) return null;
  const graph = value as RunnerGraph;
  return Array.isArray(graph.nodes) && Array.isArray(graph.edges) ? graph : null;
}

function workflowCommandErrorMessage(error: unknown): string {
  if (error instanceof Error && error.message) return error.message;
  return "服务端未保存本次修改";
}
