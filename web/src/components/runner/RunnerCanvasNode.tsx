import { Handle, Position, type NodeProps } from "@xyflow/react";
import { AlertTriangle, CheckCircle2, LoaderCircle } from "lucide-react";

import type { RunnerNode, RunnerPort } from "./canvasGraphAdapter";

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

  return (
    <div
      className={["runner-canvas-node", selected ? "selected" : "", runStatus ? `run-${runStatus}` : "", `tone-${meta.tone || "slate"}`].filter(Boolean).join(" ")}
      data-testid={`canvas-node-${id}`}
      onDoubleClick={(event) => {
        event.stopPropagation();
        nodeData.onOpenConfig?.(id);
      }}
      onContextMenu={(event) => {
        event.preventDefault();
        event.stopPropagation();
        nodeData.onNodeAction?.("open-menu", id);
      }}
    >
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
