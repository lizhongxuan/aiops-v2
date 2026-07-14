package appui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"aiops-v2/internal/hostops"
	"aiops-v2/internal/resourcebinding"
	"aiops-v2/internal/runtimecontract"
	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/specialinputmemory"
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

type systemTurnRuntimeGateway interface {
	CommitSystemTurn(ctx context.Context, req runtimekernel.SystemTurnRequest) (runtimekernel.TurnResult, error)
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
	specialInputObservations := inputMentionsToSpecialInputObservations(structuredMentions)
	specialInputObservations = appendRawCorootCapabilityObservation(content, structuredMentions, specialInputObservations)
	specialInputIntent := specialInputIntentFromContent(content, specialInputObservations, cmd.Metadata, commandSession)
	if commandSession != nil {
		commandSession.SpecialInputMemory, _ = specialinputmemory.Consolidate(commandSession.SpecialInputMemory, specialinputmemory.ConsolidateInput{
			SessionID: commandSession.ID,
			TaskID:    firstNonEmptyString(strings.TrimSpace(cmd.ClientTurnID), commandSession.ID),
			TurnID:    req.TurnID,
			Now:       time.Now().UTC(),
			Mentions:  specialInputObservations,
			Intent:    specialInputIntent,
		})
		if writer, ok := s.sessions.(SessionStore); ok {
			writer.Update(commandSession)
		}
	}
	if commandSession != nil && specialInputIntentConsumesTurn(specialInputIntent) {
		opsRun := ensureOpsRunMetadata(&req)
		return attachOpsRunToTurnResponse(TurnResponse{
			SessionID:       req.SessionID,
			TurnID:          req.TurnID,
			ClientTurnID:    req.ClientTurnID,
			ClientMessageID: req.ClientMessageID,
			Status:          "completed",
			Output:          specialInputIntentOutput(specialInputIntent),
		}, opsRun), nil
	}
	specialInputReadPlan := buildSpecialInputReadPlan(ctx, commandSession, req.TurnID, cmd.ClientTurnID, s.hosts)
	priorCorootContextActive := shouldActivatePriorCorootContextForInput(content)
	specialInputReadPlan = specialInputReadPlanForCurrentTurn(specialInputReadPlan, priorCorootContextActive)
	sessionCorootContext := sessionHasPriorCorootContext(commandSession) && priorCorootContextActive
	route := BuildChatRuntimeRoute(content, mentions, evidence)
	envelope := BuildEvidenceEnvelope(content, nil, nil)
	intentFrame := BuildIntentFrame(content, envelope, nil)
	intentRoute := BuildChatRuntimeRouteFromIntentFrame(intentFrame, route)
	activeRoute, routingMode := selectActiveChatRuntimeRoute(route, intentRoute, intentFrame, s.intentFrameRoutingMode(ctx))
	applyStructuredMentionRouteHints(&activeRoute, structuredMentions)
	applySpecialInputCapabilityRouteHints(&activeRoute, specialInputReadPlan)
	applySessionCorootContextRouteHints(&activeRoute, sessionCorootContext)
	if selectedHostContextShouldUseHostBoundRoute(activeRoute, intentFrame, selectedHostID) {
		activeRoute = promoteSelectedHostContextRoute(activeRoute, selectedHostID)
		mentions = appendSelectedHostContextMention(mentions, selectedHostID)
	}
	if specialInputReadPlanShouldUseHostBoundRoute(activeRoute, specialInputReadPlan) {
		activeRoute = promoteSpecialInputGrantRoute(activeRoute, specialInputReadPlan.ActiveExecutionScope.ResourceID)
		mentions = appendSpecialInputGrantMention(mentions, specialInputReadPlan.ActiveExecutionScope)
	}
	routeBindingMentions := append([]hostops.HostMention(nil), mentions...)
	sessionTargetRoute := s.selectSessionTargetRoute(ctx, activeRoute, intentFrame, commandSession, content, structuredMentions, mentions, req.Metadata)
	if sessionTargetRoute.Applied || sessionTargetRoute.RequiresClarification {
		activeRoute = sessionTargetRoute.Route
	}
	if sessionTargetRoute.Applied {
		routeBindingMentions = appendSessionTargetRouteMentions(routeBindingMentions, sessionTargetRoute)
	}
	applyChatRuntimeRouteMetadata(&req, activeRoute)
	applyIntentFrameRouteMetadata(&req, route, intentRoute, activeRoute, intentFrame, routingMode)
	applyRuntimeMutationPolicies(&req, intentFrameForActiveRoute(intentFrame, activeRoute))
	req.Metadata["aiops.route.activeSource"] = routingMode
	applySessionTargetRouteMetadata(&req, sessionTargetRoute)
	applyChatRuntimeToolSurfaceMetadata(&req, activeRoute)
	applySpecialInputReadPlanMetadata(&req, specialInputReadPlan)
	applySessionCorootContextMetadata(&req, sessionCorootContext, specialInputReadPlan)
	applyStructuredCapabilityMetadata(req.Metadata, structuredMentions)
	applyInputMentionDiagnosticValues(&req, mentionSource, mentionValidation)
	applyUserEvidenceMetadata(&req, evidence)
	applyFollowupPromptProfileMetadata(&req, commandSession, content, evidence)
	applyChatRuntimeRouteHostBinding(&req, activeRoute, routeBindingMentions)
	applyExplicitSelectedHostContext(&req, activeRoute, selectedHostID, requestedSessionType, requestedMode)
	s.applyHostOpsTurnMetadata(ctx, cmd, &req, &structuredMentions, routeBindingMentions)
	applyWorkflowAgentRuntimeMetadata(&req)
	if req.SessionID == "" {
		req.SessionID = strings.TrimSpace(cmd.SessionID)
	}
	if req.SessionID == "" {
		req.SessionID = fmt.Sprintf("sess-%d", time.Now().UnixNano())
	}
	opsRun := ensureOpsRunMetadata(&req)
	ensureCorootRCAMetadata(&req)
	s.enrichTurnHostMetadata(&req)
	applyChatRuntimeResourceProjection(&req, mentions)
	applySessionTargetRouteResourceProjection(&req, sessionTargetRoute)
	applyChatRuntimeSessionTargetRoleTrace(&req, commandSession, content, mentions)
	if !isWorkflowAIChatSource(req.Metadata) {
		if notice, ok := workflowCreationMigrationNotice(content); ok {
			response, err := s.commitWorkflowCreationMigrationTurn(ctx, req, notice)
			response = attachOpsRunToTurnResponse(response, opsRun)
			return response, err
		}
	}
	if s.workflowGeneration != nil && !isWorkflowAIChatSource(req.Metadata) {
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

func workflowCreationMigrationNotice(input string) (string, bool) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", false
	}
	if _, ok := parseAddWorkflowMention(trimmed); ok {
		return workflowCreationMigrationNoticeText(trimmed), true
	}
	if _, ok := parsePlainWorkflowWritingRequest(trimmed); ok {
		return workflowCreationMigrationNoticeText(trimmed), true
	}
	return "", false
}

