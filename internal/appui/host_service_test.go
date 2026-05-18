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
	if created.Host.ID != "host-b" || created.Host.Transport != "ssh_bootstrap" {
		t.Fatalf("created = %+v, want created host-b with ssh_bootstrap", created)
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

func TestHostServiceCreateHostStoresSSHInstallContract(t *testing.T) {
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
	if created.Host.Transport != "ssh_bootstrap" || created.Host.Status != "installing" || created.Host.InstallState != "pending_install" {
		t.Fatalf("created host state = %+v", created.Host)
	}
}
