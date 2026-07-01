import { describe, expect, it } from "vitest";

import { searchCapabilityMentionSuggestions } from "./mentionCatalog";

describe("mentionCatalog", () => {
  it("returns canonical capability suggestions", () => {
    expect(searchCapabilityMentionSuggestions("co")).toEqual([
      expect.objectContaining({
        key: "capability-coroot",
        mention: "@Coroot",
        label: "Coroot",
        kind: "capability",
        path: "capability://coroot",
        payload: { capability: "coroot" },
      }),
    ]);
  });

  it("matches ops manuals aliases", () => {
    expect(searchCapabilityMentionSuggestions("ops_m").map((item) => item.path)).toContain("capability://ops_manuals");
    expect(searchCapabilityMentionSuggestions("manual").map((item) => item.path)).toContain("capability://ops_manuals");
  });
});
