import {
  useAssistantApi,
  ComposerPrimitive,
  useAssistantTransportSendCommand,
  useAssistantTransportState,
  useComposer,
  useComposerRuntime,
  useThread,
} from "@assistant-ui/react";
import { ArrowUp, Check, FileText, LoaderCircle, Square, Wrench } from "lucide-react";
import { type FormEvent, useEffect, useRef, useState } from "react";

import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { isAiopsTransportRunning } from "@/transport/aiopsTransportConverter";
import { useAiopsTransportCommands } from "@/transport/useAiopsTransportCommands";
import type { AiopsTransportApproval, AiopsTransportState } from "@/transport/aiopsTransportTypes";

import { buildOpsManualParamFormSubmit, resolveStopDispatchTarget } from "./aiopsComposerActions";
import { useSessionTargetContext } from "./SessionTargetContext";
import { useSessionWorkspaceContext } from "./SessionWorkspaceContext";

type GenerationConfirmation = {
  action: string;
  title: string;
  sourceTitle: string;
  artifactId?: string;
};

const SUPPORTED_CONFIRMATION_ACTIONS = new Set([
  "generate_ops_manual_candidate",
  "generate_runner_workflow_candidate",
  "start_runner_workflow_dry_run",
]);

type ContextFormField = {
  id: string;
  label: string;
  type?: string;
  required?: boolean;
  sensitive?: boolean;
  uiControl?: string;
  placeholder?: string;
  default?: unknown;
  candidates?: Array<{ value?: unknown; label?: string; hint?: string; source?: string; confidence?: number; evidence?: string }>;
};

