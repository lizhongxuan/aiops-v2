import { BaseEdge, EdgeLabelRenderer, getBezierPath, type EdgeProps } from "@xyflow/react";

export function RunnerCanvasEdge(props: EdgeProps) {
  const [edgePath, labelX, labelY] = getBezierPath(props);
  const kind = String((props.data as { kind?: string } | undefined)?.kind || props.label || "next");

  return (
    <>
      <BaseEdge id={props.id} path={edgePath} markerEnd={props.markerEnd} className="runner-flow-edge-path" />
      <EdgeLabelRenderer>
        <span
          className="runner-flow-edge-label"
          style={{ transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px)` }}
          data-testid={`runner-edge-${props.id}`}
        >
          {kind}
        </span>
      </EdgeLabelRenderer>
    </>
  );
}
