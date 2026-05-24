package http

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	nethttp "net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"runner/modules"
)

const (
	defaultTimeout       = 10 * time.Second
	defaultResponseLimit = 1 << 20
	maxResponseLimit     = 10 << 20
)

type Module struct {
	client *nethttp.Client
}

func New() *Module {
	return &Module{}
}

func NewWithClient(client *nethttp.Client) *Module {
	return &Module{client: client}
}

func (m *Module) Check(ctx context.Context, req modules.Request) (modules.Result, error) {
	cfg, err := readConfig(req)
	if err != nil {
		return modules.Result{}, err
	}
	return modules.Result{
		Changed: false,
		Diff: map[string]any{
			"method":          cfg.Method,
			"url":             redactURL(cfg.URL, cfg.Redactions),
			"expected_status": cfg.ExpectedStatus,
			"timeout":         cfg.Timeout.String(),
			"retry":           cfg.Retry,
			"response_limit":  cfg.ResponseLimit,
		},
	}, nil
}

func (m *Module) Apply(ctx context.Context, req modules.Request) (modules.Result, error) {
	cfg, err := readConfig(req)
	if err != nil {
		return modules.Result{}, err
	}
	client := m.client
	if client == nil {
		client = httpClient(cfg)
	}

	var last modules.Result
	var lastErr error
	attempts := cfg.Retry + 1
	for attempt := 1; attempt <= attempts; attempt++ {
		result, err := m.do(ctx, client, cfg, attempt)
		last = result
		lastErr = err
		if err == nil {
			return result, nil
		}
		if attempt < attempts {
			select {
			case <-ctx.Done():
				return result, ctx.Err()
			case <-time.After(retryBackoff(cfg, attempt)):
			}
		}
	}
	return last, lastErr
}

func (m *Module) Rollback(ctx context.Context, req modules.Request) (modules.Result, error) {
	return modules.Result{}, fmt.Errorf("http.request rollback not supported")
}

func (m *Module) do(ctx context.Context, client *nethttp.Client, cfg requestConfig, attempt int) (modules.Result, error) {
	body := bytes.NewReader(cfg.Body)
	httpReq, err := nethttp.NewRequestWithContext(ctx, cfg.Method, cfg.URL, body)
	if err != nil {
		return modules.Result{}, err
	}
	for k, values := range cfg.Headers {
		for _, value := range values {
			httpReq.Header.Add(k, value)
		}
	}

	start := time.Now()
	resp, err := client.Do(httpReq)
	elapsed := time.Since(start)
	if err != nil {
		safeErr := redactText(err.Error(), cfg.Redactions)
		output := map[string]any{
			"ok":          false,
			"attempt":     attempt,
			"elapsed_ms":  elapsed.Milliseconds(),
			"error":       safeErr,
			"request_url": redactURL(cfg.URL, cfg.Redactions),
		}
		return modules.Result{
			Changed: false,
			Output:  wrapHTTPOutput(output, "failed", "http.request failed", false, elapsed, cfg),
		}, fmt.Errorf("http.request failed: %s", safeErr)
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, int64(cfg.ResponseLimit)+1)
	respBody, readErr := io.ReadAll(limited)
	truncated := len(respBody) > cfg.ResponseLimit
	if truncated {
		respBody = respBody[:cfg.ResponseLimit]
	}
	bodyText := redactText(string(respBody), cfg.Redactions)
	headers := redactHeaders(resp.Header, cfg.Redactions)
	output := map[string]any{
		"ok":              statusMatches(resp.StatusCode, cfg.ExpectedStatus),
		"attempt":         attempt,
		"method":          cfg.Method,
		"url":             redactURL(cfg.URL, cfg.Redactions),
		"status_code":     resp.StatusCode,
		"status":          resp.Status,
		"headers":         headers,
		"body":            bodyText,
		"body_bytes":      len(respBody),
		"truncated":       truncated,
		"elapsed_ms":      elapsed.Milliseconds(),
		"expected_status": cfg.ExpectedStatus,
	}
	if cfg.JSONPath != "" {
		value, found, pathErr := extractJSONPath(respBody, cfg.JSONPath)
		output["json_path"] = cfg.JSONPath
		output["json_path_found"] = found
		if pathErr != nil {
			output["json_path_error"] = pathErr.Error()
		} else if found {
			output["json_path_value"] = redactAny(value, cfg.Redactions)
		}
	}
	if len(cfg.JSONPaths) > 0 {
		values := map[string]any{}
		found := map[string]bool{}
		errors := map[string]string{}
		for key, jsonPath := range cfg.JSONPaths {
			value, ok, pathErr := extractJSONPath(respBody, jsonPath)
			found[key] = ok
			if pathErr != nil {
				errors[key] = pathErr.Error()
				continue
			}
			if ok {
				values[key] = redactAny(value, cfg.Redactions)
			}
		}
		output["json_paths"] = values
		output["json_paths_found"] = found
		if len(errors) > 0 {
			output["json_paths_errors"] = errors
		}
	}
	status := "success"
	summary := fmt.Sprintf("http.request %s returned %d", cfg.Method, resp.StatusCode)
	if readErr != nil {
		output["error"] = readErr.Error()
		result := modules.Result{Changed: false, Output: wrapHTTPOutput(output, "failed", "http.request read response failed", false, elapsed, cfg)}
		return result, fmt.Errorf("http.request read response: %w", readErr)
	}
	if !statusMatches(resp.StatusCode, cfg.ExpectedStatus) {
		status = "failed"
		summary = fmt.Sprintf("http.request unexpected status %d", resp.StatusCode)
		result := modules.Result{Changed: false, Output: wrapHTTPOutput(output, status, summary, false, elapsed, cfg)}
		return result, fmt.Errorf("http.request unexpected status %d", resp.StatusCode)
	}
	result := modules.Result{Changed: false, Output: wrapHTTPOutput(output, status, summary, false, elapsed, cfg)}
	return result, nil
}

