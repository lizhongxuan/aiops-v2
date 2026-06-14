package appui

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"aiops-v2/internal/store"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"gopkg.in/yaml.v3"
)

type HostAgentInstaller interface {
	Install(ctx context.Context, hostID string, req HostInstallRequest) (HostInstallRun, error)
}

type HostAgentSSHTester interface {
	TestSSH(ctx context.Context, hostID, credentialRef string) (HostSSHTestResponse, error)
}

type SSHBootstrapResult struct {
	Stdout string
	Stderr string
}

type SSHBootstrapClient interface {
	Run(ctx context.Context, command string, stdin []byte) (SSHBootstrapResult, error)
	Close() error
}

type SSHBootstrapDialer interface {
	DialHost(ctx context.Context, host store.HostRecord, credential ResolvedSSHCredential) (SSHBootstrapClient, error)
}

type HostAgentArtifact struct {
	Path   string
	Bytes  []byte
	SHA256 string
}

type HostAgentArtifactBuilder interface {
	BuildHostAgentArtifact(ctx context.Context, goos, goarch, version string) (HostAgentArtifact, error)
}

type DirectHostAgentInstallerOption func(*DirectHostAgentInstaller)

type DirectHostAgentInstaller struct {
	repo          HostRepository
	resolver      CredentialResolver
	dialer        SSHBootstrapDialer
	artifactBuild HostAgentArtifactBuilder
	tokenStore    HostAgentTokenStore
	newRunID      func(string) string
	newToken      func() (string, error)
	sleep         func(context.Context, time.Duration) error
}

type detectedHostPlatform struct {
	Platform  string
	OS        string
	Arch      string
	GOOS      string
	GOARCH    string
	OSID      string
	OSName    string
	UnameOS   string
	UnameArch string
}

func NewDirectHostAgentInstaller(repo HostRepository, resolver CredentialResolver, opts ...DirectHostAgentInstallerOption) *DirectHostAgentInstaller {
	installer := &DirectHostAgentInstaller{
		repo:          repo,
		resolver:      resolver,
		dialer:        defaultSSHBootstrapDialer{Timeout: 15 * time.Second},
		artifactBuild: goBuildHostAgentArtifactBuilder{RepoRoot: defaultHostInstallRepoRoot()},
		newRunID:      newDirectHostInstallRunID,
		newToken:      newHostAgentInstallToken,
		sleep:         sleepContext,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(installer)
		}
	}
	return installer
}

func WithSSHBootstrapDialer(dialer SSHBootstrapDialer) DirectHostAgentInstallerOption {
	return func(installer *DirectHostAgentInstaller) {
		if dialer != nil {
			installer.dialer = dialer
		}
	}
}

func WithHostAgentArtifactBuilder(builder HostAgentArtifactBuilder) DirectHostAgentInstallerOption {
	return func(installer *DirectHostAgentInstaller) {
		if builder != nil {
			installer.artifactBuild = builder
		}
	}
}

func WithDirectHostAgentTokenStore(store HostAgentTokenStore) DirectHostAgentInstallerOption {
	return func(installer *DirectHostAgentInstaller) {
		installer.tokenStore = store
	}
}

