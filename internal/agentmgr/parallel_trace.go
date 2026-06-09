package agentmgr

import "strings"

type AgentParallelTraceGroup struct {
	MissionID      string                   `json:"missionId"`
	RequestedCount int                      `json:"requestedCount"`
	SpawnedInTurn  []string                 `json:"spawnedInTurn"`
	Queued         []string                 `json:"queued,omitempty"`
	SerialReasons  []AgentSerialReasonTrace `json:"serialReasons,omitempty"`
}

type AgentSerialReasonTrace struct {
	AgentID string `json:"agentId"`
	Reason  string `json:"reason"` // budget_exceeded | resource_lock_conflict | policy_blocked | missing_parallel_spawn_batch
}

type AgentParallelTraceInput struct {
	MissionID         string
	ParallelRequested bool
	RequestedCount    int
	SpawnedInTurn     []string
	Queued            []string
	SerialReasons     []AgentSerialReasonTrace
}

func BuildAgentParallelTraceGroup(in AgentParallelTraceInput) AgentParallelTraceGroup {
	group := AgentParallelTraceGroup{
		MissionID:      strings.TrimSpace(in.MissionID),
		RequestedCount: in.RequestedCount,
		SpawnedInTurn:  cloneNonEmptyAgentIDs(in.SpawnedInTurn),
		Queued:         cloneNonEmptyAgentIDs(in.Queued),
		SerialReasons:  cloneAgentSerialReasons(in.SerialReasons),
	}
	if group.RequestedCount == 0 {
		group.RequestedCount = len(group.SpawnedInTurn) + len(group.Queued)
	}
	if in.ParallelRequested && len(group.SpawnedInTurn) < 2 {
		agentID := ""
		if len(group.SpawnedInTurn) == 1 {
			agentID = group.SpawnedInTurn[0]
		}
		group.SerialReasons = append(group.SerialReasons, AgentSerialReasonTrace{
			AgentID: agentID,
			Reason:  "missing_parallel_spawn_batch",
		})
	}
	for _, queued := range group.Queued {
		group.SerialReasons = append(group.SerialReasons, AgentSerialReasonTrace{
			AgentID: queued,
			Reason:  "budget_exceeded",
		})
	}
	return group
}

func cloneNonEmptyAgentIDs(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func cloneAgentSerialReasons(values []AgentSerialReasonTrace) []AgentSerialReasonTrace {
	if len(values) == 0 {
		return nil
	}
	out := make([]AgentSerialReasonTrace, 0, len(values))
	for _, value := range values {
		value.AgentID = strings.TrimSpace(value.AgentID)
		value.Reason = strings.TrimSpace(value.Reason)
		if value.AgentID == "" && value.Reason == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}
