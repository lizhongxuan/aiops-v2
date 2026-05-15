package app

import (
	"context"
	"fmt"
	"net/http"

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

type RuntimeOptions struct {
	Config                     config.Config
	OpsManualRunRecordSink     service.OpsManualRunRecordSink
	WorkflowReferenceGuardMode service.WorkflowReferenceGuardMode
}

type Runtime struct {
	Handler     http.Handler
	runSvc      *service.RunService
	workflowSvc *service.WorkflowService
}

func NewRuntime(_ context.Context, opts RuntimeOptions) (*Runtime, error) {
	cfg := opts.Config
	readiness := &api.HealthHandler{
		Checker: readinessChecker{cfg: cfg},
	}

	workflowSvc := service.NewWorkflowService(cfg.Stores.WorkflowsDir)
	if opts.WorkflowReferenceGuardMode != "" {
		workflowSvc.SetWorkflowReferenceGuardMode(opts.WorkflowReferenceGuardMode)
	}
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
		MaxConcurrentRuns:      cfg.Execution.MaxConcurrentRuns,
		MaxOutputBytes:         cfg.Execution.MaxOutputBytes,
		MetaStore:              service.NewFileRunRecordStore(service.DeriveRunRecordFile(cfg.Stores.RunStateFile)),
		EventStore:             eventstore.NewFileStore(eventstore.DeriveRunEventDir(cfg.Stores.RunStateFile)),
		AgentDispatchToken:     cfg.Agent.DispatchToken,
		OpsManualRunRecordSink: opts.OpsManualRunRecordSink,
	}, workflowSvc, preprocessor, runStore, runQueue, eventHub, collector)
	actionCatalog := service.NewActionCatalog()
	visualWorkflowSvc := service.NewVisualWorkflowService(service.VisualWorkflowServiceConfig{
		WorkflowService: workflowSvc,
		RunService:      runSvc,
		Preprocessor:    preprocessor,
		ActionCatalog:   actionCatalog,
	})
	dashboardSvc := service.NewDashboardService(runSvc, agentSvc)
	systemSvc := service.NewSystemService(runSvc, agentSvc)

	var uiHandler http.Handler
	if cfg.UI.Enabled {
		embeddedUI, _ := ui.EmbeddedFS()
		handler, err := api.NewUIHandler(cfg.UI.DistDir, cfg.UI.BasePath, embeddedUI, ui.FallbackFS())
		if err != nil {
			runSvc.Close()
			return nil, fmt.Errorf("init runner ui handler: %w", err)
		}
		uiHandler = handler
	}

	router := api.NewRouter(api.RouterOptions{
		AuthEnabled:    cfg.Auth.Enabled,
		AuthToken:      cfg.Auth.Token,
		CORSOrigins:    cfg.UI.CORSOrigins,
		UIBasePath:     cfg.UI.BasePath,
		Health:         readiness,
		Workflow:       api.NewWorkflowHandler(workflowSvc),
		VisualWorkflow: api.NewVisualWorkflowHandler(visualWorkflowSvc),
		Script:         api.NewScriptHandler(scriptSvc),
		Run:            api.NewRunHandler(runSvc),
		Agent:          api.NewAgentHandler(agentSvc),
		Skill:          api.NewSkillHandler(skillSvc),
		Environment:    api.NewEnvironmentHandler(environmentSvc),
		MCP:            api.NewMcpHandler(mcpSvc),
		Dashboard:      api.NewDashboardHandler(dashboardSvc),
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

	return &Runtime{Handler: router, runSvc: runSvc, workflowSvc: workflowSvc}, nil
}

func (r *Runtime) SetWorkflowReferenceChecker(checker service.WorkflowReferenceChecker) {
	if r == nil || r.workflowSvc == nil {
		return
	}
	r.workflowSvc.SetWorkflowReferenceChecker(checker)
}

func (r *Runtime) SetWorkflowReferenceGuardMode(mode service.WorkflowReferenceGuardMode) {
	if r == nil || r.workflowSvc == nil {
		return
	}
	r.workflowSvc.SetWorkflowReferenceGuardMode(mode)
}

func (r *Runtime) SetOpsManualRunRecordSink(sink service.OpsManualRunRecordSink) {
	if r == nil || r.runSvc == nil {
		return
	}
	r.runSvc.SetOpsManualRunRecordSink(sink)
}

func (r *Runtime) Close(ctx context.Context) error {
	if r == nil || r.runSvc == nil {
		return nil
	}
	done := make(chan struct{})
	go func() {
		r.runSvc.Close()
		close(done)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}
