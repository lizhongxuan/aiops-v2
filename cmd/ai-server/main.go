// Package main is the entry point for the AIOps V2 AI Server.
// It initializes all core components and starts HTTP/WebSocket/gRPC servers.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/cloudwego/eino/components/tool"
	runnerservice "runner/server/service"

	"aiops-v2/internal/agentmgr"
	"aiops-v2/internal/agents"
	"aiops-v2/internal/appui"
	"aiops-v2/internal/auth"
	"aiops-v2/internal/commands"
	"aiops-v2/internal/evidence"
	"aiops-v2/internal/featureflag"
	"aiops-v2/internal/hooks"
	agenttools "aiops-v2/internal/integrations/agents"
	agentuitools "aiops-v2/internal/integrations/agentui"
	evidencetools "aiops-v2/internal/integrations/evidence"
	"aiops-v2/internal/integrations/localtools"
	mcpresourcetools "aiops-v2/internal/integrations/mcpresources"
	opsgraphtools "aiops-v2/internal/integrations/opsgraph"
	opsmanualtools "aiops-v2/internal/integrations/opsmanuals"
	"aiops-v2/internal/integrations/toolsearch"
	"aiops-v2/internal/lsp"
	"aiops-v2/internal/mcp"
	mcpruntime "aiops-v2/internal/mcp/runtime"
	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/observability"
	opsgraphstore "aiops-v2/internal/opsgraph"
	"aiops-v2/internal/opsmanual"
	"aiops-v2/internal/outputstyle"
	"aiops-v2/internal/permissions"
	"aiops-v2/internal/plugins"
	"aiops-v2/internal/policyengine"
	"aiops-v2/internal/projection"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/runnerembed"
	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/server"
	"aiops-v2/internal/settings"
	"aiops-v2/internal/skills"
	"aiops-v2/internal/store"
	"aiops-v2/internal/terminal"
	"aiops-v2/internal/tooling"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("ai-server: %v", err)
	}
}

