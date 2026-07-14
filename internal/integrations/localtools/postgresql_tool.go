package localtools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aiops-v2/internal/tooling"
)

const postgresqlToolTimeout = 60 * time.Second

func NewEnsurePostgreSQLInstalledTool(opts Options) tooling.Tool {
	opts = opts.normalize()
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:             "ensure_postgresql_installed",
			Aliases:          []string{"install_postgresql", "ensure_pg_installed"},
			Origin:           tooling.ToolOriginBuiltin,
			Description:      "Ensure PostgreSQL is installed on the currently bound host. It first checks the existing psql version and skips reinstall when present. If installation is needed, it requests approval before package/service changes and then executes through the bound host-agent.",
			Domain:           "host.postgresql",
			Layer:            tooling.ToolLayerDeferred,
			Pack:             "database_ops",
			DeferByDefault:   true,
			SearchHint:       "database package install middleware postgresql postgres",
			RiskLevel:        tooling.ToolRiskMedium,
			Mutating:         true,
			RequiresApproval: true,
			ResourceLocks: []tooling.ToolResourceLockKey{{
				ResourceType:  "host",
				ResourceID:    "selected_host",
				OperationKind: "package_install",
			}, {
				ResourceType:  "package",
				ResourceID:    "postgresql",
				OperationKind: "install",
			}},
			Idempotency: tooling.ToolIdempotencyMetadata{
				Strategy: tooling.ToolIdempotencyStrategyArgumentsHash,
				PostCheckRefs: []string{
					"psql --version",
					"systemctl is-active postgresql or pg_isready when available",
				},
			},
			Rollback: &tooling.ToolRollbackMetadata{
				Strategy:  tooling.ToolRollbackStrategyManualTakeover,
				Reference: "localtools.ensure_postgresql_installed.rollback-v1",
			},
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind: "database_ops",
				ResourceTypes:  []string{"database", "host", "package"},
				OperationKinds: []string{"inspect", "install", "modify"},
				RequiresSelect: true,
			},
			ResultBudget: tooling.ResultBudget{
				MaxInlineResultBytes: opts.MaxOutputBytes,
				SpillPolicy:          tooling.ResultSpillPolicySummaryInline,
				SummarizeLargeResult: true,
			},
		},
		Visibility: tooling.Visibility{SessionTypes: []string{"host"}, Modes: []string{"execute"}},
		InputSchemaData: json.RawMessage(`{
			"type": "object",
			"properties": {
				"reason": {"type": "string", "description": "Why PostgreSQL is needed on this host."}
			}
		}`),
		OutputSchemaData: json.RawMessage(`{
			"type": "object",
			"properties": {
				"schemaVersion": {"type": "string"},
				"tool": {"type": "string"},
				"status": {"type": "string"},
				"source": {"type": "string"},
				"hostId": {"type": "string"},
				"version": {"type": "string"},
				"stdout": {"type": "string"},
				"stderr": {"type": "string"}
			},
			"required": ["schemaVersion", "tool", "status", "hostId"]
		}`),
		ReadOnlyFunc:        func(json.RawMessage) bool { return false },
		DestructiveFunc:     func(json.RawMessage) bool { return true },
		ConcurrencySafeFunc: func(json.RawMessage) bool { return false },
		CheckPermissionsFunc: func(ctx context.Context, _ json.RawMessage) tooling.PermissionDecision {
			hostID := selectedRemoteHostID(ctx)
			if strings.TrimSpace(hostID) == "" {
				return tooling.PermissionDecision{Action: tooling.PermissionActionDeny, Reason: "PostgreSQL installation requires a bound remote host"}
			}
			if _, err := lookupSelectedRemoteHost(ctx, opts.HostRepository); err != nil {
				return tooling.PermissionDecision{Action: tooling.PermissionActionDeny, Reason: err.Error()}
			}
			version, err := postgresqlVersion(ctx, opts, hostID)
			if err == nil && strings.TrimSpace(version) != "" {
				return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
			}
			return tooling.PermissionDecision{
				Action: tooling.PermissionActionNeedApproval,
				Reason: "PostgreSQL is not installed on the bound host; package installation and service start require approval",
				Approval: &tooling.PermissionApprovalPayload{
					Command:        "install PostgreSQL and start PostgreSQL service if needed",
					Reason:         "PostgreSQL package installation changes host packages and service state",
					Risk:           string(tooling.ToolRiskHigh),
					Source:         "host-agent",
					ExpectedEffect: "PostgreSQL is installed, service is enabled/started when supported, and psql --version succeeds",
					Rollback:       "Review package manager history and remove PostgreSQL only after explicit follow-up approval; this tool does not delete existing data",
				},
			}
		},
		ExecuteFunc: func(ctx context.Context, _ json.RawMessage) (tooling.ToolResult, error) {
			host, err := lookupSelectedRemoteHost(ctx, opts.HostRepository)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			if opts.HostAgentCommandRunner == nil {
				return tooling.ToolResult{}, fmt.Errorf("host-agent command runner is not configured")
			}
			version, err := postgresqlVersion(ctx, opts, host.ID)
			if err == nil && strings.TrimSpace(version) != "" {
				return postgresqlToolResult(opts, host.ID, "skipped_existing", version, version, "")
			}
			installResult, err := runPostgreSQLInstall(ctx, opts, host.ID)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			version, versionErr := postgresqlVersion(ctx, opts, host.ID)
			if versionErr != nil || strings.TrimSpace(version) == "" {
				return tooling.ToolResult{}, fmt.Errorf("PostgreSQL install completed but version check failed: %v; stdout: %s; stderr: %s", versionErr, truncateString(installResult.Stdout, opts.MaxOutputBytes/2), truncateString(installResult.Stderr, opts.MaxOutputBytes/2))
			}
			stdout := strings.TrimSpace(installResult.Stdout)
			if stdout != "" {
				stdout += "\n"
			}
			stdout += strings.TrimSpace(version)
			return postgresqlToolResult(opts, host.ID, "installed", version, stdout, installResult.Stderr)
		},
	}
}

