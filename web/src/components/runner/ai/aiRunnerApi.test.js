import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it, vi } from "vitest";
import { createAiRunnerApi } from "./aiRunnerApi";

function createRecordingHttpClient() {
  const calls = [];
  return {
    calls,
    post: vi.fn((path, body) => {
      calls.push({ method: "POST", path, body });
      return Promise.resolve({ graph_patch: { operations: [] }, diff_summary: {} });
    }),
  };
}

describe("aiRunnerApi", () => {
  it("generates AI workflow drafts through the same-origin Runner Studio API", async () => {
    const http = createRecordingHttpClient();
    const api = createAiRunnerApi(http);
    const payload = {
      workflow_status: "draft",
      instruction: "生成 PostgreSQL 恢复流程",
      graph: { nodes: [] },
    };

    await api.generateRunnerWorkflowDraft(payload);

    expect(http.calls).toEqual([
      {
        method: "POST",
        path: "/api/runner-studio/ai/draft",
        body: payload,
      },
    ]);
  });

  it("does not hardcode LLM URL, API key, or model in the frontend AI API", () => {
    const source = readFileSync(join(process.cwd(), "src/components/runner/ai/aiRunnerApi.js"), "utf8");

    expect(source).not.toMatch(/127\.0\.0\.1|8317|sk-|gpt-5\.4|api[_-]?key|base[_-]?url/i);
  });
});
