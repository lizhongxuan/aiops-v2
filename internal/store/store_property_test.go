package store

import (
	"encoding/json"
	"testing"
	"time"

	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/tooling"
	"pgregory.net/rapid"
)

// Feature: aiops-codex-eino-rewrite, Property 31: 会话序列化 Round-Trip
// **Validates: Requirements 11.2, 11.5**

// genSessionType generates a valid SessionType.
func genSessionType(t *rapid.T) runtimekernel.SessionType {
	types := runtimekernel.AllSessionTypes()
	return types[rapid.IntRange(0, len(types)-1).Draw(t, "sessionTypeIdx")]
}

// genMode generates a valid Mode.
func genMode(t *rapid.T) runtimekernel.Mode {
	modes := runtimekernel.AllModes()
	return modes[rapid.IntRange(0, len(modes)-1).Draw(t, "modeIdx")]
}

// genMessage generates a random Message.
func genMessage(t *rapid.T) runtimekernel.Message {
	roles := []string{"user", "assistant", "system", "tool"}
	role := roles[rapid.IntRange(0, len(roles)-1).Draw(t, "roleIdx")]
	msg := runtimekernel.Message{
		ID:        rapid.StringMatching(`[a-z0-9]{8,16}`).Draw(t, "msgId"),
		Role:      role,
		Content:   rapid.String().Draw(t, "content"),
		Timestamp: time.Now().Truncate(time.Second),
	}
	// Optionally add tool calls for assistant messages
	if role == "assistant" && rapid.Bool().Draw(t, "hasToolCalls") {
		numCalls := rapid.IntRange(1, 3).Draw(t, "numToolCalls")
		for i := 0; i < numCalls; i++ {
			tc := runtimekernel.ToolCall{
				ID:        rapid.StringMatching(`[a-z0-9]{8}`).Draw(t, "tcId"),
				Name:      rapid.StringMatching(`[a-z_]{3,12}`).Draw(t, "tcName"),
				Arguments: json.RawMessage(`{"key":"value"}`),
			}
			msg.ToolCalls = append(msg.ToolCalls, tc)
		}
	}
	// Optionally add tool result for tool messages
	if role == "tool" {
		msg.ToolResult = &runtimekernel.ToolResult{
			ToolCallID: rapid.StringMatching(`[a-z0-9]{8}`).Draw(t, "trCallId"),
			Content:    rapid.String().Draw(t, "trContent"),
		}
	}
	return msg
}

// genContextWindow generates a valid ContextWindow.
func genContextWindow(t *rapid.T) runtimekernel.ContextWindow {
	maxTokens := rapid.IntRange(1000, 128000).Draw(t, "maxTokens")
	usedTokens := rapid.IntRange(0, maxTokens).Draw(t, "usedTokens")
	return runtimekernel.ContextWindow{
		MaxTokens:   maxTokens,
		UsedTokens:  usedTokens,
		Messages:    rapid.IntRange(0, 1000).Draw(t, "ctxMessages"),
		TruncatedAt: rapid.IntRange(0, 100).Draw(t, "truncatedAt"),
	}
}

// genActivityStats generates random ActivityStats.
func genActivityStats(t *rapid.T) runtimekernel.ActivityStats {
	return runtimekernel.ActivityStats{
		SearchCount:    rapid.IntRange(0, 100).Draw(t, "searchCount"),
		BrowseCount:    rapid.IntRange(0, 100).Draw(t, "browseCount"),
		CommandCount:   rapid.IntRange(0, 100).Draw(t, "commandCount"),
		FileReadCount:  rapid.IntRange(0, 100).Draw(t, "fileReadCount"),
		FileWriteCount: rapid.IntRange(0, 100).Draw(t, "fileWriteCount"),
	}
}

