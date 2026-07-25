import { useLayoutEffect, useMemo, useState } from "react";
import { CheckCircle, Maximize2, PanelRight, Play, Plus, Trash2, X } from "lucide-react";

import { createInputParam, normalizeInputParams, valueSourceLabel, variableToValueSource } from "@/components/runner/io/ioTypes";
import { createOutputParam, normalizeOutputParams } from "@/components/runner/io/outputTypes";
import { getNodeCanvasMeta } from "@/components/runner/nodeTypeRegistry";
import { collectRunnerVariables } from "@/components/runner/runnerVariables";
import type { RunnerGraph, RunnerNode } from "@/components/runner/canvasGraphAdapter";
import { WorkflowDeleteConfirmation, deriveWorkflowNodeDeleteImpact } from "@/runner/WorkflowDeleteConfirmation";
import {
  DEFAULT_HOST_GROUPS,
  actionEditorKind,
  cloneNode,
  collectNodeRunFailureDetails,
  ensureAggregatorOutput,
  formatRuntimeValue,
  isSystemRunnerNode,
  nodeDisplayName,
  normalizeAggregatorSources,
  normalizeEditableHostGroups,
  normalizeEndCallbacks,
  normalizeTargetLabels,
  objectValue,
  readNodeAggregator,
  readNodeCondition,
  splitList,
  systemNodeHint,
  systemNodeLabel,
  variableToAggregatorSource,
  workflowHostGroups,
  type RunnerEndCallback,
  type RunnerHostGroup,
  type RunnerRunState,
  type RunnerVariableOption,
} from "@/runner/workflowNodeInspectorModel";

export function StartHostGroupsEditor({ groups, onChange }: { groups: RunnerHostGroup[]; onChange: (groups: RunnerHostGroup[]) => void }) {
  const visibleGroups = groups.length ? groups : DEFAULT_HOST_GROUPS;
  const updateGroup = (index: number, patch: Partial<RunnerHostGroup>) => onChange(visibleGroups.map((group, itemIndex) => itemIndex === index ? { ...group, ...patch } : group));
  const removeGroup = (index: number) => onChange(visibleGroups.filter((_, itemIndex) => itemIndex !== index));
  return (
    <section className="runner-host-groups-editor" data-testid="runner-start-host-groups">
      <div className="runner-editor-section-head"><strong>运行主机组</strong><span>每个标签对应一台或多台主机。</span></div>
      {visibleGroups.map((group, index) => (
        <div key={`${group.label}-${index}`} className="runner-host-group-row">
          <label>标签<input data-testid={`runner-host-group-label-${index}`} value={group.label} placeholder="web" onChange={(event) => updateGroup(index, { label: event.target.value })} /></label>
          <label>主机<textarea data-testid={`runner-host-group-hosts-${index}`} value={group.hosts.join("\n")} placeholder={"web-01\nweb-02"} onChange={(event) => updateGroup(index, { hosts: splitList(event.target.value) })} /></label>
          <button type="button" onClick={() => removeGroup(index)} disabled={visibleGroups.length <= 1}>移除</button>
        </div>
      ))}
      <button type="button" data-testid="runner-host-group-add" onClick={() => onChange([...visibleGroups, { label: "", hosts: [] }])}>添加主机标签</button>
    </section>
  );
}

export function RunnerNodeTargetsEditor({ groups, targets, onChange }: { groups: RunnerHostGroup[]; targets: string[]; onChange: (targets: string[]) => void }) {
  const toggleTarget = (label: string) => {
    if (targets.includes(label)) onChange(targets.filter((item) => item !== label));
    else onChange([...targets, label]);
  };
  return (
    <section className="runner-node-targets" data-testid="runner-node-targets">
      <label>目标标签<input data-testid="runner-node-target-labels-input" value={targets.join(", ")} placeholder="web, db" onChange={(event) => onChange(normalizeTargetLabels(event.target.value))} /></label>
      <div className="runner-node-target-options" data-testid="runner-node-target-options">
        {groups.map((group) => (
          <button key={group.label} type="button" className={targets.includes(group.label) ? "active" : ""} onClick={() => toggleTarget(group.label)}>
            {group.label}<span>{group.hosts.length} 台</span>
          </button>
        ))}
      </div>
    </section>
  );
}

