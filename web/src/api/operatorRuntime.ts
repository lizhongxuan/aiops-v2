import httpClient from "./httpClient";

export type OperatorRuntimeItem = Record<string, unknown>;

export type ResourceEndpoint = {
  id: string;
  role: "primary" | "replica" | string;
  host: string;
  port: number;
  serviceName?: string;
  labels?: Record<string, string>;
};

export type ManagedResource = OperatorRuntimeItem & {
  id: string;
  name: string;
  kind: string;
  provider?: string;
  environment?: string;
  endpoints: ResourceEndpoint[];
  credentialRefs?: { monitor?: string; repair?: string };
  primary?: ResourceEndpoint;
  replicas?: ResourceEndpoint[];
  monitorCredentialRef?: string;
  repairCredentialRef?: string;
  tags?: string[];
  labels?: Record<string, string>;
};

export type PGInstance = ResourceEndpoint;
export type PGCluster = ManagedResource;

export type InspectionTemplate = OperatorRuntimeItem & {
  id: string;
  name: string;
  objectKind: "postgres_replication" | string;
  intervalSeconds: number;
  primarySql: string;
  replicaSql: string;
  outputFields: Array<{ name: string; type: string }>;
};

export type ProblemType = OperatorRuntimeItem & {
  id: string;
  displayName: string;
  severity: "warning" | "critical" | string;
  conditions: Array<Record<string, unknown>>;
  forSeconds: number;
  autoRepairAllowed: boolean;
  recommendedActionRefs: string[];
};

export type ActionCatalogItem = OperatorRuntimeItem & {
  id: string;
  displayName: string;
  riskLevel: "readonly" | "low" | "medium" | "high" | "critical" | string;
  targetKind: "postgres_replica" | string;
  inputSchema?: Record<string, unknown>;
  confirmationRequiredSteps?: string[];
};

export type WorkflowBinding = OperatorRuntimeItem & {
  id: string;
  actionRef: string;
  workflowRef: string;
  workflowVersion?: string;
  capabilities: string[];
  inputMapping: Record<string, string>;
  verifyPolicy: Record<string, unknown>;
};

export type GuardRule = OperatorRuntimeItem & {
  id: string;
  name: string;
  resourceRef?: string;
  clusterRef?: string;
  templateRef: string;
  problemTypeRefs: string[];
  actionRefs: string[];
  workflowBindingRefs: string[];
  scheduleSeconds: number;
  cooldownSeconds?: number;
  maxConcurrency?: number;
  enabled?: boolean;
  status?: string;
  policy?: Record<string, unknown>;
};

export type GuardRun = OperatorRuntimeItem & {
  id: string;
  state?: string;
  status?: string;
  ruleName?: string;
  resourceName?: string;
  resourceId?: string;
  clusterName?: string;
  problemType?: string;
  evidence?: unknown;
};

export type OperatorRuntimeFieldError = {
  field: string;
  message: string;
};

export class OperatorRuntimeError extends Error {
  status: number;
  fieldErrors: OperatorRuntimeFieldError[];
  payload: unknown;

  constructor(
    message: string,
    {
      status = 0,
      fieldErrors = [],
      payload = null,
    }: { status?: number; fieldErrors?: OperatorRuntimeFieldError[]; payload?: unknown } = {},
  ) {
    super(message);
    this.name = "OperatorRuntimeError";
    this.status = status;
    this.fieldErrors = fieldErrors;
    this.payload = payload;
  }
}

export type OperatorRuntimeListResponse<T extends OperatorRuntimeItem = OperatorRuntimeItem> = {
  items: T[];
};

export type OperatorRuntimeItemResponse<T extends OperatorRuntimeItem = OperatorRuntimeItem> = {
  item: T;
};

type OperatorRuntimeHttpClient = {
  get(path: string): Promise<unknown>;
  post(path: string, body?: unknown): Promise<unknown>;
};

export type PollRunOptions<T extends GuardRun = GuardRun> = {
  intervalMs?: number;
  maxAttempts?: number;
  signal?: AbortSignal;
  shouldStop?: (run: T) => boolean;
};

const endpoints = {
  resources: "/api/v1/guards/resources",
  pgClusters: "/api/v1/guards/pg/clusters",
  inspectionTemplates: "/api/v1/guards/inspection-templates",
  problemTypes: "/api/v1/guards/problem-types",
  actions: "/api/v1/guards/actions",
  workflowBindings: "/api/v1/guards/workflow-bindings",
  rules: "/api/v1/guards/rules",
  runs: "/api/v1/guards/runs",
} as const;

