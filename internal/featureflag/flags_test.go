package featureflag

import (
	"reflect"
	"testing"
	"time"
)

type runtimeSettingsStub struct {
	diagnosticProtocol bool
	retryEnabled       bool
	maxPerCall         int
	maxPerTurn         int
	backoffBaseMs      int
	backoffMaxMs       int
}

func (s runtimeSettingsStub) RuntimeDiagnosticProtocol() bool        { return s.diagnosticProtocol }
func (s runtimeSettingsStub) RuntimeReadOnlyRetryEnabled() bool      { return s.retryEnabled }
func (s runtimeSettingsStub) RuntimeReadOnlyRetryMaxPerCall() int    { return s.maxPerCall }
func (s runtimeSettingsStub) RuntimeReadOnlyRetryMaxPerTurn() int    { return s.maxPerTurn }
func (s runtimeSettingsStub) RuntimeReadOnlyRetryBackoffBaseMs() int { return s.backoffBaseMs }
func (s runtimeSettingsStub) RuntimeReadOnlyRetryBackoffMaxMs() int  { return s.backoffMaxMs }

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

func TestFromRuntimeSettingsDiagnosticProtocolCanBeDisabled(t *testing.T) {
	f := FromRuntimeSettings(runtimeSettingsStub{diagnosticProtocol: false})
	if f.DiagnosticProtocol {
		t.Fatalf("diagnostic protocol should parse explicit 0 as disabled: %#v", f)
	}
}

func TestFromRuntimeSettingsDiagnosticProtocolDefaultsOn(t *testing.T) {
	f := FromRuntimeSettings(nil)
	if !f.DiagnosticProtocol {
		t.Fatalf("diagnostic protocol should default on when settings are nil: %#v", f)
	}
}

func TestReadOnlyToolRetryFlags(t *testing.T) {
	f := FromRuntimeSettings(runtimeSettingsStub{
		diagnosticProtocol: true,
		retryEnabled:       true,
		maxPerCall:         2,
		maxPerTurn:         5,
		backoffBaseMs:      100,
		backoffMaxMs:       1000,
	})
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
