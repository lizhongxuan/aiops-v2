package appui

import "aiops-v2/internal/agentui"

type AgentEventKind = agentui.AgentEventKind
type AgentEventPhase = agentui.AgentEventPhase
type AgentEventStatus = agentui.AgentEventStatus
type AgentEventVisibility = agentui.AgentEventVisibility
type AgentEventSource = agentui.AgentEventSource

const (
	AgentEventTurn      = agentui.AgentEventTurn
	AgentEventAgent     = agentui.AgentEventAgent
	AgentEventAssistant = agentui.AgentEventAssistant
	AgentEventTool      = agentui.AgentEventTool
	AgentEventApproval  = agentui.AgentEventApproval
	AgentEventArtifact  = agentui.AgentEventArtifact
	AgentEventDiff      = agentui.AgentEventDiff
	AgentEventBrowser   = agentui.AgentEventBrowser
	AgentEventSystem    = agentui.AgentEventSystem
)

const (
	AgentEventPhaseRequested = agentui.AgentEventPhaseRequested
	AgentEventPhaseStarted   = agentui.AgentEventPhaseStarted
	AgentEventPhaseDelta     = agentui.AgentEventPhaseDelta
	AgentEventPhaseUpdated   = agentui.AgentEventPhaseUpdated
	AgentEventPhaseCompleted = agentui.AgentEventPhaseCompleted
	AgentEventPhaseFailed    = agentui.AgentEventPhaseFailed
	AgentEventPhaseCanceled  = agentui.AgentEventPhaseCanceled
	AgentEventPhaseBlocked   = agentui.AgentEventPhaseBlocked
	AgentEventPhaseResolved  = agentui.AgentEventPhaseResolved
)

const (
	AgentEventStatusQueued    = agentui.AgentEventStatusQueued
	AgentEventStatusRunning   = agentui.AgentEventStatusRunning
	AgentEventStatusWaiting   = agentui.AgentEventStatusWaiting
	AgentEventStatusBlocked   = agentui.AgentEventStatusBlocked
	AgentEventStatusCompleted = agentui.AgentEventStatusCompleted
	AgentEventStatusFailed    = agentui.AgentEventStatusFailed
	AgentEventStatusCanceled  = agentui.AgentEventStatusCanceled
	AgentEventStatusSkipped   = agentui.AgentEventStatusSkipped
)

const (
	AgentEventVisibilityPrimary   = agentui.AgentEventVisibilityPrimary
	AgentEventVisibilitySecondary = agentui.AgentEventVisibilitySecondary
	AgentEventVisibilityDebug     = agentui.AgentEventVisibilityDebug
	AgentEventVisibilityHidden    = agentui.AgentEventVisibilityHidden
)

const (
	AgentEventSourceRuntime    = agentui.AgentEventSourceRuntime
	AgentEventSourceTool       = agentui.AgentEventSourceTool
	AgentEventSourceMCP        = agentui.AgentEventSourceMCP
	AgentEventSourceApproval   = agentui.AgentEventSourceApproval
	AgentEventSourceUI         = agentui.AgentEventSourceUI
	AgentEventSourceProjection = agentui.AgentEventSourceProjection
	AgentEventSourceSystem     = agentui.AgentEventSourceSystem
)

type AgentEvent = agentui.AgentEvent
type AgentConfig = agentui.AgentConfig
type AgentStats = agentui.AgentStats
type TurnPayload = agentui.TurnPayload
type AgentPayload = agentui.AgentPayload
type AssistantPayload = agentui.AssistantPayload
type ToolPayload = agentui.ToolPayload
type ApprovalPayload = agentui.ApprovalPayload
type ArtifactPayload = agentui.ArtifactPayload
type DiffFile = agentui.DiffFile
type DiffPayload = agentui.DiffPayload
type BrowserPayload = agentui.BrowserPayload

type RuntimeLiveness = agentui.RuntimeLiveness
type AgentEventProjection = agentui.AgentEventProjection
type TimelineEntry = agentui.TimelineEntry
type AgentProjection = agentui.AgentProjection
type ApprovalProjection = agentui.ApprovalProjection
type ArtifactProjection = agentui.ArtifactProjection
type DiffProjection = agentui.DiffProjection
type AssistantFinal = agentui.AssistantFinal
