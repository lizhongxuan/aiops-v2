package hostops

import (
	"context"
	"fmt"
	"strings"
)

// HostRecordView is the minimal inventory shape needed to resolve @host
// mentions without coupling hostops to the persistence layer.
type HostRecordView struct {
	ID          string
	Address     string
	Hostname    string
	DisplayName string
	Managed     bool
	Executable  bool
	AgentURL    string
}

type HostLookup interface {
	ListHosts(ctx context.Context) ([]HostRecordView, error)
}

type MentionResolutionError struct {
	Raw    string
	Reason string
}

func (e MentionResolutionError) Error() string {
	if e.Reason == "" {
		return fmt.Sprintf("host mention %q could not be resolved", e.Raw)
	}
	return fmt.Sprintf("host mention %q could not be resolved: %s", e.Raw, e.Reason)
}

type Resolver struct {
	lookup HostLookup
}

func NewResolver(lookup HostLookup) *Resolver {
	return &Resolver{lookup: lookup}
}

func (r *Resolver) Resolve(ctx context.Context, mentions []HostMention) ([]HostMention, []MentionResolutionError) {
	resolved := append([]HostMention(nil), mentions...)
	if len(resolved) == 0 {
		return resolved, nil
	}
	if r == nil || r.lookup == nil {
		return resolved, unresolvedErrors(resolved, "host inventory is unavailable")
	}
	hosts, err := r.lookup.ListHosts(ctx)
	if err != nil {
		return resolved, unresolvedErrors(resolved, err.Error())
	}
	index := buildHostIndex(hosts)
	errs := make([]MentionResolutionError, 0)
	for i := range resolved {
		keyCandidates := mentionResolutionKeys(resolved[i])
		host, ok := firstHostMatch(index, keyCandidates)
		if !ok {
			errs = append(errs, MentionResolutionError{Raw: resolved[i].Raw, Reason: "no matching inventory host"})
			continue
		}
		resolved[i].HostID = host.ID
		resolved[i].Address = host.Address
		resolved[i].DisplayName = firstNonEmpty(host.DisplayName, host.Hostname, host.Address, host.ID)
		resolved[i].Source = HostMentionSourceInventory
		resolved[i].Resolved = true
		resolved[i].Confidence = 1
	}
	return resolved, errs
}

func buildHostIndex(hosts []HostRecordView) map[string]HostRecordView {
	index := make(map[string]HostRecordView, len(hosts)*4)
	for _, host := range hosts {
		addHostIndex(index, "id", host.ID, host)
		addHostIndex(index, "addr", host.Address, host)
		addHostIndex(index, "name", host.Hostname, host)
		addHostIndex(index, "name", host.DisplayName, host)
	}
	return index
}

func addHostIndex(index map[string]HostRecordView, prefix, value string, host HostRecordView) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return
	}
	index[prefix+":"+value] = host
	if trimmed := strings.TrimPrefix(value, "@"); trimmed != "" && trimmed != value {
		index[prefix+":"+trimmed] = host
	}
}

func mentionResolutionKeys(mention HostMention) []string {
	keys := make([]string, 0, 4)
	if mention.HostID != "" {
		keys = append(keys, "id:"+strings.ToLower(strings.TrimSpace(mention.HostID)))
	}
	value := strings.TrimPrefix(strings.TrimSpace(mention.Raw), "@")
	if mention.Address != "" {
		value = mention.Address
		keys = append(keys, "addr:"+strings.ToLower(strings.TrimSpace(mention.Address)))
	}
	if mention.DisplayName != "" {
		keys = append(keys, "name:"+strings.ToLower(strings.TrimSpace(mention.DisplayName)))
	}
	if value != "" {
		normalized := strings.ToLower(strings.TrimSpace(value))
		keys = append(keys, "id:"+normalized, "addr:"+normalized, "name:"+normalized)
	}
	return keys
}

func firstHostMatch(index map[string]HostRecordView, keys []string) (HostRecordView, bool) {
	for _, key := range keys {
		if host, ok := index[key]; ok {
			return host, true
		}
	}
	return HostRecordView{}, false
}

func unresolvedErrors(mentions []HostMention, reason string) []MentionResolutionError {
	errs := make([]MentionResolutionError, 0, len(mentions))
	for _, mention := range mentions {
		errs = append(errs, MentionResolutionError{Raw: mention.Raw, Reason: reason})
	}
	return errs
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
