package workflow

import "testing"

func TestEvalWhenBasic(t *testing.T) {
	ok, err := EvalWhen("", nil)
	if err != nil || !ok {
		t.Fatalf("expected empty expression to be true")
	}
	ok, err = EvalWhen("false", nil)
	if err != nil || ok {
		t.Fatalf("expected false to be false")
	}
	ok, err = EvalWhen("yes", nil)
	if err != nil || !ok {
		t.Fatalf("expected yes to be true")
	}
}

func TestEvalWhenVars(t *testing.T) {
	vars := map[string]any{"BACKUP_OK": "true", "NUM": 2}
	ok, err := EvalWhen("${BACKUP_OK} == \"true\"", vars)
	if err != nil || !ok {
		t.Fatalf("expected expression to be true")
	}
	ok, err = EvalWhen("NUM > 1", vars)
	if err != nil || !ok {
		t.Fatalf("expected numeric expression to be true")
	}
	ok, err = EvalWhen("${MISSING} == \"\"", vars)
	if err != nil || !ok {
		t.Fatalf("expected missing var to compare as empty")
	}
}

func TestEvalWhenLogical(t *testing.T) {
	vars := map[string]any{"A": "x", "B": "y"}
	ok, err := EvalWhen("A == \"x\" && B == \"y\"", vars)
	if err != nil || !ok {
		t.Fatalf("expected AND expression to be true")
	}
	ok, err = EvalWhen("A == \"x\" || B == \"z\"", vars)
	if err != nil || !ok {
		t.Fatalf("expected OR expression to be true")
	}
}
