export const WORKFLOW_MANAGER_STATE_KEY = "runner.studio.workflowManager";

function uniqueStrings(values = []) {
  const seen = new Set();
  const result = [];
  for (const value of values) {
    const key = String(value || "").trim();
    if (!key || seen.has(key)) continue;
    seen.add(key);
    result.push(key);
  }
  return result;
}

export function workflowKey(workflow) {
  return workflow?.name || workflow?.id || "";
}

export function createWorkflowManagerState(input = {}) {
  return {
    recent: uniqueStrings(input.recent),
    favorites: uniqueStrings(input.favorites),
  };
}

export function readWorkflowManagerState(storage = globalThis.localStorage, key = WORKFLOW_MANAGER_STATE_KEY) {
  if (!storage) return createWorkflowManagerState();
  try {
    return createWorkflowManagerState(JSON.parse(storage.getItem(key) || "{}"));
  } catch {
    return createWorkflowManagerState();
  }
}

export function writeWorkflowManagerState(state, storage = globalThis.localStorage, key = WORKFLOW_MANAGER_STATE_KEY) {
  const normalized = createWorkflowManagerState(state);
  if (!storage) return normalized;
  try {
    storage.setItem(key, JSON.stringify(normalized));
  } catch {
    // UI preference persistence is best-effort only.
  }
  return normalized;
}

export function recordRecentWorkflow(state, name, limit = 8) {
  const key = String(name || "").trim();
  if (!key) return createWorkflowManagerState(state);
  const normalized = createWorkflowManagerState(state);
  return {
    ...normalized,
    recent: uniqueStrings([key, ...normalized.recent]).slice(0, limit),
  };
}

export function toggleFavoriteWorkflow(state, name) {
  const key = String(name || "").trim();
  const normalized = createWorkflowManagerState(state);
  if (!key) return normalized;
  const favorites = new Set(normalized.favorites);
  if (favorites.has(key)) favorites.delete(key);
  else favorites.add(key);
  return {
    ...normalized,
    favorites: Array.from(favorites),
  };
}

export function getQuickWorkflows(workflows = [], state = createWorkflowManagerState(), limit = 8) {
  const normalized = createWorkflowManagerState(state);
  const workflowByKey = new Map(workflows.map((workflow) => [workflowKey(workflow), workflow]));
  return uniqueStrings([...normalized.favorites, ...normalized.recent])
    .map((key) => workflowByKey.get(key))
    .filter(Boolean)
    .slice(0, limit);
}

export function filterWorkflowManagerItems(workflows = [], options = {}) {
  const query = String(options.query || "").trim().toLowerCase();
  const status = String(options.status || "").trim();
  const includeArchived = Boolean(options.includeArchived);

  return workflows.filter((workflow) => {
    if (!includeArchived && (workflow.archived || workflow.status === "archived")) return false;
    if (status && (workflow.status || "draft") !== status) return false;
    if (!query) return true;
    return [workflow.name, workflow.id, workflow.title, workflow.description]
      .filter(Boolean)
      .some((value) => String(value).toLowerCase().includes(query));
  });
}
