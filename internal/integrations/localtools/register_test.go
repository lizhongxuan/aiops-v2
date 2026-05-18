package localtools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"aiops-v2/internal/actionproposal"
	"aiops-v2/internal/evidence"
	"aiops-v2/internal/store"
	"aiops-v2/internal/tooling"
	"pgregory.net/rapid"
)

type fakeLLMRepo struct {
	cfg *store.LLMConfig
}

func (r *fakeLLMRepo) GetLLMConfig() (*store.LLMConfig, error) {
	return r.cfg, nil
}

func TestRegisterBuiltinsExposesChatToolsWithoutInternalPlanTool(t *testing.T) {
	registry := tooling.NewRegistry()
	repo := &fakeLLMRepo{cfg: &store.LLMConfig{
		Provider: "openai",
		Model:    "gpt-5.4",
		BaseURL:  "http://127.0.0.1:8317/v1",
		APIKey:   "secret-key",
	}}

	if err := RegisterBuiltins(registry, repo, Options{WorkingDir: t.TempDir()}); err != nil {
		t.Fatalf("RegisterBuiltins() error = %v", err)
	}

	tools := registry.AssembleTools("host", "chat")
	names := make(map[string]tooling.Tool)
	for _, tool := range tools {
		names[tool.Metadata().Name] = tool
	}
	for _, name := range []string{"web_search", "browse_url", "exec_command", "get_current_model_config"} {
		if _, ok := names[name]; !ok {
			t.Fatalf("assembled tools missing %q; got %v", name, toolNames(tools))
		}
	}
	if _, ok := names["update_plan"]; ok {
		t.Fatalf("update_plan should be internal/meta-only in default chat tools; got %v", toolNames(tools))
	}
	if native := names["web_search"].Metadata().ProviderNative; native == nil || !native.Prefer || native.Type != "web_search" {
		t.Fatalf("web_search provider-native metadata = %#v, want preferred web_search", native)
	}
}

func TestExecCommandToolDescriptionIncludesHostOSGuidance(t *testing.T) {
	tool := NewExecCommandTool(Options{WorkingDir: t.TempDir()})
	description := tool.Metadata().Description

	if !strings.Contains(description, "Host OS: "+runtime.GOOS) {
		t.Fatalf("description = %q, want current host OS guidance", description)
	}
	if runtime.GOOS == "darwin" && !strings.Contains(description, "vm_stat") {
		t.Fatalf("description = %q, want macOS resource inspection guidance", description)
	}
	if runtime.GOOS == "darwin" && !strings.Contains(description, "avoid Linux-only commands") {
		t.Fatalf("description = %q, want explicit Linux-only command avoidance on darwin", description)
	}
}

func TestExecCommandToolSchemaIncludesActionTokenAndIntent(t *testing.T) {
	tool := NewExecCommandTool(Options{WorkingDir: t.TempDir()})
	schema := string(tool.InputSchema())

	for _, field := range []string{`"actionToken"`, `"intent"`, `"cmd"`} {
		if !strings.Contains(schema, field) {
			t.Fatalf("schema missing %s: %s", field, schema)
		}
	}
}

func TestCurrentModelConfigToolDoesNotLeakSecrets(t *testing.T) {
	tool := NewCurrentModelConfigTool(&fakeLLMRepo{cfg: &store.LLMConfig{
		Provider: "openai",
		Model:    "gpt-5.4",
		BaseURL:  "http://127.0.0.1:8317/v1",
		APIKey:   "sk-secret",
	}})

	result, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if strings.Contains(result.Content, "sk-secret") {
		t.Fatalf("tool leaked api key in result: %s", result.Content)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("result is not json: %v", err)
	}
	if payload["model"] != "gpt-5.4" {
		t.Fatalf("model = %v, want gpt-5.4", payload["model"])
	}
	if payload["apiKeySet"] != true {
		t.Fatalf("apiKeySet = %v, want true", payload["apiKeySet"])
	}
}

func TestExecCommandToolAllowsSafeReadCommand(t *testing.T) {
	tool := NewExecCommandTool(Options{WorkingDir: t.TempDir()})
	input := json.RawMessage(`{"command":"kubectl","args":["get","events","-n","prod"]}`)

	if !tool.IsReadOnly(input) {
		t.Fatal("allowlisted kubectl get events should be classified read-only")
	}
	decision := tool.CheckPermissions(context.Background(), input)
	if decision.Action != tooling.PermissionActionAllow {
		t.Fatalf("CheckPermissions() = %#v, want allow", decision)
	}
}

