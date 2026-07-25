#!/usr/bin/env node
// @ts-check

import { appendFile, mkdir } from "node:fs/promises";
import http from "node:http";
import path from "node:path";
import { fileURLToPath } from "node:url";

const DEFAULT_HOST = "127.0.0.1";
const MAX_REQUEST_BYTES = 2 * 1024 * 1024;
const FINAL_ONLY_MARKER = "WORKFLOW_HARNESS_FINAL_ONLY";
const WORKFLOW_READ_MARKER = "WORKFLOW_MCP_READ";
const WORKFLOW_PREVIEW_MARKER = "WORKFLOW_MCP_PREVIEW";
const WORKFLOW_APPLY_MARKER = "WORKFLOW_MCP_APPLY";

/**
 * Starts a deterministic OpenAI-compatible server used by the real ai-server.
 * It records only a redacted request shape: authorization values and message
 * bodies are never persisted.
 *
 * @param {{host?: string, port?: number, recordPath?: string}} [options]
 */
export async function startRecordedWorkflowModelServer(options = {}) {
  const host = options.host || DEFAULT_HOST;
  const requests = [];
  let requestSequence = 0;
  const server = http.createServer(async (request, response) => {
    try {
      const url = new URL(request.url || "/", `http://${request.headers.host || host}`);
      if (request.method === "GET" && url.pathname === "/healthz") {
        return json(response, 200, { status: "ok", kind: "recorded-workflow-model" });
      }
      if (request.method === "GET" && url.pathname === "/__recorded__/requests") {
        return json(response, 200, { requests });
      }
      if (request.method !== "POST" || !url.pathname.endsWith("/chat/completions")) {
        return json(response, 404, { error: { message: "recorded endpoint not found", type: "not_found" } });
      }

      const body = await readJSONBody(request);
      const sequence = ++requestSequence;
      const decision = recordedDecision(body, sequence);
      const record = redactedRequestRecord(body, request, sequence, decision.kind);
      requests.push(record);
      if (options.recordPath) {
        await mkdir(path.dirname(options.recordPath), { recursive: true });
        await appendFile(options.recordPath, `${JSON.stringify(record)}\n`, "utf8");
      }

      if (body.stream === true) {
        return streamCompletion(response, decision, sequence, String(body.model || "recorded-workflow-model"));
      }
      return json(response, 200, completionPayload(decision, sequence, String(body.model || "recorded-workflow-model")));
    } catch (error) {
      return json(response, 400, {
        error: {
          message: error instanceof Error ? error.message : "invalid recorded model request",
          type: "invalid_request_error",
        },
      });
    }
  });

  await new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(options.port || 0, host, resolve);
  });
  const address = server.address();
  if (!address || typeof address === "string") throw new Error("recorded model server did not expose a TCP address");
  const origin = `http://${host}:${address.port}`;
  return {
    origin,
    baseURL: `${origin}/v1`,
    requests,
    close: () => new Promise((resolve, reject) => server.close((error) => error ? reject(error) : resolve())),
  };
}

