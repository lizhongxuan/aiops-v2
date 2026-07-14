package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/agentmgr"
	"aiops-v2/internal/agents"
	"aiops-v2/internal/appui"
	"aiops-v2/internal/commands"
	"aiops-v2/internal/hostops"
	agenttools "aiops-v2/internal/integrations/agents"
	"aiops-v2/internal/integrations/localtools"
	opsmanualtools "aiops-v2/internal/integrations/opsmanuals"
	"aiops-v2/internal/lsp"
	"aiops-v2/internal/mcp"
	"aiops-v2/internal/opsmanual"
	"aiops-v2/internal/outputstyle"
	"aiops-v2/internal/permissions"
	"aiops-v2/internal/plugins"
	"aiops-v2/internal/policyengine"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/runtimekernel"
	aiserver "aiops-v2/internal/server"
	"aiops-v2/internal/settings"
	"aiops-v2/internal/skills"
	"aiops-v2/internal/store"
	"aiops-v2/internal/tooling"
	"runner/scheduler"
	runnerservice "runner/server/service"
)

type fakeLLMConfigRepo struct {
	cfg *store.LLMConfig
}

func (r fakeLLMConfigRepo) GetLLMConfig() (*store.LLMConfig, error) {
	return r.cfg, nil
}

type registryAdapterCorootConfigRepo struct {
	cfg *store.CorootConfig
	err error
}

func (r *registryAdapterCorootConfigRepo) GetCorootConfig() (*store.CorootConfig, error) {
	if r.err != nil {
		return nil, r.err
	}
	if r.cfg == nil {
		return nil, fmt.Errorf("coroot config not found")
	}
	cp := *r.cfg
	return &cp, nil
}

func (r *registryAdapterCorootConfigRepo) SaveCorootConfig(config *store.CorootConfig) error {
	if config == nil {
		return fmt.Errorf("config is nil")
	}
	cp := *config
	r.cfg = &cp
	r.err = nil
	return nil
}

type failingHostCommandRunner struct {
	err error
}

func (r failingHostCommandRunner) RunHostAgentCommand(context.Context, localtools.HostAgentCommandRequest) (localtools.HostAgentCommandResult, error) {
	return localtools.HostAgentCommandResult{}, r.err
}

type fakeSSHCredentialResolver struct {
	credential appui.ResolvedSSHCredential
	err        error
}

func (r fakeSSHCredentialResolver) ResolveSSHCredential(context.Context, string) (appui.ResolvedSSHCredential, error) {
	if r.err != nil {
		return appui.ResolvedSSHCredential{}, r.err
	}
	return r.credential, nil
}

type fakeSSHCommandExecutor struct {
	requests []localtools.HostAgentCommandRequest
	result   localtools.HostAgentCommandResult
	err      error
}

func (e *fakeSSHCommandExecutor) RunSSHCommand(_ context.Context, _ store.HostRecord, _ appui.ResolvedSSHCredential, req localtools.HostAgentCommandRequest) (localtools.HostAgentCommandResult, error) {
	e.requests = append(e.requests, req)
	if e.err != nil {
		return localtools.HostAgentCommandResult{}, e.err
	}
	return e.result, nil
}

type registryAdapterMockTool struct {
	name     string
	meta     tooling.ToolMetadata
	sessions []string
	modes    []string
}

func TestBuildRuntimeObserverReturnsNoop(t *testing.T) {
	observer, provider := buildRuntimeObserver(context.Background())
	defer provider.Shutdown(context.Background())
	if !isNoopRuntimeObserver(observer) {
		t.Fatalf("observer type = %T, want runtimekernel.NoopObserver", observer)
	}
	if provider.Enabled() {
		t.Fatal("provider should be disabled")
	}
}

func TestBuildRuntimeObserverIgnoresOTelEnv(t *testing.T) {
	observer, provider := buildRuntimeObserver(context.Background())
	defer provider.Shutdown(context.Background())
	if !isNoopRuntimeObserver(observer) {
		t.Fatalf("observer type = %T, want runtimekernel.NoopObserver", observer)
	}
	if provider.Enabled() {
		t.Fatal("provider should ignore OTEL env and stay disabled")
	}
}

func TestNewServerAgentRunnerUsesRuntimeKernelRunner(t *testing.T) {
	lockGate := agentmgr.NewToolResourceLockGate(agentmgr.NewResourceLockManager())
	runner := newServerAgentRunner(
		&policyengine.Engine{ModePolicy: policyengine.NewDefaultModePolicies()},
		permissions.NewEngine(nil),
		nil,
		nil,
		runtimekernel.NewSessionManager(nil),
		nil,
		nil,
		nil,
		nil,
		nil,
		lockGate,
		runtimekernel.NoopObserver{},
	)
	if runner == nil {
		t.Fatal("runner is nil")
	}
	if _, ok := runner.(*runtimekernel.AgentConfigRunner); !ok {
		t.Fatalf("runner type = %T, want *runtimekernel.AgentConfigRunner", runner)
	}
	cfg := reflect.ValueOf(runner).Elem().FieldByName("cfg")
	if cfg.FieldByName("ResourceLockGate").IsNil() {
		t.Fatal("ResourceLockGate is nil, want shared runtime resource lock gate")
	}
}

type agentMCPCatalogGovernanceRepoStub struct {
	items []store.AgentMCPCatalogEntry
}

func (r agentMCPCatalogGovernanceRepoStub) GetAgentMCPCatalog() ([]store.AgentMCPCatalogEntry, error) {
	return append([]store.AgentMCPCatalogEntry(nil), r.items...), nil
}

func TestAgentMCPCatalogGovernanceProviderMapsCatalogEntry(t *testing.T) {
	provider := agentMCPCatalogGovernanceProvider{repo: agentMCPCatalogGovernanceRepoStub{items: []store.AgentMCPCatalogEntry{{
		ID:                           "ops",
		Permission:                   "readwrite",
		Risk:                         "high",
		RequiresExplicitUserApproval: true,
	}}}}

	governance := provider.ServerGovernance("ops")
	if governance.ID != "ops" || governance.Permission != "readwrite" || governance.Risk != "high" || !governance.RequiresExplicitUserApproval {
		t.Fatalf("ServerGovernance() = %#v", governance)
	}
}

func TestHostAgentCommandRunnerPrefersHTTPExec(t *testing.T) {
	tokenStore := appui.NewLocalHostAgentTokenStore(t.TempDir())
	tokenRef, err := tokenStore.StoreHostAgentToken(context.Background(), "host-http", "runner-token")
	if err != nil {
		t.Fatalf("StoreHostAgentToken() error = %v", err)
	}
	repo, err := store.NewJSONFileStore(t.TempDir(), time.Hour)
	if err != nil {
		t.Fatalf("NewJSONFileStore() error = %v", err)
	}
	defer repo.Close()

	var gotAuth string
	var gotExecRequest aiserver.HostExecRequest
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/exec" || r.Method != http.MethodPost {
			t.Fatalf("request = %s %s, want POST /exec", r.Method, r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotExecRequest); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(aiserver.HostExecResponse{Status: "success", Stdout: "runner-ok\n", ExitCode: 0})
	}))
	defer testServer.Close()

	if err := repo.SaveHost(&store.HostRecord{
		ID:                  "host-http",
		Name:                "host-http",
		Address:             "10.0.0.11",
		AgentURL:            testServer.URL,
		AgentTokenSecretRef: tokenRef,
		Executable:          true,
		ControlMode:         "managed",
	}); err != nil {
		t.Fatalf("SaveHost() error = %v", err)
	}

	runner := hostAgentCommandRunner{
		repo:          repo,
		tokenResolver: tokenStore,
		httpClient:    testServer.Client(),
		now:           func() time.Time { return time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC) },
	}
	result, err := runner.RunHostAgentCommand(context.Background(), localtools.HostAgentCommandRequest{
		HostID:         "host-http",
		Command:        "printf",
		Args:           []string{"runner-ok"},
		MaxOutputBytes: 4096,
	})
	if err != nil {
		t.Fatalf("RunHostAgentCommand() error = %v", err)
	}
	if gotAuth != "Bearer runner-token" {
		t.Fatalf("Authorization header = %q, want bearer token", gotAuth)
	}
	if gotExecRequest.Command != "printf" || len(gotExecRequest.Args) != 1 || gotExecRequest.Args[0] != "runner-ok" {
		t.Fatalf("exec request = %#v, want printf runner-ok", gotExecRequest)
	}
	if result.Stdout != "runner-ok\n" || result.ExitCode != 0 || result.Source != "host.agent_http_exec" {
		t.Fatalf("result = %#v, want HTTP /exec stdout", result)
	}
}

