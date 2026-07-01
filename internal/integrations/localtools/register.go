package localtools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"aiops-v2/internal/actionproposal"
	"aiops-v2/internal/evidence"
	"aiops-v2/internal/integrations/publicweb"
	"aiops-v2/internal/store"
	"aiops-v2/internal/terminalpolicy"
	"aiops-v2/internal/tooling"
)

const (
	defaultCommandTimeout = 15 * time.Second
	defaultWebTimeout     = 60 * time.Second
	defaultMaxOutputBytes = 20000
)

// LLMConfigRepository is the minimal settings read path needed by runtime tools.
type LLMConfigRepository interface {
	GetLLMConfig() (*store.LLMConfig, error)
}

type HostRepository interface {
	GetHost(id string) (*store.HostRecord, error)
	ListHosts() ([]store.HostRecord, error)
}

type HostAgentCommandRequest struct {
	HostID         string
	Command        string
	Args           []string
	WorkingDir     string
	Timeout        time.Duration
	MaxOutputBytes int
}

type HostAgentCommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Source   string
}

type HostAgentCommandRunner interface {
	RunHostAgentCommand(ctx context.Context, req HostAgentCommandRequest) (HostAgentCommandResult, error)
}

// Options configures builtin local tools.
type Options struct {
	WorkingDir                    string
	HTTPClient                    *http.Client
	CommandTimeout                time.Duration
	WebTimeout                    time.Duration
	MaxOutputBytes                int
	PublicSearchBaseURL           string
	ActionTokenSecret             []byte
	Now                           func() time.Time
	RequireApprovalForLowRiskExec bool
	EvidenceService               *evidence.Service
	HostRepository                HostRepository
	HostAgentCommandRunner        HostAgentCommandRunner
	TerminalPolicy                terminalpolicy.Provider
}

func (o Options) normalize() Options {
	if strings.TrimSpace(o.WorkingDir) == "" {
		if wd, err := os.Getwd(); err == nil {
			o.WorkingDir = wd
		}
	}
	if o.CommandTimeout <= 0 {
		o.CommandTimeout = defaultCommandTimeout
	}
	if o.WebTimeout <= 0 {
		o.WebTimeout = defaultWebTimeout
	}
	if o.MaxOutputBytes <= 0 {
		o.MaxOutputBytes = defaultMaxOutputBytes
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	return o
}

// RegisterBuiltins installs the local host tool surface into the single Tool registry.
func RegisterBuiltins(registry *tooling.Registry, repo LLMConfigRepository, opts Options) error {
	if registry == nil {
		return fmt.Errorf("localtools: registry is required")
	}
	for _, tool := range GetAllBaseTools(repo, opts) {
		if err := registry.Register(tool); err != nil {
			return err
		}
	}
	return nil
}

// GetAllBaseTools returns the local built-in base registry. It is a runtime
// helper, not a model-callable tool.
func GetAllBaseTools(repo LLMConfigRepository, opts Options) []tooling.Tool {
	tools := []tooling.Tool{
		NewWebSearchTool(repo, opts),
		NewBrowseURLTool(opts),
		NewExecCommandTool(opts),
		NewGrepTool(opts),
		NewPowerShellCommandTool(opts),
		NewREPLTool(opts),
		NewEnsurePostgreSQLInstalledTool(opts),
		NewCurrentModelConfigTool(repo),
	}
	if err := tooling.ValidateBaseRegistryTools(tools); err != nil {
		panic(err)
	}
	return tools
}

// NewCurrentModelConfigTool returns a safe model-settings inspection tool.
func NewCurrentModelConfigTool(repo LLMConfigRepository) tooling.Tool {
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:           "get_current_model_config",
			Aliases:        []string{"current_model_config", "get_model_config"},
			Origin:         tooling.ToolOriginBuiltin,
			Description:    "Read the currently configured LLM provider, model, base URL, and provider-native tool support without exposing secrets.",
			Layer:          tooling.ToolLayerDeferred,
			Pack:           "runtime_config",
			DeferByDefault: true,
			RiskLevel:      tooling.ToolRiskLow,
			Triggers:       []string{"current model", "model name", "model config", "模型配置", "当前模型", "当前模型配置"},
			SearchHint:     "current model provider config context size reasoning effort runtime configuration active llm",
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind: "runtime_config",
				ResourceTypes:  []string{"model", "runtime", "configuration"},
				OperationKinds: []string{"read", "inspect"},
				RequiresSelect: true,
			},
		},
		Visibility: tooling.Visibility{SessionTypes: []string{"host", "workspace"}},
		InputSchemaData: json.RawMessage(`{
			"type": "object",
			"properties": {}
		}`),
		OutputSchemaData: json.RawMessage(`{
			"type": "object",
			"properties": {
				"provider": {"type": "string"},
				"model": {"type": "string"},
				"baseURL": {"type": "string"},
				"apiKeySet": {"type": "boolean"},
				"reasoningEffort": {"type": "string"},
				"supportsReasoning": {"type": "boolean"},
				"providerNativeTools": {"type": "array", "items": {"type": "string"}}
			}
		}`),
		ReadOnlyFunc:        func(json.RawMessage) bool { return true },
		ConcurrencySafeFunc: func(json.RawMessage) bool { return true },
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			cfg := currentConfig(repo)
			payload := map[string]any{
				"provider":          cfg.Provider,
				"model":             cfg.Model,
				"baseURL":           cfg.BaseURL,
				"apiKeySet":         strings.TrimSpace(cfg.APIKey) != "",
				"reasoningEffort":   normalizeCurrentConfigReasoningEffort(cfg.ReasoningEffort),
				"supportsReasoning": providerSupportsReasoning(cfg.Provider, cfg.Model),
			}
			var nativeTools []string
			if providerSupportsNativeWebSearch(cfg.Provider, cfg.Model) {
				nativeTools = append(nativeTools, "web_search")
			}
			payload["providerNativeTools"] = nativeTools
			data, _ := json.Marshal(payload)
			return tooling.ToolResult{
				Content: string(data),
				Display: &tooling.ToolDisplayPayload{
					Type:  "model_config",
					Title: "Current model configuration",
				},
			}, nil
		},
	}
}

