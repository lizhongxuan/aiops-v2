package terminalpolicy

import "testing"

func TestTerminalReadOnlyStillRequiresAllowlist(t *testing.T) {
	allowed := []struct {
		command string
		args    []string
	}{
		{command: "kubectl", args: []string{"get", "events", "-n", "prod"}},
		{command: "kubectl", args: []string{"describe", "pod/redis-0", "-n", "prod"}},
		{command: "kubectl", args: []string{"logs", "deploy/redis", "-n", "prod", "--tail=100"}},
		{command: "redis-cli", args: []string{"-h", "redis.prod", "INFO"}},
		{command: "redis-cli", args: []string{"-u", "redis://redis.prod:6379", "MEMORY", "STATS"}},
		{command: "docker", args: []string{"ps", "--filter", "name=aiops-eval-nginx-0614", "--format", "{{.ID}}\t{{.Names}}\t{{.Status}}\t{{.Ports}}"}},
		{command: "docker", args: []string{"container", "ls", "--filter", "publish=18081", "--format", "{{.ID}} {{.Names}} {{.Ports}}"}},
		{command: "docker", args: []string{"inspect", "aiops-eval-nginx-0614"}},
	}
	denied := []struct {
		command string
		args    []string
	}{
		{command: "bash", args: []string{"-lc", "cat /etc/passwd"}},
		{command: "python", args: []string{"-c", "print('hi')"}},
		{command: "kubectl", args: []string{"logs", "-f", "deploy/redis", "-n", "prod"}},
		{command: "kubectl", args: []string{"delete", "pod/redis-0", "-n", "prod"}},
		{command: "redis-cli", args: []string{"FLUSHALL"}},
		{command: "docker", args: []string{"run", "-d", "--name", "nginx", "nginx:latest"}},
		{command: "docker", args: []string{"rm", "-f", "nginx"}},
		{command: "docker", args: []string{"logs", "-f", "nginx"}},
	}
	for _, input := range allowed {
		if !IsAllowedReadOnlyTerminal(input.command, input.args) {
			t.Fatalf("expected allowed: %#v", input)
		}
	}
	for _, input := range denied {
		if IsAllowedReadOnlyTerminal(input.command, input.args) {
			t.Fatalf("expected denied: %#v", input)
		}
	}
}

func TestAllowedHostInspectionTerminalAllowsBoundedResourceCommands(t *testing.T) {
	allowed := []struct {
		command string
		args    []string
	}{
		{command: "uptime"},
		{command: "top", args: []string{"-l", "1", "-s", "0"}},
		{command: "top", args: []string{"-b", "-n", "1"}},
		{command: "sysctl", args: []string{"-n", "hw.ncpu"}},
		{command: "vm_stat"},
		{command: "df", args: []string{"-h"}},
		{command: "free", args: []string{"-h"}},
		{command: "lscpu"},
		{command: "nproc"},
		{command: "who"},
		{command: "hostnamectl"},
		{command: "hostnamectl", args: []string{"status"}},
		{command: "systemctl", args: []string{"status", "nginx"}},
		{command: "systemctl", args: []string{"is-active", "--quiet", "nginx"}},
		{command: "systemctl", args: []string{"show", "nginx", "--property=ActiveState,SubState"}},
		{command: "nginx", args: []string{"-v"}},
		{command: "ps", args: []string{"-e"}},
		{command: "ps", args: []string{"-eo", "comm,pid"}},
		{command: "ps", args: []string{"-o", "pid,comm,pcpu,pmem"}},
		{command: "du", args: []string{"-h", "-d", "1", "/opt"}},
		{command: "du", args: []string{"-h", "--max-depth=1", "/opt"}},
		{command: "docker", args: []string{"stats", "--no-stream"}},
		{command: "docker", args: []string{"stats", "--no-stream", "--format", "{{.Name}} {{.CPUPerc}} {{.MemUsage}}"}},
		{command: "lsof", args: []string{"-i", ":1234"}},
		{command: "lsof", args: []string{"-n", "-P", "-iTCP:1234", "-sTCP:LISTEN"}},
		{command: "ss", args: []string{"-tlnp", "sport", "=", "18082"}},
		{command: "ss", args: []string{"-ltnp"}},
	}
	for _, input := range allowed {
		if !IsAllowedHostInspectionTerminal(input.command, input.args) {
			t.Fatalf("expected host inspection allowed: %#v", input)
		}
	}
}

