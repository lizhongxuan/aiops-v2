package appui

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/hostops"
	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/store"
)

type chatRuntimeCapture struct {
	mu           sync.Mutex
	runCalled    bool
	runReq       runtimekernel.TurnRequest
	resumeCalled bool
	resumeReq    runtimekernel.ResumeRequest
	cancelReq    runtimekernel.CancelRequest
}

type cancelledChatRuntime struct {
	started chan runtimekernel.TurnRequest
}

func newCancelledChatRuntime() *cancelledChatRuntime {
	return &cancelledChatRuntime{started: make(chan runtimekernel.TurnRequest, 1)}
}

func (r *cancelledChatRuntime) RunTurn(_ context.Context, req runtimekernel.TurnRequest) (runtimekernel.TurnResult, error) {
	r.started <- req
	return runtimekernel.TurnResult{
		SessionID:       req.SessionID,
		TurnID:          req.TurnID,
		ClientMessageID: req.ClientMessageID,
		ClientTurnID:    req.ClientTurnID,
		Status:          "cancelled",
	}, nil
}

func (r *cancelledChatRuntime) ResumeTurn(context.Context, runtimekernel.ResumeRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}

func (r *cancelledChatRuntime) CancelTurn(context.Context, runtimekernel.CancelRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}

type blockedChatRuntime struct {
	started chan runtimekernel.TurnRequest
}

func newBlockedChatRuntime() *blockedChatRuntime {
	return &blockedChatRuntime{started: make(chan runtimekernel.TurnRequest, 1)}
}

func (r *blockedChatRuntime) RunTurn(_ context.Context, req runtimekernel.TurnRequest) (runtimekernel.TurnResult, error) {
	r.started <- req
	return runtimekernel.TurnResult{
		SessionID:       req.SessionID,
		TurnID:          req.TurnID,
		ClientMessageID: req.ClientMessageID,
		ClientTurnID:    req.ClientTurnID,
		Status:          "blocked",
		Error:           "approval required",
	}, nil
}

func (r *blockedChatRuntime) ResumeTurn(context.Context, runtimekernel.ResumeRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}

func (r *blockedChatRuntime) CancelTurn(context.Context, runtimekernel.CancelRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}

type blockingChatRuntime struct {
	started chan runtimekernel.TurnRequest
	release chan struct{}
}

func newBlockingChatRuntime() *blockingChatRuntime {
	return &blockingChatRuntime{
		started: make(chan runtimekernel.TurnRequest, 1),
		release: make(chan struct{}),
	}
}

func (r *blockingChatRuntime) RunTurn(_ context.Context, req runtimekernel.TurnRequest) (runtimekernel.TurnResult, error) {
	r.started <- req
	<-r.release
	return runtimekernel.TurnResult{
		SessionID:       req.SessionID,
		TurnID:          req.TurnID,
		ClientMessageID: req.ClientMessageID,
		ClientTurnID:    req.ClientTurnID,
		Status:          "completed",
		Output:          "final output should not be in accepted response",
	}, nil
}

func (r *blockingChatRuntime) ResumeTurn(context.Context, runtimekernel.ResumeRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}

func (r *blockingChatRuntime) CancelTurn(context.Context, runtimekernel.CancelRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}

type lifecycleContextRuntime struct {
	ctxErr chan error
}

func newLifecycleContextRuntime() *lifecycleContextRuntime {
	return &lifecycleContextRuntime{ctxErr: make(chan error, 1)}
}

func (r *lifecycleContextRuntime) RunTurn(ctx context.Context, req runtimekernel.TurnRequest) (runtimekernel.TurnResult, error) {
	r.ctxErr <- ctx.Err()
	return runtimekernel.TurnResult{
		SessionID:       req.SessionID,
		TurnID:          req.TurnID,
		ClientMessageID: req.ClientMessageID,
		ClientTurnID:    req.ClientTurnID,
		Status:          "cancelled",
	}, nil
}

func (r *lifecycleContextRuntime) ResumeTurn(context.Context, runtimekernel.ResumeRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}

func (r *lifecycleContextRuntime) CancelTurn(context.Context, runtimekernel.CancelRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}

func TestChatServiceSendMessageHandlesAddWorkflowWithoutRuntimeTools(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := newBlockingChatRuntime()
	events := NewAgentEventService(nil)
	service := NewChatService(runtime, sessions, events)

	result, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID:       "sess-workflowgen",
		Content:         "@add_workflow 每天早上8点自动抓取AI行业新闻，提取三条关键内容直接返回给我",
		ClientMessageID: "client-msg-workflowgen",
		ClientTurnID:    "client-turn-workflowgen",
		HostID:          "server-local",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("Status = %q, want completed", result.Status)
	}
	select {
	case <-runtime.started:
		t.Fatal("runtime RunTurn was called; @add_workflow should use controlled internal workflow generation")
	default:
	}
	session := sessions.Get("sess-workflowgen")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("workflow generation did not write current turn")
	}
	if !strings.Contains(session.CurrentTurn.FinalOutput, "工作流计划") {
		t.Fatalf("FinalOutput = %q, want workflow plan summary", session.CurrentTurn.FinalOutput)
	}
	if !strings.Contains(session.CurrentTurn.FinalOutput, "初始生成大纲") ||
		!strings.Contains(session.CurrentTurn.FinalOutput, "拆分、合并或调整节点") {
		t.Fatalf("FinalOutput = %q, want plan to be described as adjustable generation outline", session.CurrentTurn.FinalOutput)
	}
	var artifactPayload string
	for _, item := range session.CurrentTurn.AgentItems {
		if item.Type == "tool_result" && strings.Contains(string(item.Payload.Data), "runner_workflow_generation") {
			artifactPayload = string(item.Payload.Data)
		}
	}
	if artifactPayload == "" {
		t.Fatalf("agent items = %#v, want runner_workflow_generation artifact payload", session.CurrentTurn.AgentItems)
	}
	if !strings.Contains(artifactPayload, `"planIsProvisional":true`) ||
		!strings.Contains(artifactPayload, `"status":"planned"`) {
		t.Fatalf("artifact payload = %s, want provisional plan step status", artifactPayload)
	}
}

