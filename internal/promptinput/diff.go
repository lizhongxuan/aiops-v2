package promptinput

// TraceDiff captures semantic prompt-input items added or removed between two
// model-input traces.
type TraceDiff struct {
	Added   []TraceItem `json:"added,omitempty"`
	Removed []TraceItem `json:"removed,omitempty"`
}

// DiffTrace computes a compact semantic diff between two prompt-input traces.
func DiffTrace(prev, next PromptInputTrace) TraceDiff {
	prevSet := traceItemSet(prev.Items)
	nextSet := traceItemSet(next.Items)

	diff := TraceDiff{}
	for key, item := range nextSet {
		if _, ok := prevSet[key]; !ok {
			diff.Added = append(diff.Added, item)
		}
	}
	for key, item := range prevSet {
		if _, ok := nextSet[key]; !ok {
			diff.Removed = append(diff.Removed, item)
		}
	}
	return diff
}

func traceItemSet(items []TraceItem) map[string]TraceItem {
	out := make(map[string]TraceItem, len(items))
	for _, item := range items {
		out[traceItemKey(item)] = item
	}
	return out
}

func traceItemKey(item TraceItem) string {
	return item.Source + "\x00" + item.SemanticRole + "\x00" + item.ProviderRole + "\x00" + item.PromptLayer + "\x00" + item.ID + "\x00" + item.Status + "\x00" + item.Content
}
