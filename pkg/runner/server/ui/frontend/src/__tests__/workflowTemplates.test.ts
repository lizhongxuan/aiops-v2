import { describe, expect, it } from "vitest";
import { createWorkflowGraphFromTemplate, prepareWorkflowGraphForCreate } from "../utils/workflowTemplates";
import type { WorkflowGraph } from "../types/workflow";

describe("workflow templates", () => {
  it("creates a runnable cmd.run starter graph without runtime state", () => {
    const graph = createWorkflowGraphFromTemplate({
      kind: "cmd-run-basic",
      name: "visual-create-smoke",
      version: "v0.1",
      description: "smoke workflow",
    });

    expect(graph.workflow).toMatchObject({ name: "visual-create-smoke", version: "v0.1", description: "smoke workflow" });
    expect(graph.layout?.direction).toBe("LR");
    expect(graph.nodes.map((node) => [node.id, node.type])).toEqual([
      ["start", "start"],
      ["run-command", "action"],
      ["end", "end"],
    ]);
    expect(graph.nodes[1].step).toEqual({
      id: "run-command",
      name: "run-command",
      action: "cmd.run",
      args: { cmd: "echo hello" },
    });
    expect(graph.edges.map((edge) => [edge.id, edge.source, edge.target, edge.kind])).toEqual([
      ["start-run-command", "start", "run-command", "next"],
      ["run-command-end", "run-command", "end", "next"],
    ]);
    expect(JSON.stringify(graph)).not.toContain("resource_version");
    expect(JSON.stringify(graph)).not.toContain('"state"');
  });

  it("creates shell and manual approval templates with production defaults", () => {
    const shell = createWorkflowGraphFromTemplate({ kind: "shell-run-basic", name: "shell-create", version: "v0.1" });
    expect(shell.nodes.find((node) => node.id === "run-shell")?.step).toMatchObject({
      id: "run-shell",
      name: "run-shell",
      action: "shell.run",
      args: { script: "echo hello" },
    });

    const approval = createWorkflowGraphFromTemplate({ kind: "manual-approval-basic", name: "approval-create", version: "v0.1" });
    expect(approval.nodes.find((node) => node.id === "approve")?.approval).toEqual({
      subjects: ["ops"],
      timeout: "30m",
      on_timeout: "reject",
    });
    expect(approval.edges.map((edge) => edge.id)).toEqual(["start-approve", "approve-run-command", "run-command-end"]);
  });

  it("prepares cloned graphs for create by replacing metadata and clearing runtime state", () => {
    const source: WorkflowGraph = {
      version: "v1",
      workflow: { version: "v1", name: "source" },
      ui: { resource_version: "sha256:old", theme: "dark" },
      nodes: [
        { id: "start", type: "start", position: { x: 0, y: 0 }, state: { status: "success" } },
        { id: "end", type: "end", position: { x: 240, y: 0 }, state: { status: "running" } },
      ],
      edges: [{ id: "start-end", source: "start", target: "end", kind: "next", state: { status: "selected" } }],
    };

    const cloned = prepareWorkflowGraphForCreate(source, {
      name: "target",
      version: "v0.2",
      description: "cloned",
    });

    expect(cloned.workflow).toMatchObject({ name: "target", version: "v0.2", description: "cloned" });
    expect(cloned.ui).toEqual({ theme: "dark" });
    expect(cloned.nodes.every((node) => node.state === undefined)).toBe(true);
    expect(cloned.edges.every((edge) => edge.state === undefined)).toBe(true);
    expect(source.workflow.name).toBe("source");
  });
});
