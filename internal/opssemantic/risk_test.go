package opssemantic

import "testing"

func TestRiskClassifierCoversGenericOperationalLevels(t *testing.T) {
	tests := []struct {
		name string
		text string
		want OpsRiskLevel
	}{
		{name: "status query", text: "查看状态", want: RiskReadOnly},
		{name: "logs", text: "查看日志", want: RiskReadOnly},
		{name: "version check", text: "检查版本", want: RiskReadOnly},
		{name: "temporary file", text: "创建临时文件", want: RiskLowWrite},
		{name: "install dependency", text: "安装依赖", want: RiskMediumWrite},
		{name: "start service", text: "启动服务", want: RiskMediumWrite},
		{name: "system service", text: "修改系统服务", want: RiskHighWrite},
		{name: "network", text: "调整网络配置", want: RiskHighWrite},
		{name: "permission", text: "修改权限", want: RiskHighWrite},
		{name: "delete", text: "删除文件", want: RiskDestructive},
		{name: "overwrite", text: "覆盖配置", want: RiskDestructive},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyRisk(tt.text); got != tt.want {
				t.Fatalf("ClassifyRisk(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

func TestRiskClassifierKeepsExplicitReadOnlyInspectionReadOnly(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{name: "kernel info", text: "查看系统版本和内核信息，只读执行 uname -a 和 hostnamectl 并总结"},
		{name: "network listeners", text: "检查网络监听端口情况，只读执行 ss -ltnp 并总结"},
		{name: "certificate expiry", text: "查看证书过期时间并回传证据"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyRisk(tt.text); got != RiskReadOnly {
				t.Fatalf("ClassifyRisk(%q) = %q, want %q", tt.text, got, RiskReadOnly)
			}
		})
	}
}

func TestRiskRequiresApprovalFromMediumWriteUp(t *testing.T) {
	tests := []struct {
		risk OpsRiskLevel
		want bool
	}{
		{RiskReadOnly, false},
		{RiskLowWrite, false},
		{RiskMediumWrite, true},
		{RiskHighWrite, true},
		{RiskDestructive, true},
	}

	for _, tt := range tests {
		if got := RiskRequiresApproval(tt.risk); got != tt.want {
			t.Fatalf("RiskRequiresApproval(%q) = %v, want %v", tt.risk, got, tt.want)
		}
	}
}