// NewExecCommandTool returns a local command tool. Shell operators are rejected
// by the tool itself; non-read-only commands must pass policy/approval first.
func NewExecCommandTool(opts Options) tooling.Tool {
	opts = opts.normalize()
	signer := actionproposal.NewSigner(opts.ActionTokenSecret, opts.Now)
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "exec_command",
			Aliases:     []string{"terminal_command", "shell_command"},
			Origin:      tooling.ToolOriginBuiltin,
			Description: execCommandDescription(),
			Layer:       tooling.ToolLayerCore,
			AlwaysLoad:  true,
			RiskLevel:   tooling.ToolRiskHigh,
			ResourceLocks: []tooling.ToolResourceLockKey{{
				ResourceType:  "host",
				ResourceID:    "selected_host",
				OperationKind: "terminal_command",
			}},
			Idempotency: tooling.ToolIdempotencyMetadata{
				Strategy: tooling.ToolIdempotencyStrategyArgumentsHash,
				PostCheckRefs: []string{
					"run an explicit read-only verification command for the changed service, process, file, package, or endpoint",
				},
			},
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind:  "host_fact",
				ResourceTypes:   []string{"host", "system", "主机", "系统"},
				OperationKinds:  []string{"inspect", "read", "execute", "查看", "读取"},
				DiscoveryTags:   []string{"bash", "shell", "execute", "cpu", "memory", "disk", "load", "filesystem", "network", "process", "resource", "资源", "信息", "监控", "状态"},
				PermissionScope: "argument_scoped",
			},
			ResultBudget: tooling.ResultBudget{
				MaxInlineResultBytes: opts.MaxOutputBytes,
				SpillPolicy:          tooling.ResultSpillPolicySummaryInline,
				SummarizeLargeResult: true,
			},
		},
		Visibility: tooling.Visibility{},
		InputSchemaData: json.RawMessage(`{
			"type": "object",
			"properties": {
				"command": {"type": "string", "description": "Executable name, for example date, pwd, ls, curl."},
				"args": {"type": "array", "items": {"type": "string"}, "description": "Command arguments. Prefer this over shell syntax."},
				"cmd": {"type": "string", "description": "Compatibility command line. Shell operators are rejected."},
				"workingDir": {"type": "string", "description": "Optional working directory."},
				"timeoutMs": {"type": "integer", "description": "Optional timeout in milliseconds, max 60000."},
				"actionToken": {"type": "string", "description": "Optional signed ActionToken for governed non-read-only actions."},
				"intent": {"type": "string", "description": "Audit-only human intent. It is never used as authorization evidence."}
			}
		}`),
		OutputSchemaData: json.RawMessage(`{
			"type": "object",
			"properties": {
				"schemaVersion": {"type": "string"},
				"tool": {"type": "string"},
				"status": {"type": "string"},
				"source": {"type": "string"},
				"stdout": {"type": "string"},
				"stderr": {"type": "string"},
				"exitCode": {"type": "integer"},
				"evidenceRefs": {"type": "array", "items": {"type": "string"}}
			},
			"required": ["schemaVersion", "tool", "status"]
		}`),
		ReadOnlyFunc: func(input json.RawMessage) bool {
			req, err := parseCommandInput(input)
			return err == nil && isAllowedExecReadOnly(opts.TerminalPolicy, req.command, req.args)
		},
		DestructiveFunc: func(input json.RawMessage) bool {
			req, err := parseCommandInput(input)
			return err != nil || !isAllowedExecReadOnly(opts.TerminalPolicy, req.command, req.args)
		},
		ConcurrencySafeFunc: func(input json.RawMessage) bool {
			req, err := parseCommandInput(input)
			return err == nil && isAllowedExecReadOnly(opts.TerminalPolicy, req.command, req.args)
		},
		CheckPermissionsFunc: func(ctx context.Context, input json.RawMessage) tooling.PermissionDecision {
			req, err := parseCommandInput(input)
			if err != nil {
				return tooling.PermissionDecision{Action: tooling.PermissionActionDeny, Reason: err.Error()}
			}
			if decision, ok := evaluateConfiguredTerminalPolicy(opts.TerminalPolicy, req.command, req.args); ok {
				return terminalPolicyDecisionToPermission(decision, req.command, req.args)
			}
			if selectedRemoteHostID(ctx) != "" {
				if isForbiddenExecCommand(req.command, req.args) {
					return tooling.PermissionDecision{Action: tooling.PermissionActionDeny, Reason: "forbidden terminal command is blocked by policy"}
				}
				host, err := lookupSelectedRemoteHost(ctx, opts.HostRepository)
				if err != nil {
					return tooling.PermissionDecision{Action: tooling.PermissionActionDeny, Reason: err.Error()}
				}
				if !terminalpolicy.IsReadOnlyCommand(req.command, req.args) {
					approval := buildRemoteExecApprovalPayload(req, host)
					return tooling.PermissionDecision{
						Action:   tooling.PermissionActionNeedApproval,
						Reason:   approval.Reason,
						Approval: approval,
					}
				}
				return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
			}
			if isAllowedExecReadOnly(opts.TerminalPolicy, req.command, req.args) {
				return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
			}
			return checkExecCommandActionToken(ctx, input, req, opts, signer)
		},
		ValidateInputFunc: func(_ context.Context, input json.RawMessage) error {
			_, err := parseCommandInput(input)
			return err
		},
		ExecuteFunc: func(ctx context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			req, err := parseCommandInput(input)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			timeout := opts.CommandTimeout
			if req.TimeoutMs > 0 {
				timeout = time.Duration(req.TimeoutMs) * time.Millisecond
				if timeout > 60*time.Second {
					timeout = 60 * time.Second
				}
			}
			if selectedRemoteHostID(ctx) != "" {
				return executeHostAgentCommand(ctx, opts, req, timeout)
			}
			runCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			cmd := exec.CommandContext(runCtx, req.command, req.args...)
			cmd.Dir = resolveWorkingDir(opts.WorkingDir, req.WorkingDir)
			cmd.Env = withLocalNoProxy(os.Environ())
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			err = cmd.Run()
			if runCtx.Err() != nil {
				return tooling.ToolResult{}, runCtx.Err()
			}
			exitCode := 0
			if cmd.ProcessState != nil {
				exitCode = cmd.ProcessState.ExitCode()
			}
			stdoutText := truncateString(stdout.String(), opts.MaxOutputBytes)
			stderrText := truncateString(stderr.String(), opts.MaxOutputBytes/2)
			if err != nil && !canReturnNonZeroExecResult(req, exitCode, err) {
				return tooling.ToolResult{}, fmt.Errorf("command failed: %w; stderr: %s", err, truncateString(stderr.String(), opts.MaxOutputBytes/2))
			}
			return execTerminalToolResult(ctx, opts, req, "terminal.break_glass", "", stdoutText, stderrText, exitCode)
		},
	}
}

func buildRemoteExecApprovalPayload(req commandInput, host *store.HostRecord) *tooling.PermissionApprovalPayload {
	hostID := "selected host"
	if host != nil && strings.TrimSpace(host.ID) != "" {
		hostID = strings.TrimSpace(host.ID)
	}
	commandText := displayCommand(req.command, req.args)
	risk := terminalpolicy.TerminalRiskLevel(req.command, req.args)
	if risk == "" || risk == "low" {
		risk = "high"
	}
	return &tooling.PermissionApprovalPayload{
		Command:        commandText,
		Reason:         fmt.Sprintf("mutating terminal command on %s requires approval", hostID),
		Risk:           risk,
		Source:         "ai_chat_direct",
		ExpectedEffect: fmt.Sprintf("Execute the requested change on %s: %s.", hostID, commandText),
		Rollback:       "If verification fails, stop further mutation and use a scoped rollback or recovery command appropriate to the changed service or configuration after fresh approval.",
		Validation:     "Run a read-only post-check for the affected service, process, port, log, or endpoint; report failure or unknown state instead of claiming completion.",
	}
}

