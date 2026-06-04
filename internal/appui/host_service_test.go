package appui

import (
	"context"
	"fmt"
	"testing"

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
	if created.Host.Status != "offline" || created.Host.InstallState != "inventory" {
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
