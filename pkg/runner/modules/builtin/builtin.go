package builtin

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	nethttp "net/http"
	"net/url"
	"strings"
	"time"

	"runner/modules"
)

const defaultTimeout = 5 * time.Second

type Module struct {
	kind string
}

func NewTCPPing() *Module {
	return &Module{kind: "tcp_ping"}
}

func NewHTTPCheck() *Module {
	return &Module{kind: "http_check"}
}

func NewSSLExpiryCheck() *Module {
	return &Module{kind: "ssl_expiry_check"}
}

func NewDNSResolve() *Module {
	return &Module{kind: "dns_resolve"}
}

func NewICMPPingPlaceholder() *Module {
	return &Module{kind: "icmp_ping"}
}

func (m *Module) Check(ctx context.Context, req modules.Request) (modules.Result, error) {
	return modules.Result{
		Changed: false,
		Diff: map[string]any{
			"kind": m.kind,
			"args": req.Step.Args,
		},
	}, nil
}

func (m *Module) Apply(ctx context.Context, req modules.Request) (modules.Result, error) {
	switch m.kind {
	case "tcp_ping":
		return tcpPing(ctx, req)
	case "http_check":
		return httpCheck(ctx, req)
	case "ssl_expiry_check":
		return sslExpiryCheck(ctx, req)
	case "dns_resolve":
		return dnsResolve(ctx, req)
	case "icmp_ping":
		return modules.Result{}, fmt.Errorf("builtin.icmp_ping capability placeholder is not implemented")
	default:
		return modules.Result{}, fmt.Errorf("unsupported builtin module %q", m.kind)
	}
}

func (m *Module) Rollback(ctx context.Context, req modules.Request) (modules.Result, error) {
	return modules.Result{}, fmt.Errorf("builtin.%s rollback not supported", m.kind)
}

func tcpPing(ctx context.Context, req modules.Request) (modules.Result, error) {
	host, port, err := readHostPort(req, "host", "port", "")
	if err != nil {
		return modules.Result{}, err
	}
	timeout := readDuration(req.Step.Args, "timeout", defaultTimeout)
	dialer := &net.Dialer{Timeout: timeout}
	start := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	elapsed := time.Since(start)
	remoteAddr := ""
	if err == nil && conn != nil && conn.RemoteAddr() != nil {
		remoteAddr = conn.RemoteAddr().String()
	}
	output := map[string]any{
		"ok":          err == nil,
		"reachable":   err == nil,
		"host":        host,
		"port":        port,
		"address":     net.JoinHostPort(host, port),
		"remote_addr": remoteAddr,
		"elapsed_ms":  elapsed.Milliseconds(),
		"latency_ms":  elapsed.Milliseconds(),
	}
	if err != nil {
		output["error"] = err.Error()
		return modules.Result{Changed: false, Output: wrapBuiltinOutput(output, "failed", "builtin.tcp_ping failed", "builtin.tcp_ping", elapsed, req)}, fmt.Errorf("builtin.tcp_ping failed: %w", err)
	}
	_ = conn.Close()
	return modules.Result{Changed: false, Output: wrapBuiltinOutput(output, "success", "tcp port is reachable", "builtin.tcp_ping", elapsed, req)}, nil
}

