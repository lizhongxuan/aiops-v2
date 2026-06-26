package localtools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"aiops-v2/internal/actionproposal"
	"aiops-v2/internal/tooling"
)

func NewPowerShellCommandTool(opts Options) tooling.Tool {
	opts = opts.normalize()
	signer := actionproposal.NewSigner(opts.ActionTokenSecret, opts.Now)
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:           "powershell_command",
			Aliases:        []string{"powershell", "pwsh"},
			Origin:         tooling.ToolOriginBuiltin,
			Description:    "Run a PowerShell command when the runtime explicitly advertises PowerShell support. It follows the same approval boundary as exec_command.",
			Layer:          tooling.ToolLayerConditional,
			Pack:           "powershell",
			DeferByDefault: true,
			RiskLevel:      tooling.ToolRiskHigh,
			ResourceLocks: []tooling.ToolResourceLockKey{{
				ResourceType:  "host",
				ResourceID:    "selected_host",
				OperationKind: "powershell_command",
			}},
			Idempotency: tooling.ToolIdempotencyMetadata{
				Strategy: tooling.ToolIdempotencyStrategyArgumentsHash,
				PostCheckRefs: []string{
					"run an explicit read-only PowerShell verification command for the changed service, process, file, package, or endpoint",
				},
			},
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind:    "execute",
				ResourceTypes:     []string{"host", "windows", "system"},
				OperationKinds:    []string{"inspect", "read", "execute"},
				ToolPackIDs:       []string{"powershell"},
				DiscoveryTags:     []string{"powershell", "windows", "pwsh"},
				RequiresSelect:    true,
				PermissionScope:   "host_command",
				PromptBudgetClass: "compact",
				SchemaBudgetClass: "on_demand",
			},
		},
		Visibility:       tooling.Visibility{SessionTypes: []string{"host", "workspace"}, Modes: []string{"chat", "inspect", "plan", "execute"}},
		InputSchemaData:  json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"},"args":{"type":"array","items":{"type":"string"}},"timeoutMs":{"type":"integer"},"actionToken":{"type":"string"}},"required":["command"]}`),
		OutputSchemaData: json.RawMessage(`{"type":"object"}`),
		ReadOnlyFunc:     func(json.RawMessage) bool { return false },
		DestructiveFunc:  func(json.RawMessage) bool { return true },
		ConcurrencySafeFunc: func(json.RawMessage) bool {
			return false
		},
		CheckPermissionsFunc: func(_ context.Context, input json.RawMessage) tooling.PermissionDecision {
			var req struct {
				ActionToken string `json:"actionToken,omitempty"`
			}
			_ = json.Unmarshal(input, &req)
			if req.ActionToken != "" {
				if _, err := signer.Verify(req.ActionToken, actionproposal.ActionTokenClaims{
					ToolName: "powershell_command",
					Risk:     actionproposal.RiskHigh,
				}); err == nil {
					return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
				}
			}
			return tooling.PermissionDecision{
				Action: tooling.PermissionActionNeedApproval,
				Reason: "PowerShell commands require explicit approval and are only available when the runtime advertises powershell capability.",
			}
		},
		ExecuteFunc: executePowerShell(opts),
	}
}

func executePowerShell(opts Options) func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
	return func(ctx context.Context, input json.RawMessage) (tooling.ToolResult, error) {
		var req struct {
			Command   string   `json:"command"`
			Args      []string `json:"args,omitempty"`
			TimeoutMs int      `json:"timeoutMs,omitempty"`
		}
		if err := json.Unmarshal(input, &req); err != nil {
			return tooling.ToolResult{}, err
		}
		req.Command = strings.TrimSpace(req.Command)
		if req.Command == "" {
			return tooling.ToolResult{}, fmt.Errorf("powershell_command: command is required")
		}
		binary := "pwsh"
		if runtime.GOOS == "windows" {
			binary = "powershell"
		}
		if _, err := exec.LookPath(binary); err != nil {
			return tooling.ToolResult{}, fmt.Errorf("powershell_command: %s is not available in this runtime", binary)
		}
		timeout := opts.CommandTimeout
		if req.TimeoutMs > 0 {
			timeout = time.Duration(req.TimeoutMs) * time.Millisecond
		}
		runCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		args := []string{"-NoProfile", "-Command", req.Command}
		args = append(args, req.Args...)
		cmd := exec.CommandContext(runCtx, binary, args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return tooling.ToolResult{}, err
		}
		data, _ := json.Marshal(map[string]any{
			"schemaVersion": "aiops.powershell/v1",
			"tool":          "powershell_command",
			"stdout":        string(out),
		})
		return tooling.ToolResult{Content: string(data)}, nil
	}
}

func NewREPLTool(opts Options) tooling.Tool {
	opts = opts.normalize()
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:             "repl",
			Aliases:          []string{"runtime_repl"},
			Origin:           tooling.ToolOriginBuiltin,
			Description:      "Run a small sandbox/debug REPL snippet only under debug/dev/sandbox profiles. Not available in normal operational chat.",
			Layer:            tooling.ToolLayerProfile,
			Pack:             "repl",
			Profiles:         []string{"debug", "dev", "sandbox"},
			DeferByDefault:   true,
			RiskLevel:        tooling.ToolRiskMedium,
			RequiresApproval: true,
			ResourceLocks: []tooling.ToolResourceLockKey{{
				ResourceType:  "runtime",
				ResourceID:    "scratch",
				OperationKind: "debug_repl",
			}},
			Idempotency: tooling.ToolIdempotencyMetadata{
				Strategy: tooling.ToolIdempotencyStrategyArgumentsHash,
				PostCheckRefs: []string{
					"run a read-only verification expression or command for any runtime state touched by the REPL code",
				},
			},
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind:    "repl",
				ResourceTypes:     []string{"runtime", "scratch"},
				OperationKinds:    []string{"execute"},
				ToolPackIDs:       []string{"repl"},
				DiscoveryTags:     []string{"debug", "sandbox", "repl"},
				RequiresSelect:    true,
				PermissionScope:   "debug_runtime",
				PromptBudgetClass: "compact",
				SchemaBudgetClass: "on_demand",
			},
			ResultBudget: tooling.ResultBudget{
				MaxInlineResultBytes: opts.MaxOutputBytes,
				SpillPolicy:          tooling.ResultSpillPolicySummaryInline,
				SummarizeLargeResult: true,
			},
		},
		Visibility:       tooling.Visibility{SessionTypes: []string{"host", "workspace"}, Modes: []string{"chat", "inspect", "plan", "execute"}},
		InputSchemaData:  json.RawMessage(`{"type":"object","properties":{"language":{"type":"string"},"code":{"type":"string"}},"required":["language","code"]}`),
		OutputSchemaData: json.RawMessage(`{"type":"object"}`),
		ReadOnlyFunc:     func(json.RawMessage) bool { return false },
		DestructiveFunc:  func(json.RawMessage) bool { return true },
		ConcurrencySafeFunc: func(json.RawMessage) bool {
			return false
		},
		CheckPermissionsFunc: func(context.Context, json.RawMessage) tooling.PermissionDecision {
			return tooling.PermissionDecision{Action: tooling.PermissionActionNeedApproval, Reason: "REPL execution requires explicit approval in debug/dev/sandbox profile"}
		},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{}, fmt.Errorf("repl: runtime adapter is not configured")
		},
	}
}
