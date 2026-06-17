import { describe, expect, it } from "vitest";

import { capabilityGroupForKind, defaultCapabilityGroups } from "./contentModel";

describe("capability content model", () => {
  it("uses the converged default top-level groups", () => {
    expect(defaultCapabilityGroups.map((group) => group.title)).toEqual(["常用", "资产", "能力", "文件"]);
  });

  it.each(["skill", "plugin", "mcp_server", "connector"])("groups %s as capability", (kind) => {
    expect(capabilityGroupForKind(kind)).toBe("能力");
  });
});
