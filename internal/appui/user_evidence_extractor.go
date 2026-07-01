package appui

import (
	"strings"
)

type UserEvidenceExtraction struct {
	HasEvidence       bool
	UserProhibitsExec bool
	EvidenceKinds     []string
	Commands          []string
	Signals           []string
	RawExcerpt        string
}

func ExtractUserEvidence(input string) UserEvidenceExtraction {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return UserEvidenceExtraction{}
	}
	lower := strings.ToLower(trimmed)
	out := UserEvidenceExtraction{
		UserProhibitsExec: evidenceConstraintNoHostExec(lower),
		RawExcerpt:        truncateEvidenceExcerpt(trimmed, 1200),
	}
	if looksLikeSQLResult(lower) {
		out.HasEvidence = true
		out.EvidenceKinds = appendUniqueEvidenceString(out.EvidenceKinds, "sql_result")
	}
	if looksLikeCommandOutput(lower) {
		out.HasEvidence = true
		out.EvidenceKinds = appendUniqueEvidenceString(out.EvidenceKinds, "command_output")
	}
	if evidenceWeakSignalLogLikeText(lower) {
		out.HasEvidence = true
		out.EvidenceKinds = appendUniqueEvidenceString(out.EvidenceKinds, "log")
	}
	if looksLikeMonitoringEvidence(lower) {
		out.HasEvidence = true
		out.EvidenceKinds = appendUniqueEvidenceString(out.EvidenceKinds, "monitoring")
	}
	if looksLikeStackTraceEvidence(lower) {
		out.HasEvidence = true
		out.EvidenceKinds = appendUniqueEvidenceString(out.EvidenceKinds, "stack_trace")
	}
	if evidenceWeakSignalConfigLikeText(lower) {
		out.HasEvidence = true
		out.EvidenceKinds = appendUniqueEvidenceString(out.EvidenceKinds, "config")
	}
	out.Signals = extractEvidenceSignals(lower)
	if len(out.Signals) > 0 {
		out.HasEvidence = true
	}
	if !out.HasEvidence {
		out.RawExcerpt = ""
	}
	return out
}

func evidenceConstraintNoHostExec(lower string) bool {
	phrases := []string{
		"不要执行",
		"不要连接",
		"不要采集",
		"不要运行",
		"不要操作",
		"不执行",
		"不连接",
		"只基于",
		"仅基于",
		"do not execute",
		"do not connect",
		"don't execute",
		"don't connect",
		"without running",
		"without executing",
		"without connecting",
	}
	for _, phrase := range phrases {
		if strings.Contains(lower, strings.ToLower(phrase)) {
			return true
		}
	}
	return false
}

func looksLikeSQLResult(lower string) bool {
	return strings.Contains(lower, "select ") ||
		strings.Contains(lower, "in_recovery") ||
		strings.Contains(lower, "=>")
}

func looksLikeCommandOutput(lower string) bool {
	return strings.Contains(lower, "control data") ||
		strings.Contains(lower, "database cluster state") ||
		strings.Contains(lower, "latest checkpoint") ||
		strings.Contains(lower, "ls: cannot access") ||
		strings.Contains(lower, "no such file or directory") ||
		strings.Contains(lower, "$ ") ||
		strings.Contains(lower, "# ")
}

func evidenceWeakSignalLogLikeText(lower string) bool {
	return strings.Contains(lower, " log:") ||
		strings.Contains(lower, "日志") ||
		strings.Contains(lower, "warning:") ||
		strings.Contains(lower, "error:") ||
		strings.Contains(lower, "checkpoint too frequent") ||
		strings.Contains(lower, "checkpoints are occurring too frequently") ||
		strings.Contains(lower, "write latency spike") ||
		strings.Contains(lower, "recovery complete") ||
		strings.Contains(lower, "restored log")
}

func looksLikeMonitoringEvidence(lower string) bool {
	return strings.Contains(lower, "cpu usage") ||
		strings.Contains(lower, "memory usage") ||
		strings.Contains(lower, "error rate") ||
		strings.Contains(lower, "p95") ||
		strings.Contains(lower, "p99") ||
		strings.Contains(lower, "slo") ||
		strings.Contains(lower, "latency") ||
		strings.Contains(lower, "qps") ||
		strings.Contains(lower, "prometheus") ||
		strings.Contains(lower, "grafana")
}

func looksLikeStackTraceEvidence(lower string) bool {
	return strings.Contains(lower, "exception in thread") ||
		strings.Contains(lower, "stack trace") ||
		strings.Contains(lower, "traceback (most recent call last)") ||
		strings.Contains(lower, "panic:") ||
		strings.Contains(lower, "goroutine ") ||
		strings.Contains(lower, "\tat ") ||
		strings.Contains(lower, ".java:") ||
		strings.Contains(lower, ".go:")
}

