package runtimekernel

import "aiops-v2/internal/modeltrace"

func writeLegacyTraceForTest(req RuntimeTraceDebugRequest) (string, error) {
	return modeltrace.Write(buildModelInputTraceRequest(req))
}