func httpCheck(ctx context.Context, req modules.Request) (modules.Result, error) {
	rawURL, ok := readString(req.Step.Args, "url")
	if !ok || strings.TrimSpace(rawURL) == "" {
		host, port, err := readHostPort(req, "host", "port", "80")
		if err != nil {
			return modules.Result{}, fmt.Errorf("builtin.http_check requires args.url or host/port: %w", err)
		}
		rawURL = "http://" + net.JoinHostPort(host, port)
	}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return modules.Result{}, fmt.Errorf("builtin.http_check requires absolute http(s) url")
	}
	method := strings.ToUpper(strings.TrimSpace(readStringDefault(req.Step.Args, "method", nethttp.MethodGet)))
	expected := readExpectedStatuses(req.Step.Args["expected_status"], []int{200})
	timeout := readDuration(req.Step.Args, "timeout", defaultTimeout)
	transport := nethttp.DefaultTransport
	if parsed.Scheme == "https" && readBool(req.Step.Args, "insecure_skip_verify", false) {
		transport = &nethttp.Transport{TLSClientConfig: &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: true,
		}}
	}
	client := &nethttp.Client{Timeout: timeout, Transport: transport}
	httpReq, err := nethttp.NewRequestWithContext(ctx, method, parsed.String(), nil)
	if err != nil {
		return modules.Result{}, err
	}
	start := time.Now()
	resp, err := client.Do(httpReq)
	elapsed := time.Since(start)
	output := map[string]any{
		"ok":              false,
		"matched":         false,
		"method":          method,
		"url":             parsed.String(),
		"expected_status": expected,
		"elapsed_ms":      elapsed.Milliseconds(),
		"latency_ms":      elapsed.Milliseconds(),
	}
	if err != nil {
		output["error"] = err.Error()
		return modules.Result{Changed: false, Output: wrapBuiltinOutput(output, "failed", "builtin.http_check failed", "builtin.http_check", elapsed, req)}, fmt.Errorf("builtin.http_check failed: %w", err)
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4097))
	truncated := len(body) > 4096
	if truncated {
		body = body[:4096]
	}
	bodyText := string(body)
	output["status_code"] = resp.StatusCode
	output["status"] = resp.Status
	output["body_excerpt"] = bodyText
	output["truncated"] = truncated
	if readErr != nil {
		output["error"] = readErr.Error()
		return modules.Result{Changed: false, Output: wrapBuiltinOutput(output, "failed", "builtin.http_check read response failed", "builtin.http_check", elapsed, req)}, fmt.Errorf("builtin.http_check read response: %w", readErr)
	}
	statusOK := statusMatches(resp.StatusCode, expected)
	bodyOK := true
	if contains := strings.TrimSpace(readStringDefault(req.Step.Args, "body_contains", "")); contains != "" {
		bodyOK = strings.Contains(bodyText, contains)
		output["body_contains"] = contains
		output["body_contains_matched"] = bodyOK
	}
	jsonPathOK := true
	if path := strings.TrimSpace(readStringDefault(req.Step.Args, "json_path", "")); path != "" {
		value, found, pathErr := extractJSONPath(body, path)
		jsonPathOK = found && pathErr == nil
		output["json_path"] = path
		output["json_path_found"] = found
		if pathErr != nil {
			output["json_path_error"] = pathErr.Error()
		} else if found {
			output["json_path_value"] = value
		}
	}
	matched := statusOK && bodyOK && jsonPathOK
	output["ok"] = matched
	output["matched"] = matched
	if !statusOK {
		return modules.Result{Changed: false, Output: wrapBuiltinOutput(output, "failed", fmt.Sprintf("builtin.http_check unexpected status %d", resp.StatusCode), "builtin.http_check", elapsed, req)}, fmt.Errorf("builtin.http_check unexpected status %d", resp.StatusCode)
	}
	if !bodyOK {
		return modules.Result{Changed: false, Output: wrapBuiltinOutput(output, "failed", "builtin.http_check body does not contain expected text", "builtin.http_check", elapsed, req)}, fmt.Errorf("builtin.http_check body does not contain expected text")
	}
	if !jsonPathOK {
		return modules.Result{Changed: false, Output: wrapBuiltinOutput(output, "failed", "builtin.http_check json_path did not match", "builtin.http_check", elapsed, req)}, fmt.Errorf("builtin.http_check json_path did not match")
	}
	return modules.Result{Changed: false, Output: wrapBuiltinOutput(output, "success", "http check matched expectations", "builtin.http_check", elapsed, req)}, nil
}

