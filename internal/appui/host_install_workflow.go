package appui

import (
	"fmt"
	"strings"

	"runner/workflow"
	"runner/workflow/visual"
)

const BuiltinHostAgentInstallWorkflowID = "builtin.host-agent-install/v1"

var builtinHostAgentInstallSteps = []string{
	"validate-inputs",
	"tcp-preflight",
	"ssh-preflight",
	"detect-platform",
	"resolve-artifact",
	"upload-artifact",
	"install-files",
	"install-service",
	"start-service",
	"verify-local-health",
	"verify-aiops-heartbeat",
	"finalize-host",
}

var forbiddenHostAgentInstallActions = map[string]struct{}{
	"cmd.run":   {},
	"shell.run": {},
}

var forbiddenHostAgentInstallActionPrefixes = []string{
	"llm.",
	"prompt.",
	"chat.",
	"completion.",
	"agent.",
}

func BuiltinHostAgentInstallGraph() visual.Graph {
	nodes := make([]visual.Node, 0, len(builtinHostAgentInstallSteps)+2)
	nodes = append(nodes, visual.Node{
		ID:       "start",
		Type:     visual.NodeTypeStart,
		Position: visual.Position{X: 0, Y: 0},
		Label:    "Start",
	})

	steps := make([]workflow.Step, 0, len(builtinHostAgentInstallSteps))
	for i, name := range builtinHostAgentInstallSteps {
		step := workflow.Step{
			ID:      name,
			Name:    name,
			Action:  "script.shell",
			Targets: []string{"server-local"},
			Args: map[string]any{
				"script":      hostAgentInstallScript(name),
				"export_vars": true,
			},
		}
		steps = append(steps, step)
		stepCopy := step
		nodes = append(nodes, visual.Node{
			ID:       name,
			Type:     visual.NodeTypeAction,
			Position: visual.Position{X: float64((i + 1) * 220), Y: 0},
			StepID:   name,
			StepName: name,
			Step:     &stepCopy,
			Label:    name,
		})
	}
	nodes = append(nodes, visual.Node{
		ID:       "end",
		Type:     visual.NodeTypeEnd,
		Position: visual.Position{X: float64((len(builtinHostAgentInstallSteps) + 1) * 220), Y: 0},
		Label:    "End",
	})

	edges := make([]visual.Edge, 0, len(nodes)-1)
	for i := 0; i < len(nodes)-1; i++ {
		edges = append(edges, visual.Edge{
			ID:     fmt.Sprintf("edge-%s-%s", nodes[i].ID, nodes[i+1].ID),
			Source: nodes[i].ID,
			Target: nodes[i+1].ID,
			Kind:   visual.EdgeKindNext,
		})
	}

	return visual.Graph{
		Version: visual.GraphVersion,
		Workflow: workflow.Workflow{
			Version:     "v0.1",
			Name:        "host-agent-install",
			Description: "Built-in controlled SSH bootstrap workflow for host-agent installation.",
			Plan:        workflow.Plan{Mode: "auto", Strategy: "graph"},
			Inventory: workflow.Inventory{Hosts: map[string]workflow.Host{
				"server-local": {Address: "local"},
			}},
			Steps: steps,
			Vars: map[string]any{
				"workflow_id": BuiltinHostAgentInstallWorkflowID,
			},
		},
		Layout: visual.Layout{Direction: "LR"},
		Nodes:  nodes,
		Edges:  edges,
		UI: map[string]any{
			"builtin_workflow_id": BuiltinHostAgentInstallWorkflowID,
		},
	}
}

func ValidateHostAgentInstallGraph(graph visual.Graph) error {
	if err := visual.ValidateGraph(graph); err != nil {
		return err
	}

	actionNodes := make([]visual.Node, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
		step := node.Step
		if step != nil {
			if err := validateHostAgentInstallAction(node.ID, step.Action); err != nil {
				return err
			}
		}
		if node.Type == visual.NodeTypeAction {
			actionNodes = append(actionNodes, node)
		}
	}
	if len(actionNodes) != len(builtinHostAgentInstallSteps) {
		return fmt.Errorf("host-agent install workflow must contain exactly %d action nodes, got %d", len(builtinHostAgentInstallSteps), len(actionNodes))
	}
	for i, want := range builtinHostAgentInstallSteps {
		node := actionNodes[i]
		if node.Step == nil {
			return fmt.Errorf("host-agent install node %q is missing step", node.ID)
		}
		if node.Step.Name != want {
			return fmt.Errorf("host-agent install action node %d step = %q, want %q", i, node.Step.Name, want)
		}
		if node.Step.Action != "script.shell" {
			return fmt.Errorf("host-agent install step %q action = %q, want script.shell", want, node.Step.Action)
		}
	}
	return nil
}