func TestAllowedHostInspectionTerminalRejectsBroadOrMutatingCommands(t *testing.T) {
	denied := []struct {
		command string
		args    []string
	}{
		{command: "cat", args: []string{"/etc/passwd"}},
		{command: "bash", args: []string{"-lc", "uptime && rm -rf /tmp/nope"}},
		{command: "reboot", args: []string{"version"}},
		{command: "rm", args: []string{"--version"}},
		{command: "sysctl", args: []string{"-w", "kern.maxfiles=1024"}},
		{command: "top", args: []string{"-pid", "1"}},
		{command: "ps", args: []string{"--forest"}},
		{command: "ps", args: []string{"-eo", "pid;rm"}},
		{command: "ps", args: []string{"-eo", "pid,unknown_field"}},
		{command: "df", args: []string{"-x"}},
		{command: "du", args: []string{"-h", "-d", "many", "/opt"}},
		{command: "du", args: []string{"-h", "--max-depth=all", "/opt"}},
		{command: "docker", args: []string{"stats"}},
		{command: "hostnamectl", args: []string{"set-hostname", "changed"}},
		{command: "systemctl", args: []string{"restart", "nginx"}},
		{command: "systemctl", args: []string{"status", "nginx", "postgresql"}},
		{command: "lsof", args: []string{"/etc/passwd"}},
		{command: "lsof", args: []string{"-i", ":1234", "-F", "p"}},
		{command: "lsof", args: []string{"-i", "-n"}},
		{command: "ss", args: []string{"-K", "dst", "1.2.3.4"}},
		{command: "ss", args: []string{"-tlnp", "sport", "=", "not-a-port"}},
	}
	for _, input := range denied {
		if IsAllowedHostInspectionTerminal(input.command, input.args) {
			t.Fatalf("expected host inspection denied: %#v", input)
		}
	}
}

func TestMutatingTerminalCommandRequiresHighRiskApproval(t *testing.T) {
	if !RequiresHighRiskApproval("kubectl", []string{"rollout", "restart", "deploy/redis", "-n", "prod"}) {
		t.Fatal("mutating kubectl command must require high-risk approval")
	}
	if TerminalRiskLevel("kubectl", []string{"get", "events", "-n", "prod"}) != "low" {
		t.Fatal("allowlisted read-only kubectl command should remain low risk")
	}
}

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

func TestIsReadOnlyCommandAllowsSafeCurlStatusCheck(t *testing.T) {
	if !IsReadOnlyCommand("curl", []string{"-fsS", "-o", "/dev/null", "-w", "%{http_code}", "http://127.0.0.1:18081"}) {
		t.Fatal("curl GET with /dev/null output and write-out status should be read-only")
	}
	if !IsReadOnlyCommand("curl", []string{"-fsSI", "http://127.0.0.1:18081"}) {
		t.Fatal("curl HEAD status check should be read-only")
	}
	if IsReadOnlyCommand("curl", []string{"-s", "-o", "%{http_code}", "http://127.0.0.1:18081"}) {
		t.Fatal("curl output to a regular file must not be classified read-only")
	}
}

func TestIsReadOnlyCommandAllowsHostResourceInspection(t *testing.T) {
	cases := []struct {
		command string
		args    []string
	}{
		{command: "uptime"},
		{command: "nproc"},
		{command: "who"},
		{command: "lscpu"},
		{command: "free", args: []string{"-h"}},
		{command: "sysctl", args: []string{"-n", "hw.ncpu"}},
		{command: "sysctl", args: []string{"hw.memsize"}},
		{command: "vm_stat"},
		{command: "which", args: []string{"go"}},
		{command: "systemctl", args: []string{"status", "nginx"}},
		{command: "nginx", args: []string{"-v"}},
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
