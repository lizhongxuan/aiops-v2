package mcp

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestHealthRegistryCachesWithinTTL(t *testing.T) {
	now := time.Unix(100, 0)
	registry := NewHealthRegistryWithClock(30*time.Second, func() time.Time { return now })
	cfg := ServerConfig{ID: "synthetic_obs", Name: "Synthetic Observability"}
	probes := 0
	probe := func(context.Context, ServerConfig) HealthProbeResult {
		probes++
		return HealthProbeResult{Status: HealthHealthy, Capabilities: []string{"tools"}}
	}

	first := registry.Refresh(context.Background(), cfg, false, false, probe)
	second := registry.Refresh(context.Background(), cfg, false, false, probe)

	if probes != 1 {
		t.Fatalf("probe count = %d, want 1 while TTL is valid", probes)
	}
	if first.LastCheckedAt != second.LastCheckedAt {
		t.Fatalf("cached LastCheckedAt changed: %v -> %v", first.LastCheckedAt, second.LastCheckedAt)
	}

	now = now.Add(31 * time.Second)
	third := registry.Refresh(context.Background(), cfg, false, false, probe)
	if probes != 2 {
		t.Fatalf("probe count after TTL expiry = %d, want 2", probes)
	}
	if !third.LastCheckedAt.After(first.LastCheckedAt) {
		t.Fatalf("LastCheckedAt after TTL = %v, want after %v", third.LastCheckedAt, first.LastCheckedAt)
	}
}

func TestHealthRegistryClassifiesConnectionErrorsAndRedactsSecrets(t *testing.T) {
	now := time.Unix(200, 0)
	registry := NewHealthRegistryWithClock(time.Minute, func() time.Time { return now })

	snapshot := registry.Refresh(context.Background(), ServerConfig{ID: "synthetic_obs"}, false, true, func(context.Context, ServerConfig) HealthProbeResult {
		return HealthProbeResult{Err: errors.New("502 bad gateway token=secret password=hidden sk-testkey")}
	})

	if snapshot.Status != HealthUnavailable {
		t.Fatalf("Status = %q, want %q", snapshot.Status, HealthUnavailable)
	}
	for _, forbidden := range []string{"secret", "hidden", "sk-testkey"} {
		if strings.Contains(snapshot.LastError, forbidden) {
			t.Fatalf("LastError leaked %q: %q", forbidden, snapshot.LastError)
		}
	}
	if !strings.Contains(snapshot.LastError, "[REDACTED]") {
		t.Fatalf("LastError = %q, want redaction marker", snapshot.LastError)
	}
}

func TestRegistryHealthSnapshotForDisabledServer(t *testing.T) {
	registry := NewRegistry()
	if err := registry.RegisterServer(ServerConfig{ID: "synthetic_obs", Transport: "http", Command: []string{"http://synthetic.invalid"}}); err != nil {
		t.Fatalf("RegisterServer() error = %v", err)
	}
	registry.SetServerDisabled("synthetic_obs", true)

	probes := 0
	snapshot := registry.RefreshServerHealth(context.Background(), "synthetic_obs", true, func(context.Context, ServerConfig) HealthProbeResult {
		probes++
		return HealthProbeResult{Status: HealthHealthy}
	})

	if probes != 0 {
		t.Fatalf("disabled server probe count = %d, want 0", probes)
	}
	if snapshot.Status != HealthDisabled {
		t.Fatalf("Status = %q, want %q", snapshot.Status, HealthDisabled)
	}
}