func isAllowedExecReadOnly(provider terminalpolicy.Provider, command string, args []string) bool {
	if decision, ok := evaluateConfiguredTerminalPolicy(provider, command, args); ok {
		return decision.Action == terminalpolicy.PolicyActionAllow
	}
	return terminalpolicy.IsAllowedReadOnlyTerminal(command, args) ||
		terminalpolicy.IsAllowedHostInspectionTerminal(command, args)
}

func evaluateConfiguredTerminalPolicy(provider terminalpolicy.Provider, command string, args []string) (terminalpolicy.Decision, bool) {
	if provider == nil {
		return terminalpolicy.Decision{}, false
	}
	decision := provider.Evaluate(terminalpolicy.CommandRequest{Command: command, Args: append([]string(nil), args...)})
	return decision, decision.Action != terminalpolicy.PolicyActionDefault
}

func terminalPolicyDecisionToPermission(decision terminalpolicy.Decision, command string, args []string) tooling.PermissionDecision {
	reason := strings.TrimSpace(decision.Reason)
	if reason == "" {
		reason = strings.TrimSpace(decision.RuleID)
	}
	switch decision.Action {
	case terminalpolicy.PolicyActionAllow:
		return tooling.PermissionDecision{Action: tooling.PermissionActionAllow, Reason: reason}
	case terminalpolicy.PolicyActionDeny:
		return tooling.PermissionDecision{Action: tooling.PermissionActionDeny, Reason: firstNonEmptyString(reason, "terminal command denied by policy")}
	case terminalpolicy.PolicyActionNeedApproval:
		reason = firstNonEmptyString(reason, "terminal command requires approval by policy")
		return tooling.PermissionDecision{
			Action: tooling.PermissionActionNeedApproval,
			Reason: reason,
			Approval: &tooling.PermissionApprovalPayload{
				Command:        displayCommand(command, args),
				Reason:         reason,
				Risk:           "medium",
				Source:         "terminal_policy",
				ExpectedEffect: "execute terminal command after policy approval",
				Rollback:       "no automatic rollback for read-only inspection commands",
			},
		}
	default:
		return tooling.PermissionDecision{Action: tooling.PermissionActionNeedEvidence, Reason: firstNonEmptyString(reason, "terminal command requires policy review")}
	}
}

func selectedRemoteHostID(ctx context.Context) string {
	execCtx, ok := tooling.ToolExecutionContextFrom(ctx)
	if !ok {
		return ""
	}
	hostID := strings.TrimSpace(execCtx.HostID)
	if hostID == "" || hostID == "server-local" {
		return ""
	}
	return hostID
}

func lookupSelectedRemoteHost(ctx context.Context, repo HostRepository) (*store.HostRecord, error) {
	hostID := selectedRemoteHostID(ctx)
	if hostID == "" {
		return nil, nil
	}
	if repo == nil {
		return nil, fmt.Errorf("remote host repository is not configured")
	}
	host, err := lookupHostByInventoryIdentity(repo, hostID)
	if err != nil {
		return nil, err
	}
	if host == nil {
		return nil, fmt.Errorf("selected host %s was not found", hostID)
	}
	if !host.Executable && strings.TrimSpace(host.ControlMode) != "managed" && !hostHasSSHCommandAccess(host) {
		return nil, fmt.Errorf("selected host %s is not managed by host-agent and has no SSH command credential", hostID)
	}
	return host, nil
}

func lookupHostByInventoryIdentity(repo HostRepository, value string) (*store.HostRecord, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	host, err := repo.GetHost(value)
	if err == nil && host != nil {
		return host, nil
	}
	directErr := err
	hosts, listErr := repo.ListHosts()
	if listErr != nil {
		if directErr != nil {
			return nil, fmt.Errorf("load selected host %s: %w", value, directErr)
		}
		return nil, fmt.Errorf("list hosts while resolving selected host %s: %w", value, listErr)
	}
	if host := matchHostByInventoryIdentity(hosts, value); host != nil {
		return host, nil
	}
	if directErr != nil {
		return nil, fmt.Errorf("load selected host %s: %w", value, directErr)
	}
	return nil, nil
}

func matchHostByInventoryIdentity(hosts []store.HostRecord, value string) *store.HostRecord {
	normalized := normalizeHostInventoryIdentity(value)
	if normalized == "" {
		return nil
	}
	for _, host := range hosts {
		for _, candidate := range []string{host.ID, host.Name} {
			if normalizeHostInventoryIdentity(candidate) == normalized {
				cp := host
				return &cp
			}
		}
	}
	for _, host := range hosts {
		for _, candidate := range []string{host.Address, host.Labels["aliasOf"]} {
			if normalizeHostInventoryIdentity(candidate) == normalized {
				cp := host
				return &cp
			}
		}
	}
	return nil
}

func normalizeHostInventoryIdentity(value string) string {
	return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(value, "@")))
}

func hostHasSSHCommandAccess(host *store.HostRecord) bool {
	if host == nil {
		return false
	}
	return strings.TrimSpace(host.Address) != "" &&
		strings.TrimSpace(host.SSHUser) != "" &&
		strings.TrimSpace(host.SSHCredentialRef) != ""
}

func executeHostAgentCommand(ctx context.Context, opts Options, req commandInput, timeout time.Duration) (tooling.ToolResult, error) {
	host, err := lookupSelectedRemoteHost(ctx, opts.HostRepository)
	if err != nil {
		return tooling.ToolResult{}, err
	}
	if opts.HostAgentCommandRunner == nil {
		return tooling.ToolResult{}, fmt.Errorf("host-agent command runner is not configured")
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result, err := opts.HostAgentCommandRunner.RunHostAgentCommand(runCtx, HostAgentCommandRequest{
		HostID:         host.ID,
		Command:        req.command,
		Args:           append([]string(nil), req.args...),
		WorkingDir:     req.WorkingDir,
		Timeout:        timeout,
		MaxOutputBytes: opts.MaxOutputBytes,
	})
	if runCtx.Err() != nil {
		return tooling.ToolResult{}, runCtx.Err()
	}
	if err != nil {
		return tooling.ToolResult{}, err
	}
	stdoutText := truncateString(result.Stdout, opts.MaxOutputBytes)
	stderrText := truncateString(result.Stderr, opts.MaxOutputBytes/2)
	if result.ExitCode != 0 && !terminalpolicy.IsReadOnlyCommand(req.command, req.args) {
		return tooling.ToolResult{}, fmt.Errorf("host-agent command failed: exit status %d; stderr: %s", result.ExitCode, stderrText)
	}
	source := strings.TrimSpace(result.Source)
	if source == "" {
		source = "host.agent"
	}
	return execTerminalToolResult(ctx, opts, req, source, host.ID, stdoutText, stderrText, result.ExitCode)
}

func canReturnNonZeroExecResult(req commandInput, exitCode int, err error) bool {
	if err == nil || exitCode == 0 || !terminalpolicy.IsReadOnlyCommand(req.command, req.args) {
		return false
	}
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr)
}