export function createOperatorRuntimeClient(client: OperatorRuntimeHttpClient = httpClient) {
  const list = async <T extends OperatorRuntimeItem>(path: string): Promise<OperatorRuntimeListResponse<T>> => {
    return request(() => client.get(path), (payload) => normalizeListResponse<T>(payload));
  };
  const create = async <T extends OperatorRuntimeItem>(path: string, body: unknown): Promise<OperatorRuntimeItemResponse<T>> => {
    return request(() => client.post(path, body), (payload) => normalizeItemResponse<T>(payload));
  };
  const postItem = async <T extends OperatorRuntimeItem>(path: string): Promise<OperatorRuntimeItemResponse<T>> => {
    return request(() => client.post(path), (payload) => normalizeItemResponse<T>(payload));
  };

  return {
    listResources: <T extends ManagedResource = ManagedResource>() => list<T>(endpoints.resources),
    createResource: <T extends ManagedResource = ManagedResource>(body: unknown) => create<T>(endpoints.resources, body),
    listPgClusters: <T extends PGCluster = PGCluster>() => list<T>(endpoints.pgClusters),
    createPgCluster: <T extends PGCluster = PGCluster>(body: unknown) => create<T>(endpoints.pgClusters, body),
    listInspectionTemplates: <T extends InspectionTemplate = InspectionTemplate>() => list<T>(endpoints.inspectionTemplates),
    createInspectionTemplate: <T extends InspectionTemplate = InspectionTemplate>(body: unknown) =>
      create<T>(endpoints.inspectionTemplates, body),
    listProblemTypes: <T extends ProblemType = ProblemType>() => list<T>(endpoints.problemTypes),
    createProblemType: <T extends ProblemType = ProblemType>(body: unknown) => create<T>(endpoints.problemTypes, body),
    listActions: <T extends ActionCatalogItem = ActionCatalogItem>() => list<T>(endpoints.actions),
    createAction: <T extends ActionCatalogItem = ActionCatalogItem>(body: unknown) => create<T>(endpoints.actions, body),
    listWorkflowBindings: <T extends WorkflowBinding = WorkflowBinding>() => list<T>(endpoints.workflowBindings),
    createWorkflowBinding: <T extends WorkflowBinding = WorkflowBinding>(body: unknown) =>
      create<T>(endpoints.workflowBindings, body),
    listRules: <T extends GuardRule = GuardRule>() => list<T>(endpoints.rules),
    createRule: <T extends GuardRule = GuardRule>(body: unknown) => create<T>(endpoints.rules, body),
    enableRule: <T extends GuardRule = GuardRule>(id: string) =>
      postItem<T>(`${endpoints.rules}/${encodeURIComponent(id)}/enable`),
    disableRule: <T extends GuardRule = GuardRule>(id: string) =>
      postItem<T>(`${endpoints.rules}/${encodeURIComponent(id)}/disable`),
    listRuns: <T extends GuardRun = GuardRun>() => list<T>(endpoints.runs),
    getRun: <T extends GuardRun = GuardRun>(id: string) =>
      request(() => client.get(`${endpoints.runs}/${encodeURIComponent(id)}`), (payload) => normalizeItemResponse<T>(payload)),
    pollRun: async <T extends GuardRun = GuardRun>(id: string, options: PollRunOptions<T> = {}) => {
      const intervalMs = options.intervalMs ?? 2000;
      const maxAttempts = options.maxAttempts ?? 60;
      const shouldStop = options.shouldStop ?? ((run: T) => isTerminalRun(run));
      let lastRun: T | undefined;
      for (let attempt = 0; attempt < maxAttempts; attempt += 1) {
        if (options.signal?.aborted) {
          throw new OperatorRuntimeError("GuardRun polling was aborted", { status: 0 });
        }
        const result = await request(
          () => client.get(`${endpoints.runs}/${encodeURIComponent(id)}`),
          (payload) => normalizeItemResponse<T>(payload),
        );
        lastRun = result.item;
        if (shouldStop(lastRun)) return result;
        if (attempt < maxAttempts - 1) {
          await wait(intervalMs, options.signal);
        }
      }
      return { item: lastRun ?? ({} as T) };
    },
    approveRun: <T extends GuardRun = GuardRun>(id: string) =>
      postItem<T>(`${endpoints.runs}/${encodeURIComponent(id)}/approve`),
    rejectRun: <T extends GuardRun = GuardRun>(id: string) =>
      postItem<T>(`${endpoints.runs}/${encodeURIComponent(id)}/reject`),
  };
}

