package api

import "net/http"

type WorkflowHandler interface {
	List(w http.ResponseWriter, r *http.Request)
	Get(w http.ResponseWriter, r *http.Request)
	Create(w http.ResponseWriter, r *http.Request)
	Update(w http.ResponseWriter, r *http.Request)
	Delete(w http.ResponseWriter, r *http.Request)
	Validate(w http.ResponseWriter, r *http.Request)
	DryRun(w http.ResponseWriter, r *http.Request)
}

type ScriptHandler interface {
	List(w http.ResponseWriter, r *http.Request)
	Get(w http.ResponseWriter, r *http.Request)
	Create(w http.ResponseWriter, r *http.Request)
	Update(w http.ResponseWriter, r *http.Request)
	Delete(w http.ResponseWriter, r *http.Request)
	Render(w http.ResponseWriter, r *http.Request)
}

type RunHandler interface {
	Submit(w http.ResponseWriter, r *http.Request)
	Get(w http.ResponseWriter, r *http.Request)
	List(w http.ResponseWriter, r *http.Request)
	Cancel(w http.ResponseWriter, r *http.Request)
	Events(w http.ResponseWriter, r *http.Request)
	EventsHistory(w http.ResponseWriter, r *http.Request)
}

type AgentHandler interface {
	List(w http.ResponseWriter, r *http.Request)
	Get(w http.ResponseWriter, r *http.Request)
	Create(w http.ResponseWriter, r *http.Request)
	Update(w http.ResponseWriter, r *http.Request)
	Delete(w http.ResponseWriter, r *http.Request)
	Heartbeat(w http.ResponseWriter, r *http.Request)
	Probe(w http.ResponseWriter, r *http.Request)
}

type SkillHandler interface {
	List(w http.ResponseWriter, r *http.Request)
	Get(w http.ResponseWriter, r *http.Request)
	Create(w http.ResponseWriter, r *http.Request)
	Update(w http.ResponseWriter, r *http.Request)
	Delete(w http.ResponseWriter, r *http.Request)
}

type EnvironmentHandler interface {
	List(w http.ResponseWriter, r *http.Request)
	Get(w http.ResponseWriter, r *http.Request)
	Create(w http.ResponseWriter, r *http.Request)
	AddVar(w http.ResponseWriter, r *http.Request)
	UpdateVar(w http.ResponseWriter, r *http.Request)
	DeleteVar(w http.ResponseWriter, r *http.Request)
}

type MCPHandler interface {
	List(w http.ResponseWriter, r *http.Request)
	Get(w http.ResponseWriter, r *http.Request)
	Create(w http.ResponseWriter, r *http.Request)
	Update(w http.ResponseWriter, r *http.Request)
	Delete(w http.ResponseWriter, r *http.Request)
	Toggle(w http.ResponseWriter, r *http.Request)
	Tools(w http.ResponseWriter, r *http.Request)
}

type DashboardHandler interface {
	Stats(w http.ResponseWriter, r *http.Request)
}

type SystemInfoHandler interface {
	Info(w http.ResponseWriter, r *http.Request)
	Metrics(w http.ResponseWriter, r *http.Request)
}

type RouterOptions struct {
	AuthEnabled    bool
	AuthToken      string
	CORSOrigins    []string
	UIBasePath     string

	Health *HealthHandler

	Workflow    WorkflowHandler
	Script      ScriptHandler
	Run         RunHandler
	Agent       AgentHandler
	Skill       SkillHandler
	Environment EnvironmentHandler
	MCP         MCPHandler
	Dashboard   DashboardHandler
	System      SystemInfoHandler

	MetricsHandler http.Handler
	UI             http.Handler
}

