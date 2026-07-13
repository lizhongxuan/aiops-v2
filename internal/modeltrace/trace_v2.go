package modeltrace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"aiops-v2/internal/diagnostics"
	"aiops-v2/internal/specialinputmemory"
)

const TraceDocumentV2SchemaVersion = "aiops.trace/v2"

type TraceDocumentV2 struct {
	SchemaVersion               string                                            `json:"schemaVersion"`
	CreatedAt                   string                                            `json:"createdAt"`
	SessionID                   string                                            `json:"sessionId"`
	TurnID                      string                                            `json:"turnId"`
	Iteration                   int                                               `json:"iteration"`
	Metadata                    map[string]string                                 `json:"metadata,omitempty"`
	VisibleTools                []string                                          `json:"visibleTools,omitempty"`
	PromptFingerprint           map[string]string                                 `json:"promptFingerprint,omitempty"`
	PreviousPromptFingerprint   map[string]string                                 `json:"previousPromptFingerprint,omitempty"`
	TurnContext                 any                                               `json:"turnContext,omitempty"`
	StepContext                 any                                               `json:"stepContext,omitempty"`
	StepContextHash             string                                            `json:"stepContextHash,omitempty"`
	HarnessTurn                 any                                               `json:"harnessTurn,omitempty"`
	ProviderRequest             ProviderRequestTrace                              `json:"providerRequest,omitempty"`
	ToolSurface                 any                                               `json:"toolSurface,omitempty"`
	TurnAssembly                any                                               `json:"turnAssembly,omitempty"`
	LegacyAgentAssemblySnapshot any                                               `json:"legacyAgentAssemblySnapshot,omitempty"`
	TurnAssemblyShadow          any                                               `json:"turnAssemblyShadow,omitempty"`
	Prompt                      Prompt                                            `json:"prompt,omitempty"`
	ModelInput                  any                                               `json:"modelInput,omitempty"`
	LLMRequests                 []any                                             `json:"llmRequests,omitempty"`
	SpecialInputWorldState      *specialinputmemory.SpecialInputWorldStateSection `json:"specialInputWorldState,omitempty"`
	PromptInputTrace            any                                               `json:"promptInputTrace,omitempty"`
	PromptInputDiff             any                                               `json:"promptInputDiff,omitempty"`
	DiagnosticTrace             any                                               `json:"diagnosticTrace,omitempty"`
	FinalEvidenceState          any                                               `json:"finalEvidenceState,omitempty"`
	RawPayloadRefs              []RawPayloadRef                                   `json:"rawPayloadRefs,omitempty"`
}

type ProviderRequestTrace struct {
	ModelInputHash        string `json:"modelInputHash,omitempty"`
	ProviderMessagesHash  string `json:"providerMessagesHash,omitempty"`
	RequestPropertiesHash string `json:"requestPropertiesHash,omitempty"`
	PromptCacheKey        string `json:"promptCacheKey,omitempty"`
}

func WriteTraceDocumentV2(root string, doc TraceDocumentV2) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		root = DefaultRootDir("")
	}
	if doc.SchemaVersion == "" {
		doc.SchemaVersion = TraceDocumentV2SchemaVersion
	}
	if doc.CreatedAt == "" {
		doc.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	doc = redactTraceDocumentV2(doc)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	name := traceDocumentV2FileName(doc)
	path := filepath.Join(root, name)
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	if err := writeTraceDocumentV2Markdown(strings.TrimSuffix(path, filepath.Ext(path))+".md", doc); err != nil {
		return "", err
	}
	if doc.PromptInputDiff != nil {
		if err := writeTraceDocumentV2DiffMarkdown(filepath.Join(root, "input.diff.md"), doc.PromptInputDiff); err != nil {
			return "", err
		}
	}
	if err := writeTraceDocumentV2Index(root, path, doc); err != nil {
		return "", err
	}
	return path, nil
}

