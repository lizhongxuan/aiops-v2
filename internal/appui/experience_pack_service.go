package appui

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"aiops-v2/internal/experiencepack"
)

var (
	ErrExperiencePackNotFound             = errors.New("experience pack not found")
	ErrExperiencePackCandidateNotFound    = errors.New("experience pack candidate not found")
	ErrExperiencePackCandidateNotApproved = errors.New("experience pack candidate is not approved")
	ErrExperiencePackValidationBlocked    = errors.New("experience pack validation gate is blocked")
	experiencePackSignalTokenPattern      = regexp.MustCompile(`[a-z0-9_][a-z0-9_.-]{2,40}`)
)

type ExperiencePackRepository interface {
	ListExperiencePacks(ListExperiencePacksRequest) (ExperiencePackLibraryList, error)
	ListExperiencePackCandidates(ListExperiencePackCandidatesRequest) (ExperiencePackCandidateList, error)
	SaveExperiencePackCandidate(ExperiencePackCandidate) error
	GetExperiencePackCandidate(candidateID string) (ExperiencePackCandidate, error)
	GetExperiencePack(packID string) (ExperiencePack, error)
	SaveExperiencePack(ExperiencePack) error
	ListExperiencePackReuseRecords(packID string, req ListExperiencePackReuseRecordsRequest) (ExperiencePackReuseRecordList, error)
}

type ExperiencePackIndexedRepository interface {
	RetrieveExperiencePacks(ExperiencePackRetrieveRequest) (ExperiencePackMatchList, error)
}

type defaultExperiencePackService struct {
	mu         sync.Mutex
	repo       ExperiencePackRepository
	domain     *experiencepack.Service
	candidates map[string]ExperiencePackCandidate
	packs      map[string]ExperiencePack
	reuse      map[string][]ExperiencePackReuseRecord
}

func NewExperiencePackService(repo ExperiencePackRepository) ExperiencePackService {
	return &defaultExperiencePackService{
		repo:       repo,
		domain:     experiencepack.NewService(experiencepack.NewMemoryStore()),
		candidates: map[string]ExperiencePackCandidate{},
		packs:      map[string]ExperiencePack{},
		reuse:      map[string][]ExperiencePackReuseRecord{},
	}
}

func firstNonNilExperiencePackService(service ExperiencePackService) ExperiencePackService {
	if service != nil {
		return service
	}
	return NewExperiencePackService(nil)
}

func (s *defaultExperiencePackService) ListPacks(req ListExperiencePacksRequest) (ExperiencePackLibraryList, error) {
	if s.repo != nil {
		return s.repo.ListExperiencePacks(req)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]ExperiencePack, 0, len(s.packs))
	for _, pack := range s.packs {
		pack = normalizeExperiencePack(pack)
		if req.Status != "" && pack.Status != req.Status {
			continue
		}
		if req.Category != "" && pack.Category != req.Category {
			continue
		}
		if req.UsageShape != "" && pack.UsageShape != req.UsageShape {
			continue
		}
		if req.Middleware != "" && !strings.Contains(strings.ToLower(pack.Middleware+" "+strings.Join(pack.Tags, " ")), strings.ToLower(req.Middleware)) {
			continue
		}
		if req.HasRunnerBinding == "true" && !packHasExecutableRunnerBinding(pack) {
			continue
		}
		items = append(items, cloneExperiencePack(pack))
	}
	return ExperiencePackLibraryList{Items: items, Total: len(items)}, nil
}

func (s *defaultExperiencePackService) ListCandidates(req ListExperiencePackCandidatesRequest) (ExperiencePackCandidateList, error) {
	if s.repo != nil {
		list, err := s.repo.ListExperiencePackCandidates(req)
		if err != nil {
			return ExperiencePackCandidateList{}, err
		}
		for i := range list.Items {
			if pack, err := s.repo.GetExperiencePack(list.Items[i].PackID); err == nil {
				pack = normalizeExperiencePack(pack)
				list.Items[i].ExperiencePack = &pack
			}
		}
		return list, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]ExperiencePackCandidate, 0, len(s.candidates))
	for _, candidate := range s.candidates {
		if pack, ok := s.packs[candidate.PackID]; ok {
			pack = normalizeExperiencePack(pack)
			candidate.ExperiencePack = &pack
		}
		items = append(items, cloneExperiencePackCandidate(candidate))
	}
	return ExperiencePackCandidateList{Items: items, Total: len(items)}, nil
}