func wrapHTTPOutput(output map[string]any, status, summary string, changed bool, elapsed time.Duration, cfg requestConfig) map[string]any {
	return modules.WithResultEnvelope(output, modules.ResultEnvelopeOptions{
		Status:     status,
		Changed:    changed,
		Summary:    summary,
		Data:       modules.RedactAny(output, cfg.Redactions).(map[string]any),
		Evidence:   []map[string]any{{"type": "http", "source": "http.request", "method": cfg.Method, "url": redactURL(cfg.URL, cfg.Redactions)}},
		Redactions: cfg.Redactions,
		Mock:       cfg.Mock,
		Duration:   elapsed,
	})
}

type requestConfig struct {
	Method         string
	URL            string
	Headers        nethttp.Header
	Body           []byte
	ExpectedStatus []int
	JSONPath       string
	JSONPaths      map[string]string
	Retry          int
	RetryBackoff   time.Duration
	Timeout        time.Duration
	ResponseLimit  int
	Redactions     []string
	SSRFPolicy     ssrfPolicy
	Mock           bool
}

type ssrfPolicy struct {
	AllowPrivateNetworks bool
	FollowRedirect       bool
	AllowHostPatterns    []string
	AllowedCIDRs         []*net.IPNet
	BlockedCIDRs         []*net.IPNet
}