func TestChatServiceSendMessageHandlesPlainWorkflowWritingRequestWithoutRuntimeTools(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := newBlockingChatRuntime()
	service := NewChatService(runtime, sessions, NewAgentEventService(nil))

	result, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID:       "sess-workflowgen-plain",
		Content:         "帮我写一个workflow,让主机A和主机B的PG两个节点可以通过主机C的pg_mon形成PG集群",
		ClientMessageID: "client-msg-workflowgen-plain",
		ClientTurnID:    "client-turn-workflowgen-plain",
		HostID:          "server-local",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("Status = %q, want completed", result.Status)
	}
	select {
	case <-runtime.started:
		t.Fatal("runtime RunTurn was called; plain workflow writing request should use controlled internal workflow generation")
	default:
	}
	session := sessions.Get("sess-workflowgen-plain")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("workflow generation did not write current turn")
	}
	if !strings.Contains(session.CurrentTurn.FinalOutput, "工作流计划") {
		t.Fatalf("FinalOutput = %q, want workflow plan summary", session.CurrentTurn.FinalOutput)
	}
	if !strings.Contains(session.CurrentTurn.FinalOutput, "主机A") ||
		!strings.Contains(session.CurrentTurn.FinalOutput, "主机B") ||
		!strings.Contains(session.CurrentTurn.FinalOutput, "主机C") ||
		!strings.Contains(session.CurrentTurn.FinalOutput, "pg_mon") {
		t.Fatalf("FinalOutput = %q, want resource roles from user request", session.CurrentTurn.FinalOutput)
	}
	if !strings.Contains(session.CurrentTurn.FinalOutput, "generate_resource_ops_workflow") ||
		!strings.Contains(session.CurrentTurn.FinalOutput, "pending_review") ||
		!strings.Contains(session.CurrentTurn.FinalOutput, "preflight") ||
		!strings.Contains(session.CurrentTurn.FinalOutput, "verify") {
		t.Fatalf("FinalOutput = %q, want resource workflow contract signals", session.CurrentTurn.FinalOutput)
	}
	var hasModelCall, hasEvidence bool
	var artifactPayload string
	for _, item := range session.CurrentTurn.AgentItems {
		if item.Type == "model_call" {
			hasModelCall = true
		}
		if item.Type == "evidence" {
			hasEvidence = true
		}
		if item.Type == "tool_result" && strings.Contains(string(item.Payload.Data), "runner_workflow_generation") {
			artifactPayload = string(item.Payload.Data)
		}
	}
	if !hasModelCall || !hasEvidence {
		t.Fatalf("agent items = %#v, want model_call and evidence items", session.CurrentTurn.AgentItems)
	}
	for _, want := range []string{"generate_resource_ops_workflow", "pending_review", "data_node", "monitor", "draft_until_reviewed", "secret_ref_only"} {
		if !strings.Contains(artifactPayload, want) {
			t.Fatalf("artifact payload = %s, want %q", artifactPayload, want)
		}
	}
}

func TestChatServiceDispatchesGenericStatefulRepairToRuntime(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := newBlockingChatRuntime()
	service := NewChatService(runtime, sessions, NewAgentEventService(nil))

	result, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID:       "sess-generic-repair",
		Content:         "主机A和主机B的Redis主从集群异常，请帮忙恢复，只需要Redis集群正常运行，sentinel部署在主机C。",
		ClientMessageID: "client-msg-generic-repair",
		ClientTurnID:    "client-turn-generic-repair",
		HostID:          "server-local",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	t.Cleanup(func() { close(runtime.release) })
	if result.Status != "accepted" {
		t.Fatalf("Status = %q, want accepted", result.Status)
	}
	select {
	case req := <-runtime.started:
		if req.Input != "主机A和主机B的Redis主从集群异常，请帮忙恢复，只需要Redis集群正常运行，sentinel部署在主机C。" {
			t.Fatalf("RunTurn input = %q", req.Input)
		}
	case <-time.After(time.Second):
		t.Fatal("RunTurn was not called for generic stateful repair")
	}
}

func TestChatServiceDoesNotReviseActiveWorkflowForNewStatefulRepairRequest(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := newBlockingChatRuntime()
	service := NewChatService(runtime, sessions, NewAgentEventService(nil))
	sessionID := "sess-workflow-then-repair"

	first, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID:       sessionID,
		Content:         "帮我写一个workflow,让主机A和主机B的PG两个节点可以通过主机C的pg_mon形成PG集群",
		ClientMessageID: "client-msg-workflow-first",
		ClientTurnID:    "client-turn-workflow-first",
		HostID:          "server-local",
	})
	if err != nil {
		t.Fatalf("first SendMessage() error = %v", err)
	}
	if first.Status != "completed" {
		t.Fatalf("first Status = %q, want completed workflow response", first.Status)
	}
	select {
	case <-runtime.started:
		t.Fatal("runtime RunTurn was called for initial workflow request")
	default:
	}

	second, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID:       sessionID,
		Content:         "主机A和主机B的PG主从集群异常,请帮忙恢复,数据可以不要,只需要PG主从集群可以正常运行,他们的pg_mon部署在主机C.",
		ClientMessageID: "client-msg-repair-second",
		ClientTurnID:    "client-turn-repair-second",
		HostID:          "server-local",
	})
	if err != nil {
		t.Fatalf("second SendMessage() error = %v", err)
	}
	t.Cleanup(func() { close(runtime.release) })
	if second.Status != "accepted" {
		t.Fatalf("second Status = %q, want accepted runtime repair path", second.Status)
	}
	select {
	case req := <-runtime.started:
		if req.ClientMessageID != "client-msg-repair-second" {
			t.Fatalf("runtime ClientMessageID = %q, want repair request", req.ClientMessageID)
		}
		if strings.Contains(req.Input, "写一个workflow") {
			t.Fatalf("runtime input = %q, want new repair request", req.Input)
		}
	case <-time.After(time.Second):
		t.Fatal("RunTurn was not called for new stateful repair request")
	}
}

func TestChatServiceDoesNotTreatWorkflowConfirmationAsNewPlainRequestWithoutActiveSession(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := newCancelledChatRuntime()
	service := NewChatService(runtime, sessions, NewAgentEventService(nil))

	result, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID: "sess-workflowgen-confirm-without-active",
		Content:   "确认生成工作流候选：Redis 运维手册",
		HostID:    "server-local",
		Metadata:  map[string]string{"opsManualAction": "generate_runner_workflow_candidate"},
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if result.Status != "accepted" {
		t.Fatalf("Status = %q, want accepted async runtime path", result.Status)
	}
	select {
	case <-runtime.started:
	case <-time.After(time.Second):
		t.Fatal("runtime RunTurn was not called; confirmation without active workflow session must not create a new workflow plan")
	}
}

