package eval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/policyengine"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/runtimecontract"
	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/tooling"
	"github.com/cloudwego/eino/components/model"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// RuntimeRolloutReplayBackend replays fixtures through RuntimeKernel and appui.
type RuntimeRolloutReplayBackend struct{}

var _ RolloutReplayBackend = (*RuntimeRolloutReplayBackend)(nil)

func NewRuntimeRolloutReplayBackend() *RuntimeRolloutReplayBackend {
	return &RuntimeRolloutReplayBackend{}
}
func (*RuntimeRolloutReplayBackend) ReplayProviderFixture(ctx context.Context, fixture RolloutReplayFixture) (ReplayExecution, error) {
	return replayRuntimeStory(ctx, fixture, false)
}
func (*RuntimeRolloutReplayBackend) ReplayFullStory(ctx context.Context, fixture RolloutReplayFixture) (ReplayExecution, error) {
	return replayRuntimeStory(ctx, fixture, true)
}
func replayRuntimeStory(ctx context.Context, fixture RolloutReplayFixture, project bool) (ReplayExecution, error) {
	story, err := loadRuntimeReplayStory(fixture)
	if err != nil {
		return ReplayExecution{}, err
	}
	engine, err := newRuntimeReplayEngine(story)
	if err != nil {
		return ReplayExecution{}, err
	}
	sessionID, threadID := engine.chat.sessionID, "replay-thread-"+runtimeReplaySlug(story.Name)
	state := appui.NewAiopsTransportState(sessionID, threadID)
	approval := replaySyncApprovalService{ApprovalService: appui.NewApprovalService(engine.kernel, engine.sessions, nil)}
	handler := appui.NewTransportCommandHandler(engine.chat, approval, nil, nil)
	projector := appui.NewTransportProjector()
	var turnID string
	for _, request := range runtimeReplayCommands(story) {
		command, commandErr := request.transportCommand(state, engine.sessions, story)
		if commandErr != nil {
			return ReplayExecution{}, commandErr
		}
		state, _, err = handler.Apply(ctx, state, command)
		if err != nil {
			return ReplayExecution{}, err
		}
		snapshot, snapshotErr := engine.sessions.GetSnapshot(sessionID)
		if snapshotErr != nil {
			return ReplayExecution{}, snapshotErr
		}
		if snapshot != nil && snapshot.CurrentTurn != nil {
			turnID = snapshot.CurrentTurn.ID
			if project {
				state, err = projector.ProjectTurnSnapshot(state, snapshot.CurrentTurn)
				if err != nil {
					return ReplayExecution{}, err
				}
			}
		}
	}
	if project {
		return engine.execution(ctx, sessionID, turnID, &state)
	}
	return engine.execution(ctx, sessionID, turnID, nil)
}

