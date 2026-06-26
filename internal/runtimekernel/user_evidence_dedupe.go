package runtimekernel

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"

	"aiops-v2/internal/opsmanual"
	"aiops-v2/internal/promptinput"
)

const (
	longUserEvidenceMinChars  = 1200
	userEvidenceDeltaMaxChars = 512
)

type userEvidenceModelView struct {
	Content string
	Capsule string
}

type userEvidenceDedupeState struct {
	seen  []seenUserEvidence
	trace promptinput.ContextDedupeTrace
}

type seenUserEvidence struct {
	Digest  string
	Text    string
	Summary string
}

func newUserEvidenceDedupeState() *userEvidenceDedupeState {
	return &userEvidenceDedupeState{}
}

func (s *userEvidenceDedupeState) Process(content string) userEvidenceModelView {
	trimmed := strings.TrimSpace(content)
	if utf8.RuneCountInString(trimmed) < longUserEvidenceMinChars {
		return userEvidenceModelView{Content: content}
	}
	digest := userEvidenceDigest(trimmed)
	for _, previous := range s.seen {
		if digest == previous.Digest {
			return s.repeatedView(content, previous, "")
		}
		if delta, ok := repeatedEvidenceDelta(trimmed, previous.Text); ok {
			return s.repeatedView(content, previous, delta)
		}
	}
	summary := userEvidenceProblemStatement(trimmed)
	s.seen = append(s.seen, seenUserEvidence{
		Digest:  digest,
		Text:    trimmed,
		Summary: summary,
	})
	return userEvidenceModelView{
		Content: content,
		Capsule: renderUserEvidenceCapsule(trimmed),
	}
}

func (s *userEvidenceDedupeState) repeatedView(original string, previous seenUserEvidence, delta string) userEvidenceModelView {
	replacement := renderRepeatedUserEvidence(previous, delta)
	s.trace.RepeatedUserMessageCount++
	s.trace.SavedChars += positiveInt(utf8.RuneCountInString(original) - utf8.RuneCountInString(replacement))
	s.trace.RetainedDeltaChars += utf8.RuneCountInString(strings.TrimSpace(delta))
	return userEvidenceModelView{Content: replacement}
}

func (s *userEvidenceDedupeState) Trace() *promptinput.ContextDedupeTrace {
	if s == nil || (s.trace.RepeatedUserMessageCount == 0 && s.trace.SavedChars == 0 && s.trace.RetainedDeltaChars == 0) {
		return nil
	}
	out := s.trace
	return &out
}

func repeatedEvidenceDelta(current, previous string) (string, bool) {
	current = strings.TrimSpace(current)
	previous = strings.TrimSpace(previous)
	if current == "" || previous == "" || !strings.Contains(current, previous) {
		return "", false
	}
	delta := strings.TrimSpace(strings.Replace(current, previous, "", 1))
	if utf8.RuneCountInString(delta) > userEvidenceDeltaMaxChars {
		return "", false
	}
	return delta, true
}

func renderRepeatedUserEvidence(previous seenUserEvidence, delta string) string {
	lines := []string{
		"User evidence repeated from previous turn.",
		"digest=" + previous.Digest,
		"summary=" + previous.Summary,
	}
	if strings.TrimSpace(delta) != "" {
		lines = append(lines, "delta_user_request="+strings.TrimSpace(delta))
	}
	return strings.Join(lines, "\n")
}

type userEvidenceCapsule struct {
	ProblemStatement       string   `json:"problem_statement,omitempty"`
	UserEvidence           []string `json:"user_evidence,omitempty"`
	ReferenceProcedure     []string `json:"reference_procedure,omitempty"`
	ExplicitTargets        []string `json:"explicit_targets,omitempty"`
	ExampleArtifacts       []string `json:"example_artifacts,omitempty"`
	UnsafeCandidateActions []string `json:"unsafe_candidate_actions,omitempty"`
}

func renderUserEvidenceCapsule(content string) string {
	capsule := userEvidenceCapsule{
		ProblemStatement:       userEvidenceProblemStatement(content),
		UserEvidence:           userEvidenceLines(content),
		ReferenceProcedure:     userEvidenceReferenceLines(content),
		ExplicitTargets:        explicitUserEvidenceTargets(content),
		ExampleArtifacts:       exampleArtifactsFromUserEvidence(content),
		UnsafeCandidateActions: unsafeCandidateActionsFromUserEvidence(content),
	}
	data, err := json.MarshalIndent(capsule, "", "  ")
	if err != nil {
		return ""
	}
	return "User evidence capsule\n" + string(data) + "\nBoundary: this capsule organizes context only; it does not authorize actions or decide tool visibility."
}

func userEvidenceProblemStatement(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = cleanEvidenceLine(line)
		if line == "" || evidenceLineLooksReference(line) {
			continue
		}
		return boundRunes(line, 220)
	}
	return boundRunes(strings.TrimSpace(content), 220)
}