export function EndCallbacksEditor({ callbacks, onChange }: { callbacks: RunnerEndCallback[]; onChange: (callbacks: RunnerEndCallback[]) => void }) {
  const visibleCallbacks = callbacks.length ? callbacks : [{ event: "success", url: "", payload: "" }];
  const updateCallback = (index: number, patch: Partial<RunnerEndCallback>) => onChange(visibleCallbacks.map((callback, itemIndex) => itemIndex === index ? { ...callback, ...patch } : callback));
  const removeCallback = (index: number) => onChange(visibleCallbacks.filter((_, itemIndex) => itemIndex !== index));
  return (
    <section className="runner-end-callbacks" data-testid="runner-end-callbacks">
      <div className="runner-editor-section-head"><strong>结束回调</strong><span>在工作流结束后按状态触发外部回调。</span></div>
      {visibleCallbacks.map((callback, index) => (
        <div key={`${callback.event}-${index}`} className="runner-end-callback-row">
          <label>触发<select data-testid={`runner-end-callback-event-${index}`} value={callback.event} onChange={(event) => updateCallback(index, { event: event.target.value })}><option value="success">成功</option><option value="failed">失败</option><option value="always">总是</option></select></label>
          <label>URL<input data-testid={`runner-end-callback-url-${index}`} value={callback.url} placeholder="https://hooks.example/runner" onChange={(event) => updateCallback(index, { url: event.target.value })} /></label>
          <label>请求内容<textarea data-testid={`runner-end-callback-payload-${index}`} value={callback.payload} placeholder='{"run":"{{sys.run_id}}"}' onChange={(event) => updateCallback(index, { payload: event.target.value })} /></label>
          <button type="button" onClick={() => removeCallback(index)} disabled={visibleCallbacks.length <= 1}>移除</button>
        </div>
      ))}
      <button type="button" onClick={() => onChange([...visibleCallbacks, { event: "success", url: "", payload: "" }])}>添加回调</button>
    </section>
  );
}

export function RunnerVariablePicker({ variables, onPick }: { variables: RunnerVariableOption[]; onPick: (variable: RunnerVariableOption) => void }) {
  const [query, setQuery] = useState("");
  const visible = variables.filter((variable) => {
    const text = [variable.expression, variable.scope, variable.name, variable.nodeId || variable.node_id || variable.sourceNodeId].join(" ").toLowerCase();
    return !query.trim() || text.includes(query.trim().toLowerCase());
  }).slice(0, 30);
  return (
    <section className="runner-variable-picker" data-testid="runner-variable-picker">
      <input value={query} placeholder="搜索变量" onChange={(event) => setQuery(event.target.value)} />
      <div>
        {visible.length ? visible.map((variable) => (
          <button key={variable.expression} type="button" onClick={() => onPick(variable)}>
            <code>{variable.expression}</code>
            <span>{runnerValueTypeLabel(variable.type)}{variable.displayValue !== undefined ? ` · ${variable.displayValue}` : ""}</span>
          </button>
        )) : <p>当前节点暂无可用变量。</p>}
      </div>
    </section>
  );
}

