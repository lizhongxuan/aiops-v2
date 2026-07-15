import {
  useAssistantApi,
  ComposerPrimitive,
  useAssistantTransportSendCommand,
  useAssistantTransportState,
  useComposer,
  useComposerRuntime,
  useThread,
} from "@assistant-ui/react";
import {
  ArrowUp,
  Check,
  FileText,
  LoaderCircle,
  Square,
  Wrench,
} from "lucide-react";
import {
  type FormEvent,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";

import { listHostInventory, type HostInventoryItem } from "@/api/hostInventory";
import { opsManualsApi, type OpsManualView } from "@/api/opsManuals";
import { listOpsGraphs } from "@/api/opsgraph";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/utils";
import type { OpsGraphSummary } from "@/pages/opsgraph/opsGraphTypes";
import { isAiopsTransportRunning } from "@/transport/aiopsTransportConverter";
import { useAiopsTransportCommands } from "@/transport/useAiopsTransportCommands";
import type {
  AiopsTransportApproval,
  AiopsTransportState,
} from "@/transport/aiopsTransportTypes";

import {
  buildAiopsSpecialMentionMetadata,
  buildOpsManualParamFormSubmit,
  resolveStopDispatchTarget,
} from "./aiopsComposerActions";
import type { DisplayHostMention } from "./HostMentionChip";
import {
  HostMentionSuggestionPopover,
  type ComposerMentionSuggestion,
} from "./HostMentionSuggestionPopover";
import { useSessionTargetContext } from "./SessionTargetContext";
import { useSessionWorkspaceContext } from "./SessionWorkspaceContext";
import {
  buildHostMentionMetadata,
  parseHostMentionCandidates,
  parseSpecialAiMentionCandidates,
} from "../hostMentions";
import {
  findActiveHostMentionToken,
  replaceActiveHostMention,
  searchHostMentionSuggestions,
  type ActiveHostMentionToken,
} from "../hostMentionSearch";
import {
  buildCapabilityMentionBinding,
  buildHostMentionBinding,
  buildInputMentionMetadata,
  buildOpsGraphMentionBinding,
  buildOpsManualMentionBinding,
  deriveCapabilityMentionMetadata,
  deriveHostMentionMetadata,
  reconcileMentionBindings,
  type AiopsMentionBinding,
} from "../inputMentions";
import {
  searchCapabilityMentionSuggestions,
  searchMentionCategorySuggestions,
  searchOpsGraphMentionSuggestions,
  searchOpsManualMentionSuggestions,
  type MentionCategory,
} from "../mentionCatalog";
import { HostMentionInlineOverlay, type ResourceInlineMentionCandidate } from "./HostMentionInlineOverlay";

type GenerationConfirmation = {
  action: string;
  title: string;
  sourceTitle: string;
  artifactId?: string;
  metadata?: Record<string, string>;
};

const SUPPORTED_CONFIRMATION_ACTIONS = new Set([
  "generate_ops_manual_candidate",
  "generate_runner_workflow_candidate",
  "start_runner_workflow_dry_run",
  "confirm_runner_workflow_execution",
  "request_runner_workflow_approval",
]);
const LLM_CONFIG_REQUIRED_REASON = "请先在设置中配置 LLM";

type ContextFormField = {
  id: string;
  label: string;
  type?: string;
  required?: boolean;
  sensitive?: boolean;
  uiControl?: string;
  placeholder?: string;
  default?: unknown;
  candidates?: Array<{
    value?: unknown;
    label?: string;
    hint?: string;
    source?: string;
    confidence?: number;
    evidence?: string;
  }>;
};

type ContextFormRequest = {
  artifactId?: string;
  manualId?: string;
  workflowId?: string;
  submitAction?: string;
  key: string;
  dismissKeys?: string[];
  title: string;
  summary?: string;
  contextText?: string;
  fields: ContextFormField[];
  force?: boolean;
};

const DISMISSED_CONTEXT_REQUEST_STORAGE_PREFIX =
  "aiops:composer-context-request:dismissed:v2:";
const APPROVAL_DECISION_TIMEOUT_MS = 10_000;

export function AiopsComposer({
  className = "",
  variant = "default",
}: {
  className?: string;
  variant?: "default" | "chat";
} = {}) {
  const state = useAssistantTransportState() as AiopsTransportState;
  const threadIsRunning = useThread((snapshot) => snapshot.isRunning);
  const workspace = useSessionWorkspaceContext();
  const visibleDisabledReason =
    workspace.composerDisabledReason === LLM_CONFIG_REQUIRED_REASON
      ? ""
      : workspace.composerDisabledReason;
  const sendCommand = useAssistantTransportSendCommand();
  const target = useSessionTargetContext();
  const transportTerminal =
    state.status === "failed" || state.status === "canceled";
  const isRunning = isAiopsTransportRunning(state) || (threadIsRunning && !transportTerminal);
  const [generationConfirmation, setGenerationConfirmation] =
    useState<GenerationConfirmation | null>(null);
  const [contextRequest, setContextRequest] =
    useState<ContextFormRequest | null>(null);
  const dismissedContextRequestKeysRef = useRef(new Set<string>());

  useEffect(() => {
    function handleConfirmation(event: Event) {
      const detail =
        (event as CustomEvent<Partial<GenerationConfirmation>>).detail || {};
      const action = String(detail.action || "").trim();
      if (!SUPPORTED_CONFIRMATION_ACTIONS.has(action)) {
        return;
      }
      setGenerationConfirmation({
        action,
        title: String(detail.title || confirmationTitle(action)),
        sourceTitle: String(detail.sourceTitle || "当前对话"),
        artifactId: detail.artifactId ? String(detail.artifactId) : undefined,
        metadata:
          detail.metadata && typeof detail.metadata === "object"
            ? detail.metadata
            : undefined,
      });
    }
    window.addEventListener("aiops:composer-confirmation", handleConfirmation);
    return () =>
      window.removeEventListener(
        "aiops:composer-confirmation",
        handleConfirmation,
      );
  }, []);

  useEffect(() => {
    function handleContextSubmit(event: Event) {
      const detail =
        (
          event as CustomEvent<{
            text?: string;
            artifactId?: string;
            metadata?: Record<string, string>;
          }>
        ).detail || {};
      const text = String(detail.text || "").trim();
      if (!text) return;
      const metadata =
        detail.metadata && typeof detail.metadata === "object"
          ? detail.metadata
          : {};
      sendCommand({
        type: "add-message",
        message: {
          role: "user",
          metadata: {
            ...target.metadata,
            opsManualAction:
              metadata.opsManualAction || "submit_required_context",
            sourceArtifactId: detail.artifactId,
            ...metadata,
          },
          ...(target.hostId ? { hostId: target.hostId } : {}),
          parts: [{ type: "text", text }],
        },
      } as Parameters<typeof sendCommand>[0]);
      setContextRequest(null);
    }
    window.addEventListener(
      "aiops:composer-context-submit",
      handleContextSubmit,
    );
    return () =>
      window.removeEventListener(
        "aiops:composer-context-submit",
        handleContextSubmit,
      );
  }, [sendCommand, target.hostId, target.metadata]);

  useEffect(() => {
    function handleContextRequest(event: Event) {
      const detail =
        (event as CustomEvent<Partial<ContextFormRequest>>).detail || {};
      const fields = Array.isArray(detail.fields)
        ? detail.fields
            .map((field) => {
              const rawField = field as ContextFormField & {
                ui_control?: unknown;
              };
              return {
                id: String(rawField?.id || "").trim(),
                label: String(rawField?.label || rawField?.id || "").trim(),
                type: rawField?.type ? String(rawField.type) : "",
                required: Boolean(rawField?.required),
                sensitive: Boolean(rawField?.sensitive),
                uiControl: rawField?.uiControl
                  ? String(rawField.uiControl)
                  : rawField?.ui_control
                    ? String(rawField.ui_control)
                    : "",
                placeholder: rawField?.placeholder
                  ? String(rawField.placeholder)
                  : "",
                default: rawField?.default,
                candidates: normalizeContextCandidates(rawField?.candidates),
              };
            })
            .filter((field) => field.id && field.label)
        : [];
      if (fields.length === 0) return;
      const key = contextRequestKey(
        state.threadId || state.sessionId,
        detail.artifactId ? String(detail.artifactId) : undefined,
        fields,
      );
      const fallbackKey = contextRequestKey(
        undefined,
        detail.artifactId ? String(detail.artifactId) : undefined,
        fields,
      );
      if (
        [key, fallbackKey].some((candidate) =>
          isDismissedContextRequestKey(
            candidate,
            dismissedContextRequestKeysRef.current,
          ),
        )
      ) {
        return;
      }
      rememberContextRequestKeys(
        [key, fallbackKey],
        dismissedContextRequestKeysRef.current,
      );
      setContextRequest({
        artifactId: detail.artifactId ? String(detail.artifactId) : undefined,
        manualId: detail.manualId ? String(detail.manualId) : undefined,
        workflowId: detail.workflowId ? String(detail.workflowId) : undefined,
        submitAction: detail.submitAction
          ? String(detail.submitAction)
          : undefined,
        key,
        dismissKeys: [key, fallbackKey],
        title: String(detail.title || "补充运维信息"),
        summary: detail.summary ? String(detail.summary) : "",
        contextText: detail.contextText ? String(detail.contextText) : "",
        fields,
      });
    }
    window.addEventListener(
      "aiops:composer-context-request",
      handleContextRequest,
    );
    return () =>
      window.removeEventListener(
        "aiops:composer-context-request",
        handleContextRequest,
      );
  }, [state.sessionId, state.threadId]);

  const pendingApproval = selectComposerApproval(state);
  if (pendingApproval) {
    return <BlockedApprovalComposer approval={pendingApproval} />;
  }
  if (generationConfirmation) {
    return (
      <GenerationConfirmationComposer
        confirmation={generationConfirmation}
        variant={variant}
        className={className}
        onCancel={() => setGenerationConfirmation(null)}
        onComplete={() => setGenerationConfirmation(null)}
      />
    );
  }
  if (contextRequest) {
    return (
      <ContextRequestComposer
        request={contextRequest}
        variant={variant}
        className={className}
        onCancel={() => {
          dismissContextRequestKeys(
            contextRequest.dismissKeys || [contextRequest.key],
            dismissedContextRequestKeysRef.current,
          );
          setContextRequest(null);
        }}
        onComplete={() => {
          dismissContextRequestKeys(
            contextRequest.dismissKeys || [contextRequest.key],
            dismissedContextRequestKeysRef.current,
          );
          setContextRequest(null);
        }}
      />
    );
  }

  return (
    <div
      className={[
        variant === "chat"
          ? "shrink-0 bg-white px-4 pb-4 pt-0 md:pb-6"
          : "border-t border-zinc-200 bg-white px-4 py-3 lg:px-8",
        className,
      ]
        .filter(Boolean)
        .join(" ")}
      data-testid="aiops-composer-shell"
    >
      <div className="mx-auto flex max-w-[49.5rem] flex-col gap-2">
        <ComposerBody
          variant={variant}
          isRunning={isRunning}
          state={state}
          threadIsRunning={threadIsRunning}
        />
        {visibleDisabledReason ? (
          <div className="px-1 text-xs text-amber-700">
            {visibleDisabledReason}
          </div>
        ) : null}
      </div>
    </div>
  );
}

function ContextRequestComposer({
  request,
  variant,
  className,
  onCancel,
  onComplete,
}: {
  request: ContextFormRequest;
  variant: "default" | "chat";
  className: string;
  onCancel: () => void;
  onComplete: () => void;
}) {
  const sendCommand = useAssistantTransportSendCommand();
  const target = useSessionTargetContext();
  const formRef = useRef<HTMLFormElement | null>(null);

  function submitForm(form: HTMLFormElement | null) {
    if (!form) return;
    const formData = new FormData(form);
    const params = Object.fromEntries(
      request.fields
        .map((field) => [field.id, contextFieldSubmitValue(field, formData)])
        .filter(([, value]) => String(value || "").trim()),
    ) as Record<string, string>;
    if (Object.keys(params).length === 0) return;
    const submission =
      request.submitAction === "submit_ops_manual_param_form"
        ? buildOpsManualParamFormSubmit({
            artifactId: request.artifactId,
            manualId: request.manualId,
            workflowId: request.workflowId,
            params,
          })
        : legacyContextFormSubmit(request, params);
    sendCommand({
      type: "add-message",
      message: {
        role: "user",
        metadata: {
          ...target.metadata,
          ...submission.metadata,
        },
        ...(target.hostId ? { hostId: target.hostId } : {}),
        parts: [{ type: "text", text: submission.text }],
      },
    } as Parameters<typeof sendCommand>[0]);
    onComplete();
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    submitForm(event.currentTarget);
  }

  return (
    <div
      className={[
        variant === "chat"
          ? "shrink-0 bg-white px-4 pb-4 pt-2 md:pb-6"
          : "border-t border-zinc-200 bg-white px-4 py-3 lg:px-8",
        className,
      ]
        .filter(Boolean)
        .join(" ")}
      data-testid="ops-manual-context-composer"
    >
      <form
        ref={formRef}
        className="mx-auto max-w-3xl rounded-2xl border border-slate-200 bg-white p-3 shadow-[0_10px_28px_rgba(15,23,42,0.10)]"
        onSubmit={submit}
      >
        <div className="flex items-center gap-2">
          <span className="rounded-lg bg-slate-100 p-2 text-slate-700">
            <FileText className="h-4 w-4" />
          </span>
          <div className="min-w-0 flex-1">
            <div className="truncate text-[15px] font-semibold leading-6 text-slate-950">
              {request.title}
            </div>
          </div>
        </div>
        <div className="mt-3 grid gap-3 sm:grid-cols-2">
          {request.fields.map((field) => {
            const control = contextFieldControl(field);
            if (control === "select") {
              return (
                <div
                  key={field.id}
                  className="grid gap-2 rounded-xl border border-slate-200 bg-slate-50/70 p-3 text-sm text-slate-600"
                >
                  <label htmlFor={field.id} className="font-medium">
                    {contextFieldDisplayLabel(field)}
                  </label>
                  <select
                    id={field.id}
                    name={field.id}
                    defaultValue={contextFieldDefaultValue(field)}
                    className="h-10 rounded-lg border border-slate-200 bg-white px-3 text-sm text-slate-900 outline-none focus:border-slate-400"
                  >
                    {field.candidates?.map((candidate, index) => (
                      <option
                        key={`${contextCandidateValue(candidate)}-${index}`}
                        value={contextCandidateValue(candidate)}
                      >
                        {contextCandidateLabel(candidate)}
                      </option>
                    ))}
                  </select>
                </div>
              );
            }
            if (control === "radio_cards") {
              return (
                <div
                  key={field.id}
                  className="grid gap-2 rounded-xl border border-slate-200 bg-slate-50/70 p-3 text-sm text-slate-600"
                >
                  <span className="font-medium">
                    {contextFieldDisplayLabel(field)}
                  </span>
                  <div className="grid gap-2">
                    {field.candidates?.map((candidate, index) => (
                      <label
                        key={`${contextCandidateValue(candidate)}-${index}`}
                        className="flex items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 py-2"
                      >
                        <input
                          type="radio"
                          name={field.id}
                          value={contextCandidateValue(candidate)}
                          defaultChecked={
                            index === 0 ||
                            contextCandidateValue(candidate) ===
                              contextFieldDefaultValue(field)
                          }
                        />
                        <span>{contextCandidateLabel(candidate)}</span>
                      </label>
                    ))}
                  </div>
                </div>
              );
            }
            const label = contextFieldDisplayLabel(field);
            return (
              <label
                key={field.id}
                className="grid gap-2 rounded-xl border border-slate-200 bg-slate-50/70 p-3 text-sm text-slate-600"
              >
                <span className="font-medium">{label}</span>
                <input
                  type={contextFieldInputType(field)}
                  name={field.id}
                  className="h-10 rounded-lg border border-slate-200 bg-white px-3 text-sm text-slate-900 outline-none focus:border-slate-400"
                  placeholder={contextFieldPlaceholder(field)}
                  defaultValue={contextFieldDefaultValue(field)}
                  autoComplete={
                    contextFieldIsSensitive(field) ? "off" : undefined
                  }
                  spellCheck={
                    contextFieldIsSensitive(field) ? false : undefined
                  }
                />
              </label>
            );
          })}
        </div>
        <div className="mt-3 flex justify-end gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="rounded-md"
            onClick={onCancel}
          >
            取消
          </Button>
          <Button
            type="submit"
            size="sm"
            className="rounded-md bg-slate-950 text-white hover:bg-slate-800"
          >
            提交并继续
          </Button>
        </div>
      </form>
    </div>
  );
}

function normalizeContextFieldId(id: string) {
  return id.trim().toLowerCase();
}

function contextFieldControl(field: ContextFormField) {
  const explicit = String(field.uiControl || "")
    .trim()
    .toLowerCase();
  if (explicit === "select" || explicit === "radio_cards") {
    return field.candidates?.length ? explicit : "text";
  }
  if (field.candidates?.length) return "select";
  return "text";
}

function contextFieldDisplayLabel(field: ContextFormField) {
  const id = normalizeContextFieldId(field.id);
  if (id === "target_location") return "目标位置（可选）";
  if (id === "symptom") return "现象/证据（可选）";
  if (
    contextFieldIsSensitive(field) &&
    !/secret|引用|敏感/i.test(field.label)
  ) {
    return `${field.label}（Secret 引用）`;
  }
  return field.label;
}

function contextFieldSubmitLabel(field: ContextFormField) {
  const id = normalizeContextFieldId(field.id);
  if (id === "target_location") return "目标位置";
  if (id === "target_instance") return "实例/服务";
  if (id === "execution_surface") return "访问/执行入口";
  if (id === "symptom") return "现象/证据";
  return field.label.replace(/（可选）/g, "").trim();
}

function contextFieldPlaceholder(field: ContextFormField) {
  const id = normalizeContextFieldId(field.id);
  if (contextFieldIsSensitive(field)) {
    return (
      field.placeholder || "例如 secret://team/db-password，避免填写明文密码"
    );
  }
  if (id === "target_location") return "留空使用当前选择主机";
  if (id === "symptom")
    return (
      field.placeholder || "指标、日志、报错、Trace/Case ID、时间范围或关键参数"
    );
  return field.placeholder || field.label;
}

function contextFieldDefaultValue(field: ContextFormField) {
  if (contextFieldIsSensitive(field) && !field.candidates?.length) return "";
  if (field.default !== undefined && field.default !== null)
    return String(field.default);
  const firstCandidate = field.candidates?.[0];
  return firstCandidate ? contextCandidateValue(firstCandidate) : "";
}

function contextFieldSubmitValue(field: ContextFormField, formData: FormData) {
  const value = String(formData.get(field.id) || "").trim();
  if (value) return value;
  if (contextFieldIsSensitive(field)) return "";
  return String(contextFieldDefaultValue(field) || "").trim();
}

function contextFieldInputType(field: ContextFormField) {
  return contextFieldIsSensitive(field) ? "password" : "text";
}

function contextFieldIsSensitive(field: ContextFormField) {
  const id = normalizeContextFieldId(field.id);
  const type = String(field.type || "")
    .trim()
    .toLowerCase();
  const control = String(field.uiControl || "")
    .trim()
    .toLowerCase();
  return (
    Boolean(field.sensitive) ||
    type === "secret_ref" ||
    type === "secret" ||
    control === "secret_ref" ||
    control === "secret" ||
    /(^|[_-])(password|passwd|secret|token|credential|api[_-]?key)([_-]|$)/i.test(
      id,
    )
  );
}

function contextCandidateValue(
  candidate: NonNullable<ContextFormField["candidates"]>[number],
) {
  return String(candidate.value ?? candidate.label ?? "");
}

function contextCandidateLabel(
  candidate: NonNullable<ContextFormField["candidates"]>[number],
) {
  return String(candidate.label || candidate.value || "候选项");
}

function legacyContextFormSubmit(
  request: ContextFormRequest,
  params: Record<string, string>,
) {
  const lines = request.fields
    .map((field) => {
      const value = params[field.id];
      return value ? `${contextFieldSubmitLabel(field)}：${value}` : "";
    })
    .filter(Boolean);
  const contextLine = request.contextText?.trim()
    ? [`关联上下文：${request.contextText.trim()}`]
    : [];
  return {
    text: `补充必要信息，继续下一步自动排查：\n${[...contextLine, ...lines].join("\n")}`,
    metadata: {
      opsManualAction: "submit_required_context",
      ...(request.artifactId ? { sourceArtifactId: request.artifactId } : {}),
    },
  };
}

function normalizeContextCandidates(
  value: unknown,
): ContextFormField["candidates"] {
  if (!Array.isArray(value)) return [];
  return value
    .map((item) => {
      if (!item || typeof item !== "object" || Array.isArray(item)) return null;
      const record = item as Record<string, unknown>;
      return {
        value: record.value ?? record.id ?? record.label,
        label: record.label ? String(record.label) : undefined,
        hint: record.hint ? String(record.hint) : undefined,
        source: record.source ? String(record.source) : undefined,
        confidence:
          typeof record.confidence === "number" ? record.confidence : undefined,
        evidence: record.evidence ? String(record.evidence) : undefined,
      };
    })
    .filter(
      (
        candidate,
      ): candidate is NonNullable<ContextFormField["candidates"]>[number] =>
        Boolean(
          candidate && (candidate.value !== undefined || candidate.label),
        ),
    );
}

function contextRequestKey(
  scopeId: string | undefined,
  artifactId: string | undefined,
  fields: ContextFormField[],
) {
  return `${scopeId || "unknown-thread"}:${artifactId || "unknown"}:${fields.map((field) => field.id).join("|")}`;
}

function dismissContextRequestKeys(keys: string[], memory: Set<string>) {
  for (const key of keys) {
    memory.add(key);
    try {
      window.localStorage.setItem(
        `${DISMISSED_CONTEXT_REQUEST_STORAGE_PREFIX}${key}`,
        "1",
      );
    } catch {
      // Local storage may be unavailable in restricted browser contexts.
    }
  }
}

function rememberContextRequestKeys(keys: string[], memory: Set<string>) {
  for (const key of keys) {
    memory.add(key);
  }
}

function isDismissedContextRequestKey(key: string, memory: Set<string>) {
  if (memory.has(key)) {
    return true;
  }
  try {
    if (
      window.localStorage.getItem(
        `${DISMISSED_CONTEXT_REQUEST_STORAGE_PREFIX}${key}`,
      ) === "1"
    ) {
      memory.add(key);
      return true;
    }
  } catch {
    return false;
  }
  return false;
}

function GenerationConfirmationComposer({
  confirmation,
  variant,
  className,
  onCancel,
  onComplete,
}: {
  confirmation: GenerationConfirmation;
  variant: "default" | "chat";
  className: string;
  onCancel: () => void;
  onComplete: () => void;
}) {
  const sendCommand = useAssistantTransportSendCommand();
  const target = useSessionTargetContext();
  const Icon =
    confirmation.action === "generate_ops_manual_candidate" ? FileText : Wrench;
  const copy = confirmationCopy(confirmation.action, confirmation.sourceTitle);

  function confirm() {
    sendCommand({
      type: "add-message",
      message: {
        role: "user",
        metadata: {
          ...target.metadata,
          opsManualAction: confirmation.action,
          sourceArtifactId: confirmation.artifactId,
          ...confirmation.metadata,
        },
        ...(target.hostId ? { hostId: target.hostId } : {}),
        parts: [{ type: "text", text: copy.message }],
      },
    } as Parameters<typeof sendCommand>[0]);
    onComplete();
  }

  return (
    <div
      className={[
        variant === "chat"
          ? "shrink-0 bg-white px-4 pb-4 pt-2 md:pb-6"
          : "border-t border-zinc-200 bg-white px-4 py-3 lg:px-8",
        className,
      ]
        .filter(Boolean)
        .join(" ")}
      data-testid="ops-manual-generation-confirmation"
    >
      <div className="mx-auto max-w-3xl rounded-[1.25rem] border border-slate-200 bg-white p-4 shadow-[0_10px_28px_rgba(15,23,42,0.10)]">
        <div className="flex items-start gap-3">
          <span className="rounded-md bg-slate-100 p-2 text-slate-700">
            <Icon className="h-4 w-4" />
          </span>
          <div className="min-w-0 flex-1">
            <div className="text-xs font-medium text-slate-400">二次确认</div>
            <div className="mt-1 text-[15px] font-semibold leading-6 text-slate-950">
              {confirmation.title}
            </div>
            <p className="mt-1 text-sm leading-6 text-slate-600">
              {copy.description}
            </p>
          </div>
        </div>
        <div className="mt-4 flex justify-end gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="rounded-md"
            onClick={onCancel}
          >
            取消
          </Button>
          <Button
            type="button"
            size="sm"
            className="rounded-md bg-slate-950 text-white hover:bg-slate-800"
            onClick={confirm}
          >
            {copy.confirmLabel}
          </Button>
        </div>
      </div>
    </div>
  );
}

function confirmationTitle(action: string) {
  if (action === "generate_ops_manual_candidate") return "生成运维手册候选";
  if (action === "generate_runner_workflow_candidate") return "生成工作流候选";
  if (action === "start_runner_workflow_dry_run") return "进入 Dry Run";
  if (action === "confirm_runner_workflow_execution") return "确认执行";
  if (action === "request_runner_workflow_approval") return "发起审批";
  return "确认下一步";
}

function confirmationCopy(action: string, sourceTitle: string) {
  if (action === "generate_ops_manual_candidate") {
    return {
      description: `将基于「${sourceTitle}」生成运维手册候选，仍需验证和发布检查后才能进入运维手册库。`,
      confirmLabel: "确认生成",
      message: `确认生成运维手册候选：${sourceTitle}`,
    };
  }
  if (action === "start_runner_workflow_dry_run") {
    return {
      description: `将基于「${sourceTitle}」进入 Runner Workflow Dry Run，只生成执行计划和校验结果，不会执行真实变更。`,
      confirmLabel: "确认进入 Dry Run",
      message: `确认进入 Dry Run：${sourceTitle}`,
    };
  }
  if (action === "confirm_runner_workflow_execution") {
    return {
      description: `将基于「${sourceTitle}」确认执行绑定 Workflow。执行前仍会遵循当前风险策略和审批结果。`,
      confirmLabel: "确认执行",
      message: `确认执行 Workflow：${sourceTitle}`,
    };
  }
  if (action === "request_runner_workflow_approval") {
    return {
      description: `将基于「${sourceTitle}」发起人工审批。审批通过后才允许执行绑定 Workflow。`,
      confirmLabel: "发起审批",
      message: `发起 Workflow 审批：${sourceTitle}`,
    };
  }
  return {
    description: `将基于「${sourceTitle}」生成工作流候选，仍需验证和发布检查后才能进入运维手册库。`,
    confirmLabel: "确认生成",
    message: `确认生成工作流候选：${sourceTitle}`,
  };
}

function selectComposerApproval(
  state: AiopsTransportState,
): AiopsTransportApproval | undefined {
  const livePendingApprovals = state.runtimeLiveness?.pendingApprovals;
  const approvals = Object.values(state.pendingApprovals || {}).filter(
    (approval) => {
      const approvalId = approval.id?.trim();
      if (livePendingApprovals && approvalId && !livePendingApprovals[approvalId]) {
        return false;
      }
      const status = approval.status?.trim();
      return !status || status === "pending" || status === "blocked";
    },
  );
  if (approvals.length === 0) {
    return undefined;
  }
  const currentTurnID = state.currentTurnId?.trim();
  const currentTurnApproval = approvals.find(
    (approval) => approval.turnId?.trim() === currentTurnID,
  );
  if (currentTurnApproval) {
    return currentTurnApproval;
  }
  return approvals.sort((a, b) =>
    (b.requestedAt || "").localeCompare(a.requestedAt || ""),
  )[0];
}

function ComposerBody({
  variant,
  isRunning,
  state,
  threadIsRunning,
}: {
  variant: "default" | "chat";
  isRunning: boolean;
  state: AiopsTransportState;
  threadIsRunning: boolean;
}) {
  const workspace = useSessionWorkspaceContext();
  const composer = useComposerRuntime();
  const composerState = useComposer((snapshot) => snapshot);
  const sendCommand = useAssistantTransportSendCommand();
  const inputRef = useRef<HTMLTextAreaElement | null>(null);
  const [hosts, setHosts] = useState<HostInventoryItem[]>([]);
  const [hostInventoryFailed, setHostInventoryFailed] = useState(false);
  const [opsManuals, setOpsManuals] = useState<OpsManualView[]>([]);
  const [opsManualsFailed, setOpsManualsFailed] = useState(false);
  const [opsGraphs, setOpsGraphs] = useState<OpsGraphSummary[]>([]);
  const [opsGraphsFailed, setOpsGraphsFailed] = useState(false);
  const [activeToken, setActiveToken] = useState<ActiveHostMentionToken | null>(
    null,
  );
  const [inputText, setInputText] = useState("");
  const [composerCaretIndex, setComposerCaretIndex] = useState<number | null>(null);
  const [highlightedIndex, setHighlightedIndex] = useState(0);
  const suppressedSubmittedTextRef = useRef("");
  const selectedMentionBindingsRef = useRef<AiopsMentionBinding[]>([]);
  const suggestionPopoverId = "host-mention-suggestions";

  useEffect(() => {
    inputRef.current?.focus();
  }, [workspace.composerFocusNonce]);

  useEffect(() => {
    let cancelled = false;
    listHostInventory()
      .then((items) => {
        if (!cancelled) {
          setHosts(items);
          setHostInventoryFailed(false);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setHostInventoryFailed(true);
        }
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    let cancelled = false;
    opsManualsApi.list({ status: "verified", limit: 100 })
      .then((payload) => {
        if (!cancelled) {
          setOpsManuals(payload.items);
          setOpsManualsFailed(false);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setOpsManualsFailed(true);
        }
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    let cancelled = false;
    void listOpsGraphs()
      .then((payload) => {
        if (!cancelled) {
          setOpsGraphs(payload.graphs || payload.items || []);
          setOpsGraphsFailed(false);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setOpsGraphsFailed(true);
        }
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const suggestions = useMemo(
    () => mentionSuggestionsForToken({
      activeToken,
      hosts,
      hostInventoryFailed,
      opsManuals,
      opsManualsFailed,
      opsGraphs,
      opsGraphsFailed,
    }),
    [activeToken, hostInventoryFailed, hosts, opsGraphs, opsGraphsFailed, opsManuals, opsManualsFailed],
  );
  const composerSnapshotText = composerState?.text || "";
  const suppressedSubmittedText = suppressedSubmittedTextRef.current;
  const currentComposerText =
    inputText ||
    (suppressedSubmittedText && composerSnapshotText === suppressedSubmittedText
      ? ""
      : composerSnapshotText);
  const inlineHostMentions = useMemo(
    () => displayHostMentions(currentComposerText, hosts),
    [currentComposerText, hosts],
  );
  const inlineSpecialMentions = useMemo(
    () => parseSpecialAiMentionCandidates(currentComposerText),
    [currentComposerText],
  );
  const selectedHostMentions = useMemo(
    () => uniqueDisplayHostMentions(inlineHostMentions),
    [inlineHostMentions],
  );
  const selectedMentionBindings = useMemo(
    () =>
      reconcileMentionBindings(
        currentComposerText,
        selectedMentionBindingsRef.current,
      ),
    [currentComposerText],
  );
  const reconciledMentionBindings = useMemo(() => {
    const selectedRanges = new Set(
      selectedMentionBindings.map(
        (binding) => `${binding.range.start}:${binding.range.end}`,
      ),
    );
    const typedCapabilities = inlineSpecialMentions
      .filter(
        (mention) => !selectedRanges.has(`${mention.start}:${mention.end}`),
      )
      .map((mention) =>
        buildCapabilityMentionBinding({
          tokenId: mention.tokenId,
          rawText: currentComposerText.slice(mention.start, mention.end),
          range: { start: mention.start, end: mention.end },
          capability:
            mention.value === "ops_manus"
              ? "ops_manuals"
              : mention.value,
        }),
      );
    return [...selectedMentionBindings, ...typedCapabilities];
  }, [currentComposerText, inlineSpecialMentions, selectedMentionBindings]);
  const inlineResourceMentions = useMemo(
    () => displayResourceMentions(reconciledMentionBindings),
    [reconciledMentionBindings],
  );
  const inlineMentions = useMemo(
    () => mergeInlineMentions(inlineHostMentions, inlineSpecialMentions, inlineResourceMentions),
    [inlineHostMentions, inlineResourceMentions, inlineSpecialMentions],
  );
  const suggestionOpen =
    Boolean(activeToken) &&
    !isRunning &&
    !workspace.composerDisabledReason;
  const hasInlineMentions = inlineMentions.length > 0;

  const refreshActiveToken = useCallback(() => {
    const input = inputRef.current;
    if (!input) return;
    suppressedSubmittedTextRef.current = "";
    setInputText(input.value);
    const cursor = input.selectionStart ?? input.value.length;
    const selectionEnd = input.selectionEnd ?? cursor;
    setComposerCaretIndex(cursor === selectionEnd ? cursor : null);
    const nextToken = findActiveHostMentionToken(input.value, cursor);
    setActiveToken(nextToken);
    setHighlightedIndex(0);
  }, [inlineMentions]);

  const applySuggestion = useCallback(
    (suggestion: ComposerMentionSuggestion) => {
      const input = inputRef.current;
      if (!input || !activeToken) return;
      if (suggestion.kind === "category") {
        const nextText = `${input.value.slice(0, activeToken.start)}${suggestion.prefix}${input.value.slice(activeToken.end)}`;
        const nextCursor = activeToken.start + suggestion.prefix.length;
        const nextToken: ActiveHostMentionToken = {
          start: activeToken.start,
          end: nextCursor,
          query: suggestion.prefix.slice(1),
          raw: suggestion.prefix,
        };
        selectedMentionBindingsRef.current = selectedMentionBindingsRef.current.filter(
          (binding) => binding.range.start !== activeToken.start,
        );
        composer.setText(nextText);
        setInputText(nextText);
        input.value = nextText;
        input.setSelectionRange(nextCursor, nextCursor);
        input.dispatchEvent(new Event("input", { bubbles: true }));
        input.dispatchEvent(new Event("change", { bubbles: true }));
        setActiveToken(nextToken);
        setHighlightedIndex(0);
        return;
      }
      const next = replaceActiveHostMention(
        input.value,
        activeToken,
        suggestion,
      );
      const rawText = next.text.slice(activeToken.start, next.cursor).trim();
      const existingBindings = selectedMentionBindingsRef.current.filter(
        (binding) => binding.range.start !== activeToken.start,
      );
      selectedMentionBindingsRef.current =
        suggestion.kind === "capability"
          ? [
              ...existingBindings,
              buildCapabilityMentionBinding({
                tokenId: `mention-${activeToken.start}-${suggestion.key}`,
                rawText,
                range: {
                  start: activeToken.start,
                  end: activeToken.start + rawText.length,
                },
                capability: suggestion.payload.capability,
              }),
            ]
          : suggestion.kind === "resource" && suggestion.payload.resourceKind === "ops_manual"
            ? [
                ...existingBindings,
                buildOpsManualMentionBinding({
                  tokenId: `mention-${activeToken.start}-${suggestion.key}`,
                  rawText,
                  range: {
                    start: activeToken.start,
                    end: activeToken.start + rawText.length,
                  },
                  manualId: suggestion.payload.manualId,
                  title: suggestion.payload.title,
                  workflowId: suggestion.payload.workflowId,
                  status: suggestion.payload.status,
                }),
              ]
            : suggestion.kind === "resource" && suggestion.payload.resourceKind === "ops_graph"
              ? [
                  ...existingBindings,
                  buildOpsGraphMentionBinding({
                    tokenId: `mention-${activeToken.start}-${suggestion.key}`,
                    rawText,
                    range: {
                      start: activeToken.start,
                      end: activeToken.start + rawText.length,
                    },
                    graphId: suggestion.payload.graphId,
                    name: suggestion.payload.name,
                    environment: suggestion.payload.environment,
                  }),
                ]
          : [
              ...existingBindings,
              buildHostMentionBinding({
                tokenId: `mention-${activeToken.start}-${suggestion.key}`,
                rawText,
                range: {
                  start: activeToken.start,
                  end: activeToken.start + rawText.length,
                },
                hostId: suggestion.payload.hostId,
                address: suggestion.payload.address,
                displayName: suggestion.payload.displayName,
                status: suggestion.payload.status,
                hostMentionSource: suggestion.payload.source,
              }),
            ];
      composer.setText(next.text);
      setInputText(next.text);
      input.value = next.text;
      input.setSelectionRange(next.cursor, next.cursor);
      input.dispatchEvent(new Event("input", { bubbles: true }));
      input.dispatchEvent(new Event("change", { bubbles: true }));
      setActiveToken(null);
      setComposerCaretIndex(next.cursor);
      setHighlightedIndex(0);
    },
    [activeToken, composer],
  );
  useEffect(() => {
    if (
      !suppressedSubmittedTextRef.current ||
      inputText ||
      composerSnapshotText !== suppressedSubmittedTextRef.current
    ) {
      return;
    }
    composer.setText("");
    if (inputRef.current?.value === suppressedSubmittedTextRef.current) {
      inputRef.current.value = "";
    }
  }, [composer, composerSnapshotText, inputText]);

  const clearComposerInput = useCallback((submittedText = "") => {
    suppressedSubmittedTextRef.current = submittedText;
    const clear = () => {
      composer.setText("");
      setInputText("");
      setActiveToken(null);
      setComposerCaretIndex(null);
      setHighlightedIndex(0);
      selectedMentionBindingsRef.current = [];
      if (inputRef.current) {
        inputRef.current.value = "";
      }
    };
    clear();
    window.requestAnimationFrame(clear);
    window.setTimeout(clear, 0);
  }, [composer]);

  const submitComposerMessage = useCallback(() => {
    if (isRunning) return;
    const text = composer.getState().text.trim();
    if (!text) return;
    composer.setText("");
    clearComposerInput(text);
    sendCommand({
      type: "add-message",
      message: {
        role: "user",
        metadata: {
          ...buildInputMentionMetadata(reconciledMentionBindings),
          ...hostMentionMetadataForSubmit(
            reconciledMentionBindings,
            selectedHostMentions,
          ),
          ...capabilityMetadataForSubmit(reconciledMentionBindings, text),
        },
        ...hostIdMessageFieldForSubmit(
          reconciledMentionBindings,
          selectedHostMentions,
        ),
        parts: [{ type: "text", text }],
      },
    } as Parameters<typeof sendCommand>[0]);
  }, [
    clearComposerInput,
    composer,
    isRunning,
    reconciledMentionBindings,
    selectedHostMentions,
    sendCommand,
  ]);

  useEffect(() => {
    if (!isRunning) {
      return;
    }
    const submittedText =
      inputRef.current?.value || inputText || composerSnapshotText;
    if (!submittedText) {
      return;
    }
    clearComposerInput(submittedText);
  }, [clearComposerInput, composerSnapshotText, inputText, isRunning]);

  return (
    <ComposerPrimitive.Root
      onSubmit={(event) => {
        event.preventDefault();
        submitComposerMessage();
      }}
      className={
        variant === "chat"
          ? "relative z-10 flex flex-col gap-2 rounded-[1.5rem] border border-slate-200 bg-white p-2 shadow-[0_10px_28px_rgba(15,23,42,0.10)] transition-shadow focus-within:border-slate-300 focus-within:shadow-[0_12px_36px_rgba(15,23,42,0.14)]"
          : "mx-auto flex max-w-5xl items-end gap-2"
      }
    >
      {suggestionOpen ? (
        <HostMentionSuggestionPopover
          id={suggestionPopoverId}
          suggestions={suggestions}
          highlightedIndex={highlightedIndex}
          onHighlight={setHighlightedIndex}
          onSelect={applySuggestion}
        />
      ) : null}
      <div
        className={
          variant === "chat" ? "relative min-h-12" : "relative min-w-0 flex-1"
        }
      >
        <HostMentionInlineOverlay
          text={currentComposerText}
          mentions={inlineMentions}
          caretIndex={hasInlineMentions ? composerCaretIndex : null}
          variant={variant}
        />
        <ComposerPrimitive.Input asChild submitOnEnter={false}>
          <Textarea
            ref={inputRef}
            data-testid="omnibar-input"
            rows={1}
            placeholder="输入你的问题或任务"
            spellCheck={false}
            disabled={Boolean(workspace.composerDisabledReason) || isRunning}
            aria-controls={suggestionOpen ? suggestionPopoverId : undefined}
            aria-expanded={suggestionOpen}
            onFocus={refreshActiveToken}
            onBlur={() => setComposerCaretIndex(null)}
            onInput={refreshActiveToken}
            onClick={refreshActiveToken}
            onSelect={refreshActiveToken}
            onKeyUp={(event) => {
              if (
                ["ArrowDown", "ArrowUp", "Enter", "Tab", "Escape"].includes(
                  event.key,
                )
              ) {
                return;
              }
              refreshActiveToken();
            }}
            onKeyDown={(event) => {
              if (suggestionOpen) {
                if (event.key === "Escape") {
                  event.preventDefault();
                  setActiveToken(null);
                  return;
                }
                if (event.key === "ArrowDown") {
                  event.preventDefault();
                  setHighlightedIndex((index) =>
                    suggestions.length ? (index + 1) % suggestions.length : 0,
                  );
                  return;
                }
                if (event.key === "ArrowUp") {
                  event.preventDefault();
                  setHighlightedIndex((index) =>
                    suggestions.length
                      ? (index - 1 + suggestions.length) % suggestions.length
                      : 0,
                  );
                  return;
                }
                if (
                  (event.key === "Enter" || event.key === "Tab") &&
                  suggestions[highlightedIndex]
                ) {
                  event.preventDefault();
                  applySuggestion(suggestions[highlightedIndex]);
                }
                return;
              }
              if (
                event.key === "Enter" &&
                !event.shiftKey &&
                !event.nativeEvent.isComposing
              ) {
                event.preventDefault();
                submitComposerMessage();
              }
            }}
            className={cn(
              variant === "chat"
                ? "relative z-10 max-h-40 min-h-12 resize-none border-0 bg-transparent px-3 py-2 text-[16px] leading-7 shadow-none focus-visible:ring-0 md:text-[16px]"
                : "relative z-10 max-h-44 min-h-11 resize-none rounded-lg border-zinc-300 bg-zinc-50 text-sm",
              hasInlineMentions &&
                "text-transparent caret-transparent selection:bg-sky-200/70",
            )}
          />
        </ComposerPrimitive.Input>
      </div>

      <div
        className={
          variant === "chat"
            ? "flex shrink-0 items-center justify-between"
            : "mb-1 flex shrink-0 items-center gap-2"
        }
      >
        <span className="text-xs text-slate-400 pl-1">
          {workspace.llmLabel}
        </span>
        <TargetAwareSendButton
          variant={variant}
          isRunning={isRunning}
          state={state}
          threadIsRunning={threadIsRunning}
          onSubmit={submitComposerMessage}
        />
      </div>
    </ComposerPrimitive.Root>
  );
}

function displayHostMentions(
  text: string,
  hosts: HostInventoryItem[],
): DisplayHostMention[] {
  const mentions: DisplayHostMention[] = [];
  for (const mention of parseHostMentionCandidates(text)) {
    if (mention.source === "local_alias") {
      mentions.push({
        ...mention,
        value: "server-local",
        hostId: "server-local",
        address: "server-local",
        displayName: "local",
        resolved: true,
      });
      continue;
    }
    const host = findHostForMention(hosts, mention.value);
    if (!host) {
      continue;
    }
    const displayMention: DisplayHostMention = {
      ...mention,
      hostId: cleanHostText(host.id) || cleanHostText(host.hostId),
      address:
        cleanHostText(host.ip) ||
        cleanHostText(
          (host as HostInventoryItem & { address?: string }).address,
        ),
      displayName:
        cleanHostText(host.name) ||
        cleanHostText(host.ip) ||
        cleanHostText(
          (host as HostInventoryItem & { address?: string }).address,
        ) ||
        mention.raw,
      resolved: true,
    };
    mentions.push(displayMention);
  }
  return mentions;
}

function mentionSuggestionsForToken({
  activeToken,
  hosts,
  hostInventoryFailed,
  opsManuals,
  opsManualsFailed,
  opsGraphs,
  opsGraphsFailed,
}: {
  activeToken: ActiveHostMentionToken | null;
  hosts: HostInventoryItem[];
  hostInventoryFailed: boolean;
  opsManuals: OpsManualView[];
  opsManualsFailed: boolean;
  opsGraphs: OpsGraphSummary[];
  opsGraphsFailed: boolean;
}): ComposerMentionSuggestion[] {
  if (!activeToken) {
    return [];
  }
  const route = mentionMenuRoute(activeToken.query);
  if (route.kind === "host") {
    return hostInventoryFailed
      ? []
      : searchHostMentionSuggestions(hosts, route.query, { limit: 8 });
  }
  if (route.kind === "capability") {
    return searchCapabilityMentionSuggestions(route.query, {
      category: route.category,
    }).slice(0, 8);
  }
  if (route.kind === "ops_manuals") {
    return opsManualsFailed
      ? []
      : searchOpsManualMentionSuggestions(opsManuals, route.query, { limit: 8 });
  }
  if (route.kind === "ops_graph") {
    return opsGraphsFailed
      ? []
      : searchOpsGraphMentionSuggestions(opsGraphs, route.query, { limit: 8 });
  }
  return searchMentionCategorySuggestions(route.query).slice(0, 8);
}

type MentionMenuRoute =
  | { kind: "category"; query: string }
  | { kind: "host"; query: string }
  | { kind: "capability"; category: Exclude<MentionCategory, "host" | "ops_graph" | "ops_manuals">; query: string }
  | { kind: "ops_graph"; query: string }
  | { kind: "ops_manuals"; query: string };

function mentionMenuRoute(query: string): MentionMenuRoute {
  const normalized = cleanHostText(query);
  const hostQuery = stripMentionCategoryPrefix(normalized, "host-");
  if (hostQuery !== null) {
    return { kind: "host", query: hostQuery };
  }
  const monitorQuery = stripMentionCategoryPrefix(normalized, "monitor-");
  if (monitorQuery !== null) {
    return { kind: "capability", category: "monitor", query: monitorQuery };
  }
  const graphQuery = stripMentionCategoryPrefix(normalized, "opsgraph-");
  if (graphQuery !== null) {
    return { kind: "ops_graph", query: graphQuery };
  }
  const manualQuery = stripMentionCategoryPrefix(normalized, "manual-");
  if (manualQuery !== null) {
    return { kind: "ops_manuals", query: manualQuery };
  }
  return { kind: "category", query: normalized };
}

function stripMentionCategoryPrefix(query: string, prefix: string) {
  const normalized = query.toLowerCase();
  return normalized.startsWith(prefix) ? query.slice(prefix.length) : null;
}

function mergeInlineMentions(
  hostMentions: DisplayHostMention[],
  specialMentions: ReturnType<typeof parseSpecialAiMentionCandidates>,
  resourceMentions: ResourceInlineMentionCandidate[],
) {
  return [...hostMentions, ...specialMentions, ...resourceMentions].sort((a, b) => a.start - b.start || a.end - b.end);
}

function displayResourceMentions(bindings: AiopsMentionBinding[]): ResourceInlineMentionCandidate[] {
  return bindings
    .filter((binding) => binding.kind === "ops_manual" || binding.kind === "ops_graph")
    .map((binding) => {
      const payload = objectPayload(binding.payload);
      const value =
        binding.kind === "ops_manual"
          ? cleanHostText(payload.manualId) || binding.rawText.replace(/^@/, "")
          : cleanHostText(payload.graphId) || binding.rawText.replace(/^@/, "");
      const displayName =
        binding.kind === "ops_manual"
          ? cleanHostText(payload.title) || value
          : cleanHostText(payload.name) || value;
      return {
        tokenId: binding.tokenId,
        raw: binding.rawText,
        value,
        start: binding.range.start,
        end: binding.range.end,
        source: "ops_resource" as const,
        kind: binding.kind as "ops_manual" | "ops_graph",
        displayName,
      };
    });
}

function uniqueDisplayHostMentions(
  mentions: DisplayHostMention[],
): DisplayHostMention[] {
  const seen = new Set<string>();
  const selected: DisplayHostMention[] = [];
  for (const displayMention of mentions) {
    const key = hostDisplayMentionKey(displayMention);
    if (!key || seen.has(key)) {
      continue;
    }
    seen.add(key);
    selected.push(displayMention);
  }
  return selected;
}

function findHostForMention(hosts: HostInventoryItem[], value: string) {
  const normalized = normalizeHostMentionValue(value);
  if (!normalized) {
    return undefined;
  }
  return hosts.find((host) => {
    const extended = host as HostInventoryItem & {
      address?: string;
      hostId?: string;
    };
    return [host.name, host.ip, extended.address, host.id, extended.hostId]
      .map(normalizeHostMentionValue)
      .some((candidate) => candidate === normalized);
  });
}

function hostDisplayMentionKey(mention: DisplayHostMention) {
  return normalizeHostMentionValue(
    mention.hostId || mention.value || mention.raw,
  );
}

function normalizeHostMentionValue(value: unknown) {
  return cleanHostText(value).replace(/^@+/, "").toLowerCase();
}

function cleanHostText(value: unknown) {
  return typeof value === "string" ? value.trim() : "";
}

function TargetAwareSendButton({
  variant,
  isRunning,
  state,
  threadIsRunning,
  onSubmit,
}: {
  variant: "default" | "chat";
  isRunning: boolean;
  state: AiopsTransportState;
  threadIsRunning: boolean;
  onSubmit: () => void;
}) {
  const api = useAssistantApi();
  const composerState = useComposer((snapshot) => snapshot);
  const commands = useAiopsTransportCommands();
  const workspace = useSessionWorkspaceContext();
  const [stopping, setStopping] = useState(false);
  const [forceStopVisible, setForceStopVisible] = useState(false);

  // Reset stopping and forceStopVisible state when the run completes/cancels
  useEffect(() => {
    if (!isRunning) {
      setStopping(false);
      setForceStopVisible(false);
    }
  }, [isRunning]);

  // 2-second timeout: if stop hasn't completed, show force-stop button
  useEffect(() => {
    if (!stopping) return;
    const timer = setTimeout(() => {
      if (isRunning) {
        setForceStopVisible(true);
      }
    }, 2000);
    return () => clearTimeout(timer);
  }, [stopping, isRunning]);

  const disabled =
    (stopping && !forceStopVisible) ||
    (!isRunning &&
      (!composerState?.text?.trim() ||
        Boolean(workspace.composerDisabledReason)));
  const stopDispatchTarget = resolveStopDispatchTarget(state, threadIsRunning);

  async function stopRun(reason: string) {
    if (stopDispatchTarget === "runtime") {
      api.thread().cancelRun();
      return;
    }
    commands.stop(reason);
    api.thread().cancelRun();
  }

  return (
    <Button
      type="button"
      size={variant === "chat" ? "icon" : "icon-lg"}
      data-testid="omnibar-primary-action"
      aria-label={
        forceStopVisible
          ? "强制停止"
          : stopping
            ? "正在停止"
            : isRunning
              ? "停止生成"
              : "send message"
      }
      disabled={disabled}
      className={
        variant === "chat"
          ? isRunning
            ? forceStopVisible
              ? "h-8 w-8 rounded-full border border-red-400 bg-red-50 text-red-700 shadow-sm hover:bg-red-100"
              : "h-8 w-8 rounded-full border border-slate-300 bg-white text-slate-700 shadow-sm hover:bg-slate-50"
            : "h-8 w-8 rounded-full bg-slate-950 text-white shadow-sm hover:bg-slate-800 disabled:bg-slate-200 disabled:text-slate-400"
          : forceStopVisible
            ? "border-red-400 bg-red-50 text-red-700 hover:bg-red-100"
            : ""
      }
      onClick={() => {
        if (forceStopVisible) {
          void stopRun("user force stop");
          return;
        }
        if (isRunning) {
          setStopping(true);
          void stopRun("user requested stop");
          return;
        }
        onSubmit();
      }}
    >
      {forceStopVisible ? (
        <Square className="h-3.5 w-3.5 fill-red-600 text-red-600" />
      ) : stopping ? (
        <LoaderCircle className="h-3.5 w-3.5 animate-spin" />
      ) : isRunning ? (
        <Square className="h-3.5 w-3.5 fill-current" />
      ) : (
        <ArrowUp className="h-4 w-4" />
      )}
    </Button>
  );
}

function hostIdMessageField(hostMentions: DisplayHostMention[]) {
  const hostIds = Array.from(
    new Set(hostMentions.map((mention) => cleanHostText(mention.hostId)).filter(Boolean)),
  );
  return hostIds.length === 1 ? { hostId: hostIds[0] } : {};
}

function hostMentionMetadataForSubmit(
  mentionBindings: AiopsMentionBinding[],
  hostMentions: DisplayHostMention[],
) {
  const structuredMetadata = deriveHostMentionMetadata(mentionBindings);
  return Object.keys(structuredMetadata).length
    ? structuredMetadata
    : buildHostMentionMetadata(hostMentions);
}

function capabilityMetadataForSubmit(
  mentionBindings: AiopsMentionBinding[],
  text: string,
) {
  const structuredMetadata = deriveCapabilityMentionMetadata(mentionBindings);
  return Object.keys(structuredMetadata).length
    ? structuredMetadata
    : buildAiopsSpecialMentionMetadata(text);
}

function hostIdMessageFieldForSubmit(
  mentionBindings: AiopsMentionBinding[],
  hostMentions: DisplayHostMention[],
) {
  const structuredHostIds = Array.from(
    new Set(
      mentionBindings
        .filter((binding) => binding.kind === "host")
        .map((binding) =>
          cleanHostText(
            objectPayload(binding.payload).hostId || decodeHostPath(binding.path),
          ),
        )
        .filter(Boolean),
    ),
  );
  if (structuredHostIds.length === 1) return { hostId: structuredHostIds[0] };
  if (structuredHostIds.length > 1) return {};
  return hostIdMessageField(hostMentions);
}

function decodeHostPath(path: string) {
  const raw = cleanHostText(path).replace(/^host:\/\//, "");
  try {
    return decodeURIComponent(raw);
  } catch {
    return raw;
  }
}

function objectPayload(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
}

function BlockedApprovalComposer({
  approval,
}: {
  approval: AiopsTransportApproval;
}) {
  const commands = useAiopsTransportCommands();
  const [decision, setDecision] = useState<
    "accept" | "accept_session" | "reject"
  >("accept");
  const [submittingDecision, setSubmittingDecision] = useState<
    "accept" | "accept_session" | "reject" | null
  >(null);
  const [submitError, setSubmitError] = useState("");
  const timeoutRef = useRef<ReturnType<typeof window.setTimeout> | null>(null);
  const commandText = approval.command || approval.reason || approval.id;
  const isSubmitting = Boolean(submittingDecision) && !submitError;

  useEffect(() => {
    clearApprovalDecisionTimeout(timeoutRef);
    setDecision("accept");
    setSubmittingDecision(null);
    setSubmitError("");
    return () => clearApprovalDecisionTimeout(timeoutRef);
  }, [approval.id]);

  function submitDecision(
    nextDecision: "accept" | "accept_session" | "reject" = decision,
  ) {
    if (isSubmitting) {
      return;
    }
    setSubmittingDecision(nextDecision);
    setSubmitError("");
    clearApprovalDecisionTimeout(timeoutRef);
    try {
      commands.approvalDecision(approval.id, nextDecision);
      timeoutRef.current = window.setTimeout(() => {
        setSubmittingDecision(null);
        setSubmitError(
          `审批请求超时：后端 ${APPROVAL_DECISION_TIMEOUT_MS / 1000} 秒内未返回继续执行状态，请刷新状态或重试。`,
        );
      }, APPROVAL_DECISION_TIMEOUT_MS);
    } catch (error) {
      clearApprovalDecisionTimeout(timeoutRef);
      setSubmittingDecision(null);
      setSubmitError(
        error instanceof Error ? error.message : "提交审批失败，请重试",
      );
    }
  }

  return (
    <div
      className="shrink-0 bg-white px-4 pb-4 pt-2 md:pb-6"
      data-testid="codex-approval-inline"
    >
      <div className="mx-auto max-w-3xl rounded-[1.75rem] border border-slate-200 bg-white p-4 shadow-[0_10px_28px_rgba(15,23,42,0.10)]">
        <div className="space-y-3">
          <div>
            <div className="text-xs font-medium text-slate-400">
              {approvalInlineTitle(approval)}
            </div>
            <div className="mt-1 text-[15px] font-semibold leading-6 text-slate-950">
              要执行这个命令，需要你确认吗？
            </div>
          </div>
          <div
            className="rounded-xl bg-slate-100 px-4 py-3 font-mono text-[13px] leading-6 text-slate-700"
            data-testid="codex-approval-command"
          >
            {commandText}
          </div>
          {approvalInlineDetails(approval).length ? (
            <dl
              className="grid gap-1.5 rounded-xl border border-slate-200 bg-slate-50 px-3 py-2 text-xs leading-5 text-slate-700 sm:grid-cols-[4.5rem_1fr]"
              data-testid="codex-approval-details"
            >
              {approvalInlineDetails(approval).map((detail) => (
                <div key={detail.label} className="contents">
                  <dt className="font-medium text-slate-500">{detail.label}</dt>
                  <dd className="min-w-0 break-words text-slate-800">
                    {detail.value}
                  </dd>
                </div>
              ))}
            </dl>
          ) : null}
          <div className="space-y-1" role="radiogroup" aria-label="审批选项">
            <ApprovalChoice
              selected={decision === "accept"}
              onClick={() => setDecision("accept")}
              label="1. 是"
              disabled={isSubmitting}
            />
            <ApprovalChoice
              selected={decision === "accept_session"}
              onClick={() => setDecision("accept_session")}
              label="2. 是，且对于以后类似命令不再询问"
              disabled={isSubmitting}
            />
            <ApprovalChoice
              selected={decision === "reject"}
              onClick={() => setDecision("reject")}
              label="3. 否，请告知 AIOps 如何调整"
              tone="muted"
              disabled={isSubmitting}
            />
          </div>
          {isSubmitting ? (
            <div
              className="flex items-center gap-2 rounded-xl bg-blue-50 px-3 py-2 text-sm text-blue-700"
              role="status"
            >
              <LoaderCircle className="h-4 w-4 animate-spin" />
              <span>已提交确认，正在继续执行...</span>
            </div>
          ) : null}
          {submitError ? (
            <div
              className="rounded-xl bg-red-50 px-3 py-2 text-sm text-red-700"
              role="alert"
            >
              {submitError}
            </div>
          ) : null}
        </div>
        <div className="mt-4 flex justify-end gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="rounded-full border-slate-200 bg-white px-4"
            disabled={isSubmitting}
            onClick={() => submitDecision("reject")}
          >
            跳过
          </Button>
          <Button
            type="button"
            size="sm"
            className="rounded-full bg-slate-950 px-4 text-white hover:bg-slate-800"
            disabled={isSubmitting}
            onClick={() => submitDecision()}
          >
            {isSubmitting ? "提交中" : "提交"}
          </Button>
        </div>
      </div>
    </div>
  );
}

function approvalInlineDetails(approval: AiopsTransportApproval) {
  return [
    { label: "目标", value: approval.targetSummary },
    { label: "风险", value: approval.risk },
    { label: "来源", value: approvalInlineSourceLabel(approval.source) },
    { label: "影响", value: approval.expectedEffect },
    { label: "回滚", value: approval.rollback },
    { label: "验收", value: approval.validation },
  ].filter((item): item is { label: string; value: string } =>
    Boolean(item.value),
  );
}

function approvalInlineTitle(approval: AiopsTransportApproval) {
  const target = approvalInlinePrimaryTarget(approval.targetSummary);
  return target ? `${target} 等待审批` : "等待审批";
}

function approvalInlinePrimaryTarget(targetSummary?: string) {
  const value = targetSummary?.trim();
  if (!value) return "";
  const first = value
    .split(/[；;,，]/)
    .map((item) => item.trim())
    .find(Boolean);
  if (!first) return "";
  if (first.startsWith("host:")) return first.slice("host:".length).trim();
  return first;
}

function approvalInlineSourceLabel(source?: string) {
  switch (source) {
    case "ai_chat_direct":
      return "AI Chat";
    case "ops_manual":
      return "运维手册";
    case "workflow":
      return "Workflow";
    case "runbook":
      return "Runbook";
    case "terminal_policy":
      return "终端策略";
    default:
      return source;
  }
}

function clearApprovalDecisionTimeout(
  timeoutRef: { current: ReturnType<typeof window.setTimeout> | null },
) {
  if (timeoutRef.current) {
    window.clearTimeout(timeoutRef.current);
    timeoutRef.current = null;
  }
}

function ApprovalChoice({
  selected,
  onClick,
  label,
  tone = "default",
  disabled = false,
}: {
  selected: boolean;
  onClick: () => void;
  label: string;
  tone?: "default" | "muted";
  disabled?: boolean;
}) {
  return (
    <button
      type="button"
      role="radio"
      aria-checked={selected}
      disabled={disabled}
      className={[
        "flex w-full items-center justify-between rounded-xl px-3 py-2 text-left text-[15px] leading-6 transition-colors",
        disabled ? "cursor-not-allowed opacity-70" : "",
        selected
          ? "bg-slate-100 text-slate-950"
          : tone === "muted"
            ? "text-slate-400 hover:bg-slate-50"
            : "text-slate-500 hover:bg-slate-50",
      ].join(" ")}
      onClick={onClick}
    >
      <span>{label}</span>
      {selected ? <Check className="h-4 w-4 text-slate-500" /> : null}
    </button>
  );
}
