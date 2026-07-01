package hostagent

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestHostAgentConfigLoadsYAMLAndTokenFile(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token")
	if err := os.WriteFile(tokenPath, []byte("secret-token\n"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
	configPath := filepath.Join(dir, "host-agent.yaml")
	yaml := `
server_url: http://127.0.0.1:8080
grpc_url: 127.0.0.1:18090
host_id: prod-web-01
listen_addr: :7072
token_ref: ` + tokenPath + `
heartbeat_interval: 15s
labels:
  env: prod
  role: web
capabilities:
  - script.shell
  - script.python
  - terminal
`
	if err := os.WriteFile(configPath, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.ServerURL != "http://127.0.0.1:8080" {
		t.Fatalf("ServerURL = %q", cfg.ServerURL)
	}
	if cfg.GRPCURL != "127.0.0.1:18090" {
		t.Fatalf("GRPCURL = %q", cfg.GRPCURL)
	}
	if cfg.ConnectionMode != ConnectionModeNodePushGRPC {
		t.Fatalf("ConnectionMode = %q, want legacy node push mode", cfg.ConnectionMode)
	}
	if cfg.HostID != "prod-web-01" {
		t.Fatalf("HostID = %q", cfg.HostID)
	}
	if cfg.ListenAddr != ":7072" {
		t.Fatalf("ListenAddr = %q", cfg.ListenAddr)
	}
	if cfg.Token != "secret-token" {
		t.Fatalf("Token = %q", cfg.Token)
	}
	if cfg.HeartbeatInterval != 15*time.Second {
		t.Fatalf("HeartbeatInterval = %v", cfg.HeartbeatInterval)
	}
	if cfg.Labels["env"] != "prod" || cfg.Labels["role"] != "web" {
		t.Fatalf("Labels = %#v", cfg.Labels)
	}
	wantCaps := []string{"script.shell", "script.python", "terminal"}
	if !reflect.DeepEqual(cfg.Capabilities, wantCaps) {
		t.Fatalf("Capabilities = %#v, want %#v", cfg.Capabilities, wantCaps)
	}
}

func TestHostAgentDefaultCapabilitiesExcludeLegacyShellActions(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token")
	if err := os.WriteFile(tokenPath, []byte("secret-token"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
	configPath := filepath.Join(dir, "host-agent.yaml")
	yaml := `
server_url: https://aiops.example.test
host_id: prod-web-01
listen_addr: 127.0.0.1:7072
token_ref: ` + tokenPath + `
heartbeat_interval: 30s
`
	if err := os.WriteFile(configPath, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	wantCaps := []string{"script.shell", "script.python", "terminal"}
	if !reflect.DeepEqual(cfg.Capabilities, wantCaps) {
		t.Fatalf("Capabilities = %#v, want %#v", cfg.Capabilities, wantCaps)
	}
	for _, denied := range []string{"cmd.run", "shell.run"} {
		for _, got := range cfg.Capabilities {
			if got == denied {
				t.Fatalf("default capabilities include denied action %q", denied)
			}
		}
	}
}

func TestHostAgentConfigAllowsAIOPSPullWithoutServerURL(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token")
	if err := os.WriteFile(tokenPath, []byte("secret-token"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
	configPath := filepath.Join(dir, "host-agent.yaml")
	yaml := `
connection_mode: aiops_pull
host_id: prod-web-01
listen_addr: :7072
token_ref: ` + tokenPath + `
heartbeat_interval: 30s
`
	if err := os.WriteFile(configPath, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v, want pull mode without server_url to load", err)
	}
	if cfg.ConnectionMode != ConnectionModeAIOPSPull {
		t.Fatalf("ConnectionMode = %q, want aiops_pull", cfg.ConnectionMode)
	}
	if cfg.ServerURL != "" || cfg.GRPCURL != "" {
		t.Fatalf("ServerURL=%q GRPCURL=%q, want empty pull-mode callback fields", cfg.ServerURL, cfg.GRPCURL)
	}
}

func TestHostAgentConfigRequiresServerURLForNodePushGRPC(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token")
	if err := os.WriteFile(tokenPath, []byte("secret-token"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
	configPath := filepath.Join(dir, "host-agent.yaml")
	yaml := `
connection_mode: node_push_grpc
host_id: prod-web-01
listen_addr: :7072
token_ref: ` + tokenPath + `
heartbeat_interval: 30s
`
	if err := os.WriteFile(configPath, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() error = nil, want missing server_url error")
	}
	if !strings.Contains(err.Error(), "server_url") {
		t.Fatalf("Load() error = %v, want server_url", err)
	}
}
