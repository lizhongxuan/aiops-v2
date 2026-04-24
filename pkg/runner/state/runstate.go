package state

import "time"

func CloneRunState(input RunState) RunState {
	out := input
	out.Args = cloneMap(input.Args)
	if len(input.Resources) > 0 {
		out.Resources = make(map[string]ResourceState, len(input.Resources))
		for k, v := range input.Resources {
			out.Resources[k] = cloneResource(v)
		}
	}
	if len(input.Steps) > 0 {
		out.Steps = make([]StepState, 0, len(input.Steps))
		for _, step := range input.Steps {
			out.Steps = append(out.Steps, cloneStep(step))
		}
	}
	return out
}

func (r *RunState) UpsertStepStart(stepName string, now time.Time) {
	step := r.ensureStep(stepName)
	if step.StartedAt.IsZero() {
		step.StartedAt = now
	}
	step.Status = RunStatusRunning
}

func (r *RunState) UpsertStepFinish(stepName, status, message string, now time.Time) {
	step := r.ensureStep(stepName)
	if step.StartedAt.IsZero() {
		step.StartedAt = now
	}
	step.Status = status
	step.Message = message
	step.FinishedAt = now
}

func (r *RunState) UpsertHostResult(stepName string, host HostResult) {
	step := r.ensureStep(stepName)
	if step.Hosts == nil {
		step.Hosts = map[string]HostResult{}
	}
	step.Hosts[host.Host] = cloneHost(host)
}

func (r *RunState) ensureStep(name string) *StepState {
	for i := range r.Steps {
		if r.Steps[i].Name == name {
			return &r.Steps[i]
		}
	}
	r.Steps = append(r.Steps, StepState{
		Name:  name,
		Hosts: map[string]HostResult{},
	})
	return &r.Steps[len(r.Steps)-1]
}

func cloneResource(input ResourceState) ResourceState {
	out := input
	out.Current = cloneMap(input.Current)
	out.Desired = cloneMap(input.Desired)
	out.Diff = cloneMap(input.Diff)
	return out
}

func cloneStep(input StepState) StepState {
	out := input
	if len(input.Hosts) > 0 {
		out.Hosts = make(map[string]HostResult, len(input.Hosts))
		for host, res := range input.Hosts {
			out.Hosts[host] = cloneHost(res)
		}
	}
	return out
}

func cloneHost(input HostResult) HostResult {
	out := input
	out.Output = cloneMap(input.Output)
	return out
}

func cloneMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}
