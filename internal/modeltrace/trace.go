package modeltrace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"aiops-v2/internal/agentassembly"
	"aiops-v2/internal/diagnostics"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/promptinput"
	"aiops-v2/internal/resourcebinding"
	"aiops-v2/internal/specialinputmemory"
)

type Config struct {
	Enabled bool
	RootDir string
}

func DefaultRootDir(dataDir string) string {
	root := strings.TrimSpace(dataDir)
	if root == "" {
		root = ".data"
	}
	return filepath.Join(root, "model-input-traces")
}

func DefaultConfig() Config {
	return Config{
		Enabled: true,
		RootDir: DefaultRootDir(""),
	}
}

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
	Kind                          string
	TraceID                       string
	SessionID                     string
	TurnID                        string
	Iteration                     int
	CaseID                        string
	Metadata                      map[string]string
	VisibleTools                  []string
	PromptFingerprint             map[string]string
	Prompt                        Prompt
	ModelInput                    []promptinput.ModelInputItem
	PromptInputTrace              promptinput.PromptInputTrace
	PromptInputDiff               *promptinput.TraceDiff
	DiagnosticTrace               diagnostics.DiagnosticTrace
	PlanModeState                 *promptinput.PlanModeTraceState
	PlanArtifactRef               string
	PlanTransitions               []promptinput.PlanTransitionTrace
	PlanRequirementDecision       *promptinput.PlanRequirementDecisionTrace
	PlanCompletionGate            *promptinput.PlanCompletionGateTrace
	TaskClaims                    []promptinput.TaskClaimTrace
	PlanApprovalScope             *promptinput.PlanApprovalScopeTrace
	PlanRejectionEvents           []promptinput.PlanRejectionEventTrace
	TaskTodoState                 *promptinput.TaskTodoTraceState
	ToolSurfaceFingerprint        string
	ToolSurfacePolicySnapshotHash string
	HarnessTurn                   any
	AssemblySource                string
	PromptCompilerSource          string
	ToolSurfaceSource             string
	AdapterName                   string
	LoadedToolsDelta              []string
	LoadedPacksDelta              []string
	SkillIndexHash                string
	LoadedSkillsDelta             []string
	ToolSearchEvents              []promptinput.ToolSearchTraceEvent
	ToolSelectionEvents           []promptinput.ToolSelectionTraceEvent
	RejectedToolCalls             []promptinput.RejectedToolCallTraceEvent
	DispatchDecisions             []promptinput.DispatchDecisionTrace
	SkillSearchEvents             []promptinput.SkillSearchTraceEvent
	SkillReadEvents               []promptinput.SkillReadTraceEvent
	RejectedSkillActivations      []promptinput.RejectedSkillActivationTraceEvent
	MCPInstructionDeltas          []promptinput.MCPInstructionDeltaTrace
	ParallelDispatchGroups        []promptinput.ParallelDispatchTraceGroup
	FailedToolSummaries           []promptinput.FailedToolSummary
	AgentIndexHash                string
	AgentIndexEntries             []promptinput.AgentIndexEntryTrace
	AgentIndexDropped             []promptinput.DroppedAgentIndexEntryTrace
	AgentIndexDelta               []string
	AgentDelegationDecision       *promptinput.AgentDelegationDecisionTrace
	AgentAssignmentLint           []promptinput.AgentAssignmentLintTrace
	AgentParallelTraceGroups      []promptinput.AgentParallelTraceGroup
	ResourceBindings              []resourcebinding.ResourceBindingSnapshot
	ResourceRoleBindings          []resourcebinding.ResourceRoleBinding
	ResourceCapabilities          []resourcebinding.ResourceCapability
	ResourceEvidenceRefs          []resourcebinding.EvidenceRef
	SessionTargetSnapshot         *resourcebinding.SessionTargetSnapshot
	RoleBindingConflicts          []resourcebinding.RoleBindingConflict
	AgentAssemblySnapshot         *agentassembly.AgentAssemblySnapshot
	SpecialInputWorldState        *specialinputmemory.SpecialInputWorldStateSection
	ResourceLocks                 []promptinput.ResourceLockTrace
	OwnerWriteTraces              []promptinput.OwnerWriteTrace
	AgentFinalGate                *promptinput.AgentFinalGateDecisionTrace
	AgentNotifications            []promptinput.AgentNotificationTrace
	VerificationAgentReport       *promptinput.VerificationAgentReportTrace
	VerificationReportRef         string
	VerificationStatus            string
	TaskDepth                     *promptinput.TaskDepthTrace
	EvidenceCoverage              *promptinput.EvidenceCoverageTrace
	GenericityTrace               *promptinput.GenericityTrace
	CompletionGate                *promptinput.CompletionGateTrace
	SafetySignals                 []promptinput.SafetySignalTrace
	UnexpectedStateGate           *promptinput.UnexpectedStateGateTrace
	ApprovalScope                 *promptinput.ApprovalScopeTrace
	FinalEvidenceState            any
}

type payload struct {
	SchemaVersion                 int                                               `json:"schemaVersion"`
	Kind                          string                                            `json:"kind,omitempty"`
	CreatedAt                     string                                            `json:"createdAt"`
	TraceID                       string                                            `json:"traceId,omitempty"`
	SessionID                     string                                            `json:"sessionId,omitempty"`
	TurnID                        string                                            `json:"turnId,omitempty"`
	Iteration                     int                                               `json:"iteration,omitempty"`
	CaseID                        string                                            `json:"caseId,omitempty"`
	Metadata                      map[string]string                                 `json:"metadata,omitempty"`
	VisibleTools                  []string                                          `json:"visibleTools,omitempty"`
	VisibleToolCount              int                                               `json:"visibleToolCount,omitempty"`
	PromptCharCount               int                                               `json:"promptCharCount,omitempty"`
	ModelInputStats               modelInputStats                                   `json:"modelInputStats,omitempty"`
	ToolRegistryCharCount         int                                               `json:"toolRegistryCharCount,omitempty"`
	PromptFingerprint             map[string]string                                 `json:"promptFingerprint,omitempty"`
	PlanModeState                 *promptinput.PlanModeTraceState                   `json:"planModeState,omitempty"`
	PlanArtifactRef               string                                            `json:"planArtifactRef,omitempty"`
	PlanTransitions               []promptinput.PlanTransitionTrace                 `json:"planTransitions,omitempty"`
	PlanRequirementDecision       *promptinput.PlanRequirementDecisionTrace         `json:"planRequirementDecision,omitempty"`
	PlanCompletionGate            *promptinput.PlanCompletionGateTrace              `json:"planCompletionGate,omitempty"`
	TaskClaims                    []promptinput.TaskClaimTrace                      `json:"taskClaims,omitempty"`
	PlanApprovalScope             *promptinput.PlanApprovalScopeTrace               `json:"planApprovalScope,omitempty"`
	PlanRejectionEvents           []promptinput.PlanRejectionEventTrace             `json:"planRejectionEvents,omitempty"`
	TaskTodoState                 *promptinput.TaskTodoTraceState                   `json:"taskTodoState,omitempty"`
	ToolSurfaceFingerprint        string                                            `json:"toolSurfaceFingerprint,omitempty"`
	ToolSurfacePolicySnapshotHash string                                            `json:"toolSurfacePolicySnapshotHash,omitempty"`
	AssemblySource                string                                            `json:"assembly_source,omitempty"`
	PromptCompilerSource          string                                            `json:"prompt_compiler_source,omitempty"`
	ToolSurfaceSource             string                                            `json:"tool_surface_source,omitempty"`
	AdapterName                   string                                            `json:"adapter_name,omitempty"`
	ToolSurfaceSnapshot           *promptinput.ToolSurfaceSnapshot                  `json:"toolSurfaceSnapshot,omitempty"`
	ToolSurfaceTrace              *ToolSurfaceTrace                                 `json:"toolSurfaceTrace,omitempty"`
	LoadedToolsDelta              []string                                          `json:"loadedToolsDelta,omitempty"`
	LoadedPacksDelta              []string                                          `json:"loadedPacksDelta,omitempty"`
	SkillIndexHash                string                                            `json:"skillIndexHash,omitempty"`
	LoadedSkillsDelta             []string                                          `json:"loadedSkillsDelta,omitempty"`
	ToolSearchEvents              []promptinput.ToolSearchTraceEvent                `json:"toolSearchEvents,omitempty"`
	ToolSelectionEvents           []promptinput.ToolSelectionTraceEvent             `json:"toolSelectionEvents,omitempty"`
	RejectedToolCalls             []promptinput.RejectedToolCallTraceEvent          `json:"rejectedToolCalls,omitempty"`
	DispatchDecisions             []promptinput.DispatchDecisionTrace               `json:"dispatchDecisions,omitempty"`
	SkillSearchEvents             []promptinput.SkillSearchTraceEvent               `json:"skillSearchEvents,omitempty"`
	SkillReadEvents               []promptinput.SkillReadTraceEvent                 `json:"skillReadEvents,omitempty"`
	RejectedSkillActivations      []promptinput.RejectedSkillActivationTraceEvent   `json:"rejectedSkillActivations,omitempty"`
	MCPInstructionDeltas          []promptinput.MCPInstructionDeltaTrace            `json:"mcpInstructionDeltas,omitempty"`
	ParallelDispatchGroups        []promptinput.ParallelDispatchTraceGroup          `json:"parallelDispatchGroups,omitempty"`
	FailedToolSummaries           []promptinput.FailedToolSummary                   `json:"failedToolSummaries,omitempty"`
	AgentIndexHash                string                                            `json:"agentIndexHash,omitempty"`
	AgentIndexEntries             []promptinput.AgentIndexEntryTrace                `json:"agentIndexEntries,omitempty"`
	AgentIndexDropped             []promptinput.DroppedAgentIndexEntryTrace         `json:"agentIndexDropped,omitempty"`
	AgentIndexDelta               []string                                          `json:"agentIndexDelta,omitempty"`
	AgentDelegationDecision       *promptinput.AgentDelegationDecisionTrace         `json:"agentDelegationDecision,omitempty"`
	AgentAssignmentLint           []promptinput.AgentAssignmentLintTrace            `json:"agentAssignmentLint,omitempty"`
	AgentParallelTraceGroups      []promptinput.AgentParallelTraceGroup             `json:"agentParallelTraceGroups,omitempty"`
	ResourceBindings              []resourcebinding.ResourceBindingSnapshot         `json:"resourceBindings,omitempty"`
	ResourceRoleBindings          []resourcebinding.ResourceRoleBinding             `json:"resourceRoleBindings,omitempty"`
	ResourceCapabilities          []resourcebinding.ResourceCapability              `json:"resourceCapabilities,omitempty"`
	ResourceEvidenceRefs          []resourcebinding.EvidenceRef                     `json:"resourceEvidenceRefs,omitempty"`
	SessionTargetSnapshot         *resourcebinding.SessionTargetSnapshot            `json:"sessionTargetSnapshot,omitempty"`
	RoleBindingConflicts          []resourcebinding.RoleBindingConflict             `json:"roleBindingConflicts,omitempty"`
	AgentAssemblySnapshot         *agentassembly.AgentAssemblySnapshot              `json:"agentAssemblySnapshot,omitempty"`
	SpecialInputWorldState        *specialinputmemory.SpecialInputWorldStateSection `json:"specialInputWorldState,omitempty"`
	ResourceLocks                 []promptinput.ResourceLockTrace                   `json:"resourceLocks,omitempty"`
	OwnerWriteTraces              []promptinput.OwnerWriteTrace                     `json:"ownerWriteTraces,omitempty"`
	AgentFinalGate                *promptinput.AgentFinalGateDecisionTrace          `json:"agentFinalGate,omitempty"`
	AgentNotifications            []promptinput.AgentNotificationTrace              `json:"agentNotifications,omitempty"`
	VerificationAgentReport       *promptinput.VerificationAgentReportTrace         `json:"verificationAgentReport,omitempty"`
	VerificationReportRef         string                                            `json:"verificationReportRef,omitempty"`
	VerificationStatus            string                                            `json:"verificationStatus,omitempty"`
	TaskDepth                     *promptinput.TaskDepthTrace                       `json:"taskDepth,omitempty"`
	EvidenceCoverage              *promptinput.EvidenceCoverageTrace                `json:"evidenceCoverage,omitempty"`
	GenericityTrace               *promptinput.GenericityTrace                      `json:"genericityTrace,omitempty"`
	CompletionGate                *promptinput.CompletionGateTrace                  `json:"completionGate,omitempty"`
	SafetySignals                 []promptinput.SafetySignalTrace                   `json:"safetySignals,omitempty"`
	UnexpectedStateGate           *promptinput.UnexpectedStateGateTrace             `json:"unexpectedStateGate,omitempty"`
	ApprovalScope                 *promptinput.ApprovalScopeTrace                   `json:"approvalScope,omitempty"`
	FinalEvidenceState            any                                               `json:"finalEvidenceState,omitempty"`
	Prompt                        Prompt                                            `json:"prompt"`
	ModelInput                    []traceMessage                                    `json:"modelInput"`
	ContextDedupe                 *promptinput.ContextDedupeTrace                   `json:"contextDedupe,omitempty"`
	ContextGovernance             []promptinput.ContextGovernanceTraceItem          `json:"contextGovernance,omitempty"`
	PromptInputTrace              promptinput.PromptInputTrace                      `json:"promptInputTrace,omitempty"`
	DiagnosticTrace               *diagnostics.DiagnosticTrace                      `json:"diagnosticTrace,omitempty"`
}

type ToolSurfaceTrace struct {
	InitialTools        []string                           `json:"initialTools,omitempty"`
	BaseRegistryCount   int                                `json:"baseRegistryCount,omitempty"`
	DeferredFamilies    []DeferredToolFamilyTrace          `json:"deferredFamilies,omitempty"`
	LoadedTools         []string                           `json:"loadedTools,omitempty"`
	LoadedPacks         []string                           `json:"loadedPacks,omitempty"`
	FilteredTools       []FilteredToolTrace                `json:"filteredTools,omitempty"`
	MCPHealth           map[string]string                  `json:"mcpHealth,omitempty"`
	ToolSearchEvents    []promptinput.ToolSearchTraceEvent `json:"toolSearchEvents,omitempty"`
	SelectedTools       []string                           `json:"selectedTools,omitempty"`
	RejectedToolReasons []RejectedToolReasonTrace          `json:"rejectedToolReasons,omitempty"`
}