func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// ---------------------------------------------------------------------------
	// Configuration
	// ---------------------------------------------------------------------------
	dataDir := envOrDefault("AIOPS_DATA_DIR", ".data")
	httpAddr := envOrDefault("AIOPS_HTTP_ADDR", ":8080")
	grpcAddr := envOrDefault("AIOPS_GRPC_ADDR", ":18090")
	webDistDir := envOrDefault("AIOPS_WEB_DIST_DIR", "web/dist")
	defaultProvider := envOrDefault("AIOPS_LLM_PROVIDER", "openai")
	oauthAuthorizeURL := envOrDefault("AIOPS_AUTH_OAUTH_AUTHORIZE_URL", "")
	oauthEmail := envOrDefault("AIOPS_AUTH_OAUTH_EMAIL", "")
	oauthPlanType := envOrDefault("AIOPS_AUTH_OAUTH_PLAN_TYPE", "plus")
	runnerStudioUpstreamURL := runnerStudioUpstreamFromEnv(os.Getenv)
	opsManualAutoRetrieval := envBoolDefault("AIOPS_OPS_MANUAL_AUTO_RETRIEVAL", false)
	workflowReferenceGuardMode := workflowReferenceGuardModeFromEnv(os.Getenv)

	// ---------------------------------------------------------------------------
	// 1. Store (persistence layer)
	// ---------------------------------------------------------------------------
	dataStore, err := openConfiguredStore(dataDir, os.Getenv)
	if err != nil {
		return fmt.Errorf("init store: %w", err)
	}
	defer dataStore.Close()
	opsManualRepo, ok := any(dataStore).(opsmanual.ManualRepository)
	if !ok {
		opsManualRepo = opsmanual.NewMemoryStore()
	}
	if summary, err := opsmanual.ImportLegacyJSONFilesIfEmpty(opsManualRepo, dataDir); err != nil {
		return fmt.Errorf("import legacy ops manuals: %w", err)
	} else if summary.ManualsImported > 0 || summary.CandidatesImported > 0 || summary.RunRecordsImported > 0 {
		log.Printf("ai-server: imported legacy ops manuals (manuals=%d candidates=%d run_records=%d)", summary.ManualsImported, summary.CandidatesImported, summary.RunRecordsImported)
	}
	opsManualDomainService := opsmanual.NewService(opsManualRepo, opsmanual.WithResourceDiscovery(opsmanual.NewLocalResourceDiscovery()))

	// ---------------------------------------------------------------------------
	// 2. Registries
	// ---------------------------------------------------------------------------
	toolRegistry := tooling.NewRegistry()
	skillRegistry, err := loadSkillRegistryFromEnv()
	if err != nil {
		return fmt.Errorf("init skill registry: %w", err)
	}
	commandRegistry := buildCommandRegistryFromSkills(skillRegistry)
	flags := featureflag.FromEnv(os.Getenv)
	permissionEngine := permissions.NewEngine(nil)
	pluginHookRegistry := hooks.NewRegistry()
	var runtimeHookRegistry *hooks.Registry
	if flags.HooksV2 {
		runtimeHookRegistry = pluginHookRegistry
	}
	governance := settings.NewGovernance()
	commandRegistry.SetGovernance(governance)

	// ---------------------------------------------------------------------------
	// 3. PromptCompiler
	// ---------------------------------------------------------------------------
	compiler := promptcompiler.NewCompiler()

	// ---------------------------------------------------------------------------
	// 4. PolicyEngine
	// ---------------------------------------------------------------------------
	policyEngine := &policyengine.Engine{
		ModePolicy:       policyengine.NewDefaultModePolicies(),
		CompletionPolicy: &policyengine.DefaultCompletionEvaluator{},
	}

	// ---------------------------------------------------------------------------
	// 5. Projector (projection layer)
	// ---------------------------------------------------------------------------
	projector := projection.NewProjector()

	// ---------------------------------------------------------------------------
	// 6. ModelRouter (LLM provider routing)
	// ---------------------------------------------------------------------------
	providers := buildProviders()
	fallbacks := []modelrouter.FallbackEntry{
		{Primary: "openai", Fallback: "ollama"},
		{Primary: "anthropic", Fallback: "openai"},
	}
	router := modelrouter.NewRouter(defaultProvider, providers, fallbacks)
	router.SetAgentKindConfig(modelrouter.AgentKindPlanner, modelrouter.AgentKindConfig{
		Provider: defaultProvider,
		Model:    "gpt-4o",
	})
	router.SetAgentKindConfig(modelrouter.AgentKindWorker, modelrouter.AgentKindConfig{
		Provider: defaultProvider,
		Model:    "gpt-4o-mini",
	})

	// ---------------------------------------------------------------------------
	// 7. AgentFactory & AgentManager
	// ---------------------------------------------------------------------------
	mcpRegistry := mcp.NewRegistry()
	mcpRegistry.SetGovernance(governance)
	mcpRuntime := mcpruntime.New(mcpruntime.RuntimeOptions{
		Registry:      mcpRegistry,
		ClientFactory: mcpruntime.DefaultClientFactory{},
	})
	toolAssembler := tooling.NewAssembler(toolRegistry, mcpRegistry)
	agentFactory := agentmgr.NewAgentFactory(toolAssembler, compiler, router, policyEngine)
	agentRegistry := agents.NewRegistry()
	agentRegistry.SetGovernance(governance)
	if err := registerBuiltinAgentDefinitions(agentRegistry, agentFactory); err != nil {
		return fmt.Errorf("init agent registry: %w", err)
	}

	agentManager := agentmgr.NewAgentManager(agentFactory, nil, projector)

	// ---------------------------------------------------------------------------
	// 7.5 Long-running recovery state
	// ---------------------------------------------------------------------------
	sessionManager := runtimekernel.NewSessionManager(dataStore)
	taskManager := runtimekernel.NewTaskManager(dataStore)
	budgetController, err := runtimekernel.NewBudgetController(32)
	if err != nil {
		return fmt.Errorf("init workspace budget controller: %w", err)
	}
	recoveryState, err := runtimekernel.RestoreRuntimeState(sessionManager, taskManager, budgetController)
	if err != nil {
		return fmt.Errorf("restore runtime state: %w", err)
	}
	if recoveryState.LatestSession != nil {
		log.Printf("ai-server: restored latest session %s (%s/%s)", recoveryState.LatestSession.ID, recoveryState.LatestSession.Type, recoveryState.LatestSession.Mode)
	}
	if recoveryState.ReconcileSummary != nil && len(recoveryState.ReconcileSummary.ReconciledTasks) > 0 {
		log.Printf("ai-server: reconciled %d workspace tasks after restart", len(recoveryState.ReconcileSummary.ReconciledTasks))
	}

	// ---------------------------------------------------------------------------
	// 8. Extensions (Coroot, Lab, Generator)
	// ---------------------------------------------------------------------------
	pluginHookRegistry.SetGovernance(governance)
	lspRegistry := lsp.NewRegistry()
	outputStyleRegistry := outputstyle.NewRegistry()
	settingsRegistry := settings.NewRegistry()
	pluginRegistrar := &plugins.Registrar{
		Tools:        toolRegistry,
		Commands:     commandRegistry,
		Skills:       skillRegistry,
		Agents:       agentRegistry,
		MCP:          mcpRegistry,
		Hooks:        pluginHookRegistry,
		LSP:          lspRegistry,
		OutputStyles: outputStyleRegistry,
		Settings:     settingsRegistry,
		Governance:   governance,
	}
	pluginSpecs, err := registerPluginsFromEnv(pluginRegistrar)
	if err != nil {
		return fmt.Errorf("init plugins: %w", err)
	}
	builtinSpecs, err := registerBuiltinPlugins(pluginRegistrar, dataStore)
	if err != nil {
		return fmt.Errorf("init builtin plugins: %w", err)
	}
	pluginSpecs = append(pluginSpecs, builtinSpecs...)
	evidenceService := evidence.NewService(evidence.NewInMemoryStore(), time.Now)
	if err := localtools.RegisterBuiltins(toolRegistry, dataStore, localtools.Options{EvidenceService: evidenceService}); err != nil {
		return fmt.Errorf("init local tools: %w", err)
	}
	if err := registerAIOpsToolSurfaceWithCatalog(toolRegistry, mcpRegistry, evidenceService, newOpsInvestigationAgentToolManager(agentManager, agentFactory), toolAssembler); err != nil {
		return fmt.Errorf("init aiops tool surface: %w", err)
	}
	if err := opsmanualtools.RegisterBuiltins(toolRegistry, opsManualDomainService); err != nil {
		return fmt.Errorf("init ops manual tools: %w", err)
	}
	if err := mcpRuntime.Start(ctx); err != nil {
		return fmt.Errorf("start mcp runtime: %w", err)
	}

	// ---------------------------------------------------------------------------
	// 9. EinoKernel (RuntimeKernel)
	// ---------------------------------------------------------------------------
	runtimeObserver, otelProvider := buildRuntimeObserver(ctx, os.Getenv)
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := otelProvider.Shutdown(shutdownCtx); err != nil {
			log.Printf("otel shutdown: %v", err)
		}
	}()
	kernelCfg := runtimekernel.EinoKernelConfig{
		ToolSource:      newRegistryAdapter(toolAssembler, commandRegistry, flags),
		Compiler:        compiler,
		Policy:          policyEngine,
		Permissions:     permissionEngine,
		Hooks:           runtimeHookRegistry,
		Projector:       projector,
		ModelRouter:     router,
		AgentMgr:        newAgentManagerAdapter(agentManager, agentFactory),
		Sessions:        sessionManager,
		SessionRepo:     dataStore,
		SpillRepo:       dataStore,
		EvidenceService: evidenceService,
		Observer:        runtimeObserver,
	}
	kernel := runtimekernel.NewEinoKernel(kernelCfg)

	var oauthProvider auth.OAuthProvider
	if strings.TrimSpace(oauthAuthorizeURL) != "" {
		oauthProvider = &auth.DefaultOAuthProvider{
			AuthorizeURL: oauthAuthorizeURL,
			ExchangeFunc: func(context.Context, auth.OAuthCallbackRequest) (auth.CredentialTruth, error) {
				return auth.CredentialTruth{
					Mode:     auth.ModeChatGPT,
					Email:    strings.TrimSpace(oauthEmail),
					PlanType: strings.TrimSpace(oauthPlanType),
				}, nil
			},
		}
	}
	authManager := auth.NewManager(oauthProvider)
	llmResolver := &storeLLMResolver{repo: dataStore, fallback: authManager}
	router.SetCredentialResolver(llmResolver)
	router.SetProviderConfigResolver(llmResolver)
	terminalManager := terminal.NewManager()

	// ---------------------------------------------------------------------------
	// 10. HTTP/WebSocket Server
	// ---------------------------------------------------------------------------
	webAssets, err := server.NewWebAssetsHandler(webDistDir)
	if err != nil {
		return fmt.Errorf("init web assets: %w", err)
	}
	var runnerRuntime *runnerembed.Runtime
	if strings.TrimSpace(os.Getenv("AIOPS_RUNNER_DISABLED")) != "1" {
		runnerRuntime, err = runnerembed.NewRuntime(ctx, runnerembed.Options{
			DataDir:                    dataDir,
			WorkflowReferenceGuardMode: workflowReferenceGuardMode,
		})
		if err != nil {
			return fmt.Errorf("init runner runtime: %w", err)
		}
		runnerRuntime.SetWorkflowReferenceChecker(opsManualWorkflowReferenceChecker{repo: dataStore})
		runnerRuntime.SetOpsManualRunRecordSink(opsManualRunRecordSink{repo: dataStore})
	}
	if strings.TrimSpace(runnerStudioUpstreamURL) != "" {
		if runnerRuntime != nil {
			log.Printf("ai-server: Runner Studio upstream configuration is deprecated and ignored while embedded Runner is enabled")
		} else {
			log.Printf("ai-server: Runner Studio upstream configuration is deprecated; embedded Runner is disabled")
		}
	}
	httpOptions := []server.HTTPServerOption{
		server.WithWebAssets(webAssets),
		server.WithTerminalManager(terminalManager),
		server.WithRunnerStudioUpstreamURL(runnerStudioUpstreamURL),
		server.WithOpsManualAutoRetrieval(opsManualAutoRetrieval),
	}
	if runnerRuntime != nil {
		httpOptions = append(httpOptions, server.WithRunnerStudioHandler(runnerRuntime.Handler))
	}
	secretDir := strings.TrimSpace(os.Getenv("AIOPS_SECRET_DIR"))
	if secretDir == "" {
		secretDir = filepath.Join(dataDir, "secrets")
	}
	serviceOptions := []appui.ServicesOption{
		appui.WithStore(dataStore),
		appui.WithMCPRegistry(mcpRegistry),
		appui.WithMCPRuntime(mcpRuntime),
		appui.WithPluginSpecs(pluginSpecs),
		appui.WithAuthManager(authManager),
		appui.WithTerminalManager(terminalManager),
		appui.WithOpsManualService(appui.NewOpsManualService(opsManualDomainService)),
		appui.WithLifecycleContext(ctx),
		appui.WithCredentialResolver(appui.NewLocalSecretCredentialResolver(secretDir)),
	}
	if runnerRuntime != nil {
		serviceOptions = append(serviceOptions, appui.WithHostBootstrapRunner(runnerembed.NewBootstrapClient(runnerRuntime)))
	}
	httpServer := server.NewHTTPServer(
		appui.NewServices(
			kernel,
			sessionManager,
			serviceOptions...,
		),
		httpOptions...,
	)
	if subscriber := httpServer.ProjectionSubscriber(); subscriber != nil {
		projector.AddSubscriber(subscriber)
	}
	httpSrv := &http.Server{
		Addr:    httpAddr,
		Handler: httpServer.Handler(),
	}

	// ---------------------------------------------------------------------------
	// 11. gRPC Server
	// ---------------------------------------------------------------------------
	grpcServer := server.NewGRPCServer()

	// ---------------------------------------------------------------------------
	// Start servers
	// ---------------------------------------------------------------------------
	errCh := make(chan error, 2)

	// HTTP server
	go func() {
		log.Printf("ai-server: HTTP listening on %s", httpAddr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("http server: %w", err)
		}
	}()

	// gRPC server (stub listener — real implementation uses net.Listen + grpc.Server)
	go func() {
		ln, err := net.Listen("tcp", grpcAddr)
		if err != nil {
			errCh <- fmt.Errorf("grpc listen: %w", err)
			return
		}
		log.Printf("ai-server: gRPC listening on %s", grpcAddr)
		// Accept connections and hand off to GRPCServer.HandleStream
		for {
			conn, err := ln.Accept()
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					log.Printf("grpc accept: %v", err)
					continue
				}
			}
			// In production, this would be handled by a real gRPC server.
			// The GRPCServer.HandleStream processes bidirectional streams.
			_ = conn
			_ = grpcServer
			conn.Close()
		}
	}()

	// Wait for shutdown signal or error
	select {
	case <-ctx.Done():
		log.Println("ai-server: shutting down...")
	case err := <-errCh:
		return err
	}

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Printf("http shutdown: %v", err)
	}

	if runnerRuntime != nil {
		if err := runnerRuntime.Close(shutdownCtx); err != nil {
			log.Printf("runner runtime shutdown: %v", err)
		}
	}

	if err := dataStore.Flush(); err != nil {
		log.Printf("store flush: %v", err)
	}

	log.Println("ai-server: stopped")
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func envBoolDefault(key string, defaultVal bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch value {
	case "":
		return defaultVal
	case "1", "true", "yes", "on", "enabled":
		return true
	case "0", "false", "no", "off", "disabled":
		return false
	default:
		return defaultVal
	}
}

