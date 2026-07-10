import { afterEach, describe, expect, it, vi } from "vitest";

import { fetchCorootConfig, saveCorootConfig, testCorootConnection } from "@/api/coroot";

describe("coroot api", () => {
  afterEach(() => vi.restoreAllMocks());

  it("fetches embedded config contract", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(
        JSON.stringify({
          configured: true,
          project: "5hxbfx6p",
          entryPath: "/coroot/p/5hxbfx6p/applications",
          iframeEntryPath: "/_coroot/p/5hxbfx6p/applications?embed=1",
          authMode: "anonymous_readonly",
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );

    await expect(fetchCorootConfig()).resolves.toMatchObject({
      configured: true,
      project: "5hxbfx6p",
      entryPath: "/coroot/p/5hxbfx6p/applications",
    });
  });

  it("saves config with auth mode and project", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ configured: true, project: "5hxbfx6p" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await saveCorootConfig({
      baseUrl: "http://172.18.13.11:8000/coroot",
      project: "5hxbfx6p",
      authMode: "anonymous_readonly",
      clearToken: false,
      timeout: "30s",
    });

    const request = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(JSON.parse(String(request.body))).toMatchObject({
      authMode: "anonymous_readonly",
      clearToken: false,
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/coroot/config",
      expect.objectContaining({ method: "POST" }),
    );
  });

  it("saves the AI Chat Coroot Web credentials separately from embed trust", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ configured: true, project: "5hxbfx6p", tokenConfigured: true }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await saveCorootConfig({
      baseUrl: "http://172.18.13.11:8000",
      project: "5hxbfx6p",
      authMode: "session_passthrough",
      token: "coroot_session=web-session",
      username: "admin",
      password: "secret",
      clearToken: false,
      clearPassword: false,
      timeout: "30s",
    });

    const request = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(JSON.parse(String(request.body))).toMatchObject({
      authMode: "session_passthrough",
      token: "coroot_session=web-session",
      username: "admin",
      password: "secret",
      clearToken: false,
      clearPassword: false,
    });
  });

  it("saves embed trust shared secret separately from token", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ configured: true, project: "5hxbfx6p", authMode: "embed_trust" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await saveCorootConfig({
      baseUrl: "http://172.18.13.11:8000/coroot",
      project: "5hxbfx6p",
      authMode: "embed_trust",
      embedMode: "full",
      uiGatewayEnabled: true,
      embedTrustSecret: "shared-secret",
      timeout: "30s",
    });

    const request = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(JSON.parse(String(request.body))).toMatchObject({
      authMode: "embed_trust",
      embedMode: "full",
      uiGatewayEnabled: true,
      embedTrustSecret: "shared-secret",
    });
    expect(JSON.parse(String(request.body))).not.toHaveProperty("token");
  });

  it("posts connection test input", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ ok: true, latencyMs: 12 }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await expect(
      testCorootConnection({
        baseUrl: "http://172.18.13.11:8000/coroot",
        project: "5hxbfx6p",
        authMode: "anonymous_readonly",
      }),
    ).resolves.toMatchObject({ ok: true });

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/coroot/test-connection",
      expect.objectContaining({ method: "POST" }),
    );
  });
});
