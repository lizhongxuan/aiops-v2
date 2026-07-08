import { describe, expect, it } from "vitest";

import {
  buildHostMentionMetadata,
  parseHostMentionCandidates,
  parseSpecialAiMentionCandidates,
  uniqueHostMentionKeys,
} from "./hostMentions";

describe("hostMentions", () => {
  it("parses Chinese connector host mentions", () => {
    const result = parseHostMentionCandidates("@1.1.1.1和@1.1.1.2作为pg节点,@1.1.1.3作为pg_mon");

    expect(result.map((item) => item.raw)).toEqual(["@1.1.1.1", "@1.1.1.2", "@1.1.1.3"]);
  });

  it("keeps a host mention boundary before a Chinese noun suffix", () => {
    const result = parseHostMentionCandidates("这是@1.1.1.1主机,查看其内存情况");

    expect(result.map((item) => item.raw)).toEqual(["@1.1.1.1"]);
  });

  it("does not treat email addresses as host mentions", () => {
    expect(parseHostMentionCandidates("联系 sre@example.com")).toEqual([]);
  });

  it("recognizes @local as an explicit local host mention", () => {
    expect(parseHostMentionCandidates("@local 检查系统状态")).toEqual([
      expect.objectContaining({
        raw: "@local",
        value: "local",
        source: "local_alias",
      }),
    ]);
  });

  it("does not treat special AI tool mentions as host-ops mentions", () => {
    expect(parseHostMentionCandidates("请 @Coroot 分析 checkout 根因")).toEqual([]);
    expect(parseHostMentionCandidates("请 @ops_graph 分析业务影响")).toEqual([]);
    expect(parseHostMentionCandidates("请 @ops_manus 搜索运维手册")).toEqual([]);
    expect(parseHostMentionCandidates("请 @ops_manuals 搜索运维手册")).toEqual([]);
  });

  it("parses special AI tool mentions for composer highlighting", () => {
    const result = parseSpecialAiMentionCandidates("请 @coroot 用 @ops_graph 和 @ops_manus 分析");

    expect(result.map((item) => item.raw)).toEqual(["@coroot", "@ops_graph", "@ops_manus"]);
    expect(result.every((item) => item.source === "ai_tool")).toBe(true);
  });

  it("dedupes repeated host tokens", () => {
    const result = parseHostMentionCandidates("@db-1 检查 @db-1");

    expect(uniqueHostMentionKeys(result)).toEqual(["db-1"]);
  });

  it("builds serialized metadata for detected multi-host mentions", () => {
    const mentions = parseHostMentionCandidates("@1.1.1.1 和 @db-1 检查");
    const metadata = buildHostMentionMetadata(mentions);

    expect(JSON.parse(metadata["aiops.hostops.mentions"])).toEqual([
        expect.objectContaining({
          raw: "@1.1.1.1",
          value: "1.1.1.1",
          source: "ip_literal",
          address: "1.1.1.1",
          resolved: false,
        }),
        expect.objectContaining({
          raw: "@db-1",
          value: "db-1",
          source: "hostname_literal",
          address: "db-1",
          resolved: false,
        }),
      ]);
    expect(metadata).toEqual({
      "aiops.hostops.mentions": metadata["aiops.hostops.mentions"],
      "aiops.hostops.clientDetectedMultiHost": "true",
    });
  });

  it("does not emit host-ops metadata when no host mention is selected", () => {
    expect(buildHostMentionMetadata([])).toEqual({});
  });

  it("serializes @local as server-local host metadata", () => {
    const mentions = parseHostMentionCandidates("@local 帮我只读检查 uname");

    expect(JSON.parse(buildHostMentionMetadata(mentions)["aiops.hostops.mentions"])).toEqual([
      expect.objectContaining({
        raw: "@local",
        value: "server-local",
        source: "local_alias",
        hostId: "server-local",
      }),
    ]);
  });

  it("serializes selected inventory host metadata as resolved host mention", () => {
    const mentions = parseHostMentionCandidates("@server-local 查看 CPU");
    const selected = [
      {
        raw: "@server-local",
        value: "server-local",
        hostId: "server-local",
        address: "server-local",
        displayName: "server-local",
        source: "inventory" as const,
        resolved: true,
        confidence: 1,
      },
    ];

    expect(
      JSON.parse(
        buildHostMentionMetadata(mentions, selected)[
          "aiops.hostops.mentions"
        ],
      ),
    ).toEqual([
      expect.objectContaining({
        raw: "@server-local",
        value: "server-local",
        hostId: "server-local",
        address: "server-local",
        displayName: "server-local",
        source: "inventory",
        resolved: true,
      }),
    ]);
  });

  it("normalizes explicit local aliases to server-local metadata", () => {
    for (const raw of [
      "@local",
      "@server-local",
      "@localhost",
      "@127.0.0.1",
    ]) {
      const metadata = buildHostMentionMetadata(
        parseHostMentionCandidates(`${raw} 查看 CPU`),
      );
      const [mention] = JSON.parse(metadata["aiops.hostops.mentions"]);
      expect(mention).toEqual(
        expect.objectContaining({
          hostId: "server-local",
          value: "server-local",
          source: "local_alias",
        }),
      );
    }
  });
});
