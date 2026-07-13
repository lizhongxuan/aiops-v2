package agentruntime

import (
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/tooling"
)

// Config is the runtime-facing view of an assembled child agent.
//
// It intentionally lives outside agentmgr and runtimekernel so the manager can
// pass assembled configs to the shared AI Chat runtime without import cycles.
type Config interface {
	RuntimeKind() string
	RuntimeModel() modelrouter.ChatModel
	RuntimeInstructions() []*schema.Message
	RuntimeTools() []tool.BaseTool
	RuntimeAssembledTools() []tooling.Tool
	RuntimeMaxIterations() int
	RuntimeHostID() string
	RuntimeMissionID() string
	RuntimeSessionID() string
	RuntimeInput() string
	RuntimeMetadata() map[string]string
}

// PromptEnvelopeV2Config is the typed prompt extension for child-agent
// configs. Keeping it separate preserves compatibility with legacy Config
// implementations while allowing the runtime to prefer validated envelopes.
type PromptEnvelopeV2Config interface {
	RuntimePromptEnvelopeV2() promptcompiler.PromptEnvelopeV2
}