func TestExecCommandToolAllowsSafeCurlGet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %q, want GET", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","source":"local-fixture"}`))
	}))
	defer server.Close()

	tool := NewExecCommandTool(Options{WorkingDir: t.TempDir()})
	input := json.RawMessage(`{"command":"curl","args":["-sS","--max-time","5","` + server.URL + `"]}`)

	if !tool.IsReadOnly(input) {
		t.Fatal("safe curl GET command should be classified read-only")
	}
	decision := tool.CheckPermissions(context.Background(), input)
	if decision.Action != tooling.PermissionActionAllow {
		t.Fatalf("CheckPermissions() = %#v, want allow", decision)
	}
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stdout := execCommandStdout(t, result.Content); !strings.Contains(stdout, `"status":"ok"`) {
		t.Fatalf("stdout = %q, want curl response body", stdout)
	}
}

func TestExecCommandToolRecordsTerminalEvidenceRef(t *testing.T) {
	service := evidence.NewService(evidence.NewInMemoryStore(), func() time.Time {
		return time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC)
	})
	tool := NewExecCommandTool(Options{WorkingDir: t.TempDir(), EvidenceService: service})
	ctx := tooling.ContextWithToolExecution(context.Background(), tooling.ToolExecutionContext{
		SessionID:  "sess-terminal-evidence",
		TurnID:     "turn-terminal-evidence",
		ToolCallID: "call-terminal-evidence",
		HostID:     "server-local",
	})
	input := json.RawMessage(`{"command":"printf","args":["ok"]}`)

	result, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var payload struct {
		SchemaVersion string   `json:"schemaVersion"`
		Tool          string   `json:"tool"`
		Status        string   `json:"status"`
		Stdout        string   `json:"stdout"`
		EvidenceRefs  []string `json:"evidenceRefs"`
	}
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("result is not terminal envelope JSON: %v\n%s", err, result.Content)
	}
	if payload.SchemaVersion != "aiops.terminal/v1" || payload.Tool != "exec_command" || payload.Status != "ok" {
		t.Fatalf("payload = %#v, want terminal envelope", payload)
	}
	if !strings.Contains(payload.Stdout, "ok") {
		t.Fatalf("stdout = %q, want command output", payload.Stdout)
	}
	if len(payload.EvidenceRefs) != 1 {
		t.Fatalf("evidenceRefs = %#v, want one ref", payload.EvidenceRefs)
	}
	rec, ok := service.Get(context.Background(), payload.EvidenceRefs[0])
	if !ok {
		t.Fatalf("evidence ref %q was not recorded", payload.EvidenceRefs[0])
	}
	if rec.SourceTool != "exec_command" || rec.Source != "terminal.break_glass" || rec.ToolCallID != "call-terminal-evidence" {
		t.Fatalf("record = %#v, want terminal exec context", rec)
	}
}

func TestExecCommandToolAllowsSafeCurlGetCommandLineInCommandField(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %q, want GET", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","source":"command-field"}`))
	}))
	defer server.Close()

	tool := NewExecCommandTool(Options{WorkingDir: t.TempDir()})
	input := json.RawMessage(`{"command":"curl -sS --max-time 5 ` + server.URL + `"}`)

	if !tool.IsReadOnly(input) {
		t.Fatal("safe curl command line in command field should be classified read-only")
	}
	decision := tool.CheckPermissions(context.Background(), input)
	if decision.Action != tooling.PermissionActionAllow {
		t.Fatalf("CheckPermissions() = %#v, want allow", decision)
	}
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stdout := execCommandStdout(t, result.Content); !strings.Contains(stdout, `"source":"command-field"`) {
		t.Fatalf("stdout = %q, want curl response body", stdout)
	}
}

func TestExecCommandToolRequiresEvidenceForNonAllowlistedReadOnlyCommand(t *testing.T) {
	tool := NewExecCommandTool(Options{WorkingDir: t.TempDir()})
	input := json.RawMessage(`{"command":"bash","args":["-lc","date '+%F %A %u %T %Z'"]}`)

	if tool.IsReadOnly(input) {
		t.Fatal("bash -lc date should not bypass break-glass allowlist")
	}
	decision := tool.CheckPermissions(context.Background(), input)
	if decision.Action != tooling.PermissionActionNeedEvidence {
		t.Fatalf("CheckPermissions() = %#v, want need evidence", decision)
	}
}

func TestExecCommandToolAllowsShellWrappedSafeCurlGet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %q, want GET", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"source":"shell-wrapped"}`))
	}))
	defer server.Close()

	tool := NewExecCommandTool(Options{WorkingDir: t.TempDir()})
	input := json.RawMessage(`{"command":"bash","args":["-lc","curl -L --max-time 5 -A 'Mozilla/5.0' '` + server.URL + `?symbol=000001&fields=f1,f2'"]}`)

	if !tool.IsReadOnly(input) {
		t.Fatal("bash -lc safe curl GET should be classified read-only")
	}
	decision := tool.CheckPermissions(context.Background(), input)
	if decision.Action != tooling.PermissionActionAllow {
		t.Fatalf("CheckPermissions() = %#v, want allow", decision)
	}
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stdout := execCommandStdout(t, result.Content); !strings.Contains(stdout, `"source":"shell-wrapped"`) {
		t.Fatalf("stdout = %q, want curl response body", stdout)
	}
}

func TestExecCommandToolRequiresEvidenceForPythonEvenIfDiagnosticIntent(t *testing.T) {
	tool := NewExecCommandTool(Options{WorkingDir: t.TempDir()})
	input := json.RawMessage(`{"command":"python","args":["-c","print('hi')"],"intent":"diagnostic one-liner"}`)

	if tool.IsReadOnly(input) {
		t.Fatal("python must not be allowlisted as read-only terminal")
	}
	decision := tool.CheckPermissions(context.Background(), input)
	if decision.Action != tooling.PermissionActionNeedEvidence {
		t.Fatalf("CheckPermissions() = %#v, want need evidence", decision)
	}
}

