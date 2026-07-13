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
	TurnContext                 any                                               `json:"turnContext,omitempty"`
	StepContext                 any                                               `json:"stepContext,omitempty"`
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
