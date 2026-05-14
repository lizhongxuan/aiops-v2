package experiencepack

import (
	"context"
	"sort"
	"strings"
)

type RetrievalQuery struct {
	CaseID        string                 `json:"caseId,omitempty"`
	ChatSessionID string                 `json:"chatSessionId,omitempty"`
	UserText      string                 `json:"userText,omitempty"`
	Signals       []string               `json:"signals,omitempty"`
	Services      []string               `json:"services,omitempty"`
	Middleware    []string               `json:"middleware,omitempty"`
	Operation     string                 `json:"operation,omitempty"`
	Environment   string                 `json:"environment,omitempty"`
	OSFingerprint EnvironmentFingerprint `json:"osFingerprint,omitempty"`
	EvidenceRefs  []string               `json:"evidenceRefs,omitempty"`
	UserScopes    []AuthorizationScope   `json:"userScopes,omitempty"`
}

type ExperienceMatch struct {
	PackID                  string    `json:"packId"`
	SkillTitle              string    `json:"skillTitle"`
	SkillSummary            string    `json:"skillSummary"`
	ExperienceType          string    `json:"experienceType"`
	UsageMode               string    `json:"usageMode"`
	Score                   float64   `json:"score"`
	Confidence              float64   `json:"confidence"`
	MatchedSignals          []string  `json:"matchedSignals"`
	MatchedSkillFields      []string  `json:"matchedSkillFields"`
	PreconditionGaps        []string  `json:"preconditionGaps"`
	RiskWarnings            []string  `json:"riskWarnings"`
	SelectedGeneID          string    `json:"selectedGeneId,omitempty"`
	SelectedRunnerBindingID string    `json:"selectedRunnerBindingId,omitempty"`
	NextActions             []string  `json:"nextActions"`
	GeneAssetID             AssetID   `json:"geneAssetId,omitempty"`
	CapsuleAssetIDs         []AssetID `json:"capsuleAssetIds,omitempty"`
}

type Retriever struct {
	store *MemoryStore
}

func NewRetriever(store *MemoryStore) *Retriever {
	return &Retriever{store: store}
}

func (r *Retriever) Retrieve(ctx context.Context, query RetrievalQuery) ([]ExperienceMatch, error) {
	signals := append([]string{}, query.Signals...)
	signals = append(signals, ExtractSignals(query.UserText)...)
	signals = append(signals, query.Services...)
	signals = append(signals, query.Middleware...)
	signals = append(signals, query.EvidenceRefs...)
	if query.Operation != "" {
		signals = append(signals, query.Operation)
	}
	if query.Environment != "" {
		signals = append(signals, query.Environment)
	}
	enabled := true
	manifests, err := r.store.ListManifests(ctx, ManifestFilter{Enabled: &enabled})
	if err != nil {
		return nil, err
	}
	var matches []ExperienceMatch
	for _, manifest := range manifests {
		if manifest.Status != PackStatusEnabled || manifest.ReviewStatus != PackStatusApproved {
			continue
		}
		if err := manifest.Validate(); err != nil {
			continue
		}
		if !scopeMatches(query.UserScopes, manifest.AuthorizationScopes) {
			continue
		}
		genes, err := r.store.ListGenes(ctx, manifest.ID)
		if err != nil {
			return nil, err
		}
		genes = executableGenes(genes)
		if len(genes) == 0 {
			continue
		}
		skills, _ := r.store.ListSkills(ctx, manifest.ID)
		capsules, _ := r.store.ListCapsules(ctx, manifest.ID)
		avoidCues, _ := r.store.ListAvoidCues(ctx, manifest.ID)
		bindings, _ := r.store.ListRunnerBindings(ctx, manifest.ID)
		gene, matchedSignals, geneScore := bestGene(genes, signals)
		skillScore, skillFields := scoreSkill(manifest, skills, query.UserText)
		if gene.ID == "" && skillScore == 0 {
			continue
		}
		selectedGene, runner, gaps := ResolveVariant(VariantContextFromFingerprint(query.OSFingerprint), genes, bindings)
		if selectedGene.ID != "" && len(MatchSignals(selectedGene.SignalsMatch, signals)) > 0 {
			gene = selectedGene
			matchedSignals = MatchSignals(gene.SignalsMatch, signals)
			geneScore = float64(len(matchedSignals)) * 2
		}
		if gene.ID == "" {
			gene = selectedGene
		}
		warnings, blocked := avoidWarnings(avoidCues, signals)
		score := geneScore + skillScore + capsuleScore(capsules, query.OSFingerprint)
		confidence := clamp(score / 10)
		if blocked {
			score = score - 5
			confidence = clamp(confidence - 0.4)
			gaps = append(gaps, "命中 AVOID 失败警告，需要人工审核后才能生成 Runner plan")
		}
		if query.OSFingerprint.OSDistribution == "" && hasOSSpecificBinding(bindings) {
			gaps = appendGap(gaps, "需要确认目标主机操作系统")
		}
		if score <= 0 {
			continue
		}
		nextActions := []string{"view_skill", "check_preconditions", "view_history", "mark_not_applicable"}
		if runner.ID != "" && !blocked && len(gaps) == 0 {
			nextActions = append(nextActions, "create_dry_run")
		}
		var capsuleIDs []AssetID
		for _, capsule := range capsules {
			capsuleIDs = append(capsuleIDs, capsule.AssetID)
		}
		matches = append(matches, ExperienceMatch{
			PackID: manifest.ID, SkillTitle: firstNonEmpty(manifest.Title, manifest.Name, manifest.Skill.Title),
			SkillSummary: manifest.Summary, ExperienceType: manifest.Category, UsageMode: usageMode(manifest, bindings),
			Score: score, Confidence: confidence, MatchedSignals: matchedSignals, MatchedSkillFields: skillFields,
			PreconditionGaps: gaps, RiskWarnings: warnings, SelectedGeneID: gene.ID,
			SelectedRunnerBindingID: runner.ID, NextActions: nextActions, GeneAssetID: gene.AssetID, CapsuleAssetIDs: capsuleIDs,
		})
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Score > matches[j].Score })
	return matches, nil
}