func sslExpiryCheck(ctx context.Context, req modules.Request) (modules.Result, error) {
	host, port, err := readHostPort(req, "host", "port", "443")
	if err != nil {
		return modules.Result{}, err
	}
	serverName := strings.TrimSpace(readStringDefault(req.Step.Args, "server_name", host))
	timeout := readDuration(req.Step.Args, "timeout", defaultTimeout)
	minDays := readIntAny(firstPresent(req.Step.Args, "min_days", "warn_days"), 0)
	dialer := &net.Dialer{Timeout: timeout}
	cfg := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		ServerName:         serverName,
		InsecureSkipVerify: readBool(req.Step.Args, "insecure_skip_verify", false),
	}
	start := time.Now()
	conn, err := tls.DialWithDialer(dialer, "tcp", net.JoinHostPort(host, port), cfg)
	elapsed := time.Since(start)
	output := map[string]any{
		"ok":          false,
		"host":        host,
		"port":        port,
		"server_name": serverName,
		"elapsed_ms":  elapsed.Milliseconds(),
	}
	if err != nil {
		output["error"] = err.Error()
		return modules.Result{Changed: false, Output: wrapBuiltinOutput(output, "failed", "builtin.ssl_expiry_check failed", "builtin.ssl_expiry_check", elapsed, req)}, fmt.Errorf("builtin.ssl_expiry_check failed: %w", err)
	}
	defer conn.Close()
	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		output["error"] = "peer certificate not found"
		return modules.Result{Changed: false, Output: wrapBuiltinOutput(output, "failed", "builtin.ssl_expiry_check peer certificate not found", "builtin.ssl_expiry_check", elapsed, req)}, fmt.Errorf("builtin.ssl_expiry_check peer certificate not found")
	}
	cert := state.PeerCertificates[0]
	now := time.Now()
	days := int(time.Until(cert.NotAfter).Hours() / 24)
	ok := now.Before(cert.NotAfter) && (minDays <= 0 || days >= minDays)
	output["ok"] = ok
	output["subject"] = cert.Subject.CommonName
	output["issuer"] = cert.Issuer.CommonName
	output["not_before"] = cert.NotBefore.UTC().Format(time.RFC3339)
	output["not_after"] = cert.NotAfter.UTC().Format(time.RFC3339)
	output["days_remaining"] = days
	output["min_days"] = minDays
	if !ok {
		return modules.Result{Changed: false, Output: wrapBuiltinOutput(output, "failed", fmt.Sprintf("builtin.ssl_expiry_check certificate expires in %d days", days), "builtin.ssl_expiry_check", elapsed, req)}, fmt.Errorf("builtin.ssl_expiry_check certificate expires in %d days", days)
	}
	return modules.Result{Changed: false, Output: wrapBuiltinOutput(output, "success", "certificate is valid beyond threshold", "builtin.ssl_expiry_check", elapsed, req)}, nil
}

func dnsResolve(ctx context.Context, req modules.Request) (modules.Result, error) {
	name := strings.TrimSpace(readStringDefault(req.Step.Args, "name", ""))
	if name == "" {
		name = strings.TrimSpace(readStringDefault(req.Step.Args, "host", req.Host.Address))
	}
	if name == "" {
		name = strings.TrimSpace(req.Host.Name)
	}
	if name == "" {
		return modules.Result{}, fmt.Errorf("builtin.dns_resolve requires args.name or host address")
	}
	recordType := strings.ToUpper(strings.TrimSpace(readStringDefault(req.Step.Args, "record_type", "A")))
	if recordType == "" {
		recordType = "A"
	}
	timeout := readDuration(req.Step.Args, "timeout", defaultTimeout)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	records, err := lookupRecords(ctx, name, recordType)
	elapsed := time.Since(start)
	output := map[string]any{
		"ok":               err == nil,
		"name":             name,
		"host":             name,
		"record_type":      recordType,
		"records":          records,
		"addresses":        records,
		"ips":              records,
		"matched_expected": false,
		"resolver":         "default",
		"elapsed_ms":       elapsed.Milliseconds(),
		"latency_ms":       elapsed.Milliseconds(),
	}
	if expected := readStringList(req.Step.Args["expected"]); len(expected) > 0 {
		output["expected"] = expected
		output["matched_expected"] = recordsContainAll(records, expected)
	}
	if err != nil {
		output["error"] = err.Error()
		return modules.Result{Changed: false, Output: wrapBuiltinOutput(output, "failed", "builtin.dns_resolve failed", "builtin.dns_resolve", elapsed, req)}, fmt.Errorf("builtin.dns_resolve failed: %w", err)
	}
	return modules.Result{Changed: false, Output: wrapBuiltinOutput(output, "success", "dns records resolved", "builtin.dns_resolve", elapsed, req)}, nil
}

func wrapBuiltinOutput(output map[string]any, status, summary, source string, elapsed time.Duration, req modules.Request) map[string]any {
	redactions := modules.ReadRedactionRules(req)
	return modules.WithResultEnvelope(output, modules.ResultEnvelopeOptions{
		Status:     status,
		Changed:    false,
		Summary:    summary,
		Data:       modules.RedactAny(output, redactions).(map[string]any),
		Evidence:   []map[string]any{{"type": "probe", "source": source}},
		Redactions: redactions,
		Mock:       modules.ReadMockFlag(req),
		Duration:   elapsed,
	})
}

func recordsContainAll(records, expected []string) bool {
	recordSet := map[string]struct{}{}
	for _, record := range records {
		recordSet[strings.TrimSpace(record)] = struct{}{}
	}
	for _, item := range expected {
		if _, ok := recordSet[strings.TrimSpace(item)]; !ok {
			return false
		}
	}
	return true
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
	path = strings.TrimSpace(strings.TrimPrefix(path, "$"))
	path = strings.TrimPrefix(path, ".")
	if path == "" {
		return value, true, nil
	}
	current := value
	for _, part := range strings.Split(path, ".") {
		if part == "" {
			continue
		}
		key, index, hasIndex, err := parsePathPart(part)
		if err != nil {
			return nil, false, err
		}
		if key != "" {
			obj, ok := current.(map[string]any)
			if !ok {
				return nil, false, nil
			}
			current, ok = obj[key]
			if !ok {
				return nil, false, nil
			}
		}
		if hasIndex {
			arr, ok := current.([]any)
			if !ok || index < 0 || index >= len(arr) {
				return nil, false, nil
			}
			current = arr[index]
		}
	}
	return current, true, nil
}