func TestHostAgentCommandRunnerFallsBackToHTTPRunForOldAgents(t *testing.T) {
	tokenStore := appui.NewLocalHostAgentTokenStore(t.TempDir())
	tokenRef, err := tokenStore.StoreHostAgentToken(context.Background(), "host-http", "runner-token")
	if err != nil {
		t.Fatalf("StoreHostAgentToken() error = %v", err)
	}
	repo, err := store.NewJSONFileStore(t.TempDir(), time.Hour)
	if err != nil {
		t.Fatalf("NewJSONFileStore() error = %v", err)
	}
	defer repo.Close()

	var gotExec bool
	var gotRun bool
	var gotScript string
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/exec":
			gotExec = true
			http.NotFound(w, r)
			return
		case "/run":
			gotRun = true
			var body hostAgentRunRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			gotScript, _ = body.Task.Step.Args["script"].(string)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(hostAgentRunResponse{
				Result: mapHostAgentRunnerResult(body.Task.ID, "success", "runner-ok\n", ""),
				RunID:  body.Task.RunID,
			})
		default:
			t.Fatalf("request = %s %s, want /exec then /run", r.Method, r.URL.Path)
		}
	}))
	defer testServer.Close()

	if err := repo.SaveHost(&store.HostRecord{
		ID:                  "host-http",
		Name:                "host-http",
		Address:             "10.0.0.11",
		AgentURL:            testServer.URL,
		AgentTokenSecretRef: tokenRef,
		Executable:          true,
		ControlMode:         "managed",
	}); err != nil {
		t.Fatalf("SaveHost() error = %v", err)
	}

	runner := hostAgentCommandRunner{
		repo:          repo,
		tokenResolver: tokenStore,
		httpClient:    testServer.Client(),
		now:           func() time.Time { return time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC) },
	}
	result, err := runner.RunHostAgentCommand(context.Background(), localtools.HostAgentCommandRequest{
		HostID:         "host-http",
		Command:        "printf",
		Args:           []string{"runner-ok"},
		MaxOutputBytes: 4096,
	})
	if err != nil {
		t.Fatalf("RunHostAgentCommand() error = %v", err)
	}
	if !gotExec || !gotRun {
		t.Fatalf("gotExec=%v gotRun=%v, want both", gotExec, gotRun)
	}
	if !strings.Contains(gotScript, "'printf' 'runner-ok'") {
		t.Fatalf("script = %q, want quoted printf command", gotScript)
	}
	if result.Stdout != "runner-ok\n" || result.ExitCode != 0 || result.Source != "host.agent_http_run" {
		t.Fatalf("result = %#v, want HTTP /run stdout", result)
	}
}

func TestFallbackHostCommandRunnerUsesSSHForReadOnlyInventoryHost(t *testing.T) {
	repo, err := store.NewJSONFileStore(t.TempDir(), time.Hour)
	if err != nil {
		t.Fatalf("NewJSONFileStore() error = %v", err)
	}
	defer repo.Close()
	if err := repo.SaveHost(&store.HostRecord{
		ID:               "host-ssh",
		Name:             "host-ssh",
		Address:          "10.0.0.12",
		SSHUser:          "root",
		SSHPort:          22,
		SSHCredentialRef: "secret://hosts/host-ssh/ssh-password",
		Transport:        "manual",
		ControlMode:      "inventory",
	}); err != nil {
		t.Fatalf("SaveHost() error = %v", err)
	}
	executor := &fakeSSHCommandExecutor{
		result: localtools.HostAgentCommandResult{Stdout: "Linux host\n", ExitCode: 0, Source: "host.ssh"},
	}
	runner := fallbackHostCommandRunner{
		primary: failingHostCommandRunner{err: fmt.Errorf("host %q does not have an agent URL", "host-ssh")},
		fallback: hostSSHCommandRunner{
			repo:               repo,
			credentialResolver: fakeSSHCredentialResolver{credential: appui.ResolvedSSHCredential{Password: "redacted-test-password"}},
			executor:           executor,
		},
	}

	result, err := runner.RunHostAgentCommand(context.Background(), localtools.HostAgentCommandRequest{
		HostID:  "host-ssh",
		Command: "uname",
		Args:    []string{"-a"},
	})
	if err != nil {
		t.Fatalf("RunHostAgentCommand() error = %v", err)
	}
	if result.Source != "host.ssh" || result.Stdout != "Linux host\n" {
		t.Fatalf("result = %#v, want SSH fallback output", result)
	}
	if len(executor.requests) != 1 || executor.requests[0].Command != "uname" {
		t.Fatalf("executor requests = %#v, want uname request", executor.requests)
	}
}

func TestFallbackHostCommandRunnerDoesNotUseSSHForMutatingCommand(t *testing.T) {
	repo, err := store.NewJSONFileStore(t.TempDir(), time.Hour)
	if err != nil {
		t.Fatalf("NewJSONFileStore() error = %v", err)
	}
	defer repo.Close()
	if err := repo.SaveHost(&store.HostRecord{
		ID:               "host-ssh",
		Name:             "host-ssh",
		Address:          "10.0.0.12",
		SSHUser:          "root",
		SSHCredentialRef: "secret://hosts/host-ssh/ssh-password",
	}); err != nil {
		t.Fatalf("SaveHost() error = %v", err)
	}
	executor := &fakeSSHCommandExecutor{result: localtools.HostAgentCommandResult{Source: "host.ssh"}}
	runner := fallbackHostCommandRunner{
		primary: failingHostCommandRunner{err: fmt.Errorf("host %q does not have an agent URL", "host-ssh")},
		fallback: hostSSHCommandRunner{
			repo:               repo,
			credentialResolver: fakeSSHCredentialResolver{credential: appui.ResolvedSSHCredential{Password: "redacted-test-password"}},
			executor:           executor,
		},
	}

	_, err = runner.RunHostAgentCommand(context.Background(), localtools.HostAgentCommandRequest{
		HostID:  "host-ssh",
		Command: "systemctl",
		Args:    []string{"restart", "postgresql"},
	})
	if err == nil || !strings.Contains(err.Error(), "ssh fallback requires a read-only command") {
		t.Fatalf("RunHostAgentCommand() error = %v, want read-only SSH fallback rejection", err)
	}
	if len(executor.requests) != 0 {
		t.Fatalf("executor requests = %#v, want no SSH execution for mutating command", executor.requests)
	}
}

func mapHostAgentRunnerResult(taskID, status, stdout, stderr string) scheduler.Result {
	return scheduler.Result{
		TaskID: taskID,
		Status: status,
		Output: map[string]any{
			"stdout": stdout,
			"stderr": stderr,
		},
	}
}

func TestRunnerStudioEmbeddedRuntimeDoesNotUseDisableEnvFlag(t *testing.T) {
	legacyFlag := "AIOPS_RUNNER_" + "DISABLED"
	for _, path := range []string{
		"main.go",
		filepath.Join("..", "..", "scripts", "start-ai-chat-trace-dev.sh"),
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", path, err)
		}
		if strings.Contains(string(data), legacyFlag) {
			t.Fatalf("%s still references deprecated runner disable flag; embedded Runner should start by default", path)
		}
	}
}

