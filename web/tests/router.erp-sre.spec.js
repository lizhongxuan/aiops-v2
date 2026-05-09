import { describe, expect, it } from "vitest";
import { matchPath } from "react-router-dom";

import { routeInventory } from "../src/app/navigation";

describe("ERP SRE router", () => {
  it("registers incident, ERP, graph, runbook, Runner, and postmortem routes", () => {
    const routes = new Set(routeInventory.map((route) => route.path));

    expect(routes.has("/incidents")).toBe(true);
    expect(routes.has("/incidents/:incidentId")).toBe(true);
    expect(routes.has("/erp")).toBe(true);
    expect(routes.has("/opsgraph")).toBe(true);
    expect(routes.has("/runbooks")).toBe(true);
    expect(routes.has("/runbooks/:runbookId")).toBe(true);
    expect(routes.has("/runner")).toBe(true);
    expect(routes.has("/runner/:workflowName")).toBe(true);
    expect(routes.has("/postmortems/:postmortemId")).toBe(true);
  });

  it("resolves detail route params without falling back to chat", () => {
    const paths = routeInventory.map((route) => route.path);

    expect(matches(paths, "/incidents/inc-20260503")).toBe("/incidents/:incidentId");
    expect(matches(paths, "/runbooks/erp-order-submit-slow")).toBe("/runbooks/:runbookId");
    expect(matches(paths, "/runner/pg-restore")).toBe("/runner/:workflowName");
    expect(matches(paths, "/postmortems/pm-20260503")).toBe("/postmortems/:postmortemId");
  });
});

function matches(patterns, pathname) {
  return patterns.find((path) => matchPath({ path, end: true }, pathname));
}
