package mcp

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type HealthStatus string

const (
	HealthUnknown     HealthStatus = "unknown"
	HealthHealthy     HealthStatus = "healthy"
	HealthDegraded    HealthStatus = "degraded"
	HealthUnavailable HealthStatus = "unavailable"
	HealthDisabled    HealthStatus = "disabled"
)

const DefaultHealthTTL = 30 * time.Second

type HealthSnapshot struct {
	ServerID      string       `json:"serverId"`
	Status        HealthStatus `json:"status"`
	LastCheckedAt time.Time    `json:"lastCheckedAt,omitempty"`
	LastSuccessAt time.Time    `json:"lastSuccessAt,omitempty"`
	LastError     string       `json:"lastError,omitempty"`
	TTLSeconds    int          `json:"ttlSeconds,omitempty"`
	Capabilities  []string     `json:"capabilities,omitempty"`
}

type HealthProbeResult struct {
	Status       HealthStatus
	Capabilities []string
	LastError    string
	Err          error
}

type HealthProbe func(context.Context, ServerConfig) HealthProbeResult

type HealthRegistry struct {
	mu        sync.RWMutex
	ttl       time.Duration
	now       func() time.Time
	snapshots map[string]HealthSnapshot
}

func NewHealthRegistry(ttl time.Duration) *HealthRegistry {
	return NewHealthRegistryWithClock(ttl, time.Now)
}

func NewHealthRegistryWithClock(ttl time.Duration, now func() time.Time) *HealthRegistry {
	if ttl <= 0 {
		ttl = DefaultHealthTTL
	}
	if now == nil {
		now = time.Now
	}
	return &HealthRegistry{
		ttl:       ttl,
		now:       now,
		snapshots: make(map[string]HealthSnapshot),
	}
}

func (r *HealthRegistry) Snapshot(serverID string) (HealthSnapshot, bool) {
	if r == nil {
		return HealthSnapshot{}, false
	}
	serverID = strings.TrimSpace(serverID)
	if serverID == "" {
		return HealthSnapshot{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	snapshot, ok := r.snapshots[serverID]
	if !ok {
		return HealthSnapshot{}, false
	}
	return cloneHealthSnapshot(snapshot), true
}

func (r *HealthRegistry) List() []HealthSnapshot {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]HealthSnapshot, 0, len(r.snapshots))
	for _, snapshot := range r.snapshots {
		out = append(out, cloneHealthSnapshot(snapshot))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ServerID < out[j].ServerID })
	return out
}

func (r *HealthRegistry) Set(snapshot HealthSnapshot) {
	if r == nil {
		return
	}
	snapshot = normalizeHealthSnapshot(snapshot, r.ttl)
	if snapshot.ServerID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.snapshots[snapshot.ServerID] = snapshot
}

func (r *HealthRegistry) Refresh(ctx context.Context, cfg ServerConfig, disabled bool, force bool, probe HealthProbe) HealthSnapshot {
	if r == nil {
		return HealthSnapshot{}
	}
	serverID := strings.TrimSpace(cfg.ID)
	if serverID == "" {
		serverID = strings.TrimSpace(cfg.Name)
	}
	if serverID == "" {
		return HealthSnapshot{}
	}
	now := r.now()
	if disabled {
		snapshot := HealthSnapshot{
			ServerID:      serverID,
			Status:        HealthDisabled,
			LastCheckedAt: now,
			TTLSeconds:    int(r.ttl.Seconds()),
		}
		r.Set(snapshot)
		return snapshot
	}
	if !force {
		if cached, ok := r.Snapshot(serverID); ok && healthSnapshotFresh(cached, now, r.ttl) {
			return cached
		}
	}

	var previous HealthSnapshot
	if snapshot, ok := r.Snapshot(serverID); ok {
		previous = snapshot
	}
	snapshot := HealthSnapshot{
		ServerID:      serverID,
		Status:        HealthUnknown,
		LastCheckedAt: now,
		LastSuccessAt: previous.LastSuccessAt,
		TTLSeconds:    int(r.ttl.Seconds()),
	}
	if probe != nil {
		result := probe(ctx, cfg)
		snapshot.Status = normalizeHealthStatus(result.Status)
		snapshot.Capabilities = cloneSortedCompactStrings(result.Capabilities)
		if result.Err != nil {
			snapshot.Status = classifyHealthError(result.Err.Error())
			snapshot.LastError = RedactHealthError(result.Err.Error())
		} else if strings.TrimSpace(result.LastError) != "" {
			snapshot.LastError = RedactHealthError(result.LastError)
			if snapshot.Status == HealthUnknown {
				snapshot.Status = HealthDegraded
			}
		}
	}
	if snapshot.Status == "" {
		snapshot.Status = HealthUnknown
	}
	if snapshot.Status == HealthHealthy || snapshot.Status == HealthDegraded {
		snapshot.LastSuccessAt = now
	}
	r.Set(snapshot)
	return snapshot
}

func healthSnapshotFresh(snapshot HealthSnapshot, now time.Time, ttl time.Duration) bool {
	if snapshot.LastCheckedAt.IsZero() || ttl <= 0 {
		return false
	}
	return now.Sub(snapshot.LastCheckedAt) < ttl
}

func normalizeHealthSnapshot(snapshot HealthSnapshot, ttl time.Duration) HealthSnapshot {
	snapshot.ServerID = strings.TrimSpace(snapshot.ServerID)
	snapshot.Status = normalizeHealthStatus(snapshot.Status)
	if snapshot.Status == "" {
		snapshot.Status = HealthUnknown
	}
	snapshot.LastError = RedactHealthError(snapshot.LastError)
	snapshot.Capabilities = cloneSortedCompactStrings(snapshot.Capabilities)
	if snapshot.TTLSeconds <= 0 && ttl > 0 {
		snapshot.TTLSeconds = int(ttl.Seconds())
	}
	return snapshot
}

func cloneHealthSnapshot(snapshot HealthSnapshot) HealthSnapshot {
	snapshot.LastError = RedactHealthError(snapshot.LastError)
	snapshot.Capabilities = append([]string(nil), snapshot.Capabilities...)
	return snapshot
}

func normalizeHealthStatus(status HealthStatus) HealthStatus {
	switch HealthStatus(strings.ToLower(strings.TrimSpace(string(status)))) {
	case HealthHealthy:
		return HealthHealthy
	case HealthDegraded:
		return HealthDegraded
	case HealthUnavailable:
		return HealthUnavailable
	case HealthDisabled:
		return HealthDisabled
	case HealthUnknown:
		return HealthUnknown
	default:
		return HealthUnknown
	}
}

func classifyHealthError(message string) HealthStatus {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "502"),
		strings.Contains(lower, "bad gateway"),
		strings.Contains(lower, "connection refused"),
		strings.Contains(lower, "connect: refused"),
		strings.Contains(lower, "no such host"),
		strings.Contains(lower, "i/o timeout"):
		return HealthUnavailable
	default:
		return HealthDegraded
	}
}

func RedactHealthError(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return ""
	}
	replacers := []struct {
		pattern string
		repl    string
	}{
		{`(?i)\b(password|token|secret|api[_-]?key|authorization)=\S+`, `$1=[REDACTED]`},
		{`(?i)\bbearer\s+\S+`, `Bearer [REDACTED]`},
		{`\bsk-[A-Za-z0-9._-]+`, `[REDACTED]`},
	}
	for _, replacer := range replacers {
		message = regexp.MustCompile(replacer.pattern).ReplaceAllString(message, replacer.repl)
	}
	return message
}

func cloneSortedCompactStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	sort.Strings(out)
	return out
}
