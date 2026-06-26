package runtimekernel

import "aiops-v2/internal/modeltrace"

func writeModelInputDebugTrace(req RuntimeTraceDebugRequest) (string, error) {
	return modeltrace.Write(buildModelInputTraceRequest(req))
}