func (i *DirectHostAgentInstaller) Install(ctx context.Context, hostID string, req HostInstallRequest) (HostInstallRun, error) {
	if i == nil || i.repo == nil {
		return HostInstallRun{}, fmt.Errorf("host repository is not configured")
	}
	if i.dialer == nil {
		return HostInstallRun{}, fmt.Errorf("ssh dialer is not configured")
	}
	host, err := i.loadInstallHost(strings.TrimSpace(hostID), req)
	if err != nil {
		return HostInstallRun{}, err
	}
	run := HostInstallRun{
		HostID:       host.ID,
		RunID:        i.newRunID(host.ID),
		Status:       "running",
		CurrentStep:  "validate-inputs",
		AgentVersion: host.AgentVersion,
	}
	if err := i.saveInstallProgress(&host, run, "", ""); err != nil {
		return HostInstallRun{}, err
	}
	if err := validateBootstrapHost(host); err != nil {
		return i.failInstall(&host, run, "validate-inputs", "failed", err)
	}

	i.setStep(&host, &run, "connect-ssh")
	credential, err := i.resolveSSHCredential(ctx, host.SSHCredentialRef)
	if err != nil {
		return i.failInstall(&host, run, "connect-ssh", "failed", err)
	}
	if credential.Cleanup != nil {
		defer func() { _ = credential.Cleanup() }()
	}
	client, err := i.dialer.DialHost(ctx, host, credential)
	if err != nil {
		return i.failInstall(&host, run, "connect-ssh", "failed", err)
	}
	defer func() { _ = client.Close() }()

	i.setStep(&host, &run, "detect-platform")
	platform, err := detectHostAgentPlatform(ctx, client)
	if err != nil {
		state := "failed"
		if isUnsupportedPlatformError(err) {
			state = "unsupported_platform"
			run.Platform = unsupportedPlatformLabel(platform)
		}
		return i.failInstall(&host, run, "detect-platform", state, err)
	}
	run.Platform = platform.Platform
	host.OS = platform.OS
	host.Arch = platform.Arch

	sudoMode, err := detectSudoMode(ctx, client)
	if err != nil {
		return i.failInstall(&host, run, "ssh-preflight", "failed", err)
	}
	if sudoMode == "none" {
		return i.failInstall(&host, run, "ssh-preflight", "failed", fmt.Errorf("ssh user must be root or have sudo available"))
	}
	sudoStdin := sudoPasswordStdin(sudoMode, credential)

	i.setStep(&host, &run, "build-artifact")
	artifact, err := i.artifactBuild.BuildHostAgentArtifact(ctx, platform.GOOS, platform.GOARCH, host.AgentVersion)
	if err != nil {
		return i.failInstall(&host, run, "build-artifact", "failed", err)
	}
	if len(artifact.Bytes) == 0 {
		return i.failInstall(&host, run, "build-artifact", "failed", fmt.Errorf("host-agent artifact is empty"))
	}

	token, err := i.newToken()
	if err != nil {
		return i.failInstall(&host, run, "write-config", "failed", err)
	}
	host.AgentTokenRef = hostAgentTokenHashRef(token)
	if i.tokenStore != nil {
		ref, err := i.tokenStore.StoreHostAgentToken(ctx, host.ID, token)
		if err != nil {
			return i.failInstall(&host, run, "write-config", "failed", err)
		}
		host.AgentTokenSecretRef = ref
	}
	remoteTmp := "/tmp/aiops-host-agent-" + safeRemoteName(host.ID)
	if err := i.runRemote(ctx, client, "upload-artifact", &host, &run, "mkdir -p "+shellQuote(remoteTmp), nil); err != nil {
		return run, err
	}
	if err := i.uploadRemote(ctx, client, "upload-artifact", &host, &run, remoteTmp+"/host-agent", "755", artifact.Bytes); err != nil {
		return run, err
	}

	config, layout, err := buildHostAgentRemoteConfig(host, platform, token)
	if err != nil {
		return i.failInstall(&host, run, "write-config", "failed", err)
	}
	if err := i.uploadRemote(ctx, client, "write-config", &host, &run, remoteTmp+"/host-agent.yaml", "600", config); err != nil {
		return run, err
	}
	if err := i.uploadRemote(ctx, client, "write-config", &host, &run, remoteTmp+"/host-agent.token", "600", []byte(token+"\n")); err != nil {
		return run, err
	}

	if err := i.runRemote(ctx, client, "install-files", &host, &run, installFilesScript(platform, layout, remoteTmp), sudoStdin); err != nil {
		return run, err
	}
	serviceFile, err := serviceDefinition(platform)
	if err != nil {
		return i.failInstall(&host, run, "install-service", "failed", err)
	}
	if err := i.uploadRemote(ctx, client, "install-service", &host, &run, remoteTmp+"/"+layout.ServiceFileName, "644", serviceFile); err != nil {
		return run, err
	}
	if err := i.runRemote(ctx, client, "install-service", &host, &run, installServiceScript(platform, layout, remoteTmp), sudoStdin); err != nil {
		return run, err
	}
	if err := i.runRemote(ctx, client, "start-service", &host, &run, startServiceScript(platform), sudoStdin); err != nil {
		return run, err
	}
	if err := i.verifyLocalHealth(ctx, client, &host, &run); err != nil {
		return run, err
	}

	run.Status = "success"
	run.CurrentStep = "finalize-host"
	host.Status = "online"
	host.InstallState = "installed"
	host.InstallStep = "finalize-host"
	host.InstallWorkflowID = ""
	host.Transport = "agent_http"
	host.ControlMode = "managed"
	host.TerminalCapable = true
	host.Executable = true
	host.AgentURL = "http://" + net.JoinHostPort(host.Address, "7072")
	host.LastError = ""
	if err := i.repo.SaveHost(&host); err != nil {
		return HostInstallRun{}, err
	}
	return run, nil
}

