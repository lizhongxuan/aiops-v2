package appui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aiops-v2/internal/auth"
	"aiops-v2/internal/incidents"
	"aiops-v2/internal/mcp"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/store"
	"aiops-v2/internal/terminal"
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

type AgentEventService interface {
	Append(ctx context.Context, event AgentEvent) (AgentEvent, error)
	Subscribe(ctx context.Context, sessionID string, afterSeq int64) (<-chan AgentEvent, func())
	Projection(ctx context.Context, sessionID string) (AgentEventProjection, error)
	Replay(ctx context.Context, sessionID string, afterSeq int64) ([]AgentEvent, error)
}

type servicesConfig struct {
	settings         SettingsRepository
	hosts            HostRepository
	mcps             MCPRepository
	mcpReg           *mcp.Registry
	auth             *auth.Manager
	terminal         *terminal.Manager
	skills           SkillCatalogRepository
	agentMCP         AgentMCPCatalogRepository
	profiles         AgentProfileRepository
	agentEvents      AgentEventRepository
	incidents        incidents.Store
	lifecycleContext context.Context
	experiencePacks  ExperiencePackService
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
		cfg.hosts = dataStore
		if repo, ok := any(dataStore).(MCPRepository); ok {
			cfg.mcps = repo
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
	}
}