func NewRouter(opts RouterOptions) http.Handler {
	mux := http.NewServeMux()
	if opts.Health != nil {
		mux.HandleFunc("GET /healthz", opts.Health.Healthz)
		mux.HandleFunc("GET /readyz", opts.Health.Readyz)
	}
	if opts.MetricsHandler != nil {
		mux.Handle("GET /metrics", opts.MetricsHandler)
	}

	if opts.Workflow != nil {
		mux.HandleFunc("GET /api/v1/workflows", opts.Workflow.List)
		mux.HandleFunc("GET /api/v1/workflows/{name}", opts.Workflow.Get)
		mux.HandleFunc("POST /api/v1/workflows", opts.Workflow.Create)
		mux.HandleFunc("POST /api/v1/workflows/dry-run", opts.Workflow.DryRun)
		mux.HandleFunc("PUT /api/v1/workflows/{name}", opts.Workflow.Update)
		mux.HandleFunc("DELETE /api/v1/workflows/{name}", opts.Workflow.Delete)
		mux.HandleFunc("POST /api/v1/workflows/{name}/validate", opts.Workflow.Validate)
	}
	if opts.Script != nil {
		mux.HandleFunc("GET /api/v1/scripts", opts.Script.List)
		mux.HandleFunc("GET /api/v1/scripts/{name}", opts.Script.Get)
		mux.HandleFunc("POST /api/v1/scripts", opts.Script.Create)
		mux.HandleFunc("PUT /api/v1/scripts/{name}", opts.Script.Update)
		mux.HandleFunc("DELETE /api/v1/scripts/{name}", opts.Script.Delete)
		mux.HandleFunc("POST /api/v1/scripts/{name}/render", opts.Script.Render)
	}
	if opts.Run != nil {
		mux.HandleFunc("POST /api/v1/runs", opts.Run.Submit)
		mux.HandleFunc("GET /api/v1/runs", opts.Run.List)
		mux.HandleFunc("GET /api/v1/runs/{id}", opts.Run.Get)
		mux.HandleFunc("POST /api/v1/runs/{id}/cancel", opts.Run.Cancel)
		mux.HandleFunc("GET /api/v1/runs/{id}/events/history", opts.Run.EventsHistory)
		mux.HandleFunc("GET /api/v1/runs/{id}/events", opts.Run.Events)
	}
	if opts.Agent != nil {
		mux.HandleFunc("GET /api/v1/agents", opts.Agent.List)
		mux.HandleFunc("GET /api/v1/agents/{id}", opts.Agent.Get)
		mux.HandleFunc("POST /api/v1/agents", opts.Agent.Create)
		mux.HandleFunc("PUT /api/v1/agents/{id}", opts.Agent.Update)
		mux.HandleFunc("DELETE /api/v1/agents/{id}", opts.Agent.Delete)
		mux.HandleFunc("POST /api/v1/agents/{id}/heartbeat", opts.Agent.Heartbeat)
		mux.HandleFunc("POST /api/v1/agents/{id}/probe", opts.Agent.Probe)
	}
	if opts.Skill != nil {
		mux.HandleFunc("GET /api/v1/skills", opts.Skill.List)
		mux.HandleFunc("GET /api/v1/skills/{name}", opts.Skill.Get)
		mux.HandleFunc("POST /api/v1/skills", opts.Skill.Create)
		mux.HandleFunc("PUT /api/v1/skills/{name}", opts.Skill.Update)
		mux.HandleFunc("DELETE /api/v1/skills/{name}", opts.Skill.Delete)
	}
	if opts.Environment != nil {
		mux.HandleFunc("GET /api/v1/environments", opts.Environment.List)
		mux.HandleFunc("GET /api/v1/environments/{name}", opts.Environment.Get)
		mux.HandleFunc("POST /api/v1/environments", opts.Environment.Create)
		mux.HandleFunc("POST /api/v1/environments/{name}/vars", opts.Environment.AddVar)
		mux.HandleFunc("PUT /api/v1/environments/{name}/vars/{key}", opts.Environment.UpdateVar)
		mux.HandleFunc("DELETE /api/v1/environments/{name}/vars/{key}", opts.Environment.DeleteVar)
	}
	if opts.MCP != nil {
		mux.HandleFunc("GET /api/v1/mcp/servers", opts.MCP.List)
		mux.HandleFunc("GET /api/v1/mcp/servers/{id}", opts.MCP.Get)
		mux.HandleFunc("POST /api/v1/mcp/servers", opts.MCP.Create)
		mux.HandleFunc("PUT /api/v1/mcp/servers/{id}", opts.MCP.Update)
		mux.HandleFunc("DELETE /api/v1/mcp/servers/{id}", opts.MCP.Delete)
		mux.HandleFunc("POST /api/v1/mcp/servers/{id}/toggle", opts.MCP.Toggle)
		mux.HandleFunc("GET /api/v1/mcp/servers/{id}/tools", opts.MCP.Tools)
	}
	if opts.Dashboard != nil {
		mux.HandleFunc("GET /api/v1/dashboard", opts.Dashboard.Stats)
	}
	if opts.System != nil {
		mux.HandleFunc("GET /api/v1/system/info", opts.System.Info)
		mux.HandleFunc("GET /api/v1/system/metrics", opts.System.Metrics)
	}
	if opts.UI != nil {
		mux.Handle("/", opts.UI)
	}

	middlewares := []func(http.Handler) http.Handler{
		BasePathMiddleware(opts.UIBasePath),
	}
	if len(opts.CORSOrigins) > 0 {
		middlewares = append(middlewares, CORSMiddleware(opts.CORSOrigins))
	}
	middlewares = append(middlewares,
		AuthMiddleware(opts.AuthEnabled, opts.AuthToken),
		TraceAndAccessLogMiddleware,
	)

	return Chain(mux, middlewares...)
}