func (i *DirectHostAgentInstaller) TestSSH(ctx context.Context, hostID, credentialRef string) (HostSSHTestResponse, error) {
	if i == nil || i.repo == nil {
		return HostSSHTestResponse{}, fmt.Errorf("host repository is not configured")
	}
	if i.dialer == nil {
		return HostSSHTestResponse{}, fmt.Errorf("ssh dialer is not configured")
	}
	host, err := i.repo.GetHost(strings.TrimSpace(hostID))
	if err != nil {
		return HostSSHTestResponse{}, err
	}
	if host == nil {
		return HostSSHTestResponse{}, fmt.Errorf("host not found: %s", hostID)
	}
	next := cloneHostRecord(*host)
	if ref := strings.TrimSpace(credentialRef); ref != "" {
		next.SSHCredentialRef = ref
	}
	if err := validateBootstrapHost(next); err != nil {
		return HostSSHTestResponse{}, err
	}
	credential, err := i.resolveSSHCredential(ctx, next.SSHCredentialRef)
	if err != nil {
		return HostSSHTestResponse{}, err
	}
	if credential.Cleanup != nil {
		defer func() { _ = credential.Cleanup() }()
	}
	client, err := i.dialer.DialHost(ctx, next, credential)
	if err != nil {
		return HostSSHTestResponse{}, redactSSHTestError(err)
	}
	defer func() { _ = client.Close() }()

	platform, err := detectHostAgentPlatform(ctx, client)
	if err != nil && !isUnsupportedPlatformError(err) {
		return HostSSHTestResponse{}, redactSSHTestError(err)
	}
	sudoMode, sudoErr := detectSudoMode(ctx, client)
	if sudoErr != nil {
		return HostSSHTestResponse{}, redactSSHTestError(sudoErr)
	}
	if err != nil {
		return HostSSHTestResponse{
			Status:   "unsupported",
			Platform: unsupportedPlatformLabel(platform),
			OS:       platform.OS,
			Arch:     platform.Arch,
			Sudo:     sudoMode,
			Message:  redactInstallError(err.Error()),
		}, nil
	}
	return HostSSHTestResponse{
		Status:   "ok",
		Platform: platform.Platform,
		OS:       platform.OS,
		Arch:     platform.Arch,
		Sudo:     sudoMode,
		Message:  "SSH connected",
	}, nil
}

func (i *DirectHostAgentInstaller) loadInstallHost(hostID string, req HostInstallRequest) (store.HostRecord, error) {
	if hostID == "" {
		return store.HostRecord{}, fmt.Errorf("host id is required")
	}
	host, err := i.repo.GetHost(hostID)
	if err != nil {
		return store.HostRecord{}, err
	}
	if host == nil {
		return store.HostRecord{}, fmt.Errorf("host not found: %s", hostID)
	}
	next := cloneHostRecord(*host)
	if ref := strings.TrimSpace(req.SSHCredentialRef); ref != "" {
		next.SSHCredentialRef = ref
	}
	if version := strings.TrimSpace(req.AgentVersion); version != "" {
		next.AgentVersion = version
	}
	if next.AgentVersion == "" {
		next.AgentVersion = "v0.1.0"
	}
	return next, nil
}

func (i *DirectHostAgentInstaller) resolveSSHCredential(ctx context.Context, ref string) (ResolvedSSHCredential, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ResolvedSSHCredential{}, nil
	}
	if i == nil || i.resolver == nil {
		return ResolvedSSHCredential{}, fmt.Errorf("credential resolver is not configured")
	}
	return i.resolver.ResolveSSHCredential(ctx, ref)
}

func (i *DirectHostAgentInstaller) setStep(host *store.HostRecord, run *HostInstallRun, step string) {
	if host == nil || run == nil {
		return
	}
	run.CurrentStep = step
	_ = i.saveInstallProgress(host, *run, "", "")
}

func (i *DirectHostAgentInstaller) saveInstallProgress(host *store.HostRecord, run HostInstallRun, status, state string) error {
	if host == nil {
		return fmt.Errorf("host is nil")
	}
	if status == "" {
		status = "installing"
	}
	if state == "" {
		state = "running"
	}
	host.Transport = "ssh_bootstrap"
	host.Status = status
	host.InstallState = state
	host.InstallRunID = run.RunID
	host.InstallWorkflowID = ""
	host.InstallStep = run.CurrentStep
	host.ControlMode = "managed"
	host.LastError = ""
	return i.repo.SaveHost(host)
}