func readConfig(req modules.Request) (requestConfig, error) {
	if req.Step.Args == nil {
		return requestConfig{}, fmt.Errorf("http.request requires args.url")
	}
	rawURL, ok := readString(req.Step.Args, "url")
	if !ok || strings.TrimSpace(rawURL) == "" {
		return requestConfig{}, fmt.Errorf("http.request requires args.url")
	}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return requestConfig{}, fmt.Errorf("http.request requires absolute http(s) url")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return requestConfig{}, fmt.Errorf("http.request only supports http and https")
	}

	method, _ := readString(req.Step.Args, "method")
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = nethttp.MethodGet
	}
	secrets := readSecretProvider(req)
	secretRedactions := make([]string, 0, len(secrets))
	for _, value := range secrets {
		secretRedactions = append(secretRedactions, value)
	}
	headers, err := readHeaders(req.Step.Args["headers"], secrets)
	if err != nil {
		return requestConfig{}, err
	}
	if err := applyAuth(req.Step.Args, headers, secrets); err != nil {
		return requestConfig{}, err
	}
	body, err := readBody(firstPresent(req.Step.Args, "body", "body_json"))
	if err != nil {
		return requestConfig{}, err
	}
	expected, err := readExpectedStatus(req.Step.Args["expected_status"])
	if err != nil {
		return requestConfig{}, err
	}
	if len(expected) == 0 {
		expected = []int{200}
	}
	timeout := readTimeout(req.Step.Args, defaultTimeout)
	retry, retryBackoff := readRetry(req.Step.Args)
	if retry < 0 {
		retry = 0
	}
	if retryBackoff <= 0 {
		retryBackoff = 100 * time.Millisecond
	}
	limit := readIntAny(firstPresent(req.Step.Args, "response_limit", "max_response_bytes"), defaultResponseLimit)
	if limit <= 0 {
		limit = defaultResponseLimit
	}
	if limit > maxResponseLimit {
		limit = maxResponseLimit
	}
	redactions := readStringList(req.Step.Args["redaction"])
	redactions = append(redactions, readStringList(req.Step.Args["redactions"])...)
	redactions = append(redactions, secretRedactions...)
	policy, err := readSSRFPolicy(req.Step.Args)
	if err != nil {
		return requestConfig{}, err
	}
	return requestConfig{
		Method:         method,
		URL:            parsed.String(),
		Headers:        headers,
		Body:           body,
		ExpectedStatus: expected,
		JSONPath:       strings.TrimSpace(readStringDefault(req.Step.Args, "json_path", "")),
		JSONPaths:      readJSONPaths(req.Step.Args),
		Retry:          retry,
		RetryBackoff:   retryBackoff,
		Timeout:        timeout,
		ResponseLimit:  limit,
		Redactions:     compactStrings(redactions),
		SSRFPolicy:     policy,
		Mock:           modules.ReadMockFlag(req),
	}, nil
}

func retryBackoff(cfg requestConfig, attempt int) time.Duration {
	backoff := cfg.RetryBackoff
	if backoff <= 0 {
		backoff = 100 * time.Millisecond
	}
	if attempt < 1 {
		attempt = 1
	}
	return time.Duration(attempt) * backoff
}

func readTimeout(args map[string]any, fallback time.Duration) time.Duration {
	if raw := firstPresent(args, "timeout_ms"); raw != nil {
		ms := readIntAny(raw, 0)
		if ms > 0 {
			return time.Duration(ms) * time.Millisecond
		}
	}
	return readDuration(args, "timeout", fallback)
}

func readRetry(args map[string]any) (int, time.Duration) {
	raw := firstPresent(args, "retry", "retries")
	attempts := 0
	backoff := time.Duration(0)
	if record, ok := raw.(map[string]any); ok {
		attempts = readIntAny(firstPresent(record, "max_attempts", "attempts"), 0)
		if attempts > 0 {
			attempts--
		}
		if ms := readIntAny(firstPresent(record, "backoff_ms"), 0); ms > 0 {
			backoff = time.Duration(ms) * time.Millisecond
		}
		return attempts, backoff
	}
	return readIntAny(raw, 0), backoff
}

func httpClient(cfg requestConfig) *nethttp.Client {
	dialer := &net.Dialer{Timeout: cfg.Timeout}
	transport := &nethttp.Transport{
		Proxy: nethttp.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			if err := cfg.SSRFPolicy.ValidateHost(ctx, host); err != nil {
				return nil, err
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(host, port))
		},
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
	}
	client := &nethttp.Client{Timeout: cfg.Timeout, Transport: transport}
	client.CheckRedirect = func(req *nethttp.Request, via []*nethttp.Request) error {
		if !cfg.SSRFPolicy.FollowRedirect {
			return nethttp.ErrUseLastResponse
		}
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		if err := cfg.SSRFPolicy.ValidateHost(req.Context(), req.URL.Hostname()); err != nil {
			return err
		}
		return nil
	}
	return client
}

