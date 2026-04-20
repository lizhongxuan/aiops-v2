package policyengine

import (
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// CapabilityLayer classifies tools into three capability tiers.
// ---------------------------------------------------------------------------

// CapabilityLayer represents the three-layer capability classification.
type CapabilityLayer string

const (
	// LayerStructuredRead — 14 standardized read-only interfaces, no approval needed.
	LayerStructuredRead CapabilityLayer = "structured_read"
	// LayerControlledMutation — 5 controlled mutation interfaces, mandatory approval.
	LayerControlledMutation CapabilityLayer = "controlled_mutation"
	// LayerRawShell — raw command execution, approval per policy.
	LayerRawShell CapabilityLayer = "raw_shell"
)

// AllCapabilityLayers returns the three canonical capability layers.
func AllCapabilityLayers() []CapabilityLayer {
	return []CapabilityLayer{LayerStructuredRead, LayerControlledMutation, LayerRawShell}
}

// ---------------------------------------------------------------------------
// Structured Read tools (14 standardized read-only interfaces).
// ---------------------------------------------------------------------------

var structuredReadTools = map[string]bool{
	"host_list":       true,
	"host_info":       true,
	"host_status":     true,
	"file_read":       true,
	"file_list":       true,
	"file_search":     true,
	"log_tail":        true,
	"log_search":      true,
	"process_list":    true,
	"disk_usage":      true,
	"memory_usage":    true,
	"network_status":  true,
	"service_status":  true,
	"container_list":  true,
}

// ---------------------------------------------------------------------------
// Controlled Mutation tools (5 controlled mutation interfaces).
// ---------------------------------------------------------------------------

var controlledMutationTools = map[string]bool{
	"file_write":       true,
	"service_restart":  true,
	"service_stop":     true,
	"config_update":    true,
	"container_remove": true,
}

// ---------------------------------------------------------------------------
// Raw Shell patterns — commands that go through raw shell execution.
// ---------------------------------------------------------------------------

var rawShellPatterns = []string{
	"command_exec", "script_run", "shell_exec", "raw_exec",
}

// ---------------------------------------------------------------------------
// High-risk command patterns — commands that must have TTL on whitelist entries.
// ---------------------------------------------------------------------------

var highRiskPatterns = []string{
	"rm -rf /",
	"rm -rf /*",
	"sudo su",
	"sudo -i",
	"iptables -F",
	"iptables --flush",
	"dd if=",
	"mkfs.",
	"fdisk",
	"shutdown",
	"reboot",
	"init 0",
	"init 6",
	"kill -9 1",
	"chmod -R 777 /",
}

// ---------------------------------------------------------------------------
// ClassifyTool determines which capability layer a tool belongs to.
// ---------------------------------------------------------------------------

// ClassifyTool classifies a tool into exactly one of the three capability layers.
// Classification priority: StructuredRead > ControlledMutation > RawShell.
// Tools that don't match any specific category default to RawShell if they
// match shell patterns, otherwise they are classified based on mutation patterns.
func ClassifyTool(toolName string) CapabilityLayer {
	// Check structured read first
	if structuredReadTools[toolName] {
		return LayerStructuredRead
	}

	// Check controlled mutation
	if controlledMutationTools[toolName] {
		return LayerControlledMutation
	}

	// Check raw shell patterns
	lower := strings.ToLower(toolName)
	for _, pattern := range rawShellPatterns {
		if strings.Contains(lower, pattern) {
			return LayerRawShell
		}
	}

	// Default classification based on mutation detection
	if isMutation(toolName) {
		return LayerControlledMutation
	}

	// Read-only tools that aren't in the structured read set still get structured_read
	if isReadOnly(toolName) {
		return LayerStructuredRead
	}

	// Unknown tools default to raw_shell (most restrictive)
	return LayerRawShell
}

// ---------------------------------------------------------------------------
// IsHighRiskCommand checks if a command matches high-risk patterns.
// ---------------------------------------------------------------------------

// IsHighRiskCommand reports whether the given command matches any high-risk pattern.
func IsHighRiskCommand(command string) bool {
	lower := strings.ToLower(strings.TrimSpace(command))
	for _, pattern := range highRiskPatterns {
		if strings.Contains(lower, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// WhitelistEntry represents an authorization whitelist entry.
// ---------------------------------------------------------------------------

// WhitelistEntry represents a host-level authorization whitelist entry.
type WhitelistEntry struct {
	ID        string        `json:"id"`
	HostID    string        `json:"hostId"`
	ToolName  string        `json:"toolName"`
	Command   string        `json:"command,omitempty"`
	TTL       time.Duration `json:"ttl,omitempty"`
	CreatedAt time.Time     `json:"createdAt"`
	ExpiresAt *time.Time    `json:"expiresAt,omitempty"`
	Enabled   bool          `json:"enabled"`
	Revoked   bool          `json:"revoked"`
}

// IsExpired reports whether the whitelist entry has expired.
func (w *WhitelistEntry) IsExpired(now time.Time) bool {
	if w.ExpiresAt == nil {
		return false
	}
	return now.After(*w.ExpiresAt)
}

// IsActive reports whether the whitelist entry is currently active
// (enabled, not revoked, not expired).
func (w *WhitelistEntry) IsActive(now time.Time) bool {
	return w.Enabled && !w.Revoked && !w.IsExpired(now)
}

// ---------------------------------------------------------------------------
// WhitelistManager manages authorization whitelist entries.
// ---------------------------------------------------------------------------

// WhitelistManager manages host-level authorization whitelist entries.
type WhitelistManager struct {
	mu      sync.RWMutex
	entries map[string]*WhitelistEntry // keyed by ID
}

// NewWhitelistManager creates a new empty whitelist manager.
func NewWhitelistManager() *WhitelistManager {
	return &WhitelistManager{
		entries: make(map[string]*WhitelistEntry),
	}
}

// Create adds a new whitelist entry. Returns an error if a high-risk command
// is provided without a TTL.
func (m *WhitelistManager) Create(entry WhitelistEntry) error {
	// High-risk commands must have a TTL
	if entry.Command != "" && IsHighRiskCommand(entry.Command) && entry.TTL == 0 {
		return &HighRiskNoTTLError{Command: entry.Command}
	}

	// Set expiration if TTL is provided
	if entry.TTL > 0 {
		expires := entry.CreatedAt.Add(entry.TTL)
		entry.ExpiresAt = &expires
	}

	entry.Enabled = true
	entry.Revoked = false

	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[entry.ID] = &entry
	return nil
}

// Revoke marks a whitelist entry as revoked.
func (m *WhitelistManager) Revoke(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.entries[id]; ok {
		e.Revoked = true
		return true
	}
	return false
}

// Disable disables a whitelist entry without revoking it.
func (m *WhitelistManager) Disable(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.entries[id]; ok {
		e.Enabled = false
		return true
	}
	return false
}

// Enable re-enables a previously disabled whitelist entry.
func (m *WhitelistManager) Enable(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.entries[id]; ok {
		e.Enabled = true
		return true
	}
	return false
}

// IsAuthorized checks if a tool/command is authorized for a given host at the given time.
func (m *WhitelistManager) IsAuthorized(hostID, toolName, command string, now time.Time) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, e := range m.entries {
		if e.HostID == hostID && e.ToolName == toolName && e.IsActive(now) {
			// If the entry has a specific command, it must match
			if e.Command != "" && e.Command != command {
				continue
			}
			return true
		}
	}
	return false
}

// Get returns a whitelist entry by ID.
func (m *WhitelistManager) Get(id string) (*WhitelistEntry, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	e, ok := m.entries[id]
	return e, ok
}

// ---------------------------------------------------------------------------
// HighRiskNoTTLError is returned when a high-risk command whitelist entry
// is created without a TTL.
// ---------------------------------------------------------------------------

// HighRiskNoTTLError indicates that a high-risk command cannot be whitelisted without a TTL.
type HighRiskNoTTLError struct {
	Command string
}

func (e *HighRiskNoTTLError) Error() string {
	return "high-risk command cannot be whitelisted without TTL: " + e.Command
}

// ---------------------------------------------------------------------------
// GatewayPolicy implements the three-layer capability gateway approval logic.
// ---------------------------------------------------------------------------

// GatewayPolicy implements the three-layer capability gateway.
// It determines the approval requirement based on the tool's capability layer.
type GatewayPolicy struct {
	Whitelist *WhitelistManager
}

// CheckApproval determines the approval requirement for a tool call based on
// the three-layer gateway classification.
func (g *GatewayPolicy) CheckApproval(toolName, command, hostID string, now time.Time) PolicyDecision {
	layer := ClassifyTool(toolName)

	switch layer {
	case LayerStructuredRead:
		// Structured Read: no approval needed
		return PolicyDecision{Action: PolicyActionAllow}

	case LayerControlledMutation:
		// Controlled Mutation: mandatory approval (unless whitelisted)
		if g.Whitelist != nil && g.Whitelist.IsAuthorized(hostID, toolName, command, now) {
			return PolicyDecision{Action: PolicyActionAllow}
		}
		return PolicyDecision{
			Action: PolicyActionNeedApproval,
			Reason: "controlled mutation requires approval",
			Approval: &ApprovalRequest{
				ToolName: toolName,
				Command:  command,
				HostID:   hostID,
				Reason:   "controlled mutation operation",
			},
		}

	case LayerRawShell:
		// Raw Shell: approval per policy (check whitelist and high-risk)
		if g.Whitelist != nil && g.Whitelist.IsAuthorized(hostID, toolName, command, now) {
			return PolicyDecision{Action: PolicyActionAllow}
		}
		reason := "raw shell execution requires approval"
		if IsHighRiskCommand(command) {
			reason = "high-risk command requires approval"
		}
		return PolicyDecision{
			Action: PolicyActionNeedApproval,
			Reason: reason,
			Approval: &ApprovalRequest{
				ToolName: toolName,
				Command:  command,
				HostID:   hostID,
				Reason:   reason,
			},
		}
	}

	return PolicyDecision{Action: PolicyActionDeny, Reason: "unknown capability layer"}
}
