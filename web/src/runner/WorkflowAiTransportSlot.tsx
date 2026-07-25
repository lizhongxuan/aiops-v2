import { useEffect, useMemo, useState } from "react";

import type { RunnerGraph } from "@/components/runner/canvasGraphAdapter";
import {
  WorkflowAiTransportPanel,
  type WorkflowScopeEnvironmentOption,
  type WorkflowScopeWorkflowOption,
} from "@/runner/WorkflowAiTransportPanel";
import { workflowTransportIdentity } from "@/runner/transport/WorkflowTransportBoundary";
import type { WorkflowEnvironmentFact } from "@/runner/workspace/workflowWorkspaceTypes";

export type WorkflowAiTransportSlotProps = {
  graph: RunnerGraph;
  environment?: WorkflowEnvironmentFact;
  open: boolean;
  onClose?: () => void;
  workflowId: string;
  workflowOptions?: readonly WorkflowScopeWorkflowOption[];
  environmentOptions?: readonly WorkflowScopeEnvironmentOption[];
  onWorkflowScopeChange?: (option: WorkflowScopeWorkflowOption) => void;
  onEnvironmentScopeChange?: (option: WorkflowScopeEnvironmentOption) => void;
};

export function WorkflowAiTransportSlot({
  graph,
  environment: authoritativeEnvironment,
  open,
  onClose,
  workflowId,
  workflowOptions,
  environmentOptions,
  onWorkflowScopeChange,
  onEnvironmentScopeChange,
}: WorkflowAiTransportSlotProps) {
  const environmentInput = useMemo(() => workflowEnvironmentDigestInput(graph), [graph]);
  const [environment, setEnvironment] = useState<WorkflowEnvironmentFact>(() => ({
    name: environmentInput.name,
    bindingVersion: "",
  }));

  useEffect(() => {
    let current = true;
    setEnvironment({ name: environmentInput.name, bindingVersion: "" });
    void sha256(environmentInput.payload).then((bindingVersion) => {
      if (current) setEnvironment({ name: environmentInput.name, bindingVersion });
    }).catch(() => {
      // A missing digest means the composer remains disabled. Never invent an
      // environment binding that could make a stale mutation look valid.
    });
    return () => { current = false; };
  }, [environmentInput.name, environmentInput.payload]);

  const effectiveEnvironment = selectWorkflowEnvironmentBinding(authoritativeEnvironment, environment);

  if (!open) return null;
  const identity = workflowTransportIdentity(workflowId);
  return (
    <WorkflowAiTransportPanel
      workflowId={workflowId}
      environment={effectiveEnvironment}
      workflowOptions={workflowOptions}
      environmentOptions={environmentOptions}
      transportThreadId={identity.threadId}
      onWorkflowScopeChange={onWorkflowScopeChange}
      onEnvironmentScopeChange={onEnvironmentScopeChange}
      onClose={onClose}
      embedded
      className="absolute inset-y-0 right-0 z-[55] w-full max-w-[420px] shadow-[-18px_0_42px_rgba(15,23,42,0.14)]"
    />
  );
}

// The committed workspace projection is the authority for validation basis.
// A browser-computed digest is only a compatibility fallback for callers that
// have no projection at all; an explicitly supplied empty binding must keep
// the composer disabled instead of silently inventing authority.
export function selectWorkflowEnvironmentBinding(
  authoritative: WorkflowEnvironmentFact | undefined,
  fallback: WorkflowEnvironmentFact,
) {
  return authoritative === undefined ? fallback : authoritative;
}

export function workflowEnvironmentDigestInput(graph: RunnerGraph) {
  const workflow = record(graph.workflow);
  const name = String(workflow.validation_env ?? workflow.validationEnv ?? "").trim();
  const packages = workflow.env_packages ?? workflow.envPackages ?? null;
  const sourceInventory = record(workflow.inventory);
  // Match Runner's json.Marshal([]any{ValidationEnv, EnvPackages, Inventory})
  // field order. Nested maps arrive from Go with deterministic key ordering.
  const inventory = {
    groups: sourceInventory.groups ?? null,
    hosts: sourceInventory.hosts ?? null,
    vars: sourceInventory.vars ?? null,
  };
  return {
    name: name || "默认环境",
    payload: JSON.stringify([name, packages, inventory]),
  };
}

async function sha256(value: string) {
  if (!globalThis.crypto?.subtle) throw new Error("Web Crypto is unavailable");
  const bytes = new TextEncoder().encode(value);
  const digest = await globalThis.crypto.subtle.digest("SHA-256", bytes);
  return `sha256:${Array.from(new Uint8Array(digest), (item) => item.toString(16).padStart(2, "0")).join("")}`;
}

function record(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};
}
