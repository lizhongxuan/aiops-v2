import { useState, type MouseEvent } from "react";
import { BaseEdge, EdgeLabelRenderer, getBezierPath, type EdgeProps } from "@xyflow/react";

import type { WorkflowCanvasDeltaVisual } from "./workflowCanvasProjection";

export function RunnerCanvasEdge(props: EdgeProps) {
  const [edgePath, labelX, labelY] = getBezierPath(props);
  const [insertVisible, setInsertVisible] = useState(false);
  const data = props.data as {
    kind?: string;
    displayKind?: string;
    active?: boolean;
    candidate?: WorkflowCanvasDeltaVisual;
    onSelectEdge?: (edgeId: string) => void;
    onInsertEdge?: (edgeId: string) => void;
    onOpenEdgeMenu?: (edgeId: string, event: MouseEvent<SVGPathElement>) => void;
  } | undefined;
  const isInsertVisible = Boolean(data?.active || insertVisible);
  const candidateCopy = candidateEdgeCopy(data?.candidate);
  const color = candidateEdgeColor(data?.candidate);

  return (
    <>
      {data?.candidate ? <title>{`${candidateCopy.change}${candidateCopy.blocked ? ` · ${candidateCopy.blocked}` : ""}`}</title> : null}
      <BaseEdge
        id={props.id}
        path={edgePath}
        markerEnd={props.markerEnd}
        className={`runner-flow-edge-path ${props.selected ? "is-selected" : ""}`}
        data-testid={`runner-edge-path-${props.id}`}
        data-candidate-change={data?.candidate?.change}
        data-candidate-blocked={data?.candidate?.blocked}
        role={data?.candidate ? "img" : undefined}
        aria-label={[candidateCopy.change, candidateCopy.blocked, data?.candidate?.fieldPaths.length ? `字段 ${data.candidate.fieldPaths.join("、")}` : ""].filter(Boolean).join("，") || undefined}
        style={data?.candidate ? { stroke: color, strokeWidth: data.candidate.blocked ? 3 : 2.5, strokeDasharray: data.candidate.change === "deleted" ? "7 5" : undefined, opacity: data.candidate.change === "deleted" ? 0.65 : 1 } : undefined}
      />
      {data?.onInsertEdge ? (
        <path
          d={edgePath}
          className="runner-flow-edge-hover-path"
          onClick={(event) => {
            event.stopPropagation();
            data.onSelectEdge?.(props.id);
          }}
          onContextMenu={(event) => data.onOpenEdgeMenu?.(props.id, event)}
        />
      ) : null}
      <EdgeLabelRenderer>
        <div
          className={`runner-flow-edge-label ${isInsertVisible || data?.candidate ? "active" : ""}`}
          style={{ transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px)` }}
          data-testid={`runner-edge-${props.id}`}
          onMouseEnter={() => setInsertVisible(true)}
          onMouseLeave={() => setInsertVisible(false)}
        >
          {data?.candidate ? (
            <span
              title={data.candidate.messages.join("；") || data.candidate.fieldPaths.join("、")}
              style={{ display: "inline-flex", alignItems: "center", gap: 3, border: "1px solid currentColor", borderRadius: 999, background: "#fff", color, fontSize: 10, fontWeight: 900, padding: "3px 6px", whiteSpace: "nowrap", pointerEvents: "none" }}
            >
              <span aria-hidden="true">{data.candidate.blocked ? "⚠" : data.candidate.change === "added" ? "+" : data.candidate.change === "deleted" ? "−" : "✎"}</span>
              {candidateCopy.change}{candidateCopy.blocked ? ` · ${candidateCopy.blocked}` : ""}
            </span>
          ) : null}
          {data?.onInsertEdge ? (
            <button
              type="button"
              className="runner-flow-edge-insert"
              aria-label="在连线上插入节点"
              title="插入节点"
              data-testid={`runner-edge-insert-${props.id}`}
              onFocus={() => setInsertVisible(true)}
              onBlur={() => setInsertVisible(false)}
              onClick={(event) => {
                event.stopPropagation();
                data.onInsertEdge?.(props.id);
              }}
            >
              +
            </button>
          ) : null}
        </div>
      </EdgeLabelRenderer>
    </>
  );
}

function candidateEdgeCopy(candidate?: WorkflowCanvasDeltaVisual) {
  return {
    change: candidate?.change === "added" ? "候选新增" : candidate?.change === "deleted" ? "候选删除" : candidate?.change === "changed" ? "候选修改" : candidate?.blocked ? "候选状态" : "",
    blocked: candidate?.blocked === "conflict" ? "冲突" : candidate?.blocked === "warning" ? "警告" : "",
  };
}

export function candidateEdgeColor(candidate?: WorkflowCanvasDeltaVisual) {
  if (candidate?.blocked === "conflict") return "#b91c1c";
  if (candidate?.blocked === "warning") return "#b45309";
  if (candidate?.change === "added") return "#15803d";
  if (candidate?.change === "deleted") return "#dc2626";
  if (candidate?.change === "changed") return "#2563eb";
  return "#94a3b8";
}
