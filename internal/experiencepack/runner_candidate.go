package experiencepack

import (
	"fmt"
	"regexp"
	"strings"
)

type Trajectory struct {
	CaseID        string                 `json:"caseId,omitempty"`
	ChatSessionID string                 `json:"chatSessionId,omitempty"`
	UserGoal      string                 `json:"userGoal,omitempty"`
	Commands      []string               `json:"commands,omitempty"`
	ToolCalls     []string               `json:"toolCalls,omitempty"`
	RunnerRunIDs  []string               `json:"runnerRunIds,omitempty"`
	ProofID       string                 `json:"proofId,omitempty"`
	Outcome       string                 `json:"outcome,omitempty"`
	Environment   EnvironmentFingerprint `json:"environment,omitempty"`
}

func GenerateRunnerWorkflowCandidate(tr Trajectory) RunnerWorkflowCandidate {
	steps := []RunnerStep{
		{ID: "precheck", Kind: "precheck", Name: "检查主机、端口、数据目录和权限"},
		{ID: "dry_run", Kind: "dry_run", Name: "生成执行计划并估算爆炸半径"},
		{ID: "execute", Kind: "execute", Name: "按审批后的计划执行"},
		{ID: "validate", Kind: "validate", Name: "执行只读验证任务"},
		{ID: "rollback", Kind: "rollback", Name: "失败时执行受控回滚"},
	}
	params := parameterizeCommands(tr.Commands)
	candidate := RunnerWorkflowCandidate{
		ID:           "runner_candidate_" + stableSuffix(tr.CaseID, tr.ChatSessionID, tr.UserGoal),
		WorkflowName: "经验包 Runner Workflow 候选",
		Status:       "draft",
		Steps:        steps,
		Parameters:   params,
		Guards: map[string]any{
			"dry_run_required": true, "approval_required": true, "host_lease_required": true,
			"forbidden_params": []string{"password", "token", "secret", "drop_existing_data"},
			"max_blast_radius": map[string]any{"hosts": maxInt(1, tr.Environment.HostCount)},
		},
		StudioDraftLink: "/runner/" + "runner_candidate_" + stableSuffix(tr.CaseID, tr.ChatSessionID, tr.UserGoal),
	}
	candidate.AssetID = MustHashCanonicalJSON(candidate)
	return candidate
}

func parameterizeCommands(commands []string) []string {
	keys := map[string]bool{"primary_host": true, "standby_host": true, "monitor_host": true, "port": true, "version": true, "data_dir": true, "username": true}
	hostRe := regexp.MustCompile(`(?i)\b[a-z0-9._-]+\.(internal|local|corp)\b|\bxx[A-Z]\b`)
	for _, cmd := range commands {
		lower := strings.ToLower(cmd)
		if strings.Contains(lower, "pg_mon") {
			keys["monitor_host"] = true
		}
		if hostRe.MatchString(cmd) {
			keys["primary_host"] = true
			keys["standby_host"] = true
		}
		if strings.Contains(lower, "--port") || strings.Contains(lower, "5432") {
			keys["port"] = true
		}
	}
	result := make([]string, 0, len(keys))
	for key := range keys {
		result = append(result, key)
	}
	return result
}

func stableSuffix(values ...string) string {
	raw := strings.TrimSpace(strings.Join(values, "_"))
	if raw == "" {
		raw = "manual"
	}
	raw = strings.ToLower(regexp.MustCompile(`[^a-zA-Z0-9]+`).ReplaceAllString(raw, "_"))
	raw = strings.Trim(raw, "_")
	if len(raw) > 48 {
		raw = raw[:48]
	}
	return fmt.Sprintf("%s", raw)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