async function request<T>(operation: () => Promise<unknown>, normalize: (payload: unknown) => T | Promise<T>): Promise<T> {
  try {
    return await normalize(await operation());
  } catch (error) {
    throw normalizeOperatorRuntimeError(error);
  }
}

export function normalizeOperatorRuntimeError(error: unknown) {
  if (error instanceof OperatorRuntimeError) return error;
  if (isRecord(error)) {
    const payload = error.payload;
    const payloadRecord = isRecord(payload) ? payload : undefined;
    const fieldErrors = normalizeFieldErrors(payloadRecord?.fieldErrors ?? payloadRecord?.errors);
    const message =
      stringValue(payloadRecord?.message) ||
      stringValue(payloadRecord?.error) ||
      stringValue(error.message) ||
      "Operator Runtime request failed";
    return new OperatorRuntimeError(message, {
      status: typeof error.status === "number" ? error.status : 0,
      fieldErrors,
      payload,
    });
  }
  return new OperatorRuntimeError(error instanceof Error ? error.message : String(error || "Operator Runtime request failed"));
}

export function normalizeListResponse<T extends OperatorRuntimeItem>(payload: unknown): OperatorRuntimeListResponse<T> {
  if (isRecord(payload) && Array.isArray(payload.items)) {
    return { items: payload.items.filter(isRecord) as T[] };
  }
  return { items: [] };
}

export async function normalizeItemResponse<T extends OperatorRuntimeItem>(
  payloadOrPromise: unknown | Promise<unknown>,
): Promise<OperatorRuntimeItemResponse<T>> {
  const payload = await payloadOrPromise;
  if (isRecord(payload) && isRecord(payload.item)) {
    return { item: payload.item as T };
  }
  return { item: {} as T };
}

function isRecord(value: unknown): value is OperatorRuntimeItem {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

function stringValue(value: unknown) {
  return typeof value === "string" && value ? value : "";
}

function normalizeFieldErrors(value: unknown): OperatorRuntimeFieldError[] {
  if (!Array.isArray(value)) return [];
  return value
    .map((item) => {
      if (!isRecord(item)) return undefined;
      const field = stringValue(item.field ?? item.path ?? item.name);
      const message = stringValue(item.message ?? item.error);
      return field && message ? { field, message } : undefined;
    })
    .filter((item): item is OperatorRuntimeFieldError => Boolean(item));
}

function isTerminalRun(run: GuardRun) {
  const status = String(run.status ?? run.state ?? "").toLowerCase();
  return ["succeeded", "failed", "blocked", "approval_rejected", "rejected", "approved", "cancelled", "canceled"].includes(status);
}

async function wait(ms: number, signal?: AbortSignal) {
  if (ms <= 0) return;
  await new Promise<void>((resolve, reject) => {
    const timeout = window.setTimeout(resolve, ms);
    signal?.addEventListener(
      "abort",
      () => {
        window.clearTimeout(timeout);
        reject(new OperatorRuntimeError("GuardRun polling was aborted", { status: 0 }));
      },
      { once: true },
    );
  });
}

const operatorRuntimeClient = createOperatorRuntimeClient();

export const listResources = operatorRuntimeClient.listResources;
export const createResource = operatorRuntimeClient.createResource;
export const listPgClusters = operatorRuntimeClient.listPgClusters;
export const createPgCluster = operatorRuntimeClient.createPgCluster;
export const listInspectionTemplates = operatorRuntimeClient.listInspectionTemplates;
export const createInspectionTemplate = operatorRuntimeClient.createInspectionTemplate;
export const listProblemTypes = operatorRuntimeClient.listProblemTypes;
export const createProblemType = operatorRuntimeClient.createProblemType;
export const listActions = operatorRuntimeClient.listActions;
export const createAction = operatorRuntimeClient.createAction;
export const listWorkflowBindings = operatorRuntimeClient.listWorkflowBindings;
export const createWorkflowBinding = operatorRuntimeClient.createWorkflowBinding;
export const listRules = operatorRuntimeClient.listRules;
export const createRule = operatorRuntimeClient.createRule;
export const enableRule = operatorRuntimeClient.enableRule;
export const disableRule = operatorRuntimeClient.disableRule;
export const listRuns = operatorRuntimeClient.listRuns;
export const getRun = operatorRuntimeClient.getRun;
export const pollRun = operatorRuntimeClient.pollRun;
export const approveRun = operatorRuntimeClient.approveRun;
export const rejectRun = operatorRuntimeClient.rejectRun;
