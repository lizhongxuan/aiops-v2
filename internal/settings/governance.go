package settings

import (
	"fmt"
	"strings"
	"sync"
)

// CustomizationSurface identifies a governance-controlled customization surface.
type CustomizationSurface string

const (
	SurfaceSkills CustomizationSurface = "skills"
	SurfaceAgents CustomizationSurface = "agents"
	SurfaceHooks  CustomizationSurface = "hooks"
	SurfaceMCP    CustomizationSurface = "mcp"
)

var customizationSurfaces = []CustomizationSurface{
	SurfaceSkills,
	SurfaceAgents,
	SurfaceHooks,
	SurfaceMCP,
}

// GovernanceContribution stores one governance overlay contributed by a plugin or settings source.
type GovernanceContribution struct {
	RestrictToPluginOnly  []CustomizationSurface
	AllowedMCPServers     []string
	AdditionalDirectories []string
}

// Governance aggregates governance overlays contributed by registered components.
type Governance struct {
	mu    sync.RWMutex
	items map[string]GovernanceContribution
	order []string
}

// GovernanceSnapshot is an immutable merged view of active governance contributions.
type GovernanceSnapshot struct {
	restricted            map[CustomizationSurface]struct{}
	allowedMCPServers     []string
	additionalDirectories []string
}

// NewGovernance creates an empty governance aggregator.
func NewGovernance() *Governance {
	return &Governance{
		items: make(map[string]GovernanceContribution),
	}
}

// AllCustomizationSurfaces returns all governance-controlled customization surfaces.
func AllCustomizationSurfaces() []CustomizationSurface {
	return append([]CustomizationSurface(nil), customizationSurfaces...)
}

// Register stores a named governance contribution.
func (g *Governance) Register(name string, contribution GovernanceContribution) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("settings: governance contribution name is required")
	}
	contribution = normalizeContribution(contribution)

	g.mu.Lock()
	defer g.mu.Unlock()

	if _, exists := g.items[name]; exists {
		return fmt.Errorf("settings: governance contribution %q already registered", name)
	}
	g.items[name] = contribution
	g.order = append(g.order, name)
	return nil
}

// Unregister removes a governance contribution by name.
func (g *Governance) Unregister(name string) {
	name = strings.TrimSpace(name)

	g.mu.Lock()
	defer g.mu.Unlock()

	delete(g.items, name)
	filtered := g.order[:0]
	for _, candidate := range g.order {
		if candidate == name {
			continue
		}
		filtered = append(filtered, candidate)
	}
	g.order = filtered
}

// Snapshot returns a merged governance view.
func (g *Governance) Snapshot() GovernanceSnapshot {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return snapshotFrom(g.order, g.items)
}

// Project returns the merged governance view after hypothetically adding the contribution.
func (g *Governance) Project(name string, contribution GovernanceContribution) (GovernanceSnapshot, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return GovernanceSnapshot{}, fmt.Errorf("settings: governance contribution name is required")
	}
	contribution = normalizeContribution(contribution)

	g.mu.RLock()
	defer g.mu.RUnlock()

	if _, exists := g.items[name]; exists {
		return GovernanceSnapshot{}, fmt.Errorf("settings: governance contribution %q already registered", name)
	}

	items := make(map[string]GovernanceContribution, len(g.items)+1)
	for key, item := range g.items {
		items[key] = item
	}
	order := append([]string(nil), g.order...)
	items[name] = contribution
	order = append(order, name)
	return snapshotFrom(order, items), nil
}

// IsRestrictedToPluginOnly reports whether the surface is locked to admin-trusted sources.
func (s GovernanceSnapshot) IsRestrictedToPluginOnly(surface CustomizationSurface) bool {
	_, ok := s.restricted[surface]
	return ok
}