function recordedDecision(body, sequence) {
  const messages = Array.isArray(body.messages) ? body.messages : [];
  const content = messages.map((message) => String(message?.content || "")).join("\n");
  const results = recordedToolResults(messages);
  const workflowContextTool = workflowToolName(body, "workflow.get_context");

  if (content.includes(WORKFLOW_PREVIEW_MARKER)) {
    const scope = markerScope(content, WORKFLOW_PREVIEW_MARKER);
    const failed = [...results].reverse().find((result) => result?.isError === true);
    if (failed) {
      return {
        kind: "workflow_preview_error_final",
        content: `RECORDED_WORKFLOW_MCP_PREVIEW_ERROR：${String(failed.summary || failed.error || "工具返回错误")}`,
        toolCalls: [],
        finishReason: "stop",
      };
    }
    if (results.some((result) => result?.structuredContent?.kind === "preview")) {
      return {
        kind: "workflow_preview_final",
        content: "RECORDED_WORKFLOW_MCP_PREVIEW_OK：候选修改已生成，等待用户继续审阅。",
        toolCalls: [],
        finishReason: "stop",
      };
    }
    return workflowPreviewStepDecision(body, sequence, results, scope, workflowContextTool, "preview");
  }

  if (content.includes(WORKFLOW_APPLY_MARKER)) {
    const scope = markerScope(content, WORKFLOW_APPLY_MARKER);
    const receipt = results.find((result) => result?.structuredContent?.kind === "receipt");
    if (receipt) {
      return {
        kind: "workflow_apply_final",
        content: "RECORDED_WORKFLOW_MCP_APPLY_OK：候选修改已通过精确审批并提交。",
        toolCalls: [],
        finishReason: "stop",
      };
    }
    const failed = [...results].reverse().find((result) => result?.isError === true);
    if (failed) {
      return {
        kind: "workflow_apply_error_final",
        content: `RECORDED_WORKFLOW_MCP_APPLY_ERROR：${String(failed.summary || failed.error || "工具返回错误")}`,
        toolCalls: [],
        finishReason: "stop",
      };
    }
    const preview = results.find((result) => result?.structuredContent?.kind === "preview")?.structuredContent;
    if (preview) {
      const applyTool = workflowToolName(body, "workflow.apply_patch");
      if (!applyTool) return missingWorkflowToolDecision("workflow.apply_patch");
      const changeSet = preview.changeSet || {};
      const candidate = preview.candidate || {};
      return toolCallDecision("workflow_apply", applyTool, sequence, {
        schemaVersion: "aiops.workflow.edit.v2",
        workflowKey: { tenantId: scope.tenantId, workflowId: scope.workflowId },
        changeSetId: String(changeSet.id || candidate.currentChangeSetId || ""),
        previewHash: String(changeSet.previewHash || candidate.currentPreviewHash || ""),
        expectedRevision: String(changeSet?.base?.resourceVersion || scope.revision),
        idempotencyKey: `recorded-apply-${String(changeSet.id || candidate.currentChangeSetId || sequence)}`,
        candidateGrantId: `recorded-grant-${String(changeSet.id || candidate.currentChangeSetId || sequence)}`,
        applyApprovalId: `recorded-approval-${String(changeSet.id || candidate.currentChangeSetId || sequence)}`,
        reason: "用户要求应用已审阅的录制模型候选修改",
      });
    }
    return workflowPreviewStepDecision(body, sequence, results, scope, workflowContextTool, "apply");
  }

  if (content.includes(WORKFLOW_READ_MARKER) && workflowContextTool && results.length === 0) {
    const scope = markerScope(content, WORKFLOW_READ_MARKER);
    return toolCallDecision("workflow_get_context", workflowContextTool, sequence, {
      schemaVersion: "aiops.workflow.edit.v2",
      revision: scope.revision,
      view: "summary",
      limit: 20,
    });
  }
  if (content.includes(WORKFLOW_READ_MARKER) && results.some((result) => result?.structuredContent?.kind === "context")) {
    return {
      kind: "workflow_read_final",
      content: "RECORDED_WORKFLOW_MCP_READ_OK：已通过真实 RuntimeKernel 工具回合读取 Workflow 上下文。",
      toolCalls: [],
      finishReason: "stop",
    };
  }
  return {
    kind: content.includes(FINAL_ONLY_MARKER) ? "final_only" : "default_final",
    content: "RECORDED_WORKFLOW_MODEL_OK：真实 RuntimeKernel 已完成录制模型回合。",
    toolCalls: [],
    finishReason: "stop",
  };
}