func readSSRFPolicy(args map[string]any) (ssrfPolicy, error) {
	policy := ssrfPolicy{
		AllowPrivateNetworks: readBool(args, "allow_private_networks", false),
		FollowRedirect:       readBool(args, "follow_redirect", true),
		AllowHostPatterns:    readStringList(firstPresent(args, "allow_host_patterns", "allowed_host_patterns", "allowed_hosts")),
	}
	if raw, ok := args["ssrf_policy"].(map[string]any); ok {
		policy.AllowPrivateNetworks = readBool(raw, "allow_private_networks", policy.AllowPrivateNetworks)
		policy.FollowRedirect = readBool(raw, "follow_redirect", policy.FollowRedirect)
		if patterns := readStringList(firstPresent(raw, "allow_host_patterns", "allowed_host_patterns", "allowed_hosts")); len(patterns) > 0 {
			policy.AllowHostPatterns = patterns
		}
		if cidrs := readStringList(firstPresent(raw, "allowed_cidrs", "allow_cidrs")); len(cidrs) > 0 {
			policy.AllowedCIDRs = nil
			for _, cidr := range cidrs {
				network, err := parseCIDR(cidr)
				if err != nil {
					return ssrfPolicy{}, err
				}
				policy.AllowedCIDRs = append(policy.AllowedCIDRs, network)
			}
		}
		if cidrs := readStringList(firstPresent(raw, "blocked_cidrs", "block_cidrs")); len(cidrs) > 0 {
			policy.BlockedCIDRs = nil
			for _, cidr := range cidrs {
				network, err := parseCIDR(cidr)
				if err != nil {
					return ssrfPolicy{}, err
				}
				policy.BlockedCIDRs = append(policy.BlockedCIDRs, network)
			}
		}
	}
	for _, cidr := range readStringList(firstPresent(args, "allowed_cidrs", "allow_cidrs")) {
		network, err := parseCIDR(cidr)
		if err != nil {
			return ssrfPolicy{}, err
		}
		policy.AllowedCIDRs = append(policy.AllowedCIDRs, network)
	}
	for _, cidr := range readStringList(firstPresent(args, "blocked_cidrs", "block_cidrs")) {
		network, err := parseCIDR(cidr)
		if err != nil {
			return ssrfPolicy{}, err
		}
		policy.BlockedCIDRs = append(policy.BlockedCIDRs, network)
	}
	policy.AllowHostPatterns = compactStrings(policy.AllowHostPatterns)
	return policy, nil
}

func parseCIDR(raw string) (*net.IPNet, error) {
	_, network, err := net.ParseCIDR(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("invalid cidr %q: %w", raw, err)
	}
	return network, nil
}

func (p ssrfPolicy) ValidateHost(ctx context.Context, host string) error {
	host = strings.Trim(strings.ToLower(strings.TrimSpace(host)), "[]")
	if host == "" {
		return fmt.Errorf("http.request requires redirect host")
	}
	hostPatternAllowed, err := p.hostAllowed(host)
	if err != nil {
		return err
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return err
	}
	if len(ips) == 0 {
		return fmt.Errorf("host %q resolved no addresses", host)
	}
	for _, addr := range ips {
		if ipInCIDRs(addr.IP, p.BlockedCIDRs) {
			return fmt.Errorf("blocked address %s for host %q by blocked_cidrs", addr.IP.String(), host)
		}
		if len(p.AllowHostPatterns) > 0 || len(p.AllowedCIDRs) > 0 {
			if hostPatternAllowed || ipInCIDRs(addr.IP, p.AllowedCIDRs) {
				continue
			}
			return fmt.Errorf("blocked host %q by ssrf allow policy", host)
		}
		if p.AllowPrivateNetworks {
			continue
		}
		if isBlockedIP(addr.IP) {
			return fmt.Errorf("blocked private address %s for host %q", addr.IP.String(), host)
		}
	}
	return nil
}

func (p ssrfPolicy) hostAllowed(host string) (bool, error) {
	for _, pattern := range p.AllowHostPatterns {
		pattern = strings.ToLower(strings.TrimSpace(pattern))
		if pattern == "" {
			continue
		}
		matched, err := path.Match(pattern, host)
		if err != nil {
			return false, fmt.Errorf("invalid allow_host_patterns entry %q: %w", pattern, err)
		}
		if matched || host == pattern {
			return true, nil
		}
	}
	return false, nil
}

func ipInCIDRs(ip net.IP, cidrs []*net.IPNet) bool {
	for _, cidr := range cidrs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	return false
}

