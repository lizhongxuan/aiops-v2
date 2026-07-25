// @ts-check

import assert from "node:assert/strict";
import test from "node:test";

import { startRecordedWorkflowModelServer } from "./recordedWorkflowModelServer.js";

const tools = ["workflow_get_context", "workflow_preview_patch", "workflow_apply_patch"].map((name) => ({
  type: "function",
  function: { name, parameters: { type: "object" } },
}));

test("recorded Workflow model drives typed context, preview, approval-bound Apply, and final", async (t) => {
  const model = await startRecordedWorkflowModelServer();
  t.after(() => model.close());
  const marker = "WORKFLOW_MCP_APPLY tenant=runner-default workflow=workflow-a revision=rv-1 environment=env-1 node=action label_b64=QXBwcm92ZWQ";
  const messages = [{ role: "user", content: marker }];

  const contextCall = await complete(model.baseURL, messages);
  assert.equal(contextCall.finish_reason, "tool_calls");
  const contextArgs = JSON.parse(contextCall.message.tool_calls[0].function.arguments);
  assert.deepEqual(contextArgs, { schemaVersion: "aiops.workflow.edit.v2", revision: "rv-1", view: "summary", limit: 20 });
  assert.equal(Object.hasOwn(contextArgs, "workflowKey"), false);
  messages.push(contextCall.message, toolMessage(contextCall, {
    kind: "context",
    revision: { resourceVersion: "rv-1", semanticHash: "semantic-1", layoutHash: "layout-1" },
  }));

  const previewCall = await complete(model.baseURL, messages);
  assert.equal(previewCall.message.tool_calls[0].function.name, "workflow_preview_patch");
  const previewArgs = JSON.parse(previewCall.message.tool_calls[0].function.arguments);
  assert.equal(previewArgs.expected.semanticHash, "semantic-1");
  assert.equal(previewArgs.validationBasis.environmentBindingVersion, "env-1");
  assert.deepEqual(previewArgs.operations, [{ op: "update_node", nodeId: "action", set: { label: "Approved" } }]);
  messages.push(previewCall.message, toolMessage(previewCall, {
    kind: "preview",
    changeSet: { id: "change-1", previewHash: "sha256:preview", base: { resourceVersion: "rv-1" } },
    candidate: { currentChangeSetId: "change-1", currentPreviewHash: "sha256:preview" },
  }));

  const applyCall = await complete(model.baseURL, messages);
  assert.equal(applyCall.message.tool_calls[0].function.name, "workflow_apply_patch");
  const applyArgs = JSON.parse(applyCall.message.tool_calls[0].function.arguments);
  assert.equal(applyArgs.changeSetId, "change-1");
  assert.equal(applyArgs.previewHash, "sha256:preview");
  assert.equal(applyArgs.expectedRevision, "rv-1");
  assert.match(applyArgs.candidateGrantId, /^recorded-grant-/);
  assert.match(applyArgs.applyApprovalId, /^recorded-approval-/);
  messages.push(applyCall.message, toolMessage(applyCall, { kind: "receipt", transactionId: "tx-1", status: "completed" }));

  const final = await complete(model.baseURL, messages);
  assert.equal(final.finish_reason, "stop");
  assert.match(final.message.content, /RECORDED_WORKFLOW_MCP_APPLY_OK/);
});

test("recorded Workflow read uses server-owned scope and requires a typed context result", async (t) => {
  const model = await startRecordedWorkflowModelServer();
  t.after(() => model.close());
  const messages = [{ role: "user", content: "WORKFLOW_MCP_READ tenant=runner-default workflow=workflow-a revision=rv-1" }];
  const call = await complete(model.baseURL, messages);
  const args = JSON.parse(call.message.tool_calls[0].function.arguments);
  assert.equal(Object.hasOwn(args, "workflowKey"), false);
  messages.push(call.message, toolMessage(call, { kind: "context", revision: { resourceVersion: "rv-1" } }));
  const final = await complete(model.baseURL, messages);
  assert.match(final.message.content, /RECORDED_WORKFLOW_MCP_READ_OK/);
});

test("recorded Workflow model emits a selected-node threshold-only patch", async (t) => {
  const model = await startRecordedWorkflowModelServer();
  t.after(() => model.close());
  const messages = [{
    role: "user",
    content: "WORKFLOW_MCP_APPLY tenant=runner-default workflow=workflow-branch revision=rv-2 environment=env-2 node=condition operation=threshold threshold=95",
  }];

  const contextCall = await complete(model.baseURL, messages);
  messages.push(contextCall.message, toolMessage(contextCall, {
    kind: "context",
    revision: { resourceVersion: "rv-2", semanticHash: "semantic-2", layoutHash: "layout-2" },
  }));
  const previewCall = await complete(model.baseURL, messages);
  assert.equal(previewCall.message.tool_calls[0].function.name, "workflow_preview_patch");
  const previewArgs = JSON.parse(previewCall.message.tool_calls[0].function.arguments);
  assert.equal(previewArgs.intent, "仅更新节点 condition 的 threshold");
  assert.deepEqual(previewArgs.operations, [{
    op: "update_node",
    nodeId: "condition",
    set: {
      inputs: [{
        key: "threshold",
        type: "integer",
        label: "threshold",
        required: true,
        value_source: { type: "literal", value: 95 },
      }],
    },
  }]);
});