func (s *defaultExperiencePackService) Retrieve(req ExperiencePackRetrieveRequest) (ExperiencePackMatchList, error) {
	text := strings.TrimSpace(firstNonEmptyExperiencePackString(req.UserText, req.Query))
	signals := signalsFromAny(req.Signals)
	if text != "" {
		signals = appendExperiencePackSignals(signals, experiencePackSignalsFromText(text)...)
	}
	if s.repo != nil {
		if indexed, ok := s.repo.(ExperiencePackIndexedRepository); ok {
			return indexed.RetrieveExperiencePacks(req)
		}
		list, err := s.repo.ListExperiencePacks(ListExperiencePacksRequest{Limit: 1000})
		if err != nil {
			return ExperiencePackMatchList{}, err
		}
		return retrieveExperiencePackMatches(list.Items, text, signals, req), nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	packs := make([]ExperiencePack, 0, len(s.packs))
	for _, pack := range s.packs {
		packs = append(packs, pack)
	}
	return retrieveExperiencePackMatches(packs, text, signals, req), nil
}

func retrieveExperiencePackMatches(packs []ExperiencePack, text string, signals []string, req ExperiencePackRetrieveRequest) ExperiencePackMatchList {
	matches := []ExperiencePackMatch{}
	for _, pack := range packs {
		pack = normalizeExperiencePack(pack)
		if pack.ReviewStatus != "approved" || !pack.Enabled {
			continue
		}
		haystack := strings.ToLower(pack.Title + " " + pack.Summary + " " + pack.Middleware + " " + strings.Join(pack.Tags, " "))
		score := 0.0
		matched := []string{}
		for _, signal := range signals {
			signal = strings.TrimSpace(strings.ToLower(signal))
			if signal == "" {
				continue
			}
			if strings.Contains(haystack, signal) {
				score += 0.25
				if len(matched) < 12 && !containsExperiencePackSignal(matched, signal) {
					matched = append(matched, signal)
				}
			}
		}
		if text != "" && strings.Contains(haystack, strings.ToLower(text)) {
			score += 0.5
		}
		if score == 0 && text != "" {
			continue
		}
		if score == 0 {
			score = 0.5
		}
		if score > 1 {
			score = 1
		}
		nextActions := []string{"view_skill", "check_preconditions", "view_history", "mark_not_applicable"}
		runner, hasRunner := executableRunnerBindingFromPack(pack)
		if hasRunner {
			nextActions = append(nextActions, "create_dry_run")
		}
		matches = append(matches, ExperiencePackMatch{
			PackID: pack.ID, Skill: pack.Skill, Confidence: score, MatchedSignals: matched,
			MatchReasons: []string{"Skill 语义与 Gene 信号摘要命中"}, NextActions: nextActions,
			OSVariant: firstNonEmptyExperiencePackString(req.OS, req.Environment), RunnerBinding: runner, History: pack.History, AdvancedRefs: pack.AdvancedRefs,
		})
	}
	return ExperiencePackMatchList{Items: matches, Total: len(matches)}
}

func (s *defaultExperiencePackService) RetrieveCandidate(candidateID string) (ExperiencePack, error) {
	candidate, err := s.getCandidate(candidateID)
	if err != nil {
		candidate, err = s.getCandidateByPackID(candidateID)
		if err != nil {
			return ExperiencePack{}, err
		}
	}
	if candidate.Status != "approved" {
		return ExperiencePack{}, ErrExperiencePackCandidateNotApproved
	}
	return s.GetPack(candidate.PackID)
}

func (s *defaultExperiencePackService) EvaluateSuggestions(req ExperiencePackSuggestionEvaluateRequest) (ExperiencePackSuggestionEvaluateResult, error) {
	suggestions := []ExperiencePackSuggestion{}
	result := experiencepack.EvaluateChatSuggestion(experiencepack.SuggestionInput{
		CaseID:                   req.CaseID,
		CommandCount:             req.CommandCount,
		Outcome:                  req.Outcome,
		RedactionStatus:          req.RedactionStatus,
		LLMOperationalValueScore: req.LLMOperationalValueScore,
		MatchedPackID:            req.MatchedPackID,
		MemoryGraphWritable:      req.MemoryGraphWritable,
		ReusableStepCount:        req.ReusableStepCount,
	})
	for _, item := range result.Suggestions {
		suggestions = append(suggestions, ExperiencePackSuggestion{
			ID:     firstNonEmptyExperiencePackString(item.Type, item.Label),
			Type:   item.Type,
			Label:  item.Label,
			Reason: item.Reason,
		})
	}
	return ExperiencePackSuggestionEvaluateResult{Items: suggestions, Suggestions: suggestions, Total: len(suggestions)}, nil
}

func (s *defaultExperiencePackService) PrepareCandidate(req ExperiencePackPrepareCandidateRequest) (ExperiencePackCandidate, error) {
	packID := strings.TrimSpace(req.PackID)
	if packID == "" {
		packID = strings.TrimSpace(req.CaseID)
	}
	if packID == "" {
		packID = fmt.Sprintf("pack-%d", time.Now().UnixNano())
	}
	candidateID := "candidate-" + packID
	bundle, err := s.domain.GenerateAndPersistCandidate(context.Background(), experiencepack.CandidateInput{
		PackID:   packID,
		Name:     firstNonEmptyExperiencePackString(req.Title, packID),
		Summary:  firstNonEmptyExperiencePackString(req.Summary, req.Title, packID),
		Category: experiencepack.CategoryInnovate,
		Trajectory: experiencepack.Trajectory{
			CaseID:        req.CaseID,
			ChatSessionID: firstNonEmptyExperiencePackString(req.ChatSessionID, metadataString(req.Metadata, "chatSessionId")),
			UserGoal:      firstNonEmptyExperiencePackString(req.Summary, req.Title, packID),
			Commands:      firstNonEmptyStringSlice(req.Commands, stringsFromAny(req.Metadata["commands"])),
			ProofID:       firstNonEmptyExperiencePackString(metadataString(req.Metadata, "proofId"), "proof-required"),
			Outcome:       "success",
			Environment: experiencepack.EnvironmentFingerprint{
				OS:             "linux",
				OSDistribution: firstNonEmptyExperiencePackString(req.Environment, "unknown"),
				HostCount:      1,
			},
		},
	})
	if err != nil {
		return ExperiencePackCandidate{}, err
	}
	pack := normalizeExperiencePack(ExperiencePack{
		ID:           packID,
		PackID:       packID,
		Title:        bundle.Manifest.Name,
		Summary:      bundle.Manifest.Summary,
		Category:     bundle.Manifest.Category,
		UsageShape:   "guided",
		Middleware:   firstNonEmptyExperiencePackString(req.Service, "aiops"),
		Status:       "disabled",
		ReviewStatus: "pending",
		Enabled:      false,
		Skill:        ExperiencePackSkill{ID: string(bundle.Skill.AssetID), Name: bundle.Skill.Title, Summary: bundle.Skill.Summary, Path: bundle.Skill.Path},
		ValidationGate: ExperiencePackValidationGate{
			Status: "passed",
			Checks: []map[string]any{{"validator": "runner.readonly_probe", "mode": "read_only"}},
		},
		WorkflowBinding: ExperiencePackWorkflowBinding{WorkflowID: "wf-" + packID, WorkflowName: firstNonEmptyExperiencePackString(req.Title, packID) + " Workflow", Status: "draft"},
		History:         ExperiencePackHistory{RecentResult: bundle.Capsule.Outcome.Status, SuccessCount: 1},
		AdvancedRefs:    ExperiencePackAdvancedRefs{GeneAssetID: string(bundle.Gene.AssetID), CapsuleAssetIDs: []string{string(bundle.Capsule.AssetID)}},
		Metadata:        req.Metadata,
	})
	candidate := ExperiencePackCandidate{
		ID:             candidateID,
		CandidateID:    candidateID,
		PackID:         pack.ID,
		Title:          pack.Title,
		Summary:        pack.Summary,
		Status:         "candidate",
		SourceCaseID:   req.CaseID,
		ExperiencePack: &pack,
		CreatedAt:      time.Now().UTC().Format(time.RFC3339),
		Metadata:       req.Metadata,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.packs[pack.ID] = pack
	s.candidates[candidate.ID] = candidate
	if s.repo != nil {
		if err := s.repo.SaveExperiencePack(pack); err != nil {
			return ExperiencePackCandidate{}, err
		}
		if err := s.repo.SaveExperiencePackCandidate(candidate); err != nil {
			return ExperiencePackCandidate{}, err
		}
	}
	return cloneExperiencePackCandidate(candidate), nil
}

func (s *defaultExperiencePackService) ConfirmCandidate(candidateID string, req ExperiencePackReviewRequest) (ExperiencePack, error) {
	if strings.TrimSpace(candidateID) == "" {
		candidateID = firstNonEmptyExperiencePackString(req.CandidateID, req.ConfirmationToken)
	}
	if strings.TrimSpace(candidateID) == "" {
		candidateID = s.latestCandidateID()
	}
	candidate, err := s.getCandidate(candidateID)
	if err != nil {
		return ExperiencePack{}, err
	}
	pack, err := s.GetPack(candidate.PackID)
	if err != nil {
		return ExperiencePack{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	approved := req.Decision == "" || req.Decision == "approve" || req.Decision == "approved"
	candidate.Status = firstNonEmptyExperiencePackString(req.Decision, "approved")
	if candidate.Status == "approve" {
		candidate.Status = "approved"
	}
	candidate.UpdatedAt = now
	pack.ReviewStatus = "approved"
	pack.Status = "approved"
	pack.UpdatedAt = now
	if req.Decision != "" && req.Decision != "approve" && req.Decision != "approved" {
		pack.ReviewStatus = req.Decision
	}
	if approved && pack.ValidationGate.Status != "blocked" {
		pack.Enabled = true
		pack.Status = "enabled"
		if len(pack.AuthorizationScopes) == 0 {
			pack.AuthorizationScopes = normalizeExperiencePackAuthorizationScopes([]ExperiencePackAuthorizationScope{{
				Type:       "environment",
				Value:      "prod",
				Searchable: true,
				Reason:     "审核通过后默认进入经验库检索范围",
			}})
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.candidates[candidate.ID] = candidate
	s.packs[pack.ID] = pack
	if s.repo != nil {
		if err := s.repo.SaveExperiencePack(pack); err != nil {
			return ExperiencePack{}, err
		}
		if err := s.repo.SaveExperiencePackCandidate(candidate); err != nil {
			return ExperiencePack{}, err
		}
	}
	return cloneExperiencePack(pack), nil
}

func (s *defaultExperiencePackService) PrepareRunnerCandidate(req ExperiencePackRunnerCandidateRequest) (ExperiencePackRunnerCandidate, error) {
	return s.buildRunnerCandidate(req)
}

func (s *defaultExperiencePackService) ConfirmRunnerCandidate(req ExperiencePackRunnerCandidateRequest) (ExperiencePackRunnerCandidate, error) {
	result, err := s.buildRunnerCandidate(req)
	if err != nil {
		return ExperiencePackRunnerCandidate{}, err
	}
	packID := strings.TrimSpace(result.PackID)
	if packID == "" {
		return result, nil
	}
	pack, err := s.GetPack(packID)
	if err != nil {
		if errors.Is(err, ErrExperiencePackNotFound) {
			return result, nil
		}
		return ExperiencePackRunnerCandidate{}, err
	}
	pack.RunnerBindings = upsertExperiencePackRunnerBinding(pack.RunnerBindings, result.RunnerBinding)
	pack.UsageShape = "guided"
	pack.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	_, err = s.savePack(pack)
	return result, err
}

func (s *defaultExperiencePackService) GetPack(packID string) (ExperiencePack, error) {
	if s.repo != nil {
		pack, err := s.repo.GetExperiencePack(packID)
		if err == nil {
			return normalizeExperiencePack(pack), nil
		}
		if !errors.Is(err, ErrExperiencePackNotFound) {
			return ExperiencePack{}, err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	pack, ok := s.packs[packID]
	if !ok {
		return ExperiencePack{}, ErrExperiencePackNotFound
	}
	return cloneExperiencePack(pack), nil
}

func (s *defaultExperiencePackService) GetValidationGate(packID string) (ExperiencePackValidationGate, error) {
	pack, err := s.GetPack(packID)
	if err != nil {
		return ExperiencePackValidationGate{}, err
	}
	return pack.ValidationGate, nil
}

func (s *defaultExperiencePackService) EnablePack(packID string, req ExperiencePackReviewRequest) (ExperiencePack, error) {
	pack, err := s.GetPack(packID)
	if err != nil {
		return ExperiencePack{}, err
	}
	if pack.ValidationGate.Status == "blocked" {
		return ExperiencePack{}, ErrExperiencePackValidationBlocked
	}
	if pack.ReviewStatus != "approved" {
		return ExperiencePack{}, ErrExperiencePackCandidateNotApproved
	}
	pack.Enabled = true
	pack.Status = "enabled"
	pack.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return s.savePack(pack)
}

func (s *defaultExperiencePackService) buildRunnerCandidate(req ExperiencePackRunnerCandidateRequest) (ExperiencePackRunnerCandidate, error) {
	req = s.enrichRunnerCandidateRequest(req)
	commands := firstNonEmptyStringSlice(req.Commands, stringsFromAny(req.Metadata["commands"]))
	candidate := experiencepack.GenerateRunnerWorkflowCandidate(experiencepack.Trajectory{
		CaseID:        firstNonEmptyExperiencePackString(req.CaseID, req.CandidateID, req.PackID),
		ChatSessionID: firstNonEmptyExperiencePackString(req.ChatSessionID, metadataString(req.Metadata, "chatSessionId")),
		UserGoal:      firstNonEmptyExperiencePackString(req.Summary, req.Title, "生成工作流"),
		Commands:      append([]string(nil), commands...),
		Outcome:       "success",
		Environment: experiencepack.EnvironmentFingerprint{
			OS:             "linux",
			OSDistribution: firstNonEmptyExperiencePackString(req.Environment, "unknown"),
			HostCount:      1,
		},
	})
	workflowID := candidate.ID
	workflowName := firstNonEmptyExperiencePackString(req.Title, candidate.WorkflowName)
	if !strings.Contains(strings.ToLower(workflowName), "workflow") {
		workflowName += " Workflow"
	}
	link := firstNonEmptyExperiencePackString(candidate.StudioDraftLink, "/runner/"+workflowID)
	req.Commands = commands
	graph := runnerCandidateGraph(workflowID, workflowName, req, candidate)
	workflow := map[string]any{
		"id":                      workflowID,
		"name":                    workflowID,
		"title":                   workflowName,
		"status":                  "draft",
		"local_draft":             true,
		"ai_generated_draft":      true,
		"experience_pack_binding": map[string]any{"pack_id": req.PackID, "runner_candidate_id": candidate.ID, "asset_id": string(candidate.AssetID)},
		"graph":                   graph,
		"validation_result":       map[string]any{"valid": false, "warnings": []string{"AI 只生成草稿；保存、Dry Run、发布前必须人工审核节点参数。"}},
		"risk_summary":            map[string]any{"level": "medium", "items": candidate.Guards},
	}
	binding := ExperiencePackRunnerBinding{
		ID:           "binding-" + workflowID,
		WorkflowID:   workflowID,
		WorkflowName: workflowName,
		Status:       "draft",
		ReviewStatus: "pending",
		Version:      "draft",
		Metadata: map[string]any{
			"runner_candidate_id": candidate.ID,
			"studio_draft_link":   link,
			"asset_id":            string(candidate.AssetID),
			"published":           false,
			"workflow_status":     "draft",
		},
	}
	return ExperiencePackRunnerCandidate{
		ID:              candidate.ID,
		PackID:          req.PackID,
		WorkflowID:      workflowID,
		WorkflowName:    workflowName,
		Status:          "draft",
		StudioDraftLink: link,
		Workflow:        workflow,
		Graph:           graph,
		RunnerBinding:   binding,
		Metadata: map[string]any{
			"parameters": candidate.Parameters,
			"guards":     candidate.Guards,
			"asset_id":   string(candidate.AssetID),
		},
	}, nil
}

func (s *defaultExperiencePackService) enrichRunnerCandidateRequest(req ExperiencePackRunnerCandidateRequest) ExperiencePackRunnerCandidateRequest {
	req.PackID = strings.TrimSpace(req.PackID)
	req.CandidateID = strings.TrimSpace(firstNonEmptyExperiencePackString(req.CandidateID, req.ConfirmationToken))
	if req.PackID == "" && req.CandidateID != "" {
		if candidate, err := s.getCandidate(req.CandidateID); err == nil {
			req.PackID = candidate.PackID
			req.Title = firstNonEmptyExperiencePackString(req.Title, candidate.Title)
			req.Summary = firstNonEmptyExperiencePackString(req.Summary, candidate.Summary)
			req.CaseID = firstNonEmptyExperiencePackString(req.CaseID, candidate.SourceCaseID)
		}
	}
	if req.PackID != "" {
		if pack, err := s.GetPack(req.PackID); err == nil {
			req.Title = firstNonEmptyExperiencePackString(req.Title, pack.Title)
			req.Summary = firstNonEmptyExperiencePackString(req.Summary, pack.Summary)
			req.Service = firstNonEmptyExperiencePackString(req.Service, pack.Middleware)
		}
	}
	return req
}

func runnerCandidateGraph(workflowID, workflowName string, req ExperiencePackRunnerCandidateRequest, candidate experiencepack.RunnerWorkflowCandidate) map[string]any {
	portsInNextFailure := []map[string]any{{"id": "in", "type": "input", "label": "输入"}, {"id": "next", "type": "output", "label": "下一步"}, {"id": "failure", "type": "output", "label": "失败"}}
	portsApproval := []map[string]any{{"id": "in", "type": "input", "label": "输入"}, {"id": "approved", "type": "output", "label": "通过"}, {"id": "rejected", "type": "output", "label": "拒绝"}}
	commands := req.Commands
	if len(commands) == 0 {
		commands = []string{"echo review and replace this placeholder with approved Runner steps"}
	}
	executionScript := strings.Join(commands, "\n")
	nodes := []map[string]any{
		{"id": "start", "type": "start", "label": "Start", "position": map[string]any{"x": 80, "y": 160}, "ports": []map[string]any{{"id": "next", "type": "output", "label": "下一步"}}},
		{"id": "precheck", "type": "action", "label": "环境预检查", "position": map[string]any{"x": 300, "y": 120}, "ports": portsInNextFailure, "step": map[string]any{"name": "experience-precheck", "action": "shell.run", "args": map[string]any{"script": "echo check host lease, OS, package manager and required privileges"}}},
		{"id": "approval", "type": "manual_approval", "label": "人工审批", "position": map[string]any{"x": 520, "y": 120}, "ports": portsApproval, "risk": map[string]any{"level": "high"}, "step": map[string]any{"name": "experience-approval", "action": "manual.approval", "args": map[string]any{"risk_reason": "经验包生成的 Runner 草稿必须人工确认参数、爆炸半径和回滚策略。"}}, "approval": map[string]any{"subjects": []string{"sre"}, "timeout": "30m", "on_timeout": "reject"}},
		{"id": "dry_run", "type": "action", "label": "Dry Run", "position": map[string]any{"x": 740, "y": 120}, "ports": portsInNextFailure, "step": map[string]any{"name": "experience-dry-run", "action": "shell.run", "args": map[string]any{"script": "echo estimate blast radius and render execution plan"}}},
		{"id": "execute", "type": "action", "label": "受控执行", "position": map[string]any{"x": 960, "y": 120}, "ports": portsInNextFailure, "step": map[string]any{"name": "experience-execute", "action": "shell.run", "args": map[string]any{"script": executionScript}}},
		{"id": "validate", "type": "action", "label": "恢复验证", "position": map[string]any{"x": 1180, "y": 120}, "ports": portsInNextFailure, "step": map[string]any{"name": "experience-validate", "action": "shell.run", "args": map[string]any{"script": "echo run readonly validation probes and proof of recovery"}}},
		{"id": "rollback", "type": "action", "label": "受控回滚", "position": map[string]any{"x": 960, "y": 300}, "ports": portsInNextFailure, "step": map[string]any{"name": "experience-rollback", "action": "shell.run", "args": map[string]any{"script": "echo rollback with approved recovery plan"}}},
		{"id": "end", "type": "end", "label": "End", "position": map[string]any{"x": 1400, "y": 160}, "ports": []map[string]any{{"id": "in", "type": "input", "label": "输入"}}},
	}
	edges := []map[string]any{
		{"id": "start-precheck", "source": "start", "source_port": "next", "target": "precheck", "target_port": "in", "kind": "next"},
		{"id": "precheck-approval", "source": "precheck", "source_port": "next", "target": "approval", "target_port": "in", "kind": "next"},
		{"id": "approval-dry-run", "source": "approval", "source_port": "approved", "target": "dry_run", "target_port": "in", "kind": "approval_approved"},
		{"id": "approval-end", "source": "approval", "source_port": "rejected", "target": "end", "target_port": "in", "kind": "approval_rejected"},
		{"id": "dry-run-execute", "source": "dry_run", "source_port": "next", "target": "execute", "target_port": "in", "kind": "next"},
		{"id": "execute-validate", "source": "execute", "source_port": "next", "target": "validate", "target_port": "in", "kind": "next"},
		{"id": "validate-end", "source": "validate", "source_port": "next", "target": "end", "target_port": "in", "kind": "next"},
		{"id": "precheck-rollback", "source": "precheck", "source_port": "failure", "target": "rollback", "target_port": "in", "kind": "failure"},
		{"id": "dry-run-rollback", "source": "dry_run", "source_port": "failure", "target": "rollback", "target_port": "in", "kind": "failure"},
		{"id": "execute-rollback", "source": "execute", "source_port": "failure", "target": "rollback", "target_port": "in", "kind": "failure"},
		{"id": "rollback-end", "source": "rollback", "source_port": "next", "target": "end", "target_port": "in", "kind": "next"},
	}
	return map[string]any{
		"version": "v1",
		"workflow": map[string]any{
			"name":                    workflowID,
			"title":                   workflowName,
			"description":             firstNonEmptyExperiencePackString(req.Summary, "由经验包候选生成的 Runner Workflow 草稿"),
			"inputs":                  runnerWorkflowInputs(candidate.Parameters),
			"experience_pack_binding": map[string]any{"pack_id": req.PackID, "runner_candidate_id": candidate.ID},
		},
		"nodes": nodes,
		"edges": edges,
	}
}

func runnerWorkflowInputs(parameters []string) []map[string]any {
	inputs := make([]map[string]any, 0, len(parameters))
	for _, parameter := range parameters {
		if strings.TrimSpace(parameter) == "" {
			continue
		}
		inputs = append(inputs, map[string]any{"name": parameter, "type": "string", "required": true})
	}
	return inputs
}

func upsertExperiencePackRunnerBinding(bindings []ExperiencePackRunnerBinding, binding ExperiencePackRunnerBinding) []ExperiencePackRunnerBinding {
	for i := range bindings {
		if bindings[i].ID == binding.ID || bindings[i].WorkflowID == binding.WorkflowID {
			next := append([]ExperiencePackRunnerBinding(nil), bindings...)
			next[i] = binding
			return next
		}
	}
	next := append([]ExperiencePackRunnerBinding(nil), bindings...)
	return append(next, binding)
}

func packHasExecutableRunnerBinding(pack ExperiencePack) bool {
	_, ok := executableRunnerBindingFromPack(pack)
	return ok
}

func executableRunnerBindingFromPack(pack ExperiencePack) (ExperiencePackRunnerBinding, bool) {
	for _, binding := range pack.RunnerBindings {
		if runnerBindingExecutable(binding) {
			return binding, true
		}
	}
	if workflowBindingExecutable(pack.WorkflowBinding) {
		return ExperiencePackRunnerBinding{
			WorkflowID:   pack.WorkflowBinding.WorkflowID,
			WorkflowName: pack.WorkflowBinding.WorkflowName,
			Status:       pack.WorkflowBinding.Status,
			Version:      pack.WorkflowBinding.Version,
			Metadata:     pack.WorkflowBinding.Metadata,
		}, true
	}
	return ExperiencePackRunnerBinding{}, false
}

func runnerBindingExecutable(binding ExperiencePackRunnerBinding) bool {
	if strings.TrimSpace(binding.WorkflowID) == "" {
		return false
	}
	review := strings.ToLower(strings.TrimSpace(binding.ReviewStatus))
	if review != "" && review != "approved" && review != "approve" {
		return false
	}
	return runnerWorkflowPublished(binding.Status, binding.Metadata)
}

func workflowBindingExecutable(binding ExperiencePackWorkflowBinding) bool {
	if strings.TrimSpace(binding.WorkflowID) == "" {
		return false
	}
	return runnerWorkflowPublished(binding.Status, binding.Metadata)
}

func runnerWorkflowPublished(status string, metadata map[string]any) bool {
	normalized := strings.ToLower(strings.TrimSpace(status))
	if normalized == "published" {
		return true
	}
	if metadataBool(metadata, "published") {
		return true
	}
	workflowStatus := strings.ToLower(firstNonEmptyExperiencePackString(metadataString(metadata, "workflow_status"), metadataString(metadata, "workflowStatus")))
	return workflowStatus == "published"
}

func (s *defaultExperiencePackService) PausePack(packID string, req ExperiencePackReviewRequest) (ExperiencePack, error) {
	pack, err := s.GetPack(packID)
	if err != nil {
		return ExperiencePack{}, err
	}
	pack.Enabled = false
	pack.Status = "paused"
	pack.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return s.savePack(pack)
}

func (s *defaultExperiencePackService) SetPackEnabled(packID string, enabled bool, req ExperiencePackReviewRequest) (ExperiencePack, error) {
	if enabled {
		return s.EnablePack(packID, req)
	}
	return s.PausePack(packID, req)
}

func (s *defaultExperiencePackService) SaveAuthorizationScopes(packID string, scopes []ExperiencePackAuthorizationScope) (ExperiencePack, error) {
	pack, err := s.GetPack(packID)
	if err != nil {
		return ExperiencePack{}, err
	}
	pack.AuthorizationScopes = normalizeExperiencePackAuthorizationScopes(scopes)
	pack.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return s.savePack(pack)
}

func (s *defaultExperiencePackService) GetAuthorizationScopes(packID string) ([]ExperiencePackAuthorizationScope, error) {
	pack, err := s.GetPack(packID)
	if err != nil {
		return nil, err
	}
	return append([]ExperiencePackAuthorizationScope(nil), pack.AuthorizationScopes...), nil
}

func (s *defaultExperiencePackService) SaveRunnerBindings(packID string, bindings []ExperiencePackRunnerBinding) (ExperiencePack, error) {
	pack, err := s.GetPack(packID)
	if err != nil {
		return ExperiencePack{}, err
	}
	pack.RunnerBindings = append([]ExperiencePackRunnerBinding(nil), bindings...)
	pack.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return s.savePack(pack)
}

func (s *defaultExperiencePackService) ReviewRunnerBindings(packID string, req ExperiencePackRunnerBindingReviewRequest) (ExperiencePack, error) {
	pack, err := s.GetPack(packID)
	if err != nil {
		return ExperiencePack{}, err
	}
	decision := firstNonEmptyExperiencePackString(req.Decision, "approved")
	selected := map[string]bool{}
	for _, id := range req.BindingIDs {
		selected[id] = true
	}
	for i := range pack.RunnerBindings {
		if len(selected) == 0 || selected[pack.RunnerBindings[i].ID] || selected[pack.RunnerBindings[i].WorkflowID] {
			pack.RunnerBindings[i].ReviewStatus = decision
		}
	}
	pack.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return s.savePack(pack)
}

func (s *defaultExperiencePackService) latestCandidateID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id := range s.candidates {
		return id
	}
	return ""
}

func (s *defaultExperiencePackService) ListReuseRecords(packID string, req ListExperiencePackReuseRecordsRequest) (ExperiencePackReuseRecordList, error) {
	if s.repo != nil {
		return s.repo.ListExperiencePackReuseRecords(packID, req)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]ExperiencePackReuseRecord, 0, len(s.reuse[packID]))
	items = append(items, s.reuse[packID]...)
	return ExperiencePackReuseRecordList{Items: items, Total: len(items)}, nil
}

func (s *defaultExperiencePackService) getCandidate(candidateID string) (ExperiencePackCandidate, error) {
	if s.repo != nil {
		candidate, err := s.repo.GetExperiencePackCandidate(candidateID)
		if err == nil {
			return cloneExperiencePackCandidate(candidate), nil
		}
		if !errors.Is(err, ErrExperiencePackCandidateNotFound) {
			return ExperiencePackCandidate{}, err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	candidate, ok := s.candidates[candidateID]
	if !ok {
		return ExperiencePackCandidate{}, ErrExperiencePackCandidateNotFound
	}
	return cloneExperiencePackCandidate(candidate), nil
}

func (s *defaultExperiencePackService) getCandidateByPackID(packID string) (ExperiencePackCandidate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, candidate := range s.candidates {
		if candidate.PackID == packID {
			return cloneExperiencePackCandidate(candidate), nil
		}
	}
	return ExperiencePackCandidate{}, ErrExperiencePackCandidateNotFound
}

func (s *defaultExperiencePackService) savePack(pack ExperiencePack) (ExperiencePack, error) {
	pack = normalizeExperiencePack(pack)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.packs[pack.ID] = pack
	if s.repo != nil {
		if err := s.repo.SaveExperiencePack(pack); err != nil {
			return ExperiencePack{}, err
		}
	}
	return cloneExperiencePack(pack), nil
}

func normalizeExperiencePack(pack ExperiencePack) ExperiencePack {
	pack.ID = strings.TrimSpace(firstNonEmptyExperiencePackString(pack.ID, pack.PackID))
	pack.PackID = firstNonEmptyExperiencePackString(pack.PackID, pack.ID)
	if pack.Status == "" {
		if pack.Enabled {
			pack.Status = "enabled"
		} else {
			pack.Status = "disabled"
		}
	}
	if pack.ReviewStatus == "" {
		pack.ReviewStatus = "pending"
	}
	if pack.ValidationGate.Status == "" {
		pack.ValidationGate.Status = "pending"
	}
	if pack.Category == "" {
		pack.Category = "repair"
	}
	if pack.UsageShape == "" {
		if len(pack.RunnerBindings) > 0 || pack.WorkflowBinding.WorkflowID != "" {
			pack.UsageShape = "guided"
		} else {
			pack.UsageShape = "diagnostic"
		}
	}
	if pack.Skill.Path == "" {
		pack.Skill = ExperiencePackSkill{ID: "skill-" + pack.ID, Name: firstNonEmptyExperiencePackString(pack.Title, pack.ID), Summary: pack.Summary, Path: "skills/SKILL.md"}
	}
	pack.AuthorizationScopes = normalizeExperiencePackAuthorizationScopes(pack.AuthorizationScopes)
	return pack
}

func normalizeExperiencePackAuthorizationScopes(scopes []ExperiencePackAuthorizationScope) []ExperiencePackAuthorizationScope {
	out := make([]ExperiencePackAuthorizationScope, 0, len(scopes))
	for _, scope := range scopes {
		scope.Type = strings.TrimSpace(scope.Type)
		scope.Value = strings.TrimSpace(scope.Value)
		if scope.ID == "" {
			scope.ID = scope.Type + ":" + scope.Value
		}
		out = append(out, scope)
	}
	return out
}

func cloneExperiencePackCandidate(candidate ExperiencePackCandidate) ExperiencePackCandidate {
	if candidate.ExperiencePack != nil {
		pack := cloneExperiencePack(*candidate.ExperiencePack)
		candidate.ExperiencePack = &pack
	}
	return candidate
}

func cloneExperiencePack(pack ExperiencePack) ExperiencePack {
	pack.AuthorizationScopes = append([]ExperiencePackAuthorizationScope(nil), pack.AuthorizationScopes...)
	pack.RunnerBindings = append([]ExperiencePackRunnerBinding(nil), pack.RunnerBindings...)
	return pack
}

func firstNonEmptyExperiencePackString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func signalsFromAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		return appendExperiencePackSignals(nil, typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
				out = appendExperiencePackSignals(out, text)
			}
		}
		return out
	case map[string]any:
		out := []string{}
		for key, item := range typed {
			out = appendExperiencePackSignals(out, key)
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
				out = appendExperiencePackSignals(out, text)
			}
		}
		return out
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return appendExperiencePackSignals(nil, typed)
	default:
		return nil
	}
}

func experiencePackSignalsFromText(text string) []string {
	normalized := strings.ToLower(text)
	signals := []string{}
	for _, candidate := range []string{
		"postgresql", "postgres", "pg", "pg_mon", "pg-primary", "pg-standby", "pg-mon",
		"mysql", "redis", "kubernetes", "k8s", "coroot", "p95", "maxmemory",
		"primary", "standby", "replication", "aiops_exp",
		"主从", "部署", "备份", "恢复", "排障", "异常", "验证", "回滚", "监控",
	} {
		if strings.Contains(normalized, strings.ToLower(candidate)) {
			signals = appendExperiencePackSignals(signals, candidate)
		}
	}
	for _, token := range experiencePackSignalTokenPattern.FindAllString(normalized, -1) {
		if !experiencePackTokenLooksOperational(token) {
			continue
		}
		signals = appendExperiencePackSignals(signals, token)
		if len(signals) >= 24 {
			break
		}
	}
	return signals
}

func experiencePackTokenLooksOperational(token string) bool {
	if len(token) > 40 || len(token) < 3 {
		return false
	}
	for _, marker := range []string{"pg", "postgres", "mysql", "redis", "k8s", "kubernetes", "coroot", "p95", "maxmemory", "primary", "standby", "replication", "aiops"} {
		if strings.Contains(token, marker) {
			return true
		}
	}
	return false
}

func appendExperiencePackSignals(base []string, values ...string) []string {
	for _, value := range values {
		signal := strings.TrimSpace(strings.ToLower(value))
		if signal == "" || containsExperiencePackSignal(base, signal) {
			continue
		}
		base = append(base, signal)
	}
	return base
}

func containsExperiencePackSignal(signals []string, signal string) bool {
	for _, existing := range signals {
		if existing == signal {
			return true
		}
	}
	return false
}

func stringsFromAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
				out = append(out, text)
			}
		}
		return out
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		lines := strings.Split(typed, "\n")
		out := make([]string, 0, len(lines))
		for _, line := range lines {
			if text := strings.TrimSpace(line); text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func firstNonEmptyStringSlice(values ...[]string) []string {
	for _, value := range values {
		out := make([]string, 0, len(value))
		for _, item := range value {
			if text := strings.TrimSpace(item); text != "" {
				out = append(out, text)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

func metadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	value := metadata[key]
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func metadataBool(metadata map[string]any, key string) bool {
	if metadata == nil {
		return false
	}
	value := metadata[key]
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true") || strings.TrimSpace(typed) == "1"
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	default:
		return false
	}
}