func TestOpsManualRunRecordSinkPersistsRunnerTerminalRecord(t *testing.T) {
	repo, err := store.NewJSONFileStore(t.TempDir(), time.Hour)
	if err != nil {
		t.Fatalf("NewJSONFileStore() error = %v", err)
	}
	defer repo.Close()
	sink := opsManualRunRecordSink{repo: repo}

	err = sink.RecordRun(context.Background(), runnerservice.OpsManualRunRecord{
		RunID:           "run-redis-1",
		ManualID:        "manual-redis-memory",
		WorkflowID:      "workflow-redis-memory",
		WorkflowVersion: "v3",
		WorkflowDigest:  "sha256:abc",
		Status:          "success",
		TriggeredBy:     "sre",
		Metadata: map[string]any{
			"dry_run_status":    "passed",
			"validation_status": "passed",
			"rollback_status":   "skipped",
			"operation_frame": map[string]any{
				"target":    map[string]any{"type": "redis"},
				"operation": map[string]any{"action": "rca_or_repair"},
			},
			"environment":  map[string]any{"os": "ubuntu", "execution_surface": "ssh"},
			"vars":         map[string]any{"target_instance": "redis-prod-01", "password": "secret"},
			"approval_ref": "approval-1",
		},
		CreatedAt:  time.Date(2026, 5, 14, 9, 0, 0, 0, time.UTC),
		StartedAt:  time.Date(2026, 5, 14, 9, 1, 0, 0, time.UTC),
		FinishedAt: time.Date(2026, 5, 14, 9, 2, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("RecordRun() error = %v", err)
	}

	records, err := repo.ListOpsManualRunRecords("manual-redis-memory", "workflow-redis-memory", 10)
	if err != nil {
		t.Fatalf("ListOpsManualRunRecords() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %#v, want one", records)
	}
	record := records[0]
	if record.ID != "run-redis-1" || record.ExecutionStatus != "success" || record.ValidationStatus != "passed" || record.DryRunStatus != "passed" {
		t.Fatalf("record = %#v, want mapped runner status fields", record)
	}
	if record.OperationFrame.Target.Type != "redis" || record.OperationFrame.Operation.Action != "rca_or_repair" {
		t.Fatalf("operation frame = %#v, want redis rca_or_repair", record.OperationFrame)
	}
	if got := record.RedactedParameters["password"]; got != opsmanual.RedactedValue {
		t.Fatalf("password = %#v, want redacted", got)
	}
}

func TestOpenConfiguredStoreDefaultsToJSONFileStore(t *testing.T) {
	dataDir := t.TempDir()
	got, err := openConfiguredStore(dataDir, func(string) string { return "" })
	if err != nil {
		t.Fatalf("openConfiguredStore() error = %v", err)
	}
	defer got.Close()
	if _, ok := got.(*store.JSONFileStore); !ok {
		t.Fatalf("openConfiguredStore() type = %T, want *store.JSONFileStore", got)
	}
}

func TestOpenConfiguredStoreRequiresPostgresDSN(t *testing.T) {
	_, err := openConfiguredStore(t.TempDir(), func(key string) string {
		if key == "AIOPS_STORE_DRIVER" {
			return "postgres"
		}
		return ""
	})
	if err == nil {
		t.Fatal("openConfiguredStore() succeeded without postgres dsn")
	}
	if !strings.Contains(err.Error(), "AIOPS_POSTGRES_DSN") {
		t.Fatalf("error = %q, want AIOPS_POSTGRES_DSN", err.Error())
	}
}

func TestOpenConfiguredStoreRejectsUnknownDriver(t *testing.T) {
	_, err := openConfiguredStore(t.TempDir(), func(key string) string {
		if key == "AIOPS_STORE_DRIVER" {
			return "oracle"
		}
		return ""
	})
	if err == nil {
		t.Fatal("openConfiguredStore() succeeded with unknown driver")
	}
	if !strings.Contains(err.Error(), "unsupported store driver") {
		t.Fatalf("error = %q, want unsupported store driver", err.Error())
	}
}

func TestStoreLLMResolverIncludesManualContextWindow(t *testing.T) {
	resolver := &storeLLMResolver{
		repo: fakeLLMConfigRepo{cfg: &store.LLMConfig{
			Provider:         "openai",
			Model:            "gpt-5.4",
			BaseURL:          "http://127.0.0.1:8317/v1",
			MaxContextTokens: 9000,
		}},
	}

	cfg, ok := resolver.ResolveProviderConfig("")
	if !ok {
		t.Fatal("ResolveProviderConfig() ok = false, want true")
	}
	if cfg.MaxContextTokens != 10000 {
		t.Fatalf("MaxContextTokens = %d, want min-clamped 10000", cfg.MaxContextTokens)
	}
}

func TestStoreLLMResolverDefaultsManualContextWindow(t *testing.T) {
	resolver := &storeLLMResolver{
		repo: fakeLLMConfigRepo{cfg: &store.LLMConfig{
			Provider: "openai",
			Model:    "gpt-5.4",
		}},
	}

	cfg, ok := resolver.ResolveProviderConfig("")
	if !ok {
		t.Fatal("ResolveProviderConfig() ok = false, want true")
	}
	if cfg.MaxContextTokens != 200000 {
		t.Fatalf("MaxContextTokens = %d, want default 200000", cfg.MaxContextTokens)
	}
}

func TestStoreLLMResolverPassesRequestTimeout(t *testing.T) {
	resolver := &storeLLMResolver{
		repo: fakeLLMConfigRepo{cfg: &store.LLMConfig{
			Provider:         "openai",
			Model:            "gpt-5.4",
			RequestTimeoutMs: 25000,
		}},
	}

	cfg, ok := resolver.ResolveProviderConfig("")
	if !ok {
		t.Fatal("ResolveProviderConfig() ok = false, want true")
	}
	if cfg.RequestTimeoutMs != 25000 {
		t.Fatalf("RequestTimeoutMs = %d, want 25000", cfg.RequestTimeoutMs)
	}
}

func TestStoreLLMResolverPassesProviderSpecificGenerationConfig(t *testing.T) {
	temperature := 1.0
	topP := 0.95
	resolver := &storeLLMResolver{
		repo: fakeLLMConfigRepo{cfg: &store.LLMConfig{
			Provider:         "deepseek",
			Model:            "deepseek-v4-pro",
			BaseURL:          "https://api.deepseek.com",
			MaxContextTokens: 1000000,
			MaxOutputTokens:  20000,
			Temperature:      &temperature,
			TopP:             &topP,
			ReasoningEffort:  "max",
			ThinkingType:     "enabled",
			ToolStream:       true,
		}},
	}

	cfg, ok := resolver.ResolveProviderConfig("")
	if !ok {
		t.Fatal("ResolveProviderConfig() ok = false, want true")
	}
	if cfg.Provider != "deepseek" || cfg.Model != "deepseek-v4-pro" || cfg.BaseURL != "https://api.deepseek.com" {
		t.Fatalf("provider config route = %+v, want saved deepseek route", cfg)
	}
	if cfg.MaxContextTokens != 1000000 || cfg.MaxTokens != 20000 {
		t.Fatalf("provider config context/output = %d/%d, want 1000000/20000", cfg.MaxContextTokens, cfg.MaxTokens)
	}
	if cfg.Temperature != 1 || cfg.TopP != 0.95 {
		t.Fatalf("provider config temperature/topP = %v/%v, want 1/0.95", cfg.Temperature, cfg.TopP)
	}
	if cfg.ReasoningEffort != "max" || cfg.ThinkingType != "enabled" || !cfg.ToolStream {
		t.Fatalf("provider config reasoning/thinking/toolStream = %q/%q/%v, want max/enabled/true", cfg.ReasoningEffort, cfg.ThinkingType, cfg.ToolStream)
	}
}

func TestRegisterAIOpsToolSurfaceExposesOpsToolsAndOmitsRemovedTools(t *testing.T) {
	toolRegistry := tooling.NewRegistry()
	mcpRegistry := mcp.NewRegistry()

	if err := registerAIOpsToolSurface(toolRegistry, mcpRegistry, nil, nil); err != nil {
		t.Fatalf("registerAIOpsToolSurface() error = %v", err)
	}

	tools := toolRegistry.AssembleTools("host", "inspect")
	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.Metadata().Name] = true
	}
	for _, required := range []string{"list_mcp_resources", "read_mcp_resource", "tool_search"} {
		if !names[required] {
			t.Fatalf("assembled tools missing %q; got %v", required, registryAdapterToolNames(tools))
		}
	}
	for _, deferredDefault := range []string{"opsgraph.lookup"} {
		if names[deferredDefault] {
			t.Fatalf("deferred tool %q should not be visible in default tools; got %v", deferredDefault, registryAdapterToolNames(tools))
		}
	}
	deferredTools := toolRegistry.AssembleToolsWithOptions("host", "inspect", tooling.AssembleOptions{EnabledPacks: []string{"mcp_resource", "opsgraph"}})
	deferredNames := map[string]bool{}
	for _, tool := range deferredTools {
		deferredNames[tool.Metadata().Name] = true
	}
	for _, required := range []string{"list_mcp_resources", "read_mcp_resource", "opsgraph.lookup"} {
		if !deferredNames[required] {
			t.Fatalf("deferred tools missing %q; got %v", required, registryAdapterToolNames(deferredTools))
		}
	}
	for _, removedDefault := range []string{
		"changes.recent_deployments",
		"changes.recent_config_changes",
		"k8s.get_workload",
		"k8s.get_events",
		"k8s.get_logs",
		"k8s.rollout_status",
		"k8s.restart_workload",
		"k8s.scale_workload",
		"k8s.rollout_undo",
	} {
		if names[removedDefault] {
			t.Fatalf("mock tool %q should not be visible in production default tools; got %v", removedDefault, registryAdapterToolNames(tools))
		}
	}
	for _, prefix := range []string{"k8s.", "changes.", "runbook.", "fallback.", "erp."} {
		for name := range names {
			if strings.HasPrefix(name, prefix) {
				t.Fatalf("removed tool %q is still visible; tools=%v", name, registryAdapterToolNames(tools))
			}
		}
	}
	if names["update_plan"] {
		t.Fatalf("update_plan should not be visible in default inspect tools; got %v", registryAdapterToolNames(tools))
	}
}