func (i *DirectHostAgentInstaller) failInstall(host *store.HostRecord, run HostInstallRun, step, state string, cause error) (HostInstallRun, error) {
	if step == "" {
		step = run.CurrentStep
	}
	if state == "" {
		state = "failed"
	}
	message := "host-agent install failed"
	if cause != nil {
		message = cause.Error()
	}
	message = redactInstallError(message)
	run.Status = "failed"
	run.CurrentStep = step
	run.LastError = message
	if host != nil {
		host.Status = "install_failed"
		host.InstallState = state
		host.InstallRunID = run.RunID
		host.InstallWorkflowID = ""
		host.InstallStep = step
		host.ControlMode = "managed"
		host.LastError = message
		_ = i.repo.SaveHost(host)
	}
	return run, fmt.Errorf("%s", message)
}

func (i *DirectHostAgentInstaller) runRemote(ctx context.Context, client SSHBootstrapClient, step string, host *store.HostRecord, run *HostInstallRun, command string, stdin []byte) error {
	if run != nil {
		run.CurrentStep = step
	}
	if host != nil && run != nil {
		_ = i.saveInstallProgress(host, *run, "", "")
	}
	result, err := client.Run(ctx, command, stdin)
	if err != nil {
		_, failErr := i.failInstall(host, *run, step, "failed", fmt.Errorf("%w: %s", err, result.Stderr))
		return failErr
	}
	return nil
}

func (i *DirectHostAgentInstaller) uploadRemote(ctx context.Context, client SSHBootstrapClient, step string, host *store.HostRecord, run *HostInstallRun, path, mode string, data []byte) error {
	dir := filepath.ToSlash(filepath.Dir(path))
	command := fmt.Sprintf("umask 077 && mkdir -p %s && cat > %s && chmod %s %s", shellQuote(dir), shellQuote(path), shellQuote(mode), shellQuote(path))
	return i.runRemote(ctx, client, step, host, run, command, data)
}

