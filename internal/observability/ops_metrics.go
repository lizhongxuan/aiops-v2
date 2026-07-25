package observability

import (
	"sync"
	"time"

	"runner/server/service"
)

const (
	OpsMetricPlanGeneration        = "plan_generation"
	OpsMetricPlanAcceptance        = "plan_acceptance"
	OpsMetricHostAgentCreation     = "host_agent_creation"
	OpsMetricCommandApproval       = "command_approval"
	OpsMetricCommandExecution      = "command_execution"
	OpsMetricTerminalConnection    = "terminal_connection"
	OpsMetricManualTerminalCommand = "manual_terminal_command"
	OpsMetricHumanHandoff          = "human_handoff"

	WorkflowMetricConflict          = "workflow_conflict"
	WorkflowMetricReceiptReplay     = "workflow_receipt_replay"
	WorkflowMetricWALRecovery       = "workflow_wal_recovery"
	WorkflowMetricEventLag          = "workflow_event_lag"
	WorkflowMetricEventGap          = "workflow_event_gap"
	WorkflowMetricCandidateAbandon  = "workflow_candidate_abandonment"
	WorkflowMetricAuthorizationDeny = "workflow_authorization_deny"
	WorkflowMetricInvalidCompleted  = "workflow_invalid_completed"

	WorkflowAlertRecoveryFailed        = "workflow_partial_commit_recovery_failed"
	WorkflowAlertDuplicateTransaction  = "workflow_duplicate_transaction"
	WorkflowAlertUnauthorizedMutation  = "workflow_unauthorized_mutation"
	WorkflowAlertEventGapUnrecoverable = "workflow_event_gap_unrecoverable"
	WorkflowAlertInvalidCompleted      = "workflow_invalid_completed"
)

type WorkflowOperationMetric struct {
	Count          uint64        `json:"count"`
	Success        uint64        `json:"success"`
	Failure        uint64        `json:"failure"`
	TotalLatency   time.Duration `json:"totalLatency"`
	MaximumLatency time.Duration `json:"maximumLatency"`
}

type WorkflowMetricsSnapshot struct {
	Operations map[string]WorkflowOperationMetric `json:"operations"`
	Signals    map[string]uint64                  `json:"signals"`
	Alerts     map[string]uint64                  `json:"alerts"`
}

type WorkflowMetrics struct {
	mu         sync.RWMutex
	operations map[string]WorkflowOperationMetric
	signals    map[string]uint64
	alerts     map[string]uint64
}

func NewWorkflowMetrics() *WorkflowMetrics {
	return &WorkflowMetrics{operations: map[string]WorkflowOperationMetric{}, signals: map[string]uint64{}, alerts: map[string]uint64{}}
}

func (m *WorkflowMetrics) Record(observation service.WorkflowEditObservation) {
	if m == nil || observation.Operation == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	name := string(observation.Operation)
	metric := m.operations[name]
	metric.Count++
	if observation.Outcome == service.WorkflowEditOutcomeSuccess {
		metric.Success++
	} else {
		metric.Failure++
	}
	metric.TotalLatency += observation.Duration
	if observation.Duration > metric.MaximumLatency {
		metric.MaximumLatency = observation.Duration
	}
	m.operations[name] = metric

	if observation.Outcome == service.WorkflowEditOutcomeConflict {
		m.signals[WorkflowMetricConflict]++
	}
	if observation.ReceiptReplay {
		m.signals[WorkflowMetricReceiptReplay]++
	}
	if observation.Operation == service.WorkflowEditObserveRecovery {
		m.signals[WorkflowMetricWALRecovery]++
		if observation.Outcome != service.WorkflowEditOutcomeSuccess {
			m.alerts[WorkflowAlertRecoveryFailed]++
		}
	}
	if observation.Operation == service.WorkflowEditObserveEvent && observation.EventLag > 0 {
		m.signals[WorkflowMetricEventLag]++
	}
	if observation.Operation == service.WorkflowEditObserveGap {
		m.signals[WorkflowMetricEventGap] += max(observation.GapSize, 1)
	}
	if observation.Operation == service.WorkflowEditObserveAbandon {
		m.signals[WorkflowMetricCandidateAbandon]++
	}
	switch observation.Outcome {
	case service.WorkflowEditOutcomeAuthorizationDenied:
		m.signals[WorkflowMetricAuthorizationDeny]++
		m.alerts[WorkflowAlertUnauthorizedMutation]++
	case service.WorkflowEditOutcomeDuplicate:
		m.alerts[WorkflowAlertDuplicateTransaction]++
	case service.WorkflowEditOutcomeGapUnrecoverable:
		m.alerts[WorkflowAlertEventGapUnrecoverable]++
	case service.WorkflowEditOutcomeInvalidCompleted:
		m.signals[WorkflowMetricInvalidCompleted]++
		m.alerts[WorkflowAlertInvalidCompleted]++
	}
}

func (m *WorkflowMetrics) Snapshot() WorkflowMetricsSnapshot {
	if m == nil {
		return WorkflowMetricsSnapshot{Operations: map[string]WorkflowOperationMetric{}, Signals: map[string]uint64{}, Alerts: map[string]uint64{}}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := WorkflowMetricsSnapshot{Operations: make(map[string]WorkflowOperationMetric, len(m.operations)), Signals: make(map[string]uint64, len(m.signals)), Alerts: make(map[string]uint64, len(m.alerts))}
	for key, value := range m.operations {
		result.Operations[key] = value
	}
	for key, value := range m.signals {
		result.Signals[key] = value
	}
	for key, value := range m.alerts {
		result.Alerts[key] = value
	}
	return result
}

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
var defaultWorkflowMetrics = NewWorkflowMetrics()

func RecordOpsMetric(name string, success bool) {
	defaultOpsMetrics.Record(name, success)
}

func OpsMetricsSnapshot() map[string]OpsMetricCounter {
	return defaultOpsMetrics.Snapshot()
}

func ResetOpsMetricsForTest() {
	defaultOpsMetrics = NewOpsMetrics()
}

func DefaultWorkflowMetrics() *WorkflowMetrics {
	return defaultWorkflowMetrics
}

func CurrentWorkflowMetricsSnapshot() WorkflowMetricsSnapshot {
	return defaultWorkflowMetrics.Snapshot()
}

func ResetWorkflowMetricsForTest() {
	defaultWorkflowMetrics = NewWorkflowMetrics()
}
