import { useEffect, useMemo, useState } from "react";
import CodeEditor from "./CodeEditor";
import type { GraphDiffSummary } from "../utils/graphDiff";
import {
  createArgPatch,
  createStepPatch,
  createSubflowPatch,
  envTextToObject,
  findActionSpec,
  formatJSON,
  getActionArgFields,
  getTargetOptions,
  nodeAction,
  nodeArgs,
  nodeExecutableName,
  normalizeStringList,
  objectToEnvText,
  readSubflowVars,
  readSubflowWorkflowName,
  readStepTargets,
  replaceStepFromJSON,
  validateActionArgs,
  validateTargets,
} from "../utils/actionForm";
import type { ActionSpec, WorkflowDefinition, WorkflowNode, WorkflowStep, WorkflowSummary } from "../types/workflow";

interface PropertyPanelProps {
  node: WorkflowNode | null;
  actions: ActionSpec[];
  workflow: WorkflowDefinition | null;
  workflows: WorkflowSummary[];
  diffSummary?: GraphDiffSummary;
  onUpdateNode: (nodeId: string, patch: Partial<WorkflowNode>) => void;
  onUpdateWorkflow: (patch: Partial<WorkflowDefinition>) => void;
}

export default function PropertyPanel({ node, actions, workflow, workflows, diffSummary, onUpdateNode, onUpdateWorkflow }: PropertyPanelProps) {
  const [activeTab, setActiveTab] = useState("config");
  const [stepJSON, setStepJSON] = useState("{}");
  const [stepJSONError, setStepJSONError] = useState<string | null>(null);
  const [workflowVarsJSON, setWorkflowVarsJSON] = useState("{}");
  const [workflowInventoryJSON, setWorkflowInventoryJSON] = useState("{}");
  const [workflowVarsJSONError, setWorkflowVarsJSONError] = useState<string | null>(null);
  const [workflowInventoryJSONError, setWorkflowInventoryJSONError] = useState<string | null>(null);
  const [subflowVarsJSON, setSubflowVarsJSON] = useState("{}");
  const [subflowVarsJSONError, setSubflowVarsJSONError] = useState<string | null>(null);
  const [argJSONErrors, setArgJSONErrors] = useState<Record<string, string>>({});

  const canEditStep = ["action", "condition", "subflow", "handler"].includes(node?.type || "");
  const isHandlerNode = node?.type === "handler";
  const currentAction = nodeAction(node);
  const actionSpec = useMemo(() => findActionSpec(actions, currentAction), [actions, currentAction]);
  const argFields = useMemo(() => getActionArgFields(actionSpec, currentAction), [actionSpec, currentAction]);
  const targets = isHandlerNode ? [] : readStepTargets(node?.step);
  const targetIssues = isHandlerNode ? [] : validateTargets(workflow, currentAction, targets);
  const fieldIssues = [...targetIssues, ...validateActionArgs(actionSpec, currentAction, nodeArgs(node))];
  const fieldIssueMap = Object.fromEntries(fieldIssues.map((issue) => [issue.field, issue]));
  const subflowWorkflowName = readSubflowWorkflowName(node);
  const subflowWorkflowOptions = useMemo(() => {
    const options = new Map<string, string>();
    workflows.forEach((item) => item.name && options.set(item.name, item.version ? `${item.name} · ${item.version}` : item.name));
    if (subflowWorkflowName && !options.has(subflowWorkflowName)) options.set(subflowWorkflowName, `${subflowWorkflowName} · current`);
    return [...options.entries()].sort((left, right) => left[0].localeCompare(right[0]));
  }, [workflows, subflowWorkflowName]);

  useEffect(() => {
    setActiveTab("config");
    setStepJSON(formatJSON(isHandlerNode ? node?.handler : node?.step || {}));
    setSubflowVarsJSON(formatJSON(readSubflowVars(node)));
    setArgJSONErrors({});
  }, [node?.id]);

  useEffect(() => {
    setWorkflowVarsJSON(formatJSON(workflow?.vars || {}));
    setWorkflowInventoryJSON(formatJSON(workflow?.inventory || {}));
  }, [workflow?.name]);

  function updateStep(patch: Partial<WorkflowStep>) {
    if (!node) return;
    onUpdateNode(node.id, createStepPatch(node, patch));
  }

  function updateArg(key: string, value: unknown) {
    if (!node) return;
    onUpdateNode(node.id, createArgPatch(node, key, value));
  }

  function updateStepJSON(value: string) {
    setStepJSON(value);
    if (!node) return;
    try {
      onUpdateNode(node.id, replaceStepFromJSON(node, value));
      setStepJSONError(null);
    } catch (error) {
      setStepJSONError(error instanceof Error ? error.message : "Invalid step JSON.");
    }
  }

  function updateWorkflowObject(value: string, label: string, setter: (value: string) => void, errorSetter: (value: string | null) => void, field: "vars" | "inventory") {
    setter(value);
    try {
      onUpdateWorkflow({ [field]: parseObject(value, label) } as Partial<WorkflowDefinition>);
      errorSetter(null);
    } catch (error) {
      errorSetter(error instanceof Error ? error.message : `Invalid ${label} JSON.`);
    }
  }

  function updateSubflowWorkflowName(value: string) {
    if (!node) return;
    onUpdateNode(node.id, createSubflowPatch(node, { workflow_name: value.trim() }));
  }

  function updateSubflowVarsJSON(value: string) {
    setSubflowVarsJSON(value);
    if (!node) return;
    try {
      onUpdateNode(node.id, createSubflowPatch(node, { vars: parseObject(value, "Subflow input vars") }));
      setSubflowVarsJSONError(null);
    } catch (error) {
      setSubflowVarsJSONError(error instanceof Error ? error.message : "Invalid subflow vars JSON.");
    }
  }

  return (
    <aside className="property-panel">
      <div className="panel-heading">
        <span>Properties</span>
        {node ? <span className="tag">{node.type}</span> : null}
      </div>
      {!node ? <div className="empty-state">Select a node</div> : (
        <>
          <label className="field-block">
            <span>Label</span>
            <input value={node.label || node.id} onChange={(event) => onUpdateNode(node.id, { label: event.target.value })} />
          </label>
          <TabButtons active={activeTab} tabs={["config", "run", "diff", "advanced", "workflow"]} onChange={setActiveTab} />
          {activeTab === "config" ? (
            <div className="property-form">
              <div className="node-summary">
                <Description label="Node ID">{node.id}</Description>
                {nodeAction(node) ? <Description label="Action">{nodeAction(node)}</Description> : null}
                {node.state?.status ? <Description label="Run"><span className="tag">{node.state.status}</span></Description> : null}
              </div>
              {node.type === "subflow" ? (
                <>
                  <FormItem label="Workflow" feedback={subflowWorkflowFeedback(node, workflow)}>
                    <select value={subflowWorkflowName} onChange={(event) => updateSubflowWorkflowName(event.target.value)}>
                      <option value="">Select workflow</option>
                      {subflowWorkflowOptions.map(([value, label]) => <option key={value} value={value}>{label}</option>)}
                    </select>
                  </FormItem>
                  <FormItem label="Input vars">
                    <CodeEditor value={subflowVarsJSON} language="json" height="220px" onChange={updateSubflowVarsJSON} />
                    {subflowVarsJSONError ? <div className="inline-alert is-error">{subflowVarsJSONError}</div> : null}
                  </FormItem>
                </>
              ) : canEditStep ? (
                <>
                  <FormItem label={isHandlerNode ? "Handler name" : "Step name"}>
                    <input value={nodeExecutableName(node)} onChange={(event) => updateStep({ name: event.target.value.trim() || undefined })} />
                  </FormItem>
                  <FormItem label="Action">
                    <select value={currentAction} onChange={(event) => updateStep({ action: event.target.value, args: nodeArgs(node) || {} })}>
                      {actionOptions(actions, currentAction).map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
                    </select>
                  </FormItem>
                  {actionSpec?.description ? <div className="inline-alert">{actionSpec.description}</div> : null}
                  {fieldIssues.length ? <div className={`inline-alert ${fieldIssues.some((issue) => issue.severity === "error") ? "is-error" : "is-warning"}`}>{fieldIssues.length} field issue{fieldIssues.length > 1 ? "s" : ""} found. Fix highlighted fields before running.</div> : null}
                  {!isHandlerNode ? (
                    <>
                      <FormItem label="Targets" feedback={targetFeedback(fieldIssueMap.targets)}>
                        <TagInput value={targets} onChange={(value) => updateStep({ targets: normalizeStringList(value), target: undefined })} />
                      </FormItem>
                      <FormItem label="When"><input value={node.step?.when || ""} placeholder="${environment} == 'staging'" onChange={(event) => updateStep({ when: event.target.value.trim() || undefined })} /></FormItem>
                      <div className="form-grid">
                        <FormItem label="Retries"><input type="number" value={node.step?.retries || 0} min={0} onChange={(event) => updateStep({ retries: Math.max(0, Math.trunc(Number(event.target.value) || 0)) })} /></FormItem>
                        <FormItem label="Timeout"><input value={node.step?.timeout || ""} placeholder="5m" onChange={(event) => updateStep({ timeout: event.target.value.trim() || undefined })} /></FormItem>
                      </div>
                      <FormItem label="Expect vars"><TagInput value={node.step?.expect_vars || []} onChange={(value) => updateStep({ expect_vars: normalizeStringList(value) })} /></FormItem>
                      <FormItem label="Must vars"><TagInput value={node.step?.must_vars || []} onChange={(value) => updateStep({ must_vars: normalizeStringList(value) })} /></FormItem>
                    </>
                  ) : null}
                  <div className="section-title">Action arguments</div>
                  {!argFields.length ? <div className="inline-alert is-warning">No action schema is available. Use Advanced JSON for custom arguments.</div> : null}
                  {argFields.map((field) => (
                    <FormItem key={field.key} label={field.title} feedback={renderFieldDescription(field.required, field.description, fieldIssueMap[field.key])}>
                      {renderArgField(field.kind, field.key, node, updateArg, argJSONErrors, setArgJSONErrors)}
                    </FormItem>
                  ))}
                  {actionSpec?.outputs?.length ? <div className="section-title">Outputs</div> : null}
                  {actionSpec?.outputs?.length ? <div className="output-list">{actionSpec.outputs.map((output) => <span key={output.name} className="tag">{output.name}{output.type ? `: ${output.type}` : ""}</span>)}</div> : null}
                </>
              ) : node.type === "manual_approval" ? (
                <>
                  <FormItem label="Approvers"><TagInput value={normalizeStringList(node.approval?.subjects)} onChange={(value) => onUpdateNode(node.id, { approval: { ...node.approval, subjects: normalizeStringList(value) } })} /></FormItem>
                  <FormItem label="Timeout"><input value={node.approval?.timeout || ""} placeholder="30m" onChange={(event) => onUpdateNode(node.id, { approval: { ...node.approval, timeout: event.target.value.trim() || undefined } })} /></FormItem>
                  <FormItem label="Timeout policy">
                    <select value={node.approval?.on_timeout || "reject"} onChange={(event) => onUpdateNode(node.id, { approval: { ...node.approval, on_timeout: event.target.value } })}>
                      <option value="reject">Reject</option>
                      <option value="approve">Approve</option>
                      <option value="continue">Continue</option>
                    </select>
                  </FormItem>
                </>
              ) : node.type === "join" ? (
                <>
                  <FormItem label="Join strategy">
                    <select value={node.join?.strategy || "all_success"} onChange={(event) => onUpdateNode(node.id, { join: { ...node.join, strategy: event.target.value } })}>
                      <option value="all_success">All success</option>
                      <option value="any_success">Any success</option>
                      <option value="always">Always</option>
                      <option value="failure_threshold">Failure threshold</option>
                    </select>
                  </FormItem>
                  {node.join?.strategy === "failure_threshold" ? <FormItem label="Failure threshold"><input type="number" value={node.join?.failure_threshold || 1} min={1} onChange={(event) => onUpdateNode(node.id, { join: { ...node.join, failure_threshold: Math.max(1, Math.trunc(Number(event.target.value) || 1)) } })} /></FormItem> : null}
                </>
              ) : null}
            </div>
          ) : null}
          {activeTab === "run" ? <RunStatePanel node={node} /> : null}
          {activeTab === "diff" ? <DiffPanel diffSummary={diffSummary} /> : null}
          {activeTab === "advanced" ? (
            <div>
              {canEditStep ? (
                <>
                  <div className="modal-tab-heading"><span className="tag">{isHandlerNode ? "Handler JSON" : "Step JSON"}</span><small>Valid JSON is applied immediately.</small></div>
                  <CodeEditor value={stepJSON} language="json" height="360px" onChange={updateStepJSON} />
                  {stepJSONError ? <div className="inline-alert is-error">{stepJSONError}</div> : null}
                </>
              ) : <div className="inline-alert">This node does not have step JSON.</div>}
            </div>
          ) : null}
          {activeTab === "workflow" ? (
            <div>
              <div className="modal-tab-heading"><span className="tag">Workflow vars</span><small>JSON object, applied immediately.</small></div>
              <CodeEditor value={workflowVarsJSON} language="json" height="180px" onChange={(value) => updateWorkflowObject(value, "Vars", setWorkflowVarsJSON, setWorkflowVarsJSONError, "vars")} />
              {workflowVarsJSONError ? <div className="inline-alert is-error">{workflowVarsJSONError}</div> : null}
              <div className="modal-tab-heading workflow-editor-heading"><span className="tag">Inventory</span><small>hosts / groups / vars</small></div>
              <CodeEditor value={workflowInventoryJSON} language="json" height="260px" onChange={(value) => updateWorkflowObject(value, "Inventory", setWorkflowInventoryJSON, setWorkflowInventoryJSONError, "inventory")} />
              {workflowInventoryJSONError ? <div className="inline-alert is-error">{workflowInventoryJSONError}</div> : null}
            </div>
          ) : null}
        </>
      )}
    </aside>
  );
}

function TabButtons({ active, tabs, onChange }: { active: string; tabs: string[]; onChange: (tab: string) => void }) {
  return <div className="tab-buttons">{tabs.map((tab) => <button key={tab} type="button" className={active === tab ? "is-active" : ""} onClick={() => onChange(tab)}>{tab}</button>)}</div>;
}

function Description({ label, children }: { label: string; children: React.ReactNode }) {
  return <div className="description-item" data-label={label}><strong>{label}</strong><span>{children}</span></div>;
}

function FormItem({ label, feedback, children }: { label: string; feedback?: string; children: React.ReactNode }) {
  return <label className="form-item" data-label={label}><span>{label}</span>{children}{feedback ? <small>{feedback}</small> : null}</label>;
}

function TagInput({ value, onChange }: { value: string[]; onChange: (value: string[]) => void }) {
  return <input value={value.join(", ")} onChange={(event) => onChange(event.target.value.split(",").map((item) => item.trim()).filter(Boolean))} />;
}

function renderArgField(kind: string, key: string, node: WorkflowNode, updateArg: (key: string, value: unknown) => void, errors: Record<string, string>, setErrors: (next: Record<string, string>) => void) {
  const value = nodeArgs(node)?.[key];
  if (kind === "boolean") return <input type="checkbox" checked={value === true} onChange={(event) => updateArg(key, event.target.checked)} />;
  if (kind === "string-array") return <TagInput value={normalizeStringList(value)} onChange={(next) => updateArg(key, normalizeStringList(next))} />;
  if (kind === "env") return <textarea value={objectToEnvText(value)} placeholder="KEY=value" onChange={(event) => updateArg(key, envTextToObject(event.target.value))} />;
  if (kind === "json") {
    return <>
      <CodeEditor value={formatJSON(value)} language="json" height="180px" onChange={(next) => {
        try {
          updateArg(key, JSON.parse(next));
          const clean = { ...errors };
          delete clean[key];
          setErrors(clean);
        } catch (error) {
          setErrors({ ...errors, [key]: error instanceof Error ? error.message : "Invalid JSON." });
        }
      }} />
      {errors[key] ? <div className="inline-alert is-error">{errors[key]}</div> : null}
    </>;
  }
  if (kind === "multiline") return <textarea value={value === undefined || value === null ? "" : String(value)} onChange={(event) => updateArg(key, event.target.value || undefined)} />;
  return <input value={value === undefined || value === null ? "" : String(value)} onChange={(event) => updateArg(key, event.target.value || undefined)} />;
}

function RunStatePanel({ node }: { node: WorkflowNode }) {
  return <div>{node.state ? <div className="node-summary"><Description label="Status"><span className="tag">{node.state.status || "unknown"}</span></Description>{node.state.message ? <Description label="Message">{node.state.message}</Description> : null}{node.state.started_at ? <Description label="Started">{node.state.started_at}</Description> : null}{node.state.finished_at ? <Description label="Finished">{node.state.finished_at}</Description> : null}</div> : <div className="empty-state">No run state for this node</div>}{node.state?.hosts ? <><div className="modal-tab-heading workflow-editor-heading"><span className="tag">Host results</span><small>runtime overlay</small></div><CodeEditor value={formatJSON(node.state.hosts)} language="json" height="200px" readonly /></> : null}</div>;
}

function DiffPanel({ diffSummary }: { diffSummary?: GraphDiffSummary }) {
  const sections = diffSummary?.sections || [];
  if (!sections.length) return <div className="empty-state">No baseline diff available</div>;
  return <div className="diff-section-list">{sections.map((section) => <div key={section.kind} className={`diff-section ${section.changed ? "is-changed" : ""}`}><div className="diff-section-heading"><span className="tag">{section.kind}</span><strong>{section.title}</strong><small>{section.changed ? `${section.paths.length} change${section.paths.length === 1 ? "" : "s"}` : "unchanged"}</small></div>{section.paths.length ? <ul>{section.paths.map((path) => <li key={path}><code>{path}</code></li>)}</ul> : null}</div>)}</div>;
}

function actionOptions(actions: ActionSpec[], currentAction: string) {
  const options = actions.filter((action) => !action.node_type || action.node_type === "action").map((action) => ({ label: `${action.title} (${action.action})`, value: action.action }));
  if (currentAction && !options.some((option) => option.value === currentAction)) options.unshift({ label: currentAction, value: currentAction });
  return options;
}

function subflowWorkflowFeedback(node: WorkflowNode, workflow: WorkflowDefinition | null) {
  if (node.type !== "subflow") return "";
  const name = readSubflowWorkflowName(node);
  if (!name) return "Workflow is required before validation, dry-run, or run.";
  if (name === workflow?.name) return "This subflow points to the current workflow. Confirm recursion is intended.";
  return "Select an existing workflow or enter a workflow name.";
}

function renderFieldDescription(required: boolean, description: string | undefined, issue: { message: string; suggestion: string } | undefined): string {
  if (issue) return `${issue.message} ${issue.suggestion}`;
  return required ? `${description || ""}${description ? " " : ""}Required.` : description || "";
}

function targetFeedback(issue: { message: string; suggestion: string } | undefined): string {
  if (!issue) return "Select inventory hosts or groups. Custom targets are allowed but should pass dry-run.";
  return `${issue.message} ${issue.suggestion}`;
}

function parseObject(value: string, label: string): Record<string, unknown> {
  const parsed = JSON.parse(value);
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) throw new Error(`${label} JSON must be an object.`);
  return parsed as Record<string, unknown>;
}
