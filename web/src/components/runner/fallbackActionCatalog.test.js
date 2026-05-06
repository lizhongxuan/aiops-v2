import { describe, expect, it } from "vitest";
import { FALLBACK_RUNNER_ACTIONS } from "./fallbackActionCatalog";

describe("FALLBACK_RUNNER_ACTIONS", () => {
  it("keeps structured input and output schemas for core local actions", () => {
    for (const actionName of ["cmd.run", "shell.run", "script.shell", "notify.send"]) {
      const action = FALLBACK_RUNNER_ACTIONS.find((item) => item.action === actionName);

      expect(action?.inputs_schema).toMatchObject({ type: "object", properties: expect.any(Object) });
      expect(action?.input_schema).toEqual(action?.inputs_schema);
      expect(action?.outputs_schema).toMatchObject({ type: "object", properties: expect.any(Object) });
      expect(action?.output_schema).toEqual(action?.outputs_schema);
      expect(action?.outputs_schema.properties).not.toEqual({});
      expect(action?.default_ports?.inputs?.map((port) => port.id)).toContain("in");
      expect(action?.default_ports?.outputs?.length).toBeGreaterThan(0);
      expect(action?.capabilities?.length).toBeGreaterThan(0);
    }

    expect(FALLBACK_RUNNER_ACTIONS.find((item) => item.action === "cmd.run").inputs_schema.required).toContain("cmd");
    expect(FALLBACK_RUNNER_ACTIONS.find((item) => item.action === "shell.run").inputs_schema.required).toContain("script");
  });
});
