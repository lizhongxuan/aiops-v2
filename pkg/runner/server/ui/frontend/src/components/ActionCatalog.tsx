import { useMemo, useState } from "react";
import type { ActionSpec } from "../types/workflow";
import { filterActionCatalog } from "../utils/actionCatalog";
import { filterControlNodes, type ControlNodeType } from "../utils/graphEditing";

interface ActionCatalogProps {
  actions: ActionSpec[];
  onAddAction: (action: string) => void;
  onAddControlNode: (type: ControlNodeType) => void;
}

export default function ActionCatalog({ actions, onAddAction, onAddControlNode }: ActionCatalogProps) {
  const [query, setQuery] = useState("");
  const groups = useMemo(() => filterActionCatalog(actions, query), [actions, query]);
  const controlNodes = useMemo(() => filterControlNodes(query), [query]);
  const visibleCount = groups.reduce((total, group) => total + group.actions.length, 0) + controlNodes.length;

  return (
    <aside className="library-panel">
      <div className="panel-heading">
        <span>Actions</span>
        <small>
          {visibleCount} / {actions.length}
        </small>
      </div>

      <label className="catalog-search">
        <span>⌕</span>
        <input value={query} type="search" placeholder="Search actions" onChange={(event) => setQuery(event.target.value)} />
      </label>

      {controlNodes.length || groups.length ? (
        <div className="catalog-groups">
          {controlNodes.length ? (
            <section className="catalog-group">
              <div className="catalog-group-heading">
                <span>Graph controls</span>
                <small>{controlNodes.length}</small>
              </div>
              {controlNodes.map((control) => (
                <button
                  key={control.type}
                  className="action-card control-card"
                  draggable
                  type="button"
                  onClick={() => onAddControlNode(control.type)}
                  onDragStart={(event) => handleControlDragStart(event, control.type)}
                >
                  <span className="action-card-title">
                    <strong>{control.title}</strong>
                    <small className="risk-badge risk-read_only">{control.type}</small>
                  </span>
                  <span className="action-description">{control.description}</span>
                </button>
              ))}
            </section>
          ) : null}

          {groups.map((group) => (
            <section key={group.category} className="catalog-group">
              <div className="catalog-group-heading">
                <span>{group.category}</span>
                <small>{group.actions.length}</small>
              </div>
              {group.actions.map((action) => (
                <button
                  key={action.action}
                  className="action-card"
                  draggable
                  type="button"
                  onClick={() => onAddAction(action.action)}
                  onDragStart={(event) => handleDragStart(event, action)}
                >
                  <span className="action-card-title">
                    <strong>{action.title}</strong>
                    <small className={`risk-badge ${riskClass(action)}`}>{riskLabel(action)}</small>
                  </span>
                  <small>{action.action}</small>
                  {action.description ? <span className="action-description">{action.description}</span> : null}
                  <span className="action-flags">
                    {action.experimental ? <small>experimental</small> : null}
                    {action.deprecated ? <small>deprecated</small> : null}
                    {action.required_args?.length ? <small>requires {action.required_args.join(", ")}</small> : null}
                  </span>
                </button>
              ))}
            </section>
          ))}
        </div>
      ) : (
        <div className="empty-state">No actions match this search.</div>
      )}
    </aside>
  );
}

function riskClass(action: ActionSpec) {
  return `risk-${(action.risk || "medium").replace(/[^a-z0-9_-]/gi, "_")}`;
}

function riskLabel(action: ActionSpec) {
  return action.risk || "medium";
}

function handleDragStart(event: React.DragEvent, action: ActionSpec) {
  event.dataTransfer.setData("application/runner-action", action.action);
  event.dataTransfer.setData("text/plain", action.action);
  event.dataTransfer.effectAllowed = "copy";
}

function handleControlDragStart(event: React.DragEvent, type: ControlNodeType) {
  event.dataTransfer.setData("application/runner-node-type", type);
  event.dataTransfer.setData("text/plain", type);
  event.dataTransfer.effectAllowed = "copy";
}
