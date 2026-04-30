package localtools

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"aiops-v2/internal/planning"
	"aiops-v2/internal/store"
	"aiops-v2/internal/terminalpolicy"
	"aiops-v2/internal/tooling"
	nethtml "golang.org/x/net/html"
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

// Options configures builtin local tools.
type Options struct {
	WorkingDir          string
	HTTPClient          *http.Client
	CommandTimeout      time.Duration
	WebTimeout          time.Duration
	MaxOutputBytes      int
	PublicSearchBaseURL string
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
	return o
}

// RegisterBuiltins installs the local host tool surface into the single Tool registry.
func RegisterBuiltins(registry *tooling.Registry, repo LLMConfigRepository, opts Options) error {
	if registry == nil {
		return fmt.Errorf("localtools: registry is required")
	}
	for _, tool := range []tooling.Tool{
		NewWebSearchTool(repo, opts),
		NewBrowseURLTool(opts),
		NewExecCommandTool(opts),
		NewCurrentModelConfigTool(repo),
		planning.NewUpdatePlanTool(),
	} {
		if err := registry.Register(tool); err != nil {
			return err
		}
	}
	return nil
}

// NewCurrentModelConfigTool returns a safe model-settings inspection tool.
func NewCurrentModelConfigTool(repo LLMConfigRepository) tooling.Tool {
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "get_current_model_config",
			Aliases:     []string{"current_model_config", "get_model_config"},
			Origin:      tooling.ToolOriginBuiltin,
			Description: "Read the currently configured LLM provider, model, base URL, and provider-native tool support without exposing secrets.",
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
				"providerNativeTools": {"type": "array", "items": {"type": "string"}}
			}
		}`),
		ReadOnlyFunc:        func(json.RawMessage) bool { return true },
		ConcurrencySafeFunc: func(json.RawMessage) bool { return true },
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			cfg := currentConfig(repo)
			payload := map[string]any{
				"provider":  cfg.Provider,
				"model":     cfg.Model,
				"baseURL":   cfg.BaseURL,
				"apiKeySet": strings.TrimSpace(cfg.APIKey) != "",
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
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "exec_command",
			Aliases:     []string{"terminal_command", "shell_command"},
			Origin:      tooling.ToolOriginBuiltin,
			Description: execCommandDescription(),
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
				"command": {"type": "string", "description": "Executable name, for example date, pwd, ls, curl."},
				"args": {"type": "array", "items": {"type": "string"}, "description": "Command arguments. Prefer this over shell syntax."},
				"cmd": {"type": "string", "description": "Compatibility command line. Shell operators are rejected."},
				"workingDir": {"type": "string", "description": "Optional working directory."},
				"timeoutMs": {"type": "integer", "description": "Optional timeout in milliseconds, max 60000."}
			}
		}`),
		OutputSchemaData: json.RawMessage(`{
			"type": "object",
			"properties": {
				"stdout": {"type": "string"},
				"stderr": {"type": "string"},
				"exitCode": {"type": "integer"}
			}
		}`),
		ReadOnlyFunc: func(input json.RawMessage) bool {
			req, err := parseCommandInput(input)
			return err == nil && terminalpolicy.IsReadOnlyCommand(req.command, req.args)
		},
		DestructiveFunc: func(input json.RawMessage) bool {
			req, err := parseCommandInput(input)
			return err != nil || !terminalpolicy.IsReadOnlyCommand(req.command, req.args)
		},
		ConcurrencySafeFunc: func(input json.RawMessage) bool {
			req, err := parseCommandInput(input)
			return err == nil && terminalpolicy.IsReadOnlyCommand(req.command, req.args)
		},
		CheckPermissionsFunc: func(_ context.Context, input json.RawMessage) tooling.PermissionDecision {
			req, err := parseCommandInput(input)
			if err != nil {
				return tooling.PermissionDecision{Action: tooling.PermissionActionDeny, Reason: err.Error()}
			}
			if terminalpolicy.IsReadOnlyCommand(req.command, req.args) {
				return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
			}
			return tooling.PermissionDecision{
				Action: tooling.PermissionActionNeedApproval,
				Reason: "local terminal command may mutate host state",
			}
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
			if err != nil {
				return tooling.ToolResult{}, fmt.Errorf("command failed: %w; stderr: %s", err, truncateString(stderr.String(), opts.MaxOutputBytes/2))
			}
			content := stdout.String()
			if strings.TrimSpace(stderr.String()) != "" {
				content = strings.TrimRight(content, "\n") + "\nstderr:\n" + stderr.String()
			}
			content = truncateString(content, opts.MaxOutputBytes)
			return tooling.ToolResult{
				Content: content,
				Display: &tooling.ToolDisplayPayload{
					Type:  "terminal",
					Title: req.command,
				},
			}, nil
		},
	}
}

