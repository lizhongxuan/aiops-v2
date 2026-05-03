package promptdiag

import (
	"fmt"
	"strings"

	"aiops-v2/internal/eval"
)

func applyRules(score eval.CaseScore, expected eval.Case, evidence EvidenceSummary, movement string, baseline eval.CaseScore, promptSizeWarning int) []RuleHit {
	if promptSizeWarning <= 0 {
		promptSizeWarning = 30000
	}
	var hits []RuleHit
	add := func(ruleID, severity, rootCause, message, detail string) {
		hits = append(hits, RuleHit{
			RuleID:    ruleID,
			Severity:  severity,
			RootCause: rootCause,
			Message:   message,
			Evidence:  detail,
		})
	}
	if strings.TrimSpace(score.Error) != "" {
		root := RootCauseContext
		errorText := strings.ToLower(score.Error)
		if strings.Contains(errorText, "subscription_not_found") || strings.Contains(errorText, "403") || strings.Contains(errorText, "api key") {
			root = RootCauseModelOrProvider
		}
		add("run-error", "error", root, "eval case 运行错误，优先修运行链路或模型配置。", score.Error)
	}
	if !score.Passed && len(evidence.TraceFiles) == 0 {
		add("trace-missing", "warning", RootCauseContext, "没有找到关联 Prompt Trace，先确认 trace 开关和 -trace-dir 是否指向本次 eval。", "trace files: 0")
	}
	if len(evidence.TraceFiles) > 0 && !evidence.HasUserMessage {
		add("missing-user-message", "error", RootCauseContext, "模型输入里没有 user message，这不是 prompt 优化问题。", "trace modelInput[] 缺少 providerRole=user")
	}
	if len(evidence.ExpectedTools) > 0 {
		missingVisible := missingStrings(evidence.ExpectedTools, evidence.VisibleTools)
		if len(missingVisible) > 0 {
			add("expected-tool-not-visible", "error", RootCauseToolOrPolicy, "case 期望工具不可见，先查 tool assembly、权限或 policy。", "missing visible tools: "+strings.Join(missingVisible, ", "))
		} else if len(evidence.MissingExpectedTools) > 0 {
			add("expected-tool-not-called", "warning", RootCausePromptOrToolDescription, "工具可见但模型没有调用，优先改 developer 工具选择规则或 tool description。", "missing tool calls: "+strings.Join(evidence.MissingExpectedTools, ", "))
		}
	}
	if evidence.ToolCallCount > 0 && evidence.ToolResultCount == 0 {
		add("tool-result-missing", "error", RootCauseContext, "存在 tool_call 但没有 tool_result，先查 runtime/tool result 注入。", fmt.Sprintf("toolCalls=%d toolResults=%d", evidence.ToolCallCount, evidence.ToolResultCount))
	}
	if evidence.FailedToolResultCount > 0 {
		add("failed-tool-result", "warning", RootCausePrompt, "存在失败 tool_result，最终答案应明确失败证据和 fallback，而不是盲目重试。", "failed tools: "+strings.Join(evidence.FailedToolNames, ", "))
	}
	if failedCheck(score, "expectedEvidence") && evidence.ToolResultCount > 0 {
		add("evidence-not-used", "warning", RootCausePrompt, "工具结果存在但 evidence 断言失败，检查最终回答是否引用了最新工具证据。", "failed check: expectedEvidence")
	}
	if failedCheck(score, "maxIterations") {
		add("max-iterations", "error", RootCauseCompletionGate, "超过最大模型轮次，优先收紧停止条件或 completion gate。", "failed check: maxIterations")
	}
	if failedCheck(score, "maxToolCalls") {
		add("max-tool-calls", "warning", RootCausePrompt, "工具调用过多，优先收紧工具调用边界和已有证据足够时的停止规则。", "failed check: maxToolCalls")
	}
	if failedCheck(score, "planPresence") || failedCheck(score, "mustNotHavePlan") || failedCheck(score, "expectedPlanStatuses") {
		add("plan-policy", "warning", RootCausePrompt, "plan 行为不符合 case，优先调整 simple/complex 任务的 plan 触发规则。", "failed plan-related check")
	}
	if failedCheck(score, "mustInclude") || failedCheck(score, "mustNotInclude") || failedCheck(score, "mustMentionFiles") {
		add("answer-content", "warning", RootCausePrompt, "最终回答内容不符合断言，检查 prompt 是否明确要求输出这些用户价值证据。", "failed checks: "+strings.Join(failedChecks(score), ", "))
	}
	if evidence.PromptSizeChars > promptSizeWarning {
		add("prompt-too-large", "warning", RootCauseContext, "prompt size 偏大，优先检查历史上下文、tool output 和 memory 是否需要裁剪。", fmt.Sprintf("promptSizeChars=%d threshold=%d", evidence.PromptSizeChars, promptSizeWarning))
	}
	if movement == eval.ComparisonWorse {
		add("baseline-regression", "error", RootCauseRegression, "current 比 baseline 退化，不能直接保留这次 prompt 改动。", fmt.Sprintf("baseline %.2f -> current %.2f", baseline.Score, score.Score))
	}
	if movement != "" && movement != eval.ComparisonNew && baseline.CaseID != "" {
		baseFP := lastFingerprint(baseline.PromptFingerprints)
		curFP := lastFingerprint(score.PromptFingerprints)
		if baseFP != nil && curFP != nil && baseFP["stableHash"] == curFP["stableHash"] && baseFP["developerHash"] == curFP["developerHash"] && baseFP["toolRegistryHash"] == curFP["toolRegistryHash"] && movement != eval.ComparisonSame {
			add("prompt-hash-unchanged", "warning", RootCauseDeploymentOrConfig, "分数变化但稳定 prompt hash 没变，先确认是否是 runtime/context/model 波动，而不是 prompt 改动生效。", "stable/developer/toolRegistry hashes unchanged")
		}
	}
	if len(hits) == 0 && !score.Passed {
		add("unclassified-failure", "warning", RootCauseUnknown, "没有命中硬规则，需要人工打开 trace 和 artifacts 继续判断。", "failed checks: "+strings.Join(failedChecks(score), ", "))
	}
	return hits
}

