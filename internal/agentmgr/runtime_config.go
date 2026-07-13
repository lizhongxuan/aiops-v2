package agentmgr

import (
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/tooling"
)

func (c *AgentConfig) RuntimeKind() string {
	if c == nil {
		return ""
	}
	return string(c.Kind)
}

func (c *AgentConfig) RuntimeModel() modelrouter.ChatModel {
	if c == nil {
		return nil
	}
	return c.Model
}

func (c *AgentConfig) RuntimeInstructions() []*schema.Message {
	if c == nil {
		return nil
	}
	return c.Instructions
}

func (c *AgentConfig) RuntimePromptEnvelopeV2() promptcompiler.PromptEnvelopeV2 {
	if c == nil {
		return promptcompiler.PromptEnvelopeV2{}
	}
	return clonePromptEnvelopeV2(c.PromptEnvelopeV2)
}

func (c *AgentConfig) RuntimeTools() []tool.BaseTool {
	if c == nil {
		return nil
	}
	return c.Tools
}

func (c *AgentConfig) RuntimeAssembledTools() []tooling.Tool {
	if c == nil {
		return nil
	}
	return c.AssembledTools
}

func (c *AgentConfig) RuntimeMaxIterations() int {
	if c == nil {
		return 0
	}
	return c.MaxIterations
}

func (c *AgentConfig) RuntimeHostID() string {
	if c == nil {
		return ""
	}
	return c.HostID
}

func (c *AgentConfig) RuntimeMissionID() string {
	if c == nil {
		return ""
	}
	return c.MissionID
}

func (c *AgentConfig) RuntimeSessionID() string {
	if c == nil {
		return ""
	}
	return c.SessionID
}

func (c *AgentConfig) RuntimeInput() string {
	if c == nil {
		return ""
	}
	return c.Input
}

func (c *AgentConfig) RuntimeMetadata() map[string]string {
	if c == nil {
		return nil
	}
	return c.Metadata
}
