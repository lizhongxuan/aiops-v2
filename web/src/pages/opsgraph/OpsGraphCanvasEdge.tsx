import { BaseEdge, EdgeLabelRenderer, getBezierPath, type EdgeProps } from "@xyflow/react";

export function OpsGraphCanvasEdge(props: EdgeProps) {
  const relationship = props.data?.relationship as { id?: string; type?: string; properties?: Record<string, string> } | undefined;
  const onSelectRelationship = props.data?.onSelectRelationship as ((relationshipId: string) => void) | undefined;
  const toneClass = relationship?.type === "publishes" || relationship?.type === "consumes"
    ? "border-violet-100 text-violet-700"
    : relationship?.type === "proxies_to"
      ? "border-emerald-100 text-emerald-700"
      : "border-slate-100 text-slate-600";
  const details = [relationship?.properties?.protocol, relationship?.properties?.port, relationship?.properties?.path].filter(Boolean).join(" · ");
  const selectedClass = props.selected ? "ring-2 ring-slate-300" : "";
  const laneOffset = Number(props.data?.laneOffset || 0);
  const [path, labelX, labelY] = laneOffset ? getLaneBezierPath({
    sourceX: props.sourceX,
    sourceY: props.sourceY,
    targetX: props.targetX,
    targetY: props.targetY,
    laneOffset,
  }) : getBezierPath({
    sourceX: props.sourceX,
    sourceY: props.sourceY,
    sourcePosition: props.sourcePosition,
    targetX: props.targetX,
    targetY: props.targetY,
    targetPosition: props.targetPosition,
  });
  return (
    <>
      <BaseEdge path={path} markerEnd={props.markerEnd} style={props.style} interactionWidth={props.interactionWidth || 26} />
      {props.label ? (
        <EdgeLabelRenderer>
          <button
            type="button"
            style={{
              pointerEvents: "all",
              transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px)`,
              zIndex: 20,
            }}
            title={details || undefined}
            className={`nodrag nopan absolute cursor-pointer rounded-full border bg-white px-2 py-0.5 text-xs shadow-sm ${toneClass} ${selectedClass}`}
            onClick={(event) => {
              event.stopPropagation();
              if (relationship?.id) {
                onSelectRelationship?.(relationship.id);
              }
            }}
          >
            {props.label}
          </button>
        </EdgeLabelRenderer>
      ) : null}
    </>
  );
}

function getLaneBezierPath({
  laneOffset,
  sourceX,
  sourceY,
  targetX,
  targetY,
}: {
  laneOffset: number;
  sourceX: number;
  sourceY: number;
  targetX: number;
  targetY: number;
}): [string, number, number] {
  const direction = targetX >= sourceX ? 1 : -1;
  const controlDistance = Math.max(72, Math.abs(targetX - sourceX) * 0.45);
  const c1x = sourceX + controlDistance * direction;
  const c2x = targetX - controlDistance * direction;
  const c1y = sourceY + laneOffset;
  const c2y = targetY + laneOffset;
  const labelX = (sourceX + targetX) / 2;
  const labelY = (sourceY + targetY) / 2 + laneOffset;
  return [`M${sourceX},${sourceY} C${c1x},${c1y} ${c2x},${c2y} ${targetX},${targetY}`, labelX, labelY];
}
