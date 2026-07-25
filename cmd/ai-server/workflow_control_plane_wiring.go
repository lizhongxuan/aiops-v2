package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"aiops-v2/internal/auth"
	workflowintegration "aiops-v2/internal/integrations/workfloweditor"
	"aiops-v2/internal/mcp"
	"aiops-v2/internal/observability"
	"aiops-v2/internal/runnerembed"
	"aiops-v2/internal/server"
	"aiops-v2/internal/store"
	"aiops-v2/internal/tooling"
	"aiops-v2/internal/workfloweditor"
	runnerservice "runner/server/service"
	workflowedit "runner/workflow/edit"
)

const workflowResourceSigningKeyFile = "workflow-resource-signing.key"

func registerWorkflowControlPlane(registry *mcp.Registry, runtime *runnerembed.Runtime, secretDir string, runtimeSettings ...store.RuntimeSettings) (*runnerservice.WorkflowEditApplication, error) {
	if registry == nil || runtime == nil || runtime.WorkflowEditApplication() == nil {
		return nil, fmt.Errorf("workflow control plane: Runner application is required")
	}
	application := runtime.WorkflowEditApplication()
	settings := store.DefaultRuntimeSettings()
	if len(runtimeSettings) > 0 {
		settings = runtimeSettings[0]
	}
	capabilitySnapshot := workflowCapabilitySnapshotFromRuntimeSettings(settings)
	if !capabilitySnapshot.Capabilities.WorkflowMCPRead {
		// Disabled and rollout-excluded tenants do not get a Runner MCP server,
		// resource provider, or signing key. The Runner application remains the
		// authoritative manual/HTTP data plane.
		return application, nil
	}
	key, err := loadOrCreateWorkflowResourceSigningKey(secretDir)
	if err != nil {
		return nil, fmt.Errorf("workflow control plane: load resource signer: %w", err)
	}
	resources, err := workflowintegration.NewWorkflowResourcesV2WithKey(application, key)
	if err != nil {
		return nil, fmt.Errorf("workflow control plane: create resources: %w", err)
	}
	provider, err := workflowintegration.NewWorkflowResourceProviderV2(resources)
	if err != nil {
		return nil, fmt.Errorf("workflow control plane: create resource provider: %w", err)
	}
	registration, err := workflowintegration.WorkflowResourceProviderRegistrationV2(provider)
	if err != nil {
		return nil, fmt.Errorf("workflow control plane: configure resource provider: %w", err)
	}
	tools := append([]tooling.Tool{}, workflowintegration.WorkflowReadToolsV2(application, resources)...)
	tools = append(tools, workflowintegration.WorkflowValidateToolV2(application))
	if capabilitySnapshot.Capabilities.WorkflowMCPPreview {
		tools = append(tools, workflowintegration.WorkflowPreviewPatchToolV2(application))
	}
	if capabilitySnapshot.Capabilities.WorkflowPlanIR {
		tools = append(tools, workflowintegration.WorkflowCompilePlanToolV2(application))
	}
	if capabilitySnapshot.Capabilities.WorkflowMCPMutation {
		tools = append(tools, workflowintegration.WorkflowApplyPatchToolV2(application))
	}
	if err := registry.RegisterRunnerCapability(tools); err != nil {
		return nil, fmt.Errorf("workflow control plane: register tools: %w", err)
	}
	if err := registry.RegisterResourceProvider(registration); err != nil {
		registry.UnregisterServer(workflowintegration.ServerID)
		return nil, fmt.Errorf("workflow control plane: register resources: %w", err)
	}
	return application, nil
}

func workflowCapabilitySnapshotFromRuntimeSettings(settings store.RuntimeSettings) workfloweditor.WorkflowCapabilitySnapshot {
	settings = store.NormalizeRuntimeSettings(settings)
	snapshot, err := workfloweditor.EvaluateWorkflowRollout(settings.Workflow.Rollout, runnerservice.VisualWorkflowDefaultTenantID)
	if err == nil {
		return snapshot
	}
	// NormalizeRuntimeSettings already fails closed. Keep the fallback local so
	// production registration never depends on a caller handling an error.
	fallback, _ := workfloweditor.EvaluateWorkflowRollout(store.DefaultRuntimeSettings().Workflow.Rollout, runnerservice.VisualWorkflowDefaultTenantID)
	return fallback
}

