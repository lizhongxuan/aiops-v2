package appui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aiops-v2/internal/auth"
	"aiops-v2/internal/hostops"
	"aiops-v2/internal/incidents"
	"aiops-v2/internal/mcp"
	"aiops-v2/internal/opsmanual"
	"aiops-v2/internal/plugins"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/store"
	"aiops-v2/internal/terminal"
	"aiops-v2/internal/tooling"
)

// RuntimeGateway is the runtime-facing dependency used by the Web application
// layer. It keeps transport handlers away from runtimekernel details.
type RuntimeGateway interface {
	RunTurn(ctx context.Context, req runtimekernel.TurnRequest) (runtimekernel.TurnResult, error)
	ResumeTurn(ctx context.Context, req runtimekernel.ResumeRequest) (runtimekernel.TurnResult, error)
	CancelTurn(ctx context.Context, req runtimekernel.CancelRequest) (runtimekernel.TurnResult, error)
}

// SessionSource is the read-side session dependency used by state/session
// application services.
type SessionSource interface {
	Get(id string) *runtimekernel.SessionState
	GetLatest() *runtimekernel.SessionState
	List() []*runtimekernel.SessionState
}

// SessionStore is the write-capable session dependency used by session
// mutation services.
type SessionStore interface {
	SessionSource
	GetOrCreate(sessionID string, sessionType runtimekernel.SessionType, mode runtimekernel.Mode) *runtimekernel.SessionState
	Update(session *runtimekernel.SessionState)
}

// SettingsRepository is the persisted backing store for lightweight web
// settings and LLM configuration.
type SettingsRepository interface {
	GetWebSettings() (*store.WebSettings, error)
	SaveWebSettings(settings *store.WebSettings) error
	GetLLMConfig() (*store.LLMConfig, error)
	SaveLLMConfig(config *store.LLMConfig) error
}

// CorootConfigRepository is the persisted backing store for the Coroot
// observability page connection config.
type CorootConfigRepository interface {
	GetCorootConfig() (*store.CorootConfig, error)
	SaveCorootConfig(config *store.CorootConfig) error
}

// MCPRepository is the persisted backing store for MCP server runtime config.
type MCPRepository interface {
	GetMCPServers() ([]store.MCPServerRecord, error)
	SaveMCPServers(items []store.MCPServerRecord) error
}

// SkillCatalogRepository is the persisted backing store for the agent skill
// catalog.
type SkillCatalogRepository interface {
	GetSkillCatalog() ([]store.SkillCatalogEntry, error)
	SaveSkillCatalog(items []store.SkillCatalogEntry) error
}

// AgentMCPCatalogRepository is the persisted backing store for agent MCP
// bindings.
type AgentMCPCatalogRepository interface {
	GetAgentMCPCatalog() ([]store.AgentMCPCatalogEntry, error)
	SaveAgentMCPCatalog(items []store.AgentMCPCatalogEntry) error
}

// AgentProfileRepository is the persisted backing store for editable agent
// profile documents.
type AgentProfileRepository interface {
	GetAgentProfiles() ([]store.AgentProfileRecord, error)
	SaveAgentProfiles(items []store.AgentProfileRecord) error
}

// HostRepository is the persisted backing store for host inventory.
type HostRepository interface {
	GetHost(id string) (*store.HostRecord, error)
	ListHosts() ([]store.HostRecord, error)
	SaveHost(host *store.HostRecord) error
	DeleteHost(id string) error
}

// ToolResultSpillRepository is the read-side store for externalized tool
// results.
type ToolResultSpillRepository interface {
	GetToolResultSpill(id string) (*tooling.ResultSpill, error)
	ListToolResultSpills() ([]*tooling.ResultSpill, error)
}

type AgentEventService interface {
	Append(ctx context.Context, event AgentEvent) (AgentEvent, error)
	Subscribe(ctx context.Context, sessionID string, afterSeq int64) (<-chan AgentEvent, func())
	Projection(ctx context.Context, sessionID string) (AgentEventProjection, error)
	Replay(ctx context.Context, sessionID string, afterSeq int64) ([]AgentEvent, error)
}

type HostOperationView struct {
	ID             string               `json:"id"`
	ThreadID       string               `json:"threadId,omitempty"`
	UserTurnID     string               `json:"userTurnId,omitempty"`
	ManagerAgentID string               `json:"managerAgentId,omitempty"`
	Status         string               `json:"status"`
	PlanRequired   bool                 `json:"planRequired"`
	PlanAccepted   bool                 `json:"planAccepted"`
	MentionedHosts []HostMentionView    `json:"mentionedHosts,omitempty"`
	Plan           *HostPlanView        `json:"plan,omitempty"`
	ChildAgents    []HostChildAgentView `json:"childAgents,omitempty"`
	CreatedAt      string               `json:"createdAt,omitempty"`
	UpdatedAt      string               `json:"updatedAt,omitempty"`
}

type HostMissionCreateCommand struct {
	ID             string                `json:"id,omitempty"`
	ThreadID       string                `json:"threadId,omitempty"`
	SessionID      string                `json:"sessionId,omitempty"`
	UserTurnID     string                `json:"userTurnId,omitempty"`
	ManagerAgentID string                `json:"managerAgentId,omitempty"`
	Goal           string                `json:"goal"`
	Mentions       []hostops.HostMention `json:"mentions,omitempty"`
	HostIDs        []string              `json:"hostIds,omitempty"`
}

