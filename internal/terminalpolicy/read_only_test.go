package terminalpolicy

import "testing"

func TestIsReadOnlyCommandAllowsShellWrappedDate(t *testing.T) {
	if !IsReadOnlyCommand("bash", []string{"-lc", "date '+%F %A %u %T %Z'"}) {
		t.Fatal("bash -lc date should be classified read-only")
	}
}

func TestIsReadOnlyCommandAllowsShellWrappedCurlGet(t *testing.T) {
	args := []string{
		"-lc",
		"curl -L --max-time 20 -A 'Mozilla/5.0' 'https://example.com/data.json?symbol=000001&fields=f1,f2'",
	}
	if !IsReadOnlyCommand("bash", args) {
		t.Fatal("bash -lc safe curl GET should be classified read-only")
	}
}

func TestIsReadOnlyCommandRejectsShellWrappedMutation(t *testing.T) {
	if IsReadOnlyCommand("bash", []string{"-lc", "curl -X POST https://example.com/api"}) {
		t.Fatal("bash -lc curl POST must not be classified read-only")
	}
	if IsReadOnlyCommand("bash", []string{"-lc", "date && rm -rf /tmp/nope"}) {
		t.Fatal("bash -lc command with shell operators must not be classified read-only")
	}
}
