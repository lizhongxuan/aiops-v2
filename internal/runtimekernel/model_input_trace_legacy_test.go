package runtimekernel

import (
	"testing"

	"aiops-v2/internal/modeltrace"
)

var legacyTraceConfigForTest = modeltrace.Config{Enabled: true, RootDir: modeltrace.DefaultRootDir("")}

func setLegacyTraceRootForTest(t *testing.T, root string) {
	t.Helper()
	previous := legacyTraceConfigForTest
	legacyTraceConfigForTest = modeltrace.Config{Enabled: true, RootDir: root}
	t.Cleanup(func() {
		legacyTraceConfigForTest = previous
	})
}

func setLegacyTraceDisabledForTest(t *testing.T) {
	t.Helper()
	previous := legacyTraceConfigForTest
	legacyTraceConfigForTest = modeltrace.Config{Enabled: false, RootDir: t.TempDir()}
	t.Cleanup(func() {
		legacyTraceConfigForTest = previous
	})
}

func writeLegacyTraceForTest(req RuntimeTraceDebugRequest) (string, error) {
	return modeltrace.WriteWithConfig(legacyTraceConfigForTest, buildModelInputTraceRequest(req))
}

func runtimeDebugConfigForLegacyTraceTest() RuntimeDebugConfig {
	return RuntimeDebugConfig{
		ModelInputTrace:     legacyTraceConfigForTest.Enabled,
		ModelInputTraceRoot: legacyTraceConfigForTest.RootDir,
	}
}
