import { describe, expect, it } from "vitest";

import { loadWorkflowCanvasManifest, validateWorkflowCanvasManifest } from "./helpers/workflowCanvasFixtures.js";

describe("workflow canvas MCP test manifest", () => {
  it("enumerates the complete protected design case set", () => {
    const manifest = loadWorkflowCanvasManifest();
    expect(() => validateWorkflowCanvasManifest(manifest)).not.toThrow();
    expect(manifest.cases).toHaveLength(169);
    expect(new Set(manifest.cases.map((entry: { id: string }) => entry.id)).size).toBe(169);
  });

  it("keeps future cases explicitly planned", () => {
    const manifest = loadWorkflowCanvasManifest();
    const future = manifest.cases.filter(
      (entry: { activationPhase: number }) => entry.activationPhase > manifest.currentActivationPhase,
    );
    expect(future.length).toBeGreaterThan(0);
    expect(future.every((entry: { activationState: string }) => entry.activationState === "planned")).toBe(true);
  });

  it("declares selector, owner, evidence, and fixture references for every case", () => {
    const manifest = loadWorkflowCanvasManifest();
    const fixtureIds = new Set(manifest.fixtures.map((entry: { id: string }) => entry.id));
    for (const entry of manifest.cases) {
      expect(entry.selector).toBeTruthy();
      expect(entry.owner).toBeTruthy();
      expect(entry.evidence.length).toBeGreaterThan(0);
      expect(entry.fixtures.every((id: string) => fixtureIds.has(id))).toBe(true);
    }
  });
});
