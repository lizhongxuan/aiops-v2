package hostagent

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const defaultHeartbeatInterval = 30 * time.Second

var defaultCapabilities = []string{"script.shell", "script.python", "terminal"}

type Config struct {
	ServerURL         string            `yaml:"server_url"`
	GRPCURL           string            `yaml:"grpc_url"`
	HostID            string            `yaml:"host_id"`
	ListenAddr        string            `yaml:"listen_addr"`
	TokenRef          string            `yaml:"token_ref"`
	Token             string            `yaml:"-"`
	HeartbeatInterval time.Duration     `yaml:"heartbeat_interval"`
	Labels            map[string]string `yaml:"labels"`
	Capabilities      []string          `yaml:"capabilities"`
}

type rawConfig struct {
	ServerURL         string            `yaml:"server_url"`
	GRPCURL           string            `yaml:"grpc_url"`
	HostID            string            `yaml:"host_id"`
	ListenAddr        string            `yaml:"listen_addr"`
	TokenRef          string            `yaml:"token_ref"`
	HeartbeatInterval string            `yaml:"heartbeat_interval"`
	Labels            map[string]string `yaml:"labels"`
	Capabilities      []string          `yaml:"capabilities"`
}

func DefaultCapabilities() []string {
	return append([]string{}, defaultCapabilities...)
}

func Load(path string) (Config, error) {
	rawData, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read host-agent config: %w", err)
	}

	var raw rawConfig
	if err := yaml.Unmarshal(rawData, &raw); err != nil {
		return Config{}, fmt.Errorf("parse host-agent config: %w", err)
	}

	cfg := Config{
		ServerURL:         strings.TrimRight(strings.TrimSpace(raw.ServerURL), "/"),
		GRPCURL:           strings.TrimSpace(raw.GRPCURL),
		HostID:            strings.TrimSpace(raw.HostID),
		ListenAddr:        strings.TrimSpace(raw.ListenAddr),
		TokenRef:          strings.TrimSpace(raw.TokenRef),
		HeartbeatInterval: defaultHeartbeatInterval,
		Labels:            normalizeLabels(raw.Labels),
		Capabilities:      normalizeCapabilities(raw.Capabilities),
	}
	if strings.TrimSpace(raw.HeartbeatInterval) != "" {
		interval, err := time.ParseDuration(strings.TrimSpace(raw.HeartbeatInterval))
		if err != nil {
			return Config{}, fmt.Errorf("heartbeat_interval must be a duration: %w", err)
		}
		cfg.HeartbeatInterval = interval
	}
	if cfg.TokenRef != "" && !filepath.IsAbs(cfg.TokenRef) {
		cfg.TokenRef = filepath.Join(filepath.Dir(path), cfg.TokenRef)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	tokenData, err := os.ReadFile(cfg.TokenRef)
	if err != nil {
		return Config{}, fmt.Errorf("read token_ref: %w", err)
	}
	cfg.Token = strings.TrimSpace(string(tokenData))
	if cfg.Token == "" {
		return Config{}, fmt.Errorf("token_ref file is empty")
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.ServerURL) == "" {
		return fmt.Errorf("server_url is required")
	}
	parsed, err := url.Parse(c.ServerURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("server_url must be an absolute URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("server_url scheme must be http or https")
	}
	if strings.TrimSpace(c.GRPCURL) != "" {
		if strings.Contains(c.GRPCURL, "://") {
			grpcParsed, err := url.Parse(c.GRPCURL)
			if err != nil || grpcParsed.Host == "" {
				return fmt.Errorf("grpc_url must be host:port or an absolute grpc target")
			}
		} else if _, _, err := net.SplitHostPort(c.GRPCURL); err != nil {
			return fmt.Errorf("grpc_url must be host:port: %w", err)
		}
	}
	if strings.TrimSpace(c.HostID) == "" {
		return fmt.Errorf("host_id is required")
	}
	if strings.TrimSpace(c.ListenAddr) == "" {
		return fmt.Errorf("listen_addr is required")
	}
	if _, _, err := net.SplitHostPort(c.ListenAddr); err != nil {
		return fmt.Errorf("listen_addr must be host:port or :port: %w", err)
	}
	if strings.TrimSpace(c.TokenRef) == "" {
		return fmt.Errorf("token_ref is required")
	}
	if c.HeartbeatInterval <= 0 {
		return fmt.Errorf("heartbeat_interval must be positive")
	}
	for key := range c.Labels {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("labels must not contain empty keys")
		}
	}
	if len(c.Capabilities) == 0 {
		return fmt.Errorf("capabilities must not be empty")
	}
	allowed := map[string]bool{
		"script.shell":  true,
		"script.python": true,
		"terminal":      true,
	}
	for _, capability := range c.Capabilities {
		if strings.TrimSpace(capability) == "" {
			return fmt.Errorf("capabilities must not contain empty values")
		}
		if capability == "cmd.run" || capability == "shell.run" {
			return fmt.Errorf("capability %q is not allowed", capability)
		}
		if !allowed[capability] {
			return fmt.Errorf("unsupported capability %q", capability)
		}
	}
	return nil
}

func normalizeLabels(input map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}
	labels := make(map[string]string, len(input))
	for key, value := range input {
		labels[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return labels
}

func normalizeCapabilities(input []string) []string {
	if len(input) == 0 {
		return DefaultCapabilities()
	}
	capabilities := make([]string, 0, len(input))
	seen := map[string]bool{}
	for _, capability := range input {
		capability = strings.TrimSpace(capability)
		if capability == "" || seen[capability] {
			continue
		}
		seen[capability] = true
		capabilities = append(capabilities, capability)
	}
	return capabilities
}