func redactTraceDocumentV2(doc TraceDocumentV2) TraceDocumentV2 {
	doc.Metadata = redactTraceV2StringMap(doc.Metadata)
	doc.TurnContext = redactTraceV2Value(doc.TurnContext)
	doc.StepContext = redactTraceV2Value(doc.StepContext)
	doc.HarnessTurn = redactTraceV2Value(doc.HarnessTurn)
	doc.ToolSurface = redactTraceV2Value(doc.ToolSurface)
	doc.TurnAssembly = redactTraceV2Value(doc.TurnAssembly)
	doc.LegacyAgentAssemblySnapshot = redactTraceV2Value(doc.LegacyAgentAssemblySnapshot)
	doc.TurnAssemblyShadow = redactTraceV2Value(doc.TurnAssemblyShadow)
	doc.Prompt = redactPrompt(doc.Prompt)
	doc.ModelInput = redactTraceV2Value(doc.ModelInput)
	if redacted := redactTraceV2Value(doc.LLMRequests); redacted != nil {
		doc.LLMRequests, _ = redacted.([]any)
	} else {
		doc.LLMRequests = nil
	}
	doc.SpecialInputWorldState = redactTraceV2WorldState(doc.SpecialInputWorldState)
	doc.PromptInputTrace = redactTraceV2Value(doc.PromptInputTrace)
	doc.PromptInputDiff = redactTraceV2Value(doc.PromptInputDiff)
	doc.DiagnosticTrace = redactTraceV2Value(doc.DiagnosticTrace)
	doc.FinalEvidenceState = redactTraceV2Value(doc.FinalEvidenceState)
	return doc
}

func redactTraceV2WorldState(input *specialinputmemory.SpecialInputWorldStateSection) *specialinputmemory.SpecialInputWorldStateSection {
	if input == nil {
		return nil
	}
	redacted := redactTraceV2Value(input)
	data, err := json.Marshal(redacted)
	if err != nil {
		return nil
	}
	var out specialinputmemory.SpecialInputWorldStateSection
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return &out
}

func redactTraceV2StringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		if traceV2SensitiveKey(key) {
			out[key] = "[REDACTED]"
			continue
		}
		out[key] = diagnostics.RedactSensitiveText(value)
	}
	return out
}

func redactTraceV2Value(input any) any {
	if input == nil {
		return nil
	}
	data, err := json.Marshal(input)
	if err != nil {
		return nil
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil
	}
	return redactTraceV2JSONValue(value)
}

func redactTraceV2JSONValue(value any) any {
	switch typed := value.(type) {
	case string:
		return diagnostics.RedactSensitiveText(typed)
	case []any:
		out := make([]any, len(typed))
		for index := range typed {
			out[index] = redactTraceV2JSONValue(typed[index])
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			if traceV2SensitiveKey(key) {
				out[key] = "[REDACTED]"
				continue
			}
			out[key] = redactTraceV2JSONValue(item)
		}
		return out
	default:
		return typed
	}
}

func traceV2SensitiveKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	key = strings.ReplaceAll(key, "_", "")
	key = strings.ReplaceAll(key, "-", "")
	for _, sensitive := range []string{"password", "passwd", "pwd", "token", "secret", "apikey", "authorization"} {
		if key == sensitive || strings.HasSuffix(key, sensitive) {
			return true
		}
	}
	if strings.Contains(key, "secretref") || strings.HasPrefix(key, "authorization") || strings.HasSuffix(key, "authorization") ||
		strings.Contains(key, "accesstoken") || strings.Contains(key, "refreshtoken") ||
		(strings.HasPrefix(key, "secret") && (strings.HasSuffix(key, "key") || strings.HasSuffix(key, "value") || strings.HasSuffix(key, "ref"))) {
		return true
	}
	return false
}

func WriteTraceDocumentV2FromRequest(req Request) (string, error) {
	return WriteTraceDocumentV2FromRequestWithConfig(DefaultConfig(), req)
}

func WriteTraceDocumentV2FromRequestWithConfig(cfg Config, req Request) (string, error) {
	cfg = normalizeConfig(cfg)
	if !cfg.Enabled {
		return "", nil
	}
	root, err := traceDirectory(cfg.RootDir, req)
	if err != nil {
		return "", err
	}
	metadata := redactStringMap(copyStringMap(req.Metadata))
	if metadata == nil {
		metadata = map[string]string{}
	}
	if kind := strings.TrimSpace(req.Kind); kind != "" {
		metadata["kind"] = kind
	}
	if traceID := strings.TrimSpace(req.TraceID); traceID != "" {
		metadata["traceId"] = traceID
	}
	if caseID := firstNonEmptyTraceV2(req.CaseID, req.Metadata["eval.caseId"], req.Metadata["caseId"]); caseID != "" {
		metadata["caseId"] = caseID
	}
	promptTrace := mergeRequestToolTraceFields(req)
	doc := TraceDocumentV2{
		SchemaVersion:     TraceDocumentV2SchemaVersion,
		SessionID:         firstNonEmptyTraceV2(req.SessionID, req.Kind, "session"),
		TurnID:            firstNonEmptyTraceV2(req.TurnID, req.TraceID, req.Kind, "turn"),
		Iteration:         req.Iteration,
		Metadata:          metadata,
		VisibleTools:      append([]string(nil), req.VisibleTools...),
		PromptFingerprint: copyStringMap(req.PromptFingerprint),
		HarnessTurn:       req.HarnessTurn,
		Prompt:            redactPrompt(req.Prompt),
		ModelInput:        req.ModelInput,
		StepContext: map[string]any{
			"modelInput": req.ModelInput,
		},
		SpecialInputWorldState: specialinputmemory.CloneWorldStateSection(promptTrace.SpecialInputWorldState),
		PromptInputTrace:       promptTrace,
		PromptInputDiff:        req.PromptInputDiff,
		FinalEvidenceState:     req.FinalEvidenceState,
	}
	if len(req.ModelInput) > 0 {
		doc.ProviderRequest.ModelInputHash = stableTraceV2Hash(req.ModelInput)
	}
	if !diagnosticTraceEmpty(req.DiagnosticTrace) {
		doc.DiagnosticTrace = req.DiagnosticTrace
	}
	return WriteTraceDocumentV2(root, doc)
}

