package observability

import "testing"

func TestOpsMetricsRecordsSuccessRates(t *testing.T) {
	metrics := NewOpsMetrics()
	metrics.Record(OpsMetricPlanGeneration, true)
	metrics.Record(OpsMetricPlanGeneration, false)
	metrics.Record(OpsMetricPlanGeneration, true)

	snapshot := metrics.Snapshot()
	counter := snapshot[OpsMetricPlanGeneration]
	if counter.Success != 2 || counter.Failure != 1 || counter.Total != 3 {
		t.Fatalf("counter = %#v, want 2 success, 1 failure, 3 total", counter)
	}
	if counter.Rate < 0.66 || counter.Rate > 0.67 {
		t.Fatalf("rate = %f, want about 0.666", counter.Rate)
	}
}

func TestDefaultOpsMetricsSnapshot(t *testing.T) {
	ResetOpsMetricsForTest()
	RecordOpsMetric(OpsMetricCommandApproval, true)
	RecordOpsMetric(OpsMetricCommandApproval, false)

	counter := OpsMetricsSnapshot()[OpsMetricCommandApproval]
	if counter.Success != 1 || counter.Failure != 1 || counter.Rate != 0.5 {
		t.Fatalf("counter = %#v, want 1/1/0.5", counter)
	}
}
