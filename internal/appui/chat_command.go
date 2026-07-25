package appui

// ChatCommand is the transport-independent input for one user turn.
// WorkflowContext is untrusted page state and must be frozen against the
// authenticated request context before it can enter RuntimeKernel.
type ChatCommand struct {
	SessionID       string
	SessionType     string
	Mode            string
	Content         string
	Role            string
	HostID          string
	ClientMessageID string
	ClientTurnID    string
	Metadata        map[string]string
	WorkflowContext *WorkflowTurnContextRequest
}