type runtimeReplayStory struct {
	Name              string                    `json:"name"`
	SessionType       string                    `json:"sessionType,omitempty"`
	Mode              string                    `json:"mode,omitempty"`
	Command           json.RawMessage           `json:"command,omitempty"`
	Requests          []runtimeReplayRequest    `json:"requests,omitempty"`
	ProviderResponses []runtimeProviderResponse `json:"providerResponses"`
	ToolOutcomes      []runtimeToolOutcome      `json:"toolOutcomes"`
}
type runtimeReplayRequest struct {
	Command    json.RawMessage       `json:"command,omitempty"`
	Type       string                `json:"type,omitempty"`
	Message    *runtimeReplayMessage `json:"message,omitempty"`
	ApprovalID string                `json:"approvalId,omitempty"`
	Decision   string                `json:"decision,omitempty"`
}
type runtimeReplayMessage struct {
	ID    string `json:"id,omitempty"`
	Parts []struct {
		Text string `json:"text,omitempty"`
	} `json:"parts,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}
type runtimeProviderResponse struct {
	Role      string `json:"role"`
	Content   string `json:"content,omitempty"`
	ToolCalls []struct {
		ID, Name  string
		Arguments json.RawMessage `json:"arguments"`
	} `json:"toolCalls,omitempty"`
}
type runtimeToolOutcome struct {
	Name, Description, Content, Error, Risk, PermissionScope string
	InputSchema                                              json.RawMessage                                                              `json:"inputSchema,omitempty"`
	Mutating                                                 bool                                                                         `json:"mutating,omitempty"`
	PostChecks                                               []string                                                                     `json:"postChecks,omitempty"`
	Approval                                                 *struct{ Reason, Risk, Source, ExpectedEffect, Rollback, Validation string } `json:"approval,omitempty"`
}

func loadRuntimeReplayStory(fixture RolloutReplayFixture) (runtimeReplayStory, error) {
	if fixture.Source.Kind != "assistant_transport_story" || len(fixture.sourceData) == 0 {
		return runtimeReplayStory{}, errors.New("runtime rollout replay requires a source bound by LoadRolloutReplayFixtureFile")
	}
	data := append([]byte(nil), fixture.sourceData...)
	if actual := replaySourceContentHash(data); actual != fixture.Source.Hash {
		return runtimeReplayStory{}, fmt.Errorf("runtime replay source hash mismatch: expected %s, actual %s", fixture.Source.Hash, actual)
	}
	var story runtimeReplayStory
	if err := json.Unmarshal(data, &story); err != nil {
		return runtimeReplayStory{}, fmt.Errorf("decode runtime replay source: %w", err)
	}
	if strings.TrimSpace(story.Name) == "" || len(story.ProviderResponses) == 0 {
		return runtimeReplayStory{}, errors.New("runtime replay source requires name and provider responses")
	}
	return story, nil
}
func runtimeReplayCommands(story runtimeReplayStory) []runtimeReplayRequest {
	if len(story.Requests) == 0 {
		return []runtimeReplayRequest{{Command: story.Command}}
	}
	return story.Requests
}
func (r runtimeReplayRequest) decoded() (runtimeReplayRequest, error) {
	if len(r.Command) == 0 {
		return r, nil
	}
	var command runtimeReplayRequest
	if err := json.Unmarshal(r.Command, &command); err != nil {
		return runtimeReplayRequest{}, fmt.Errorf("decode runtime replay command: %w", err)
	}
	return command, nil
}
func (r runtimeReplayRequest) transportCommand(state appui.AiopsTransportState, sessions *runtimekernel.SessionManager, story runtimeReplayStory) (appui.TransportCommand, error) {
	command, err := r.decoded()
	if err != nil {
		return appui.TransportCommand{}, err
	}
	switch command.Type {
	case string(appui.TransportCommandTypeAddMessage):
		text, id, metadata := runtimeReplayMessageData(command.Message)
		return appui.TransportCommand{Type: appui.TransportCommandTypeAddMessage, AddMessage: &appui.TransportAddMessageCommand{SessionID: state.SessionID, SessionType: firstRuntimeReplayValue(story.SessionType, string(runtimekernel.SessionTypeWorkspace)), Mode: firstRuntimeReplayValue(story.Mode, string(runtimekernel.ModeChat)), ThreadID: state.ThreadID, SourceID: id, ClientMessageID: id, ClientTurnID: "replay-client-turn-" + runtimeReplaySlug(story.Name), Message: appui.TransportUserMessage{Text: text}, Metadata: metadata}}, nil
	case string(appui.TransportCommandTypeApprovalDecision):
		snapshot, snapshotErr := sessions.GetSnapshot(state.SessionID)
		if snapshotErr != nil {
			return appui.TransportCommand{}, snapshotErr
		}
		approval, approvalErr := currentRuntimeReplayApproval(snapshot)
		if approvalErr != nil {
			return appui.TransportCommand{}, approvalErr
		}
		return appui.TransportCommand{Type: appui.TransportCommandTypeApprovalDecision, ApprovalDecision: &appui.TransportApprovalDecisionCommand{SessionID: state.SessionID, TurnID: approval.TurnID, ApprovalID: approval.ID, Decision: command.Decision}}, nil
	default:
		return appui.TransportCommand{}, fmt.Errorf("unsupported runtime replay command %q", command.Type)
	}
}

type runtimeReplayEngine struct {
	kernel    *runtimekernel.RuntimeKernel
	sessions  *runtimekernel.SessionManager
	provider  *runtimeReplayProvider
	chat      *runtimeReplayChatService
	artifacts *runtimeReplayArtifactCollector
}

func newRuntimeReplayEngine(story runtimeReplayStory) (*runtimeReplayEngine, error) {
	registry := tooling.NewRegistry()
	for _, outcome := range story.ToolOutcomes {
		if err := registry.Register(runtimeReplayTool(outcome)); err != nil {
			return nil, err
		}
	}
	provider, err := newRuntimeReplayProvider(story.ProviderResponses)
	if err != nil {
		return nil, err
	}
	router := modelrouter.NewRouter("replay", map[string]modelrouter.ChatModel{"replay": provider}, nil)
	router.SetProviderConfigResolver(runtimeReplayProviderResolver{})
	sessions := runtimekernel.NewSessionManager()
	artifacts := &runtimeReplayArtifactCollector{}
	kernel := runtimekernel.NewRuntimeKernel(runtimekernel.RuntimeKernelConfig{ToolSource: runtimeReplayToolSource{registry}, Compiler: promptcompiler.NewCompiler(), Policy: &policyengine.Engine{ModePolicy: policyengine.NewDefaultModePolicies(), CompletionPolicy: &policyengine.DefaultCompletionEvaluator{}}, Projector: runtimeReplayEmitter{}, ModelRouter: router, Sessions: sessions, ReplayArtifactSink: artifacts, DebugConfig: func(context.Context) runtimekernel.RuntimeDebugConfig { return runtimekernel.RuntimeDebugConfig{} }})
	chat := &runtimeReplayChatService{kernel: kernel, sessionID: "replay-session-" + runtimeReplaySlug(story.Name), sessionType: story.SessionType, mode: story.Mode}
	return &runtimeReplayEngine{kernel: kernel, sessions: sessions, provider: provider, chat: chat, artifacts: artifacts}, nil
}
func (e *runtimeReplayEngine) execution(ctx context.Context, sessionID, turnID string, state *appui.AiopsTransportState) (ReplayExecution, error) {
	if err := e.provider.assertExhausted(); err != nil {
		return ReplayExecution{}, err
	}
	events, err := e.kernel.CanonicalRolloutEvents(ctx, sessionID, turnID)
	if err != nil {
		return ReplayExecution{}, err
	}
	contract := e.artifacts.contract()
	return ReplayExecution{Rollout: events, TransportState: state, Contract: contract}, nil
}

type runtimeReplayProvider struct {
	mu        sync.Mutex
	responses []*schema.Message
}

func newRuntimeReplayProvider(fixtures []runtimeProviderResponse) (*runtimeReplayProvider, error) {
	provider := &runtimeReplayProvider{}
	for _, fixture := range fixtures {
		if !strings.EqualFold(strings.TrimSpace(fixture.Role), "assistant") {
			return nil, fmt.Errorf("runtime replay provider role %q is not assistant", fixture.Role)
		}
		calls := make([]schema.ToolCall, 0, len(fixture.ToolCalls))
		for _, call := range fixture.ToolCalls {
			calls = append(calls, schema.ToolCall{ID: call.ID, Type: "function", Function: schema.FunctionCall{Name: call.Name, Arguments: string(call.Arguments)}})
		}
		provider.responses = append(provider.responses, schema.AssistantMessage(fixture.Content, calls))
	}
	return provider, nil
}
func (p *runtimeReplayProvider) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	return p.next()
}
func (p *runtimeReplayProvider) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	message, err := p.next()
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{message}), nil
}
func (*runtimeReplayProvider) BindTools([]*schema.ToolInfo) error { return nil }
func (p *runtimeReplayProvider) next() (*schema.Message, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.responses) == 0 {
		return nil, errors.New("runtime replay provider has no response remaining")
	}
	message := p.responses[0]
	p.responses = p.responses[1:]
	return message, nil
}
func (p *runtimeReplayProvider) assertExhausted() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.responses) != 0 {
		return fmt.Errorf("%d runtime replay provider response(s) were not consumed", len(p.responses))
	}
	return nil
}

type runtimeReplayProviderResolver struct{}

func (runtimeReplayProviderResolver) ResolveProviderConfig(modelrouter.AgentKind) (modelrouter.ProviderConfig, bool) {
	return modelrouter.ProviderConfig{Provider: "replay", Model: "replay", MaxContextTokens: runtimekernel.DefaultMaxTokens}, true
}

type runtimeReplayToolSource struct{ registry *tooling.Registry }

func (s runtimeReplayToolSource) CompileContext(session runtimekernel.SessionType, mode runtimekernel.Mode) promptcompiler.CompileContext {
	return promptcompiler.CompileContext{SessionType: string(session), Mode: string(mode), AssembledTools: s.registry.AssembleTools(string(session), string(mode))}
}
func (s runtimeReplayToolSource) AssembleToolPool(session runtimekernel.SessionType, mode runtimekernel.Mode) []einotool.BaseTool {
	return s.registry.AssembleToolPool(string(session), string(mode))
}
func (s runtimeReplayToolSource) CompileContextWithMetadata(session runtimekernel.SessionType, mode runtimekernel.Mode, metadata map[string]string) []promptcompiler.Tool {
	return s.registry.CompileContextWithMetadata(string(session), string(mode), metadata)
}
func (s runtimeReplayToolSource) AssembleToolPoolWithMetadata(session runtimekernel.SessionType, mode runtimekernel.Mode, metadata map[string]string) []einotool.BaseTool {
	return s.registry.AssembleToolPoolWithMetadata(string(session), string(mode), metadata)
}

type runtimeReplayEmitter struct{}

func (runtimeReplayEmitter) Emit(runtimekernel.LifecycleEvent) {}
func runtimeReplayTool(outcome runtimeToolOutcome) *tooling.StaticTool {
	schemaData := outcome.InputSchema
	if len(schemaData) == 0 {
		schemaData = json.RawMessage(`{"type":"object","additionalProperties":true}`)
	}
	definition := &tooling.StaticTool{Meta: tooling.ToolMetadata{Name: outcome.Name, Description: firstRuntimeReplayValue(outcome.Description, "deterministic replay tool "+outcome.Name), Origin: tooling.ToolOriginBuiltin, Layer: tooling.ToolLayerCore, AlwaysLoad: true, RiskLevel: tooling.ToolRiskLevel(firstRuntimeReplayValue(outcome.Risk, string(tooling.ToolRiskLow))), Mutating: outcome.Mutating, RequiresApproval: outcome.Approval != nil, Discovery: tooling.ToolDiscoveryMetadata{PermissionScope: outcome.PermissionScope}}, InputSchemaData: schemaData, ReadOnlyFunc: func(json.RawMessage) bool { return !outcome.Mutating }}
	definition.CheckPermissionsFunc = func(context.Context, json.RawMessage) tooling.PermissionDecision {
		if outcome.Approval == nil {
			return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
		}
		return tooling.PermissionDecision{Action: tooling.PermissionActionNeedApproval, Reason: outcome.Approval.Reason, Approval: &tooling.PermissionApprovalPayload{Reason: outcome.Approval.Reason, Risk: firstRuntimeReplayValue(outcome.Approval.Risk, outcome.Risk), Source: outcome.Approval.Source, ExpectedEffect: outcome.Approval.ExpectedEffect, Rollback: outcome.Approval.Rollback, Validation: outcome.Approval.Validation}}
	}
	definition.ExecuteFunc = func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
		if outcome.Error != "" {
			return tooling.ToolResult{Error: outcome.Error}, errors.New(outcome.Error)
		}
		return tooling.ToolResult{Content: outcome.Content}, nil
	}
	if outcome.Mutating {
		definition.Meta.Layer = tooling.ToolLayerMutation
		definition.Meta.ResourceLocks = []tooling.ToolResourceLockKey{{ResourceType: "replay_resource", ResourceID: runtimeReplaySlug(outcome.Name), OperationKind: "mutation"}}
		definition.Meta.Idempotency = tooling.ToolIdempotencyMetadata{Strategy: tooling.ToolIdempotencyStrategyArgumentsHash, PostCheckRefs: append([]string(nil), outcome.PostChecks...)}
	}
	if outcome.Approval != nil {
		definition.Meta.Discovery.PermissionScope = "argument_scoped"
	}
	return definition
}

type runtimeReplayChatService struct {
	appui.ChatService
	kernel                       *runtimekernel.RuntimeKernel
	sessionID, sessionType, mode string
}

func (s *runtimeReplayChatService) turnRequest(command runtimeReplayRequest) runtimekernel.TurnRequest {
	text, messageID, metadata := runtimeReplayMessageData(command.Message)
	sessionType := runtimekernel.SessionType(firstRuntimeReplayValue(s.sessionType, string(runtimekernel.SessionTypeWorkspace)))
	mode := runtimekernel.Mode(firstRuntimeReplayValue(s.mode, string(runtimekernel.ModeChat)))
	hostID := strings.TrimSpace(metadata["aiops.target.hostId"])
	if hostID == "" {
		hostID = runtimeReplayMentionHost(metadata["aiops.input.mentions.v1"])
	}
	intent := appui.BuildIntentFrame(text, appui.BuildEvidenceEnvelope(text, nil, nil), nil)
	req := runtimekernel.TurnRequest{SessionID: s.sessionID, SessionType: sessionType, Mode: mode, TurnID: "replay-turn-" + runtimeReplaySlug(messageID), ClientTurnID: "replay-client-turn-" + runtimeReplaySlug(messageID), ClientMessageID: messageID, Input: text, HostID: hostID, IntentFrame: &intent, Metadata: metadata}
	if intent.Kind == runtimecontract.IntentKindChange {
		req.PermissionProfile = runtimekernel.RuntimePermissionProfileApprovalRequired
		req.RollbackPolicy = runtimekernel.RuntimeRollbackPolicyActionContractRequired
	}
	return req
}
func (s *runtimeReplayChatService) SendMessage(ctx context.Context, command appui.ChatCommand) (appui.TurnResponse, error) {
	request := runtimeReplayRequest{Type: string(appui.TransportCommandTypeAddMessage), Message: &runtimeReplayMessage{ID: command.ClientMessageID, Metadata: command.Metadata}}
	request.Message.Parts = append(request.Message.Parts, struct {
		Text string `json:"text,omitempty"`
	}{Text: command.Content})
	if command.SessionType != "" {
		s.sessionType = command.SessionType
	}
	if command.Mode != "" {
		s.mode = command.Mode
	}
	result, err := s.kernel.RunTurn(ctx, s.turnRequest(request))
	return appui.TurnResponse{SessionID: result.SessionID, TurnID: result.TurnID, ClientTurnID: result.ClientTurnID, ClientMessageID: result.ClientMessageID, Status: result.Status, Output: result.Output, Error: result.Error}, err
}

type replaySyncApprovalService struct{ appui.ApprovalService }
type runtimeReplayArtifactCollector struct {
	mu        sync.Mutex
	artifacts []runtimekernel.ReplayArtifact
}

func (s *runtimeReplayArtifactCollector) CaptureReplayArtifact(_ context.Context, artifact runtimekernel.ReplayArtifact) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.artifacts = append(s.artifacts, artifact)
	return nil
}
func (s *runtimeReplayArtifactCollector) contract() ReplayContractArtifacts {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out ReplayContractArtifacts
	for _, artifact := range s.artifacts {
		if artifact.TurnAssembly != nil && out.TurnAssembly.Hash == "" {
			out.TurnAssembly = *artifact.TurnAssembly
		}
		if artifact.StepContext != nil {
			out.Steps = append(out.Steps, *artifact.StepContext)
		}
		if artifact.ActionToken != nil {
			out.ActionTokens = append(out.ActionTokens, *artifact.ActionToken)
		}
		if artifact.Final != nil {
			out.FinalRuntimeFacts = artifact.Final.RuntimeFacts
			out.FinalContract = artifact.Final.Contract
		}
	}
	return out
}

func runtimeReplayMessageData(message *runtimeReplayMessage) (string, string, map[string]string) {
	if message == nil {
		return "", "", map[string]string{}
	}
	var parts []string
	for _, part := range message.Parts {
		if strings.TrimSpace(part.Text) != "" {
			parts = append(parts, strings.TrimSpace(part.Text))
		}
	}
	metadata := make(map[string]string, len(message.Metadata))
	for key, value := range message.Metadata {
		metadata[key] = value
	}
	return strings.Join(parts, "\n"), strings.TrimSpace(message.ID), metadata
}
func runtimeReplayMentionHost(raw string) string {
	var value struct {
		Mentions []struct {
			Payload struct {
				HostID string `json:"hostId"`
			} `json:"payload"`
		} `json:"mentions"`
	}
	_ = json.Unmarshal([]byte(raw), &value)
	if len(value.Mentions) > 0 {
		return strings.TrimSpace(value.Mentions[0].Payload.HostID)
	}
	return ""
}
func currentRuntimeReplayApproval(session *runtimekernel.SessionState) (runtimekernel.PendingApproval, error) {
	if session != nil {
		if session.CurrentTurn != nil && len(session.CurrentTurn.PendingApprovals) > 0 {
			return session.CurrentTurn.PendingApprovals[0], nil
		}
		if len(session.PendingApprovals) > 0 {
			return session.PendingApprovals[0], nil
		}
	}
	return runtimekernel.PendingApproval{}, errors.New("runtime replay expected a pending approval")
}
func runtimeReplaySlug(value string) string {
	return strings.Trim(strings.ToLower(strings.NewReplacer(" ", "-", "/", "-", "_", "-").Replace(value)), "-")
}
func firstRuntimeReplayValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
