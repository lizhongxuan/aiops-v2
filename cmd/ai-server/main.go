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
	"aiops-v2/internal/capability"
	"aiops-v2/internal/extensions/coroot"
	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/policyengine"
	"aiops-v2/internal/projection"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/server"
	"aiops-v2/internal/skills"
	"aiops-v2/internal/store"
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
	defaultProvider := envOrDefault("AIOPS_LLM_PROVIDER", "openai")
	corootEndpoint := envOrDefault("AIOPS_COROOT_ENDPOINT", "")

	// ---------------------------------------------------------------------------
	// 1. Store (persistence layer)
	// ---------------------------------------------------------------------------
	dataStore, err := store.NewJSONFileStore(dataDir, 5*time.Second)
	if err != nil {
		return fmt.Errorf("init store: %w", err)
	}
	defer dataStore.Close()

	// ---------------------------------------------------------------------------
	// 2. Capability Registry
	// ---------------------------------------------------------------------------
	registry := capability.NewRegistry()
	skillRegistry, err := loadSkillRegistryFromEnv()
	if err != nil {
		return fmt.Errorf("init skill registry: %w", err)
	}

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
	agentFactory := agentmgr.NewAgentFactory(registry, compiler, router, policyEngine)
	agentRegistry := agents.NewRegistry()
	if err := registerBuiltinAgentDefinitions(agentRegistry, agentFactory); err != nil {
		return fmt.Errorf("init agent registry: %w", err)
	}

	agentManager := agentmgr.NewAgentManager(agentFactory, nil, projector)

	// ---------------------------------------------------------------------------
	// 8. Extensions (Coroot, Lab, Generator)
	// ---------------------------------------------------------------------------
	extManager := capability.NewExtensionManager(registry)

	if corootEndpoint != "" {
		corootExt := coroot.NewCorootExtension(corootEndpoint)
		if err := extManager.Register(corootExt); err != nil {
			log.Printf("warn: coroot extension registration failed: %v", err)
		}
	}

	// Lab and Generator extensions are registered when their configs are available.
	// They follow the same pattern: extManager.Register(labExt) / extManager.Register(genExt)

	// ---------------------------------------------------------------------------
	// 9. EinoKernel (RuntimeKernel)
	// ---------------------------------------------------------------------------
	kernelCfg := runtimekernel.EinoKernelConfig{
		Registry:    newRegistryAdapter(registry, skillRegistry),
		Compiler:    compiler,
		Policy:      policyEngine,
		Projector:   projector,
		ModelRouter: router,
		AgentMgr:    newAgentManagerAdapter(agentManager, agentFactory),
	}
	kernel := runtimekernel.NewEinoKernel(kernelCfg)

	// ---------------------------------------------------------------------------
	// 10. HTTP/WebSocket Server
	// ---------------------------------------------------------------------------
	httpServer := server.NewHTTPServer(kernel)
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

// buildProviders creates ChatModel instances for configured providers.
// In production, these would be real eino-ext ChatModel implementations.
func buildProviders() map[string]modelrouter.ChatModel {
	providers := make(map[string]modelrouter.ChatModel)
	// Provider instances are created via modelrouter/openai.go, anthropic.go, ollama.go
	// when their API keys are configured. For now, return empty map — the ModelRouter
	// will return ProviderNotFoundError until providers are configured.
	//
	// Example with real providers:
	//   if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
	//       providers["openai"] = modelrouter.NewOpenAIModel(apiKey, "gpt-4o")
	//   }
	return providers
}

// ---------------------------------------------------------------------------
// RegistryAdapter adapts *capability.Registry to runtimekernel.CapabilitySource.
// ---------------------------------------------------------------------------

type registryAdapter struct {
	registry      *capability.Registry
	skillRegistry *skills.Registry
}

func newRegistryAdapter(registry *capability.Registry, skillRegistry *skills.Registry) *registryAdapter {
	return &registryAdapter{registry: registry, skillRegistry: skillRegistry}
}

func (a *registryAdapter) CompileContext(session runtimekernel.SessionType, mode runtimekernel.Mode) promptcompiler.CompileContext {
	return promptcompiler.CompileContext{
		SessionType:       string(session),
		Mode:              string(mode),
		AssembledTools:    a.registry.AssembleTools(string(session), string(mode)),
		SkillPromptAssets: a.skillPromptAssets(string(session), string(mode)),
		MCPPromptAssets:   a.registry.MCPPromptAssets(string(session), string(mode)),
	}
}

func (a *registryAdapter) AssembleToolPool(session runtimekernel.SessionType, mode runtimekernel.Mode) []tool.BaseTool {
	return a.registry.AssembleToolPool(string(session), string(mode))
}

func (a *registryAdapter) skillPromptAssets(session, mode string) []string {
	var assets []string
	if a.skillRegistry != nil {
		assets = append(assets, a.skillRegistry.PromptAssets()...)
	}
	assets = append(assets, a.registry.SkillPromptAssets(session, mode)...)
	return dedupeStrings(assets)
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

func registerBuiltinAgentDefinitions(agentRegistry *agents.Registry, agentFactory *agentmgr.AgentFactory) error {
	defs := []agents.Definition{
		{
			Kind:          string(agentmgr.AgentKindPlanner),
			Name:          "planner",
			Description:   "Coordinates planning and worker execution.",
			MaxIterations: 10,
		},
		{
			Kind:          string(agentmgr.AgentKindWorker),
			Name:          "worker",
			Description:   "Executes host and workspace tasks.",
			MaxIterations: 25,
			CapabilityKinds: []string{
				string(capability.KindTool),
				string(capability.KindMCPTool),
			},
		},
	}
	if err := agentRegistry.RegisterBatch(defs); err != nil {
		return err
	}

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