func execTerminalToolResult(ctx context.Context, opts Options, req commandInput, source, hostID, stdoutText, stderrText string, exitCode int) (tooling.ToolResult, error) {
	evidenceRefs, err := recordTerminalEvidence(ctx, opts.EvidenceService, req, stdoutText, stderrText, exitCode)
	if err != nil {
		return tooling.ToolResult{}, err
	}
	status := "ok"
	if exitCode != 0 {
		status = "exit_nonzero"
	}
	payload := map[string]any{
		"schemaVersion": "aiops.terminal/v1",
		"tool":          "exec_command",
		"status":        status,
		"source":        source,
		"command":       terminalCommandString(req.command, req.args),
		"stdout":        stdoutText,
		"stderr":        stderrText,
		"exitCode":      exitCode,
		"evidenceRefs":  evidenceRefs,
	}
	if strings.TrimSpace(hostID) != "" {
		payload["hostId"] = strings.TrimSpace(hostID)
	}
	content, err := json.Marshal(payload)
	if err != nil {
		return tooling.ToolResult{}, err
	}
	return tooling.ToolResult{
		Content: string(content),
		Display: &tooling.ToolDisplayPayload{
			Type:  "terminal",
			Title: req.command,
		},
	}, nil
}

func recordTerminalEvidence(ctx context.Context, service *evidence.Service, req commandInput, stdoutText, stderrText string, exitCode int) ([]string, error) {
	if service == nil {
		return []string{}, nil
	}
	execCtx, _ := tooling.ToolExecutionContextFrom(ctx)
	summary := terminalEvidenceSummary(req.command, req.args, stdoutText, stderrText, exitCode)
	rec, err := service.Record(ctx, evidence.RecordRequest{
		IncidentID:  execCtx.IncidentID,
		SourceTool:  "exec_command",
		Source:      "terminal.break_glass",
		Kind:        terminalEvidenceKind(req.command, req.args),
		Summary:     summary,
		Data:        terminalEvidenceData(req, stdoutText, stderrText, exitCode),
		SessionID:   execCtx.SessionID,
		TurnID:      execCtx.TurnID,
		ToolCallID:  execCtx.ToolCallID,
		Environment: strings.TrimSpace(execCtx.HostID),
	})
	if err != nil {
		return nil, fmt.Errorf("record terminal evidence: %w", err)
	}
	return []string{rec.Ref}, nil
}

func terminalEvidenceSummary(command string, args []string, stdoutText, stderrText string, exitCode int) string {
	preview := strings.TrimSpace(stdoutText)
	if preview == "" {
		preview = strings.TrimSpace(stderrText)
	}
	preview = strings.Join(strings.Fields(preview), " ")
	if preview == "" {
		preview = fmt.Sprintf("exitCode=%d", exitCode)
	}
	return truncateString(fmt.Sprintf("terminal %s returned %s", terminalCommandString(command, args), preview), 300)
}

func terminalEvidenceKind(command string, args []string) evidence.Kind {
	command = strings.ToLower(strings.TrimSpace(command))
	joined := strings.ToLower(strings.Join(args, " "))
	switch {
	case command == "kubectl" && (strings.Contains(joined, " logs") || strings.HasPrefix(joined, "logs")):
		return evidence.KindLog
	case command == "kubectl" && strings.Contains(joined, "events"):
		return evidence.KindEvent
	case command == "redis-cli":
		return evidence.KindMetric
	case command == "curl":
		return evidence.KindOther
	default:
		return evidence.KindOther
	}
}

func terminalEvidenceData(req commandInput, stdoutText, stderrText string, exitCode int) map[string]any {
	return map[string]any{
		"command":    terminalCommandString(req.command, req.args),
		"executable": req.command,
		"args":       append([]string(nil), req.args...),
		"workingDir": req.WorkingDir,
		"stdout":     stdoutText,
		"stderr":     stderrText,
		"exitCode":   exitCode,
	}
}

func terminalCommandString(command string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	if strings.TrimSpace(command) != "" {
		parts = append(parts, strings.TrimSpace(command))
	}
	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if arg != "" {
			parts = append(parts, arg)
		}
	}
	return strings.Join(parts, " ")
}

func execCommandDescription() string {
	description := "Execute a terminal command on the selected host. For server-local this runs locally in the ai-server environment; for managed remote hosts this sends read-only commands to the selected host-agent over gRPC/HTTP, and for inventory hosts with stored SSH credentials the runtime may use a read-only SSH fallback through the same exec_command tool. Prefer explicit command + args. For read-only inspection, do not wrap commands in sh/bash/zsh -c and do not use pipes, redirection, or command chaining; use narrower commands or native flags instead. Read-only inspection commands, including safe curl GET/HEAD requests, are allowed in chat; for HTTP status checks use curl -fsS -o /dev/null -w %{http_code} URL or curl -fsSI URL, and do not use -o %{http_code}. Mutation commands must go through the runtime approval gate, so call the scoped command instead of asking for prose approval. Host OS: " + runtime.GOOS + " for server-local."
	switch runtime.GOOS {
	case "darwin":
		return description + " For host resource inspection on macOS, prefer uptime, sysctl -n hw.ncpu, vm_stat, df -h, and top -l 1 -s 0; avoid Linux-only commands such as lscpu, nproc, free -h, and /proc/*."
	case "linux":
		return description + " For host resource inspection on Linux, prefer uptime, nproc, free -h, df -hT -x tmpfs -x devtmpfs, and cat /proc/loadavg. If free is unavailable, use cat /proc/meminfo without pipes and summarize MemTotal, MemFree, MemAvailable, SwapTotal, and SwapFree from the output."
	default:
		return description + " Choose commands compatible with this OS; avoid Linux-only commands unless Host OS is linux."
	}
}

// NewBrowseURLTool returns a compatibility alias for web_search operation=open.
func NewBrowseURLTool(opts Options) tooling.Tool {
	opts = opts.normalize()
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:           "browse_url",
			Aliases:        []string{"web_browser", "web_fetch", "fetch_url", "open_url"},
			Origin:         tooling.ToolOriginBuiltin,
			Description:    "Compatibility alias for web_search with operation=open. Prefer web_search directly for both public web search and reading a known public http(s) URL.",
			Layer:          tooling.ToolLayerDeferred,
			Pack:           "public_web",
			DeferByDefault: true,
			SearchHint:     "fetch browse open public web url page",
			Discovery: tooling.ToolDiscoveryMetadata{
				DiscoveryGroup:    "public_web",
				DiscoveryTags:     []string{"official_docs", "version_match", "applicability", "external_knowledge"},
				CapabilityKind:    "web",
				ResourceTypes:     []string{"url", "web_page", "public_web"},
				OperationKinds:    []string{"read", "fetch"},
				RequiresSelect:    true,
				PermissionScope:   "read",
				PromptBudgetClass: "compact",
				SchemaBudgetClass: "on_demand",
				HiddenFromPrompt:  true,
			},
			ResultBudget: tooling.ResultBudget{
				MaxInlineResultBytes: opts.MaxOutputBytes,
				SpillPolicy:          tooling.ResultSpillPolicySummaryInline,
				SummarizeLargeResult: true,
			},
		},
		Visibility: tooling.Visibility{SessionTypes: []string{"host", "workspace"}},
		InputSchemaData: json.RawMessage(`{
			"type": "object",
			"properties": {
				"url": {"type": "string", "description": "Absolute http(s) URL to fetch."},
				"maxBytes": {"type": "integer", "description": "Optional maximum response bytes, capped by server policy."}
			},
			"required": ["url"]
		}`),
		OutputSchemaData: json.RawMessage(`{
			"type": "object",
			"properties": {
				"url": {"type": "string"},
				"contentType": {"type": "string"},
				"text": {"type": "string"}
			}
		}`),
		ReadOnlyFunc:        func(json.RawMessage) bool { return true },
		ConcurrencySafeFunc: func(json.RawMessage) bool { return true },
		ValidateInputFunc: func(_ context.Context, input json.RawMessage) error {
			_, err := parseBrowseURLInput(input)
			return err
		},
		ExecuteFunc: func(ctx context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			req, err := parseBrowseURLInput(input)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			raw := map[string]any{
				"operation": "open",
				"url":       req.URL,
				"max_bytes": boundedMaxBytes(req.MaxBytes, opts.MaxOutputBytes),
			}
			data, _ := json.Marshal(raw)
			webTool := NewWebSearchTool(nil, opts)
			return webTool.Execute(ctx, data)
		},
	}
}

