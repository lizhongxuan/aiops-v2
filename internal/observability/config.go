package observability

import "strings"

type Config struct {
	Enabled       bool
	Endpoint      string
	ServiceName   string
	Project       string
	IncludePrompt bool
}

func ConfigFromEnv(getenv func(string) string) Config {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	return Config{
		Enabled:       parseBool(getenv("AIOPS_OTEL_ENABLED")),
		Endpoint:      envOrDefault(getenv, "AIOPS_OTEL_ENDPOINT", "http://localhost:6006/v1/traces"),
		ServiceName:   envOrDefault(getenv, "AIOPS_OTEL_SERVICE_NAME", "aiops-v2-agent"),
		Project:       strings.TrimSpace(getenv("AIOPS_OTEL_PROJECT")),
		IncludePrompt: parseBool(getenv("AIOPS_OTEL_INCLUDE_PROMPT")),
	}
}

func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "t", "true", "y", "yes", "on", "enabled":
		return true
	default:
		return false
	}
}

func envOrDefault(getenv func(string) string, key string, fallback string) string {
	value := strings.TrimSpace(getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
