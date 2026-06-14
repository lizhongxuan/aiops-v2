package hostops

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var ErrInvalidHostAgentContext = errors.New("invalid host agent runtime context")

const (
	ContextDecisionScopeViolation = "scope_violation"
	contextDecisionIncluded       = "included"
	contextDecisionExternalized   = "externalized"
)

type HostAgentContextBuildInput struct {
	MissionID          string
	ParentAgentID      string
	HostAgentID        string
	SessionID          string
	HostID             string
	HostAddress        string
	HostDisplayName    string
	PlanStep           PlanStep
	Goal               string
	Constraints        []string
	ContextRefs        []ContextRef
	AllowedToolScopes  []string
	AllowedSkillScopes []string
	AllowedMCPScope    []string
	CompletionContract string
}

type HostAgentRuntimeContext struct {
	MissionID            string
	ParentAgentID        string
	HostAgentID          string
	SessionID            string
	Host                 HostRuntimeContextHost
	PlanStep             HostRuntimeContextPlanStep
	Goal                 string
	Constraints          []string
	Risk                 string
	EvidenceRequirements []string
	CompletionContract   string
	ContextRefs          []ContextRef
	AllowedToolScopes    []string
	AllowedSkillScopes   []string
	AllowedMCPScope      []string
}

type HostRuntimeContextHost struct {
	ID          string
	Address     string
	DisplayName string
}

type HostRuntimeContextPlanStep struct {
	ID      string
	Title   string
	Summary string
}

type ContextRef struct {
	ID          string
	Kind        string
	ScopeHostID string
	Summary     string
	Content     string
	ArtifactRef string
	Digest      string
}

type ContextDecisionTrace struct {
	SourceID     string
	Included     []ContextDecisionTraceItem
	Excluded     []ContextDecisionTraceItem
	Externalized []ContextDecisionTraceItem
}

type ContextDecisionTraceItem struct {
	ID     string
	Kind   string
	Reason string
	Digest string
	Ref    string
}