func genCheckpointMetadata(t *rapid.T, sessionID, turnID string, iteration int) *runtimekernel.CheckpointMetadata {
	now := time.Now().Truncate(time.Second)
	return &runtimekernel.CheckpointMetadata{
		ID:          rapid.StringMatching(`chk-[a-z0-9]{6,12}`).Draw(t, "checkpointId"),
		SessionID:   sessionID,
		TurnID:      turnID,
		Iteration:   iteration,
		Sequence:    rapid.IntRange(0, 10).Draw(t, "checkpointSequence"),
		Kind:        rapid.SampledFrom([]string{"assistant_checkpoint", "tool_checkpoint", "resume_checkpoint"}).Draw(t, "checkpointKind"),
		Source:      "runtimekernel",
		Lifecycle:   runtimekernel.TurnLifecycleSuspended,
		ResumeState: runtimekernel.TurnResumeStateCheckpointReady,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func genPendingApproval(t *rapid.T, sessionID, turnID string, iteration int) runtimekernel.PendingApproval {
	now := time.Now().Truncate(time.Second)
	return runtimekernel.PendingApproval{
		ID:        rapid.StringMatching(`approval-[a-z0-9]{6,12}`).Draw(t, "approvalId"),
		SessionID: sessionID,
		TurnID:    turnID,
		Iteration: iteration,
		ToolName:  rapid.StringMatching(`[a-z_]{3,12}`).Draw(t, "approvalTool"),
		Status:    "pending",
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func genPendingEvidence(t *rapid.T, sessionID, turnID string, iteration int) runtimekernel.PendingEvidence {
	now := time.Now().Truncate(time.Second)
	return runtimekernel.PendingEvidence{
		ID:        rapid.StringMatching(`evidence-[a-z0-9]{6,12}`).Draw(t, "evidenceId"),
		SessionID: sessionID,
		TurnID:    turnID,
		Iteration: iteration,
		ToolName:  rapid.StringMatching(`[a-z_]{3,12}`).Draw(t, "evidenceTool"),
		Status:    "pending",
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func genExternalReference(t *rapid.T, sessionID, turnID string, iteration int) runtimekernel.ExternalReference {
	now := time.Now().Truncate(time.Second)
	return runtimekernel.ExternalReference{
		ID:        rapid.StringMatching(`ref-[a-z0-9]{6,12}`).Draw(t, "referenceId"),
		SessionID: sessionID,
		TurnID:    turnID,
		Iteration: iteration,
		Kind:      "blob",
		URI:       "file:///tmp/blob",
		CreatedAt: now,
	}
}

func genCompactedSegment(t *rapid.T, sessionID, turnID string, iteration int) runtimekernel.CompactedSegment {
	checkpoint := genCheckpointMetadata(t, sessionID, turnID, iteration)
	refs := []runtimekernel.ExternalReference{genExternalReference(t, sessionID, turnID, iteration)}
	return runtimekernel.CompactedSegment{
		ID:                 rapid.StringMatching(`segment-[a-z0-9]{6,12}`).Draw(t, "segmentId"),
		SessionID:          sessionID,
		TurnID:             turnID,
		Iteration:          iteration,
		StartIndex:         rapid.IntRange(0, 5).Draw(t, "segmentStart"),
		EndIndex:           rapid.IntRange(5, 10).Draw(t, "segmentEnd"),
		Summary:            rapid.String().Draw(t, "segmentSummary"),
		ReferenceIDs:       []string{refs[0].ID},
		ExternalReferences: refs,
		Checkpoint:         checkpoint,
		CreatedAt:          time.Now().Truncate(time.Second),
	}
}

func genIterationState(t *rapid.T, sessionID, turnID string, iteration int) runtimekernel.IterationState {
	checkpoint := genCheckpointMetadata(t, sessionID, turnID, iteration)
	approval := genPendingApproval(t, sessionID, turnID, iteration)
	evidence := genPendingEvidence(t, sessionID, turnID, iteration)
	segment := genCompactedSegment(t, sessionID, turnID, iteration)
	ref := genExternalReference(t, sessionID, turnID, iteration)
	return runtimekernel.IterationState{
		ID:                 rapid.StringMatching(`iter-[a-z0-9]{6,12}`).Draw(t, "iterationId"),
		SessionID:          sessionID,
		TurnID:             turnID,
		Iteration:          iteration,
		Lifecycle:          runtimekernel.TurnLifecycleSuspended,
		ResumeState:        runtimekernel.TurnResumeStateCheckpointReady,
		MessagesForModel:   []runtimekernel.Message{genMessage(t)},
		ToolCalls:          []runtimekernel.ToolCall{{ID: rapid.StringMatching(`[a-z0-9]{8}`).Draw(t, "iterToolCall"), Name: rapid.StringMatching(`[a-z_]{3,12}`).Draw(t, "iterToolName"), Arguments: json.RawMessage(`{"key":"value"}`)}},
		ToolResults:        []runtimekernel.ToolResult{{ToolCallID: "tc", Content: rapid.String().Draw(t, "iterToolResult")}},
		RefreshedTools:     []string{rapid.StringMatching(`[a-z_]{3,12}`).Draw(t, "refreshedTool")},
		PromptDelta:        rapid.String().Draw(t, "promptDelta"),
		TokenBudget:        rapid.IntRange(0, 4096).Draw(t, "tokenBudget"),
		ResultBudget:       rapid.IntRange(0, 8192).Draw(t, "resultBudget"),
		Checkpoint:         checkpoint,
		PendingApprovals:   []runtimekernel.PendingApproval{approval},
		PendingEvidence:    []runtimekernel.PendingEvidence{evidence},
		CompactedSegments:  []runtimekernel.CompactedSegment{segment},
		ExternalReferences: []runtimekernel.ExternalReference{ref},
		StartedAt:          time.Now().Truncate(time.Second),
		UpdatedAt:          time.Now().Truncate(time.Second),
	}
}

func genTurnSnapshot(t *rapid.T, sessionID string, sessionType runtimekernel.SessionType, mode runtimekernel.Mode) *runtimekernel.TurnSnapshot {
	turnID := rapid.StringMatching(`turn-[a-z0-9]{6,12}`).Draw(t, "turnId")
	iteration := rapid.IntRange(0, 5).Draw(t, "turnIteration")
	checkpoint := genCheckpointMetadata(t, sessionID, turnID, iteration)
	approval := genPendingApproval(t, sessionID, turnID, iteration)
	evidence := genPendingEvidence(t, sessionID, turnID, iteration)
	segment := genCompactedSegment(t, sessionID, turnID, iteration)
	return &runtimekernel.TurnSnapshot{
		ID:                    turnID,
		SessionID:             sessionID,
		SessionType:           sessionType,
		Mode:                  mode,
		Lifecycle:             runtimekernel.TurnLifecycleSuspended,
		ResumeState:           runtimekernel.TurnResumeStateCheckpointReady,
		Iteration:             iteration,
		StartedAt:             time.Now().Truncate(time.Second),
		UpdatedAt:             time.Now().Truncate(time.Second),
		StablePromptHash:      rapid.StringMatching(`[a-f0-9]{8,16}`).Draw(t, "stablePromptHash"),
		StableToolFingerprint: rapid.StringMatching(`[a-f0-9]{8,16}`).Draw(t, "stableToolFingerprint"),
		GovernanceSnapshot:    rapid.String().Draw(t, "governanceSnapshot"),
		PromptSections:        []string{"system", "tools"},
		LatestCheckpoint:      checkpoint,
		Iterations:            []runtimekernel.IterationState{genIterationState(t, sessionID, turnID, iteration)},
		PendingApprovals:      []runtimekernel.PendingApproval{approval},
		PendingEvidence:       []runtimekernel.PendingEvidence{evidence},
		CompactedSegments:     []runtimekernel.CompactedSegment{segment},
		ExternalReferences:    []runtimekernel.ExternalReference{genExternalReference(t, sessionID, turnID, iteration)},
		FinalOutput:           rapid.String().Draw(t, "finalOutput"),
	}
}

// genSessionState generates a valid SessionState for property testing.
func genSessionState(t *rapid.T) *runtimekernel.SessionState {
	numMessages := rapid.IntRange(0, 10).Draw(t, "numMessages")
	messages := make([]runtimekernel.Message, numMessages)
	for i := range messages {
		messages[i] = genMessage(t)
	}

	now := time.Now().Truncate(time.Second)
	sessionID := rapid.StringMatching(`sess-[a-z0-9]{8,16}`).Draw(t, "sessId")
	sessionType := genSessionType(t)
	mode := genMode(t)
	currentTurn := genTurnSnapshot(t, sessionID, sessionType, mode)
	return &runtimekernel.SessionState{
		ID:                 sessionID,
		Type:               sessionType,
		Mode:               mode,
		HostID:             rapid.String().Draw(t, "hostId"),
		Messages:           messages,
		Context:            genContextWindow(t),
		Activity:           genActivityStats(t),
		CurrentTurn:        currentTurn,
		TurnHistory:        []runtimekernel.TurnSnapshot{*currentTurn},
		PendingApprovals:   []runtimekernel.PendingApproval{genPendingApproval(t, sessionID, currentTurn.ID, currentTurn.Iteration)},
		PendingEvidence:    []runtimekernel.PendingEvidence{genPendingEvidence(t, sessionID, currentTurn.ID, currentTurn.Iteration)},
		LatestCheckpoint:   currentTurn.LatestCheckpoint,
		CompactedSegments:  []runtimekernel.CompactedSegment{genCompactedSegment(t, sessionID, currentTurn.ID, currentTurn.Iteration)},
		ExternalReferences: []runtimekernel.ExternalReference{genExternalReference(t, sessionID, currentTurn.ID, currentTurn.Iteration)},
		CreatedAt:          now.Add(-time.Duration(rapid.IntRange(0, 3600).Draw(t, "createdOffset")) * time.Second),
		UpdatedAt:          now,
	}
}

func TestProperty31_SessionSerializationRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		original := genSessionState(t)

		// Serialize to JSON
		data, err := json.Marshal(original)
		if err != nil {
			t.Fatalf("marshal failed: %v", err)
		}

		// Deserialize from JSON
		var restored runtimekernel.SessionState
		if err := json.Unmarshal(data, &restored); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}

		// Verify equivalence
		if original.ID != restored.ID {
			t.Fatalf("ID mismatch: %q vs %q", original.ID, restored.ID)
		}
		if original.Type != restored.Type {
			t.Fatalf("Type mismatch: %q vs %q", original.Type, restored.Type)
		}
		if original.Mode != restored.Mode {
			t.Fatalf("Mode mismatch: %q vs %q", original.Mode, restored.Mode)
		}
		if original.HostID != restored.HostID {
			t.Fatalf("HostID mismatch: %q vs %q", original.HostID, restored.HostID)
		}
		if len(original.Messages) != len(restored.Messages) {
			t.Fatalf("Messages length mismatch: %d vs %d", len(original.Messages), len(restored.Messages))
		}
		for i, origMsg := range original.Messages {
			resMsg := restored.Messages[i]
			if origMsg.ID != resMsg.ID {
				t.Fatalf("Message[%d].ID mismatch: %q vs %q", i, origMsg.ID, resMsg.ID)
			}
			if origMsg.Role != resMsg.Role {
				t.Fatalf("Message[%d].Role mismatch: %q vs %q", i, origMsg.Role, resMsg.Role)
			}
			if origMsg.Content != resMsg.Content {
				t.Fatalf("Message[%d].Content mismatch", i)
			}
			if len(origMsg.ToolCalls) != len(resMsg.ToolCalls) {
				t.Fatalf("Message[%d].ToolCalls length mismatch: %d vs %d", i, len(origMsg.ToolCalls), len(resMsg.ToolCalls))
			}
			for j, origTC := range origMsg.ToolCalls {
				resTC := resMsg.ToolCalls[j]
				if origTC.ID != resTC.ID || origTC.Name != resTC.Name {
					t.Fatalf("Message[%d].ToolCalls[%d] mismatch", i, j)
				}
				if string(origTC.Arguments) != string(resTC.Arguments) {
					t.Fatalf("Message[%d].ToolCalls[%d].Arguments mismatch", i, j)
				}
			}
			if (origMsg.ToolResult == nil) != (resMsg.ToolResult == nil) {
				t.Fatalf("Message[%d].ToolResult nil mismatch", i)
			}
			if origMsg.ToolResult != nil {
				if origMsg.ToolResult.ToolCallID != resMsg.ToolResult.ToolCallID {
					t.Fatalf("Message[%d].ToolResult.ToolCallID mismatch", i)
				}
				if origMsg.ToolResult.Content != resMsg.ToolResult.Content {
					t.Fatalf("Message[%d].ToolResult.Content mismatch", i)
				}
			}
		}
		// Context window
		if original.Context != restored.Context {
			t.Fatalf("Context mismatch: %+v vs %+v", original.Context, restored.Context)
		}
		// Activity stats
		if original.Activity != restored.Activity {
			t.Fatalf("Activity mismatch: %+v vs %+v", original.Activity, restored.Activity)
		}
		if original.CurrentTurn == nil || restored.CurrentTurn == nil {
			t.Fatalf("CurrentTurn mismatch: original nil=%v restored nil=%v", original.CurrentTurn == nil, restored.CurrentTurn == nil)
		}
		if original.CurrentTurn.ID != restored.CurrentTurn.ID {
			t.Fatalf("CurrentTurn ID mismatch: %q vs %q", original.CurrentTurn.ID, restored.CurrentTurn.ID)
		}
		if len(original.TurnHistory) != len(restored.TurnHistory) {
			t.Fatalf("TurnHistory length mismatch: %d vs %d", len(original.TurnHistory), len(restored.TurnHistory))
		}
		if len(original.PendingApprovals) != len(restored.PendingApprovals) || len(original.PendingEvidence) != len(restored.PendingEvidence) {
			t.Fatalf("pending state mismatch after round trip")
		}
		if original.LatestCheckpoint == nil || restored.LatestCheckpoint == nil {
			t.Fatalf("latest checkpoint mismatch after round trip")
		}
		if len(original.CompactedSegments) != len(restored.CompactedSegments) || len(original.ExternalReferences) != len(restored.ExternalReferences) {
			t.Fatalf("compaction/reference mismatch after round trip")
		}
		// Timestamps (compare Unix to avoid timezone issues)
		if !original.CreatedAt.Equal(restored.CreatedAt) {
			t.Fatalf("CreatedAt mismatch: %v vs %v", original.CreatedAt, restored.CreatedAt)
		}
		if !original.UpdatedAt.Equal(restored.UpdatedAt) {
			t.Fatalf("UpdatedAt mismatch: %v vs %v", original.UpdatedAt, restored.UpdatedAt)
		}
	})
}

// TestProperty31_StoreRoundTrip verifies that saving a session to the store
// and reading it back produces an equivalent object (simulating service restart).
func TestProperty31_StoreRoundTrip(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		original := genSessionState(rt)

		dataDir := t.TempDir()
		store, err := NewJSONFileStore(dataDir, 50*time.Millisecond)
		if err != nil {
			t.Fatalf("create store: %v", err)
		}

		// Save session
		if err := store.SaveSession(original); err != nil {
			t.Fatalf("save session: %v", err)
		}

		// Flush to disk
		if err := store.Flush(); err != nil {
			t.Fatalf("flush: %v", err)
		}
		if err := store.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}

		// Simulate restart: create new store from same directory
		store2, err := NewJSONFileStore(dataDir, 50*time.Millisecond)
		if err != nil {
			t.Fatalf("create store2: %v", err)
		}
		defer store2.Close()

		// Read back
		restored, err := store2.GetSession(original.ID)
		if err != nil {
			t.Fatalf("get session after restart: %v", err)
		}

		// Verify equivalence
		if original.ID != restored.ID {
			t.Fatalf("ID mismatch after restart: %q vs %q", original.ID, restored.ID)
		}
		if original.Type != restored.Type {
			t.Fatalf("Type mismatch after restart: %q vs %q", original.Type, restored.Type)
		}
		if original.Mode != restored.Mode {
			t.Fatalf("Mode mismatch after restart: %q vs %q", original.Mode, restored.Mode)
		}
		if original.HostID != restored.HostID {
			t.Fatalf("HostID mismatch after restart")
		}
		if len(original.Messages) != len(restored.Messages) {
			t.Fatalf("Messages length mismatch after restart: %d vs %d", len(original.Messages), len(restored.Messages))
		}
		if original.Context != restored.Context {
			t.Fatalf("Context mismatch after restart")
		}
		if original.Activity != restored.Activity {
			t.Fatalf("Activity mismatch after restart")
		}
		if !original.CreatedAt.Equal(restored.CreatedAt) {
			t.Fatalf("CreatedAt mismatch after restart")
		}
		if !original.UpdatedAt.Equal(restored.UpdatedAt) {
			t.Fatalf("UpdatedAt mismatch after restart")
		}
	})
}