export function RunnerCodeInputVariablesEditor({
  inputs,
  variables,
  onAdd,
  onUpdate,
  onRemove,
  onUpdateSource,
}: {
  inputs: Array<Record<string, unknown>>;
  variables: RunnerVariableOption[];
  onAdd: () => void;
  onUpdate: (index: number, patch: Record<string, unknown>) => void;
  onRemove: (index: number) => void;
  onUpdateSource: (index: number, source: Record<string, unknown>) => void;
}) {
  const [openPickerIndex, setOpenPickerIndex] = useState<number | null>(null);
  return (
    <section className="runner-code-variable-editor" data-testid="runner-code-input-variables">
      <div className="runner-code-variable-editor-head">
        <strong>输入变量</strong>
        <button type="button" data-testid="runner-code-input-add" aria-label="添加输入变量" onClick={onAdd}><Plus size={15} /></button>
      </div>
      {inputs.length ? inputs.map((item, index) => {
        const key = String(item.key || `input_${index + 1}`);
        const source = objectValue(item.value_source);
        const label = valueSourceLabel(source) || "设置变量值";
        return (
          <div key={`${key}-${index}`} className="runner-code-variable-row">
            <input value={key} aria-label="输入变量名" onChange={(event) => onUpdate(index, { key: event.target.value })} />
            <button
              type="button"
              className="runner-code-variable-value"
              data-testid={`runner-code-input-value-${index}`}
              onClick={() => setOpenPickerIndex(openPickerIndex === index ? null : index)}
            >
              {label}
            </button>
            <button type="button" className="runner-code-variable-remove" aria-label="删除输入变量" onClick={() => onRemove(index)}><Trash2 size={15} /></button>
            {openPickerIndex === index ? (
              <div className="runner-code-variable-picker" data-testid={`runner-code-input-variable-picker-${index}`}>
                <RunnerVariablePicker
                  variables={variables}
                  onPick={(variable) => {
                    onUpdateSource(index, variableToValueSource(variable));
                    setOpenPickerIndex(null);
                  }}
                />
              </div>
            ) : null}
          </div>
        );
      }) : <p>暂无输入变量。</p>}
    </section>
  );
}

export function RunnerCodeOutputVariablesEditor({
  outputs,
  onAdd,
  onUpdate,
  onRemove,
}: {
  outputs: Array<Record<string, unknown>>;
  onAdd: () => void;
  onUpdate: (index: number, patch: Record<string, unknown>) => void;
  onRemove: (index: number) => void;
}) {
  return (
    <section className="runner-code-variable-editor" data-testid="runner-code-output-variables">
      <div className="runner-code-variable-editor-head">
        <strong>输出变量</strong>
        <button type="button" data-testid="runner-code-output-add" aria-label="添加输出变量" onClick={onAdd}><Plus size={15} /></button>
      </div>
      {outputs.length ? outputs.map((item, index) => {
        const key = String(item.key || `output_${index + 1}`);
        return (
          <div key={`${key}-${index}`} className="runner-code-variable-row">
            <input value={key} aria-label="输出变量名" onChange={(event) => onUpdate(index, { key: event.target.value })} />
            <select value={String(item.type || "string")} aria-label="输出变量类型" onChange={(event) => onUpdate(index, { type: event.target.value })}>
              <option value="string">文本</option>
              <option value="number">数字</option>
              <option value="boolean">是/否</option>
              <option value="object">对象</option>
              <option value="array">列表</option>
              <option value="any">任意类型</option>
            </select>
            <button type="button" className="runner-code-variable-remove" aria-label="删除输出变量" onClick={() => onRemove(index)}><Trash2 size={15} /></button>
          </div>
        );
      }) : <p>暂无输出变量。</p>}
    </section>
  );
}

