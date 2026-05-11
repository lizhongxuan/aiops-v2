import { useState, type MouseEvent } from "react";
import { BaseEdge, EdgeLabelRenderer, getBezierPath, type EdgeProps } from "@xyflow/react";

export function RunnerCanvasEdge(props: EdgeProps) {
  const [edgePath, labelX, labelY] = getBezierPath(props);
  const [insertVisible, setInsertVisible] = useState(false);
  const data = props.data as {
    kind?: string;
    displayKind?: string;
    active?: boolean;
    onInsertEdge?: (edgeId: string) => void;
    onDeleteEdge?: (edgeId: string) => void;
    onOpenEdgeMenu?: (edgeId: string, event: MouseEvent<SVGPathElement>) => void;
  } | undefined;
  const isInsertVisible = Boolean(data?.active || insertVisible);

  return (
    <>
      <BaseEdge id={props.id} path={edgePath} markerEnd={props.markerEnd} className="runner-flow-edge-path" />
      {data?.onInsertEdge ? (
        <path
          d={edgePath}
          className="runner-flow-edge-hover-path"
          onContextMenu={(event) => data.onOpenEdgeMenu?.(props.id, event)}
        />
      ) : null}
      <EdgeLabelRenderer>
        <div
          className={`runner-flow-edge-label ${isInsertVisible ? "active" : ""}`}
          style={{ transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px)` }}
          data-testid={`runner-edge-${props.id}`}
          onMouseEnter={() => setInsertVisible(true)}
          onMouseLeave={() => setInsertVisible(false)}
        >
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
          {data?.onDeleteEdge ? (
            <button
              type="button"
              className="runner-flow-edge-delete"
              aria-label="删除连线"
              title="删除连线"
              data-testid={`runner-edge-delete-${props.id}`}
              onFocus={() => setInsertVisible(true)}
              onBlur={() => setInsertVisible(false)}
              onClick={(event) => {
                event.stopPropagation();
                data.onDeleteEdge?.(props.id);
              }}
            >
              ×
            </button>
          ) : null}
        </div>
      </EdgeLabelRenderer>
    </>
  );
}
