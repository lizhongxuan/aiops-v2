package engine

import (
	"runner/modules"
	"runner/modules/cmd"
	"runner/modules/script"
	"runner/modules/shell"
	"runner/modules/wait"
)

// DefaultRegistry returns a registry populated with built-in modules.
func DefaultRegistry() *modules.Registry {
	reg := modules.NewRegistry()
	reg.Register("cmd.run", cmd.New())
	reg.Register("shell.run", shell.New())
	reg.Register("script.shell", script.New("shell"))
	reg.Register("script.python", script.New("python"))
	reg.Register("wait.event", wait.NewEvent())
	return reg
}
