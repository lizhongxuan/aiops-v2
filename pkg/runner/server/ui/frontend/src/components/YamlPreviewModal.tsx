import { useEffect, useMemo, useState } from "react";
import CodeEditor from "./CodeEditor";
import type { WorkflowGraph } from "../types/workflow";
import { buildGraphDiffSummary } from "../utils/graphDiff";
import { formatGraphJSON, graphPreviewText } from "../utils/graphPreview";

interface YamlPreviewModalProps {
  show: boolean;
  graph: WorkflowGraph | null;
  baselineGraph?: WorkflowGraph | null;
  yamlPreview: string;
  previewCompiling?: boolean;
  onClose: () => void;
  onReplaceGraph: (graph: WorkflowGraph) => void;
  onCompileYaml: () => void;
  onParseYaml: (yaml: string) => void;
}

export default function YamlPreviewModal(props: YamlPreviewModalProps) {
  const [activeTab, setActiveTab] = useState("preview");
  const [yamlEdit, setYamlEdit] = useState("");
  const [yamlEditError, setYamlEditError] = useState<string | null>(null);
  const [graphJSON, setGraphJSON] = useState(formatGraphJSON(props.graph));
  const [graphJSONError, setGraphJSONError] = useState<string | null>(null);
  const previewText = useMemo(() => graphPreviewText(props.graph, props.yamlPreview), [props.graph, props.yamlPreview]);
  const diffSummary = useMemo(() => buildGraphDiffSummary(props.baselineGraph, props.graph), [props.baselineGraph, props.graph]);

  useEffect(() => {
    if (!props.show) return;
    setActiveTab("preview");
    setYamlEdit(props.yamlPreview.trim() ? props.yamlPreview : "");
    setGraphJSON(formatGraphJSON(props.graph));
    setYamlEditError(null);
    setGraphJSONError(null);
  }, [props.show, props.graph, props.yamlPreview]);

  if (!props.show) return null;

  function applyYamlEdit() {
    const yaml = yamlEdit.trim();
    if (!yaml) {
      setYamlEditError("Compile the graph first, or paste workflow YAML.");
      return;
    }
    props.onParseYaml(yaml);
    setYamlEditError(null);
  }

  function applyGraphJSON() {
    try {
      const parsed = JSON.parse(graphJSON) as WorkflowGraph;
      if (!parsed || typeof parsed !== "object" || !Array.isArray(parsed.nodes) || !Array.isArray(parsed.edges)) {
        throw new Error("Graph JSON must include nodes[] and edges[].");
      }
      props.onReplaceGraph(parsed);
      setGraphJSONError(null);
    } catch (error) {
      setGraphJSONError(error instanceof Error ? error.message : "Invalid graph JSON.");
    }
  }

  return (
    <div className="modal-backdrop">
      <section className="yaml-preview-modal modal-card">
        <div className="modal-header"><h2>Workflow YAML</h2><button type="button" onClick={props.onClose}>×</button></div>
        <TabButtons active={activeTab} tabs={["preview", "yaml", "diff", "graph-json"]} onChange={setActiveTab} />
        {activeTab === "preview" ? <><div className="modal-tab-heading"><span className="tag">{props.yamlPreview.trim() ? "Compiled YAML" : "Graph YAML-like"}</span><small>{props.previewCompiling ? "Compiling latest graph..." : "Editing graph; compiled YAML refreshes when API is available."}</small></div><CodeEditor value={previewText} language="yaml" readonly height="520px" /></> : null}
        {activeTab === "yaml" ? <><div className="modal-tab-heading"><span className="tag">Editable workflow YAML</span><small>Applies through the server YAML parser into the current graph store.</small></div><CodeEditor value={yamlEdit} language="yaml" height="520px" onChange={(value) => { setYamlEdit(value); setYamlEditError(null); }} />{yamlEditError ? <div className="inline-alert is-error">{yamlEditError}</div> : null}<div className="modal-actions"><button type="button" onClick={props.onCompileYaml}>Compile latest graph</button><button type="button" onClick={() => setYamlEdit(props.yamlPreview.trim() ? props.yamlPreview : "")}>Reset</button><button type="button" onClick={applyYamlEdit}>Apply YAML to graph</button></div></> : null}
        {activeTab === "diff" ? <div className="diff-section-grid">{diffSummary.sections.map((section) => <section key={section.kind} className={`diff-section-card ${section.changed ? "diff-section-card--changed" : ""}`}><div className="diff-section-heading"><strong>{section.title}</strong><span className={`tag ${section.changed ? "tag-warning" : "tag-success"}`}>{section.changed ? "Changed" : "Clean"}</span></div>{section.paths.length ? <ul className="diff-path-list">{section.paths.slice(0, 16).map((path) => <li key={path}><code>{path}</code></li>)}</ul> : <p className="diff-empty">No changes in this category.</p>}</section>)}</div> : null}
        {activeTab === "graph-json" ? <><div className="modal-tab-heading"><span className="tag">Advanced editor</span><small>Applies directly to the current graph store.</small></div><CodeEditor value={graphJSON} language="json" height="520px" onChange={(value) => { setGraphJSON(value); setGraphJSONError(null); }} />{graphJSONError ? <div className="inline-alert is-error">{graphJSONError}</div> : null}<div className="modal-actions"><button type="button" onClick={() => setGraphJSON(formatGraphJSON(props.graph))}>Reset</button><button type="button" onClick={applyGraphJSON}>Apply graph JSON</button></div></> : null}
        <div className="modal-actions"><button type="button" onClick={props.onClose}>Close</button></div>
      </section>
    </div>
  );
}

function TabButtons({ active, tabs, onChange }: { active: string; tabs: string[]; onChange: (tab: string) => void }) {
  return <div className="tab-buttons">{tabs.map((tab) => <button key={tab} type="button" className={active === tab ? "is-active" : ""} onClick={() => onChange(tab)}>{tab}</button>)}</div>;
}
