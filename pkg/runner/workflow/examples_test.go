package workflow

import (
	"path/filepath"
	"testing"
)

func TestExampleYAMLsLoadAndValidate(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "examples", "*.yaml"))
	if err != nil {
		t.Fatalf("glob example yamls: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("expected at least one top-level example yaml")
	}

	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			wf, err := LoadFile(file)
			if err != nil {
				t.Fatalf("load example: %v", err)
			}
			if err := wf.Validate(); err != nil {
				t.Fatalf("validate example: %v", err)
			}
		})
	}
}
