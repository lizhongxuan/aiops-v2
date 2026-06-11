package observability

import "sync"

const (
	OpsMetricPlanGeneration        = "plan_generation"
	OpsMetricPlanAcceptance        = "plan_acceptance"
	OpsMetricHostAgentCreation     = "host_agent_creation"
	OpsMetricCommandApproval       = "command_approval"
	OpsMetricCommandExecution      = "command_execution"
	OpsMetricTerminalConnection    = "terminal_connection"
	OpsMetricManualTerminalCommand = "manual_terminal_command"
	OpsMetricHumanHandoff          = "human_handoff"
)

type OpsMetricCounter struct {
	Success int     `json:"success"`
	Failure int     `json:"failure"`
	Total   int     `json:"total"`
	Rate    float64 `json:"rate"`
}

type OpsMetrics struct {
	mu       sync.RWMutex
	counters map[string]OpsMetricCounter
}

func NewOpsMetrics() *OpsMetrics {
	return &OpsMetrics{counters: map[string]OpsMetricCounter{}}
}

func (m *OpsMetrics) Record(name string, success bool) {
	if m == nil || name == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	counter := m.counters[name]
	if success {
		counter.Success++
	} else {
		counter.Failure++
	}
	counter.Total = counter.Success + counter.Failure
	if counter.Total > 0 {
		counter.Rate = float64(counter.Success) / float64(counter.Total)
	}
	m.counters[name] = counter
}

func (m *OpsMetrics) Snapshot() map[string]OpsMetricCounter {
	if m == nil {
		return map[string]OpsMetricCounter{}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]OpsMetricCounter, len(m.counters))
	for name, counter := range m.counters {
		out[name] = counter
	}
	return out
}

var defaultOpsMetrics = NewOpsMetrics()

func RecordOpsMetric(name string, success bool) {
	defaultOpsMetrics.Record(name, success)
}

func OpsMetricsSnapshot() map[string]OpsMetricCounter {
	return defaultOpsMetrics.Snapshot()
}

func ResetOpsMetricsForTest() {
	defaultOpsMetrics = NewOpsMetrics()
}
