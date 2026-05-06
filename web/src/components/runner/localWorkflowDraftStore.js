const LOCAL_DRAFTS_KEY = "runner.studio.localDrafts";

function canUseLocalStorage() {
  return typeof window !== "undefined" && Boolean(window.localStorage);
}

function readDraftMap() {
  if (!canUseLocalStorage()) return {};
  try {
    const parsed = JSON.parse(window.localStorage.getItem(LOCAL_DRAFTS_KEY) || "{}");
    return parsed && typeof parsed === "object" && !Array.isArray(parsed) ? parsed : {};
  } catch (_err) {
    return {};
  }
}

function writeDraftMap(nextMap) {
  if (!canUseLocalStorage()) return;
  window.localStorage.setItem(LOCAL_DRAFTS_KEY, JSON.stringify(nextMap || {}));
}

function workflowIdForDraft(workflow) {
  const fromId = String(workflow?.id || workflow?.slug || workflow?.graph?.workflow?.name || "").trim();
  if (fromId) return fromId;
  return String(workflow?.name || "").trim();
}

function normalizeDraft(workflow, sequence) {
  const id = workflowIdForDraft(workflow);
  if (!id) {
    throw new Error("local workflow draft requires id, slug, graph.workflow.name, or name");
  }
  const graph = {
    version: workflow?.graph?.version || "v1",
    ...(workflow?.graph || {}),
    workflow: {
      ...(workflow?.graph?.workflow || {}),
      name: workflow?.graph?.workflow?.name || id,
    },
    nodes: Array.isArray(workflow?.graph?.nodes) ? workflow.graph.nodes : [],
    edges: Array.isArray(workflow?.graph?.edges) ? workflow.graph.edges : [],
  };
  const now = new Date().toISOString();
  return {
    ...workflow,
    id,
    name: id,
    title: workflow?.title || workflow?.display_name || workflow?.name || id,
    status: workflow?.status || "draft",
    graph,
    local_draft: true,
    updated_at: workflow?.updated_at || now,
    local_sequence: Number(sequence || workflow?.local_sequence || 0),
  };
}

export function saveLocalWorkflowDraft(workflow) {
  const drafts = readDraftMap();
  const nextSequence =
    Object.values(drafts).reduce((max, draft) => Math.max(max, Number(draft?.local_sequence || 0)), 0) + 1;
  const draft = normalizeDraft(workflow, nextSequence);
  writeDraftMap({
    ...drafts,
    [draft.id]: draft,
  });
  return draft;
}

export function loadLocalWorkflowDraft(workflowId) {
  const id = String(workflowId || "").trim();
  if (!id) return null;
  const draft = readDraftMap()[id];
  return draft ? normalizeDraft(draft) : null;
}

export function listLocalWorkflowDrafts() {
  return Object.values(readDraftMap())
    .map((draft) => normalizeDraft(draft))
    .sort((left, right) => {
      const sequenceDiff = Number(right.local_sequence || 0) - Number(left.local_sequence || 0);
      if (sequenceDiff !== 0) return sequenceDiff;
      return String(right.updated_at || "").localeCompare(String(left.updated_at || ""));
    });
}

export function removeLocalWorkflowDraft(workflowId) {
  const id = String(workflowId || "").trim();
  if (!id) return;
  const drafts = readDraftMap();
  if (!Object.prototype.hasOwnProperty.call(drafts, id)) return;
  delete drafts[id];
  writeDraftMap(drafts);
}
