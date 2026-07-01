package appui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"aiops-v2/internal/hostops"
	"aiops-v2/internal/runtimecontract"
	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/workflowgen"
)

type defaultChatService struct {
	runtime            RuntimeGateway
	sessions           SessionSource
	hosts              HostRepository
	hostOps            HostOpsService
	agentEvents        AgentEventService
	runtimeSettings    RuntimeSettingsProvider
	turnRunner         AsyncTurnRunner
	baseContext        context.Context
	workflowGeneration *WorkflowGenerationChatService
}

type AsyncTurnRunner interface {
	Start(ctx context.Context, req runtimekernel.TurnRequest)
}

type defaultAsyncTurnRunner struct {
	runtime     RuntimeGateway
	agentEvents AgentEventService
	baseContext context.Context
}

func NewChatService(runtime RuntimeGateway, sessions SessionSource, agentEvents ...AgentEventService) ChatService {
	return NewChatServiceWithContext(context.Background(), runtime, sessions, agentEvents...)
}

func NewChatServiceWithContext(baseContext context.Context, runtime RuntimeGateway, sessions SessionSource, agentEvents ...AgentEventService) ChatService {
	return NewChatServiceWithContextAndHosts(baseContext, runtime, sessions, nil, agentEvents...)
}

func NewChatServiceWithHosts(runtime RuntimeGateway, sessions SessionSource, hosts HostRepository, agentEvents ...AgentEventService) ChatService {
	return NewChatServiceWithContextAndHosts(context.Background(), runtime, sessions, hosts, agentEvents...)
}

func NewChatServiceWithContextAndHosts(baseContext context.Context, runtime RuntimeGateway, sessions SessionSource, hosts HostRepository, agentEvents ...AgentEventService) ChatService {
	return NewChatServiceWithContextHostsAndHostOps(baseContext, runtime, sessions, hosts, nil, agentEvents...)
}

func NewChatServiceWithContextHostsAndHostOps(baseContext context.Context, runtime RuntimeGateway, sessions SessionSource, hosts HostRepository, hostOps HostOpsService, agentEvents ...AgentEventService) ChatService {
	return NewChatServiceWithContextHostsHostOpsAndRuntimeSettings(baseContext, runtime, sessions, hosts, hostOps, nil, agentEvents...)
}

func NewChatServiceWithContextHostsHostOpsAndRuntimeSettings(baseContext context.Context, runtime RuntimeGateway, sessions SessionSource, hosts HostRepository, hostOps HostOpsService, runtimeSettings RuntimeSettingsProvider, agentEvents ...AgentEventService) ChatService {
	var eventService AgentEventService
	if len(agentEvents) > 0 {
		eventService = agentEvents[0]
	}
	if eventService == nil {
		eventService = NewAgentEventService(nil)
	}
	baseContext = normalizeBaseContext(baseContext)
	var workflowGeneration *WorkflowGenerationChatService
	if sessionStore, ok := sessions.(SessionStore); ok {
		workflowGeneration = NewWorkflowGenerationChatService(sessionStore, workflowgen.NewMemorySessionStore(), workflowgen.DeterministicPlanBuilder{}, workflowgen.RunnerGraphGenerator{}, eventService, runtimeSettings)
	}
	return &defaultChatService{
		runtime:            runtime,
		sessions:           sessions,
		hosts:              hosts,
		hostOps:            hostOps,
		agentEvents:        eventService,
		runtimeSettings:    runtimeSettings,
		baseContext:        baseContext,
		workflowGeneration: workflowGeneration,
		turnRunner: defaultAsyncTurnRunner{
			runtime:     runtime,
			agentEvents: eventService,
			baseContext: baseContext,
		},
	}
}

func (s *defaultChatService) intentFrameRoutingMode(ctx context.Context) string {
	if s != nil && s.runtimeSettings != nil {
		return s.runtimeSettings.Snapshot(ctx).AgentRuntime.IntentFrameRouting
	}
	return intentFrameRoutingTraceOnly
}

func normalizeBaseContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func (s *defaultChatService) SendMessage(ctx context.Context, cmd ChatCommand) (TurnResponse, error) {
	content := strings.TrimSpace(cmd.Content)
	if req, ok := s.buildPendingEvidenceResumeRequest(cmd, content); ok {
		result, err := s.runtime.ResumeTurn(ctx, req)
		if err != nil {
			return TurnResponse{}, err
		}
		return mapTurnResponse(result), nil
	}
	sessionID := strings.TrimSpace(cmd.SessionID)
	var commandSession *runtimekernel.SessionState
	if session := s.resolveCommandSession(sessionID); session != nil {
		commandSession = session
		if sessionID == "" {
			sessionID = session.ID
		}
		if strings.TrimSpace(cmd.SessionType) == "" {
			cmd.SessionType = string(session.Type)
		}
		if strings.TrimSpace(cmd.Mode) == "" {
			cmd.Mode = string(session.Mode)
		}
	}
	selectedHostID := s.resolveSelectedHostContextID(ctx, firstNonEmptyString(cmd.HostID, sessionHostID(commandSession)))
	if selectedHostID != "" && commandSession != nil && strings.TrimSpace(commandSession.HostID) != selectedHostID {
		commandSession.HostID = selectedHostID
		if writer, ok := s.sessions.(SessionStore); ok {
			writer.Update(commandSession)
		}
	}
	req := runtimekernel.TurnRequest{
		SessionType:     mapSessionType(cmd.SessionType),
		Mode:            mapMode(cmd.Mode),
		SessionID:       sessionID,
		TurnID:          fmt.Sprintf("turn-%d", time.Now().UnixNano()),
		ClientTurnID:    strings.TrimSpace(cmd.ClientTurnID),
		ClientMessageID: strings.TrimSpace(cmd.ClientMessageID),
		Input:           content,
		HostID:          cmd.HostID,
		Metadata:        cloneStringMetadata(cmd.Metadata),
	}
	requestedSessionType := req.SessionType
	requestedMode := req.Mode
	evidence := ExtractUserEvidence(content)
	structuredMentions := parseInputMentions(content, cmd.Metadata)
	mentions := s.hostOpsMentionsForCommand(ctx, cmd, content, &structuredMentions)
	mentionSource, mentionValidation := mentionSourceForCommand(cmd, content, structuredMentions, mentions)
	route := BuildChatRuntimeRoute(content, mentions, evidence)
	envelope := BuildEvidenceEnvelope(content, nil, nil)
	intentFrame := BuildIntentFrame(content, envelope, nil)
	intentRoute := BuildChatRuntimeRouteFromIntentFrame(intentFrame, route)
	activeRoute, routingMode := selectActiveChatRuntimeRoute(route, intentRoute, intentFrame, s.intentFrameRoutingMode(ctx))
	applyStructuredMentionRouteHints(&activeRoute, structuredMentions)
	if selectedHostContextShouldUseHostBoundRoute(activeRoute, intentFrame, selectedHostID) {
		activeRoute = promoteSelectedHostContextRoute(activeRoute, selectedHostID)
		mentions = appendSelectedHostContextMention(mentions, selectedHostID)
	}
	applyChatRuntimeRouteMetadata(&req, activeRoute)
	applyIntentFrameRouteMetadata(&req, route, intentRoute, activeRoute, intentFrame, routingMode)
	req.Metadata["aiops.route.activeSource"] = routingMode
	applyChatRuntimeToolSurfaceMetadata(&req, activeRoute)
	applyStructuredCapabilityMetadata(req.Metadata, structuredMentions)
	applyInputMentionDiagnosticValues(&req, mentionSource, mentionValidation)
	applyUserEvidenceMetadata(&req, evidence)
	applyFollowupPromptProfileMetadata(&req, commandSession, content, evidence)
	applyChatRuntimeRouteHostBinding(&req, activeRoute, mentions)
	applyExplicitSelectedHostContext(&req, activeRoute, selectedHostID, requestedSessionType, requestedMode)
	s.applyHostOpsTurnMetadata(ctx, cmd, &req, &structuredMentions)
	if req.SessionID == "" {
		req.SessionID = strings.TrimSpace(cmd.SessionID)
	}
	if req.SessionID == "" {
		req.SessionID = fmt.Sprintf("sess-%d", time.Now().UnixNano())
	}
	opsRun := ensureOpsRunMetadata(&req)
	ensureCorootRCAMetadata(&req)
	s.enrichTurnHostMetadata(&req)
	if s.workflowGeneration != nil {
		if response, handled, err := s.workflowGeneration.Handle(ctx, cmd, req); handled || err != nil {
			response = attachOpsRunToTurnResponse(response, opsRun)
			return response, err
		}
	}
	if response, handled, err := s.handleGenericOpsRepair(ctx, cmd, req); handled || err != nil {
		response = attachOpsRunToTurnResponse(response, opsRun)
		return response, err
	}
	if chatSessionHasRunningRegularTurn(commandSession) {
		result, err := s.runtime.RunTurn(ctx, req)
		response := mapTurnResponse(result)
		response = attachOpsRunToTurnResponse(response, opsRun)
		return response, err
	}
	s.appendTurnAcceptedEvents(req)
	s.turnRunner.Start(ctx, req)
	return TurnResponse{
		SessionID:       req.SessionID,
		TurnID:          req.TurnID,
		ClientTurnID:    req.ClientTurnID,
		ClientMessageID: req.ClientMessageID,
		Status:          "accepted",
		OpsRun:          &opsRun,
	}, nil
}

func chatSessionHasRunningRegularTurn(session *runtimekernel.SessionState) bool {
	return session != nil &&
		session.CurrentTurn != nil &&
		session.CurrentTurn.Lifecycle == runtimekernel.TurnLifecycleRunning &&
		session.CurrentTurn.ResumeState == runtimekernel.TurnResumeStateNone
}