func TestChatServiceGeneratesWorkflowDraftFromConfirmationWithoutRuntimeTools(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := newBlockingChatRuntime()
	service := NewChatService(runtime, sessions, NewAgentEventService(nil))

	if _, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID: "sess-workflowgen-confirm",
		Content:   "@add_workflow 每天早上8点抓取AI新闻，提取三条关键内容直接返回给我",
		HostID:    "server-local",
	}); err != nil {
		t.Fatalf("initial SendMessage() error = %v", err)
	}
	result, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID: "sess-workflowgen-confirm",
		Content:   "确认生成工作流候选：AI 新闻摘要工作流",
		HostID:    "server-local",
		Metadata:  map[string]string{"opsManualAction": "generate_runner_workflow_candidate"},
	})
	if err != nil {
		t.Fatalf("confirmation SendMessage() error = %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("Status = %q, want completed", result.Status)
	}
	select {
	case <-runtime.started:
		t.Fatal("runtime RunTurn was called; workflow draft generation should stay inside controlled service")
	default:
	}
	session := sessions.Get("sess-workflowgen-confirm")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("workflow generation did not write confirmation turn")
	}
	if !strings.Contains(session.CurrentTurn.FinalOutput, "静态验证通过") {
		t.Fatalf("FinalOutput = %q, want static validation summary", session.CurrentTurn.FinalOutput)
	}
	if !strings.Contains(session.CurrentTurn.FinalOutput, "Docker") {
		t.Fatalf("FinalOutput = %q, want Docker provider boundary mentioned", session.CurrentTurn.FinalOutput)
	}
	var artifactPayload string
	for _, item := range session.CurrentTurn.AgentItems {
		if item.Type == "tool_result" && strings.Contains(string(item.Payload.Data), "runner_workflow_generation") {
			artifactPayload = string(item.Payload.Data)
			break
		}
	}
	if !strings.Contains(artifactPayload, `"scriptLanguage":"python"`) || !strings.Contains(artifactPayload, `"scriptPreview"`) {
		t.Fatalf("artifact payload = %s, want generated node script details", artifactPayload)
	}
	if !strings.Contains(artifactPayload, `"validationDetails"`) || !strings.Contains(artifactPayload, `"mode":"static"`) {
		t.Fatalf("artifact payload = %s, want validation details", artifactPayload)
	}
}

func TestWorkflowGenerationValidationImagesUsesConfiguredImage(t *testing.T) {
	t.Setenv("AIOPS_WORKFLOW_VALIDATION_IMAGE", "python:3.12-bookworm")

	images := workflowGenerationValidationImages()
	if len(images) != 1 || images[0] != "python:3.12-bookworm" {
		t.Fatalf("workflowGenerationValidationImages() = %#v, want configured image", images)
	}
}

func TestWorkflowGenerationValidationImagesUsesMetadataImage(t *testing.T) {
	t.Setenv("AIOPS_WORKFLOW_VALIDATION_IMAGE", "python:3.12-bookworm")

	images := workflowGenerationValidationImages(map[string]string{
		"workflowValidationImage": "python:3.11-slim",
	})
	if len(images) != 1 || images[0] != "python:3.11-slim" {
		t.Fatalf("workflowGenerationValidationImages(metadata) = %#v, want metadata image", images)
	}
}

func TestChatService_SendMessageAcceptedOnlyStartsRuntimeAsync(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := newBlockingChatRuntime()
	events := NewAgentEventService(nil)
	service := NewChatService(runtime, sessions, events)

	start := time.Now()
	result, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID:       "sess-async",
		Content:         "需要异步执行",
		ClientMessageID: "client-msg-async",
		ClientTurnID:    "client-turn-async",
		HostID:          "server-local",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("SendMessage() took %s, want accepted-only quick return", elapsed)
	}
	if result.Status != "accepted" {
		t.Fatalf("Status = %q, want accepted", result.Status)
	}
	if result.Output != "" {
		t.Fatalf("Output = %q, want empty accepted response", result.Output)
	}
	if result.ClientMessageID != "client-msg-async" || result.ClientTurnID != "client-turn-async" {
		t.Fatalf("client ids = %q/%q", result.ClientMessageID, result.ClientTurnID)
	}

	select {
	case req := <-runtime.started:
		if req.ClientMessageID != "client-msg-async" || req.ClientTurnID != "client-turn-async" {
			t.Fatalf("runtime client ids = %q/%q", req.ClientMessageID, req.ClientTurnID)
		}
	case <-time.After(time.Second):
		t.Fatal("runtime did not start asynchronously")
	}
	close(runtime.release)
	replayed := waitForAgentEvents(t, events, "sess-async", 3)
	if replayed[0].Kind != AgentEventTurn || replayed[0].Phase != AgentEventPhaseRequested {
		t.Fatalf("first event = %s/%s, want turn/requested", replayed[0].Kind, replayed[0].Phase)
	}
	if replayed[1].Kind != AgentEventAgent || replayed[1].Phase != AgentEventPhaseStarted {
		t.Fatalf("second event = %s/%s, want agent/started", replayed[1].Kind, replayed[1].Phase)
	}
	if replayed[2].Kind != AgentEventAgent || replayed[2].Phase != AgentEventPhaseCompleted || replayed[2].Status != AgentEventStatusCompleted {
		t.Fatalf("third event = %s/%s/%s, want agent/completed/completed", replayed[2].Kind, replayed[2].Phase, replayed[2].Status)
	}
}

func TestChatService_SendMessageRecordsAcceptedEventsWhenRequestContextCanceled(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := newBlockingChatRuntime()
	defer close(runtime.release)
	events := NewAgentEventService(nil)
	service := NewChatService(runtime, sessions, events)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := service.SendMessage(ctx, ChatCommand{
		SessionID:       "sess-canceled-request",
		Content:         "请求上下文已取消但 accepted 事件仍应记录",
		ClientMessageID: "client-msg-canceled-request",
		ClientTurnID:    "client-turn-canceled-request",
		HostID:          "server-local",
	}); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}

	select {
	case <-runtime.started:
	case <-time.After(time.Second):
		t.Fatal("runtime did not start asynchronously")
	}
	replayed := waitForAgentEvents(t, events, "sess-canceled-request", 2)
	if replayed[0].Kind != AgentEventTurn || replayed[0].Phase != AgentEventPhaseRequested {
		t.Fatalf("first event = %s/%s, want turn/requested", replayed[0].Kind, replayed[0].Phase)
	}
	if replayed[1].Kind != AgentEventAgent || replayed[1].Phase != AgentEventPhaseStarted {
		t.Fatalf("second event = %s/%s, want agent/started", replayed[1].Kind, replayed[1].Phase)
	}
}

func TestDefaultAsyncTurnRunnerUsesLifecycleContext(t *testing.T) {
	baseCtx, cancel := context.WithCancel(context.Background())
	cancel()
	runtime := newLifecycleContextRuntime()
	runner := defaultAsyncTurnRunner{runtime: runtime, baseContext: baseCtx}

	runner.run(runtimekernel.TurnRequest{SessionID: "sess-lifecycle", TurnID: "turn-lifecycle"})

	select {
	case err := <-runtime.ctxErr:
		if err != context.Canceled {
			t.Fatalf("RunTurn context error = %v, want context.Canceled", err)
		}
	default:
		t.Fatal("RunTurn was not called")
	}
}

