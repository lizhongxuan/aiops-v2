package runtimekernel

import "aiops-v2/internal/promptinput"

func appendResourceLockTraces(snapshot *TurnSnapshot, traces []promptinput.ResourceLockTrace) {
	if snapshot == nil || len(traces) == 0 {
		return
	}
	iter := latestIteration(snapshot)
	if iter == nil {
		return
	}
	iter.ResourceLocks = append(iter.ResourceLocks, traces...)
}