func execCommandDescription() string {
	description := "Execute a local terminal command on the selected server-local host. Prefer explicit command + args. Read-only inspection commands, including safe curl GET/HEAD requests, are allowed in chat; mutation commands must go through the runtime approval gate, so call the scoped command instead of asking for prose approval. Host OS: " + runtime.GOOS + "."
	switch runtime.GOOS {
	case "darwin":
		return description + " For host resource inspection on macOS, prefer uptime, sysctl -n hw.ncpu, vm_stat, df -h, and top -l 1 -s 0; avoid Linux-only commands such as nproc, free -h, and /proc/*."
	case "linux":
		return description + " For host resource inspection on Linux, prefer uptime, nproc, free -h, df -hT -x tmpfs -x devtmpfs, and cat /proc/loadavg."
	default:
		return description + " Choose commands compatible with this OS; avoid Linux-only commands unless Host OS is linux."
	}
}

// NewBrowseURLTool returns a read-only web page fetcher. Search discovery still
// belongs to web_search; this tool opens a known URL and extracts readable text.
func NewBrowseURLTool(opts Options) tooling.Tool {
	opts = opts.normalize()
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "browse_url",
			Aliases:     []string{"web_fetch", "fetch_url", "open_url"},
			Origin:      tooling.ToolOriginBuiltin,
			Description: "Fetch a specific http(s) URL and return readable page text. Use this after web_search returns URLs or when the user provides a URL. Do not use exec_command/bash/python to browse web pages.",
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
			client := opts.HTTPClient
			if client == nil {
				client = &http.Client{Timeout: opts.WebTimeout}
			}
			text, contentType, err := fetchReadableURLText(ctx, client, req.URL, boundedMaxBytes(req.MaxBytes, opts.MaxOutputBytes))
			if err != nil {
				return tooling.ToolResult{}, err
			}
			payload := map[string]string{
				"url":         req.URL,
				"contentType": contentType,
				"text":        truncateString(text, opts.MaxOutputBytes),
			}
			data, _ := json.Marshal(payload)
			return tooling.ToolResult{
				Content: string(data),
				Display: &tooling.ToolDisplayPayload{
					Type:  "web_page",
					Title: req.URL,
				},
			}, nil
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
			Description: "Search the web using the current model provider's native web_search tool first; fall back to public web results only when the provider returns no usable text. Use precise, self-contained queries. For current or latest information, include the current date or target date, key entities, and the data you need. Prefer authoritative sources and cite source URLs. Use allowed_domains or blocked_domains when you need Claude Code-style source control. Avoid vague one-word queries; if results are weak or irrelevant, refine the query with source names, official domains, or site: filters.",
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
				"query": {"type": "string", "description": "Precise search query. For current/latest information include the current date or target date, key entities, and the desired data. Avoid vague queries."},
				"search_context_size": {"type": "string", "enum": ["low", "medium", "high"], "description": "Provider-native search context size."},
				"allowed_domains": {"type": "array", "items": {"type": "string"}, "description": "Optional authoritative domains to restrict public fallback results, for example sse.com.cn. Do not combine with blocked_domains."},
				"blocked_domains": {"type": "array", "items": {"type": "string"}, "description": "Optional domains to exclude from public fallback results. Do not combine with allowed_domains."}
			},
			"required": ["query"]
		}`),
		OutputSchemaData: json.RawMessage(`{
			"type": "object",
			"properties": {
				"query": {"type": "string"},
				"source": {"type": "string"},
				"content": {"type": "string"}
			}
		}`),
		ReadOnlyFunc:        func(json.RawMessage) bool { return true },
		ConcurrencySafeFunc: func(json.RawMessage) bool { return true },
		ValidateInputFunc: func(_ context.Context, input json.RawMessage) error {
			_, err := parseWebSearchInput(input)
			return err
		},
		ExecuteFunc: func(ctx context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			req, err := parseWebSearchInput(input)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			cfg := currentConfig(repo)
			if strings.TrimSpace(cfg.APIKey) == "" {
				return tooling.ToolResult{}, fmt.Errorf("web_search: current model provider has no API key configured")
			}
			if !providerSupportsNativeWebSearch(cfg.Provider, cfg.Model) {
				return tooling.ToolResult{}, fmt.Errorf("web_search: provider %q model %q has no known native web_search support", cfg.Provider, cfg.Model)
			}
			client := opts.HTTPClient
			if client == nil {
				client = &http.Client{Timeout: opts.WebTimeout}
			}
			content, source, err := runProviderNativeWebSearch(ctx, client, cfg, req, opts)
			if err != nil {
				return tooling.ToolResult{}, err
			}
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
			}, nil
		},
	}
}

type commandInput struct {
	Command    string   `json:"command"`
	Args       []string `json:"args"`
	Cmd        string   `json:"cmd"`
	WorkingDir string   `json:"workingDir"`
	TimeoutMs  int      `json:"timeoutMs"`

	command string
	args    []string
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

func parseWebSearchInput(input json.RawMessage) (webSearchInput, error) {
	var req webSearchInput
	if err := json.Unmarshal(input, &req); err != nil {
		return webSearchInput{}, fmt.Errorf("invalid web_search input: %w", err)
	}
	req.Query = strings.TrimSpace(req.Query)
	if req.Query == "" {
		return webSearchInput{}, errors.New("query is required")
	}
	if isVagueWebSearchQuery(req.Query) {
		return webSearchInput{}, errors.New("query is too vague; provide a precise self-contained query with entities, date or target data, and source/domain hints when relevant")
	}
	req.SearchContextSize = strings.TrimSpace(req.SearchContextSize)
	if req.SearchContextSize == "" {
		req.SearchContextSize = "medium"
	}
	switch req.SearchContextSize {
	case "low", "medium", "high":
	default:
		return webSearchInput{}, fmt.Errorf("invalid search_context_size %q", req.SearchContextSize)
	}
	var err error
	req.AllowedDomains, err = normalizeDomainFilters(req.AllowedDomains)
	if err != nil {
		return webSearchInput{}, fmt.Errorf("invalid allowed_domains: %w", err)
	}
	req.BlockedDomains, err = normalizeDomainFilters(req.BlockedDomains)
	if err != nil {
		return webSearchInput{}, fmt.Errorf("invalid blocked_domains: %w", err)
	}
	if len(req.AllowedDomains) > 0 && len(req.BlockedDomains) > 0 {
		return webSearchInput{}, errors.New("allowed_domains and blocked_domains cannot be used together")
	}
	return req, nil
}

func normalizeDomainFilters(values []string) ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		domain, err := normalizeDomainFilter(value)
		if err != nil {
			return nil, err
		}
		if seen[domain] {
			continue
		}
		seen[domain] = true
		out = append(out, domain)
	}
	return out, nil
}

func normalizeDomainFilter(value string) (string, error) {
	raw := strings.ToLower(strings.TrimSpace(value))
	raw = strings.TrimPrefix(raw, "site:")
	raw = strings.TrimPrefix(raw, "*.")
	if raw == "" {
		return "", errors.New("domain cannot be empty")
	}
	host := raw
	if strings.Contains(raw, "://") {
		parsed, err := url.Parse(raw)
		if err != nil || parsed == nil || parsed.Hostname() == "" {
			return "", fmt.Errorf("invalid domain %q", value)
		}
		host = parsed.Hostname()
	} else if strings.Contains(raw, "/") {
		parsed, err := url.Parse("https://" + raw)
		if err != nil || parsed == nil || parsed.Hostname() == "" {
			return "", fmt.Errorf("invalid domain %q", value)
		}
		host = parsed.Hostname()
	}
	host = strings.Trim(host, ".")
	if host == "" || strings.ContainsAny(host, " /\\\t\r\n") {
		return "", fmt.Errorf("invalid domain %q", value)
	}
	return host, nil
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

func fetchReadableURLText(ctx context.Context, client *http.Client, rawURL string, maxBytes int) (string, string, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", "", err
	}
	httpReq.Header.Set("User-Agent", "aiops-v2-browse-url/1.0")
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, int64(maxBytes)+1))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", resp.Header.Get("Content-Type"), fmt.Errorf("browse_url request failed: status %d: %s", resp.StatusCode, truncateString(string(body), 1000))
	}
	contentType := resp.Header.Get("Content-Type")
	text := string(body)
	if strings.Contains(strings.ToLower(contentType), "html") {
		text = htmlToReadableText(text)
	}
	text = compactWhitespace(html.UnescapeString(text))
	if text == "" {
		text = "(empty response)"
	}
	return truncateString(text, maxBytes), contentType, nil
}

var (
	htmlScriptStyleRE = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script>|<style\b[^>]*>.*?</style>|<noscript\b[^>]*>.*?</noscript>`)
	htmlTagRE         = regexp.MustCompile(`(?s)<[^>]+>`)
	whitespaceRE      = regexp.MustCompile(`\s+`)
)

