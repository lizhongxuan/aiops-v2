package runtimekernel

import "aiops-v2/internal/tooling"

// ToolResultSpillRepository persists externally spilled tool results.
// The JSON file store is the canonical implementation used by the runtime.
type ToolResultSpillRepository interface {
	GetToolResultSpill(id string) (*tooling.ResultSpill, error)
	ListToolResultSpills() ([]*tooling.ResultSpill, error)
	SaveToolResultSpill(spill *tooling.ResultSpill) error
	DeleteToolResultSpill(id string) error
}
