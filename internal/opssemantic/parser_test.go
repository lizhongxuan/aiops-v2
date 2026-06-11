package opssemantic

import "testing"

func TestParserIdentifiesMultipleHostMentionsAndRequiresPlan(t *testing.T) {
	task := ParseTask(ParseInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Text:      "请在 @host-a 和 @10.0.0.2 上查看运行状态并汇总证据",
	})

	if len(task.HostScope) != 2 {
		t.Fatalf("len(HostScope) = %d, want 2: %#v", len(task.HostScope), task.HostScope)
	}
	if task.HostScope[0].Raw != "@host-a" || task.HostScope[1].Raw != "@10.0.0.2" {
		t.Fatalf("HostScope = %#v, want ordered host mentions", task.HostScope)
	}
	if !task.PlanRequired {
		t.Fatalf("PlanRequired = false, want true for multi-host task")
	}
	if task.RiskLevel != RiskReadOnly {
		t.Fatalf("RiskLevel = %q, want %q", task.RiskLevel, RiskReadOnly)
	}
	if len(task.MissingSlots) != 0 {
		t.Fatalf("MissingSlots = %#v, want none", task.MissingSlots)
	}
}

func TestParserReportsMissingTargetHost(t *testing.T) {
	task := ParseTask(ParseInput{Text: "查看运行状态并给出证据"})

	if len(task.MissingSlots) != 1 {
		t.Fatalf("len(MissingSlots) = %d, want 1: %#v", len(task.MissingSlots), task.MissingSlots)
	}
	if task.MissingSlots[0].Name != SlotTargetHost {
		t.Fatalf("MissingSlots[0].Name = %q, want %q", task.MissingSlots[0].Name, SlotTargetHost)
	}
}

func TestParserClassifiesGenericActionRisk(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		wantAction OpsActionType
		wantRisk   OpsRiskLevel
	}{
		{name: "read only", text: "@host-a 查看状态和日志", wantAction: ActionReadOnly, wantRisk: RiskReadOnly},
		{name: "low write", text: "@host-a 创建临时文件用于排查", wantAction: ActionWrite, wantRisk: RiskLowWrite},
		{name: "medium write", text: "@host-a 安装依赖并启动普通服务", wantAction: ActionWrite, wantRisk: RiskMediumWrite},
		{name: "high write", text: "@host-a 修改系统服务和防火墙规则", wantAction: ActionWrite, wantRisk: RiskHighWrite},
		{name: "destructive", text: "@host-a 删除目录并覆盖配置", wantAction: ActionWrite, wantRisk: RiskDestructive},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := ParseTask(ParseInput{Text: tt.text})
			if task.ActionType != tt.wantAction {
				t.Fatalf("ActionType = %q, want %q", task.ActionType, tt.wantAction)
			}
			if task.RiskLevel != tt.wantRisk {
				t.Fatalf("RiskLevel = %q, want %q", task.RiskLevel, tt.wantRisk)
			}
		})
	}
}
