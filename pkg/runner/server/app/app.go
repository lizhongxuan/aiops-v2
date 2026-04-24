package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"go.uber.org/zap"
	"runner/logging"
	"runner/scriptstore"
	"runner/server/api"
	"runner/server/config"
	"runner/server/events"
	"runner/server/metrics"
	"runner/server/queue"
	"runner/server/service"
	"runner/server/store/agentstore"
	"runner/server/store/envstore"
	"runner/server/store/eventstore"
	"runner/server/store/mcpstore"
	"runner/server/store/skillstore"
	"runner/server/ui"
	"runner/state"
)

var (
	Version   = "dev"
	BuildTime = "-"
)

type Options struct {
	ProgramName       string
	DefaultConfigPath string
	ForceUI           bool
}

type readinessChecker struct {
	cfg config.Config
}

func (c readinessChecker) Ready(_ *http.Request) error {
	dirs := []string{
		c.cfg.Stores.WorkflowsDir,
		c.cfg.Stores.ScriptsDir,
		c.cfg.Stores.SkillsDir,
		c.cfg.Stores.EnvironmentsDir,
		c.cfg.Stores.MCPDir,
		eventstore.DeriveRunEventDir(c.cfg.Stores.RunStateFile),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("prepare dir %s: %w", dir, err)
		}
	}

	files := []string{
		c.cfg.Stores.RunStateFile,
		c.cfg.Stores.AgentStateFile,
	}
	for _, file := range files {
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			return fmt.Errorf("prepare file dir %s: %w", filepath.Dir(file), err)
		}
	}

	if c.cfg.UI.Enabled {
		if err := ensureUIAssetsReady(c.cfg.UI.DistDir); err != nil {
			return err
		}
	}

	return nil
}