type ContextFormRequest = {
  artifactId?: string;
  manualId?: string;
  workflowId?: string;
  submitAction?: string;
  key: string;
  title: string;
  summary?: string;
  contextText?: string;
  fields: ContextFormField[];
  force?: boolean;
};

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
  const sendCommand = useAssistantTransportSendCommand();
  const target = useSessionTargetContext();
  const isRunning = isAiopsTransportRunning(state) || threadIsRunning;
  const [generationConfirmation, setGenerationConfirmation] = useState<GenerationConfirmation | null>(null);
  const [contextRequest, setContextRequest] = useState<ContextFormRequest | null>(null);
  const dismissedContextRequestKeysRef = useRef(new Set<string>());

  useEffect(() => {
    function handleConfirmation(event: Event) {
      const detail = (event as CustomEvent<Partial<GenerationConfirmation>>).detail || {};
      const action = String(detail.action || "").trim();
      if (!SUPPORTED_CONFIRMATION_ACTIONS.has(action)) {
        return;
      }
      setGenerationConfirmation({
        action,
        title: String(detail.title || confirmationTitle(action)),
        sourceTitle: String(detail.sourceTitle || "当前对话"),
        artifactId: detail.artifactId ? String(detail.artifactId) : undefined,
      });
    }
    window.addEventListener("aiops:composer-confirmation", handleConfirmation);
    return () => window.removeEventListener("aiops:composer-confirmation", handleConfirmation);
  }, []);

  useEffect(() => {
    function handleContextSubmit(event: Event) {
      const detail = (event as CustomEvent<{ text?: string; artifactId?: string; metadata?: Record<string, string> }>).detail || {};
      const text = String(detail.text || "").trim();
      if (!text) return;
      const metadata = detail.metadata && typeof detail.metadata === "object" ? detail.metadata : {};
      sendCommand({
        type: "add-message",
        message: {
          role: "user",
          metadata: {
            ...target.metadata,
            opsManualAction: metadata.opsManualAction || "submit_required_context",
            sourceArtifactId: detail.artifactId,
            ...metadata,
          },
          ...(target.hostId ? { hostId: target.hostId } : {}),
          parts: [{ type: "text", text }],
        },
      } as Parameters<typeof sendCommand>[0]);
      setContextRequest(null);
    }
    window.addEventListener("aiops:composer-context-submit", handleContextSubmit);
    return () => window.removeEventListener("aiops:composer-context-submit", handleContextSubmit);
  }, [sendCommand, target.hostId, target.metadata]);

  useEffect(() => {
    function handleContextRequest(event: Event) {
      const detail = (event as CustomEvent<Partial<ContextFormRequest>>).detail || {};
      const fields = Array.isArray(detail.fields)
        ? detail.fields
            .map((field) => {
              const rawField = field as ContextFormField & { ui_control?: unknown };
              return {
                id: String(rawField?.id || "").trim(),
                label: String(rawField?.label || rawField?.id || "").trim(),
                type: rawField?.type ? String(rawField.type) : "",
                required: Boolean(rawField?.required),
                sensitive: Boolean(rawField?.sensitive),
                uiControl: rawField?.uiControl ? String(rawField.uiControl) : rawField?.ui_control ? String(rawField.ui_control) : "",
                placeholder: rawField?.placeholder ? String(rawField.placeholder) : "",
                default: rawField?.default,
                candidates: normalizeContextCandidates(rawField?.candidates),
              };
            })
            .filter((field) => field.id && field.label)
        : [];
      if (fields.length === 0) return;
      const key = contextRequestKey(detail.artifactId ? String(detail.artifactId) : undefined, fields);
      if (!detail.force && dismissedContextRequestKeysRef.current.has(key)) {
        return;
      }
      setContextRequest({
        artifactId: detail.artifactId ? String(detail.artifactId) : undefined,
        manualId: detail.manualId ? String(detail.manualId) : undefined,
        workflowId: detail.workflowId ? String(detail.workflowId) : undefined,
        submitAction: detail.submitAction ? String(detail.submitAction) : undefined,
        key,
        title: String(detail.title || "补充运维信息"),
        summary: detail.summary ? String(detail.summary) : "",
        contextText: detail.contextText ? String(detail.contextText) : "",
        fields,
      });
    }
    window.addEventListener("aiops:composer-context-request", handleContextRequest);
    return () => window.removeEventListener("aiops:composer-context-request", handleContextRequest);
  }, []);

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
          dismissedContextRequestKeysRef.current.add(contextRequest.key);
          setContextRequest(null);
        }}
        onComplete={() => {
          dismissedContextRequestKeysRef.current.add(contextRequest.key);
          setContextRequest(null);
        }}
      />
    );
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
      data-testid="aiops-composer-shell"
    >
      <div className="mx-auto flex max-w-3xl flex-col gap-2">
        <ComposerBody
          variant={variant}
          isRunning={isRunning}
          state={state}
          threadIsRunning={threadIsRunning}
        />
        {workspace.composerDisabledReason ? (
          <div className="px-1 text-xs text-amber-700">{workspace.composerDisabledReason}</div>
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
    const submission = request.submitAction === "submit_ops_manual_param_form"
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
        variant === "chat" ? "shrink-0 bg-white px-4 pb-4 pt-2 md:pb-6" : "border-t border-zinc-200 bg-white px-4 py-3 lg:px-8",
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
            <div className="truncate text-[15px] font-semibold leading-6 text-slate-950">{request.title}</div>
          </div>
        </div>
        <div className="mt-3 grid gap-3 sm:grid-cols-2">
          {request.fields.map((field) => {
            const control = contextFieldControl(field);
            if (control === "select") {
              return (
                <div key={field.id} className="grid gap-2 rounded-xl border border-slate-200 bg-slate-50/70 p-3 text-sm text-slate-600">
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
                      <option key={`${contextCandidateValue(candidate)}-${index}`} value={contextCandidateValue(candidate)}>
                        {contextCandidateLabel(candidate)}
                      </option>
                    ))}
                  </select>
                </div>
              );
            }
            if (control === "radio_cards") {
              return (
                <div key={field.id} className="grid gap-2 rounded-xl border border-slate-200 bg-slate-50/70 p-3 text-sm text-slate-600">
                  <span className="font-medium">{contextFieldDisplayLabel(field)}</span>
                  <div className="grid gap-2">
                    {field.candidates?.map((candidate, index) => (
                      <label key={`${contextCandidateValue(candidate)}-${index}`} className="flex items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 py-2">
                        <input
                          type="radio"
                          name={field.id}
                          value={contextCandidateValue(candidate)}
                          defaultChecked={index === 0 || contextCandidateValue(candidate) === contextFieldDefaultValue(field)}
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
              <label key={field.id} className="grid gap-2 rounded-xl border border-slate-200 bg-slate-50/70 p-3 text-sm text-slate-600">
                <span className="font-medium">{label}</span>
                <input
                  type={contextFieldInputType(field)}
                  name={field.id}
                  className="h-10 rounded-lg border border-slate-200 bg-white px-3 text-sm text-slate-900 outline-none focus:border-slate-400"
                  placeholder={contextFieldPlaceholder(field)}
                  defaultValue={contextFieldDefaultValue(field)}
                  autoComplete={contextFieldIsSensitive(field) ? "off" : undefined}
                  spellCheck={contextFieldIsSensitive(field) ? false : undefined}
                />
              </label>
            );
          })}
        </div>
        <div className="mt-3 flex justify-end gap-2">
          <Button type="button" variant="outline" size="sm" className="rounded-md" onClick={onCancel}>
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
  const explicit = String(field.uiControl || "").trim().toLowerCase();
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
  if (contextFieldIsSensitive(field) && !/secret|引用|敏感/i.test(field.label)) {
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
    return field.placeholder || "例如 secret://team/db-password，避免填写明文密码";
  }
  if (id === "target_location") return "留空使用当前选择主机";
  if (id === "symptom") return field.placeholder || "指标、日志、报错、Trace/Case ID、时间范围或关键参数";
  return field.placeholder || field.label;
}

function contextFieldDefaultValue(field: ContextFormField) {
  if (contextFieldIsSensitive(field) && !field.candidates?.length) return "";
  if (field.default !== undefined && field.default !== null) return String(field.default);
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
  const type = String(field.type || "").trim().toLowerCase();
  const control = String(field.uiControl || "").trim().toLowerCase();
  return (
    Boolean(field.sensitive) ||
    type === "secret_ref" ||
    type === "secret" ||
    control === "secret_ref" ||
    control === "secret" ||
    /(^|[_-])(password|passwd|secret|token|credential|api[_-]?key)([_-]|$)/i.test(id)
  );
}

function contextCandidateValue(candidate: NonNullable<ContextFormField["candidates"]>[number]) {
  return String(candidate.value ?? candidate.label ?? "");
}

function contextCandidateLabel(candidate: NonNullable<ContextFormField["candidates"]>[number]) {
  return String(candidate.label || candidate.value || "候选项");
}

function legacyContextFormSubmit(request: ContextFormRequest, params: Record<string, string>) {
  const lines = request.fields
    .map((field) => {
      const value = params[field.id];
      return value ? `${contextFieldSubmitLabel(field)}：${value}` : "";
    })
    .filter(Boolean);
  const contextLine = request.contextText?.trim() ? [`关联上下文：${request.contextText.trim()}`] : [];
  return {
    text: `补充必要信息，继续下一步自动排查：\n${[...contextLine, ...lines].join("\n")}`,
    metadata: {
      opsManualAction: "submit_required_context",
      ...(request.artifactId ? { sourceArtifactId: request.artifactId } : {}),
    },
  };
}

function normalizeContextCandidates(value: unknown): ContextFormField["candidates"] {
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
        confidence: typeof record.confidence === "number" ? record.confidence : undefined,
        evidence: record.evidence ? String(record.evidence) : undefined,
      };
    })
    .filter((candidate): candidate is NonNullable<ContextFormField["candidates"]>[number] => Boolean(candidate && (candidate.value !== undefined || candidate.label)));
}