func TestExecCommandToolRequiresApprovalForUnsafeCurlArgs(t *testing.T) {
	tool := NewExecCommandTool(Options{WorkingDir: t.TempDir(), ActionTokenSecret: []byte("localtools-secret")})
	input := json.RawMessage(`{"command":"curl","args":["-sS","-X","POST","https://example.com/api"]}`)

	if tool.IsReadOnly(input) {
		t.Fatal("curl with mutation method must not be classified read-only")
	}
	decision := tool.CheckPermissions(context.Background(), input)
	if decision.Action != tooling.PermissionActionNeedEvidence {
		t.Fatalf("CheckPermissions() = %#v, want need evidence", decision)
	}
}

func TestExecCommandToolRejectsShellOperators(t *testing.T) {
	tool := NewExecCommandTool(Options{WorkingDir: t.TempDir()})
	input := json.RawMessage(`{"cmd":"echo ok && rm -rf /tmp/nope"}`)

	if tool.IsReadOnly(input) {
		t.Fatal("command with shell operators must not be read-only")
	}
	decision := tool.CheckPermissions(context.Background(), input)
	if decision.Action != tooling.PermissionActionDeny {
		t.Fatalf("CheckPermissions() = %#v, want deny", decision)
	}
	if _, err := tool.Execute(context.Background(), input); err == nil {
		t.Fatal("Execute() should reject shell operators")
	}
}

func TestExecCommandToolRequiresApprovalForNonReadOnlyCommand(t *testing.T) {
	tool := NewExecCommandTool(Options{WorkingDir: t.TempDir(), ActionTokenSecret: []byte("localtools-secret")})
	input := json.RawMessage(`{"command":"touch","args":["marker"]}`)

	if tool.IsReadOnly(input) {
		t.Fatal("touch command must not be read-only")
	}
	decision := tool.CheckPermissions(context.Background(), input)
	if decision.Action != tooling.PermissionActionNeedEvidence {
		t.Fatalf("CheckPermissions() = %#v, want need evidence", decision)
	}
}

func TestExecCommandToolValidHighRiskTokenNeedsApprovalWithPayload(t *testing.T) {
	now := time.Date(2026, 5, 4, 10, 0, 0, 0, time.UTC)
	secret := []byte("localtools-secret")
	tool := NewExecCommandTool(Options{
		WorkingDir:        t.TempDir(),
		ActionTokenSecret: secret,
		Now:               func() time.Time { return now },
	})
	input := json.RawMessage(`{"command":"systemctl","args":["restart","erp-report.service"],"intent":"restart report worker after runbook diagnosis"}`)
	token := signExecActionToken(t, secret, now, input, actionproposal.SourceRunbook, actionproposal.RiskHigh)
	withToken := injectActionToken(t, input, token)

	decision := tool.CheckPermissions(context.Background(), withToken)
	if decision.Action != tooling.PermissionActionNeedApproval {
		t.Fatalf("CheckPermissions() = %#v, want need approval", decision)
	}
	if decision.Approval == nil {
		t.Fatalf("CheckPermissions() approval payload = nil")
	}
	if decision.Approval.Command != "systemctl restart erp-report.service" {
		t.Fatalf("approval command = %q", decision.Approval.Command)
	}
	if decision.Approval.Risk != string(actionproposal.RiskHigh) || decision.Approval.Source != string(actionproposal.SourceRunbook) {
		t.Fatalf("approval payload = %#v, want high/runbook", decision.Approval)
	}
	if decision.Approval.ExpectedEffect == "" || decision.Approval.Rollback == "" || decision.Approval.RunbookStep == "" {
		t.Fatalf("approval payload missing governed fields: %#v", decision.Approval)
	}
}

func TestExecCommandToolValidLowRiskTokenStillNeedsApprovalForBreakGlassMutation(t *testing.T) {
	now := time.Date(2026, 5, 4, 10, 0, 0, 0, time.UTC)
	secret := []byte("localtools-secret")
	tool := NewExecCommandTool(Options{
		WorkingDir:        t.TempDir(),
		ActionTokenSecret: secret,
		Now:               func() time.Time { return now },
	})
	input := json.RawMessage(`{"command":"touch","args":["marker"]}`)
	token := signExecActionToken(t, secret, now, input, actionproposal.SourceFallback, actionproposal.RiskLow)
	withToken := injectActionToken(t, input, token)

	decision := tool.CheckPermissions(context.Background(), withToken)
	if decision.Action != tooling.PermissionActionNeedApproval {
		t.Fatalf("CheckPermissions() = %#v, want need approval", decision)
	}
}