func isWorkflowAIChatSource(metadata map[string]string) bool {
	return strings.EqualFold(strings.TrimSpace(metadata["source"]), "workflow_ai_chat")
}

func workflowCreationMigrationNoticeText(input string) string {
	requirement := strings.TrimSpace(input)
	if parsed, ok := parseAddWorkflowMention(requirement); ok {
		requirement = parsed
	}
	if parsed, ok := parsePlainWorkflowWritingRequest(requirement); ok {
		requirement = parsed
	}
	link := "/runner?workflow_ai=create"
	if requirement != "" {
		link += "&prompt=" + url.QueryEscape(requirement)
	}
	return "Workflow 创建已经迁移到 Runner Studio 的 Workflow AI Chat。\n\n[打开 Workflow AI Chat 创建](" + link + ") [复制需求]"
}

func (s *defaultChatService) commitWorkflowCreationMigrationTurn(ctx context.Context, req runtimekernel.TurnRequest, notice string) (TurnResponse, error) {
	gateway, ok := s.runtime.(systemTurnRuntimeGateway)
	if !ok {
		return TurnResponse{}, fmt.Errorf("runtime gateway does not support deterministic system turns")
	}
	result, err := gateway.CommitSystemTurn(ctx, runtimekernel.SystemTurnRequest{
		Turn: req,
		Output: runtimekernel.SystemTurnOutput{
			Kind:           runtimekernel.SystemTurnKindNotice,
			FinalText:      notice,
			ContractStatus: runtimekernel.FinalContractStatusPartial,
			FailureCodes:   []string{"workflow_creation_migrated_to_runner_studio"},
		},
	})
	return mapTurnResponse(result), err
}

func chatSessionHasRunningRegularTurn(session *runtimekernel.SessionState) bool {
	return session != nil &&
		session.CurrentTurn != nil &&
		session.CurrentTurn.Lifecycle == runtimekernel.TurnLifecycleRunning &&
		session.CurrentTurn.ResumeState == runtimekernel.TurnResumeStateNone
}

type sessionTargetRouteDecision struct {
	Enabled               bool
	Applied               bool
	RequiresClarification bool
	Reason                string
	HostIDs               []string
	TargetSetID           string
	SourceTurnID          string
	Route                 ChatRuntimeRoute
}

const (
	metadataSessionTargetRouteEnabled = "aiops.sessionTarget.route.enabled"
	envSessionTargetRouteEnabled      = "AIOPS_SESSION_TARGET_ROUTE"
	sessionTargetRouteMinConfidence   = 0.8
)

func (s *defaultChatService) selectSessionTargetRoute(ctx context.Context, route ChatRuntimeRoute, frame runtimecontract.IntentFrame, session *runtimekernel.SessionState, input string, structured parsedInputMentions, mentions []hostops.HostMention, metadata map[string]string) sessionTargetRouteDecision {
	decision := sessionTargetRouteDecision{Route: route}
	if !sessionTargetRouteFeatureEnabled(metadata) {
		return decision
	}
	decision.Enabled = true
	if session == nil || session.SessionTargetSnapshot == nil {
		decision.Reason = "session_target_absent"
		return decision
	}
	snapshot := session.SessionTargetSnapshot
	decision.TargetSetID = strings.TrimSpace(snapshot.ActiveTargetSetID)
	decision.SourceTurnID = strings.TrimSpace(snapshot.SourceTurnID)
	if userHasExplicitHostTarget(input, structured, mentions) {
		decision.Reason = "explicit_target_overrides_session_target"
		return decision
	}
	if route.Mode != ChatRouteAdvisory || route.UserProhibitedHostExec || strings.TrimSpace(route.EnvironmentReadOnlyReason) != "" {
		decision.Reason = "route_not_eligible"
		return decision
	}
	if !sessionTargetIntentAllowsRoute(frame) {
		decision.Reason = "intent_not_target_operation"
		return decision
	}
	if snapshot.Expired() {
		return sessionTargetClarificationDecision(decision, route, "session_target_expired")
	}
	if snapshot.RequiresConfirmation {
		return sessionTargetClarificationDecision(decision, route, "session_target_requires_confirmation")
	}
	if snapshot.Confidence < sessionTargetRouteMinConfidence {
		return sessionTargetClarificationDecision(decision, route, "session_target_low_confidence")
	}
	if len(session.RoleBindingConflicts) > 0 {
		return sessionTargetClarificationDecision(decision, route, "role_binding_conflict")
	}
	hostIDs := s.validatedSessionTargetHostIDs(ctx, snapshot)
	if len(hostIDs) == 0 {
		return sessionTargetClarificationDecision(decision, route, "session_target_hosts_unavailable")
	}
	decision.HostIDs = hostIDs
	decision.Applied = true
	switch len(hostIDs) {
	case 1:
		decision.Route = promoteSessionTargetSingleHostRoute(route, hostIDs[0])
		decision.Reason = "session_target_single_host"
	default:
		decision.Route = promoteSessionTargetMultiHostRoute(route)
		decision.Reason = "session_target_multi_host"
	}
	return decision
}