function contextRequestKey(artifactId: string | undefined, fields: ContextFormField[]) {
  return `${artifactId || "unknown"}:${fields.map((field) => field.id).join("|")}`;
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
  const Icon = confirmation.action === "generate_ops_manual_candidate" ? FileText : Wrench;
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
        variant === "chat" ? "shrink-0 bg-white px-4 pb-4 pt-2 md:pb-6" : "border-t border-zinc-200 bg-white px-4 py-3 lg:px-8",
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
            <div className="mt-1 text-[15px] font-semibold leading-6 text-slate-950">{confirmation.title}</div>
            <p className="mt-1 text-sm leading-6 text-slate-600">{copy.description}</p>
          </div>
        </div>
        <div className="mt-4 flex justify-end gap-2">
          <Button type="button" variant="outline" size="sm" className="rounded-md" onClick={onCancel}>
            取消
          </Button>
          <Button type="button" size="sm" className="rounded-md bg-slate-950 text-white hover:bg-slate-800" onClick={confirm}>
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
  return {
    description: `将基于「${sourceTitle}」生成工作流候选，仍需验证和发布检查后才能进入运维手册库。`,
    confirmLabel: "确认生成",
    message: `确认生成工作流候选：${sourceTitle}`,
  };
}

function selectComposerApproval(state: AiopsTransportState): AiopsTransportApproval | undefined {
  const approvals = Object.values(state.pendingApprovals || {}).filter((approval) => {
    const status = approval.status?.trim();
    return !status || status === "pending" || status === "blocked";
  });
  if (approvals.length === 0) {
    return undefined;
  }
  const currentTurnID = state.currentTurnId?.trim();
  const currentTurnApproval = approvals.find((approval) => approval.turnId?.trim() === currentTurnID);
  if (currentTurnApproval) {
    return currentTurnApproval;
  }
  return approvals.sort((a, b) => (b.requestedAt || "").localeCompare(a.requestedAt || ""))[0];
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
  const inputRef = useRef<HTMLTextAreaElement | null>(null);

  useEffect(() => {
    inputRef.current?.focus();
  }, [workspace.composerFocusNonce]);

  return (
    <ComposerPrimitive.Root
      className={
        variant === "chat"
          ? "flex flex-col gap-2 rounded-[1.75rem] border border-slate-200 bg-white p-2 shadow-[0_10px_28px_rgba(15,23,42,0.10)] transition-shadow focus-within:border-slate-300 focus-within:shadow-[0_12px_36px_rgba(15,23,42,0.14)]"
          : "mx-auto flex max-w-5xl items-end gap-2"
      }
    >
      <ComposerPrimitive.Input asChild submitOnEnter>
        <Textarea
          ref={inputRef}
          data-testid="omnibar-input"
          rows={1}
          placeholder="输入你的问题或任务"
          disabled={Boolean(workspace.composerDisabledReason) || isRunning}
          className={
            variant === "chat"
              ? "max-h-40 min-h-12 resize-none border-0 bg-transparent px-3 py-2 text-[16px] leading-7 shadow-none focus-visible:ring-0 md:text-[16px]"
              : "max-h-44 min-h-11 resize-none rounded-lg border-zinc-300 bg-zinc-50 text-sm"
          }
        />
      </ComposerPrimitive.Input>

      <div className={variant === "chat" ? "flex shrink-0 items-center justify-between" : "mb-1 flex shrink-0 items-center gap-2"}>
        <span className="text-xs text-slate-400 pl-1">{workspace.llmLabel}</span>
        <TargetAwareSendButton variant={variant} isRunning={isRunning} state={state} threadIsRunning={threadIsRunning} />
      </div>
    </ComposerPrimitive.Root>
  );
}

