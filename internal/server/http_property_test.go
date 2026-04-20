package server

import (
	"testing"

	"pgregory.net/rapid"
)

// Feature: aiops-codex-eino-rewrite, Property 18: Chat Message → Eino 格式转换
//
// *For any* 前端发送的 /api/v1/chat/message 请求，转换为 Eino Message 格式后应保留 role、content 和 metadata
//
// **Validates: Requirements 6.4**

func TestProperty18_ChatMessageToEinoConversion(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate arbitrary chat message request
		role := rapid.SampledFrom([]string{"user", "assistant", "system", "tool", ""}).Draw(t, "role")
		content := rapid.String().Draw(t, "content")

		// Generate arbitrary metadata map
		metaKeys := rapid.SliceOfN(rapid.StringMatching(`[a-z]{1,10}`), 0, 5).Draw(t, "metaKeys")
		metadata := make(map[string]string)
		for _, k := range metaKeys {
			metadata[k] = rapid.String().Draw(t, "metaVal_"+k)
		}

		req := ChatMessageRequest{
			Role:     role,
			Content:  content,
			Metadata: metadata,
		}

		// Convert to Eino message
		einoMsg := ConvertChatToEinoMessage(req)

		// Property: role is preserved (defaults to "user" if empty)
		expectedRole := role
		if expectedRole == "" {
			expectedRole = "user"
		}
		if einoMsg.Role != expectedRole {
			t.Fatalf("role not preserved: got %q, want %q", einoMsg.Role, expectedRole)
		}

		// Property: content is preserved exactly
		if einoMsg.Content != content {
			t.Fatalf("content not preserved: got %q, want %q", einoMsg.Content, content)
		}

		// Property: metadata is preserved (all keys and values match)
		if len(metadata) == 0 && len(einoMsg.Metadata) == 0 {
			// Both nil/empty — ok
		} else {
			if len(einoMsg.Metadata) != len(metadata) {
				t.Fatalf("metadata length mismatch: got %d, want %d", len(einoMsg.Metadata), len(metadata))
			}
			for k, v := range metadata {
				if einoMsg.Metadata[k] != v {
					t.Fatalf("metadata[%q] not preserved: got %q, want %q", k, einoMsg.Metadata[k], v)
				}
			}
		}
	})
}
