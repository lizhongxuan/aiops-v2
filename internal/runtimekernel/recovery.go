package runtimekernel

import (
	"fmt"
	"runtime/debug"
	"time"
)

// ---------------------------------------------------------------------------
// Panic recovery and error handling for RuntimeKernel.
// Ensures panics in tool execution or agent loops do not crash the process.
// Sessions remain recoverable after a panic.
// ---------------------------------------------------------------------------

// PanicError represents a recovered panic with stack trace information.
type PanicError struct {
	Value       interface{}
	Stack       string
	SessionID   string
	TurnID      string
	RecoveredAt time.Time
}

// TurnRecoveryState describes whether a suspended/resumable turn can be resumed.
type TurnRecoveryState struct {
	SessionID        string
	TurnID           string
	Lifecycle        TurnLifecycleState
	ResumeState      TurnResumeState
	PendingApprovals int
	PendingEvidence  int
	HasCheckpoint    bool
	HasIterations    bool
	Resumable        bool
	Reasons          []string
}

// RuntimeRecoverySnapshot captures startup-time recovery state loaded from persistence.
type RuntimeRecoverySnapshot struct {
	LatestSession    *SessionState
	WorkspaceTasks   int
	ReconcileSummary *ReconcileSummary
}

// Validate checks the structural integrity of the recovery state.
func (s TurnRecoveryState) Validate() error {
	if s.SessionID == "" {
		return fmt.Errorf("session id is required")
	}
	if s.TurnID == "" {
		return fmt.Errorf("turn id is required")
	}
	if !s.Lifecycle.IsValid() {
		return fmt.Errorf("invalid lifecycle %q", s.Lifecycle)
	}
	if !s.ResumeState.IsValid() {
		return fmt.Errorf("invalid resume state %q", s.ResumeState)
	}
	if s.PendingApprovals < 0 {
		return fmt.Errorf("pending approvals must be >= 0")
	}
	if s.PendingEvidence < 0 {
		return fmt.Errorf("pending evidence must be >= 0")
	}
	return nil
}

// InspectTurnRecovery derives the resumability of a turn snapshot.
func InspectTurnRecovery(snapshot *TurnSnapshot) TurnRecoveryState {
	state := TurnRecoveryState{}
	if snapshot == nil {
		state.Reasons = append(state.Reasons, "snapshot is nil")
		return state
	}

	state.SessionID = snapshot.SessionID
	state.TurnID = snapshot.ID
	state.Lifecycle = snapshot.Lifecycle
	state.ResumeState = snapshot.ResumeState
	state.PendingApprovals = len(snapshot.PendingApprovals)
	state.PendingEvidence = len(snapshot.PendingEvidence)
	state.HasCheckpoint = snapshot.LatestCheckpoint != nil
	state.HasIterations = len(snapshot.Iterations) > 0

	state.Resumable = snapshot.Lifecycle.CanResume()
	switch snapshot.ResumeState {
	case TurnResumeStatePendingApproval:
		if state.PendingApprovals == 0 {
			state.Reasons = append(state.Reasons, "resume state requires pending approvals")
		}
	case TurnResumeStatePendingEvidence:
		if state.PendingEvidence == 0 {
			state.Reasons = append(state.Reasons, "resume state requires pending evidence")
		}
	case TurnResumeStateCheckpointReady, TurnResumeStateResumable:
		if !state.HasCheckpoint && !state.HasIterations {
			state.Reasons = append(state.Reasons, "resume state requires a checkpoint or iteration history")
		}
	}
	if !snapshot.Lifecycle.CanResume() {
		state.Resumable = false
		state.Reasons = append(state.Reasons, fmt.Sprintf("lifecycle %q is not resumable", snapshot.Lifecycle))
	}
	if !state.HasCheckpoint && !state.HasIterations {
		state.Reasons = append(state.Reasons, "turn has no checkpoint or iteration history")
	}
	return state
}

// ValidateTurnRecoveryPreconditions checks whether a snapshot can be resumed.
func ValidateTurnRecoveryPreconditions(snapshot *TurnSnapshot) error {
	state := InspectTurnRecovery(snapshot)
	if err := state.Validate(); err != nil {
		return err
	}
	if !state.Resumable {
		return fmt.Errorf("turn %s is not resumable", state.TurnID)
	}
	if len(state.Reasons) > 0 {
		return fmt.Errorf("turn %s cannot resume: %v", state.TurnID, state.Reasons)
	}
	return nil
}

// RecoverTurnFromSnapshot validates the snapshot before wrapping the turn.
func RecoverTurnFromSnapshot(snapshot *TurnSnapshot, fn func() (TurnResult, error)) (TurnResult, error) {
	MarkRunningMutatingInvocationsSideEffectUnknown(snapshot, time.Now())
	if err := ValidateTurnRecoveryPreconditions(snapshot); err != nil {
		return TurnResult{}, err
	}
	return RecoverTurn(snapshot.SessionID, snapshot.ID, snapshot.SessionType, snapshot.Mode, fn)
}

// Error implements the error interface.
func (e *PanicError) Error() string {
	return fmt.Sprintf("panic recovered in session %s turn %s: %v", e.SessionID, e.TurnID, e.Value)
}

// RecoverTurn wraps a turn execution function with panic recovery.
// If the function panics, it returns a TurnResult with error status
// and the session remains in a recoverable state.
func RecoverTurn(sessionID, turnID string, sessionType SessionType, mode Mode, fn func() (TurnResult, error)) (result TurnResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			stack := string(debug.Stack())
			panicErr := &PanicError{
				Value:       r,
				Stack:       stack,
				SessionID:   sessionID,
				TurnID:      turnID,
				RecoveredAt: time.Now(),
			}
			result = TurnResult{
				SessionType: sessionType,
				Mode:        mode,
				SessionID:   sessionID,
				TurnID:      turnID,
				Status:      "error",
				Error:       panicErr.Error(),
			}
			err = nil // Don't propagate panic as error — return structured result
		}
	}()

	return fn()
}

// RecoverToolExec wraps a tool execution with panic recovery.
// Returns an error string if a panic occurred, empty string otherwise.
func RecoverToolExec(toolName string, fn func() error) (panicMsg string) {
	defer func() {
		if r := recover(); r != nil {
			panicMsg = fmt.Sprintf("tool %q panicked: %v", toolName, r)
		}
	}()

	if err := fn(); err != nil {
		return err.Error()
	}
	return ""
}

// SafeExecute runs a function with panic recovery, returning any error
// including recovered panics as a standard error.
func SafeExecute(fn func() error) error {
	var result error
	func() {
		defer func() {
			if r := recover(); r != nil {
				result = fmt.Errorf("panic: %v", r)
			}
		}()
		result = fn()
	}()
	return result
}

// RestoreRuntimeState loads the latest persisted session into the shared
// SessionManager cache and reconciles persisted workspace task state.
func RestoreRuntimeState(sm *SessionManager, tm *TaskManager, bc *BudgetController) (*RuntimeRecoverySnapshot, error) {
	if sm == nil {
		return nil, fmt.Errorf("SessionManager is nil")
	}
	if tm == nil {
		return nil, fmt.Errorf("TaskManager is nil")
	}
	if bc == nil {
		return nil, fmt.Errorf("BudgetController is nil")
	}

	latest := sm.GetLatest()
	tasks := tm.ListTasks()
	summary, err := Reconcile(tm, bc)
	if err != nil {
		return nil, err
	}

	return &RuntimeRecoverySnapshot{
		LatestSession:    latest,
		WorkspaceTasks:   len(tasks),
		ReconcileSummary: summary,
	}, nil
}
