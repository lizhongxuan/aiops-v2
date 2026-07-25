import ELK from "elkjs/lib/elk.bundled.js";
import type { ElkNode } from "elkjs/lib/elk-api.js";

export type WorkflowLayoutWorkerRequest = { requestId: string; graph: ElkNode };
export type WorkflowLayoutWorkerResponse = { requestId: string; graph?: ElkNode; error?: string };

export async function runWorkflowElkLayout(graph: ElkNode): Promise<ElkNode> {
  const elk = new ELK({ algorithms: ["layered"] });
  try {
    return await elk.layout(graph);
  } finally {
    elk.terminateWorker();
  }
}

const scope = globalThis as typeof globalThis & {
  document?: unknown;
  postMessage?: (message: WorkflowLayoutWorkerResponse) => void;
  addEventListener?: (type: string, listener: (event: MessageEvent<WorkflowLayoutWorkerRequest>) => void) => void;
};

if (scope.document === undefined && typeof scope.postMessage === "function" && typeof scope.addEventListener === "function") {
  scope.addEventListener("message", (event) => {
    const request = event.data;
    if (!request?.requestId || !request.graph) return;
    void runWorkflowElkLayout(request.graph)
      .then((graph) => scope.postMessage?.({ requestId: request.requestId, graph }))
      .catch((error) => scope.postMessage?.({ requestId: request.requestId, error: error instanceof Error ? error.message : "ELK layout failed" }));
  });
}