func TestProductionToolPromptRegistryStaysBelowP0Budget(t *testing.T) {
	toolRegistry := tooling.NewRegistry()
	repo := fakeLLMConfigRepo{cfg: &store.LLMConfig{Provider: "openai", Model: "gpt-5.4", BaseURL: "http://127.0.0.1:8317/v1", APIKey: "test"}}
	if err := localtools.RegisterBuiltins(toolRegistry, repo, localtools.Options{WorkingDir: t.TempDir()}); err != nil {
		t.Fatalf("localtools.RegisterBuiltins() error = %v", err)
	}
	if err := registerAIOpsToolSurface(toolRegistry, mcp.NewRegistry(), nil, nil); err != nil {
		t.Fatalf("registerAIOpsToolSurface() error = %v", err)
	}
	manualService := opsmanual.NewService(opsmanual.NewMemoryStore())
	if err := opsmanualtools.RegisterBuiltins(toolRegistry, manualService); err != nil {
		t.Fatalf("opsmanuals.RegisterBuiltins() error = %v", err)
	}

	assembled := toolRegistry.AssembleTools("host", "inspect")
	compiled, err := promptcompiler.NewCompiler().Compile(promptcompiler.CompileContext{
		SessionType:    "host",
		Mode:           "inspect",
		AssembledTools: assembled,
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if got := len(compiled.Tools.Content); got == 0 || got > 10000 {
		t.Fatalf("tool registry char count = %d, want 1..10000\n%s", got, compiled.Tools.Content)
	}
	if got := len(assembled); got == 0 {
		t.Fatal("visible tool count should be recorded")
	}
}

func TestRegisterAIOpsToolSurfaceWiresToolSearchToCatalogProvider(t *testing.T) {
	toolRegistry := tooling.NewRegistry()
	mcpRegistry := mcp.NewRegistry()
	if err := mcpRegistry.OnServerConnected("dynamic-observability", []tooling.Tool{&tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "observability.service_metrics",
			Description: "Read service metrics from dynamic MCP",
			Domain:      "observability",
		},
		ReadOnlyFunc: func(json.RawMessage) bool {
			return true
		},
	}}); err != nil {
		t.Fatalf("OnServerConnected() error = %v", err)
	}
	catalogProvider := tooling.NewAssembler(toolRegistry, mcpRegistry)
	if err := registerAIOpsToolSurfaceWithCatalog(toolRegistry, mcpRegistry, nil, nil, catalogProvider); err != nil {
		t.Fatalf("registerAIOpsToolSurfaceWithCatalog() error = %v", err)
	}
	tool, ok := toolRegistry.Get("tool_search")
	if !ok {
		t.Fatal("tool_search should be registered")
	}

	initialNames := registryAdapterToolNames(catalogProvider.AssembleToolsWithOptions("host", "chat", tooling.AssembleOptions{}))
	if registryAdapterHasToolByName(initialNames, "observability.service_metrics") {
		t.Fatalf("initial tool surface = %v, dynamic MCP tool should be deferred until tool_search select", initialNames)
	}

	input, err := json.Marshal(map[string]any{"query": "dynamic service metrics", "limit": 10})
	if err != nil {
		t.Fatal(err)
	}
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("tool_search Execute() error = %v", err)
	}
	for _, want := range []string{`"ranker":"bm25"`, `"kind":"pack"`, `"observability.service_metrics"`, `"source":"mcp"`, `"requiresSelect":true`} {
		if !strings.Contains(result.Content, want) {
			t.Fatalf("tool_search result missing %s: %s", want, result.Content)
		}
	}

	selectInput, err := json.Marshal(map[string]any{
		"mode":   "select",
		"tools":  []string{"observability.service_metrics"},
		"reason": "need dynamic MCP metrics evidence",
	})
	if err != nil {
		t.Fatal(err)
	}
	selectResult, err := tool.Execute(context.Background(), selectInput)
	if err != nil {
		t.Fatalf("tool_search select Execute() error = %v", err)
	}
	if !strings.Contains(selectResult.Content, `"loadedTools":["observability.service_metrics"]`) {
		t.Fatalf("tool_search select result = %s, want dynamic MCP tool loaded", selectResult.Content)
	}
}

func TestRegisterAIOpsToolSurfaceRegistersHostOpsManagerTools(t *testing.T) {
	registry := tooling.NewRegistry()
	orchestrator := hostops.NewOrchestrator(hostops.NewInMemoryMissionStore(), hostops.NewInMemoryTranscriptStore(), nil)

	if err := registerAIOpsToolSurfaceWithCatalog(registry, nil, nil, nil, registry, orchestrator); err != nil {
		t.Fatalf("registerAIOpsToolSurfaceWithCatalog() error = %v", err)
	}

	for _, name := range []string{
		hostops.ToolSpawnHostAgent,
		hostops.ToolSendHostAgentMessage,
		hostops.ToolWaitHostAgents,
		hostops.ToolStopHostAgent,
	} {
		tool, ok := registry.Get(name)
		if !ok {
			t.Fatalf("registry.Get(%q) missing", name)
		}
		meta := tool.Metadata()
		if meta.Domain != "hostops" {
			t.Fatalf("%s domain = %q, want hostops", name, meta.Domain)
		}
	}

	assembled := registry.AssembleToolsWithOptions("workspace", "execute", tooling.AssembleOptionsForTurnMetadata(map[string]string{
		"enableToolPack": hostops.ToolPackHostOps,
		"profile":        "host_manager",
	}))
	if !registryAdapterHasTool(assembled, hostops.ToolSpawnHostAgent) {
		t.Fatalf("AssembleToolsWithOptions(workspace, execute, hostops pack) missing %s; got %v", hostops.ToolSpawnHostAgent, registryAdapterToolNames(assembled))
	}
}

