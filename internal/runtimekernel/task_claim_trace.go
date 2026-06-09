package runtimekernel

import (
	"encoding/json"

	"aiops-v2/internal/planning"
	"aiops-v2/internal/promptinput"
)

func taskClaimTracesFromSnapshot(snapshot *TurnSnapshot) []promptinput.TaskClaimTrace {
	if snapshot == nil {
		return nil
	}
	var out []promptinput.TaskClaimTrace
	for _, iteration := range snapshot.Iterations {
		for _, result := range iteration.ToolResults {
			if result.Display == nil || result.Display.Type != "plan_task_claim" {
				continue
			}
			var payload planning.ClaimNextTaskResult
			if len(result.Display.Data) > 0 {
				_ = json.Unmarshal(result.Display.Data, &payload)
			}
			if payload.TaskID == "" && result.Content != "" {
				_ = json.Unmarshal([]byte(result.Content), &payload)
			}
			if payload.TaskID == "" && payload.Reason == "" {
				continue
			}
			status := "claimed"
			if !payload.Claimed {
				status = "not_claimed"
			}
			out = append(out, promptinput.TaskClaimTrace{
				TaskID: payload.TaskID,
				Owner:  payload.Owner,
				Status: status,
				Reason: payload.Reason,
			})
		}
	}
	return out
}