func TestExecCommandToolRejectsWrongToolToken(t *testing.T) {
	now := time.Date(2026, 5, 4, 10, 0, 0, 0, time.UTC)
	secret := []byte("localtools-secret")
	tool := NewExecCommandTool(Options{
		WorkingDir:        t.TempDir(),
		ActionTokenSecret: secret,
		Now:               func() time.Time { return now },
	})
	input := json.RawMessage(`{"command":"systemctl","args":["restart","erp-report.service"]}`)
	hash, err := actionproposal.NormalizedInputHash(input)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	token, err := actionproposal.NewSigner(secret, func() time.Time { return now }).Sign(actionproposal.ActionTokenClaims{
		ToolName:  "ops.restart_workload",
		InputHash: hash,
		Source:    actionproposal.SourceRunbook,
		Risk:      actionproposal.RiskHigh,
		ExpiresAt: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	decision := tool.CheckPermissions(context.Background(), injectActionToken(t, input, token))
	if decision.Action != tooling.PermissionActionNeedEvidence {
		t.Fatalf("CheckPermissions() = %#v, want need evidence", decision)
	}
}

func TestExecCommandToolRejectsUnknownTokenSource(t *testing.T) {
	now := time.Date(2026, 5, 4, 10, 0, 0, 0, time.UTC)
	secret := []byte("localtools-secret")
	tool := NewExecCommandTool(Options{
		WorkingDir:        t.TempDir(),
		ActionTokenSecret: secret,
		Now:               func() time.Time { return now },
	})
	input := json.RawMessage(`{"command":"systemctl","args":["restart","erp-report.service"]}`)
	token := signExecActionToken(t, secret, now, input, actionproposal.Source("manual"), actionproposal.RiskHigh)

	decision := tool.CheckPermissions(context.Background(), injectActionToken(t, input, token))
	if decision.Action != tooling.PermissionActionDeny {
		t.Fatalf("CheckPermissions() = %#v, want deny", decision)
	}
}

func TestExecCommandToolRejectsForbiddenCommandEvenWithToken(t *testing.T) {
	now := time.Date(2026, 5, 4, 10, 0, 0, 0, time.UTC)
	secret := []byte("localtools-secret")
	tool := NewExecCommandTool(Options{
		WorkingDir:        t.TempDir(),
		ActionTokenSecret: secret,
		Now:               func() time.Time { return now },
	})
	input := json.RawMessage(`{"command":"rm","args":["-rf","/tmp/aiops-danger"]}`)
	token := signExecActionToken(t, secret, now, input, actionproposal.SourceBreakGlass, actionproposal.RiskCritical)

	decision := tool.CheckPermissions(context.Background(), injectActionToken(t, input, token))
	if decision.Action != tooling.PermissionActionDeny {
		t.Fatalf("CheckPermissions() = %#v, want deny", decision)
	}
}

func TestExecCommandToolPropertyTamperedCommandArgsInvalidateToken(t *testing.T) {
	workingDir := t.TempDir()
	rapid.Check(t, func(rt *rapid.T) {
		now := time.Date(2026, 5, 4, 10, 0, 0, 0, time.UTC)
		secret := []byte("localtools-secret")
		tool := NewExecCommandTool(Options{
			WorkingDir:        workingDir,
			ActionTokenSecret: secret,
			Now:               func() time.Time { return now },
		})
		command := rapid.SampledFrom([]string{"systemctl", "touch", "kubectl", "curl"}).Draw(rt, "command")
		arg := rapid.StringMatching(`[a-zA-Z0-9._/-]{1,24}`).Draw(rt, "arg")
		input := mustMarshalRaw(t, map[string]any{"command": command, "args": []string{arg}})
		token := signExecActionToken(t, secret, now, input, actionproposal.SourceRunbook, actionproposal.RiskHigh)
		tampered := mustMarshalRaw(t, map[string]any{"command": command, "args": []string{arg, "tampered"}, "actionToken": token})

		decision := tool.CheckPermissions(context.Background(), tampered)
		if decision.Action == tooling.PermissionActionAllow || decision.Action == tooling.PermissionActionNeedApproval {
			t.Fatalf("tampered token decision = %#v, want not allow/approval", decision)
		}
	})
}

func TestExecCommandToolPropertyAllowlistedTerminalCommandsDoNotNeedToken(t *testing.T) {
	workingDir := t.TempDir()
	rapid.Check(t, func(rt *rapid.T) {
		command := rapid.SampledFrom([]string{"kubectl", "redis-cli"}).Draw(rt, "command")
		args := []string{"get", "events", "-n", "prod"}
		if command == "redis-cli" {
			args = []string{"-h", "redis.prod", "INFO"}
		}
		tool := NewExecCommandTool(Options{WorkingDir: workingDir})
		input := mustMarshalRaw(t, map[string]any{"command": command, "args": args})

		if tool.IsDestructive(input) {
			t.Fatalf("%s should not be destructive", command)
		}
		decision := tool.CheckPermissions(context.Background(), input)
		if decision.Action != tooling.PermissionActionAllow {
			t.Fatalf("CheckPermissions(%s) = %#v, want allow", command, decision)
		}
	})
}

func TestExecCommandToolPropertyForbiddenCommandsAlwaysDeny(t *testing.T) {
	workingDir := t.TempDir()
	rapid.Check(t, func(rt *rapid.T) {
		now := time.Date(2026, 5, 4, 10, 0, 0, 0, time.UTC)
		secret := []byte("localtools-secret")
		tool := NewExecCommandTool(Options{
			WorkingDir:        workingDir,
			ActionTokenSecret: secret,
			Now:               func() time.Time { return now },
		})
		command := rapid.SampledFrom([]string{"rm", "reboot", "shutdown", "halt", "poweroff", "mkfs", "dd", "chmod", "chown"}).Draw(rt, "command")
		arg := rapid.StringMatching(`[a-zA-Z0-9._/-]{1,24}`).Draw(rt, "arg")
		input := mustMarshalRaw(t, map[string]any{"command": command, "args": []string{arg}})
		token := signExecActionToken(t, secret, now, input, actionproposal.SourceBreakGlass, actionproposal.RiskCritical)

		decision := tool.CheckPermissions(context.Background(), injectActionToken(t, input, token))
		if decision.Action != tooling.PermissionActionDeny {
			t.Fatalf("CheckPermissions(%s) = %#v, want deny", command, decision)
		}
	})
}

func signExecActionToken(t testing.TB, secret []byte, now time.Time, input json.RawMessage, source actionproposal.Source, risk actionproposal.Risk) string {
	t.Helper()
	hash, err := actionproposal.NormalizedInputHash(input)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	token, err := actionproposal.NewSigner(secret, func() time.Time { return now }).Sign(actionproposal.ActionTokenClaims{
		SessionID:        "sess-1",
		TurnID:           "turn-1",
		IncidentID:       "inc-1",
		ToolName:         "exec_command",
		InputHash:        hash,
		Source:           source,
		Risk:             risk,
		Reason:           "runbook guarded terminal action",
		RunbookID:        "order-submit-slow",
		RunbookStepID:    "restart-report-service",
		RunbookStepTitle: "重启报表服务释放数据库连接",
		ExpectedEffect:   "释放报表服务占用的数据库连接，订单提交延迟应下降。",
		Rollback:         "验证服务状态；失败时回退到人工接管。",
		ExpiresAt:        now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return token
}

func injectActionToken(t testing.TB, input json.RawMessage, token string) json.RawMessage {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(input, &payload); err != nil {
		t.Fatalf("unmarshal input: %v", err)
	}
	payload["actionToken"] = token
	return mustMarshalRaw(t, payload)
}

func mustMarshalRaw(t testing.TB, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return json.RawMessage(data)
}

func execCommandStdout(t testing.TB, content string) string {
	t.Helper()
	var payload struct {
		Stdout string `json:"stdout"`
	}
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		t.Fatalf("exec_command result is not JSON: %v\n%s", err, content)
	}
	return payload.Stdout
}

func TestWebSearchToolUsesProviderNativeResponsesAPI(t *testing.T) {
	var gotPath string
	var gotToolType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		var payload struct {
			Tools []struct {
				Type string `json:"type"`
			} `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(payload.Tools) > 0 {
			gotToolType = payload.Tools[0].Type
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output_text":"provider native search result"}`))
	}))
	defer server.Close()

	tool := NewWebSearchTool(&fakeLLMRepo{cfg: &store.LLMConfig{
		Provider: "openai",
		Model:    "gpt-5.4",
		BaseURL:  server.URL,
		APIKey:   "sk-test",
	}}, Options{HTTPClient: server.Client()})

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"OpenAI web_search docs"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if gotPath != "/responses" {
		t.Fatalf("request path = %q, want /responses", gotPath)
	}
	if gotToolType != "web_search" {
		t.Fatalf("tool type = %q, want web_search", gotToolType)
	}
	if !strings.Contains(result.Content, "provider native search result") {
		t.Fatalf("result content = %q, want provider native result", result.Content)
	}
}

