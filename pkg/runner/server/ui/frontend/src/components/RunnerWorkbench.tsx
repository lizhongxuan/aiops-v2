import { useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import ActionCatalog from "./ActionCatalog";
import NewWorkflowModal from "./NewWorkflowModal";
import PropertyPanel from "./PropertyPanel";
import RunDrawer from "./RunDrawer";
import WorkflowCanvas from "./WorkflowCanvas";
import YamlPreviewModal from "./YamlPreviewModal";
import { useGraphStore } from "../stores/graphStore";
import { buildGraphDiffSummary } from "../utils/graphDiff";
import type { WorkflowGraph } from "../types/workflow";

type NewWorkflowMode = "cmd-run-basic" | "shell-run-basic" | "manual-approval-basic" | "from-yaml" | "clone-current";

export default function RunnerWorkbench() {
  const store = useGraphStore();
  useSyncExternalStore(store.subscribe, () => store.state, () => store.state);
  const [, forceRender] = useState(0);
  const [yamlPreviewOpen, setYamlPreviewOpen] = useState(false);
  const [historyOpen, setHistoryOpen] = useState(false);
  const [newWorkflowOpen, setNewWorkflowOpen] = useState(false);
  const [newWorkflowInitialMode, setNewWorkflowInitialMode] = useState<NewWorkflowMode>("cmd-run-basic");
  const [switchConfirmOpen, setSwitchConfirmOpen] = useState(false);
  const [pendingWorkflowName, setPendingWorkflowName] = useState("");
  const bundleFileInput = useRef<HTMLInputElement | null>(null);

  useEffect(() => store.subscribe(() => forceRender((value) => value + 1)), [store]);
  useEffect(() => {
    void store.load();
  }, []);

  const workflowTitle = store.state.graph?.workflow.name || "Runner";
  const workflowVersionText = store.state.graph?.workflow.version || "v0.1";
  const graphWithRunState = store.graphWithRunState.value;
  const selectedNode = store.selectedNode.value;
  const diffSummary = useMemo(() => buildGraphDiffSummary(store.state.baselineGraph, store.state.graph), [store.state.baselineGraph, store.state.graph]);
  const executionSemanticsChanged = store.executionSemanticsChanged.value;
  const workflowSelectOptions = [...store.state.workflowOptions]
    .sort((a, b) => (b.updated_at || "").localeCompare(a.updated_at || "") || a.name.localeCompare(b.name))
    .map((workflow) => ({ label: [workflow.name, workflow.version, workflow.status].filter(Boolean).join(" · "), value: workflow.name }));

  async function exportBundle() {
    const bundle = await store.exportWorkflowBundle();
    if (!bundle) return;
    const blob = new Blob([JSON.stringify(bundle, null, 2)], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = `${safeBundleFileName(bundle.name || workflowTitle)}.workflow-bundle.json`;
    document.body.appendChild(anchor);
    anchor.click();
    anchor.remove();
    URL.revokeObjectURL(url);
  }

  async function importBundleFile(event: React.ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file) return;
    await store.importWorkflowBundle(await file.text());
  }

  function openNewWorkflow(mode: NewWorkflowMode = "cmd-run-basic") {
    setNewWorkflowInitialMode(mode);
    setNewWorkflowOpen(true);
  }

  async function createWorkflow(payload: { graph: WorkflowGraph; labels?: Record<string, string>; saveNote?: string }) {
    await store.createWorkflowFromGraph(payload.graph, { labels: payload.labels, saveNote: payload.saveNote });
    if (!store.state.error) setNewWorkflowOpen(false);
  }

  function requestWorkflowSwitch(name: string) {
    if (!name || name === store.state.graph?.workflow.name) return;
    if (store.state.dirty) {
      setPendingWorkflowName(name);
      setSwitchConfirmOpen(true);
      return;
    }
    void store.switchWorkflow(name);
  }

  async function confirmWorkflowSwitch() {
    const name = pendingWorkflowName;
    setSwitchConfirmOpen(false);
    setPendingWorkflowName("");
    if (name) await store.switchWorkflow(name, { force: true });
  }

  return (
    <div className="workbench-shell">
      <header className="topbar">
        <div className="brand-block"><div className="app-mark">R</div><div><h1>{workflowTitle}</h1><p>Runner visual workflow editor</p></div></div>
        <div className="topbar-actions">
          <select className="workflow-select" value={store.state.graph?.workflow.name || ""} disabled={store.state.loading || store.state.switchingWorkflow} onChange={(event) => requestWorkflowSwitch(event.target.value)}>
            {workflowSelectOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
          </select>
          <button type="button" disabled={store.state.creatingWorkflow} onClick={() => openNewWorkflow("cmd-run-basic")}>New</button>
          <button type="button" disabled={!store.state.graph} onClick={() => openNewWorkflow("clone-current")}>Clone</button>
          <span className={`tag ${store.state.offline ? "tag-warning" : "tag-success"}`}>{store.state.offline ? "Mock data" : "API connected"}</span>
          <span className="tag">{workflowVersionText}</span>
          <span className="tag">{store.state.workflowStatus}</span>
          <span className="tag">{store.state.dirty ? "unsaved" : "saved"}</span>
          {store.state.run.runId ? <span className="tag">{store.state.run.status}</span> : null}
          <span className={`tag ${store.state.validation?.valid ? "tag-success" : store.state.validation ? "tag-error" : ""}`}>{store.state.validation ? (store.state.validation.valid ? "valid" : "invalid") : "not validated"}</span>
          <button type="button" disabled={store.state.loading} onClick={() => store.load()}>Reload</button>
          <input className="save-note-input" value={store.state.saveNote} placeholder="Save note" maxLength={160} onChange={(event) => { store.state.saveNote = event.target.value; }} />
          <button type="button" disabled={!store.state.graph || store.state.saving} onClick={() => store.saveDraft()}>Save draft</button>
          <button type="button" disabled={!store.state.graph || store.state.dirty || store.state.publishing} onClick={() => store.publishWorkflow()}>Publish</button>
          <button type="button" disabled={!store.state.graph || store.state.validating} onClick={() => store.validateGraph()}>Validate</button>
          <button type="button" disabled={!store.state.graph} onClick={() => setYamlPreviewOpen(true)}>YAML</button>
          {executionSemanticsChanged ? <label className="risk-ack"><input type="checkbox" checked={store.state.semanticChangeAcknowledged} onChange={(event) => { store.state.semanticChangeAcknowledged = event.target.checked; }} /> Execution reviewed</label> : null}
          <button type="button" disabled={!store.state.graph || store.state.exportingBundle} onClick={exportBundle}>Export</button>
          <button type="button" disabled={store.state.importingBundle} onClick={() => bundleFileInput.current?.click()}>Import Bundle</button>
          <input ref={bundleFileInput} type="file" accept="application/json,.json" hidden onChange={importBundleFile} />
          <button type="button" disabled={!store.state.graph || store.state.loadingVersions} onClick={() => { setHistoryOpen(true); void store.loadWorkflowVersions(); }}>History</button>
          <button type="button" disabled={!store.state.graph || store.state.dryRunning} onClick={() => store.dryRunGraph()}>Dry run</button>
          {store.state.validation?.warnings?.length || store.state.dryRun?.warnings?.length ? <label className="risk-ack"><input type="checkbox" checked={store.state.warningAcknowledged} onChange={(event) => { store.state.warningAcknowledged = event.target.checked; }} /> Warnings reviewed</label> : null}
          <button type="button" disabled={!store.state.graph || store.state.submitting} onClick={() => store.submitRun()}>Run</button>
          <button type="button" disabled={!store.state.run.runId || store.state.canceling} onClick={() => store.cancelRun()}>Cancel</button>
        </div>
      </header>

      {store.state.error ? <div className="global-error">{store.state.error}</div> : null}
      <main className="workspace-grid">
        <ActionCatalog actions={store.state.actions} onAddAction={(action) => store.addActionNodeFromCatalog(action)} onAddControlNode={(type) => store.addControlNode(type)} />
        <WorkflowCanvas
          graph={graphWithRunState}
          selectedNodeId={store.state.selectedNodeId}
          canUndo={store.state.historyPast.length > 0}
          canRedo={store.state.historyFuture.length > 0}
          canPaste={Boolean(store.state.clipboardNode)}
          onSelectNode={store.selectNode}
          onAddAction={store.addActionNodeFromCatalog}
          onAddControlNode={store.addControlNode}
          onUpdateNodePosition={(nodeId, position) => store.updateNode(nodeId, { position })}
          onConnectNodes={store.connectNodes}
          onDeleteSelected={store.deleteSelectedNode}
          onCopySelected={store.copySelectedNode}
          onPasteNode={store.pasteNode}
          onUndo={store.undo}
          onRedo={store.redo}
          onAutoLayout={store.autoLayout}
        />
        <PropertyPanel
          node={selectedNode}
          actions={store.state.actions}
          workflow={store.state.graph?.workflow || null}
          workflows={store.state.workflowOptions}
          diffSummary={diffSummary}
          onUpdateNode={store.updateNode}
          onUpdateWorkflow={store.updateWorkflow}
        />
      </main>
      <RunDrawer
        run={store.state.run}
        validation={store.state.validation}
        dryRun={store.state.dryRun}
        error={store.state.error}
        eventConnected={store.state.eventConnected}
        replaying={store.state.replaying}
        approvalNodes={store.waitingApprovalNodes.value}
        resolvingApprovalNodeId={store.state.resolvingApprovalNodeId}
        resolvingApprovalAction={store.state.resolvingApprovalAction}
        onReplayRun={store.replayRunHistory}
        onApproveNode={store.approveNode}
        onRejectNode={store.rejectNode}
      />

      <YamlPreviewModal
        show={yamlPreviewOpen}
        graph={store.state.graph}
        baselineGraph={store.state.baselineGraph}
        yamlPreview={store.state.yamlPreview}
        previewCompiling={store.state.previewCompiling}
        onClose={() => setYamlPreviewOpen(false)}
        onReplaceGraph={store.replaceGraph}
        onCompileYaml={store.compilePreview}
        onParseYaml={store.importGraphYAML}
      />
      <NewWorkflowModal
        show={newWorkflowOpen}
        workflows={store.state.workflowOptions}
        currentGraph={store.state.graph}
        creating={store.state.creatingWorkflow}
        error={store.state.error}
        initialMode={newWorkflowInitialMode}
        onClose={() => setNewWorkflowOpen(false)}
        onCreate={createWorkflow}
      />
      {historyOpen ? <HistoryModal store={store} onClose={() => setHistoryOpen(false)} /> : null}
      {switchConfirmOpen ? <ConfirmModal title="Switch workflow?" body="Current workflow has unsaved changes. Switch anyway and discard the edit session?" onCancel={() => setSwitchConfirmOpen(false)} onConfirm={confirmWorkflowSwitch} /> : null}
    </div>
  );
}

function HistoryModal({ store, onClose }: { store: ReturnType<typeof useGraphStore>; onClose: () => void }) {
  return <div className="modal-backdrop"><section className="history-modal modal-card"><div className="modal-header"><h2>Version history</h2><button type="button" onClick={onClose}>×</button></div><div className="history-list">{store.state.workflowVersions.length ? store.state.workflowVersions.map((version) => <article key={version.id || version.version || version.created_at} className="history-item"><div><strong>{version.version || version.id || "version"}</strong><small>{version.created_at || version.published_at || ""}</small></div><span className="tag">{version.status || "snapshot"}</span><button type="button" disabled={store.state.rollingBack} onClick={() => store.rollbackWorkflowVersion(version.id || version.version || "")}>Rollback</button></article>) : <div className="empty-text">No versions loaded.</div>}</div></section></div>;
}

function ConfirmModal({ title, body, onCancel, onConfirm }: { title: string; body: string; onCancel: () => void; onConfirm: () => void }) {
  return <div className="modal-backdrop"><section className="confirm-modal modal-card"><div className="modal-header"><h2>{title}</h2></div><p>{body}</p><div className="modal-actions"><button type="button" onClick={onCancel}>Cancel</button><button type="button" onClick={onConfirm}>Confirm</button></div></section></div>;
}

function safeBundleFileName(name: string) {
  return name.trim().replace(/[^a-zA-Z0-9._-]+/g, "-") || "workflow";
}
