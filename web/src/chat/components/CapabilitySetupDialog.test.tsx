import { describe, expect, it } from "vitest";

import { capabilitySetupHref } from "./CapabilitySetupDialog";

describe("CapabilitySetupDialog", () => {
  it.each(["skill", "plugin", "mcp_server", "connector"])("routes %s setup to the unified capabilities entry", (kind) => {
    expect(capabilitySetupHref(kind)).toMatch(/^\/capabilities/);
  });

  it("does not route connectors to the old settings page", () => {
    expect(capabilitySetupHref("connector")).not.toBe("/settings/connectors");
  });
});

