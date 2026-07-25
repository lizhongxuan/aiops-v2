package appui_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/appui"
	workflowintegration "aiops-v2/internal/integrations/workfloweditor"
	"aiops-v2/internal/mcp"
	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/policyengine"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/tooling"
	"runner/server/service"
	"runner/workflow"
	contract "runner/workflow/edit"
	"runner/workflow/visual"
	"runner/workflowstore"
)

const (
	workflowStoryTenantID       = "tenant-a"
	workflowStoryUserID         = "user-a"
	workflowStorySessionID      = "workflow-story-session"
	workflowStoryWorkflowID     = "workflow-a"
	workflowStorySecretMarker   = "story-sensitive-marker"
	workflowStoryFakeFinalTxnID = "fake-transaction-from-prose"
	workflowStoryPreviewCallID  = "workflow-story-preview-call"
	workflowStoryApplyCallID    = "workflow-story-apply-call"
	workflowStoryGrantID        = "grant-story"
	workflowStoryApprovalID     = "workflow-approval-story"
)

type workflowStoryApprovalMode string

const (
	workflowStoryApprovalActive  workflowStoryApprovalMode = "active"
	workflowStoryApprovalExpired workflowStoryApprovalMode = "expired"
	workflowStoryApprovalRevoked workflowStoryApprovalMode = "revoked"
)

type workflowMCPStoryFixture struct {
	app          *service.WorkflowEditApplication
	approvals    *service.WorkflowApprovalFileStore
	commits      *service.WorkflowCommitStore
	assembler    *tooling.Assembler
	kernel       *runtimekernel.RuntimeKernel
	sessions     *runtimekernel.SessionManager
	chat         appui.ChatService
	model        *workflowStoryModel
	baseContext  context.Context
	snapshot     service.WorkflowEditSnapshot
	approvalMode workflowStoryApprovalMode
}

type workflowStoryBackend struct {
	snapshot service.WorkflowEditSnapshot
	visual   *service.VisualWorkflowService
}

func (b *workflowStoryBackend) ReadWorkflow(_ context.Context, key contract.WorkflowKey) (service.WorkflowEditSnapshot, error) {
	if key != b.snapshot.Key {
		return service.WorkflowEditSnapshot{}, service.ErrWorkflowEditSnapshot
	}
	return b.snapshot, nil
}

func (b *workflowStoryBackend) ListWorkflowActions(ctx context.Context, _ contract.WorkflowKey, filter service.ActionCatalogFilter) ([]service.ActionSpec, error) {
	return b.visual.ListActions(ctx, filter), nil
}

func (b *workflowStoryBackend) ValidateWorkflow(ctx context.Context, _ contract.WorkflowKey, graph visual.Graph) (*service.VisualWorkflowValidationResult, error) {
	return b.visual.Validate(ctx, graph)
}

func (b *workflowStoryBackend) CompileWorkflow(ctx context.Context, _ contract.WorkflowKey, graph visual.Graph) (*service.CompiledVisualWorkflow, error) {
	return b.visual.Compile(ctx, graph)
}

type workflowStoryAccess struct{}

func (workflowStoryAccess) ResolveWorkflowActionAccess(context.Context, contract.WorkflowKey, service.ActionSpec) (service.WorkflowActionAccess, error) {
	return service.WorkflowActionAccess{Allowed: true}, nil
}

type workflowStoryReferences struct{}

func (workflowStoryReferences) ValidateWorkflowReferences(context.Context, contract.WorkflowKey, visual.Graph) ([]service.VisualWorkflowIssue, error) {
	return nil, nil
}

type workflowStoryModel struct {
	mu             sync.Mutex
	previewRequest contract.PreviewPatchRequest
	applyRequest   contract.ApplyPatchRequest
	boundTools     [][]string
}

func (m *workflowStoryModel) Generate(_ context.Context, messages []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	return m.next(messages)
}

func (m *workflowStoryModel) Stream(_ context.Context, messages []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	response, err := m.next(messages)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{response}), nil
}

func (m *workflowStoryModel) BindTools(tools []*schema.ToolInfo) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	names := make([]string, 0, len(tools))
	for _, info := range tools {
		if info != nil {
			names = append(names, info.Name)
		}
	}
	sort.Strings(names)
	m.boundTools = append(m.boundTools, names)
	return nil
}