func TestChatService_SendMessageCancelledRuntimeDoesNotEmitTerminalFailureOrCompletion(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := newCancelledChatRuntime()
	events := NewAgentEventService(nil)
	service := NewChatService(runtime, sessions, events)

	if _, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID:       "sess-async-cancelled",
		Content:         "需要异步取消",
		ClientMessageID: "client-msg-cancelled",
		ClientTurnID:    "client-turn-cancelled",
		HostID:          "server-local",
	}); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}

	select {
	case <-runtime.started:
	case <-time.After(time.Second):
		t.Fatal("runtime did not start asynchronously")
	}

	replayed := waitForAgentEvents(t, events, "sess-async-cancelled", 2)
	if len(replayed) != 2 {
		t.Fatalf("agent events = %+v, want only requested + agent started", replayed)
	}
	if replayed[0].Kind != AgentEventTurn || replayed[0].Phase != AgentEventPhaseRequested {
		t.Fatalf("first event = %s/%s, want turn/requested", replayed[0].Kind, replayed[0].Phase)
	}
	if replayed[1].Kind != AgentEventAgent || replayed[1].Phase != AgentEventPhaseStarted {
		t.Fatalf("second event = %s/%s, want agent/started", replayed[1].Kind, replayed[1].Phase)
	}
}

func TestChatService_SendMessageBlockedRuntimeDoesNotEmitTerminalFailure(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := newBlockedChatRuntime()
	events := NewAgentEventService(nil)
	service := NewChatService(runtime, sessions, events)

	if _, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID:       "sess-async-blocked",
		Content:         "需要审批",
		ClientMessageID: "client-msg-blocked",
		ClientTurnID:    "client-turn-blocked",
		HostID:          "server-local",
	}); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}

	select {
	case <-runtime.started:
	case <-time.After(time.Second):
		t.Fatal("runtime did not start asynchronously")
	}

	replayed := waitForAgentEvents(t, events, "sess-async-blocked", 3)
	if len(replayed) != 3 {
		t.Fatalf("agent events = %+v, want requested + agent started + agent blocked", replayed)
	}
	if replayed[0].Kind != AgentEventTurn || replayed[0].Phase != AgentEventPhaseRequested {
		t.Fatalf("first event = %s/%s, want turn/requested", replayed[0].Kind, replayed[0].Phase)
	}
	if replayed[1].Kind != AgentEventAgent || replayed[1].Phase != AgentEventPhaseStarted {
		t.Fatalf("second event = %s/%s, want agent/started", replayed[1].Kind, replayed[1].Phase)
	}
	if replayed[2].Kind != AgentEventAgent || replayed[2].Phase != AgentEventPhaseBlocked || replayed[2].Status != AgentEventStatusBlocked {
		t.Fatalf("third event = %s/%s/%s, want agent/blocked/blocked", replayed[2].Kind, replayed[2].Phase, replayed[2].Status)
	}
	for _, event := range replayed {
		if event.Kind == AgentEventTurn && event.Phase == AgentEventPhaseFailed {
			t.Fatalf("blocked runtime emitted terminal failure event: %+v", event)
		}
	}
}

func (r *chatRuntimeCapture) RunTurn(_ context.Context, req runtimekernel.TurnRequest) (runtimekernel.TurnResult, error) {
	r.mu.Lock()
	r.runCalled = true
	r.runReq = req
	r.mu.Unlock()
	return runtimekernel.TurnResult{
		SessionID:       req.SessionID,
		TurnID:          req.TurnID,
		ClientMessageID: req.ClientMessageID,
		ClientTurnID:    req.ClientTurnID,
		Status:          "completed",
	}, nil
}

func (r *chatRuntimeCapture) ResumeTurn(_ context.Context, req runtimekernel.ResumeRequest) (runtimekernel.TurnResult, error) {
	r.mu.Lock()
	r.resumeCalled = true
	r.resumeReq = req
	r.mu.Unlock()
	return runtimekernel.TurnResult{SessionID: req.SessionID, TurnID: req.TurnID, Status: "completed"}, nil
}

func (r *chatRuntimeCapture) CancelTurn(_ context.Context, req runtimekernel.CancelRequest) (runtimekernel.TurnResult, error) {
	r.mu.Lock()
	r.cancelReq = req
	r.mu.Unlock()
	return runtimekernel.TurnResult{SessionID: req.SessionID, TurnID: req.TurnID, Status: "cancelled"}, nil
}

func (r *chatRuntimeCapture) runSnapshot() (runtimekernel.TurnRequest, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.runReq, r.runCalled
}

func (r *chatRuntimeCapture) resumeSnapshot() (runtimekernel.ResumeRequest, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.resumeReq, r.resumeCalled
}

func (r *chatRuntimeCapture) cancelSnapshot() runtimekernel.CancelRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cancelReq
}

func waitForRunTurn(t *testing.T, runtime *chatRuntimeCapture) runtimekernel.TurnRequest {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if req, ok := runtime.runSnapshot(); ok {
			return req
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("RunTurn was not called")
	return runtimekernel.TurnRequest{}
}

func waitForAgentEvents(t *testing.T, events AgentEventService, sessionID string, wantAtLeast int) []AgentEvent {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		replayed, err := events.Replay(context.Background(), sessionID, 0)
		if err != nil {
			t.Fatalf("Replay() error = %v", err)
		}
		if len(replayed) >= wantAtLeast {
			return replayed
		}
		time.Sleep(10 * time.Millisecond)
	}
	replayed, err := events.Replay(context.Background(), sessionID, 0)
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	t.Fatalf("agent events = %+v, want at least %d events", replayed, wantAtLeast)
	return nil
}

type chatHostOpsServiceCapture struct {
	created bool
	command HostMissionCreateCommand
}

func (s *chatHostOpsServiceCapture) CreateMission(_ context.Context, command HostMissionCreateCommand) (HostOperationView, error) {
	s.created = true
	s.command = command
	return HostOperationView{ID: command.ID, Status: "waiting_plan_acceptance", MentionedHosts: mentionViews(command.Mentions)}, nil
}

func (s *chatHostOpsServiceCapture) GetMission(context.Context, string) (HostOperationView, error) {
	return HostOperationView{}, nil
}

func (s *chatHostOpsServiceCapture) AcceptPlan(context.Context, string, string) (HostOperationView, error) {
	return HostOperationView{}, nil
}

func (s *chatHostOpsServiceCapture) RevisePlan(context.Context, string, string) (HostOperationView, error) {
	return HostOperationView{}, nil
}

func (s *chatHostOpsServiceCapture) SendChildMessage(context.Context, string, string) (HostChildAgentView, error) {
	return HostChildAgentView{}, nil
}

func (s *chatHostOpsServiceCapture) StopChildAgent(context.Context, string) (HostChildAgentView, error) {
	return HostChildAgentView{}, nil
}

func (s *chatHostOpsServiceCapture) ChildTranscript(context.Context, string) (HostChildTranscriptView, error) {
	return HostChildTranscriptView{}, nil
}

func hostMentionIDsForTest(mentions []hostops.HostMention) map[string]bool {
	out := map[string]bool{}
	for _, mention := range mentions {
		if mention.HostID != "" {
			out[mention.HostID] = true
		}
	}
	return out
}