func TestRegisterAIOpsToolSurfaceIncludesAgentUIArtifactEmitter(t *testing.T) {
	toolRegistry := tooling.NewRegistry()
	mcpRegistry := mcp.NewRegistry()

	if err := registerAIOpsToolSurface(toolRegistry, mcpRegistry, nil, nil); err != nil {
		t.Fatalf("registerAIOpsToolSurface() error = %v", err)
	}

	if tools := toolRegistry.AssembleTools("host", "inspect"); registryAdapterHasTool(tools, "aiops.ui_artifact_emit") {
		t.Fatalf("aiops.ui_artifact_emit should be hidden from default surface; got %v", registryAdapterToolNames(tools))
	}
	tool, ok := toolRegistry.Get("aiops.ui_artifact_emit")
	if !ok {
		t.Fatal("aiops.ui_artifact_emit not registered")
	}
	if tool.Metadata().Layer != tooling.ToolLayerInternal {
		t.Fatalf("aiops.ui_artifact_emit layer = %q, want internal", tool.Metadata().Layer)
	}
	if !tool.IsReadOnly(nil) {
		t.Fatal("aiops.ui_artifact_emit should be read-only")
	}
}

func TestRegisterAIOpsToolSurfaceCanExposeOpsInvestigationAgents(t *testing.T) {
	toolRegistry := tooling.NewRegistry()
	if err := registerAIOpsToolSurface(toolRegistry, mcp.NewRegistry(), nil, fakeOpsInvestigationAgentManager{}); err != nil {
		t.Fatalf("registerAIOpsToolSurface() error = %v", err)
	}

	tools := toolRegistry.AssembleTools("workspace", "inspect")
	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.Metadata().Name] = true
	}
	for _, required := range []string{"spawn_agent", "wait_agent"} {
		if !names[required] {
			t.Fatalf("assembled tools missing %q; got %v", required, registryAdapterToolNames(tools))
		}
	}
}

func TestVisibleAIOpsToolsHaveOutputSchema(t *testing.T) {
	toolRegistry := tooling.NewRegistry()
	if err := registerAIOpsToolSurface(toolRegistry, mcp.NewRegistry(), nil, fakeOpsInvestigationAgentManager{}); err != nil {
		t.Fatalf("registerAIOpsToolSurface() error = %v", err)
	}

	for _, tool := range toolRegistry.AssembleTools("host", "inspect") {
		if len(bytes.TrimSpace(tool.OutputSchema())) == 0 {
			t.Fatalf("tool %q missing OutputSchemaData", tool.Metadata().Name)
		}
	}
}

func isNoopRuntimeObserver(observer runtimekernel.Observer) bool {
	_, ok := observer.(runtimekernel.NoopObserver)
	return ok
}

type fakeOpsInvestigationAgentManager struct{}

func (fakeOpsInvestigationAgentManager) SpawnInvestigationAgent(context.Context, agenttools.SpawnRequest) (agenttools.SpawnResult, error) {
	return agenttools.SpawnResult{AgentID: "agent-1", AgentType: "metrics_investigator", Status: "running"}, nil
}

func (fakeOpsInvestigationAgentManager) WaitEvidenceReports(context.Context, []string) ([]agentmgr.EvidenceReport, error) {
	return []agentmgr.EvidenceReport{{AgentID: "agent-1", Summary: "evidence collected", EvidenceRefs: []string{"ev-1"}, Confidence: "medium"}}, nil
}

func (m *registryAdapterMockTool) Metadata() tooling.ToolMetadata {
	meta := m.meta
	if meta.Name == "" {
		meta.Name = m.name
	}
	if meta.Description == "" {
		meta.Description = m.name
	}
	return meta
}

func (m *registryAdapterMockTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}
func (m *registryAdapterMockTool) OutputSchema() json.RawMessage { return nil }
func (m *registryAdapterMockTool) Description(json.RawMessage, tooling.DescribeContext) string {
	return m.Metadata().Description
}
func (m *registryAdapterMockTool) Prompt(tooling.PromptContext) string {
	return m.Metadata().Description
}
func (m *registryAdapterMockTool) IsEnabled(ctx tooling.ToolContext) bool {
	return matchRegistryAdapterValue(m.sessions, ctx.SessionType) && matchRegistryAdapterValue(m.modes, ctx.Mode)
}
func (m *registryAdapterMockTool) IsReadOnly(json.RawMessage) bool        { return true }
func (m *registryAdapterMockTool) IsDestructive(json.RawMessage) bool     { return false }
func (m *registryAdapterMockTool) IsConcurrencySafe(json.RawMessage) bool { return true }
func (m *registryAdapterMockTool) ValidateInput(context.Context, json.RawMessage) error {
	return nil
}
func (m *registryAdapterMockTool) CheckPermissions(context.Context, json.RawMessage) tooling.PermissionDecision {
	return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
}
func (m *registryAdapterMockTool) Execute(context.Context, json.RawMessage) (tooling.ToolResult, error) {
	return tooling.ToolResult{Content: "ok"}, nil
}

func matchRegistryAdapterValue(expected []string, actual string) bool {
	if len(expected) == 0 {
		return true
	}
	for _, candidate := range expected {
		if candidate == actual {
			return true
		}
	}
	return false
}

func registerRegistryAdapterMockTool(t *testing.T, registry *tooling.Registry, tool *registryAdapterMockTool) {
	t.Helper()
	if err := registry.Register(tool); err != nil {
		t.Fatalf("register %q: %v", tool.Metadata().Name, err)
	}
}

func registryAdapterToolNames(tools []tooling.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Metadata().Name)
	}
	return names
}

func registryAdapterHasTool(tools []tooling.Tool, name string) bool {
	for _, tool := range tools {
		if tool.Metadata().Name == name {
			return true
		}
	}
	return false
}

func registryAdapterHasToolByName(names []string, name string) bool {
	for _, candidate := range names {
		if candidate == name {
			return true
		}
	}
	return false
}

func TestRegistryAdapterSkillPromptAssetsPreferSkillRegistryOverCommandSurface(t *testing.T) {
	registry := tooling.NewRegistry()

	skillRegistry := skills.NewRegistry()
	skillRegistry.Register(skills.Definition{
		Name:   "filesystem",
		Prompt: "filesystem prompt asset",
	})

	commandRegistry := commands.NewRegistry()
	if err := commandRegistry.RegisterPrompt(commands.PromptCommand{
		Name:       "filesystem",
		Prompt:     "command-surface filesystem prompt asset",
		Source:     "repo",
		LoadedFrom: "skills/filesystem/SKILL.md",
	}); err != nil {
		t.Fatalf("register prompt command: %v", err)
	}

	adapter := newRegistryAdapter(registry, commandRegistry)
	ctx := adapter.CompileContext(runtimekernel.SessionType("host"), runtimekernel.Mode("chat"))

	if len(ctx.SkillPromptAssets) != 1 {
		t.Fatalf("SkillPromptAssets len = %d, want 1", len(ctx.SkillPromptAssets))
	}
	if ctx.SkillPromptAssets[0] != "command-surface filesystem prompt asset" {
		t.Fatalf("SkillPromptAssets[0] = %q, want command-surface filesystem prompt asset", ctx.SkillPromptAssets[0])
	}

	cmds := adapter.skillPromptCommands()
	if len(cmds) != 1 {
		t.Fatalf("skillPromptCommands len = %d, want 1", len(cmds))
	}
	if cmds[0].Prompt != "command-surface filesystem prompt asset" {
		t.Fatalf("skillPromptCommands[0].Prompt = %q, want command-surface filesystem prompt asset", cmds[0].Prompt)
	}
	if !strings.HasSuffix(filepath.ToSlash(cmds[0].LoadedFrom), "skills/filesystem/SKILL.md") {
		t.Fatalf("skillPromptCommands[0].LoadedFrom = %q, want suffix %q", cmds[0].LoadedFrom, "skills/filesystem/SKILL.md")
	}
}