type HostMentionView struct {
	Raw         string `json:"raw,omitempty"`
	HostID      string `json:"hostId,omitempty"`
	Address     string `json:"address,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	Source      string `json:"source,omitempty"`
	Resolved    bool   `json:"resolved"`
}

type HostPlanView struct {
	ID             string             `json:"id,omitempty"`
	Version        int                `json:"version,omitempty"`
	Status         string             `json:"status,omitempty"`
	Steps          []HostPlanStepView `json:"steps,omitempty"`
	CompletedCount int                `json:"completedCount"`
	TotalCount     int                `json:"totalCount"`
}

type HostPlanStepView struct {
	ID               string   `json:"id"`
	Index            int      `json:"index"`
	Title            string   `json:"title"`
	Summary          string   `json:"summary,omitempty"`
	Status           string   `json:"status"`
	HostIDs          []string `json:"hostIds,omitempty"`
	ChildAgentIDs    []string `json:"childAgentIds,omitempty"`
	Risk             string   `json:"risk,omitempty"`
	ApprovalRequired bool     `json:"approvalRequired"`
}

type HostChildAgentView struct {
	ID                string   `json:"id"`
	MissionID         string   `json:"missionId,omitempty"`
	ParentAgentID     string   `json:"parentAgentId,omitempty"`
	SessionID         string   `json:"sessionId,omitempty"`
	HostID            string   `json:"hostId,omitempty"`
	HostAddress       string   `json:"hostAddress,omitempty"`
	HostDisplayName   string   `json:"hostDisplayName,omitempty"`
	Role              string   `json:"role,omitempty"`
	Task              string   `json:"task,omitempty"`
	Status            string   `json:"status"`
	PlanStepIDs       []string `json:"planStepIds,omitempty"`
	CurrentStepTitle  string   `json:"currentStepTitle,omitempty"`
	LastInputPreview  string   `json:"lastInputPreview,omitempty"`
	LastOutputPreview string   `json:"lastOutputPreview,omitempty"`
	Error             string   `json:"error,omitempty"`
	StartedAt         string   `json:"startedAt,omitempty"`
	UpdatedAt         string   `json:"updatedAt,omitempty"`
	CompletedAt       string   `json:"completedAt,omitempty"`
}

type HostChildTranscriptView struct {
	ChildAgentID string                   `json:"childAgentId"`
	Items        []hostops.TranscriptItem `json:"items"`
}

type HostOpsService interface {
	CreateMission(ctx context.Context, command HostMissionCreateCommand) (HostOperationView, error)
	GetMission(ctx context.Context, missionID string) (HostOperationView, error)
	AcceptPlan(ctx context.Context, missionID, planID string) (HostOperationView, error)
	RevisePlan(ctx context.Context, missionID, instruction string) (HostOperationView, error)
	SendChildMessage(ctx context.Context, childAgentID, content string) (HostChildAgentView, error)
	StopChildAgent(ctx context.Context, childAgentID string) (HostChildAgentView, error)
	ChildTranscript(ctx context.Context, childAgentID string) (HostChildTranscriptView, error)
}

type servicesConfig struct {
	settings            SettingsRepository
	coroot              CorootConfigRepository
	hosts               HostRepository
	mcps                MCPRepository
	mcpReg              *mcp.Registry
	mcpRuntime          MCPRuntime
	auth                *auth.Manager
	terminal            *terminal.Manager
	uiCards             UICardRepository
	skills              SkillCatalogRepository
	agentMCP            AgentMCPCatalogRepository
	profiles            AgentProfileRepository
	agentEvents         AgentEventRepository
	incidents           incidents.Store
	opsManuals          OpsManualService
	opsManualRepo       opsmanual.ManualRepository
	opsgraph            OpsGraphService
	toolResultSpills    ToolResultSpillRepository
	lifecycleContext    context.Context
	credentialResolver  CredentialResolver
	sshPasswordStore    HostSSHPasswordStore
	hostAgentTokenStore HostAgentTokenStore
	hostBootstrapRunner HostBootstrapRunner
	hostAgentInstaller  HostAgentInstaller
	hostOps             HostOpsService
	terminalPolicy      TerminalPolicyService
	pluginSpecs         []plugins.Spec
}

// ServicesOption customizes first-party Web services.
type ServicesOption func(*servicesConfig)

// WithStore wires one store-backed repository into all compatible appui
// services.
func WithStore(dataStore store.Store) ServicesOption {
	return func(cfg *servicesConfig) {
		if dataStore == nil {
			return
		}
		cfg.settings = dataStore
		if repo, ok := any(dataStore).(CorootConfigRepository); ok {
			cfg.coroot = repo
		}
		cfg.hosts = dataStore
		if repo, ok := any(dataStore).(MCPRepository); ok {
			cfg.mcps = repo
		}
		if repo, ok := any(dataStore).(UICardRepository); ok {
			cfg.uiCards = repo
		}
		if repo, ok := any(dataStore).(SkillCatalogRepository); ok {
			cfg.skills = repo
		}
		if repo, ok := any(dataStore).(AgentMCPCatalogRepository); ok {
			cfg.agentMCP = repo
		}
		if repo, ok := any(dataStore).(AgentProfileRepository); ok {
			cfg.profiles = repo
		}
		if repo, ok := any(dataStore).(AgentEventRepository); ok {
			cfg.agentEvents = repo
		}
		if repo, ok := any(dataStore).(incidents.Store); ok {
			cfg.incidents = repo
		}
		if repo, ok := any(dataStore).(opsmanual.ManualRepository); ok {
			cfg.opsManualRepo = repo
		}
		if repo, ok := any(dataStore).(ToolResultSpillRepository); ok {
			cfg.toolResultSpills = repo
		}
	}
}

// WithSettingsRepository overrides the settings repository.
func WithSettingsRepository(repo SettingsRepository) ServicesOption {
	return func(cfg *servicesConfig) {
		cfg.settings = repo
	}
}

func WithCorootConfigRepository(repo CorootConfigRepository) ServicesOption {
	return func(cfg *servicesConfig) {
		cfg.coroot = repo
	}
}

// WithHostRepository overrides the host repository.
func WithHostRepository(repo HostRepository) ServicesOption {
	return func(cfg *servicesConfig) {
		cfg.hosts = repo
	}
}

// WithMCPRepository overrides the MCP repository.
func WithMCPRepository(repo MCPRepository) ServicesOption {
	return func(cfg *servicesConfig) {
		cfg.mcps = repo
	}
}

// WithMCPRegistry overrides the live MCP registry used by runtime actions.
func WithMCPRegistry(registry *mcp.Registry) ServicesOption {
	return func(cfg *servicesConfig) {
		cfg.mcpReg = registry
	}
}

// WithMCPRuntime connects the MCP app service to the live runtime connector.
func WithMCPRuntime(runtime MCPRuntime) ServicesOption {
	return func(cfg *servicesConfig) {
		cfg.mcpRuntime = runtime
	}
}

// WithAuthManager overrides the auth domain manager.
func WithAuthManager(manager *auth.Manager) ServicesOption {
	return func(cfg *servicesConfig) {
		cfg.auth = manager
	}
}

// WithTerminalManager overrides the terminal domain manager.
func WithTerminalManager(manager *terminal.Manager) ServicesOption {
	return func(cfg *servicesConfig) {
		cfg.terminal = manager
	}
}

// WithSkillCatalogRepository overrides the skill catalog repository.
func WithSkillCatalogRepository(repo SkillCatalogRepository) ServicesOption {
	return func(cfg *servicesConfig) {
		cfg.skills = repo
	}
}

// WithAgentMCPCatalogRepository overrides the agent MCP catalog repository.
func WithAgentMCPCatalogRepository(repo AgentMCPCatalogRepository) ServicesOption {
	return func(cfg *servicesConfig) {
		cfg.agentMCP = repo
	}
}

// WithAgentProfileRepository overrides the agent profile repository.
func WithAgentProfileRepository(repo AgentProfileRepository) ServicesOption {
	return func(cfg *servicesConfig) {
		cfg.profiles = repo
	}
}

// WithLifecycleContext sets the application lifecycle context used by
// background services that must outlive individual HTTP request contexts.
func WithLifecycleContext(ctx context.Context) ServicesOption {
	return func(cfg *servicesConfig) {
		cfg.lifecycleContext = ctx
	}
}

func WithCredentialResolver(resolver CredentialResolver) ServicesOption {
	return func(cfg *servicesConfig) {
		cfg.credentialResolver = resolver
	}
}

func WithHostSSHPasswordStore(store HostSSHPasswordStore) ServicesOption {
	return func(cfg *servicesConfig) {
		cfg.sshPasswordStore = store
	}
}

func WithHostAgentTokenStore(store HostAgentTokenStore) ServicesOption {
	return func(cfg *servicesConfig) {
		cfg.hostAgentTokenStore = store
	}
}

func WithHostBootstrapRunner(runner HostBootstrapRunner) ServicesOption {
	return func(cfg *servicesConfig) {
		cfg.hostBootstrapRunner = runner
	}
}

func WithDirectHostAgentInstaller(installer HostAgentInstaller) ServicesOption {
	return func(cfg *servicesConfig) {
		cfg.hostAgentInstaller = installer
	}
}

func WithHostOpsService(service HostOpsService) ServicesOption {
	return func(cfg *servicesConfig) {
		cfg.hostOps = service
	}
}

func WithTerminalPolicyService(service TerminalPolicyService) ServicesOption {
	return func(cfg *servicesConfig) {
		cfg.terminalPolicy = service
	}
}

func WithOpsManualService(service OpsManualService) ServicesOption {
	return func(cfg *servicesConfig) {
		cfg.opsManuals = service
	}
}

func WithOpsGraphService(service OpsGraphService) ServicesOption {
	return func(cfg *servicesConfig) {
		cfg.opsgraph = service
	}
}

func WithPluginSpecs(specs []plugins.Spec) ServicesOption {
	return func(cfg *servicesConfig) {
		cfg.pluginSpecs = append([]plugins.Spec(nil), specs...)
	}
}

// HTTPServices is the interface consumed by internal/server handlers.
type HTTPServices interface {
	ChatService() ChatService
	StateService() StateService
	SessionService() SessionService
	ApprovalService() ApprovalService
	ChoiceService() ChoiceService
	SettingsService() SettingsService
	HostService() HostService
	MCPService() MCPService
	AgentProfileService() AgentProfileService
	AuthService() AuthService
	TerminalService() TerminalService
}

// Services is the default first-party Web application service set.
type Services struct {
	chat           ChatService
	state          StateService
	sessions       SessionService
	sessionSource  SessionSource
	approvals      ApprovalService
	choices        ChoiceService
	settings       SettingsService
	hosts          HostService
	hostAgents     HostAgentService
	mcps           MCPService
	profiles       AgentProfileService
	auth           AuthService
	terminal       TerminalService
	uiCards        UICardService
	coroot         CorootConfigRepository
	agentEvents    AgentEventService
	incidents      IncidentService
	chatArchive    ChatArchiveService
	postmortems    PostmortemService
	corootWebhooks CorootWebhookService
	runbooks       RunbookService
	opsgraph       OpsGraphService
	erp            ERPContextService
	changes        ChangeContextService
	opsManuals     OpsManualService
	toolSpills     ToolResultSpillRepository
	hostOps        HostOpsService
	terminalPolicy TerminalPolicyService
	capabilities   CapabilityService
}

// NewServices wires the default appui services over the runtime and session
// sources.
func NewServices(runtime RuntimeGateway, sessions SessionSource, opts ...ServicesOption) *Services {
	cfg := servicesConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	builder := NewSnapshotBuilderWithSettings(cfg.hosts, cfg.settings)
	var sessionStore SessionStore
	if cast, ok := sessions.(SessionStore); ok {
		sessionStore = cast
	}
	registry := cfg.mcpReg
	if registry == nil {
		registry = mcp.DefaultRegistry()
	}
	settingsService := NewSettingsService(cfg.settings, cfg.auth)
	authService := NewAuthService(cfg.auth)
	agentEvents := NewAgentEventService(cfg.agentEvents)
	incidentService := NewIncidentService(incidents.NewService(cfg.incidents, nil))
	chatArchiveService := NewChatArchiveService(sessions, incidentService)
	opsManualService := cfg.opsManuals
	if opsManualService == nil {
		repo := cfg.opsManualRepo
		if repo == nil {
			repo = opsmanual.NewMemoryStore()
		}
		opsManualService = NewOpsManualService(opsmanual.NewService(
			repo,
			opsmanual.WithResourceDiscovery(opsmanual.NewLocalResourceDiscovery()),
			opsmanual.WithSessionOpsContextStore(opsmanual.NewMemorySessionOpsContextStore()),
		))
	}
	var hostBootstrap *HostBootstrapService
	hostAgentInstaller := cfg.hostAgentInstaller
	if hostAgentInstaller == nil && cfg.hosts != nil && cfg.credentialResolver != nil {
		tokenStore := cfg.hostAgentTokenStore
		if tokenStore == nil {
			tokenStore = NewLocalHostAgentTokenStore(defaultHostInstallSecretDir())
		}
		hostAgentInstaller = NewDirectHostAgentInstaller(cfg.hosts, cfg.credentialResolver, WithDirectHostAgentTokenStore(tokenStore))
	}
	if cfg.hostBootstrapRunner != nil || hostAgentInstaller != nil {
		hostBootstrap = NewHostBootstrapService(cfg.hosts, cfg.hostBootstrapRunner, WithHostAgentInstaller(hostAgentInstaller))
	}
	sshPasswordStore := cfg.sshPasswordStore
	if sshPasswordStore == nil {
		sshPasswordStore = NewLocalHostSSHPasswordStore(defaultHostInstallSecretDir())
	}
	var uiCards UICardService
	if cfg.uiCards != nil {
		uiCards = NewUICardService(cfg.uiCards, WithUICardPluginSpecs(cfg.pluginSpecs))
	}
	hostOpsService := cfg.hostOps
	chatService := NewChatServiceWithContextHostsAndHostOps(cfg.lifecycleContext, runtime, sessions, cfg.hosts, hostOpsService, agentEvents)
	return &Services{
		chat:           chatService,
		state:          NewStateService(sessions, builder),
		sessions:       NewSessionService(sessions, sessionStore, builder),
		sessionSource:  sessions,
		approvals:      NewApprovalServiceWithContext(cfg.lifecycleContext, runtime, sessions, builder),
		choices:        NewChoiceService(runtime, sessions),
		settings:       settingsService,
		hosts:          NewHostServiceWithOptions(sessionStore, cfg.hosts, builder, hostBootstrap, WithHostServiceSSHPasswordStore(sshPasswordStore)),
		hostAgents:     NewHostAgentService(cfg.hosts),
		mcps:           NewMCPServiceWithRuntime(cfg.mcps, registry, cfg.mcpRuntime),
		profiles:       NewAgentProfileService(newAgentProfileRepositories(cfg.skills, cfg.agentMCP, cfg.profiles), WithAgentProfilePluginSpecs(cfg.pluginSpecs)),
		auth:           authService,
		terminal:       NewTerminalServiceWithCredentialResolver(cfg.terminal, cfg.credentialResolver, cfg.hosts),
		uiCards:        uiCards,
		coroot:         cfg.coroot,
		agentEvents:    agentEvents,
		incidents:      incidentService,
		chatArchive:    chatArchiveService,
		postmortems:    NewPostmortemService(incidentService),
		corootWebhooks: NewCorootWebhookService(incidentService, chatService),
		runbooks:       NewRunbookService("", nil),
		opsgraph:       firstNonNilOpsGraphService(cfg.opsgraph, NewOpsGraphService("")),
		erp:            NewERPContextService(),
		changes:        NewChangeContextService(),
		opsManuals:     opsManualService,
		toolSpills:     cfg.toolResultSpills,
		hostOps:        hostOpsService,
		terminalPolicy: cfg.terminalPolicy,
		capabilities:   NewCapabilityService(cfg.skills, cfg.agentMCP, cfg.pluginSpecs),
	}
}

func (s *Services) ChatService() ChatService         { return s.chat }
func (s *Services) StateService() StateService       { return s.state }
func (s *Services) SessionService() SessionService   { return s.sessions }
func (s *Services) SessionSource() SessionSource     { return s.sessionSource }
func (s *Services) ApprovalService() ApprovalService { return s.approvals }
func (s *Services) ChoiceService() ChoiceService     { return s.choices }
func (s *Services) SettingsService() SettingsService { return s.settings }
func (s *Services) HostService() HostService         { return s.hosts }
func (s *Services) HostAgentService() HostAgentService {
	return s.hostAgents
}
func (s *Services) MCPService() MCPService { return s.mcps }
func (s *Services) AgentProfileService() AgentProfileService {
	return s.profiles
}
func (s *Services) AuthService() AuthService         { return s.auth }
func (s *Services) TerminalService() TerminalService { return s.terminal }
func (s *Services) UICardService() UICardService     { return s.uiCards }
func (s *Services) CorootConfigRepository() CorootConfigRepository {
	return s.coroot
}
func (s *Services) AgentEventService() AgentEventService {
	return s.agentEvents
}
func (s *Services) IncidentService() IncidentService           { return s.incidents }
func (s *Services) ChatArchiveService() ChatArchiveService     { return s.chatArchive }
func (s *Services) PostmortemService() PostmortemService       { return s.postmortems }
func (s *Services) CorootWebhookService() CorootWebhookService { return s.corootWebhooks }
func (s *Services) RunbookService() RunbookService             { return s.runbooks }
func (s *Services) OpsGraphService() OpsGraphService           { return s.opsgraph }
func (s *Services) ERPContextService() ERPContextService       { return s.erp }
func (s *Services) ChangeContextService() ChangeContextService { return s.changes }
func (s *Services) OpsManualService() OpsManualService         { return s.opsManuals }
func (s *Services) ToolResultSpillRepository() ToolResultSpillRepository {
	return s.toolSpills
}
func (s *Services) HostOpsService() HostOpsService { return s.hostOps }
func (s *Services) TerminalPolicyService() TerminalPolicyService {
	return s.terminalPolicy
}
func (s *Services) CapabilityService() CapabilityService { return s.capabilities }

func firstNonNilOpsGraphService(primary, fallback OpsGraphService) OpsGraphService {
	if primary != nil {
		return primary
	}
	return fallback
}

type ChatCommand struct {
	SessionID       string
	SessionType     string
	Mode            string
	Content         string
	Role            string
	HostID          string
	ClientMessageID string
	ClientTurnID    string
	Metadata        map[string]string
}

type ResumeCommand struct {
	SessionID    string            `json:"sessionId"`
	TurnID       string            `json:"turnId"`
	ApprovalID   string            `json:"approvalId,omitempty"`
	CheckpointID string            `json:"checkpointId,omitempty"`
	ResumeState  string            `json:"resumeState,omitempty"`
	Decision     string            `json:"decision,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

type CancelCommand struct {
	SessionID string `json:"sessionId"`
	TurnID    string `json:"turnId"`
	Reason    string `json:"reason,omitempty"`
}

type StopCommand struct {
	SessionID string `json:"sessionId,omitempty"`
	TurnID    string `json:"turnId,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

type TurnResponse struct {
	SessionID       string            `json:"sessionId"`
	TurnID          string            `json:"turnId"`
	ClientTurnID    string            `json:"clientTurnId,omitempty"`
	ClientMessageID string            `json:"clientMessageId,omitempty"`
	Status          string            `json:"status"`
	Output          string            `json:"output,omitempty"`
	Error           string            `json:"error,omitempty"`
	OpsRun          *ChatRunTraceView `json:"opsRun,omitempty"`
}

type TurnEventType string

const (
	TurnEventStarted              TurnEventType = "turn.started"
	TurnEventAssistantIntentDelta TurnEventType = "assistant.intent.delta"
	TurnEventAssistantFinalDelta  TurnEventType = "assistant.final.delta"
	TurnEventToolCallStart        TurnEventType = "tool.call.start"
	TurnEventToolStatusDelta      TurnEventType = "tool.status.delta"
	TurnEventToolResultDone       TurnEventType = "tool.result.done"
	TurnEventToolResultError      TurnEventType = "tool.result.error"
	TurnEventPhaseEnd             TurnEventType = "phase.end"
	TurnEventProcessSummary       TurnEventType = "process.summary"
	TurnEventApprovalRequired     TurnEventType = "approval.required"
	TurnEventDone                 TurnEventType = "turn.done"
	TurnEventError                TurnEventType = "turn.error"
	TurnEventAborted              TurnEventType = "turn.aborted"
)

type TurnEvent struct {
	Type      TurnEventType  `json:"type"`
	SessionID string         `json:"sessionId"`
	TurnID    string         `json:"turnId"`
	EventID   string         `json:"eventId"`
	Seq       int64          `json:"seq"`
	CreatedAt string         `json:"createdAt"`
	Payload   map[string]any `json:"payload,omitempty"`
}

type ProcessItemPayload struct {
	ToolCallID string `json:"toolCallId,omitempty"`
	ToolName   string `json:"toolName,omitempty"`
	Title      string `json:"title,omitempty"`
	Detail     string `json:"detail,omitempty"`
	Status     string `json:"status,omitempty"`
	Result     string `json:"result,omitempty"`
	Error      string `json:"error,omitempty"`
}

type PhaseSummaryPayload struct {
	PhaseID string `json:"phaseId,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type StateSnapshot struct {
	SessionID            string                `json:"sessionId,omitempty"`
	Kind                 string                `json:"kind"`
	SelectedHostID       string                `json:"selectedHostId"`
	LastActivityAt       string                `json:"lastActivityAt,omitempty"`
	Auth                 AuthSummary           `json:"auth"`
	Hosts                []HostSummary         `json:"hosts"`
	Cards                []CardView            `json:"cards"`
	Approvals            []ApprovalView        `json:"approvals"`
	ToolInvocations      []ToolInvocationView  `json:"toolInvocations,omitempty"`
	EvidenceSummaries    []EvidenceSummaryView `json:"evidenceSummaries,omitempty"`
	CurrentMode          string                `json:"currentMode,omitempty"`
	CurrentStage         string                `json:"currentStage,omitempty"`
	CurrentLane          string                `json:"currentLane,omitempty"`
	RequiredNextTool     string                `json:"requiredNextTool,omitempty"`
	FinalGateStatus      string                `json:"finalGateStatus,omitempty"`
	MissingRequirements  []string              `json:"missingRequirements,omitempty"`
	TurnPolicy           TurnPolicyView        `json:"turnPolicy"`
	PromptEnvelope       PromptEnvelopeView    `json:"promptEnvelope"`
	AgentEventProjection *AgentEventProjection `json:"agentEventProjection,omitempty"`
	Config               map[string]any        `json:"config"`
	Runtime              RuntimeSnapshot       `json:"runtime"`
}

type AuthSummary struct {
	Connected bool `json:"connected"`
}

type HostSummary struct {
	ID                  string            `json:"id"`
	Name                string            `json:"name"`
	Status              string            `json:"status"`
	AgentStatus         string            `json:"agentStatus,omitempty"`
	SSHStatus           string            `json:"sshStatus,omitempty"`
	RuntimeReachability string            `json:"runtimeReachability,omitempty"`
	Kind                string            `json:"kind,omitempty"`
	Address             string            `json:"address,omitempty"`
	Transport           string            `json:"transport,omitempty"`
	Executable          bool              `json:"executable,omitempty"`
	TerminalCapable     bool              `json:"terminalCapable,omitempty"`
	OS                  string            `json:"os,omitempty"`
	Arch                string            `json:"arch,omitempty"`
	OSRelease           string            `json:"osRelease,omitempty"`
	KernelVersion       string            `json:"kernelVersion,omitempty"`
	CPUCores            int               `json:"cpuCores,omitempty"`
	MemoryBytes         uint64            `json:"memoryBytes,omitempty"`
	AgentVersion        string            `json:"agentVersion,omitempty"`
	LastHeartbeat       string            `json:"lastHeartbeat,omitempty"`
	Labels              map[string]string `json:"labels,omitempty"`
	LastError           string            `json:"lastError,omitempty"`
	SSHUser             string            `json:"sshUser,omitempty"`
	SSHPort             int               `json:"sshPort,omitempty"`
	SSHCredentialRef    string            `json:"sshCredentialRef,omitempty"`
	AgentURL            string            `json:"agentUrl,omitempty"`
	AgentTokenRef       string            `json:"agentTokenRef,omitempty"`
	InstallState        string            `json:"installState,omitempty"`
	InstallRunID        string            `json:"installRunId,omitempty"`
	InstallWorkflowID   string            `json:"installWorkflowId,omitempty"`
	InstallStep         string            `json:"installStep,omitempty"`
	ControlMode         string            `json:"controlMode,omitempty"`
}

type CardView struct {
	ID              string `json:"id"`
	ClientMessageID string `json:"clientMessageId,omitempty"`
	ClientTurnID    string `json:"clientTurnId,omitempty"`
	Type            string `json:"type"`
	Role            string `json:"role,omitempty"`
	Text            string `json:"text,omitempty"`
	Message         string `json:"message,omitempty"`
	Summary         string `json:"summary,omitempty"`
	Timestamp       string `json:"timestamp,omitempty"`
}

type ApprovalView struct {
	ID             string `json:"id"`
	SessionID      string `json:"sessionId,omitempty"`
	TurnID         string `json:"turnId,omitempty"`
	MissionID      string `json:"missionId,omitempty"`
	ChildAgentID   string `json:"childAgentId,omitempty"`
	PlanStepID     string `json:"planStepId,omitempty"`
	GroupID        string `json:"groupId,omitempty"`
	GroupSize      int    `json:"groupSize,omitempty"`
	ToolName       string `json:"toolName,omitempty"`
	Command        string `json:"command,omitempty"`
	Reason         string `json:"reason,omitempty"`
	Risk           string `json:"risk,omitempty"`
	Source         string `json:"source,omitempty"`
	RunbookID      string `json:"runbookId,omitempty"`
	RunbookStep    string `json:"runbookStep,omitempty"`
	ExpectedEffect string `json:"expectedEffect,omitempty"`
	Rollback       string `json:"rollback,omitempty"`
	Validation     string `json:"validation,omitempty"`
	HostID         string `json:"hostId,omitempty"`
	Status         string `json:"status"`
	CreatedAt      string `json:"createdAt,omitempty"`
}

type ToolInvocationView struct {
	ID            string `json:"id"`
	Name          string `json:"name,omitempty"`
	DisplayName   string `json:"displayName,omitempty"`
	Kind          string `json:"kind,omitempty"`
	Status        string `json:"status,omitempty"`
	InputJSON     string `json:"inputJson,omitempty"`
	OutputJSON    string `json:"outputJson,omitempty"`
	InputSummary  string `json:"inputSummary,omitempty"`
	OutputSummary string `json:"outputSummary,omitempty"`
	HostID        string `json:"hostId,omitempty"`
	ApprovalID    string `json:"approvalId,omitempty"`
	EvidenceID    string `json:"evidenceId,omitempty"`
	StartedAt     string `json:"startedAt,omitempty"`
	CompletedAt   string `json:"completedAt,omitempty"`
}

type EvidenceSummaryView struct {
	ID                 string         `json:"id"`
	CitationKey        string         `json:"citationKey,omitempty"`
	InvocationID       string         `json:"invocationId,omitempty"`
	RelatedEvidenceIDs []string       `json:"relatedEvidenceIds,omitempty"`
	SourceKind         string         `json:"sourceKind,omitempty"`
	SourceRef          string         `json:"sourceRef,omitempty"`
	Kind               string         `json:"kind,omitempty"`
	Title              string         `json:"title,omitempty"`
	Summary            string         `json:"summary,omitempty"`
	Content            any            `json:"content,omitempty"`
	HostID             string         `json:"hostId,omitempty"`
	HostName           string         `json:"hostName,omitempty"`
	Metadata           map[string]any `json:"metadata,omitempty"`
	CreatedAt          string         `json:"createdAt,omitempty"`
}

type TurnPolicyView struct {
	IntentClass           string   `json:"intentClass,omitempty"`
	Lane                  string   `json:"lane,omitempty"`
	RequiredTools         []string `json:"requiredTools,omitempty"`
	RequiredEvidenceKinds []string `json:"requiredEvidenceKinds,omitempty"`
	NeedsPlanArtifact     bool     `json:"needsPlanArtifact,omitempty"`
	NeedsApproval         bool     `json:"needsApproval,omitempty"`
	NeedsAssumptions      bool     `json:"needsAssumptions,omitempty"`
	NeedsDisambiguation   bool     `json:"needsDisambiguation,omitempty"`
	RequiresExternalFacts bool     `json:"requiresExternalFacts,omitempty"`
	RequiresRealtimeData  bool     `json:"requiresRealtimeData,omitempty"`
	MinimumEvidenceCount  int      `json:"minimumEvidenceCount,omitempty"`
	RequiredNextTool      string   `json:"requiredNextTool,omitempty"`
	FinalGateStatus       string   `json:"finalGateStatus,omitempty"`
	MissingRequirements   []string `json:"missingRequirements,omitempty"`
	ClassificationReason  string   `json:"classificationReason,omitempty"`
	UpdatedAt             string   `json:"updatedAt,omitempty"`
}

type PromptEnvelopeSectionView struct {
	Name    string `json:"name,omitempty"`
	Content string `json:"content,omitempty"`
}

type PromptEnvelopeToolView struct {
	Name        string   `json:"name,omitempty"`
	DisplayName string   `json:"displayName,omitempty"`
	Kind        string   `json:"kind,omitempty"`
	Description string   `json:"description,omitempty"`
	Aliases     []string `json:"aliases,omitempty"`
	Reason      string   `json:"reason,omitempty"`
}

type PromptEnvelopeView struct {
	StaticSections      []PromptEnvelopeSectionView `json:"staticSections,omitempty"`
	LaneSections        []PromptEnvelopeSectionView `json:"laneSections,omitempty"`
	RuntimePolicy       PromptEnvelopeSectionView   `json:"runtimePolicy"`
	ContextAttachments  []PromptEnvelopeSectionView `json:"contextAttachments,omitempty"`
	VisibleTools        []PromptEnvelopeToolView    `json:"visibleTools,omitempty"`
	HiddenTools         []PromptEnvelopeToolView    `json:"hiddenTools,omitempty"`
	TokenEstimate       int                         `json:"tokenEstimate,omitempty"`
	CompressionState    string                      `json:"compressionState,omitempty"`
	CurrentLane         string                      `json:"currentLane,omitempty"`
	IntentClass         string                      `json:"intentClass,omitempty"`
	FinalGateStatus     string                      `json:"finalGateStatus,omitempty"`
	MissingRequirements []string                    `json:"missingRequirements,omitempty"`
	UpdatedAt           string                      `json:"updatedAt,omitempty"`
}

type RuntimeSnapshot struct {
	Turn     RuntimeTurnSnapshot `json:"turn"`
	Codex    map[string]any      `json:"codex"`
	Activity map[string]any      `json:"activity"`
}

type RuntimeTurnSnapshot struct {
	Active          bool   `json:"active"`
	Phase           string `json:"phase"`
	HostID          string `json:"hostId"`
	ClientTurnID    string `json:"clientTurnId,omitempty"`
	ClientMessageID string `json:"clientMessageId,omitempty"`
}

type SessionSummary struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	Title          string `json:"title"`
	Preview        string `json:"preview"`
	SelectedHostID string `json:"selectedHostId"`
	Status         string `json:"status"`
	MessageCount   int    `json:"messageCount"`
	LastActivityAt string `json:"lastActivityAt,omitempty"`
}

