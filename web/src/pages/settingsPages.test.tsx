import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AppRouter } from "@/router";

const statePayload = {
  hosts: [{ id: "server-local", name: "server-local", status: "online" }],
};

const hostsPayload = {
  items: [
    {
      id: "host-prod-07",
      name: "web-07",
      address: "10.10.4.27",
      sshUser: "root",
      sshPort: 22,
      transport: "ssh_bootstrap",
      status: "online",
      terminalCapable: true,
      agentVersion: "1.8.4",
      lastHeartbeat: new Date().toISOString(),
      labels: { env: "prod", role: "web", cluster: "ops-k8s" },
    },
  ],
};

const llmPayload = {
  provider: "openai",
  model: "gpt-5.4",
  apiKeySet: true,
  apiKeyMasked: "sk-***",
  baseURL: "https://api.openai.com/v1",
  bifrostActive: true,
};

const skillPayload = {
  items: [{ id: "ops-triage", name: "Ops Triage", description: "Triage incidents", source: "builtin", defaultEnabled: true, defaultActivationMode: "default_enabled" }],
};

const mcpPayload = {
  items: [{ id: "metrics", name: "Metrics MCP", type: "http", source: "builtin", defaultEnabled: true, permission: "readonly" }],
};

const profilesPayload = {
  items: [
    {
      id: "main-agent",
      name: "Main Agent",
      description: "Main runtime profile",
      systemPrompt: { content: "You are the main AIOps agent." },
      runtime: { model: "gpt-5.4", approvalMode: "on-request", sandboxMode: "workspace-write" },
      skills: skillPayload.items,
      mcps: mcpPayload.items,
    },
  ],
  skillCatalog: skillPayload.items,
  mcpCatalog: mcpPayload.items,
};

function jsonResponse(payload: unknown) {
  return Promise.resolve(new Response(JSON.stringify(payload), { status: 200, headers: { "Content-Type": "application/json" } }));
}

function mockFetch(input: RequestInfo | URL, init?: RequestInit) {
  const url = String(input);
  if (url.endsWith("/api/v1/state")) return jsonResponse(statePayload);
  if (url.endsWith("/api/v1/hosts")) {
    if (init?.method === "POST") return jsonResponse({ ok: true });
    return jsonResponse(hostsPayload);
  }
  if (url.endsWith("/api/v1/sessions")) return jsonResponse({ activeSessionId: "sess-1", sessions: [{ id: "sess-1", kind: "single_host", title: "Nginx chat", selectedHostId: "host-prod-07" }] });
  if (url.endsWith("/api/v1/terminal/sessions")) return jsonResponse({ items: [{ id: "term-1", status: "running" }] });
  if (url.endsWith("/api/v1/llm-config")) return jsonResponse(init?.method === "PUT" ? { ok: true, message: "saved" } : llmPayload);
  if (url.endsWith("/api/v1/agent-skills")) return jsonResponse(skillPayload);
  if (url.includes("/api/v1/agent-skills/")) return jsonResponse({ items: skillPayload.items, item: skillPayload.items[0] });
  if (url.endsWith("/api/v1/agent-mcps")) return jsonResponse(mcpPayload);
  if (url.includes("/api/v1/agent-mcps/")) return jsonResponse({ items: mcpPayload.items, item: mcpPayload.items[0] });
  if (url.endsWith("/api/v1/agent-profiles")) return jsonResponse(profilesPayload);
  if (url.endsWith("/api/v1/agent-profile")) return jsonResponse(init?.method === "PUT" ? profilesPayload.items[0] : profilesPayload.items[0]);
  if (url.endsWith("/api/v1/agent-profile/reset")) return jsonResponse(profilesPayload.items[0]);
  if (url.endsWith("/api/v1/agent-profiles/export")) return jsonResponse(profilesPayload);
  if (url.endsWith("/api/v1/agent-profiles/import")) return jsonResponse(profilesPayload);
  return jsonResponse({});
}

async function flush() {
  await act(async () => {
    for (let index = 0; index < 5; index += 1) {
      await Promise.resolve();
    }
  });
}

describe("React settings pages", () => {
  let container: HTMLDivElement;
  let root: Root;

  async function renderPath(path: string) {
    await act(async () => {
      root.render(
        <MemoryRouter initialEntries={[path]}>
          <AppRouter />
        </MemoryRouter>,
      );
    });
    await flush();
  }

  async function remountPath(path: string) {
    act(() => {
      root.unmount();
    });
    container.innerHTML = "";
    root = createRoot(container);
    await renderPath(path);
  }

  beforeEach(() => {
    (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
    globalThis.ResizeObserver = class ResizeObserver {
      observe() {}
      unobserve() {}
      disconnect() {}
    };
    vi.spyOn(globalThis, "fetch").mockImplementation(mockFetch as typeof fetch);
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => {
      root.unmount();
    });
    container.remove();
  });

  it.each([
    ["/settings", "设置"],
    ["/settings/llm", "LLM 配置"],
    ["/settings/hosts", "env=prod"],
    ["/settings/experience-packs", "经验包库"],
    ["/settings/agent", "Agent Profile"],
    ["/settings/skills", "Ops Triage"],
    ["/settings/mcp", "Metrics MCP"],
  ])("renders migrated settings route %s", async (path, expectedText) => {
    await renderPath(path);

    expect(container.textContent).toContain(expectedText);
    expect(container.textContent).not.toContain("Migration Placeholder");
  });

  it("supports refresh, save, delete, and import settings operations", async () => {
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);
    await renderPath("/settings/llm");

    const saveLlm = Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("保存并重启 Runtime"));
    expect(saveLlm).toBeTruthy();
    await act(async () => {
      saveLlm?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    await flush();
    expect(globalThis.fetch).toHaveBeenCalledWith(
      "/api/v1/llm-config",
      expect.objectContaining({ method: "PUT" }),
    );

    await remountPath("/settings/skills");
    const deleteSkill = Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("删除"));
    expect(deleteSkill).toBeTruthy();
    await act(async () => {
      deleteSkill?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    await flush();
    expect(confirmSpy).toHaveBeenCalled();
    expect(globalThis.fetch).toHaveBeenCalledWith(
      "/api/v1/agent-skills/ops-triage",
      expect.objectContaining({ method: "DELETE" }),
    );

    await remountPath("/settings/agent");
    const importInput = container.querySelector('input[type="file"]') as HTMLInputElement;
    const importFile = new File([JSON.stringify(profilesPayload)], "profiles.json", { type: "application/json" });
    Object.defineProperty(importInput, "files", { configurable: true, value: [importFile] });
    await act(async () => {
      importInput.dispatchEvent(new Event("change", { bubbles: true }));
    });
    await flush();
    expect(globalThis.fetch).toHaveBeenCalledWith(
      "/api/v1/agent-profiles/import",
      expect.objectContaining({ method: "POST" }),
    );
  });
});