// NewWebSearchTool returns a search broker that delegates to the configured
// provider's native web_search tool when available.
func NewWebSearchTool(repo LLMConfigRepository, opts Options) tooling.Tool {
	opts = opts.normalize()
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "web_search",
			Aliases:     []string{"search_web"},
			Origin:      tooling.ToolOriginBuiltin,
			Description: "Search the public web or read a specific public http(s) URL with one tool. Use operation=search for public/current internet facts and operation=open when a known result URL needs readable page text. Use this for public/current internet facts, not for current host, selected resource, private environment, local runtime, prompt trace, tool status, or deployment facts; those require environment-bound tools such as exec_command or the relevant observability tool. Use precise, self-contained queries. For current or latest public information, include the current date or target date, key entities, and the data you need. Prefer authoritative sources and cite source URLs. For realtime price or market quote questions, do not stop after one unreadable or dynamic page; try another authoritative source or a more specific official/API query until you can cross-check the current numeric value, timestamp, and quote currency. Use allowed_domains or blocked_domains when source control is needed. Avoid vague one-word queries; if results are weak or irrelevant, refine the query with source names, official domains, or site: filters.",
			Layer:       tooling.ToolLayerCore,
			Pack:        "public_web",
			AlwaysLoad:  true,
			SearchHint:  "search public web current internet latest authoritative source",
			Discovery: tooling.ToolDiscoveryMetadata{
				DiscoveryGroup:    "public_web",
				DiscoveryTags:     []string{"official_docs", "version_match", "applicability", "external_knowledge"},
				CapabilityKind:    "web",
				ResourceTypes:     []string{"public_web", "internet"},
				OperationKinds:    []string{"search", "read"},
				PermissionScope:   "read",
				PromptBudgetClass: "compact",
				SchemaBudgetClass: "on_demand",
			},
			ProviderNative: &tooling.ProviderNativeToolInfo{
				Provider: "openai",
				Type:     "web_search",
				Prefer:   true,
			},
			ResultBudget: tooling.ResultBudget{
				MaxInlineResultBytes: opts.MaxOutputBytes,
				SpillPolicy:          tooling.ResultSpillPolicySummaryInline,
				SummarizeLargeResult: true,
			},
		},
		Visibility: tooling.Visibility{SessionTypes: []string{"host", "workspace"}},
		InputSchemaData: json.RawMessage(`{
			"type": "object",
			"properties": {
				"operation": {"type": "string", "enum": ["search", "open"], "description": "Use search for query search or open for reading a specific public http(s) URL."},
				"query": {"type": "string", "description": "Precise search query. For current/latest information include the current date or target date, key entities, and the desired data. Avoid vague queries."},
				"url": {"type": "string", "description": "Public http(s) URL to read when operation=open."},
				"search_context_size": {"type": "string", "enum": ["low", "medium", "high"], "description": "Provider-native search context size."},
				"allowed_domains": {"type": "array", "items": {"type": "string"}, "description": "Optional authoritative domains to restrict public fallback results, for example sse.com.cn. Do not combine with blocked_domains."},
				"blocked_domains": {"type": "array", "items": {"type": "string"}, "description": "Optional domains to exclude from public fallback results. Do not combine with allowed_domains."},
				"limit": {"type": "integer", "description": "Maximum search results returned after filtering."},
				"max_results": {"type": "integer", "description": "Compatibility alias for limit."},
				"fetch_content": {"type": "boolean", "description": "When true, fetch bounded readable text for the first matching search results."},
				"max_content_results": {"type": "integer", "description": "Maximum number of search results to fetch for content."},
				"max_bytes": {"type": "integer", "description": "Maximum inline bytes for opened or fetched page text."}
			}
		}`),
		OutputSchemaData: json.RawMessage(`{
			"type": "object",
			"properties": {
				"operation": {"type": "string"},
				"query": {"type": "string"},
				"url": {"type": "string"},
				"source": {"type": "string"},
				"content": {"type": "string"},
				"results": {"type": "array", "items": {"type": "object"}},
				"meta": {"type": "object"}
			}
		}`),
		ReadOnlyFunc:        func(json.RawMessage) bool { return true },
		ConcurrencySafeFunc: func(json.RawMessage) bool { return true },
		ValidateInputFunc: func(_ context.Context, input json.RawMessage) error {
			_, err := publicweb.ParseRequest(input)
			return err
		},
		ExecuteFunc: func(ctx context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			req, err := publicweb.ParseRequest(input)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			cfg := currentConfig(repo)
			client := opts.HTTPClient
			if client == nil {
				client = &http.Client{Timeout: opts.WebTimeout}
			}
			if req.Operation == publicweb.OperationSearch && providerSupportsNativeWebSearch(cfg.Provider, cfg.Model) && strings.TrimSpace(cfg.APIKey) != "" {
				legacyReq := webSearchInput{
					Query:             req.Query,
					SearchContextSize: req.SearchContextSize,
					AllowedDomains:    req.AllowedDomains,
					BlockedDomains:    req.BlockedDomains,
				}
				content, source, err := runProviderNativeWebSearch(ctx, client, cfg, legacyReq, opts)
				if err == nil {
					return webSearchProviderNativeToolResult(req, content, source, opts), nil
				}
			}
			broker := publicweb.NewBroker(
				publicweb.NewLightweightBackend(client, opts.PublicSearchBaseURL),
				publicweb.NewSafeFetcher(client),
			)
			env, err := broker.Execute(ctx, req)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			return publicWebToolResult(env, opts), nil
		},
	}
}

func webSearchProviderNativeToolResult(req publicweb.SearchRequest, content, source string, opts Options) tooling.ToolResult {
	return publicWebToolResult(publicweb.FormatProviderNativeEnvelope(req, content, source), opts)
}

