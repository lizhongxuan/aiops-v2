package tooling

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"github.com/cloudwego/eino/components/tool"
)

// DynamicToolProvider supplies dynamic tools that should participate in the
// same assembly pass as statically registered tools.
type DynamicToolProvider interface {
	DynamicTools() []Tool
}

// DynamicToolRefreshTokenProvider optionally supplies a stable token that
// changes whenever the provider's dynamic tool surface changes.
type DynamicToolRefreshTokenProvider interface {
	DynamicToolRefreshToken() string
}

// Assembler composes the base tool registry with dynamic providers so prompt,
// runtime, and agent assembly all read from one tool source of truth.
type Assembler struct {
	registry  *Registry
	providers []DynamicToolProvider
}

// NewAssembler creates an assembler backed by a base registry plus any number
// of dynamic tool providers such as MCP registries.
func NewAssembler(registry *Registry, providers ...DynamicToolProvider) *Assembler {
	filtered := make([]DynamicToolProvider, 0, len(providers))
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		filtered = append(filtered, provider)
	}
	return &Assembler{
		registry:  registry,
		providers: filtered,
	}
}

// AssembleToolsWithOptions returns the visible tool set after merging the base
// registry with all dynamic providers.
func (a *Assembler) AssembleToolsWithOptions(session, mode string, opts AssembleOptions) []Tool {
	if a == nil {
		return nil
	}

	merged := opts
	merged.ExtraTools = append([]Tool(nil), opts.ExtraTools...)
	for _, provider := range a.providers {
		merged.ExtraTools = append(merged.ExtraTools, provider.DynamicTools()...)
	}

	if a.registry == nil {
		tmp := NewRegistry()
		return tmp.AssembleToolsWithOptions(session, mode, merged)
	}
	return a.registry.AssembleToolsWithOptions(session, mode, merged)
}

// AssembleToolPoolWithOptions adapts the assembled tools into Eino base tools.
func (a *Assembler) AssembleToolPoolWithOptions(session, mode string, opts AssembleOptions) []tool.BaseTool {
	return AssembleEinoToolPool(a.AssembleToolsWithOptions(session, mode, opts))
}

// RefreshToken returns a stable token that changes when the assembled tool
// surface changes. It can be used by runtimekernel to decide whether an
// iteration must refresh its tool context.
func (a *Assembler) RefreshToken(session, mode string, opts AssembleOptions) string {
	return a.StableToolFingerprint(session, mode, opts)
}

// StableToolFingerprint returns a deterministic fingerprint for the final
// assembled tool surface, including dynamic provider refresh tokens when
// available.
func (a *Assembler) StableToolFingerprint(session, mode string, opts AssembleOptions) string {
	if a == nil {
		return ""
	}

	h := sha256.New()

	for _, provider := range a.providers {
		if tokenProvider, ok := any(provider).(DynamicToolRefreshTokenProvider); ok {
			writeFingerprintPart(h, "provider-token", tokenProvider.DynamicToolRefreshToken())
		} else {
			for _, tool := range provider.DynamicTools() {
				writeFingerprintPart(h, "provider-tool", fingerprintTool(session, mode, tool))
			}
		}
	}

	for _, tool := range a.AssembleToolsWithOptions(session, mode, opts) {
		writeFingerprintPart(h, "tool", fingerprintTool(session, mode, tool))
	}

	return hex.EncodeToString(h.Sum(nil))
}

func fingerprintTool(session, mode string, tool Tool) string {
	if tool == nil {
		return ""
	}

	meta := tool.Metadata()
	meta.Aliases = append([]string(nil), meta.Aliases...)
	sort.Strings(meta.Aliases)

	payload := struct {
		Metadata        ToolMetadata `json:"metadata"`
		InputSchema     string       `json:"inputSchema,omitempty"`
		OutputSchema    string       `json:"outputSchema,omitempty"`
		Description     string       `json:"description,omitempty"`
		Prompt          string       `json:"prompt,omitempty"`
		Enabled         bool         `json:"enabled"`
		ReadOnly        bool         `json:"readOnly"`
		Destructive     bool         `json:"destructive"`
		ConcurrencySafe bool         `json:"concurrencySafe"`
	}{
		Metadata:        meta,
		InputSchema:     strings.TrimSpace(string(tool.InputSchema())),
		OutputSchema:    strings.TrimSpace(string(tool.OutputSchema())),
		Description:     strings.TrimSpace(tool.Description(nil, DescribeContext{SessionType: session, Mode: mode, Metadata: meta})),
		Prompt:          strings.TrimSpace(tool.Prompt(PromptContext{SessionType: session, Mode: mode, Metadata: meta})),
		Enabled:         tool.IsEnabled(ToolContext{SessionType: session, Mode: mode, Metadata: meta}),
		ReadOnly:        tool.IsReadOnly(nil),
		Destructive:     tool.IsDestructive(nil),
		ConcurrencySafe: tool.IsConcurrencySafe(nil),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return meta.Name
	}
	return string(data)
}

func writeFingerprintPart(h interface{ Write([]byte) (int, error) }, kind, value string) {
	_, _ = h.Write([]byte(kind))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(value))
	_, _ = h.Write([]byte{0})
}
