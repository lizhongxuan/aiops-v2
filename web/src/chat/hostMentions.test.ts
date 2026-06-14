import { describe, expect, it } from "vitest";

import {
  buildHostMentionMetadata,
  parseHostMentionCandidates,
  uniqueHostMentionKeys,
} from "./hostMentions";

describe("hostMentions", () => {
  it("parses Chinese connector host mentions", () => {
    const result = parseHostMentionCandidates("@1.1.1.1和@1.1.1.2作为pg节点,@1.1.1.3作为pg_mon");

    expect(result.map((item) => item.raw)).toEqual(["@1.1.1.1", "@1.1.1.2", "@1.1.1.3"]);
  });

  it("does not treat email addresses as host mentions", () => {
    expect(parseHostMentionCandidates("联系 sre@example.com")).toEqual([]);
  });

  it("dedupes repeated host tokens", () => {
    const result = parseHostMentionCandidates("@db-1 检查 @db-1");

    expect(uniqueHostMentionKeys(result)).toEqual(["db-1"]);
  });

  it("builds serialized metadata for detected multi-host mentions", () => {
    const mentions = parseHostMentionCandidates("@1.1.1.1 和 @db-1 检查");

    expect(buildHostMentionMetadata(mentions)).toEqual({
      "aiops.hostops.mentions": JSON.stringify(mentions),
      "aiops.hostops.clientDetectedMultiHost": "true",
    });
  });
});
