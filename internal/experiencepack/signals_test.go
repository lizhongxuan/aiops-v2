package experiencepack

import "testing"

func TestExtractSignalsFromChatText(t *testing.T) {
	signals := ExtractSignals("我要对 xxA 主机和 xxB 主机部署 pg 主从，Ubuntu 22.04，pg_mon 放在 xxC 主机")
	for _, expected := range []string{"postgres", "primary standby", "replication", "pg_mon", "deploy", "os:ubuntu", "hosts:3"} {
		if !contains(signals, expected) {
			t.Fatalf("signals = %#v, want %q", signals, expected)
		}
	}
}