func publicWebToolResult(env publicweb.ResultEnvelope, opts Options) tooling.ToolResult {
	env.Content = truncateString(env.Content, opts.MaxOutputBytes)
	for i := range env.Results {
		env.Results[i].Text = truncateString(env.Results[i].Text, opts.MaxOutputBytes)
		env.Results[i].Markdown = truncateString(env.Results[i].Markdown, opts.MaxOutputBytes)
		env.Results[i].Snippet = truncateString(env.Results[i].Snippet, 1200)
	}
	data, _ := json.Marshal(env)
	return tooling.ToolResult{
		Content: truncateString(string(data), opts.MaxOutputBytes),
		Display: &tooling.ToolDisplayPayload{
			Type:  "web_search",
			Title: firstNonEmptyString(env.Query, env.URL),
		},
	}
}

func webSearchToolResult(req webSearchInput, content, source string, opts Options) tooling.ToolResult {
	payload := map[string]string{
		"query":   req.Query,
		"source":  source,
		"content": truncateString(content, opts.MaxOutputBytes),
	}
	data, _ := json.Marshal(payload)
	return tooling.ToolResult{
		Content: string(data),
		Display: &tooling.ToolDisplayPayload{
			Type:  "web_search",
			Title: req.Query,
		},
	}
}

type commandInput struct {
	Command     string   `json:"command"`
	Args        []string `json:"args"`
	Cmd         string   `json:"cmd"`
	WorkingDir  string   `json:"workingDir"`
	TimeoutMs   int      `json:"timeoutMs"`
	ActionToken string   `json:"actionToken"`
	Intent      string   `json:"intent"`

	command string
	args    []string
}

func checkExecCommandActionToken(ctx context.Context, input json.RawMessage, req commandInput, opts Options, signer *actionproposal.Signer) tooling.PermissionDecision {
	if isForbiddenExecCommand(req.command, req.args) {
		return tooling.PermissionDecision{
			Action: tooling.PermissionActionDeny,
			Reason: "forbidden terminal command is blocked by policy",
		}
	}
	token := strings.TrimSpace(req.ActionToken)
	execCtx, hasExecCtx := tooling.ToolExecutionContextFrom(ctx)
	if hasExecCtx && strings.TrimSpace(execCtx.ActionToken) != "" {
		token = strings.TrimSpace(execCtx.ActionToken)
	}
	if token == "" {
		return tooling.PermissionDecision{
			Action: tooling.PermissionActionNeedEvidence,
			Reason: "non-read-only terminal command requires a signed ActionToken from runbook, fallback, or break_glass",
		}
	}
	hashInput := input
	if hasExecCtx && len(execCtx.SanitizedInput) > 0 {
		hashInput = execCtx.SanitizedInput
	}
	inputHash, err := actionproposal.NormalizedInputHash(hashInput)
	if err != nil {
		return tooling.PermissionDecision{
			Action: tooling.PermissionActionNeedEvidence,
			Reason: "unable to normalize terminal command for ActionToken verification: " + err.Error(),
		}
	}
	expected := actionproposal.ActionTokenClaims{
		ToolName:  "exec_command",
		InputHash: inputHash,
	}
	if hasExecCtx {
		expected.SessionID = execCtx.SessionID
		expected.TurnID = execCtx.TurnID
		expected.TenantID = execCtx.TenantID
		expected.UserID = execCtx.UserID
		expected.IncidentID = execCtx.IncidentID
	}
	claims, err := signer.Verify(token, expected)
	if err != nil {
		return tooling.PermissionDecision{
			Action: tooling.PermissionActionNeedEvidence,
			Reason: "invalid ActionToken for terminal command: " + err.Error(),
		}
	}
	if !isAllowedActionTokenSource(claims.Source) {
		return tooling.PermissionDecision{
			Action: tooling.PermissionActionDeny,
			Reason: "ActionToken source is not allowed for terminal command",
		}
	}
	if execRiskRank(claims.Risk) == 0 {
		return tooling.PermissionDecision{
			Action: tooling.PermissionActionNeedEvidence,
			Reason: "ActionToken risk is required for terminal command",
		}
	}
	if claims.Risk == actionproposal.RiskLow && !opts.RequireApprovalForLowRiskExec && !terminalpolicy.RequiresHighRiskApproval(req.command, req.args) {
		return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
	}
	approval := buildExecApprovalPayload(req, claims)
	return tooling.PermissionDecision{
		Action:   tooling.PermissionActionNeedApproval,
		Reason:   approvalReason(approval),
		Approval: approval,
	}
}

func isAllowedActionTokenSource(source actionproposal.Source) bool {
	switch source {
	case actionproposal.SourceRunbook, actionproposal.SourceFallback, actionproposal.SourceBreakGlass:
		return true
	default:
		return false
	}
}

func buildExecApprovalPayload(req commandInput, claims actionproposal.ActionTokenClaims) *tooling.PermissionApprovalPayload {
	reason := strings.TrimSpace(claims.Reason)
	if reason == "" {
		reason = "local terminal command requires approval"
	}
	runbookStep := strings.TrimSpace(claims.RunbookStepID)
	if strings.TrimSpace(claims.RunbookStepTitle) != "" {
		if runbookStep != "" {
			runbookStep += " · "
		}
		runbookStep += strings.TrimSpace(claims.RunbookStepTitle)
	}
	return &tooling.PermissionApprovalPayload{
		Command:        displayCommand(req.command, req.args),
		Reason:         reason,
		Risk:           string(claims.Risk),
		Source:         string(claims.Source),
		RunbookID:      claims.RunbookID,
		RunbookStep:    runbookStep,
		ExpectedEffect: strings.TrimSpace(claims.ExpectedEffect),
		Rollback:       strings.TrimSpace(claims.Rollback),
	}
}

