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

func TestIsReadOnlyCommandAllowsHostResourceInspection(t *testing.T) {
	cases := []struct {
		command string
		args    []string
	}{
		{command: "uptime"},
		{command: "nproc"},
		{command: "free", args: []string{"-h"}},
		{command: "sysctl", args: []string{"-n", "hw.ncpu"}},
		{command: "sysctl", args: []string{"hw.memsize"}},
		{command: "vm_stat"},
		{command: "which", args: []string{"go"}},
	}
	for _, tc := range cases {
		if !IsReadOnlyCommand(tc.command, tc.args) {
			t.Fatalf("%s %v should be classified read-only", tc.command, tc.args)
		}
	}
}

func TestIsReadOnlyCommandAllowsSedPrintRange(t *testing.T) {
	if !IsReadOnlyCommand("sed", []string{"-n", "1,220p", "testdata/eval_cases/high-risk-approval-required.json"}) {
		t.Fatal("sed -n line-range print should be classified read-only")
	}
}

func TestIsReadOnlyCommandRejectsMutatingSed(t *testing.T) {
	if IsReadOnlyCommand("sed", []string{"-i", "s/a/b/", "file.txt"}) {
		t.Fatal("sed -i must not be classified read-only")
	}
	if IsReadOnlyCommand("sed", []string{"-n", "1,10w", "out.txt", "file.txt"}) {
		t.Fatal("sed write command must not be classified read-only")
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

func TestIsReadOnlyCommandRejectsMutatingSysctl(t *testing.T) {
	if IsReadOnlyCommand("sysctl", []string{"-w", "kern.maxfiles=1024"}) {
		t.Fatal("sysctl -w must not be classified read-only")
	}
}
