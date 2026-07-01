package appui

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"aiops-v2/internal/hostops"
	"aiops-v2/internal/runtimecontract"
	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/store"
)

type chatRuntimeCapture struct {
	mu           sync.Mutex
	runCalled    bool
	runReq       runtimekernel.TurnRequest
	runResult    runtimekernel.TurnResult
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
	settings := store.DefaultRuntimeSettings()
	settings.Workflow.ValidationImage = "python:3.12-bookworm"

	images := workflowGenerationValidationImages(settings)
	if len(images) != 1 || images[0] != "python:3.12-bookworm" {
		t.Fatalf("workflowGenerationValidationImages() = %#v, want configured image", images)
	}
}

func TestWorkflowGenerationValidationImagesUsesMetadataImage(t *testing.T) {
	settings := store.DefaultRuntimeSettings()
	settings.Workflow.ValidationImage = "python:3.12-bookworm"

	images := workflowGenerationValidationImages(settings, map[string]string{
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
	result := r.runResult
	r.mu.Unlock()
	if result.Status != "" {
		if result.SessionID == "" {
			result.SessionID = req.SessionID
		}
		if result.TurnID == "" {
			result.TurnID = req.TurnID
		}
		if result.ClientMessageID == "" {
			result.ClientMessageID = req.ClientMessageID
		}
		if result.ClientTurnID == "" {
			result.ClientTurnID = req.ClientTurnID
		}
		return result, nil
	}
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

func (r *chatRuntimeCapture) resetRunSnapshot() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runReq = runtimekernel.TurnRequest{}
	r.runCalled = false
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

func TestPendingInputChatServiceUsesActiveTurnRuntimePath(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	now := time.Now().UTC()
	session := sessions.GetOrCreate("sess-appui-active", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	session.ActiveTurn = runtimekernel.ActiveTurnState{TurnID: "turn-active", Kind: "regular", Status: string(runtimekernel.TurnLifecycleRunning)}
	session.CurrentTurn = &runtimekernel.TurnSnapshot{
		ID:          "turn-active",
		SessionID:   session.ID,
		SessionType: session.Type,
		Mode:        session.Mode,
		Lifecycle:   runtimekernel.TurnLifecycleRunning,
		ResumeState: runtimekernel.TurnResumeStateNone,
		StartedAt:   now,
		UpdatedAt:   now,
	}
	sessions.Update(session)
	runtime := &chatRuntimeCapture{runResult: runtimekernel.TurnResult{
		SessionID:       session.ID,
		TurnID:          "turn-active",
		ClientTurnID:    "client-turn-pending",
		ClientMessageID: "client-message-pending",
		Status:          "pending_input",
	}}
	service := NewChatService(runtime, sessions)

	resp, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID:       session.ID,
		SessionType:     string(runtimekernel.SessionTypeHost),
		Mode:            string(runtimekernel.ModeChat),
		ClientTurnID:    "client-turn-pending",
		ClientMessageID: "client-message-pending",
		Content:         "补充一个条件",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if resp.Status != "pending_input" || resp.TurnID != "turn-active" {
		t.Fatalf("response = %#v, want pending_input on active turn", resp)
	}
	req, ok := runtime.runSnapshot()
	if !ok {
		t.Fatal("runtime RunTurn was not called")
	}
	if req.Input != "补充一个条件" || req.ClientMessageID != "client-message-pending" {
		t.Fatalf("runtime request = %#v, want pending input request", req)
	}
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
	if runReq.HostID != "" {
		t.Fatalf("RunTurn hostId = %q, want empty advisory binding", runReq.HostID)
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
	if runReq.SessionType != runtimekernel.SessionTypeWorkspace {
		t.Fatalf("RunTurn sessionType = %q, want workspace advisory", runReq.SessionType)
	}
	if runReq.Mode != runtimekernel.ModeChat {
		t.Fatalf("RunTurn mode = %q, want chat advisory", runReq.Mode)
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
	if got := runReq.Metadata["aiops.mentions.observabilityProvider"]; got != "coroot" {
		t.Fatalf("observability provider metadata = %q; metadata=%#v", got, runReq.Metadata)
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
	if got := runReq.Metadata["aiops.coroot.explicitRCA"]; got != "false" {
		t.Fatalf("explicit RCA metadata = %q, want false; metadata=%#v", got, runReq.Metadata)
	}
	if got := runReq.Metadata["aiops.tool.corootRCAAllowed"]; got != "false" {
		t.Fatalf("RCA allowed metadata = %q, want false; metadata=%#v", got, runReq.Metadata)
	}
}

func TestChatService_SendMessageDoesNotDefaultNewHostSessionToServerLocal(t *testing.T) {
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
	if runReq.SessionType != runtimekernel.SessionTypeWorkspace {
		t.Fatalf("RunTurn sessionType = %q, want workspace advisory", runReq.SessionType)
	}
	if runReq.HostID != "" {
		t.Fatalf("RunTurn hostId = %q, want empty advisory binding", runReq.HostID)
	}
	if got := runReq.Metadata["aiops.target.binding"]; got != "none" {
		t.Fatalf("target binding = %q; metadata=%#v", got, runReq.Metadata)
	}
}

func TestChatServicePlainQuestionDoesNotBindServerLocal(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &chatRuntimeCapture{}
	service := NewChatService(runtime, sessions)

	_, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID:   "sess-v2-advisory",
		SessionType: string(runtimekernel.SessionTypeHost),
		HostID:      "server-local",
		Content:     "pg_auto_failover timeline 为什么会比主库高？",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	runReq := waitForRunTurn(t, runtime)
	if runReq.HostID != "" {
		t.Fatalf("RunTurn hostId = %q, want empty for advisory", runReq.HostID)
	}
	if runReq.SessionType != runtimekernel.SessionTypeWorkspace {
		t.Fatalf("RunTurn sessionType = %q, want workspace", runReq.SessionType)
	}
	if got := runReq.Metadata["aiops.route.mode"]; got != string(ChatRouteAdvisory) {
		t.Fatalf("route mode = %q; metadata=%#v", got, runReq.Metadata)
	}
	if got := runReq.Metadata["aiops.tool.execCommandAllowed"]; got != "false" {
		t.Fatalf("exec allowed = %q; metadata=%#v", got, runReq.Metadata)
	}
}

func TestChatServicePastedEvidenceSetsEvidenceMetadata(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &chatRuntimeCapture{}
	service := NewChatService(runtime, sessions)

	_, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID:   "sess-v2-evidence",
		SessionType: string(runtimekernel.SessionTypeHost),
		HostID:      "server-local",
		Content:     "不要执行命令，只基于输出分析：\npostgres=# select pg_is_in_recovery();\n f\npg_controldata: Latest checkpoint's TimeLineID: 11\nls: cannot access 'standby.signal': No such file or directory",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	runReq := waitForRunTurn(t, runtime)
	if runReq.HostID != "" {
		t.Fatalf("RunTurn hostId = %q, want empty for evidence RCA", runReq.HostID)
	}
	if got := runReq.Metadata["aiops.route.mode"]; got != string(ChatRouteEvidenceRCA) {
		t.Fatalf("route mode = %q; metadata=%#v", got, runReq.Metadata)
	}
	if got := runReq.Metadata["aiops.userEvidence.present"]; got != "true" {
		t.Fatalf("user evidence present = %q; metadata=%#v", got, runReq.Metadata)
	}
	if got := runReq.Metadata["aiops.tool.execCommandAllowed"]; got != "false" {
		t.Fatalf("exec allowed = %q; metadata=%#v", got, runReq.Metadata)
	}
}

func TestChatServiceLocalMentionBindsServerLocal(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &chatRuntimeCapture{}
	service := NewChatService(runtime, sessions)

	_, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID:   "sess-v2-local",
		SessionType: string(runtimekernel.SessionTypeHost),
		Content:     "@local 帮我只读检查 PG 状态",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	runReq := waitForRunTurn(t, runtime)
	if runReq.HostID != "server-local" {
		t.Fatalf("RunTurn hostId = %q, want server-local", runReq.HostID)
	}
	if got := runReq.Metadata["aiops.route.mode"]; got != string(ChatRouteHostBoundOps) {
		t.Fatalf("route mode = %q; metadata=%#v", got, runReq.Metadata)
	}
	if got := runReq.Metadata["aiops.tool.execCommandAllowed"]; got != "true" {
		t.Fatalf("exec allowed = %q; metadata=%#v", got, runReq.Metadata)
	}
}

func TestChatServiceRawMentionFallbackMarksMentionSource(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &chatRuntimeCapture{}
	service := NewChatService(runtime, sessions)

	_, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID: "sess-raw-fallback",
		Content:   "@local 检查状态",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	runReq := waitForRunTurn(t, runtime)
	if got := runReq.Metadata["aiops.input.mentionSource"]; got != "raw_text_fallback" {
		t.Fatalf("mentionSource = %q, want raw_text_fallback; metadata=%#v", got, runReq.Metadata)
	}
	if got := runReq.Metadata["aiops.input.mentionValidation"]; got != "confirmed" {
		t.Fatalf("mentionValidation = %q, want confirmed; metadata=%#v", got, runReq.Metadata)
	}
}

func TestChatServiceLegacyHostMetadataMarksMentionSource(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &chatRuntimeCapture{}
	service := NewChatService(runtime, sessions)

	_, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID: "sess-legacy-hostops-metadata",
		Content:   "@local 检查状态",
		Metadata: map[string]string{
			"aiops.hostops.mentions": `[{"raw":"@local","value":"server-local","hostId":"server-local","address":"server-local","displayName":"local","source":"local_alias","resolved":true,"confidence":1}]`,
		},
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	runReq := waitForRunTurn(t, runtime)
	if got := runReq.Metadata["aiops.input.mentionSource"]; got != "legacy_hostops_metadata" {
		t.Fatalf("mentionSource = %q, want legacy_hostops_metadata; metadata=%#v", got, runReq.Metadata)
	}
}

func TestChatServiceStrictMentionModeDoesNotBindRawTextFallback(t *testing.T) {
	t.Setenv("AIOPS_INPUT_MENTION_STRICT", "1")
	sessions := runtimekernel.NewSessionManager()
	runtime := &chatRuntimeCapture{}
	service := NewChatService(runtime, sessions)

	_, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID: "sess-strict-raw",
		Content:   "@local 检查状态",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	runReq := waitForRunTurn(t, runtime)
	if runReq.HostID != "" {
		t.Fatalf("RunTurn HostID = %q, want empty in strict raw mode; metadata=%#v", runReq.HostID, runReq.Metadata)
	}
	if got := runReq.Metadata["aiops.input.mentionSource"]; got != "raw_text_fallback" {
		t.Fatalf("mentionSource = %q, want raw_text_fallback; metadata=%#v", got, runReq.Metadata)
	}
	if got := runReq.Metadata["aiops.input.mentionValidation"]; got != "weak" {
		t.Fatalf("mentionValidation = %q, want weak; metadata=%#v", got, runReq.Metadata)
	}
}

func TestChatServiceStructuredHostMentionBindsAfterServerResolution(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &chatRuntimeCapture{}
	hosts := newHostRepoStub(store.HostRecord{
		ID:         "host-a",
		Name:       "pg-primary",
		Address:    "120.77.239.90",
		Status:     "online",
		Executable: true,
		AgentURL:   "http://host-a:7072",
	})
	service := NewChatServiceWithHosts(runtime, sessions, hosts)

	_, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID: "sess-structured-host",
		Content:   "@120.77.239.90 检查状态",
		Metadata: map[string]string{
			metadataInputMentionsV1: `{"version":1,"mentions":[{"version":1,"tokenId":"mention-0-host-a","sigil":"@","display":"@120.77.239.90","rawText":"@120.77.239.90","kind":"host","path":"host://host-a","source":"selection","range":{"start":0,"end":14},"payload":{"hostId":"host-a","address":"120.77.239.90","displayName":"pg-primary"}}]}`,
		},
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	runReq := waitForRunTurn(t, runtime)
	if runReq.HostID != "host-a" {
		t.Fatalf("RunTurn HostID = %q, want host-a; metadata=%#v", runReq.HostID, runReq.Metadata)
	}
	if got := runReq.Metadata["aiops.input.mentionSource"]; got != "structured" {
		t.Fatalf("mentionSource = %q, want structured; metadata=%#v", got, runReq.Metadata)
	}
	if got := runReq.Metadata["aiops.input.mentionValidation"]; got != "confirmed" {
		t.Fatalf("mentionValidation = %q, want confirmed; metadata=%#v", got, runReq.Metadata)
	}
	if got := runReq.Metadata["aiops.route.mode"]; got != string(ChatRouteHostBoundOps) {
		t.Fatalf("route mode = %q, want host_bound_ops; metadata=%#v", got, runReq.Metadata)
	}
}

func TestChatServiceStructuredHostMentionFailsClosedWhenStale(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &chatRuntimeCapture{}
	hosts := newHostRepoStub(store.HostRecord{ID: "host-a", Name: "pg-primary", Address: "120.77.239.90", Status: "online", Executable: true, AgentURL: "http://host-a:7072"})
	service := NewChatServiceWithHosts(runtime, sessions, hosts)

	_, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID: "sess-structured-stale",
		Content:   "@host-b 检查状态",
		Metadata: map[string]string{
			metadataInputMentionsV1: `{"version":1,"mentions":[{"version":1,"tokenId":"mention-0-host-a","sigil":"@","display":"@120.77.239.90","rawText":"@120.77.239.90","kind":"host","path":"host://host-a","source":"selection","range":{"start":0,"end":14},"payload":{"hostId":"host-a","address":"120.77.239.90","displayName":"pg-primary"}}]}`,
		},
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	runReq := waitForRunTurn(t, runtime)
	if runReq.HostID != "" {
		t.Fatalf("RunTurn HostID = %q, want fail-closed empty host; metadata=%#v", runReq.HostID, runReq.Metadata)
	}
	if got := runReq.Metadata["aiops.input.mentionValidation"]; got != "invalid" {
		t.Fatalf("mentionValidation = %q, want invalid; metadata=%#v", got, runReq.Metadata)
	}
	if got := runReq.Metadata["aiops.tool.execCommandAllowed"]; got != "false" {
		t.Fatalf("exec allowed = %q, want false; metadata=%#v", got, runReq.Metadata)
	}
}

func TestChatServiceStructuredHostMentionFailsClosedWhenHostMissing(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &chatRuntimeCapture{}
	service := NewChatServiceWithHosts(runtime, sessions, newHostRepoStub())

	_, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID: "sess-structured-missing",
		Content:   "@missing 检查状态",
		Metadata: map[string]string{
			metadataInputMentionsV1: `{"version":1,"mentions":[{"version":1,"tokenId":"mention-0-missing","sigil":"@","display":"@missing","rawText":"@missing","kind":"host","path":"host://missing","source":"selection","range":{"start":0,"end":8},"payload":{"hostId":"missing","address":"10.255.255.255","displayName":"missing"}}]}`,
		},
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	runReq := waitForRunTurn(t, runtime)
	if runReq.HostID != "" {
		t.Fatalf("RunTurn HostID = %q, want empty for missing host; metadata=%#v", runReq.HostID, runReq.Metadata)
	}
	if got := runReq.Metadata["aiops.input.mentionValidation"]; got != "invalid" {
		t.Fatalf("mentionValidation = %q, want invalid; metadata=%#v", got, runReq.Metadata)
	}
}

func TestChatServiceStructuredCorootCapabilityDoesNotBindHost(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &chatRuntimeCapture{}
	service := NewChatService(runtime, sessions)

	_, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID: "sess-structured-coroot",
		Content:   "@Coroot 分析 checkout",
		Metadata: map[string]string{
			metadataInputMentionsV1: `{"version":1,"mentions":[{"version":1,"tokenId":"mention-0-coroot","sigil":"@","display":"@Coroot","rawText":"@Coroot","kind":"capability","path":"capability://coroot","source":"selection","range":{"start":0,"end":7}}]}`,
		},
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	runReq := waitForRunTurn(t, runtime)
	if runReq.HostID != "" {
		t.Fatalf("RunTurn HostID = %q, want no host binding; metadata=%#v", runReq.HostID, runReq.Metadata)
	}
	if got := runReq.Metadata["aiops.coroot.explicitRCA"]; got != "true" {
		t.Fatalf("coroot explicit = %q, want true; metadata=%#v", got, runReq.Metadata)
	}
	if got := runReq.Metadata["aiops.tool.execCommandAllowed"]; got != "false" {
		t.Fatalf("exec allowed = %q, want false; metadata=%#v", got, runReq.Metadata)
	}
}

func TestChatServiceStructuredOpsManualsCapabilityEnablesToolSurface(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &chatRuntimeCapture{}
	service := NewChatService(runtime, sessions)

	_, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID: "sess-structured-manuals",
		Content:   "@manuals 搜索 Redis 巡检流程",
		Metadata: map[string]string{
			metadataInputMentionsV1: `{"version":1,"mentions":[{"version":1,"tokenId":"mention-0-manuals","sigil":"@","display":"@manuals","rawText":"@manuals","kind":"capability","path":"capability://ops_manuals","source":"selection","range":{"start":0,"end":8}}]}`,
		},
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	runReq := waitForRunTurn(t, runtime)
	if runReq.HostID != "" {
		t.Fatalf("RunTurn HostID = %q, want no host binding; metadata=%#v", runReq.HostID, runReq.Metadata)
	}
	if got := runReq.Metadata["aiops.opsManuals.explicitMention"]; got != "true" {
		t.Fatalf("ops manuals explicit = %q, want true; metadata=%#v", got, runReq.Metadata)
	}
	if !strings.Contains(runReq.Metadata["enableToolPack"], "ops_manual_flow") {
		t.Fatalf("enableToolPack = %q, want ops_manual_flow; metadata=%#v", runReq.Metadata["enableToolPack"], runReq.Metadata)
	}
	if !strings.Contains(runReq.Metadata["enableTool"], "search_ops_manuals") {
		t.Fatalf("enableTool = %q, want search_ops_manuals; metadata=%#v", runReq.Metadata["enableTool"], runReq.Metadata)
	}
}

func TestChatServiceBareInventoryHostDoesNotBindOrAllowExec(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &chatRuntimeCapture{}
	hosts := newHostRepoStub(store.HostRecord{
		ID:          "host-a",
		Name:        "db-a",
		Address:     "10.10.0.11",
		Status:      "online",
		AgentStatus: "online",
		Transport:   "agent_http",
		OS:          "linux",
		Arch:        "amd64",
		Executable:  true,
		AgentURL:    "http://host-a:7072",
	})
	service := NewChatServiceWithHosts(runtime, sessions, hosts)

	_, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID: "sess-bare-host-a-readonly",
		Content:   "在 host-a 上只读检查 CPU、内存和磁盘空间，并给出证据摘要。",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	runReq := waitForRunTurn(t, runtime)
	if runReq.HostID != "" {
		t.Fatalf("RunTurn hostId = %q, want empty without @host or selected host context", runReq.HostID)
	}
	if got := runReq.Metadata["aiops.route.mode"]; got != string(ChatRouteAdvisory) {
		t.Fatalf("route mode = %q; metadata=%#v", got, runReq.Metadata)
	}
	if got := runReq.Metadata["aiops.tool.execCommandAllowed"]; got != "false" {
		t.Fatalf("exec allowed = %q; metadata=%#v", got, runReq.Metadata)
	}
	if got := runReq.Metadata["aiops.target.hostId"]; got != "" {
		t.Fatalf("target host metadata = %q; metadata=%#v", got, runReq.Metadata)
	}
	if got := runReq.Metadata["aiops.host.os"]; got != "" {
		t.Fatalf("host os metadata = %q; metadata=%#v", got, runReq.Metadata)
	}
}

func TestChatServiceRouteProfileCanSwitchPerTurnInSameSession(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &chatRuntimeCapture{}
	service := NewChatService(runtime, sessions)
	const sessionID = "sess-v2-route-switch"

	first, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID:   sessionID,
		SessionType: string(runtimekernel.SessionTypeHost),
		HostID:      serverLocalHostID,
		Content:     "解释这个中间件同步异常可能有哪些通用原因，不要执行命令",
	})
	if err != nil {
		t.Fatalf("first SendMessage() error = %v", err)
	}
	firstReq := waitForRunTurn(t, runtime)
	if first.SessionID != sessionID || firstReq.SessionID != sessionID {
		t.Fatalf("first session response/request = %q/%q, want %q", first.SessionID, firstReq.SessionID, sessionID)
	}
	if firstReq.Metadata["toolProfile"] != string(ChatRouteAdvisory) {
		t.Fatalf("first toolProfile = %q; metadata=%#v", firstReq.Metadata["toolProfile"], firstReq.Metadata)
	}
	if firstReq.HostID != "" {
		t.Fatalf("first HostID = %q, want empty advisory binding", firstReq.HostID)
	}

	runtime.resetRunSnapshot()
	second, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID:   sessionID,
		SessionType: string(runtimekernel.SessionTypeHost),
		Content:     "@local 只读检查当前系统状态",
	})
	if err != nil {
		t.Fatalf("second SendMessage() error = %v", err)
	}
	secondReq := waitForRunTurn(t, runtime)
	if second.SessionID != sessionID || secondReq.SessionID != sessionID {
		t.Fatalf("second session response/request = %q/%q, want %q", second.SessionID, secondReq.SessionID, sessionID)
	}
	if secondReq.Metadata["toolProfile"] != string(ChatRouteHostBoundOps) {
		t.Fatalf("second toolProfile = %q; metadata=%#v", secondReq.Metadata["toolProfile"], secondReq.Metadata)
	}
	if secondReq.HostID != serverLocalHostID {
		t.Fatalf("second HostID = %q, want %s", secondReq.HostID, serverLocalHostID)
	}
	if first.OpsRun == nil || second.OpsRun == nil || first.OpsRun.ID == "" || second.OpsRun.ID == "" {
		t.Fatalf("ops run metadata missing: first=%+v second=%+v", first.OpsRun, second.OpsRun)
	}
}

func TestChatServiceSelectedRemoteHostDiagnosticFollowupKeepsExecToolSurface(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	session := sessions.GetOrCreate("sess-selected-remote-followup", runtimekernel.SessionTypeHost, runtimekernel.ModeExecute)
	session.HostID = "host-a"
	sessions.Update(session)
	runtime := &chatRuntimeCapture{}
	hosts := newHostRepoStub(store.HostRecord{
		ID:         "host-a",
		Name:       "host-a",
		Address:    "120.77.239.90",
		Status:     "online",
		Executable: true,
		AgentURL:   "http://host-a:7072",
	})
	service := NewChatServiceWithHosts(runtime, sessions, hosts)

	_, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID: "sess-selected-remote-followup",
		Content:   "为什么120.77.239.90没注册? 明明注册了,在主机列表中,你再看看",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	runReq := waitForRunTurn(t, runtime)
	if runReq.HostID != "host-a" {
		t.Fatalf("RunTurn HostID = %q, want host-a; metadata=%#v", runReq.HostID, runReq.Metadata)
	}
	if got := runReq.Metadata["aiops.route.mode"]; got != string(ChatRouteHostBoundOps) {
		t.Fatalf("route mode = %q, want host_bound_ops; metadata=%#v", got, runReq.Metadata)
	}
	if got := runReq.Metadata["aiops.tool.execCommandAllowed"]; got != "true" {
		t.Fatalf("exec allowed = %q, want true; metadata=%#v", got, runReq.Metadata)
	}
	if got := runReq.Metadata["toolProfile"]; got != string(ChatRouteHostBoundOps) {
		t.Fatalf("toolProfile = %q, want host_bound_ops; metadata=%#v", got, runReq.Metadata)
	}
}

func TestChatServiceShortFollowupUsesConcisePromptProfile(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	session := sessions.GetOrCreate("sess-followup-profile", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	session.Messages = []runtimekernel.Message{
		{ID: "msg-user-1", Role: "user", Content: "请分析这段现象", Timestamp: time.Now().Add(-time.Minute)},
		{ID: "msg-assistant-1", Role: "assistant", Content: "结论：已有一轮完整分析。", Timestamp: time.Now().Add(-30 * time.Second)},
	}
	sessions.Update(session)
	runtime := &chatRuntimeCapture{}
	service := NewChatService(runtime, sessions)

	if _, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID:   session.ID,
		SessionType: string(runtimekernel.SessionTypeHost),
		Content:     "下一步呢？",
	}); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	runReq := waitForRunTurn(t, runtime)
	if got := runReq.Metadata[metadataTurnFollowup]; got != "true" {
		t.Fatalf("followup metadata = %q; metadata=%#v", got, runReq.Metadata)
	}
	if got := runReq.Metadata[metadataTurnHasExistingEvidence]; got != "true" {
		t.Fatalf("existing evidence metadata = %q; metadata=%#v", got, runReq.Metadata)
	}
	if got := runReq.Metadata[metadataTurnNoNewEvidence]; got != "true" {
		t.Fatalf("no new evidence metadata = %q; metadata=%#v", got, runReq.Metadata)
	}
	if runReq.Metadata["reasoningEffort"] != "low" || runReq.Metadata["answerStyle"] != "concise" {
		t.Fatalf("prompt profile metadata = %#v, want low/concise", runReq.Metadata)
	}
}

func TestChatServiceShortInputWithNewEvidenceKeepsNormalPromptProfile(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	session := sessions.GetOrCreate("sess-followup-new-evidence", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	session.Messages = []runtimekernel.Message{
		{ID: "msg-assistant-1", Role: "assistant", Content: "结论：已有一轮完整分析。", Timestamp: time.Now().Add(-30 * time.Second)},
	}
	sessions.Update(session)
	runtime := &chatRuntimeCapture{}
	service := NewChatService(runtime, sessions)

	if _, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID:   session.ID,
		SessionType: string(runtimekernel.SessionTypeHost),
		Content:     "error: timeout",
	}); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	runReq := waitForRunTurn(t, runtime)
	if got := runReq.Metadata[metadataTurnFollowup]; got != "" {
		t.Fatalf("followup metadata = %q, want empty for new evidence; metadata=%#v", got, runReq.Metadata)
	}
	if got := runReq.Metadata["reasoningEffort"]; got == "low" {
		t.Fatalf("reasoningEffort = %q, want not lowered for new evidence; metadata=%#v", got, runReq.Metadata)
	}
	if got := runReq.Metadata["answerStyle"]; got == "concise" {
		t.Fatalf("answerStyle = %q, want not lowered for new evidence; metadata=%#v", got, runReq.Metadata)
	}
}

func TestChatServiceMultipleHostMentionsCreateHostOpsMission(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &chatRuntimeCapture{}
	hosts := newHostRepoStub(
		store.HostRecord{ID: "v2-host-a", Name: "hostA", Address: "10.10.1.11", Status: "online", Executable: true, AgentURL: "http://host-a:7072"},
		store.HostRecord{ID: "v2-host-b", Name: "hostB", Address: "10.10.1.12", Status: "online", Executable: true, AgentURL: "http://host-b:7072"},
	)
	hostOps := &chatHostOpsServiceCapture{}
	services := NewServices(runtime, sessions, WithHostRepository(hosts), WithHostOpsService(hostOps))

	result, err := services.ChatService().SendMessage(context.Background(), ChatCommand{
		SessionID: "sess-v2-multi-host",
		Content:   "@hostA @hostB 对比 PG 状态",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	runReq := waitForRunTurn(t, runtime)
	if hostOps.created {
		t.Fatalf("HostOpsService.CreateMission was called from appui legacy route: %+v", hostOps.command)
	}
	if runReq.Metadata["aiops.hostops.missionId"] != "hostops:"+result.TurnID {
		t.Fatalf("mission metadata = %q, want hostops:%s", runReq.Metadata["aiops.hostops.missionId"], result.TurnID)
	}
	for _, want := range []string{"v2-host-a", "v2-host-b"} {
		if !strings.Contains(runReq.Metadata["aiops.hostops.mentions"], want) {
			t.Fatalf("mentions metadata = %q, want resolved host %s", runReq.Metadata["aiops.hostops.mentions"], want)
		}
	}
}

func TestChatServiceIgnoresLegacySessionHostForAdvisory(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	session := sessions.GetOrCreate("sess-v2-legacy-host", runtimekernel.SessionTypeHost, runtimekernel.ModeExecute)
	session.HostID = "server-local"
	sessions.Update(session)
	runtime := &chatRuntimeCapture{}
	service := NewChatService(runtime, sessions)

	_, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID: "sess-v2-legacy-host",
		Content:   "pg_auto_failover timeline 为什么会比主库高？",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	runReq := waitForRunTurn(t, runtime)
	if runReq.HostID != "" {
		t.Fatalf("RunTurn hostId = %q, want empty despite legacy session host", runReq.HostID)
	}
	if got := runReq.Metadata["aiops.target.binding"]; got != "none" {
		t.Fatalf("target binding = %q; metadata=%#v", got, runReq.Metadata)
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
		AgentStatus: "online",
		SSHStatus:   "ok",
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
		"aiops.host.metadataAvailable":   "true",
		"aiops.host.id":                  "remote-linux-01",
		"aiops.host.os":                  "linux",
		"aiops.host.arch":                "amd64",
		"aiops.host.transport":           "agent_http",
		"aiops.host.status":              "online",
		"aiops.host.agentStatus":         "online",
		"aiops.host.sshStatus":           "ok",
		"aiops.host.runtimeReachability": "agent_online",
		"aiops.host.address":             "10.10.20.30",
		"aiops.host.sshUser":             "root",
		"aiops.host.sshPort":             "22",
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
	runReq := waitForRunTurn(t, runtime)
	if hostOps.created {
		t.Fatalf("HostOpsService.CreateMission was called from appui legacy route: %+v", hostOps.command)
	}
	if runReq.Metadata["aiops.hostops.missionId"] != "hostops:"+result.TurnID {
		t.Fatalf("mission metadata = %q, want hostops:%s", runReq.Metadata["aiops.hostops.missionId"], result.TurnID)
	}
	for _, want := range []string{"accept-host-a", "accept-host-b", "accept-host-c"} {
		if !strings.Contains(runReq.Metadata["aiops.hostops.mentions"], want) {
			t.Fatalf("mentions metadata = %q, want resolved host %s", runReq.Metadata["aiops.hostops.mentions"], want)
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
	runReq := waitForRunTurn(t, runtime)
	if hostOps.created {
		t.Fatalf("HostOpsService.CreateMission was called from appui legacy route: %+v", hostOps.command)
	}
	for _, want := range []string{"host-a", "host-b", "host-c"} {
		if !strings.Contains(runReq.Metadata["aiops.hostops.mentions"], want) {
			t.Fatalf("mentions metadata = %q, want resolved host %s", runReq.Metadata["aiops.hostops.mentions"], want)
		}
	}
	if got := runReq.Metadata["aiops.hostops.planRequired"]; got != "true" {
		t.Fatalf("planRequired metadata = %q, want true; metadata=%#v", got, runReq.Metadata)
	}
	if runReq.Metadata[metadataCorootExplicitRCA] == "true" || runReq.Metadata[metadataCorootRCADisplayAllowed] == "true" {
		t.Fatalf("metadata = %#v, golden path without @Coroot must not show RCA", runReq.Metadata)
	}
}

func TestChatService_SendMessageHostOpsRouteDoesNotPersistTerminalTurn(t *testing.T) {
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
	runReq := waitForRunTurn(t, runtime)
	session := sessions.Get(result.SessionID)
	if session != nil && session.CurrentTurn != nil && session.CurrentTurn.Lifecycle == runtimekernel.TurnLifecycleCompleted {
		t.Fatalf("appui persisted terminal host-ops turn: %+v", session.CurrentTurn)
	}
	if got := runReq.Metadata["aiops.hostops.routeKind"]; got != "" {
		t.Fatalf("routeKind metadata = %q, want empty for single host-bound route; metadata=%#v", got, runReq.Metadata)
	}
	if got := runReq.Metadata["aiops.route.mode"]; got != string(ChatRouteHostBoundOps) {
		t.Fatalf("route mode = %q, want host_bound_ops; metadata=%#v", got, runReq.Metadata)
	}
	if got := runReq.Metadata[runtimecontract.MetadataIntentKind]; got != string(runtimecontract.IntentKindVerify) {
		t.Fatalf("intent kind = %q, want verify; metadata=%#v", got, runReq.Metadata)
	}
	if !strings.Contains(runReq.Metadata[runtimecontract.MetadataIntentRiskBudget], string(runtimecontract.ActionRiskHostExec)) {
		t.Fatalf("intent riskBudget = %q, want host_exec; metadata=%#v", runReq.Metadata[runtimecontract.MetadataIntentRiskBudget], runReq.Metadata)
	}
	if runReq.Input != "这是@120.77.239.90主机,查看其内存情况" {
		t.Fatalf("RunTurn input = %q, want original hostops request", runReq.Input)
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