func readHeaders(raw any, secrets map[string]string) (nethttp.Header, error) {
	headers := nethttp.Header{}
	switch v := raw.(type) {
	case nil:
	case map[string]string:
		for k, value := range v {
			headers.Set(k, value)
		}
	case map[string]any:
		for k, value := range v {
			switch typed := value.(type) {
			case map[string]any:
				resolved, err := readSecretValue(typed, secrets)
				if err != nil {
					return nil, err
				}
				headers.Set(k, resolved)
			case []any:
				for _, item := range typed {
					headers.Add(k, fmt.Sprint(item))
				}
			case []string:
				for _, item := range typed {
					headers.Add(k, item)
				}
			default:
				headers.Set(k, fmt.Sprint(value))
			}
		}
	default:
		return nil, fmt.Errorf("http.request headers must be a map")
	}
	return headers, nil
}

func applyAuth(args map[string]any, headers nethttp.Header, secrets map[string]string) error {
	raw, ok := args["auth"].(map[string]any)
	if !ok {
		return nil
	}
	authType := strings.ToLower(strings.TrimSpace(fmt.Sprint(firstPresent(raw, "type", "scheme"))))
	switch authType {
	case "", "none":
		return nil
	case "bearer":
		token, err := readSecretValue(raw, secrets)
		if err != nil {
			return err
		}
		headers.Set("Authorization", "Bearer "+token)
	case "basic":
		username := fmt.Sprint(firstPresent(raw, "username", "user"))
		password, err := readSecretValue(raw, secrets)
		if err != nil {
			return err
		}
		req := &nethttp.Request{Header: headers}
		req.SetBasicAuth(username, password)
	case "header":
		name := strings.TrimSpace(fmt.Sprint(firstPresent(raw, "name", "header", "key")))
		if name == "" {
			return fmt.Errorf("http.request auth header requires name")
		}
		value, err := readSecretValue(raw, secrets)
		if err != nil {
			return err
		}
		headers.Set(name, value)
	default:
		return fmt.Errorf("unsupported http.request auth type %q", authType)
	}
	return nil
}

func readSecretProvider(req modules.Request) map[string]string {
	secrets := map[string]string{}
	mergeSecrets(secrets, req.Vars["secrets"])
	mergeSecrets(secrets, req.Vars["secret"])
	mergeSecrets(secrets, req.Step.Args["secrets"])
	return secrets
}

func mergeSecrets(out map[string]string, raw any) {
	switch v := raw.(type) {
	case map[string]string:
		for key, value := range v {
			out[key] = value
		}
	case map[string]any:
		for key, value := range v {
			if value != nil {
				out[key] = fmt.Sprint(value)
			}
		}
	}
}

func readSecretValue(record map[string]any, secrets map[string]string) (string, error) {
	if value := firstPresent(record, "value", "token", "password"); value != nil {
		return fmt.Sprint(value), nil
	}
	ref := strings.TrimSpace(fmt.Sprint(firstPresent(record, "secret_ref", "token_secret_ref", "password_secret_ref", "value_secret_ref")))
	if ref == "" || ref == "<nil>" {
		return "", fmt.Errorf("http.request secret_ref is required")
	}
	value, ok := secrets[ref]
	if !ok {
		return "", fmt.Errorf("http.request secret_ref %q not found", ref)
	}
	return value, nil
}

func readBody(raw any) ([]byte, error) {
	switch v := raw.(type) {
	case nil:
		return nil, nil
	case string:
		return []byte(v), nil
	case []byte:
		return append([]byte{}, v...), nil
	default:
		body, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("http.request body must be string, bytes, or json value: %w", err)
		}
		return body, nil
	}
}