func (m *workflowStoryModel) next(messages []*schema.Message) (*schema.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var response *schema.Message
	if workflowStoryModelHasToolResult(messages, workflowStoryApplyCallID) {
		response = schema.AssistantMessage("事务 "+workflowStoryFakeFinalTxnID+" 已完成；此句只是模型文本。", nil)
	} else if raw, found := workflowStoryModelToolResult(messages, workflowStoryPreviewCallID); found {
		request, err := workflowStoryApplyRequestFromPreview(raw)
		if err != nil {
			return nil, err
		}
		m.applyRequest = request
		arguments, err := json.Marshal(request)
		if err != nil {
			return nil, err
		}
		response = schema.AssistantMessage("候选预览已生成，提交前需要用户批准。", []schema.ToolCall{{
			ID: workflowStoryApplyCallID, Type: "function",
			Function: schema.FunctionCall{Name: "workflow.apply_patch", Arguments: string(arguments)},
		}})
	} else {
		arguments, err := json.Marshal(m.previewRequest)
		if err != nil {
			return nil, err
		}
		response = schema.AssistantMessage("我先生成当前 Workflow 修改的真实候选预览。", []schema.ToolCall{{
			ID: workflowStoryPreviewCallID, Type: "function",
			Function: schema.FunctionCall{Name: "workflow.preview_patch", Arguments: string(arguments)},
		}})
	}

	raw, err := json.Marshal(response)
	if err != nil {
		return nil, err
	}
	var cloned schema.Message
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return nil, err
	}
	return &cloned, nil
}

func (m *workflowStoryModel) ApplyRequest() (contract.ApplyPatchRequest, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.applyRequest, m.applyRequest.ChangeSetID != ""
}

func workflowStoryModelHasToolResult(messages []*schema.Message, toolCallID string) bool {
	_, found := workflowStoryModelToolResult(messages, toolCallID)
	return found
}

func workflowStoryModelToolResult(messages []*schema.Message, toolCallID string) (json.RawMessage, bool) {
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		if message == nil || message.Role != schema.Tool || message.ToolCallID != toolCallID {
			continue
		}
		return json.RawMessage(message.Content), true
	}
	return nil, false
}

func workflowStoryApplyRequestFromPreview(raw json.RawMessage) (contract.ApplyPatchRequest, error) {
	var modelResult struct {
		SchemaVersion     string          `json:"schemaVersion"`
		StructuredContent json.RawMessage `json:"structuredContent"`
		IsError           bool            `json:"isError"`
	}
	if err := json.Unmarshal(raw, &modelResult); err != nil || modelResult.SchemaVersion != "aiops.model_tool_result.v1" || modelResult.IsError || len(modelResult.StructuredContent) == 0 {
		return contract.ApplyPatchRequest{}, errors.New("workflow story model did not receive a successful typed preview result")
	}
	var result contract.ToolResult[contract.ChangeSetPreview]
	if err := json.Unmarshal(modelResult.StructuredContent, &result); err != nil {
		return contract.ApplyPatchRequest{}, err
	}
	preview := result.StructuredContent
	if result.IsError || preview.Kind != "preview" || preview.ChangeSet.ID == "" || preview.ChangeSet.PreviewHash == "" || preview.ChangeSet.Base.ResourceVersion == "" {
		return contract.ApplyPatchRequest{}, errors.New("workflow story model received an incomplete typed preview")
	}
	return contract.ApplyPatchRequest{
		SchemaVersion: contract.SchemaVersion, WorkflowKey: preview.ChangeSet.Key,
		ChangeSetID: preview.ChangeSet.ID, PreviewHash: preview.ChangeSet.PreviewHash,
		ExpectedRevision: preview.ChangeSet.Base.ResourceVersion,
		IdempotencyKey:   "workflow-story-idempotency", CandidateGrantID: workflowStoryGrantID,
		ApplyApprovalID: workflowStoryApprovalID,
		Reason:          "apply reviewed candidate password=" + workflowStorySecretMarker,
	}, nil
}

type workflowStoryToolSource struct {
	assembler *tooling.Assembler
}

func (s *workflowStoryToolSource) CompileContext(session runtimekernel.SessionType, mode runtimekernel.Mode) promptcompiler.CompileContext {
	return promptcompiler.CompileContext{
		SessionType:    string(session),
		Mode:           string(mode),
		AssembledTools: s.assembler.AssembleToolsWithOptions(string(session), string(mode), tooling.AssembleOptions{}),
	}
}

func (s *workflowStoryToolSource) AssembleToolPool(session runtimekernel.SessionType, mode runtimekernel.Mode) []tool.BaseTool {
	return s.assembler.AssembleToolPoolWithOptions(string(session), string(mode), tooling.AssembleOptions{})
}

