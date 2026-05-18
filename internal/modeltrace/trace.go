package modeltrace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/diagnostics"
	"aiops-v2/internal/promptinput"
)

const (
	EnabledEnv = "AIOPS_DEBUG_MODEL_INPUT_TRACE"
	DirEnv     = "AIOPS_DEBUG_MODEL_INPUT_TRACE_DIR"
)

type Prompt struct {
	StableHash string `json:"stableHash,omitempty"`
	Stable     string `json:"stable,omitempty"`
	Dynamic    string `json:"dynamic,omitempty"`
	System     string `json:"system,omitempty"`
	Developer  string `json:"developer,omitempty"`
	Tools      string `json:"tools,omitempty"`
	Policy     string `json:"policy,omitempty"`
}

type Request struct {
	Kind              string
	TraceID           string
	SessionID         string
	TurnID            string
	Iteration         int
	CaseID            string
	Metadata          map[string]string
	VisibleTools      []string
	PromptFingerprint map[string]string
	Prompt            Prompt
	ModelInput        []*schema.Message
	PromptInputTrace  promptinput.PromptInputTrace
	PromptInputDiff   *promptinput.TraceDiff
	DiagnosticTrace   diagnostics.DiagnosticTrace
}

type payload struct {
	SchemaVersion         int                          `json:"schemaVersion"`
	Kind                  string                       `json:"kind,omitempty"`
	CreatedAt             string                       `json:"createdAt"`
	TraceID               string                       `json:"traceId,omitempty"`
	SessionID             string                       `json:"sessionId,omitempty"`
	TurnID                string                       `json:"turnId,omitempty"`
	Iteration             int                          `json:"iteration,omitempty"`
	CaseID                string                       `json:"caseId,omitempty"`
	Metadata              map[string]string            `json:"metadata,omitempty"`
	VisibleTools          []string                     `json:"visibleTools,omitempty"`
	VisibleToolCount      int                          `json:"visibleToolCount,omitempty"`
	PromptCharCount       int                          `json:"promptCharCount,omitempty"`
	ToolRegistryCharCount int                          `json:"toolRegistryCharCount,omitempty"`
	PromptFingerprint     map[string]string            `json:"promptFingerprint,omitempty"`
	Prompt                Prompt                       `json:"prompt"`
	ModelInput            []traceMessage               `json:"modelInput"`
	PromptInputTrace      promptinput.PromptInputTrace `json:"promptInputTrace,omitempty"`
	DiagnosticTrace       *diagnostics.DiagnosticTrace `json:"diagnosticTrace,omitempty"`
}

type traceMessage struct {
	Index        int               `json:"index"`
	ProviderRole string            `json:"providerRole"`
	SemanticRole string            `json:"semanticRole,omitempty"`
	PromptLayer  string            `json:"promptLayer,omitempty"`
	Name         string            `json:"name,omitempty"`
	Content      string            `json:"content,omitempty"`
	ToolCallID   string            `json:"toolCallId,omitempty"`
	ToolName     string            `json:"toolName,omitempty"`
	ToolCalls    []schema.ToolCall `json:"toolCalls,omitempty"`
}

func Write(req Request) (string, error) {
	if !Enabled() {
		return "", nil
	}
	traceDir, err := traceDirectory(req)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(traceDir, 0o755); err != nil {
		return "", fmt.Errorf("create model input trace dir: %w", err)
	}

	payload := buildPayload(req)
	stamp := time.Now().UTC().Format("20060102T150405.000000000Z")
	base := traceFileBase(req, stamp)
	jsonPath := filepath.Join(traceDir, base+".json")
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal model input trace: %w", err)
	}
	if err := os.WriteFile(jsonPath, append(data, '\n'), 0o644); err != nil {
		return "", fmt.Errorf("write model input trace json: %w", err)
	}
	if err := os.WriteFile(filepath.Join(traceDir, base+".md"), []byte(renderMarkdown(payload)), 0o644); err != nil {
		return "", fmt.Errorf("write model input trace markdown: %w", err)
	}
	if req.PromptInputDiff != nil {
		diffMarkdown := []byte(promptinput.RenderDiffMarkdown(*req.PromptInputDiff))
		if err := os.WriteFile(filepath.Join(traceDir, "input.diff.md"), diffMarkdown, 0o644); err != nil {
			return "", fmt.Errorf("write model input diff markdown: %w", err)
		}
		if err := os.WriteFile(filepath.Join(traceDir, base+".diff.md"), diffMarkdown, 0o644); err != nil {
			return "", fmt.Errorf("write timestamped model input diff markdown: %w", err)
		}
	}
	return jsonPath, nil
}

func Enabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnabledEnv))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func buildPayload(req Request) payload {
	visibleTools := append([]string(nil), req.VisibleTools...)
	modelInput := traceMessages(req.ModelInput)
	return payload{
		SchemaVersion:         1,
		Kind:                  strings.TrimSpace(req.Kind),
		CreatedAt:             time.Now().UTC().Format(time.RFC3339Nano),
		TraceID:               strings.TrimSpace(req.TraceID),
		SessionID:             strings.TrimSpace(req.SessionID),
		TurnID:                strings.TrimSpace(req.TurnID),
		Iteration:             req.Iteration,
		CaseID:                firstNonEmpty(req.CaseID, req.Metadata["eval.caseId"], req.Metadata["caseId"]),
		Metadata:              redactStringMap(copyStringMap(req.Metadata)),
		VisibleTools:          visibleTools,
		VisibleToolCount:      len(visibleTools),
		PromptCharCount:       modelInputCharCount(modelInput),
		ToolRegistryCharCount: len(req.Prompt.Tools),
		PromptFingerprint:     copyStringMap(req.PromptFingerprint),
		Prompt:                redactPrompt(req.Prompt),
		ModelInput:            modelInput,
		PromptInputTrace:      redactPromptInputTrace(req.PromptInputTrace),
		DiagnosticTrace:       diagnosticTracePayload(req.DiagnosticTrace),
	}
}

func modelInputCharCount(messages []traceMessage) int {
	total := 0
	for _, msg := range messages {
		total += len(msg.Content)
	}
	return total
}

func diagnosticTracePayload(trace diagnostics.DiagnosticTrace) *diagnostics.DiagnosticTrace {
	if diagnosticTraceEmpty(trace) {
		return nil
	}
	redacted := diagnostics.RedactTrace(trace)
	return &redacted
}

func diagnosticTraceEmpty(trace diagnostics.DiagnosticTrace) bool {
	return strings.TrimSpace(trace.TurnID) == "" &&
		strings.TrimSpace(trace.ScopeHash) == "" &&
		strings.TrimSpace(trace.ScopeSummary) == "" &&
		len(trace.Hypotheses) == 0 &&
		len(trace.ObservedEvidence) == 0 &&
		len(trace.RefutingEvidence) == 0 &&
		len(trace.MissingEvidence) == 0 &&
		len(trace.ToolFailures) == 0 &&
		strings.TrimSpace(trace.ManualBindingID) == "" &&
		trace.Confidence == "" &&
		strings.TrimSpace(trace.ConfidenceReason) == "" &&
		!trace.RequiresApproval
}

func traceMessages(messages []*schema.Message) []traceMessage {
	out := make([]traceMessage, 0, len(messages))
	for i, msg := range messages {
		if msg == nil {
			continue
		}
		item := traceMessage{
			Index:        i,
			ProviderRole: string(msg.Role),
			Name:         msg.Name,
			Content:      diagnostics.RedactSensitiveText(msg.Content),
			ToolCallID:   msg.ToolCallID,
			ToolName:     msg.ToolName,
			ToolCalls:    redactToolCalls(msg.ToolCalls),
		}
		if msg.Extra != nil {
			if role, ok := msg.Extra["semantic_role"].(string); ok {
				item.SemanticRole = role
			}
			if layer, ok := msg.Extra["prompt_layer"].(string); ok {
				item.PromptLayer = layer
			}
		}
		out = append(out, item)
	}
	return out
}

