package noderesult

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	SchemaVersion = "aiops.node_result/v1"
	BeginMarker   = "AIOPS_NODE_RESULT_BEGIN"
	EndMarker     = "AIOPS_NODE_RESULT_END"

	StatusSuccess = "success"
	StatusFailed  = "failed"
)

type Envelope struct {
	SchemaVersion string         `json:"schema_version"`
	RunID         string         `json:"run_id,omitempty"`
	NodeID        string         `json:"node_id,omitempty"`
	NodeType      string         `json:"node_type,omitempty"`
	Status        string         `json:"status"`
	StartedAt     time.Time      `json:"started_at,omitempty"`
	FinishedAt    time.Time      `json:"finished_at,omitempty"`
	DurationMs    int64          `json:"duration_ms,omitempty"`
	ExitCode      int            `json:"exit_code,omitempty"`
	Outputs       map[string]any `json:"outputs,omitempty"`
	Metrics       map[string]any `json:"metrics,omitempty"`
	Artifacts     []Artifact     `json:"artifacts,omitempty"`
	Logs          Logs           `json:"logs,omitempty"`
	Error         *ErrorInfo     `json:"error,omitempty"`
	Security      map[string]any `json:"security,omitempty"`
}

type Artifact struct {
	Name        string         `json:"name,omitempty"`
	Type        string         `json:"type,omitempty"`
	URI         string         `json:"uri,omitempty"`
	Summary     string         `json:"summary,omitempty"`
	ContentType string         `json:"content_type,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type Logs struct {
	StdoutPreview string `json:"stdout_preview,omitempty"`
	StderrPreview string `json:"stderr_preview,omitempty"`
	Truncated     bool   `json:"truncated,omitempty"`
}

type ErrorInfo struct {
	Code    string         `json:"code,omitempty"`
	Message string         `json:"message,omitempty"`
	Details map[string]any `json:"details,omitempty"`
}

type Options struct {
	RunID      string
	NodeID     string
	NodeType   string
	Outputs    map[string]any
	Metrics    map[string]any
	Artifacts  []Artifact
	StartedAt  time.Time
	FinishedAt time.Time
	ExitCode   int
	Security   map[string]any
}

func Success(opts Options) Envelope {
	env := Envelope{
		SchemaVersion: SchemaVersion,
		RunID:         opts.RunID,
		NodeID:        opts.NodeID,
		NodeType:      opts.NodeType,
		Status:        StatusSuccess,
		StartedAt:     opts.StartedAt,
		FinishedAt:    opts.FinishedAt,
		ExitCode:      opts.ExitCode,
		Outputs:       opts.Outputs,
		Metrics:       opts.Metrics,
		Artifacts:     opts.Artifacts,
		Security:      opts.Security,
	}
	if !opts.StartedAt.IsZero() && !opts.FinishedAt.IsZero() {
		env.DurationMs = opts.FinishedAt.Sub(opts.StartedAt).Milliseconds()
	}
	return env
}

func Failure(opts Options, code, message string) Envelope {
	env := Success(opts)
	env.Status = StatusFailed
	env.Error = &ErrorInfo{Code: code, Message: message}
	return env
}

func ParseStdout(raw string) (Envelope, bool, error) {
	content, marked := markedPayload(raw)
	if marked {
		env, err := parseEnvelope(content)
		if err != nil {
			return Envelope{}, false, err
		}
		env.Logs.StdoutPreview, env.Logs.Truncated = preview(raw, 4096)
		return env, true, nil
	}

	lines := strings.Split(raw, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "{") || !strings.Contains(line, `"schema_version"`) {
			continue
		}
		env, err := parseEnvelope(line)
		if err != nil {
			continue
		}
		env.Logs.StdoutPreview, env.Logs.Truncated = preview(raw, 4096)
		return env, true, nil
	}
	return Envelope{}, false, nil
}

func MarshalMarked(env Envelope) ([]byte, error) {
	if env.SchemaVersion == "" {
		env.SchemaVersion = SchemaVersion
	}
	data, err := json.Marshal(env)
	if err != nil {
		return nil, err
	}
	out := BeginMarker + "\n" + string(data) + "\n" + EndMarker
	return []byte(out), nil
}

func markedPayload(raw string) (string, bool) {
	begin := strings.Index(raw, BeginMarker)
	if begin < 0 {
		return "", false
	}
	afterBegin := begin + len(BeginMarker)
	end := strings.Index(raw[afterBegin:], EndMarker)
	if end < 0 {
		return "", false
	}
	return strings.TrimSpace(raw[afterBegin : afterBegin+end]), true
}

func parseEnvelope(raw string) (Envelope, error) {
	var env Envelope
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &env); err != nil {
		return Envelope{}, err
	}
	if env.SchemaVersion != SchemaVersion {
		return Envelope{}, errors.New("unsupported node result schema version")
	}
	return env, nil
}

func preview(raw string, limit int) (string, bool) {
	if limit <= 0 || len(raw) <= limit {
		return raw, false
	}
	preview := raw[:limit]
	for !utf8.ValidString(preview) && len(preview) > 0 {
		preview = preview[:len(preview)-1]
	}
	return preview, true
}