type SessionListResponse struct {
	ActiveSessionID string           `json:"activeSessionId,omitempty"`
	Sessions        []SessionSummary `json:"sessions"`
}

type SessionMutationResponse struct {
	ActiveSessionID string           `json:"activeSessionId,omitempty"`
	Sessions        []SessionSummary `json:"sessions"`
	Snapshot        StateSnapshot    `json:"snapshot"`
}

type ApprovalDecision struct {
	SessionID string
	TurnID    string
	ID        string
	Decision  string
}

type ChoiceAnswer struct {
	RequestID string
	Answers   []any
}

type WebSettingsPayload struct {
	Quota           string                     `json:"quota,omitempty"`
	Model           string                     `json:"model,omitempty"`
	ReasoningEffort string                     `json:"reasoningEffort,omitempty"`
	Models          []store.SettingModelOption `json:"models,omitempty"`
}

type LLMConfigView struct {
	Provider         string   `json:"provider,omitempty"`
	Model            string   `json:"model,omitempty"`
	BaseURL          string   `json:"baseURL,omitempty"`
	MaxContextTokens int      `json:"maxContextTokens,omitempty"`
	MaxOutputTokens  int      `json:"maxOutputTokens,omitempty"`
	Temperature      *float64 `json:"temperature,omitempty"`
	TopP             *float64 `json:"topP,omitempty"`
	ThinkingType     string   `json:"thinkingType,omitempty"`
	ReasoningEffort  string   `json:"reasoningEffort,omitempty"`
	ToolStream       bool     `json:"toolStream,omitempty"`
	FallbackProvider string   `json:"fallbackProvider,omitempty"`
	FallbackModel    string   `json:"fallbackModel,omitempty"`
	CompactModel     string   `json:"compactModel,omitempty"`
	BifrostActive    bool     `json:"bifrostActive"`
	APIKeySet        bool     `json:"apiKeySet"`
	APIKeyMasked     string   `json:"apiKeyMasked,omitempty"`
}