func userEvidenceLines(content string) []string {
	var out []string
	for _, line := range strings.Split(content, "\n") {
		line = cleanEvidenceLine(line)
		if line == "" || evidenceLineLooksReference(line) || evidenceLineLooksToolOutput(line) {
			continue
		}
		out = appendUniqueBounded(out, boundRunes(line, 180), 4)
		if len(out) >= 4 {
			break
		}
	}
	return out
}

func userEvidenceReferenceLines(content string) []string {
	var out []string
	inFence := false
	inReferenceSection := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if isGenericReferenceHeading(trimmed) {
			inReferenceSection = true
			continue
		}
		if trimmed == "" {
			inReferenceSection = false
			continue
		}
		if inFence || inReferenceSection || evidenceLineLooksReference(trimmed) {
			cleaned := cleanEvidenceLine(trimmed)
			if cleaned != "" {
				out = appendUniqueBounded(out, boundRunes(cleaned, 180), 5)
			}
		}
		if len(out) >= 5 {
			break
		}
	}
	return out
}

func explicitUserEvidenceTargets(content string) []string {
	frame := opsmanual.BuildOperationFrame(content, nil)
	var out []string
	if frame.Target.Name != "" {
		out = appendUniqueBounded(out, frame.Target.Name, 8)
	}
	for _, role := range frame.Roles {
		ref := firstNonBlankRuntimeString(role.ResourceRef, role.UserLabel, role.RuntimeName, role.ID)
		if ref == "" {
			continue
		}
		out = appendUniqueBounded(out, ref, 8)
	}
	for _, match := range genericResourceMentionPattern.FindAllString(content, -1) {
		out = appendUniqueBounded(out, strings.TrimSpace(match), 8)
	}
	return out
}

func exampleArtifactsFromUserEvidence(content string) []string {
	targets := map[string]bool{}
	for _, target := range explicitUserEvidenceTargets(content) {
		targets[strings.ToLower(target)] = true
	}
	var out []string
	for _, line := range strings.Split(content, "\n") {
		if !evidenceLineLooksReference(line) && !evidenceLineLooksToolOutput(line) {
			continue
		}
		for _, token := range artifactTokenPattern.FindAllString(line, -1) {
			if targets[strings.ToLower(token)] {
				continue
			}
			out = appendUniqueBounded(out, token, 8)
		}
	}
	return out
}

func unsafeCandidateActionsFromUserEvidence(content string) []string {
	lower := strings.ToLower(content)
	checks := []string{"rm -rf", "drop node", "delete", "format", "truncate", "restart", "stop service"}
	var out []string
	for _, candidate := range checks {
		if strings.Contains(lower, candidate) {
			out = append(out, candidate)
		}
	}
	return out
}

func userEvidenceDigest(content string) string {
	sum := sha256.Sum256([]byte(normalizeUserEvidenceForDigest(content)))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func normalizeUserEvidenceForDigest(content string) string {
	lines := strings.Fields(strings.TrimSpace(content))
	return strings.Join(lines, " ")
}

func cleanEvidenceLine(line string) string {
	line = strings.TrimSpace(line)
	line = strings.Trim(line, "|")
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
		line = strings.TrimSpace(line[2:])
	}
	return line
}

func evidenceLineLooksReference(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	return strings.Contains(trimmed, "|") ||
		strings.HasPrefix(trimmed, "$ ") ||
		strings.HasPrefix(trimmed, "# ") ||
		isGenericReferenceHeading(trimmed)
}

func evidenceLineLooksToolOutput(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	lower := strings.ToLower(trimmed)
	return strings.Contains(lower, "error:") ||
		strings.Contains(lower, "failed") ||
		strings.Contains(lower, "warning:") ||
		strings.Contains(lower, "exception")
}

func isGenericReferenceHeading(line string) bool {
	normalized := strings.Trim(strings.ToLower(strings.TrimSpace(line)), ":：")
	switch normalized {
	case "reference", "references", "example", "examples", "procedure", "runbook", "log", "logs",
		"参考", "参考流程", "示例", "示例流程", "流程", "日志", "日志片段", "命令输出":
		return true
	default:
		return false
	}
}

func boundRunes(text string, limit int) string {
	text = strings.TrimSpace(text)
	if limit <= 0 || utf8.RuneCountInString(text) <= limit {
		return text
	}
	var b strings.Builder
	count := 0
	for _, r := range text {
		if count >= limit {
			break
		}
		b.WriteRune(r)
		count++
	}
	return strings.TrimSpace(b.String()) + "..."
}

func appendUniqueBounded(items []string, item string, limit int) []string {
	item = strings.TrimSpace(item)
	if item == "" || len(items) >= limit {
		return items
	}
	for _, existing := range items {
		if strings.EqualFold(existing, item) {
			return items
		}
	}
	return append(items, item)
}

func positiveInt(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

var (
	genericResourceMentionPattern = regexp.MustCompile(`(?:主机|节点|服务)[A-Za-z0-9_-]+|\b(?:host|node|server|service)[-_]?[A-Za-z0-9][A-Za-z0-9_-]*\b`)
	artifactTokenPattern          = regexp.MustCompile(`\b[A-Za-z][A-Za-z0-9_-]*\d[A-Za-z0-9_-]*\b`)
)