func (s *workflowStoryToolSource) CompileContextWithMetadata(session runtimekernel.SessionType, mode runtimekernel.Mode, metadata map[string]string) []promptcompiler.Tool {
	return s.assembler.CompileContextWithMetadata(string(session), string(mode), metadata)
}

func (s *workflowStoryToolSource) AssembleToolPoolWithMetadata(session runtimekernel.SessionType, mode runtimekernel.Mode, metadata map[string]string) []tool.BaseTool {
	return s.assembler.AssembleToolPoolWithMetadata(string(session), string(mode), metadata)
}

func (s *workflowStoryToolSource) AssembleToolsWithOptions(session, mode string, opts tooling.AssembleOptions) []tooling.Tool {
	return s.assembler.AssembleToolsWithOptions(session, mode, opts)
}

func (s *workflowStoryToolSource) RefreshToken(session runtimekernel.SessionType, mode runtimekernel.Mode) string {
	return s.assembler.RefreshToken(string(session), string(mode), tooling.AssembleOptions{})
}

type workflowStoryProviderConfig struct{}

func (workflowStoryProviderConfig) ResolveProviderConfig(modelrouter.AgentKind) (modelrouter.ProviderConfig, bool) {
	return modelrouter.ProviderConfig{Provider: "workflow-story", Model: "workflow-story", MaxContextTokens: runtimekernel.DefaultMaxTokens}, true
}

type workflowStoryEventEmitter struct{}

func (workflowStoryEventEmitter) Emit(runtimekernel.LifecycleEvent) {}