type LLMConfigUpdate struct {
	Provider         string   `json:"provider,omitempty"`
	Model            string   `json:"model,omitempty"`
	APIKey           string   `json:"apiKey,omitempty"`
	BaseURL          string   `json:"baseURL,omitempty"`
	MaxContextTokens int      `json:"maxContextTokens,omitempty"`
	MaxOutputTokens  int      `json:"maxOutputTokens,omitempty"`
	Temperature      *float64 `json:"temperature,omitempty"`
	TopP             *float64 `json:"topP,omitempty"`
	ThinkingType     string   `json:"thinkingType,omitempty"`
	ReasoningEffort  string   `json:"reasoningEffort,omitempty"`
	ToolStream       bool     `json:"toolStream,omitempty"`
	FallbackProvider string   `json:"fallbackProvider,omitempty"`
	FallbackModel    string   `json:"fallbackModel,omitempty"`
	FallbackAPIKey   string   `json:"fallbackApiKey,omitempty"`
	CompactModel     string   `json:"compactModel,omitempty"`
}

type LLMConfigUpdateResult struct {
	OK               bool   `json:"ok"`
	Message          string `json:"message,omitempty"`
	Error            string `json:"error,omitempty"`
	MaxContextTokens int    `json:"maxContextTokens,omitempty"`
	MaxOutputTokens  int    `json:"maxOutputTokens,omitempty"`
}

