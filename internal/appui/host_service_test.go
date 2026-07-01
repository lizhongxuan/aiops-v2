package appui

import (
	"context"
	"fmt"
	"testing"
	"time"

	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/store"
)

type hostRepoStub struct {
	items map[string]store.HostRecord
}

func newHostRepoStub(records ...store.HostRecord) *hostRepoStub {
	out := &hostRepoStub{items: map[string]store.HostRecord{}}
	for _, record := range records {
		out.items[record.ID] = record
	}
	return out
}

func (h *hostRepoStub) GetHost(id string) (*store.HostRecord, error) {
	record, ok := h.items[id]
	if !ok {
		return nil, fmt.Errorf("host not found")
	}
	cp := record
	if len(cp.Labels) > 0 {
		labels := make(map[string]string, len(cp.Labels))
		for key, value := range cp.Labels {
			labels[key] = value
		}
		cp.Labels = labels
	}
	return &cp, nil
}

func (h *hostRepoStub) ListHosts() ([]store.HostRecord, error) {
	items := make([]store.HostRecord, 0, len(h.items))
	for _, record := range h.items {
		items = append(items, record)
	}
	return items, nil
}

func (h *hostRepoStub) SaveHost(host *store.HostRecord) error {
	cp := *host
	h.items[cp.ID] = cp
	return nil
}

func (h *hostRepoStub) DeleteHost(id string) error {
	delete(h.items, id)
	return nil
}

type fakeHostSSHPasswordStore struct {
	hostID   string
	password string
	ref      string
}

func (f *fakeHostSSHPasswordStore) StoreHostSSHPassword(_ context.Context, hostID, password string) (string, error) {
	f.hostID = hostID
	f.password = password
	if f.ref != "" {
		return f.ref, nil
	}
	return "secret://hosts/" + hostID + "/ssh-password", nil
}

type fakeHostSSHInstaller struct {
	credentialRef string
	installReq    HostInstallRequest
}

func (f *fakeHostSSHInstaller) Install(_ context.Context, hostID string, req HostInstallRequest) (HostInstallRun, error) {
	f.installReq = req
	return HostInstallRun{HostID: hostID, Status: "ok", AgentVersion: req.AgentVersion}, nil
}

func (f *fakeHostSSHInstaller) TestSSH(_ context.Context, _ string, credentialRef string) (HostSSHTestResponse, error) {
	f.credentialRef = credentialRef
	return HostSSHTestResponse{Status: "ok", Message: "tested"}, nil
}

type fakeHostNodeHealthChecker struct {
	calledWith string
	result     HostNodeHealth
	err        error
}

func (f *fakeHostNodeHealthChecker) CheckHostNodeHealth(_ context.Context, host store.HostRecord) (HostNodeHealth, error) {
	f.calledWith = host.ID
	return f.result, f.err
}

func TestHostServiceCrudAndSelect(t *testing.T) {
	hostRepo := newHostRepoStub(store.HostRecord{
		ID:              "host-a",
		Name:            "web-01",
		Status:          "online",
		Executable:      true,
		TerminalCapable: true,
		Address:         "10.0.0.11",
	})
	builder := NewSnapshotBuilder(hostRepo)
	sessions := runtimekernel.NewSessionManager()

	service := NewHostService(sessions, hostRepo, builder)

	created, err := service.CreateHost(context.Background(), HostUpsert{
		ID:            "host-b",
		Name:          "web-02",
		Address:       "10.0.0.12",
		SSHUser:       "ubuntu",
		SSHPort:       22,
		Labels:        map[string]string{"env": "prod"},
		InstallViaSSH: true,
	})
	if err != nil {
		t.Fatalf("CreateHost() error = %v", err)
	}
	if created.Host.ID != "host-b" || created.Host.Transport != "manual" {
		t.Fatalf("created = %+v, want created host-b with manual transport", created)
	}

	items, err := service.ListHosts(context.Background())
	if err != nil {
		t.Fatalf("ListHosts() error = %v", err)
	}
	if len(items) < 3 {
		t.Fatalf("len(ListHosts()) = %d, want server-local + 2 hosts", len(items))
	}

	snapshot, err := service.SelectHost(context.Background(), "host-b")
	if err != nil {
		t.Fatalf("SelectHost() error = %v", err)
	}
	if snapshot.SelectedHostID != "host-b" {
		t.Fatalf("snapshot.SelectedHostID = %q, want host-b", snapshot.SelectedHostID)
	}
	if latest := sessions.GetLatest(); latest == nil || latest.HostID != "host-b" {
		t.Fatalf("latest session = %+v, want host-b selected", latest)
	}
}

