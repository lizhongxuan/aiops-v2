import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AppShellChromeProvider } from "@/app/AppShellChromeContext";
import { CapabilityManagementPage } from "./CapabilityManagementPage";

const payload = {
  items: [
    {
      id: "skill.ops-triage",
      name: "Ops Triage",
      description: "Triage production incidents",
      source: "skill",
      connection: { name: "builtin", status: "connected" },
      permissions: ["read_metrics", "read_logs"],
      risks: ["requires approval before write"],
      runtime: { host: "web-07", mode: "agent" },
      audit: { lastUsedAt: "2026-06-16T08:00:00+08:00", updatedBy: "ops" },
    },
    {
      id: "connector.management",
      name: "Connector 管理",
      source: "connector",
    },
  ],
};

function jsonResponse(data: unknown) {
  return Promise.resolve(new Response(JSON.stringify(data), { status: 200, headers: { "Content-Type": "application/json" } }));
}

async function flush() {
  await act(async () => {
    for (let index = 0; index < 8; index += 1) {
      await Promise.resolve();
    }
  });
}

describe("CapabilityManagementPage", () => {
  let container: HTMLDivElement;
  let root: Root;
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
    fetchMock = vi.fn(() => jsonResponse(payload));
    vi.stubGlobal("fetch", fetchMock);
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    vi.unstubAllGlobals();
  });

  it("renders the unified capability list without connector management or removed summary stats", async () => {
    await act(async () => {
      root.render(
        <AppShellChromeProvider>
          <CapabilityManagementPage />
        </AppShellChromeProvider>,
      );
    });
    await flush();

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/capabilities",
      expect.objectContaining({ method: "GET", credentials: "include" }),
    );
    expect(container.textContent).toContain("能力管理");
    expect(container.textContent).toContain("Ops Triage");
    expect(container.textContent).not.toContain("Connector 管理");
    expect(container.textContent).not.toContain("总数");
    expect(container.textContent).not.toContain("筛选结果");
    expect(container.textContent).not.toContain("显式确认");
  });

  it("opens capability detail in a dialog with required sections", async () => {
    await act(async () => {
      root.render(
        <AppShellChromeProvider>
          <CapabilityManagementPage />
        </AppShellChromeProvider>,
      );
    });
    await flush();

    const openButton = Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("Ops Triage")) as HTMLButtonElement;
    await act(async () => openButton.click());

    expect(document.body.textContent).toContain("Ops Triage");
    expect(document.body.textContent).toContain("来源");
    expect(document.body.textContent).toContain("连接");
    expect(document.body.textContent).toContain("权限与风险");
    expect(document.body.textContent).toContain("运行时");
    expect(document.body.textContent).toContain("审计");
  });
});
