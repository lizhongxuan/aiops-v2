import type {
  WorkflowCandidateActionTarget,
  WorkflowDraftDryRunTarget,
  WorkflowEnvironmentConfigurationTarget,
  WorkflowHistoryActionTarget,
  WorkflowPanelActionTarget,
  WorkflowPublishedRevisionTarget,
  WorkflowSavedDraftTarget,
  WorkflowStudioActionCapability,
  WorkflowStudioActionViewModel,
} from "./workspace/workflowInteractionContract";

export type WorkflowStudioActionTargetById = {
  apply_changes: WorkflowCandidateActionTarget;
  resolve_conflict: WorkflowCandidateActionTarget;
  check: WorkflowSavedDraftTarget;
  fix_issues: WorkflowPanelActionTarget;
  configure: WorkflowEnvironmentConfigurationTarget;
  dry_run: WorkflowDraftDryRunTarget;
  publish: WorkflowSavedDraftTarget;
  publish_changes: WorkflowSavedDraftTarget;
  run_published: WorkflowPublishedRevisionTarget;
  undo: WorkflowHistoryActionTarget;
  redo: WorkflowHistoryActionTarget;
};

export type WorkflowStudioActionHandler<
  ActionId extends WorkflowStudioActionViewModel["id"] = WorkflowStudioActionViewModel["id"],
> = (target: WorkflowStudioActionTargetById[ActionId]) => void | Promise<void>;

export type WorkflowStudioActionHandlers = Partial<{
  [ActionId in WorkflowStudioActionViewModel["id"]]:
    WorkflowStudioActionHandler<ActionId>;
}>;

export type WorkflowStudioActionCapabilities = Partial<
  Record<WorkflowStudioActionCapability, boolean>
>;

export type WorkflowStudioActionRegistry = {
  canExecute(action: WorkflowStudioActionViewModel): boolean;
  execute(action: WorkflowStudioActionViewModel): Promise<void>;
  registeredActionIds(): WorkflowStudioActionViewModel["id"][];
};

export class WorkflowStudioActionUnavailableError extends Error {
  readonly actionId: WorkflowStudioActionViewModel["id"];

  constructor(action: WorkflowStudioActionViewModel) {
    super(`workflow studio action ${action.id} is unavailable`);
    this.name = "WorkflowStudioActionUnavailableError";
    this.actionId = action.id;
  }
}

/**
 * Creates the only executor used by desktop, mobile and confirmation
 * presenters. An action exists only when both its projected capability and
 * its exact handler exist. There is deliberately no legacy Graph fallback.
 */
export function createWorkflowStudioActionRegistry({
  capabilities,
  handlers,
}: {
  capabilities: WorkflowStudioActionCapabilities;
  handlers: WorkflowStudioActionHandlers;
}): WorkflowStudioActionRegistry {
  const inFlight = new Map<string, Promise<void>>();

  function canExecute(action: WorkflowStudioActionViewModel): boolean {
    return action.enabled &&
      capabilities[action.capability] === true &&
      typeof handlers[action.id] === "function";
  }

  function execute(action: WorkflowStudioActionViewModel): Promise<void> {
    if (!canExecute(action)) return Promise.reject(new WorkflowStudioActionUnavailableError(action));
    const key = workflowStudioActionIdentity(action);
    const existing = inFlight.get(key);
    if (existing) return existing;

    const handler = handlers[action.id] as WorkflowStudioActionHandler;
    let result: void | Promise<void>;
    try {
      result = handler(action.target);
    } catch (error) {
      return Promise.reject(error);
    }
    const execution = Promise.resolve(result)
      .then(() => undefined)
      .finally(() => {
        if (inFlight.get(key) === execution) inFlight.delete(key);
      });
    inFlight.set(key, execution);
    return execution;
  }

  function registeredActionIds(): WorkflowStudioActionViewModel["id"][] {
    return Object.keys(handlers)
      .filter((id): id is WorkflowStudioActionViewModel["id"] =>
        typeof handlers[id as WorkflowStudioActionViewModel["id"]] === "function")
      .filter((id) => capabilities[capabilityForActionId(id)] === true)
      .sort();
  }

  return { canExecute, execute, registeredActionIds };
}

function capabilityForActionId(
  id: WorkflowStudioActionViewModel["id"],
): WorkflowStudioActionCapability {
  if (id === "check") return "workflow.validate";
  if (id === "dry_run") return "workflow.draft_dry_run";
  if (id === "publish" || id === "publish_changes") return "workflow.publish";
  if (id === "run_published") return "workflow.run_published";
  if (id === "fix_issues") return "workflow.review_issues";
  if (id === "configure") return "workflow.configure_environment";
  if (id === "apply_changes") return "workflow.apply_candidate";
  if (id === "resolve_conflict") return "workflow.resolve_conflict";
  return id === "undo" ? "workflow.undo" : "workflow.redo";
}

function workflowStudioActionIdentity(action: WorkflowStudioActionViewModel): string {
  return `${action.id}:${stableStructuredValue(action.target)}`;
}

function stableStructuredValue(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map(stableStructuredValue).join(",")}]`;
  if (value && typeof value === "object") {
    return `{${Object.entries(value)
      .sort(([left], [right]) => left.localeCompare(right))
      .map(([key, item]) => `${key}:${stableStructuredValue(item)}`)
      .join(",")}}`;
  }
  return JSON.stringify(value);
}