func readExpectedStatus(raw any) ([]int, error) {
	switch v := raw.(type) {
	case nil:
		return nil, nil
	case int:
		return []int{v}, nil
	case int64:
		return []int{int(v)}, nil
	case float64:
		return []int{int(v)}, nil
	case string:
		if strings.TrimSpace(v) == "" {
			return nil, nil
		}
		parts := strings.Split(v, ",")
		out := make([]int, 0, len(parts))
		for _, part := range parts {
			var n int
			if _, err := fmt.Sscanf(strings.TrimSpace(part), "%d", &n); err != nil {
				return nil, fmt.Errorf("invalid expected_status %q", part)
			}
			out = append(out, n)
		}
		return out, nil
	case []int:
		return append([]int{}, v...), nil
	case []any:
		out := make([]int, 0, len(v))
		for _, item := range v {
			n, ok := anyInt(item)
			if !ok {
				return nil, fmt.Errorf("expected_status entries must be integers")
			}
			out = append(out, n)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("expected_status must be integer or list")
	}
}

func statusMatches(status int, expected []int) bool {
	for _, value := range expected {
		if status == value {
			return true
		}
	}
	return false
}

func extractJSONPath(body []byte, path string) (any, bool, error) {
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return nil, false, err
	}
	segments, err := parseJSONPath(path)
	if err != nil {
		return nil, false, err
	}
	if len(segments) == 0 {
		return value, true, nil
	}
	current := []any{value}
	for _, segment := range segments {
		next := []any{}
		for _, item := range current {
			values, err := segment.apply(item)
			if err != nil {
				return nil, false, err
			}
			next = append(next, values...)
		}
		if len(next) == 0 {
			return nil, false, nil
		}
		current = next
	}
	if len(current) == 1 {
		return current[0], true, nil
	}
	return current, true, nil
}

type jsonPathSegment struct {
	Key       string
	Index     *int
	FilterKey string
	FilterVal string
}

func (s jsonPathSegment) apply(value any) ([]any, error) {
	current := value
	if s.Key != "" {
		obj, ok := current.(map[string]any)
		if !ok {
			return nil, nil
		}
		var exists bool
		current, exists = obj[s.Key]
		if !exists {
			return nil, nil
		}
	}
	if s.Index != nil {
		arr, ok := current.([]any)
		if !ok || *s.Index < 0 || *s.Index >= len(arr) {
			return nil, nil
		}
		current = arr[*s.Index]
	}
	if s.FilterKey != "" {
		arr, ok := current.([]any)
		if !ok {
			return nil, nil
		}
		out := []any{}
		for _, item := range arr {
			obj, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if fmt.Sprint(obj[s.FilterKey]) == s.FilterVal {
				out = append(out, item)
			}
		}
		return out, nil
	}
	return []any{current}, nil
}

func parseJSONPath(raw string) ([]jsonPathSegment, error) {
	path := strings.TrimSpace(raw)
	path = strings.TrimPrefix(path, "$")
	path = strings.TrimPrefix(path, ".")
	if path == "" {
		return nil, nil
	}
	parts := splitJSONPath(path)
	segments := make([]jsonPathSegment, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		segment, err := parsePathPart(part)
		if err != nil {
			return nil, err
		}
		segments = append(segments, segment)
	}
	return segments, nil
}