func TestWebSearchToolPromptGuidesPreciseCurrentSearchesAndSources(t *testing.T) {
	tool := NewWebSearchTool(&fakeLLMRepo{cfg: &store.LLMConfig{
		Provider: "openai",
		Model:    "gpt-5.4",
		BaseURL:  "http://127.0.0.1:8317/v1",
		APIKey:   "sk-test",
	}}, Options{})

	prompt := tool.Prompt(tooling.PromptContext{})
	for _, want := range []string{
		"precise",
		"current date",
		"authoritative",
		"source",
		"avoid vague",
		"realtime price",
		"try another authoritative source",
	} {
		if !strings.Contains(strings.ToLower(prompt), want) {
			t.Fatalf("web_search prompt missing %q guidance:\n%s", want, prompt)
		}
	}
}

func TestWebSearchToolSupportsDomainFiltersLikeClaudeCode(t *testing.T) {
	var gotSearchQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/responses":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"output": [],
				"tool_usage": {
					"web_search": {
						"num_requests": 1
					}
				}
			}`))
		case "/chat/completions":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":null},"finish_reason":"stop"}]}`))
		case "/search":
			gotSearchQuery = r.URL.Query().Get("q")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body><ol id="b_results">
				<li class="b_algo">
					<h2><a href="https://news.example.com/markets">Generic market article</a></h2>
					<div class="b_caption"><p>A股 上证指数 深证成指 创业板指 行情。</p></div>
				</li>
				<li class="b_algo">
					<h2><a href="https://www.sse.com.cn/market/stockdata/overview/">上交所 A股 行情 官方数据</a></h2>
					<div class="b_caption"><p>上海证券交易所 官方 上证指数 A股 行情。</p></div>
				</li>
			</ol></body></html>`))
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	tool := NewWebSearchTool(&fakeLLMRepo{cfg: &store.LLMConfig{
		Provider: "openai",
		Model:    "gpt-5.4",
		BaseURL:  server.URL,
		APIKey:   "sk-test",
	}}, Options{HTTPClient: server.Client(), PublicSearchBaseURL: server.URL})

	result, err := tool.Execute(context.Background(), json.RawMessage(`{
		"query":"A股 官方 行情 上证指数",
		"allowed_domains":["sse.com.cn"]
	}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(gotSearchQuery, "site:sse.com.cn") {
		t.Fatalf("search query = %q, want site:sse.com.cn refinement", gotSearchQuery)
	}
	if strings.Contains(result.Content, "news.example.com") {
		t.Fatalf("result content = %q, should filter non-allowed domain", result.Content)
	}
	if !strings.Contains(result.Content, "sse.com.cn") {
		t.Fatalf("result content = %q, want allowed domain result", result.Content)
	}
}

