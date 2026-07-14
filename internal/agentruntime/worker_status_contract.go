package agentruntime

import (
	"encoding/json"
	"strings"
)

// WorkerStatusFact is the source-authenticated worker state projected by a
// versioned orchestration tool contract.
type WorkerStatusFact struct {
	AgentID string
	Status  string
}

type workerStatusContract struct {
	SchemaVersion string `json:"schemaVersion"`
	Children      []struct {
		AgentID string `json:"childAgentId"`
		Status  string `json:"status"`
	} `json:"children"`
}

var workerStatusSchemasByTool = map[string]string{
	"spawn_host_agent": "aiops.hostops.child/v1",
	"wait_host_agents": "aiops.hostops.wait/v1",
}

// ParseWorkerStatusContract binds a machine payload to the orchestration tool
// that is authorized to emit its schema. A matching schema from any other tool
// is not a worker lifecycle fact.
func ParseWorkerStatusContract(toolName string, data []byte) ([]WorkerStatusFact, bool) {
	wantSchema := workerStatusSchemasByTool[strings.TrimSpace(toolName)]
	if wantSchema == "" {
		return nil, false
	}
	var payload workerStatusContract
	if json.Unmarshal(data, &payload) != nil || payload.SchemaVersion != wantSchema {
		return nil, false
	}
	out := make([]WorkerStatusFact, 0, len(payload.Children))
	for _, child := range payload.Children {
		id := strings.TrimSpace(child.AgentID)
		status := strings.ToLower(strings.TrimSpace(child.Status))
		if id == "" || status == "" {
			continue
		}
		out = append(out, WorkerStatusFact{AgentID: id, Status: status})
	}
	return out, true
}
