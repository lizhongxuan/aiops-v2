package appui

import (
	"strings"
	"testing"
)

func TestExtractUserEvidenceDetectsDatabaseRecoveryEvidence(t *testing.T) {
	input := `
不要执行命令，只基于下面输出分析。
db=> select is_in_recovery();
 is_in_recovery
-------------------
 f

control data: Latest checkpoint history branch id: 11
replica marker: missing
LOG: archive recovery complete
`
	got := ExtractUserEvidence(input)
	if !got.HasEvidence {
		t.Fatalf("HasEvidence = false, want true")
	}
	if !got.UserProhibitsExec {
		t.Fatalf("UserProhibitsExec = false, want true")
	}
	for _, want := range []string{"sql_result", "command_output", "log"} {
		if !containsString(got.EvidenceKinds, want) {
			t.Fatalf("EvidenceKinds = %#v, missing %q", got.EvidenceKinds, want)
		}
	}
	for _, want := range []string{"database_recovery_inactive", "replica_marker_missing", "history_branch_id", "archive_recovery_completed"} {
		if !containsString(got.Signals, want) {
			t.Fatalf("Signals = %#v, missing %q", got.Signals, want)
		}
	}
	if strings.TrimSpace(got.RawExcerpt) == "" {
		t.Fatalf("RawExcerpt is empty")
	}
}

func TestExtractUserEvidenceDetectsTimelineMismatchEvidence(t *testing.T) {
	input := `
请只基于下面证据分析 PG timeline 异常，不要执行主机命令。

主机A:
$ pg_controldata /var/lib/postgresql/15/main | egrep 'Database cluster state|Latest checkpoint|TimeLineID'
Database cluster state:               in production
Latest checkpoint's TimeLineID:        7
Latest checkpoint's PrevTimeLineID:    6

主机B:
$ pg_controldata /var/lib/postgresql/15/main | egrep 'Database cluster state|Latest checkpoint|TimeLineID'
Database cluster state:               in archive recovery
Latest checkpoint's TimeLineID:        9
Latest checkpoint's PrevTimeLineID:    8
$ test -f /var/lib/postgresql/15/main/standby.signal; echo $?
0
日志片段:
2026-06-23 10:13:02 CST [415] FATAL: requested timeline 7 is not a child of this server's history
`
	got := ExtractUserEvidence(input)
	if !got.HasEvidence {
		t.Fatalf("HasEvidence = false, want true")
	}
	if !got.UserProhibitsExec {
		t.Fatalf("UserProhibitsExec = false, want true")
	}
	for _, want := range []string{"database_control_timeline", "timeline_history_not_child", "timeline_mismatch", "archive_recovery_active", "standby_marker_seen"} {
		if !containsString(got.Signals, want) {
			t.Fatalf("Signals = %#v, missing %q", got.Signals, want)
		}
	}
}

func TestExtractUserEvidenceDetectsNoExecInstruction(t *testing.T) {
	got := ExtractUserEvidence("不要执行本机命令，只基于我贴出来的日志分析")
	if !got.UserProhibitsExec {
		t.Fatalf("UserProhibitsExec = false, want true")
	}
}

func TestExtractUserEvidenceDetectsCompositeNoHostCommandInstruction(t *testing.T) {
	got := ExtractUserEvidence("先只做原理分析和证据清单，不要连接或执行任何主机命令。")
	if !got.UserProhibitsExec {
		t.Fatalf("UserProhibitsExec = false, want true")
	}
}

func TestExtractUserEvidenceDetectsPlainLogBlock(t *testing.T) {
	input := `
2026-06-23 10:00:01 WARNING: archiving write-ahead log file failed too many times
2026-06-23 10:00:02 ERROR: could not open archived segment "0000000A000000000000000E": No such file or directory
`
	got := ExtractUserEvidence(input)
	if !got.HasEvidence {
		t.Fatalf("HasEvidence = false, want true")
	}
	if !containsString(got.EvidenceKinds, "log") {
		t.Fatalf("EvidenceKinds = %#v, want log", got.EvidenceKinds)
	}
	if strings.TrimSpace(got.RawExcerpt) == "" {
		t.Fatalf("RawExcerpt is empty")
	}
}

func TestExtractUserEvidenceDetectsPostgresCheckpointLogKeywords(t *testing.T) {
	input := "基于我贴出的 PostgreSQL 日志做 RCA：checkpoint too frequent, write latency spike。不要执行命令。"
	got := ExtractUserEvidence(input)
	if !got.HasEvidence {
		t.Fatalf("HasEvidence = false, want true")
	}
	if !got.UserProhibitsExec {
		t.Fatalf("UserProhibitsExec = false, want true")
	}
	for _, want := range []string{"log", "monitoring"} {
		if !containsString(got.EvidenceKinds, want) {
			t.Fatalf("EvidenceKinds = %#v, missing %q", got.EvidenceKinds, want)
		}
	}
	for _, want := range []string{"checkpoint_too_frequent", "write_latency_spike"} {
		if !containsString(got.Signals, want) {
			t.Fatalf("Signals = %#v, missing %q", got.Signals, want)
		}
	}
}

func TestExtractUserEvidenceDetectsMonitoringAndStackTrace(t *testing.T) {
	input := `
Coroot chart: p99 latency 2.4s, error rate 12%, CPU usage 91%.
panic: nil pointer
goroutine 42 [running]:
main.handleRequest()
	/app/server.go:88 +0x12
`
	got := ExtractUserEvidence(input)
	if !got.HasEvidence {
		t.Fatalf("HasEvidence = false, want true")
	}
	for _, want := range []string{"monitoring", "stack_trace"} {
		if !containsString(got.EvidenceKinds, want) {
			t.Fatalf("EvidenceKinds = %#v, missing %q", got.EvidenceKinds, want)
		}
	}
}

func TestExtractUserEvidenceReturnsEmptyForShortQuestion(t *testing.T) {
	got := ExtractUserEvidence("复制集群 history branch 为什么会变高？")
	if got.HasEvidence {
		t.Fatalf("HasEvidence = true, want false: %+v", got)
	}
}
