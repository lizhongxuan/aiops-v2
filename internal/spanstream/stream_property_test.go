package spanstream

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// Feature: aiops-codex-eino-rewrite, Property 40: SpanTree 结构完整性
// Validates: Requirements 14.1, 14.2
//
// For any turn 执行过程中创建的 Span，每个 Span 应有唯一 ID，parentId 指向有效的父 Span
// 或为空（根节点），不存在孤立 Span 或循环引用
func TestProperty40_SpanTreeStructuralIntegrity(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a root span
		rootID := rapid.StringMatching(`^span-root-[a-z0-9]{4}$`).Draw(t, "rootID")
		root := &Span{
			ID:        rootID,
			Type:      SpanTypeTurn,
			Status:    SpanStatusRunning,
			Name:      "root-turn",
			StartTime: time.Now(),
		}
		tree := NewSpanTree(root)

		// Generate random child spans to add
		numChildren := rapid.IntRange(1, 20).Draw(t, "numChildren")
		spanTypes := []SpanType{SpanTypeToolCall, SpanTypeSearch, SpanTypeFileRead, SpanTypeShellExec, SpanTypeSummary}
		addedIDs := []string{rootID}

		for i := 0; i < numChildren; i++ {
			childID := rapid.StringMatching(`^span-child-[a-z0-9]{6}$`).Draw(t, "childID")
			spanType := spanTypes[rapid.IntRange(0, len(spanTypes)-1).Draw(t, "spanTypeIdx")]
			// Pick a random valid parent from already-added spans
			parentIdx := rapid.IntRange(0, len(addedIDs)-1).Draw(t, "parentIdx")
			parentID := addedIDs[parentIdx]

			child := &Span{
				ID:        childID,
				Type:      spanType,
				Status:    SpanStatusRunning,
				Name:      "child-" + childID,
				StartTime: time.Now(),
			}

			ok := tree.AddChild(parentID, child)
			if !ok {
				t.Fatalf("failed to add child %s under parent %s", childID, parentID)
			}
			addedIDs = append(addedIDs, childID)
		}

		// Verify structural integrity
		allIDs := make(map[string]bool)
		var verifyTree func(span *Span, expectedParent string)
		verifyTree = func(span *Span, expectedParent string) {
			// Unique ID check
			if allIDs[span.ID] {
				t.Fatalf("duplicate span ID: %s", span.ID)
			}
			allIDs[span.ID] = true

			// ParentID validity check
			if expectedParent != "" && span.ParentID != expectedParent {
				t.Fatalf("span %s has parentID %s, expected %s", span.ID, span.ParentID, expectedParent)
			}

			// Recurse children
			for _, child := range span.Children {
				verifyTree(child, span.ID)
			}
		}
		verifyTree(tree.RootSpan, "")

		// All added IDs should be findable
		for _, id := range addedIDs {
			found := tree.FindSpan(id)
			if found == nil {
				t.Fatalf("span %s not found in tree (orphan)", id)
			}
		}

		// No cycles: verify total span count matches
		if len(allIDs) != len(addedIDs) {
			t.Fatalf("expected %d spans, found %d (possible cycle or orphan)", len(addedIDs), len(allIDs))
		}
	})
}

// Feature: aiops-codex-eino-rewrite, Property 41: 类型化事件块顺序保证
// Validates: Requirements 14.3, 14.6
//
// For any MultiplexedStream 发射的事件序列，SpanStart 事件应在对应的 SpanComplete/SpanFail
// 事件之前；Text 事件的顺序应与 LLM 输出顺序一致
func TestProperty41_TypedEventChunkOrdering(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		root := &Span{
			ID:        "root-turn",
			Type:      SpanTypeTurn,
			Status:    SpanStatusRunning,
			Name:      "turn",
			StartTime: time.Now(),
		}
		tree := NewSpanTree(root)
		ms := NewMultiplexedStream(tree, 1024)

		// Generate a sequence of operations
		numSpans := rapid.IntRange(1, 10).Draw(t, "numSpans")
		numTexts := rapid.IntRange(1, 10).Draw(t, "numTexts")

		// Emit text events with sequential content
		for i := 0; i < numTexts; i++ {
			ms.EmitText("text-" + string(rune('A'+i)))
		}

		// Start and complete spans
		spanIDs := make([]string, 0, numSpans)
		for i := 0; i < numSpans; i++ {
			spanType := []SpanType{SpanTypeToolCall, SpanTypeSearch, SpanTypeFileRead}[i%3]
			spanID := ms.StartSpan("root-turn", spanType, "op-"+string(rune('0'+i)))
			spanIDs = append(spanIDs, spanID)
		}

		// Complete or fail spans
		for i, spanID := range spanIDs {
			if i%3 == 2 {
				ms.FailSpan(spanID, nil)
			} else {
				ms.CompleteSpan(spanID, "done", "detail")
			}
		}

		// Drain all events and verify ordering
		ms.Close()

		var events []TypedEventChunk
		for chunk := range ms.Chunks() {
			events = append(events, chunk)
		}

		// Verify: SpanStart before SpanComplete for each span
		spanStartSeen := make(map[string]int)  // spanID → index of start event
		spanEndSeen := make(map[string]int)    // spanID → index of complete event

		for idx, ev := range events {
			if ev.Type == ChunkTypeSpanStart {
				spanStartSeen[ev.SpanID] = idx
			}
			if ev.Type == ChunkTypeSpanComplete {
				spanEndSeen[ev.SpanID] = idx
			}
		}

		for spanID, endIdx := range spanEndSeen {
			startIdx, hasStart := spanStartSeen[spanID]
			if !hasStart {
				t.Fatalf("SpanComplete for %s without preceding SpanStart", spanID)
			}
			if startIdx >= endIdx {
				t.Fatalf("SpanStart (idx=%d) not before SpanComplete (idx=%d) for span %s",
					startIdx, endIdx, spanID)
			}
		}

		// Verify: Text events maintain order
		var textContents []string
		for _, ev := range events {
			if ev.Type == ChunkTypeText {
				var text string
				json.Unmarshal(ev.Data, &text)
				textContents = append(textContents, text)
			}
		}
		for i := 1; i < len(textContents); i++ {
			if textContents[i-1] >= textContents[i] {
				t.Fatalf("text events out of order: %q >= %q", textContents[i-1], textContents[i])
			}
		}
	})
}