func workflowReferenceGuardModeFromEnv(getenv func(string) string) runnerservice.WorkflowReferenceGuardMode {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	switch strings.ToLower(strings.TrimSpace(getenv("AIOPS_WORKFLOW_REFERENCE_GUARD_MODE"))) {
	case "warn", "warning":
		return runnerservice.WorkflowReferenceGuardModeWarn
	default:
		return runnerservice.WorkflowReferenceGuardModeEnforce
	}
}

func openConfiguredStore(dataDir string, getenv func(string) string) (store.Store, error) {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	driver := strings.ToLower(strings.TrimSpace(getenv("AIOPS_STORE_DRIVER")))
	switch driver {
	case "", "json", "file":
		return store.NewJSONFileStore(dataDir, 5*time.Second)
	case "postgres", "postgresql":
		dsn := strings.TrimSpace(getenv("AIOPS_POSTGRES_DSN"))
		if dsn == "" {
			dsn = strings.TrimSpace(getenv("DATABASE_URL"))
		}
		if dsn == "" {
			return nil, fmt.Errorf("AIOPS_POSTGRES_DSN is required when AIOPS_STORE_DRIVER=postgres")
		}
		return store.NewPostgresStore(dsn)
	case "mysql":
		dsn := strings.TrimSpace(getenv("AIOPS_MYSQL_DSN"))
		if dsn == "" {
			return nil, fmt.Errorf("AIOPS_MYSQL_DSN is required when AIOPS_STORE_DRIVER=mysql")
		}
		return store.NewMySQLStore(dsn)
	default:
		return nil, fmt.Errorf("unsupported store driver %q", driver)
	}
}

