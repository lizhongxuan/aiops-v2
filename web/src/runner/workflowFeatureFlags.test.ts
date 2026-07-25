import { describe, expect, it } from "vitest";

import {
  isWorkflowCapabilityEnabled,
  parseWorkflowCapabilitySnapshot,
  type WorkflowCapability,
} from "./workflowFeatureFlags";

const capabilities = (overrides: Partial<Record<WorkflowCapability, boolean>> = {}) => ({
  workflow_agent_ui: true,
  workflow_mcp_read: true,
  workflow_mcp_preview: true,
  workflow_mcp_mutation: false,
  workflow_delete: false,
  workflow_publish: false,
  workflow_execute: false,
  workflow_domain_events: true,
  workflow_plan_ir: false,
  workflow_elk_layout: false,
  workflow_edit_grants: false,
  workflow_external_mcp: false,
  workflow_mcp_apps: false,
  ...overrides,
});

const snapshot = (overrides: Record<string, unknown> = {}) => ({
  schemaVersion: "aiops.workflow.rollout.v1",
  stage: "read_only",
  saltVersion: "workflow-rollout-2026-07-v1",
  rolloutPercentage: 25,
  bucketBasisPoints: 1724,
  decision: "percentage",
  capabilities: capabilities(),
  ...overrides,
});

describe("workflowFeatureFlags", () => {
  it("strictly parses a server capability snapshot and reads only explicit booleans", () => {
    const source = snapshot();
    const parsed = parseWorkflowCapabilitySnapshot(source);

    expect(parsed).toEqual(source);
    expect(isWorkflowCapabilityEnabled(parsed, "workflow_mcp_read")).toBe(true);
    expect(isWorkflowCapabilityEnabled(parsed, "workflow_mcp_mutation")).toBe(false);

    source.capabilities.workflow_mcp_read = false;
    expect(parsed?.capabilities.workflow_mcp_read).toBe(true);
  });

  it("fails closed on unknown, missing, malformed, or future fields", () => {
    expect(parseWorkflowCapabilitySnapshot({ ...snapshot(), unknown: true })).toBeUndefined();
    expect(parseWorkflowCapabilitySnapshot(snapshot({ capabilities: { ...capabilities(), unknown: true } }))).toBeUndefined();
    const missing = capabilities() as Record<string, unknown>;
    delete missing.workflow_publish;
    expect(parseWorkflowCapabilitySnapshot(snapshot({ capabilities: missing }))).toBeUndefined();
    expect(parseWorkflowCapabilitySnapshot(snapshot({ schemaVersion: "aiops.workflow.rollout.v2" }))).toBeUndefined();
    expect(parseWorkflowCapabilitySnapshot(snapshot({ stage: "future" }))).toBeUndefined();
    expect(parseWorkflowCapabilitySnapshot(snapshot({ rolloutPercentage: 101 }))).toBeUndefined();
    expect(parseWorkflowCapabilitySnapshot(snapshot({ bucketBasisPoints: 10_000 }))).toBeUndefined();
    expect(parseWorkflowCapabilitySnapshot(snapshot({ capabilities: capabilities({ workflow_mcp_read: 1 as unknown as boolean }) }))).toBeUndefined();
  });

  it("rejects contradictory disabled/deny snapshots instead of authorizing from stage or local state", () => {
    expect(parseWorkflowCapabilitySnapshot(snapshot({
      stage: "disabled",
      decision: "deny_list",
      capabilities: capabilities({ workflow_agent_ui: true }),
    }))).toBeUndefined();
    expect(parseWorkflowCapabilitySnapshot(snapshot({ stage: "general", decision: "deny_list" }))).toBeUndefined();
    expect(isWorkflowCapabilityEnabled({ capabilities: { workflow_mcp_mutation: true } }, "workflow_mcp_mutation")).toBe(false);
    expect(isWorkflowCapabilityEnabled(snapshot({ schemaVersion: "forged", capabilities: capabilities({ workflow_mcp_mutation: true }) }), "workflow_mcp_mutation")).toBe(false);
  });

  it.each([
    "disabled", "read_only", "internal_apply", "canary", "general",
  ])("accepts the typed %s stage when the server snapshot is internally consistent", (stage) => {
    const disabled = stage === "disabled";
    const value = snapshot({
      stage,
      decision: disabled ? "percentage_excluded" : "percentage",
      capabilities: disabled ? capabilities(Object.fromEntries(Object.keys(capabilities()).map((key) => [key, false]))) : capabilities(),
    });
    expect(parseWorkflowCapabilitySnapshot(value)?.stage).toBe(stage);
  });
});