function workflowPreviewStepDecision(body, sequence, results, scope, workflowContextTool, journey) {
  const contextResult = results.find((result) => result?.structuredContent?.kind === "context")?.structuredContent;
  if (contextResult) {
    const previewTool = workflowToolName(body, "workflow.preview_patch");
    if (!previewTool) return missingWorkflowToolDecision("workflow.preview_patch");
    const revision = contextResult.revision || {};
    const operation = scope.operation === "delete"
      ? { op: "delete_node", nodeId: scope.nodeId, cascadeEdges: true }
      : scope.operation === "threshold"
        ? {
            op: "update_node",
            nodeId: scope.nodeId,
            set: {
              inputs: [{
                key: "threshold",
                type: "integer",
                label: "threshold",
                required: true,
                value_source: { type: "literal", value: scope.threshold },
              }],
            },
          }
        : scope.operation === "invalid"
          ? { op: "update_node", nodeId: "", set: { label: scope.label } }
        : { op: "update_node", nodeId: scope.nodeId, set: { label: scope.label } };
    return toolCallDecision("workflow_preview", previewTool, sequence, {
      schemaVersion: "aiops.workflow.edit.v2",
      workflowKey: { tenantId: scope.tenantId, workflowId: scope.workflowId },
      expected: {
        resourceVersion: String(revision.resourceVersion || scope.revision),
        semanticHash: String(revision.semanticHash || scope.semanticHash),
        layoutHash: String(revision.layoutHash || scope.layoutHash),
      },
      validationBasis: {
        resourceVersion: String(revision.resourceVersion || scope.revision),
        semanticHash: String(revision.semanticHash || scope.semanticHash),
        catalogVersion: "recorded-model-server-owned",
        actionSchemaDigest: "recorded-model-server-owned",
        referenceSnapshotHash: "recorded-model-server-owned",
        environmentBindingVersion: scope.environmentBindingVersion,
      },
      intent: scope.operation === "delete"
        ? `删除节点 ${scope.nodeId}`
        : scope.operation === "threshold"
          ? `仅更新节点 ${scope.nodeId} 的 threshold`
          : scope.operation === "invalid"
            ? "触发录制模型的类型化 Preview 校验失败"
          : `更新节点 ${scope.nodeId} 的名称`,
      operations: [operation],
    });
  }
  if (!workflowContextTool) return missingWorkflowToolDecision("workflow.get_context");
  return toolCallDecision(`workflow_get_context_for_${journey}`, workflowContextTool, sequence, {
    schemaVersion: "aiops.workflow.edit.v2",
    revision: scope.revision,
    view: "summary",
    limit: 20,
  });
}

function workflowToolName(body, canonicalName) {
  const expected = canonicalName.replaceAll(/[._-]/g, "").toLowerCase();
  return (Array.isArray(body.tools) ? body.tools : [])
    .map((tool) => tool?.function?.name)
    .find((name) => typeof name === "string" && name.replaceAll(/[._-]/g, "").toLowerCase().includes(expected));
}

function toolCallDecision(kind, toolName, sequence, args) {
  return {
    kind,
    content: "",
    toolCalls: [{
      id: `recorded-workflow-call-${sequence}`,
      type: "function",
      function: { name: toolName, arguments: JSON.stringify(args) },
    }],
    finishReason: "tool_calls",
  };
}

function missingWorkflowToolDecision(toolName) {
  return {
    kind: "workflow_tool_missing",
    content: `RECORDED_WORKFLOW_MCP_APPLY_ERROR：缺少 ${toolName}。`,
    toolCalls: [],
    finishReason: "stop",
  };
}

function markerScope(content, marker) {
  const line = content.split(/\r?\n/).filter((value) => value.includes(marker)).at(-1) || "";
  const values = Object.fromEntries([...line.matchAll(/([a-zA-Z][a-zA-Z0-9_]*)=([^\s]+)/g)].map((match) => [match[1], match[2]]));
  return {
    tenantId: values.tenant || "runner-default",
    workflowId: values.workflow || "missing-workflow",
    revision: values.revision || "missing-revision",
    semanticHash: values.semantic || "missing-semantic-hash",
    layoutHash: values.layout || "missing-layout-hash",
    environmentBindingVersion: values.environment || "missing-environment-binding",
    nodeId: values.node || "action",
    label: decodeMarkerValue(values.label_b64) || values.label || "AI 已审批修改",
    operation: ["delete", "threshold", "invalid"].includes(values.operation) ? values.operation : "update",
    threshold: finiteMarkerNumber(values.threshold, 90),
  };
}

function finiteMarkerNumber(value, fallback) {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : fallback;
}

function decodeMarkerValue(value) {
  if (!value) return "";
  try {
    return Buffer.from(value, "base64url").toString("utf8");
  } catch {
    return "";
  }
}

function recordedToolResults(messages) {
  return messages
    .filter((message) => message?.role === "tool")
    .map((message) => parseRecordedToolResult(message?.content))
    .filter(Boolean);
}

