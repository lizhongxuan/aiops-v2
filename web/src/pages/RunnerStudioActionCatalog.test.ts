import { describe, expect, it } from "vitest";

import { normalizeRunnerActionCatalog } from "./RunnerStudioPage";

describe("normalizeRunnerActionCatalog", () => {
  it("projects the exact Runner execution allowlist onto legacy catalog items", () => {
    const actions = normalizeRunnerActionCatalog({
      capabilities: { allowed_actions: ["script.shell"] },
      items: [
        { action: "script.shell", label: "Shell" },
        { action: "builtin.tcp_ping", label: "TCP Ping" },
      ],
    });

    expect(actions).toEqual([
      expect.objectContaining({ action: "script.shell", execution_available: true }),
      expect.objectContaining({ action: "builtin.tcp_ping", execution_available: false }),
    ]);
  });

  it("fails closed when a legacy catalog omits the runtime allowlist", () => {
    expect(normalizeRunnerActionCatalog({ items: [{ action: "script.shell" }] })).toEqual([
      expect.objectContaining({ action: "script.shell", execution_available: false }),
    ]);
  });

  it("accepts the Workflow Edit access-aware catalog shape without trusting legacy metadata", () => {
    const actions = normalizeRunnerActionCatalog({
      items: [
        { spec: { action: "script.shell" }, access: { allowed: true } },
        { spec: { action: "script.python" }, access: { allowed: false, reason: "review required" } },
      ],
    });

    expect(actions).toEqual([
      expect.objectContaining({ action: "script.shell", execution_available: true, access: { allowed: true } }),
      expect.objectContaining({ action: "script.python", execution_available: false, access: expect.objectContaining({ allowed: false }) }),
    ]);
  });
});
