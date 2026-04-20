package store

import (
	"encoding/json"
	"testing"
	"time"

	"aiops-v2/internal/runtimekernel"
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

// genSessionState generates a valid SessionState for property testing.
func genSessionState(t *rapid.T) *runtimekernel.SessionState {
	numMessages := rapid.IntRange(0, 10).Draw(t, "numMessages")
	messages := make([]runtimekernel.Message, numMessages)
	for i := range messages {
		messages[i] = genMessage(t)
	}

	now := time.Now().Truncate(time.Second)
	return &runtimekernel.SessionState{
		ID:        rapid.StringMatching(`sess-[a-z0-9]{8,16}`).Draw(t, "sessId"),
		Type:      genSessionType(t),
		Mode:      genMode(t),
		HostID:    rapid.String().Draw(t, "hostId"),
		Messages:  messages,
		Context:   genContextWindow(t),
		Activity:  genActivityStats(t),
		CreatedAt: now.Add(-time.Duration(rapid.IntRange(0, 3600).Draw(t, "createdOffset")) * time.Second),
		UpdatedAt: now,
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