func TestMapHostRecordMarksStaleAgentHeartbeat(t *testing.T) {
	summary := mapHostRecord(store.HostRecord{
		ID:                  "host-stale",
		Name:                "host-stale",
		Status:              "online",
		AgentStatus:         "online",
		RuntimeReachability: "agent_online",
		Address:             "10.0.0.11",
		SSHUser:             "root",
		LastHeartbeat:       time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339),
	})

	if summary.Status != "stale" || summary.AgentStatus != "stale" || summary.RuntimeReachability != "agent_stale" {
		t.Fatalf("summary = %+v, want stale agent heartbeat", summary)
	}
}

func TestHostServiceListHostsRefreshesAIOPSPullHealthFromReachableNode(t *testing.T) {
	oldHeartbeat := time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339)
	nodeHeartbeat := time.Now().UTC().Format(time.RFC3339)
	hostRepo := newHostRepoStub(store.HostRecord{
		ID:                  "remote-a",
		Name:                "remote-a",
		Status:              "online",
		AgentStatus:         "online",
		RuntimeReachability: "agent_online",
		Address:             "10.0.0.11",
		SSHUser:             "root",
		AgentURL:            "http://10.0.0.11:7072",
		ConnectionMode:      HostConnectionModeAIOPSPull,
		Transport:           "agent_http",
		InstallState:        "installed",
		LastHeartbeat:       oldHeartbeat,
		Executable:          true,
		TerminalCapable:     true,
	})
	checker := &fakeHostNodeHealthChecker{
		result: HostNodeHealth{
			Status:        "ok",
			LastHeartbeat: nodeHeartbeat,
			AgentVersion:  "v0.1.0",
		},
	}
	service := NewHostServiceWithOptions(nil, hostRepo, NewSnapshotBuilder(hostRepo), nil, WithHostServiceNodeHealthChecker(checker))

	items, err := service.ListHosts(context.Background())
	if err != nil {
		t.Fatalf("ListHosts() error = %v", err)
	}
	if checker.calledWith != "remote-a" {
		t.Fatalf("health checker called with %q, want remote-a", checker.calledWith)
	}
	var got *HostSummary
	for i := range items {
		if items[i].ID == "remote-a" {
			got = &items[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("ListHosts() = %+v, want remote-a", items)
	}
	if got.Status != "online" || got.AgentStatus != "online" || got.RuntimeReachability != "agent_online" {
		t.Fatalf("summary = %+v, want aiops_pull node health to keep host online", got)
	}
	if got.LastHeartbeat != nodeHeartbeat {
		t.Fatalf("LastHeartbeat = %q, want node health heartbeat %q", got.LastHeartbeat, nodeHeartbeat)
	}
	stored, err := hostRepo.GetHost("remote-a")
	if err != nil {
		t.Fatalf("GetHost() error = %v", err)
	}
	if stored.LastHeartbeat != nodeHeartbeat || stored.AgentStatus != "online" || stored.RuntimeReachability != "agent_online" {
		t.Fatalf("stored host = %+v, want refreshed online node health persisted", stored)
	}
}

func TestHostServiceCreateHostStoresSSHPasswordAsInternalCredentialRef(t *testing.T) {
	hostRepo := newHostRepoStub()
	passwordStore := &fakeHostSSHPasswordStore{ref: "secret://hosts/prod-web-01/ssh-password"}
	service := NewHostServiceWithOptions(nil, hostRepo, NewSnapshotBuilder(hostRepo), nil, WithHostServiceSSHPasswordStore(passwordStore))

	created, err := service.CreateHost(context.Background(), HostUpsert{
		ID:            "prod-web-01",
		Name:          "prod-web-01",
		Address:       "10.0.0.11",
		SSHUser:       "ubuntu",
		SSHPort:       22,
		SSHPassword:   "password-from-form",
		AgentVersion:  "v0.1.0",
		InstallViaSSH: true,
	})
	if err != nil {
		t.Fatalf("CreateHost() error = %v", err)
	}
	if passwordStore.hostID != "prod-web-01" || passwordStore.password != "password-from-form" {
		t.Fatalf("password store called with hostID=%q password=%q", passwordStore.hostID, passwordStore.password)
	}
	if created.Host.SSHCredentialRef != "secret://hosts/prod-web-01/ssh-password" {
		t.Fatalf("SSHCredentialRef = %q", created.Host.SSHCredentialRef)
	}
	stored, err := hostRepo.GetHost("prod-web-01")
	if err != nil {
		t.Fatalf("GetHost() error = %v", err)
	}
	if stored.SSHCredentialRef != "secret://hosts/prod-web-01/ssh-password" {
		t.Fatalf("stored SSHCredentialRef = %q", stored.SSHCredentialRef)
	}
}

func TestHostServiceCreateHostGeneratesInternalIDAndAllowsBlankName(t *testing.T) {
	hostRepo := newHostRepoStub()
	service := NewHostService(nil, hostRepo, NewSnapshotBuilder(hostRepo))

	created, err := service.CreateHost(context.Background(), HostUpsert{
		Address:       "172.18.13.13",
		SSHUser:       "kduser",
		SSHPort:       22,
		AgentVersion:  "v0.1.0",
		InstallViaSSH: true,
	})
	if err != nil {
		t.Fatalf("CreateHost() error = %v", err)
	}
	if created.Host.ID == "" || created.Host.ID == serverLocalHostID {
		t.Fatalf("generated host ID = %q", created.Host.ID)
	}
	stored, err := hostRepo.GetHost(created.Host.ID)
	if err != nil {
		t.Fatalf("GetHost() error = %v", err)
	}
	if stored.Name != "" {
		t.Fatalf("stored Name = %q, want blank", stored.Name)
	}
	if stored.Address != "172.18.13.13" {
		t.Fatalf("stored Address = %q", stored.Address)
	}
}

func TestHostServiceCreateHostRejectsExistingExplicitID(t *testing.T) {
	hostRepo := newHostRepoStub(store.HostRecord{
		ID:      "host-a",
		Name:    "web-01",
		Address: "10.0.0.11",
	})
	service := NewHostService(nil, hostRepo, NewSnapshotBuilder(hostRepo))

	_, err := service.CreateHost(context.Background(), HostUpsert{
		ID:      "host-a",
		Name:    "web-02",
		Address: "10.0.0.12",
		SSHUser: "ubuntu",
		SSHPort: 22,
	})
	if err == nil {
		t.Fatal("CreateHost() error = nil, want duplicate ID rejected")
	}
	stored, getErr := hostRepo.GetHost("host-a")
	if getErr != nil {
		t.Fatalf("GetHost() error = %v", getErr)
	}
	if stored.Name != "web-01" || stored.Address != "10.0.0.11" {
		t.Fatalf("stored host = %+v, want original record preserved", stored)
	}
}

func TestHostServiceRejectsDuplicateNonBlankHostName(t *testing.T) {
	hostRepo := newHostRepoStub(store.HostRecord{
		ID:      "host-a",
		Name:    "kme-node",
		Address: "172.18.13.11",
	})
	service := NewHostService(nil, hostRepo, NewSnapshotBuilder(hostRepo))

	if _, err := service.CreateHost(context.Background(), HostUpsert{
		Name:    " KME-node ",
		Address: "172.18.13.13",
		SSHUser: "kduser",
		SSHPort: 22,
	}); err == nil {
		t.Fatal("CreateHost() error = nil, want duplicate name rejected")
	}

	if _, err := service.UpdateHost(context.Background(), "host-a", HostUpsert{
		Name:    "",
		Address: "172.18.13.11",
		SSHUser: "kduser",
		SSHPort: 22,
	}); err != nil {
		t.Fatalf("UpdateHost() clearing own name error = %v", err)
	}
}

func TestHostServiceUpdateHostKeepsExistingSSHCredentialRefWhenPasswordIsBlank(t *testing.T) {
	hostRepo := newHostRepoStub(store.HostRecord{
		ID:               "prod-web-01",
		Name:             "prod-web-01",
		Address:          "10.0.0.11",
		SSHUser:          "ubuntu",
		SSHPort:          22,
		SSHCredentialRef: "secret://hosts/prod-web-01/ssh-password",
		AgentVersion:     "v0.1.0",
	})
	service := NewHostServiceWithOptions(nil, hostRepo, NewSnapshotBuilder(hostRepo), nil, WithHostServiceSSHPasswordStore(&fakeHostSSHPasswordStore{}))

	updated, err := service.UpdateHost(context.Background(), "prod-web-01", HostUpsert{
		ID:           "prod-web-01",
		Name:         "prod-web-01-renamed",
		Address:      "10.0.0.12",
		SSHUser:      "ubuntu",
		SSHPort:      22,
		AgentVersion: "v0.1.0",
	})
	if err != nil {
		t.Fatalf("UpdateHost() error = %v", err)
	}
	if updated.Host.SSHCredentialRef != "secret://hosts/prod-web-01/ssh-password" {
		t.Fatalf("SSHCredentialRef = %q", updated.Host.SSHCredentialRef)
	}
}

func TestHostServiceStoresAgentServerURLSeparatelyFromAgentURL(t *testing.T) {
	hostRepo := newHostRepoStub()
	service := NewHostService(nil, hostRepo, NewSnapshotBuilder(hostRepo))

	created, err := service.CreateHost(context.Background(), HostUpsert{
		ID:             "prod-web-01",
		Name:           "prod-web-01",
		Address:        "10.0.0.11",
		SSHUser:        "ubuntu",
		SSHPort:        22,
		AgentVersion:   "v0.1.0",
		AgentServerURL: "http://aiops.example.test:18080",
	})
	if err != nil {
		t.Fatalf("CreateHost() error = %v", err)
	}
	if created.Host.AgentServerURL != "http://aiops.example.test:18080" {
		t.Fatalf("AgentServerURL = %q", created.Host.AgentServerURL)
	}

	current, err := hostRepo.GetHost("prod-web-01")
	if err != nil {
		t.Fatalf("GetHost() error = %v", err)
	}
	current.AgentURL = "http://10.0.0.11:7072"
	if err := hostRepo.SaveHost(current); err != nil {
		t.Fatalf("SaveHost() error = %v", err)
	}

	preserved, err := service.UpdateHost(context.Background(), "prod-web-01", HostUpsert{
		ID:             "prod-web-01",
		Name:           "prod-web-01",
		Address:        "10.0.0.12",
		SSHUser:        "ubuntu",
		SSHPort:        22,
		AgentVersion:   "v0.1.0",
		AgentServerURL: "http://aiops.internal:18080",
	})
	if err != nil {
		t.Fatalf("UpdateHost() preserving AgentURL error = %v", err)
	}
	if preserved.Host.AgentURL != "http://10.0.0.11:7072" {
		t.Fatalf("preserved AgentURL = %q, want existing host-agent endpoint", preserved.Host.AgentURL)
	}

	updated, err := service.UpdateHost(context.Background(), "prod-web-01", HostUpsert{
		ID:             "prod-web-01",
		Name:           "prod-web-01",
		Address:        "10.0.0.12",
		SSHUser:        "ubuntu",
		SSHPort:        22,
		AgentVersion:   "v0.1.0",
		AgentURL:       "http://10.0.0.12:7072",
		AgentServerURL: "http://aiops.internal:18080",
	})
	if err != nil {
		t.Fatalf("UpdateHost() error = %v", err)
	}
	if updated.Host.AgentURL != "http://10.0.0.12:7072" {
		t.Fatalf("AgentURL = %q, want updated host-agent endpoint", updated.Host.AgentURL)
	}
	if updated.Host.AgentServerURL != "http://aiops.internal:18080" {
		t.Fatalf("AgentServerURL = %q", updated.Host.AgentServerURL)
	}
}

func TestHostServiceCreateHostOnlyStoresInventoryConfig(t *testing.T) {
	hostRepo := newHostRepoStub()
	service := NewHostService(nil, hostRepo, NewSnapshotBuilder(hostRepo))

	created, err := service.CreateHost(context.Background(), HostUpsert{
		ID:               "prod-web-01",
		Name:             "prod-web-01",
		Address:          "10.0.0.11",
		SSHUser:          "ubuntu",
		SSHPort:          22,
		SSHCredentialRef: "secret://ops/prod-web-01-ssh-key",
		AgentVersion:     "v0.1.0",
		InstallViaSSH:    true,
	})
	if err != nil {
		t.Fatalf("CreateHost() error = %v", err)
	}
	if created.Host.SSHCredentialRef != "secret://ops/prod-web-01-ssh-key" {
		t.Fatalf("SSHCredentialRef = %q", created.Host.SSHCredentialRef)
	}
	if created.Host.AgentVersion != "v0.1.0" {
		t.Fatalf("AgentVersion = %q", created.Host.AgentVersion)
	}
	if created.Host.Transport != "manual" || created.Host.Status != "offline" || created.Host.InstallState != "inventory" {
		t.Fatalf("created host state = %+v, want saved inventory config only", created.Host)
	}
	if created.Host.ConnectionMode != HostConnectionModeAIOPSPull {
		t.Fatalf("ConnectionMode = %q, want aiops_pull default", created.Host.ConnectionMode)
	}
}

func TestHostServiceCreateHostDoesNotStartSSHInstall(t *testing.T) {
	hostRepo := newHostRepoStub()
	installer := &fakeHostAgentInstaller{run: HostInstallRun{RunID: "direct-1", Status: "success"}}
	bootstrap := NewHostBootstrapService(hostRepo, nil, WithHostAgentInstaller(installer))
	service := NewHostService(nil, hostRepo, NewSnapshotBuilder(hostRepo), bootstrap)

	created, err := service.CreateHost(context.Background(), HostUpsert{
		ID:            "failed-host",
		Name:          "saved-host",
		Address:       "10.0.0.12",
		SSHUser:       "kduser",
		SSHPort:       22,
		AgentVersion:  "v0.1.0",
		InstallViaSSH: true,
	})
	if err != nil {
		t.Fatalf("CreateHost() error = %v", err)
	}
	if installer.called {
		t.Fatal("installer was called during CreateHost, want save-only behavior")
	}
	if created.Host.Status != "offline" || created.Host.AgentStatus != "offline" || created.Host.SSHStatus != "unknown" || created.Host.RuntimeReachability != "ssh_unverified" || created.Host.InstallState != "inventory" {
		t.Fatalf("created host = %+v, want inventory state", created.Host)
	}
}

func TestHostServiceInstallHostAllowsMissingSSHCredentialRef(t *testing.T) {
	hostRepo := newHostRepoStub(store.HostRecord{
		ID:           "prod-web-01",
		Name:         "prod-web-01",
		Address:      "10.0.0.11",
		SSHUser:      "ubuntu",
		SSHPort:      22,
		AgentVersion: "v0.1.0",
		Status:       "offline",
		InstallState: "failed",
	})
	service := NewHostService(nil, hostRepo, NewSnapshotBuilder(hostRepo))

	resp, err := service.InstallHost(context.Background(), "prod-web-01", HostInstallRequest{
		AgentVersion: "v0.1.0",
	})
	if err != nil {
		t.Fatalf("InstallHost() error = %v", err)
	}
	if resp.Host.SSHCredentialRef != "" {
		t.Fatalf("SSHCredentialRef = %q, want empty", resp.Host.SSHCredentialRef)
	}
	if resp.Host.Transport != "ssh_bootstrap" || resp.Host.Status != "installing" || resp.Host.InstallState != "pending_install" {
		t.Fatalf("install response host = %+v", resp.Host)
	}
}

func TestHostServiceInstallHostDefaultsToAIOPSPullAndDoesNotPassSavedAgentServerURLToInstaller(t *testing.T) {
	hostRepo := newHostRepoStub(store.HostRecord{
		ID:               "prod-web-01",
		Name:             "prod-web-01",
		Address:          "10.0.0.11",
		SSHUser:          "ubuntu",
		SSHPort:          22,
		SSHCredentialRef: "secret://hosts/prod-web-01/ssh-password",
		AgentVersion:     "v0.1.0",
		AgentServerURL:   "http://aiops.example.test:18080",
		Status:           "offline",
		InstallState:     "failed",
	})
	installer := &fakeHostSSHInstaller{}
	bootstrap := NewHostBootstrapService(hostRepo, nil, WithHostAgentInstaller(installer))
	service := NewHostServiceWithOptions(nil, hostRepo, NewSnapshotBuilder(hostRepo), bootstrap)

	if _, err := service.InstallHost(context.Background(), "prod-web-01", HostInstallRequest{AgentVersion: "v0.1.0"}); err != nil {
		t.Fatalf("InstallHost() error = %v", err)
	}
	if installer.installReq.ConnectionMode != HostConnectionModeAIOPSPull {
		t.Fatalf("installer ConnectionMode = %q, want aiops_pull", installer.installReq.ConnectionMode)
	}
	if installer.installReq.AgentServerURL != "" {
		t.Fatalf("installer AgentServerURL = %q, want no callback URL for aiops_pull", installer.installReq.AgentServerURL)
	}
}

func TestHostServiceInstallHostPassesSavedAgentServerURLToNodePushGRPCInstaller(t *testing.T) {
	hostRepo := newHostRepoStub(store.HostRecord{
		ID:               "prod-web-01",
		Name:             "prod-web-01",
		Address:          "10.0.0.11",
		SSHUser:          "ubuntu",
		SSHPort:          22,
		SSHCredentialRef: "secret://hosts/prod-web-01/ssh-password",
		AgentVersion:     "v0.1.0",
		AgentServerURL:   "http://aiops.example.test:18080",
		ConnectionMode:   HostConnectionModeNodePushGRPC,
		Status:           "offline",
		InstallState:     "failed",
	})
	installer := &fakeHostSSHInstaller{}
	bootstrap := NewHostBootstrapService(hostRepo, nil, WithHostAgentInstaller(installer))
	service := NewHostServiceWithOptions(nil, hostRepo, NewSnapshotBuilder(hostRepo), bootstrap)

	if _, err := service.InstallHost(context.Background(), "prod-web-01", HostInstallRequest{AgentVersion: "v0.1.0"}); err != nil {
		t.Fatalf("InstallHost() error = %v", err)
	}
	if installer.installReq.ConnectionMode != HostConnectionModeNodePushGRPC {
		t.Fatalf("installer ConnectionMode = %q, want node_push_grpc", installer.installReq.ConnectionMode)
	}
	if installer.installReq.AgentServerURL != "http://aiops.example.test:18080" {
		t.Fatalf("installer AgentServerURL = %q, want saved callback URL", installer.installReq.AgentServerURL)
	}
}

func TestHostServiceSSHTestAllowsMissingSSHCredentialRefWithoutBootstrap(t *testing.T) {
	hostRepo := newHostRepoStub(store.HostRecord{
		ID:      "prod-web-01",
		Name:    "prod-web-01",
		Address: "10.0.0.11",
		SSHUser: "ubuntu",
		SSHPort: 22,
	})
	service := NewHostService(nil, hostRepo, NewSnapshotBuilder(hostRepo))

	resp, err := service.TestHostSSH(context.Background(), "prod-web-01", HostSSHTestRequest{})
	if err != nil {
		t.Fatalf("TestHostSSH() error = %v", err)
	}
	if resp.Status != "ok" {
		t.Fatalf("response = %+v, want ok", resp)
	}
}

func TestHostServiceSSHTestStoresPasswordBeforeBootstrap(t *testing.T) {
	hostRepo := newHostRepoStub(store.HostRecord{
		ID:      "prod-web-01",
		Name:    "prod-web-01",
		Address: "10.0.0.11",
		SSHUser: "ubuntu",
		SSHPort: 22,
	})
	passwords := &fakeHostSSHPasswordStore{ref: "secret://hosts/prod-web-01/ssh-password"}
	installer := &fakeHostSSHInstaller{}
	bootstrap := NewHostBootstrapService(hostRepo, nil, WithHostAgentInstaller(installer))
	service := NewHostServiceWithOptions(nil, hostRepo, NewSnapshotBuilder(hostRepo), bootstrap, WithHostServiceSSHPasswordStore(passwords))

	resp, err := service.TestHostSSH(context.Background(), "prod-web-01", HostSSHTestRequest{SSHPassword: "password-from-form"})
	if err != nil {
		t.Fatalf("TestHostSSH() error = %v", err)
	}
	if resp.Status != "ok" {
		t.Fatalf("response = %+v, want ok", resp)
	}
	if passwords.hostID != "prod-web-01" || passwords.password != "password-from-form" {
		t.Fatalf("stored password host/password = %q/%q", passwords.hostID, passwords.password)
	}
	if installer.credentialRef != "secret://hosts/prod-web-01/ssh-password" {
		t.Fatalf("bootstrap credentialRef = %q, want stored secret ref", installer.credentialRef)
	}
	stored, err := hostRepo.GetHost("prod-web-01")
	if err != nil {
		t.Fatalf("GetHost() error = %v", err)
	}
	if stored.SSHStatus != "ok" || stored.RuntimeReachability != "ssh_available" || stored.SSHCredentialRef != "secret://hosts/prod-web-01/ssh-password" {
		t.Fatalf("stored host ssh status = %+v, want ok ssh_available and credential ref persisted", stored)
	}
}
