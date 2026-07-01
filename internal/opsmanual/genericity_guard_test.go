package opsmanual

import (
	"os"
	"strings"
	"testing"
)

func TestOpsManualCoreAvoidsMonitorProductHardcode(t *testing.T) {
	paths := []string{
		"operation_frame.go",
		"resource_role_extractor.go",
		"capability_registry.go",
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		lower := strings.ToLower(string(data))
		if strings.Contains(lower, "pg_mon") {
			t.Fatalf("%s contains pg_mon hardcode; monitor components must be runtime resource roles", path)
		}
	}
}
