package runtimekernel

// unclassifiedAssistantMessageData marks provider text whose response shape
// and terminal boundary are not known yet. It is persisted for recovery and
// trace purposes, but presentation layers must not expose it as commentary or
// a final answer.
func unclassifiedAssistantMessageData(data assistantMessageData, state AssistantMessageStreamState) assistantMessageData {
	data.Phase = AssistantMessagePhaseUnclassified
	data.StreamState = state
	return data
}