func TestRegistryAdapterSkillPromptAssetsPreferSkillRegistry(t *testing.T) {
	adapter := newRegistryAdapter(tooling.NewRegistry(), nil)
	ctx := adapter.CompileContext(runtimekernel.SessionType("host"), runtimekernel.Mode("chat"))

	if len(ctx.SkillPromptAssets) != 0 {
		t.Fatalf("SkillPromptAssets len = %d, want 0", len(ctx.SkillPromptAssets))
	}

	cmds := adapter.skillPromptCommands()
	if len(cmds) != 0 {
		t.Fatalf("skillPromptCommands len = %d, want 0", len(cmds))
	}
}

func TestRegistryAdapterSkillPromptAssetsDoNotFallbackWithoutCommandSurface(t *testing.T) {
	adapter := newRegistryAdapter(tooling.NewRegistry(), nil)
	ctx := adapter.CompileContext(runtimekernel.SessionType("host"), runtimekernel.Mode("chat"))

	if len(ctx.SkillPromptAssets) != 0 {
		t.Fatalf("SkillPromptAssets len = %d, want 0", len(ctx.SkillPromptAssets))
	}

	cmds := adapter.skillPromptCommands()
	if len(cmds) != 0 {
		t.Fatalf("skillPromptCommands len = %d, want 0", len(cmds))
	}
}

func TestRegistryAdapterSkillPromptCommandsUseOnlyCommandSurface(t *testing.T) {
	skillRegistry := skills.NewRegistry()
	skillRegistry.Register(skills.Definition{
		Name:   "filesystem",
		Prompt: "filesystem prompt asset",
	})

	commandRegistry := buildCommandRegistryFromSkills(skillRegistry)
	adapter := newRegistryAdapter(tooling.NewRegistry(), commandRegistry)
	cmds := adapter.skillPromptCommands()

	if len(cmds) != 1 {
		t.Fatalf("skillPromptCommands len = %d, want 1", len(cmds))
	}
	if cmds[0].Name != "filesystem" {
		t.Fatalf("skillPromptCommands[0].Name = %q, want filesystem", cmds[0].Name)
	}
}

func TestRegistryAdapterSkillPromptCommandsProjectSkillRegistrySources(t *testing.T) {
	skillRegistry := skills.NewRegistry()
	skillRegistry.Register(skills.Definition{
		Name:   "plugin-skill",
		Prompt: "plugin prompt asset",
		Source: "/Users/me/.codex/plugins/cache/example/plugin-skill/SKILL.md",
	})
	skillRegistry.Register(skills.Definition{
		Name:   "bundled-skill",
		Prompt: "bundled prompt asset",
		Source: "/Users/me/.codex/skills/.system/bundled-skill/SKILL.md",
	})
	skillRegistry.Register(skills.Definition{
		Name:   "project-skill",
		Prompt: "project prompt asset",
		Source: "/repo/skills/project-skill/SKILL.md",
	})

	commandRegistry := buildCommandRegistryFromSkills(skillRegistry)
	adapter := newRegistryAdapter(tooling.NewRegistry(), commandRegistry)
	cmds := adapter.skillPromptCommands()

	if len(cmds) != 3 {
		t.Fatalf("skillPromptCommands len = %d, want 3", len(cmds))
	}
	if cmds[0].Source != commands.SourcePlugin {
		t.Fatalf("cmds[0].Source = %q, want %q", cmds[0].Source, commands.SourcePlugin)
	}
	if cmds[1].Source != commands.SourceBundled {
		t.Fatalf("cmds[1].Source = %q, want %q", cmds[1].Source, commands.SourceBundled)
	}
	if cmds[2].Source != commands.SourceProjectSettings {
		t.Fatalf("cmds[2].Source = %q, want %q", cmds[2].Source, commands.SourceProjectSettings)
	}
}

func TestLoadSkillRegistryFromEnvInjectsPromptAssets(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "filesystem")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: filesystem
description: Filesystem helper
---
Use filesystem skill prompt asset.
`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	t.Setenv("AIOPS_SKILLS_DIRS", root)
	skillRegistry, err := loadSkillRegistryFromEnv()
	if err != nil {
		t.Fatalf("loadSkillRegistryFromEnv() error = %v", err)
	}

	commandRegistry := buildCommandRegistryFromSkills(skillRegistry)
	adapter := newRegistryAdapter(tooling.NewRegistry(), commandRegistry)
	ctx := adapter.CompileContext(runtimekernel.SessionType("host"), runtimekernel.Mode("chat"))

	if len(ctx.SkillPromptAssets) != 1 {
		t.Fatalf("SkillPromptAssets len = %d, want 1", len(ctx.SkillPromptAssets))
	}
	if ctx.SkillPromptAssets[0] != "Use filesystem skill prompt asset." {
		t.Fatalf("SkillPromptAssets[0] = %q, want %q", ctx.SkillPromptAssets[0], "Use filesystem skill prompt asset.")
	}

	cmds := adapter.skillPromptCommands()
	if len(cmds) != 1 {
		t.Fatalf("skillPromptCommands len = %d, want 1", len(cmds))
	}
	if cmds[0].Name != "filesystem" {
		t.Fatalf("skillPromptCommands[0].Name = %q, want filesystem", cmds[0].Name)
	}
}

func TestRegisterPluginsFromEnvRegistersManifestComponents(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "example-plugin")

	writeMainTestFile(t, filepath.Join(pluginDir, ".codex-plugin", "plugin.json"), `{
  "name": "example-plugin",
  "commandsPath": "commands",
  "agentsPath": "agents",
  "skillsPath": "skills",
  "outputStylesPath": "output-styles",
  "mcpServers": [
    {"id":"plugin-mcp","name":"plugin-mcp","transport":"stdio","command":["plugin-mcp"]}
  ],
  "lspServers": [
    {"id":"plugin-lsp","name":"plugin-lsp","command":["plugin-lsp"],"languages":["go"],"roots":["."]}
  ],
  "settings": [
    {"name":"plugin-settings","values":{"enabled":true}}
  ]
}`)
	writeMainTestFile(t, filepath.Join(pluginDir, "commands", "deploy.json"), `{
  "name":"deploy",
  "description":"deploy command",
  "prompt":"Deploy carefully.",
  "source":"plugin"
}`)
	writeMainTestFile(t, filepath.Join(pluginDir, "agents", "worker.json"), `{
  "kind":"worker",
  "name":"plugin-worker",
  "source":"plugin",
  "description":"worker agent"
}`)
	writeMainTestFile(t, filepath.Join(pluginDir, "skills", "filesystem", "SKILL.md"), `---
name: filesystem
description: Filesystem helper
---

