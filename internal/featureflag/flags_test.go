package featureflag

import (
	"reflect"
	"testing"
	"time"
)

func TestDefaultAndClone(t *testing.T) {
	f := Default()
	if !f.DiagnosticProtocol {
		t.Fatalf("diagnostic protocol should default on: %#v", f)
	}
	if f.ReadOnlyToolRetryEnabled {
		t.Fatalf("read-only retry should default off: %#v", f)
	}

	clone := f.Clone()
	if !reflect.DeepEqual(f, clone) {
		t.Fatalf("clone should match original: %#v vs %#v", f, clone)
	}
}

func TestFromEnvDiagnosticProtocolCanBeDisabled(t *testing.T) {
	f := FromEnv(func(key string) string {
		if key == envDiagnosticProtocol {
			return "0"
		}
		return ""
	})
	if f.DiagnosticProtocol {
		t.Fatalf("diagnostic protocol should parse explicit 0 as disabled: %#v", f)
	}
}

func TestFromEnvDiagnosticProtocolDefaultsOn(t *testing.T) {
	f := FromEnv(func(string) string { return "" })
	if !f.DiagnosticProtocol {
		t.Fatalf("diagnostic protocol should default on when env is unset: %#v", f)
	}
}

func TestReadOnlyToolRetryFlags(t *testing.T) {
	lookup := func(key string) string {
		switch key {
		case envReadOnlyToolRetry:
			return "1"
		case envReadOnlyToolRetryMaxPerCall:
			return "2"
		case envReadOnlyToolRetryMaxPerTurn:
			return "5"
		case envReadOnlyToolRetryBackoffBaseMS:
			return "100"
		case envReadOnlyToolRetryBackoffMaxMS:
			return "1000"
		default:
			return ""
		}
	}

	f := FromEnv(lookup)
	if !f.ReadOnlyToolRetryEnabled {
		t.Fatalf("ReadOnlyToolRetryEnabled = false, want true")
	}
	if f.ReadOnlyToolRetryMaxPerCall != 2 {
		t.Fatalf("MaxPerCall = %d, want 2", f.ReadOnlyToolRetryMaxPerCall)
	}
	if f.ReadOnlyToolRetryMaxPerTurn != 5 {
		t.Fatalf("MaxPerTurn = %d, want 5", f.ReadOnlyToolRetryMaxPerTurn)
	}
	if f.ReadOnlyToolRetryBackoffBase != 100*time.Millisecond {
		t.Fatalf("BackoffBase = %s, want 100ms", f.ReadOnlyToolRetryBackoffBase)
	}
	if f.ReadOnlyToolRetryBackoffMax != time.Second {
		t.Fatalf("BackoffMax = %s, want 1s", f.ReadOnlyToolRetryBackoffMax)
	}
}
