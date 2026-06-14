package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"aiops-v2/internal/mcp"
	"aiops-v2/internal/tooling"
)

type RuntimeOptions struct {
	Registry           *mcp.Registry
	ClientFactory      ClientFactory
	GovernanceProvider ServerGovernanceProvider
}

type Runtime struct {
	mu                 sync.RWMutex
	registry           *mcp.Registry
	factory            ClientFactory
	governanceProvider ServerGovernanceProvider
	clients            map[string]Client
}

func New(opts RuntimeOptions) *Runtime {
	return &Runtime{
		registry:           opts.Registry,
		factory:            opts.ClientFactory,
		governanceProvider: opts.GovernanceProvider,
		clients:            map[string]Client{},
	}
}

func (r *Runtime) Start(ctx context.Context) error {
	if r == nil || r.registry == nil {
		return nil
	}
	for _, cfg := range r.registry.ListServers() {
		if cfg.Disabled {
			continue
		}
		if !isConnectable(cfg) {
			continue
		}
		if err := r.Connect(ctx, cfg.ID); err != nil {
			return err
		}
	}
	return nil
}

func isConnectable(cfg mcp.ServerConfig) bool {
	switch strings.ToLower(strings.TrimSpace(cfg.Transport)) {
	case "http", "stdio":
		return len(cfg.Command) > 0 && strings.TrimSpace(cfg.Command[0]) != ""
	default:
		return false
	}
}

func (r *Runtime) Connect(ctx context.Context, serverID string) error {
	if r == nil {
		return fmt.Errorf("mcp runtime is nil")
	}
	serverID = strings.TrimSpace(serverID)
	if serverID == "" {
		return fmt.Errorf("mcp runtime: server id is required")
	}
	if r.registry == nil {
		return fmt.Errorf("mcp runtime: registry is required")
	}
	cfg, ok := r.registry.GetServer(serverID)
	if !ok {
		return fmt.Errorf("mcp runtime: server %q not found", serverID)
	}
	if cfg.Disabled || r.registry.IsServerDisabled(serverID) {
		r.registry.OnServerDisconnected(serverID)
		r.registry.SetServerStatus(serverID, mcp.ServerStatus{State: mcp.ServerStateDisconnected})
		return nil
	}
	if r.factory == nil {
		err := fmt.Errorf("mcp runtime: client factory is required")
		r.registry.SetServerStatus(serverID, mcp.ServerStatus{State: mcp.ServerStateFailed, LastError: err.Error()})
		return err
	}
	r.registry.SetServerStatus(serverID, mcp.ServerStatus{State: mcp.ServerStateConnecting})
	client, err := r.factory.NewClient(ctx, cfg)
	if err != nil {
		r.registry.OnServerDisconnected(serverID)
		r.registry.SetServerStatus(serverID, mcp.ServerStatus{State: mcp.ServerStateFailed, LastError: err.Error()})
		return err
	}
	r.mu.Lock()
	if previous := r.clients[serverID]; previous != nil {
		_ = previous.Close(ctx)
	}
	r.clients[serverID] = client
	r.mu.Unlock()

	return r.refresh(ctx, cfg, client)
}

func (r *Runtime) Disconnect(ctx context.Context, serverID string) error {
	if r == nil {
		return nil
	}
	serverID = strings.TrimSpace(serverID)
	r.mu.Lock()
	client := r.clients[serverID]
	delete(r.clients, serverID)
	r.mu.Unlock()
	if client != nil {
		if err := client.Close(ctx); err != nil {
			return err
		}
	}
	if r.registry != nil {
		r.registry.OnServerDisconnected(serverID)
		r.registry.SetServerStatus(serverID, mcp.ServerStatus{State: mcp.ServerStateDisconnected})
	}
	return nil
}

func (r *Runtime) RefreshTools(ctx context.Context, serverID string) error {
	if r == nil {
		return fmt.Errorf("mcp runtime is nil")
	}
	serverID = strings.TrimSpace(serverID)
	if r.registry == nil {
		return fmt.Errorf("mcp runtime: registry is required")
	}
	cfg, ok := r.registry.GetServer(serverID)
	if !ok {
		return fmt.Errorf("mcp runtime: server %q not found", serverID)
	}
	r.mu.RLock()
	client := r.clients[serverID]
	r.mu.RUnlock()
	if client == nil {
		return r.Connect(ctx, serverID)
	}
	return r.refresh(ctx, cfg, client)
}

func (r *Runtime) CallTool(ctx context.Context, serverID, toolName string, input json.RawMessage) (ToolCallResult, error) {
	if r == nil {
		return ToolCallResult{}, fmt.Errorf("mcp runtime is nil")
	}
	serverID = strings.TrimSpace(serverID)
	toolName = strings.TrimSpace(toolName)
	if serverID == "" {
		return ToolCallResult{}, fmt.Errorf("mcp runtime: server id is required")
	}
	if toolName == "" {
		return ToolCallResult{}, fmt.Errorf("mcp runtime: tool name is required")
	}
	r.mu.RLock()
	client := r.clients[serverID]
	r.mu.RUnlock()
	if client == nil {
		return ToolCallResult{}, fmt.Errorf("mcp runtime: server %q is not connected", serverID)
	}
	return client.CallTool(ctx, toolName, input)
}

func (r *Runtime) ListResources(ctx context.Context, serverID string) ([]mcp.Resource, error) {
	if err := r.RefreshTools(ctx, serverID); err != nil {
		return nil, err
	}
	if r.registry == nil {
		return nil, nil
	}
	return r.registry.ListResources(serverID), nil
}

func (r *Runtime) ReadResource(ctx context.Context, serverID, uri string) (mcp.ResourceContent, error) {
	serverID = strings.TrimSpace(serverID)
	uri = strings.TrimSpace(uri)
	r.mu.RLock()
	client := r.clients[serverID]
	r.mu.RUnlock()
	if client == nil {
		return mcp.ResourceContent{}, fmt.Errorf("mcp runtime: server %q is not connected", serverID)
	}
	content, err := client.ReadResource(ctx, uri)
	if err != nil {
		return mcp.ResourceContent{}, err
	}
	content.ServerID = serverID
	content.URI = uri
	return content, nil
}

func (r *Runtime) refresh(ctx context.Context, cfg mcp.ServerConfig, client Client) error {
	tools, err := client.ListTools(ctx)
	if err != nil {
		if r.registry != nil {
			r.registry.OnServerDisconnected(cfg.ID)
			r.registry.SetServerStatus(cfg.ID, mcp.ServerStatus{State: mcp.ServerStateFailed, LastError: err.Error()})
		}
		return err
	}
	adapted := make([]tooling.Tool, 0, len(tools))
	governance := mcp.ServerGovernance{}
	if r.governanceProvider != nil {
		governance = r.governanceProvider.ServerGovernance(cfg.ID)
	}
	for _, def := range tools {
		if strings.TrimSpace(def.Name) == "" {
			continue
		}
		adapted = append(adapted, makeTool(cfg, governance, def, r.CallTool))
	}
	if err := r.registry.OnServerConnected(cfg.ID, adapted); err != nil {
		return err
	}
	resources, err := client.ListResources(ctx)
	if err != nil {
		r.registry.SetServerStatus(cfg.ID, mcp.ServerStatus{State: mcp.ServerStateStale, LastError: err.Error()})
		return nil
	}
	if err := r.registry.OnServerResources(cfg.ID, resources); err != nil {
		return err
	}
	r.registry.SetServerStatus(cfg.ID, mcp.ServerStatus{State: mcp.ServerStateConnected})
	return nil
}
