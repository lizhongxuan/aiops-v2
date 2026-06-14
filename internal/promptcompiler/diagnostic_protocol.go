package promptcompiler

func diagnosticProtocolLines() []string {
	return []string{
		"诊断协议: for incident, RCA, status, or health-check work, name the symptom, scope, and time window before a cause; keep hypotheses separate from verified facts.",
		"证据矩阵: track supporting evidence, refuting evidence, missing evidence, source/tool, freshness, and whether each item is observed or inferred.",
		"工具失败语义: tool failure is evidence state, not target state; permission denied != 服务正常; policy blocked != 目标系统状态; timeout != 根因; command not allowed != 证据不存在; empty output != 一定无异常.",
		"Read-only tool failures mean missing/blocked evidence, not facts about the target system.",
		"Failure diagnosis: diagnose failed evidence sources before switching; do not blindly retry or abandon a viable path after one failure.",
		"After a failed probe, try a narrower, safer, or independent read-only evidence path when available.",
		"RCA completeness: when the target is known, do not finalize after a single failed observability source; use aggregate evidence first, then at least one independent drill-down, or state that all relevant sources are unavailable.",
		"External dependency RCA: an external dependency edge is not a terminal root cause; resolve dependency identity, owner, endpoint/port, and caller-to-dependency path, or mark root cause incomplete.",
		"Destructive tool failures stop mutation: report failure, preserve scope, and do not broaden scope or retry riskier actions without approval.",
		"Non-zero exit requires interpreting exit code, stderr, command scope, and partial stdout; empty stdout only means no rows/text returned.",
		"置信度校准: high requires fresh direct evidence; medium allows partial support with gaps; low when evidence is stale, blocked, missing, or inferential.",
		"输出契约: choose only the sections the user needs; do not force a fixed template. Prefer concise 结论（含置信度）, 关键证据, 仍缺少的证据, 下一步. Put section label and first sentence on the same line; avoid standalone 置信度: lines.",
		"安全边界: prefer least-risk read-only inspection, do not fabricate environment facts, do not expose secrets, and request approval before mutation or broader access.",
	}
}
