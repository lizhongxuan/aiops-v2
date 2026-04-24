package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSubmitAndWait_Success(t *testing.T) {
	var pollCount int32
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/runs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)

		// 验证 env vars 结构
		vars, _ := req["vars"].(map[string]any)
		env, _ := vars["env"].(map[string]any)
		if env["BACKUP_DIR"] != "/data/backup" {
			t.Errorf("expected BACKUP_DIR=/data/backup, got %v", env["BACKUP_DIR"])
		}

		// 验证 workflow_yaml 包含注入的 host
		yamlText, _ := req["workflow_yaml"].(string)
		if !strings.Contains(yamlText, "http://10.0.0.5:9990") {
			t.Errorf("yaml should contain injected host address")
		}

		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]any{
			"run_id": "run-001",
			"status": "queued",
		})
	})
	mux.HandleFunc("/api/v1/runs/run-001", func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&pollCount, 1)
		status := "running"
		if n >= 3 {
			status = "success"
		}
		json.NewEncoder(w).Encode(map[string]any{
			"run_id":  "run-001",
			"status":  status,
			"message": "",
		})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	c := New(server.URL, "test-token",
		WithPollInterval(50*time.Millisecond),
		WithTimeout(5*time.Second),
	)

	yamlText := `version: "1"
name: test
inventory:
  hosts:
    backup-host:
      address: ""
steps:
  - name: run
    targets: [backup-host]
    action: cmd.run
    args:
      cmd: "echo ok"
`
	result, err := c.SubmitAndWait(context.Background(), yamlText, RunOptions{
		Env:         map[string]string{"BACKUP_DIR": "/data/backup", "DB_PASSWORD": "secret"},
		Hosts:       map[string]string{"backup-host": "http://10.0.0.5:9990"},
		TriggeredBy: "test",
	})
	if err != nil {
		t.Fatalf("SubmitAndWait failed: %v", err)
	}
	if result.Status != "success" {
		t.Fatalf("expected success, got %s", result.Status)
	}
	if result.RunID != "run-001" {
		t.Fatalf("expected run-001, got %s", result.RunID)
	}
}

func TestSubmitAndWait_Failed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/runs", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]any{"run_id": "run-fail", "status": "queued"})
	})
	mux.HandleFunc("/api/v1/runs/run-fail", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"run_id":  "run-fail",
			"status":  "failed",
			"message": "backup command exited with code 1",
		})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	c := New(server.URL, "token", WithPollInterval(10*time.Millisecond))
	_, err := c.SubmitAndWait(context.Background(), "version: '1'\nname: t", RunOptions{})
	if err == nil {
		t.Fatal("expected error for failed run")
	}
	if !strings.Contains(err.Error(), "failed") {
		t.Fatalf("error should mention 'failed': %v", err)
	}
}

func TestSubmitAndWait_ContextCancel(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/runs", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]any{"run_id": "run-cancel", "status": "queued"})
	})
	mux.HandleFunc("/api/v1/runs/run-cancel", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"run_id": "run-cancel", "status": "running"})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	c := New(server.URL, "token", WithPollInterval(50*time.Millisecond))
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := c.SubmitAndWait(ctx, "version: '1'\nname: t", RunOptions{})
	if err == nil {
		t.Fatal("expected error on context cancel")
	}
}

func TestInjectHosts(t *testing.T) {
	yamlText := `version: "1"
name: test
inventory:
  hosts:
    backup-host:
      address: ""
steps:
  - name: run
    targets: [backup-host]
    action: cmd.run
    args:
      cmd: "echo ok"
`
	result := InjectHosts(yamlText, map[string]string{
		"backup-host": "http://10.0.0.5:9990",
		"extra-host":  "http://10.0.0.6:9990",
	})
	if !strings.Contains(result, "http://10.0.0.5:9990") {
		t.Fatal("should contain backup-host address")
	}
	if !strings.Contains(result, "http://10.0.0.6:9990") {
		t.Fatal("should contain extra-host address")
	}
}

func TestBuildEnvVars(t *testing.T) {
	result := BuildEnvVars(map[string]string{
		"BACKUP_DIR":  "/data",
		"DB_PASSWORD": "secret",
	})
	env, ok := result["env"].(map[string]any)
	if !ok {
		t.Fatal("result should have env key")
	}
	if env["BACKUP_DIR"] != "/data" {
		t.Fatalf("BACKUP_DIR mismatch: %v", env["BACKUP_DIR"])
	}
	if env["DB_PASSWORD"] != "secret" {
		t.Fatalf("DB_PASSWORD mismatch: %v", env["DB_PASSWORD"])
	}
}
