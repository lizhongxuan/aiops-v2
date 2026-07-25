import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";
import {
  WORKFLOW_CONTRACT_SCHEMA_VERSION,
  WorkflowContractParseError,
  parseWorkflowPatchRequestV2,
  parseWorkflowToolResultV2,
  serializeWorkflowPatchRequestV2,
} from "./workflowMcpContracts";

const requestFixture = {
  schemaVersion: WORKFLOW_CONTRACT_SCHEMA_VERSION,
  workflowKey: { tenantId: "tenant-a", workflowId: "disk-guard" },
  expected: {
    resourceVersion: "rv-12",
    semanticHash: "sha256:semantic",
    layoutHash: "sha256:layout",
  },
  validationBasis: {
    resourceVersion: "rv-12",
    semanticHash: "sha256:semantic",
    catalogVersion: "catalog-7",
    actionSchemaDigest: "sha256:catalog-schema",
    referenceSnapshotHash: "sha256:refs",
    environmentBindingVersion: "env-4",
  },
  intent: "新增检查步骤",
  operations: [
    {
      op: "add_node",
      clientRef: "check",
      node: {
        type: "action",
        position: { x: 0, y: 0 },
        stepName: "check_capacity",
        label: "检查容量",
        inputs: [{ key: "credential", value_source: { type: "secret_ref", secret_ref: "secret://runner/shell" } }],
      },
    },
    {
      op: "update_layout",
      layout: {
        nodes: [{ nodeId: "start", position: { x: 120, y: 240 } }],
        viewport: { x: 0, y: 0, zoom: 1 },
      },
    },
  ],
} as const;

describe("workflow MCP v2 frontend contract", () => {
  it("round-trips the Go-first golden fixture as the single editing source", () => {
    const golden = readFileSync(
      resolve(process.cwd(), "../pkg/runner/workflow/edit/testdata/preview_patch_v2.golden.json"),
      "utf8",
    );
    expect(JSON.parse(serializeWorkflowPatchRequestV2(parseWorkflowPatchRequestV2(golden)))).toEqual(JSON.parse(golden));
  });

  it("normalizes and round-trips the canonical request without losing fields", () => {
    const parsed = parseWorkflowPatchRequestV2(JSON.stringify(requestFixture));
    expect(parsed).toEqual(requestFixture);
    expect(parseWorkflowPatchRequestV2(serializeWorkflowPatchRequestV2(parsed))).toEqual(parsed);
  });

  it("accepts exactly the nine frozen operation variants", () => {
    const operations = [
      requestFixture.operations[0],
      { op: "update_node", nodeId: "check", set: { label: "检查根分区" } },
      { op: "delete_node", nodeId: "old", cascadeEdges: true },
      { op: "add_edge", clientRef: "check_to_end", edge: { source: "check", sourcePort: "next", target: "end", targetPort: "in", kind: "next" } },
      { op: "update_edge", edgeId: "edge-1", set: { targetPort: "success" } },
      { op: "delete_edge", edgeId: "edge-old" },
      { op: "update_workflow", set: { name: "Capacity guard" } },
      requestFixture.operations[1],
      { op: "attach_manual_binding", manualRef: { manualId: "manual-1", candidateId: "candidate-1" }, workflowDigest: "sha256:manual" },
    ];
    expect(parseWorkflowPatchRequestV2({ ...requestFixture, operations }).operations.map((item) => item.op)).toEqual([
      "add_node", "update_node", "delete_node", "add_edge", "update_edge", "delete_edge", "update_workflow", "update_layout", "attach_manual_binding",
    ]);
  });

  it("rejects unknown operation kinds and fields instead of silently degrading", () => {
    expect(() =>
      parseWorkflowPatchRequestV2({
        ...requestFixture,
        operations: [{ op: "replace_full_graph", graph: {} }],
      }),
    ).toThrowError(expect.objectContaining({ code: "UNKNOWN_OPERATION", compatibility: "refresh_required" }));

    expect(() =>
      parseWorkflowPatchRequestV2({
        ...requestFixture,
        debug: true,
      }),
    ).toThrowError(expect.objectContaining({ code: "UNKNOWN_FIELD" }));

    expect(() =>
      parseWorkflowPatchRequestV2({
        ...requestFixture,
        operations: [{ ...requestFixture.operations[0], node: { ...requestFixture.operations[0].node, ui: {} } }],
      }),
    ).toThrowError(expect.objectContaining({ code: "UNKNOWN_FIELD" }));
  });

  it("rejects unsupported schema versions and inline credential fields", () => {
    expect(() =>
      parseWorkflowPatchRequestV2({ ...requestFixture, schemaVersion: "aiops.workflow.edit.v3" }),
    ).toThrowError(expect.objectContaining({ code: "UNSUPPORTED_SCHEMA_VERSION" }));

    expect(() =>
      parseWorkflowPatchRequestV2({
        ...requestFixture,
        operations: [
          {
            op: "add_node",
            clientRef: "unsafe",
            node: {
              type: "action",
              position: { x: 0, y: 0 },
              label: "Unsafe",
              inputs: [{ key: "credential", default: { password: "plaintext" } }],
            },
          },
        ],
      }),
    ).toThrowError(expect.objectContaining({ code: "INLINE_SECRET_FORBIDDEN" }));

    expect(() =>
      parseWorkflowPatchRequestV2({
        ...requestFixture,
        workflowKey: { tenantId: "tenant-a", workflowId: "../other-tenant/workflow" },
      }),
    ).toThrowError(expect.objectContaining({ code: "INVALID_TYPE" }));
  });

  it("requires structured tool results and refuses the old fake ok envelope", () => {
    expect(() => parseWorkflowToolResultV2({ status: "ok" })).toThrowError(WorkflowContractParseError);

    expect(
      parseWorkflowToolResultV2({
        schemaVersion: WORKFLOW_CONTRACT_SCHEMA_VERSION,
        isError: true,
        content: [{ type: "text", text: "Workflow 在预览后已被修改" }],
        structuredContent: {
          kind: "error",
          errorCode: "STALE_REVISION",
          message: "Workflow 在预览后已被修改",
          retryHint: "重新读取 context",
        },
      }),
    ).toMatchObject({ isError: true, structuredContent: { kind: "error", errorCode: "STALE_REVISION" } });
  });

  it("parses a typed receipt whose validation is bound to the full basis", () => {
    expect(
      parseWorkflowToolResultV2({
        schemaVersion: WORKFLOW_CONTRACT_SCHEMA_VERSION,
        isError: false,
        content: [{ type: "text", text: "已保存：新增 1 个步骤" }],
        structuredContent: {
          schemaVersion: WORKFLOW_CONTRACT_SCHEMA_VERSION,
          kind: "receipt",
          transactionId: "txn-1",
          changeSetId: "change-1",
          workflowKey: requestFixture.workflowKey,
          status: "applied",
          revisionBefore: requestFixture.expected,
          revisionAfter: { ...requestFixture.expected, resourceVersion: "rv-13" },
          delta: {
            addedNodeIds: ["check"], updatedNodeIds: [], deletedNodeIds: [],
            addedEdgeIds: [], updatedEdgeIds: [], deletedEdgeIds: [],
          },
          validation: { basis: { ...requestFixture.validationBasis, resourceVersion: "rv-13" }, valid: true, diagnostics: [] },
          auditId: "audit-1",
        },
      }),
    ).toMatchObject({
      isError: false,
      structuredContent: { kind: "receipt", transactionId: "txn-1", validation: { basis: { resourceVersion: "rv-13" } } },
    });
  });
});
