// @ts-check
import { expect, test } from "@playwright/test";

const realSmokeEnabled = process.env.AIOPS_REAL_HOST_OPS_SMOKE === "1";
const isolatedSmokeEnabled = realSmokeEnabled && process.env.AIOPS_REAL_HOST_OPS_ISOLATED === "1";

test.describe("real host ops PostgreSQL smoke", () => {
  test.skip(
    !isolatedSmokeEnabled,
    "set AIOPS_REAL_HOST_OPS_SMOKE=1, AIOPS_REAL_HOST_OPS_ISOLATED=1, and AIOPS_TEST_* env vars to run the real host-agent PostgreSQL smoke",
  );

  test("starts one host child agent and verifies PostgreSQL through host-agent", async ({ page, request }) => {
    test.setTimeout(15 * 60 * 1000);

    const llmBaseURL = requiredEnv("AIOPS_TEST_LLM_BASE_URL");
    const llmAPIKey = requiredEnv("AIOPS_TEST_LLM_API_KEY");
    const llmModel = process.env.AIOPS_TEST_LLM_MODEL || "gpt-5.4";
    const hostAddress = requiredEnv("AIOPS_TEST_HOST_ADDRESS");
    const sshUser = requiredEnv("AIOPS_TEST_HOST_SSH_USER");
    const sshPassword = requiredEnv("AIOPS_TEST_HOST_SSH_PASSWORD");
    const sshPort = Number.parseInt(process.env.AIOPS_TEST_HOST_SSH_PORT || "22", 10);
    const hostId = process.env.AIOPS_TEST_HOST_ID || `real-pg-${hostAddress.replace(/[^a-zA-Z0-9_.-]/g, "-")}`;
    const existingHost = await findHost(request, hostId);
    if (existingHost && process.env.AIOPS_REAL_HOST_OPS_REUSE_HOST !== "1") {
      throw new Error(`host ${hostId} already exists; set AIOPS_TEST_HOST_ID to a fresh id or AIOPS_REAL_HOST_OPS_REUSE_HOST=1`);
    }
    const createdHost = !existingHost;

    try {
      await putJSON(request, "/api/v1/llm-config", {
        provider: "openai",
        baseURL: llmBaseURL,
        apiKey: llmAPIKey,
        model: llmModel,
        maxContextTokens: 200000,
      });

      await upsertHost(request, hostId, {
        id: hostId,
        name: hostId,
        address: hostAddress,
        sshUser,
        sshPort,
        sshPassword,
        agentVersion: "v0.1.0",
        labels: {
          "aiops.smoke": "postgresql",
        },
      });

      await postJSON(request, `/api/v1/hosts/${encodeURIComponent(hostId)}/ssh/test`, {
        sshPassword,
      });

      await postJSON(request, `/api/v1/hosts/${encodeURIComponent(hostId)}/install`, {
        sshPassword,
        force: false,
      });
      await waitForManagedHost(request, hostId);

      await page.goto("/", { waitUntil: "domcontentloaded" });
      await expect(page.getByTestId("omnibar-input")).toBeVisible({ timeout: 30000 });

      await page.getByTestId("omnibar-input").fill(`@${hostAddress} 安装 PostgreSQL，只做单机安装和版本检查，不配置主从，不删除已有数据。`);
      await page.getByTestId("omnibar-primary-action").click();

      const panel = page.getByTestId("host-ops-status-panel");
      await expect(panel).toBeVisible({ timeout: 120000 });
      await expect(panel).toContainText("1 个后台智能体", { timeout: 120000 });
      await expect(panel).toContainText(hostAddress, { timeout: 120000 });

      const row = page.locator('[data-testid^="host-subagent-status-row-"]').first();
      await expect(row).toBeVisible({ timeout: 120000 });
      const childTestId = await row.getAttribute("data-testid");
      const childAgentId = childTestId?.replace("host-subagent-status-row-", "");
      expect(childAgentId, "child agent id from status row").toBeTruthy();
      await row.click();

      const drawer = page.getByTestId("host-subagent-drawer");
      await expect(drawer).toBeVisible({ timeout: 30000 });
      await expect(drawer).toContainText(hostAddress, { timeout: 30000 });

      const approval = page.getByTestId("codex-approval-inline");
      if (await approval.isVisible({ timeout: 5000 }).catch(() => false)) {
        await expect(page.getByTestId("codex-approval-command")).toContainText("PostgreSQL", { timeout: 30000 });
        await approval.getByRole("button", { name: "提交" }).click();
      }

      await expect(row).toContainText("已完成", { timeout: 8 * 60 * 1000 });
      await expect(drawer).toContainText(/PostgreSQL|psql/i, { timeout: 8 * 60 * 1000 });
      await expect(page.locator("body")).toContainText(/PostgreSQL|psql|安装|版本/i, { timeout: 8 * 60 * 1000 });

      const transcript = await getJSON(request, `/api/v1/host-ops/child-agents/${encodeURIComponent(childAgentId)}/transcript`);
      const items = Array.isArray(transcript.items) ? transcript.items : [];
      expect(items.some((item) => item?.type === "manager_message" && String(item.content || "").includes(hostAddress))).toBeTruthy();
      expect(items.some((item) => item?.type === "tool_call" && String(item.toolName || "").includes("ensure_postgresql_installed"))).toBeTruthy();
      expect(items.some((item) => item?.type === "tool_result" && /PostgreSQL|psql/i.test(String(item.content || "")))).toBeTruthy();
      expect(items.some((item) => /\bpsql\s+\(PostgreSQL\)\s+\d+/i.test(String(item.content || "")))).toBeTruthy();
    } finally {
      if (createdHost) {
        await deleteJSON(request, `/api/v1/hosts/${encodeURIComponent(hostId)}`).catch(() => {});
      }
    }
  });
});