func postgresqlVersion(ctx context.Context, opts Options, hostID string) (string, error) {
	result, err := runHostAgentShell(ctx, opts, hostID, "if command -v psql >/dev/null 2>&1; then psql --version; else exit 127; fi", 15*time.Second)
	if err != nil {
		return "", err
	}
	if result.ExitCode != 0 {
		return "", fmt.Errorf("psql not found")
	}
	version := strings.TrimSpace(result.Stdout)
	if version == "" {
		return "", fmt.Errorf("psql version output is empty")
	}
	return version, nil
}

func runPostgreSQLInstall(ctx context.Context, opts Options, hostID string) (HostAgentCommandResult, error) {
	result, err := runHostAgentShell(ctx, opts, hostID, postgresqlInstallScript(), postgresqlToolTimeout)
	if err != nil {
		return HostAgentCommandResult{}, err
	}
	if result.ExitCode != 0 {
		return HostAgentCommandResult{}, fmt.Errorf("PostgreSQL install failed: exit status %d; stderr: %s", result.ExitCode, truncateString(result.Stderr, opts.MaxOutputBytes/2))
	}
	return result, nil
}

func runHostAgentShell(ctx context.Context, opts Options, hostID, script string, timeout time.Duration) (HostAgentCommandResult, error) {
	if opts.HostAgentCommandRunner == nil {
		return HostAgentCommandResult{}, fmt.Errorf("host-agent command runner is not configured")
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result, err := opts.HostAgentCommandRunner.RunHostAgentCommand(runCtx, HostAgentCommandRequest{
		HostID:         hostID,
		Command:        "sh",
		Args:           []string{"-lc", script},
		Timeout:        timeout,
		MaxOutputBytes: opts.MaxOutputBytes,
	})
	if runCtx.Err() != nil {
		return HostAgentCommandResult{}, runCtx.Err()
	}
	return result, err
}

func postgresqlInstallScript() string {
	return strings.Join([]string{
		"set -e",
		"if command -v psql >/dev/null 2>&1; then psql --version; exit 0; fi",
		"run_privileged() {",
		"  if [ \"$(id -u)\" -eq 0 ]; then",
		"    \"$@\"",
		"  elif command -v sudo >/dev/null 2>&1 && sudo -n true >/dev/null 2>&1; then",
		"    sudo -n \"$@\"",
		"  else",
		"    echo 'PostgreSQL install requires root or passwordless sudo for the host-agent user' >&2",
		"    exit 126",
		"  fi",
		"}",
		"if command -v apt-get >/dev/null 2>&1; then",
		"  export DEBIAN_FRONTEND=noninteractive",
		"  run_privileged apt-get update",
		"  run_privileged apt-get install -y postgresql",
		"elif command -v dnf >/dev/null 2>&1; then",
		"  run_privileged dnf install -y postgresql-server postgresql-contrib || run_privileged dnf install -y postgresql",
		"elif command -v yum >/dev/null 2>&1; then",
		"  run_privileged yum install -y postgresql-server postgresql-contrib || run_privileged yum install -y postgresql",
		"elif command -v apk >/dev/null 2>&1; then",
		"  run_privileged apk add --no-cache postgresql postgresql-client",
		"else",
		"  echo 'unsupported package manager for PostgreSQL install' >&2",
		"  exit 127",
		"fi",
		"if command -v postgresql-setup >/dev/null 2>&1; then run_privileged postgresql-setup --initdb >/dev/null 2>&1 || true; fi",
		"if command -v systemctl >/dev/null 2>&1; then",
		"  run_privileged systemctl enable --now postgresql >/dev/null 2>&1 || run_privileged systemctl start postgresql >/dev/null 2>&1 || { echo 'failed to start PostgreSQL service' >&2; exit 1; }",
		"  systemctl is-active --quiet postgresql || { echo 'PostgreSQL service is not active' >&2; exit 1; }",
		"  if command -v pg_isready >/dev/null 2>&1; then pg_isready -q || { echo 'PostgreSQL service is not accepting connections' >&2; exit 1; }; fi",
		"else",
		"  echo 'systemctl not available; PostgreSQL package installed and version will be verified, service startup not verified' >&2",
		"fi",
		"psql --version",
	}, "\n")
}

func postgresqlToolResult(opts Options, hostID, status, version, stdout, stderr string) (tooling.ToolResult, error) {
	payload := map[string]any{
		"schemaVersion": "aiops.postgresql/v1",
		"tool":          "ensure_postgresql_installed",
		"status":        status,
		"source":        "host.agent",
		"hostId":        hostID,
		"version":       strings.TrimSpace(version),
		"stdout":        truncateString(stdout, opts.MaxOutputBytes),
		"stderr":        truncateString(stderr, opts.MaxOutputBytes/2),
	}
	content, err := json.Marshal(payload)
	if err != nil {
		return tooling.ToolResult{}, err
	}
	return tooling.ToolResult{
		Content: string(content),
		Display: &tooling.ToolDisplayPayload{
			Type:  "host_postgresql",
			Title: "PostgreSQL",
		},
	}, nil
}