func sessionTargetRouteFeatureEnabled(metadata map[string]string) bool {
	if raw, ok := metadata[metadataSessionTargetRouteEnabled]; ok {
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "false", "0", "no", "n", "off":
			return false
		case "true", "1", "yes", "y", "on":
			return true
		}
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv(envSessionTargetRouteEnabled))) {
	case "false", "0", "no", "n", "off":
		return false
	case "true", "1", "yes", "y", "on":
		return true
	default:
		return true
	}
}

func userHasExplicitHostTarget(input string, structured parsedInputMentions, mentions []hostops.HostMention) bool {
	if structured.Present {
		return true
	}
	if len(mentions) > 0 {
		return true
	}
	return len(hostops.ParseHostMentions(input)) > 0
}

func sessionTargetIntentAllowsRoute(frame runtimecontract.IntentFrame) bool {
	frame = runtimecontract.NormalizeIntentFrame(frame)
	switch frame.Kind {
	case runtimecontract.IntentKindExplain, runtimecontract.IntentKindResearch, runtimecontract.IntentKindRunbookAuthoring, runtimecontract.IntentKindPlan:
		return false
	}
	return runtimecontract.ContainsDataScope(frame.DataScopes, runtimecontract.DataScopeLocalRuntime) ||
		runtimecontract.ContainsActionRisk(frame.RiskBudget, runtimecontract.ActionRiskHostExec)
}

func sessionTargetClarificationDecision(decision sessionTargetRouteDecision, route ChatRuntimeRoute, reason string) sessionTargetRouteDecision {
	route.AllowsExecCommand = false
	route.RequiresHostBinding = false
	if route.Mode == ChatRouteHostBoundOps || route.Mode == ChatRouteMultiHostOps {
		route.Mode = ChatRouteAdvisory
	}
	route.Reasons = appendUniqueEvidenceString(route.Reasons, "session target requires clarification: "+strings.TrimSpace(reason))
	if strings.TrimSpace(route.Confidence) == "" {
		route.Confidence = "medium"
	}
	decision.Route = route
	decision.RequiresClarification = true
	decision.Reason = strings.TrimSpace(reason)
	return decision
}

func promoteSessionTargetSingleHostRoute(route ChatRuntimeRoute, hostID string) ChatRuntimeRoute {
	route.Mode = ChatRouteHostBoundOps
	route.RequiresHostBinding = true
	route.AllowsExecCommand = true
	route.Reasons = appendUniqueEvidenceString(route.Reasons, "session target single host: "+strings.TrimSpace(hostID))
	if strings.TrimSpace(route.Confidence) == "" {
		route.Confidence = "medium"
	}
	return route
}

func promoteSessionTargetMultiHostRoute(route ChatRuntimeRoute) ChatRuntimeRoute {
	route.Mode = ChatRouteMultiHostOps
	route.RequiresHostBinding = true
	route.AllowsExecCommand = false
	route.Reasons = appendUniqueEvidenceString(route.Reasons, "session target multi host")
	if strings.TrimSpace(route.Confidence) == "" {
		route.Confidence = "medium"
	}
	return route
}

func (s *defaultChatService) validatedSessionTargetHostIDs(ctx context.Context, snapshot *resourcebinding.SessionTargetSnapshot) []string {
	hostIDs := resourcebinding.HostIDsFromSessionTarget(snapshot)
	if len(hostIDs) == 0 {
		return nil
	}
	if s == nil || s.hosts == nil {
		return uniqueSortedHostIDs(hostIDs)
	}
	valid := make([]string, 0, len(hostIDs))
	for _, hostID := range hostIDs {
		hostID = strings.TrimSpace(hostID)
		if hostID == "" {
			continue
		}
		if hostID == serverLocalHostID {
			valid = append(valid, hostID)
			continue
		}
		host, err := s.hosts.GetHost(hostID)
		if err == nil && host != nil && strings.TrimSpace(host.ID) != "" {
			valid = append(valid, strings.TrimSpace(host.ID))
			continue
		}
		if s.resolveSelectedHostContextID(ctx, hostID) != "" {
			valid = append(valid, hostID)
		}
	}
	return uniqueSortedHostIDs(valid)
}