func approvalReason(payload *tooling.PermissionApprovalPayload) string {
	if payload == nil {
		return "local terminal command requires approval"
	}
	parts := []string{
		firstNonEmptyString(payload.Reason, "local terminal command requires approval"),
		"command=" + payload.Command,
		"risk=" + payload.Risk,
		"source=" + payload.Source,
	}
	if payload.RunbookStep != "" {
		parts = append(parts, "runbookStep="+payload.RunbookStep)
	}
	if payload.ExpectedEffect != "" {
		parts = append(parts, "expectedEffect="+payload.ExpectedEffect)
	}
	if payload.Rollback != "" {
		parts = append(parts, "rollback="+payload.Rollback)
	}
	return strings.Join(parts, "; ")
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func displayCommand(command string, args []string) string {
	parts := []string{strings.TrimSpace(command)}
	for _, arg := range args {
		parts = append(parts, strings.TrimSpace(arg))
	}
	return strings.Join(parts, " ")
}

func isForbiddenExecCommand(command string, args []string) bool {
	return terminalpolicy.IsHardDeniedCommand(command, args)
}

func unwrapShellCommand(base string, args []string) (string, []string, bool) {
	switch base {
	case "bash", "sh", "zsh":
	default:
		return "", nil, false
	}
	if len(args) != 2 {
		return "", nil, false
	}
	switch strings.TrimSpace(args[0]) {
	case "-c", "-lc":
	default:
		return "", nil, false
	}
	command, commandArgs, ok := terminalpolicy.SplitCommandLine(args[1])
	if !ok {
		return "", nil, false
	}
	return command, commandArgs, true
}

func execRiskRank(risk actionproposal.Risk) int {
	switch risk {
	case actionproposal.RiskLow:
		return 1
	case actionproposal.RiskMedium:
		return 2
	case actionproposal.RiskHigh:
		return 3
	case actionproposal.RiskCritical:
		return 4
	default:
		return 0
	}
}

func parseCommandInput(input json.RawMessage) (commandInput, error) {
	var req commandInput
	if len(input) > 0 {
		if err := json.Unmarshal(input, &req); err != nil {
			return commandInput{}, fmt.Errorf("invalid command input: %w", err)
		}
	}
	command := strings.TrimSpace(req.Command)
	args := append([]string(nil), req.Args...)
	if command != "" && len(args) == 0 {
		parsedCommand, parsedArgs, ok := terminalpolicy.SplitCommandLine(command)
		if !ok {
			return commandInput{}, errors.New("shell operators are not allowed; pass command and args explicitly")
		}
		command = parsedCommand
		args = parsedArgs
	}
	if command == "" && strings.TrimSpace(req.Cmd) != "" {
		parsedCommand, parsedArgs, ok := terminalpolicy.SplitCommandLine(req.Cmd)
		if !ok {
			return commandInput{}, errors.New("shell operators are not allowed; pass command and args explicitly")
		}
		command = parsedCommand
		args = parsedArgs
	}
	if command == "" {
		return commandInput{}, errors.New("command is required")
	}
	if hasShellOperators(command) {
		return commandInput{}, errors.New("command contains unsupported shell syntax")
	}
	for _, arg := range args {
		if strings.ContainsAny(arg, "\x00\n\r") {
			return commandInput{}, errors.New("arguments cannot contain control characters")
		}
	}
	req.command = command
	req.args = args
	return req, nil
}

func hasShellOperators(command string) bool {
	return strings.ContainsAny(command, ";&|<>`$")
}

func resolveWorkingDir(defaultDir, requested string) string {
	if trimmed := strings.TrimSpace(requested); trimmed != "" {
		return trimmed
	}
	if trimmed := strings.TrimSpace(defaultDir); trimmed != "" {
		return trimmed
	}
	return "."
}

func withLocalNoProxy(env []string) []string {
	const localNoProxy = "localhost,127.0.0.1,::1"
	next := append([]string(nil), env...)
	seenUpper := false
	seenLower := false
	for i, entry := range next {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		switch key {
		case "NO_PROXY":
			seenUpper = true
			if !noProxyHasLocal(value) {
				next[i] = key + "=" + appendNoProxy(value, localNoProxy)
			}
		case "no_proxy":
			seenLower = true
			if !noProxyHasLocal(value) {
				next[i] = key + "=" + appendNoProxy(value, localNoProxy)
			}
		}
	}
	if !seenUpper {
		next = append(next, "NO_PROXY="+localNoProxy)
	}
	if !seenLower {
		next = append(next, "no_proxy="+localNoProxy)
	}
	return next
}

func appendNoProxy(value, suffix string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return suffix
	}
	return value + "," + suffix
}

func noProxyHasLocal(value string) bool {
	for _, item := range strings.Split(value, ",") {
		switch strings.TrimSpace(item) {
		case "*", "localhost", "127.0.0.1", "::1":
			return true
		}
	}
	return false
}

type webSearchInput struct {
	Query             string   `json:"query"`
	SearchContextSize string   `json:"search_context_size"`
	AllowedDomains    []string `json:"allowed_domains"`
	BlockedDomains    []string `json:"blocked_domains"`
}

type browseURLInput struct {
	URL      string `json:"url"`
	MaxBytes int    `json:"maxBytes"`
}

func parseBrowseURLInput(input json.RawMessage) (browseURLInput, error) {
	var req browseURLInput
	if err := json.Unmarshal(input, &req); err != nil {
		return browseURLInput{}, fmt.Errorf("invalid browse_url input: %w", err)
	}
	req.URL = strings.TrimSpace(req.URL)
	parsed, err := url.Parse(req.URL)
	if err != nil || parsed == nil || parsed.Host == "" {
		return browseURLInput{}, fmt.Errorf("invalid url %q", req.URL)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return browseURLInput{}, fmt.Errorf("browse_url only supports http(s), got %q", parsed.Scheme)
	}
	return req, nil
}

func boundedMaxBytes(requested, fallback int) int {
	if fallback <= 0 {
		fallback = defaultMaxOutputBytes
	}
	if requested <= 0 || requested > fallback {
		return fallback
	}
	return requested
}

func currentConfig(repo LLMConfigRepository) store.LLMConfig {
	cfg := store.LLMConfig{
		Provider:        "openai",
		Model:           "gpt-5.4",
		CompactModel:    "gpt-5.4-mini",
		ReasoningEffort: "medium",
	}
	if repo == nil {
		return cfg
	}
	stored, err := repo.GetLLMConfig()
	if err != nil || stored == nil {
		return cfg
	}
	if strings.TrimSpace(stored.Provider) != "" {
		cfg.Provider = strings.TrimSpace(stored.Provider)
	}
	if strings.TrimSpace(stored.Model) != "" {
		cfg.Model = strings.TrimSpace(stored.Model)
	}
	cfg.BaseURL = strings.TrimSpace(stored.BaseURL)
	cfg.APIKey = strings.TrimSpace(stored.APIKey)
	cfg.FallbackProvider = strings.TrimSpace(stored.FallbackProvider)
	cfg.FallbackModel = strings.TrimSpace(stored.FallbackModel)
	cfg.CompactModel = strings.TrimSpace(stored.CompactModel)
	cfg.ReasoningEffort = normalizeCurrentConfigReasoningEffort(stored.ReasoningEffort)
	return cfg
}

func normalizeCurrentConfigReasoningEffort(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "low":
		return "low"
	case "high":
		return "high"
	case "medium":
		return "medium"
	default:
		return "medium"
	}
}

func providerSupportsReasoning(provider, model string) bool {
	provider = strings.ToLower(strings.TrimSpace(provider))
	model = strings.ToLower(strings.TrimSpace(model))
	switch provider {
	case "openai":
		return strings.HasPrefix(model, "gpt-5") || strings.Contains(model, "o") || isGLM47OpenAICompatibleModel(model)
	case "anthropic":
		return strings.Contains(model, "sonnet") || strings.Contains(model, "opus")
	default:
		return false
	}
}

func isGLM47OpenAICompatibleModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	return model == "glm-4.7" ||
		strings.HasPrefix(model, "glm-4.7-") ||
		strings.Contains(model, "/glm-4.7")
}

func providerSupportsNativeWebSearch(provider, model string) bool {
	provider = strings.ToLower(strings.TrimSpace(provider))
	model = strings.ToLower(strings.TrimSpace(model))
	if provider != "openai" {
		return false
	}
	return strings.HasPrefix(model, "gpt-")
}

