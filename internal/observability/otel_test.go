package observability

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestInitDisabledReturnsNoopProvider(t *testing.T) {
	provider, err := Init(context.Background(), Config{})
	if err != nil {
		t.Fatalf("Init disabled error = %v", err)
	}
	if provider.Enabled() {
		t.Fatal("disabled provider should report Enabled false")
	}
	if provider.Tracer() == nil {
		t.Fatal("disabled provider returned nil tracer")
	}
	if err := provider.Shutdown(context.Background()); err != nil {
		t.Fatalf("disabled shutdown error = %v", err)
	}
}

func TestInitEnabledWithEmptyEndpointFailsClearly(t *testing.T) {
	_, err := Init(context.Background(), Config{Enabled: true, Endpoint: "", ServiceName: "aiops-v2-agent"})
	if err == nil {
		t.Fatal("expected empty endpoint error")
	}
	if !strings.Contains(err.Error(), "otel endpoint is required") {
		t.Fatalf("error = %v", err)
	}
}

func TestInitEnabledWithUnreachableEndpointStillReturnsProvider(t *testing.T) {
	provider, err := Init(context.Background(), Config{
		Enabled:     true,
		Endpoint:    "http://127.0.0.1:9/v1/traces",
		ServiceName: "aiops-v2-agent",
	})
	if err != nil {
		t.Fatalf("Init with unreachable endpoint should not dial synchronously: %v", err)
	}
	if !provider.Enabled() {
		t.Fatal("provider should be enabled")
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_ = provider.Shutdown(shutdownCtx)
}
