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
});