func TestChatService_SendMessageResumesPendingEvidenceTurn(t *testing.T) {
	now := time.Now().UTC()
	sessions := runtimekernel.NewSessionManager()
	session := sessions.GetOrCreate("sess-evidence", runtimekernel.SessionTypeWorkspace, runtimekernel.ModeExecute)
	session.CurrentTurn = &runtimekernel.TurnSnapshot{
		ID:          "turn-evidence",
		SessionID:   session.ID,
		SessionType: session.Type,
		Mode:        session.Mode,
		Lifecycle:   runtimekernel.TurnLifecycleSuspended,
		ResumeState: runtimekernel.TurnResumeStatePendingEvidence,
		Iteration:   2,
		StartedAt:   now,
		UpdatedAt:   now,
		PendingEvidence: []runtimekernel.PendingEvidence{{
			ID:         "evidence-1",
			SessionID:  session.ID,
			TurnID:     "turn-evidence",
			Iteration:  2,
			ToolName:   "readonly_host_inspect",
			ToolCallID: "call-1",
			Status:     "pending",
			CreatedAt:  now,
			UpdatedAt:  now,
		}},
	}
	session.PendingEvidence = append([]runtimekernel.PendingEvidence(nil), session.CurrentTurn.PendingEvidence...)
	sessions.Update(session)

	runtime := &chatRuntimeCapture{}
	service := NewChatService(runtime, sessions)

	_, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID: "sess-evidence",
		Content:   "这是补充证据和操作上下文",
		Metadata:  map[string]string{"client": "protocol-workspace"},
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if _, ok := runtime.runSnapshot(); ok {
		t.Fatal("SendMessage() called RunTurn, want ResumeTurn for pending evidence")
	}
	resumeReq, resumeCalled := runtime.resumeSnapshot()
	if !resumeCalled {
		t.Fatal("SendMessage() did not call ResumeTurn")
	}
	if resumeReq.SessionID != "sess-evidence" || resumeReq.TurnID != "turn-evidence" {
		t.Fatalf("ResumeTurn target = %+v, want sess-evidence/turn-evidence", resumeReq)
	}
	if resumeReq.ResumeState != runtimekernel.TurnResumeStatePendingEvidence {
		t.Fatalf("ResumeState = %q, want pending_evidence", resumeReq.ResumeState)
	}
	if resumeReq.CheckpointID != "evidence-1" {
		t.Fatalf("CheckpointID = %q, want evidence-1", resumeReq.CheckpointID)
	}
	if got := resumeReq.Metadata["resume.input"]; got != "这是补充证据和操作上下文" {
		t.Fatalf("metadata[resume.input] = %q, want follow-up content", got)
	}
	if got := resumeReq.Metadata["evidence.id"]; got != "evidence-1" {
		t.Fatalf("metadata[evidence.id] = %q, want evidence-1", got)
	}
}

