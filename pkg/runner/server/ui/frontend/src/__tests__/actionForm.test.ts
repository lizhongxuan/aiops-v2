import { describe, expect, it } from "vitest";
import type { ActionSpec, WorkflowNode } from "../types/workflow";
import {
  createArgPatch,
  createSubflowPatch,
  createStepPatch,
  getActionArgFields,
  getTargetOptions,
  readStepTargets,
  replaceStepFromJSON,
  validateActionArgs,
  validateTargets,
} from "../utils/actionForm";

describe("action form helpers", () => {
  it("renders fields from catalog args_schema with required markers", () => {
    const spec: ActionSpec = {
      action: "script.shell",
      title: "Shell Script",
      category: "script",
      required_args: ["script"],
      args_schema: {
        type: "object",
        properties: {
          script: { type: "string", title: "Shell Script" },
          env: { type: "object", title: "Environment" },
          export_vars: { type: "boolean", title: "Export variables" },
        },
      },
    };

    expect(getActionArgFields(spec, "script.shell")).toEqual([
      expect.objectContaining({ key: "script", kind: "multiline", required: true }),
      expect.objectContaining({ key: "env", kind: "env", required: false }),
      expect.objectContaining({ key: "export_vars", kind: "boolean", required: false }),
    ]);
  });

  it("falls back to common script fields when the catalog omits args_schema", () => {
    const fields = getActionArgFields(
      {
        action: "script.python",
        title: "Stored Python Script",
        category: "script",
      },
      "script.python",
    );

    expect(fields.map((field) => [field.key, field.kind])).toEqual([
      ["script", "multiline"],
      ["args", "string-array"],
      ["dir", "string"],
      ["env", "env"],
      ["export_vars", "boolean"],
    ]);
  });

  it("validates required args, value types, and repair suggestions from schema", () => {
    const spec: ActionSpec = {
      action: "script.shell",
      title: "Shell Script",
      category: "script",
      required_args: ["script"],
      args_schema: {
        type: "object",
        additionalProperties: false,
        properties: {
          script: { type: "string", title: "Shell Script", minLength: 2 },
          export_vars: { type: "boolean", title: "Export variables" },
        },
      },
    };

    expect(validateActionArgs(spec, "script.shell", { export_vars: "yes", extra: true })).toEqual([
      expect.objectContaining({ field: "script", code: "required_arg_missing", severity: "error" }),
      expect.objectContaining({ field: "export_vars", code: "type_mismatch", severity: "error" }),
      expect.objectContaining({ field: "extra", code: "unknown_arg", severity: "warning" }),
      expect.objectContaining({ field: "script", code: "script_reference_missing", severity: "error" }),
    ]);
  });

  it("requires inline scripts for script actions", () => {
    expect(validateActionArgs(undefined, "script.python", {})).toEqual([
      expect.objectContaining({
        field: "script",
        code: "script_reference_missing",
        suggestion: "Provide inline script content in args.script.",
      }),
    ]);
  });

  it("builds target options and validates target capabilities from inventory", () => {
    const workflow = {
      version: "v0.1",
      name: "targets",
      inventory: {
        groups: {
          app: { hosts: ["web-01"], vars: { capabilities: ["script.shell"] } },
        },
        hosts: {
          "web-01": { address: "agent://web-01", vars: { capabilities: ["script.shell"] } },
          "db-01": { address: "agent://db-01", vars: { runner_capabilities: ["script.python"] } },
        },
      },
    };

    expect(getTargetOptions(workflow, ["custom-host"]).map((option) => [option.value, option.targetType])).toEqual([
      ["local", "local"],
      ["app", "group"],
      ["db-01", "host"],
      ["web-01", "host"],
      ["custom-host", "adhoc"],
    ]);

    expect(validateTargets(workflow, "script.python", ["web-01", "custom-host"])).toEqual([
      expect.objectContaining({ field: "targets", code: "capability_mismatch", severity: "warning" }),
      expect.objectContaining({ field: "targets", code: "target_not_in_inventory", severity: "warning" }),
    ]);
  });

  it("creates immutable step patches for targets and args", () => {
    const node: WorkflowNode = {
      id: "restart",
      type: "action",
      position: { x: 0, y: 0 },
      step: {
        name: "restart",
        action: "script.shell",
        target: "old-host",
        args: { script: "uptime" },
      },
    };

    const targetPatch = createStepPatch(node, { targets: ["host-a", "host-b"], target: undefined });
    const argPatch = createArgPatch(node, "dir", "/srv/app");

    expect(readStepTargets(targetPatch.step)).toEqual(["host-a", "host-b"]);
    expect(argPatch.step?.args).toEqual({ script: "uptime", dir: "/srv/app" });
    expect(node.step?.target).toBe("old-host");
    expect(node.step?.args).toEqual({ script: "uptime" });
  });

  it("parses advanced step JSON into a step patch", () => {
    const node: WorkflowNode = {
      id: "probe",
      type: "action",
      position: { x: 0, y: 0 },
      step: { name: "probe", action: "script.shell", args: { script: "echo old" } },
    };

    const patch = replaceStepFromJSON(
      node,
      JSON.stringify({
        name: "probe-hosts",
        action: "script.shell",
        targets: ["local"],
        args: { script: "echo ok" },
        expect_vars: ["READY"],
      }),
    );

    expect(patch.step_name).toBe("probe-hosts");
    expect(patch.step).toMatchObject({
      action: "script.shell",
      targets: ["local"],
      args: { script: "echo ok" },
      expect_vars: ["READY"],
    });
  });

  it("writes handler nodes into handler fields instead of step fields", () => {
    const node: WorkflowNode = {
      id: "notify",
      type: "handler",
      position: { x: 0, y: 0 },
      handler: { name: "notify", action: "script.shell", args: { script: "echo old" } },
    };

    const argPatch = createArgPatch(node, "script", "echo new");
    const jsonPatch = replaceStepFromJSON(node, JSON.stringify({ name: "page-oncall", action: "script.shell", args: { script: "echo page" } }));

    expect(argPatch.step).toBeUndefined();
    expect(argPatch.handler).toMatchObject({ name: "notify", action: "script.shell", args: { script: "echo new" } });
    expect(jsonPatch.handler_name).toBe("page-oncall");
    expect(jsonPatch.handler).toMatchObject({ action: "script.shell", args: { script: "echo page" } });
  });

  it("mirrors subflow workflow and input vars into node.subflow and workflow.run step args", () => {
    const node: WorkflowNode = {
      id: "child",
      type: "subflow",
      position: { x: 0, y: 0 },
      step_name: "child",
      step: {
        name: "child",
        action: "workflow.run",
        args: { workflow: "old-child", vars: { region: "old" } },
      },
      subflow: { workflow_name: "old-child", vars: { region: "old" } },
    };

    const patch = createSubflowPatch(node, {
      workflow_name: "restore-verify",
      vars: { backup_id: "${vars.backup_id}", region: "cn-hz" },
    });

    expect(patch.subflow).toEqual({
      workflow_name: "restore-verify",
      vars: { backup_id: "${vars.backup_id}", region: "cn-hz" },
    });
    expect(patch.step).toEqual({
      name: "child",
      action: "workflow.run",
      args: {
        workflow: "restore-verify",
        vars: { backup_id: "${vars.backup_id}", region: "cn-hz" },
      },
    });
  });
});