func TestWebSearchToolRejectsConflictingDomainFilters(t *testing.T) {
	tool := NewWebSearchTool(&fakeLLMRepo{cfg: &store.LLMConfig{
		Provider: "openai",
		Model:    "gpt-5.4",
		BaseURL:  "http://127.0.0.1:8317/v1",
		APIKey:   "sk-test",
	}}, Options{})

	_, err := tool.Execute(context.Background(), json.RawMessage(`{
		"query":"OpenAI web_search docs",
		"allowed_domains":["openai.com"],
		"blocked_domains":["openai.com"]
	}`))
	if err == nil {
		t.Fatal("Execute() should reject simultaneous allowed and blocked domains")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "allowed_domains") {
		t.Fatalf("error = %v, want allowed_domains guidance", err)
	}
}

func TestBrowseURLToolFetchesReadableText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html>
			<html>
				<head><title>Market Snapshot</title><script>ignore()</script></head>
				<body><h1>Market Snapshot</h1><p>Index moved higher today.</p><style>.x{}</style></body>
			</html>`))
	}))
	defer server.Close()

	tool := NewBrowseURLTool(Options{HTTPClient: server.Client(), MaxOutputBytes: 1000})
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"url":"`+server.URL+`"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Content, "Market Snapshot") || !strings.Contains(result.Content, "Index moved higher today.") {
		t.Fatalf("result content = %q, want readable page text", result.Content)
	}
	if strings.Contains(result.Content, "ignore()") || strings.Contains(result.Content, ".x{}") {
		t.Fatalf("result content = %q, should strip script/style content", result.Content)
	}
}

func TestBrowseURLToolRejectsNonHTTPURL(t *testing.T) {
	tool := NewBrowseURLTool(Options{})
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"url":"file:///etc/passwd"}`)); err == nil {
		t.Fatal("Execute() should reject non-http URL")
	}
}

func TestTruncateStringPreservesUTF8(t *testing.T) {
	got := truncateString("中文内容abc", 5)
	if !utf8.ValidString(got) {
		t.Fatalf("truncateString returned invalid UTF-8: %q", got)
	}
	if len(got) > 5 {
		t.Fatalf("truncateString returned %d bytes, want at most 5: %q", len(got), got)
	}
	if got != "..." {
		t.Fatalf("truncateString = %q, want byte-budget-safe ellipsis", got)
	}
}

func TestWebSearchToolTreatsProviderNativeEmptyTextAsSuccessfulSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/responses":
			_, _ = w.Write([]byte(`{
				"output": [],
				"tool_usage": {
					"web_search": {
						"num_requests": 1
					}
				}
			}`))
		case "/chat/completions":
			http.Error(w, "chat fallback unavailable", http.StatusBadGateway)
		case "/search":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body><ol id="b_results"></ol></body></html>`))
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	tool := NewWebSearchTool(&fakeLLMRepo{cfg: &store.LLMConfig{
		Provider: "openai",
		Model:    "gpt-5.4",
		BaseURL:  server.URL,
		APIKey:   "sk-test",
	}}, Options{HTTPClient: server.Client(), PublicSearchBaseURL: server.URL})

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"OpenAI web_search docs"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Content, `"source":"provider_native:responses:web_search"`) {
		t.Fatalf("result content = %q, want provider-native source", result.Content)
	}
	if !strings.Contains(result.Content, "provider-native web_search completed") {
		t.Fatalf("result content = %q, want provider-native completion note", result.Content)
	}
}

func TestWebSearchToolFallsBackToPublicSearchWhenNativeSearchHasNoText(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/responses":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"output": [],
				"tool_usage": {
					"web_search": {
						"num_requests": 1
					}
				}
			}`))
		case "/chat/completions":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":null},"finish_reason":"stop"}]}`))
		case "/search":
			if got := r.URL.Query().Get("q"); got != "market status" {
				t.Fatalf("search query = %q, want market status", got)
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body><ol id="b_results">
				<li class="b_algo">
					<h2><a href="https://example.com/market">Market report</a></h2>
					<div class="b_caption"><p>Index moved higher with public source details.</p></div>
				</li>
			</ol></body></html>`))
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	tool := NewWebSearchTool(&fakeLLMRepo{cfg: &store.LLMConfig{
		Provider: "openai",
		Model:    "gpt-5.4",
		BaseURL:  server.URL,
		APIKey:   "sk-test",
	}}, Options{HTTPClient: server.Client(), PublicSearchBaseURL: server.URL})

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"market status"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := strings.Join(paths, ","); got != "/responses,/chat/completions,/search" {
		t.Fatalf("paths = %q, want native responses, chat fallback, public search", got)
	}
	if !strings.Contains(result.Content, "Market report") || !strings.Contains(result.Content, "https://example.com/market") {
		t.Fatalf("result content = %q, want parsed public search result", result.Content)
	}
	if !strings.Contains(result.Content, "provider_native:responses:web_search+public_web_search:bing_fallback") {
		t.Fatalf("result content = %q, want combined provider-native and public search source", result.Content)
	}
	if strings.Contains(result.Content, "provider returned no textual summary") {
		t.Fatalf("result content = %q, should not return provider no-summary placeholder when public fallback succeeds", result.Content)
	}
}

func TestWebSearchToolPublicFallbackDropsLowRelevanceResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/responses":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"output": [],
				"tool_usage": {
					"web_search": {
						"num_requests": 1
					}
				}
			}`))
		case "/chat/completions":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":null},"finish_reason":"stop"}]}`))
		case "/search":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body><ol id="b_results">
				<li class="b_algo">
					<h2><a href="https://music.youtube.com/watch?v=UGMGQo3gmvI">Agnaldo Timóteo - Escudo ( Clipe Oficial ) - YouTube Music</a></h2>
					<div class="b_caption"><p>Brazilian music video unrelated to Chinese equity markets.</p></div>
				</li>
				<li class="b_algo">
					<h2><a href="https://example.com/a-share-close">A股收盘：上证指数 深证成指 创业板指 市场行情</a></h2>
					<div class="b_caption"><p>A股 今日 收盘 上证指数 深证成指 创业板指 成交额。</p></div>
				</li>
			</ol></body></html>`))
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	tool := NewWebSearchTool(&fakeLLMRepo{cfg: &store.LLMConfig{
		Provider: "openai",
		Model:    "gpt-5.4",
		BaseURL:  server.URL,
		APIKey:   "sk-test",
	}}, Options{HTTPClient: server.Client(), PublicSearchBaseURL: server.URL})

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"A股 今日 收盘 上证指数 深证成指 创业板指 2026-04-26"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if strings.Contains(result.Content, "Agnaldo") || strings.Contains(result.Content, "youtube.com") {
		t.Fatalf("result content = %q, should drop low-relevance public search results", result.Content)
	}
	if !strings.Contains(result.Content, "A股收盘") || !strings.Contains(result.Content, "https://example.com/a-share-close") {
		t.Fatalf("result content = %q, want relevant public search result", result.Content)
	}
}

func TestWebSearchToolRejectsVagueGenericQueries(t *testing.T) {
	tool := NewWebSearchTool(&fakeLLMRepo{cfg: &store.LLMConfig{
		Provider: "openai",
		Model:    "gpt-5.4",
		BaseURL:  "http://127.0.0.1:8317/v1",
		APIKey:   "sk-test",
	}}, Options{})

	_, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"web"}`))
	if err == nil {
		t.Fatal("Execute() should reject vague generic query")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "vague") {
		t.Fatalf("error = %v, want vague query guidance", err)
	}
}

