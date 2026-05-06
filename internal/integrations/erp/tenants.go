package erp

func sampleMetrics(capability, service string) []BusinessMetric {
	return []BusinessMetric{
		{Name: "order_submit_success_rate", Value: 98.7, Unit: "%", Threshold: 99.5, Status: "warning"},
		{Name: "order_submit_p95_latency_ms", Value: 1420, Unit: "ms", Threshold: 800, Status: "critical"},
		{Name: "payment_callback_lag_s", Value: 18, Unit: "s", Threshold: 60, Status: "ok"},
	}
}

func sampleTenantImpact(capability string) []TenantImpact {
	return []TenantImpact{
		{TenantID: "tenant-east-01", TenantName: "华东直营网点", Severity: "high", KeyProcesses: []string{"订单提交", "库存预留", "财务入账"}},
		{TenantID: "tenant-north-03", TenantName: "华北渠道", Severity: "medium", KeyProcesses: []string{"订单提交", "发票申请"}},
	}
}

func sampleJobStatus(service string) []JobStatus {
	return []JobStatus{
		{Name: "report-nightly-close", Status: "running", QueueDepth: 17, Dependencies: []string{"erp-report", "pg-primary"}},
		{Name: "order-retry-worker", Status: "degraded", QueueDepth: 42, RecentFailure: "timeout waiting for inventory reservation"},
	}
}