func evidenceWeakSignalConfigLikeText(lower string) bool {
	return strings.Contains(lower, "restore command") ||
		strings.Contains(lower, "restore_command") ||
		strings.Contains(lower, "recovery target") ||
		strings.Contains(lower, "recovery_target")
}

func extractEvidenceSignals(lower string) []string {
	var signals []string
	if looksLikeRecoveryInactiveEvidence(lower) {
		signals = appendUniqueEvidenceString(signals, "database_recovery_inactive")
	}
	if strings.Contains(lower, "archive recovery") {
		signals = appendUniqueEvidenceString(signals, "archive_recovery_active")
	}
	if strings.Contains(lower, "standby.signal") {
		signals = appendUniqueEvidenceString(signals, "standby_marker_seen")
	}
	if (strings.Contains(lower, "replica marker") || strings.Contains(lower, "standby marker")) &&
		(strings.Contains(lower, "no such file") || strings.Contains(lower, "missing") || strings.Contains(lower, "absent") || strings.Contains(lower, "cannot access")) {
		signals = appendUniqueEvidenceString(signals, "replica_marker_missing")
	}
	if looksLikeHistoryBranchEvidence(lower) {
		signals = appendUniqueEvidenceString(signals, "history_branch_id")
	}
	if evidenceWeakSignalTimelineLikeSequence(lower) {
		signals = appendUniqueEvidenceString(signals, "database_control_timeline")
	}
	if strings.Contains(lower, "not a child of this server's history") || strings.Contains(lower, "not a child of this servers history") {
		signals = appendUniqueEvidenceString(signals, "timeline_history_not_child")
	}
	if looksLikeTimelineMismatchEvidence(lower) {
		signals = appendUniqueEvidenceString(signals, "timeline_mismatch")
	}
	if strings.Contains(lower, "recovery complete") {
		signals = appendUniqueEvidenceString(signals, "archive_recovery_completed")
	}
	if strings.Contains(lower, "restore command") || strings.Contains(lower, "restore_command") {
		signals = appendUniqueEvidenceString(signals, "restore_command_configured")
	}
	if strings.Contains(lower, "recovery target history branch") || strings.Contains(lower, "target history branch") ||
		strings.Contains(lower, "recovery_target_timeline") || strings.Contains(lower, "recovery_target") {
		signals = appendUniqueEvidenceString(signals, "recovery_target_history_branch_configured")
	}
	if strings.Contains(lower, "checkpoint too frequent") || strings.Contains(lower, "checkpoints are occurring too frequently") {
		signals = appendUniqueEvidenceString(signals, "checkpoint_too_frequent")
	}
	if strings.Contains(lower, "write latency spike") ||
		(strings.Contains(lower, "write") && strings.Contains(lower, "latency") && strings.Contains(lower, "spike")) {
		signals = appendUniqueEvidenceString(signals, "write_latency_spike")
	}
	return signals
}

func looksLikeRecoveryInactiveEvidence(lower string) bool {
	return (strings.Contains(lower, "in_recovery") ||
		strings.Contains(lower, "in recovery") ||
		strings.Contains(lower, "recovery status") ||
		strings.Contains(lower, "recovery_state")) &&
		(strings.Contains(lower, " f") ||
			strings.Contains(lower, "false") ||
			strings.Contains(lower, "inactive") ||
			strings.Contains(lower, "not in recovery"))
}

func looksLikeHistoryBranchEvidence(lower string) bool {
	return strings.Contains(lower, "history branch id") ||
		strings.Contains(lower, "lineage id:") ||
		strings.Contains(lower, "branch id:")
}

func evidenceWeakSignalTimelineLikeSequence(lower string) bool {
	return strings.Contains(lower, "pg_controldata") ||
		strings.Contains(lower, "latest checkpoint's timelineid") ||
		strings.Contains(lower, "latest checkpoint's prevtimelineid") ||
		strings.Contains(lower, "latest checkpoint timelineid") ||
		strings.Contains(lower, "latest checkpoint prevtimelineid")
}

func looksLikeTimelineMismatchEvidence(lower string) bool {
	hasTimeline := strings.Contains(lower, "timeline") || strings.Contains(lower, "timelineid")
	if !hasTimeline {
		return false
	}
	return strings.Contains(lower, "not a child") ||
		strings.Contains(lower, "prevtimelineid") ||
		strings.Contains(lower, "selected new timeline") ||
		strings.Contains(lower, "history")
}

func truncateEvidenceExcerpt(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len([]rune(value)) <= limit {
		return value
	}
	runes := []rune(value)
	return strings.TrimSpace(string(runes[:limit])) + "..."
}

func appendUniqueEvidenceString(values []string, next string) []string {
	next = strings.TrimSpace(next)
	if next == "" {
		return values
	}
	if containsString(values, next) {
		return values
	}
	return append(values, next)
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