function TargetAwareSendButton({
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
  const api = useAssistantApi();
  const composer = useComposerRuntime();
  const composerState = useComposer((snapshot) => snapshot);
  const sendCommand = useAssistantTransportSendCommand();
  const commands = useAiopsTransportCommands();
  const target = useSessionTargetContext();
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

  const disabled = (stopping && !forceStopVisible) || (!isRunning && (!composerState?.text?.trim() || Boolean(workspace.composerDisabledReason)));
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
      aria-label={forceStopVisible ? "强制停止" : stopping ? "正在停止" : isRunning ? "停止生成" : "send message"}
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
        const text = composer.getState().text.trim();
        if (!text) return;
        composer.setText("");
        const command = {
          type: "add-message",
          message: {
            role: "user",
            metadata: target.metadata,
            ...(target.hostId ? { hostId: target.hostId } : {}),
            parts: [{ type: "text", text }],
          },
        } as Parameters<typeof sendCommand>[0];
        sendCommand(command);
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

function BlockedApprovalComposer({ approval }: { approval: AiopsTransportApproval }) {
  const commands = useAiopsTransportCommands();
  const [decision, setDecision] = useState<"accept" | "accept_session" | "reject">("accept");
  const [submittingDecision, setSubmittingDecision] = useState<"accept" | "accept_session" | "reject" | null>(null);
  const [submitError, setSubmitError] = useState("");
  const commandText = approval.command || approval.reason || approval.id;
  const isSubmitting = Boolean(submittingDecision) && !submitError;

  useEffect(() => {
    setDecision("accept");
    setSubmittingDecision(null);
    setSubmitError("");
  }, [approval.id]);

  function submitDecision(nextDecision: "accept" | "accept_session" | "reject" = decision) {
    if (isSubmitting) {
      return;
    }
    setSubmittingDecision(nextDecision);
    setSubmitError("");
    try {
      commands.approvalDecision(approval.id, nextDecision);
    } catch (error) {
      setSubmittingDecision(null);
      setSubmitError(error instanceof Error ? error.message : "提交审批失败，请重试");
    }
  }

  return (
    <div className="shrink-0 bg-white px-4 pb-4 pt-2 md:pb-6" data-testid="codex-approval-inline">
      <div className="mx-auto max-w-3xl rounded-[1.75rem] border border-slate-200 bg-white p-4 shadow-[0_10px_28px_rgba(15,23,42,0.10)]">
        <div className="space-y-3">
          <div>
            <div className="text-xs font-medium text-slate-400">等待审批</div>
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
            <div className="flex items-center gap-2 rounded-xl bg-blue-50 px-3 py-2 text-sm text-blue-700" role="status">
              <LoaderCircle className="h-4 w-4 animate-spin" />
              <span>已提交确认，正在继续执行...</span>
            </div>
          ) : null}
          {submitError ? (
            <div className="rounded-xl bg-red-50 px-3 py-2 text-sm text-red-700" role="alert">
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
