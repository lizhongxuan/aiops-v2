import { useEffect, useState } from "react";
import { useAssistantTransportState } from "@assistant-ui/react";

import { confirmRunnerCandidate, prepareExperiencePackCandidate, retrieveExperiencePacks, type ExperienceMatchView } from "@/api/experiencePacks";
import { Button } from "@/components/ui/button";
import { useAiopsTransportCommands } from "@/transport/useAiopsTransportCommands";
import type {
  AiopsTransportAgentUiArtifact,
  AiopsTransportExperiencePackSuggestion,
  AiopsTransportState,
} from "@/transport/aiopsTransportTypes";

export function ExperiencePackChatArtifacts({
  draftText,
  onSelectSuggestion,
}: {
  draftText: string;
  onSelectSuggestion: (suggestion: AiopsTransportExperiencePackSuggestion) => void;
}) {
  const state = useAssistantTransportState() as AiopsTransportState;
  const suggestions = (state.experiencePackSuggestions || []).filter((item) => item && item.type && item.label);
  const intent = detectExperiencePackIntent(draftText);
  if (!intent && !suggestions.length) return null;

  return (
    <div className="grid gap-2" data-testid="experience-pack-agent-ui-region">
      {intent ? <ExperiencePackRequestPreview intent={intent} /> : null}
      {suggestions.length ? (
        <section className="rounded-lg border border-slate-200 bg-white p-3 text-sm shadow-sm" data-testid="experience-pack-suggestion-bar">
          <div className="font-medium text-slate-950">可沉淀的运维资产</div>
          <p className="mt-1 text-xs leading-5 text-slate-500">
            仅在本次运维闭环具备复用价值时展示。点击后会进入二次确认，不会自动执行 Runner，也不会直接写入经验库。
          </p>
          <div className="mt-3 flex flex-wrap gap-2">
            {suggestions.map((suggestion) => (
              <Button
                key={suggestion.id || suggestion.type}
                type="button"
                size="sm"
                variant="outline"
                className="rounded-full border-slate-200 bg-white"
                onClick={() => onSelectSuggestion(suggestion)}
              >
                {suggestionDisplayLabel(suggestion)}
              </Button>
            ))}
          </div>
        </section>
      ) : null}
    </div>
  );
}

export function ExperiencePackSuggestionConfirmation({
  suggestion,
  onCancel,
}: {
  suggestion: AiopsTransportExperiencePackSuggestion;
  onCancel: () => void;
}) {
  const [status, setStatus] = useState("");
  const commands = useAiopsTransportCommands();

  async function confirmSelected() {
    setStatus("正在生成候选...");
    try {
      const payload = {
        ...experiencePackSuggestionPayload(suggestion),
        confirmationToken: suggestion.id || suggestion.type,
        suggestionType: suggestion.type,
      };
      if (suggestion.type === "generate_runner_workflow_candidate") {
        const result = await confirmRunnerCandidate(payload);
        commands.insertAgentUiArtifact(runnerWorkflowCandidateArtifact(suggestion, result));
        setStatus("已生成工作流草稿，等待人工审核、Dry Run 和验证");
      } else {
        const result = await prepareExperiencePackCandidate(payload);
        commands.insertAgentUiArtifact(experiencePackCandidateArtifact(suggestion, result));
        setStatus("已生成经验包候选，审核通过后才会进入经验库");
      }
      onCancel();
    } catch (cause) {
      setStatus((cause as Error).message || "候选生成失败");
    }
  }

  return (
    <div className="shrink-0 bg-white px-4 pb-4 pt-2 md:pb-6" data-testid="experience-pack-confirmation-composer">
      <div className="mx-auto max-w-3xl rounded-[1.75rem] border border-slate-200 bg-white p-4 shadow-[0_10px_28px_rgba(15,23,42,0.10)]" role="dialog" aria-label="确认生成经验包候选">
        <div className="font-medium text-slate-950">确认{suggestionDisplayLabel(suggestion)}</div>
        <p className="mt-1 text-sm leading-6 text-slate-700">{suggestion.reason || "将基于当前运维过程生成候选资产，确认前不会写入经验包。"}</p>
        <p className="mt-1 text-xs leading-5 text-slate-500">确认前不会写入经验包，也不会自动执行 Runner。生成后仍需人工审核、Dry Run、受控执行和恢复验证。</p>
        <div className="mt-2 text-xs text-slate-500">
          引用来源：{(suggestion.sourceRefs || suggestion.source_refs || []).join("、") || "当前 AI Chat 会话"}
        </div>
        {status ? <div className="mt-2 text-xs text-slate-500">{status}</div> : null}
        <div className="mt-4 flex justify-end gap-2">
          <Button type="button" size="sm" variant="outline" onClick={onCancel}>取消</Button>
          <Button type="button" size="sm" onClick={() => void confirmSelected()}>确认生成</Button>
        </div>
      </div>
    </div>
  );
}

