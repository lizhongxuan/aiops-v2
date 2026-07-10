import { describe, expect, it } from "vitest";

import { fromCorootRouteMessage, toCorootIframePath, toCorootProductPath } from "@/pages/coroot/corootRoutes";

describe("coroot route helpers", () => {
  it("builds default product entry path", () => {
    expect(toCorootProductPath({ projectId: "5hxbfx6p", view: "applications" })).toBe("/coroot/p/5hxbfx6p/applications");
  });

  it("maps product route to gateway iframe route and preserves query", () => {
    expect(
      toCorootIframePath({
        pathname: "/coroot/p/5hxbfx6p/applications/aiops-host-agent/CPU",
        search: "?from=now-1h&to=now",
      }),
    ).toBe("/_coroot/p/5hxbfx6p/applications/aiops-host-agent/CPU?embed=1&from=now-1h&to=now");
  });

  it("defaults missing view to applications", () => {
    expect(toCorootIframePath({ pathname: "/coroot/p/5hxbfx6p", search: "" })).toBe("/_coroot/p/5hxbfx6p/applications?embed=1");
  });

  it("maps coroot route message to outer aiops url", () => {
    expect(
      fromCorootRouteMessage({
        type: "aiops.coroot.route.v1",
        projectId: "5hxbfx6p",
        view: "applications",
        id: "aiops-host-agent",
        report: "CPU",
        query: { from: "now-1h", to: "now" },
      }),
    ).toBe("/coroot/p/5hxbfx6p/applications/aiops-host-agent/CPU?from=now-1h&to=now");
  });

  it("ignores non-coroot route messages", () => {
    expect(fromCorootRouteMessage({ type: "irrelevant", projectId: "5hxbfx6p" })).toBeNull();
  });
});