func uniqueSortedHostIDs(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func appendSessionTargetRouteMentions(mentions []hostops.HostMention, decision sessionTargetRouteDecision) []hostops.HostMention {
	if !decision.Applied {
		return mentions
	}
	out := append([]hostops.HostMention(nil), mentions...)
	for _, hostID := range decision.HostIDs {
		hostID = strings.TrimSpace(hostID)
		if hostID == "" {
			continue
		}
		alreadyPresent := false
		for _, mention := range out {
			if strings.EqualFold(strings.TrimSpace(mention.HostID), hostID) {
				alreadyPresent = true
				break
			}
		}
		if alreadyPresent {
			continue
		}
		out = append(out, hostops.HostMention{
			TokenID:     "session-target:" + strings.TrimSpace(decision.TargetSetID) + ":" + hostID,
			Raw:         "@" + hostID,
			HostID:      hostID,
			DisplayName: hostID,
			Source:      hostops.HostMentionSourceInventory,
			Resolved:    true,
			Confidence:  1,
			CreatedAt:   time.Now(),
		})
	}
	return out
}

func (s *defaultChatService) applyHostOpsTurnMetadata(ctx context.Context, cmd ChatCommand, req *runtimekernel.TurnRequest, structured *parsedInputMentions, routeMentions []hostops.HostMention) {
	if s == nil || req == nil {
		return
	}
	if routeMode := strings.TrimSpace(req.Metadata["aiops.route.mode"]); routeMode != "" && routeMode != string(ChatRouteMultiHostOps) {
		return
	}
	mentions := append([]hostops.HostMention(nil), routeMentions...)
	if len(mentions) == 0 {
		mentions = s.hostOpsMentionsForCommand(ctx, cmd, req.Input, structured)
	}
	decision := hostops.DetectRoute(req.Input, mentions)
	if strings.TrimSpace(req.Metadata["aiops.route.mode"]) == string(ChatRouteMultiHostOps) &&
		req.Metadata["aiops.sessionTarget.route.applied"] == "true" &&
		len(mentions) >= 2 {
		decision = hostops.RouteDecision{
			Kind:         hostops.RouteKindHostOps,
			Mentions:     append([]hostops.HostMention(nil), mentions...),
			PlanRequired: true,
			Reason:       "session target multi-host operation requires plan mode",
		}
	}
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
	if strings.TrimSpace(cmd.Metadata["aiops.hostops.mentions"]) != "" {
		return s.confirmHostOpsMetadataMentions(ctx, mentions)
	}
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

func (s *defaultChatService) confirmHostOpsMetadataMentions(ctx context.Context, mentions []hostops.HostMention) []hostops.HostMention {
	if len(mentions) == 0 {
		return nil
	}
	if s == nil || s.hosts == nil {
		return filterHostOpsMetadataWithoutRepository(mentions)
	}
	resolutionHints := make([]hostops.HostMention, 0, len(mentions))
	originals := make([]hostops.HostMention, 0, len(mentions))
	for _, mention := range mentions {
		hint := mention
		hint.Resolved = false
		if hint.Source == hostops.HostMentionSourceInventory && strings.TrimSpace(hint.HostID) != "" {
			hint.Raw = "@" + strings.TrimSpace(hint.HostID)
			hint.Address = ""
			hint.DisplayName = ""
		}
		resolutionHints = append(resolutionHints, hint)
		originals = append(originals, mention)
	}
	resolved, _ := hostops.NewResolver(hostRepositoryLookup{repo: s.hosts}).Resolve(ctx, resolutionHints)
	confirmed := make([]hostops.HostMention, 0, len(resolved))
	for index, mention := range resolved {
		if !mention.Resolved || strings.TrimSpace(mention.HostID) == "" {
			continue
		}
		if index < len(originals) {
			mention.TokenID = firstNonEmptyString(strings.TrimSpace(originals[index].TokenID), mention.TokenID)
			mention.Raw = firstNonEmptyString(strings.TrimSpace(originals[index].Raw), mention.Raw)
			mention.SpanStart = firstNonZeroInt(originals[index].SpanStart, mention.SpanStart)
			mention.SpanEnd = firstNonZeroInt(originals[index].SpanEnd, mention.SpanEnd)
		}
		confirmed = append(confirmed, mention)
	}
	return filterHostOpsRouteMentions(confirmed)
}

func filterHostOpsMetadataWithoutRepository(mentions []hostops.HostMention) []hostops.HostMention {
	safe := make([]hostops.HostMention, 0, len(mentions))
	for _, mention := range mentions {
		switch mention.Source {
		case hostops.HostMentionSourceLocalAlias:
			mention.HostID = serverLocalHostID
			mention.Address = firstNonEmptyString(strings.TrimSpace(mention.Address), serverLocalHostID)
			mention.DisplayName = firstNonEmptyString(strings.TrimSpace(mention.DisplayName), serverLocalHostID)
			mention.Resolved = true
			mention.Confidence = 1
			safe = append(safe, mention)
		case hostops.HostMentionSourceIPLiteral:
			mention.HostID = ""
			mention.Resolved = false
			safe = append(safe, mention)
		}
	}
	return filterHostOpsRouteMentions(safe)
}

func firstNonZeroInt(values ...int) int {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
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

func buildSpecialInputReadPlan(ctx context.Context, session *runtimekernel.SessionState, turnID, taskID string, hosts HostRepository) *specialinputmemory.MemoryReadPlan {
	if session == nil {
		return nil
	}
	plan := specialinputmemory.BuildMemoryReadPlan(ctx, session.SpecialInputMemory, specialinputmemory.MemoryReadPlanInput{
		SessionID:    session.ID,
		TaskID:       firstNonEmptyString(strings.TrimSpace(taskID), session.ID),
		TurnID:       strings.TrimSpace(turnID),
		Now:          time.Now().UTC(),
		HostResolver: specialInputHostResolver{hosts: hosts},
	})
	if plan.ActiveExecutionScope == nil && len(plan.VisibleFacts) == 0 && len(plan.PendingConfirmations) == 0 && len(plan.SuspendedGrants) == 0 {
		return nil
	}
	return &plan
}

func specialInputIntentFromContent(input string, observations []specialinputmemory.MentionObservation, metadata map[string]string, session *runtimekernel.SessionState) specialinputmemory.UserSpecialInputIntent {
	text := strings.ToLower(strings.TrimSpace(input))
	if text == "" {
		return specialinputmemory.UserSpecialInputIntent{}
	}
	targetKind, targetKey := specialInputIntentTarget(observations)
	if targetKind == "" && specialInputTextMentionsHost(text) {
		targetKind = specialinputmemory.FactKindHost
	}
	if specialInputContainsAny(text, "忘记", "清除", "清空", "不要再用", "别再用", "移除") {
		return specialinputmemory.UserSpecialInputIntent{
			Kind:         specialinputmemory.IntentForget,
			TargetKind:   targetKind,
			CanonicalKey: targetKey,
			Reason:       "user_forget",
		}
	}
	if specialInputContainsAny(text, "不对", "说错", "错了") ||
		(len(observations) > 0 && specialInputContainsAny(text, "应该是", "改成", "换成")) {
		return specialinputmemory.UserSpecialInputIntent{
			Kind:         specialinputmemory.IntentCorrection,
			TargetKind:   firstNonEmptyString(targetKind, specialinputmemory.FactKindHost),
			CanonicalKey: "",
			Reason:       "user_correction",
		}
	}
	if specialInputContainsAny(text, "确认", "就用", "是的", "没错") && specialInputConfirmAllowed(metadata, targetKey, session) {
		return specialinputmemory.UserSpecialInputIntent{
			Kind:         specialinputmemory.IntentConfirm,
			TargetKind:   firstNonEmptyString(targetKind, specialinputmemory.FactKindHost),
			CanonicalKey: targetKey,
			Reason:       "user_confirmation",
		}
	}
	return specialinputmemory.UserSpecialInputIntent{}
}

func appendRawCorootCapabilityObservation(input string, structured parsedInputMentions, observations []specialinputmemory.MentionObservation) []specialinputmemory.MentionObservation {
	if structured.Present || !hasExplicitCorootMention(input) || specialInputObservationsContainCapability(observations, "coroot") {
		return observations
	}
	return append(observations, corootCapabilityObservation("@coroot", specialinputmemory.SourceSystem))
}

func corootCapabilityObservation(raw string, source string) specialinputmemory.MentionObservation {
	return specialinputmemory.MentionObservation{
		Kind:           specialinputmemory.FactKindCapability,
		CanonicalKey:   "capability:coroot",
		Display:        "Coroot",
		RawText:        strings.TrimSpace(raw),
		Path:           "capability://coroot",
		Source:         source,
		TrustLevel:     specialinputmemory.TrustLevelServerConfirmed,
		ResourceKind:   specialinputmemory.ResourceKindCapability,
		ResourceID:     "coroot",
		AllowedActions: []string{specialinputmemory.ActionInspect, specialinputmemory.ActionRead},
	}
}

func specialInputObservationsContainCapability(observations []specialinputmemory.MentionObservation, capability string) bool {
	capability = strings.ToLower(strings.TrimSpace(capability))
	for _, observation := range observations {
		if strings.TrimSpace(observation.ResourceKind) != specialinputmemory.ResourceKindCapability {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(observation.ResourceID), capability) || strings.EqualFold(strings.TrimSpace(observation.CanonicalKey), "capability:"+capability) {
			return true
		}
	}
	return false
}

func specialInputConfirmAllowed(metadata map[string]string, targetKey string, session *runtimekernel.SessionState) bool {
	if strings.EqualFold(strings.TrimSpace(metadata["aiops.specialInput.command"]), "confirm") {
		return true
	}
	if strings.TrimSpace(targetKey) != "" {
		return true
	}
	if session == nil {
		return false
	}
	for _, fact := range session.SpecialInputMemory.Facts {
		if strings.TrimSpace(fact.Status) == specialinputmemory.FactStatusActive &&
			strings.TrimSpace(fact.TrustLevel) == specialinputmemory.TrustLevelRawTyped {
			return true
		}
	}
	return false
}

func specialInputIntentTarget(observations []specialinputmemory.MentionObservation) (string, string) {
	for _, observation := range observations {
		kind := strings.TrimSpace(observation.Kind)
		key := strings.TrimSpace(observation.CanonicalKey)
		if kind != "" || key != "" {
			return kind, key
		}
	}
	return "", ""
}

func specialInputTextMentionsHost(text string) bool {
	return strings.Contains(text, "主机") || strings.Contains(text, "host")
}

func specialInputContainsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, strings.ToLower(strings.TrimSpace(needle))) {
			return true
		}
	}
	return false
}