type HostUpsert struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Address          string            `json:"address"`
	SSHUser          string            `json:"sshUser"`
	SSHPort          int               `json:"sshPort"`
	SSHCredentialRef string            `json:"sshCredentialRef"`
	SSHPassword      string            `json:"sshPassword,omitempty"`
	AgentVersion     string            `json:"agentVersion"`
	Labels           map[string]string `json:"labels"`
	InstallViaSSH    bool              `json:"installViaSsh"`
}

type HostInstallRequest struct {
	AgentVersion     string `json:"agentVersion"`
	SSHCredentialRef string `json:"sshCredentialRef"`
	SSHPassword      string `json:"sshPassword,omitempty"`
	Force            bool   `json:"force"`
}

type HostSSHTestRequest struct {
	SSHCredentialRef string `json:"sshCredentialRef"`
	SSHPassword      string `json:"sshPassword,omitempty"`
}

type HostSSHTestResponse struct {
	Status   string `json:"status"`
	Platform string `json:"platform,omitempty"`
	OS       string `json:"os,omitempty"`
	Arch     string `json:"arch,omitempty"`
	Sudo     string `json:"sudo,omitempty"`
	Message  string `json:"message,omitempty"`
}

type HostAgentRegisterRequest struct {
	HostID        string            `json:"hostId"`
	Hostname      string            `json:"hostname,omitempty"`
	OS            string            `json:"os"`
	Arch          string            `json:"arch"`
	OSRelease     string            `json:"osRelease,omitempty"`
	KernelVersion string            `json:"kernelVersion,omitempty"`
	CPUCores      int               `json:"cpuCores,omitempty"`
	MemoryBytes   uint64            `json:"memoryBytes,omitempty"`
	AgentVersion  string            `json:"agentVersion"`
	Capabilities  []string          `json:"capabilities,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	ListenAddress string            `json:"listenAddress,omitempty"`
}

type HostAgentRegisterResponse struct {
	Status        string      `json:"status"`
	HostID        string      `json:"hostId"`
	AgentURL      string      `json:"agentUrl,omitempty"`
	AgentVersion  string      `json:"agentVersion,omitempty"`
	LastHeartbeat string      `json:"lastHeartbeat,omitempty"`
	Host          HostSummary `json:"host"`
}

type HostAgentHeartbeatRequest struct {
	HostID        string   `json:"hostId"`
	AgentVersion  string   `json:"agentVersion,omitempty"`
	Timestamp     string   `json:"timestamp,omitempty"`
	Capabilities  []string `json:"capabilities,omitempty"`
	OSRelease     string   `json:"osRelease,omitempty"`
	KernelVersion string   `json:"kernelVersion,omitempty"`
	CPUCores      int      `json:"cpuCores,omitempty"`
	MemoryBytes   uint64   `json:"memoryBytes,omitempty"`
}

type HostAgentHeartbeatResponse struct {
	Status        string      `json:"status"`
	HostID        string      `json:"hostId"`
	LastHeartbeat string      `json:"lastHeartbeat"`
	Host          HostSummary `json:"host"`
}

type HostMutationResponse struct {
	Host              HostSummary   `json:"host"`
	Items             []HostSummary `json:"items,omitempty"`
	InstallRunID      string        `json:"installRunId,omitempty"`
	InstallWorkflowID string        `json:"installWorkflowId,omitempty"`
}

type HostInstallRun struct {
	HostID       string `json:"hostId,omitempty"`
	RunID        string `json:"runId,omitempty"`
	WorkflowID   string `json:"workflowId,omitempty"`
	Status       string `json:"status,omitempty"`
	CurrentStep  string `json:"currentStep,omitempty"`
	LastError    string `json:"lastError,omitempty"`
	Platform     string `json:"platform,omitempty"`
	AgentVersion string `json:"agentVersion,omitempty"`
}

type MCPServerUpsert struct {
	Name      string            `json:"name"`
	Transport string            `json:"transport,omitempty"`
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	URL       string            `json:"url,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Disabled  bool              `json:"disabled,omitempty"`
}