func (s *defaultChatService) applyHostOpsTurnMetadata(ctx context.Context, cmd ChatCommand, req *runtimekernel.TurnRequest, structured *parsedInputMentions) {
	if s == nil || req == nil {
		return
	}
	if routeMode := strings.TrimSpace(req.Metadata["aiops.route.mode"]); routeMode != "" && routeMode != string(ChatRouteMultiHostOps) {
		return
	}
	mentions := s.hostOpsMentionsForCommand(ctx, cmd, req.Input, structured)
	decision := hostops.DetectRoute(req.Input, mentions)
	if decision.Kind != hostops.RouteKindHostOps {
		return
	}
	if req.Metadata == nil {
		req.Metadata = map[string]string{}
	}
	missionID := "hostops:" + strings.TrimSpace(req.TurnID)
	req.Metadata["aiops.hostops.routeKind"] = string(decision.Kind)
	req.Metadata["aiops.hostops.planRequired"] = boolMetadataString(decision.PlanRequired)
	req.Metadata["aiops.hostops.serverDetectedMultiHost"] = boolMetadataString(decision.PlanRequired)
	req.Metadata["aiops.hostops.missionId"] = missionID
	req.Metadata["aiops.hostops.managerAgentId"] = firstNonEmptyString(strings.TrimSpace(req.Metadata["aiops.hostops.managerAgentId"]), "hostops-manager:"+req.TurnID)
	applyHostOpsManagerRuntimeMetadata(req.Metadata)
	if serialized, err := json.Marshal(decision.Mentions); err == nil {
		req.Metadata["aiops.hostops.mentions"] = string(serialized)
	}
}

func (s *defaultChatService) hostOpsMentionsForCommand(ctx context.Context, cmd ChatCommand, content string, structured *parsedInputMentions) []hostops.HostMention {
	if structured != nil && structured.Present {
		if structured.Invalid {
			return nil
		}
		mentions := inputMentionHostHintsToHostMentions(structured.Hosts)
		if len(mentions) == 0 {
			return nil
		}
		if s == nil || s.hosts == nil {
			markInputMentionsInvalid(structured)
			return nil
		}
		resolved, errs := hostops.NewResolver(hostRepositoryLookup{repo: s.hosts}).Resolve(ctx, mentions)
		if len(errs) > 0 {
			markInputMentionsInvalid(structured)
			return nil
		}
		filtered := filterHostOpsRouteMentions(resolved)
		if len(filtered) == 0 {
			markInputMentionsInvalid(structured)
			return nil
		}
		return filtered
	}
	mentions := hostOpsMentionsFromMetadata(cmd.Metadata["aiops.hostops.mentions"])
	if len(mentions) == 0 {
		if inputMentionStrictMode() {
			return nil
		}
		mentions = hostops.ParseHostMentions(content)
	}
	if len(mentions) == 0 {
		return nil
	}
	if s == nil || s.hosts == nil {
		return filterHostOpsRouteMentions(mentions)
	}
	resolved, _ := hostops.NewResolver(hostRepositoryLookup{repo: s.hosts}).Resolve(ctx, mentions)
	return filterHostOpsRouteMentions(resolved)
}

func (s *defaultChatService) resolveSelectedHostContextID(ctx context.Context, candidate string) string {
	candidate = strings.TrimSpace(strings.TrimPrefix(candidate, "@"))
	if candidate == "" {
		return ""
	}
	if candidate == serverLocalHostID || strings.EqualFold(candidate, "local") {
		return serverLocalHostID
	}
	if s == nil || s.hosts == nil {
		return candidate
	}
	mention := hostops.HostMention{
		Raw:         "@" + candidate,
		HostID:      candidate,
		Address:     candidate,
		DisplayName: candidate,
		Source:      hostops.HostMentionSourceHostnameLiteral,
		Resolved:    true,
		Confidence:  0.75,
		CreatedAt:   time.Now(),
	}
	resolved, errs := hostops.NewResolver(hostRepositoryLookup{repo: s.hosts}).Resolve(ctx, []hostops.HostMention{mention})
	if len(errs) > 0 || len(resolved) == 0 {
		return ""
	}
	return strings.TrimSpace(resolved[0].HostID)
}

func selectedHostContextShouldUseHostBoundRoute(route ChatRuntimeRoute, frame runtimecontract.IntentFrame, selectedHostID string) bool {
	if strings.TrimSpace(selectedHostID) == "" || route.UserProhibitedHostExec || route.EnvironmentReadOnlyReason != "" {
		return false
	}
	if route.Mode != ChatRouteAdvisory {
		return false
	}
	frame = runtimecontract.NormalizeIntentFrame(frame)
	return runtimecontract.ContainsDataScope(frame.DataScopes, runtimecontract.DataScopeLocalRuntime) ||
		runtimecontract.ContainsActionRisk(frame.RiskBudget, runtimecontract.ActionRiskHostExec)
}

func promoteSelectedHostContextRoute(route ChatRuntimeRoute, selectedHostID string) ChatRuntimeRoute {
	route.Mode = ChatRouteHostBoundOps
	route.RequiresHostBinding = true
	route.AllowsExecCommand = true
	route.Reasons = appendUniqueEvidenceString(route.Reasons, "selected host context: "+strings.TrimSpace(selectedHostID))
	if strings.TrimSpace(route.Confidence) == "" {
		route.Confidence = "medium"
	}
	return route
}