function parseRecordedToolResult(content) {
  if (typeof content !== "string" || content.trim() === "") return null;
  try {
    const parsed = JSON.parse(content);
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return null;
    const typed = parsed.structuredContent && typeof parsed.structuredContent === "object"
      ? parsed.structuredContent
      : null;
    const envelope = typed?.structuredContent && typeof typed.structuredContent === "object" ? typed : parsed;
    return {
      summary: String(parsed.summary || envelope?.content?.[0]?.text || ""),
      isError: parsed.isError === true || envelope?.isError === true,
      error: String(parsed.error || ""),
      structuredContent: envelope?.structuredContent || null,
    };
  } catch {
    return null;
  }
}

function completionPayload(decision, sequence, model) {
  return {
    id: `chatcmpl-recorded-${sequence}`,
    object: "chat.completion",
    created: 0,
    model,
    choices: [{
      index: 0,
      message: {
        role: "assistant",
        content: decision.content,
        ...(decision.toolCalls.length ? { tool_calls: decision.toolCalls } : {}),
      },
      finish_reason: decision.finishReason,
    }],
    usage: { prompt_tokens: 16, completion_tokens: 8, total_tokens: 24 },
  };
}

function streamCompletion(response, decision, sequence, model) {
  response.writeHead(200, {
    "Content-Type": "text/event-stream; charset=utf-8",
    "Cache-Control": "no-cache",
    Connection: "keep-alive",
  });
  const id = `chatcmpl-recorded-${sequence}`;
  const delta = {
    role: "assistant",
    ...(decision.content ? { content: decision.content } : {}),
    ...(decision.toolCalls.length ? { tool_calls: decision.toolCalls.map((call, index) => ({ index, ...call })) } : {}),
  };
  response.write(`data: ${JSON.stringify({ id, object: "chat.completion.chunk", created: 0, model, choices: [{ index: 0, delta, finish_reason: null }] })}\n\n`);
  response.write(`data: ${JSON.stringify({ id, object: "chat.completion.chunk", created: 0, model, choices: [{ index: 0, delta: {}, finish_reason: decision.finishReason }] })}\n\n`);
  response.end("data: [DONE]\n\n");
}

function redactedRequestRecord(body, request, sequence, decision) {
  return {
    sequence,
    decision,
    model: String(body.model || ""),
    stream: body.stream === true,
    messageRoles: (Array.isArray(body.messages) ? body.messages : []).map((message) => String(message?.role || "")),
    toolNames: (Array.isArray(body.tools) ? body.tools : []).map((tool) => String(tool?.function?.name || "")).filter(Boolean),
    hasAuthorization: Boolean(request.headers.authorization),
  };
}

async function readJSONBody(request) {
  const chunks = [];
  let size = 0;
  for await (const chunk of request) {
    size += chunk.length;
    if (size > MAX_REQUEST_BYTES) throw new Error("recorded model request exceeds size limit");
    chunks.push(chunk);
  }
  const text = Buffer.concat(chunks).toString("utf8");
  const body = JSON.parse(text || "{}");
  if (!body || typeof body !== "object" || Array.isArray(body)) throw new Error("recorded model request must be an object");
  return body;
}

function json(response, status, payload) {
  response.writeHead(status, { "Content-Type": "application/json; charset=utf-8" });
  response.end(JSON.stringify(payload));
}

function cliOptions(argv) {
  const options = {};
  for (let index = 0; index < argv.length; index += 1) {
    const value = argv[index];
    if (value === "--host") options.host = argv[++index];
    else if (value === "--port") options.port = Number.parseInt(argv[++index], 10);
    else if (value === "--record") options.recordPath = argv[++index];
    else throw new Error(`unknown argument: ${value}`);
  }
  return options;
}

const invokedAsScript = process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url);
if (invokedAsScript) {
  const options = cliOptions(process.argv.slice(2));
  const started = await startRecordedWorkflowModelServer(options);
  process.stdout.write(`recorded workflow model listening at ${started.baseURL}\n`);
  const stop = async () => {
    await started.close();
    process.exit(0);
  };
  process.once("SIGINT", stop);
  process.once("SIGTERM", stop);
}