type MCPServerView struct {
	Name          string             `json:"name"`
	Transport     string             `json:"transport,omitempty"`
	Command       string             `json:"command,omitempty"`
	Args          []string           `json:"args,omitempty"`
	URL           string             `json:"url,omitempty"`
	Env           map[string]string  `json:"env,omitempty"`
	Disabled      bool               `json:"disabled,omitempty"`
	Status        string             `json:"status,omitempty"`
	Error         string             `json:"error,omitempty"`
	ToolCount     int                `json:"toolCount,omitempty"`
	ResourceCount int                `json:"resourceCount,omitempty"`
	Health        mcp.HealthSnapshot `json:"health,omitempty"`
}

type MCPServersPayload struct {
	ConfigPath string          `json:"configPath,omitempty"`
	Items      []MCPServerView `json:"items"`
}

type MCPHealthPayload struct {
	Items []MCPHealthView `json:"items"`
}

type MCPHealthView struct {
	ServerID           string `json:"serverId"`
	DisplayName        string `json:"displayName,omitempty"`
	Status             string `json:"status"`
	LastCheckedAt      string `json:"lastCheckedAt,omitempty"`
	LastError          string `json:"lastError,omitempty"`
	AvailableToolCount int    `json:"availableToolCount,omitempty"`
	DisabledReason     string `json:"disabledReason,omitempty"`
	RetryAfterSeconds  int    `json:"retryAfterSeconds,omitempty"`
}