func failedCheck(score eval.CaseScore, name string) bool {
	for _, check := range score.Checks {
		if check.Name == name && !check.Passed {
			return true
		}
	}
	return false
}

func likelyRootCause(hits []RuleHit, passed bool, movement string) string {
	if movement == eval.ComparisonWorse {
		return RootCauseRegression
	}
	if len(hits) == 0 {
		if passed {
			return ""
		}
		return RootCauseUnknown
	}
	for _, severity := range []string{"error", "warning", "info"} {
		for _, hit := range hits {
			if hit.Severity == severity && strings.TrimSpace(hit.RootCause) != "" {
				return hit.RootCause
			}
		}
	}
	return hits[0].RootCause
}

func suggestionsFor(hits []RuleHit) []Suggestion {
	var out []Suggestion
	for _, hit := range hits {
		switch hit.RuleID {
		case "expected-tool-not-called":
			out = appendSuggestion(out, Suggestion{
				Area:      "prompt/tool_description",
				Action:    "在 developer rules 或 tool description 里补“何时必须调用该工具”的触发条件，并重跑该 case。",
				Rationale: hit.Message,
			})
			continue
		case "expected-tool-not-visible":
			out = appendSuggestion(out, Suggestion{
				Area:      "tool/policy",
				Action:    "检查 tool assembly、permission mode、policy gate，先让期望工具进入 visible tools。",
				Rationale: hit.Message,
			})
			continue
		case "prompt-too-large":
			out = appendSuggestion(out, Suggestion{
				Area:      "context",
				Action:    "裁剪历史、压缩 tool result、去掉重复 context；先让 prompt size 降到阈值以下再评估 prompt 文案。",
				Rationale: hit.Message,
			})
			continue
		case "failed-tool-result":
			out = appendSuggestion(out, Suggestion{
				Area:      "prompt/result_summary",
				Action:    "要求最终回答引用失败 tool_result，并说明 fallback 或限制；不要继续盲目重试同一失败路径。",
				Rationale: hit.Message,
			})
			continue
		case "plan-policy":
			out = appendSuggestion(out, Suggestion{
				Area:      "prompt/plan_policy",
				Action:    "把 simple chat 与复杂执行任务的 plan 触发条件拆清楚，并用 mustHavePlan/mustNotHavePlan case 回归。",
				Rationale: hit.Message,
			})
			continue
		case "max-iterations", "max-tool-calls":
			out = appendSuggestion(out, Suggestion{
				Area:      "completion_gate",
				Action:    "补“已有足够证据就停止调用工具并最终回答”的规则，同时检查 runtime completion gate。",
				Rationale: hit.Message,
			})
			continue
		case "run-error":
			out = appendSuggestion(out, Suggestion{
				Area:      "model/provider",
				Action:    "先修模型 URL、模型名、API key、订阅或本地 server 连接，再重跑同一批 case。",
				Rationale: hit.Message,
			})
			continue
		}
		switch hit.RootCause {
		case RootCausePrompt:
			out = appendSuggestion(out, Suggestion{
				Area:      "prompt",
				Action:    "只改一条 developer/tool/completion 规则，并用同一批 P0 case 重跑。",
				Rationale: hit.Message,
			})
		case RootCausePromptOrToolDescription:
			out = appendSuggestion(out, Suggestion{
				Area:      "prompt/tool_description",
				Action:    "先看 Tools 和 Prompt 层，确认工具说明是否清楚；如果工具可见但未调用，补工具选择条件。",
				Rationale: hit.Message,
			})
		case RootCauseToolOrPolicy:
			out = appendSuggestion(out, Suggestion{
				Area:      "tool/policy",
				Action:    "先修 tool assembly、权限或 policy，让期望工具在 trace 的 visible tools 中出现。",
				Rationale: hit.Message,
			})
		case RootCauseContext:
			out = appendSuggestion(out, Suggestion{
				Area:      "runtime/context",
				Action:    "先修用户输入、tool result、resume 或 context 注入链路，不要靠 prompt 补救。",
				Rationale: hit.Message,
			})
		case RootCauseCompletionGate:
			out = appendSuggestion(out, Suggestion{
				Area:      "completion_gate",
				Action:    "收紧已有证据足够时的停止规则，并补 maxIterations/maxToolCalls 回归 case。",
				Rationale: hit.Message,
			})
		case RootCauseDeploymentOrConfig:
			out = appendSuggestion(out, Suggestion{
				Area:      "deployment/config",
				Action:    "重启 aiops server，确认 Prompt Fingerprint 对应 hash 已变化后再判断模型行为。",
				Rationale: hit.Message,
			})
		case RootCauseRegression:
			out = appendSuggestion(out, Suggestion{
				Area:      "experiment",
				Action:    "先不要保留改动，打开 compare.zh.md 查看退化 case，再缩小 prompt 改动范围。",
				Rationale: hit.Message,
			})
		case RootCauseModelOrProvider:
			out = appendSuggestion(out, Suggestion{
				Area:      "model/provider",
				Action:    "先修本地 LLM URL、模型名、API key 或订阅权限，再重跑 eval。",
				Rationale: hit.Message,
			})
		}
	}
	return out
}