func TestChatService_SendMessageDefaultsToLatestSessionWhenSessionIDMissing(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	older := sessions.GetOrCreate("sess-older", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	older.UpdatedAt = time.Now().Add(-time.Minute)
	sessions.Update(older)

	latest := sessions.GetOrCreate("sess-latest", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	sessions.Update(latest)

	runtime := &chatRuntimeCapture{}
	service := NewChatService(runtime, sessions)

	_, err := service.SendMessage(context.Background(), ChatCommand{
		Content: "今天几号",
		HostID:  "server-local",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	runReq := waitForRunTurn(t, runtime)
	if runReq.SessionID != latest.ID {
		t.Fatalf("RunTurn sessionId = %q, want latest session %q", runReq.SessionID, latest.ID)
	}
	if runReq.HostID != serverLocalHostID {
		t.Fatalf("RunTurn hostId = %q, want %s", runReq.HostID, serverLocalHostID)
	}
}

func TestChatService_SendMessageUsesSessionModeWhenSessionIDProvided(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	session := sessions.GetOrCreate("sess-host-exec", runtimekernel.SessionTypeHost, runtimekernel.ModeExecute)
	session.HostID = "server-local"
	sessions.Update(session)

	runtime := &chatRuntimeCapture{}
	service := NewChatService(runtime, sessions)

	result, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID:       "sess-host-exec",
		Content:         "帮我启动 docker",
		HostID:          "server-local",
		ClientMessageID: "client-msg-1",
		ClientTurnID:    "client-turn-1",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if result.Status != "accepted" {
		t.Fatalf("result status = %q, want accepted", result.Status)
	}
	runReq := waitForRunTurn(t, runtime)
	if runReq.SessionType != runtimekernel.SessionTypeHost {
		t.Fatalf("RunTurn sessionType = %q, want host", runReq.SessionType)
	}
	if runReq.Mode != runtimekernel.ModeExecute {
		t.Fatalf("RunTurn mode = %q, want execute", runReq.Mode)
	}
	if runReq.SessionID != "sess-host-exec" {
		t.Fatalf("RunTurn sessionID = %q, want sess-host-exec", runReq.SessionID)
	}
}

func TestChatService_SendMessageCarriesClientIDs(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &chatRuntimeCapture{}
	service := NewChatService(runtime, sessions)

	result, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID:       "sess-client",
		Content:         "需要即时反馈",
		ClientMessageID: "client-msg-1",
		ClientTurnID:    "client-turn-1",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	runReq := waitForRunTurn(t, runtime)
	if runReq.ClientMessageID != "client-msg-1" {
		t.Fatalf("RunTurn ClientMessageID = %q, want client-msg-1", runReq.ClientMessageID)
	}
	if runReq.ClientTurnID != "client-turn-1" {
		t.Fatalf("RunTurn ClientTurnID = %q, want client-turn-1", runReq.ClientTurnID)
	}
	if result.ClientMessageID != "client-msg-1" {
		t.Fatalf("TurnResponse ClientMessageID = %q, want client-msg-1", result.ClientMessageID)
	}
	if result.ClientTurnID != "client-turn-1" {
		t.Fatalf("TurnResponse ClientTurnID = %q, want client-turn-1", result.ClientTurnID)
	}
}

func TestChatService_SendMessageInjectsOpsRunMetadata(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &chatRuntimeCapture{}
	service := NewChatService(runtime, sessions)

	result, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID:    "sess-opsrun",
		Content:      "主机A跟主机B上PG不同步，请先只读排查",
		ClientTurnID: "client-turn-opsrun",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	runReq := waitForRunTurn(t, runtime)
	if got := runReq.Metadata["aiops.opsRunId"]; got != "opsrun-"+runReq.TurnID {
		t.Fatalf("opsRun metadata = %q, want opsrun-%s; metadata=%#v", got, runReq.TurnID, runReq.Metadata)
	}
	if got := runReq.Metadata["aiops.chat.source"]; got != "chat" {
		t.Fatalf("chat source metadata = %q, want chat; metadata=%#v", got, runReq.Metadata)
	}
	if got := runReq.Metadata["aiops.sessionId"]; got != "sess-opsrun" {
		t.Fatalf("session metadata = %q, want sess-opsrun; metadata=%#v", got, runReq.Metadata)
	}
	if got := runReq.Metadata["aiops.turnId"]; got != runReq.TurnID {
		t.Fatalf("turn metadata = %q, want %q; metadata=%#v", got, runReq.TurnID, runReq.Metadata)
	}
	if result.OpsRun == nil || result.OpsRun.ID != "opsrun-"+runReq.TurnID {
		t.Fatalf("TurnResponse OpsRun = %#v, want %q", result.OpsRun, "opsrun-"+runReq.TurnID)
	}
	if result.OpsRun.Title != "主机A跟主机B上PG不同步，请先只读排查" {
		t.Fatalf("OpsRun title = %q", result.OpsRun.Title)
	}
}

func TestChatService_SendMessageMarksExplicitCorootRCA(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &chatRuntimeCapture{}
	service := NewChatService(runtime, sessions)

	_, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID: "sess-coroot-rca",
		Content:   "@Coroot checkout 服务异常，请深入分析根因",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	runReq := waitForRunTurn(t, runtime)
	if got := runReq.Metadata["aiops.coroot.explicitRCA"]; got != "true" {
		t.Fatalf("explicit RCA metadata = %q; metadata=%#v", got, runReq.Metadata)
	}
	if got := runReq.Metadata["aiops.coroot.rcaDisplayAllowed"]; got != "true" {
		t.Fatalf("RCA display metadata = %q; metadata=%#v", got, runReq.Metadata)
	}
}

func TestChatService_SendMessageDoesNotMarkCorootRCAWithoutMention(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &chatRuntimeCapture{}
	service := NewChatService(runtime, sessions)

	_, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID: "sess-coroot-evidence",
		Content:   "请结合 Coroot 指标证据排查 checkout 服务异常",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	runReq := waitForRunTurn(t, runtime)
	if got := runReq.Metadata["aiops.coroot.explicitRCA"]; got != "" {
		t.Fatalf("explicit RCA metadata = %q, want empty; metadata=%#v", got, runReq.Metadata)
	}
	if got := runReq.Metadata["aiops.coroot.rcaDisplayAllowed"]; got != "" {
		t.Fatalf("RCA display metadata = %q, want empty; metadata=%#v", got, runReq.Metadata)
	}
}

func TestChatService_SendMessageDefaultsNewHostSessionToServerLocal(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &chatRuntimeCapture{}
	service := NewChatService(runtime, sessions)

	_, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID: "sess-new-host-default",
		Content:   "排查 Redis",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	runReq := waitForRunTurn(t, runtime)
	if runReq.SessionType != runtimekernel.SessionTypeHost {
		t.Fatalf("RunTurn sessionType = %q, want host", runReq.SessionType)
	}
	if runReq.HostID != "server-local" {
		t.Fatalf("RunTurn hostId = %q, want server-local", runReq.HostID)
	}
}

func TestChatService_SendMessageInjectsSelectedHostRuntimeMetadata(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &chatRuntimeCapture{}
	hosts := newHostRepoStub(store.HostRecord{
		ID:          "remote-linux-01",
		Name:        "remote-linux-01",
		Address:     "10.10.20.30",
		Status:      "online",
		Transport:   "agent_http",
		OS:          "linux",
		Arch:        "amd64",
		SSHUser:     "root",
		SSHPort:     22,
		Executable:  true,
		AgentURL:    "http://remote-linux-01:7072",
		ControlMode: "managed",
	})
	service := NewChatServiceWithHosts(runtime, sessions, hosts)

	_, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID:   "sess-remote-linux",
		Content:     "查看远程主机资源",
		SessionType: string(runtimekernel.SessionTypeHost),
		HostID:      "remote-linux-01",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	runReq := waitForRunTurn(t, runtime)
	for key, want := range map[string]string{
		"aiops.host.metadataAvailable": "true",
		"aiops.host.id":                "remote-linux-01",
		"aiops.host.os":                "linux",
		"aiops.host.arch":              "amd64",
		"aiops.host.transport":         "agent_http",
		"aiops.host.status":            "online",
		"aiops.host.address":           "10.10.20.30",
		"aiops.host.sshUser":           "root",
		"aiops.host.sshPort":           "22",
	} {
		if got := runReq.Metadata[key]; got != want {
			t.Fatalf("RunTurn metadata[%s] = %q, want %q; metadata=%#v", key, got, want, runReq.Metadata)
		}
	}
}

func TestChatService_SendMessageRoutesMultiHostMentionToHostOpsMission(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &chatRuntimeCapture{}
	hosts := newHostRepoStub(
		store.HostRecord{ID: "accept-host-a", Name: "@pg-a", Address: "10.10.0.11", Status: "online", Executable: true, AgentURL: "http://pg-a:7072"},
		store.HostRecord{ID: "accept-host-b", Name: "@pg-b", Address: "10.10.0.12", Status: "online", Executable: true, AgentURL: "http://pg-b:7072"},
		store.HostRecord{ID: "accept-host-c", Name: "@pg-mon", Address: "10.10.0.13", Status: "online", Executable: true, AgentURL: "http://pg-mon:7072"},
	)
	hostOps := &chatHostOpsServiceCapture{}
	services := NewServices(runtime, sessions, WithHostRepository(hosts), WithHostOpsService(hostOps))

	result, err := services.ChatService().SendMessage(context.Background(), ChatCommand{
		SessionID: "sess-hostops-chat",
		Content:   "主机A=@pg-a, 主机B=@pg-b, 主机C=@pg-mon。先做通用运维诊断。",
		Metadata: map[string]string{
			"aiops.hostops.clientDetectedMultiHost": "true",
		},
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if result.Status != "accepted" {
		t.Fatalf("Status = %q, want accepted", result.Status)
	}
	if _, ok := runtime.runSnapshot(); ok {
		t.Fatal("RunTurn was called; multi-host mention should create a host-ops mission instead of binding main runtime to server-local")
	}
	if !hostOps.created {
		t.Fatal("HostOpsService.CreateMission was not called")
	}
	if hostOps.command.ID != "hostops:"+result.TurnID {
		t.Fatalf("mission ID = %q, want hostops:%s", hostOps.command.ID, result.TurnID)
	}
	if len(hostOps.command.Mentions) != 3 {
		t.Fatalf("len(Mentions) = %d, want 3: %#v", len(hostOps.command.Mentions), hostOps.command.Mentions)
	}
	for _, want := range []string{"accept-host-a", "accept-host-b", "accept-host-c"} {
		if !hostMentionIDsForTest(hostOps.command.Mentions)[want] {
			t.Fatalf("mentions = %#v, want resolved host %s", hostOps.command.Mentions, want)
		}
	}
}

func TestChatService_SendMessageRoutesV1ClosurePGGoldenPathToGenericHostOps(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &chatRuntimeCapture{}
	hosts := newHostRepoStub(
		store.HostRecord{ID: "host-a", Name: "主机A", Address: "10.10.0.11", Status: "online", Executable: true, AgentURL: "http://host-a:7072"},
		store.HostRecord{ID: "host-b", Name: "主机B", Address: "10.10.0.12", Status: "online", Executable: true, AgentURL: "http://host-b:7072"},
		store.HostRecord{ID: "host-c", Name: "主机C", Address: "10.10.0.13", Status: "online", Executable: true, AgentURL: "http://host-c:7072"},
	)
	hostOps := &chatHostOpsServiceCapture{}
	services := NewServices(runtime, sessions, WithHostRepository(hosts), WithHostOpsService(hostOps))
	mentions, err := json.Marshal([]hostMentionMetadataItem{
		{Raw: "主机A", HostID: "host-a", Address: "10.10.0.11", DisplayName: "主机A", Source: "inventory", Resolved: true, Confidence: 1},
		{Raw: "主机B", HostID: "host-b", Address: "10.10.0.12", DisplayName: "主机B", Source: "inventory", Resolved: true, Confidence: 1},
		{Raw: "主机C", HostID: "host-c", Address: "10.10.0.13", DisplayName: "主机C", Source: "inventory", Resolved: true, Confidence: 1},
	})
	if err != nil {
		t.Fatalf("marshal host mentions: %v", err)
	}

	result, err := services.ChatService().SendMessage(context.Background(), ChatCommand{
		SessionID: "sess-v1-closure-pg-golden-path",
		Content:   "主机A跟主机B上PG不同步，pg_mon部署在主机C，请修复。先只读排查复制状态、延迟、WAL/LSN、角色、pg_mon观测结果和主机网络，确认风险后再进入修复流程；需要执行修复前必须让我审批。",
		Metadata: map[string]string{
			"aiops.hostops.clientDetectedMultiHost": "true",
			"aiops.hostops.mentions":                string(mentions),
		},
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if result.Status != "accepted" {
		t.Fatalf("Status = %q, want accepted", result.Status)
	}
	if _, ok := runtime.runSnapshot(); ok {
		t.Fatal("RunTurn was called; v1 closure golden path should use generic host-ops mission")
	}
	if !hostOps.created {
		t.Fatal("HostOpsService.CreateMission was not called")
	}
	for _, want := range []string{"host-a", "host-b", "host-c"} {
		if !hostMentionIDsForTest(hostOps.command.Mentions)[want] {
			t.Fatalf("mentions = %#v, want resolved host %s", hostOps.command.Mentions, want)
		}
	}
	session := sessions.Get(result.SessionID)
	if session == nil || session.CurrentTurn == nil {
		t.Fatalf("session = %+v, want persisted host-ops turn", session)
	}
	if got := session.CurrentTurn.Metadata["aiops.hostops.planRequired"]; got != "true" {
		t.Fatalf("planRequired metadata = %q, want true; metadata=%#v", got, session.CurrentTurn.Metadata)
	}
	if session.CurrentTurn.Metadata[metadataCorootExplicitRCA] == "true" || session.CurrentTurn.Metadata[metadataCorootRCADisplayAllowed] == "true" {
		t.Fatalf("metadata = %#v, golden path without @Coroot must not show RCA", session.CurrentTurn.Metadata)
	}
}

func TestChatService_SendMessagePersistsHostOpsMissionTurn(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &chatRuntimeCapture{}
	hosts := newHostRepoStub(store.HostRecord{
		ID:         "remote-linux-01",
		Name:       "120.77.239.90",
		Address:    "120.77.239.90",
		Status:     "online",
		Executable: true,
		AgentURL:   "http://120.77.239.90:7072",
	})
	missions := hostops.NewInMemoryMissionStore()
	transcripts := hostops.NewInMemoryTranscriptStore()
	hostOps := NewHostOpsService(missions, transcripts, hostops.NewOrchestrator(missions, transcripts, &hostOpsServiceTestSpawner{}))
	services := NewServices(runtime, sessions, WithHostRepository(hosts), WithHostOpsService(hostOps))

	result, err := services.ChatService().SendMessage(context.Background(), ChatCommand{
		SessionID: "sess-hostops-persist",
		Content:   "这是@120.77.239.90主机,查看其内存情况",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if _, ok := runtime.runSnapshot(); ok {
		t.Fatal("RunTurn was called; host-ops mission should be persisted without binding main runtime to server-local")
	}
	session := sessions.Get(result.SessionID)
	if session == nil || session.CurrentTurn == nil {
		t.Fatalf("session = %+v, want persisted current host-ops turn", session)
	}
	if session.CurrentTurn.ID != result.TurnID {
		t.Fatalf("CurrentTurn.ID = %q, want %q", session.CurrentTurn.ID, result.TurnID)
	}
	if got := session.CurrentTurn.Metadata["aiops.hostops.routeKind"]; got != string(hostops.RouteKindHostOps) {
		t.Fatalf("routeKind metadata = %q, want host_ops; metadata=%#v", got, session.CurrentTurn.Metadata)
	}
	if session.CurrentTurn.Metadata["aiops.hostops.mentions"] == "" {
		t.Fatalf("hostops mentions metadata missing: %#v", session.CurrentTurn.Metadata)
	}
	if len(session.Messages) == 0 || session.Messages[0].Role != "user" || session.Messages[0].Content != "这是@120.77.239.90主机,查看其内存情况" {
		t.Fatalf("Messages = %#v, want persisted user message", session.Messages)
	}
	var hasUserItem bool
	for _, item := range session.CurrentTurn.AgentItems {
		if item.Type == agentstate.TurnItemTypeUserMessage && strings.Contains(item.Payload.Summary, "查看其内存情况") {
			hasUserItem = true
		}
	}
	if !hasUserItem {
		t.Fatalf("AgentItems = %#v, want user message item", session.CurrentTurn.AgentItems)
	}
	projected, err := NewTransportProjector().ProjectTurnSnapshot(NewAiopsTransportState(session.ID, session.ID), session.CurrentTurn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	mission := projected.HostMissions[session.CurrentTurn.Metadata["aiops.hostops.missionId"]]
	if len(mission.ChildAgentIDs) != 1 {
		t.Fatalf("projected mission = %+v, want one persisted child agent", mission)
	}
	child := projected.ChildAgents[mission.ChildAgentIDs[0]]
	if child.HostID != "remote-linux-01" || !strings.Contains(child.Task, "查看其内存情况") {
		t.Fatalf("projected child = %+v, want remote-linux-01 bound to original task", child)
	}
}

func TestChatService_SendMessageRoutesWorkflowWritingBeforeHostOps(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &chatRuntimeCapture{}
	hosts := newHostRepoStub(
		store.HostRecord{ID: "accept-host-a", Name: "@pg-a", Address: "10.10.0.11", Status: "online", Executable: true, AgentURL: "http://pg-a:7072"},
		store.HostRecord{ID: "accept-host-b", Name: "@pg-b", Address: "10.10.0.12", Status: "online", Executable: true, AgentURL: "http://pg-b:7072"},
		store.HostRecord{ID: "accept-host-c", Name: "@pg-mon", Address: "10.10.0.13", Status: "online", Executable: true, AgentURL: "http://pg-mon:7072"},
	)
	hostOps := &chatHostOpsServiceCapture{}
	services := NewServices(runtime, sessions, WithHostRepository(hosts), WithHostOpsService(hostOps))

	result, err := services.ChatService().SendMessage(context.Background(), ChatCommand{
		SessionID: "sess-workflow-before-hostops",
		Content:   "帮我写一个workflow，让主机A=@pg-a和主机B=@pg-b的PG两个节点可以通过主机C=@pg-mon的pg_mon形成PG集群",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("Status = %q, want completed workflow generation response", result.Status)
	}
	if !strings.Contains(result.Output, "Workflow") && !strings.Contains(result.Output, "workflow") {
		t.Fatalf("Output = %q, want workflow generation response", result.Output)
	}
	if hostOps.created {
		t.Fatalf("workflow writing request was routed to HostOpsService.CreateMission: %+v", hostOps.command)
	}
	if _, ok := runtime.runSnapshot(); ok {
		t.Fatal("RunTurn was called; workflow writing request should be handled by workflow generation service")
	}
}

func TestChatService_SendMessageDoesNotTreatUnresolvedToolMentionAsHostOps(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &chatRuntimeCapture{}
	hosts := newHostRepoStub(store.HostRecord{ID: "accept-host-a", Name: "@pg-a", Address: "10.10.0.11", Status: "online"})
	hostOps := &chatHostOpsServiceCapture{}
	services := NewServices(runtime, sessions, WithHostRepository(hosts), WithHostOpsService(hostOps))

	_, err := services.ChatService().SendMessage(context.Background(), ChatCommand{
		SessionID: "sess-coroot-chat",
		Content:   "@coroot 分析环境A的A服务为什么异常",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if hostOps.created {
		t.Fatalf("@coroot was routed to HostOpsService.CreateMission: %+v", hostOps.command)
	}
	_ = waitForRunTurn(t, runtime)
}

func TestChatService_CancelTurnAppendsCanceledAgentEvent(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &chatRuntimeCapture{}
	events := NewAgentEventService(nil)
	service := NewChatService(runtime, sessions, events)

	result, err := service.CancelTurn(context.Background(), CancelCommand{
		SessionID: "sess-cancel",
		TurnID:    "turn-cancel",
		Reason:    "user stop",
	})
	if err != nil {
		t.Fatalf("CancelTurn() error = %v", err)
	}
	if result.Status != "cancelled" {
		t.Fatalf("CancelTurn status = %q, want cancelled", result.Status)
	}

	replayed, err := events.Replay(context.Background(), "sess-cancel", 0)
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if len(replayed) != 2 {
		t.Fatalf("agent events = %+v, want agent canceled and turn canceled events", replayed)
	}
	if replayed[0].Kind != AgentEventAgent || replayed[0].Phase != AgentEventPhaseCanceled || replayed[0].Status != AgentEventStatusCanceled {
		t.Fatalf("agent cancel event = %s/%s/%s, want agent/canceled/canceled", replayed[0].Kind, replayed[0].Phase, replayed[0].Status)
	}
	if replayed[1].Kind != AgentEventTurn || replayed[1].Phase != AgentEventPhaseCanceled || replayed[1].Status != AgentEventStatusCanceled {
		t.Fatalf("turn cancel event = %s/%s/%s, want turn/canceled/canceled", replayed[1].Kind, replayed[1].Phase, replayed[1].Status)
	}
}

func TestChatService_StopTurnAppendsCanceledAgentEvent(t *testing.T) {
	now := time.Now().UTC()
	sessions := runtimekernel.NewSessionManager()
	session := sessions.GetOrCreate("sess-stop", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	session.CurrentTurn = &runtimekernel.TurnSnapshot{
		ID:        "turn-stop",
		SessionID: session.ID,
		Lifecycle: runtimekernel.TurnLifecycleRunning,
		StartedAt: now,
		UpdatedAt: now,
	}
	sessions.Update(session)
	runtime := &chatRuntimeCapture{}
	events := NewAgentEventService(nil)
	service := NewChatService(runtime, sessions, events)

	result, err := service.StopTurn(context.Background(), StopCommand{
		SessionID: "sess-stop",
		Reason:    "user stop",
	})
	if err != nil {
		t.Fatalf("StopTurn() error = %v", err)
	}
	if result.Status != "cancelled" {
		t.Fatalf("StopTurn status = %q, want cancelled", result.Status)
	}

	replayed, err := events.Replay(context.Background(), "sess-stop", 0)
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if len(replayed) != 2 {
		t.Fatalf("agent events = %+v, want agent canceled and turn canceled events", replayed)
	}
	if replayed[0].Kind != AgentEventAgent || replayed[0].TurnID != "turn-stop" || replayed[0].Phase != AgentEventPhaseCanceled || replayed[0].Status != AgentEventStatusCanceled {
		t.Fatalf("agent stop event = %+v, want turn-stop agent canceled event", replayed[0])
	}
	if replayed[1].Kind != AgentEventTurn || replayed[1].TurnID != "turn-stop" || replayed[1].Phase != AgentEventPhaseCanceled || replayed[1].Status != AgentEventStatusCanceled {
		t.Fatalf("turn stop event = %+v, want turn-stop canceled event", replayed[1])
	}
}

func TestChatService_StopTurnCancelsAcceptedTurnByExplicitIDsWithoutCurrentTurn(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	sessions.GetOrCreate("sess-explicit-stop", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	runtime := &chatRuntimeCapture{}
	events := NewAgentEventService(nil)
	service := NewChatService(runtime, sessions, events)

	result, err := service.StopTurn(context.Background(), StopCommand{
		SessionID: "sess-explicit-stop",
		TurnID:    "turn-explicit-stop",
		Reason:    "user stop",
	})
	if err != nil {
		t.Fatalf("StopTurn() error = %v", err)
	}
	if result.Status != "cancelled" {
		t.Fatalf("StopTurn status = %q, want cancelled", result.Status)
	}
	cancelReq := runtime.cancelSnapshot()
	if cancelReq.SessionID != "sess-explicit-stop" || cancelReq.TurnID != "turn-explicit-stop" {
		t.Fatalf("CancelTurn request = %+v, want explicit session/turn ids", cancelReq)
	}

	replayed, err := events.Replay(context.Background(), "sess-explicit-stop", 0)
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if len(replayed) != 2 {
		t.Fatalf("agent events = %+v, want agent canceled and turn canceled events", replayed)
	}
}
