import { describe, expect, it } from "vitest";

import {
  loadWorkflowCanvasFixture,
  loadWorkflowCanvasManifest,
  selectWorkflowCanvasCases,
} from "./workflowCanvasFixtures.js";

describe("workflowCanvasFixtures", () => {
  it("loads shared workflow, identity, grant, revision, and failure fixtures", () => {
    const manifest = loadWorkflowCanvasManifest();
    for (const entry of manifest.fixtures) {
      const fixture = loadWorkflowCanvasFixture(entry.id, manifest);
      expect(fixture.fixtureId).toBe(entry.id);
      expect(fixture.schemaVersion).toBe("aiops.workflow-canvas.fixture/v1");
    }
  });

  it("selects only active cases at or below the effective phase", () => {
    const manifest = loadWorkflowCanvasManifest();
    const selected = selectWorkflowCanvasCases(manifest);
    expect(selected.length).toBeGreaterThan(0);
    expect(selected.every((entry) => entry.activationState === "active")).toBe(true);
    expect(selected.every((entry) => entry.activationPhase <= manifest.currentActivationPhase)).toBe(true);
    expect(selected.every((entry) => ["P0", "P1"].includes(entry.priority))).toBe(true);
  });

  it("does not allow a local phase preview to lower the repository phase", () => {
    const manifest = loadWorkflowCanvasManifest();
    expect(() => selectWorkflowCanvasCases(manifest, { phase: manifest.currentActivationPhase - 1 })).toThrow(
      /cannot lower/,
    );
  });
});