func appendSelectedHostContextMention(mentions []hostops.HostMention, selectedHostID string) []hostops.HostMention {
	selectedHostID = strings.TrimSpace(selectedHostID)
	if selectedHostID == "" {
		return mentions
	}
	for _, mention := range mentions {
		if strings.EqualFold(strings.TrimSpace(mention.HostID), selectedHostID) {
			return mentions
		}
	}
	return append(mentions, hostops.HostMention{
		Raw:         "@" + selectedHostID,
		HostID:      selectedHostID,
		DisplayName: selectedHostID,
		Source:      hostops.HostMentionSourceInventory,
		Resolved:    true,
		Confidence:  1,
		CreatedAt:   time.Now(),
	})
}

func sessionHostID(session *runtimekernel.SessionState) string {
	if session == nil {
		return ""
	}
	return strings.TrimSpace(session.HostID)
}

func applyInputMentionDiagnostics(req *runtimekernel.TurnRequest, mentions parsedInputMentions) {
	applyInputMentionDiagnosticValues(req, mentions.Source, mentions.Validation)
}

func applyInputMentionDiagnosticValues(req *runtimekernel.TurnRequest, source, validation string) {
	if req == nil {
		return
	}
	if req.Metadata == nil {
		req.Metadata = map[string]string{}
	}
	if source != "" {
		req.Metadata["aiops.input.mentionSource"] = source
	}
	if validation != "" {
		req.Metadata["aiops.input.mentionValidation"] = validation
	}
}

func markInputMentionsInvalid(mentions *parsedInputMentions) {
	if mentions == nil {
		return
	}
	mentions.Invalid = true
	mentions.Validation = "invalid"
}

func applyStructuredMentionRouteHints(route *ChatRuntimeRoute, mentions parsedInputMentions) {
	if route == nil || !mentions.Present || mentions.Invalid {
		return
	}
	if mentions.HasCapability("coroot") {
		route.AllowsCorootRCA = true
		route.Reasons = appendUniqueEvidenceString(route.Reasons, "structured capability: coroot")
	}
}

func applyStructuredCapabilityMetadata(metadata map[string]string, mentions parsedInputMentions) {
	if metadata == nil || !mentions.Present || mentions.Invalid {
		return
	}
	if mentions.HasCapability("ops_graph") {
		metadata["aiops.opsGraph.explicitMention"] = "true"
		metadata["enableToolPack"] = appendMetadataListValue(metadata["enableToolPack"], "opsgraph")
	}
	if mentions.HasCapability("ops_manuals") {
		metadata["aiops.opsManuals.explicitMention"] = "true"
		metadata["enableToolPack"] = appendMetadataListValue(metadata["enableToolPack"], "ops_manual_flow")
		metadata["enableTool"] = appendMetadataListValue(metadata["enableTool"], "search_ops_manuals")
	}
	for _, resource := range mentions.Resources {
		switch strings.TrimSpace(resource.Kind) {
		case "ops_graph":
			if id := strings.TrimSpace(resource.ID); id != "" {
				metadata["aiops.opsGraph.graphId"] = id
			}
			if title := strings.TrimSpace(resource.Title); title != "" {
				metadata["aiops.opsGraph.graphName"] = title
			}
		case "ops_manual":
			if id := strings.TrimSpace(resource.ID); id != "" {
				metadata["opsManualManualId"] = id
				metadata["manualId"] = id
			}
			if title := strings.TrimSpace(resource.Title); title != "" {
				metadata["opsManualManualTitle"] = title
			}
		}
	}
}

func mentionSourceForCommand(cmd ChatCommand, content string, structured parsedInputMentions, mentions []hostops.HostMention) (string, string) {
	if structured.Present {
		return structured.Source, structured.Validation
	}
	if strings.TrimSpace(cmd.Metadata["aiops.hostops.mentions"]) != "" {
		if len(mentions) > 0 {
			return "legacy_hostops_metadata", "confirmed"
		}
		return "legacy_hostops_metadata", "invalid"
	}
	rawMentions := hostops.ParseHostMentions(content)
	if len(rawMentions) > 0 {
		if inputMentionStrictMode() {
			return "raw_text_fallback", "weak"
		}
		if len(mentions) > 0 {
			return "raw_text_fallback", "confirmed"
		}
		return "raw_text_fallback", "invalid"
	}
	return "absent", "absent"
}

func inputMentionStrictMode() bool {
	value := strings.TrimSpace(os.Getenv("AIOPS_INPUT_MENTION_STRICT"))
	return strings.EqualFold(value, "1") || strings.EqualFold(value, "true")
}

func filterHostOpsRouteMentions(mentions []hostops.HostMention) []hostops.HostMention {
	out := make([]hostops.HostMention, 0, len(mentions))
	for _, mention := range mentions {
		if mention.Source == hostops.HostMentionSourceLocalAlias {
			mention.HostID = firstNonEmptyString(strings.TrimSpace(mention.HostID), serverLocalHostID)
			mention.DisplayName = firstNonEmptyString(strings.TrimSpace(mention.DisplayName), "local")
			mention.Resolved = true
			out = append(out, mention)
			continue
		}
		if strings.TrimSpace(mention.HostID) != "" || mention.Resolved {
			out = append(out, mention)
			continue
		}
		if mention.Source == hostops.HostMentionSourceIPLiteral && strings.TrimSpace(mention.Address) != "" {
			out = append(out, mention)
		}
	}
	return out
}