Use filesystem skill.`)
	writeMainTestFile(t, filepath.Join(pluginDir, "output-styles", "concise.json"), `{
  "name":"concise",
  "description":"Concise output",
  "prompt":"Be concise.",
  "source":"plugin"
}`)

	t.Setenv("AIOPS_PLUGIN_DIRS", pluginDir)

	commandRegistry := commands.NewRegistry()
	skillRegistry := skills.NewRegistry()
	agentRegistry := agents.NewRegistry()
	mcpRegistry := mcp.NewRegistry()
	lspRegistry := lsp.NewRegistry()
	outputStyleRegistry := outputstyle.NewRegistry()
	settingsRegistry := settings.NewRegistry()

	registrar := &plugins.Registrar{
		Commands:     commandRegistry,
		Skills:       skillRegistry,
		Agents:       agentRegistry,
		MCP:          mcpRegistry,
		LSP:          lspRegistry,
		OutputStyles: outputStyleRegistry,
		Settings:     settingsRegistry,
	}

	specs, err := registerPluginsFromEnv(registrar)
	if err != nil {
		t.Fatalf("registerPluginsFromEnv() error = %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("registerPluginsFromEnv() specs len = %d, want 1", len(specs))
	}

	if _, ok := commandRegistry.GetPrompt("deploy"); !ok {
		t.Fatal("expected plugin command to be registered")
	}
	if _, ok := skillRegistry.Get("filesystem"); !ok {
		t.Fatal("expected plugin skill to be registered")
	}
	skillCmds := commandRegistry.ListSkillLikePromptCommands()
	if len(skillCmds) < 1 {
		t.Fatal("expected at least one skill-like command")
	}
	var sawFilesystem bool
	for _, cmd := range skillCmds {
		if cmd.Name == "filesystem" {
			sawFilesystem = true
			break
		}
	}
	if !sawFilesystem {
		t.Fatalf("skill-like commands = %#v, want filesystem to be present", skillCmds)
	}
	if _, ok := agentRegistry.Get("plugin-worker"); !ok {
		t.Fatal("expected plugin agent to be registered")
	}
	if _, ok := mcpRegistry.GetServer("plugin-mcp"); !ok {
		t.Fatal("expected plugin MCP server to be registered")
	}
	if _, ok := lspRegistry.GetServer("plugin-lsp"); !ok {
		t.Fatal("expected plugin LSP server to be registered")
	}
	if _, ok := outputStyleRegistry.Get("concise"); !ok {
		t.Fatal("expected plugin output style to be registered")
	}
	if _, ok := settingsRegistry.Get("plugin-settings"); !ok {
		t.Fatal("expected plugin settings to be registered")
	}
}

func TestRegistryAdapterUsesSameMetadataAssemblyForPromptAndRuntimePools(t *testing.T) {
	registry := tooling.NewRegistry()
	registerRegistryAdapterMockTool(t, registry, &registryAdapterMockTool{name: "host_read", sessions: []string{"host"}, modes: []string{"chat"}})
	registerRegistryAdapterMockTool(t, registry, &registryAdapterMockTool{name: "exec_command", sessions: []string{"host"}, modes: []string{"chat"}})
	adapter := newRegistryAdapter(registry, nil)
	metadata := map[string]string{"aiops.tool.execCommandAllowed": "false"}

	tools := adapter.CompileContextWithMetadata(runtimekernel.SessionType("host"), runtimekernel.Mode("chat"), metadata)
	if got := registryAdapterToolNames(tools); fmt.Sprintf("%v", got) != "[host_read]" {
		t.Fatalf("CompileContextWithMetadata tools = %v, want [host_read]", got)
	}

	pool := adapter.AssembleToolPoolWithMetadata(runtimekernel.SessionType("host"), runtimekernel.Mode("chat"), metadata)
	if len(pool) != 1 {
		t.Fatalf("AssembleToolPoolWithMetadata() len = %d, want 1", len(pool))
	}
	info, err := pool[0].Info(context.Background())
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if info.Name != "host_read" {
		t.Fatalf("tool pool Info().Name = %q, want host_read", info.Name)
	}
}

func TestRegistryAdapterKeepsCorootMCPVisibleForAdvisoryToolProfile(t *testing.T) {
	mcpRegistry := mcp.NewRegistry()
	repo := &registryAdapterCorootConfigRepo{cfg: &store.CorootConfig{
		BaseURL: "http://coroot.example",
		Token:   "saved-token",
		Project: "saved-project",
		Timeout: "1s",
	}}
	if _, err := registerBuiltinPlugins(&plugins.Registrar{MCP: mcpRegistry}, repo); err != nil {
		t.Fatalf("registerBuiltinPlugins() error = %v", err)
	}
	adapter := newRegistryAdapter(tooling.NewAssembler(tooling.NewRegistry(), mcpRegistry), nil)

	tools := adapter.CompileContextWithMetadata(runtimekernel.SessionType("workspace"), runtimekernel.Mode("chat"), map[string]string{
		"toolProfile":                       "chat_advisory",
		"aiops.coroot.explicitRCA":          "true",
		"aiops.tool.corootRCAAllowed":       "true",
		"aiops.toolPack.coroot_rca.allowed": "true",
		"enableToolPack":                    "coroot_rca",
	})
	names := registryAdapterToolNames(tools)
	if !registryAdapterHasToolByName(names, "coroot.collect_rca_context") {
		t.Fatalf("CompileContextWithMetadata tools = %v, want coroot.collect_rca_context", names)
	}
}

func TestRegistryAdapterEnablesBroadCorootAnomalyTools(t *testing.T) {
	mcpRegistry := mcp.NewRegistry()
	repo := &registryAdapterCorootConfigRepo{cfg: &store.CorootConfig{
		BaseURL: "http://coroot.example",
		Token:   "saved-token",
		Project: "saved-project",
		Timeout: "1s",
	}}
	if _, err := registerBuiltinPlugins(&plugins.Registrar{MCP: mcpRegistry}, repo); err != nil {
		t.Fatalf("registerBuiltinPlugins() error = %v", err)
	}
	adapter := newRegistryAdapter(tooling.NewAssembler(tooling.NewRegistry(), mcpRegistry), nil)
	catalog := adapter.AssembleToolsWithOptions("workspace", "chat", tooling.AssembleOptions{IncludeDeferredCatalog: true})
	matches := tooling.MatchToolPacksByMetadata(catalog, "@Coroot 查看有哪些异常")
	var enabledPacks []string
	for _, match := range matches {
		enabledPacks = append(enabledPacks, match.Pack)
	}

	tools := adapter.CompileContextWithMetadata(runtimekernel.SessionType("workspace"), runtimekernel.Mode("chat"), map[string]string{
		"toolProfile":                               "chat_advisory",
		"aiops.coroot.explicitRCA":                  "true",
		"aiops.tool.corootRCAAllowed":               "true",
		"aiops.toolPack.coroot_rca.allowed":         "true",
		"aiops.toolPack.coroot_incident.allowed":    "true",
		"aiops.toolPack.mcp_dynamic_coroot.allowed": "true",
		"enableToolPack":                            strings.Join(enabledPacks, ","),
	})
	names := registryAdapterToolNames(tools)
	for _, want := range []string{"coroot.list_services", "coroot.incidents"} {
		if !registryAdapterHasToolByName(names, want) {
			t.Fatalf("CompileContextWithMetadata tools = %v, want %s for broad Coroot anomaly prompt; matches=%v", names, want, matches)
		}
	}
}

func TestRegistryAdapterExposesDeferredCatalogForProgressiveIntent(t *testing.T) {
	registry := tooling.NewRegistry()
	registerRegistryAdapterMockTool(t, registry, &registryAdapterMockTool{name: "read_file", sessions: []string{"host"}, modes: []string{"chat"}})
	registerRegistryAdapterMockTool(t, registry, &registryAdapterMockTool{
		sessions: []string{"host"},
		modes:    []string{"chat"},
		meta: tooling.ToolMetadata{
			Name:           "get_current_model_config",
			Layer:          tooling.ToolLayerDeferred,
			Pack:           "runtime_config",
			DeferByDefault: true,
			RiskLevel:      tooling.ToolRiskLow,
			Triggers:       []string{"current model", "model name"},
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind: "runtime_config",
				ResourceTypes:  []string{"model", "runtime", "configuration"},
				OperationKinds: []string{"read", "inspect"},
				RequiresSelect: true,
			},
		},
	})
	adapter := newRegistryAdapter(registry, nil)

	defaultNames := registryAdapterToolNames(adapter.AssembleToolsWithOptions("host", "chat", tooling.AssembleOptions{}))
	if registryAdapterHasToolByName(defaultNames, "get_current_model_config") {
		t.Fatalf("default AssembleToolsWithOptions tools = %v, should not include deferred runtime config tool", defaultNames)
	}
	catalogNames := registryAdapterToolNames(adapter.AssembleToolsWithOptions("host", "chat", tooling.AssembleOptions{IncludeDeferredCatalog: true}))
	if !registryAdapterHasToolByName(catalogNames, "get_current_model_config") {
		t.Fatalf("deferred catalog tools = %v, want get_current_model_config for intent matching", catalogNames)
	}
}

func TestRegistryAdapterFiltersOpsManualToolsWhenUserOptedOut(t *testing.T) {
	registry := tooling.NewRegistry()
	for _, name := range []string{"search_ops_manuals", "resolve_ops_manual_params", "run_ops_manual_preflight", "host_read"} {
		registerRegistryAdapterMockTool(t, registry, &registryAdapterMockTool{name: name, sessions: []string{"host"}, modes: []string{"chat"}})
	}
	adapter := newRegistryAdapter(registry, nil)
	metadata := map[string]string{
		"opsManualAction":  "skip_ops_manual",
		"opsManualSkipped": "true",
	}

	tools := adapter.CompileContextWithMetadata(runtimekernel.SessionType("host"), runtimekernel.Mode("chat"), metadata)
	if got := registryAdapterToolNames(tools); fmt.Sprintf("%v", got) != "[host_read]" {
		t.Fatalf("CompileContextWithMetadata tools = %v, want [host_read]", got)
	}

	pool := adapter.AssembleToolPoolWithMetadata(runtimekernel.SessionType("host"), runtimekernel.Mode("chat"), metadata)
	if len(pool) != 1 {
		t.Fatalf("AssembleToolPoolWithMetadata() len = %d, want 1", len(pool))
	}
	info, err := pool[0].Info(context.Background())
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if info.Name != "host_read" {
		t.Fatalf("tool pool Info().Name = %q, want host_read", info.Name)
	}
}

func TestRegistryAdapterToolPromptSetMatchesRuntimeToolPool(t *testing.T) {
	registry := tooling.NewRegistry()
	registerRegistryAdapterMockTool(t, registry, &registryAdapterMockTool{name: "read_file", sessions: []string{"host"}, modes: []string{"chat"}})
	registerRegistryAdapterMockTool(t, registry, &registryAdapterMockTool{name: "write_file", sessions: []string{"host"}, modes: []string{"chat"}})

	adapter := newRegistryAdapter(registry, nil)
	compiler := promptcompiler.NewCompiler()

	ctx := adapter.CompileContext(runtimekernel.SessionType("host"), runtimekernel.Mode("chat"))
	compiled, err := compiler.Compile(ctx)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	if len(compiled.Tools.Entries) != 2 {
		t.Fatalf("compiled tool entries len = %d, want 2", len(compiled.Tools.Entries))
	}
	if !strings.Contains(compiled.Tools.Content, "read_file") {
		t.Fatalf("tool prompt content = %q, want read_file entry", compiled.Tools.Content)
	}
	if !strings.Contains(compiled.Tools.Content, "write_file") {
		t.Fatalf("tool prompt content = %q, want write_file entry", compiled.Tools.Content)
	}

	pool := adapter.AssembleToolPool(runtimekernel.SessionType("host"), runtimekernel.Mode("chat"))
	if len(pool) != 2 {
		t.Fatalf("AssembleToolPool() len = %d, want 2", len(pool))
	}
	for i, tool := range pool {
		info, err := tool.Info(context.Background())
		if err != nil {
			t.Fatalf("pool[%d].Info() error = %v", i, err)
		}
		if !strings.Contains(compiled.Tools.Content, info.Name) {
			t.Fatalf("tool prompt content %q should include runtime tool %q", compiled.Tools.Content, info.Name)
		}
	}
}

func TestRegistryAdapterCompileContextDoesNotLeakMCPPromptAssetsForUnselectedTools(t *testing.T) {
	registry := tooling.NewRegistry()
	registerRegistryAdapterMockTool(t, registry, &registryAdapterMockTool{
		name:     "coroot.query",
		sessions: []string{"host"},
		modes:    []string{"inspect"},
		meta: tooling.ToolMetadata{
			IsMCP: true,
			MCPInfo: tooling.MCPInfo{
				ServerID:   "coroot",
				ServerName: "coroot",
				ToolName:   "coroot.query",
			},
		},
	})

	adapter := newRegistryAdapter(registry, nil)
	ctx := adapter.CompileContext(runtimekernel.SessionType("host"), runtimekernel.Mode("inspect"))

	if len(ctx.AssembledTools) != 0 {
		t.Fatalf("CompileContext AssembledTools len = %d, want 0", len(ctx.AssembledTools))
	}
	if len(ctx.MCPPromptAssets) != 0 {
		t.Fatalf("MCPPromptAssets len = %d, want 0", len(ctx.MCPPromptAssets))
	}
}

func TestRegisterBuiltinAgentDefinitionsWorkerUsesToolScopeForMCPTraits(t *testing.T) {
	agentRegistry := agents.NewRegistry()
	agentFactory := agentmgr.NewAgentFactory(tooling.NewRegistry(), nil, nil, nil)

	if err := registerBuiltinAgentDefinitions(agentRegistry, agentFactory); err != nil {
		t.Fatalf("registerBuiltinAgentDefinitions() error = %v", err)
	}

	worker, ok := agentRegistry.Get("worker")
	if !ok {
		t.Fatal("expected builtin worker definition to be registered")
	}

	if len(worker.Tools) != 0 {
		t.Fatalf("worker.Tools = %#v, want empty allowlist", worker.Tools)
	}
}

func TestRegistryAdapterDefaultFlagsMatchUnflaggedRegistryAssembly(t *testing.T) {
	registry := tooling.NewRegistry()
	registerRegistryAdapterMockTool(t, registry, &registryAdapterMockTool{name: "read_file", sessions: []string{"host"}, modes: []string{"chat"}})
	registerRegistryAdapterMockTool(t, registry, &registryAdapterMockTool{name: "exec_command", sessions: []string{"host"}, modes: []string{"chat"}})

	adapter := newRegistryAdapter(registry, nil)
	ctx := adapter.CompileContext(runtimekernel.SessionType("host"), runtimekernel.Mode("chat"))
	wantTools := registry.AssembleToolsWithOptions("host", "chat", tooling.AssembleOptions{})

	if len(ctx.AssembledTools) != len(wantTools) {
		t.Fatalf("CompileContext AssembledTools len = %d, want %d", len(ctx.AssembledTools), len(wantTools))
	}
	for i := range wantTools {
		gotMeta := ctx.AssembledTools[i].Metadata()
		wantMeta := wantTools[i].Metadata()
		if gotMeta.Name != wantMeta.Name {
			t.Fatalf("CompileContext tool[%d].Name = %q, want %q", i, gotMeta.Name, wantMeta.Name)
		}
		if gotMeta.ShouldDefer != wantMeta.ShouldDefer || gotMeta.HasMCPSource() != wantMeta.HasMCPSource() {
			t.Fatalf("CompileContext tool[%d] metadata = %#v, want %#v", i, gotMeta, wantMeta)
		}
	}

	pool := adapter.AssembleToolPool(runtimekernel.SessionType("host"), runtimekernel.Mode("chat"))
	wantPool := tooling.AssembleEinoToolPool(wantTools)
	if len(pool) != len(wantPool) {
		t.Fatalf("AssembleToolPool() len = %d, want %d", len(pool), len(wantPool))
	}
	for i := range wantPool {
		gotInfo, err := pool[i].Info(context.Background())
		if err != nil {
			t.Fatalf("pool[%d].Info() error = %v", i, err)
		}
		wantInfo, err := wantPool[i].Info(context.Background())
		if err != nil {
			t.Fatalf("wantPool[%d].Info() error = %v", i, err)
		}
		if gotInfo.Name != wantInfo.Name {
			t.Fatalf("pool[%d].Info().Name = %q, want %q", i, gotInfo.Name, wantInfo.Name)
		}
	}
}

func writeMainTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", path, err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimLeft(fmt.Sprintf("%s", content), "\n")), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}