// AllowsSource reports whether the source is allowed on the surface under the active governance state.
func (s GovernanceSnapshot) AllowsSource(surface CustomizationSurface, source string) bool {
	if !s.IsRestrictedToPluginOnly(surface) {
		return true
	}
	return IsAdminTrustedSource(source)
}

// AllowsMCPServer reports whether an MCP server is allowed for the given source and server id.
func (s GovernanceSnapshot) AllowsMCPServer(source, serverID string) bool {
	if !s.AllowsSource(SurfaceMCP, source) {
		return false
	}
	if IsAdminTrustedSource(source) {
		return true
	}
	if len(s.allowedMCPServers) == 0 {
		return true
	}
	serverID = strings.TrimSpace(serverID)
	for _, allowed := range s.allowedMCPServers {
		if allowed == serverID {
			return true
		}
	}
	return false
}

// AllowedMCPServers returns the merged MCP allowlist.
func (s GovernanceSnapshot) AllowedMCPServers() []string {
	return append([]string(nil), s.allowedMCPServers...)
}

// AdditionalDirectories returns the merged additional permission scope directories.
func (s GovernanceSnapshot) AdditionalDirectories() []string {
	return append([]string(nil), s.additionalDirectories...)
}

// IsAdminTrustedSource reports whether the source bypasses strict plugin-only restrictions.
func IsAdminTrustedSource(source string) bool {
	switch strings.TrimSpace(source) {
	case "",
		"plugin",
		string(SourcePolicySettings),
		"built-in",
		"builtin",
		"bundled":
		return true
	default:
		return false
	}
}

func normalizeContribution(contribution GovernanceContribution) GovernanceContribution {
	contribution.RestrictToPluginOnly = dedupeSurfaces(contribution.RestrictToPluginOnly)
	contribution.AllowedMCPServers = dedupeGovernanceStrings(contribution.AllowedMCPServers)
	contribution.AdditionalDirectories = dedupeGovernanceStrings(contribution.AdditionalDirectories)
	return contribution
}

func dedupeSurfaces(surfaces []CustomizationSurface) []CustomizationSurface {
	if len(surfaces) == 0 {
		return nil
	}
	allowed := make(map[CustomizationSurface]struct{}, len(customizationSurfaces))
	for _, surface := range customizationSurfaces {
		allowed[surface] = struct{}{}
	}
	seen := make(map[CustomizationSurface]struct{}, len(surfaces))
	out := make([]CustomizationSurface, 0, len(surfaces))
	for _, surface := range surfaces {
		surface = CustomizationSurface(strings.TrimSpace(string(surface)))
		if surface == "" {
			continue
		}
		if _, ok := allowed[surface]; !ok {
			continue
		}
		if _, ok := seen[surface]; ok {
			continue
		}
		seen[surface] = struct{}{}
		out = append(out, surface)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func snapshotFrom(order []string, items map[string]GovernanceContribution) GovernanceSnapshot {
	restricted := make(map[CustomizationSurface]struct{})
	var allowedMCPServers []string
	var additionalDirectories []string
	seenMCP := make(map[string]struct{})
	seenDirectories := make(map[string]struct{})

	for _, name := range order {
		contribution, ok := items[name]
		if !ok {
			continue
		}
		for _, surface := range contribution.RestrictToPluginOnly {
			restricted[surface] = struct{}{}
		}
		for _, server := range contribution.AllowedMCPServers {
			if _, ok := seenMCP[server]; ok {
				continue
			}
			seenMCP[server] = struct{}{}
			allowedMCPServers = append(allowedMCPServers, server)
		}
		for _, directory := range contribution.AdditionalDirectories {
			if _, ok := seenDirectories[directory]; ok {
				continue
			}
			seenDirectories[directory] = struct{}{}
			additionalDirectories = append(additionalDirectories, directory)
		}
	}

	return GovernanceSnapshot{
		restricted:            restricted,
		allowedMCPServers:     allowedMCPServers,
		additionalDirectories: additionalDirectories,
	}
}

func dedupeGovernanceStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
