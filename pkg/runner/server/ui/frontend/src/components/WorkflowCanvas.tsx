import { useEffect, useMemo, useRef } from "react";
import type { WorkflowGraph } from "../types/workflow";
import type { ControlNodeType } from "../utils/graphEditing";
import { toFlowEdges, toFlowNodes } from "../composables/workflowGraph";

interface WorkflowCanvasProps {
  graph: WorkflowGraph | null;
  selectedNodeId: string | null;
  canUndo: boolean;
  canRedo: boolean;
  canPaste: boolean;
  onSelectNode: (nodeId: string | null) => void;
  onAddAction: (action: string, position: { x: number; y: number }) => void;
  onAddControlNode: (type: ControlNodeType, position: { x: number; y: number }) => void;
  onUpdateNodePosition: (nodeId: string, position: { x: number; y: number }) => void;
  onConnectNodes: (source: string | null | undefined, target: string | null | undefined) => void;
  onDeleteSelected: () => void;
  onCopySelected: () => void;
  onPasteNode: () => void;
  onUndo: () => void;
  onRedo: () => void;
  onAutoLayout: () => void;
}

export default function WorkflowCanvas(props: WorkflowCanvasProps) {
  const canvasRef = useRef<HTMLDivElement | null>(null);
  const nodes = useMemo(() => toFlowNodes(props.graph), [props.graph]);
  const edges = useMemo(() => toFlowEdges(props.graph), [props.graph]);
  const nodeMap = useMemo(() => new Map(nodes.map((node) => [node.id, node])), [nodes]);

  useEffect(() => {
    function handleKeydown(event: KeyboardEvent) {
      if (isEditableTarget(event.target)) return;
      const key = event.key.toLowerCase();
      const command = event.metaKey || event.ctrlKey;
      if (command && key === "c" && props.selectedNodeId) {
        event.preventDefault();
        props.onCopySelected();
      } else if (command && key === "v" && props.canPaste) {
        event.preventDefault();
        props.onPasteNode();
      } else if (command && key === "z" && event.shiftKey && props.canRedo) {
        event.preventDefault();
        props.onRedo();
      } else if (command && key === "z" && props.canUndo) {
        event.preventDefault();
        props.onUndo();
      } else if ((event.key === "Delete" || event.key === "Backspace") && props.selectedNodeId) {
        event.preventDefault();
        props.onDeleteSelected();
      }
    }
    window.addEventListener("keydown", handleKeydown);
    return () => window.removeEventListener("keydown", handleKeydown);
  }, [props]);

  function canvasPoint(event: React.DragEvent | React.MouseEvent) {
    const rect = canvasRef.current?.getBoundingClientRect();
    return {
      x: Math.round(event.clientX - (rect?.left || 0)),
      y: Math.round(event.clientY - (rect?.top || 0)),
    };
  }

  function handleDrop(event: React.DragEvent) {
    const action = event.dataTransfer.getData("application/runner-action");
    const nodeType = event.dataTransfer.getData("application/runner-node-type") as ControlNodeType;
    if (!action && !nodeType) return;
    event.preventDefault();
    const position = canvasPoint(event);
    if (action) {
      props.onAddAction(action, position);
      return;
    }
    props.onAddControlNode(nodeType, position);
  }

  return (
    <section className="canvas-panel">
      <div className="canvas-toolbar">
        <div>
          <span>⑂</span>
          <span>Graph</span>
        </div>
        <div className="canvas-toolbar-actions">
          <small>
            {nodes.length} nodes / {edges.length} edges
          </small>
          <button className="icon-button" type="button" title="Undo" disabled={!props.canUndo} onClick={props.onUndo}>
            ↶
          </button>
          <button className="icon-button" type="button" title="Redo" disabled={!props.canRedo} onClick={props.onRedo}>
            ↷
          </button>
          <button className="icon-button" type="button" title="Copy selected node" disabled={!props.selectedNodeId} onClick={props.onCopySelected}>
            ⧉
          </button>
          <button className="icon-button" type="button" title="Paste node" disabled={!props.canPaste} onClick={props.onPasteNode}>
            ⧠
          </button>
          <button className="icon-button" type="button" title="Auto layout" onClick={props.onAutoLayout}>
            ⊞
          </button>
          <button className="icon-button" type="button" title="Delete selected node" disabled={!props.selectedNodeId} onClick={props.onDeleteSelected}>
            ×
          </button>
        </div>
      </div>
      <div
        ref={canvasRef}
        className="workflow-canvas react-workflow-canvas"
        onClick={() => props.onSelectNode(null)}
        onDragOver={(event) => {
          if (event.dataTransfer.types.includes("application/runner-action") || event.dataTransfer.types.includes("application/runner-node-type")) {
            event.preventDefault();
            event.dataTransfer.dropEffect = "copy";
          }
        }}
        onDrop={handleDrop}
      >
        <svg className="workflow-edge-layer">
          {edges.map((edge) => {
            const source = nodeMap.get(edge.source);
            const target = nodeMap.get(edge.target);
            if (!source || !target) return null;
            const x1 = source.position.x + 168;
            const y1 = source.position.y + 28;
            const x2 = target.position.x;
            const y2 = target.position.y + 28;
            const mid = Math.max(24, (x2 - x1) / 2);
            return (
              <g key={edge.id} className={edge.className}>
                <path d={`M ${x1} ${y1} C ${x1 + mid} ${y1}, ${x2 - mid} ${y2}, ${x2} ${y2}`} />
                {edge.label ? (
                  <text x={(x1 + x2) / 2} y={(y1 + y2) / 2 - 6}>
                    {edge.label}
                  </text>
                ) : null}
              </g>
            );
          })}
        </svg>
        {nodes.map((node) => (
          <button
            key={node.id}
            type="button"
            className={`${node.className} ${node.id === props.selectedNodeId ? "is-selected" : ""}`}
            style={{ left: node.position.x, top: node.position.y }}
            onClick={(event) => {
              event.stopPropagation();
              props.onSelectNode(node.id);
            }}
            draggable
            onDragEnd={(event) => {
              const point = canvasPoint(event);
              props.onUpdateNodePosition(node.id, point);
            }}
          >
            <strong>{node.label}</strong>
            <small>{node.data.action || node.data.nodeType}</small>
          </button>
        ))}
      </div>
    </section>
  );
}

function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  const tag = target.tagName.toLowerCase();
  return target.isContentEditable || tag === "input" || tag === "textarea" || tag === "select";
}