function ExperiencePackRequestPreview({ intent }: { intent: { title: string; rawText: string; service: string; operation: string } }) {
  const [matches, setMatches] = useState<ExperienceMatchView[]>([]);
  const [status, setStatus] = useState<"idle" | "loading" | "ready" | "error">("idle");

  useEffect(() => {
    let cancelled = false;
    const timer = window.setTimeout(() => {
      setStatus("loading");
      retrieveExperiencePacks({
        userText: intent.rawText,
        query: intent.rawText,
        signals: extractIntentSignals(intent.rawText),
        environment: extractEnvironment(intent.rawText),
        metadata: { source: "ai-chat-draft-preview", service: intent.service, operation: intent.operation },
      })
        .then((result) => {
          if (cancelled) return;
          setMatches(result.items.slice(0, 2));
          setStatus("ready");
        })
        .catch(() => {
          if (cancelled) return;
          setMatches([]);
          setStatus("error");
        });
    }, 250);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [intent.rawText, intent.service, intent.operation]);

  return (
    <section data-testid="experience-pack-intent-preview" className="rounded-lg border border-slate-200 bg-white p-3 text-sm shadow-sm">
      <div className="font-medium text-slate-950">经验包预检：{intent.title}</div>
      <div className="mt-3 grid gap-2 lg:grid-cols-2">
        <div className="rounded-xl bg-slate-50 p-3">
          <div className="font-medium text-slate-700">Skill + Runner</div>
          <p className="mt-1 text-xs leading-5 text-slate-600">Runner 负责怎么执行；Skill 负责为什么这么做、什么时候适用、环境不同时怎么调整、怎么验证和回滚。</p>
        </div>
        <div className="rounded-xl bg-slate-50 p-3">
          <div className="font-medium text-slate-700">GEP 治理</div>
          <p className="mt-1 text-xs leading-5 text-slate-600">GEP 记录经验来自哪次故障、哪个 Gene、哪个环境、失败警告、验证方式，以及新旧版本关系。</p>
        </div>
        <div className="rounded-xl bg-slate-50 p-3">
          <div className="font-medium text-slate-700">混合检索</div>
          <p className="mt-1 text-xs leading-5 text-slate-600">结构化条件过滤 + 关键词/BM25 + 向量语义检索 + 环境指纹匹配 + GEP Gene signals_match；上万经验包使用 PostgreSQL + pgvector。</p>
        </div>
        <div className="rounded-xl bg-slate-50 p-3">
          <div className="font-medium text-slate-700">受控使用</div>
          <p className="mt-1 text-xs leading-5 text-slate-600">高度匹配时推荐经验包、执行计划、Runner、风险范围和验证方式；环境不同则先生成适配计划和 Runner 变体。</p>
        </div>
      </div>
      <div className="mt-3 rounded-xl border border-slate-100 bg-slate-50 p-3">
        <div className="font-medium text-slate-700">实时检索结果</div>
        {status === "loading" ? <p className="mt-1 text-xs text-slate-500">正在检索可复用经验...</p> : null}
        {status === "error" ? <p className="mt-1 text-xs text-amber-700">检索服务暂不可用，发送后仍会在服务端重新检索。</p> : null}
        {status === "ready" && matches.length === 0 ? <p className="mt-1 text-xs text-slate-500">当前未命中已审核可检索经验；如果本次运维成功闭环，后续可生成候选经验包。</p> : null}
        {matches.length ? (
          <div className="mt-2 grid gap-2">
            {matches.map((match) => (
              <div key={match.packId} className="rounded-lg border border-emerald-100 bg-white p-2">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div className="font-medium text-slate-900">{match.skill.name || match.packId}</div>
                  <span className="rounded-full border border-emerald-200 bg-emerald-50 px-2 py-0.5 text-xs text-emerald-700">命中 {formatConfidence(match.confidence)}</span>
                </div>
                <p className="mt-1 text-xs leading-5 text-slate-500">将返回推荐经验包、执行计划、Runner 工作流、风险范围和验证方式，仍需你确认后使用。</p>
              </div>
            ))}
          </div>
        ) : null}
      </div>
    </section>
  );
}

function detectExperiencePackIntent(draftText: string) {
  const value = draftText.trim().toLowerCase();
  if (value.length < 8) return null;
  const opsKeywords = ["pg", "postgres", "postgresql", "mysql", "k8s", "kubernetes", "备份", "部署", "主从", "集群", "异常", "排查", "恢复", "服务"];
  if (!opsKeywords.some((keyword) => value.includes(keyword))) return null;
  const middleware = value.includes("mysql")
    ? "MySQL"
    : value.includes("k8s") || value.includes("kubernetes")
      ? "Kubernetes"
      : value.includes("pg") || value.includes("postgres")
        ? "PostgreSQL"
        : "运维";
  const action = value.includes("备份")
    ? "备份"
    : value.includes("部署") || value.includes("主从")
      ? "部署"
      : value.includes("异常") || value.includes("排查") || value.includes("恢复")
        ? "排障恢复"
        : "运维";
  return { title: `${middleware} ${action}请求`, rawText: draftText.trim(), service: middleware.toLowerCase(), operation: action };
}

function suggestionDisplayLabel(suggestion: AiopsTransportExperiencePackSuggestion) {
  if (suggestion.type === "generate_runner_workflow_candidate") return "生成工作流";
  if (suggestion.type === "generate_experience_pack_candidate") return "生成经验包";
  return suggestion.label;
}

function experiencePackSuggestionPayload(suggestion: AiopsTransportExperiencePackSuggestion) {
  const metadata = asRecord(suggestion.metadata);
  const commands = stringArray(metadata.commands);
  const signals = stringArray(metadata.signals);
  return {
    caseId: text(suggestion.caseId || metadata.caseId),
    packId: text(suggestion.packId || metadata.packId),
    title: text(suggestion.title || metadata.title),
    summary: text(suggestion.summary || metadata.summary),
    service: text(suggestion.service || metadata.service),
    environment: text(suggestion.environment || metadata.environment),
    chatSessionId: text(metadata.chatSessionId),
    commands,
    metadata: {
      ...metadata,
      commands,
      signals,
      sourceRefs: suggestion.sourceRefs || suggestion.source_refs || [],
      suggestionId: suggestion.id || suggestion.type,
    },
  };
}

function experiencePackCandidateArtifact(
  suggestion: AiopsTransportExperiencePackSuggestion,
  result: unknown,
): AiopsTransportAgentUiArtifact {
  const response = asRecord(result);
  const candidate = asRecord(response.candidate || response.experience_pack || response.pack || response);
  const candidateId = text(response.candidate_id || response.candidateId || candidate.candidate_id || candidate.candidateId || suggestion.id);
  const packId = text(response.pack_id || response.packId || candidate.pack_id || candidate.packId || candidate.id);
  const title = text(response.title || candidate.title || candidate.name) || suggestion.label;
  const reviewStatus = text(response.review_status || response.reviewStatus || candidate.review_status || candidate.reviewStatus || candidate.status) || "candidate";
  const artifactId = `experience-pack-candidate-${candidateId || packId || suggestion.id || suggestion.type}`;
  return {
    id: artifactId,
    type: "experience_pack_candidate",
    titleZh: "经验包候选已生成",
    summaryZh: `${title} 已生成候选资产，审核通过后才能启用。`,
    status: "ready",
    source: "ai-chat",
    redactionStatus: "redacted",
    inlineData: {
      candidateId,
      packId,
      reviewStatus,
      suggestionType: suggestion.type,
      sourceRefs: suggestion.sourceRefs || suggestion.source_refs || [],
    },
    actions: [
      {
        id: "review-experience-pack-candidate",
        label: "去审核",
        href: "/settings/experience-packs?tab=review",
        mutation: false,
      },
    ],
  };
}

function runnerWorkflowCandidateArtifact(
  suggestion: AiopsTransportExperiencePackSuggestion,
  result: unknown,
): AiopsTransportAgentUiArtifact {
  const response = asRecord(result);
  const workflowId = text(response.workflowId || response.workflow_id || response.id || suggestion.id);
  const workflowName = text(response.workflowName || response.workflow_name || response.title || response.name) || "Runner Workflow 草稿";
  const graph = asRecord(response.graph);
  const nodes = extractRunnerNodes(graph);
  return {
    id: `runner-workflow-candidate-${workflowId || suggestion.id || suggestion.type}`,
    type: "runner_workflow_candidate",
    titleZh: "Runner Workflow 草稿已生成",
    summaryZh: `${workflowName} 已写入 Runner Studio 本地草稿，发布前需要人工审核、Dry Run 和验证。`,
    status: "ready",
    source: "ai-chat",
    redactionStatus: "redacted",
    inlineData: {
      workflowId,
      workflowName,
      reviewStatus: "draft",
      suggestionType: suggestion.type,
      nodes,
      graph,
      sourceRefs: suggestion.sourceRefs || suggestion.source_refs || [],
    },
    actions: [],
  };
}

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

function text(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function stringArray(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return value.map((item) => typeof item === "string" ? item.trim() : String(item || "").trim()).filter(Boolean);
}

function extractRunnerNodes(graph: Record<string, unknown>) {
  const nodes = Array.isArray(graph.nodes) ? graph.nodes : [];
  return nodes
    .map((node) => {
      const record = asRecord(node);
      const id = text(record.id);
      const label = text(record.label || record.title || record.name);
      if (!label || label.toLowerCase() === "start" || label.toLowerCase() === "end") return null;
      const step = asRecord(record.step);
      return {
        id: id || label,
        title: label,
        detail: text(step.action) ? `${text(step.action)} · ${text(asRecord(step.args).script) || "等待参数审核"}` : "由后端 Runner graph 生成的只读节点。",
      };
    })
    .filter(Boolean);
}

function extractIntentSignals(value: string) {
  const normalized = value.toLowerCase();
  return ["postgres", "postgresql", "pg", "mysql", "redis", "kubernetes", "k8s", "主从", "备份", "部署", "异常", "恢复", "p95", "coroot"]
    .filter((signal) => normalized.includes(signal.toLowerCase()));
}

function extractEnvironment(value: string) {
  const normalized = value.toLowerCase();
  if (normalized.includes("prod") || value.includes("生产")) return "prod";
  if (normalized.includes("staging") || value.includes("预发")) return "staging";
  return "unknown";
}

function formatConfidence(value: number | null) {
  if (typeof value !== "number" || !Number.isFinite(value)) return "--";
  return `${Math.round(value * 100)}%`;
}