// Feature: aiops-codex-eino-rewrite, Property 42: 异步总结不阻塞主流程
// Validates: Requirements 14.4
//
// For any 触发异步总结的场景，ContextCompressor.CompressAsync 不应阻塞 RuntimeKernel
// 的主 ReAct loop，总结完成后通过 channel 异步回传
func TestProperty42_AsyncSummaryNonBlocking(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		delay := time.Duration(rapid.IntRange(50, 200).Draw(t, "delayMs")) * time.Millisecond
		numCalls := rapid.IntRange(1, 8).Draw(t, "numCalls")

		mock := &mockSummaryModel{response: "compressed summary", delay: delay}
		cc := NewContextCompressor(mock, 4)

		span := &Span{
			ID:   "span-async",
			Type: SpanTypeToolCall,
			Name: "async-test",
		}
		messages := []Message{{Role: "tool", Content: "verbose output data"}}

		// All CompressAsync calls should return immediately (non-blocking)
		channels := make([]<-chan string, numCalls)
		start := time.Now()
		for i := 0; i < numCalls; i++ {
			channels[i] = cc.CompressAsync(context.Background(), span, messages)
		}
		callDuration := time.Since(start)

		// The calls themselves should complete in well under the model delay
		// (they just launch goroutines)
		maxAllowedCallTime := 20 * time.Millisecond
		if callDuration > maxAllowedCallTime {
			t.Fatalf("CompressAsync blocked for %v (max allowed %v), should be non-blocking",
				callDuration, maxAllowedCallTime)
		}

		// All results should eventually arrive
		for i, ch := range channels {
			select {
			case result := <-ch:
				if result != "compressed summary" {
					t.Fatalf("call %d: unexpected result %q", i, result)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("call %d: timeout waiting for async result", i)
			}
		}
	})
}

// Feature: aiops-codex-eino-rewrite, Property 43: Context 压缩后 Token 减少
// Validates: Requirements 14.5
//
// For any 经过 Summary 替换的上下文，替换后的 Token 数应严格小于替换前的 Token 数，
// 且 Summary 保留了原始内容的关键信息
func TestProperty43_ContextCompressionReducesTokens(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate verbose original content (simulating tool output)
		numLines := rapid.IntRange(5, 50).Draw(t, "numLines")
		var originalContent string
		for i := 0; i < numLines; i++ {
			line := rapid.StringMatching(`^[a-zA-Z0-9 ]{20,80}$`).Draw(t, "line")
			originalContent += line + "\n"
		}

		// The summary model returns a short summary (simulating real compression)
		summaryText := rapid.StringMatching(`^[A-Z][a-z ]{10,40}$`).Draw(t, "summary")

		mock := &mockSummaryModel{response: summaryText}
		cc := NewContextCompressor(mock, 2)

		span := &Span{
			ID:   "span-compress",
			Type: SpanTypeToolCall,
			Name: "compress-test",
		}
		messages := []Message{{Role: "tool", Content: originalContent}}

		ch := cc.CompressAsync(context.Background(), span, messages)
		select {
		case summary := <-ch:
			// Token approximation: count words (rough proxy for tokens)
			originalTokens := approximateTokenCount(originalContent)
			summaryTokens := approximateTokenCount(summary)

			if summary == "" {
				t.Fatal("expected non-empty summary")
			}

			// Summary should be strictly shorter than original
			if summaryTokens >= originalTokens {
				t.Fatalf("summary tokens (%d) should be less than original tokens (%d)",
					summaryTokens, originalTokens)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for compression result")
		}
	})
}