// TestProperty32_ToolResultSpillRoundTrip verifies that spilled tool results
// survive a flush/restart cycle without losing their externalized payload.
func TestProperty32_ToolResultSpillRoundTrip(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		content := rapid.String().Draw(rt, "spillContent")
		original := &tooling.ResultSpill{
			ID:          rapid.StringMatching(`spill-[a-z0-9]{8,16}`).Draw(rt, "spillId"),
			ContentType: rapid.SampledFrom([]string{"text/plain", "application/json"}).Draw(rt, "spillContentType"),
			Content:     []byte(content),
			Bytes:       int64(len(content)),
			CreatedAt:   time.Now().Truncate(time.Second),
		}

		dataDir := t.TempDir()
		store, err := NewJSONFileStore(dataDir, 50*time.Millisecond)
		if err != nil {
			t.Fatalf("create store: %v", err)
		}

		if err := store.SaveToolResultSpill(original); err != nil {
			t.Fatalf("save spill: %v", err)
		}
		if err := store.Flush(); err != nil {
			t.Fatalf("flush: %v", err)
		}
		if err := store.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}

		store2, err := NewJSONFileStore(dataDir, 50*time.Millisecond)
		if err != nil {
			t.Fatalf("create store2: %v", err)
		}
		defer store2.Close()

		restored, err := store2.GetToolResultSpill(original.ID)
		if err != nil {
			t.Fatalf("get spill after restart: %v", err)
		}

		if original.ID != restored.ID {
			t.Fatalf("ID mismatch after restart: %q vs %q", original.ID, restored.ID)
		}
		if original.ContentType != restored.ContentType {
			t.Fatalf("ContentType mismatch after restart: %q vs %q", original.ContentType, restored.ContentType)
		}
		if string(original.Content) != string(restored.Content) {
			t.Fatalf("Content mismatch after restart")
		}
		if original.Bytes != restored.Bytes {
			t.Fatalf("Bytes mismatch after restart: %d vs %d", original.Bytes, restored.Bytes)
		}
		if !original.CreatedAt.Equal(restored.CreatedAt) {
			t.Fatalf("CreatedAt mismatch after restart: %v vs %v", original.CreatedAt, restored.CreatedAt)
		}
	})
}