func BuildHostAgentRuntimeContext(input HostAgentContextBuildInput) (HostAgentRuntimeContext, ContextDecisionTrace, error) {
	input.MissionID = strings.TrimSpace(input.MissionID)
	input.HostID = strings.TrimSpace(input.HostID)
	input.PlanStep.ID = strings.TrimSpace(input.PlanStep.ID)
	input.Goal = strings.TrimSpace(input.Goal)
	if input.MissionID == "" || input.HostID == "" || input.PlanStep.ID == "" || input.Goal == "" {
		return HostAgentRuntimeContext{}, ContextDecisionTrace{}, ErrInvalidHostAgentContext
	}
	if len(input.PlanStep.HostIDs) > 0 && !containsNormalized(input.PlanStep.HostIDs, input.HostID) {
		return HostAgentRuntimeContext{}, ContextDecisionTrace{}, fmt.Errorf("%w: plan step is not bound to host", ErrInvalidHostAgentContext)
	}

	trace := ContextDecisionTrace{SourceID: hostContextSourceID(input.MissionID, input.HostID, input.PlanStep.ID)}
	trace.Included = append(trace.Included, ContextDecisionTraceItem{ID: trace.SourceID, Kind: "host_runtime_binding", Reason: contextDecisionIncluded})
	ctx := HostAgentRuntimeContext{
		MissionID:            input.MissionID,
		ParentAgentID:        strings.TrimSpace(input.ParentAgentID),
		HostAgentID:          strings.TrimSpace(input.HostAgentID),
		SessionID:            strings.TrimSpace(input.SessionID),
		Host:                 HostRuntimeContextHost{ID: input.HostID, Address: strings.TrimSpace(input.HostAddress), DisplayName: strings.TrimSpace(input.HostDisplayName)},
		PlanStep:             HostRuntimeContextPlanStep{ID: input.PlanStep.ID, Title: RedactSensitiveText(input.PlanStep.Title), Summary: RedactSensitiveText(input.PlanStep.Summary)},
		Goal:                 RedactSensitiveText(input.Goal),
		Constraints:          redactStringSlice(input.Constraints),
		Risk:                 strings.TrimSpace(string(input.PlanStep.RiskLevel)),
		EvidenceRequirements: append([]string(nil), input.PlanStep.EvidenceRequired...),
		CompletionContract:   strings.TrimSpace(input.CompletionContract),
		AllowedToolScopes:    cleanContextList(input.AllowedToolScopes),
		AllowedSkillScopes:   cleanContextList(input.AllowedSkillScopes),
		AllowedMCPScope:      cleanContextList(input.AllowedMCPScope),
	}
	if ctx.CompletionContract == "" {
		ctx.CompletionContract = "Return a HostTaskReport with status, evidence refs, command summaries, blockers, errors, and next steps."
	}
	for _, ref := range input.ContextRefs {
		ref = normalizeContextRef(ref)
		if ref.ID == "" {
			continue
		}
		if ref.ScopeHostID != "" && !strings.EqualFold(ref.ScopeHostID, input.HostID) {
			trace.Excluded = append(trace.Excluded, ContextDecisionTraceItem{ID: ref.ID, Kind: ref.Kind, Reason: ContextDecisionScopeViolation})
			continue
		}
		if len(ref.Content) > 512 {
			digest := digestText(ref.Content)
			ref.Summary = firstNonEmptyString(ref.Summary, "Large context externalized; use artifact ref and digest for retrieval.")
			ref.Digest = digest
			ref.ArtifactRef = firstNonEmptyString(ref.ArtifactRef, "artifact://"+input.MissionID+"/"+input.HostID+"/"+ref.ID)
			ref.Content = ""
			trace.Externalized = append(trace.Externalized, ContextDecisionTraceItem{ID: ref.ID, Kind: ref.Kind, Reason: contextDecisionExternalized, Digest: digest, Ref: ref.ArtifactRef})
		} else {
			trace.Included = append(trace.Included, ContextDecisionTraceItem{ID: ref.ID, Kind: ref.Kind, Reason: contextDecisionIncluded, Digest: ref.Digest, Ref: ref.ArtifactRef})
		}
		ctx.ContextRefs = append(ctx.ContextRefs, ref)
	}
	return ctx, trace, nil
}

func hostContextSourceID(missionID, hostID, stepID string) string {
	return "host-context:" + sanitizeIDPart(missionID) + ":" + sanitizeIDPart(hostID) + ":" + sanitizeIDPart(stepID)
}

func normalizeContextRef(ref ContextRef) ContextRef {
	ref.ID = strings.TrimSpace(ref.ID)
	ref.Kind = strings.TrimSpace(ref.Kind)
	ref.ScopeHostID = strings.TrimSpace(ref.ScopeHostID)
	ref.Summary = RedactSensitiveText(ref.Summary)
	ref.Content = RedactSensitiveText(ref.Content)
	ref.ArtifactRef = strings.TrimSpace(ref.ArtifactRef)
	ref.Digest = strings.TrimSpace(ref.Digest)
	return ref
}

func containsNormalized(values []string, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	for _, value := range values {
		if strings.ToLower(strings.TrimSpace(value)) == target {
			return true
		}
	}
	return false
}

func redactStringSlice(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, RedactSensitiveText(value))
	}
	return out
}

func cleanContextList(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

var sensitivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(bearer)\s+[A-Za-z0-9._~+/=-]+`),
	regexp.MustCompile(`(?i)--(token|password|secret)(=|\s+)[^,\s;]+`),
	regexp.MustCompile(`(?i)(token|password|secret)\s*[:=]\s*[^,\s;]+`),
	regexp.MustCompile(`(?i)(cookie)\s*:\s*[^,\n]+`),
	regexp.MustCompile(`(?is)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`(?i)private key\s+[^,\n]+`),
}

func RedactSensitiveText(text string) string {
	out := strings.TrimSpace(text)
	if out == "" {
		return ""
	}
	for _, pattern := range sensitivePatterns {
		out = pattern.ReplaceAllStringFunc(out, func(match string) string {
			return "[REDACTED:" + digestText(match)[:12] + "]"
		})
	}
	return out
}

func digestText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}
