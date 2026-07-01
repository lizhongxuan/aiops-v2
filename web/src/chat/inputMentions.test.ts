import { describe, expect, it } from "vitest";

import {
  INPUT_MENTIONS_METADATA_KEY,
  buildCapabilityMentionBinding,
  buildHostMentionBinding,
  buildInputMentionMetadata,
  deriveCapabilityMentionMetadata,
  deriveHostMentionMetadata,
  reconcileMentionBindings,
} from "./inputMentions";

describe("inputMentions", () => {
  it("serializes selected host and capability bindings into aiops.input.mentions.v1", () => {
    const bindings = [
      buildHostMentionBinding({
        tokenId: "mention-0-local",
        rawText: "@local",
        range: { start: 0, end: 6 },
        hostId: "server-local",
        address: "server-local",
        displayName: "local",
      }),
      buildCapabilityMentionBinding({
        tokenId: "mention-8-coroot",
        rawText: "@Coroot",
        range: { start: 8, end: 15 },
        capability: "coroot",
      }),
    ];

    const metadata = buildInputMentionMetadata(bindings);
    expect(Object.keys(metadata)).toEqual([INPUT_MENTIONS_METADATA_KEY]);
    expect(JSON.parse(metadata[INPUT_MENTIONS_METADATA_KEY])).toEqual({
      version: 1,
      mentions: [
        expect.objectContaining({
          kind: "host",
          path: "host://server-local",
          source: "selection",
          payload: expect.objectContaining({
            hostId: "server-local",
            address: "server-local",
            displayName: "local",
          }),
        }),
        expect.objectContaining({
          kind: "capability",
          path: "capability://coroot",
          source: "selection",
        }),
      ],
    });
  });

  it("drops stale selected bindings when the visible token is deleted or changed", () => {
    const binding = buildHostMentionBinding({
      tokenId: "mention-0-local",
      rawText: "@local",
      range: { start: 0, end: 6 },
      hostId: "server-local",
      address: "server-local",
      displayName: "local",
    });

    expect(reconcileMentionBindings("@local 查看 CPU", [binding])).toHaveLength(1);
    expect(reconcileMentionBindings("查看 CPU", [binding])).toEqual([]);
    expect(reconcileMentionBindings("@host-a 查看 CPU", [binding])).toEqual([]);
  });

  it("derives legacy host metadata only from reconciled host bindings", () => {
    const [binding] = reconcileMentionBindings("@local 查看 CPU", [
      buildHostMentionBinding({
        tokenId: "mention-0-local",
        rawText: "@local",
        range: { start: 0, end: 6 },
        hostId: "server-local",
        address: "server-local",
        displayName: "local",
      }),
    ]);

    const metadata = deriveHostMentionMetadata([binding]);
    expect(JSON.parse(metadata["aiops.hostops.mentions"])).toEqual([
      expect.objectContaining({
        raw: "@local",
        hostId: "server-local",
        value: "server-local",
        source: "inventory",
        resolved: true,
      }),
    ]);
    expect(metadata["aiops.hostops.clientDetectedMultiHost"]).toBe("false");
  });

  it("derives capability metadata from structured capability bindings", () => {
    const metadata = deriveCapabilityMentionMetadata([
      buildCapabilityMentionBinding({
        tokenId: "mention-0-coroot",
        rawText: "@Coroot",
        range: { start: 0, end: 7 },
        capability: "coroot",
      }),
      buildCapabilityMentionBinding({
        tokenId: "mention-8-manuals",
        rawText: "@ops_manuals",
        range: { start: 8, end: 20 },
        capability: "ops_manuals",
      }),
    ]);

    expect(metadata).toEqual({
      "aiops.coroot.explicitRCA": "true",
      "aiops.coroot.rcaDisplayAllowed": "true",
      "aiops.opsManuals.explicitMention": "true",
      enableTool: "search_ops_manuals",
      enableToolPack: "ops_manual_flow",
    });
  });

  it("does not create strong metadata for manually typed text without bindings", () => {
    expect(buildInputMentionMetadata([])).toEqual({});
    expect(deriveHostMentionMetadata([])).toEqual({});
    expect(deriveCapabilityMentionMetadata([])).toEqual({});
  });

  it("keeps typed fallback bindings out of strong derived metadata", () => {
    const bindings = [
      buildHostMentionBinding({
        tokenId: "mention-0-local",
        rawText: "@local",
        range: { start: 0, end: 6 },
        hostId: "server-local",
        address: "server-local",
        displayName: "local",
        source: "typed_fallback",
      }),
      buildCapabilityMentionBinding({
        tokenId: "mention-8-coroot",
        rawText: "@Coroot",
        range: { start: 8, end: 15 },
        capability: "coroot",
        source: "typed_fallback",
      }),
    ];

    expect(JSON.parse(buildInputMentionMetadata(bindings)[INPUT_MENTIONS_METADATA_KEY])).toEqual({
      version: 1,
      mentions: [
        expect.objectContaining({ kind: "host", source: "typed_fallback" }),
        expect.objectContaining({ kind: "capability", source: "typed_fallback" }),
      ],
    });
    expect(deriveHostMentionMetadata(bindings)).toEqual({});
    expect(deriveCapabilityMentionMetadata(bindings)).toEqual({});
  });
});