export function NodeLastRunView({ nodeId, runState }: { nodeId: string; runState: RunnerRunState }) {
  const runNode = runState.nodes?.[nodeId];
  const logs = (runState.logs || []).filter((log: { nodeId?: string }) => log.nodeId === nodeId);
  const outputs = (runState.variables?.outputs || []).filter((item: { nodeId?: string }) => item.nodeId === nodeId);
  const result = runNode?.result;
  const failureDetails = collectNodeRunFailureDetails(runState, nodeId);
  if (!runNode && !logs.length && !outputs.length && !failureDetails.length) {
    return <section className="runner-last-run-empty">这个节点还没有运行记录。</section>;
  }
  return (
    <section className="runner-last-run-view" data-testid="runner-node-last-run-view">
      {failureDetails.length ? (
        <div className="runner-last-run-block runner-last-run-failure" data-testid="runner-node-last-run-failure">
          <strong>失败原因</strong>
          <ul>
            {failureDetails.map((item) => <li key={item}>{item}</li>)}
          </ul>
        </div>
      ) : null}
      <div className="runner-last-run-grid">
        <span>状态<strong>{runnerStatusLabel(runNode?.status)}</strong></span>
        <span>耗时<strong>{runNode?.durationMs ? `${runNode.durationMs} ms` : "-"}</strong></span>
      </div>
      {result !== undefined ? <div className="runner-last-run-block"><strong>输出</strong><pre>{formatRuntimeValue(result)}</pre></div> : null}
      {outputs.length ? <div className="runner-last-run-block"><strong>变量输出</strong>{outputs.map((item: { key?: string; name?: string; value?: unknown }, index: number) => <p key={`${item.key || item.name}-${index}`}><code>{item.key || item.name}</code><span>{formatRuntimeValue(item.value)}</span></p>)}</div> : null}
      {logs.length ? <div className="runner-last-run-block"><strong>日志</strong>{logs.map((log: { stream?: string; message?: string }, index: number) => <pre key={`${log.stream}-${index}`}>{log.stream || "log"}: {log.message || ""}</pre>)}</div> : null}
    </section>
  );
}

export function RunnerVariableInspector({ graph, state, selectedNodeId }: { graph: RunnerGraph; state: RunnerRunState; selectedNodeId: string }) {
  const fallbackNode = graph.nodes?.length ? graph.nodes[graph.nodes.length - 1]?.id || "" : "";
  const variables = collectRunnerVariables(graph, selectedNodeId || fallbackNode, { runState: state });
  const grouped = variables.reduce((acc: Record<string, typeof variables>, variable: typeof variables[number]) => {
    const scope = variable.scope || "other";
    if (!acc[scope]) acc[scope] = [];
    acc[scope].push(variable);
    return acc;
  }, {});
  const scopes = Object.keys(grouped);
  return (
    <details className="publish-review-card runner-variable-inspector runner-run-collapsible" data-testid="runner-variable-inspector">
      <summary>
        <span>变量检查器</span>
        <small>{selectedNodeId ? `当前节点：${selectedNodeId}` : "按工作流末端上下文展示"}</small>
      </summary>
      <div className="runner-run-collapsible-body">
        {scopes.length ? scopes.map((scope) => (
          <div key={scope} className="runner-variable-scope">
            <strong>{scope}</strong>
            {grouped[scope].map((variable) => (
              <div key={variable.expression} className="runner-variable-row">
                <code>{variable.expression}</code>
                <span>{runnerValueTypeLabel(variable.type)}</span>
                <small>{variable.displayValue !== undefined ? variable.displayValue : "暂无运行值"}</small>
              </div>
            ))}
          </div>
        )) : <p>暂无可见变量。</p>}
      </div>
    </details>
  );
}
export type RunnerNodeInspectorAction =
  | { type: "apply-node"; node: RunnerNode }
  | { type: "run-node"; nodeId: string }
  | { type: "delete-node"; nodeId: string }
  | { type: "close" };

type RunnerNodeInspectorProps = {
  node: RunnerNode;
  graph: RunnerGraph;
  runState: RunnerRunState;
  onAction: (action: RunnerNodeInspectorAction) => void;
};

