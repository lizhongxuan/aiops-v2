package appui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aiops-v2/internal/store"
)

type fakeSSHCredentialResolver struct {
	credential ResolvedSSHCredential
	err        error
	refs       []string
}

func (f *fakeSSHCredentialResolver) ResolveSSHCredential(_ context.Context, ref string) (ResolvedSSHCredential, error) {
	f.refs = append(f.refs, ref)
	if f.err != nil {
		return ResolvedSSHCredential{}, f.err
	}
	return f.credential, nil
}

type fakeSSHBootstrapDialer struct {
	client      *fakeSSHBootstrapClient
	err         error
	calls       int
	credentials []ResolvedSSHCredential
}

func (f *fakeSSHBootstrapDialer) DialHost(_ context.Context, _ store.HostRecord, credential ResolvedSSHCredential) (SSHBootstrapClient, error) {
	f.calls++
	f.credentials = append(f.credentials, credential)
	if f.err != nil {
		return nil, f.err
	}
	return f.client, nil
}

type fakeSSHBootstrapClient struct {
	responses map[string]SSHBootstrapResult
	errors    map[string]error
	commands  []string
	stdins    [][]byte
	runs      []fakeSSHRun
	closed    bool
}

type fakeSSHRun struct {
	command string
	stdin   []byte
}

func (f *fakeSSHBootstrapClient) Run(_ context.Context, command string, stdin []byte) (SSHBootstrapResult, error) {
	f.commands = append(f.commands, command)
	f.runs = append(f.runs, fakeSSHRun{
		command: command,
		stdin:   append([]byte(nil), stdin...),
	})
	if stdin != nil {
		cp := append([]byte(nil), stdin...)
		f.stdins = append(f.stdins, cp)
	}
	if err := f.errors[command]; err != nil {
		return SSHBootstrapResult{}, err
	}
	if result, ok := f.responses[command]; ok {
		return result, nil
	}
	return SSHBootstrapResult{}, nil
}

func (f *fakeSSHBootstrapClient) Close() error {
	f.closed = true
	return nil
}

type fakeHostAgentArtifactBuilder struct {
	artifact HostAgentArtifact
	err      error
	calls    []string
}

func (f *fakeHostAgentArtifactBuilder) BuildHostAgentArtifact(_ context.Context, goos, goarch, version string) (HostAgentArtifact, error) {
	f.calls = append(f.calls, goos+"/"+goarch+":"+version)
	if f.err != nil {
		return HostAgentArtifact{}, f.err
	}
	return f.artifact, nil
}

