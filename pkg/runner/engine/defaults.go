package engine

import (
	"runner/modules"
	"runner/modules/script"
	"runner/modules/wait"
)

// DefaultRegistry returns a registry populated with built-in modules.
func DefaultRegistry() *modules.Registry {
	reg := modules.NewRegistry()
	reg.Register("script.shell", script.New("shell"))
	reg.Register("script.python", script.New("python"))
	reg.Register("wait.event", wait.NewEvent())
	return reg
}