export function RunnerNodeInspector({ node, graph, runState, onAction }: RunnerNodeInspectorProps) {
  const [draft, setDraft] = useState(() => cloneNode(node));
  const [tab, setTab] = useState("settings");
  const [scriptModalOpen, setScriptModalOpen] = useState(false);
  const [deleteConfirmationOpen, setDeleteConfirmationOpen] = useState(false);
  const nodeId = String(node.id || "");
  // Reset the editor before paint when selection changes. A passive effect can
  // briefly leave the previous node's destructive actions clickable after a
  // fast canvas selection, which risks deleting the wrong node.
  useLayoutEffect(() => { setDraft(cloneNode(node)); setTab("settings"); setScriptModalOpen(false); setDeleteConfirmationOpen(false); }, [nodeId]);
  const meta = getNodeCanvasMeta(draft);
  const deleteImpact = useMemo(() => deriveWorkflowNodeDeleteImpact(graph, draft), [draft, graph]);
  const runNode = runState.nodes?.[draft.id];
  const inputs = normalizeInputParams(draft.inputs);
  const outputs = normalizeOutputParams(draft.outputs);
  const variables = collectRunnerVariables(graph, draft.id, { runState });
  const isCondition = meta.key === "condition" || draft.type === "condition" || draft.step?.action === "condition.evaluate";
  const isAggregator = meta.key === "variable-aggregator" || draft.type === "variable_aggregator" || draft.step?.action === "variable.aggregate";
  const isSystemNode = isSystemRunnerNode(draft, meta);
  const isStartNode = meta.key === "start" || String(draft.type || "").toLowerCase() === "start";
  const isEndNode = meta.key === "end" || String(draft.type || "").toLowerCase() === "end";
  const displayName = nodeDisplayName(draft, meta);
  const actionName = String(draft.step?.action || meta.action || draft.type || "").trim();
  const actionKind = actionEditorKind(actionName);
  const stepArgs = objectValue(draft.step?.args);
  const nodeUi = objectValue(draft.ui);
  const hostGroups = isStartNode ? normalizeEditableHostGroups(nodeUi.host_groups) : workflowHostGroups(graph, draft);
  const stepTargets = normalizeTargetLabels(draft.step?.targets || []);
  const endCallbacks = normalizeEndCallbacks(nodeUi.callbacks);
  const condition = readNodeCondition(draft);
  const aggregator = readNodeAggregator(draft);
  const aggregatorSources = normalizeAggregatorSources(aggregator.sources);
  const continueOnError = Boolean(draft.step?.continue_on_error || draft.step?.continueOnError);
  const isCodeAction = actionKind === "script" || actionKind === "command";
  const updateInput = (index: number, patch: Record<string, unknown>) => setDraft({ ...draft, inputs: inputs.map((item: Record<string, unknown>, itemIndex: number) => itemIndex === index ? { ...item, ...patch } : item) });
  const updateInputSource = (index: number, value_source: Record<string, unknown>) => updateInput(index, { value_source });
  const removeInput = (index: number) => setDraft({ ...draft, inputs: inputs.filter((_, itemIndex: number) => itemIndex !== index) });
  const updateOutput = (index: number, patch: Record<string, unknown>) => setDraft({ ...draft, outputs: outputs.map((item: Record<string, unknown>, itemIndex: number) => itemIndex === index ? { ...item, ...patch } : item) });
  const removeOutput = (index: number) => setDraft({ ...draft, outputs: outputs.filter((_, itemIndex: number) => itemIndex !== index) });
  const appendConditionVariable = (variable: { expression?: string }) => {
    const expression = String(variable.expression || "").trim();
    if (!expression) return;
    const current = String(condition.if || "").trim();
    updateCondition({ if: current ? `${current} ${expression}` : expression });
  };
  const updateCondition = (patch: Record<string, unknown>) => {
    const nextCondition = { ...condition, ...patch };
    setDraft({
      ...draft,
      type: "condition",
      condition: nextCondition,
      data: { ...objectValue(draft.data), condition: nextCondition },
      step: { ...(draft.step || {}), name: draft.step?.name || draft.id, action: draft.step?.action || "condition.evaluate" },
    });
  };
  const updateAggregator = (patch: Record<string, unknown>) => {
    const nextAggregator = { ...aggregator, ...patch };
    const outputKey = String(nextAggregator.output_key || "").trim();
    setDraft({
      ...draft,
      type: "variable_aggregator",
      aggregator: nextAggregator,
      data: { ...objectValue(draft.data), aggregator: nextAggregator },
      outputs: ensureAggregatorOutput(outputs, outputKey),
      step: { ...(draft.step || {}), name: draft.step?.name || draft.id, action: draft.step?.action || "variable.aggregate" },
    });
  };
  const addAggregatorSource = (variable: RunnerVariableOption) => {
    const source = variableToAggregatorSource(variable);
    if (!source.expression) return;
    updateAggregator({ sources: [...aggregatorSources, source] });
  };
  const updateNodeUi = (patch: Record<string, unknown>) => setDraft({ ...draft, ui: { ...objectValue(draft.ui), ...patch } });
  const updateStep = (patch: Record<string, unknown>) => setDraft({ ...draft, step: { ...(draft.step || {}), ...patch } });
  const updateStepArg = (key: string, value: unknown) => updateStep({ args: { ...objectValue(draft.step?.args), [key]: value } });
  const tabs = [
    ...(actionKind === "script" ? [["script", "脚本"]] : []),
    ["settings", "设置"],
    ...(isCodeAction ? [["io", "输入输出"]] : []),
    ...(isAggregator ? [["aggregate", "聚合"]] : []),
    ...(isCondition ? [["branches", "分支"]] : []),
    ["error", "错误处理"],
    ["advanced", "高级"],
    ["last-run", "上次运行"],
  ];
  return (
    <aside className="runner-node-panel" role="complementary" aria-label="节点配置面板" data-testid="runner-node-panel">
      <header className="runner-node-panel-head">
        <div className="runner-node-panel-identity">
          <span className={`runner-node-panel-icon tone-${meta.tone}`}>{meta.iconText}</span>
          <div><p>{isSystemNode ? systemNodeLabel(draft, meta) : "执行节点"}</p><h2 data-testid="runner-node-panel-title">{displayName}</h2><span className={`runner-node-panel-status status-${runNode?.status || "not_run"}`}>{runnerStatusLabel(runNode?.status)}</span></div>
        </div>
        <div className="runner-node-panel-actions">
          <button type="button" data-testid="runner-node-panel-run" onClick={() => onAction({ type: "run-node", nodeId: draft.id })}><Play size={15} />运行</button>
          <button type="button" data-testid="runner-node-panel-open-run" onClick={() => setTab("last-run")}><PanelRight size={15} />详情</button>
          <button type="button" className="primary" data-testid="runner-node-panel-apply" onClick={() => onAction({ type: "apply-node", node: cloneNode(draft) })}><CheckCircle size={15} />保存</button>
          {!isSystemNode ? <button type="button" className="danger" aria-label="删除节点" data-testid="runner-node-panel-delete" onClick={() => {
            if (deleteImpact.requiresConfirmation) setDeleteConfirmationOpen(true);
            else onAction({ type: "delete-node", nodeId: draft.id });
          }}><Trash2 size={15} />删除</button> : null}
          <button type="button" aria-label="关闭节点配置" data-testid="runner-node-panel-close" onClick={() => onAction({ type: "close" })}><X size={16} /></button>
        </div>
      </header>
      {deleteConfirmationOpen && !isSystemNode ? (
        <WorkflowDeleteConfirmation
          impact={deleteImpact}
          onCancel={() => setDeleteConfirmationOpen(false)}
          onConfirm={() => {
            setDeleteConfirmationOpen(false);
            onAction({ type: "delete-node", nodeId: draft.id });
          }}
        />
      ) : null}
      <nav className="runner-node-panel-tabs" aria-label="节点配置页签" data-testid="runner-node-panel-tabs">
        {tabs.map(([key, label]) => <button key={key} type="button" className={tab === key ? "active" : ""} data-testid={`runner-node-panel-tab-${key}`} onClick={() => setTab(key)}>{label}</button>)}
      </nav>
      <main className="runner-node-panel-body">
        {tab === "settings" ? (
          <section className="node-config-form runner-node-settings" data-testid="runner-node-settings">
            {isSystemNode ? (
              <>
                <section className="runner-node-system-card" data-testid="runner-node-system-card">
                  <strong>{systemNodeLabel(draft, meta)}</strong>
                  <p>{systemNodeHint(draft, meta)}</p>
                  <span>节点类型 <code>{draft.type || meta.key}</code></span>
                </section>
                {isStartNode ? <StartHostGroupsEditor groups={hostGroups} onChange={(groups) => updateNodeUi({ host_groups: groups })} /> : null}
                {isEndNode ? (
                  <>
                    <section className="runner-end-final-variables" data-testid="runner-end-final-variables">
                      <div className="runner-editor-section-head"><strong>最终变量</strong><span>工作流结束时可查看所有上游输出和系统变量。</span></div>
                      <RunnerVariableInspector graph={graph} state={runState} selectedNodeId="" />
                    </section>
                    <EndCallbacksEditor callbacks={endCallbacks} onChange={(callbacks) => updateNodeUi({ callbacks })} />
                  </>
                ) : null}
              </>
            ) : (
              <>
                <RunnerNodeTargetsEditor groups={hostGroups} targets={stepTargets} onChange={(targets) => updateStep({ targets })} />
                {actionKind === "command" ? (
                  <label>命令<input data-testid="runner-node-command-editor" value={String(stepArgs.cmd || "")} placeholder="df -h" onChange={(event) => updateStepArg("cmd", event.target.value)} /></label>
                ) : null}
              </>
            )}
          </section>
        ) : null}
        {tab === "script" && actionKind === "script" ? (
          <section className="node-config-form runner-node-script-tab" data-testid="runner-node-script-tab">
            <section className="runner-script-context" data-testid="runner-script-context">
              <strong>脚本上下文</strong>
              <p>目标：{displayName}</p>
              <p>动作：{actionName || "-"}</p>
            </section>
            <label className="runner-node-script-field">
              <span className="runner-node-field-label">脚本内容<button type="button" data-testid="runner-node-script-expand" aria-label="放大脚本编辑器" onClick={() => setScriptModalOpen(true)}><Maximize2 size={14} />放大</button></span>
              <textarea
                data-testid="runner-node-script-editor"
                value={String(stepArgs.script || "")}
                placeholder={actionName === "script.python" ? "print('ok')" : "set -e\necho ok"}
                rows={9}
                spellCheck={false}
                onChange={(event) => updateStepArg("script", event.target.value)}
              />
            </label>
            {(actionName === "script.shell" || actionName === "script.python") ? (
              <label>脚本引用<input value={String(stepArgs.script_ref || "")} placeholder="restore.sh" onChange={(event) => updateStepArg("script_ref", event.target.value)} /></label>
            ) : null}
          </section>
        ) : null}
        {tab === "io" && isCodeAction ? (
          <section className="node-config-form runner-node-io-tab" data-testid="runner-node-io-tab">
            <div className="runner-editor-section-head">
              <strong>输入输出</strong>
              <span>配置当前脚本或命令节点读取的变量，以及它向后续节点暴露的输出。</span>
            </div>
            <RunnerCodeInputVariablesEditor
              inputs={inputs}
              variables={variables}
              onAdd={() => setDraft({ ...draft, inputs: [...inputs, createInputParam(`input_${inputs.length + 1}`)] })}
              onUpdate={updateInput}
              onRemove={removeInput}
              onUpdateSource={updateInputSource}
            />
            <RunnerCodeOutputVariablesEditor
              outputs={outputs}
              onAdd={() => setDraft({ ...draft, outputs: [...outputs, createOutputParam(`output_${outputs.length + 1}`)] })}
              onUpdate={updateOutput}
              onRemove={removeOutput}
            />
          </section>
        ) : null}
        {tab === "aggregate" ? (
          <section className="node-config-form runner-aggregate-editor" data-testid="runner-aggregate-editor">
            <label>输出名称<input value={String(aggregator.output_key || "")} placeholder="aggregated_value" onChange={(event) => updateAggregator({ output_key: event.target.value })} /></label>
            <label>策略<select value={String(aggregator.strategy || "first_non_empty")} onChange={(event) => updateAggregator({ strategy: event.target.value })}><option value="first_non_empty">首个非空值</option><option value="prefer_success">优先成功值</option><option value="array">合并为列表</option></select></label>
            <div className="runner-aggregate-sources">
              <strong>候选变量</strong>
              {aggregatorSources.length ? aggregatorSources.map((source: Record<string, unknown>, index: number) => (
                <div key={`${source.expression}-${index}`} className="runner-aggregate-source">
                  <code>{String(source.expression || "")}</code>
                  <button type="button" onClick={() => updateAggregator({ sources: aggregatorSources.filter((_, itemIndex) => itemIndex !== index) })}>移除</button>
                </div>
              )) : <p>从下方变量列表选择候选源。</p>}
            </div>
            <RunnerVariablePicker variables={variables} onPick={addAggregatorSource} />
          </section>
        ) : null}
        {tab === "branches" ? (
          <section className="node-config-form runner-branch-editor" data-testid="runner-branch-editor">
            <label>条件表达式<input value={String(condition.if || "")} placeholder="deploy == true" onChange={(event) => updateCondition({ if: event.target.value })} /></label>
            <RunnerVariablePicker variables={variables} onPick={appendConditionVariable} />
            <label className="runner-checkbox-field"><input type="checkbox" checked={Boolean(condition.else ?? true)} onChange={(event) => updateCondition({ else: event.target.checked })} />启用兜底分支</label>
            <p>条件成立时走匹配路径；没有条件命中时执行兜底分支。</p>
          </section>
        ) : null}
        {tab === "error" ? (
          <section className="node-config-form runner-error-editor" data-testid="runner-error-editor">
            <label className="runner-checkbox-field"><input type="checkbox" checked={continueOnError} onChange={(event) => updateStep({ continue_on_error: event.target.checked })} />失败后继续执行可用分支</label>
            <label>重试次数<input type="number" min="0" value={Number(draft.step?.retries || 0)} onChange={(event) => updateStep({ retries: Number(event.target.value) || 0 })} /></label>
            <label>超时<input value={String(draft.step?.timeout || "")} placeholder="30s / 5m" onChange={(event) => updateStep({ timeout: event.target.value })} /></label>
          </section>
        ) : null}
        {tab === "advanced" ? <pre>{JSON.stringify(draft, null, 2)}</pre> : null}
        {tab === "last-run" ? <NodeLastRunView nodeId={draft.id} runState={runState} /> : null}
        {scriptModalOpen && actionKind === "script" ? (
          <section className="runner-script-editor-backdrop" data-testid="runner-script-editor-modal" role="dialog" aria-modal="true" aria-label="脚本内容编辑器">
            <div className="runner-script-editor-modal">
              <header>
                <strong>脚本内容</strong>
                <button type="button" data-testid="runner-script-editor-modal-close" aria-label="关闭脚本编辑器" onClick={() => setScriptModalOpen(false)}><X size={16} /></button>
              </header>
              <textarea
                data-testid="runner-script-editor-modal-textarea"
                value={String(stepArgs.script || "")}
                spellCheck={false}
                onChange={(event) => updateStepArg("script", event.target.value)}
              />
            </div>
          </section>
        ) : null}
      </main>
    </aside>
  );
}

function runnerStatusLabel(status: unknown) {
  const normalized = String(status || "").trim().toLowerCase();
  return ({
    pending: "等待运行",
    queued: "等待运行",
    running: "运行中",
    succeeded: "运行成功",
    success: "运行成功",
    completed: "运行完成",
    failed: "运行失败",
    error: "运行失败",
    skipped: "已跳过",
    canceled: "已取消",
    cancelled: "已取消",
  } as Record<string, string>)[normalized] || "暂无状态";
}

function runnerValueTypeLabel(type: unknown) {
  const normalized = String(type || "any").trim().toLowerCase();
  return ({ string: "文本", number: "数字", boolean: "是/否", object: "对象", array: "列表", any: "任意类型" } as Record<string, string>)[normalized] || "自定义类型";
}