// WithSettingsRepository overrides the settings repository.
func WithSettingsRepository(repo SettingsRepository) ServicesOption {
	return func(cfg *servicesConfig) {
		cfg.settings = repo
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

func WithExperiencePackService(service ExperiencePackService) ServicesOption {
	return func(cfg *servicesConfig) {
		cfg.experiencePacks = service
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
	chat            ChatService
	state           StateService
	sessions        SessionService
	sessionSource   SessionSource
	approvals       ApprovalService
	choices         ChoiceService
	settings        SettingsService
	hosts           HostService
	mcps            MCPService
	profiles        AgentProfileService
	auth            AuthService
	terminal        TerminalService
	agentEvents     AgentEventService
	incidents       IncidentService
	postmortems     PostmortemService
	corootWebhooks  CorootWebhookService
	runbooks        RunbookService
	opsgraph        OpsGraphService
	erp             ERPContextService
	changes         ChangeContextService
	experiencePacks ExperiencePackService
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
	return &Services{
		chat:            NewChatServiceWithContext(cfg.lifecycleContext, runtime, sessions, agentEvents),
		state:           NewStateService(sessions, builder),
		sessions:        NewSessionService(sessions, sessionStore, builder),
		sessionSource:   sessions,
		approvals:       NewApprovalServiceWithContext(cfg.lifecycleContext, runtime, sessions, builder),
		choices:         NewChoiceService(runtime, sessions),
		settings:        settingsService,
		hosts:           NewHostService(sessionStore, cfg.hosts, builder),
		mcps:            NewMCPService(cfg.mcps, registry),
		profiles:        NewAgentProfileService(newAgentProfileRepositories(cfg.skills, cfg.agentMCP, cfg.profiles)),
		auth:            authService,
		terminal:        NewTerminalService(cfg.terminal, cfg.hosts),
		agentEvents:     agentEvents,
		incidents:       incidentService,
		postmortems:     NewPostmortemService(incidentService),
		corootWebhooks:  NewCorootWebhookService(incidentService),
		runbooks:        NewRunbookService("", nil),
		opsgraph:        NewOpsGraphService(""),
		erp:             NewERPContextService(),
		changes:         NewChangeContextService(),
		experiencePacks: firstNonNilExperiencePackService(cfg.experiencePacks),
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
func (s *Services) MCPService() MCPService           { return s.mcps }
func (s *Services) AgentProfileService() AgentProfileService {
	return s.profiles
}
func (s *Services) AuthService() AuthService         { return s.auth }
func (s *Services) TerminalService() TerminalService { return s.terminal }
func (s *Services) AgentEventService() AgentEventService {
	return s.agentEvents
}
func (s *Services) IncidentService() IncidentService             { return s.incidents }
func (s *Services) PostmortemService() PostmortemService         { return s.postmortems }
func (s *Services) CorootWebhookService() CorootWebhookService   { return s.corootWebhooks }
func (s *Services) RunbookService() RunbookService               { return s.runbooks }
func (s *Services) OpsGraphService() OpsGraphService             { return s.opsgraph }
func (s *Services) ERPContextService() ERPContextService         { return s.erp }
func (s *Services) ChangeContextService() ChangeContextService   { return s.changes }
func (s *Services) ExperiencePackService() ExperiencePackService { return s.experiencePacks }

type ExperiencePackServiceProvider interface {
	ExperiencePackService() ExperiencePackService
}

type ExperiencePackService interface {
	ListPacks(ListExperiencePacksRequest) (ExperiencePackLibraryList, error)
	ListCandidates(ListExperiencePackCandidatesRequest) (ExperiencePackCandidateList, error)
	Retrieve(ExperiencePackRetrieveRequest) (ExperiencePackMatchList, error)
	RetrieveCandidate(candidateID string) (ExperiencePack, error)
	EvaluateSuggestions(ExperiencePackSuggestionEvaluateRequest) (ExperiencePackSuggestionEvaluateResult, error)
	PrepareCandidate(ExperiencePackPrepareCandidateRequest) (ExperiencePackCandidate, error)
	ConfirmCandidate(candidateID string, req ExperiencePackReviewRequest) (ExperiencePack, error)
	PrepareRunnerCandidate(ExperiencePackRunnerCandidateRequest) (ExperiencePackRunnerCandidate, error)
	ConfirmRunnerCandidate(ExperiencePackRunnerCandidateRequest) (ExperiencePackRunnerCandidate, error)
	GetPack(packID string) (ExperiencePack, error)
	GetValidationGate(packID string) (ExperiencePackValidationGate, error)
	EnablePack(packID string, req ExperiencePackReviewRequest) (ExperiencePack, error)
	PausePack(packID string, req ExperiencePackReviewRequest) (ExperiencePack, error)
	SetPackEnabled(packID string, enabled bool, req ExperiencePackReviewRequest) (ExperiencePack, error)
	SaveAuthorizationScopes(packID string, scopes []ExperiencePackAuthorizationScope) (ExperiencePack, error)
	GetAuthorizationScopes(packID string) ([]ExperiencePackAuthorizationScope, error)
	SaveRunnerBindings(packID string, bindings []ExperiencePackRunnerBinding) (ExperiencePack, error)
	ReviewRunnerBindings(packID string, req ExperiencePackRunnerBindingReviewRequest) (ExperiencePack, error)
	ListReuseRecords(packID string, req ListExperiencePackReuseRecordsRequest) (ExperiencePackReuseRecordList, error)
}

type ListExperiencePacksRequest struct {
	Status           string `json:"status,omitempty"`
	Category         string `json:"category,omitempty"`
	UsageShape       string `json:"usageShape,omitempty"`
	Middleware       string `json:"middleware,omitempty"`
	Tag              string `json:"tag,omitempty"`
	HasRunnerBinding string `json:"hasRunnerBinding,omitempty"`
	Limit            int    `json:"limit,omitempty"`
	Cursor           string `json:"cursor,omitempty"`
}

type ListExperiencePackCandidatesRequest struct {
	CaseID      string `json:"caseId,omitempty"`
	Service     string `json:"service,omitempty"`
	Environment string `json:"environment,omitempty"`
	Limit       int    `json:"limit,omitempty"`
	Cursor      string `json:"cursor,omitempty"`
}

type ExperiencePackSuggestionEvaluateRequest struct {
	CaseID                   string         `json:"caseId,omitempty"`
	Service                  string         `json:"service,omitempty"`
	Environment              string         `json:"environment,omitempty"`
	Signals                  any            `json:"signals,omitempty"`
	Limit                    int            `json:"limit,omitempty"`
	CommandCount             int            `json:"commandCount,omitempty"`
	Outcome                  string         `json:"outcome,omitempty"`
	RedactionStatus          string         `json:"redactionStatus,omitempty"`
	LLMOperationalValueScore float64        `json:"llmOperationalValueScore,omitempty"`
	MatchedPackID            string         `json:"matchedPackId,omitempty"`
	MemoryGraphWritable      bool           `json:"memoryGraphWritable,omitempty"`
	ReusableStepCount        int            `json:"reusableStepCount,omitempty"`
	Metadata                 map[string]any `json:"metadata,omitempty"`
}

type ExperiencePackRetrieveRequest struct {
	CaseID        string         `json:"caseId,omitempty"`
	ChatSessionID string         `json:"chatSessionId,omitempty"`
	Query         string         `json:"query,omitempty"`
	UserText      string         `json:"userText,omitempty"`
	Signals       any            `json:"signals,omitempty"`
	OS            string         `json:"os,omitempty"`
	Environment   string         `json:"environment,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

type ExperiencePackPrepareCandidateRequest struct {
	CaseID            string         `json:"caseId,omitempty"`
	ChatSessionID     string         `json:"chatSessionId,omitempty"`
	PackID            string         `json:"packId,omitempty"`
	Title             string         `json:"title,omitempty"`
	Summary           string         `json:"summary,omitempty"`
	Service           string         `json:"service,omitempty"`
	Environment       string         `json:"environment,omitempty"`
	Commands          []string       `json:"commands,omitempty"`
	SuggestionID      string         `json:"suggestionId,omitempty"`
	ConfirmationToken string         `json:"confirmationToken,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
}

type ExperiencePackReviewRequest struct {
	Reviewer          string         `json:"reviewer,omitempty"`
	Comment           string         `json:"comment,omitempty"`
	Decision          string         `json:"decision,omitempty"`
	Reason            string         `json:"reason,omitempty"`
	CandidateID       string         `json:"candidateId,omitempty"`
	ConfirmationToken string         `json:"confirmationToken,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
}

type ExperiencePackRunnerCandidateRequest struct {
	CaseID            string         `json:"caseId,omitempty"`
	ChatSessionID     string         `json:"chatSessionId,omitempty"`
	PackID            string         `json:"packId,omitempty"`
	CandidateID       string         `json:"candidateId,omitempty"`
	Title             string         `json:"title,omitempty"`
	Summary           string         `json:"summary,omitempty"`
	Service           string         `json:"service,omitempty"`
	Environment       string         `json:"environment,omitempty"`
	Commands          []string       `json:"commands,omitempty"`
	ConfirmationToken string         `json:"confirmationToken,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
}

type ExperiencePackRunnerCandidate struct {
	ID              string                      `json:"id"`
	PackID          string                      `json:"pack_id,omitempty"`
	WorkflowID      string                      `json:"workflow_id,omitempty"`
	WorkflowName    string                      `json:"workflow_name,omitempty"`
	Status          string                      `json:"status"`
	StudioDraftLink string                      `json:"studio_draft_link,omitempty"`
	Workflow        map[string]any              `json:"workflow,omitempty"`
	Graph           map[string]any              `json:"graph,omitempty"`
	RunnerBinding   ExperiencePackRunnerBinding `json:"runner_binding,omitempty"`
	Metadata        map[string]any              `json:"metadata,omitempty"`
}

type ListExperiencePackReuseRecordsRequest struct {
	CaseID string `json:"caseId,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}

type ExperiencePackCandidateList struct {
	Items      []ExperiencePackCandidate `json:"items"`
	Total      int                       `json:"total"`
	NextCursor string                    `json:"nextCursor,omitempty"`
}

type ExperiencePackLibraryList struct {
	Items      []ExperiencePack `json:"items"`
	Total      int              `json:"total"`
	NextCursor string           `json:"nextCursor,omitempty"`
}

type ExperiencePackSuggestionEvaluateResult struct {
	Items       []ExperiencePackSuggestion `json:"items"`
	Suggestions []ExperiencePackSuggestion `json:"suggestions"`
	Total       int                        `json:"total"`
	NextCursor  string                     `json:"nextCursor,omitempty"`
}

type ExperiencePackReuseRecordList struct {
	Items      []ExperiencePackReuseRecord `json:"items"`
	Total      int                         `json:"total"`
	NextCursor string                      `json:"nextCursor,omitempty"`
}

type ExperiencePackCandidate struct {
	ID             string          `json:"id"`
	CandidateID    string          `json:"candidate_id,omitempty"`
	PackID         string          `json:"pack_id"`
	Title          string          `json:"title"`
	Summary        string          `json:"summary,omitempty"`
	Status         string          `json:"status"`
	MatchReason    string          `json:"match_reason,omitempty"`
	SourceCaseID   string          `json:"source_case_id,omitempty"`
	ExperiencePack *ExperiencePack `json:"experience_pack,omitempty"`
	CreatedAt      string          `json:"created_at,omitempty"`
	UpdatedAt      string          `json:"updated_at,omitempty"`
	Metadata       map[string]any  `json:"metadata,omitempty"`
}

type ExperiencePack struct {
	ID                  string                             `json:"id"`
	PackID              string                             `json:"pack_id,omitempty"`
	Title               string                             `json:"title"`
	Summary             string                             `json:"summary,omitempty"`
	Version             string                             `json:"version,omitempty"`
	Category            string                             `json:"category,omitempty"`
	UsageShape          string                             `json:"usage_shape,omitempty"`
	Middleware          string                             `json:"middleware,omitempty"`
	Tags                []string                           `json:"tags,omitempty"`
	Status              string                             `json:"status"`
	ReviewStatus        string                             `json:"review_status"`
	Enabled             bool                               `json:"enabled"`
	Skill               ExperiencePackSkill                `json:"skill,omitempty"`
	History             ExperiencePackHistory              `json:"history,omitempty"`
	AdvancedRefs        ExperiencePackAdvancedRefs         `json:"advanced_refs,omitempty"`
	RetrievalEval       ExperiencePackRetrievalEval        `json:"retrieval_eval"`
	ValidationGate      ExperiencePackValidationGate       `json:"validation_gate"`
	WorkflowBinding     ExperiencePackWorkflowBinding      `json:"workflow_binding"`
	RunnerBindings      []ExperiencePackRunnerBinding      `json:"runner_bindings,omitempty"`
	AuthorizationScopes []ExperiencePackAuthorizationScope `json:"authorization_scopes"`
	CreatedAt           string                             `json:"created_at,omitempty"`
	UpdatedAt           string                             `json:"updated_at,omitempty"`
	Metadata            map[string]any                     `json:"metadata,omitempty"`
}

type ExperiencePackSkill struct {
	ID      string `json:"id,omitempty"`
	Name    string `json:"name,omitempty"`
	Summary string `json:"summary,omitempty"`
	Path    string `json:"path,omitempty"`
}

type ExperiencePackHistory struct {
	SuccessCount int    `json:"success_count,omitempty"`
	FailureCount int    `json:"failure_count,omitempty"`
	RecentResult string `json:"recent_result,omitempty"`
}

type ExperiencePackAdvancedRefs struct {
	GeneAssetID     string   `json:"gene_asset_id,omitempty"`
	CapsuleAssetIDs []string `json:"capsule_asset_ids,omitempty"`
}

type ExperiencePackMatchList struct {
	Items []ExperiencePackMatch `json:"items"`
	Total int                   `json:"total"`
}

type ExperiencePackMatch struct {
	PackID              string                      `json:"pack_id"`
	Skill               ExperiencePackSkill         `json:"skill"`
	Confidence          float64                     `json:"confidence,omitempty"`
	CompatibilityStatus string                      `json:"compatibility_status,omitempty"`
	CompatibilityGaps   []string                    `json:"compatibility_gaps,omitempty"`
	MatchedSignals      []string                    `json:"matched_signals,omitempty"`
	MatchReasons        []string                    `json:"match_reasons,omitempty"`
	PreconditionGaps    []string                    `json:"precondition_gaps,omitempty"`
	RiskWarnings        []string                    `json:"risk_warnings,omitempty"`
	NextActions         []string                    `json:"next_actions,omitempty"`
	OSVariant           string                      `json:"os_variant,omitempty"`
	RunnerBinding       ExperiencePackRunnerBinding `json:"runner_binding,omitempty"`
	History             ExperiencePackHistory       `json:"history,omitempty"`
	AdvancedRefs        ExperiencePackAdvancedRefs  `json:"advanced_refs,omitempty"`
}

type ExperiencePackSuggestion struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	Label       string         `json:"label"`
	Reason      string         `json:"reason,omitempty"`
	CaseID      string         `json:"caseId,omitempty"`
	PackID      string         `json:"packId,omitempty"`
	Title       string         `json:"title,omitempty"`
	Summary     string         `json:"summary,omitempty"`
	Service     string         `json:"service,omitempty"`
	Environment string         `json:"environment,omitempty"`
	SourceRefs  []string       `json:"source_refs,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type ExperiencePackRetrievalEval struct {
	Score           *float64       `json:"score,omitempty"`
	MatchedCases    int            `json:"matched_cases,omitempty"`
	Verdict         string         `json:"verdict,omitempty"`
	LastEvaluatedAt string         `json:"last_evaluated_at,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

type ExperiencePackValidationGate struct {
	Status    string           `json:"status"`
	Reasons   []string         `json:"reasons,omitempty"`
	Checks    []map[string]any `json:"checks,omitempty"`
	UpdatedAt string           `json:"updated_at,omitempty"`
}

type ExperiencePackWorkflowBinding struct {
	WorkflowID   string         `json:"workflow_id,omitempty"`
	WorkflowName string         `json:"workflow_name,omitempty"`
	Status       string         `json:"status,omitempty"`
	Version      string         `json:"version,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

type ExperiencePackRunnerBinding struct {
	ID           string         `json:"id,omitempty"`
	WorkflowID   string         `json:"workflow_id,omitempty"`
	WorkflowName string         `json:"workflow_name,omitempty"`
	Status       string         `json:"status,omitempty"`
	ReviewStatus string         `json:"review_status,omitempty"`
	Version      string         `json:"version,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

type ExperiencePackRunnerBindingReviewRequest struct {
	Reviewer   string   `json:"reviewer,omitempty"`
	Decision   string   `json:"decision,omitempty"`
	Comment    string   `json:"comment,omitempty"`
	BindingIDs []string `json:"bindingIds,omitempty"`
}

type ExperiencePackAuthorizationScope struct {
	ID         string         `json:"id,omitempty"`
	Type       string         `json:"type"`
	Value      string         `json:"value"`
	Searchable bool           `json:"searchable"`
	Reason     string         `json:"reason,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type ExperiencePackReuseRecord struct {
	ID       string         `json:"id"`
	PackID   string         `json:"pack_id"`
	CaseID   string         `json:"case_id,omitempty"`
	Result   string         `json:"result,omitempty"`
	Summary  string         `json:"summary,omitempty"`
	ReusedBy string         `json:"reused_by,omitempty"`
	ReusedAt string         `json:"reused_at,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
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
	SessionID       string `json:"sessionId"`
	TurnID          string `json:"turnId"`
	ClientTurnID    string `json:"clientTurnId,omitempty"`
	ClientMessageID string `json:"clientMessageId,omitempty"`
	Status          string `json:"status"`
	Output          string `json:"output,omitempty"`
	Error           string `json:"error,omitempty"`
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
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Status          string            `json:"status"`
	Kind            string            `json:"kind,omitempty"`
	Address         string            `json:"address,omitempty"`
	Transport       string            `json:"transport,omitempty"`
	Executable      bool              `json:"executable,omitempty"`
	TerminalCapable bool              `json:"terminalCapable,omitempty"`
	OS              string            `json:"os,omitempty"`
	Arch            string            `json:"arch,omitempty"`
	AgentVersion    string            `json:"agentVersion,omitempty"`
	LastHeartbeat   string            `json:"lastHeartbeat,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	LastError       string            `json:"lastError,omitempty"`
	SSHUser         string            `json:"sshUser,omitempty"`
	SSHPort         int               `json:"sshPort,omitempty"`
	InstallState    string            `json:"installState,omitempty"`
	ControlMode     string            `json:"controlMode,omitempty"`
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
	ToolName       string `json:"toolName,omitempty"`
	Command        string `json:"command,omitempty"`
	Reason         string `json:"reason,omitempty"`
	Risk           string `json:"risk,omitempty"`
	Source         string `json:"source,omitempty"`
	RunbookID      string `json:"runbookId,omitempty"`
	RunbookStep    string `json:"runbookStep,omitempty"`
	ExpectedEffect string `json:"expectedEffect,omitempty"`
	Rollback       string `json:"rollback,omitempty"`
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
	ID       string
	Decision string
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
	Provider         string `json:"provider,omitempty"`
	Model            string `json:"model,omitempty"`
	BaseURL          string `json:"baseURL,omitempty"`
	FallbackProvider string `json:"fallbackProvider,omitempty"`
	FallbackModel    string `json:"fallbackModel,omitempty"`
	CompactModel     string `json:"compactModel,omitempty"`
	BifrostActive    bool   `json:"bifrostActive"`
	APIKeySet        bool   `json:"apiKeySet"`
	APIKeyMasked     string `json:"apiKeyMasked,omitempty"`
}

type LLMConfigUpdate struct {
	Provider         string `json:"provider,omitempty"`
	Model            string `json:"model,omitempty"`
	APIKey           string `json:"apiKey,omitempty"`
	BaseURL          string `json:"baseURL,omitempty"`
	FallbackProvider string `json:"fallbackProvider,omitempty"`
	FallbackModel    string `json:"fallbackModel,omitempty"`
	FallbackAPIKey   string `json:"fallbackApiKey,omitempty"`
	CompactModel     string `json:"compactModel,omitempty"`
}

type LLMConfigUpdateResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

type HostUpsert struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Address       string            `json:"address"`
	SSHUser       string            `json:"sshUser"`
	SSHPort       int               `json:"sshPort"`
	Labels        map[string]string `json:"labels"`
	InstallViaSSH bool              `json:"installViaSsh"`
}

type HostMutationResponse struct {
	Host  HostSummary   `json:"host"`
	Items []HostSummary `json:"items,omitempty"`
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
	Name          string            `json:"name"`
	Transport     string            `json:"transport,omitempty"`
	Command       string            `json:"command,omitempty"`
	Args          []string          `json:"args,omitempty"`
	URL           string            `json:"url,omitempty"`
	Env           map[string]string `json:"env,omitempty"`
	Disabled      bool              `json:"disabled,omitempty"`
	Status        string            `json:"status,omitempty"`
	Error         string            `json:"error,omitempty"`
	ToolCount     int               `json:"toolCount,omitempty"`
	ResourceCount int               `json:"resourceCount,omitempty"`
}

type MCPServersPayload struct {
	ConfigPath string          `json:"configPath,omitempty"`
	Items      []MCPServerView `json:"items"`
}

type SkillCatalogItem struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	Description           string `json:"description,omitempty"`
	Source                string `json:"source,omitempty"`
	Enabled               bool   `json:"enabled,omitempty"`
	DefaultEnabled        bool   `json:"defaultEnabled,omitempty"`
	ActivationMode        string `json:"activationMode,omitempty"`
	DefaultActivationMode string `json:"defaultActivationMode,omitempty"`
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
	Enabled                      bool   `json:"enabled,omitempty"`
	DefaultEnabled               bool   `json:"defaultEnabled,omitempty"`
	Permission                   string `json:"permission,omitempty"`
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
	ProfileID         string           `json:"profileId"`
	ProfileType       string           `json:"profileType,omitempty"`
	SystemPrompt      string           `json:"systemPrompt,omitempty"`
	SystemPromptLines int              `json:"systemPromptLines"`
	CommandSummary    []string         `json:"commandSummary,omitempty"`
	CapabilitySummary []string         `json:"capabilitySummary,omitempty"`
	EnabledSkills     []map[string]any `json:"enabledSkills,omitempty"`
	EnabledMcps       []map[string]any `json:"enabledMcps,omitempty"`
	Runtime           map[string]any   `json:"runtime,omitempty"`
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
	CreateSession(ctx context.Context, kind string) (SessionMutationResponse, error)
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
	DeleteHost(ctx context.Context, hostID string) error
	SelectHost(ctx context.Context, hostID string) (StateSnapshot, error)
}

type MCPService interface {
	List(ctx context.Context) (MCPServersPayload, error)
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