func redactPrompt(prompt Prompt) Prompt {
	return Prompt{
		StableHash: prompt.StableHash,
		Stable:     diagnostics.RedactSensitiveText(prompt.Stable),
		Dynamic:    diagnostics.RedactSensitiveText(prompt.Dynamic),
		System:     diagnostics.RedactSensitiveText(prompt.System),
		Developer:  diagnostics.RedactSensitiveText(prompt.Developer),
		Tools:      diagnostics.RedactSensitiveText(prompt.Tools),
		Policy:     diagnostics.RedactSensitiveText(prompt.Policy),
	}
}

func redactToolCalls(calls []schema.ToolCall) []schema.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]schema.ToolCall, 0, len(calls))
	for _, call := range calls {
		call.Function.Name = diagnostics.RedactSensitiveText(call.Function.Name)
		call.Function.Arguments = diagnostics.RedactSensitiveText(call.Function.Arguments)
		out = append(out, call)
	}
	return out
}

func redactPromptInputTrace(trace promptinput.PromptInputTrace) promptinput.PromptInputTrace {
	if len(trace.Items) == 0 {
		return promptinput.PromptInputTrace{}
	}
	out := promptinput.PromptInputTrace{Items: make([]promptinput.TraceItem, 0, len(trace.Items))}
	for _, item := range trace.Items {
		item.ID = diagnostics.RedactSensitiveText(item.ID)
		item.Content = diagnostics.RedactSensitiveText(item.Content)
		out.Items = append(out.Items, item)
	}
	return out
}

func redactStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = diagnostics.RedactSensitiveText(value)
	}
	return out
}

func traceDirectory(req Request) (string, error) {
	root := strings.TrimSpace(os.Getenv(DirEnv))
	if root == "" {
		root = filepath.Join(".data", "model-input-traces")
	}
	kind := sanitizePath(firstNonEmpty(req.Kind, "model-call"))
	if strings.TrimSpace(req.SessionID) != "" || strings.TrimSpace(req.TurnID) != "" {
		return filepath.Join(root, sanitizePath(req.SessionID), sanitizePath(req.TurnID)), nil
	}
	return filepath.Join(root, kind, sanitizePath(req.TraceID)), nil
}

func traceFileBase(req Request, stamp string) string {
	if strings.TrimSpace(req.SessionID) != "" || strings.TrimSpace(req.TurnID) != "" || req.Iteration > 0 {
		return fmt.Sprintf("iteration-%03d-%s", req.Iteration, stamp)
	}
	return fmt.Sprintf("%s-%s", sanitizePath(firstNonEmpty(req.Kind, "model-call")), stamp)
}