func specialInputIntentConsumesTurn(intent specialinputmemory.UserSpecialInputIntent) bool {
	switch strings.TrimSpace(intent.Kind) {
	case specialinputmemory.IntentCorrection, specialinputmemory.IntentForget, specialinputmemory.IntentConfirm:
		return true
	default:
		return false
	}
}

func specialInputIntentOutput(intent specialinputmemory.UserSpecialInputIntent) string {
	switch strings.TrimSpace(intent.Kind) {
	case specialinputmemory.IntentCorrection:
		return "已更新特殊输入短期记忆。"
	case specialinputmemory.IntentForget:
		return "已清除对应的特殊输入短期记忆。"
	case specialinputmemory.IntentConfirm:
		return "已确认特殊输入短期记忆。"
	default:
		return "已处理特殊输入短期记忆。"
	}
}

type specialInputHostResolver struct {
	hosts HostRepository
}

func (r specialInputHostResolver) RevalidateHost(_ context.Context, ref specialinputmemory.HostRef) (specialinputmemory.HostValidation, error) {
	hostID := strings.TrimSpace(ref.ResourceID)
	if hostID == "" {
		hostID = strings.TrimPrefix(strings.TrimSpace(ref.CanonicalKey), "host:")
	}
	if hostID == "" || hostID == serverLocalHostID {
		return specialinputmemory.HostValidation{ResourceID: hostID, Available: hostID != ""}, nil
	}
	if r.hosts == nil {
		return specialinputmemory.HostValidation{ResourceID: hostID, Available: true}, nil
	}
	host, err := r.hosts.GetHost(hostID)
	if err != nil || host == nil || strings.TrimSpace(host.ID) == "" {
		return specialinputmemory.HostValidation{ResourceID: hostID, Available: false, Reason: "host_not_found"}, nil
	}
	available := strings.TrimSpace(host.Status) != "offline"
	return specialinputmemory.HostValidation{
		ResourceID:     strings.TrimSpace(host.ID),
		Available:      available,
		ValidationHash: firstNonEmptyString(strings.TrimSpace(host.AgentURL), strings.TrimSpace(host.Address), strings.TrimSpace(host.ID)),
		Reason:         "",
	}, nil
}

