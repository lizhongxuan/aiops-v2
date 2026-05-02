package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/eval"
)

func main() {
	os.Exit(runCLI(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func runCLI(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	var id string
	var category string
	var input string
	var answerFile string
	var toolCallsFile string
	var turnItemsFile string
	var out string

	flags := flag.NewFlagSet("agent-eval-case", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&id, "id", "", "draft eval case id")
	flags.StringVar(&category, "category", "agent-debug", "draft eval case category")
	flags.StringVar(&input, "input", "", "original user input")
	flags.StringVar(&answerFile, "answer-file", "", "path to answer.txt artifact")
	flags.StringVar(&toolCallsFile, "tool-calls-file", "", "path to tool_calls.json artifact")
	flags.StringVar(&turnItemsFile, "turn-items-file", "", "path to turn_items.json artifact")
	flags.StringVar(&out, "out", "", "output case JSON path")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(out) == "" {
		fmt.Fprintln(stderr, "agent-eval-case: -out is required")
		return 2
	}
	if err := ctx.Err(); err != nil {
		fmt.Fprintln(stderr, "agent-eval-case:", err)
		return 1
	}

	answer, err := readText(answerFile)
	if err != nil {
		return printError(stderr, err)
	}
	toolCalls, err := readToolCalls(toolCallsFile)
	if err != nil {
		return printError(stderr, err)
	}
	turnItems, err := readTurnItems(turnItemsFile)
	if err != nil {
		return printError(stderr, err)
	}
	draft := eval.DraftCaseFromRunOutput(eval.DraftCaseInput{
		ID:       id,
		Category: category,
		Input:    input,
		Output: eval.RunOutput{
			Answer:    answer,
			ToolCalls: toolCalls,
			TurnItems: turnItems,
		},
	})
	if err := writeDraft(out, draft); err != nil {
		return printError(stderr, err)
	}
	fmt.Fprintf(stdout, "draft case: %s\n", out)
	fmt.Fprintf(stdout, "sidecar: %s.draft.md\n", out)
	return 0
}

func readText(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read answer file: %w", err)
	}
	return string(data), nil
}

func readToolCalls(path string) ([]eval.ToolCall, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	var calls []eval.ToolCall
	if err := readJSON(path, &calls); err != nil {
		return nil, fmt.Errorf("read tool calls file: %w", err)
	}
	return calls, nil
}

func readTurnItems(path string) ([]agentstate.TurnItem, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	var items []agentstate.TurnItem
	if err := readJSON(path, &items); err != nil {
		return nil, fmt.Errorf("read turn items file: %w", err)
	}
	return items, nil
}

func readJSON(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return err
	}
	return nil
}

func writeDraft(out string, draft eval.DraftCaseResult) error {
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	data, err := json.MarshalIndent(draft.Case, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal draft case: %w", err)
	}
	if err := os.WriteFile(out, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write draft case: %w", err)
	}
	if err := os.WriteFile(out+".draft.md", []byte(draft.SidecarMarkdown), 0o644); err != nil {
		return fmt.Errorf("write draft sidecar: %w", err)
	}
	return nil
}

func printError(stderr io.Writer, err error) int {
	fmt.Fprintln(stderr, "agent-eval-case:", err)
	return 1
}