type hostMentionMetadataItem struct {
	TokenID     string  `json:"tokenId"`
	Raw         string  `json:"raw"`
	Value       string  `json:"value"`
	Start       int     `json:"start"`
	End         int     `json:"end"`
	SpanStart   int     `json:"spanStart"`
	SpanEnd     int     `json:"spanEnd"`
	HostID      string  `json:"hostId"`
	Address     string  `json:"address"`
	DisplayName string  `json:"displayName"`
	Source      string  `json:"source"`
	Resolved    bool    `json:"resolved"`
	Confidence  float64 `json:"confidence"`
}

func hostOpsMentionsFromMetadata(raw string) []hostops.HostMention {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var items []hostMentionMetadataItem
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil
	}
	now := time.Now().UTC()
	mentions := make([]hostops.HostMention, 0, len(items))
	for _, item := range items {
		value := strings.TrimSpace(item.Value)
		mentionRaw := strings.TrimSpace(item.Raw)
		if mentionRaw == "" && value != "" {
			mentionRaw = "@" + strings.TrimPrefix(value, "@")
		}
		if mentionRaw == "" && strings.TrimSpace(item.HostID) == "" && strings.TrimSpace(item.Address) == "" {
			continue
		}
		spanStart := item.SpanStart
		if spanStart == 0 {
			spanStart = item.Start
		}
		spanEnd := item.SpanEnd
		if spanEnd == 0 {
			spanEnd = item.End
		}
		displayName := strings.TrimSpace(item.DisplayName)
		if displayName == "" && value != "" {
			displayName = strings.TrimPrefix(value, "@")
		}
		source := hostops.HostMentionSource(strings.TrimSpace(item.Source))
		if source == "" {
			source = hostops.HostMentionSourceHostnameLiteral
		}
		confidence := item.Confidence
		if confidence == 0 {
			confidence = 0.75
		}
		mentions = append(mentions, hostops.HostMention{
			TokenID:     strings.TrimSpace(item.TokenID),
			Raw:         mentionRaw,
			SpanStart:   spanStart,
			SpanEnd:     spanEnd,
			HostID:      strings.TrimSpace(item.HostID),
			Address:     strings.TrimSpace(item.Address),
			DisplayName: displayName,
			Source:      source,
			Resolved:    item.Resolved,
			Confidence:  confidence,
			CreatedAt:   now,
		})
	}
	return mentions
}

type hostRepositoryLookup struct {
	repo HostRepository
}

func (l hostRepositoryLookup) ListHosts(context.Context) ([]hostops.HostRecordView, error) {
	if l.repo == nil {
		return nil, nil
	}
	records, err := l.repo.ListHosts()
	if err != nil {
		return nil, err
	}
	hosts := make([]hostops.HostRecordView, 0, len(records))
	for _, record := range records {
		hosts = append(hosts, hostops.HostRecordView{
			ID:          strings.TrimSpace(record.ID),
			Address:     strings.TrimSpace(record.Address),
			Hostname:    strings.TrimSpace(record.Name),
			DisplayName: strings.TrimSpace(record.Name),
			Managed:     strings.TrimSpace(record.AgentURL) != "",
			Executable:  record.Executable,
			AgentURL:    strings.TrimSpace(record.AgentURL),
		})
	}
	return hosts, nil
}

func (s *defaultChatService) enrichTurnHostMetadata(req *runtimekernel.TurnRequest) {
	if s == nil || s.hosts == nil || req == nil || strings.TrimSpace(req.HostID) == "" {
		return
	}
	host, err := s.hosts.GetHost(strings.TrimSpace(req.HostID))
	if err != nil || host == nil {
		return
	}
	if req.Metadata == nil {
		req.Metadata = map[string]string{}
	}
	setMetadataIfEmpty(req.Metadata, "aiops.host.metadataAvailable", "true")
	setMetadataIfEmpty(req.Metadata, "aiops.host.id", host.ID)
	setMetadataIfEmpty(req.Metadata, "aiops.host.label", firstNonEmpty(host.Name, host.ID))
	setMetadataIfEmpty(req.Metadata, "aiops.host.os", host.OS)
	setMetadataIfEmpty(req.Metadata, "aiops.host.arch", host.Arch)
	setMetadataIfEmpty(req.Metadata, "aiops.host.transport", host.Transport)
	setMetadataIfEmpty(req.Metadata, "aiops.host.status", host.Status)
	setMetadataIfEmpty(req.Metadata, "aiops.host.agentStatus", hostAgentStatus(*host))
	setMetadataIfEmpty(req.Metadata, "aiops.host.sshStatus", hostSSHStatus(*host))
	setMetadataIfEmpty(req.Metadata, "aiops.host.runtimeReachability", hostRuntimeReachability(*host))
	setMetadataIfEmpty(req.Metadata, "aiops.host.address", host.Address)
	setMetadataIfEmpty(req.Metadata, "aiops.host.sshUser", host.SSHUser)
	if host.SSHPort > 0 {
		setMetadataIfEmpty(req.Metadata, "aiops.host.sshPort", strconv.Itoa(host.SSHPort))
	}
}

func setMetadataIfEmpty(metadata map[string]string, key, value string) {
	value = strings.TrimSpace(value)
	if value == "" || strings.TrimSpace(metadata[key]) != "" {
		return
	}
	metadata[key] = value
}