test("recorded Workflow preview marker stops after Candidate v1 without opening an Apply approval", async (t) => {
  const model = await startRecordedWorkflowModelServer();
  t.after(() => model.close());
  const messages = [{
    role: "user",
    content: "WORKFLOW_MCP_PREVIEW tenant=runner-default workflow=workflow-a revision=rv-1 environment=env-1 node=action label_b64=Q2FuZGlkYXRlIHYx",
  }];

  const contextCall = await complete(model.baseURL, messages);
  assert.equal(contextCall.message.tool_calls[0].function.name, "workflow_get_context");
  messages.push(contextCall.message, toolMessage(contextCall, {
    kind: "context",
    revision: { resourceVersion: "rv-1", semanticHash: "semantic-1", layoutHash: "layout-1" },
  }));

  const previewCall = await complete(model.baseURL, messages);
  assert.equal(previewCall.message.tool_calls[0].function.name, "workflow_preview_patch");
  assert.deepEqual(JSON.parse(previewCall.message.tool_calls[0].function.arguments).operations, [
    { op: "update_node", nodeId: "action", set: { label: "Candidate v1" } },
  ]);
  messages.push(previewCall.message, toolMessage(previewCall, {
    kind: "preview",
    changeSet: { id: "change-1", previewHash: "sha256:preview", base: { resourceVersion: "rv-1" } },
    candidate: { currentChangeSetId: "change-1", currentPreviewHash: "sha256:preview" },
  }));

  const final = await complete(model.baseURL, messages);
  assert.equal(final.finish_reason, "stop");
  assert.equal(final.message.tool_calls, undefined);
  assert.match(final.message.content, /RECORDED_WORKFLOW_MCP_PREVIEW_OK/);
});

test("recorded Workflow preview marker can drive a typed validation failure", async (t) => {
  const model = await startRecordedWorkflowModelServer();
  t.after(() => model.close());
  const messages = [{
    role: "user",
    content: [
      "WORKFLOW_MCP_PREVIEW tenant=runner-default workflow=workflow-a revision=stale-rv environment=stale-env node=stale-node label_b64=U3RhbGU",
      "WORKFLOW_MCP_PREVIEW tenant=runner-default workflow=workflow-a revision=rv-1 environment=env-1 node=action operation=invalid label_b64=RmFpbGVk",
    ].join("\n"),
  }];

  const contextCall = await complete(model.baseURL, messages);
  assert.equal(JSON.parse(contextCall.message.tool_calls[0].function.arguments).revision, "rv-1");
  messages.push(contextCall.message, toolMessage(contextCall, {
    kind: "context",
    revision: { resourceVersion: "rv-1", semanticHash: "semantic-1", layoutHash: "layout-1" },
  }));

  const previewCall = await complete(model.baseURL, messages);
  assert.equal(previewCall.message.tool_calls[0].function.name, "workflow_preview_patch");
  assert.deepEqual(JSON.parse(previewCall.message.tool_calls[0].function.arguments).operations, [
    { op: "update_node", nodeId: "", set: { label: "Failed" } },
  ]);
  messages.push(previewCall.message, toolErrorMessage(previewCall, "invalid workflow preview operation: nodeId is required"));

  const final = await complete(model.baseURL, messages);
  assert.equal(final.finish_reason, "stop");
  assert.equal(final.message.tool_calls, undefined);
  assert.match(final.message.content, /RECORDED_WORKFLOW_MCP_PREVIEW_ERROR/);
  assert.match(final.message.content, /nodeId is required/);
});

async function complete(baseURL, messages) {
  const response = await fetch(`${baseURL}/chat/completions`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ model: "recorded", stream: false, messages, tools }),
  });
  assert.equal(response.status, 200);
  return (await response.json()).choices[0];
}

function toolMessage(choice, structuredContent) {
  return {
    role: "tool",
    tool_call_id: choice.message.tool_calls[0].id,
    content: JSON.stringify({
      schemaVersion: "aiops.model_tool_result.v1",
      summary: "typed result",
      isError: false,
      structuredContent: {
        schemaVersion: "aiops.workflow.edit.v2",
        content: [{ type: "text", text: "typed result" }],
        structuredContent,
        isError: false,
      },
    }),
  };
}

function toolErrorMessage(choice, summary) {
  return {
    role: "tool",
    tool_call_id: choice.message.tool_calls[0].id,
    content: JSON.stringify({
      schemaVersion: "aiops.model_tool_result.v1",
      summary,
      isError: true,
    }),
  };
}
