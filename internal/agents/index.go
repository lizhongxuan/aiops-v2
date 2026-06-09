package agents

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

// AgentIndexOptions controls short catalog listing selection and ordering.
type AgentIndexOptions struct {
	Query            string
	Mode             string
	ResourceType     string
	OperationKind    string
	MaxChars         int
	MaxContextTokens int
}

// AgentIndexResult is the prompt-safe short catalog listing.
type AgentIndexResult struct {
	Entries []AgentIndexEntry
	Dropped []DroppedAgentIndexEntry
	Bytes   int
	Hash    string
}

// AgentIndexEntry is a compact, prompt-safe view of an agent definition.
type AgentIndexEntry struct {
	Kind            string   `json:"kind,omitempty"`
	Name            string   `json:"name,omitempty"`
	Description     string   `json:"description,omitempty"`
	WhenToUse       string   `json:"whenToUse,omitempty"`
	CapabilityKinds []string `json:"capabilityKinds,omitempty"`
	ResourceTypes   []string `json:"resourceTypes,omitempty"`
	OperationKinds  []string `json:"operationKinds,omitempty"`
	Modes           []string `json:"modes,omitempty"`
	ModelInvocable  bool     `json:"modelInvocable,omitempty"`
	MaxConcurrent   int      `json:"maxConcurrent,omitempty"`
	CostClass       string   `json:"costClass,omitempty"`
}

// DroppedAgentIndexEntry records catalog entries omitted from the listing.
type DroppedAgentIndexEntry struct {
	Kind   string `json:"kind,omitempty"`
	Name   string `json:"name,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type scoredAgentIndexEntry struct {
	entry AgentIndexEntry
	score int
	order int
}

// BuildAgentIndex returns a short, prompt-safe catalog listing. It never includes full prompts.
func BuildAgentIndex(defs []Definition, opts AgentIndexOptions) AgentIndexResult {
	scored := make([]scoredAgentIndexEntry, 0, len(defs))
	for i, def := range defs {
		discovery := normalizeDiscoveryMetadata(def.Discovery)
		budget := normalizeBudgetMetadata(def.Budget)
		entry := AgentIndexEntry{
			Kind:            strings.TrimSpace(def.Kind),
			Name:            strings.TrimSpace(def.Name),
			Description:     strings.TrimSpace(def.Description),
			WhenToUse:       discovery.WhenToUse,
			CapabilityKinds: append([]string(nil), discovery.CapabilityKinds...),
			ResourceTypes:   append([]string(nil), discovery.ResourceTypes...),
			OperationKinds:  append([]string(nil), discovery.OperationKinds...),
			Modes:           append([]string(nil), discovery.Modes...),
			ModelInvocable:  discovery.ModelInvocable,
			MaxConcurrent:   budget.MaxConcurrent,
			CostClass:       budget.CostClass,
		}
		scored = append(scored, scoredAgentIndexEntry{
			entry: entry,
			score: scoreAgentIndexEntry(entry, opts),
			order: i,
		})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		if scored[i].entry.Kind != scored[j].entry.Kind {
			return scored[i].entry.Kind < scored[j].entry.Kind
		}
		if scored[i].entry.Name != scored[j].entry.Name {
			return scored[i].entry.Name < scored[j].entry.Name
		}
		return scored[i].order < scored[j].order
	})

	entries := make([]AgentIndexEntry, 0, len(scored))
	for _, item := range scored {
		entries = append(entries, item.entry)
	}

	limit := indexCharLimit(opts)
	dropped := make([]DroppedAgentIndexEntry, 0)
	if limit > 0 {
		for len(entries) > 0 {
			encoded := marshalAgentIndexEntries(entries)
			if len(encoded) <= limit || len(entries) == 1 {
				break
			}
			last := entries[len(entries)-1]
			entries = entries[:len(entries)-1]
			dropped = append(dropped, DroppedAgentIndexEntry{
				Kind:   last.Kind,
				Name:   last.Name,
				Reason: "budget_exceeded",
			})
		}
	}

	encoded := marshalAgentIndexEntries(entries)
	sum := sha256.Sum256(encoded)
	return AgentIndexResult{
		Entries: entries,
		Dropped: dropped,
		Bytes:   len(encoded),
		Hash:    hex.EncodeToString(sum[:]),
	}
}

func scoreAgentIndexEntry(entry AgentIndexEntry, opts AgentIndexOptions) int {
	score := 0
	query := strings.ToLower(strings.TrimSpace(opts.Query))
	if query != "" {
		haystack := strings.ToLower(strings.Join([]string{
			entry.Kind,
			entry.Name,
			entry.Description,
			entry.WhenToUse,
			strings.Join(entry.CapabilityKinds, " "),
			strings.Join(entry.ResourceTypes, " "),
			strings.Join(entry.OperationKinds, " "),
			strings.Join(entry.Modes, " "),
		}, " "))
		for _, token := range strings.Fields(query) {
			if strings.Contains(haystack, token) {
				score += 10
			}
		}
	}
	if containsFold(entry.ResourceTypes, opts.ResourceType) {
		score += 100
	}
	if containsFold(entry.OperationKinds, opts.OperationKind) {
		score += 80
	}
	if containsFold(entry.Modes, opts.Mode) {
		score += 60
	}
	if strings.EqualFold(entry.CostClass, "low") {
		score += 3
	} else if strings.EqualFold(entry.CostClass, "medium") {
		score += 2
	} else if strings.EqualFold(entry.CostClass, "high") {
		score++
	}
	return score
}

func containsFold(values []string, needle string) bool {
	needle = strings.TrimSpace(needle)
	if needle == "" {
		return false
	}
	for _, value := range values {
		if strings.EqualFold(value, needle) {
			return true
		}
	}
	return false
}

func indexCharLimit(opts AgentIndexOptions) int {
	limit := opts.MaxChars
	if opts.MaxContextTokens > 0 {
		tokenChars := opts.MaxContextTokens * 4
		if limit == 0 || tokenChars < limit {
			limit = tokenChars
		}
	}
	return limit
}

func marshalAgentIndexEntries(entries []AgentIndexEntry) []byte {
	encoded, err := json.Marshal(entries)
	if err != nil {
		return []byte("[]")
	}
	return encoded
}
