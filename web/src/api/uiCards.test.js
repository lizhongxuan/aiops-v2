import { describe, expect, it, vi } from "vitest";

import { createUiCardsClient } from "./uiCards";

function createRecordingHttpClient() {
  const calls = [];
  const makeMethod = (method) =>
    vi.fn((path, body) => {
      calls.push({ method, path, body });
      return Promise.resolve({ ok: true });
    });

  return {
    calls,
    get: makeMethod("GET"),
    post: makeMethod("POST"),
    put: makeMethod("PUT"),
    delete: makeMethod("DELETE"),
  };
}

describe("uiCards client", () => {
  it("routes every method through exact same-origin UI card paths", async () => {
    const http = createRecordingHttpClient();
    const client = createUiCardsClient(http);
    const payload = { id: "card/1", type: "coroot_chart" };

    await client.fetchUiCards();
    await client.getUiCard("card/1");
    await client.createUiCard(payload);
    await client.updateUiCard("card/1", payload);
    await client.deleteUiCard("card/1");
    await client.previewUiCard("card/1", { input: { service: "checkout" } });
    await client.validateUiCard("card/1", { schema: "v1" });
    await client.createUiCardVersion("card/1", { reason: "schema update" });
    await client.updateUiCardStatus("card/1", "disabled");

    expect(http.calls).toEqual([
      { method: "GET", path: "/api/v1/ui-cards", body: undefined },
      { method: "GET", path: "/api/v1/ui-cards/card%2F1", body: undefined },
      { method: "POST", path: "/api/v1/ui-cards", body: payload },
      { method: "PUT", path: "/api/v1/ui-cards/card%2F1", body: payload },
      { method: "DELETE", path: "/api/v1/ui-cards/card%2F1", body: undefined },
      { method: "POST", path: "/api/v1/ui-cards/card%2F1/preview", body: { input: { service: "checkout" } } },
      { method: "POST", path: "/api/v1/ui-cards/card%2F1/validate", body: { schema: "v1" } },
      { method: "POST", path: "/api/v1/ui-cards/card%2F1/versions", body: { reason: "schema update" } },
      { method: "PUT", path: "/api/v1/ui-cards/card%2F1/status", body: { status: "disabled" } },
    ]);
  });
});