func Main(opts Options) {
	fs := flag.NewFlagSet(opts.ProgramName, flag.ExitOnError)
	configPath := fs.String("config", opts.DefaultConfigPath, "config file path")
	if err := fs.Parse(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "parse args: %v\n", err)
		os.Exit(1)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}
	if opts.ForceUI {
		cfg.UI.Enabled = true
	}

	if _, err := logging.Init(logging.Config{
		LogLevel:  cfg.Logging.Level,
		LogFormat: cfg.Logging.Format,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "init logger: %v\n", err)
		os.Exit(1)
	}

	readiness := &api.HealthHandler{
		Checker: readinessChecker{cfg: cfg},
	}

	workflowSvc := service.NewWorkflowService(cfg.Stores.WorkflowsDir)
	scriptStore := scriptstore.NewFileStore(cfg.Stores.ScriptsDir)
	scriptSvc := service.NewScriptService(scriptStore)
	agentStore := agentstore.NewFileStore(cfg.Stores.AgentStateFile)
	agentSvc := service.NewAgentService(agentStore, cfg.Agent.OfflineGraceSec)
	skillStore := skillstore.NewFileStore(cfg.Stores.SkillsDir)
	skillSvc := service.NewSkillService(skillStore)
	environmentStore := envstore.NewFileStore(cfg.Stores.EnvironmentsDir)
	environmentSvc := service.NewEnvironmentService(environmentStore)
	mcpStore := mcpstore.NewFileStore(cfg.Stores.MCPDir)
	mcpSvc := service.NewMcpService(mcpStore)
	preprocessor := service.NewPreprocessor(scriptSvc, agentSvc, cfg.Security.AllowedActions)
	runStore := state.NewFileStore(cfg.Stores.RunStateFile)
	runQueue := queue.NewMemoryQueue(cfg.Execution.QueueSize)
	eventHub := events.NewHub()
	collector := metrics.NewCollector()
	runSvc := service.NewRunService(service.RunServiceConfig{
		MaxConcurrentRuns:  cfg.Execution.MaxConcurrentRuns,
		MaxOutputBytes:     cfg.Execution.MaxOutputBytes,
		MetaStore:          service.NewFileRunRecordStore(service.DeriveRunRecordFile(cfg.Stores.RunStateFile)),
		EventStore:         eventstore.NewFileStore(eventstore.DeriveRunEventDir(cfg.Stores.RunStateFile)),
		AgentDispatchToken: cfg.Agent.DispatchToken,
	}, workflowSvc, preprocessor, runStore, runQueue, eventHub, collector)
	defer runSvc.Close()
	dashboardSvc := service.NewDashboardService(runSvc, agentSvc)
	systemSvc := service.NewSystemService(runSvc, agentSvc)

	var uiHandler http.Handler
	if cfg.UI.Enabled {
		embeddedUI, _ := ui.EmbeddedFS()
		handler, err := api.NewUIHandler(cfg.UI.DistDir, cfg.UI.BasePath, embeddedUI, ui.FallbackFS())
		if err != nil {
			logging.L().Error("init ui handler failed", zap.Error(err))
			os.Exit(1)
		}
		uiHandler = handler
	}

	router := api.NewRouter(api.RouterOptions{
		AuthEnabled: cfg.Auth.Enabled,
		AuthToken:   cfg.Auth.Token,
		CORSOrigins: cfg.UI.CORSOrigins,
		UIBasePath:  cfg.UI.BasePath,
		Health:      readiness,
		Workflow:    api.NewWorkflowHandler(workflowSvc),
		Script:      api.NewScriptHandler(scriptSvc),
		Run:         api.NewRunHandler(runSvc),
		Agent:       api.NewAgentHandler(agentSvc),
		Skill:       api.NewSkillHandler(skillSvc),
		Environment: api.NewEnvironmentHandler(environmentSvc),
		MCP:         api.NewMcpHandler(mcpSvc),
		Dashboard:   api.NewDashboardHandler(dashboardSvc),
		System: api.NewSystemHandler(api.SystemInfo{
			Version:     Version,
			BuildTime:   BuildTime,
			DocsURL:     cfg.UI.DocsURL,
			RepoURL:     cfg.UI.RepoURL,
			AuthEnabled: cfg.Auth.Enabled,
		}, systemSvc),
		MetricsHandler: collector.Handler(),
		UI:             uiHandler,
	})

	server := &http.Server{
		Addr:         cfg.Server.Addr,
		Handler:      router,
		ReadTimeout:  cfg.ReadTimeout(),
		WriteTimeout: cfg.WriteTimeout(),
	}

	logging.L().Info("runner server start",
		zap.String("program", opts.ProgramName),
		zap.String("addr", cfg.Server.Addr),
		zap.Bool("auth_enabled", cfg.Auth.Enabled),
		zap.Bool("ui_enabled", cfg.UI.Enabled),
		zap.String("ui_dist_dir", cfg.UI.DistDir),
	)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		err := server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case sig := <-stop:
		logging.L().Info("runner server shutdown signal", zap.String("signal", sig.String()))
		ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout())
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			logging.L().Error("runner server shutdown failed", zap.Error(err))
			os.Exit(1)
		}
		logging.L().Info("runner server stopped")
	case err := <-errCh:
		if err != nil {
			logging.L().Error("runner server failed", zap.Error(err))
			os.Exit(1)
		}
	}

	time.Sleep(50 * time.Millisecond)
}

func ensureUIAssetsReady(distDir string) error {
	if info, err := os.Stat(distDir); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("ui.dist_dir is not a directory: %s", distDir)
		}
		indexPath := filepath.Join(distDir, "index.html")
		if _, err := os.Stat(indexPath); err == nil {
			return nil
		}
	}

	embeddedUI, ok := ui.EmbeddedFS()
	if !ok {
		return fmt.Errorf("prepare ui dist dir %s: %w", distDir, os.ErrNotExist)
	}
	if _, err := fs.ReadFile(embeddedUI, "index.html"); err != nil {
		return fmt.Errorf("prepare embedded ui index: %w", err)
	}
	return nil
}
