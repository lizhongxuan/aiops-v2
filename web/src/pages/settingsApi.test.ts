import { afterEach, describe, expect, it, vi } from "vitest";

import { updateLlmConfig } from "@/pages/settingsApi";

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
        body: JSON.stringify({ provider: "openai", model: "gpt-5.4", maxContextTokens: 200000 }),
      }),
    );
  });

  it("normalizes small, decimal, and empty LLM context sizes before saving", async () => {
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockImplementation(() => Promise.resolve(new Response(JSON.stringify({ ok: true }), { status: 200 })));

    await updateLlmConfig({ provider: "openai", model: "gpt-5.4", maxContextTokens: "9999.8" });
    await updateLlmConfig({ provider: "openai", model: "gpt-5.4", maxContextTokens: "12000.8" });
    await updateLlmConfig({ provider: "openai", model: "gpt-5.4", maxContextTokens: "" });

    const bodies = fetchSpy.mock.calls.map((call) => JSON.parse(String((call[1] as RequestInit).body)));
    expect(bodies.map((body) => body.maxContextTokens)).toEqual([10000, 12000, 200000]);
  });
});