func splitJSONPath(path string) []string {
	parts := []string{}
	start := 0
	depth := 0
	for i, r := range path {
		switch r {
		case '[':
			depth++
		case ']':
			if depth > 0 {
				depth--
			}
		case '.':
			if depth == 0 {
				parts = append(parts, path[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, path[start:])
}

func parsePathPart(part string) (jsonPathSegment, error) {
	open := strings.Index(part, "[")
	if open < 0 {
		return jsonPathSegment{Key: part}, nil
	}
	close := strings.TrimSuffix(part[open+1:], "]")
	if close == part[open+1:] {
		return jsonPathSegment{}, fmt.Errorf("invalid json_path segment %q", part)
	}
	segment := jsonPathSegment{Key: part[:open]}
	if strings.HasPrefix(close, "?(") && strings.HasSuffix(close, ")") {
		filterKey, filterVal, err := parseFilterPredicate(close[2 : len(close)-1])
		if err != nil {
			return jsonPathSegment{}, err
		}
		segment.FilterKey = filterKey
		segment.FilterVal = filterVal
		return segment, nil
	}
	var index int
	if _, err := fmt.Sscanf(close, "%d", &index); err != nil {
		return jsonPathSegment{}, fmt.Errorf("invalid json_path index %q", close)
	}
	segment.Index = &index
	return segment, nil
}

func parseFilterPredicate(raw string) (string, string, error) {
	predicate := strings.TrimSpace(raw)
	parts := strings.SplitN(predicate, "==", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid json_path filter %q", raw)
	}
	left := strings.TrimSpace(parts[0])
	if !strings.HasPrefix(left, "@.") {
		return "", "", fmt.Errorf("invalid json_path filter key %q", left)
	}
	key := strings.TrimSpace(strings.TrimPrefix(left, "@."))
	value := strings.TrimSpace(parts[1])
	value = strings.Trim(value, `"'`)
	if key == "" {
		return "", "", fmt.Errorf("invalid json_path filter key %q", left)
	}
	return key, value, nil
}

func readJSONPaths(args map[string]any) map[string]string {
	raw := firstPresent(args, "json_paths", "output.json_paths")
	if output, ok := args["output"].(map[string]any); ok {
		raw = firstPresent(output, "json_paths")
	}
	switch v := raw.(type) {
	case nil:
		return nil
	case map[string]string:
		out := map[string]string{}
		for key, value := range v {
			if strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
				out[key] = strings.TrimSpace(value)
			}
		}
		return out
	case map[string]any:
		out := map[string]string{}
		for key, value := range v {
			path := strings.TrimSpace(fmt.Sprint(value))
			if strings.TrimSpace(key) != "" && path != "" {
				out[key] = path
			}
		}
		return out
	default:
		return nil
	}
}

func redactHeaders(headers nethttp.Header, redactions []string) map[string]any {
	out := map[string]any{}
	for key, values := range headers {
		copied := make([]string, 0, len(values))
		for _, value := range values {
			copied = append(copied, redactText(value, redactions))
		}
		if isSensitiveKey(key) {
			out[key] = "[REDACTED]"
		} else {
			out[key] = copied
		}
	}
	return out
}

func redactAny(value any, redactions []string) any {
	return modules.RedactAny(value, redactions)
}

func redactURL(raw string, redactions []string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return redactText(raw, redactions)
	}
	if parsed.User != nil {
		parsed.User = url.User("[REDACTED]")
	}
	return redactText(parsed.String(), redactions)
}

func redactText(text string, redactions []string) string {
	return modules.RedactText(text, redactions)
}

func isSensitiveKey(key string) bool {
	return modules.IsSensitiveKey(key)
}

func readString(args map[string]any, key string) (string, bool) {
	raw, ok := args[key]
	if !ok || raw == nil {
		return "", false
	}
	return fmt.Sprint(raw), true
}

func readStringDefault(args map[string]any, key, fallback string) string {
	if value, ok := readString(args, key); ok {
		return value
	}
	return fallback
}

func readDuration(args map[string]any, key string, fallback time.Duration) time.Duration {
	raw, ok := args[key]
	if !ok || raw == nil {
		return fallback
	}
	if n, ok := anyInt(raw); ok {
		if n <= 0 {
			return fallback
		}
		return time.Duration(n) * time.Second
	}
	parsed, err := time.ParseDuration(fmt.Sprint(raw))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func readInt(args map[string]any, key string, fallback int) int {
	raw, ok := args[key]
	if !ok || raw == nil {
		return fallback
	}
	return readIntAny(raw, fallback)
}

func readIntAny(raw any, fallback int) int {
	if n, ok := anyInt(raw); ok {
		return n
	}
	var out int
	if _, err := fmt.Sscanf(fmt.Sprint(raw), "%d", &out); err == nil {
		return out
	}
	return fallback
}

func firstPresent(args map[string]any, keys ...string) any {
	for _, key := range keys {
		if raw, ok := args[key]; ok && raw != nil {
			return raw
		}
	}
	return nil
}

func readBool(args map[string]any, key string, fallback bool) bool {
	raw, ok := args[key]
	if !ok || raw == nil {
		return fallback
	}
	switch v := raw.(type) {
	case bool:
		return v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return fallback
}

func readStringList(raw any) []string {
	switch v := raw.(type) {
	case nil:
		return nil
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return []string{v}
	case []string:
		return append([]string{}, v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, fmt.Sprint(item))
		}
		return out
	default:
		return []string{fmt.Sprint(v)}
	}
}

func compactStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func anyInt(raw any) (int, bool) {
	switch v := raw.(type) {
	case int:
		return v, true
	case int8:
		return int(v), true
	case int16:
		return int(v), true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case uint:
		if v > math.MaxInt {
			return 0, false
		}
		return int(v), true
	case uint8:
		return int(v), true
	case uint16:
		return int(v), true
	case uint32:
		return int(v), true
	case uint64:
		if v > math.MaxInt {
			return 0, false
		}
		return int(v), true
	case float64:
		return int(v), true
	case float32:
		return int(v), true
	default:
		return 0, false
	}
}