func htmlToReadableText(value string) string {
	value = htmlScriptStyleRE.ReplaceAllString(value, " ")
	value = strings.ReplaceAll(value, "</p>", "\n")
	value = strings.ReplaceAll(value, "</div>", "\n")
	value = strings.ReplaceAll(value, "</li>", "\n")
	value = strings.ReplaceAll(value, "<br>", "\n")
	value = strings.ReplaceAll(value, "<br/>", "\n")
	value = strings.ReplaceAll(value, "<br />", "\n")
	return htmlTagRE.ReplaceAllString(value, " ")
}

func parseBingSearchResults(body string, limit int) []publicSearchResult {
	if limit <= 0 {
		limit = 5
	}
	doc, err := nethtml.Parse(strings.NewReader(body))
	if err != nil {
		return nil
	}
	results := make([]publicSearchResult, 0, limit)
	var walk func(*nethtml.Node)
	walk = func(node *nethtml.Node) {
		if node == nil || len(results) >= limit {
			return
		}
		if isHTMLElement(node, "li") && htmlNodeHasClass(node, "b_algo") {
			if result, ok := parseBingSearchResultNode(node); ok {
				results = append(results, result)
			}
		}
		for child := node.FirstChild; child != nil && len(results) < limit; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	return results
}

func parseBingSearchResultNode(node *nethtml.Node) (publicSearchResult, bool) {
	anchor := firstSearchResultAnchor(node)
	if anchor == nil {
		return publicSearchResult{}, false
	}
	result := publicSearchResult{
		Title: compactWhitespace(html.UnescapeString(htmlNodeText(anchor))),
		URL:   cleanSearchResultURL(html.UnescapeString(htmlNodeAttr(anchor, "href"))),
	}
	if caption := firstDescendant(node, func(candidate *nethtml.Node) bool {
		return isHTMLElement(candidate, "div") && htmlNodeHasClass(candidate, "b_caption")
	}); caption != nil {
		textNode := firstDescendant(caption, func(candidate *nethtml.Node) bool {
			return isHTMLElement(candidate, "p")
		})
		if textNode == nil {
			textNode = caption
		}
		result.Snippet = compactWhitespace(html.UnescapeString(htmlNodeText(textNode)))
	}
	return result, result.Title != "" || result.Snippet != ""
}

func firstSearchResultAnchor(node *nethtml.Node) *nethtml.Node {
	if heading := firstDescendant(node, func(candidate *nethtml.Node) bool {
		return isHTMLElement(candidate, "h2")
	}); heading != nil {
		if anchor := firstDescendant(heading, func(candidate *nethtml.Node) bool {
			return isHTMLElement(candidate, "a") && htmlNodeAttr(candidate, "href") != ""
		}); anchor != nil {
			return anchor
		}
	}
	return firstDescendant(node, func(candidate *nethtml.Node) bool {
		return isHTMLElement(candidate, "a") && htmlNodeAttr(candidate, "href") != ""
	})
}

func firstDescendant(node *nethtml.Node, match func(*nethtml.Node) bool) *nethtml.Node {
	if node == nil || match == nil {
		return nil
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if match(child) {
			return child
		}
		if found := firstDescendant(child, match); found != nil {
			return found
		}
	}
	return nil
}

func isHTMLElement(node *nethtml.Node, tag string) bool {
	return node != nil && node.Type == nethtml.ElementNode && strings.EqualFold(node.Data, tag)
}

func htmlNodeAttr(node *nethtml.Node, name string) string {
	if node == nil {
		return ""
	}
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, name) {
			return attr.Val
		}
	}
	return ""
}