func applyFollowupPromptProfileMetadata(req *runtimekernel.TurnRequest, session *runtimekernel.SessionState, input string, evidence UserEvidenceExtraction) {
	if req == nil || session == nil || !shortFollowupInput(input) || evidence.HasEvidence || !sessionHasExistingEvidenceContext(session) {
		return
	}
	if req.Metadata == nil {
		req.Metadata = map[string]string{}
	}
	setMetadataIfEmpty(req.Metadata, metadataTurnFollowup, "true")
	setMetadataIfEmpty(req.Metadata, metadataTurnHasExistingEvidence, "true")
	setMetadataIfEmpty(req.Metadata, metadataTurnNoNewEvidence, "true")
	setMetadataIfEmpty(req.Metadata, "reasoningEffort", "low")
	setMetadataIfEmpty(req.Metadata, "answerStyle", "concise")
}

func shortFollowupInput(input string) bool {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return false
	}
	if utf8.RuneCountInString(trimmed) > 80 {
		return false
	}
	return len(strings.Fields(trimmed)) <= 20
}

func sessionHasExistingEvidenceContext(session *runtimekernel.SessionState) bool {
	if session == nil {
		return false
	}
	for _, msg := range session.Messages {
		switch strings.TrimSpace(msg.Role) {
		case "assistant", "tool", "system":
			if strings.TrimSpace(msg.Content) != "" || msg.ToolResult != nil || len(msg.ToolCalls) > 0 {
				return true
			}
		}
	}
	if session.CurrentTurn != nil {
		if strings.TrimSpace(session.CurrentTurn.FinalOutput) != "" || len(session.CurrentTurn.AgentItems) > 0 || len(session.CurrentTurn.ExternalReferences) > 0 {
			return true
		}
	}
	for _, turn := range session.TurnHistory {
		if strings.TrimSpace(turn.FinalOutput) != "" || len(turn.AgentItems) > 0 || len(turn.ExternalReferences) > 0 {
			return true
		}
	}
	return false
}

func (s *defaultChatService) appendTurnAcceptedEvents(req runtimekernel.TurnRequest) {
	if s == nil || s.agentEvents == nil {
		return
	}
	ctx := normalizeBaseContext(s.baseContext)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	turnPayload, _ := json.Marshal(TurnPayload{
		Prompt:          req.Input,
		Title:           req.Input,
		ClientMessageID: req.ClientMessageID,
		ClientTurnID:    req.ClientTurnID,
		Mode:            string(req.Mode),
		ReasoningEffort: req.Metadata["reasoningEffort"],
	})
	_, _ = s.agentEvents.Append(ctx, AgentEvent{
		EventID:      fmt.Sprintf("%s:turn.requested", req.TurnID),
		SessionID:    req.SessionID,
		TurnID:       req.TurnID,
		ClientTurnID: req.ClientTurnID,
		Kind:         AgentEventTurn,
		Phase:        AgentEventPhaseRequested,
		Status:       AgentEventStatusQueued,
		Visibility:   AgentEventVisibilityPrimary,
		Source:       AgentEventSourceUI,
		CreatedAt:    now,
		Payload:      turnPayload,
	})
	appendMainAgentEvent(ctx, s.agentEvents, req, AgentEventPhaseStarted, AgentEventStatusRunning, "正在启动 Agent", "")
}

func (r defaultAsyncTurnRunner) Start(_ context.Context, req runtimekernel.TurnRequest) {
	go r.run(req)
}

func (r defaultAsyncTurnRunner) run(req runtimekernel.TurnRequest) {
	if r.runtime == nil {
		return
	}
	ctx := normalizeBaseContext(r.baseContext)
	defer func() {
		if recovered := recover(); recovered != nil {
			appendMainAgentEvent(ctx, r.agentEvents, req, AgentEventPhaseFailed, AgentEventStatusFailed, "", fmt.Sprintf("panic: %v", recovered))
			appendTerminalAgentEvent(ctx, r.agentEvents, req, AgentEventPhaseFailed, AgentEventStatusFailed, fmt.Sprintf("panic: %v", recovered))
		}
	}()
	result, err := r.runtime.RunTurn(ctx, req)
	if err != nil {
		appendMainAgentEvent(ctx, r.agentEvents, req, AgentEventPhaseFailed, AgentEventStatusFailed, "", err.Error())
		appendTerminalAgentEvent(ctx, r.agentEvents, req, AgentEventPhaseFailed, AgentEventStatusFailed, err.Error())
		return
	}
	if strings.EqualFold(result.Status, "cancelled") || strings.EqualFold(result.Status, string(AgentEventStatusCanceled)) {
		return
	}
	if strings.EqualFold(result.Status, "pending_input") {
		return
	}
	if strings.EqualFold(result.Status, "blocked") || strings.EqualFold(result.Status, string(AgentEventStatusBlocked)) {
		appendMainAgentEvent(ctx, r.agentEvents, req, AgentEventPhaseBlocked, AgentEventStatusBlocked, "", strings.TrimSpace(result.Error))
		return
	}
	if strings.EqualFold(result.Status, "failed") || strings.TrimSpace(result.Error) != "" {
		appendMainAgentEvent(ctx, r.agentEvents, req, AgentEventPhaseFailed, AgentEventStatusFailed, "", strings.TrimSpace(result.Error))
		appendTerminalAgentEvent(ctx, r.agentEvents, req, AgentEventPhaseFailed, AgentEventStatusFailed, strings.TrimSpace(result.Error))
		return
	}
	appendMainAgentEvent(ctx, r.agentEvents, req, AgentEventPhaseCompleted, AgentEventStatusCompleted, "", "任务已完成")
}