func parsePathPart(part string) (string, int, bool, error) {
	open := strings.Index(part, "[")
	if open < 0 {
		return part, 0, false, nil
	}
	close := strings.TrimSuffix(part[open+1:], "]")
	if close == part[open+1:] {
		return "", 0, false, fmt.Errorf("invalid json_path segment %q", part)
	}
	var index int
	if _, err := fmt.Sscanf(close, "%d", &index); err != nil {
		return "", 0, false, fmt.Errorf("invalid json_path index %q", close)
	}
	return part[:open], index, true, nil
}

func lookupRecords(ctx context.Context, name, recordType string) ([]string, error) {
	switch recordType {
	case "A", "AAAA":
		addrs, err := net.DefaultResolver.LookupIP(ctx, "ip", name)
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, len(addrs))
		for _, addr := range addrs {
			if recordType == "A" && addr.To4() == nil {
				continue
			}
			if recordType == "AAAA" && (addr.To4() != nil || addr.To16() == nil) {
				continue
			}
			out = append(out, addr.String())
		}
		return out, nil
	case "CNAME":
		cname, err := net.DefaultResolver.LookupCNAME(ctx, name)
		if err != nil {
			return nil, err
		}
		return []string{cname}, nil
	case "TXT":
		return net.DefaultResolver.LookupTXT(ctx, name)
	case "MX":
		mx, err := net.DefaultResolver.LookupMX(ctx, name)
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, len(mx))
		for _, record := range mx {
			out = append(out, fmt.Sprintf("%d %s", record.Pref, record.Host))
		}
		return out, nil
	case "NS":
		ns, err := net.DefaultResolver.LookupNS(ctx, name)
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, len(ns))
		for _, record := range ns {
			out = append(out, record.Host)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported dns record_type %q", recordType)
	}
}

func readHostPort(req modules.Request, hostKey, portKey, defaultPort string) (string, string, error) {
	host := strings.TrimSpace(readStringDefault(req.Step.Args, hostKey, req.Host.Address))
	if host == "" {
		host = strings.TrimSpace(req.Host.Name)
	}
	port := strings.TrimSpace(readStringDefault(req.Step.Args, portKey, defaultPort))
	if h, p, err := net.SplitHostPort(host); err == nil {
		host = h
		if port == "" || port == defaultPort {
			port = p
		}
	}
	if host == "" {
		return "", "", fmt.Errorf("host is required")
	}
	if port == "" {
		return "", "", fmt.Errorf("port is required")
	}
	return host, port, nil
}

func readString(args map[string]any, key string) (string, bool) {
	if args == nil {
		return "", false
	}
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
	if n, ok := anyInt(raw); ok && n > 0 {
		return time.Duration(n) * time.Second
	}
	parsed, err := time.ParseDuration(fmt.Sprint(raw))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func readExpectedStatuses(raw any, fallback []int) []int {
	switch v := raw.(type) {
	case nil:
		return append([]int{}, fallback...)
	case int:
		return []int{v}
	case int64:
		return []int{int(v)}
	case float64:
		return []int{int(v)}
	case string:
		if strings.TrimSpace(v) == "" {
			return append([]int{}, fallback...)
		}
		parts := strings.Split(v, ",")
		out := make([]int, 0, len(parts))
		for _, part := range parts {
			if n := readIntAny(strings.TrimSpace(part), 0); n > 0 {
				out = append(out, n)
			}
		}
		if len(out) == 0 {
			return append([]int{}, fallback...)
		}
		return out
	case []int:
		if len(v) == 0 {
			return append([]int{}, fallback...)
		}
		return append([]int{}, v...)
	case []any:
		out := make([]int, 0, len(v))
		for _, item := range v {
			if n, ok := anyInt(item); ok {
				out = append(out, n)
			}
		}
		if len(out) == 0 {
			return append([]int{}, fallback...)
		}
		return out
	default:
		if n := readIntAny(raw, 0); n > 0 {
			return []int{n}
		}
		return append([]int{}, fallback...)
	}
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
