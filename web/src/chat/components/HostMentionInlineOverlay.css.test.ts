import { readFileSync } from "node:fs";
import path from "node:path";

import { describe, expect, it } from "vitest";

describe("HostMentionInlineOverlay CSS", () => {
  it("keeps the visual mention text visible instead of inheriting the transparent layout anchor", () => {
    const css = readFileSync(path.resolve(process.cwd(), "src/index.css"), "utf8");
    const visualRule = css.match(/\.aiops-inline-mention-visual\s*\{(?<body>[^}]*)\}/)?.groups?.body || "";

    expect(visualRule).not.toContain("color: inherit");
  });

  it("allows the visual chip to be wider than the raw @token layout anchor", () => {
    const css = readFileSync(path.resolve(process.cwd(), "src/index.css"), "utf8");
    const visualRule = css.match(/\.aiops-inline-mention-visual\s*\{(?<body>[^}]*)\}/)?.groups?.body || "";

    expect(visualRule).toContain("width: max-content");
    expect(visualRule).not.toContain("inset: 0");
    expect(visualRule).not.toContain("overflow: hidden");
  });
});