func renderMarkdown(payload payload) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Model Input Trace\n\n")
	fmt.Fprintf(&b, "- Schema: `%d`\n", payload.SchemaVersion)
	if payload.Kind != "" {
		fmt.Fprintf(&b, "- Kind: `%s`\n", payload.Kind)
	}
	if payload.TraceID != "" {
		fmt.Fprintf(&b, "- Trace: `%s`\n", payload.TraceID)
	}
	if payload.SessionID != "" {
		fmt.Fprintf(&b, "- Session: `%s`\n", payload.SessionID)
	}
	if payload.TurnID != "" {
		fmt.Fprintf(&b, "- Turn: `%s`\n", payload.TurnID)
	}
	if payload.Iteration > 0 {
		fmt.Fprintf(&b, "- Iteration: `%d`\n", payload.Iteration)
	}
	if payload.CaseID != "" {
		fmt.Fprintf(&b, "- Eval case: `%s`\n", payload.CaseID)
	}
	fmt.Fprintf(&b, "- Created: `%s`\n", payload.CreatedAt)
	if len(payload.VisibleTools) > 0 {
		fmt.Fprintf(&b, "- Visible tools: `%s`\n", strings.Join(payload.VisibleTools, "`, `"))
	}
	if len(payload.PromptFingerprint) > 0 {
		if stable := strings.TrimSpace(payload.PromptFingerprint["stableHash"]); stable != "" {
			fmt.Fprintf(&b, "- Prompt fingerprint: `%s`\n", stable)
		}
	}
	fmt.Fprintf(&b, "\n## Prompt Delta\n\n```text\n%s\n```\n", payload.Prompt.Dynamic)
	fmt.Fprintf(&b, "\n## Model Input\n")
	for _, msg := range payload.ModelInput {
		fmt.Fprintf(&b, "\n### %02d %s", msg.Index, msg.ProviderRole)
		if msg.SemanticRole != "" || msg.PromptLayer != "" {
			fmt.Fprintf(&b, " [%s/%s]", msg.SemanticRole, msg.PromptLayer)
		}
		fmt.Fprintf(&b, "\n\n```text\n%s\n```\n", msg.Content)
		if len(msg.ToolCalls) > 0 {
			data, _ := json.MarshalIndent(msg.ToolCalls, "", "  ")
			fmt.Fprintf(&b, "\nTool calls:\n\n```json\n%s\n```\n", string(data))
		}
	}
	if len(payload.PromptInputTrace.Items) > 0 {
		traceMarkdown := promptinput.RenderMarkdown(payload.PromptInputTrace)
		traceMarkdown = strings.Replace(traceMarkdown, "# Prompt Input Trace", "## Prompt Input Trace", 1)
		fmt.Fprintf(&b, "\n%s", traceMarkdown)
	}
	if payload.DiagnosticTrace != nil {
		fmt.Fprintf(&b, "\n%s", renderDiagnosticTraceMarkdown(*payload.DiagnosticTrace))
	}
	return b.String()
}

func renderDiagnosticTraceMarkdown(trace diagnostics.DiagnosticTrace) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n## Diagnostic Trace\n\n")
	if trace.ScopeHash != "" || trace.ScopeSummary != "" {
		fmt.Fprintf(&b, "- Scope: `%s` %s\n", trace.ScopeHash, trace.ScopeSummary)
	}
	if trace.ManualBindingID != "" {
		fmt.Fprintf(&b, "- Manual binding: `%s`\n", trace.ManualBindingID)
	}
	if trace.Confidence != "" || trace.ConfidenceReason != "" {
		fmt.Fprintf(&b, "- Confidence: `%s` %s\n", trace.Confidence, trace.ConfidenceReason)
	}
	if trace.RequiresApproval {
		fmt.Fprintf(&b, "- Requires approval: `true`\n")
	}
	writeMarkdownList(&b, "Hypotheses", trace.Hypotheses)
	writeMarkdownList(&b, "Observed Evidence", trace.ObservedEvidence)
	writeMarkdownList(&b, "Refuting Evidence", trace.RefutingEvidence)
	writeMarkdownList(&b, "Missing Evidence", trace.MissingEvidence)
	if len(trace.ToolFailures) > 0 {
		fmt.Fprintf(&b, "\n### Tool Failures\n")
		for _, failure := range trace.ToolFailures {
			fmt.Fprintf(&b, "- `%s` `%s` critical=%t: %s\n", failure.ToolName, failure.Semantic, failure.Critical, failure.Detail)
		}
	}
	return b.String()
}

func writeMarkdownList(b *strings.Builder, title string, values []string) {
	if len(values) == 0 {
		return
	}
	fmt.Fprintf(b, "\n### %s\n", title)
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		fmt.Fprintf(b, "- %s\n", value)
	}
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

var pathUnsafe = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func sanitizePath(value string) string {
	value = pathUnsafe.ReplaceAllString(strings.TrimSpace(value), "-")
	value = strings.Trim(value, ".-")
	if value == "" {
		return "unknown"
	}
	return value
}
