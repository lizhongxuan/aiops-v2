import { describe, expect, it } from "vitest";

import { resolveMcpBundlePresetKey } from "./mcpBundleResolver";
import { MCP_BUNDLE_PRESET_KEYS } from "./mcpBundlePresetRegistry";

describe("mcpBundleResolver", () => {
  it("does not infer provider-specific presets from MCP server or tool prefixes", () => {
    expect(resolveMcpBundlePresetKey({
      source: "coroot",
      mcpServer: "coroot",
      toolName: "coroot.rca_report",
      bundleKind: "monitor_bundle",
    })).toBe(MCP_BUNDLE_PRESET_KEYS.MIDDLEWARE_SERVICE_MONITOR);
  });

  it("uses generic remediation routing from data shape", () => {
    expect(resolveMcpBundlePresetKey({
      toolName: "any.provider.rca",
      rootCause: "connection pool saturated",
    })).toBe(MCP_BUNDLE_PRESET_KEYS.ROOT_CAUSE_REMEDIATION);
  });
});