func TestPublicSearchRelevanceTermsSplitCompactChineseQuery(t *testing.T) {
	results := []publicSearchResult{
		{
			Title:   "两会新华社权威速览丨一图速览 2026 年政府工作报告",
			URL:     "https://example.com/government-report",
			Snippet: "政府工作报告摘要。",
		},
		{
			Title:   "上海证券交易所 2026 年部分节假日休市安排",
			URL:     "https://www.sse.com.cn/disclosure/announcement/general/",
			Snippet: "官方发布节假日休市安排。",
		},
	}

	filtered := filterPublicSearchResultsByRelevance(results, "2026年部分节假日休市安排 上海证券交易所 官方")
	if len(filtered) != 1 {
		t.Fatalf("filtered len = %d, want 1: %#v", len(filtered), filtered)
	}
	if strings.Contains(filtered[0].Title, "政府工作报告") {
		t.Fatalf("filtered = %#v, should drop low-relevance generic 2026 result", filtered)
	}
	if !strings.Contains(filtered[0].Title, "上海证券交易所") {
		t.Fatalf("filtered = %#v, want exchange result", filtered)
	}
}

func TestPublicSearchRelevanceDropsDateOnlyMatches(t *testing.T) {
	results := []publicSearchResult{
		{
			Title:   "2026 年_百度百科",
			URL:     "https://baike.baidu.com/item/2026%E5%B9%B4/9536516",
			Snippet: "2026 年日期信息。",
		},
		{
			Title:   "中国 A股 交易日 上交所 深交所 周日休市说明",
			URL:     "https://example.com/ashare-trading-day",
			Snippet: "中国 A股 今天 是否交易日，交易日安排以上交所深交所公告为准。",
		},
	}

	filtered := filterPublicSearchResultsByRelevance(results, "2026-04-26 中国 A股 今天 是否 交易日 上交所 深交所 周日")
	if len(filtered) != 1 {
		t.Fatalf("filtered len = %d, want 1: %#v", len(filtered), filtered)
	}
	if strings.Contains(filtered[0].Title, "百度百科") {
		t.Fatalf("filtered = %#v, should drop date-only result", filtered)
	}
}

func TestWebSearchToolParsesPublicSearchResultAfterLargeSearchChrome(t *testing.T) {
	var sawSearch bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/responses":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"output": [],
				"tool_usage": {
					"web_search": {
						"num_requests": 1
					}
				}
			}`))
		case "/chat/completions":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":null},"finish_reason":"stop"}]}`))
		case "/search":
			sawSearch = true
			if got := r.URL.Query().Get("q"); got != "market status" {
				t.Fatalf("search query = %q, want market status", got)
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			var body strings.Builder
			body.WriteString(`<html><body><style>`)
			body.WriteString(strings.Repeat(".noise{color:#999}", 1600))
			body.WriteString(`</style><ol id="b_results">
				<li class="b_algo">
					<h2><a href="https://example.com/late-market">Late market report</a></h2>
					<div class="b_caption"><p>Useful result after a large search page header.</p></div>
				</li>
			</ol></body></html>`)
			_, _ = w.Write([]byte(body.String()))
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	tool := NewWebSearchTool(&fakeLLMRepo{cfg: &store.LLMConfig{
		Provider: "openai",
		Model:    "gpt-5.4",
		BaseURL:  server.URL,
		APIKey:   "sk-test",
	}}, Options{HTTPClient: server.Client(), PublicSearchBaseURL: server.URL})

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"market status"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !sawSearch {
		t.Fatal("public search fallback was not called")
	}
	if !strings.Contains(result.Content, "Late market report") || !strings.Contains(result.Content, "https://example.com/late-market") {
		t.Fatalf("result content = %q, want parsed result after large search chrome", result.Content)
	}
	if strings.Contains(result.Content, "provider returned no textual summary") {
		t.Fatalf("result content = %q, should not return provider no-summary placeholder when late public result is available", result.Content)
	}
}

