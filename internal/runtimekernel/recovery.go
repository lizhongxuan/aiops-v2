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
	Value      interface{}
	Stack      string
	SessionID  string
	TurnID     string
	RecoveredAt time.Time
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