func appendTerminalAgentEvent(ctx context.Context, agentEvents AgentEventService, req runtimekernel.TurnRequest, phase AgentEventPhase, status AgentEventStatus, message string) {
	if agentEvents == nil {
		return
	}
	ctx = normalizeBaseContext(ctx)
	payload, _ := json.Marshal(TurnPayload{Error: message, Summary: message})
	_, _ = agentEvents.Append(ctx, AgentEvent{
		EventID:      fmt.Sprintf("%s:turn.%s.async", req.TurnID, phase),
		SessionID:    req.SessionID,
		TurnID:       req.TurnID,
		ClientTurnID: req.ClientTurnID,
		Kind:         AgentEventTurn,
		Phase:        phase,
		Status:       status,
		Visibility:   AgentEventVisibilityPrimary,
		Source:       AgentEventSourceSystem,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339Nano),
		Payload:      payload,
	})
}

func appendMainAgentEvent(ctx context.Context, agentEvents AgentEventService, req runtimekernel.TurnRequest, phase AgentEventPhase, status AgentEventStatus, lastAction string, lastSummary string) {
	if agentEvents == nil {
		return
	}
	payload, _ := json.Marshal(AgentPayload{
		Handle:      "main",
		Name:        "Main Agent",
		Role:        "primary",
		LastAction:  strings.TrimSpace(lastAction),
		LastSummary: strings.TrimSpace(lastSummary),
	})
	_, _ = agentEvents.Append(ctx, AgentEvent{
		EventID:      fmt.Sprintf("%s:agent.main.%s", req.TurnID, phase),
		SessionID:    req.SessionID,
		TurnID:       req.TurnID,
		ClientTurnID: req.ClientTurnID,
		AgentID:      "agent-main",
		Kind:         AgentEventAgent,
		Phase:        phase,
		Status:       status,
		Visibility:   AgentEventVisibilitySecondary,
		Source:       AgentEventSourceSystem,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339Nano),
		Payload:      payload,
	})
}

func (s *defaultChatService) appendTerminalAgentEvent(req runtimekernel.TurnRequest, phase AgentEventPhase, status AgentEventStatus, message string) {
	if s == nil {
		return
	}
	appendTerminalAgentEvent(s.baseContext, s.agentEvents, req, phase, status, message)
}

func (s *defaultChatService) buildPendingEvidenceResumeRequest(cmd ChatCommand, content string) (runtimekernel.ResumeRequest, bool) {
	if s == nil || s.sessions == nil || content == "" {
		return runtimekernel.ResumeRequest{}, false
	}
	session := s.resolveCommandSession(cmd.SessionID)
	if session == nil || session.CurrentTurn == nil {
		return runtimekernel.ResumeRequest{}, false
	}
	turn := session.CurrentTurn
	if !turn.Lifecycle.CanResume() {
		return runtimekernel.ResumeRequest{}, false
	}
	evidence, ok := firstPendingEvidence(turn, session)
	if !ok && turn.ResumeState != runtimekernel.TurnResumeStatePendingEvidence {
		return runtimekernel.ResumeRequest{}, false
	}
	metadata := cloneStringMetadata(cmd.Metadata)
	metadata["resume.input"] = content
	if evidence.ID != "" {
		metadata["evidence.id"] = evidence.ID
	}
	if evidence.ToolCallID != "" {
		metadata["evidence.toolCallId"] = evidence.ToolCallID
	}
	if evidence.ToolName != "" {
		metadata["evidence.toolName"] = evidence.ToolName
	}
	return runtimekernel.ResumeRequest{
		SessionID:    session.ID,
		TurnID:       turn.ID,
		CheckpointID: evidence.ID,
		ResumeState:  runtimekernel.TurnResumeStatePendingEvidence,
		Metadata:     metadata,
	}, true
}

func (s *defaultChatService) resolveCommandSession(sessionID string) *runtimekernel.SessionState {
	if s == nil || s.sessions == nil {
		return nil
	}
	targetID := strings.TrimSpace(sessionID)
	if targetID != "" {
		return s.sessions.Get(targetID)
	}
	return s.sessions.GetLatest()
}

func firstPendingEvidence(turn *runtimekernel.TurnSnapshot, session *runtimekernel.SessionState) (runtimekernel.PendingEvidence, bool) {
	if turn != nil && len(turn.PendingEvidence) > 0 {
		return turn.PendingEvidence[0], true
	}
	if session != nil && len(session.PendingEvidence) > 0 {
		return session.PendingEvidence[0], true
	}
	return runtimekernel.PendingEvidence{}, false
}

func cloneStringMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(metadata)+4)
	for key, value := range metadata {
		if trimmed := strings.TrimSpace(key); trimmed != "" {
			out[trimmed] = value
		}
	}
	return out
}