func runnerStudioUpstreamFromEnv(getenv func(string) string) string {
	for _, key := range []string{
		"AIOPS_RUNNER_STUDIO_UPSTREAM_URL",
		"RUNNER_STUDIO_UPSTREAM_URL",
		"AIOPS_RUNNER_API_BASE_URL",
	} {
		if value := strings.TrimSpace(getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func registerAIOpsToolSurface(toolRegistry *tooling.Registry, mcpRegistry *mcp.Registry, evidenceService *evidence.Service, investigationAgents agenttools.Manager) error {
	return registerAIOpsToolSurfaceWithCatalog(toolRegistry, mcpRegistry, evidenceService, investigationAgents, toolRegistry)
}

func registerAIOpsToolSurfaceWithCatalog(toolRegistry *tooling.Registry, mcpRegistry *mcp.Registry, evidenceService *evidence.Service, investigationAgents agenttools.Manager, catalogProvider tooling.ToolCatalogProvider) error {
	if toolRegistry == nil {
		return fmt.Errorf("tool registry is required")
	}
	opsGraphStore, err := opsgraphstore.LoadSeedFile(projectRelativePath("data/opsgraph/erp.seed.yaml"))
	if err != nil {
		opsGraphStore = opsgraphstore.NewStore(nil, nil)
	}
	if err := opsgraphtools.RegisterBuiltins(toolRegistry, opsGraphStore); err != nil {
		return err
	}
	if evidenceService != nil {
		if err := evidencetools.RegisterBuiltins(toolRegistry, evidenceService); err != nil {
			return err
		}
	}
	if mcpRegistry != nil {
		if err := mcpresourcetools.RegisterBuiltins(toolRegistry, mcpRegistry); err != nil {
			return err
		}
	}
	if investigationAgents != nil {
		for _, tool := range []tooling.Tool{
			agenttools.NewSpawnAgentTool(investigationAgents),
			agenttools.NewWaitAgentTool(investigationAgents),
		} {
			if err := toolRegistry.Register(tool); err != nil {
				return err
			}
		}
	}
	if err := agentuitools.RegisterBuiltins(toolRegistry); err != nil {
		return err
	}
	return toolsearch.RegisterBuiltins(toolRegistry, catalogProvider)
}

func projectRelativePath(rel string) string {
	rel = strings.TrimSpace(rel)
	if rel == "" || filepath.IsAbs(rel) {
		return rel
	}
	wd, err := os.Getwd()
	if err != nil {
		return rel
	}
	for dir := wd; dir != ""; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, rel)
		if pathExists(candidate) {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return rel
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

type opsManualWorkflowReferenceRepository interface {
	ListOpsManuals() ([]opsmanual.OpsManual, error)
	ListOpsManualCandidates() ([]opsmanual.ManualCandidate, error)
}

type opsManualRunRecordRepository interface {
	SaveOpsManualRunRecord(record opsmanual.RunRecord) error
}

type opsManualRunRecordSink struct {
	repo opsManualRunRecordRepository
}

func (s opsManualRunRecordSink) RecordRun(_ context.Context, record runnerservice.OpsManualRunRecord) error {
	if s.repo == nil {
		return nil
	}
	metadata := record.Metadata
	workflowID := strings.TrimSpace(record.WorkflowID)
	if workflowID == "" {
		workflowID = strings.TrimSpace(record.WorkflowName)
	}
	if workflowID == "" {
		return nil
	}
	result := opsmanual.WorkflowResult{
		ID:                  strings.TrimSpace(record.RunID),
		ManualID:            strings.TrimSpace(record.ManualID),
		WorkflowID:          workflowID,
		WorkflowVersion:     strings.TrimSpace(record.WorkflowVersion),
		WorkflowDigest:      strings.TrimSpace(record.WorkflowDigest),
		OperationFrame:      metadataStruct[opsmanual.OperationFrame](metadata, "operation_frame"),
		EnvironmentSnapshot: metadataStruct[opsmanual.EnvironmentProfile](metadata, "environment_snapshot", "environment"),
		Parameters:          metadataMap(metadata, "vars", "parameters"),
		ApprovalRef:         metadataString(metadata, "approval_ref", "approval_id"),
		DryRunStatus:        metadataString(metadata, "dry_run_status"),
		ExecutionStatus:     strings.TrimSpace(record.Status),
		ValidationStatus:    metadataString(metadata, "validation_status"),
		RollbackStatus:      metadataString(metadata, "rollback_status"),
		FailureReason:       firstTrimmed(metadataString(metadata, "failure_reason"), record.ErrorCode, record.Message),
		Operator:            strings.TrimSpace(record.TriggeredBy),
		StartedAt:           formatRunnerTime(record.StartedAt),
		CompletedAt:         formatRunnerTime(record.FinishedAt),
	}
	if result.StartedAt == "" {
		result.StartedAt = formatRunnerTime(record.CreatedAt)
	}
	runRecord, err := opsmanual.BuildRunRecordFromWorkflowResult(result)
	if err != nil {
		return err
	}
	return s.repo.SaveOpsManualRunRecord(runRecord)
}

func metadataString(metadata map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := metadata[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			if trimmed := strings.TrimSpace(typed); trimmed != "" {
				return trimmed
			}
		default:
			if value != nil {
				if trimmed := strings.TrimSpace(fmt.Sprint(value)); trimmed != "" {
					return trimmed
				}
			}
		}
	}
	return ""
}

func metadataMap(metadata map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		value, ok := metadata[key]
		if !ok || value == nil {
			continue
		}
		if typed, ok := value.(map[string]any); ok {
			return typed
		}
		var out map[string]any
		if decodeMetadata(value, &out) == nil && len(out) > 0 {
			return out
		}
	}
	return nil
}

func metadataStruct[T any](metadata map[string]any, keys ...string) T {
	var zero T
	for _, key := range keys {
		value, ok := metadata[key]
		if !ok || value == nil {
			continue
		}
		var out T
		if err := decodeMetadata(value, &out); err == nil {
			return out
		}
	}
	return zero
}

func decodeMetadata(value any, out any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

func firstTrimmed(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func formatRunnerTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

type opsManualWorkflowReferenceChecker struct {
	repo opsManualWorkflowReferenceRepository
}

func (c opsManualWorkflowReferenceChecker) ReferencesForWorkflow(_ context.Context, workflowID string) ([]runnerservice.WorkflowReference, error) {
	workflowID = strings.TrimSpace(workflowID)
	if c.repo == nil || workflowID == "" {
		return nil, nil
	}
	var refs []runnerservice.WorkflowReference
	manuals, err := c.repo.ListOpsManuals()
	if err != nil {
		return nil, err
	}
	for _, manual := range manuals {
		if strings.TrimSpace(manual.WorkflowRef.WorkflowID) != workflowID {
			continue
		}
		refs = append(refs, runnerservice.WorkflowReference{
			ManualID: strings.TrimSpace(manual.ID),
			Status:   strings.TrimSpace(string(manual.Status)),
			Title:    strings.TrimSpace(manual.Title),
		})
	}
	candidates, err := c.repo.ListOpsManualCandidates()
	if err != nil {
		return nil, err
	}
	for _, candidate := range candidates {
		manual := candidate.ProposedManual
		if strings.TrimSpace(manual.WorkflowRef.WorkflowID) != workflowID {
			continue
		}
		status := strings.TrimSpace(candidate.ReviewStatus)
		if status == "" {
			status = "candidate"
		}
		manualID := strings.TrimSpace(manual.ID)
		if manualID == "" {
			manualID = strings.TrimSpace(candidate.ID)
		}
		refs = append(refs, runnerservice.WorkflowReference{
			ManualID: manualID,
			Status:   status,
			Title:    strings.TrimSpace(manual.Title),
		})
	}
	return refs, nil
}

func buildRuntimeObserver(ctx context.Context, getenv func(string) string) (runtimekernel.Observer, *observability.Provider) {
	otelCfg := observability.ConfigFromEnv(getenv)
	otelProvider, err := observability.Init(ctx, otelCfg)
	if err != nil {
		log.Printf("ai-server: observability disabled: %v", err)
		otelProvider, _ = observability.Init(ctx, observability.Config{})
	}
	if otelProvider == nil {
		return runtimekernel.NoopObserver{}, &observability.Provider{}
	}
	if otelProvider.Enabled() {
		return observability.NewRuntimeObserver(otelProvider.Tracer(), otelCfg), otelProvider
	}
	return runtimekernel.NoopObserver{}, otelProvider
}

// buildProviders creates ChatModel instances for configured providers.
// The router now lazily constructs auth-backed providers from live credential
// truth, so this stays empty unless we explicitly prewarm models here.
func buildProviders() map[string]modelrouter.ChatModel {
	providers := make(map[string]modelrouter.ChatModel)
	return providers
}

type storeLLMResolver struct {
	repo interface {
		GetLLMConfig() (*store.LLMConfig, error)
	}
	fallback auth.Resolver
}

func (r *storeLLMResolver) Resolve() (auth.CredentialTruth, bool) {
	if r != nil && r.fallback != nil {
		if truth, ok := r.fallback.Resolve(); ok {
			return truth, true
		}
	}
	cfg, ok := r.currentConfig()
	if !ok {
		return auth.CredentialTruth{}, false
	}
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		return auth.CredentialTruth{}, false
	}
	return auth.CredentialTruth{
		Mode:     auth.ModeAPIKey,
		PlanType: strings.TrimSpace(cfg.Provider),
		APIKey:   apiKey,
	}, true
}

func (r *storeLLMResolver) ResolveProviderConfig(modelrouter.AgentKind) (modelrouter.ProviderConfig, bool) {
	cfg, ok := r.currentConfig()
	if !ok {
		return modelrouter.ProviderConfig{}, false
	}
	provider := strings.TrimSpace(cfg.Provider)
	model := strings.TrimSpace(cfg.Model)
	if provider == "" && model == "" {
		return modelrouter.ProviderConfig{}, false
	}
	return modelrouter.ProviderConfig{
		Provider: provider,
		Model:    model,
		BaseURL:  strings.TrimSpace(cfg.BaseURL),
	}, true
}

func (r *storeLLMResolver) currentConfig() (*store.LLMConfig, bool) {
	if r == nil || r.repo == nil {
		return nil, false
	}
	cfg, err := r.repo.GetLLMConfig()
	if err != nil || cfg == nil {
		return nil, false
	}
	return cfg, true
}

// ---------------------------------------------------------------------------
// RegistryAdapter adapts the unified tool assembler to runtimekernel.ToolAssemblySource.
// ---------------------------------------------------------------------------

type registryAdapter struct {
	tools interface {
		AssembleToolsWithOptions(session, mode string, opts tooling.AssembleOptions) []tooling.Tool
	}
	commandRegistry *commands.CommandRegistry
	flags           featureflag.Flags
}

func newRegistryAdapter(tools interface {
	AssembleToolsWithOptions(session, mode string, opts tooling.AssembleOptions) []tooling.Tool
}, commandRegistry *commands.CommandRegistry, flags featureflag.Flags) *registryAdapter {
	return &registryAdapter{
		tools:           tools,
		commandRegistry: commandRegistry,
		flags:           flags.Clone(),
	}
}

func (a *registryAdapter) CompileContext(session runtimekernel.SessionType, mode runtimekernel.Mode) promptcompiler.CompileContext {
	return promptcompiler.CompileContext{
		SessionType:       string(session),
		Mode:              string(mode),
		AssembledTools:    a.assembledTools(string(session), string(mode)),
		SkillPromptAssets: commandPromptAssets(a.skillPromptCommands()),
	}
}

func (a *registryAdapter) AssembleToolPool(session runtimekernel.SessionType, mode runtimekernel.Mode) []tool.BaseTool {
	return tooling.AssembleEinoToolPool(a.assembledTools(string(session), string(mode)))
}

func (a *registryAdapter) CompileContextWithMetadata(session runtimekernel.SessionType, mode runtimekernel.Mode, metadata map[string]string) []promptcompiler.Tool {
	return a.assembledToolsWithMetadata(string(session), string(mode), metadata)
}

func (a *registryAdapter) AssembleToolPoolWithMetadata(session runtimekernel.SessionType, mode runtimekernel.Mode, metadata map[string]string) []tool.BaseTool {
	return tooling.AssembleEinoToolPool(a.assembledToolsWithMetadata(string(session), string(mode), metadata))
}

func (a *registryAdapter) RefreshToken(session runtimekernel.SessionType, mode runtimekernel.Mode) string {
	if a.tools == nil {
		return ""
	}
	if refresher, ok := a.tools.(interface {
		RefreshToken(session, mode string, opts tooling.AssembleOptions) string
	}); ok {
		return refresher.RefreshToken(string(session), string(mode), tooling.AssembleOptions{
			MetadataTransform: a.flags.ApplyToolMetadata,
			Filter: func(_ tooling.Tool, _ tooling.ToolContext, meta tooling.ToolMetadata) bool {
				return a.flags.IsToolVisible(meta)
			},
		})
	}
	return ""
}

func (a *registryAdapter) skillPromptCommands() []commands.PromptCommand {
	if a.commandRegistry != nil {
		return a.commandRegistry.ListSkillLikePromptCommands()
	}
	return nil
}

func (a *registryAdapter) assembledTools(session, mode string) []tooling.Tool {
	return a.assembledToolsWithMetadata(session, mode, nil)
}

func (a *registryAdapter) assembledToolsWithMetadata(session, mode string, metadata map[string]string) []tooling.Tool {
	if a.tools == nil {
		return nil
	}
	opts := tooling.ApplyTurnMetadataToAssembleOptions(tooling.AssembleOptions{
		MetadataTransform: a.flags.ApplyToolMetadata,
		Filter: func(_ tooling.Tool, _ tooling.ToolContext, meta tooling.ToolMetadata) bool {
			return a.flags.IsToolVisible(meta)
		},
	}, metadata)
	return a.tools.AssembleToolsWithOptions(session, mode, opts)
}

// ---------------------------------------------------------------------------
// AgentManagerAdapter adapts *agentmgr.AgentManager to runtimekernel.AgentManagerSource.
// ---------------------------------------------------------------------------

type agentManagerAdapter struct {
	manager *agentmgr.AgentManager
	factory *agentmgr.AgentFactory
}

func newAgentManagerAdapter(manager *agentmgr.AgentManager, factory *agentmgr.AgentFactory) *agentManagerAdapter {
	return &agentManagerAdapter{manager: manager, factory: factory}
}

func (a *agentManagerAdapter) CreateWorkspaceAgent(ctx context.Context, missionID string) error {
	_, err := a.factory.CreateWorkspaceAgent(ctx, missionID)
	return err
}

func (a *agentManagerAdapter) SpawnAndRunPlanner(ctx context.Context, missionID, sessionID, task string) (string, error) {
	// Spawn a planner agent instance.
	instance, err := a.manager.Spawn(ctx, agentmgr.SpawnRequest{
		ID:        fmt.Sprintf("planner-%s-%d", missionID, time.Now().UnixNano()),
		Kind:      agentmgr.AgentKindPlanner,
		MissionID: missionID,
		SessionID: sessionID,
		Task:      task,
	})
	if err != nil {
		return "", err
	}

	// Create agent config via factory.
	cfg, err := a.factory.CreateWorkspaceAgent(ctx, missionID)
	if err != nil {
		return "", err
	}

	// Run the planner agent.
	result, err := a.manager.RunAgent(ctx, instance.ID, &cfg.Planner)
	if err != nil {
		return "", err
	}
	return result.Output, nil
}

func (a *agentManagerAdapter) CollectResults(missionID string) []runtimekernel.AgentResult {
	results := a.manager.CollectResults(missionID)
	out := make([]runtimekernel.AgentResult, len(results))
	for i, r := range results {
		out[i] = runtimekernel.AgentResult{
			AgentID:    r.AgentID,
			HostID:     r.HostID,
			Status:     string(r.Status),
			Output:     r.Output,
			Error:      r.Error,
			DurationMs: r.Duration.Milliseconds(),
		}
	}
	return out
}

type opsInvestigationAgentToolManager struct {
	manager *agentmgr.AgentManager
	factory *agentmgr.AgentFactory
}

func newOpsInvestigationAgentToolManager(manager *agentmgr.AgentManager, factory *agentmgr.AgentFactory) *opsInvestigationAgentToolManager {
	if manager == nil || factory == nil {
		return nil
	}
	return &opsInvestigationAgentToolManager{manager: manager, factory: factory}
}

func (m *opsInvestigationAgentToolManager) SpawnInvestigationAgent(ctx context.Context, req agenttools.SpawnRequest) (agenttools.SpawnResult, error) {
	if m == nil || m.manager == nil {
		return agenttools.SpawnResult{}, fmt.Errorf("agent manager is required")
	}
	agentID := fmt.Sprintf("%s-%d", strings.TrimSpace(req.AgentType), time.Now().UnixNano())
	missionID := firstTrimmed(req.IncidentID, req.SessionID, "ops-investigation")
	sessionID := firstTrimmed(req.SessionID, "agent-"+agentID)
	instance, err := m.manager.Spawn(ctx, agentmgr.SpawnRequest{
		ID:        agentID,
		Kind:      agentmgr.AgentKindWorker,
		MissionID: missionID,
		HostID:    strings.TrimSpace(req.HostID),
		SessionID: sessionID,
		Task:      strings.TrimSpace(req.Task),
	})
	if err != nil {
		return agenttools.SpawnResult{}, err
	}

	go m.runInvestigationAgent(agentID, req)
	return agenttools.SpawnResult{AgentID: instance.ID, AgentType: strings.TrimSpace(req.AgentType), Status: string(instance.Status)}, nil
}

func (m *opsInvestigationAgentToolManager) runInvestigationAgent(agentID string, req agenttools.SpawnRequest) {
	if m == nil || m.manager == nil || m.factory == nil {
		return
	}
	hostID := firstTrimmed(req.HostID, "workspace")
	cfg, err := m.factory.CreateWorkerAgent(context.Background(), hostID, strings.TrimSpace(req.Task))
	if err != nil {
		m.manager.MarkAgentFailed(agentID, err)
		return
	}
	if _, err := m.manager.RunAgent(context.Background(), agentID, cfg); err != nil {
		m.manager.MarkAgentFailed(agentID, err)
	}
}

func (m *opsInvestigationAgentToolManager) WaitEvidenceReports(ctx context.Context, agentIDs []string) ([]agentmgr.EvidenceReport, error) {
	reports := make([]agentmgr.EvidenceReport, 0, len(agentIDs))
	for _, id := range agentIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		inst := m.manager.GetInstance(id)
		if inst == nil {
			return nil, fmt.Errorf("agent %q not found", id)
		}
		if !inst.Status.IsTerminal() {
			return nil, fmt.Errorf("agent %q is still %s", id, inst.Status)
		}
		report := agentmgr.EvidenceReport{
			AgentID:    inst.ID,
			Summary:    strings.TrimSpace(inst.Output),
			Confidence: "unknown",
		}
		if inst.Error != "" {
			report.Errors = []string{inst.Error}
		}
		reports = append(reports, report.Normalize())
	}
	return reports, nil
}

func loadSkillRegistryFromEnv() (*skills.Registry, error) {
	registry := skills.NewRegistry()
	dirs := splitPathList(os.Getenv("AIOPS_SKILLS_DIRS"))
	if len(dirs) == 0 {
		return registry, nil
	}

	loader := skills.NewLoader()
	for _, dir := range dirs {
		defs, err := loader.LoadDir(dir)
		if err != nil {
			return nil, fmt.Errorf("load skill dir %q: %w", dir, err)
		}
		registry.RegisterBatch(defs)
	}
	return registry, nil
}

func loadPluginSpecsFromEnv() ([]plugins.Spec, error) {
	dirs := splitPathList(os.Getenv("AIOPS_PLUGIN_DIRS"))
	if len(dirs) == 0 {
		return nil, nil
	}
	return plugins.NewManifestLoader(dirs...).Load()
}

func registerPluginsFromEnv(registrar *plugins.Registrar) ([]plugins.Spec, error) {
	if registrar == nil {
		return nil, nil
	}

	specs, err := loadPluginSpecsFromEnv()
	if err != nil {
		return nil, err
	}
	for _, spec := range specs {
		if err := registrar.Register(spec); err != nil {
			name := strings.TrimSpace(spec.Name)
			if name == "" {
				name = strings.TrimSpace(spec.Manifest.Name)
			}
			if name == "" {
				name = "<unnamed>"
			}
			return nil, fmt.Errorf("register plugin %q: %w", name, err)
		}
	}
	return specs, nil
}

func buildCommandRegistryFromSkills(skillRegistry *skills.Registry) *commands.CommandRegistry {
	if skillRegistry == nil {
		return nil
	}

	commandRegistry := commands.NewRegistry()
	for _, def := range skillRegistry.List() {
		cmd := skills.PromptCommandForDefinition(def, commands.SourceProjectSettings)
		if err := commandRegistry.RegisterPrompt(cmd); err != nil {
			log.Printf("warn: skipping skill command %q: %v", cmd.Name, err)
		}
	}
	return commandRegistry
}

func registerBuiltinAgentDefinitions(agentRegistry *agents.Registry, agentFactory *agentmgr.AgentFactory) error {
	defs := []agents.Definition{
		{
			Kind:          string(agentmgr.AgentKindPlanner),
			Name:          "planner",
			Source:        string(agents.SourceBuiltin),
			Description:   "Coordinates planning and worker execution.",
			MaxIterations: 10,
		},
		{
			Kind:          string(agentmgr.AgentKindWorker),
			Name:          "worker",
			Source:        string(agents.SourceBuiltin),
			Description:   "Executes host and workspace tasks.",
			MaxIterations: 25,
		},
	}
	if err := agentRegistry.RegisterBatch(defs); err != nil {
		return err
	}
	agentFactory.SetDefinitionRegistry(agentRegistry)

	for _, def := range agentRegistry.List() {
		agentDef := agentmgr.FromRegistryDefinition(def)
		if err := agentFactory.RegisterDefinition(&agentDef); err != nil {
			return err
		}
	}
	return nil
}

func splitPathList(raw string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, item := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == rune(os.PathListSeparator)
	}) {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func dedupeStrings(items []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func commandPromptAssets(cmds []commands.PromptCommand) []string {
	if len(cmds) == 0 {
		return nil
	}

	out := make([]string, 0, len(cmds))
	for _, cmd := range cmds {
		if prompt := strings.TrimSpace(cmd.Prompt); prompt != "" {
			out = append(out, prompt)
		}
	}
	return dedupeStrings(out)
}