type DeferredToolFamilyTrace struct {
	Pack              string   `json:"pack,omitempty"`
	Capability        string   `json:"capability,omitempty"`
	Source            string   `json:"source,omitempty"`
	MCPServerID       string   `json:"mcpServerId,omitempty"`
	HealthStatus      string   `json:"healthStatus,omitempty"`
	RequiresHealth    bool     `json:"requiresHealth,omitempty"`
	RequiresApproval  bool     `json:"requiresApproval,omitempty"`
	RequiresSelect    bool     `json:"requiresSelect,omitempty"`
	UnavailableReason string   `json:"unavailableReason,omitempty"`
	ToolCount         int      `json:"toolCount,omitempty"`
	ResourceTypes     []string `json:"resourceTypes,omitempty"`
	OperationKinds    []string `json:"operationKinds,omitempty"`
}

type FilteredToolTrace struct {
	ToolName string `json:"toolName,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

type RejectedToolReasonTrace struct {
	ToolName       string `json:"toolName,omitempty"`
	ErrorType      string `json:"errorType,omitempty"`
	Reason         string `json:"reason,omitempty"`
	RequiredAction string `json:"requiredAction,omitempty"`
}

type traceMessage struct {
	Index        int             `json:"index"`
	ProviderRole string          `json:"providerRole"`
	SemanticRole string          `json:"semanticRole,omitempty"`
	PromptLayer  string          `json:"promptLayer,omitempty"`
	Name         string          `json:"name,omitempty"`
	Content      string          `json:"content,omitempty"`
	ToolCallID   string          `json:"toolCallId,omitempty"`
	ToolName     string          `json:"toolName,omitempty"`
	ToolCalls    []traceToolCall `json:"toolCalls,omitempty"`
}

type traceToolCall struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type modelInputStats struct {
	PromptBytes  int `json:"promptBytes,omitempty"`
	MessageCount int `json:"messageCount,omitempty"`
}

func Write(req Request) (string, error) {
	return WriteWithConfig(DefaultConfig(), req)
}

func WriteWithConfig(cfg Config, req Request) (string, error) {
	cfg = normalizeConfig(cfg)
	if !cfg.Enabled {
		return "", nil
	}
	traceDir, err := traceDirectory(cfg.RootDir, req)
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

func normalizeConfig(cfg Config) Config {
	cfg.RootDir = strings.TrimSpace(cfg.RootDir)
	if cfg.RootDir == "" {
		cfg.RootDir = DefaultRootDir("")
	}
	return cfg
}

func buildPayload(req Request) payload {
	visibleTools := append([]string(nil), req.VisibleTools...)
	modelInput := traceMessages(req.ModelInput)
	prompt := redactPrompt(req.Prompt)
	promptTrace := mergeRequestToolTraceFields(req)
	redactedPromptTrace := redactPromptInputTrace(promptTrace)
	if req.Iteration > 0 {
		prompt = deltaPromptTrace(prompt)
		modelInput = deltaModelInputTrace(modelInput)
	}
	inputCharCount := modelInputCharCount(modelInput)
	return payload{
		SchemaVersion:                 1,
		Kind:                          strings.TrimSpace(req.Kind),
		CreatedAt:                     time.Now().UTC().Format(time.RFC3339Nano),
		TraceID:                       strings.TrimSpace(req.TraceID),
		SessionID:                     strings.TrimSpace(req.SessionID),
		TurnID:                        strings.TrimSpace(req.TurnID),
		Iteration:                     req.Iteration,
		CaseID:                        firstNonEmpty(req.CaseID, req.Metadata["eval.caseId"], req.Metadata["caseId"]),
		Metadata:                      redactStringMap(copyStringMap(req.Metadata)),
		VisibleTools:                  visibleTools,
		VisibleToolCount:              len(visibleTools),
		PromptCharCount:               inputCharCount,
		ModelInputStats:               buildModelInputStats(modelInput, inputCharCount),
		ToolRegistryCharCount:         len(req.Prompt.Tools),
		PromptFingerprint:             copyStringMap(req.PromptFingerprint),
		PlanModeState:                 redactedPromptTrace.PlanModeState,
		PlanArtifactRef:               redactedPromptTrace.PlanArtifactRef,
		PlanTransitions:               append([]promptinput.PlanTransitionTrace(nil), redactedPromptTrace.PlanTransitions...),
		PlanRequirementDecision:       redactedPromptTrace.PlanRequirementDecision,
		PlanCompletionGate:            redactedPromptTrace.PlanCompletionGate,
		TaskClaims:                    append([]promptinput.TaskClaimTrace(nil), redactedPromptTrace.TaskClaims...),
		PlanApprovalScope:             redactedPromptTrace.PlanApprovalScope,
		PlanRejectionEvents:           append([]promptinput.PlanRejectionEventTrace(nil), redactedPromptTrace.PlanRejectionEvents...),
		TaskTodoState:                 redactedPromptTrace.TaskTodoState,
		ToolSurfaceFingerprint:        redactedPromptTrace.ToolSurfaceFingerprint,
		ToolSurfacePolicySnapshotHash: redactedPromptTrace.ToolSurfacePolicySnapshotHash,
		AssemblySource:                firstNonEmpty(req.AssemblySource, redactedPromptTrace.AssemblySource),
		PromptCompilerSource:          firstNonEmpty(req.PromptCompilerSource, redactedPromptTrace.PromptCompilerSource),
		ToolSurfaceSource:             firstNonEmpty(req.ToolSurfaceSource, redactedPromptTrace.ToolSurfaceSource),
		AdapterName:                   firstNonEmpty(req.AdapterName, redactedPromptTrace.AdapterName),
		ToolSurfaceSnapshot:           redactedPromptTrace.ToolSurfaceSnapshot,
		ToolSurfaceTrace:              buildToolSurfaceTrace(visibleTools, redactedPromptTrace),
		LoadedToolsDelta:              append([]string(nil), redactedPromptTrace.LoadedToolsDelta...),
		LoadedPacksDelta:              append([]string(nil), redactedPromptTrace.LoadedPacksDelta...),
		SkillIndexHash:                redactedPromptTrace.SkillIndexHash,
		LoadedSkillsDelta:             append([]string(nil), redactedPromptTrace.LoadedSkillsDelta...),
		ToolSearchEvents:              append([]promptinput.ToolSearchTraceEvent(nil), redactedPromptTrace.ToolSearchEvents...),
		ToolSelectionEvents:           append([]promptinput.ToolSelectionTraceEvent(nil), redactedPromptTrace.ToolSelectionEvents...),
		RejectedToolCalls:             append([]promptinput.RejectedToolCallTraceEvent(nil), redactedPromptTrace.RejectedToolCalls...),
		DispatchDecisions:             append([]promptinput.DispatchDecisionTrace(nil), redactedPromptTrace.DispatchDecisions...),
		SkillSearchEvents:             append([]promptinput.SkillSearchTraceEvent(nil), redactedPromptTrace.SkillSearchEvents...),
		SkillReadEvents:               append([]promptinput.SkillReadTraceEvent(nil), redactedPromptTrace.SkillReadEvents...),
		RejectedSkillActivations:      append([]promptinput.RejectedSkillActivationTraceEvent(nil), redactedPromptTrace.RejectedSkillActivations...),
		MCPInstructionDeltas:          append([]promptinput.MCPInstructionDeltaTrace(nil), redactedPromptTrace.MCPInstructionDeltas...),
		ParallelDispatchGroups:        append([]promptinput.ParallelDispatchTraceGroup(nil), redactedPromptTrace.ParallelDispatchGroups...),
		FailedToolSummaries:           append([]promptinput.FailedToolSummary(nil), redactedPromptTrace.FailedToolSummaries...),
		AgentIndexHash:                redactedPromptTrace.AgentIndexHash,
		AgentIndexEntries:             append([]promptinput.AgentIndexEntryTrace(nil), redactedPromptTrace.AgentIndexEntries...),
		AgentIndexDropped:             append([]promptinput.DroppedAgentIndexEntryTrace(nil), redactedPromptTrace.AgentIndexDropped...),
		AgentIndexDelta:               append([]string(nil), redactedPromptTrace.AgentIndexDelta...),
		AgentDelegationDecision:       redactedPromptTrace.AgentDelegationDecision,
		AgentAssignmentLint:           append([]promptinput.AgentAssignmentLintTrace(nil), redactedPromptTrace.AgentAssignmentLint...),
		AgentParallelTraceGroups:      append([]promptinput.AgentParallelTraceGroup(nil), redactedPromptTrace.AgentParallelTraceGroups...),
		ResourceBindings:              append([]resourcebinding.ResourceBindingSnapshot(nil), redactedPromptTrace.ResourceBindings...),
		ResourceRoleBindings:          append([]resourcebinding.ResourceRoleBinding(nil), redactedPromptTrace.ResourceRoleBindings...),
		ResourceCapabilities:          append([]resourcebinding.ResourceCapability(nil), redactedPromptTrace.ResourceCapabilities...),
		ResourceEvidenceRefs:          append([]resourcebinding.EvidenceRef(nil), redactedPromptTrace.ResourceEvidenceRefs...),
		SessionTargetSnapshot:         redactedPromptTrace.SessionTargetSnapshot,
		RoleBindingConflicts:          append([]resourcebinding.RoleBindingConflict(nil), redactedPromptTrace.RoleBindingConflicts...),
		AgentAssemblySnapshot:         redactedPromptTrace.AgentAssemblySnapshot,
		SpecialInputWorldState:        specialinputmemory.CloneWorldStateSection(redactedPromptTrace.SpecialInputWorldState),
		ResourceLocks:                 append([]promptinput.ResourceLockTrace(nil), redactedPromptTrace.ResourceLocks...),
		OwnerWriteTraces:              append([]promptinput.OwnerWriteTrace(nil), redactedPromptTrace.OwnerWriteTraces...),
		AgentFinalGate:                redactedPromptTrace.AgentFinalGate,
		AgentNotifications:            append([]promptinput.AgentNotificationTrace(nil), redactedPromptTrace.AgentNotifications...),
		VerificationAgentReport:       redactedPromptTrace.VerificationAgentReport,
		VerificationReportRef:         redactedPromptTrace.VerificationReportRef,
		VerificationStatus:            redactedPromptTrace.VerificationStatus,
		TaskDepth:                     redactedPromptTrace.TaskDepth,
		EvidenceCoverage:              redactedPromptTrace.EvidenceCoverage,
		GenericityTrace:               redactedPromptTrace.GenericityTrace,
		CompletionGate:                redactedPromptTrace.CompletionGate,
		SafetySignals:                 append([]promptinput.SafetySignalTrace(nil), redactedPromptTrace.SafetySignals...),
		UnexpectedStateGate:           redactedPromptTrace.UnexpectedStateGate,
		ApprovalScope:                 redactedPromptTrace.ApprovalScope,
		FinalEvidenceState:            req.FinalEvidenceState,
		Prompt:                        prompt,
		ModelInput:                    modelInput,
		ContextDedupe:                 redactedPromptTrace.ContextDedupe,
		ContextGovernance:             append([]promptinput.ContextGovernanceTraceItem(nil), redactedPromptTrace.ContextGovernance...),
		PromptInputTrace:              redactedPromptTrace,
		DiagnosticTrace:               diagnosticTracePayload(req.DiagnosticTrace),
	}
}

func buildToolSurfaceTrace(visibleTools []string, trace promptinput.PromptInputTrace) *ToolSurfaceTrace {
	selectedTools := selectedToolsFromTrace(trace)
	initialTools := initialToolsFromVisible(visibleTools, selectedTools)
	surface := &ToolSurfaceTrace{
		InitialTools:        initialTools,
		BaseRegistryCount:   len(initialTools),
		DeferredFamilies:    deferredToolFamiliesFromTrace(trace.DeferredToolDirectory),
		LoadedTools:         redactStringSlice(uniqueSortedStrings(trace.LoadedToolsDelta)),
		LoadedPacks:         redactStringSlice(uniqueSortedStrings(trace.LoadedPacksDelta)),
		FilteredTools:       filteredToolsFromSelections(trace.ToolSelectionEvents),
		MCPHealth:           mcpHealthFromDeferredFamilies(trace.DeferredToolDirectory),
		ToolSearchEvents:    redactToolSearchTraceEvents(trace.ToolSearchEvents),
		SelectedTools:       redactStringSlice(selectedTools),
		RejectedToolReasons: rejectedToolReasonsFromTrace(trace.RejectedToolCalls),
	}
	if toolSurfaceTraceEmpty(surface) {
		return nil
	}
	return surface
}

func selectedToolsFromTrace(trace promptinput.PromptInputTrace) []string {
	seen := map[string]bool{}
	var out []string
	for _, name := range trace.LoadedToolsDelta {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	for _, event := range trace.ToolSelectionEvents {
		for _, name := range event.LoadedTools {
			name = strings.TrimSpace(name)
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func initialToolsFromVisible(visibleTools, selectedTools []string) []string {
	dynamic := map[string]bool{}
	for _, name := range selectedTools {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			dynamic[trimmed] = true
		}
	}
	var out []string
	seen := map[string]bool{}
	for _, name := range visibleTools {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] || dynamic[name] {
			continue
		}
		seen[name] = true
		out = append(out, diagnostics.RedactSensitiveText(name))
	}
	return out
}

func deferredToolFamiliesFromTrace(entries []promptcompiler.DeferredToolDirectoryEntry) []DeferredToolFamilyTrace {
	if len(entries) == 0 {
		return nil
	}
	out := make([]DeferredToolFamilyTrace, 0, len(entries))
	for _, entry := range entries {
		out = append(out, DeferredToolFamilyTrace{
			Pack:              diagnostics.RedactSensitiveText(strings.TrimSpace(entry.Pack)),
			Capability:        diagnostics.RedactSensitiveText(strings.TrimSpace(entry.Capability)),
			Source:            diagnostics.RedactSensitiveText(strings.TrimSpace(entry.Source)),
			MCPServerID:       diagnostics.RedactSensitiveText(strings.TrimSpace(entry.MCPServerID)),
			HealthStatus:      diagnostics.RedactSensitiveText(strings.TrimSpace(entry.HealthStatus)),
			RequiresHealth:    entry.RequiresHealth,
			RequiresApproval:  entry.RequiresApproval,
			RequiresSelect:    entry.RequiresSelect,
			UnavailableReason: diagnostics.RedactSensitiveText(strings.TrimSpace(entry.UnavailableReason)),
			ToolCount:         entry.ToolCount,
			ResourceTypes:     redactStringSlice(entry.ResourceTypes),
			OperationKinds:    redactStringSlice(entry.OperationKinds),
		})
	}
	return out
}

func filteredToolsFromSelections(events []promptinput.ToolSelectionTraceEvent) []FilteredToolTrace {
	var out []FilteredToolTrace
	seen := map[string]bool{}
	for _, event := range events {
		for _, name := range event.NotLoaded {
			name = strings.TrimSpace(name)
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, FilteredToolTrace{
				ToolName: diagnostics.RedactSensitiveText(name),
				Reason:   diagnostics.RedactSensitiveText(strings.TrimSpace(event.NotLoadedReasons[name])),
			})
		}
	}
	return out
}

func mcpHealthFromDeferredFamilies(entries []promptcompiler.DeferredToolDirectoryEntry) map[string]string {
	out := map[string]string{}
	for _, entry := range entries {
		serverID := strings.TrimSpace(entry.MCPServerID)
		if serverID == "" {
			continue
		}
		status := strings.TrimSpace(entry.HealthStatus)
		if status == "" && entry.RequiresHealth {
			status = "unknown"
		}
		if status == "" {
			continue
		}
		out[diagnostics.RedactSensitiveText(serverID)] = diagnostics.RedactSensitiveText(status)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func rejectedToolReasonsFromTrace(calls []promptinput.RejectedToolCallTraceEvent) []RejectedToolReasonTrace {
	if len(calls) == 0 {
		return nil
	}
	out := make([]RejectedToolReasonTrace, 0, len(calls))
	for _, call := range calls {
		out = append(out, RejectedToolReasonTrace{
			ToolName:       diagnostics.RedactSensitiveText(strings.TrimSpace(call.ToolName)),
			ErrorType:      diagnostics.RedactSensitiveText(strings.TrimSpace(call.ErrorType)),
			Reason:         diagnostics.RedactSensitiveText(strings.TrimSpace(call.Reason)),
			RequiredAction: diagnostics.RedactSensitiveText(strings.TrimSpace(call.RequiredAction)),
		})
	}
	return out
}

func uniqueSortedStrings(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func toolSurfaceTraceEmpty(trace *ToolSurfaceTrace) bool {
	return trace == nil ||
		len(trace.InitialTools) == 0 &&
			trace.BaseRegistryCount == 0 &&
			len(trace.DeferredFamilies) == 0 &&
			len(trace.LoadedTools) == 0 &&
			len(trace.LoadedPacks) == 0 &&
			len(trace.FilteredTools) == 0 &&
			len(trace.MCPHealth) == 0 &&
			len(trace.ToolSearchEvents) == 0 &&
			len(trace.SelectedTools) == 0 &&
			len(trace.RejectedToolReasons) == 0
}

func mergeRequestToolTraceFields(req Request) promptinput.PromptInputTrace {
	trace := req.PromptInputTrace
	if strings.TrimSpace(trace.ToolSurfaceFingerprint) == "" {
		trace.ToolSurfaceFingerprint = strings.TrimSpace(req.ToolSurfaceFingerprint)
	}
	if strings.TrimSpace(trace.ToolSurfacePolicySnapshotHash) == "" {
		trace.ToolSurfacePolicySnapshotHash = strings.TrimSpace(req.ToolSurfacePolicySnapshotHash)
	}
	if strings.TrimSpace(trace.AssemblySource) == "" {
		trace.AssemblySource = strings.TrimSpace(req.AssemblySource)
	}
	if strings.TrimSpace(trace.PromptCompilerSource) == "" {
		trace.PromptCompilerSource = strings.TrimSpace(req.PromptCompilerSource)
	}
	if strings.TrimSpace(trace.ToolSurfaceSource) == "" {
		trace.ToolSurfaceSource = strings.TrimSpace(req.ToolSurfaceSource)
	}
	if strings.TrimSpace(trace.AdapterName) == "" {
		trace.AdapterName = strings.TrimSpace(req.AdapterName)
	}
	if len(trace.LoadedToolsDelta) == 0 {
		trace.LoadedToolsDelta = append([]string(nil), req.LoadedToolsDelta...)
	}
	if len(trace.LoadedPacksDelta) == 0 {
		trace.LoadedPacksDelta = append([]string(nil), req.LoadedPacksDelta...)
	}
	if strings.TrimSpace(trace.SkillIndexHash) == "" {
		trace.SkillIndexHash = strings.TrimSpace(req.SkillIndexHash)
	}
	if len(trace.LoadedSkillsDelta) == 0 {
		trace.LoadedSkillsDelta = append([]string(nil), req.LoadedSkillsDelta...)
	}
	if len(trace.ToolSearchEvents) == 0 {
		trace.ToolSearchEvents = append([]promptinput.ToolSearchTraceEvent(nil), req.ToolSearchEvents...)
	}
	if len(trace.ToolSelectionEvents) == 0 {
		trace.ToolSelectionEvents = append([]promptinput.ToolSelectionTraceEvent(nil), req.ToolSelectionEvents...)
	}
	if len(trace.RejectedToolCalls) == 0 {
		trace.RejectedToolCalls = append([]promptinput.RejectedToolCallTraceEvent(nil), req.RejectedToolCalls...)
	}
	if len(trace.DispatchDecisions) == 0 {
		trace.DispatchDecisions = append([]promptinput.DispatchDecisionTrace(nil), req.DispatchDecisions...)
	}
	if len(trace.SkillSearchEvents) == 0 {
		trace.SkillSearchEvents = append([]promptinput.SkillSearchTraceEvent(nil), req.SkillSearchEvents...)
	}
	if len(trace.SkillReadEvents) == 0 {
		trace.SkillReadEvents = append([]promptinput.SkillReadTraceEvent(nil), req.SkillReadEvents...)
	}
	if len(trace.RejectedSkillActivations) == 0 {
		trace.RejectedSkillActivations = append([]promptinput.RejectedSkillActivationTraceEvent(nil), req.RejectedSkillActivations...)
	}
	if trace.PlanModeState == nil && req.PlanModeState != nil {
		state := *req.PlanModeState
		trace.PlanModeState = &state
	}
	if strings.TrimSpace(trace.PlanArtifactRef) == "" {
		trace.PlanArtifactRef = strings.TrimSpace(req.PlanArtifactRef)
	}
	if len(trace.PlanTransitions) == 0 {
		trace.PlanTransitions = append([]promptinput.PlanTransitionTrace(nil), req.PlanTransitions...)
	}
	if trace.PlanRequirementDecision == nil && req.PlanRequirementDecision != nil {
		decision := *req.PlanRequirementDecision
		decision.Signals = append([]string(nil), req.PlanRequirementDecision.Signals...)
		trace.PlanRequirementDecision = &decision
	}
	if trace.PlanCompletionGate == nil && req.PlanCompletionGate != nil {
		gate := *req.PlanCompletionGate
		gate.Reasons = append([]string(nil), req.PlanCompletionGate.Reasons...)
		trace.PlanCompletionGate = &gate
	}
	if len(trace.TaskClaims) == 0 {
		trace.TaskClaims = append([]promptinput.TaskClaimTrace(nil), req.TaskClaims...)
	}
	if trace.PlanApprovalScope == nil && req.PlanApprovalScope != nil {
		scope := *req.PlanApprovalScope
		scope.ApprovedScopes = append([]string(nil), req.PlanApprovalScope.ApprovedScopes...)
		scope.DeniedScopes = append([]string(nil), req.PlanApprovalScope.DeniedScopes...)
		trace.PlanApprovalScope = &scope
	}
	if len(trace.PlanRejectionEvents) == 0 {
		trace.PlanRejectionEvents = append([]promptinput.PlanRejectionEventTrace(nil), req.PlanRejectionEvents...)
	}
	if trace.TaskTodoState == nil && req.TaskTodoState != nil {
		state := *req.TaskTodoState
		state.Items = append([]promptinput.TaskTodoTraceItem(nil), req.TaskTodoState.Items...)
		trace.TaskTodoState = &state
	}
	if len(trace.MCPInstructionDeltas) == 0 {
		trace.MCPInstructionDeltas = append([]promptinput.MCPInstructionDeltaTrace(nil), req.MCPInstructionDeltas...)
	}
	if len(trace.ParallelDispatchGroups) == 0 {
		trace.ParallelDispatchGroups = append([]promptinput.ParallelDispatchTraceGroup(nil), req.ParallelDispatchGroups...)
	}
	if len(trace.FailedToolSummaries) == 0 {
		trace.FailedToolSummaries = append([]promptinput.FailedToolSummary(nil), req.FailedToolSummaries...)
	}
	if strings.TrimSpace(trace.AgentIndexHash) == "" {
		trace.AgentIndexHash = strings.TrimSpace(req.AgentIndexHash)
	}
	if len(trace.AgentIndexEntries) == 0 {
		trace.AgentIndexEntries = append([]promptinput.AgentIndexEntryTrace(nil), req.AgentIndexEntries...)
	}
	if len(trace.AgentIndexDropped) == 0 {
		trace.AgentIndexDropped = append([]promptinput.DroppedAgentIndexEntryTrace(nil), req.AgentIndexDropped...)
	}
	if len(trace.AgentIndexDelta) == 0 {
		trace.AgentIndexDelta = append([]string(nil), req.AgentIndexDelta...)
	}
	if trace.AgentDelegationDecision == nil && req.AgentDelegationDecision != nil {
		decision := *req.AgentDelegationDecision
		trace.AgentDelegationDecision = &decision
	}
	if len(trace.AgentAssignmentLint) == 0 {
		trace.AgentAssignmentLint = append([]promptinput.AgentAssignmentLintTrace(nil), req.AgentAssignmentLint...)
	}
	if len(trace.AgentParallelTraceGroups) == 0 {
		trace.AgentParallelTraceGroups = append([]promptinput.AgentParallelTraceGroup(nil), req.AgentParallelTraceGroups...)
	}
	if len(trace.ResourceBindings) == 0 {
		trace.ResourceBindings = append([]resourcebinding.ResourceBindingSnapshot(nil), req.ResourceBindings...)
	}
	if len(trace.ResourceRoleBindings) == 0 {
		trace.ResourceRoleBindings = append([]resourcebinding.ResourceRoleBinding(nil), req.ResourceRoleBindings...)
	}
	if len(trace.ResourceCapabilities) == 0 {
		trace.ResourceCapabilities = append([]resourcebinding.ResourceCapability(nil), req.ResourceCapabilities...)
	}
	if len(trace.ResourceEvidenceRefs) == 0 {
		trace.ResourceEvidenceRefs = append([]resourcebinding.EvidenceRef(nil), req.ResourceEvidenceRefs...)
	}
	if trace.SessionTargetSnapshot == nil && req.SessionTargetSnapshot != nil {
		trace.SessionTargetSnapshot = req.SessionTargetSnapshot
	}
	if len(trace.RoleBindingConflicts) == 0 {
		trace.RoleBindingConflicts = append([]resourcebinding.RoleBindingConflict(nil), req.RoleBindingConflicts...)
	}
	if trace.AgentAssemblySnapshot == nil && req.AgentAssemblySnapshot != nil {
		trace.AgentAssemblySnapshot = req.AgentAssemblySnapshot
	}
	if trace.SpecialInputWorldState == nil && req.SpecialInputWorldState != nil {
		trace.SpecialInputWorldState = specialinputmemory.CloneWorldStateSection(req.SpecialInputWorldState)
	}
	if len(trace.ResourceLocks) == 0 {
		trace.ResourceLocks = append([]promptinput.ResourceLockTrace(nil), req.ResourceLocks...)
	}
	if len(trace.OwnerWriteTraces) == 0 {
		trace.OwnerWriteTraces = append([]promptinput.OwnerWriteTrace(nil), req.OwnerWriteTraces...)
	}
	if trace.AgentFinalGate == nil && req.AgentFinalGate != nil {
		gate := *req.AgentFinalGate
		trace.AgentFinalGate = &gate
	}
	if len(trace.AgentNotifications) == 0 {
		trace.AgentNotifications = append([]promptinput.AgentNotificationTrace(nil), req.AgentNotifications...)
	}
	if trace.VerificationAgentReport == nil && req.VerificationAgentReport != nil {
		report := *req.VerificationAgentReport
		trace.VerificationAgentReport = &report
	}
	if strings.TrimSpace(trace.VerificationReportRef) == "" {
		trace.VerificationReportRef = strings.TrimSpace(req.VerificationReportRef)
	}
	if strings.TrimSpace(trace.VerificationStatus) == "" {
		trace.VerificationStatus = strings.TrimSpace(req.VerificationStatus)
	}
	if trace.TaskDepth == nil && req.TaskDepth != nil {
		depth := *req.TaskDepth
		depth.Reasons = append([]string(nil), req.TaskDepth.Reasons...)
		trace.TaskDepth = &depth
	}
	if trace.EvidenceCoverage == nil && req.EvidenceCoverage != nil {
		coverage := *req.EvidenceCoverage
		coverage.RequiredDimensions = append([]string(nil), req.EvidenceCoverage.RequiredDimensions...)
		coverage.CoveredDimensions = append([]string(nil), req.EvidenceCoverage.CoveredDimensions...)
		coverage.MissingDimensions = append([]string(nil), req.EvidenceCoverage.MissingDimensions...)
		coverage.OpenQuestions = append([]string(nil), req.EvidenceCoverage.OpenQuestions...)
		coverage.Reasons = append([]string(nil), req.EvidenceCoverage.Reasons...)
		trace.EvidenceCoverage = &coverage
	}
	if trace.GenericityTrace == nil && req.GenericityTrace != nil {
		genericity := *req.GenericityTrace
		genericity.CoreRuleDomainTerms = append([]string(nil), req.GenericityTrace.CoreRuleDomainTerms...)
		genericity.AllowedFixtureTerms = append([]string(nil), req.GenericityTrace.AllowedFixtureTerms...)
		genericity.AllowedPluginTerms = append([]string(nil), req.GenericityTrace.AllowedPluginTerms...)
		genericity.Violations = append([]string(nil), req.GenericityTrace.Violations...)
		trace.GenericityTrace = &genericity
	}
	if trace.CompletionGate == nil && req.CompletionGate != nil {
		gate := *req.CompletionGate
		gate.Reasons = append([]string(nil), req.CompletionGate.Reasons...)
		trace.CompletionGate = &gate
	}
	if len(trace.SafetySignals) == 0 {
		trace.SafetySignals = cloneSafetySignalTraces(req.SafetySignals)
	}
	if trace.UnexpectedStateGate == nil && req.UnexpectedStateGate != nil {
		gate := *req.UnexpectedStateGate
		gate.Sources = append([]string(nil), req.UnexpectedStateGate.Sources...)
		gate.AffectedScopes = append([]string(nil), req.UnexpectedStateGate.AffectedScopes...)
		gate.Reasons = append([]string(nil), req.UnexpectedStateGate.Reasons...)
		trace.UnexpectedStateGate = &gate
	}
	if trace.ApprovalScope == nil && req.ApprovalScope != nil {
		scope := *req.ApprovalScope
		scope.AllowedActions = append([]string(nil), req.ApprovalScope.AllowedActions...)
		scope.ResourceScopes = append([]string(nil), req.ApprovalScope.ResourceScopes...)
		scope.Reasons = append([]string(nil), req.ApprovalScope.Reasons...)
		trace.ApprovalScope = &scope
	}
	return trace
}

func deltaPromptTrace(prompt Prompt) Prompt {
	return Prompt{
		StableHash: prompt.StableHash,
		Dynamic:    prompt.Dynamic,
	}
}

func deltaModelInputTrace(messages []traceMessage) []traceMessage {
	if len(messages) == 0 {
		return nil
	}
	out := append([]traceMessage(nil), messages...)
	for i := range out {
		if !isPromptLayerTraceMessage(out[i]) {
			continue
		}
		out[i].Content = fmt.Sprintf("[prompt layer %s omitted after initial trace; use promptSections/changedSections and promptFingerprint for hashes]", out[i].PromptLayer)
		out[i].ToolCalls = nil
	}
	return out
}

func isPromptLayerTraceMessage(msg traceMessage) bool {
	layer := strings.TrimSpace(msg.PromptLayer)
	if layer == "" {
		return false
	}
	switch layer {
	case "memory", "ops_context_capsule":
		return false
	case "system", "developer", "tool_index", "runtime_policy":
		return true
	default:
		return strings.Contains(layer, ".") || strings.HasPrefix(layer, "dynamic")
	}
}

func modelInputCharCount(messages []traceMessage) int {
	total := 0
	for _, msg := range messages {
		total += len(msg.Content)
	}
	return total
}

func buildModelInputStats(messages []traceMessage, promptBytes int) modelInputStats {
	if len(messages) == 0 && promptBytes == 0 {
		return modelInputStats{}
	}
	return modelInputStats{
		PromptBytes:  promptBytes,
		MessageCount: len(messages),
	}
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

func traceMessages(items []promptinput.ModelInputItem) []traceMessage {
	out := make([]traceMessage, 0, len(items))
	for i, item := range items {
		content := item.Content
		if item.ProviderRole == promptinput.ProviderRoleTool && item.ToolResult != nil && strings.TrimSpace(item.ToolResult.Content) != "" {
			content = item.ToolResult.Content
		}
		toolCallID := strings.TrimSpace(firstNonEmpty(item.ToolCallID, item.ToolResultToolCallID()))
		traceMsg := traceMessage{
			Index:        i,
			ProviderRole: string(item.ProviderRole),
			SemanticRole: strings.TrimSpace(item.SemanticRole),
			PromptLayer:  modelInputItemPromptLayer(item),
			Name:         item.Name,
			Content:      diagnostics.RedactSensitiveText(content),
			ToolCallID:   toolCallID,
			ToolName:     item.Name,
			ToolCalls:    redactToolCalls(item.ToolCalls),
		}
		out = append(out, traceMsg)
	}
	return out
}

func modelInputItemPromptLayer(item promptinput.ModelInputItem) string {
	if layer := strings.TrimSpace(item.Source.Layer); layer != "" {
		return layer
	}
	if item.Metadata != nil {
		if layer := strings.TrimSpace(item.Metadata["prompt_layer"]); layer != "" {
			return layer
		}
		if layer := strings.TrimSpace(item.Metadata["source_layer"]); layer != "" {
			return layer
		}
	}
	return ""
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

func redactToolCalls(calls []promptinput.ModelInputToolCall) []traceToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]traceToolCall, 0, len(calls))
	for _, call := range calls {
		out = append(out, traceToolCall{
			ID:        diagnostics.RedactSensitiveText(call.ID),
			Name:      diagnostics.RedactSensitiveText(call.Name),
			Arguments: diagnostics.RedactSensitiveText(string(call.Arguments)),
		})
	}
	return out
}

func redactPromptInputTrace(trace promptinput.PromptInputTrace) promptinput.PromptInputTrace {
	if promptInputTraceEmpty(trace) {
		return promptinput.PromptInputTrace{}
	}
	out := promptinput.PromptInputTrace{
		Items:                         make([]promptinput.TraceItem, 0, len(trace.Items)),
		PromptSections:                redactPromptSections(trace.PromptSections),
		ChangedSections:               redactChangedPromptSections(trace.ChangedSections),
		OpsContextCapsuleChars:        trace.OpsContextCapsuleChars,
		SessionFactCount:              trace.SessionFactCount,
		LettaHintCount:                trace.LettaHintCount,
		MemoryItemCount:               trace.MemoryItemCount,
		VisibleOpsManualTools:         append([]string(nil), trace.VisibleOpsManualTools...),
		DroppedContextReasons:         append([]string(nil), trace.DroppedContextReasons...),
		ContextDedupe:                 cloneContextDedupeTrace(trace.ContextDedupe),
		ContextGovernance:             redactContextGovernanceTraceItems(trace.ContextGovernance),
		ContextUsage:                  redactContextUsage(trace.ContextUsage),
		AssemblySource:                diagnostics.RedactSensitiveText(strings.TrimSpace(trace.AssemblySource)),
		PromptCompilerSource:          diagnostics.RedactSensitiveText(strings.TrimSpace(trace.PromptCompilerSource)),
		ToolSurfaceSource:             diagnostics.RedactSensitiveText(strings.TrimSpace(trace.ToolSurfaceSource)),
		AdapterName:                   diagnostics.RedactSensitiveText(strings.TrimSpace(trace.AdapterName)),
		ToolSurfaceFingerprint:        diagnostics.RedactSensitiveText(trace.ToolSurfaceFingerprint),
		ToolSurfacePolicySnapshotHash: diagnostics.RedactSensitiveText(trace.ToolSurfacePolicySnapshotHash),
		ToolSurfaceSnapshot:           redactToolSurfaceSnapshot(trace.ToolSurfaceSnapshot),
		PublicWebBudget:               clonePublicWebBudgetTrace(trace.PublicWebBudget),
		WebSearchPolicy:               redactWebSearchPolicyTrace(trace.WebSearchPolicy),
		WebSearch:                     redactWebSearchTrace(trace.WebSearch),
		Final:                         redactFinalTrace(trace.Final),
		DeferredToolDirectory:         redactDeferredToolDirectory(trace.DeferredToolDirectory),
		LoadedToolsDelta:              redactStringSlice(trace.LoadedToolsDelta),
		LoadedPacksDelta:              redactStringSlice(trace.LoadedPacksDelta),
		SkillIndexHash:                diagnostics.RedactSensitiveText(trace.SkillIndexHash),
		LoadedSkillsDelta:             redactStringSlice(trace.LoadedSkillsDelta),
		ToolSearchEvents:              redactToolSearchTraceEvents(trace.ToolSearchEvents),
		ToolSelectionEvents:           redactToolSelectionTraceEvents(trace.ToolSelectionEvents),
		RejectedToolCalls:             redactRejectedToolCallTraceEvents(trace.RejectedToolCalls),
		DispatchDecisions:             redactDispatchDecisionTraces(trace.DispatchDecisions),
		SkillSearchEvents:             redactSkillSearchTraceEvents(trace.SkillSearchEvents),
		SkillReadEvents:               redactSkillReadTraceEvents(trace.SkillReadEvents),
		RejectedSkillActivations:      redactRejectedSkillActivationTraceEvents(trace.RejectedSkillActivations),
		PlanModeState:                 redactPlanModeTraceState(trace.PlanModeState),
		PlanArtifactRef:               diagnostics.RedactSensitiveText(trace.PlanArtifactRef),
		PlanTransitions:               redactPlanTransitionTraces(trace.PlanTransitions),
		PlanRequirementDecision:       redactPlanRequirementDecisionTrace(trace.PlanRequirementDecision),
		PlanCompletionGate:            redactPlanCompletionGateTrace(trace.PlanCompletionGate),
		TaskClaims:                    redactTaskClaimTraces(trace.TaskClaims),
		PlanApprovalScope:             redactPlanApprovalScopeTrace(trace.PlanApprovalScope),
		PlanRejectionEvents:           redactPlanRejectionEventTraces(trace.PlanRejectionEvents),
		TaskTodoState:                 redactTaskTodoTraceState(trace.TaskTodoState),
		MCPInstructionDeltas:          redactMCPInstructionDeltaTraceEvents(trace.MCPInstructionDeltas),
		ParallelDispatchGroups:        redactParallelDispatchTraceGroups(trace.ParallelDispatchGroups),
		FailedToolSummaries:           redactFailedToolSummaries(trace.FailedToolSummaries),
		AgentIndexHash:                diagnostics.RedactSensitiveText(trace.AgentIndexHash),
		AgentIndexEntries:             redactAgentIndexEntryTraces(trace.AgentIndexEntries),
		AgentIndexDropped:             redactDroppedAgentIndexEntryTraces(trace.AgentIndexDropped),
		AgentIndexDelta:               redactStringSlice(trace.AgentIndexDelta),
		AgentDelegationDecision:       redactAgentDelegationDecisionTrace(trace.AgentDelegationDecision),
		AgentAssignmentLint:           redactAgentAssignmentLintTraces(trace.AgentAssignmentLint),
		AgentParallelTraceGroups:      redactAgentParallelTraceGroups(trace.AgentParallelTraceGroups),
		ResourceBindings:              redactResourceBindingSnapshots(trace.ResourceBindings),
		ResourceRoleBindings:          redactResourceRoleBindings(trace.ResourceRoleBindings),
		ResourceCapabilities:          redactResourceCapabilities(trace.ResourceCapabilities),
		ResourceEvidenceRefs:          redactResourceEvidenceRefs(trace.ResourceEvidenceRefs),
		SessionTargetSnapshot:         redactSessionTargetSnapshot(trace.SessionTargetSnapshot),
		RoleBindingConflicts:          redactRoleBindingConflicts(trace.RoleBindingConflicts),
		AgentAssemblySnapshot:         redactAgentAssemblySnapshot(trace.AgentAssemblySnapshot),
		SpecialInputWorldState:        specialinputmemory.CloneWorldStateSection(trace.SpecialInputWorldState),
		ResourceLocks:                 redactResourceLockTraces(trace.ResourceLocks),
		OwnerWriteTraces:              redactOwnerWriteTraces(trace.OwnerWriteTraces),
		AgentFinalGate:                redactAgentFinalGateDecisionTrace(trace.AgentFinalGate),
		AgentNotifications:            redactAgentNotificationTraces(trace.AgentNotifications),
		VerificationAgentReport:       redactVerificationAgentReportTrace(trace.VerificationAgentReport),
		VerificationReportRef:         diagnostics.RedactSensitiveText(trace.VerificationReportRef),
		VerificationStatus:            diagnostics.RedactSensitiveText(trace.VerificationStatus),
		TaskDepth:                     redactTaskDepthTrace(trace.TaskDepth),
		EvidenceCoverage:              redactEvidenceCoverageTrace(trace.EvidenceCoverage),
		GenericityTrace:               redactGenericityTrace(trace.GenericityTrace),
		CompletionGate:                redactCompletionGateTrace(trace.CompletionGate),
		SafetySignals:                 redactSafetySignalTraces(trace.SafetySignals),
		UnexpectedStateGate:           redactUnexpectedStateGateTrace(trace.UnexpectedStateGate),
		ApprovalScope:                 redactApprovalScopeTrace(trace.ApprovalScope),
	}
	for _, item := range trace.Items {
		item.ID = diagnostics.RedactSensitiveText(item.ID)
		item.Content = diagnostics.RedactSensitiveText(item.Content)
		out.Items = append(out.Items, item)
	}
	return out
}

func redactWebSearchPolicyTrace(trace *promptinput.WebSearchPolicyTrace) *promptinput.WebSearchPolicyTrace {
	if trace == nil {
		return nil
	}
	out := &promptinput.WebSearchPolicyTrace{
		Level:            diagnostics.RedactSensitiveText(strings.TrimSpace(trace.Level)),
		Reason:           diagnostics.RedactSensitiveText(strings.TrimSpace(trace.Reason)),
		ReasonCodes:      redactStringSlice(trace.ReasonCodes),
		QuerySeeds:       redactStringSlice(trace.QuerySeeds),
		DisabledBy:       diagnostics.RedactSensitiveText(strings.TrimSpace(trace.DisabledBy)),
		RequireCitations: trace.RequireCitations,
	}
	if out.Level == "" && out.Reason == "" && len(out.ReasonCodes) == 0 && len(out.QuerySeeds) == 0 && out.DisabledBy == "" && !out.RequireCitations {
		return nil
	}
	return out
}

func redactWebSearchTrace(trace *promptinput.WebSearchTrace) *promptinput.WebSearchTrace {
	if trace == nil {
		return nil
	}
	out := &promptinput.WebSearchTrace{
		Attempted:     trace.Attempted,
		RetryCount:    trace.RetryCount,
		Adapter:       diagnostics.RedactSensitiveText(strings.TrimSpace(trace.Adapter)),
		SourceCount:   trace.SourceCount,
		FailureReason: diagnostics.RedactSensitiveText(strings.TrimSpace(trace.FailureReason)),
	}
	if !out.Attempted && out.RetryCount == 0 && out.Adapter == "" && out.SourceCount == 0 && out.FailureReason == "" {
		return nil
	}
	return out
}

func redactFinalTrace(trace *promptinput.FinalTrace) *promptinput.FinalTrace {
	if trace == nil || !trace.PublicWebLimitation {
		return nil
	}
	return &promptinput.FinalTrace{PublicWebLimitation: true}
}

func redactToolSurfaceSnapshot(snapshot *promptinput.ToolSurfaceSnapshot) *promptinput.ToolSurfaceSnapshot {
	if snapshot == nil {
		return nil
	}
	out := &promptinput.ToolSurfaceSnapshot{
		Fingerprint:      diagnostics.RedactSensitiveText(strings.TrimSpace(snapshot.Fingerprint)),
		VisibleTools:     redactStringSlice(snapshot.VisibleTools),
		DeferredTools:    redactStringSlice(snapshot.DeferredTools),
		HiddenTools:      redactStringSlice(snapshot.HiddenTools),
		LoadedPacksDelta: redactStringSlice(snapshot.LoadedPacksDelta),
		PolicyHash:       diagnostics.RedactSensitiveText(strings.TrimSpace(snapshot.PolicyHash)),
	}
	if len(snapshot.HiddenReasons) > 0 {
		out.HiddenReasons = make(map[string][]string, len(snapshot.HiddenReasons))
		for name, reasons := range snapshot.HiddenReasons {
			name = diagnostics.RedactSensitiveText(strings.TrimSpace(name))
			if name == "" {
				continue
			}
			out.HiddenReasons[name] = redactStringSlice(reasons)
		}
		if len(out.HiddenReasons) == 0 {
			out.HiddenReasons = nil
		}
	}
	if toolSurfaceSnapshotTraceEmpty(out) {
		return nil
	}
	return out
}

func cloneContextDedupeTrace(trace *promptinput.ContextDedupeTrace) *promptinput.ContextDedupeTrace {
	if trace == nil {
		return nil
	}
	out := *trace
	return &out
}

func clonePublicWebBudgetTrace(trace *promptinput.PublicWebBudgetTrace) *promptinput.PublicWebBudgetTrace {
	if trace == nil {
		return nil
	}
	out := *trace
	return &out
}

func toolSurfaceSnapshotTraceEmpty(snapshot *promptinput.ToolSurfaceSnapshot) bool {
	return snapshot == nil ||
		strings.TrimSpace(snapshot.Fingerprint) == "" &&
			len(snapshot.VisibleTools) == 0 &&
			len(snapshot.DeferredTools) == 0 &&
			len(snapshot.HiddenTools) == 0 &&
			len(snapshot.HiddenReasons) == 0 &&
			len(snapshot.LoadedPacksDelta) == 0 &&
			strings.TrimSpace(snapshot.PolicyHash) == ""
}

func cloneSafetySignalTraces(signals []promptinput.SafetySignalTrace) []promptinput.SafetySignalTrace {
	if len(signals) == 0 {
		return nil
	}
	out := make([]promptinput.SafetySignalTrace, 0, len(signals))
	for _, signal := range signals {
		signal.Reasons = append([]string(nil), signal.Reasons...)
		out = append(out, signal)
	}
	return out
}

func redactToolSearchTraceEvents(events []promptinput.ToolSearchTraceEvent) []promptinput.ToolSearchTraceEvent {
	if len(events) == 0 {
		return nil
	}
	out := make([]promptinput.ToolSearchTraceEvent, 0, len(events))
	for _, event := range events {
		event.Mode = diagnostics.RedactSensitiveText(event.Mode)
		event.Query = diagnostics.RedactSensitiveText(event.Query)
		event.Ranker = diagnostics.RedactSensitiveText(event.Ranker)
		event.Matches = redactStringSlice(event.Matches)
		event.RejectedReasons = redactToolSearchRejectedReasons(event.RejectedReasons)
		event.Reason = diagnostics.RedactSensitiveText(event.Reason)
		out = append(out, event)
	}
	return out
}

func redactToolSearchRejectedReasons(reasons []promptinput.ToolSearchRejectedReason) []promptinput.ToolSearchRejectedReason {
	if len(reasons) == 0 {
		return nil
	}
	out := make([]promptinput.ToolSearchRejectedReason, 0, len(reasons))
	for _, reason := range reasons {
		reason.ToolName = diagnostics.RedactSensitiveText(reason.ToolName)
		reason.Reason = diagnostics.RedactSensitiveText(reason.Reason)
		reason.Status = diagnostics.RedactSensitiveText(reason.Status)
		reason.Source = diagnostics.RedactSensitiveText(reason.Source)
		reason.MCPServerID = diagnostics.RedactSensitiveText(reason.MCPServerID)
		reason.HealthStatus = diagnostics.RedactSensitiveText(reason.HealthStatus)
		reason.FilteredReason = diagnostics.RedactSensitiveText(reason.FilteredReason)
		out = append(out, reason)
	}
	return out
}

func redactDispatchDecisionTraces(traces []promptinput.DispatchDecisionTrace) []promptinput.DispatchDecisionTrace {
	if len(traces) == 0 {
		return nil
	}
	out := make([]promptinput.DispatchDecisionTrace, 0, len(traces))
	for _, trace := range traces {
		trace.ToolName = diagnostics.RedactSensitiveText(trace.ToolName)
		trace.ToolCallID = diagnostics.RedactSensitiveText(trace.ToolCallID)
		trace.ToolSurfaceFingerprint = diagnostics.RedactSensitiveText(trace.ToolSurfaceFingerprint)
		trace.PermissionSnapshotHash = diagnostics.RedactSensitiveText(trace.PermissionSnapshotHash)
		trace.ArgumentsHash = diagnostics.RedactSensitiveText(trace.ArgumentsHash)
		out = append(out, trace)
	}
	return out
}

func redactToolSelectionTraceEvents(events []promptinput.ToolSelectionTraceEvent) []promptinput.ToolSelectionTraceEvent {
	if len(events) == 0 {
		return nil
	}
	out := make([]promptinput.ToolSelectionTraceEvent, 0, len(events))
	for _, event := range events {
		event.Source = diagnostics.RedactSensitiveText(event.Source)
		event.Reason = diagnostics.RedactSensitiveText(event.Reason)
		event.LoadedTools = redactStringSlice(event.LoadedTools)
		event.LoadedPacks = redactStringSlice(event.LoadedPacks)
		event.NotLoaded = redactStringSlice(event.NotLoaded)
		event.NotLoadedReasons = redactStringMap(event.NotLoadedReasons)
		out = append(out, event)
	}
	return out
}

func redactDeferredToolDirectory(entries []promptcompiler.DeferredToolDirectoryEntry) []promptcompiler.DeferredToolDirectoryEntry {
	if len(entries) == 0 {
		return nil
	}
	out := make([]promptcompiler.DeferredToolDirectoryEntry, 0, len(entries))
	for _, entry := range entries {
		entry.Pack = diagnostics.RedactSensitiveText(entry.Pack)
		entry.Capability = diagnostics.RedactSensitiveText(entry.Capability)
		entry.Source = diagnostics.RedactSensitiveText(entry.Source)
		entry.MCPServerID = diagnostics.RedactSensitiveText(entry.MCPServerID)
		entry.HealthStatus = diagnostics.RedactSensitiveText(entry.HealthStatus)
		entry.UnavailableReason = diagnostics.RedactSensitiveText(entry.UnavailableReason)
		entry.ResourceTypes = redactStringSlice(entry.ResourceTypes)
		entry.OperationKinds = redactStringSlice(entry.OperationKinds)
		out = append(out, entry)
	}
	return out
}

func redactSkillSearchTraceEvents(events []promptinput.SkillSearchTraceEvent) []promptinput.SkillSearchTraceEvent {
	if len(events) == 0 {
		return nil
	}
	out := make([]promptinput.SkillSearchTraceEvent, 0, len(events))
	for _, event := range events {
		event.Mode = diagnostics.RedactSensitiveText(event.Mode)
		event.Query = diagnostics.RedactSensitiveText(event.Query)
		event.Matches = redactStringSlice(event.Matches)
		event.Reason = diagnostics.RedactSensitiveText(event.Reason)
		out = append(out, event)
	}
	return out
}

func redactSkillReadTraceEvents(events []promptinput.SkillReadTraceEvent) []promptinput.SkillReadTraceEvent {
	if len(events) == 0 {
		return nil
	}
	out := make([]promptinput.SkillReadTraceEvent, 0, len(events))
	for _, event := range events {
		event.Skill = diagnostics.RedactSensitiveText(event.Skill)
		event.Source = diagnostics.RedactSensitiveText(event.Source)
		event.Reason = diagnostics.RedactSensitiveText(event.Reason)
		event.Range = diagnostics.RedactSensitiveText(event.Range)
		event.Hash = diagnostics.RedactSensitiveText(event.Hash)
		out = append(out, event)
	}
	return out
}

func redactRejectedSkillActivationTraceEvents(events []promptinput.RejectedSkillActivationTraceEvent) []promptinput.RejectedSkillActivationTraceEvent {
	if len(events) == 0 {
		return nil
	}
	out := make([]promptinput.RejectedSkillActivationTraceEvent, 0, len(events))
	for _, event := range events {
		event.SkillName = diagnostics.RedactSensitiveText(event.SkillName)
		event.Reason = diagnostics.RedactSensitiveText(event.Reason)
		event.RequiredAction = diagnostics.RedactSensitiveText(event.RequiredAction)
		event.TurnID = diagnostics.RedactSensitiveText(event.TurnID)
		out = append(out, event)
	}
	return out
}

func redactPlanModeTraceState(state *promptinput.PlanModeTraceState) *promptinput.PlanModeTraceState {
	if state == nil {
		return nil
	}
	out := *state
	out.State = diagnostics.RedactSensitiveText(out.State)
	out.PlanID = diagnostics.RedactSensitiveText(out.PlanID)
	out.ArtifactStatus = diagnostics.RedactSensitiveText(out.ArtifactStatus)
	out.ApprovalStatus = diagnostics.RedactSensitiveText(out.ApprovalStatus)
	out.ReminderLevel = diagnostics.RedactSensitiveText(out.ReminderLevel)
	out.RejectionReason = diagnostics.RedactSensitiveText(out.RejectionReason)
	return &out
}

func redactPlanTransitionTraces(events []promptinput.PlanTransitionTrace) []promptinput.PlanTransitionTrace {
	if len(events) == 0 {
		return nil
	}
	out := make([]promptinput.PlanTransitionTrace, 0, len(events))
	for _, event := range events {
		event.PlanID = diagnostics.RedactSensitiveText(event.PlanID)
		event.From = diagnostics.RedactSensitiveText(event.From)
		event.To = diagnostics.RedactSensitiveText(event.To)
		event.Reason = diagnostics.RedactSensitiveText(event.Reason)
		out = append(out, event)
	}
	return out
}

func redactPlanRequirementDecisionTrace(decision *promptinput.PlanRequirementDecisionTrace) *promptinput.PlanRequirementDecisionTrace {
	if decision == nil {
		return nil
	}
	out := *decision
	out.Decision = diagnostics.RedactSensitiveText(out.Decision)
	out.Reason = diagnostics.RedactSensitiveText(out.Reason)
	out.Signals = redactStringSlice(out.Signals)
	return &out
}

func redactPlanCompletionGateTrace(gate *promptinput.PlanCompletionGateTrace) *promptinput.PlanCompletionGateTrace {
	if gate == nil {
		return nil
	}
	out := *gate
	out.Decision = diagnostics.RedactSensitiveText(out.Decision)
	out.Reasons = redactStringSlice(out.Reasons)
	return &out
}

func redactTaskClaimTraces(claims []promptinput.TaskClaimTrace) []promptinput.TaskClaimTrace {
	if len(claims) == 0 {
		return nil
	}
	out := make([]promptinput.TaskClaimTrace, 0, len(claims))
	for _, claim := range claims {
		claim.TaskID = diagnostics.RedactSensitiveText(claim.TaskID)
		claim.Owner = diagnostics.RedactSensitiveText(claim.Owner)
		claim.Status = diagnostics.RedactSensitiveText(claim.Status)
		claim.Reason = diagnostics.RedactSensitiveText(claim.Reason)
		out = append(out, claim)
	}
	return out
}

func redactPlanApprovalScopeTrace(scope *promptinput.PlanApprovalScopeTrace) *promptinput.PlanApprovalScopeTrace {
	if scope == nil {
		return nil
	}
	out := *scope
	out.PlanID = diagnostics.RedactSensitiveText(out.PlanID)
	out.ApprovedScopes = redactStringSlice(out.ApprovedScopes)
	out.DeniedScopes = redactStringSlice(out.DeniedScopes)
	return &out
}

func redactPlanRejectionEventTraces(events []promptinput.PlanRejectionEventTrace) []promptinput.PlanRejectionEventTrace {
	if len(events) == 0 {
		return nil
	}
	out := make([]promptinput.PlanRejectionEventTrace, 0, len(events))
	for _, event := range events {
		event.PlanID = diagnostics.RedactSensitiveText(event.PlanID)
		event.Reason = diagnostics.RedactSensitiveText(event.Reason)
		event.By = diagnostics.RedactSensitiveText(event.By)
		out = append(out, event)
	}
	return out
}

func redactTaskTodoTraceState(state *promptinput.TaskTodoTraceState) *promptinput.TaskTodoTraceState {
	if state == nil {
		return nil
	}
	out := &promptinput.TaskTodoTraceState{Items: make([]promptinput.TaskTodoTraceItem, 0, len(state.Items))}
	for _, item := range state.Items {
		item.ID = diagnostics.RedactSensitiveText(item.ID)
		item.Status = diagnostics.RedactSensitiveText(item.Status)
		item.Owner = diagnostics.RedactSensitiveText(item.Owner)
		item.BlockedBy = diagnostics.RedactSensitiveText(item.BlockedBy)
		item.PendingEvidence = diagnostics.RedactSensitiveText(item.PendingEvidence)
		out.Items = append(out.Items, item)
	}
	return out
}

func redactMCPInstructionDeltaTraceEvents(events []promptinput.MCPInstructionDeltaTrace) []promptinput.MCPInstructionDeltaTrace {
	if len(events) == 0 {
		return nil
	}
	out := make([]promptinput.MCPInstructionDeltaTrace, 0, len(events))
	for _, event := range events {
		event.ServerID = diagnostics.RedactSensitiveText(event.ServerID)
		event.Action = diagnostics.RedactSensitiveText(event.Action)
		event.Hash = diagnostics.RedactSensitiveText(event.Hash)
		event.Summary = diagnostics.RedactSensitiveText(event.Summary)
		out = append(out, event)
	}
	return out
}

func redactRejectedToolCallTraceEvents(calls []promptinput.RejectedToolCallTraceEvent) []promptinput.RejectedToolCallTraceEvent {
	if len(calls) == 0 {
		return nil
	}
	out := make([]promptinput.RejectedToolCallTraceEvent, 0, len(calls))
	for _, call := range calls {
		call.ToolName = diagnostics.RedactSensitiveText(call.ToolName)
		call.ErrorType = diagnostics.RedactSensitiveText(call.ErrorType)
		call.Reason = diagnostics.RedactSensitiveText(call.Reason)
		call.RequiredAction = diagnostics.RedactSensitiveText(call.RequiredAction)
		call.SuggestedSearchQuery = diagnostics.RedactSensitiveText(call.SuggestedSearchQuery)
		call.TurnID = diagnostics.RedactSensitiveText(call.TurnID)
		call.ToolCallID = diagnostics.RedactSensitiveText(call.ToolCallID)
		out = append(out, call)
	}
	return out
}

func redactParallelDispatchTraceGroups(groups []promptinput.ParallelDispatchTraceGroup) []promptinput.ParallelDispatchTraceGroup {
	if len(groups) == 0 {
		return nil
	}
	out := make([]promptinput.ParallelDispatchTraceGroup, 0, len(groups))
	for _, group := range groups {
		group.GroupID = diagnostics.RedactSensitiveText(group.GroupID)
		group.Decision = diagnostics.RedactSensitiveText(group.Decision)
		group.Reasons = redactStringSlice(group.Reasons)
		group.SharedResourceKeys = redactStringSlice(group.SharedResourceKeys)
		for i := range group.ToolCalls {
			group.ToolCalls[i].ToolCallID = diagnostics.RedactSensitiveText(group.ToolCalls[i].ToolCallID)
			group.ToolCalls[i].ToolName = diagnostics.RedactSensitiveText(group.ToolCalls[i].ToolName)
			group.ToolCalls[i].SharedResourceKey = diagnostics.RedactSensitiveText(group.ToolCalls[i].SharedResourceKey)
		}
		for i := range group.Excluded {
			group.Excluded[i].ToolCallID = diagnostics.RedactSensitiveText(group.Excluded[i].ToolCallID)
			group.Excluded[i].ToolName = diagnostics.RedactSensitiveText(group.Excluded[i].ToolName)
			group.Excluded[i].Reasons = redactStringSlice(group.Excluded[i].Reasons)
			group.Excluded[i].SharedResourceKey = diagnostics.RedactSensitiveText(group.Excluded[i].SharedResourceKey)
		}
		out = append(out, group)
	}
	return out
}

func redactFailedToolSummaries(summaries []promptinput.FailedToolSummary) []promptinput.FailedToolSummary {
	if len(summaries) == 0 {
		return nil
	}
	out := make([]promptinput.FailedToolSummary, 0, len(summaries))
	for _, summary := range summaries {
		summary.Tool = diagnostics.RedactSensitiveText(summary.Tool)
		summary.FailureClass = diagnostics.RedactSensitiveText(summary.FailureClass)
		summary.FinalStatus = diagnostics.RedactSensitiveText(summary.FinalStatus)
		summary.ModelGuidance = diagnostics.RedactSensitiveText(summary.ModelGuidance)
		out = append(out, summary)
	}
	return out
}

func redactAgentIndexEntryTraces(entries []promptinput.AgentIndexEntryTrace) []promptinput.AgentIndexEntryTrace {
	if len(entries) == 0 {
		return nil
	}
	out := make([]promptinput.AgentIndexEntryTrace, 0, len(entries))
	for _, entry := range entries {
		entry.Kind = diagnostics.RedactSensitiveText(entry.Kind)
		entry.Name = diagnostics.RedactSensitiveText(entry.Name)
		entry.Description = diagnostics.RedactSensitiveText(entry.Description)
		entry.WhenToUse = diagnostics.RedactSensitiveText(entry.WhenToUse)
		entry.CapabilityKinds = redactStringSlice(entry.CapabilityKinds)
		entry.ResourceTypes = redactStringSlice(entry.ResourceTypes)
		entry.OperationKinds = redactStringSlice(entry.OperationKinds)
		entry.CostClass = diagnostics.RedactSensitiveText(entry.CostClass)
		out = append(out, entry)
	}
	return out
}

func redactDroppedAgentIndexEntryTraces(entries []promptinput.DroppedAgentIndexEntryTrace) []promptinput.DroppedAgentIndexEntryTrace {
	if len(entries) == 0 {
		return nil
	}
	out := make([]promptinput.DroppedAgentIndexEntryTrace, 0, len(entries))
	for _, entry := range entries {
		entry.Name = diagnostics.RedactSensitiveText(entry.Name)
		entry.Reason = diagnostics.RedactSensitiveText(entry.Reason)
		out = append(out, entry)
	}
	return out
}

func redactAgentDelegationDecisionTrace(decision *promptinput.AgentDelegationDecisionTrace) *promptinput.AgentDelegationDecisionTrace {
	if decision == nil {
		return nil
	}
	out := *decision
	out.Action = diagnostics.RedactSensitiveText(out.Action)
	out.Reason = diagnostics.RedactSensitiveText(out.Reason)
	out.CandidateAgent = diagnostics.RedactSensitiveText(out.CandidateAgent)
	out.ExistingAgentID = diagnostics.RedactSensitiveText(out.ExistingAgentID)
	out.RequiredFields = redactStringSlice(out.RequiredFields)
	return &out
}

func redactAgentAssignmentLintTraces(items []promptinput.AgentAssignmentLintTrace) []promptinput.AgentAssignmentLintTrace {
	if len(items) == 0 {
		return nil
	}
	out := make([]promptinput.AgentAssignmentLintTrace, 0, len(items))
	for _, item := range items {
		item.AgentID = diagnostics.RedactSensitiveText(item.AgentID)
		item.Status = diagnostics.RedactSensitiveText(item.Status)
		item.MissingFields = redactStringSlice(item.MissingFields)
		item.Reasons = redactStringSlice(item.Reasons)
		out = append(out, item)
	}
	return out
}

func redactAgentParallelTraceGroups(groups []promptinput.AgentParallelTraceGroup) []promptinput.AgentParallelTraceGroup {
	if len(groups) == 0 {
		return nil
	}
	out := make([]promptinput.AgentParallelTraceGroup, 0, len(groups))
	for _, group := range groups {
		group.MissionID = diagnostics.RedactSensitiveText(group.MissionID)
		group.SpawnedInTurn = redactStringSlice(group.SpawnedInTurn)
		group.Queued = redactStringSlice(group.Queued)
		for i := range group.SerialReasons {
			group.SerialReasons[i].AgentID = diagnostics.RedactSensitiveText(group.SerialReasons[i].AgentID)
			group.SerialReasons[i].Reason = diagnostics.RedactSensitiveText(group.SerialReasons[i].Reason)
		}
		out = append(out, group)
	}
	return out
}

func redactResourceBindingSnapshots(bindings []resourcebinding.ResourceBindingSnapshot) []resourcebinding.ResourceBindingSnapshot {
	if len(bindings) == 0 {
		return nil
	}
	out := make([]resourcebinding.ResourceBindingSnapshot, 0, len(bindings))
	for _, binding := range bindings {
		binding.Ref = redactResourceRef(binding.Ref)
		binding.Source = diagnostics.RedactSensitiveText(binding.Source)
		binding.VerifiedBy = diagnostics.RedactSensitiveText(binding.VerifiedBy)
		binding.TrustLevel = diagnostics.RedactSensitiveText(binding.TrustLevel)
		binding.TraceHash = diagnostics.RedactSensitiveText(binding.TraceHash)
		out = append(out, binding)
	}
	return out
}

func redactAgentAssemblySnapshot(snapshot *agentassembly.AgentAssemblySnapshot) *agentassembly.AgentAssemblySnapshot {
	if snapshot == nil {
		return nil
	}
	out := *snapshot
	out.AgentKind = diagnostics.RedactSensitiveText(out.AgentKind)
	out.Profile = diagnostics.RedactSensitiveText(out.Profile)
	out.RuntimeRole = diagnostics.RedactSensitiveText(out.RuntimeRole)
	out.RouteReason = redactStringSlice(out.RouteReason)
	out.ResourceBindings = redactResourceBindingSnapshots(out.ResourceBindings)
	out.SessionTargets = redactResourceRefs(out.SessionTargets)
	out.RoleBindings = redactResourceRoleBindings(out.RoleBindings)
	out.ToolSurface = redactAgentAssemblyToolSurface(out.ToolSurface)
	out.ContextSelector.Policy = diagnostics.RedactSensitiveText(out.ContextSelector.Policy)
	out.ContextSelector.Budget = diagnostics.RedactSensitiveText(out.ContextSelector.Budget)
	out.ContextSelector.Hash = diagnostics.RedactSensitiveText(out.ContextSelector.Hash)
	out.PromptSections = redactAgentAssemblyPromptSections(out.PromptSections)
	out.LoopPolicy.ToolCallPolicy = diagnostics.RedactSensitiveText(out.LoopPolicy.ToolCallPolicy)
	out.LoopPolicy.Hash = diagnostics.RedactSensitiveText(out.LoopPolicy.Hash)
	out.FinalContract.Shape = diagnostics.RedactSensitiveText(out.FinalContract.Shape)
	out.FinalContract.Hash = diagnostics.RedactSensitiveText(out.FinalContract.Hash)
	out.ProfilePromptHash = diagnostics.RedactSensitiveText(out.ProfilePromptHash)
	out.SpecHash = diagnostics.RedactSensitiveText(out.SpecHash)
	out.TraceTags = redactStringMap(copyStringMap(out.TraceTags))
	return &out
}

func redactAgentAssemblyToolSurface(surface agentassembly.ToolSurfaceSnapshot) agentassembly.ToolSurfaceSnapshot {
	surface.RegisteredTools = redactAgentAssemblyToolItems(surface.RegisteredTools)
	surface.ModelVisibleTools = redactAgentAssemblyToolItems(surface.ModelVisibleTools)
	surface.DispatchableTools = redactAgentAssemblyToolItems(surface.DispatchableTools)
	surface.HiddenTools = redactAgentAssemblyToolItems(surface.HiddenTools)
	surface.PolicyHash = diagnostics.RedactSensitiveText(surface.PolicyHash)
	surface.Fingerprint = diagnostics.RedactSensitiveText(surface.Fingerprint)
	surface.Hash = diagnostics.RedactSensitiveText(surface.Hash)
	return surface
}

func redactAgentAssemblyToolItems(items []agentassembly.ToolSurfaceItem) []agentassembly.ToolSurfaceItem {
	if len(items) == 0 {
		return nil
	}
	out := make([]agentassembly.ToolSurfaceItem, 0, len(items))
	for _, item := range items {
		item.Name = diagnostics.RedactSensitiveText(item.Name)
		item.Namespace = diagnostics.RedactSensitiveText(item.Namespace)
		item.DescriptionHash = diagnostics.RedactSensitiveText(item.DescriptionHash)
		item.ResourceBindingHash = diagnostics.RedactSensitiveText(item.ResourceBindingHash)
		item.Capability = diagnostics.RedactSensitiveText(item.Capability)
		item.PolicyHash = diagnostics.RedactSensitiveText(item.PolicyHash)
		item.HiddenReason = diagnostics.RedactSensitiveText(item.HiddenReason)
		out = append(out, item)
	}
	return out
}

func redactAgentAssemblyPromptSections(snapshot agentassembly.PromptSectionSnapshot) agentassembly.PromptSectionSnapshot {
	for i := range snapshot.Sections {
		snapshot.Sections[i].ID = diagnostics.RedactSensitiveText(snapshot.Sections[i].ID)
		snapshot.Sections[i].Kind = diagnostics.RedactSensitiveText(snapshot.Sections[i].Kind)
		snapshot.Sections[i].Source = diagnostics.RedactSensitiveText(snapshot.Sections[i].Source)
		snapshot.Sections[i].Hash = diagnostics.RedactSensitiveText(snapshot.Sections[i].Hash)
	}
	snapshot.Hash = diagnostics.RedactSensitiveText(snapshot.Hash)
	return snapshot
}

func redactResourceRefs(refs []resourcebinding.ResourceRef) []resourcebinding.ResourceRef {
	if len(refs) == 0 {
		return nil
	}
	out := make([]resourcebinding.ResourceRef, 0, len(refs))
	for _, ref := range refs {
		out = append(out, redactResourceRef(ref))
	}
	return out
}

func redactResourceRoleBindings(bindings []resourcebinding.ResourceRoleBinding) []resourcebinding.ResourceRoleBinding {
	if len(bindings) == 0 {
		return nil
	}
	out := make([]resourcebinding.ResourceRoleBinding, 0, len(bindings))
	for _, binding := range bindings {
		binding.BindingID = diagnostics.RedactSensitiveText(binding.BindingID)
		binding.ResourceRef = redactResourceRef(binding.ResourceRef)
		binding.Role = diagnostics.RedactSensitiveText(binding.Role)
		binding.RoleAlias = redactStringSlice(binding.RoleAlias)
		binding.SourceTurnID = diagnostics.RedactSensitiveText(binding.SourceTurnID)
		binding.SourceSpan = diagnostics.RedactSensitiveText(binding.SourceSpan)
		binding.ConflictPolicy = diagnostics.RedactSensitiveText(binding.ConflictPolicy)
		binding.TraceHash = diagnostics.RedactSensitiveText(binding.TraceHash)
		out = append(out, binding)
	}
	return out
}

func redactResourceCapabilities(capabilities []resourcebinding.ResourceCapability) []resourcebinding.ResourceCapability {
	if len(capabilities) == 0 {
		return nil
	}
	out := make([]resourcebinding.ResourceCapability, 0, len(capabilities))
	for _, capability := range capabilities {
		capability.ResourceRef = redactResourceRef(capability.ResourceRef)
		capability.Capability = diagnostics.RedactSensitiveText(capability.Capability)
		capability.ToolNames = redactStringSlice(capability.ToolNames)
		capability.PolicyHash = diagnostics.RedactSensitiveText(capability.PolicyHash)
		capability.BindingTraceHash = diagnostics.RedactSensitiveText(capability.BindingTraceHash)
		capability.TraceHash = diagnostics.RedactSensitiveText(capability.TraceHash)
		out = append(out, capability)
	}
	return out
}

func redactResourceEvidenceRefs(refs []resourcebinding.EvidenceRef) []resourcebinding.EvidenceRef {
	if len(refs) == 0 {
		return nil
	}
	out := make([]resourcebinding.EvidenceRef, 0, len(refs))
	for _, ref := range refs {
		ref.ID = diagnostics.RedactSensitiveText(ref.ID)
		ref.ResourceRef = redactResourceRef(ref.ResourceRef)
		ref.Source = diagnostics.RedactSensitiveText(ref.Source)
		ref.Kind = diagnostics.RedactSensitiveText(ref.Kind)
		ref.TraceHash = diagnostics.RedactSensitiveText(ref.TraceHash)
		out = append(out, ref)
	}
	return out
}

func redactSessionTargetSnapshot(snapshot *resourcebinding.SessionTargetSnapshot) *resourcebinding.SessionTargetSnapshot {
	if snapshot == nil {
		return nil
	}
	out := *snapshot
	out.ActiveTargetSetID = diagnostics.RedactSensitiveText(out.ActiveTargetSetID)
	out.HostIDs = redactStringSlice(out.HostIDs)
	out.SourceTurnID = diagnostics.RedactSensitiveText(out.SourceTurnID)
	out.SourceMentionIDs = redactStringSlice(out.SourceMentionIDs)
	out.BindingMode = diagnostics.RedactSensitiveText(out.BindingMode)
	out.TraceHash = diagnostics.RedactSensitiveText(out.TraceHash)
	return &out
}

func redactRoleBindingConflicts(conflicts []resourcebinding.RoleBindingConflict) []resourcebinding.RoleBindingConflict {
	if len(conflicts) == 0 {
		return nil
	}
	out := make([]resourcebinding.RoleBindingConflict, 0, len(conflicts))
	for _, conflict := range conflicts {
		conflict.ResourceID = diagnostics.RedactSensitiveText(conflict.ResourceID)
		conflict.Role = diagnostics.RedactSensitiveText(conflict.Role)
		conflict.Reasons = redactStringSlice(conflict.Reasons)
		conflict.TraceHash = diagnostics.RedactSensitiveText(conflict.TraceHash)
		out = append(out, conflict)
	}
	return out
}

func redactResourceRef(ref resourcebinding.ResourceRef) resourcebinding.ResourceRef {
	ref.Type = diagnostics.RedactSensitiveText(ref.Type)
	ref.ID = diagnostics.RedactSensitiveText(ref.ID)
	ref.DisplayName = diagnostics.RedactSensitiveText(ref.DisplayName)
	ref.Namespace = diagnostics.RedactSensitiveText(ref.Namespace)
	ref.Provider = diagnostics.RedactSensitiveText(ref.Provider)
	return ref
}

func redactResourceLockTraces(locks []promptinput.ResourceLockTrace) []promptinput.ResourceLockTrace {
	if len(locks) == 0 {
		return nil
	}
	out := make([]promptinput.ResourceLockTrace, 0, len(locks))
	for _, lock := range locks {
		lock.LeaseID = diagnostics.RedactSensitiveText(lock.LeaseID)
		lock.AgentID = diagnostics.RedactSensitiveText(lock.AgentID)
		lock.Action = diagnostics.RedactSensitiveText(lock.Action)
		lock.Reason = diagnostics.RedactSensitiveText(lock.Reason)
		lock.Holder = diagnostics.RedactSensitiveText(lock.Holder)
		lock.Key.ResourceType = diagnostics.RedactSensitiveText(lock.Key.ResourceType)
		lock.Key.ResourceID = diagnostics.RedactSensitiveText(lock.Key.ResourceID)
		lock.Key.OperationKind = diagnostics.RedactSensitiveText(lock.Key.OperationKind)
		out = append(out, lock)
	}
	return out
}

func redactAgentFinalGateDecisionTrace(gate *promptinput.AgentFinalGateDecisionTrace) *promptinput.AgentFinalGateDecisionTrace {
	if gate == nil {
		return nil
	}
	out := *gate
	out.Action = diagnostics.RedactSensitiveText(out.Action)
	out.PendingAgents = redactStringSlice(out.PendingAgents)
	out.Reasons = redactStringSlice(out.Reasons)
	return &out
}

func redactAgentNotificationTraces(items []promptinput.AgentNotificationTrace) []promptinput.AgentNotificationTrace {
	if len(items) == 0 {
		return nil
	}
	out := make([]promptinput.AgentNotificationTrace, 0, len(items))
	for _, item := range items {
		item.AgentID = diagnostics.RedactSensitiveText(item.AgentID)
		item.Status = diagnostics.RedactSensitiveText(item.Status)
		item.Summary = diagnostics.RedactSensitiveText(item.Summary)
		item.ResultRefs = redactStringSlice(item.ResultRefs)
		item.Error = diagnostics.RedactSensitiveText(item.Error)
		out = append(out, item)
	}
	return out
}

func redactVerificationAgentReportTrace(report *promptinput.VerificationAgentReportTrace) *promptinput.VerificationAgentReportTrace {
	if report == nil {
		return nil
	}
	out := *report
	out.Status = diagnostics.RedactSensitiveText(out.Status)
	out.Summary = diagnostics.RedactSensitiveText(out.Summary)
	out.EvidenceRefs = redactStringSlice(out.EvidenceRefs)
	out.Counterchecks = redactStringSlice(out.Counterchecks)
	out.Blockers = redactStringSlice(out.Blockers)
	return &out
}

func redactCompletionGateTrace(gate *promptinput.CompletionGateTrace) *promptinput.CompletionGateTrace {
	if gate == nil {
		return nil
	}
	out := *gate
	out.Decision = diagnostics.RedactSensitiveText(out.Decision)
	out.Reasons = redactStringSlice(out.Reasons)
	return &out
}

func redactTaskDepthTrace(trace *promptinput.TaskDepthTrace) *promptinput.TaskDepthTrace {
	if trace == nil {
		return nil
	}
	out := *trace
	out.Level = diagnostics.RedactSensitiveText(out.Level)
	out.Reasons = redactStringSlice(out.Reasons)
	return &out
}

func redactEvidenceCoverageTrace(trace *promptinput.EvidenceCoverageTrace) *promptinput.EvidenceCoverageTrace {
	if trace == nil {
		return nil
	}
	out := *trace
	out.Action = diagnostics.RedactSensitiveText(out.Action)
	out.RequiredDimensions = redactStringSlice(out.RequiredDimensions)
	out.CoveredDimensions = redactStringSlice(out.CoveredDimensions)
	out.MissingDimensions = redactStringSlice(out.MissingDimensions)
	out.OpenQuestions = redactStringSlice(out.OpenQuestions)
	out.VerificationStatus = diagnostics.RedactSensitiveText(out.VerificationStatus)
	out.Reasons = redactStringSlice(out.Reasons)
	return &out
}

func redactGenericityTrace(trace *promptinput.GenericityTrace) *promptinput.GenericityTrace {
	if trace == nil {
		return nil
	}
	out := *trace
	out.CoreRuleDomainTerms = redactStringSlice(out.CoreRuleDomainTerms)
	out.AllowedFixtureTerms = redactStringSlice(out.AllowedFixtureTerms)
	out.AllowedPluginTerms = redactStringSlice(out.AllowedPluginTerms)
	out.ResourceIDSource = diagnostics.RedactSensitiveText(out.ResourceIDSource)
	out.Violations = redactStringSlice(out.Violations)
	return &out
}

func redactSafetySignalTraces(signals []promptinput.SafetySignalTrace) []promptinput.SafetySignalTrace {
	if len(signals) == 0 {
		return nil
	}
	out := make([]promptinput.SafetySignalTrace, 0, len(signals))
	for _, signal := range signals {
		signal.Category = diagnostics.RedactSensitiveText(signal.Category)
		signal.Severity = diagnostics.RedactSensitiveText(signal.Severity)
		signal.Action = diagnostics.RedactSensitiveText(signal.Action)
		signal.Reasons = redactStringSlice(signal.Reasons)
		out = append(out, signal)
	}
	return out
}

func redactUnexpectedStateGateTrace(gate *promptinput.UnexpectedStateGateTrace) *promptinput.UnexpectedStateGateTrace {
	if gate == nil {
		return nil
	}
	out := *gate
	out.Action = diagnostics.RedactSensitiveText(out.Action)
	out.Sources = redactStringSlice(out.Sources)
	out.AffectedScopes = redactStringSlice(out.AffectedScopes)
	out.BlockedAction = diagnostics.RedactSensitiveText(out.BlockedAction)
	out.Reasons = redactStringSlice(out.Reasons)
	return &out
}

func redactApprovalScopeTrace(scope *promptinput.ApprovalScopeTrace) *promptinput.ApprovalScopeTrace {
	if scope == nil {
		return nil
	}
	out := *scope
	out.GrantID = diagnostics.RedactSensitiveText(out.GrantID)
	out.Status = diagnostics.RedactSensitiveText(out.Status)
	out.AllowedActions = redactStringSlice(out.AllowedActions)
	out.ResourceScopes = redactStringSlice(out.ResourceScopes)
	out.RiskCeiling = diagnostics.RedactSensitiveText(out.RiskCeiling)
	out.ExpiresAt = diagnostics.RedactSensitiveText(out.ExpiresAt)
	out.InputHash = diagnostics.RedactSensitiveText(out.InputHash)
	out.Reasons = redactStringSlice(out.Reasons)
	return &out
}

func redactPromptSections(sections []promptcompiler.PromptSectionTrace) []promptcompiler.PromptSectionTrace {
	if len(sections) == 0 {
		return nil
	}
	out := make([]promptcompiler.PromptSectionTrace, 0, len(sections))
	for _, section := range sections {
		section.ID = diagnostics.RedactSensitiveText(section.ID)
		section.Kind = diagnostics.RedactSensitiveText(section.Kind)
		section.Source = diagnostics.RedactSensitiveText(section.Source)
		section.Hash = diagnostics.RedactSensitiveText(section.Hash)
		section.Cache = diagnostics.RedactSensitiveText(section.Cache)
		out = append(out, section)
	}
	return out
}

func redactChangedPromptSections(sections []promptcompiler.ChangedPromptSection) []promptcompiler.ChangedPromptSection {
	if len(sections) == 0 {
		return nil
	}
	out := make([]promptcompiler.ChangedPromptSection, 0, len(sections))
	for _, section := range sections {
		section.ID = diagnostics.RedactSensitiveText(section.ID)
		section.Reason = diagnostics.RedactSensitiveText(section.Reason)
		section.PreviousHash = diagnostics.RedactSensitiveText(section.PreviousHash)
		section.CurrentHash = diagnostics.RedactSensitiveText(section.CurrentHash)
		out = append(out, section)
	}
	return out
}

func redactContextUsage(usage promptinput.ContextUsage) promptinput.ContextUsage {
	if contextUsageEmpty(usage) {
		return promptinput.ContextUsage{}
	}
	out := promptinput.ContextUsage{
		MaxContextTokens:     usage.MaxContextTokens,
		ReservedOutputTokens: usage.ReservedOutputTokens,
		EstimatedInputTokens: usage.EstimatedInputTokens,
		Categories:           append([]promptinput.ContextUsageCategory(nil), usage.Categories...),
		TopContributors:      make([]promptinput.ContextContributor, 0, len(usage.TopContributors)),
	}
	for _, contributor := range usage.TopContributors {
		contributor.Kind = diagnostics.RedactSensitiveText(contributor.Kind)
		contributor.ID = diagnostics.RedactSensitiveText(contributor.ID)
		contributor.Action = diagnostics.RedactSensitiveText(contributor.Action)
		out.TopContributors = append(out.TopContributors, contributor)
	}
	for i := range out.Categories {
		out.Categories[i].Name = diagnostics.RedactSensitiveText(out.Categories[i].Name)
	}
	return out
}

func redactContextGovernanceTraceItems(items []promptinput.ContextGovernanceTraceItem) []promptinput.ContextGovernanceTraceItem {
	if len(items) == 0 {
		return nil
	}
	out := make([]promptinput.ContextGovernanceTraceItem, 0, len(items))
	for _, item := range items {
		item.ID = diagnostics.RedactSensitiveText(item.ID)
		item.Layer = diagnostics.RedactSensitiveText(item.Layer)
		item.Kind = diagnostics.RedactSensitiveText(item.Kind)
		item.Message = diagnostics.RedactSensitiveText(item.Message)
		item.ToolCallID = diagnostics.RedactSensitiveText(item.ToolCallID)
		item.ToolName = diagnostics.RedactSensitiveText(item.ToolName)
		item.MaterializationTier = diagnostics.RedactSensitiveText(item.MaterializationTier)
		item.ReferenceIDs = redactStringSlice(item.ReferenceIDs)
		if item.Resource != nil {
			resource := *item.Resource
			resource.URI = diagnostics.RedactSensitiveText(resource.URI)
			resource.Digest = diagnostics.RedactSensitiveText(resource.Digest)
			resource.ContentType = diagnostics.RedactSensitiveText(resource.ContentType)
			resource.Range.Query = diagnostics.RedactSensitiveText(resource.Range.Query)
			resource.Range.Format = diagnostics.RedactSensitiveText(resource.Range.Format)
			item.Resource = &resource
		}
		if len(item.Budget) > 0 {
			budget := make(map[string]int, len(item.Budget))
			for key, value := range item.Budget {
				budget[diagnostics.RedactSensitiveText(key)] = value
			}
			item.Budget = budget
		}
		out = append(out, item)
	}
	return out
}

func redactStringSlice(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, value := range in {
		out = append(out, diagnostics.RedactSensitiveText(value))
	}
	return out
}

func redactOwnerWriteTraces(in []promptinput.OwnerWriteTrace) []promptinput.OwnerWriteTrace {
	if len(in) == 0 {
		return nil
	}
	out := make([]promptinput.OwnerWriteTrace, 0, len(in))
	for _, trace := range in {
		out = append(out, promptinput.OwnerWriteTrace{
			Responsibility: diagnostics.RedactSensitiveText(trace.Responsibility),
			Owner:          diagnostics.RedactSensitiveText(trace.Owner),
			Writer:         diagnostics.RedactSensitiveText(trace.Writer),
			SessionID:      diagnostics.RedactSensitiveText(trace.SessionID),
			TurnID:         diagnostics.RedactSensitiveText(trace.TurnID),
			Outcome:        diagnostics.RedactSensitiveText(trace.Outcome),
			CreatedAt:      diagnostics.RedactSensitiveText(trace.CreatedAt),
		})
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

func traceDirectory(root string, req Request) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		root = DefaultRootDir("")
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
	if len(payload.ContextGovernance) > 0 {
		fmt.Fprintf(&b, "\n%s", renderContextGovernanceMarkdown(payload.ContextGovernance))
	}
	if payload.ToolSurfaceFingerprint != "" || payload.ToolSurfacePolicySnapshotHash != "" {
		fmt.Fprintf(&b, "\n## Tool Surface\n\n")
		if payload.ToolSurfaceFingerprint != "" {
			fmt.Fprintf(&b, "- Fingerprint: `%s`\n", payload.ToolSurfaceFingerprint)
		}
		if payload.ToolSurfacePolicySnapshotHash != "" {
			fmt.Fprintf(&b, "- Policy snapshot: `%s`\n", payload.ToolSurfacePolicySnapshotHash)
		}
	}
	if payload.FinalEvidenceState != nil {
		data, _ := json.MarshalIndent(payload.FinalEvidenceState, "", "  ")
		fmt.Fprintf(&b, "\n## Final Evidence State\n\n```json\n%s\n```\n", string(data))
	}
	if !modelTraceVerificationSafetyEmpty(payload) {
		fmt.Fprintf(&b, "\n## Verification/Safety Trace\n\n")
		if payload.VerificationStatus != "" {
			fmt.Fprintf(&b, "- verification_status: `%s`\n", payload.VerificationStatus)
		}
		if payload.VerificationReportRef != "" {
			fmt.Fprintf(&b, "- verification_report_ref: `%s`\n", payload.VerificationReportRef)
		}
		if payload.CompletionGate != nil {
			fmt.Fprintf(&b, "- completion_gate: `%s`\n", payload.CompletionGate.Decision)
		}
		for _, signal := range payload.SafetySignals {
			fmt.Fprintf(&b, "- safety: `%s/%s`", signal.Category, signal.Severity)
			if signal.Action != "" {
				fmt.Fprintf(&b, " action=`%s`", signal.Action)
			}
			fmt.Fprintln(&b)
		}
		if payload.UnexpectedStateGate != nil {
			fmt.Fprintf(&b, "- unexpected_state_gate: `%s`", payload.UnexpectedStateGate.Action)
			if payload.UnexpectedStateGate.BlockedAction != "" {
				fmt.Fprintf(&b, " blocked_action=`%s`", payload.UnexpectedStateGate.BlockedAction)
			}
			fmt.Fprintln(&b)
		}
		if payload.ApprovalScope != nil {
			fmt.Fprintf(&b, "- approval_scope: `%s`", payload.ApprovalScope.Status)
			if payload.ApprovalScope.RiskCeiling != "" {
				fmt.Fprintf(&b, " risk=`%s`", payload.ApprovalScope.RiskCeiling)
			}
			if payload.ApprovalScope.InputHash != "" {
				fmt.Fprintf(&b, " input_hash=`%s`", payload.ApprovalScope.InputHash)
			}
			fmt.Fprintln(&b)
		}
	}
	if payload.PlanModeState != nil || payload.PlanArtifactRef != "" || payload.PlanCompletionGate != nil {
		fmt.Fprintf(&b, "\n## Plan Mode Trace\n\n")
		if payload.PlanModeState != nil {
			fmt.Fprintf(&b, "- state: `%s`\n", payload.PlanModeState.State)
			if payload.PlanModeState.PlanID != "" {
				fmt.Fprintf(&b, "- plan_id: `%s`\n", payload.PlanModeState.PlanID)
			}
			if payload.PlanModeState.ApprovalStatus != "" {
				fmt.Fprintf(&b, "- approval_status: `%s`\n", payload.PlanModeState.ApprovalStatus)
			}
		}
		if payload.PlanArtifactRef != "" {
			fmt.Fprintf(&b, "- artifact_ref: `%s`\n", payload.PlanArtifactRef)
		}
		if payload.PlanCompletionGate != nil {
			fmt.Fprintf(&b, "- completion_gate: `%s`\n", payload.PlanCompletionGate.Decision)
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
	if !promptInputTraceEmpty(payload.PromptInputTrace) {
		traceMarkdown := promptinput.RenderMarkdown(payload.PromptInputTrace)
		traceMarkdown = strings.Replace(traceMarkdown, "# Prompt Input Trace", "## Prompt Input Trace", 1)
		fmt.Fprintf(&b, "\n%s", traceMarkdown)
	}
	if payload.DiagnosticTrace != nil {
		fmt.Fprintf(&b, "\n%s", renderDiagnosticTraceMarkdown(*payload.DiagnosticTrace))
	}
	return b.String()
}

func modelTraceVerificationSafetyEmpty(payload payload) bool {
	return strings.TrimSpace(payload.VerificationReportRef) == "" &&
		strings.TrimSpace(payload.VerificationStatus) == "" &&
		payload.CompletionGate == nil &&
		len(payload.SafetySignals) == 0 &&
		payload.UnexpectedStateGate == nil &&
		payload.ApprovalScope == nil
}

func promptInputTraceEmpty(trace promptinput.PromptInputTrace) bool {
	return len(trace.Items) == 0 &&
		len(trace.PromptSections) == 0 &&
		len(trace.ChangedSections) == 0 &&
		trace.OpsContextCapsuleChars == 0 &&
		trace.SessionFactCount == 0 &&
		trace.LettaHintCount == 0 &&
		trace.MemoryItemCount == 0 &&
		len(trace.VisibleOpsManualTools) == 0 &&
		len(trace.DroppedContextReasons) == 0 &&
		trace.ContextDedupe == nil &&
		len(trace.ContextGovernance) == 0 &&
		contextUsageEmpty(trace.ContextUsage) &&
		strings.TrimSpace(trace.AssemblySource) == "" &&
		strings.TrimSpace(trace.PromptCompilerSource) == "" &&
		strings.TrimSpace(trace.ToolSurfaceSource) == "" &&
		strings.TrimSpace(trace.AdapterName) == "" &&
		strings.TrimSpace(trace.ToolSurfaceFingerprint) == "" &&
		strings.TrimSpace(trace.ToolSurfacePolicySnapshotHash) == "" &&
		toolSurfaceSnapshotTraceEmpty(trace.ToolSurfaceSnapshot) &&
		trace.PublicWebBudget == nil &&
		trace.WebSearchPolicy == nil &&
		trace.WebSearch == nil &&
		trace.Final == nil &&
		len(trace.DeferredToolDirectory) == 0 &&
		len(trace.LoadedToolsDelta) == 0 &&
		len(trace.LoadedPacksDelta) == 0 &&
		strings.TrimSpace(trace.SkillIndexHash) == "" &&
		len(trace.LoadedSkillsDelta) == 0 &&
		len(trace.ToolSearchEvents) == 0 &&
		len(trace.ToolSelectionEvents) == 0 &&
		len(trace.RejectedToolCalls) == 0 &&
		len(trace.DispatchDecisions) == 0 &&
		len(trace.SkillSearchEvents) == 0 &&
		len(trace.SkillReadEvents) == 0 &&
		len(trace.RejectedSkillActivations) == 0 &&
		len(trace.MCPInstructionDeltas) == 0 &&
		len(trace.ParallelDispatchGroups) == 0 &&
		len(trace.FailedToolSummaries) == 0 &&
		strings.TrimSpace(trace.AgentIndexHash) == "" &&
		len(trace.AgentIndexEntries) == 0 &&
		len(trace.AgentIndexDropped) == 0 &&
		len(trace.AgentIndexDelta) == 0 &&
		trace.AgentDelegationDecision == nil &&
		len(trace.AgentAssignmentLint) == 0 &&
		len(trace.AgentParallelTraceGroups) == 0 &&
		len(trace.ResourceBindings) == 0 &&
		len(trace.ResourceRoleBindings) == 0 &&
		len(trace.ResourceCapabilities) == 0 &&
		len(trace.ResourceEvidenceRefs) == 0 &&
		trace.SessionTargetSnapshot == nil &&
		len(trace.RoleBindingConflicts) == 0 &&
		trace.AgentAssemblySnapshot == nil &&
		specialinputmemory.WorldStateSectionEmpty(trace.SpecialInputWorldState) &&
		len(trace.ResourceLocks) == 0 &&
		len(trace.OwnerWriteTraces) == 0 &&
		trace.AgentFinalGate == nil &&
		len(trace.AgentNotifications) == 0 &&
		trace.VerificationAgentReport == nil &&
		strings.TrimSpace(trace.VerificationReportRef) == "" &&
		strings.TrimSpace(trace.VerificationStatus) == "" &&
		trace.TaskDepth == nil &&
		trace.EvidenceCoverage == nil &&
		trace.GenericityTrace == nil &&
		trace.CompletionGate == nil &&
		len(trace.SafetySignals) == 0 &&
		trace.UnexpectedStateGate == nil &&
		trace.ApprovalScope == nil &&
		trace.PlanModeState == nil &&
		strings.TrimSpace(trace.PlanArtifactRef) == "" &&
		len(trace.PlanTransitions) == 0 &&
		trace.PlanRequirementDecision == nil &&
		trace.PlanCompletionGate == nil &&
		len(trace.TaskClaims) == 0 &&
		trace.PlanApprovalScope == nil &&
		len(trace.PlanRejectionEvents) == 0 &&
		trace.TaskTodoState == nil
}

func contextUsageEmpty(usage promptinput.ContextUsage) bool {
	return usage.MaxContextTokens == 0 &&
		usage.ReservedOutputTokens == 0 &&
		usage.EstimatedInputTokens == 0 &&
		len(usage.Categories) == 0 &&
		len(usage.TopContributors) == 0
}

func renderContextGovernanceMarkdown(items []promptinput.ContextGovernanceTraceItem) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Context Governance\n\n")
	fmt.Fprintf(&b, "| # | layer | kind | message | retry |\n")
	fmt.Fprintf(&b, "|---:|---|---|---|---|\n")
	for i, item := range items {
		retry := ""
		if item.RetryAttempt > 0 || item.RetryMax > 0 {
			retry = fmt.Sprintf("%d/%d", item.RetryAttempt, item.RetryMax)
		}
		fmt.Fprintf(
			&b,
			"| %d | %s | %s | %s | %s |\n",
			i,
			escapeMarkdownCell(item.Layer),
			escapeMarkdownCell(item.Kind),
			escapeMarkdownCell(item.Message),
			escapeMarkdownCell(retry),
		)
	}
	renderContextBudgetMarkdown(&b, items)
	renderExternalReferencesMarkdown(&b, items)
	renderResourceDedupeMarkdown(&b, items)
	return b.String()
}

func renderContextBudgetMarkdown(b *strings.Builder, items []promptinput.ContextGovernanceTraceItem) {
	fmt.Fprintf(b, "\n### Budget\n")
	wrote := false
	for _, item := range items {
		if len(item.Budget) == 0 {
			continue
		}
		fmt.Fprintf(b, "- `%s/%s`", escapeBackticks(item.Layer), escapeBackticks(item.Kind))
		for _, key := range sortedIntMapKeys(item.Budget) {
			fmt.Fprintf(b, " %s=`%d`", escapeBackticks(key), item.Budget[key])
		}
		fmt.Fprintln(b)
		wrote = true
	}
	if !wrote {
		fmt.Fprintln(b, "_None._")
	}
}

func renderExternalReferencesMarkdown(b *strings.Builder, items []promptinput.ContextGovernanceTraceItem) {
	fmt.Fprintf(b, "\n### External References\n")
	wrote := false
	for _, item := range items {
		if len(item.ReferenceIDs) == 0 {
			continue
		}
		fmt.Fprintf(
			b,
			"- `%s/%s`: `%s`\n",
			escapeBackticks(item.Layer),
			escapeBackticks(item.Kind),
			escapeBackticks(strings.Join(item.ReferenceIDs, "`, `")),
		)
		wrote = true
	}
	if !wrote {
		fmt.Fprintln(b, "_None._")
	}
}

func renderResourceDedupeMarkdown(b *strings.Builder, items []promptinput.ContextGovernanceTraceItem) {
	fmt.Fprintf(b, "\n### Resource Dedupe\n")
	wrote := false
	for _, item := range items {
		if item.Resource == nil {
			continue
		}
		rng := item.Resource.Range
		fmt.Fprintf(
			b,
			"- `%s/%s`: uri=`%s` digest=`%s` bytes=`%d` offset=%d limit=%d page=%d query=`%s` format=`%s`\n",
			escapeBackticks(item.Layer),
			escapeBackticks(item.Kind),
			escapeBackticks(item.Resource.URI),
			escapeBackticks(item.Resource.Digest),
			item.Resource.Bytes,
			rng.Offset,
			rng.Limit,
			rng.Page,
			escapeBackticks(rng.Query),
			escapeBackticks(rng.Format),
		)
		wrote = true
	}
	if !wrote {
		fmt.Fprintln(b, "_None._")
	}
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

func sortedIntMapKeys(in map[string]int) []string {
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
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

func escapeMarkdownCell(value string) string {
	value = strings.ReplaceAll(value, "\n", "\\n")
	value = strings.ReplaceAll(value, "|", "\\|")
	return value
}

func escapeBackticks(value string) string {
	return strings.ReplaceAll(value, "`", "'")
}
