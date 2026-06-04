package gateway

import (
	"net/url"
	"strings"

	"aiops-v2/internal/mcp"
)

const EndpointTypeStreamableHTTP = "streamable_http"

type ServerConfigV2 struct {
	ID          string          `json:"id"`
	Name        string          `json:"name,omitempty"`
	Disabled    bool            `json:"disabled,omitempty"`
	Source      string          `json:"source,omitempty"`
	TenantScope mcp.TenantScope `json:"tenantScope,omitempty"`
	UserScope   mcp.UserScope   `json:"userScope,omitempty"`
	Profiles    []string        `json:"profiles,omitempty"`
	Endpoint    *EndpointConfig `json:"endpoint,omitempty"`
	Stdio       *StdioConfig    `json:"stdio,omitempty"`
}

type EndpointConfig struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type StdioConfig struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	Env     []string `json:"env,omitempty"`
}

type MigrationReport struct {
	Warnings []MigrationWarning `json:"warnings,omitempty"`
	Errors   []MigrationError   `json:"errors,omitempty"`
}

type MigrationWarning struct {
	ServerID string `json:"serverId,omitempty"`
	Message  string `json:"message"`
}

type MigrationError struct {
	ServerID string `json:"serverId,omitempty"`
	Message  string `json:"message"`
}

func (r MigrationReport) HasWarning() bool {
	return len(r.Warnings) > 0
}

func (r MigrationReport) HasError() bool {
	return len(r.Errors) > 0
}

func MigrateServerConfig(cfg mcp.ServerConfig) (ServerConfigV2, MigrationReport) {
	migrated := ServerConfigV2{
		ID:       cfg.ID,
		Name:     cfg.Name,
		Disabled: cfg.Disabled,
		Source:   cfg.Source,
		TenantScope: mcp.TenantScope{
			TenantIDs: append([]string(nil), cfg.TenantScope.TenantIDs...),
		},
		UserScope: mcp.UserScope{
			UserIDs: append([]string(nil), cfg.UserScope.UserIDs...),
		},
		Profiles: append([]string(nil), cfg.Profiles...),
	}
	var report MigrationReport

	command, args := splitCommand(cfg.Command)
	if command == "" {
		report.Warnings = append(report.Warnings, MigrationWarning{
			ServerID: cfg.ID,
			Message:  "missing endpoint or command",
		})
		return migrated, report
	}

	if isHTTPURL(command) {
		migrated.Endpoint = &EndpointConfig{
			Type: EndpointTypeStreamableHTTP,
			URL:  command,
		}
		return migrated, report
	}

	migrated.Stdio = &StdioConfig{
		Command: command,
		Args:    args,
	}
	return migrated, report
}

func MigrateServerConfigs(configs []mcp.ServerConfig) ([]ServerConfigV2, MigrationReport) {
	migrated := make([]ServerConfigV2, 0, len(configs))
	var report MigrationReport
	for _, cfg := range configs {
		next, nextReport := MigrateServerConfig(cfg)
		migrated = append(migrated, next)
		report.Warnings = append(report.Warnings, nextReport.Warnings...)
	}
	return migrated, report
}

func splitCommand(command []string) (string, []string) {
	if len(command) == 0 {
		return "", nil
	}
	executable := strings.TrimSpace(command[0])
	if executable == "" {
		return "", nil
	}
	args := append([]string(nil), command[1:]...)
	return executable, args
}

func isHTTPURL(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return parsed.Host != ""
	default:
		return false
	}
}