func traceDocumentV2FileName(doc TraceDocumentV2) string {
	return fmt.Sprintf("iteration-%03d-%s.json", doc.Iteration, traceDocumentV2Stamp(doc.CreatedAt))
}

func traceDocumentV2Stamp(createdAt string) string {
	if ts, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(createdAt)); err == nil {
		return ts.UTC().Format("20060102T150405.000000000Z")
	}
	return time.Now().UTC().Format("20060102T150405.000000000Z")
}

func writeTraceDocumentV2Index(root, path string, doc TraceDocumentV2) error {
	indexData, err := json.MarshalIndent([]map[string]any{{
		"path":          path,
		"sessionId":     doc.SessionID,
		"turnId":        doc.TurnID,
		"iteration":     doc.Iteration,
		"schemaVersion": doc.SchemaVersion,
	}}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, "index.json"), indexData, 0o644)
}

func writeTraceDocumentV2Markdown(path string, doc TraceDocumentV2) error {
	var b strings.Builder
	b.WriteString("# Trace Document V2\n\n")
	b.WriteString("schemaVersion: ")
	b.WriteString(doc.SchemaVersion)
	b.WriteString("\n\nsessionId: ")
	b.WriteString(doc.SessionID)
	b.WriteString("\n\nturnId: ")
	b.WriteString(doc.TurnID)
	b.WriteString("\n\n")
	if doc.ProviderRequest.ModelInputHash != "" || doc.ProviderRequest.ProviderMessagesHash != "" || doc.ProviderRequest.PromptCacheKey != "" {
		b.WriteString("## Provider Request\n\n")
		b.WriteString("modelInputHash: ")
		b.WriteString(doc.ProviderRequest.ModelInputHash)
		b.WriteString("\n\nproviderMessagesHash: ")
		b.WriteString(doc.ProviderRequest.ProviderMessagesHash)
		b.WriteString("\n\npromptCacheKey: ")
		b.WriteString(doc.ProviderRequest.PromptCacheKey)
		b.WriteString("\n\n")
	}
	if doc.DiagnosticTrace != nil {
		b.WriteString("## Diagnostic Trace\n\n")
		writeTraceDocumentV2JSONBlock(&b, doc.DiagnosticTrace)
	}
	if doc.PromptInputTrace != nil {
		b.WriteString("## Prompt Input Trace\n\n")
		writeTraceDocumentV2JSONBlock(&b, doc.PromptInputTrace)
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func writeTraceDocumentV2DiffMarkdown(path string, diff any) error {
	var b strings.Builder
	b.WriteString("# Prompt Input Diff\n\n")
	writeTraceDocumentV2JSONBlock(&b, diff)
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func writeTraceDocumentV2JSONBlock(b *strings.Builder, value any) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		b.WriteString("```text\n")
		b.WriteString(err.Error())
		b.WriteString("\n```\n\n")
		return
	}
	b.WriteString("```json\n")
	b.Write(data)
	b.WriteString("\n```\n\n")
}

func stableTraceV2Hash(value any) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TraceDocumentV2Directory(root, sessionID, turnID string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		root = DefaultRootDir("")
	}
	sessionID = safeTraceDocumentV2Name(firstNonEmptyTraceV2(sessionID, "session"))
	turnID = safeTraceDocumentV2Name(firstNonEmptyTraceV2(turnID, "turn"))
	return filepath.Join(root, sessionID, turnID)
}

func safeTraceDocumentV2Name(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-_")
	if out == "" {
		return "unknown"
	}
	return out
}

func firstNonEmptyTraceV2(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
