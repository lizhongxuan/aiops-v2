import { useEffect, useMemo, useState } from "react";
import { runnerApi } from "../api/client";
import type { WorkflowGraph, WorkflowSummary } from "../types/workflow";
import { createWorkflowGraphFromTemplate, prepareWorkflowGraphForCreate, type WorkflowTemplateKind } from "../utils/workflowTemplates";

type CreateMode = WorkflowTemplateKind | "from-yaml" | "clone-current";

interface NewWorkflowModalProps {
  show: boolean;
  workflows: WorkflowSummary[];
  currentGraph: WorkflowGraph | null;
  creating?: boolean;
  error?: string | null;
  initialMode?: CreateMode;
  onClose: () => void;
  onCreate: (payload: { graph: WorkflowGraph; labels?: Record<string, string>; saveNote?: string }) => void;
}

export default function NewWorkflowModal({ show, workflows, currentGraph, creating = false, error = null, initialMode = "script-shell-basic", onClose, onCreate }: NewWorkflowModalProps) {
  const [mode, setMode] = useState<CreateMode>(initialMode);
  const [name, setName] = useState("new-workflow");
  const [version, setVersion] = useState("v0.1");
  const [description, setDescription] = useState("");
  const [saveNote, setSaveNote] = useState("");
  const [yaml, setYaml] = useState("");
  const [labelsText, setLabelsText] = useState("source=visual-ui");
  const [localError, setLocalError] = useState<string | null>(null);
  const existingNames = useMemo(() => new Set(workflows.map((workflow) => workflow.name)), [workflows]);

  useEffect(() => {
    if (!show) return;
    setMode(initialMode);
    setLocalError(null);
  }, [show, initialMode]);

  if (!show) return null;

  async function submit() {
    const trimmedName = name.trim();
    if (!trimmedName) {
      setLocalError("Workflow name is required.");
      return;
    }
    if (existingNames.has(trimmedName)) {
      setLocalError("Workflow already exists. Pick a new name or use clone intentionally with another name.");
      return;
    }
    try {
      let graph: WorkflowGraph;
      if (mode === "from-yaml") {
        if (!yaml.trim()) throw new Error("Paste workflow YAML before creating.");
        graph = await runnerApi.parseGraphYAML(yaml);
        graph.workflow = { ...graph.workflow, name: trimmedName, version: version.trim() || graph.workflow.version };
      } else if (mode === "clone-current") {
        if (!currentGraph) throw new Error("No current graph to clone.");
        graph = prepareWorkflowGraphForCreate(currentGraph, { name: trimmedName, version: version.trim() || "v0.1", description: description.trim() || undefined });
      } else {
        graph = createWorkflowGraphFromTemplate({ kind: mode, name: trimmedName, version: version.trim() || "v0.1", description: description.trim() || undefined });
      }
      onCreate({ graph, labels: parseLabels(labelsText), saveNote });
    } catch (error) {
      setLocalError(error instanceof Error ? error.message : "Unable to create workflow.");
    }
  }

  return (
    <div className="modal-backdrop">
      <section className="new-workflow-dialog modal-card">
        <div className="modal-header"><h2>Create workflow</h2><button type="button" onClick={onClose}>×</button></div>
        {(localError || error) ? <div className="inline-alert is-error">{localError || error}</div> : null}
        <div className="new-workflow-form">
          <label className="field-block"><span>Mode</span><select value={mode} onChange={(event) => setMode(event.target.value as CreateMode)}><option value="script-shell-basic">Shell Script starter</option><option value="shell-run-basic">Shell starter</option><option value="manual-approval-basic">Manual approval starter</option><option value="clone-current">Clone current</option><option value="from-yaml">From YAML</option></select></label>
          <div className="new-workflow-grid">
            <label className="field-block"><span>Name</span><input value={name} onChange={(event) => setName(event.target.value)} /></label>
            <label className="field-block"><span>Version</span><input value={version} onChange={(event) => setVersion(event.target.value)} /></label>
          </div>
          <label className="field-block"><span>Description</span><input value={description} onChange={(event) => setDescription(event.target.value)} /></label>
          <label className="field-block"><span>Labels</span><input value={labelsText} placeholder="source=visual-ui,team=ops" onChange={(event) => setLabelsText(event.target.value)} /></label>
          <label className="field-block"><span>Save note</span><input value={saveNote} maxLength={160} onChange={(event) => setSaveNote(event.target.value)} /></label>
          {mode === "from-yaml" ? <label className="field-block"><span>Workflow YAML</span><textarea value={yaml} rows={10} onChange={(event) => setYaml(event.target.value)} /></label> : null}
        </div>
        <div className="modal-actions"><button type="button" onClick={onClose}>Cancel</button><button type="button" className="submit-new-workflow" disabled={creating} onClick={submit}>Create</button></div>
      </section>
    </div>
  );
}

function parseLabels(text: string): Record<string, string> | undefined {
  const labels: Record<string, string> = {};
  for (const item of text.split(",")) {
    const [key, ...rest] = item.split("=");
    const trimmedKey = key?.trim();
    const value = rest.join("=").trim();
    if (trimmedKey && value) labels[trimmedKey] = value;
  }
  return Object.keys(labels).length ? labels : undefined;
}
