import { afterEach, describe, expect, it, vi } from "vitest";

import {
  fetchHosts,
  fetchLlmConfig,
  fetchSessions,
  fetchTerminalSessions,
  normalizeLlmContextTokens,
  normalizeLlmMaxOutputTokens,
  normalizeLlmRequestTimeoutMs,
  normalizeOptionalFloat,
  updateLlmConfig,
  updateRuntimeSettings,
} from "@/pages/settingsApi";

describe("settingsApi", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("surfaces an actionable backend hint when the API proxy returns an empty server error", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response("", {
        status: 500,
        statusText: "Internal Server Error",
        headers: { "Content-Type": "text/plain" },
      }),
    );

    await expect(updateLlmConfig({ provider: "openai", model: "gpt-5.4" })).rejects.toThrow(/ai-server.*127\.0\.0\.1:18080/);
  });

  it("passes AbortSignal through read APIs", async () => {
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockImplementation(() =>
      Promise.resolve(new Response(JSON.stringify({ items: [], sessions: [] }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      })),
    );
    const controller = new AbortController();

    await fetchHosts({ signal: controller.signal });
    await fetchSessions({ signal: controller.signal });
    await fetchTerminalSessions({ signal: controller.signal });
    await fetchLlmConfig({ signal: controller.signal });

    expect(fetchSpy).toHaveBeenCalledWith(
      "/api/v1/hosts",
      expect.objectContaining({ signal: controller.signal }),
    );
    expect(fetchSpy).toHaveBeenCalledWith(
      "/api/v1/sessions",
      expect.objectContaining({ signal: controller.signal }),
    );
    expect(fetchSpy).toHaveBeenCalledWith(
      "/api/v1/terminal/sessions",
      expect.objectContaining({ signal: controller.signal }),
    );
    expect(fetchSpy).toHaveBeenCalledWith(
      "/api/v1/llm-config",
      expect.objectContaining({ signal: controller.signal }),
    );
  });

  it("keeps non-json error bodies visible instead of replacing them with a generic status", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response("upstream connect error", {
        status: 502,
        statusText: "Bad Gateway",
        headers: { "Content-Type": "text/plain" },
      }),
    );

    await expect(updateLlmConfig({ provider: "openai", model: "gpt-5.4" })).rejects.toThrow("upstream connect error");
  });

  it("adds the default LLM context size when the update payload omits it", async () => {
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({ ok: true }), { status: 200 }));

    await updateLlmConfig({ provider: "openai", model: "gpt-5.4" });

    expect(fetchSpy).toHaveBeenCalledWith(
      "/api/v1/llm-config",
      expect.objectContaining({
        method: "PUT",
        body: JSON.stringify({ provider: "openai", model: "gpt-5.4", maxContextTokens: 200000, maxOutputTokens: 20000, requestTimeoutMs: 300000 }),
      }),
    );
  });

  it("defaults empty or invalid LLM request timeout to 300000 ms before saving", async () => {
    expect(normalizeLlmRequestTimeoutMs(undefined)).toBe(300000);
    expect(normalizeLlmRequestTimeoutMs("")).toBe(300000);
    expect(normalizeLlmRequestTimeoutMs("bad")).toBe(300000);
    expect(normalizeLlmRequestTimeoutMs("-1")).toBe(300000);
    expect(normalizeLlmRequestTimeoutMs("25000.9")).toBe(25000);

    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({ ok: true }), { status: 200 }));

    await updateLlmConfig({ provider: "openai", model: "gpt-5.4", requestTimeoutMs: "" });

    expect(JSON.parse(String((fetchSpy.mock.calls[0][1] as RequestInit).body))).toMatchObject({
      requestTimeoutMs: 300000,
    });
  });

  it("passes reasoning effort through when saving LLM config", async () => {
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({ ok: true }), { status: 200 }));

    await updateLlmConfig({ provider: "openai", model: "gpt-5.4", reasoningEffort: "high" });

    expect(JSON.parse(String((fetchSpy.mock.calls[0][1] as RequestInit).body))).toMatchObject({
      provider: "openai",
      model: "gpt-5.4",
      reasoningEffort: "high",
      maxContextTokens: 200000,
      maxOutputTokens: 20000,
      requestTimeoutMs: 300000,
    });
  });

  it("normalizes small, decimal, and empty LLM context sizes before saving", async () => {
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockImplementation(() => Promise.resolve(new Response(JSON.stringify({ ok: true }), { status: 200 })));

    await updateLlmConfig({ provider: "openai", model: "gpt-5.4", maxContextTokens: "9999.8" });
    await updateLlmConfig({ provider: "openai", model: "gpt-5.4", maxContextTokens: "12000.8" });
    await updateLlmConfig({ provider: "openai", model: "gpt-5.4", maxContextTokens: "" });

    const bodies = fetchSpy.mock.calls.map((call) => JSON.parse(String((call[1] as RequestInit).body)));
    expect(bodies.map((body) => body.maxContextTokens)).toEqual([10000, 12000, 200000]);
  });

  it("normalizes max output tokens with an optional model cap", () => {
    expect(normalizeLlmContextTokens(undefined)).toBe(200000);
    expect(normalizeLlmMaxOutputTokens(undefined, 128000)).toBe(20000);
    expect(normalizeLlmMaxOutputTokens(200000, 128000)).toBe(128000);
    expect(normalizeLlmMaxOutputTokens("42.9", 128000)).toBe(42);
  });

  it("normalizes optional float fields before saving LLM config", async () => {
    expect(normalizeOptionalFloat("")).toBeUndefined();
    expect(normalizeOptionalFloat("0.95")).toBe(0.95);
    expect(normalizeOptionalFloat("bad")).toBeUndefined();

    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({ ok: true }), { status: 200 }));

    await updateLlmConfig({
      provider: "zhipu",
      model: "glm-5.2",
      apiKey: "",
      maxOutputTokens: "200000",
      temperature: "1",
      topP: "0.95",
      thinkingType: "enabled",
      reasoningEffort: "xhigh",
      toolStream: true,
    });

    expect(JSON.parse(String((fetchSpy.mock.calls[0][1] as RequestInit).body))).toMatchObject({
      provider: "zhipu",
      model: "glm-5.2",
      maxContextTokens: 200000,
      maxOutputTokens: 200000,
      requestTimeoutMs: 300000,
      temperature: 1,
      topP: 0.95,
      thinkingType: "enabled",
      reasoningEffort: "xhigh",
      toolStream: true,
    });
    expect(JSON.parse(String((fetchSpy.mock.calls[0][1] as RequestInit).body))).not.toHaveProperty("apiKey");
  });

  it("PATCHes runtime settings with a partial nested payload", async () => {
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ settings: { workflow: { validationProvider: "docker" } } }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await updateRuntimeSettings({
      workflow: { validationProvider: "docker" },
      debug: { modelInputTrace: false },
    });

    expect(fetchSpy).toHaveBeenCalledWith(
      "/api/v1/runtime-settings",
      expect.objectContaining({
        method: "PATCH",
        body: JSON.stringify({
          workflow: { validationProvider: "docker" },
          debug: { modelInputTrace: false },
        }),
      }),
    );
  });
});
