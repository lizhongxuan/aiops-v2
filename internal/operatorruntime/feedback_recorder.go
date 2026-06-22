package operatorruntime

import "time"

type FeedbackRecorder struct{}

func NewFeedbackRecorder() FeedbackRecorder {
	return FeedbackRecorder{}
}

func (FeedbackRecorder) Record(run GuardRun) GuardFeedback {
	feedback := GuardFeedback{
		GuardRunID:    run.ID,
		Result:        run.State,
		ProblemTypeID: run.ProblemTypeID,
		ActionRef:     run.ActionRef,
		CreatedAt:     time.Now().UTC(),
	}
	if run.WorkflowRun != nil {
		feedback.WorkflowStatus = run.WorkflowRun.Status
		feedback.WorkflowError = redactValue(run.WorkflowRun.Error)
	}
	if run.Recovery != nil {
		feedback.Recovered = run.Recovery.Recovered
		feedback.RecoveryReason = run.Recovery.Reason
	}
	return feedback
}
