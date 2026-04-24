package state

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type RunStateCallback struct {
	RunID        string    `json:"run_id"`
	WorkflowName string    `json:"workflow_name,omitempty"`
	Status       string    `json:"status"`
	Step         string    `json:"step,omitempty"`
	Host         string    `json:"host,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
	Error        string    `json:"error,omitempty"`
	Version      int64     `json:"version"`
}

type RunStateNotifier interface {
	NotifyRunState(ctx context.Context, payload RunStateCallback) error
}

type HTTPNotifier struct {
	URL     string
	Headers map[string]string
	Client  *http.Client
}

func NewHTTPNotifier(url string, headers map[string]string, client *http.Client) *HTTPNotifier {
	copied := map[string]string{}
	for k, v := range headers {
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k == "" || v == "" {
			continue
		}
		copied[k] = v
	}
	return &HTTPNotifier{
		URL:     strings.TrimSpace(url),
		Headers: copied,
		Client:  client,
	}
}

func (n *HTTPNotifier) NotifyRunState(ctx context.Context, payload RunStateCallback) error {
	if n == nil || strings.TrimSpace(n.URL) == "" {
		return fmt.Errorf("notifier url is required")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range n.Headers {
		req.Header.Set(k, v)
	}
	client := n.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("notifier responded with %s", resp.Status)
	}
	return nil
}
