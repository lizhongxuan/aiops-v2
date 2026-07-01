package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"aiops-v2/internal/agentstate"
)

func main() {
	if err := runCLI(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		log.Fatalf("aiops-migrate-assistant-message: %v", err)
	}
}

func runCLI(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if stdin == nil {
		stdin = bytes.NewReader(nil)
	}
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	fs := flag.NewFlagSet("aiops-migrate-assistant-message", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var finalOutput string
	fs.StringVar(&finalOutput, "final-output", "", "optional final output used when historical final item is absent")
	if err := fs.Parse(args); err != nil {
		return err
	}

	input, err := io.ReadAll(stdin)
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}
	items, embeddedFinalOutput, err := decodeInput(input)
	if err != nil {
		return err
	}
	if finalOutput == "" {
		finalOutput = embeddedFinalOutput
	}

	migrated := agentstate.MigrateLegacyAssistantItemsToAssistantMessage(items, finalOutput)
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(migrated)
}

func decodeInput(input []byte) ([]agentstate.TurnItem, string, error) {
	input = bytes.TrimSpace(input)
	if len(input) == 0 {
		return nil, "", fmt.Errorf("input JSON is required")
	}
	var items []agentstate.TurnItem
	if err := json.Unmarshal(input, &items); err == nil {
		return items, "", nil
	}
	var envelope struct {
		Items       []agentstate.TurnItem `json:"items"`
		AgentItems  []agentstate.TurnItem `json:"agentItems"`
		FinalOutput string                `json:"finalOutput"`
	}
	if err := json.Unmarshal(input, &envelope); err != nil {
		return nil, "", fmt.Errorf("decode input as turn item array or object: %w", err)
	}
	items = envelope.Items
	if len(items) == 0 {
		items = envelope.AgentItems
	}
	return items, envelope.FinalOutput, nil
}