// approximateTokenCount provides a rough token count by splitting on whitespace.
func approximateTokenCount(s string) int {
	count := 0
	inWord := false
	for _, r := range s {
		if r == ' ' || r == '\n' || r == '\t' {
			if inWord {
				count++
				inWord = false
			}
		} else {
			inWord = true
		}
	}
	if inWord {
		count++
	}
	return count
}

// Feature: aiops-codex-eino-rewrite, Property 44: SpanTree 序列化 Round-Trip
// Validates: Requirements 14.8
//
// For any 有效的 SpanTree，序列化为 JSON 后再反序列化应产生等价的树结构，
// 所有 Span 的 ID、parentId、children 关系保持不变
func TestProperty44_SpanTreeSerializationRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Build a random SpanTree
		rootID := rapid.StringMatching(`^root-[a-z0-9]{4}$`).Draw(t, "rootID")
		rootName := rapid.StringMatching(`^turn-[a-z]{3,8}$`).Draw(t, "rootName")
		now := time.Now().Truncate(time.Millisecond) // Truncate for JSON precision

		root := &Span{
			ID:        rootID,
			Type:      SpanTypeTurn,
			Status:    SpanStatusRunning,
			Name:      rootName,
			StartTime: now,
		}
		tree := NewSpanTree(root)

		// Add random children
		numChildren := rapid.IntRange(0, 15).Draw(t, "numChildren")
		spanTypes := []SpanType{SpanTypeToolCall, SpanTypeSearch, SpanTypeFileRead, SpanTypeShellExec, SpanTypeSummary}
		statuses := []SpanStatus{SpanStatusRunning, SpanStatusCompleted, SpanStatusFailed}
		addedIDs := []string{rootID}

		for i := 0; i < numChildren; i++ {
			childID := rapid.StringMatching(`^child-[a-z0-9]{6}$`).Draw(t, "childID")
			spanType := spanTypes[rapid.IntRange(0, len(spanTypes)-1).Draw(t, "typeIdx")]
			status := statuses[rapid.IntRange(0, len(statuses)-1).Draw(t, "statusIdx")]
			parentIdx := rapid.IntRange(0, len(addedIDs)-1).Draw(t, "parentIdx")
			parentID := addedIDs[parentIdx]
			childName := rapid.StringMatching(`^op-[a-z]{2,6}$`).Draw(t, "childName")

			child := &Span{
				ID:        childID,
				Type:      spanType,
				Status:    status,
				Name:      childName,
				StartTime: now.Add(time.Duration(i) * time.Second),
			}

			// Optionally set summary and detail for completed spans
			if status == SpanStatusCompleted {
				child.Summary = rapid.StringMatching(`^[a-z ]{5,20}$`).Draw(t, "summary")
				child.Detail = rapid.StringMatching(`^[a-z ]{10,40}$`).Draw(t, "detail")
				end := now.Add(time.Duration(i+1) * time.Second)
				child.EndTime = &end
			}

			tree.AddChild(parentID, child)
			addedIDs = append(addedIDs, childID)
		}

		// Serialize
		data, err := json.Marshal(tree)
		if err != nil {
			t.Fatalf("failed to marshal SpanTree: %v", err)
		}

		// Deserialize
		var restored SpanTree
		err = json.Unmarshal(data, &restored)
		if err != nil {
			t.Fatalf("failed to unmarshal SpanTree: %v", err)
		}

		// Verify structural equivalence
		verifySpanEquivalence(t, tree.RootSpan, restored.RootSpan)
	})
}

// verifySpanEquivalence recursively checks that two span trees are equivalent.
func verifySpanEquivalence(t *rapid.T, original, restored *Span) {
	if original == nil && restored == nil {
		return
	}
	if original == nil || restored == nil {
		t.Fatalf("span mismatch: original=%v, restored=%v", original, restored)
	}

	if original.ID != restored.ID {
		t.Fatalf("ID mismatch: %s vs %s", original.ID, restored.ID)
	}
	if original.ParentID != restored.ParentID {
		t.Fatalf("ParentID mismatch for %s: %s vs %s", original.ID, original.ParentID, restored.ParentID)
	}
	if original.Type != restored.Type {
		t.Fatalf("Type mismatch for %s: %s vs %s", original.ID, original.Type, restored.Type)
	}
	if original.Status != restored.Status {
		t.Fatalf("Status mismatch for %s: %s vs %s", original.ID, original.Status, restored.Status)
	}
	if original.Name != restored.Name {
		t.Fatalf("Name mismatch for %s: %s vs %s", original.ID, original.Name, restored.Name)
	}
	if original.Summary != restored.Summary {
		t.Fatalf("Summary mismatch for %s: %q vs %q", original.ID, original.Summary, restored.Summary)
	}
	if original.Detail != restored.Detail {
		t.Fatalf("Detail mismatch for %s: %q vs %q", original.ID, original.Detail, restored.Detail)
	}

	if len(original.Children) != len(restored.Children) {
		t.Fatalf("Children count mismatch for %s: %d vs %d",
			original.ID, len(original.Children), len(restored.Children))
	}

	for i := range original.Children {
		verifySpanEquivalence(t, original.Children[i], restored.Children[i])
	}
}


