package appui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"aiops-v2/internal/terminalpolicy"
)

func TestTerminalPolicyServiceSerializesConcurrentUpdates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "terminal-command-policies.json")
	service := NewTerminalPolicyService(path)

	const workers = 64
	const rounds = 4
	errs := make(chan error, workers*rounds)
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		worker := worker
		wg.Add(1)
		go func() {
			defer wg.Done()
			for round := 0; round < rounds; round++ {
				ruleID := fmt.Sprintf("allow-echo-%02d-%02d", worker, round)
				_, err := service.UpdateConfig(context.Background(), terminalpolicy.Config{
					SchemaVersion: terminalpolicy.SchemaVersion,
					Rules: []terminalpolicy.Rule{
						{
							ID:         ruleID,
							Effect:     terminalpolicy.RuleEffectAllow,
							Command:    "echo",
							ArgsPrefix: []string{ruleID},
							Reason:     "concurrent update serialization test",
						},
					},
				})
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("UpdateConfig() concurrent error = %v", err)
		}
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var config terminalpolicy.Config
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatalf("terminal policy file is not valid JSON: %v\n%s", err, raw)
	}
	if len(config.Rules) != 1 {
		t.Fatalf("len(config.Rules) = %d, want one final rule", len(config.Rules))
	}
	decision := service.Evaluate(terminalpolicy.CommandRequest{
		Command: "echo",
		Args:    config.Rules[0].ArgsPrefix,
	})
	if decision.Action != terminalpolicy.PolicyActionAllow || decision.RuleID != config.Rules[0].ID {
		t.Fatalf("Evaluate() = %#v, want final user allow rule", decision)
	}
}