func (i *DirectHostAgentInstaller) verifyLocalHealth(ctx context.Context, client SSHBootstrapClient, host *store.HostRecord, run *HostInstallRun) error {
	command := "if command -v curl >/dev/null 2>&1; then curl -fsS 'http://127.0.0.1:7072/health'; elif command -v wget >/dev/null 2>&1; then wget -qO- 'http://127.0.0.1:7072/health'; else exit 127; fi"
	var lastErr error
	for attempt := 0; attempt < 15; attempt++ {
		if err := i.runRemote(ctx, client, "verify-local-health", host, run, command, nil); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if err := i.sleep(ctx, 2*time.Second); err != nil {
			return err
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("host-agent local health check timed out")
	}
	_, err := i.failInstall(host, *run, "verify-local-health", "failed", lastErr)
	return err
}

type unsupportedPlatformError struct {
	platform detectedHostPlatform
}

func (e unsupportedPlatformError) Error() string {
	if e.platform.UnameOS != "" || e.platform.UnameArch != "" || e.platform.OSID != "" {
		detail := strings.TrimSpace(strings.Join([]string{e.platform.UnameOS, e.platform.UnameArch, e.platform.OSID}, " "))
		return "unsupported platform: " + detail
	}
	return "unsupported platform"
}

func isUnsupportedPlatformError(err error) bool {
	_, ok := err.(unsupportedPlatformError)
	return ok
}

func detectHostAgentPlatform(ctx context.Context, client SSHBootstrapClient) (detectedHostPlatform, error) {
	osResult, err := client.Run(ctx, "uname -s", nil)
	if err != nil {
		return detectedHostPlatform{}, err
	}
	archResult, err := client.Run(ctx, "uname -m", nil)
	if err != nil {
		return detectedHostPlatform{}, err
	}
	unameOS := firstOutputLine(osResult.Stdout)
	unameArch := firstOutputLine(archResult.Stdout)
	platform := detectedHostPlatform{
		UnameOS:   unameOS,
		UnameArch: unameArch,
		OS:        normalizeUnameOS(unameOS),
		Arch:      normalizeArch(unameArch),
	}
	switch strings.ToLower(unameOS) {
	case "darwin":
		if platform.Arch == "arm64" {
			platform.Platform = "darwin/arm64"
			platform.GOOS = "darwin"
			platform.GOARCH = "arm64"
			return platform, nil
		}
		return platform, unsupportedPlatformError{platform: platform}
	case "linux":
		osRelease, _ := client.Run(ctx, "cat /etc/os-release 2>/dev/null", nil)
		fields := parseOSRelease(osRelease.Stdout)
		platform.OSID = strings.ToLower(fields["ID"])
		platform.OSName = fields["NAME"]
		if platform.OSID != "" {
			platform.Platform = "linux/" + platform.OSID
		}
		if platform.Arch == "amd64" {
			if platform.OSID == "ubuntu" {
				platform.Platform = "linux/ubuntu"
			} else {
				platform.Platform = "linux/amd64"
			}
			platform.GOOS = "linux"
			platform.GOARCH = "amd64"
			return platform, nil
		}
		return platform, unsupportedPlatformError{platform: platform}
	default:
		return platform, unsupportedPlatformError{platform: platform}
	}
}

func detectSudoMode(ctx context.Context, client SSHBootstrapClient) (string, error) {
	result, err := client.Run(ctx, "if [ \"$(id -u)\" -eq 0 ]; then echo root; elif command -v sudo >/dev/null 2>&1; then echo sudo; else echo none; fi", nil)
	if err != nil {
		return "", err
	}
	mode := strings.TrimSpace(firstOutputLine(result.Stdout))
	if mode == "" {
		mode = "none"
	}
	return mode, nil
}

func sudoPasswordStdin(sudoMode string, credential ResolvedSSHCredential) []byte {
	if sudoMode != "sudo" {
		return nil
	}
	password := strings.TrimSpace(credential.Password)
	if password == "" {
		return nil
	}
	return []byte(password + "\n")
}

type hostAgentRemoteLayout struct {
	TokenPath       string
	ConfigPath      string
	InstallRoot     string
	ServiceFileName string
}

func buildHostAgentRemoteConfig(host store.HostRecord, platform detectedHostPlatform, token string) ([]byte, hostAgentRemoteLayout, error) {
	layout, err := remoteLayout(platform)
	if err != nil {
		return nil, hostAgentRemoteLayout{}, err
	}
	cfg := struct {
		ServerURL         string            `yaml:"server_url"`
		GRPCURL           string            `yaml:"grpc_url,omitempty"`
		HostID            string            `yaml:"host_id"`
		ListenAddr        string            `yaml:"listen_addr"`
		TokenRef          string            `yaml:"token_ref"`
		HeartbeatInterval string            `yaml:"heartbeat_interval"`
		Labels            map[string]string `yaml:"labels,omitempty"`
		Capabilities      []string          `yaml:"capabilities"`
	}{
		ServerURL:         firstNonEmpty(os.Getenv("AIOPS_AGENT_SERVER_URL"), host.AgentURL, "http://127.0.0.1:18080"),
		GRPCURL:           firstNonEmpty(os.Getenv("AIOPS_AGENT_GRPC_URL"), derivedAgentGRPCURL(os.Getenv("AIOPS_AGENT_SERVER_URL"))),
		HostID:            host.ID,
		ListenAddr:        "0.0.0.0:7072",
		TokenRef:          layout.TokenPath,
		HeartbeatInterval: "15s",
		Labels:            cloneStringMap(host.Labels),
		Capabilities:      []string{"script.shell", "script.python", "terminal"},
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, hostAgentRemoteLayout{}, err
	}
	if strings.TrimSpace(token) == "" {
		return nil, hostAgentRemoteLayout{}, fmt.Errorf("host-agent token is empty")
	}
	return data, layout, nil
}

func derivedAgentGRPCURL(serverURL string) string {
	trimmed := strings.TrimSpace(serverURL)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Hostname() == "" {
		return ""
	}
	return net.JoinHostPort(parsed.Hostname(), "18090")
}

func remoteLayout(platform detectedHostPlatform) (hostAgentRemoteLayout, error) {
	switch platform.Platform {
	case "linux/ubuntu", "linux/amd64":
		return hostAgentRemoteLayout{
			TokenPath:       "/etc/aiops/host-agent.token",
			ConfigPath:      "/etc/aiops/host-agent.yaml",
			InstallRoot:     "/opt/aiops/host-agent",
			ServiceFileName: "aiops-host-agent.service",
		}, nil
	case "darwin/arm64":
		return hostAgentRemoteLayout{
			TokenPath:       "/usr/local/etc/aiops/host-agent.token",
			ConfigPath:      "/usr/local/etc/aiops/host-agent.yaml",
			InstallRoot:     "/usr/local/aiops/host-agent",
			ServiceFileName: "com.aiops.host-agent.plist",
		}, nil
	default:
		return hostAgentRemoteLayout{}, fmt.Errorf("unsupported platform: %s", platform.Platform)
	}
}

func sudoScriptPreamble() string {
	return strings.Join([]string{
		`if [ "$(id -u)" -eq 0 ]; then`,
		`  run_sudo() { "$@"; }`,
		`else`,
		`  sudo_password_file="$(mktemp)"`,
		`  trap 'rm -f "$sudo_password_file"' EXIT`,
		`  cat > "$sudo_password_file"`,
		`  chmod 600 "$sudo_password_file"`,
		`  run_sudo() { sudo -S -p '' "$@" < "$sudo_password_file"; }`,
		`  run_sudo true`,
		`fi`,
	}, "\n")
}

func installFilesScript(platform detectedHostPlatform, layout hostAgentRemoteLayout, remoteTmp string) string {
	sudo := sudoScriptPreamble()
	switch platform.Platform {
	case "linux/ubuntu", "linux/amd64":
		return strings.Join([]string{
			"set -eu",
			sudo,
			"run_sudo install -d -m 755 /opt/aiops/host-agent /etc/aiops",
			"run_sudo install -m 755 " + shellQuote(remoteTmp+"/host-agent") + " " + shellQuote(layout.InstallRoot+"/host-agent"),
			"run_sudo install -m 600 " + shellQuote(remoteTmp+"/host-agent.yaml") + " " + shellQuote(layout.ConfigPath),
			"run_sudo install -m 600 " + shellQuote(remoteTmp+"/host-agent.token") + " " + shellQuote(layout.TokenPath),
			"rm -f " + shellQuote(remoteTmp+"/host-agent.token"),
		}, "\n")
	case "darwin/arm64":
		return strings.Join([]string{
			"set -eu",
			sudo,
			"run_sudo install -d -m 755 /usr/local/aiops/host-agent /usr/local/etc/aiops /usr/local/var/log/aiops",
			"run_sudo install -m 755 " + shellQuote(remoteTmp+"/host-agent") + " " + shellQuote(layout.InstallRoot+"/host-agent"),
			"run_sudo install -m 600 " + shellQuote(remoteTmp+"/host-agent.yaml") + " " + shellQuote(layout.ConfigPath),
			"run_sudo install -m 600 " + shellQuote(remoteTmp+"/host-agent.token") + " " + shellQuote(layout.TokenPath),
			"rm -f " + shellQuote(remoteTmp+"/host-agent.token"),
		}, "\n")
	default:
		return "printf 'unsupported platform: " + shellSingleQuote(platform.Platform) + "\\n' >&2\nexit 65"
	}
}

func installServiceScript(platform detectedHostPlatform, layout hostAgentRemoteLayout, remoteTmp string) string {
	sudo := sudoScriptPreamble()
	switch platform.Platform {
	case "linux/ubuntu", "linux/amd64":
		return strings.Join([]string{
			"set -eu",
			sudo,
			"run_sudo install -m 644 " + shellQuote(remoteTmp+"/"+layout.ServiceFileName) + " /etc/systemd/system/aiops-host-agent.service",
			"run_sudo systemctl daemon-reload",
		}, "\n")
	case "darwin/arm64":
		return strings.Join([]string{
			"set -eu",
			sudo,
			"run_sudo install -m 644 " + shellQuote(remoteTmp+"/"+layout.ServiceFileName) + " /Library/LaunchDaemons/com.aiops.host-agent.plist",
		}, "\n")
	default:
		return "printf 'unsupported platform: " + shellSingleQuote(platform.Platform) + "\\n' >&2\nexit 65"
	}
}

func startServiceScript(platform detectedHostPlatform) string {
	sudo := sudoScriptPreamble()
	switch platform.Platform {
	case "linux/ubuntu", "linux/amd64":
		return strings.Join([]string{
			"set -eu",
			sudo,
			"run_sudo systemctl enable aiops-host-agent.service",
			"run_sudo systemctl restart aiops-host-agent.service",
			"run_sudo systemctl is-active aiops-host-agent.service",
		}, "\n")
	case "darwin/arm64":
		return strings.Join([]string{
			"set -eu",
			sudo,
			"run_sudo launchctl bootout system /Library/LaunchDaemons/com.aiops.host-agent.plist >/dev/null 2>&1 || true",
			"run_sudo launchctl bootstrap system /Library/LaunchDaemons/com.aiops.host-agent.plist",
			"run_sudo launchctl enable system/com.aiops.host-agent",
			"run_sudo launchctl kickstart -k system/com.aiops.host-agent",
			"run_sudo launchctl print system/com.aiops.host-agent >/dev/null",
		}, "\n")
	default:
		return "printf 'unsupported platform: " + shellSingleQuote(platform.Platform) + "\\n' >&2\nexit 65"
	}
}

func serviceDefinition(platform detectedHostPlatform) ([]byte, error) {
	switch platform.Platform {
	case "linux/ubuntu", "linux/amd64":
		return []byte(`[Unit]
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
`), nil
	case "darwin/arm64":
		return []byte(`<?xml version="1.0" encoding="UTF-8"?>
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
`), nil
	default:
		return nil, fmt.Errorf("unsupported platform: %s", platform.Platform)
	}
}

type goBuildHostAgentArtifactBuilder struct {
	RepoRoot string
}

func (b goBuildHostAgentArtifactBuilder) BuildHostAgentArtifact(ctx context.Context, goos, goarch, version string) (HostAgentArtifact, error) {
	goos = strings.TrimSpace(goos)
	goarch = strings.TrimSpace(goarch)
	version = strings.TrimSpace(firstNonEmpty(version, "v0.1.0"))
	if goos == "" || goarch == "" {
		return HostAgentArtifact{}, fmt.Errorf("artifact goos/goarch is required")
	}
	repoRoot := strings.TrimSpace(firstNonEmpty(b.RepoRoot, defaultHostInstallRepoRoot()))
	artifactDir := filepath.Join(repoRoot, "artifacts", "host-agent", version, goos+"-"+goarch)
	artifactPath := filepath.Join(artifactDir, "host-agent")
	if data, err := os.ReadFile(artifactPath); err == nil && len(data) > 0 {
		sum := sha256.Sum256(data)
		return HostAgentArtifact{
			Path:   artifactPath,
			Bytes:  data,
			SHA256: fmt.Sprintf("%x", sum[:]),
		}, nil
	}
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return HostAgentArtifact{}, fmt.Errorf("create host-agent artifact dir: %w", err)
	}
	cmd := exec.CommandContext(ctx, "go", "build", "-o", artifactPath, "./cmd/host-agent")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+goos, "GOARCH="+goarch)
	output, buildErr := cmd.CombinedOutput()
	if buildErr != nil {
		return HostAgentArtifact{}, fmt.Errorf("build host-agent artifact: %w: %s", buildErr, strings.TrimSpace(string(output)))
	}
	data, err := os.ReadFile(artifactPath)
	if err != nil {
		return HostAgentArtifact{}, fmt.Errorf("read host-agent artifact: %w", err)
	}
	sum := sha256.Sum256(data)
	return HostAgentArtifact{
		Path:   artifactPath,
		Bytes:  data,
		SHA256: fmt.Sprintf("%x", sum[:]),
	}, nil
}