func TestParseBingSearchResultsHandlesHTMLAttributeVariants(t *testing.T) {
	results := parseBingSearchResults(`<html><body><ol id='b_results'>
		<li data-id='1' class='result b_algo extra'>
			<h2><a data-track='x' href='https://example.com/market'><span>Market</span> <strong>report</strong></a></h2>
			<div class='b_caption'><p>Index <em>moved</em> higher.</p></div>
		</li>
	</ol></body></html>`, 5)

	if len(results) != 1 {
		t.Fatalf("results len = %d, want 1: %#v", len(results), results)
	}
	if results[0].Title != "Market report" {
		t.Fatalf("Title = %q, want nested anchor text", results[0].Title)
	}
	if results[0].URL != "https://example.com/market" {
		t.Fatalf("URL = %q, want href", results[0].URL)
	}
	if results[0].Snippet != "Index moved higher." {
		t.Fatalf("Snippet = %q, want caption text", results[0].Snippet)
	}
}

func TestWebSearchToolFallsBackToPublicSearchWhenResponsesReturnsNoUsableText(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/responses":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"output_text": null,
				"output": [],
				"tool_usage": {
					"web_search": {
						"num_requests": 0
					}
				}
			}`))
		case "/chat/completions":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":null},"finish_reason":"stop"}]}`))
		case "/search":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body><ol id="b_results">
				<li class="b_algo">
					<h2><a href="https://example.com/generic">OpenAI web search documentation result</a></h2>
					<div class="b_caption"><p>Public fallback result for OpenAI web search documentation no-text provider response.</p></div>
				</li>
			</ol></body></html>`))
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	tool := NewWebSearchTool(&fakeLLMRepo{cfg: &store.LLMConfig{
		Provider: "openai",
		Model:    "gpt-5.4",
		BaseURL:  server.URL,
		APIKey:   "sk-test",
	}}, Options{HTTPClient: server.Client(), PublicSearchBaseURL: server.URL})

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"OpenAI web search documentation"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := strings.Join(paths, ","); got != "/responses,/chat/completions,/search" {
		t.Fatalf("paths = %q, want responses, chat, public search", got)
	}
	if !strings.Contains(result.Content, "OpenAI web search documentation result") {
		t.Fatalf("result content = %q, want public fallback result", result.Content)
	}
}

func TestWebSearchToolFallsBackToChatCompletionsWhenResponsesHasNoText(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/responses":
			_, _ = w.Write([]byte(`{
				"output": [],
				"tool_usage": {
					"web_search": {
						"num_requests": 1
					}
				}
			}`))
		case "/chat/completions":
			_, _ = w.Write([]byte(`{
				"choices": [
					{"message": {"content": "chat fallback search summary with sources"}}
				]
			}`))
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	tool := NewWebSearchTool(&fakeLLMRepo{cfg: &store.LLMConfig{
		Provider: "openai",
		Model:    "gpt-5.4",
		BaseURL:  server.URL,
		APIKey:   "sk-test",
	}}, Options{HTTPClient: server.Client()})

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"OpenAI web_search docs"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if strings.Join(paths, ",") != "/responses,/chat/completions" {
		t.Fatalf("request paths = %#v, want responses then chat fallback", paths)
	}
	if !strings.Contains(result.Content, `"source":"provider_native:chat_completions:web_search_options"`) {
		t.Fatalf("result content = %q, want chat completions source", result.Content)
	}
	if !strings.Contains(result.Content, "chat fallback search summary") {
		t.Fatalf("result content = %q, want chat fallback summary", result.Content)
	}
}

func TestWebSearchToolReturnsProviderNativeSourcesWhenAvailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Include []string `json:"include"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(payload.Include) != 1 || payload.Include[0] != "web_search_call.action.sources" {
			t.Fatalf("include = %#v, want web_search_call.action.sources", payload.Include)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"output": [
				{
					"type": "web_search_call",
					"action": {
						"sources": [
							{"url": "https://platform.openai.com/docs/guides/tools-web-search", "title": "Web search"}
						]
					}
				}
			],
			"tool_usage": {
				"web_search": {
					"num_requests": 1
				}
			}
		}`))
	}))
	defer server.Close()

	tool := NewWebSearchTool(&fakeLLMRepo{cfg: &store.LLMConfig{
		Provider: "openai",
		Model:    "gpt-5.4",
		BaseURL:  server.URL,
		APIKey:   "sk-test",
	}}, Options{HTTPClient: server.Client()})

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"OpenAI web_search docs"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Content, "https://platform.openai.com/docs/guides/tools-web-search") {
		t.Fatalf("result content = %q, want source URL", result.Content)
	}
}

func toolNames(tools []tooling.Tool) []string {
	out := make([]string, 0, len(tools))
	for _, tool := range tools {
		out = append(out, tool.Metadata().Name)
	}
	return out
}
