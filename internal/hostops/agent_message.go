package hostops

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var ErrInvalidAgentMessage = errors.New("invalid agent message")

type AgentMessageType string

const (
	AgentMessageHostSubTaskAssigned            AgentMessageType = "host_subtask.assigned"
	AgentMessageHostSubTaskUpdated             AgentMessageType = "host_subtask.updated"
	AgentMessageHostSubTaskCancelled           AgentMessageType = "host_subtask.cancelled"
	AgentMessageHostReportProgress             AgentMessageType = "host_report.progress"
	AgentMessageHostReportCompleted            AgentMessageType = "host_report.completed"
	AgentMessageHostReportBlocked              AgentMessageType = "host_report.blocked"
	AgentMessageHostRequestManagerCoordination AgentMessageType = "host_request.manager_coordination"
	AgentMessageHostRequestApprovalNeeded      AgentMessageType = "host_request.approval_needed"
)

type AgentMessage struct {
	ID            string
	MissionID     string
	FromAgentID   string
	ToAgentID     string
	Type          AgentMessageType
	CreatedAt     time.Time
	CorrelationID string
	PayloadDigest string
	Payload       any
	SourceRefs    []string
}

type HostSubTaskAssignedPayload struct {
	SubTaskID              string
	RuntimeContextRef      string
	ContextDecisionTraceID string
	SourcePlanStepID       string
	Summary                string
}

func (p HostSubTaskAssignedPayload) Validate() error {
	if strings.TrimSpace(p.SubTaskID) == "" || strings.TrimSpace(p.RuntimeContextRef) == "" {
		return ErrInvalidAgentMessage
	}
	return nil
}

type HostTaskReportMessagePayload struct {
	ReportRef       string
	Report          HostTaskReport
	ValidationState string
}

func (p HostTaskReportMessagePayload) Validate() error {
	if strings.TrimSpace(p.ReportRef) == "" || strings.TrimSpace(p.ValidationState) == "" {
		return ErrInvalidAgentMessage
	}
	if strings.TrimSpace(p.Report.MissionID) == "" || strings.TrimSpace(p.Report.HostID) == "" || strings.TrimSpace(p.Report.PlanStepID) == "" {
		return ErrInvalidAgentMessage
	}
	return nil
}

func cloneAgentMessage(msg AgentMessage) AgentMessage {
	msg.SourceRefs = append([]string(nil), msg.SourceRefs...)
	if msg.Payload != nil {
		data, err := json.Marshal(msg.Payload)
		if err == nil {
			var payload any
			if json.Unmarshal(data, &payload) == nil {
				msg.Payload = payload
			}
		}
	}
	return msg
}
