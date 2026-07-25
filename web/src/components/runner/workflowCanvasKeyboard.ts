export type WorkflowCanvasKeyboardCommand =
  | "search"
  | "add_node"
  | "delete_selection"
  | "undo"
  | "redo"
  | "open_configuration"
  | "clear_selection"
  | "fit"
  | "open_ai"
  | "open_issues"
  | "previous_candidate_change"
  | "next_candidate_change"
  | "apply_candidate";

export type WorkflowCanvasKeyboardContext = {
  selectedNodeId?: string;
  selectedEdgeId?: string;
  candidateActive?: boolean;
  candidateReady?: boolean;
  candidateChangeCount?: number;
  canUndo?: boolean;
  canRedo?: boolean;
};

export type WorkflowCanvasKeyboardEvent = {
  key: string;
  code?: string;
  metaKey?: boolean;
  ctrlKey?: boolean;
  altKey?: boolean;
  shiftKey?: boolean;
  repeat?: boolean;
  isComposing?: boolean;
  defaultPrevented?: boolean;
  target?: EventTarget | null;
};

/**
 * Resolves canvas shortcuts without performing UI or mutation side effects.
 * The caller owns preventDefault and command dispatch. Editable controls and
 * IME composition always win over canvas shortcuts.
 */
export function resolveWorkflowCanvasKeyboardCommand(
  event: WorkflowCanvasKeyboardEvent,
  context: WorkflowCanvasKeyboardContext,
): WorkflowCanvasKeyboardCommand | undefined {
  if (event.defaultPrevented || event.isComposing || isEditableEventTarget(event.target)) return undefined;
  const key = event.key.toLowerCase();
  const mod = Boolean(event.metaKey || event.ctrlKey);

  if (mod && key === "k") return "search";
  if (mod && key === "i" && event.shiftKey) return "open_issues";
  if (mod && key === "i") return "open_ai";
  if (mod && key === "enter" && context.candidateReady && (context.candidateChangeCount || 0) > 0) return "apply_candidate";
  if (mod && key === "z" && event.shiftKey && context.canRedo) return "redo";
  if ((mod && key === "y") && context.canRedo) return "redo";
  if (mod && key === "z" && context.canUndo) return "undo";

  if (event.altKey && key === "arrowup" && (context.candidateChangeCount || 0) > 0) return "previous_candidate_change";
  if (event.altKey && key === "arrowdown" && (context.candidateChangeCount || 0) > 0) return "next_candidate_change";
  if (mod || event.altKey) return undefined;

  if ((key === "delete" || key === "backspace") && !event.repeat && !context.candidateActive && (context.selectedNodeId || context.selectedEdgeId)) return "delete_selection";
  if (key === "enter" && context.selectedNodeId) return "open_configuration";
  if (key === "escape" && (context.selectedNodeId || context.selectedEdgeId)) return "clear_selection";
  if (key === "f") return "fit";
  if (key === "a" && !context.candidateActive) return "add_node";
  return undefined;
}

export function isEditableEventTarget(target: EventTarget | null | undefined): boolean {
  if (!(target instanceof HTMLElement)) return false;
  const editableAncestor = target.closest("[contenteditable]");
  return target.isContentEditable || target.contentEditable === "true" ||
    Boolean(editableAncestor && editableAncestor.getAttribute("contenteditable") !== "false") ||
    ["INPUT", "TEXTAREA", "SELECT", "BUTTON"].includes(target.tagName) ||
    Boolean(target.closest('[role="textbox"],[role="dialog"]'));
}