type defaultSSHBootstrapDialer struct {
	Timeout time.Duration
}

func (d defaultSSHBootstrapDialer) DialHost(ctx context.Context, host store.HostRecord, credential ResolvedSSHCredential) (SSHBootstrapClient, error) {
	port := host.SSHPort
	if port <= 0 {
		port = 22
	}
	auth, err := sshAuthMethods(credential)
	if err != nil {
		return nil, err
	}
	timeout := d.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	config := &ssh.ClientConfig{
		User:            strings.TrimSpace(host.SSHUser),
		Auth:            auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         timeout,
	}
	address := net.JoinHostPort(strings.TrimSpace(host.Address), strconv.Itoa(port))
	conn, err := (&net.Dialer{Timeout: timeout}).DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("ssh tcp connect failed: %w", err)
	}
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, address, config)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ssh authentication failed: %w", err)
	}
	return &goSSHBootstrapClient{client: ssh.NewClient(sshConn, chans, reqs)}, nil
}

func sshAuthMethods(credential ResolvedSSHCredential) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod
	if keyPath := strings.TrimSpace(credential.PrivateKeyPath); keyPath != "" {
		data, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("read ssh private key: %w", err)
		}
		signer, err := ssh.ParsePrivateKey(data)
		if err != nil {
			return nil, fmt.Errorf("parse ssh private key: %w", err)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}
	if password := strings.TrimSpace(credential.Password); password != "" {
		methods = append(methods,
			ssh.Password(password),
			ssh.KeyboardInteractive(func(_ string, _ string, questions []string, _ []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for idx := range answers {
					answers[idx] = password
				}
				return answers, nil
			}),
		)
	}
	if len(methods) == 0 {
		methods = append(methods, defaultSSHAuthMethods()...)
	}
	return methods, nil
}

