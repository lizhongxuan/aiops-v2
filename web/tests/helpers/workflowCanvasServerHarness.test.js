// @ts-check

import assert from "node:assert/strict";
import test from "node:test";

import { materializeWorkflowFixture } from "./workflowCanvasServerHarness.js";

test("materializes W4 as one stable 500-node/700-edge connected DAG", () => {
  const first = materializeWorkflowFixture("W4", "unit");
  const second = materializeWorkflowFixture("W4", "unit");
  assert.deepEqual(second, first);
  assert.equal(first.graph.nodes.length, 500);
  assert.equal(first.graph.edges.length, 700);
  assert.equal(new Set(first.graph.nodes.map((node) => node.id)).size, 500);
  assert.equal(new Set(first.graph.edges.map((edge) => edge.id)).size, 700);
  assert.equal(first.graph.nodes.filter((node) => node.type === "start").length, 1);
  assert.equal(first.graph.nodes.filter((node) => node.type === "end").length, 1);

  const order = new Map(first.graph.nodes.map((node, index) => [node.id, index]));
  for (const edge of first.graph.edges) {
    assert.ok(order.has(edge.source), `missing source ${edge.source}`);
    assert.ok(order.has(edge.target), `missing target ${edge.target}`);
    assert.ok(order.get(edge.source) < order.get(edge.target), `${edge.id} must point forward`);
  }
  for (let index = 0; index < first.graph.nodes.length - 1; index += 1) {
    assert.ok(first.graph.edges.some((edge) => edge.source === first.graph.nodes[index].id && edge.target === first.graph.nodes[index + 1].id));
  }
});

test("materializes W5 with an identifier-only missing Secret and no credential value", () => {
  const fixture = materializeWorkflowFixture("W5", "unit");
  const connector = fixture.graph.nodes.find((node) => node.id === "connector");
  assert.equal(connector?.step?.action, "http.request");
  assert.deepEqual(connector?.step?.args?.auth, { type: "bearer", secret_ref: null });
  assert.equal(fixture.graph.ui?.fixture_configuration_state, "configuration_required");
  assert.deepEqual(fixture.graph.ui?.fixture_secret_refs, []);

  const encoded = JSON.stringify(fixture.graph);
  assert.doesNotMatch(encoded, /password|api[_-]?key|access[_-]?token|private[_-]?key/i);
  assert.doesNotMatch(encoded, /"(?:value|token)"\s*:/i);
});

test("keeps small fixtures on the real Runner-safe action boundary", () => {
  const fixture = materializeWorkflowFixture("W1", "unit");
  assert.equal(fixture.graph.nodes.length, 3);
  assert.equal(fixture.graph.edges.length, 2);
  assert.equal(fixture.graph.nodes.find((node) => node.id === "action")?.step?.action, "script.shell");
});