func (s *defaultChatService) ResumeTurn(ctx context.Context, cmd ResumeCommand) (TurnResponse, error) {
	result, err := s.runtime.ResumeTurn(ctx, runtimekernel.ResumeRequest{
		SessionID:    cmd.SessionID,
		TurnID:       cmd.TurnID,
		ApprovalID:   cmd.ApprovalID,
		CheckpointID: cmd.CheckpointID,
		ResumeState:  runtimekernel.TurnResumeState(cmd.ResumeState),
		Decision:     cmd.Decision,
		Metadata:     cmd.Metadata,
	})
	if err != nil {
		return TurnResponse{}, err
	}
	return mapTurnResponse(result), nil
}

func (s *defaultChatService) CancelTurn(ctx context.Context, cmd CancelCommand) (TurnResponse, error) {
	result, err := s.runtime.CancelTurn(ctx, runtimekernel.CancelRequest{
		SessionID: cmd.SessionID,
		TurnID:    cmd.TurnID,
		Reason:    cmd.Reason,
	})
	if err != nil {
		return TurnResponse{}, err
	}
	s.appendCanceledEvent(result, cmd.SessionID, cmd.TurnID, "任务已取消")
	return mapTurnResponse(result), nil
}

func (s *defaultChatService) StopTurn(ctx context.Context, cmd StopCommand) (TurnResponse, error) {
	if sessionID := strings.TrimSpace(cmd.SessionID); sessionID != "" {
		if turnID := strings.TrimSpace(cmd.TurnID); turnID != "" {
			result, err := s.runtime.CancelTurn(ctx, runtimekernel.CancelRequest{
				SessionID: sessionID,
				TurnID:    turnID,
				Reason:    cmd.Reason,
			})
			if err != nil {
				return TurnResponse{}, err
			}
			s.appendCanceledEvent(result, sessionID, turnID, "任务已停止")
			return mapTurnResponse(result), nil
		}
	}
	session, turn, err := resolveTurnTarget(s.sessions, cmd.SessionID, cmd.TurnID)
	if err != nil {
		return TurnResponse{}, err
	}
	result, err := s.runtime.CancelTurn(ctx, runtimekernel.CancelRequest{
		SessionID: session.ID,
		TurnID:    turn.ID,
		Reason:    cmd.Reason,
	})
	if err != nil {
		return TurnResponse{}, err
	}
	s.appendCanceledEvent(result, session.ID, turn.ID, "任务已停止")
	return mapTurnResponse(result), nil
}

func (s *defaultChatService) appendCanceledEvent(result runtimekernel.TurnResult, fallbackSessionID string, fallbackTurnID string, message string) {
	if s == nil {
		return
	}
	if !strings.EqualFold(result.Status, "cancelled") && !strings.EqualFold(result.Status, string(AgentEventStatusCanceled)) {
		return
	}
	sessionID := strings.TrimSpace(result.SessionID)
	if sessionID == "" {
		sessionID = strings.TrimSpace(fallbackSessionID)
	}
	turnID := strings.TrimSpace(result.TurnID)
	if turnID == "" {
		turnID = strings.TrimSpace(fallbackTurnID)
	}
	if sessionID == "" || turnID == "" {
		return
	}
	ctx := normalizeBaseContext(s.baseContext)
	appendMainAgentEvent(ctx, s.agentEvents, runtimekernel.TurnRequest{
		SessionID:       sessionID,
		TurnID:          turnID,
		ClientTurnID:    result.ClientTurnID,
		ClientMessageID: result.ClientMessageID,
	}, AgentEventPhaseCanceled, AgentEventStatusCanceled, "", message)
	s.appendTerminalAgentEvent(runtimekernel.TurnRequest{
		SessionID:       sessionID,
		TurnID:          turnID,
		ClientTurnID:    result.ClientTurnID,
		ClientMessageID: result.ClientMessageID,
	}, AgentEventPhaseCanceled, AgentEventStatusCanceled, message)
}

func mapSessionType(value string) runtimekernel.SessionType {
	if value == string(runtimekernel.SessionTypeWorkspace) {
		return runtimekernel.SessionTypeWorkspace
	}
	return runtimekernel.SessionTypeHost
}

func mapMode(value string) runtimekernel.Mode {
	switch value {
	case string(runtimekernel.ModeInspect):
		return runtimekernel.ModeInspect
	case string(runtimekernel.ModePlan):
		return runtimekernel.ModePlan
	case string(runtimekernel.ModeExecute):
		return runtimekernel.ModeExecute
	default:
		return runtimekernel.ModeChat
	}
}

func mapTurnResponse(result runtimekernel.TurnResult) TurnResponse {
	return TurnResponse{
		SessionID:       result.SessionID,
		TurnID:          result.TurnID,
		ClientTurnID:    result.ClientTurnID,
		ClientMessageID: result.ClientMessageID,
		Status:          result.Status,
		Output:          result.Output,
		Error:           result.Error,
	}
}

func attachOpsRunToTurnResponse(response TurnResponse, opsRun ChatRunTraceView) TurnResponse {
	if response.OpsRun != nil || strings.TrimSpace(opsRun.ID) == "" {
		return response
	}
	response.OpsRun = &opsRun
	return response
}
