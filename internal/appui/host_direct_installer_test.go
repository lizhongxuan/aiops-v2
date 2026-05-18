package appui

import (
	"context"
	"errors"
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
	client *fakeSSHBootstrapClient
	err    error
	calls  int
}

func (f *fakeSSHBootstrapDialer) DialHost(_ context.Context, _ store.HostRecord, _ ResolvedSSHCredential) (SSHBootstrapClient, error) {
	f.calls++
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
	closed    bool
}

func (f *fakeSSHBootstrapClient) Run(_ context.Context, command string, stdin []byte) (SSHBootstrapResult, error) {
	f.commands = append(f.commands, command)
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

func TestDirectHostAgentInstallerRejectsNonAMD64LinuxAndRedactsCredential(t *testing.T) {
	repo := newHostRepoStub(store.HostRecord{
		ID:               "arm-linux-smoke",
		Address:          "120.77.239.90",
		SSHUser:          "root",
		SSHPort:          22,
		SSHCredentialRef: "secret://lab/arm-linux-smoke",
		AgentVersion:     "v0.1.0",
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
	})
	client := &fakeSSHBootstrapClient{responses: map[string]SSHBootstrapResult{
		"uname -s":                        {Stdout: "Linux\n"},
		"uname -m":                        {Stdout: "x86_64\n"},
		"cat /etc/os-release 2>/dev/null": {Stdout: "NAME=\"Ubuntu\"\nID=ubuntu\nID_LIKE=debian\n"},
		"if [ \"$(id -u)\" -eq 0 ]; then echo root; elif command -v sudo >/dev/null 2>&1; then echo sudo; else echo none; fi":                                                                       {Stdout: "root\n"},
		"if command -v curl >/dev/null 2>&1; then curl -fsS 'http://127.0.0.1:7072/health'; elif command -v wget >/dev/null 2>&1; then wget -qO- 'http://127.0.0.1:7072/health'; else exit 127; fi": {Stdout: "ok\n"},
	}}
	builder := &fakeHostAgentArtifactBuilder{artifact: HostAgentArtifact{
		Bytes:  []byte("host-agent-binary"),
		SHA256: "sha256-test",
	}}
	installer := NewDirectHostAgentInstaller(
		repo,
		&fakeSSHCredentialResolver{credential: ResolvedSSHCredential{Ref: "secret://lab/ubuntu-smoke", Password: "do-not-leak"}},
		WithSSHBootstrapDialer(&fakeSSHBootstrapDialer{client: client}),
		WithHostAgentArtifactBuilder(builder),
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

func TestDirectHostAgentInstallerInstallsGenericLinuxAMD64AgentWithSystemd(t *testing.T) {
	repo := newHostRepoStub(store.HostRecord{
		ID:               "alinux-smoke",
		Address:          "120.77.239.90",
		SSHUser:          "root",
		SSHPort:          22,
		SSHCredentialRef: "secret://lab/alinux-smoke",
		AgentVersion:     "v0.1.0",
	})
	client := &fakeSSHBootstrapClient{responses: map[string]SSHBootstrapResult{
		"uname -s":                        {Stdout: "Linux\n"},
		"uname -m":                        {Stdout: "x86_64\n"},
		"cat /etc/os-release 2>/dev/null": {Stdout: "NAME=\"Alibaba Cloud Linux\"\nID=\"alinux\"\nID_LIKE=\"rhel fedora centos anolis\"\n"},
		"if [ \"$(id -u)\" -eq 0 ]; then echo root; elif command -v sudo >/dev/null 2>&1; then echo sudo; else echo none; fi":                                                                       {Stdout: "root\n"},
		"if command -v curl >/dev/null 2>&1; then curl -fsS 'http://127.0.0.1:7072/health'; elif command -v wget >/dev/null 2>&1; then wget -qO- 'http://127.0.0.1:7072/health'; else exit 127; fi": {Stdout: "ok\n"},
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
