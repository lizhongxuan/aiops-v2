package hostops

import (
	"context"
	"testing"
)

func TestResolveMentionsMatchesInventoryAddress(t *testing.T) {
	resolver := NewResolver(staticHostLookup{
		{ID: "host-a", Address: "1.1.1.1", DisplayName: "host-a", Managed: true, Executable: true},
	})
	mentions := ParseHostMentions("@1.1.1.1 执行通用运维任务")
	resolved, errs := resolver.Resolve(context.Background(), mentions)
	if len(errs) != 0 {
		t.Fatalf("errs = %#v, want none", errs)
	}
	if !resolved[0].Resolved || resolved[0].HostID != "host-a" {
		t.Fatalf("resolved[0] = %#v, want host-a", resolved[0])
	}
}

func TestResolveMentionsMatchesInventoryHostname(t *testing.T) {
	resolver := NewResolver(staticHostLookup{
		{ID: "host-db-1", Hostname: "db-1", Address: "10.0.0.11", DisplayName: "database one"},
	})
	resolved, errs := resolver.Resolve(context.Background(), ParseHostMentions("@db-1 检查主机状态"))
	if len(errs) != 0 {
		t.Fatalf("errs = %#v, want none", errs)
	}
	if !resolved[0].Resolved || resolved[0].HostID != "host-db-1" || resolved[0].Address != "10.0.0.11" {
		t.Fatalf("resolved[0] = %#v, want host-db-1", resolved[0])
	}
}

func TestResolveMentionsLeavesUnknownIPUnresolved(t *testing.T) {
	resolver := NewResolver(staticHostLookup{})
	resolved, errs := resolver.Resolve(context.Background(), ParseHostMentions("@1.1.1.9 执行通用运维任务"))
	if len(errs) != 1 {
		t.Fatalf("len(errs) = %d, want 1", len(errs))
	}
	if resolved[0].Resolved {
		t.Fatalf("resolved[0].Resolved = true, want false")
	}
}

func TestResolverMapsLocalAliasToServerLocal(t *testing.T) {
	resolver := NewResolver(staticHostLookup{
		{ID: "server-local", Hostname: "localhost", Address: "127.0.0.1", DisplayName: "Local"},
	})
	resolved, errs := resolver.Resolve(context.Background(), ParseHostMentions("@local 检查主机"))
	if len(errs) != 0 {
		t.Fatalf("errs = %#v, want none", errs)
	}
	if len(resolved) != 1 || !resolved[0].Resolved || resolved[0].HostID != "server-local" {
		t.Fatalf("resolved = %#v, want server-local", resolved)
	}
}

type staticHostLookup []HostRecordView

func (lookup staticHostLookup) ListHosts(context.Context) ([]HostRecordView, error) {
	return append([]HostRecordView(nil), lookup...), nil
}