func specialInputReadPlanShouldUseHostBoundRoute(route ChatRuntimeRoute, plan *specialinputmemory.MemoryReadPlan) bool {
	if plan == nil || plan.ActiveExecutionScope == nil {
		return false
	}
	scope := plan.ActiveExecutionScope
	if scope.ResourceKind != specialinputmemory.ResourceKindHost || strings.TrimSpace(scope.ResourceID) == "" {
		return false
	}
	if route.Mode != ChatRouteAdvisory || route.UserProhibitedHostExec || strings.TrimSpace(route.EnvironmentReadOnlyReason) != "" {
		return false
	}
	return scope.Allows(specialinputmemory.ActionInspect) || scope.Allows(specialinputmemory.ActionRead) || scope.Allows(specialinputmemory.ActionExecLowRisk)
}

func applySpecialInputCapabilityRouteHints(route *ChatRuntimeRoute, plan *specialinputmemory.MemoryReadPlan) {
	if route == nil || !specialInputReadPlanHasCapability(plan, "coroot") {
		return
	}
	applyCorootCapabilityRouteHint(route, "special input capability: coroot")
}

func applySessionCorootContextRouteHints(route *ChatRuntimeRoute, enabled bool) {
	if route == nil || !enabled {
		return
	}
	applyCorootCapabilityRouteHint(route, "session history capability: coroot")
}

