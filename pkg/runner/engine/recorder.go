package engine

import (
	"context"

	"runner/scheduler"
	"runner/workflow"
)

type Recorder interface {
	StepStart(step workflow.Step, targets []workflow.HostSpec)
	StepFinish(step workflow.Step, status string)
	HostResult(step workflow.Step, host workflow.HostSpec, result scheduler.Result)
}

type recorderKey struct{}

func WithRecorder(ctx context.Context, recorder Recorder) context.Context {
	if recorder == nil {
		return ctx
	}
	return context.WithValue(ctx, recorderKey{}, recorder)
}

func recorderFromContext(ctx context.Context) Recorder {
	if ctx == nil {
		return nil
	}
	recorder, _ := ctx.Value(recorderKey{}).(Recorder)
	return recorder
}

type multiRecorder struct {
	recorders []Recorder
}

func MultiRecorder(recorders ...Recorder) Recorder {
	filtered := make([]Recorder, 0, len(recorders))
	for _, recorder := range recorders {
		if recorder == nil {
			continue
		}
		filtered = append(filtered, recorder)
	}
	if len(filtered) == 0 {
		return nil
	}
	if len(filtered) == 1 {
		return filtered[0]
	}
	return &multiRecorder{recorders: filtered}
}

func (r *multiRecorder) StepStart(step workflow.Step, targets []workflow.HostSpec) {
	for _, recorder := range r.recorders {
		recorder.StepStart(step, targets)
	}
}

func (r *multiRecorder) StepFinish(step workflow.Step, status string) {
	for _, recorder := range r.recorders {
		recorder.StepFinish(step, status)
	}
}

func (r *multiRecorder) HostResult(step workflow.Step, host workflow.HostSpec, result scheduler.Result) {
	for _, recorder := range r.recorders {
		recorder.HostResult(step, host, result)
	}
}

func (r *multiRecorder) GraphNodeStart(nodeID string) {
	for _, recorder := range r.recorders {
		if graphRecorder, ok := recorder.(interface{ GraphNodeStart(string) }); ok {
			graphRecorder.GraphNodeStart(nodeID)
		}
	}
}

func (r *multiRecorder) GraphNodeFinish(nodeID, status, message string) {
	for _, recorder := range r.recorders {
		if graphRecorder, ok := recorder.(interface{ GraphNodeFinish(string, string, string) }); ok {
			graphRecorder.GraphNodeFinish(nodeID, status, message)
		}
	}
}

func (r *multiRecorder) GraphEdgeSelected(edge workflow.GraphEdgeSpec) {
	for _, recorder := range r.recorders {
		if graphRecorder, ok := recorder.(interface{ GraphEdgeSelected(workflow.GraphEdgeSpec) }); ok {
			graphRecorder.GraphEdgeSelected(edge)
		}
	}
}

func (r *multiRecorder) GraphApprovalWaiting(nodeID string) {
	for _, recorder := range r.recorders {
		if graphRecorder, ok := recorder.(interface{ GraphApprovalWaiting(string) }); ok {
			graphRecorder.GraphApprovalWaiting(nodeID)
		}
	}
}

func (r *multiRecorder) GraphApprovalResolved(nodeID, status, message string) {
	for _, recorder := range r.recorders {
		if graphRecorder, ok := recorder.(interface{ GraphApprovalResolved(string, string, string) }); ok {
			graphRecorder.GraphApprovalResolved(nodeID, status, message)
		}
	}
}

func (r *multiRecorder) GraphNodeIterationStart(nodeID string, iteration int, item any) {
	for _, recorder := range r.recorders {
		if graphRecorder, ok := recorder.(interface {
			GraphNodeIterationStart(string, int, any)
		}); ok {
			graphRecorder.GraphNodeIterationStart(nodeID, iteration, item)
		}
	}
}

func (r *multiRecorder) GraphNodeIterationFinish(nodeID string, iteration int, status, message string) {
	for _, recorder := range r.recorders {
		if graphRecorder, ok := recorder.(interface {
			GraphNodeIterationFinish(string, int, string, string)
		}); ok {
			graphRecorder.GraphNodeIterationFinish(nodeID, iteration, status, message)
		}
	}
}
