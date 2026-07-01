import { beforeEach, describe, expect, it, vi } from "vitest";
import { createHttpClient, HttpClientError } from "../src/api/httpClient";
import { fetchState, resetThread, selectHost } from "../src/api/state";
import { activateSession, createSession, fetchSessions } from "../src/api/sessions";
import { sendMessage, stopMessage } from "../src/api/chat";
import { submitApprovalDecision, submitChoiceAnswer } from "../src/api/approvals";
import { fetchSettings, updateSettings } from "../src/api/settings";
import { createHost, deleteHost, updateHost } from "../src/api/hosts";
import { createServer, deleteServer, fetchServers, refreshServers, runServerAction, updateServer } from "../src/api/mcp";
import { login, logout } from "../src/api/auth";
import { createTerminalSession, fetchTerminalSessions } from "../src/api/terminal";
import {
  deleteMcpCatalogItem,
  deleteSkillCatalogItem,
  exportAgentProfiles,
  fetchAgentProfilePreview,
  fetchAgentProfiles,
  fetchMcpCatalog,
  fetchSkillCatalog,
  importAgentProfiles,
  resetAgentProfile,
  saveAgentProfile,
  saveMcpCatalogItem,
  saveSkillCatalogItem,
} from "../src/api/agentProfile";
import {
  createScriptConfig,
  deleteScriptConfig,
  dryRunScriptConfig,
  fetchScriptConfigs,
  updateScriptConfig,
} from "../src/api/scriptConfigs";
import {
  createLabEnvironment,
  deleteLabEnvironment,
  fetchLabEnvironments,
  resetLabEnvironment,
  startLabEnvironment,
  stopLabEnvironment,
} from "../src/api/labEnvironments";
import { fetchCapabilityBindings } from "../src/api/capabilityCenter";
import {
  disableApprovalGrant,
  enableApprovalGrant,
  fetchApprovalAudits,
  fetchApprovalGrants,
  revokeApprovalGrant,
} from "../src/api/approvalManagement";
import { fetchLlmConfig, updateLlmConfig } from "../src/api/llm";
import { fetchUiCards, previewUiCard, updateUiCard } from "../src/api/uiCards";
import { generateDraft, lintDraft, previewDraft, publishDraft } from "../src/api/generator";
import { fetchEvidenceDetail, fetchFilePreview, fetchInvocationDetail } from "../src/api/files";

function mockJsonResponse(payload, init = {}) {
  return {
    ok: init.ok ?? true,
    status: init.status ?? 200,
    headers: {
      get: () => "application/json",
    },
    json: vi.fn(async () => payload),
    text: vi.fn(async () => JSON.stringify(payload)),
  };
}