func newWorkflowMCPStoryFixture(t *testing.T, approvalMode workflowStoryApprovalMode) *workflowMCPStoryFixture {
	t.Helper()

	visualService := service.NewVisualWorkflowService(service.VisualWorkflowServiceConfig{})
	step := workflow.Step{ID: "check", Name: "check", Targets: []string{"local"}, Action: "script.shell", Args: map[string]any{"script": "echo check"}}
	graph := visual.Graph{
		Version: visual.GraphVersion,
		Workflow: workflow.Workflow{
			Version: "v1", Name: workflowStoryWorkflowID,
			Inventory: workflow.Inventory{Hosts: map[string]workflow.Host{"local": {Address: "127.0.0.1"}}},
			Steps:     []workflow.Step{step},
		},
		Nodes: []visual.Node{
			{ID: "start", Type: visual.NodeTypeStart},
			{ID: "check", Type: visual.NodeTypeAction, StepID: "check", Step: &step},
			{ID: "end", Type: visual.NodeTypeEnd},
		},
		Edges: []visual.Edge{
			{ID: "start-check", Source: "start", Target: "check", Kind: visual.EdgeKindNext},
			{ID: "check-end", Source: "check", Target: "end", Kind: visual.EdgeKindNext},
		},
	}
	compiled, err := visualService.Compile(context.Background(), graph)
	if err != nil {
		t.Fatal(err)
	}
	graph, err = visualService.ParseYAML(context.Background(), compiled.YAML)
	if err != nil {
		t.Fatal(err)
	}
	hashes, err := service.VisualWorkflowGraphHashes(graph)
	if err != nil {
		t.Fatal(err)
	}
	key := contract.WorkflowKey{TenantID: workflowStoryTenantID, WorkflowID: workflowStoryWorkflowID}
	snapshot := service.WorkflowEditSnapshot{
		Key: key,
		Revision: contract.Revision{
			ResourceVersion: "revision-a",
			SemanticHash:    hashes.SemanticHash,
			LayoutHash:      hashes.LayoutHash,
		},
		Graph: graph,
	}
	backend := &workflowStoryBackend{snapshot: snapshot, visual: visualService}

	root := t.TempDir()
	commits, err := service.NewWorkflowCommitStore(filepath.Join(root, "commits"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := commits.Commit(context.Background(), workflowStorySeedCommit(key, snapshot.Revision, []byte(compiled.YAML))); err != nil {
		t.Fatal(err)
	}
	candidates := service.NewWorkflowCandidateFileStore(filepath.Join(root, "candidates"))
	approvals := service.NewWorkflowApprovalFileStore(filepath.Join(root, "approvals"), commits)
	application, err := service.NewWorkflowEditMutationApplication(backend, workflowStoryAccess{}, workflowStoryReferences{}, candidates, approvals, commits)
	if err != nil {
		t.Fatal(err)
	}

	position := visual.Position{X: 240, Y: 120}
	previewRequest := contract.PreviewPatchRequest{
		SchemaVersion: contract.SchemaVersion,
		WorkflowKey:   key,
		Expected:      snapshot.Revision,
		ValidationBasis: contract.ValidationBasis{
			ResourceVersion: snapshot.Revision.ResourceVersion,
			SemanticHash:    snapshot.Revision.SemanticHash,
			CatalogVersion:  "story-catalog", ActionSchemaDigest: "story-actions",
			ReferenceSnapshotHash: "story-references", EnvironmentBindingVersion: "story-environment",
		},
		Intent: "move the reviewed check step",
		Operations: []contract.WorkflowOperation{contract.NewOperation(&contract.UpdateNodeOperation{
			NodeID: "check", Set: contract.NodePatch{Position: &position},
		})},
	}
	if approvalMode != workflowStoryApprovalActive {
		workflowStorySeedAuthorizationCollision(t, approvals, snapshot, approvalMode)
	}

	resources, err := workflowintegration.NewWorkflowResourcesV2WithKey(application, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	mcpRegistry := mcp.NewRegistry()
	if err := workflowintegration.Register(mcpRegistry, application, resources); err != nil {
		t.Fatal(err)
	}
	assembler := tooling.NewAssembler(tooling.NewRegistry(), mcpRegistry)
	model := &workflowStoryModel{previewRequest: previewRequest}
	router := modelrouter.NewRouter("workflow-story", map[string]modelrouter.ChatModel{"workflow-story": model}, nil)
	router.SetProviderConfigResolver(workflowStoryProviderConfig{})
	sessions := runtimekernel.NewSessionManager()
	kernel := runtimekernel.NewRuntimeKernel(runtimekernel.RuntimeKernelConfig{
		ToolSource:  &workflowStoryToolSource{assembler: assembler},
		Compiler:    promptcompiler.NewCompiler(),
		Policy:      &policyengine.Engine{ModePolicy: policyengine.NewDefaultModePolicies(), CompletionPolicy: &policyengine.DefaultCompletionEvaluator{}},
		Projector:   workflowStoryEventEmitter{},
		ModelRouter: router,
		Sessions:    sessions,
		DebugConfig: func(context.Context) runtimekernel.RuntimeDebugConfig { return runtimekernel.RuntimeDebugConfig{} },
	})
	baseContext := tooling.ContextWithToolExecution(context.Background(), tooling.ToolExecutionContext{
		TenantID: workflowStoryTenantID, UserID: workflowStoryUserID, SessionID: workflowStorySessionID,
		Metadata: map[string]string{"workflowId": workflowStoryWorkflowID},
	})

	return &workflowMCPStoryFixture{
		app: application, approvals: approvals, commits: commits,
		assembler: assembler, kernel: kernel, sessions: sessions,
		chat: appui.NewChatServiceWithContext(baseContext, kernel, sessions), model: model,
		baseContext: baseContext, snapshot: snapshot, approvalMode: approvalMode,
	}
}

func workflowStorySeedAuthorizationCollision(t *testing.T, approvals *service.WorkflowApprovalFileStore, snapshot service.WorkflowEditSnapshot, approvalMode workflowStoryApprovalMode) {
	t.Helper()
	now := time.Now().UTC()
	issuedAt, expiresAt := now.Add(-time.Minute), now.Add(time.Hour)
	if approvalMode == workflowStoryApprovalExpired {
		issuedAt, expiresAt = now.Add(-2*time.Hour), now.Add(-time.Hour)
	}
	grant := service.WorkflowEditGrantRecord{
		Grant: contract.WorkflowEditGrant{
			ID: workflowStoryGrantID, Key: snapshot.Key, ActorID: workflowStoryUserID,
			PlanHash: "sha256:" + strings.Repeat("a", 64), BaseSemanticHash: snapshot.Revision.SemanticHash,
			AllowedOps: []contract.OperationKind{contract.OperationUpdateNode}, MaxOperations: 1, ExpiresAt: expiresAt,
		},
		CanvasSessionID: workflowStorySessionID,
		RiskScope:       service.WorkflowGrantRiskScope{MaxRisk: service.WorkflowRiskLow},
		State:           service.WorkflowApprovalActive,
		IssuedAt:        issuedAt,
	}
	if err := approvals.IssueGrant(context.Background(), grant); err != nil {
		t.Fatal(err)
	}
	if approvalMode == workflowStoryApprovalRevoked {
		if err := approvals.RevokeGrant(context.Background(), snapshot.Key, workflowStoryGrantID, now); err != nil {
			t.Fatal(err)
		}
	}
}

func workflowStorySeedCommit(key contract.WorkflowKey, revision contract.Revision, yaml []byte) service.WorkflowCommitInput {
	basis := contract.ValidationBasis{
		ResourceVersion: revision.ResourceVersion, SemanticHash: revision.SemanticHash,
		CatalogVersion: "seed-catalog", ActionSchemaDigest: "seed-actions",
		ReferenceSnapshotHash: "seed-references", EnvironmentBindingVersion: "seed-environment",
	}
	receipt := contract.WorkflowMutationReceipt{
		SchemaVersion: contract.SchemaVersion, Kind: "receipt", TransactionID: "transaction-seed",
		ChangeSetID: "change-set-seed", Key: key, Status: "committed",
		RevisionBefore: contract.Revision{}, RevisionAfter: &revision,
		Validation: contract.ValidationResult{Kind: "validation", Basis: basis, Valid: true, Diagnostics: []contract.WorkflowDiagnostic{}},
		AuditID:    "audit-seed",
	}
	envelope := func(kind string) workflowstore.TypedEnvelope {
		return workflowstore.TypedEnvelope{Kind: kind, SchemaVersion: contract.SchemaVersion, Payload: json.RawMessage(`{"ok":true}`)}
	}
	return service.WorkflowCommitInput{
		Key: key, ExpectedResourceVersion: "", IdempotencyKey: "seed-idempotency", RequestDigest: "seed-request-digest",
		GraphPayload: append([]byte(nil), yaml...), Meta: envelope("meta"), History: envelope("history"),
		ImportResult: envelope("import"), RollbackResult: envelope("rollback"), Receipt: receipt,
		ValidationBasis: basis, Checkpoint: envelope("checkpoint"), Inverse: envelope("inverse"),
		ApprovalConsume: envelope("approval-consume"), Audit: envelope("audit"),
		Events: []workflowstore.TypedEnvelope{envelope("event")}, Outbox: []workflowstore.TypedEnvelope{envelope("outbox")},
	}
}

func workflowStoryChatCommand(fixture *workflowMCPStoryFixture) appui.ChatCommand {
	return appui.ChatCommand{
		SessionID: workflowStorySessionID,
		Content:   "请应用当前 Workflow 已审查的候选。页面提交前状态日志：workflow resourceVersion=revision-a，candidate validated。",
		Metadata: map[string]string{
			"source": "workflow_ai_chat", "aiops.workflowAgent.enabled": "true",
			"workflowId": workflowStoryWorkflowID,
		},
		WorkflowContext: &appui.WorkflowTurnContextRequest{
			WorkflowID: workflowStoryWorkflowID, Revision: fixture.snapshot.Revision,
			Environment: appui.WorkflowTurnEnvironment{BindingVersion: "story-environment"},
		},
	}
}

func waitForWorkflowStorySnapshot(t *testing.T, sessions *runtimekernel.SessionManager, sessionID string, predicate func(*runtimekernel.TurnSnapshot) bool) *runtimekernel.TurnSnapshot {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		session, err := sessions.GetSnapshot(sessionID)
		if err != nil {
			t.Fatal(err)
		}
		if session != nil && session.CurrentTurn != nil && predicate(session.CurrentTurn) {
			return session.CurrentTurn
		}
		time.Sleep(10 * time.Millisecond)
	}
	session, _ := sessions.GetSnapshot(sessionID)
	if session == nil {
		t.Fatalf("timed out waiting for workflow story session %q", sessionID)
	}
	if session.CurrentTurn == nil {
		t.Fatalf("timed out waiting for workflow story turn in session %q", sessionID)
	}
	turn := session.CurrentTurn
	visible := []string(nil)
	if len(turn.Iterations) > 0 {
		visible = turn.Iterations[len(turn.Iterations)-1].VisibleTools
	}
	t.Fatalf("timed out waiting for workflow story snapshot: lifecycle=%s resume=%s error=%q visible=%v", turn.Lifecycle, turn.ResumeState, turn.Error, visible)
	return nil
}

func workflowStoryToolResult(snapshot *runtimekernel.TurnSnapshot, toolCallID string) (runtimekernel.ToolResult, bool) {
	if snapshot == nil {
		return runtimekernel.ToolResult{}, false
	}
	for _, iteration := range snapshot.Iterations {
		for _, result := range iteration.ToolResults {
			if result.ToolCallID == toolCallID {
				return result, true
			}
		}
	}
	return runtimekernel.ToolResult{}, false
}

func workflowStoryToolNames(tools []tooling.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, candidate := range tools {
		if candidate != nil {
			names = append(names, candidate.Metadata().Name)
		}
	}
	sort.Strings(names)
	return names
}
