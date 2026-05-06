import { describe, expect, it } from "vitest";
import {
  createWorkflowManagerState,
  filterWorkflowManagerItems,
  getQuickWorkflows,
  recordRecentWorkflow,
  toggleFavoriteWorkflow,
} from "./workflowManagerState";

const workflows = [
  { name: "pg-restore", title: "PG Restore", status: "published" },
  { name: "cache-warmup", title: "Cache Warmup", status: "draft" },
  { name: "erp-health-check", title: "ERP Health Check", status: "validated" },
  { name: "legacy-archived", title: "Legacy Archived", status: "archived", archived: true },
];

describe("workflowManagerState", () => {
  it("keeps quick workflows limited to favorites and recent items without mutating workflow records", () => {
    const state = createWorkflowManagerState({
      favorites: ["erp-health-check"],
      recent: ["pg-restore", "missing-workflow"],
    });

    const quick = getQuickWorkflows(workflows, state);

    expect(quick.map((item) => item.name)).toEqual(["erp-health-check", "pg-restore"]);
    expect(quick[0]).not.toHaveProperty("favorite");
    expect(workflows[2]).not.toHaveProperty("favorite");
  });

  it("records recent order and favorites as local UI state only", () => {
    const state = createWorkflowManagerState({ recent: ["pg-restore"], favorites: ["pg-restore"] });

    const recentState = recordRecentWorkflow(state, "cache-warmup", 3);
    const favoriteState = toggleFavoriteWorkflow(recentState, "pg-restore");

    expect(recentState.recent).toEqual(["cache-warmup", "pg-restore"]);
    expect(favoriteState.favorites).toEqual([]);
    expect(favoriteState.recent).toEqual(["cache-warmup", "pg-restore"]);
  });

  it("filters the manager list by search, status, and archived visibility", () => {
    expect(filterWorkflowManagerItems(workflows, { query: "erp", status: "validated" }).map((item) => item.name)).toEqual([
      "erp-health-check",
    ]);
    expect(filterWorkflowManagerItems(workflows, { includeArchived: false }).map((item) => item.name)).not.toContain(
      "legacy-archived",
    );
    expect(filterWorkflowManagerItems(workflows, { includeArchived: true }).map((item) => item.name)).toContain(
      "legacy-archived",
    );
  });
});