func runProviderNativeWebSearch(ctx context.Context, client *http.Client, cfg store.LLMConfig, req webSearchInput, opts Options) (string, string, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = "gpt-5.4"
	}
	responsesContent, responsesErr := callResponsesWebSearch(ctx, client, baseURL, cfg.APIKey, model, req)
	if responsesErr == nil && strings.TrimSpace(responsesContent) != "" {
		return responsesContent, "provider_native:responses:web_search", nil
	}
	chatContent, chatErr := callChatCompletionsWebSearch(ctx, client, baseURL, cfg.APIKey, model, req)
	if chatErr == nil && strings.TrimSpace(chatContent) != "" {
		return chatContent, "provider_native:chat_completions:web_search_options", nil
	}
	if errors.Is(responsesErr, errProviderWebSearchNoText) {
		if publicContent, publicSource, publicErr := runCustomPublicWebSearch(ctx, client, req, opts); publicErr == nil && strings.TrimSpace(publicContent) != "" {
			return publicContent, "provider_native:responses:web_search+" + publicSource, nil
		}
		return providerNativeWebSearchNoSummary(req.Query), "provider_native:responses:web_search", nil
	}
	if responsesErr != nil {
		return "", "", responsesErr
	}
	return "", "", chatErr
}

var errProviderWebSearchNoText = errors.New("provider-native web_search returned no textual summary")

func callResponsesWebSearch(ctx context.Context, client *http.Client, baseURL, apiKey, model string, req webSearchInput) (string, error) {
	payload := map[string]any{
		"model": model,
		"tools": []map[string]any{
			{
				"type":                "web_search",
				"search_context_size": req.SearchContextSize,
			},
		},
		"include": []string{"web_search_call.action.sources"},
		"input":   providerWebSearchQuery(req),
	}
	data, _ := json.Marshal(payload)
	body, err := postProviderJSON(ctx, client, baseURL+"/responses", apiKey, data)
	if err != nil {
		return "", err
	}
	text := extractResponsesText(body)
	if strings.TrimSpace(text) == "" {
		if sources := extractResponsesSources(body); strings.TrimSpace(sources) != "" {
			return sources, nil
		}
		if responsesUsedWebSearch(body) {
			return "", errProviderWebSearchNoText
		}
		return "", errProviderWebSearchNoText
	}
	return text, nil
}

func providerNativeWebSearchNoSummary(query string) string {
	return fmt.Sprintf("provider-native web_search completed for query %q; provider returned no textual summary and public fallback found no relevant result. Do not repeat this exact query; refine with more specific entities, dates, or authoritative domains, or answer with explicit limitations if evidence is sufficient.", query)
}

func runCustomPublicWebSearch(ctx context.Context, client *http.Client, req webSearchInput, opts Options) (string, string, error) {
	broker := publicweb.NewBroker(
		publicweb.NewLightweightBackend(client, opts.PublicSearchBaseURL),
		publicweb.NewSafeFetcher(client),
	)
	env, err := broker.Execute(ctx, publicweb.SearchRequest{
		Operation:         publicweb.OperationSearch,
		Query:             req.Query,
		SearchContextSize: req.SearchContextSize,
		AllowedDomains:    req.AllowedDomains,
		BlockedDomains:    req.BlockedDomains,
		Limit:             publicweb.DefaultLimit,
		MaxContentResults: publicweb.DefaultMaxContentResults,
		MaxBytes:          boundedMaxBytes(0, opts.MaxOutputBytes),
		Timeout:           opts.WebTimeout,
	})
	if err != nil {
		return "", "", err
	}
	return env.Content, env.Source, nil
}

func providerWebSearchQuery(req webSearchInput) string {
	query := strings.TrimSpace(req.Query)
	if len(req.AllowedDomains) > 0 {
		query += "\nRestrict sources to these domains: " + strings.Join(req.AllowedDomains, ", ")
	}
	if len(req.BlockedDomains) > 0 {
		query += "\nExclude sources from these domains: " + strings.Join(req.BlockedDomains, ", ")
	}
	return query
}

func callChatCompletionsWebSearch(ctx context.Context, client *http.Client, baseURL, apiKey, model string, req webSearchInput) (string, error) {
	payload := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": providerWebSearchQuery(req)},
		},
		"web_search_options": map[string]any{},
	}
	data, _ := json.Marshal(payload)
	body, err := postProviderJSON(ctx, client, baseURL+"/chat/completions", apiKey, data)
	if err != nil {
		return "", err
	}
	text := extractChatCompletionText(body)
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("web_search chat completions returned no text")
	}
	return text, nil
}

func postProviderJSON(ctx context.Context, client *http.Client, url, apiKey string, data []byte) ([]byte, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("provider request %s failed: status %d: %s", url, resp.StatusCode, truncateString(string(body), 1000))
	}
	return body, nil
}

func extractResponsesText(body []byte) string {
	var payload struct {
		OutputText string `json:"output_text"`
		Output     []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	if strings.TrimSpace(payload.OutputText) != "" {
		return payload.OutputText
	}
	var parts []string
	for _, item := range payload.Output {
		if item.Type != "" && item.Type != "message" {
			continue
		}
		for _, content := range item.Content {
			if strings.TrimSpace(content.Text) != "" {
				parts = append(parts, content.Text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func extractResponsesSources(body []byte) string {
	var payload struct {
		Output []struct {
			Type   string `json:"type"`
			Action struct {
				Sources []struct {
					URL   string `json:"url"`
					Title string `json:"title"`
				} `json:"sources"`
			} `json:"action"`
		} `json:"output"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	var parts []string
	for _, item := range payload.Output {
		if item.Type != "web_search_call" {
			continue
		}
		for _, source := range item.Action.Sources {
			url := strings.TrimSpace(source.URL)
			if url == "" {
				continue
			}
			title := strings.TrimSpace(source.Title)
			if title == "" {
				parts = append(parts, url)
			} else {
				parts = append(parts, fmt.Sprintf("%s: %s", title, url))
			}
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "Provider-native web_search sources:\n- " + strings.Join(parts, "\n- ")
}

func responsesUsedWebSearch(body []byte) bool {
	var payload struct {
		ToolUsage struct {
			WebSearch struct {
				NumRequests int `json:"num_requests"`
			} `json:"web_search"`
		} `json:"tool_usage"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return false
	}
	return payload.ToolUsage.WebSearch.NumRequests > 0
}

func extractChatCompletionText(body []byte) string {
	var payload struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	if len(payload.Choices) == 0 {
		return ""
	}
	return payload.Choices[0].Message.Content
}

func truncateString(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	if maxBytes <= 3 {
		return utf8PrefixWithinBytes(value, maxBytes)
	}
	return utf8PrefixWithinBytes(value, maxBytes-3) + "..."
}

func utf8PrefixWithinBytes(value string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	end := 0
	for end < len(value) {
		_, size := utf8.DecodeRuneInString(value[end:])
		if size == 0 || end+size > maxBytes {
			break
		}
		end += size
	}
	return value[:end]
}