func htmlNodeHasClass(node *nethtml.Node, class string) bool {
	for _, part := range strings.Fields(htmlNodeAttr(node, "class")) {
		if part == class {
			return true
		}
	}
	return false
}

func htmlNodeText(node *nethtml.Node) string {
	var b strings.Builder
	var walk func(*nethtml.Node)
	walk = func(current *nethtml.Node) {
		if current == nil {
			return
		}
		if current.Type == nethtml.TextNode {
			b.WriteString(current.Data)
			b.WriteByte(' ')
			return
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return compactWhitespace(b.String())
}

func cleanSearchResultURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if encoded := parsed.Query().Get("u"); strings.HasPrefix(encoded, "a1") {
		if decoded := decodeBingBase64URL(strings.TrimPrefix(encoded, "a1")); decoded != "" {
			return decoded
		}
	}
	return raw
}

func decodeBingBase64URL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if pad := len(value) % 4; pad != 0 {
		value += strings.Repeat("=", 4-pad)
	}
	decoded, err := base64.URLEncoding.DecodeString(value)
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(value)
	}
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(decoded))
}

func compactWhitespace(value string) string {
	return strings.TrimSpace(whitespaceRE.ReplaceAllString(value, " "))
}

func currentConfig(repo LLMConfigRepository) store.LLMConfig {
	cfg := store.LLMConfig{
		Provider:     "openai",
		Model:        "gpt-5.4",
		CompactModel: "gpt-5.4-mini",
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
	return cfg
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
		if publicContent, publicErr := runPublicWebSearch(ctx, client, req, opts); publicErr == nil && strings.TrimSpace(publicContent) != "" {
			return publicContent, "provider_native:responses:web_search+public_web_search:bing_fallback", nil
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

type publicSearchResult struct {
	Title   string
	URL     string
	Snippet string
}

func runPublicWebSearch(ctx context.Context, client *http.Client, req webSearchInput, opts Options) (string, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(opts.PublicSearchBaseURL), "/")
	if baseURL == "" {
		baseURL = "https://www.bing.com"
	}
	searchURL, err := url.Parse(baseURL + "/search")
	if err != nil {
		return "", err
	}
	query := searchURL.Query()
	query.Set("q", publicSearchQuery(req))
	query.Set("mkt", "zh-CN")
	query.Set("setlang", "zh-CN")
	query.Set("cc", "CN")
	searchURL.RawQuery = query.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL.String(), nil)
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("User-Agent", "aiops-v2-web-search/1.0")
	httpReq.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.7")
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, int64(publicSearchFetchLimit(opts))+1))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("public web search failed: status %d: %s", resp.StatusCode, truncateString(string(body), 1000))
	}
	results := parseBingSearchResults(string(body), 12)
	results = filterPublicSearchResultsByDomain(results, req.AllowedDomains, req.BlockedDomains)
	results = filterPublicSearchResultsByRelevance(results, req.Query)
	if len(results) == 0 {
		return "", errors.New("public web search returned no relevant results")
	}
	if len(results) > 5 {
		results = results[:5]
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Public web search results for %q. Use these results as evidence and cite URLs:\n", req.Query)
	for i, result := range results {
		fmt.Fprintf(&b, "%d. %s\n", i+1, result.Title)
		if result.URL != "" {
			fmt.Fprintf(&b, "   URL: %s\n", result.URL)
		}
		if result.Snippet != "" {
			fmt.Fprintf(&b, "   Snippet: %s\n", result.Snippet)
		}
	}
	return b.String(), nil
}

