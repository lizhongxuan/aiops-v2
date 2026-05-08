import { Handle, Position, type NodeProps } from "@xyflow/react";

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
  onOpenConfig?: (nodeId: string) => void;
  onNodeAction?: (action: string, nodeId: string) => void;
};

export function RunnerCanvasNode({ id, data, selected }: NodeProps) {
  const nodeData = data as RunnerCanvasNodeData;
  const meta = nodeData.meta || {};
  const inputs = nodeData.ports?.inputs || [];
  const outputs = nodeData.ports?.outputs || [];
  const label = nodeData.label || nodeData.node?.label || nodeData.node?.step?.name || id;
  const action = meta.action || nodeData.node?.step?.action || nodeData.node?.type || "node";

  return (
    <div
      className={["runner-canvas-node", selected ? "selected" : "", `tone-${meta.tone || "slate"}`].filter(Boolean).join(" ")}
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
      <div className="runner-canvas-node-head">
        <span className="runner-canvas-node-icon">{meta.iconText || "RUN"}</span>
        <div>
          <strong>{label}</strong>
          <small>{action}</small>
        </div>
      </div>
      <p>{meta.description || meta.category || "工作流节点"}</p>
      <div className="runner-canvas-node-foot">
        <span>{meta.risk || "low"}</span>
        <span>{inputs.length} in · {outputs.length} out</span>
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