func defaultSSHAuthMethods() []ssh.AuthMethod {
	var methods []ssh.AuthMethod
	if sock := strings.TrimSpace(os.Getenv("SSH_AUTH_SOCK")); sock != "" {
		methods = append(methods, ssh.PublicKeysCallback(func() ([]ssh.Signer, error) {
			conn, err := net.Dial("unix", sock)
			if err != nil {
				return nil, err
			}
			defer func() { _ = conn.Close() }()
			signers, err := agent.NewClient(conn).Signers()
			if err != nil {
				return nil, err
			}
			if len(signers) == 0 {
				return nil, fmt.Errorf("ssh agent has no identities")
			}
			return signers, nil
		}))
	}
	for _, keyPath := range defaultSSHKeyPaths() {
		data, err := os.ReadFile(keyPath)
		if err != nil {
			continue
		}
		signer, err := ssh.ParsePrivateKey(data)
		if err != nil {
			continue
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}
	return methods
}

func defaultSSHKeyPaths() []string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return nil
	}
	return []string{
		filepath.Join(home, ".ssh", "id_ed25519"),
		filepath.Join(home, ".ssh", "id_ecdsa"),
		filepath.Join(home, ".ssh", "id_rsa"),
		filepath.Join(home, ".ssh", "id_dsa"),
	}
}

