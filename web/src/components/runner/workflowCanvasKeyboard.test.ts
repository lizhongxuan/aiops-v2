import { describe, expect, it } from "vitest";

import { resolveWorkflowCanvasKeyboardCommand } from "./workflowCanvasKeyboard";

describe("workflowCanvasKeyboard", () => {
  it("maps selection, history, configuration, fit and Candidate navigation explicitly", () => {
    expect(resolveWorkflowCanvasKeyboardCommand({ key: "Delete" }, { selectedNodeId: "node-a" })).toBe("delete_selection");
    expect(resolveWorkflowCanvasKeyboardCommand({ key: "z", metaKey: true }, { canUndo: true })).toBe("undo");
    expect(resolveWorkflowCanvasKeyboardCommand({ key: "Z", ctrlKey: true, shiftKey: true }, { canRedo: true })).toBe("redo");
    expect(resolveWorkflowCanvasKeyboardCommand({ key: "Enter" }, { selectedNodeId: "node-a" })).toBe("open_configuration");
    expect(resolveWorkflowCanvasKeyboardCommand({ key: "Escape" }, { selectedEdgeId: "edge-a" })).toBe("clear_selection");
    expect(resolveWorkflowCanvasKeyboardCommand({ key: "f" }, {})).toBe("fit");
    expect(resolveWorkflowCanvasKeyboardCommand({ key: "ArrowUp", altKey: true }, { candidateChangeCount: 3 })).toBe("previous_candidate_change");
    expect(resolveWorkflowCanvasKeyboardCommand({ key: "ArrowDown", altKey: true }, { candidateChangeCount: 3 })).toBe("next_candidate_change");
    expect(resolveWorkflowCanvasKeyboardCommand({ key: "Enter", metaKey: true }, { candidateReady: true, candidateChangeCount: 3 })).toBe("apply_candidate");
  });

  it("does not steal input, contenteditable, dialog, IME or repeated destructive keys", () => {
    const input = document.createElement("input");
    const editable = document.createElement("div");
    editable.contentEditable = "true";
    const dialog = document.createElement("div");
    dialog.setAttribute("role", "dialog");
    const child = document.createElement("span");
    dialog.appendChild(child);

    for (const target of [input, editable, child]) {
      expect(resolveWorkflowCanvasKeyboardCommand({ key: "Delete", target }, { selectedNodeId: "node-a" })).toBeUndefined();
      expect(resolveWorkflowCanvasKeyboardCommand({ key: "z", ctrlKey: true, target }, { canUndo: true })).toBeUndefined();
    }
    expect(resolveWorkflowCanvasKeyboardCommand({ key: "Delete", isComposing: true }, { selectedNodeId: "node-a" })).toBeUndefined();
    expect(resolveWorkflowCanvasKeyboardCommand({ key: "Delete", repeat: true }, { selectedNodeId: "node-a" })).toBeUndefined();
  });

  it("fails closed when a command is not currently legal", () => {
    expect(resolveWorkflowCanvasKeyboardCommand({ key: "Delete" }, {})).toBeUndefined();
    expect(resolveWorkflowCanvasKeyboardCommand({ key: "a" }, { candidateActive: true })).toBeUndefined();
    expect(resolveWorkflowCanvasKeyboardCommand({ key: "z", ctrlKey: true }, { canUndo: false })).toBeUndefined();
    expect(resolveWorkflowCanvasKeyboardCommand({ key: "Enter", ctrlKey: true }, { candidateReady: false, candidateChangeCount: 3 })).toBeUndefined();
  });
});