function requiredEnv(name) {
  const value = process.env[name];
  if (!value || !value.trim()) {
    throw new Error(`${name} is required`);
  }
  return value.trim();
}

async function upsertHost(request, hostId, payload) {
  if (await findHost(request, hostId)) {
    await putJSON(request, `/api/v1/hosts/${encodeURIComponent(hostId)}`, payload);
    return;
  }
  await postJSON(request, "/api/v1/hosts", payload);
}

async function findHost(request, hostId) {
  const hosts = await getJSON(request, "/api/v1/hosts");
  const items = Array.isArray(hosts.items) ? hosts.items : [];
  return items.find((item) => item?.id === hostId) || null;
}

async function waitForManagedHost(request, hostId) {
  const deadline = Date.now() + 120000;
  let lastHost = null;
  while (Date.now() < deadline) {
    const hosts = await getJSON(request, "/api/v1/hosts");
    const items = Array.isArray(hosts.items) ? hosts.items : [];
    lastHost = items.find((item) => item?.id === hostId) || null;
    if (lastHost?.status === "online" && lastHost?.executable && lastHost?.agentUrl) {
      return lastHost;
    }
    await new Promise((resolve) => setTimeout(resolve, 3000));
  }
  throw new Error(`host ${hostId} did not become managed/online; last status=${JSON.stringify(redactHost(lastHost))}`);
}

function redactHost(host) {
  if (!host) return null;
  const clone = { ...host };
  delete clone.sshCredentialRef;
  delete clone.agentTokenRef;
  return clone;
}

async function getJSON(request, path) {
  const response = await request.get(path);
  return parseJSONResponse(response, path);
}

async function postJSON(request, path, data) {
  const response = await request.post(path, { data });
  return parseJSONResponse(response, path);
}

async function putJSON(request, path, data) {
  const response = await request.put(path, { data });
  return parseJSONResponse(response, path);
}

async function deleteJSON(request, path) {
  const response = await request.delete(path);
  return parseJSONResponse(response, path);
}

async function parseJSONResponse(response, path) {
  const text = await response.text();
  let payload = {};
  if (text.trim()) {
    try {
      payload = JSON.parse(text);
    } catch {
      payload = { message: text.slice(0, 300) };
    }
  }
  if (!response.ok()) {
    const message = typeof payload.error === "string" ? payload.error : typeof payload.message === "string" ? payload.message : response.statusText();
    throw new Error(`${path} failed with HTTP ${response.status()}: ${message}`);
  }
  return payload;
}