func filterPublicSearchResultsByRelevance(results []publicSearchResult, query string) []publicSearchResult {
	terms := publicSearchRelevanceTerms(query)
	if len(terms) < 2 {
		return results
	}
	threshold := 1
	if len(terms) >= 4 {
		threshold = 2
	}
	filtered := make([]publicSearchResult, 0, len(results))
	for _, result := range results {
		if publicSearchResultScore(result, terms) >= threshold {
			filtered = append(filtered, result)
		}
	}
	return filtered
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

func publicSearchQuery(req webSearchInput) string {
	parts := []string{strings.TrimSpace(req.Query)}
	for _, domain := range req.AllowedDomains {
		parts = append(parts, "site:"+domain)
	}
	for _, domain := range req.BlockedDomains {
		parts = append(parts, "-site:"+domain)
	}
	return compactWhitespace(strings.Join(parts, " "))
}

func filterPublicSearchResultsByDomain(results []publicSearchResult, allowedDomains, blockedDomains []string) []publicSearchResult {
	if len(allowedDomains) == 0 && len(blockedDomains) == 0 {
		return results
	}
	filtered := make([]publicSearchResult, 0, len(results))
	for _, result := range results {
		host := searchResultHost(result.URL)
		if host == "" {
			continue
		}
		if len(allowedDomains) > 0 && !hostMatchesAnyDomain(host, allowedDomains) {
			continue
		}
		if len(blockedDomains) > 0 && hostMatchesAnyDomain(host, blockedDomains) {
			continue
		}
		filtered = append(filtered, result)
	}
	return filtered
}

func searchResultHost(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed == nil {
		return ""
	}
	return strings.ToLower(strings.Trim(parsed.Hostname(), "."))
}

func hostMatchesAnyDomain(host string, domains []string) bool {
	host = strings.ToLower(strings.Trim(host, "."))
	for _, domain := range domains {
		domain = strings.ToLower(strings.Trim(domain, "."))
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

func publicSearchRelevanceTerms(query string) []string {
	query = strings.ToLower(compactWhitespace(query))
	if query == "" {
		return nil
	}
	fields := strings.FieldsFunc(query, func(r rune) bool {
		if r == '_' {
			return false
		}
		if r >= '0' && r <= '9' {
			return false
		}
		if r >= 'a' && r <= 'z' {
			return false
		}
		if r >= '\u4e00' && r <= '\u9fff' {
			return false
		}
		return true
	})
	seen := map[string]bool{}
	terms := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if len([]rune(field)) < 2 {
			continue
		}
		if isNumericOnlyTerm(field) {
			continue
		}
		addSearchTerm(&terms, seen, field)
		if containsCJK(field) && len([]rune(field)) > 4 {
			for _, gram := range cjkBigrams(field) {
				addSearchTerm(&terms, seen, gram)
			}
		}
	}
	return terms
}

func isNumericOnlyTerm(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isVagueWebSearchQuery(query string) bool {
	normalized := strings.ToLower(compactWhitespace(query))
	switch normalized {
	case "web", "search", "internet", "网页", "搜索", "搜索网页", "web search":
		return true
	default:
		return false
	}
}

func addSearchTerm(terms *[]string, seen map[string]bool, term string) {
	term = strings.TrimSpace(term)
	if len([]rune(term)) < 2 || seen[term] {
		return
	}
	seen[term] = true
	*terms = append(*terms, term)
}

func containsCJK(value string) bool {
	for _, r := range value {
		if r >= '\u4e00' && r <= '\u9fff' {
			return true
		}
	}
	return false
}

func cjkBigrams(value string) []string {
	runes := []rune(value)
	grams := make([]string, 0, len(runes))
	for i := 0; i+1 < len(runes); i++ {
		if !isCJK(runes[i]) || !isCJK(runes[i+1]) {
			continue
		}
		grams = append(grams, string(runes[i:i+2]))
	}
	return grams
}

func isCJK(r rune) bool {
	return r >= '\u4e00' && r <= '\u9fff'
}

func publicSearchResultScore(result publicSearchResult, terms []string) int {
	haystack := strings.ToLower(strings.Join([]string{result.Title, result.Snippet, result.URL}, " "))
	score := 0
	for _, term := range terms {
		if strings.Contains(haystack, term) {
			score++
		}
	}
	return score
}

func publicSearchFetchLimit(opts Options) int {
	maxOutputBytes := opts.MaxOutputBytes
	if maxOutputBytes <= 0 {
		maxOutputBytes = defaultMaxOutputBytes
	}
	limit := maxOutputBytes * 10
	if limit < 256000 {
		limit = 256000
	}
	if limit > 1000000 {
		limit = 1000000
	}
	return limit
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
