package main

import (
	"context"
	"fmt"
	"os"

	"runner/engine"
	"runner/workflow"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: runner-simple <yaml-file>")
		os.Exit(2)
	}

	yamlPath := os.Args[1]
	wf, err := workflow.LoadFile(yamlPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load workflow: %v\n", err)
		os.Exit(1)
	}
	if err := wf.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "validate workflow: %v\n", err)
		os.Exit(1)
	}

	eng := engine.New()

	if err := eng.Apply(context.Background(), wf); err != nil {
		fmt.Fprintf(os.Stderr, "apply workflow: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("workflow applied")
}
