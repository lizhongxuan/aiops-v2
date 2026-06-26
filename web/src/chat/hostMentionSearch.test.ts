import { describe, expect, it } from "vitest";

import {
  findActiveHostMentionToken,
  replaceActiveHostMention,
  searchHostMentionSuggestions,
  type HostMentionSuggestion,
} from "./hostMentionSearch";

describe("hostMentionSearch", () => {
  it("detects an empty @ token at the cursor", () => {
    expect(findActiveHostMentionToken("@", 1)).toEqual({ start: 0, end: 1, query: "", raw: "@" });
  });

  it("detects a partial @ token in Chinese text", () => {
    expect(findActiveHostMentionToken("请检查 @pg", "请检查 @pg".length)).toEqual({
      start: 4,
      end: 7,
      query: "pg",
      raw: "@pg",
    });
  });

  it("does not trigger for email addresses", () => {
    expect(findActiveHostMentionToken("联系 sre@example.com", "联系 sre@example.com".length)).toBeNull();
  });

  it("keeps ip address dots inside the active token", () => {
    expect(findActiveHostMentionToken("@120.77", "@120.77".length)).toEqual({
      start: 0,
      end: 7,
      query: "120.77",
      raw: "@120.77",
    });
  });

  it("closes active token after spaces, punctuation, or newlines", () => {
    expect(findActiveHostMentionToken("@pg ", 4)).toBeNull();
    expect(findActiveHostMentionToken("@pg，", 4)).toBeNull();
    expect(findActiveHostMentionToken("@pg\n", 4)).toBeNull();
  });

  it("searches by host name and ip address only", () => {
    const result = searchHostMentionSuggestions(
      [
        { id: "host-a", name: "pg-primary", ip: "120.77.239.90", hostname: "ignored-hostname" } as any,
        { id: "host-b", name: "redis-node", ip: "10.0.0.8", labels: { role: "pg" } } as any,
        { id: "host-c", name: "web", address: "10.0.1.23" } as any,
      ],
      "pg",
    );

    expect(result.map((item) => item.label)).toEqual(["pg-primary"]);
    expect(result[0]).toMatchObject({
      key: "host-a",
      mention: "@120.77.239.90",
      address: "120.77.239.90",
    });
  });

  it("offers @local first for an empty mention query", () => {
    const result = searchHostMentionSuggestions(
      [{ id: "host-a", name: "pg-primary", ip: "120.77.239.90" } as any],
      "",
    );

    expect(result[0]).toMatchObject({
      key: "local",
      mention: "@local",
      label: "local",
      hostId: "server-local",
      address: "server-local",
    });
  });

  it("matches @local when the user types a local prefix", () => {
    expect(searchHostMentionSuggestions([], "loc")[0]).toMatchObject({
      mention: "@local",
      hostId: "server-local",
    });
  });

  it("does not search hostname, id, sshUser, labels, or status", () => {
    const result = searchHostMentionSuggestions(
      [
        { id: "pg-id", name: "api", ip: "10.0.0.10", hostname: "pg-host", sshUser: "pg-user", labels: { role: "pg" }, status: "pg" } as any,
      ],
      "pg",
    );

    expect(result).toEqual([]);
  });

  it("limits suggestions to 10 items", () => {
    const hosts = Array.from({ length: 12 }, (_, index) => ({
      id: `host-${index}`,
      name: `pg-${index}`,
      ip: `10.0.0.${index}`,
    }));

    expect(searchHostMentionSuggestions(hosts, "pg")).toHaveLength(10);
  });

  it("prefers prefix matches over substring matches", () => {
    const result = searchHostMentionSuggestions(
      [
        { id: "host-substring", name: "my-pg-node", ip: "10.0.0.5" },
        { id: "host-prefix", name: "pg-primary", ip: "10.0.0.6" },
      ],
      "pg",
    );

    expect(result.map((item) => item.key)).toEqual(["host-prefix", "host-substring"]);
  });

  it("replaces active token with the selected mention and returns cursor", () => {
    const suggestion: HostMentionSuggestion = {
      key: "host-a",
      mention: "@120.77.239.90",
      label: "pg-primary",
      description: "120.77.239.90 · online",
      address: "120.77.239.90",
      score: 100,
    };
    const replacementPrefix = "请检查 @120.77.239.90 ";

    expect(replaceActiveHostMention("请检查 @pg 的复制", { start: 4, end: 7, query: "pg", raw: "@pg" }, suggestion)).toEqual({
      text: `${replacementPrefix} 的复制`,
      cursor: replacementPrefix.length,
    });
  });
});