describe("api clients", () => {
  beforeEach(() => {
    global.fetch = vi.fn();
  });

  it("http client includes credentials and decodes json responses", async () => {
    global.fetch.mockResolvedValue(mockJsonResponse({ ok: true }));
    const client = createHttpClient();

    await expect(client.get("/api/v1/state")).resolves.toEqual({ ok: true });

    expect(global.fetch).toHaveBeenCalledWith(
      "/api/v1/state",
      expect.objectContaining({
        credentials: "include",
      }),
    );
  });

  it("http client normalizes non-2xx errors", async () => {
    global.fetch.mockResolvedValue(mockJsonResponse({ error: "forbidden" }, { ok: false, status: 403 }));
    const client = createHttpClient();

    await expect(client.get("/api/v1/state")).rejects.toMatchObject({
      name: "HttpClientError",
      message: "forbidden",
      status: 403,
    });
  });

  it("http client reports JSON decode failures", async () => {
    global.fetch.mockResolvedValue({
      ok: true,
      status: 200,
      headers: {
        get: () => "application/json",
      },
      json: vi.fn(async () => {
        throw new SyntaxError("Unexpected end of JSON input");
      }),
      text: vi.fn(async () => "{"),
    });
    const client = createHttpClient();

    await expect(client.get("/api/v1/state")).rejects.toMatchObject({
      name: "HttpClientError",
      code: "invalid_json",
      message: "Invalid JSON response",
    });
  });

  it("http client returns empty object for empty success bodies", async () => {
    global.fetch.mockResolvedValue({
      ok: true,
      status: 204,
      headers: {
        get: () => "",
      },
      text: vi.fn(async () => ""),
    });
    const client = createHttpClient();

    await expect(client.get("/api/v1/state")).resolves.toEqual({});
  });

  it("http client preserves non-JSON success bodies as text", async () => {
    global.fetch.mockResolvedValue({
      ok: true,
      status: 200,
      headers: {
        get: () => "text/plain; charset=utf-8",
      },
      text: vi.fn(async () => "ok"),
      json: vi.fn(async () => {
        throw new Error("should not parse json");
      }),
    });
    const client = createHttpClient();

    await expect(client.get("/healthz")).resolves.toBe("ok");
  });

  it("API modules call the expected endpoints", async () => {
    global.fetch.mockResolvedValue(mockJsonResponse({ ok: true }));

    await fetchState();
    await resetThread();
    await selectHost("web-01");
    await fetchSessions();
    await createSession("workspace");
    await activateSession("session-1");
    await sendMessage({ message: "hello", hostId: "web-01" });
    await stopMessage();
    await submitApprovalDecision("approval-1", "accept");
    await submitChoiceAnswer("choice-1", ["A"]);
    await fetchSettings();
    await updateSettings({ theme: "light" });
    await createHost({ id: "host-1" });
    await updateHost("host-1", { id: "host-1" });
    await deleteHost("host-1");
    await fetchServers();
    await createServer({ name: "coroot" });
    await updateServer("coroot", { name: "coroot" });
    await deleteServer("coroot");
    await runServerAction("coroot", "close");
    await refreshServers();
    await login({ mode: "apiKey", apiKey: "sk-test" });
    await logout();
    await fetchTerminalSessions();
    await createTerminalSession({ hostId: "host-1", cwd: "~", shell: "/bin/zsh", cols: 120, rows: 36 });
    await fetchSkillCatalog();
    await saveSkillCatalogItem({ id: "ops-triage" });
    await deleteSkillCatalogItem("ops-triage");
    await fetchMcpCatalog();
    await saveMcpCatalogItem({ id: "filesystem" });
    await deleteMcpCatalogItem("filesystem");
    await fetchAgentProfiles();
    await exportAgentProfiles();
    await importAgentProfiles({ items: [] });
    await saveAgentProfile({ id: "main-agent" });
    await resetAgentProfile("main-agent");
    await fetchAgentProfilePreview("main-agent");
    await fetchScriptConfigs();
    await createScriptConfig({ scriptName: "restart" });
    await updateScriptConfig("cfg-1", { scriptName: "restart" });
    await deleteScriptConfig("cfg-1");
    await dryRunScriptConfig("cfg-1", { force: true });
    await fetchLabEnvironments();
    await createLabEnvironment({ name: "lab-1" });
    await startLabEnvironment("lab-1");
    await stopLabEnvironment("lab-1");
    await resetLabEnvironment("lab-1");
    await deleteLabEnvironment("lab-1");
    await fetchCapabilityBindings();
    await fetchApprovalAudits({ page: 1, pageSize: 20 });
    await fetchApprovalGrants("host-1");
    await revokeApprovalGrant("grant-1");
    await disableApprovalGrant("grant-2");
    await enableApprovalGrant("grant-3");
    await fetchLlmConfig();
    await updateLlmConfig({ provider: "openai" });
    await fetchUiCards();
    await updateUiCard("card-1", { name: "Updated Card" });
    await previewUiCard("card-1", { foo: "bar" });
    await generateDraft({ source: "mcp_tool" });
    await lintDraft({ draftType: "skill" });
    await previewDraft({ draftType: "card" });
    await publishDraft({ draftType: "card" });
    await fetchFilePreview({ hostId: "server-local", path: "/tmp/a.log" });
    await fetchEvidenceDetail("session-1", "evidence-1");
    await fetchInvocationDetail("session-1", "invocation-1");

    expect(global.fetch.mock.calls).toEqual([
      ["/api/v1/state", expect.objectContaining({ method: "GET" })],
      ["/api/v1/thread/reset", expect.objectContaining({ method: "POST" })],
      ["/api/v1/host/select", expect.objectContaining({ method: "POST" })],
      ["/api/v1/sessions", expect.objectContaining({ method: "GET" })],
      ["/api/v1/sessions", expect.objectContaining({ method: "POST" })],
      ["/api/v1/sessions/session-1/activate", expect.objectContaining({ method: "POST" })],
      ["/api/v1/chat/message", expect.objectContaining({ method: "POST" })],
      ["/api/v1/chat/stop", expect.objectContaining({ method: "POST" })],
      ["/api/v1/approvals/approval-1/decision", expect.objectContaining({ method: "POST" })],
      ["/api/v1/choices/choice-1/answer", expect.objectContaining({ method: "POST" })],
      ["/api/v1/settings", expect.objectContaining({ method: "GET" })],
      ["/api/v1/settings", expect.objectContaining({ method: "POST" })],
      ["/api/v1/hosts", expect.objectContaining({ method: "POST" })],
      ["/api/v1/hosts/host-1", expect.objectContaining({ method: "PUT" })],
      ["/api/v1/hosts/host-1", expect.objectContaining({ method: "DELETE" })],
      ["/api/v1/mcp/servers", expect.objectContaining({ method: "GET" })],
      ["/api/v1/mcp/servers", expect.objectContaining({ method: "POST" })],
      ["/api/v1/mcp/servers/coroot", expect.objectContaining({ method: "PUT" })],
      ["/api/v1/mcp/servers/coroot", expect.objectContaining({ method: "DELETE" })],
      ["/api/v1/mcp/servers/coroot/close", expect.objectContaining({ method: "POST" })],
      ["/api/v1/mcp/servers/refresh", expect.objectContaining({ method: "POST" })],
      ["/api/v1/auth/login", expect.objectContaining({ method: "POST" })],
      ["/api/v1/auth/logout", expect.objectContaining({ method: "POST" })],
      ["/api/v1/terminal/sessions", expect.objectContaining({ method: "GET" })],
      ["/api/v1/terminal/sessions", expect.objectContaining({ method: "POST" })],
      ["/api/v1/agent-skills", expect.objectContaining({ method: "GET" })],
      ["/api/v1/agent-skills/ops-triage", expect.objectContaining({ method: "PUT" })],
      ["/api/v1/agent-skills/ops-triage", expect.objectContaining({ method: "DELETE" })],
      ["/api/v1/agent-mcps", expect.objectContaining({ method: "GET" })],
      ["/api/v1/agent-mcps/filesystem", expect.objectContaining({ method: "PUT" })],
      ["/api/v1/agent-mcps/filesystem", expect.objectContaining({ method: "DELETE" })],
      ["/api/v1/agent-profiles", expect.objectContaining({ method: "GET" })],
      ["/api/v1/agent-profiles/export", expect.objectContaining({ method: "GET" })],
      ["/api/v1/agent-profiles/import", expect.objectContaining({ method: "POST" })],
      ["/api/v1/agent-profile", expect.objectContaining({ method: "PUT" })],
      ["/api/v1/agent-profile/reset", expect.objectContaining({ method: "POST" })],
      ["/api/v1/agent-profile/preview?profileId=main-agent", expect.objectContaining({ method: "GET" })],
      ["/api/v1/script-configs", expect.objectContaining({ method: "GET" })],
      ["/api/v1/script-configs", expect.objectContaining({ method: "POST" })],
      ["/api/v1/script-configs/cfg-1", expect.objectContaining({ method: "PUT" })],
      ["/api/v1/script-configs/cfg-1", expect.objectContaining({ method: "DELETE" })],
      ["/api/v1/script-configs/cfg-1/dry-run", expect.objectContaining({ method: "POST" })],
      ["/api/v1/lab-environments", expect.objectContaining({ method: "GET" })],
      ["/api/v1/lab-environments", expect.objectContaining({ method: "POST" })],
      ["/api/v1/lab-environments/lab-1/start", expect.objectContaining({ method: "POST" })],
      ["/api/v1/lab-environments/lab-1/stop", expect.objectContaining({ method: "POST" })],
      ["/api/v1/lab-environments/lab-1/reset", expect.objectContaining({ method: "POST" })],
      ["/api/v1/lab-environments/lab-1", expect.objectContaining({ method: "DELETE" })],
      ["/api/v1/capability-bindings", expect.objectContaining({ method: "GET" })],
      ["/api/v1/approval-audits?page=1&pageSize=20", expect.objectContaining({ method: "GET" })],
      ["/api/v1/approval-grants?hostId=host-1", expect.objectContaining({ method: "GET" })],
      ["/api/v1/approval-grants/grant-1/revoke", expect.objectContaining({ method: "POST" })],
      ["/api/v1/approval-grants/grant-2/disable", expect.objectContaining({ method: "POST" })],
      ["/api/v1/approval-grants/grant-3/enable", expect.objectContaining({ method: "POST" })],
      ["/api/v1/llm-config", expect.objectContaining({ method: "GET" })],
      ["/api/v1/llm-config", expect.objectContaining({ method: "PUT" })],
      ["/api/v1/ui-cards", expect.objectContaining({ method: "GET" })],
      ["/api/v1/ui-cards/card-1", expect.objectContaining({ method: "PUT" })],
      ["/api/v1/ui-cards/card-1/preview", expect.objectContaining({ method: "POST" })],
      ["/api/v1/generator/generate", expect.objectContaining({ method: "POST" })],
      ["/api/v1/generator/lint", expect.objectContaining({ method: "POST" })],
      ["/api/v1/generator/preview", expect.objectContaining({ method: "POST" })],
      ["/api/v1/generator/publish-draft", expect.objectContaining({ method: "POST" })],
      ["/api/v1/files/preview?hostId=server-local&path=%2Ftmp%2Fa.log", expect.objectContaining({ method: "GET" })],
      ["/api/sessions/session-1/evidence/evidence-1", expect.objectContaining({ method: "GET" })],
      ["/api/sessions/session-1/invocations/invocation-1", expect.objectContaining({ method: "GET" })],
    ]);
  });

  it("exposes HttpClientError for callers that need structured handling", () => {
    const error = new HttpClientError("failure", { status: 500, url: "/api/v1/state", payload: { error: "failure" } });
    expect(error).toMatchObject({
      name: "HttpClientError",
      status: 500,
      url: "/api/v1/state",
      payload: { error: "failure" },
    });
  });
});