type SkillCatalogItem struct {
	ID                    string   `json:"id"`
	Name                  string   `json:"name"`
	Description           string   `json:"description,omitempty"`
	Source                string   `json:"source,omitempty"`
	SourceScope           string   `json:"sourceScope,omitempty"`
	Enabled               bool     `json:"enabled,omitempty"`
	DefaultEnabled        bool     `json:"defaultEnabled,omitempty"`
	ActivationMode        string   `json:"activationMode,omitempty"`
	DefaultActivationMode string   `json:"defaultActivationMode,omitempty"`
	InvocationMode        string   `json:"invocationMode,omitempty"`
	Risk                  string   `json:"risk,omitempty"`
	AllowedTools          []string `json:"allowedTools,omitempty"`
	DeniedTools           []string `json:"deniedTools,omitempty"`
	ResourceTypes         []string `json:"resourceTypes,omitempty"`
	TaskIntents           []string `json:"taskIntents,omitempty"`
	Paths                 []string `json:"paths,omitempty"`
	Modes                 []string `json:"modes,omitempty"`
	UserInvocable         bool     `json:"userInvocable,omitempty"`
	ModelInvocable        bool     `json:"modelInvocable,omitempty"`
}

type SkillCatalogPayload struct {
	Item  SkillCatalogItem   `json:"item"`
	Items []SkillCatalogItem `json:"items"`
}

type McpCatalogItem struct {
	ID                           string `json:"id"`
	Name                         string `json:"name"`
	Type                         string `json:"type,omitempty"`
	Source                       string `json:"source,omitempty"`
	SourceScope                  string `json:"sourceScope,omitempty"`
	Enabled                      bool   `json:"enabled,omitempty"`
	DefaultEnabled               bool   `json:"defaultEnabled,omitempty"`
	Permission                   string `json:"permission,omitempty"`
	ApprovalStatus               string `json:"approvalStatus,omitempty"`
	RuntimeStatus                string `json:"runtimeStatus,omitempty"`
	Risk                         string `json:"risk,omitempty"`
	RequiresExplicitUserApproval bool   `json:"requiresExplicitUserApproval,omitempty"`
}

type McpCatalogPayload struct {
	Item  McpCatalogItem   `json:"item"`
	Items []McpCatalogItem `json:"items"`
}

type AgentProfilesList struct {
	Items        []store.AgentProfileRecord `json:"items"`
	SkillCatalog []SkillCatalogItem         `json:"skillCatalog,omitempty"`
	McpCatalog   []McpCatalogItem           `json:"mcpCatalog,omitempty"`
}

type AgentProfilesExportPayload struct {
	Version       int                        `json:"version"`
	ConfigVersion int                        `json:"configVersion"`
	ExportedAt    string                     `json:"exportedAt,omitempty"`
	ExportedBy    string                     `json:"exportedBy,omitempty"`
	Count         int                        `json:"count"`
	Profiles      []store.AgentProfileRecord `json:"profiles"`
}

type AgentProfilesImportPayload struct {
	Profiles []store.AgentProfileRecord `json:"profiles"`
}

type AgentProfilePreview struct {
	ProfileID          string             `json:"profileId"`
	ProfileType        string             `json:"profileType,omitempty"`
	SystemPrompt       string             `json:"systemPrompt,omitempty"`
	SystemPromptLines  int                `json:"systemPromptLines"`
	CommandSummary     []string           `json:"commandSummary,omitempty"`
	CapabilitySummary  []string           `json:"capabilitySummary,omitempty"`
	EnabledSkills      []map[string]any   `json:"enabledSkills,omitempty"`
	EnabledMcps        []map[string]any   `json:"enabledMcps,omitempty"`
	CapabilitySnapshot CapabilitySnapshot `json:"capabilitySnapshot,omitempty"`
	Runtime            map[string]any     `json:"runtime,omitempty"`
}

type CapabilitySnapshot struct {
	TenantID    string                   `json:"tenantId,omitempty"`
	UserID      string                   `json:"userId,omitempty"`
	ProfileID   string                   `json:"profileId,omitempty"`
	Fingerprint string                   `json:"fingerprint"`
	Items       []CapabilitySnapshotItem `json:"items"`
}

type CapabilitySnapshotItem struct {
	ID             string   `json:"id"`
	Kind           string   `json:"kind"`
	Enabled        bool     `json:"enabled"`
	Source         string   `json:"source,omitempty"`
	SourceScope    string   `json:"sourceScope,omitempty"`
	Reason         string   `json:"reason,omitempty"`
	Policy         string   `json:"policy,omitempty"`
	RuntimeStatus  string   `json:"runtimeStatus,omitempty"`
	Risk           string   `json:"risk,omitempty"`
	InvocationMode string   `json:"invocationMode,omitempty"`
	ApprovalStatus string   `json:"approvalStatus,omitempty"`
	AllowedTools   []string `json:"allowedTools,omitempty"`
	DeniedTools    []string `json:"deniedTools,omitempty"`
}

type AgentRuntimePromptSettings struct {
	Model                   string `json:"model,omitempty"`
	ReasoningEffort         string `json:"reasoningEffort,omitempty"`
	ApprovalPolicy          string `json:"approvalPolicy,omitempty"`
	SandboxMode             string `json:"sandboxMode,omitempty"`
	PlanningPolicy          string `json:"planningPolicy,omitempty"`
	EvidencePolicy          string `json:"evidencePolicy,omitempty"`
	AnswerStyle             string `json:"answerStyle,omitempty"`
	ToolBudget              string `json:"toolBudget,omitempty"`
	ReasoningSummary        string `json:"reasoningSummary,omitempty"`
	ReasoningSummaryDisplay string `json:"reasoningSummaryDisplay,omitempty"`
	ShowRawReasoning        bool   `json:"showRawReasoning,omitempty"`
}

func AgentRuntimePromptSettingsFromProfile(profile store.AgentProfileRecord) AgentRuntimePromptSettings {
	runtime := mapField(profile, "runtime")
	return AgentRuntimePromptSettings{
		Model:                   runtimeStringField(runtime, "gpt-5.4", "model"),
		ReasoningEffort:         runtimeStringField(runtime, "medium", "reasoningEffort", "reasoning_effort"),
		ApprovalPolicy:          runtimeStringField(runtime, "untrusted", "approvalPolicy", "approval_policy"),
		SandboxMode:             runtimeStringField(runtime, "workspace-write", "sandboxMode", "sandbox_mode"),
		PlanningPolicy:          runtimeStringField(runtime, "structured_events", "planningPolicy", "planning_policy"),
		EvidencePolicy:          runtimeStringField(runtime, "tool_sourced", "evidencePolicy", "evidence_policy"),
		AnswerStyle:             runtimeStringField(runtime, "aiops_rca", "answerStyle", "answer_style"),
		ToolBudget:              runtimeStringField(runtime, "bounded", "toolBudget", "tool_budget"),
		ReasoningSummary:        runtimeStringField(runtime, "enabled", "reasoningSummary", "reasoning_summary"),
		ReasoningSummaryDisplay: runtimeStringField(runtime, "summary_only", "reasoningSummaryDisplay", "reasoning_summary_display"),
		ShowRawReasoning:        runtimeBoolField(runtime, false, "showRawReasoning", "show_raw_reasoning"),
	}
}

