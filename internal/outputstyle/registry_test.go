package outputstyle

import "testing"

func TestRegistryRegisterAndUnregister(t *testing.T) {
	r := NewRegistry()

	def := Definition{
		Name:   "concise",
		Prompt: "Be concise",
		Source: "builtin",
	}
	if err := r.Register(def); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	got, ok := r.Get("concise")
	if !ok {
		t.Fatal("expected output style to be registered")
	}
	if got.Prompt != "Be concise" {
		t.Fatalf("Get().Prompt = %q, want %q", got.Prompt, "Be concise")
	}

	r.Unregister("concise")
	if _, ok := r.Get("concise"); ok {
		t.Fatal("expected output style to be removed")
	}
}
