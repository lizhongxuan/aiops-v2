// Package main is the entry point for the AIOps V2 AI Server.
// It initializes all core components and starts HTTP/WebSocket/gRPC servers.
package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/cloudwego/eino/components/tool"

	"aiops-v2/internal/agentmgr"
	"aiops-v2/internal/agents"
	"aiops-v2/internal/appui"
	"aiops-v2/internal/auth"
	"aiops-v2/internal/commands"
	"aiops-v2/internal/featureflag"
	"aiops-v2/internal/hooks"
	"aiops-v2/internal/integrations/localtools"
	"aiops-v2/internal/lsp"
	"aiops-v2/internal/mcp"
	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/observability"
	"aiops-v2/internal/outputstyle"
	"aiops-v2/internal/permissions"
	"aiops-v2/internal/plugins"
	"aiops-v2/internal/policyengine"
	"aiops-v2/internal/projection"
	"aiops-v2/internal/promptcompiler"
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
	corootEndpoint := corootEndpointFromEnv(os.Getenv)
	oauthAuthorizeURL := envOrDefault("AIOPS_AUTH_OAUTH_AUTHORIZE_URL", "")
	oauthEmail := envOrDefault("AIOPS_AUTH_OAUTH_EMAIL", "")
	oauthPlanType := envOrDefault("AIOPS_AUTH_OAUTH_PLAN_TYPE", "plus")
	runnerStudioUpstreamURL := runnerStudioUpstreamFromEnv(os.Getenv)

	// ---------------------------------------------------------------------------
	// 1. Store (persistence layer)
	// ---------------------------------------------------------------------------
	dataStore, err := store.NewJSONFileStore(dataDir, 5*time.Second)
	if err != nil {
		return fmt.Errorf("init store: %w", err)
	}
	defer dataStore.Close()

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
	if err := registerPluginsFromEnv(pluginRegistrar); err != nil {
		return fmt.Errorf("init plugins: %w", err)
	}
	if err := localtools.RegisterBuiltins(toolRegistry, dataStore, localtools.Options{}); err != nil {
		return fmt.Errorf("init local tools: %w", err)
	}
	if err := registerBuiltinIntegrations(mcpRegistry, corootEndpoint); err != nil {
		return fmt.Errorf("init builtin integrations: %w", err)
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
		ToolSource:  newRegistryAdapter(toolAssembler, commandRegistry, flags),
		Compiler:    compiler,
		Policy:      policyEngine,
		Permissions: permissionEngine,
		Hooks:       runtimeHookRegistry,
		Projector:   projector,
		ModelRouter: router,
		AgentMgr:    newAgentManagerAdapter(agentManager, agentFactory),
		Sessions:    sessionManager,
		SessionRepo: dataStore,
		SpillRepo:   dataStore,
		Observer:    runtimeObserver,
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
	httpServer := server.NewHTTPServer(
		appui.NewServices(
			kernel,
			sessionManager,
			appui.WithStore(dataStore),
			appui.WithMCPRegistry(mcpRegistry),
			appui.WithAuthManager(authManager),
			appui.WithTerminalManager(terminalManager),
			appui.WithLifecycleContext(ctx),
		),
		server.WithWebAssets(webAssets),
		server.WithTerminalManager(terminalManager),
		server.WithRunnerStudioUpstreamURL(runnerStudioUpstreamURL),
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

func corootEndpointFromEnv(getenv func(string) string) string {
	for _, key := range []string{
		"AIOPS_COROOT_ENDPOINT",
		"AIOPS_COROOT_BASE_URL",
		"COROOT_BASE_URL",
	} {
		if value := strings.TrimSpace(getenv(key)); value != "" {
			return value
		}
	}
	return ""
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
	if a.tools == nil {
		return nil
	}
	return a.tools.AssembleToolsWithOptions(session, mode, tooling.AssembleOptions{
		MetadataTransform: a.flags.ApplyToolMetadata,
		Filter: func(_ tooling.Tool, _ tooling.ToolContext, meta tooling.ToolMetadata) bool {
			return a.flags.IsToolVisible(meta)
		},
	})
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

func registerPluginsFromEnv(registrar *plugins.Registrar) error {
	if registrar == nil {
		return nil
	}

	specs, err := loadPluginSpecsFromEnv()
	if err != nil {
		return err
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
			return fmt.Errorf("register plugin %q: %w", name, err)
		}
	}
	return nil
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
