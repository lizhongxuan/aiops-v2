package appui

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/incidents"
	"aiops-v2/internal/runtimekernel"
)

func TestChatArchiveServiceArchiveCaseCreatesIncidentOnDemand(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	session := sessions.GetOrCreate("sess-archive", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	now := time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)
	session.CurrentTurn = &runtimekernel.TurnSnapshot{
		ID:        "turn-archive",
		SessionID: "sess-archive",
		Metadata: map[string]string{
			"aiops.opsRunId": "opsrun-turn-archive",
		},
		FinalOutput: "已完成只读排查，未声明已验证成功。",
		StartedAt:   now,
		UpdatedAt:   now,
		AgentItems: []agentstate.TurnItem{
			{ID: "user-1", Type: agentstate.TurnItemTypeUserMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "主机A跟主机B上PG不同步，pg_mon部署在主机C，请修复"}, CreatedAt: now},
			{ID: "evidence-1", Type: agentstate.TurnItemTypeEvidence, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "主机A/主机B LSN 不一致，主机C pg_mon 观测到 replay 停滞"}, CreatedAt: now.Add(time.Second)},
			{ID: "approval-1", Type: agentstate.TurnItemTypeApproval, Status: agentstate.ItemStatusBlocked, Payload: agentstate.PayloadEnvelope{Summary: "修复复制关系前需要用户审批"}, CreatedAt: now.Add(2 * time.Second)},
		},
		PendingApprovals: []runtimekernel.PendingApproval{{
			ID:        "approval-pg-repair",
			SessionID: "sess-archive",
			TurnID:    "turn-archive",
			ToolName:  "host_command",
			Command:   "systemctl restart postgresql",
			Reason:    "重启数据库服务属于写操作",
			Risk:      "high_write",
			Status:    "pending",
			CreatedAt: now.Add(3 * time.Second),
			UpdatedAt: now.Add(3 * time.Second),
		}},
		Iterations: []runtimekernel.IterationState{{
			ID:        "iter-1",
			SessionID: "sess-archive",
			TurnID:    "turn-archive",
			Iteration: 1,
			Lifecycle: runtimekernel.TurnLifecycleCompleted,
			ToolCalls: []runtimekernel.ToolCall{{
				ID:        "tool-pg-status",
				Name:      "host_readonly_check",
				Arguments: json.RawMessage(`{"hostId":"host-a","check":"pg replication"}`),
			}},
			ToolResults: []runtimekernel.ToolResult{{
				ToolCallID: "tool-pg-status",
				Summary:    "host-a role=primary lsn=0/500; host-b replay_lsn=0/300 lag=128MB",
				References: []runtimekernel.ToolResultReference{{
					Kind:    runtimekernel.ToolResultReferenceKindMCPResource,
					URI:     "evidence://host/host-a/pg-readonly",
					Summary: "主机A PG 只读复制状态",
				}},
			}},
			StartedAt: now,
			UpdatedAt: now.Add(4 * time.Second),
		}},
	}
	incidentService := NewIncidentService(incidents.NewService(incidents.NewInMemoryStore(), func() time.Time { return now }))
	service := NewChatArchiveService(sessions, incidentService)

	before, err := incidentService.List(context.Background())
	if err != nil {
		t.Fatalf("List() before error = %v", err)
	}
	if len(before) != 0 {
		t.Fatalf("incidents before archive = %#v, want empty", before)
	}

	result, err := service.ArchiveCase(context.Background(), ChatArchiveRequest{OpsRunID: "opsrun-turn-archive"})
	if err != nil {
		t.Fatalf("ArchiveCase() error = %v", err)
	}
	if result.Case.ExternalID != "opsrun-turn-archive" || result.Case.Source != "ai_chat" {
		t.Fatalf("case = %#v, want ai_chat case with opsRun external id", result.Case)
	}
	if result.Case.Title != "主机A跟主机B上PG不同步，pg_mon部署在主机C，请修复" {
		t.Fatalf("case title = %q", result.Case.Title)
	}
	if len(result.Case.EvidenceRefs) < 5 || len(result.Case.Evidence) < 5 {
		t.Fatalf("case evidence = refs:%#v evidence:%#v, want user/diagnosis/evidence/approval/tool refs", result.Case.EvidenceRefs, result.Case.Evidence)
	}
	for _, source := range []string{"ai_chat", "diagnosis", "agent_evidence", "approval", "execution_result", "tool_reference"} {
		if !archiveCaseHasEvidenceSource(result.Case.Evidence, source) {
			t.Fatalf("case evidence missing source %q: %#v", source, result.Case.Evidence)
		}
	}
	if archiveCaseHasEvidenceSummary(result.Case.Evidence, "已验证成功") {
		t.Fatalf("case evidence must not claim verified success: %#v", result.Case.Evidence)
	}
}

func archiveCaseHasEvidenceSource(items []EvidenceRefView, source string) bool {
	for _, item := range items {
		if item.Source == source {
			return true
		}
	}
	return false
}

func archiveCaseHasEvidenceSummary(items []EvidenceRefView, value string) bool {
	for _, item := range items {
		if item.Summary == value {
			return true
		}
	}
	return false
}

func TestChatArchiveServiceCreatesRunRecordCandidateWithoutVerifiedSuccess(t *testing.T) {
	service := &defaultChatArchiveService{now: func() time.Time { return time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC) }}

	result, err := service.CreateRunRecord(context.Background(), ChatArchiveRequest{
		OpsRunID: "opsrun-1",
		Title:    "PG 同步排查",
		Summary:  "已完成处理记录候选，未做运行验证。",
	})
	if err != nil {
		t.Fatalf("CreateRunRecord() error = %v", err)
	}
	if result.Status != "candidate" || result.Title != "PG 同步排查" {
		t.Fatalf("run record = %#v, want candidate", result)
	}
	if result.Summary == "已验证成功" {
		t.Fatalf("run record summary must not claim verified success: %#v", result)
	}
}
