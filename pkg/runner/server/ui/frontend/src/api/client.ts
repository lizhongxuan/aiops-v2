import { mockActions, mockGraph, mockRunEvents, mockWorkflows } from "../fixtures/mockGraph";
import type {
  ActionSpec,
  CompiledWorkflowResult,
  CreatedGraphWorkflowResult,
  CreateGraphWorkflowRequest,
  DryRunResult,
  RunEvent,
  RunResponse,
  ValidationResult,
  WorkflowBundle,
  WorkflowGraph,
  WorkflowSummary,
  WorkflowVersion,
} from "../types/workflow";

// Legacy/debug standalone runner server UI only.
// Main AIOps Runner Studio must use web/src/api/runnerStudioClient.js
// and same-origin /api/runner-studio/* APIs.
const API_BASE = import.meta.env.VITE_RUNNER_API_BASE || "/api/v1";

async function requestJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE}${path}`, {
    headers: {
      "Content-Type": "application/json",
      ...init?.headers,
    },
    ...init,
  });

  if (!response.ok) {
    const detail = await response.text();
    throw new Error(detail || `Request failed with status ${response.status}`);
  }

  return response.json() as Promise<T>;
}

export const runnerApi = {
  async getGraph(name: string): Promise<WorkflowGraph> {
    return requestJSON<WorkflowGraph>(`/workflows/${encodeURIComponent(name)}/graph`);
  },

  async saveGraph(name: string, graph: WorkflowGraph, saveNote?: string) {
    const note = saveNote?.trim();
    return requestJSON<CompiledWorkflowResult>(`/workflows/${encodeURIComponent(name)}/graph`, {
      method: "PUT",
      body: JSON.stringify(note ? { graph, save_note: note } : { graph }),
    });
  },

  async createGraphWorkflow(request: CreateGraphWorkflowRequest): Promise<CreatedGraphWorkflowResult> {
    return requestJSON<CreatedGraphWorkflowResult>("/workflows/graph", {
      method: "POST",
      body: JSON.stringify(request),
    });
  },

  async publishWorkflow(name: string, options?: { saveNote?: string; riskAcknowledged?: boolean; warningAcknowledged?: boolean }): Promise<WorkflowSummary> {
    const note = options?.saveNote?.trim();
    return requestJSON<WorkflowSummary>(`/workflows/${encodeURIComponent(name)}/publish`, {
      method: "POST",
      body: JSON.stringify({
        ...(note ? { save_note: note } : {}),
        ...(options?.riskAcknowledged ? { risk_acknowledged: true } : {}),
        ...(options?.warningAcknowledged ? { warning_acknowledged: true } : {}),
      }),
    });
  },

  async listWorkflowVersions(name: string): Promise<WorkflowVersion[]> {
    const payload = await requestJSON<{ items: WorkflowVersion[] }>(`/workflows/${encodeURIComponent(name)}/versions`);
    return payload.items;
  },

  async rollbackWorkflowVersion(name: string, versionId: string, saveNote?: string): Promise<WorkflowSummary & { yaml?: string }> {
    const note = saveNote?.trim();
    return requestJSON<WorkflowSummary & { yaml?: string }>(`/workflows/${encodeURIComponent(name)}/versions/${encodeURIComponent(versionId)}/rollback`, {
      method: "POST",
      body: JSON.stringify(note ? { save_note: note } : {}),
    });
  },

  async exportWorkflowBundle(name: string): Promise<WorkflowBundle> {
    return requestJSON<WorkflowBundle>(`/workflows/${encodeURIComponent(name)}/bundle`);
  },

  async importWorkflowBundle(bundle: WorkflowBundle, options?: { overwrite?: boolean; saveNote?: string }): Promise<WorkflowSummary & { yaml?: string }> {
    const note = options?.saveNote?.trim();
    return requestJSON<WorkflowSummary & { yaml?: string }>("/workflows/bundles/import", {
      method: "POST",
      body: JSON.stringify({
        bundle,
        ...(options?.overwrite ? { overwrite: true } : {}),
        ...(note ? { save_note: note } : {}),
      }),
    });
  },

  async compileGraph(graph: WorkflowGraph): Promise<CompiledWorkflowResult> {
    return requestJSON<CompiledWorkflowResult>("/workflows/graph/compile", {
      method: "POST",
      body: JSON.stringify({ graph }),
    });
  },

  async parseGraphYAML(yaml: string): Promise<WorkflowGraph> {
    return requestJSON<WorkflowGraph>("/workflows/graph/parse", {
      method: "POST",
      body: JSON.stringify({ yaml }),
    });
  },

  async validateGraph(graph: WorkflowGraph): Promise<ValidationResult> {
    return requestJSON<ValidationResult>("/workflows/graph/validate", {
      method: "POST",
      body: JSON.stringify({ graph }),
    });
  },

  async dryRunGraph(graph: WorkflowGraph): Promise<DryRunResult> {
    return requestJSON<DryRunResult>("/workflows/graph/dry-run", {
      method: "POST",
      body: JSON.stringify({ graph, vars: {}, triggered_by: "ui" }),
    });
  },

  async submitGraphRun(graph: WorkflowGraph, options?: { riskAcknowledged?: boolean }): Promise<RunResponse> {
    return requestJSON<RunResponse>("/workflows/graph/runs", {
      method: "POST",
      body: JSON.stringify({
        graph,
        vars: {},
        triggered_by: "ui",
        ...(options?.riskAcknowledged ? { risk_acknowledged: true } : {}),
      }),
    });
  },

  async getRunGraph(runId: string): Promise<WorkflowGraph> {
    return requestJSON<WorkflowGraph>(`/runs/${encodeURIComponent(runId)}/graph`);
  },

  async getRunEventHistory(runId: string): Promise<RunEvent[]> {
    return requestJSON<RunEvent[]>(`/runs/${encodeURIComponent(runId)}/events/history`);
  },

  async cancelRun(runId: string): Promise<{ run_id: string; status: string }> {
    return requestJSON<{ run_id: string; status: string }>(`/runs/${encodeURIComponent(runId)}/cancel`, {
      method: "POST",
    });
  },

  async approveNode(runId: string, nodeId: string, options?: { actor?: string; comment?: string }): Promise<{ run_id: string; node_id: string; status: string }> {
    return requestJSON<{ run_id: string; node_id: string; status: string }>(`/runs/${encodeURIComponent(runId)}/nodes/${encodeURIComponent(nodeId)}/approve`, {
      method: "POST",
      body: JSON.stringify({
        actor: options?.actor || "ui",
        comment: options?.comment?.trim() || "",
      }),
    });
  },

  async rejectNode(runId: string, nodeId: string, options?: { actor?: string; comment?: string }): Promise<{ run_id: string; node_id: string; status: string }> {
    return requestJSON<{ run_id: string; node_id: string; status: string }>(`/runs/${encodeURIComponent(runId)}/nodes/${encodeURIComponent(nodeId)}/reject`, {
      method: "POST",
      body: JSON.stringify({
        actor: options?.actor || "ui",
        comment: options?.comment?.trim() || "",
      }),
    });
  },

  subscribeRunEvents(runId: string, onEvent: (event: RunEvent) => void, onError?: (error: Event) => void): () => void {
    const source = new EventSource(`${API_BASE}/runs/${encodeURIComponent(runId)}/events`);
    source.onmessage = (message) => {
      try {
        onEvent(JSON.parse(message.data) as RunEvent);
      } catch {
        onEvent({
          type: "client_event_parse_failed",
          run_id: runId,
          status: "failed",
          message: "Unable to parse SSE event payload.",
          timestamp: new Date().toISOString(),
        });
      }
    };
    source.onerror = (event) => {
      onError?.(event);
    };
    return () => source.close();
  },

  async listActions(): Promise<ActionSpec[]> {
    const payload = await requestJSON<{ items: ActionSpec[] }>("/actions/catalog");
    return payload.items;
  },

  async listWorkflows(limit = 200): Promise<WorkflowSummary[]> {
    const payload = await requestJSON<{ items: WorkflowSummary[] }>(`/workflows?limit=${encodeURIComponent(String(limit))}`);
    return payload.items;
  },
};

export const mockApi = {
  async getGraph(): Promise<WorkflowGraph> {
    return structuredClone(mockGraph);
  },
  async listActions(): Promise<ActionSpec[]> {
    return structuredClone(mockActions);
  },
  async listWorkflows(): Promise<WorkflowSummary[]> {
    return structuredClone(mockWorkflows);
  },
  async createGraphWorkflow(request: CreateGraphWorkflowRequest): Promise<CreatedGraphWorkflowResult> {
    return {
      name: request.graph.workflow.name,
      status: "draft",
      workflow: structuredClone(request.graph.workflow),
      graph: structuredClone(request.graph),
      yaml: `version: ${request.graph.workflow.version || "v0.1"}\nname: ${request.graph.workflow.name}\n`,
    };
  },
  async parseGraphYAML(_yaml: string): Promise<WorkflowGraph> {
    return structuredClone(mockGraph);
  },
  async getRunEventHistory(_runId: string): Promise<RunEvent[]> {
    return structuredClone(mockRunEvents);
  },
};
