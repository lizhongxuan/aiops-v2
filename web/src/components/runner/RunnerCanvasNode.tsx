import { Handle, Position, type NodeProps } from "@xyflow/react";
import { AlertTriangle, CheckCircle2, CirclePlus, LoaderCircle, Pencil, ShieldAlert, Trash2 } from "lucide-react";

import type { RunnerNode, RunnerPort } from "./canvasGraphAdapter";
import type { WorkflowCanvasDeltaVisual } from "./workflowCanvasProjection";

type RunnerCanvasNodeData = {
  label?: string;
  meta?: {
    action?: string;
    category?: string;
    description?: string;
    displayLabel?: string;
    iconText?: string;
    risk?: string;
    tone?: string;
  };
  ports?: {
    inputs?: RunnerPort[];
    outputs?: RunnerPort[];
  };
  node?: RunnerNode;
  runState?: {
    status?: string;
    label?: string;
    message?: string;
  };
  aiHighlighted?: boolean;
  candidate?: WorkflowCanvasDeltaVisual;
  candidateReadOnly?: boolean;
  onOpenConfig?: (nodeId: string) => void;
  onNodeAction?: (action: string, nodeId: string) => void;
};

export function RunnerCanvasNode({ id, data, selected }: NodeProps) {
  const nodeData = data as RunnerCanvasNodeData;
  const meta = nodeData.meta || {};
  const inputs = nodeData.ports?.inputs || [];
  const outputs = nodeData.ports?.outputs || [];
  const label = nodeData.label || nodeData.node?.label || nodeData.node?.step?.name || id;
  const runStatus = nodeData.runState?.status || "";
  const runLabel = nodeData.runState?.label || "";
  const runMessage = compactRunMessage(nodeData.runState?.message || "");
  const failureMessage = ["failed", "error"].includes(String(runStatus).toLowerCase()) ? runMessage : "";
  const candidate = nodeData.candidate;
  const candidateCopy = candidateNodeCopy(candidate);
  const CandidateIcon = candidate?.blocked === "conflict" ? ShieldAlert : candidate?.blocked === "warning" ? AlertTriangle : candidate?.change === "added" ? CirclePlus : candidate?.change === "deleted" ? Trash2 : Pencil;

  return (
    <div
      className={["runner-canvas-node", selected ? "selected" : "", nodeData.aiHighlighted ? "ai-highlighted" : "", runStatus ? `run-${runStatus}` : "", `tone-${meta.tone || "slate"}`].filter(Boolean).join(" ")}
      data-testid={`canvas-node-${id}`}
      data-candidate-change={candidate?.change}
      data-candidate-blocked={candidate?.blocked}
      aria-label={[label, candidateCopy.change, candidateCopy.blocked, candidate?.fieldPaths.length ? `字段 ${candidate.fieldPaths.join("、")}` : ""].filter(Boolean).join("，")}
      style={candidateNodeStyle(candidate)}
      onDoubleClick={(event) => {
        event.stopPropagation();
        if (nodeData.candidateReadOnly) return;
        nodeData.onOpenConfig?.(id);
      }}
      onContextMenu={(event) => {
        event.preventDefault();
        event.stopPropagation();
        if (nodeData.candidateReadOnly) return;
        nodeData.onNodeAction?.("open-menu", id);
      }}
    >
      {candidate ? (
        <div
          data-testid={`canvas-node-${id}-candidate`}
          title={candidate.messages.join("；") || candidate.fieldPaths.join("、")}
          style={{ position: "absolute", left: 8, top: -16, zIndex: 3, display: "inline-flex", alignItems: "center", gap: 4, border: "1px solid currentColor", borderRadius: 999, background: "#fff", color: candidateColor(candidate), fontSize: 10, fontWeight: 900, lineHeight: 1, padding: "4px 6px", whiteSpace: "nowrap" }}
        >
          <CandidateIcon size={11} aria-hidden="true" />
          <span>{candidateCopy.change}</span>
          {candidateCopy.blocked ? <span>· {candidateCopy.blocked}</span> : null}
        </div>
      ) : null}
      {runStatus ? (
        <div className={`runner-canvas-node-run-state status-${runStatus}`} title={nodeData.runState?.message || runLabel}>
          {runStatus === "running" ? <LoaderCircle className="runner-canvas-node-run-spinner" size={13} /> : null}
          {runStatus === "failed" ? <AlertTriangle size={13} /> : null}
          {runStatus === "success" ? <CheckCircle2 size={13} /> : null}
          <span>{runLabel}</span>
        </div>
      ) : null}
      <div className="runner-canvas-node-head">
        <span className="runner-canvas-node-icon">{meta.iconText || "RUN"}</span>
        <div>
          <strong>{label}</strong>
        </div>
      </div>
      {candidate?.fieldPaths.length ? (
        <small style={{ overflow: "hidden", color: candidateColor(candidate), fontSize: 10, fontWeight: 800, textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
          {candidate.fieldPaths.join(" · ")}
        </small>
      ) : null}
      {failureMessage ? (
        <div className="runner-canvas-node-run-message" data-testid={`canvas-node-${id}-failure-message`} title={failureMessage}>
          {failureMessage}
        </div>
      ) : null}
      {inputs.map((port, index) => (
        <Handle
          key={`in-${port.id}`}
          id={port.id}
          type="target"
          position={Position.Left}
          className="runner-canvas-handle input"
          style={{ top: `${32 + index * 24}px` }}
          title={port.label || port.id}
        />
      ))}
      {outputs.map((port, index) => (
        <Handle
          key={`out-${port.id}`}
          id={port.id}
          type="source"
          position={Position.Right}
          className="runner-canvas-handle output"
          style={{ top: `${32 + index * 24}px` }}
          title={port.label || port.id}
        />
      ))}
    </div>
  );
}

function candidateNodeCopy(candidate?: WorkflowCanvasDeltaVisual) {
  return {
    change: candidate?.change === "added" ? "候选新增" : candidate?.change === "deleted" ? "候选删除" : candidate?.change === "changed" ? "候选修改" : candidate?.blocked ? "候选状态" : "",
    blocked: candidate?.blocked === "conflict" ? "冲突" : candidate?.blocked === "warning" ? "警告" : "",
  };
}

function candidateColor(candidate?: WorkflowCanvasDeltaVisual) {
  if (candidate?.blocked === "conflict") return "#b91c1c";
  if (candidate?.blocked === "warning") return "#b45309";
  if (candidate?.change === "added") return "#15803d";
  if (candidate?.change === "deleted") return "#b91c1c";
  return "#1d4ed8";
}

function candidateNodeStyle(candidate?: WorkflowCanvasDeltaVisual) {
  if (!candidate) return undefined;
  const color = candidateColor(candidate);
  return {
    borderColor: color,
    borderStyle: candidate.change === "deleted" ? "dashed" : "solid",
    background: candidate.change === "added" ? "#f0fdf4" : candidate.change === "deleted" ? "#fef2f2" : candidate.blocked === "warning" ? "#fffbeb" : "#eff6ff",
    opacity: candidate.change === "deleted" ? 0.72 : 1,
    boxShadow: `0 0 0 3px ${color}22, 0 8px 18px rgba(15,23,42,.08)`,
  } as const;
}

function compactRunMessage(message: string) {
  return String(message || "").replace(/\s+/g, " ").trim();
}