func TestGoBuildHostAgentArtifactBuilderUsesPrebuiltArtifactWithoutGo(t *testing.T) {
	repoRoot := t.TempDir()
	artifactDir := filepath.Join(repoRoot, "artifacts", "host-agent", "v0.1.0", "linux-amd64")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	artifactPath := filepath.Join(artifactDir, "host-agent")
	if err := os.WriteFile(artifactPath, []byte("prebuilt-host-agent"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv("PATH", "")

	artifact, err := (goBuildHostAgentArtifactBuilder{RepoRoot: repoRoot}).BuildHostAgentArtifact(context.Background(), "linux", "amd64", "v0.1.0")
	if err != nil {
		t.Fatalf("BuildHostAgentArtifact() error = %v", err)
	}
	if artifact.Path != artifactPath {
		t.Fatalf("Path = %q, want %q", artifact.Path, artifactPath)
	}
	if string(artifact.Bytes) != "prebuilt-host-agent" {
		t.Fatalf("Bytes = %q, want prebuilt-host-agent", string(artifact.Bytes))
	}
}

func TestGoBuildHostAgentArtifactBuilderRequiresPrebuiltArtifact(t *testing.T) {
	repoRoot := t.TempDir()
	t.Setenv("PATH", "")

	_, err := (goBuildHostAgentArtifactBuilder{RepoRoot: repoRoot}).BuildHostAgentArtifact(context.Background(), "linux", "amd64", "v0.1.0")
	if err == nil {
		t.Fatal("BuildHostAgentArtifact() error = nil, want missing prebuilt artifact error")
	}
	if !strings.Contains(err.Error(), "prebuilt Node artifact") || !strings.Contains(err.Error(), "scripts/build-node-artifacts.sh") {
		t.Fatalf("error = %q, want prebuilt artifact build instruction", err.Error())
	}
}

func TestDirectHostAgentInstallerRejectsNonAMD64LinuxAndRedactsCredential(t *testing.T) {
	repo := newHostRepoStub(store.HostRecord{
		ID:               "arm-linux-smoke",
		Address:          "120.77.239.90",
		SSHUser:          "root",
		SSHPort:          22,
		SSHCredentialRef: "secret://lab/arm-linux-smoke",
		AgentVersion:     "v0.1.0",
		AgentServerURL:   "http://aiops.example.test:18080",
	})
	client := &fakeSSHBootstrapClient{responses: map[string]SSHBootstrapResult{
		"uname -s":                        {Stdout: "Linux\n"},
		"uname -m":                        {Stdout: "aarch64\n"},
		"cat /etc/os-release 2>/dev/null": {Stdout: "NAME=\"Alibaba Cloud Linux\"\nID=\"alinux\"\nID_LIKE=\"rhel fedora centos anolis\"\n"},
		"command -v sudo >/dev/null 2>&1": {},
		"if [ \"$(id -u)\" -eq 0 ]; then echo root; elif command -v sudo >/dev/null 2>&1; then echo sudo; else echo none; fi": {Stdout: "root\n"},
	}}
	resolver := &fakeSSHCredentialResolver{credential: ResolvedSSHCredential{
		Ref:      "secret://lab/arm-linux-smoke",
		Password: "do-not-leak",
	}}
	installer := NewDirectHostAgentInstaller(
		repo,
		resolver,
		WithSSHBootstrapDialer(&fakeSSHBootstrapDialer{client: client}),
		WithHostAgentArtifactBuilder(&fakeHostAgentArtifactBuilder{}),
	)

	run, err := installer.Install(context.Background(), "arm-linux-smoke", HostInstallRequest{AgentVersion: "v0.1.0"})
	if err == nil {
		t.Fatal("Install() error = nil, want unsupported platform")
	}
	if run.Status != "failed" || run.CurrentStep != "detect-platform" || run.Platform != "linux/alinux" {
		t.Fatalf("run = %+v, want failed detect-platform for non-amd64 linux", run)
	}
	saved, err := repo.GetHost("arm-linux-smoke")
	if err != nil {
		t.Fatalf("GetHost() error = %v", err)
	}
	if saved.Status != "install_failed" || saved.InstallState != "unsupported_platform" || saved.InstallStep != "detect-platform" {
		t.Fatalf("saved host = %+v", saved)
	}
	if strings.Contains(saved.LastError, "do-not-leak") {
		t.Fatalf("LastError leaked credential: %q", saved.LastError)
	}
	if !client.closed {
		t.Fatal("ssh client was not closed")
	}
}

func TestDirectHostAgentInstallerDefaultsToAIOPSPullWithoutCallbackURL(t *testing.T) {
	repo := newHostRepoStub(store.HostRecord{
		ID:               "remote-smoke",
		Address:          "120.77.239.90",
		SSHUser:          "root",
		SSHPort:          22,
		SSHCredentialRef: "secret://lab/remote-smoke",
		AgentVersion:     "v0.1.0",
	})
	client := &fakeSSHBootstrapClient{responses: map[string]SSHBootstrapResult{
		"uname -s":                        {Stdout: "Linux\n"},
		"uname -m":                        {Stdout: "x86_64\n"},
		"cat /etc/os-release 2>/dev/null": {Stdout: "NAME=\"Alibaba Cloud Linux\"\nID=\"alinux\"\nID_LIKE=\"rhel fedora centos anolis\"\n"},
		"if [ \"$(id -u)\" -eq 0 ]; then echo root; elif command -v sudo >/dev/null 2>&1; then echo sudo; else echo none; fi": {Stdout: "root\n"},
		hostAgentLocalDiagnosticsCommand(): {Stdout: "ok\n"},
	}}
	dialer := &fakeSSHBootstrapDialer{client: client}
	installer := NewDirectHostAgentInstaller(
		repo,
		&fakeSSHCredentialResolver{credential: ResolvedSSHCredential{Ref: "secret://lab/remote-smoke", Password: "do-not-leak"}},
		WithSSHBootstrapDialer(dialer),
		WithHostAgentArtifactBuilder(&fakeHostAgentArtifactBuilder{artifact: HostAgentArtifact{
			Bytes:  []byte("host-agent-binary"),
			SHA256: "sha256-test",
		}}),
	)

	run, err := installer.Install(context.Background(), "remote-smoke", HostInstallRequest{AgentVersion: "v0.1.0"})
	if err != nil {
		t.Fatalf("Install() error = %v, want default aiops_pull install to skip callback validation", err)
	}
	if run.Status != "success" || run.CurrentStep != "finalize-host" {
		t.Fatalf("run = %+v, want successful aiops_pull install", run)
	}
	if dialer.calls != 1 {
		t.Fatalf("dialer.calls = %d, want SSH install to continue", dialer.calls)
	}
	uploaded := ""
	for _, stdin := range client.stdins {
		text := string(stdin)
		if strings.Contains(text, "connection_mode:") {
			uploaded = text
			break
		}
	}
	if !strings.Contains(uploaded, "connection_mode: aiops_pull") {
		t.Fatalf("uploaded config = %q, want aiops_pull", uploaded)
	}
	if strings.Contains(uploaded, "server_url:") || strings.Contains(uploaded, "grpc_url:") {
		t.Fatalf("uploaded config = %q, want no push callback fields in aiops_pull mode", uploaded)
	}
	saved, err := repo.GetHost("remote-smoke")
	if err != nil {
		t.Fatalf("GetHost() error = %v", err)
	}
	if saved.ConnectionMode != HostConnectionModeAIOPSPull {
		t.Fatalf("saved ConnectionMode = %q, want aiops_pull", saved.ConnectionMode)
	}
}

func TestDirectHostAgentInstallerRejectsLoopbackCallbackForNodePushGRPCInstall(t *testing.T) {
	repo := newHostRepoStub(store.HostRecord{
		ID:               "remote-smoke",
		Address:          "120.77.239.90",
		SSHUser:          "root",
		SSHPort:          22,
		SSHCredentialRef: "secret://lab/remote-smoke",
		AgentVersion:     "v0.1.0",
	})
	dialer := &fakeSSHBootstrapDialer{client: &fakeSSHBootstrapClient{}}
	installer := NewDirectHostAgentInstaller(
		repo,
		&fakeSSHCredentialResolver{credential: ResolvedSSHCredential{Ref: "secret://lab/remote-smoke", Password: "do-not-leak"}},
		WithSSHBootstrapDialer(dialer),
		WithHostAgentArtifactBuilder(&fakeHostAgentArtifactBuilder{}),
	)

	run, err := installer.Install(context.Background(), "remote-smoke", HostInstallRequest{
		AgentVersion:   "v0.1.0",
		ConnectionMode: HostConnectionModeNodePushGRPC,
	})
	if err == nil {
		t.Fatal("Install() error = nil, want loopback callback rejection")
	}
	if !strings.Contains(err.Error(), "loopback") || !strings.Contains(err.Error(), "remote host 120.77.239.90 cannot reach it") {
		t.Fatalf("Install() error = %q, want loopback callback guidance", err.Error())
	}
	if run.Status != "failed" || run.CurrentStep != "validate-agent-server-url" {
		t.Fatalf("run = %+v, want failed validate-agent-server-url", run)
	}
	if dialer.calls != 0 {
		t.Fatalf("dialer.calls = %d, want validation to fail before SSH", dialer.calls)
	}
	saved, err := repo.GetHost("remote-smoke")
	if err != nil {
		t.Fatalf("GetHost() error = %v", err)
	}
	if saved.Status != "install_failed" || saved.InstallState != "failed" || saved.InstallStep != "validate-agent-server-url" {
		t.Fatalf("saved host = %+v, want failed validate-agent-server-url", saved)
	}
	if !strings.Contains(saved.LastError, "loopback") {
		t.Fatalf("LastError = %q, want loopback callback guidance", saved.LastError)
	}
}

func TestDirectHostAgentInstallerIgnoresSavedHostAgentEndpointAsAgentServerURL(t *testing.T) {
	repo := newHostRepoStub(store.HostRecord{
		ID:               "remote-smoke",
		Address:          "120.77.239.90",
		SSHUser:          "root",
		SSHPort:          22,
		SSHCredentialRef: "secret://lab/remote-smoke",
		AgentVersion:     "v0.1.0",
		AgentServerURL:   "http://120.77.239.90:7072",
	})
	client := &fakeSSHBootstrapClient{responses: map[string]SSHBootstrapResult{
		"uname -s":                        {Stdout: "Linux\n"},
		"uname -m":                        {Stdout: "x86_64\n"},
		"cat /etc/os-release 2>/dev/null": {Stdout: "NAME=\"Alibaba Cloud Linux\"\nID=\"alinux\"\nID_LIKE=\"rhel fedora centos anolis\"\n"},
		"if [ \"$(id -u)\" -eq 0 ]; then echo root; elif command -v sudo >/dev/null 2>&1; then echo sudo; else echo none; fi": {Stdout: "root\n"},
		hostAgentLocalDiagnosticsCommand(): {Stdout: "ok\n"},
	}}
	installer := NewDirectHostAgentInstaller(
		repo,
		&fakeSSHCredentialResolver{credential: ResolvedSSHCredential{Ref: "secret://lab/remote-smoke", Password: "do-not-leak"}},
		WithSSHBootstrapDialer(&fakeSSHBootstrapDialer{client: client}),
		WithHostAgentArtifactBuilder(&fakeHostAgentArtifactBuilder{artifact: HostAgentArtifact{
			Bytes:  []byte("host-agent-binary"),
			SHA256: "sha256-test",
		}}),
	)

	run, err := installer.Install(context.Background(), "remote-smoke", HostInstallRequest{
		AgentVersion:   "v0.1.0",
		AgentServerURL: "http://aiops.example.test:18080",
		ConnectionMode: HostConnectionModeNodePushGRPC,
	})
	if err != nil {
		t.Fatalf("Install() error = %v, want saved Node endpoint to be ignored", err)
	}
	if run.Status != "success" || run.CurrentStep != "finalize-host" {
		t.Fatalf("run = %+v, want successful direct install", run)
	}
	uploaded := ""
	for _, stdin := range client.stdins {
		text := string(stdin)
		if strings.Contains(text, "server_url:") {
			uploaded = text
			break
		}
	}
	if !strings.Contains(uploaded, "server_url: http://aiops.example.test:18080") {
		t.Fatalf("uploaded config = %q, want requested ai-server callback URL", uploaded)
	}
	if !strings.Contains(uploaded, "connection_mode: node_push_grpc") {
		t.Fatalf("uploaded config = %q, want node_push_grpc mode", uploaded)
	}
	if strings.Contains(uploaded, "server_url: http://120.77.239.90:7072") {
		t.Fatalf("uploaded config = %q, must not use saved Node endpoint as server_url", uploaded)
	}
}

func TestResolveInstallAgentServerURLDoesNotUseHostAgentEndpoint(t *testing.T) {
	got := resolveInstallAgentServerURL(store.HostRecord{
		ID:       "remote-smoke",
		Address:  "120.77.239.90",
		AgentURL: "http://120.77.239.90:7072",
	})
	if got == "http://120.77.239.90:7072" {
		t.Fatalf("resolveInstallAgentServerURL() = %q, must not reuse host-agent endpoint as ai-server callback URL", got)
	}
	if got != "http://127.0.0.1:18080" {
		t.Fatalf("resolveInstallAgentServerURL() = %q, want default ai-server URL", got)
	}
}

func TestResolveInstallAgentServerURLIgnoresSavedHostAgentEndpoint(t *testing.T) {
	got := resolveInstallAgentServerURL(store.HostRecord{
		ID:             "remote-smoke",
		Address:        "120.77.239.90",
		AgentServerURL: "http://120.77.239.90:7072",
	})
	if got == "http://120.77.239.90:7072" {
		t.Fatalf("resolveInstallAgentServerURL() = %q, must not write Node endpoint as ai-server callback URL", got)
	}
	if got != "http://127.0.0.1:18080" {
		t.Fatalf("resolveInstallAgentServerURL() = %q, want default ai-server URL", got)
	}
}

func TestBuildHostAgentRemoteConfigUsesAgentServerURLBeforeAgentEndpoint(t *testing.T) {
	config, _, err := buildHostAgentRemoteConfig(store.HostRecord{
		ID:             "remote-smoke",
		Address:        "120.77.239.90",
		AgentURL:       "http://120.77.239.90:7072",
		AgentServerURL: "http://aiops.example.test:18080",
		ConnectionMode: HostConnectionModeNodePushGRPC,
	}, detectedHostPlatform{OS: "linux", Arch: "amd64", Platform: "linux/ubuntu"}, "agent-token")
	if err != nil {
		t.Fatalf("buildHostAgentRemoteConfig() error = %v", err)
	}
	data := string(config)
	if !strings.Contains(data, "server_url: http://aiops.example.test:18080") {
		t.Fatalf("config = %s, want AgentServerURL as server_url", data)
	}
	if strings.Contains(data, "server_url: http://120.77.239.90:7072") {
		t.Fatalf("config = %s, must not use host-agent endpoint as server_url", data)
	}
}

func TestDirectHostAgentInstallerSSHTestDialsAndDetectsUbuntu(t *testing.T) {
	repo := newHostRepoStub(store.HostRecord{
		ID:               "ubuntu-smoke",
		Address:          "10.0.0.11",
		SSHUser:          "ubuntu",
		SSHPort:          22,
		SSHCredentialRef: "secret://lab/ubuntu-smoke",
	})
	client := &fakeSSHBootstrapClient{responses: map[string]SSHBootstrapResult{
		"uname -s":                        {Stdout: "Linux\n"},
		"uname -m":                        {Stdout: "x86_64\n"},
		"cat /etc/os-release 2>/dev/null": {Stdout: "NAME=\"Ubuntu\"\nID=ubuntu\nID_LIKE=debian\n"},
		"if [ \"$(id -u)\" -eq 0 ]; then echo root; elif command -v sudo >/dev/null 2>&1; then echo sudo; else echo none; fi": {Stdout: "sudo\n"},
	}}
	dialer := &fakeSSHBootstrapDialer{client: client}
	resolver := &fakeSSHCredentialResolver{credential: ResolvedSSHCredential{
		Ref:      "secret://lab/ubuntu-smoke",
		Password: "do-not-leak",
	}}
	installer := NewDirectHostAgentInstaller(repo, resolver, WithSSHBootstrapDialer(dialer))

	resp, err := installer.TestSSH(context.Background(), "ubuntu-smoke", "")
	if err != nil {
		t.Fatalf("TestSSH() error = %v", err)
	}
	if resp.Status != "ok" || resp.Platform != "linux/ubuntu" || resp.OS != "linux" || resp.Arch != "amd64" || resp.Sudo != "sudo" {
		t.Fatalf("response = %+v", resp)
	}
	if dialer.calls != 1 {
		t.Fatalf("dialer.calls = %d, want 1", dialer.calls)
	}
	if len(resolver.refs) != 1 || resolver.refs[0] != "secret://lab/ubuntu-smoke" {
		t.Fatalf("resolved refs = %#v", resolver.refs)
	}
}

func TestHostBootstrapServicePrefersDirectInstallerOverWorkflowRunner(t *testing.T) {
	repo := newHostRepoStub(store.HostRecord{
		ID:               "ubuntu-smoke",
		Address:          "10.0.0.11",
		SSHUser:          "ubuntu",
		SSHPort:          22,
		SSHCredentialRef: "secret://lab/ubuntu-smoke",
	})
	runner := &fakeHostBootstrapRunner{err: errors.New("runner should not be used")}
	installer := &fakeHostAgentInstaller{run: HostInstallRun{RunID: "direct-1", Status: "success", CurrentStep: "finalize-host"}}
	service := NewHostBootstrapService(repo, runner, WithHostAgentInstaller(installer))

	run, err := service.Install(context.Background(), "ubuntu-smoke", HostInstallRequest{AgentVersion: "v0.1.0"})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if run.RunID != "direct-1" {
		t.Fatalf("RunID = %q, want direct-1", run.RunID)
	}
	if !installer.called {
		t.Fatal("direct installer was not called")
	}
	if runner.submitCalled {
		t.Fatal("workflow runner was called")
	}
}

func TestDirectHostAgentInstallerInstallsUbuntuAgentWithScriptedCommands(t *testing.T) {
	repo := newHostRepoStub(store.HostRecord{
		ID:               "ubuntu-smoke",
		Address:          "10.0.0.11",
		SSHUser:          "ubuntu",
		SSHPort:          22,
		SSHCredentialRef: "secret://lab/ubuntu-smoke",
		AgentVersion:     "v0.1.0",
		AgentServerURL:   "http://aiops.example.test:18080",
	})
	client := &fakeSSHBootstrapClient{responses: map[string]SSHBootstrapResult{
		"uname -s":                        {Stdout: "Linux\n"},
		"uname -m":                        {Stdout: "x86_64\n"},
		"cat /etc/os-release 2>/dev/null": {Stdout: "NAME=\"Ubuntu\"\nID=ubuntu\nID_LIKE=debian\n"},
		"if [ \"$(id -u)\" -eq 0 ]; then echo root; elif command -v sudo >/dev/null 2>&1; then echo sudo; else echo none; fi": {Stdout: "root\n"},
		hostAgentLocalDiagnosticsCommand(): {Stdout: "ok\n"},
	}}
	builder := &fakeHostAgentArtifactBuilder{artifact: HostAgentArtifact{
		Bytes:  []byte("host-agent-binary"),
		SHA256: "sha256-test",
	}}
	tokenStore := NewLocalHostAgentTokenStore(t.TempDir())
	installer := NewDirectHostAgentInstaller(
		repo,
		&fakeSSHCredentialResolver{credential: ResolvedSSHCredential{Ref: "secret://lab/ubuntu-smoke", Password: "do-not-leak"}},
		WithSSHBootstrapDialer(&fakeSSHBootstrapDialer{client: client}),
		WithHostAgentArtifactBuilder(builder),
		WithDirectHostAgentTokenStore(tokenStore),
	)

	run, err := installer.Install(context.Background(), "ubuntu-smoke", HostInstallRequest{AgentVersion: "v0.1.0"})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if run.Status != "success" || run.CurrentStep != "finalize-host" || run.WorkflowID != "" {
		t.Fatalf("run = %+v", run)
	}
	if len(builder.calls) != 1 || builder.calls[0] != "linux/amd64:v0.1.0" {
		t.Fatalf("builder calls = %#v", builder.calls)
	}
	saved, err := repo.GetHost("ubuntu-smoke")
	if err != nil {
		t.Fatalf("GetHost() error = %v", err)
	}
	if saved.Status != "online" || saved.InstallState != "installed" || saved.InstallWorkflowID != "" || saved.AgentTokenRef == "" {
		t.Fatalf("saved host = %+v", saved)
	}
	if saved.AgentTokenSecretRef == "" {
		t.Fatalf("AgentTokenSecretRef is empty")
	}
	if token, err := tokenStore.ResolveHostAgentToken(context.Background(), saved.AgentTokenSecretRef); err != nil || strings.TrimSpace(token) == "" {
		t.Fatalf("ResolveHostAgentToken() token=%q err=%v, want stored token", token, err)
	}
	joinedCommands := strings.Join(client.commands, "\n")
	for _, forbidden := range []string{"llm.", "prompt.", "chat.", "completion.", "do-not-leak"} {
		if strings.Contains(strings.ToLower(joinedCommands), forbidden) {
			t.Fatalf("remote commands contained forbidden text %q:\n%s", forbidden, joinedCommands)
		}
	}
	if !strings.Contains(joinedCommands, "systemctl restart aiops-host-agent.service") {
		t.Fatalf("commands did not restart systemd service:\n%s", joinedCommands)
	}
	if len(client.stdins) == 0 {
		t.Fatal("expected installer to upload files over ssh stdin")
	}
}

func TestStartServiceScriptRetriesSystemdRestartBeforeFailing(t *testing.T) {
	script := startServiceScript(detectedHostPlatform{Platform: "linux/amd64"})

	for _, required := range []string{
		"for attempt in 1 2 3; do",
		"run_sudo systemctl stop aiops-host-agent.service >/dev/null 2>&1 || true",
		"pgrep -f '[/]opt/aiops/host-agent/host-agent --config /etc/aiops/host-agent.yaml'",
		"run_sudo kill",
		"if run_sudo systemctl restart aiops-host-agent.service; then",
		"run_sudo systemctl is-active --quiet aiops-host-agent.service",
		"sleep 2",
		"run_sudo journalctl -u aiops-host-agent.service -n 80 --no-pager >&2 || true",
	} {
		if !strings.Contains(script, required) {
			t.Fatalf("startServiceScript() missing %q:\n%s", required, script)
		}
	}
}

func TestDirectHostAgentInstallerUsesPasswordStdinForNonInteractiveSudo(t *testing.T) {
	repo := newHostRepoStub(store.HostRecord{
		ID:               "ubuntu-smoke",
		Address:          "10.0.0.11",
		SSHUser:          "kduser",
		SSHPort:          22,
		SSHCredentialRef: "secret://lab/ubuntu-smoke",
		AgentVersion:     "v0.1.0",
		AgentServerURL:   "http://aiops.example.test:18080",
	})
	client := &fakeSSHBootstrapClient{responses: map[string]SSHBootstrapResult{
		"uname -s":                        {Stdout: "Linux\n"},
		"uname -m":                        {Stdout: "x86_64\n"},
		"cat /etc/os-release 2>/dev/null": {Stdout: "NAME=\"Ubuntu\"\nID=ubuntu\nID_LIKE=debian\n"},
		"if [ \"$(id -u)\" -eq 0 ]; then echo root; elif command -v sudo >/dev/null 2>&1; then echo sudo; else echo none; fi": {Stdout: "sudo\n"},
		hostAgentLocalDiagnosticsCommand(): {Stdout: "ok\n"},
	}}
	builder := &fakeHostAgentArtifactBuilder{artifact: HostAgentArtifact{
		Bytes:  []byte("host-agent-binary"),
		SHA256: "sha256-test",
	}}
	installer := NewDirectHostAgentInstaller(
		repo,
		&fakeSSHCredentialResolver{credential: ResolvedSSHCredential{Ref: "secret://lab/ubuntu-smoke", Password: "sudo-password"}},
		WithSSHBootstrapDialer(&fakeSSHBootstrapDialer{client: client}),
		WithHostAgentArtifactBuilder(builder),
	)

	if _, err := installer.Install(context.Background(), "ubuntu-smoke", HostInstallRequest{AgentVersion: "v0.1.0"}); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	privilegedRuns := 0
	for _, run := range client.runs {
		if !strings.Contains(run.command, "systemctl") && !strings.Contains(run.command, " install ") {
			continue
		}
		privilegedRuns++
		if !strings.Contains(run.command, "sudo -S -p ''") {
			t.Fatalf("privileged command did not use non-interactive sudo:\n%s", run.command)
		}
		if string(run.stdin) != "sudo-password\n" {
			t.Fatalf("sudo stdin = %q, want password newline", string(run.stdin))
		}
		if strings.Contains(run.command, "sudo-password") {
			t.Fatalf("sudo password leaked into command text:\n%s", run.command)
		}
	}
	if privilegedRuns == 0 {
		t.Fatal("no privileged installer commands were captured")
	}
}

func TestDirectHostAgentInstallerInstallsWithDefaultSSHAuthWhenCredentialRefEmpty(t *testing.T) {
	repo := newHostRepoStub(store.HostRecord{
		ID:             "ubuntu-smoke",
		Address:        "10.0.0.11",
		SSHUser:        "ubuntu",
		SSHPort:        22,
		AgentVersion:   "v0.1.0",
		AgentServerURL: "http://aiops.example.test:18080",
	})
	client := &fakeSSHBootstrapClient{responses: map[string]SSHBootstrapResult{
		"uname -s":                        {Stdout: "Linux\n"},
		"uname -m":                        {Stdout: "x86_64\n"},
		"cat /etc/os-release 2>/dev/null": {Stdout: "NAME=\"Ubuntu\"\nID=ubuntu\nID_LIKE=debian\n"},
		"if [ \"$(id -u)\" -eq 0 ]; then echo root; elif command -v sudo >/dev/null 2>&1; then echo sudo; else echo none; fi": {Stdout: "sudo\n"},
		hostAgentLocalDiagnosticsCommand(): {Stdout: "ok\n"},
	}}
	builder := &fakeHostAgentArtifactBuilder{artifact: HostAgentArtifact{
		Bytes:  []byte("host-agent-binary"),
		SHA256: "sha256-test",
	}}
	resolver := &fakeSSHCredentialResolver{err: errors.New("resolver should not be used for empty credential ref")}
	dialer := &fakeSSHBootstrapDialer{client: client}
	installer := NewDirectHostAgentInstaller(
		repo,
		resolver,
		WithSSHBootstrapDialer(dialer),
		WithHostAgentArtifactBuilder(builder),
	)

	run, err := installer.Install(context.Background(), "ubuntu-smoke", HostInstallRequest{AgentVersion: "v0.1.0"})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if run.Status != "success" {
		t.Fatalf("run = %+v, want success", run)
	}
	if len(resolver.refs) != 0 {
		t.Fatalf("resolved refs = %#v, want none", resolver.refs)
	}
	if len(dialer.credentials) != 1 {
		t.Fatalf("dialer.credentials length = %d, want 1", len(dialer.credentials))
	}
	if dialer.credentials[0].Ref != "" || dialer.credentials[0].PrivateKeyPath != "" || dialer.credentials[0].Password != "" || dialer.credentials[0].Cleanup != nil {
		t.Fatalf("dialer credential = %+v, want empty default auth credential", dialer.credentials[0])
	}
}

func TestSSHAuthMethodsAllowNoExplicitCredential(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	methods, err := sshAuthMethods(ResolvedSSHCredential{})
	if err != nil {
		t.Fatalf("sshAuthMethods() error = %v", err)
	}
	if len(methods) != 0 {
		t.Fatalf("len(methods) = %d, want 0 without default keys or agent", len(methods))
	}
}

func TestDirectHostAgentInstallerInstallsGenericLinuxAMD64AgentWithSystemd(t *testing.T) {
	repo := newHostRepoStub(store.HostRecord{
		ID:               "alinux-smoke",
		Address:          "120.77.239.90",
		SSHUser:          "root",
		SSHPort:          22,
		SSHCredentialRef: "secret://lab/alinux-smoke",
		AgentVersion:     "v0.1.0",
		AgentServerURL:   "http://aiops.example.test:18080",
	})
	client := &fakeSSHBootstrapClient{responses: map[string]SSHBootstrapResult{
		"uname -s":                        {Stdout: "Linux\n"},
		"uname -m":                        {Stdout: "x86_64\n"},
		"cat /etc/os-release 2>/dev/null": {Stdout: "NAME=\"Alibaba Cloud Linux\"\nID=\"alinux\"\nID_LIKE=\"rhel fedora centos anolis\"\n"},
		"if [ \"$(id -u)\" -eq 0 ]; then echo root; elif command -v sudo >/dev/null 2>&1; then echo sudo; else echo none; fi": {Stdout: "root\n"},
		hostAgentLocalDiagnosticsCommand(): {Stdout: "ok\n"},
	}}
	builder := &fakeHostAgentArtifactBuilder{artifact: HostAgentArtifact{
		Bytes:  []byte("host-agent-binary"),
		SHA256: "sha256-test",
	}}
	installer := NewDirectHostAgentInstaller(
		repo,
		&fakeSSHCredentialResolver{credential: ResolvedSSHCredential{Ref: "secret://lab/alinux-smoke", Password: "do-not-leak"}},
		WithSSHBootstrapDialer(&fakeSSHBootstrapDialer{client: client}),
		WithHostAgentArtifactBuilder(builder),
	)

	run, err := installer.Install(context.Background(), "alinux-smoke", HostInstallRequest{AgentVersion: "v0.1.0"})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if run.Status != "success" || run.Platform != "linux/amd64" || run.CurrentStep != "finalize-host" {
		t.Fatalf("run = %+v", run)
	}
	if len(builder.calls) != 1 || builder.calls[0] != "linux/amd64:v0.1.0" {
		t.Fatalf("builder calls = %#v", builder.calls)
	}
	saved, err := repo.GetHost("alinux-smoke")
	if err != nil {
		t.Fatalf("GetHost() error = %v", err)
	}
	if saved.OS != "linux" || saved.Arch != "amd64" || saved.Status != "online" || saved.InstallState != "installed" {
		t.Fatalf("saved host = %+v", saved)
	}
	joinedCommands := strings.Join(client.commands, "\n")
	if !strings.Contains(joinedCommands, "systemctl restart aiops-host-agent.service") {
		t.Fatalf("commands did not restart systemd service:\n%s", joinedCommands)
	}
}

type fakeHostAgentInstaller struct {
	called bool
	run    HostInstallRun
	err    error
}

func (f *fakeHostAgentInstaller) Install(_ context.Context, _ string, _ HostInstallRequest) (HostInstallRun, error) {
	f.called = true
	if f.err != nil {
		return HostInstallRun{}, f.err
	}
	return f.run, nil
}