func validateHostAgentInstallAction(nodeID, action string) error {
	action = strings.TrimSpace(action)
	if _, forbidden := forbiddenHostAgentInstallActions[action]; forbidden {
		return fmt.Errorf("host-agent install node %q uses forbidden action %q", nodeID, action)
	}
	for _, prefix := range forbiddenHostAgentInstallActionPrefixes {
		if strings.HasPrefix(action, prefix) {
			return fmt.Errorf("host-agent install node %q uses forbidden model action %q", nodeID, action)
		}
	}
	return nil
}

func hostAgentInstallScript(step string) string {
	return fmt.Sprintf("set -euo pipefail\nprintf 'RUNNER_EXPORT_install_step=%%s\\n' '%s'\n%s\n%s\n", shellSingleQuote(step), hostAgentInstallCommonPrelude(), hostAgentInstallScriptBody(step))
}

func hostAgentInstallScriptBody(step string) string {
	switch step {
	case "validate-inputs":
		return `for key in host_id ssh_host ssh_user agent_version secret_dir repo_root; do
  required_var "$key"
done
case "$host_id" in ''|*[!A-Za-z0-9_.-]*) printf 'host_id contains unsupported characters\n' >&2; exit 64;; esac
case "$ssh_user" in ''|*[!A-Za-z0-9_.-]*) printf 'ssh_user contains unsupported characters\n' >&2; exit 64;; esac
case "$ssh_port" in ''|*[!0-9]*) printf 'ssh_port must be numeric\n' >&2; exit 64;; esac
if [ -n "${ssh_credential_ref:-}" ]; then
  credential_file="$(secret_path_from_ref "$ssh_credential_ref")"
  if [ ! -s "$credential_file" ]; then
    printf 'ssh credential ref is not readable: %s\n' "$ssh_credential_ref" >&2
    exit 66
  fi
fi
printf 'RUNNER_EXPORT_remote_tmp=%s\n' "$remote_tmp"`
	case "tcp-preflight":
		return `if command -v nc >/dev/null 2>&1; then
  nc -z "$ssh_host" "$ssh_port"
else
  bash -c ":</dev/tcp/$ssh_host/$ssh_port"
fi`
	case "ssh-preflight":
		return `ssh_run 'echo aiops-ssh-ok; id -u; if [ "$(id -u)" -eq 0 ] || command -v sudo >/dev/null 2>&1; then echo sudo-ok; else echo sudo-missing >&2; exit 70; fi'`
	case "detect-platform":
		return `os_name="$(ssh_run 'uname -s' | tr -d '\r' | tail -n 1)"
arch_name="$(ssh_run 'uname -m' | tr -d '\r' | tail -n 1)"
platform=""
artifact_goos=""
artifact_goarch=""
case "$os_name:$arch_name" in
  Linux:x86_64|Linux:amd64)
    if ssh_run 'test -r /etc/os-release && grep -qi "^ID=ubuntu" /etc/os-release'; then
      platform="linux/ubuntu"
      artifact_goos="linux"
      artifact_goarch="amd64"
    fi
    ;;
  Darwin:arm64|Darwin:aarch64)
    platform="darwin/arm64"
    artifact_goos="darwin"
    artifact_goarch="arm64"
    ;;
esac
if [ -z "$platform" ]; then
  printf 'unsupported platform: %s %s\n' "$os_name" "$arch_name" >&2
  exit 65
fi
printf 'RUNNER_EXPORT_platform=%s\n' "$platform"
printf 'RUNNER_EXPORT_artifact_goos=%s\n' "$artifact_goos"
printf 'RUNNER_EXPORT_artifact_goarch=%s\n' "$artifact_goarch"`
	case "resolve-artifact":
		return `required_var platform
required_var artifact_goos
required_var artifact_goarch
artifact_dir="$repo_root/artifacts/host-agent/$agent_version/$artifact_goos-$artifact_goarch"
artifact_path="$artifact_dir/host-agent"
mkdir -p "$artifact_dir"
if [ ! -x "$artifact_path" ]; then
  (cd "$repo_root" && CGO_ENABLED=0 GOOS="$artifact_goos" GOARCH="$artifact_goarch" go build -o "$artifact_path" ./cmd/host-agent)
fi
artifact_sha256="$(sha256_file "$artifact_path")"
printf 'RUNNER_EXPORT_artifact_ref=host-agent:%s:%s\n' "$agent_version" "$platform"
printf 'RUNNER_EXPORT_artifact_path=%s\n' "$artifact_path"
printf 'RUNNER_EXPORT_artifact_sha256=%s\n' "$artifact_sha256"`
	case "upload-artifact":
		return `required_var artifact_path
if [ ! -x "$artifact_path" ]; then
  printf 'artifact is not executable: %s\n' "$artifact_path" >&2
  exit 66
fi
ssh_run "mkdir -p '$remote_tmp'"
scp_put "$artifact_path" "$remote_tmp/host-agent"
ssh_run "chmod 755 '$remote_tmp/host-agent'"`
	case "install-files":
		return `required_var platform
cfg_file="$(mktemp)"
remember_file "$cfg_file"
case "$platform" in
  linux/ubuntu)
    token_ref="/etc/aiops/host-agent.token"
    install_root="/opt/aiops/host-agent"
    config_path="/etc/aiops/host-agent.yaml"
    ;;
  darwin/arm64)
    token_ref="/usr/local/etc/aiops/host-agent.token"
    install_root="/usr/local/aiops/host-agent"
    config_path="/usr/local/etc/aiops/host-agent.yaml"
    ;;
  *) printf 'unsupported platform: %s\n' "$platform" >&2; exit 65;;
esac
cat > "$cfg_file" <<YAML
server_url: "$agent_server_url"
host_id: "$host_id"
listen_addr: "0.0.0.0:$agent_listen_port"
token_ref: "$token_ref"
heartbeat_interval: "15s"
capabilities:
  - script.shell
  - script.python
  - terminal
YAML
scp_put "$cfg_file" "$remote_tmp/host-agent.yaml"
case "$platform" in
  linux/ubuntu)
    ssh_script <<REMOTE
set -eu
if [ "\$(id -u)" -eq 0 ]; then SUDO=""; else SUDO="sudo"; fi
\$SUDO install -d -m 755 /opt/aiops/host-agent /etc/aiops
\$SUDO install -m 755 "$remote_tmp/host-agent" /opt/aiops/host-agent/host-agent
\$SUDO install -m 600 "$remote_tmp/host-agent.yaml" /etc/aiops/host-agent.yaml
if [ ! -s /etc/aiops/host-agent.token ]; then
  token="\$(openssl rand -base64 32 2>/dev/null || (dd if=/dev/urandom bs=32 count=1 2>/dev/null | base64))"
  printf '%s\n' "\$token" > "$remote_tmp/host-agent.token"
  \$SUDO install -m 600 "$remote_tmp/host-agent.token" /etc/aiops/host-agent.token
  rm -f "$remote_tmp/host-agent.token"
fi
REMOTE
    ;;
  darwin/arm64)
    ssh_script <<REMOTE
set -eu
if [ "\$(id -u)" -eq 0 ]; then SUDO=""; else SUDO="sudo"; fi
\$SUDO install -d -m 755 /usr/local/aiops/host-agent /usr/local/etc/aiops /usr/local/var/log/aiops
\$SUDO install -m 755 "$remote_tmp/host-agent" /usr/local/aiops/host-agent/host-agent
\$SUDO install -m 600 "$remote_tmp/host-agent.yaml" /usr/local/etc/aiops/host-agent.yaml
if [ ! -s /usr/local/etc/aiops/host-agent.token ]; then
  token="\$(openssl rand -base64 32 2>/dev/null || (dd if=/dev/urandom bs=32 count=1 2>/dev/null | base64))"
  printf '%s\n' "\$token" > "$remote_tmp/host-agent.token"
  \$SUDO install -m 600 "$remote_tmp/host-agent.token" /usr/local/etc/aiops/host-agent.token
  rm -f "$remote_tmp/host-agent.token"
fi
REMOTE
    ;;
esac
printf 'RUNNER_EXPORT_agent_config_path=%s\n' "$config_path"
printf 'RUNNER_EXPORT_agent_install_root=%s\n' "$install_root"`
	case "install-service":
		return `required_var platform
case "$platform" in
  linux/ubuntu)
    service_file="$(mktemp)"
    remember_file "$service_file"
    cat > "$service_file" <<UNIT
[Unit]
Description=AIOps host-agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/opt/aiops/host-agent/host-agent --config /etc/aiops/host-agent.yaml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
UNIT
    scp_put "$service_file" "$remote_tmp/aiops-host-agent.service"
    ssh_script <<REMOTE
set -eu
if [ "\$(id -u)" -eq 0 ]; then SUDO=""; else SUDO="sudo"; fi
\$SUDO install -m 644 "$remote_tmp/aiops-host-agent.service" /etc/systemd/system/aiops-host-agent.service
\$SUDO systemctl daemon-reload
REMOTE
    ;;
  darwin/arm64)
    service_file="$(mktemp)"
    remember_file "$service_file"
    cat > "$service_file" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>com.aiops.host-agent</string>
  <key>ProgramArguments</key>
  <array>
    <string>/usr/local/aiops/host-agent/host-agent</string>
    <string>--config</string>
    <string>/usr/local/etc/aiops/host-agent.yaml</string>
  </array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>/usr/local/var/log/aiops/host-agent.out.log</string>
  <key>StandardErrorPath</key><string>/usr/local/var/log/aiops/host-agent.err.log</string>
</dict>
</plist>
PLIST
    scp_put "$service_file" "$remote_tmp/com.aiops.host-agent.plist"
    ssh_script <<REMOTE
set -eu
if [ "\$(id -u)" -eq 0 ]; then SUDO=""; else SUDO="sudo"; fi
\$SUDO install -m 644 "$remote_tmp/com.aiops.host-agent.plist" /Library/LaunchDaemons/com.aiops.host-agent.plist
REMOTE
    ;;
  *) printf 'unsupported platform: %s\n' "$platform" >&2; exit 65;;
esac`
	case "start-service":
		return `required_var platform
case "$platform" in
  linux/ubuntu)
    ssh_script <<'REMOTE'
set -eu
if [ "$(id -u)" -eq 0 ]; then SUDO=""; else SUDO="sudo"; fi
$SUDO systemctl enable --now aiops-host-agent.service
$SUDO systemctl is-active aiops-host-agent.service
REMOTE
    ;;
  darwin/arm64)
    ssh_script <<'REMOTE'
set -eu
if [ "$(id -u)" -eq 0 ]; then SUDO=""; else SUDO="sudo"; fi
$SUDO launchctl bootout system /Library/LaunchDaemons/com.aiops.host-agent.plist >/dev/null 2>&1 || true
$SUDO launchctl bootstrap system /Library/LaunchDaemons/com.aiops.host-agent.plist
$SUDO launchctl enable system/com.aiops.host-agent
$SUDO launchctl kickstart -k system/com.aiops.host-agent
$SUDO launchctl print system/com.aiops.host-agent >/dev/null
REMOTE
    ;;
  *) printf 'unsupported platform: %s\n' "$platform" >&2; exit 65;;
esac`
	case "verify-local-health":
		return `for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do
  if ssh_run "if command -v curl >/dev/null 2>&1; then curl -fsS 'http://127.0.0.1:$agent_listen_port/health'; elif command -v wget >/dev/null 2>&1; then wget -qO- 'http://127.0.0.1:$agent_listen_port/health'; else exit 127; fi" >/dev/null; then
    exit 0
  fi
  sleep 2
done
printf 'host-agent local health check timed out\n' >&2
exit 1`
	case "verify-aiops-heartbeat":
		return `required_var aiops_api_url
for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20; do
  body="$(curl -fsS "$aiops_api_url/api/v1/hosts" 2>/dev/null || true)"
  if printf '%s' "$body" | grep -q "\"id\":\"$host_id\"" &&
     printf '%s' "$body" | grep -q "\"status\":\"online\"" &&
     printf '%s' "$body" | grep -q "\"installState\":\"installed\""; then
    exit 0
  fi
  sleep 2
done
printf 'host-agent heartbeat did not mark host online\n' >&2
exit 1`
	case "finalize-host":
		return `printf 'RUNNER_EXPORT_control_mode=managed\n'
printf 'RUNNER_EXPORT_install_state=installed\n'
printf 'host-agent installation completed for host %s\n' "$host_id"`
	default:
		return fmt.Sprintf("printf 'unknown host-agent install step: %s\\n' >&2\nexit 64", shellSingleQuote(step))
	}
}