func (s AgentRuntimePromptSettings) ApplyToCompileContext(ctx promptcompiler.CompileContext) promptcompiler.CompileContext {
	normalized := normalizeAgentRuntimePromptSettings(s)
	ctx.PlanningPolicy = normalized.PlanningPolicy
	ctx.EvidencePolicy = normalized.EvidencePolicy
	ctx.AnswerStyle = normalized.AnswerStyle
	ctx.ToolBudget = normalized.ToolBudget
	ctx.ReasoningEffort = normalized.ReasoningEffort
	ctx.ReasoningSummary = normalized.ReasoningSummary
	ctx.ReasoningSummaryDisplay = normalized.ReasoningSummaryDisplay
	ctx.ShowRawReasoning = normalized.ShowRawReasoning
	return ctx
}

func normalizeAgentRuntimePromptSettings(settings AgentRuntimePromptSettings) AgentRuntimePromptSettings {
	profile := store.AgentProfileRecord{
		"runtime": map[string]any{
			"model":                   settings.Model,
			"reasoningEffort":         settings.ReasoningEffort,
			"approvalPolicy":          settings.ApprovalPolicy,
			"sandboxMode":             settings.SandboxMode,
			"planningPolicy":          settings.PlanningPolicy,
			"evidencePolicy":          settings.EvidencePolicy,
			"answerStyle":             settings.AnswerStyle,
			"toolBudget":              settings.ToolBudget,
			"reasoningSummary":        settings.ReasoningSummary,
			"reasoningSummaryDisplay": settings.ReasoningSummaryDisplay,
			"showRawReasoning":        settings.ShowRawReasoning,
		},
	}
	return AgentRuntimePromptSettingsFromProfile(profile)
}

func runtimeStringField(runtime map[string]any, fallback string, keys ...string) string {
	for _, key := range keys {
		if value, ok := runtime[key]; ok {
			text := strings.TrimSpace(fmt.Sprint(value))
			if text != "" {
				return text
			}
		}
	}
	return fallback
}

func runtimeBoolField(runtime map[string]any, fallback bool, keys ...string) bool {
	for _, key := range keys {
		value, ok := runtime[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case bool:
			return typed
		case string:
			switch strings.ToLower(strings.TrimSpace(typed)) {
			case "true", "1", "yes", "on":
				return true
			case "false", "0", "no", "off":
				return false
			}
		}
	}
	return fallback
}

type ActionResult struct {
	Status    string `json:"status"`
	SessionID string `json:"sessionId,omitempty"`
	TurnID    string `json:"turnId,omitempty"`
}

type ChatService interface {
	SendMessage(ctx context.Context, cmd ChatCommand) (TurnResponse, error)
	ResumeTurn(ctx context.Context, cmd ResumeCommand) (TurnResponse, error)
	CancelTurn(ctx context.Context, cmd CancelCommand) (TurnResponse, error)
	StopTurn(ctx context.Context, cmd StopCommand) (TurnResponse, error)
}

type StateService interface {
	GetState(ctx context.Context) (StateSnapshot, error)
}

type SessionService interface {
	ListSessions(ctx context.Context) (SessionListResponse, error)
	CreateSession(ctx context.Context, kind string, hostID ...string) (SessionMutationResponse, error)
	ActivateSession(ctx context.Context, sessionID string) (SessionMutationResponse, error)
}

type ApprovalService interface {
	List(ctx context.Context) ([]ApprovalView, error)
	Decide(ctx context.Context, decision ApprovalDecision) (ActionResult, error)
}

type ChoiceService interface {
	Answer(ctx context.Context, answer ChoiceAnswer) (ActionResult, error)
}

type SettingsService interface {
	GetSettings(ctx context.Context) (WebSettingsPayload, error)
	UpdateSettings(ctx context.Context, payload WebSettingsPayload) (WebSettingsPayload, error)
	GetLLMConfig(ctx context.Context) (LLMConfigView, error)
	UpdateLLMConfig(ctx context.Context, payload LLMConfigUpdate) (LLMConfigUpdateResult, error)
}

type HostService interface {
	ListHosts(ctx context.Context) ([]HostSummary, error)
	CreateHost(ctx context.Context, payload HostUpsert) (HostMutationResponse, error)
	UpdateHost(ctx context.Context, hostID string, payload HostUpsert) (HostMutationResponse, error)
	InstallHost(ctx context.Context, hostID string, payload HostInstallRequest) (HostMutationResponse, error)
	TestHostSSH(ctx context.Context, hostID string, payload HostSSHTestRequest) (HostSSHTestResponse, error)
	DeleteHost(ctx context.Context, hostID string) error
	SelectHost(ctx context.Context, hostID string) (StateSnapshot, error)
}

type HostAgentService interface {
	Register(ctx context.Context, req HostAgentRegisterRequest, token string) (HostAgentRegisterResponse, error)
	Heartbeat(ctx context.Context, req HostAgentHeartbeatRequest, token string) (HostAgentHeartbeatResponse, error)
}

type MCPService interface {
	List(ctx context.Context) (MCPServersPayload, error)
	Health(ctx context.Context) (MCPHealthPayload, error)
	HealthOne(ctx context.Context, serverID string) (MCPHealthView, error)
	Create(ctx context.Context, payload MCPServerUpsert) (MCPServersPayload, error)
	Update(ctx context.Context, name string, payload MCPServerUpsert) (MCPServersPayload, error)
	Delete(ctx context.Context, name string) (MCPServersPayload, error)
	Act(ctx context.Context, name, action string) (MCPServersPayload, error)
	Refresh(ctx context.Context) (MCPServersPayload, error)
}

type AgentProfileService interface {
	ListSkillCatalog(ctx context.Context) ([]SkillCatalogItem, error)
	SaveSkillCatalogItem(ctx context.Context, item SkillCatalogItem) (SkillCatalogPayload, error)
	DeleteSkillCatalogItem(ctx context.Context, id string) (SkillCatalogPayload, error)
	ListMcpCatalog(ctx context.Context) ([]McpCatalogItem, error)
	SaveMcpCatalogItem(ctx context.Context, item McpCatalogItem) (McpCatalogPayload, error)
	DeleteMcpCatalogItem(ctx context.Context, id string) (McpCatalogPayload, error)
	ListAgentProfiles(ctx context.Context) (AgentProfilesList, error)
	GetAgentProfile(ctx context.Context) (store.AgentProfileRecord, error)
	SaveAgentProfile(ctx context.Context, profile store.AgentProfileRecord) (store.AgentProfileRecord, error)
	ResetAgentProfile(ctx context.Context, profileID string) (store.AgentProfileRecord, error)
	PreviewAgentProfile(ctx context.Context, profileID string) (AgentProfilePreview, error)
	ExportAgentProfiles(ctx context.Context) (AgentProfilesExportPayload, error)
	ImportAgentProfiles(ctx context.Context, payload AgentProfilesImportPayload) (AgentProfilesExportPayload, error)
}

func isoStamp(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
