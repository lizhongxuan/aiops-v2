import { describe, it, expect } from "vitest";
import fc from "fast-check";
import {
  resolveMcpBundlePresetKey,
  MCP_BUNDLE_PRESET_KEYS,
} from "../src/lib/mcpBundleResolver";

/**
 * Property-Based Tests — mcpBundleResolver Coroot 来源识别
 *
 * **Validates: Requirements 1.1, 3.3**
 */

const COROOT_PRESET_KEYS = [
  MCP_BUNDLE_PRESET_KEYS.COROOT_SERVICE_MONITOR,
  MCP_BUNDLE_PRESET_KEYS.COROOT_INCIDENT_RCA,
];

// --- Generators ---

/** Generates a random non-empty string that includes "coroot" somewhere. */
const corootSubstring = () =>
  fc.tuple(fc.string({ minLength: 0, maxLength: 8 }), fc.string({ minLength: 0, maxLength: 8 })).map(
    ([prefix, suffix]) => `${prefix}coroot${suffix}`,
  );

/** Generates a toolName starting with "coroot." followed by arbitrary suffix. */
const corootToolName = () =>
  fc.array(fc.constantFrom(..."abcdefghijklmnopqrstuvwxyz_"), { minLength: 1, maxLength: 12 }).map(
    (chars) => `coroot.${chars.join("")}`,
  );

/** Generates arbitrary extra payload fields that should not interfere with Coroot detection. */
const extraPayloadFields = () =>
  fc.record({
    summary: fc.option(fc.string({ maxLength: 30 }), { nil: undefined }),
    confidence: fc.option(fc.double({ min: 0, max: 1, noNaN: true }), { nil: undefined }),
    subject: fc.option(
      fc.record({
        type: fc.option(fc.constantFrom("service", "middleware", "host", "pod"), { nil: undefined }),
        name: fc.option(fc.string({ maxLength: 10 }), { nil: undefined }),
      }),
      { nil: undefined },
    ),
  });

describe("mcpBundleResolver — Property 4: 来源识别完备性", () => {
  /**
   * **Property 4: 来源识别完备性**
   *
   * For any payload where mcpServer contains "coroot",
   * resolveMcpBundlePresetKey MUST return a COROOT_* preset key.
   *
   * **Validates: Requirements 1.1, 3.3**
   */
  it("always returns COROOT_* when mcpServer contains 'coroot'", () => {
    fc.assert(
      fc.property(corootSubstring(), extraPayloadFields(), (mcpServer, extras) => {
        const key = resolveMcpBundlePresetKey({ ...extras, mcpServer });
        expect(COROOT_PRESET_KEYS).toContain(key);
      }),
      { numRuns: 200 },
    );
  });

  /**
   * **Property 4: 来源识别完备性**
   *
   * For any payload where source contains "coroot",
   * resolveMcpBundlePresetKey MUST return a COROOT_* preset key.
   *
   * **Validates: Requirements 1.1, 3.3**
   */
  it("always returns COROOT_* when source contains 'coroot'", () => {
    fc.assert(
      fc.property(corootSubstring(), extraPayloadFields(), (source, extras) => {
        const key = resolveMcpBundlePresetKey({ ...extras, source });
        expect(COROOT_PRESET_KEYS).toContain(key);
      }),
      { numRuns: 200 },
    );
  });

  /**
   * **Property 4: 来源识别完备性**
   *
   * For any payload where toolName starts with "coroot.",
   * resolveMcpBundlePresetKey MUST return a COROOT_* preset key.
   *
   * **Validates: Requirements 1.1, 3.3**
   */
  it("always returns COROOT_* when toolName starts with 'coroot.'", () => {
    fc.assert(
      fc.property(corootToolName(), extraPayloadFields(), (toolName, extras) => {
        const key = resolveMcpBundlePresetKey({ ...extras, toolName });
        expect(COROOT_PRESET_KEYS).toContain(key);
      }),
      { numRuns: 200 },
    );
  });

  /**
   * **Property 4: 来源识别完备性**
   *
   * For any payload where at least one of mcpServer/source/toolName carries
   * a coroot identifier, the result is always a COROOT_* preset key —
   * regardless of which combination of identifiers is present.
   *
   * **Validates: Requirements 1.1, 3.3**
   */
  it("always returns COROOT_* for any combination of coroot identifiers", () => {
    const corootIdentifierCombo = () =>
      fc.record({
        mcpServer: fc.option(corootSubstring(), { nil: undefined }),
        source: fc.option(corootSubstring(), { nil: undefined }),
        toolName: fc.option(corootToolName(), { nil: undefined }),
      }).filter((combo) => combo.mcpServer !== undefined || combo.source !== undefined || combo.toolName !== undefined);

    fc.assert(
      fc.property(corootIdentifierCombo(), extraPayloadFields(), (identifiers, extras) => {
        const payload = { ...extras, ...identifiers };
        const key = resolveMcpBundlePresetKey(payload);
        expect(COROOT_PRESET_KEYS).toContain(key);
      }),
      { numRuns: 300 },
    );
  });

  /**
   * **Property 4: 来源识别完备性**
   *
   * Coroot identifiers passed via defaults (second argument) should also
   * trigger COROOT_* preset key resolution.
   *
   * **Validates: Requirements 1.1, 3.3**
   */
  it("always returns COROOT_* when coroot identifiers are in defaults", () => {
    const defaultsWithCoroot = () =>
      fc.oneof(
        corootSubstring().map((v) => ({ mcpServer: v })),
        corootSubstring().map((v) => ({ source: v })),
        corootToolName().map((v) => ({ toolName: v })),
      );

    fc.assert(
      fc.property(defaultsWithCoroot(), extraPayloadFields(), (defaults, extras) => {
        const key = resolveMcpBundlePresetKey(extras, defaults);
        expect(COROOT_PRESET_KEYS).toContain(key);
      }),
      { numRuns: 200 },
    );
  });
});
