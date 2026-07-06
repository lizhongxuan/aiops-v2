import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { AiopsSpecialInputContext } from "@/transport/aiopsTransportTypes";

import { SpecialInputContextBar } from "./SpecialInputContextBar";

describe("SpecialInputContextBar", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => {
      root.unmount();
    });
    container.remove();
  });

  it("renders active grant, candidate and pending confirmation from typed transport state", async () => {
    const onClear = vi.fn();
    const onConfirm = vi.fn();
    await act(async () => {
      root.render(<SpecialInputContextBar context={sampleContext()} onClear={onClear} onConfirm={onConfirm} />);
    });

    expect(container.querySelector('[data-testid="special-input-context-bar"]')).not.toBeNull();
    expect(container.textContent).toContain("host-a");
    expect(container.textContent).toContain("低信任候选 1");
    expect(container.textContent).toContain("需要确认 1");
    expect(container.textContent).toContain("确认");
    expect(container.textContent).toContain("pg_primary");
    expect(container.textContent).toContain("prod / pg-orders");
    expect(container.textContent).toContain("pg_standby -> host-b");
    expect(container.textContent).toContain("角色冲突 1");
    const confirmButton = Array.from(container.querySelectorAll("button")).find((button) => button.textContent === "确认");
    expect(confirmButton).not.toBeUndefined();
    await act(async () => {
      confirmButton?.click();
    });
    expect(onConfirm).toHaveBeenCalledTimes(1);
    const clearButton = container.querySelector('button[aria-label="清除特殊输入上下文"]') as HTMLButtonElement | null;
    expect(clearButton).not.toBeNull();
    await act(async () => {
      clearButton?.click();
    });
    expect(onClear).toHaveBeenCalledTimes(1);
  });

  it("renders nothing for empty context", async () => {
    await act(async () => {
      root.render(<SpecialInputContextBar context={undefined} />);
    });

    expect(container.querySelector('[data-testid="special-input-context-bar"]')).toBeNull();
  });
});

function sampleContext(): AiopsSpecialInputContext {
  return {
    schemaVersion: "aiops.special_input_memory.v1",
    turnId: "turn-1",
    activeGrant: {
      id: "grant-host-a",
      resourceKind: "host",
      resourceId: "host-a",
      canonicalKey: "host:host-a",
      display: "host-a",
      status: "active",
      allowedActions: ["inspect", "read", "exec_low_risk"],
    },
    candidateFacts: [
      {
        id: "fact-raw",
        kind: "host",
        resourceKind: "host",
        resourceId: "1.1.1.1",
        canonicalKey: "host:1.1.1.1",
        display: "1.1.1.1",
        trustLevel: "raw_typed",
        status: "active",
      },
    ],
    roleBindings: [
      {
        id: "role-primary",
        roleKey: "pg_primary",
        runtimeName: "pg主节点",
        resourceKind: "host",
        resourceId: "host-a",
        display: "host-a",
        environmentKey: "prod",
        clusterKey: "pg-orders",
        bindingHash: "role-hash",
        status: "active",
      },
      {
        id: "role-standby",
        roleKey: "pg_standby",
        runtimeName: "pg从节点",
        resourceKind: "host",
        resourceId: "host-b",
        display: "host-b",
        environmentKey: "prod",
        clusterKey: "pg-orders",
        bindingHash: "role-hash-standby",
        status: "active",
      },
    ],
    conflicts: [
      {
        id: "conflict-primary",
        kind: "role_binding",
        roleKey: "pg_primary",
        environmentKey: "prod",
        clusterKey: "pg-orders",
        resourceIds: ["host-a", "host-d"],
      },
    ],
    pendingConfirmations: [
      { id: "pending-target", kind: "target", reason: "active_grant_revalidate_failed" },
    ],
  };
}
