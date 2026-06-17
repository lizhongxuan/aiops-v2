import { describe, expect, it } from "vitest";

import { defaultCapabilitySearchGroups } from "./CapabilitySearchMenu";

describe("CapabilitySearchMenu", () => {
  it("does not expose connector, MCP, or skill as default top-level groups", () => {
    expect(defaultCapabilitySearchGroups.map((group) => group.title)).toEqual(["常用", "资产", "能力", "文件"]);
  });
});
