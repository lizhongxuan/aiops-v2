package settings

import "testing"

func TestRegistryRegisterAndUnregister(t *testing.T) {
	r := NewRegistry()

	r.Register(Entry{Name: "plugin-settings", Values: map[string]any{"enabled": true}})
	if _, ok := r.Get("plugin-settings"); !ok {
		t.Fatal("expected entry to be registered")
	}

	r.Unregister("plugin-settings")
	if _, ok := r.Get("plugin-settings"); ok {
		t.Fatal("expected entry to be removed")
	}
}
