package store

import (
	"strings"
	"time"
)

const (
	RuntimeIntentFrameTraceOnly = "trace_only"
	RuntimeIntentFrameShadow    = "shadow"
	RuntimeIntentFrameActive    = "active"

	RuntimeWorkflowReferenceGuardEnforce = "enforce"
	RuntimeWorkflowReferenceGuardWarning = "warning"

	RuntimeWorkflowValidationStatic = "static"
	RuntimeWorkflowValidationDocker = "docker"
)

type RuntimeSettings struct {
	AgentRuntime RuntimeAgentSettings     `json:"agentRuntime"`
	Tooling      RuntimeToolingSettings   `json:"tooling"`
	Workflow     RuntimeWorkflowSettings  `json:"workflow"`
	OpsManual    RuntimeOpsManualSettings `json:"opsManual"`
	Debug        RuntimeDebugSettings     `json:"debug"`
	PublicWeb    RuntimePublicWebSettings `json:"publicWeb"`
	UpdatedAt    time.Time                `json:"updatedAt,omitempty"`
}

type RuntimeAgentSettings struct {
	IntentFrameRouting string `json:"intentFrameRouting"`
	DiagnosticProtocol bool   `json:"diagnosticProtocol"`
}

type RuntimeToolingSettings struct {
	ReadOnlyRetryEnabled       bool `json:"readOnlyRetryEnabled"`
	ReadOnlyRetryMaxPerCall    int  `json:"readOnlyRetryMaxPerCall"`
	ReadOnlyRetryMaxPerTurn    int  `json:"readOnlyRetryMaxPerTurn"`
	ReadOnlyRetryBackoffBaseMs int  `json:"readOnlyRetryBackoffBaseMs"`
	ReadOnlyRetryBackoffMaxMs  int  `json:"readOnlyRetryBackoffMaxMs"`
}

type RuntimeWorkflowSettings struct {
	ReferenceGuardMode string `json:"referenceGuardMode"`
	ValidationProvider string `json:"validationProvider"`
	ValidationImage    string `json:"validationImage"`
}

type RuntimeOpsManualSettings struct {
	AutoRetrieval bool `json:"autoRetrieval"`
}

type RuntimeDebugSettings struct {
	ModelInputTrace      bool `json:"modelInputTrace"`
	FinalState           bool `json:"finalState"`
	TransportProjection  bool `json:"transportProjection"`
	TranscriptProjection bool `json:"transcriptProjection"`
}

type RuntimePublicWebSettings struct {
	Enabled bool `json:"enabled"`
}

func DefaultRuntimeSettings() RuntimeSettings {
	return RuntimeSettings{
		AgentRuntime: RuntimeAgentSettings{
			IntentFrameRouting: RuntimeIntentFrameTraceOnly,
			DiagnosticProtocol: true,
		},
		Tooling: RuntimeToolingSettings{
			ReadOnlyRetryEnabled:       false,
			ReadOnlyRetryMaxPerCall:    1,
			ReadOnlyRetryMaxPerTurn:    3,
			ReadOnlyRetryBackoffBaseMs: 300,
			ReadOnlyRetryBackoffMaxMs:  2000,
		},
		Workflow: RuntimeWorkflowSettings{
			ReferenceGuardMode: RuntimeWorkflowReferenceGuardEnforce,
			ValidationProvider: RuntimeWorkflowValidationStatic,
			ValidationImage:    "python:3.12-slim",
		},
		OpsManual: RuntimeOpsManualSettings{
			AutoRetrieval: false,
		},
		Debug: RuntimeDebugSettings{
			ModelInputTrace:      true,
			FinalState:           false,
			TransportProjection:  false,
			TranscriptProjection: false,
		},
		PublicWeb: RuntimePublicWebSettings{
			Enabled: true,
		},
	}
}

func NormalizeRuntimeSettings(settings RuntimeSettings) RuntimeSettings {
	settings.AgentRuntime.IntentFrameRouting = normalizeRuntimeEnum(settings.AgentRuntime.IntentFrameRouting, RuntimeIntentFrameTraceOnly, RuntimeIntentFrameTraceOnly, RuntimeIntentFrameShadow, RuntimeIntentFrameActive)
	settings.Tooling.ReadOnlyRetryMaxPerCall = clampRuntimeInt(settings.Tooling.ReadOnlyRetryMaxPerCall, 0, 10)
	settings.Tooling.ReadOnlyRetryMaxPerTurn = clampRuntimeInt(settings.Tooling.ReadOnlyRetryMaxPerTurn, 0, 10)
	if settings.Tooling.ReadOnlyRetryBackoffBaseMs <= 0 {
		settings.Tooling.ReadOnlyRetryBackoffBaseMs = 300
	}
	if settings.Tooling.ReadOnlyRetryBackoffMaxMs <= 0 {
		settings.Tooling.ReadOnlyRetryBackoffMaxMs = 2000
	}
	if settings.Tooling.ReadOnlyRetryBackoffMaxMs < settings.Tooling.ReadOnlyRetryBackoffBaseMs {
		settings.Tooling.ReadOnlyRetryBackoffMaxMs = settings.Tooling.ReadOnlyRetryBackoffBaseMs
	}
	settings.Workflow.ReferenceGuardMode = normalizeRuntimeEnum(settings.Workflow.ReferenceGuardMode, RuntimeWorkflowReferenceGuardEnforce, RuntimeWorkflowReferenceGuardEnforce, RuntimeWorkflowReferenceGuardWarning)
	settings.Workflow.ValidationProvider = normalizeRuntimeEnum(settings.Workflow.ValidationProvider, RuntimeWorkflowValidationStatic, RuntimeWorkflowValidationStatic, RuntimeWorkflowValidationDocker)
	if strings.TrimSpace(settings.Workflow.ValidationImage) == "" {
		settings.Workflow.ValidationImage = "python:3.12-slim"
	} else {
		settings.Workflow.ValidationImage = strings.TrimSpace(settings.Workflow.ValidationImage)
	}
	return settings
}

func cloneRuntimeSettings(settings RuntimeSettings) RuntimeSettings {
	return NormalizeRuntimeSettings(settings)
}

func normalizeRuntimeEnum(value string, fallback string, allowed ...string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	for _, candidate := range allowed {
		if normalized == candidate {
			return candidate
		}
	}
	return fallback
}

func clampRuntimeInt(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func (s RuntimeSettings) RuntimeDiagnosticProtocol() bool {
	return s.AgentRuntime.DiagnosticProtocol
}

func (s RuntimeSettings) RuntimeReadOnlyRetryEnabled() bool {
	return s.Tooling.ReadOnlyRetryEnabled
}

func (s RuntimeSettings) RuntimeReadOnlyRetryMaxPerCall() int {
	return s.Tooling.ReadOnlyRetryMaxPerCall
}

func (s RuntimeSettings) RuntimeReadOnlyRetryMaxPerTurn() int {
	return s.Tooling.ReadOnlyRetryMaxPerTurn
}

func (s RuntimeSettings) RuntimeReadOnlyRetryBackoffBaseMs() int {
	return s.Tooling.ReadOnlyRetryBackoffBaseMs
}

func (s RuntimeSettings) RuntimeReadOnlyRetryBackoffMaxMs() int {
	return s.Tooling.ReadOnlyRetryBackoffMaxMs
}