type goSSHBootstrapClient struct {
	client *ssh.Client
}

func (c *goSSHBootstrapClient) Run(ctx context.Context, command string, stdin []byte) (SSHBootstrapResult, error) {
	if c == nil || c.client == nil {
		return SSHBootstrapResult{}, fmt.Errorf("ssh client is not connected")
	}
	session, err := c.client.NewSession()
	if err != nil {
		return SSHBootstrapResult{}, err
	}
	defer func() { _ = session.Close() }()
	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr
	if stdin != nil {
		session.Stdin = bytes.NewReader(stdin)
	}
	done := make(chan error, 1)
	go func() {
		done <- session.Run(command)
	}()
	select {
	case <-ctx.Done():
		_ = session.Close()
		return SSHBootstrapResult{Stdout: stdout.String(), Stderr: stderr.String()}, ctx.Err()
	case err := <-done:
		result := SSHBootstrapResult{Stdout: stdout.String(), Stderr: stderr.String()}
		if err != nil {
			return result, fmt.Errorf("ssh command failed: %w", err)
		}
		return result, nil
	}
}

func (c *goSSHBootstrapClient) Close() error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Close()
}

func parseOSRelease(data string) map[string]string {
	fields := map[string]string{}
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if key != "" {
			fields[key] = value
		}
	}
	return fields
}

func normalizeUnameOS(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "linux":
		return "linux"
	case "darwin":
		return "darwin"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func normalizeArch(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "x86_64", "amd64":
		return "amd64"
	case "arm64", "aarch64":
		return "arm64"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func unsupportedPlatformLabel(platform detectedHostPlatform) string {
	if strings.TrimSpace(platform.Platform) != "" {
		return platform.Platform
	}
	if platform.OS != "" {
		return strings.TrimRight(platform.OS+"/"+platform.Arch, "/")
	}
	return strings.TrimSpace(strings.Join([]string{platform.UnameOS, platform.UnameArch}, "/"))
}

func firstOutputLine(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if trimmed := strings.TrimSpace(strings.TrimRight(line, "\r")); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func safeRemoteName(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range input {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func newDirectHostInstallRunID(hostID string) string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("direct-%s-%d", safeRemoteName(hostID), time.Now().UTC().UnixNano())
	}
	return fmt.Sprintf("direct-%s-%x", safeRemoteName(hostID), raw[:])
}

func newHostAgentInstallToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate host-agent token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func redactSSHTestError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s", redactInstallError(err.Error()))
}
