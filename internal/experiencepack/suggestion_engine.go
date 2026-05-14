package experiencepack

type SuggestionInput struct {
	CaseID                   string   `json:"caseId,omitempty"`
	ChatSessionID            string   `json:"chatSessionId,omitempty"`
	CommandCount             int      `json:"commandCount"`
	ToolCallCount            int      `json:"toolCallCount,omitempty"`
	RunnerRunIDs             []string `json:"runnerRunIds,omitempty"`
	ProofID                  string   `json:"proofId,omitempty"`
	Outcome                  string   `json:"outcome,omitempty"`
	RedactionStatus          string   `json:"redactionStatus,omitempty"`
	LLMOperationalValueScore float64  `json:"llmOperationalValueScore"`
	MatchedPackID            string   `json:"matchedPackId,omitempty"`
	MemoryGraphWritable      bool     `json:"memoryGraphWritable"`
	ReusableStepCount        int      `json:"reusableStepCount,omitempty"`
}

type ExperienceSuggestion struct {
	Type     string   `json:"type"`
	Label    string   `json:"label"`
	Reason   string   `json:"reason"`
	Refs     []string `json:"refs,omitempty"`
	Disabled bool     `json:"disabled,omitempty"`
}

type SuggestionResult struct {
	Visible     bool                   `json:"visible"`
	Suggestions []ExperienceSuggestion `json:"suggestions"`
	Reasons     []string               `json:"reasons"`
}

func EvaluateChatSuggestion(input SuggestionInput) SuggestionResult {
	var reasons []string
	if input.CommandCount < 6 {
		reasons = append(reasons, "运维命令数不足 6")
	}
	if input.LLMOperationalValueScore < 0.7 {
		reasons = append(reasons, "运维价值评分不足 0.7")
	}
	if input.Outcome == "" {
		reasons = append(reasons, "缺少明确 outcome")
	}
	if input.RedactionStatus != "redacted" && input.RedactionStatus != "safe" {
		reasons = append(reasons, "脱敏未完成")
	}
	if !input.MemoryGraphWritable {
		reasons = append(reasons, "Memory Graph 不可写")
	}
	if len(reasons) > 0 {
		return SuggestionResult{Visible: false, Reasons: reasons, Suggestions: []ExperienceSuggestion{}}
	}
	suggestions := []ExperienceSuggestion{}
	if input.MatchedPackID == "" && input.Outcome == "success" {
		suggestions = append(suggestions, ExperienceSuggestion{Type: "generate_experience_pack_candidate", Label: "生成经验包", Reason: "未命中已有经验且本次运维成功"})
	}
	if input.MatchedPackID != "" {
		suggestions = append(suggestions, ExperienceSuggestion{Type: "evolve_current_experience", Label: "进化当前经验", Reason: "本次运维命中已有经验，可追加成功或失败记忆"})
	}
	if input.ReusableStepCount >= 6 {
		suggestions = append(suggestions, ExperienceSuggestion{Type: "generate_runner_workflow_candidate", Label: "生成工作流", Reason: "检测到 6 个以上可参数化操作步骤"})
	}
	return SuggestionResult{Visible: len(suggestions) > 0, Suggestions: suggestions}
}