func hostAgentInstallCommonPrelude() string {
	return `ssh_port="${ssh_port:-22}"
agent_version="${agent_version:-v0.1.0}"
agent_listen_port="${agent_listen_port:-7072}"
agent_server_url="${agent_server_url:-http://127.0.0.1:18080}"
aiops_api_url="${aiops_api_url:-http://127.0.0.1:18080}"
secret_dir="${secret_dir:-${AIOPS_SECRET_DIR:-.data/secrets}}"
repo_root="${repo_root:-$(pwd)}"
remote_tmp="/tmp/aiops-host-agent-${host_id:-unknown}"
cleanup_files=""

remember_file() {
  cleanup_files="$cleanup_files $1"
}

cleanup_all() {
  for file in $cleanup_files; do
    if [ -n "$file" ]; then
      rm -f "$file" >/dev/null 2>&1 || true
    fi
  done
}
trap cleanup_all EXIT

required_var() {
  key="$1"
  eval "value=\${$key:-}"
  if [ -z "$value" ]; then
    printf 'missing required var: %s\n' "$key" >&2
    exit 64
  fi
}

secret_path_from_ref() {
  ref="$1"
  case "$ref" in
    secret://*) rel="${ref#secret://}" ;;
    *) printf 'unsupported secret ref: %s\n' "$ref" >&2; exit 64 ;;
  esac
  case "$rel" in
    ''|/*|*'..'*) printf 'invalid secret ref path: %s\n' "$ref" >&2; exit 64 ;;
  esac
  case "$rel" in
    *\\*) printf 'invalid secret ref path: %s\n' "$ref" >&2; exit 64 ;;
  esac
  printf '%s/%s\n' "${secret_dir%/}" "$rel"
}

setup_ssh_auth() {
  if [ "${ssh_auth_ready:-0}" = "1" ]; then
    return 0
  fi
  ssh_auth_args=""
  if [ -n "${ssh_credential_ref:-}" ]; then
    credential_file="$(secret_path_from_ref "$ssh_credential_ref")"
    credential_content="$(cat "$credential_file")"
    if printf '%s\n' "$credential_content" | grep -q 'BEGIN .*PRIVATE KEY'; then
      key_file="$(mktemp)"
      remember_file "$key_file"
      printf '%s\n' "$credential_content" > "$key_file"
      chmod 600 "$key_file"
      ssh_auth_args="-i $key_file -o IdentitiesOnly=yes"
    else
      askpass_file="$(mktemp)"
      remember_file "$askpass_file"
      cat > "$askpass_file" <<'ASKPASS'
#!/bin/sh
printf '%s\n' "$AIOPS_SSH_PASSWORD"
ASKPASS
      chmod 700 "$askpass_file"
      export AIOPS_SSH_PASSWORD="$credential_content"
      export SSH_ASKPASS="$askpass_file"
      export SSH_ASKPASS_REQUIRE=force
      export DISPLAY=none
      ssh_auth_args="-o PreferredAuthentications=password,keyboard-interactive,publickey -o PubkeyAuthentication=no"
    fi
  fi
  ssh_auth_ready=1
}

ssh_run() {
  setup_ssh_auth
  ssh -n $ssh_auth_args -o StrictHostKeyChecking=accept-new -o ServerAliveInterval=15 -p "$ssh_port" "$ssh_user@$ssh_host" "$@"
}

ssh_script() {
  setup_ssh_auth
  ssh $ssh_auth_args -o StrictHostKeyChecking=accept-new -o ServerAliveInterval=15 -p "$ssh_port" "$ssh_user@$ssh_host" 'sh -s'
}

scp_put() {
  setup_ssh_auth
  scp $ssh_auth_args -o StrictHostKeyChecking=accept-new -o ServerAliveInterval=15 -P "$ssh_port" "$1" "$ssh_user@$ssh_host:$2"
}

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}`
}

func shellSingleQuote(value string) string {
	return strings.ReplaceAll(value, "'", "'\"'\"'")
}