func executableGenes(genes []GEPGene) []GEPGene {
	result := make([]GEPGene, 0, len(genes))
	for _, gene := range genes {
		if err := gene.Validate(); err != nil {
			continue
		}
		if report := CheckValidationGate(gene); !report.Passed {
			continue
		}
		result = append(result, gene)
	}
	return result
}

func scopeMatches(userScopes, required []AuthorizationScope) bool {
	if len(required) == 0 {
		return true
	}
	if len(userScopes) == 0 {
		return false
	}
	for _, need := range required {
		needType := strings.TrimSpace(strings.ToLower(need.Type))
		needValue := strings.TrimSpace(strings.ToLower(need.Value))
		for _, have := range userScopes {
			if strings.TrimSpace(strings.ToLower(have.Type)) == needType &&
				strings.TrimSpace(strings.ToLower(have.Value)) == needValue {
				return true
			}
		}
	}
	return false
}

func bestGene(genes []GEPGene, signals []string) (GEPGene, []string, float64) {
	var best GEPGene
	var bestMatches []string
	for _, gene := range genes {
		matches := MatchSignals(gene.SignalsMatch, signals)
		if len(matches) > len(bestMatches) {
			best = gene
			bestMatches = matches
		}
	}
	return best, bestMatches, float64(len(bestMatches)) * 2
}

func scoreSkill(manifest ExperiencePackManifest, skills []SkillAsset, userText string) (float64, []string) {
	if strings.TrimSpace(userText) == "" {
		return 0, nil
	}
	haystacks := map[string]string{
		"title":   manifest.Name + " " + manifest.Title,
		"summary": manifest.Summary,
	}
	for _, skill := range skills {
		haystacks["skill"] += " " + skill.Title + " " + skill.Summary + " " + skill.Content
	}
	var score float64
	var fields []string
	for field, text := range haystacks {
		if strings.Contains(strings.ToLower(text), strings.ToLower(userText)) {
			score += 2
			fields = append(fields, field)
			continue
		}
		for _, signal := range ExtractSignals(userText) {
			if strings.Contains(strings.ToLower(text), strings.ToLower(signal)) {
				score++
				fields = append(fields, field)
				break
			}
		}
	}
	return score, fields
}

func capsuleScore(capsules []GEPCapsule, env EnvironmentFingerprint) float64 {
	var score float64
	for _, capsule := range capsules {
		if capsule.Outcome.Status == "success" {
			score += 1 + capsule.Outcome.Score
		}
		if capsule.Outcome.Status == "failed" {
			score -= 1
		}
		if env.OSDistribution != "" && capsule.EnvFingerprint.OSDistribution == env.OSDistribution {
			score += 1
		}
	}
	return score
}

func avoidWarnings(cues []AvoidCue, signals []string) ([]string, bool) {
	var warnings []string
	blocked := false
	for _, cue := range cues {
		if len(MatchSignals(cue.Signals, signals)) == 0 {
			continue
		}
		warnings = append(warnings, cue.Warning)
		if cue.Blocking {
			blocked = true
		}
	}
	return warnings, blocked
}

func usageMode(manifest ExperiencePackManifest, bindings []RunnerBinding) string {
	if manifest.UsageMode != "" {
		return manifest.UsageMode
	}
	for _, binding := range bindings {
		if binding.Published {
			return UsageExecutable
		}
	}
	if len(bindings) > 0 {
		return UsageGuided
	}
	return UsageDiagnostic
}

func hasOSSpecificBinding(bindings []RunnerBinding) bool {
	for _, binding := range bindings {
		if len(binding.EnvSelector) > 0 {
			return true
		}
	}
	return false
}

func appendGap(gaps []string, gap string) []string {
	for _, existing := range gaps {
		if existing == gap {
			return gaps
		}
	}
	return append(gaps, gap)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func clamp(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
