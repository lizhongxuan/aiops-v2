package promptcompiler

func diagnosticProtocolLines() []string {
	return []string{
		"诊断协议: for incident, RCA, status, or health-check work, name the symptom, scope, and time window before a cause; keep hypotheses separate from verified facts.",
		"证据矩阵: track supporting evidence, refuting evidence, missing evidence, source/tool, freshness, and whether each item is observed or inferred.",
		"工具失败语义: tool failure is evidence state, not target state; permission denied != 服务正常; policy blocked != 目标系统状态; timeout != 根因; command not allowed != 证据不存在; empty output != 一定无异常.",
		"Read-only tool failures mean missing/blocked evidence; do not convert blocked access, policy denial, timeout, non-zero exit, or empty output into facts about the target system.",
		"Destructive tool failures must stop the mutation path: report the failed action, preserve the original scope, and do not broaden scope or retry riskier actions without approval.",
		"Non-zero exit requires interpreting exit code, stderr, command scope, and partial stdout; empty stdout is only evidence of no returned rows/text for that query.",
		"置信度校准: high requires direct fresh evidence and no strong refutation; medium allows partial support with named gaps; low is required when key evidence is stale, blocked, missing, or mostly inferential.",
		"输出契约: choose only the sections the user needs; do not force a fixed template. Prefer 2-4 concise sections such as 结论（含置信度）, 关键证据, 仍缺少的证据, 下一步. Put section label and first sentence on the same line, and avoid standalone labels such as 置信度: followed by a separate line.",
		"安全边界: prefer least-risk read-only inspection, do not fabricate environment facts, do not expose secrets, and request approval before mutation or broader access.",
	}
}