func wireWorkflowObservability(application *runnerservice.WorkflowEditApplication, provider *observability.Provider) observability.WorkflowObserver {
	var tracer = provider.Tracer()
	observer := observability.NewWorkflowObserver(tracer, observability.DefaultWorkflowMetrics())
	if application != nil {
		application.SetObserver(observer)
	}
	return observer
}

func loadOrCreateWorkflowResourceSigningKey(secretDir string) ([]byte, error) {
	secretDir = filepath.Clean(strings.TrimSpace(secretDir))
	if secretDir == "." || secretDir == "" {
		return nil, fmt.Errorf("secret directory is required")
	}
	if err := os.MkdirAll(secretDir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(secretDir, workflowResourceSigningKeyFile)
	if key, err := readWorkflowResourceSigningKey(path); err == nil {
		return key, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	temporary, err := os.CreateTemp(secretDir, ".workflow-resource-signing-*")
	if err != nil {
		return nil, err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return nil, err
	}
	if _, err := temporary.Write(key); err != nil {
		_ = temporary.Close()
		return nil, err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return nil, err
	}
	if err := temporary.Close(); err != nil {
		return nil, err
	}
	if err := os.Link(temporaryPath, path); err != nil {
		if !os.IsExist(err) {
			return nil, err
		}
		return readWorkflowResourceSigningKey(path)
	}
	if directory, err := os.Open(secretDir); err == nil {
		if syncErr := directory.Sync(); syncErr != nil {
			_ = directory.Close()
			return nil, syncErr
		}
		_ = directory.Close()
	}
	return append([]byte(nil), key...), nil
}

func readWorkflowResourceSigningKey(path string) ([]byte, error) {
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("workflow resource signing key must contain exactly 32 bytes")
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return nil, err
	}
	return append([]byte(nil), key...), nil
}

func newWorkflowCommandScopeResolver(manager *auth.Manager, tenantID string) server.WorkflowCommandScopeResolver {
	tenantID = strings.TrimSpace(tenantID)
	return server.WorkflowCommandScopeResolverFunc(func(_ context.Context, request *http.Request, workflowID string) (server.WorkflowCommandTrustedScope, error) {
		workflowID = strings.TrimSpace(workflowID)
		key := workflowedit.WorkflowKey{TenantID: tenantID, WorkflowID: workflowID}
		if request == nil || key.Validate() != nil {
			return server.WorkflowCommandTrustedScope{}, fmt.Errorf("workflow command scope is invalid")
		}

		actorID, sessionSeed := "local-user", "local-session"
		cookie, err := request.Cookie(auth.DefaultCookieName)
		switch {
		case err == nil:
			if manager == nil {
				return server.WorkflowCommandTrustedScope{}, fmt.Errorf("workflow command authentication is unavailable")
			}
			summary, ok := manager.SummaryForCookie(cookie.Value)
			if !ok || !summary.Connected {
				return server.WorkflowCommandTrustedScope{}, fmt.Errorf("workflow command authentication is invalid")
			}
			sessionSeed = "authenticated:" + cookie.Value
			actorID = strings.TrimSpace(summary.Email)
			if actorID == "" {
				actorID = "authenticated-user-" + shortWorkflowScopeDigest(cookie.Value)
			}
		case err != http.ErrNoCookie:
			return server.WorkflowCommandTrustedScope{}, fmt.Errorf("workflow command authentication could not be read")
		}

		scopeDigest := shortWorkflowScopeDigest(tenantID + "\x00" + workflowID + "\x00" + sessionSeed)
		return server.WorkflowCommandTrustedScope{
			TenantID:        tenantID,
			ActorID:         actorID,
			CanvasSessionID: "canvas-" + scopeDigest,
			SessionID:       "workflow-" + scopeDigest,
		}, nil
	})
}

func shortWorkflowScopeDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:8])
}