func shouldActivatePriorCorootContextForInput(input string) bool {
	text := strings.TrimSpace(input)
	if text == "" {
		return false
	}
	if hasExplicitCorootMention(text) {
		return true
	}
	if isPlainConversationalTurn(text) {
		return false
	}
	if looksLikeCorootEntityReference(text) {
		return true
	}
	lower := strings.ToLower(text)
	for _, marker := range []string{
		"异常", "告警", "报警", "事件", "问题", "错误", "报错", "失败", "不可用", "健康",
		"重启", "根因", "排查", "分析", "监控", "指标", "日志", "链路", "拓扑", "依赖",
		"服务", "应用", "实例", "节点", "继续", "incident", "alert", "warning", "error",
		"failed", "failure", "restart", "unavailable", "unhealthy", "health", "service",
		"application", "instance", "pod", "node", "metric", "log", "trace", "dependency",
		"rca", "root cause",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	referential := containsAnyText(lower, "刚才", "上面", "前面", "这个", "那个", "它", "继续")
	action := containsAnyText(lower, "看", "查", "分析", "排查", "为什么", "原因", "怎么", "如何", "what", "why", "check", "show", "analyze")
	return referential && action
}

func isPlainConversationalTurn(input string) bool {
	token := compactConversationalToken(input)
	if token == "" {
		return true
	}
	switch token {
	case "你好", "您好", "哈喽", "嗨", "在吗", "谢谢", "感谢", "多谢", "好的", "好", "嗯", "嗯嗯", "ok", "okay", "hi", "hello", "thanks", "thankyou", "thx", "bye":
		return true
	default:
		return false
	}
}

func compactConversationalToken(input string) string {
	input = strings.ToLower(strings.TrimSpace(input))
	var b strings.Builder
	for _, r := range input {
		if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func looksLikeCorootEntityReference(input string) bool {
	token := strings.TrimSpace(input)
	if token == "" || strings.ContainsAny(token, " \t\r\n，。！？；;?") {
		return false
	}
	token = strings.TrimFunc(token, func(r rune) bool {
		return unicode.IsPunct(r) || unicode.IsSymbol(r)
	})
	if len([]rune(token)) < 3 || len([]rune(token)) > 96 {
		return false
	}
	lower := strings.ToLower(token)
	if containsAnyText(lower, "-", "_", ".", ":", "/") && containsASCIIAlnum(lower) {
		return true
	}
	if containsASCIIAlpha(lower) && containsASCIIDigit(lower) {
		return true
	}
	for _, marker := range []string{
		"agent", "server", "service", "node", "pod", "gateway", "worker", "nginx", "redis",
		"rabbitmq", "postgres", "mysql", "kafka", "web", "api", "mon",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func containsASCIIAlnum(input string) bool {
	return containsASCIIAlpha(input) || containsASCIIDigit(input)
}

func containsASCIIAlpha(input string) bool {
	for _, r := range input {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return true
		}
	}
	return false
}

func containsASCIIDigit(input string) bool {
	for _, r := range input {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}

func containsAnyText(input string, markers ...string) bool {
	for _, marker := range markers {
		if strings.Contains(input, marker) {
			return true
		}
	}
	return false
}

func specialInputReadPlanForCurrentTurn(plan *specialinputmemory.MemoryReadPlan, corootActive bool) *specialinputmemory.MemoryReadPlan {
	if plan == nil || corootActive || !specialInputReadPlanHasCapability(plan, "coroot") {
		return plan
	}
	filtered := *plan
	if filtered.ActiveExecutionScope != nil && corootGrantMatchesCapability(*filtered.ActiveExecutionScope, "coroot") {
		filtered.ActiveExecutionScope = nil
		filtered.ModelSummary = ""
	}
	filtered.VisibleFacts = filterCorootCapabilityFacts(filtered.VisibleFacts)
	filtered.CandidateFacts = filterCorootCapabilityFacts(filtered.CandidateFacts)
	filtered.SuspendedGrants = filterCorootCapabilityGrants(filtered.SuspendedGrants)
	filtered.CandidateRoleBindings = filterCorootCapabilityRoleBindings(filtered.CandidateRoleBindings)
	if specialinputmemory.MemoryReadPlanTraceEmpty(filtered) {
		return nil
	}
	return &filtered
}

func filterCorootCapabilityFacts(facts []specialinputmemory.MentionFact) []specialinputmemory.MentionFact {
	out := make([]specialinputmemory.MentionFact, 0, len(facts))
	for _, fact := range facts {
		if corootFactMatchesCapability(fact, "coroot") {
			continue
		}
		out = append(out, fact)
	}
	return out
}

func filterCorootCapabilityGrants(grants []specialinputmemory.ExecutionScopeGrant) []specialinputmemory.ExecutionScopeGrant {
	out := make([]specialinputmemory.ExecutionScopeGrant, 0, len(grants))
	for _, grant := range grants {
		if corootGrantMatchesCapability(grant, "coroot") {
			continue
		}
		out = append(out, grant)
	}
	return out
}

func filterCorootCapabilityRoleBindings(bindings []specialinputmemory.MentionRoleBinding) []specialinputmemory.MentionRoleBinding {
	out := make([]specialinputmemory.MentionRoleBinding, 0, len(bindings))
	for _, binding := range bindings {
		if strings.TrimSpace(binding.ResourceKind) == specialinputmemory.ResourceKindCapability &&
			strings.EqualFold(strings.TrimSpace(binding.ResourceID), "coroot") {
			continue
		}
		out = append(out, binding)
	}
	return out
}

func corootFactMatchesCapability(fact specialinputmemory.MentionFact, capability string) bool {
	return strings.TrimSpace(fact.ResourceKind) == specialinputmemory.ResourceKindCapability &&
		(strings.EqualFold(strings.TrimSpace(fact.ResourceID), capability) ||
			strings.EqualFold(strings.TrimSpace(fact.CanonicalKey), "capability:"+capability))
}

func corootGrantMatchesCapability(grant specialinputmemory.ExecutionScopeGrant, capability string) bool {
	return strings.TrimSpace(grant.ResourceKind) == specialinputmemory.ResourceKindCapability &&
		(strings.EqualFold(strings.TrimSpace(grant.ResourceID), capability) ||
			strings.EqualFold(strings.TrimSpace(grant.CanonicalKey), "capability:"+capability))
}

func specialInputReadPlanHasCapability(plan *specialinputmemory.MemoryReadPlan, capability string) bool {
	if plan == nil {
		return false
	}
	capability = strings.ToLower(strings.TrimSpace(capability))
	if capability == "" {
		return false
	}
	if grant := plan.ActiveExecutionScope; grant != nil &&
		strings.TrimSpace(grant.ResourceKind) == specialinputmemory.ResourceKindCapability &&
		strings.EqualFold(strings.TrimSpace(grant.ResourceID), capability) &&
		(grant.Allows(specialinputmemory.ActionInspect) || grant.Allows(specialinputmemory.ActionRead)) {
		return true
	}
	for _, fact := range plan.VisibleFacts {
		if strings.TrimSpace(fact.ResourceKind) != specialinputmemory.ResourceKindCapability {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(fact.ResourceID), capability) || strings.EqualFold(strings.TrimSpace(fact.CanonicalKey), "capability:"+capability) {
			return true
		}
	}
	return false
}

func sessionHasPriorCorootContext(session *runtimekernel.SessionState) bool {
	if session == nil {
		return false
	}
	for i := len(session.Messages) - 1; i >= 0 && len(session.Messages)-i <= 80; i-- {
		msg := session.Messages[i]
		if strings.EqualFold(strings.TrimSpace(msg.Role), "user") && hasExplicitCorootMention(msg.Content) {
			return true
		}
		if corootContextMetadataAllowed(msg.Metadata) {
			return true
		}
	}
	if session.CurrentTurn != nil && corootContextMetadataAllowed(session.CurrentTurn.Metadata) {
		return true
	}
	for i := len(session.TurnHistory) - 1; i >= 0 && len(session.TurnHistory)-i <= 30; i-- {
		if corootContextMetadataAllowed(session.TurnHistory[i].Metadata) {
			return true
		}
	}
	return false
}

func corootContextMetadataAllowed(metadata map[string]string) bool {
	if len(metadata) == 0 {
		return false
	}
	for _, key := range []string{
		"aiops.tool.corootRCAAllowed",
		"aiops.route.allowsCorootRCA",
		metadataCorootExplicitRCA,
	} {
		if strings.EqualFold(strings.TrimSpace(metadata[key]), "true") {
			return true
		}
	}
	return false
}

func promoteSpecialInputGrantRoute(route ChatRuntimeRoute, hostID string) ChatRuntimeRoute {
	route.Mode = ChatRouteHostBoundOps
	route.RequiresHostBinding = true
	route.AllowsExecCommand = true
	route.Reasons = appendUniqueEvidenceString(route.Reasons, "special input grant: "+strings.TrimSpace(hostID))
	if strings.TrimSpace(route.Confidence) == "" {
		route.Confidence = "medium"
	}
	return route
}

func appendSpecialInputGrantMention(mentions []hostops.HostMention, grant *specialinputmemory.ExecutionScopeGrant) []hostops.HostMention {
	if grant == nil {
		return mentions
	}
	hostID := strings.TrimSpace(grant.ResourceID)
	if hostID == "" {
		return mentions
	}
	for _, mention := range mentions {
		if strings.EqualFold(strings.TrimSpace(mention.HostID), hostID) {
			return mentions
		}
	}
	return append(mentions, hostops.HostMention{
		TokenID:     "special-input-grant:" + strings.TrimSpace(grant.ID),
		Raw:         "@" + hostID,
		HostID:      hostID,
		DisplayName: firstNonEmptyString(strings.TrimSpace(grant.Display), hostID),
		Source:      hostops.HostMentionSourceInventory,
		Resolved:    true,
		Confidence:  1,
		CreatedAt:   time.Now(),
	})
}

func applySpecialInputReadPlanMetadata(req *runtimekernel.TurnRequest, plan *specialinputmemory.MemoryReadPlan) {
	if req == nil || plan == nil {
		return
	}
	if req.Metadata == nil {
		req.Metadata = map[string]string{}
	}
	req.SpecialInputReadPlan = plan
	if plan.ActiveExecutionScope != nil {
		req.Metadata["aiops.specialInput.readPlan.activeGrantId"] = strings.TrimSpace(plan.ActiveExecutionScope.ID)
		req.Metadata["aiops.specialInput.readPlan.activeResourceId"] = strings.TrimSpace(plan.ActiveExecutionScope.ResourceID)
		req.Metadata["aiops.specialInput.readPlan.activeResourceKind"] = strings.TrimSpace(plan.ActiveExecutionScope.ResourceKind)
		req.Metadata["aiops.specialInput.readPlan.source"] = "execution_scope_grant"
	}
	if len(plan.PendingConfirmations) > 0 {
		req.Metadata["aiops.specialInput.readPlan.pendingConfirmation"] = "true"
	}
	if len(plan.SuspendedGrants) > 0 {
		req.Metadata["aiops.specialInput.readPlan.suspended"] = "true"
	}
	if specialInputReadPlanHasCapability(plan, "coroot") {
		req.Metadata["aiops.coroot.contextSource"] = "special_input_memory"
		setMetadataIfEmpty(req.Metadata, metadataCorootRCADisplayAllowed, "true")
		setMetadataIfEmpty(req.Metadata, metadataObservabilityProvider, "coroot")
	}
}

func applySessionCorootContextMetadata(req *runtimekernel.TurnRequest, enabled bool, plan *specialinputmemory.MemoryReadPlan) {
	if req == nil || !enabled || specialInputReadPlanHasCapability(plan, "coroot") {
		return
	}
	if req.Metadata == nil {
		req.Metadata = map[string]string{}
	}
	req.Metadata["aiops.coroot.contextSource"] = "session_history"
	setMetadataIfEmpty(req.Metadata, metadataCorootRCADisplayAllowed, "true")
	setMetadataIfEmpty(req.Metadata, metadataObservabilityProvider, "coroot")
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
		applyCorootCapabilityRouteHint(route, "structured capability: coroot")
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
	if req == nil {
		return
	}
	if req.Metadata == nil {
		req.Metadata = map[string]string{}
	}
	if isPlainConversationalTurn(input) {
		setMetadataIfEmpty(req.Metadata, metadataAnswerSmalltalkOnly, "true")
		setMetadataIfEmpty(req.Metadata, "reasoningEffort", "low")
		setMetadataIfEmpty(req.Metadata, "answerStyle", "smalltalk")
		return
	}
	if session == nil || !shortFollowupInput(input) || evidence.HasEvidence || !sessionHasExistingEvidenceContext(session) {
		return
	}
	setMetadataIfEmpty(req.Metadata, metadataTurnFollowup, "true")
	setMetadataIfEmpty(req.Metadata, metadataTurnHasExistingEvidence, "true")
	setMetadataIfEmpty(req.Metadata, metadataTurnNoNewEvidence, "true")
	if completeExplanationFollowupInput(input) {
		setMetadataIfEmpty(req.Metadata, metadataAnswerRequireCompleteFollowup, "true")
		setMetadataIfEmpty(req.Metadata, "reasoningEffort", "medium")
		setMetadataIfEmpty(req.Metadata, "answerStyle", "complete_explanation")
		return
	}
	setMetadataIfEmpty(req.Metadata, "reasoningEffort", "low")
	setMetadataIfEmpty(req.Metadata, "answerStyle", "concise")
}

func completeExplanationFollowupInput(input string) bool {
	lower := strings.ToLower(strings.TrimSpace(input))
	if lower == "" {
		return false
	}
	for _, signal := range []string{
		"完整答案",
		"完整回答",
		"完整写出来",
		"写完整",
		"展开讲",
		"详细讲",
		"详细解释",
		"底层实现",
		"实现原理",
		"线程安全",
		"阻塞原理",
	} {
		if strings.Contains(lower, signal) {
			return true
		}
	}
	goInternalsTerms := 0
	for _, term := range []string{"channel", "map", "slice"} {
		if strings.Contains(lower, term) {
			goInternalsTerms++
		}
	}
	if goInternalsTerms >= 2 && (strings.Contains(lower, "底层") || strings.Contains(lower, "阻塞") || strings.Contains(lower, "线程")) {
		return true
	}
	return false
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