func TestToolResultReferencesPersistAcrossStoreRestart(t *testing.T) {
	dataDir := t.TempDir()
	store, err := NewJSONFileStore(dataDir, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	now := time.Now().Truncate(time.Second)
	session := &runtimekernel.SessionState{
		ID:   "sess-refs",
		Type: runtimekernel.SessionTypeHost,
		Mode: runtimekernel.ModeInspect,
		Messages: []runtimekernel.Message{
			{
				ID:        "msg-tool",
				Role:      "tool",
				Content:   "artifacts ready",
				Timestamp: now,
				ToolResult: &runtimekernel.ToolResult{
					ToolCallID: "call-refs",
					Content:    "artifacts ready",
					References: []runtimekernel.ToolResultReference{
						{
							Kind:    runtimekernel.ToolResultReferenceKindCard,
							CardRef: "card-artifacts",
							Title:   "Artifacts card",
						},
						{
							Kind:     runtimekernel.ToolResultReferenceKindFile,
							FilePath: "/tmp/output/report.txt",
							Title:    "Artifact report",
						},
					},
					ExternalReferences: []runtimekernel.ExternalReference{
						{
							ID:        "ref-card",
							SessionID: "sess-refs",
							TurnID:    "turn-refs",
							Iteration: 1,
							Kind:      "card",
							CardRef:   "card-artifacts",
							CreatedAt: now,
						},
						{
							ID:        "ref-file",
							SessionID: "sess-refs",
							TurnID:    "turn-refs",
							Iteration: 1,
							Kind:      "file",
							FilePath:  "/tmp/output/report.txt",
							CreatedAt: now,
						},
					},
				},
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := store.SaveSession(session); err != nil {
		t.Fatalf("save session: %v", err)
	}
	if err := store.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	store2, err := NewJSONFileStore(dataDir, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("create store2: %v", err)
	}
	defer store2.Close()

	restored, err := store2.GetSession("sess-refs")
	if err != nil {
		t.Fatalf("get session after restart: %v", err)
	}
	if len(restored.Messages) != 1 || restored.Messages[0].ToolResult == nil {
		t.Fatalf("restored tool result missing: %+v", restored.Messages)
	}
	if len(restored.Messages[0].ToolResult.References) != 2 {
		t.Fatalf("restored references = %d, want 2", len(restored.Messages[0].ToolResult.References))
	}
	if restored.Messages[0].ToolResult.References[0].CardRef != "card-artifacts" {
		t.Fatalf("restored card ref = %q", restored.Messages[0].ToolResult.References[0].CardRef)
	}
	if restored.Messages[0].ToolResult.References[1].FilePath != "/tmp/output/report.txt" {
		t.Fatalf("restored file path = %q", restored.Messages[0].ToolResult.References[1].FilePath)
	}
	if len(restored.Messages[0].ToolResult.ExternalReferences) != 2 {
		t.Fatalf("restored external references = %d, want 2", len(restored.Messages[0].ToolResult.ExternalReferences))
	}
}

func TestWorkspaceTasksPersistAcrossStoreRestart(t *testing.T) {
	dataDir := t.TempDir()
	store, err := NewJSONFileStore(dataDir, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	now := time.Now().Truncate(time.Second)
	task := &runtimekernel.WorkspaceTask{
		ID:          "task-1",
		SessionID:   "workspace-1",
		TurnID:      "turn-1",
		Type:        "host_exec",
		Status:      "running",
		Description: "check disk",
		HostIDs:     []string{"host-a"},
		StartTime:   now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := store.SaveWorkspaceTask(task); err != nil {
		t.Fatalf("save task: %v", err)
	}
	if err := store.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	store2, err := NewJSONFileStore(dataDir, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("create store2: %v", err)
	}
	defer store2.Close()

	restored, err := store2.GetWorkspaceTask("task-1")
	if err != nil {
		t.Fatalf("get task after restart: %v", err)
	}
	if restored.SessionID != "workspace-1" {
		t.Fatalf("restored SessionID = %q", restored.SessionID)
	}
	if restored.TurnID != "turn-1" {
		t.Fatalf("restored TurnID = %q", restored.TurnID)
	}
	if restored.Status != "running" {
		t.Fatalf("restored Status = %q", restored.Status)
	}
}
