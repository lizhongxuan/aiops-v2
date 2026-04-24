package metrics

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type Collector struct {
	submitted   atomic.Int64
	started     atomic.Int64
	finished    atomic.Int64
	succeeded   atomic.Int64
	failed      atomic.Int64
	canceled    atomic.Int64
	interrupted atomic.Int64
	queueDepth  atomic.Int64

	mu          sync.Mutex
	durationSum float64
	durationCnt int64
}

func NewCollector() *Collector {
	return &Collector{}
}

func (c *Collector) ObserveRunSubmitted() {
	if c != nil {
		c.submitted.Add(1)
	}
}

func (c *Collector) ObserveRunStarted() {
	if c != nil {
		c.started.Add(1)
	}
}

func (c *Collector) ObserveRunFinished(status string, duration time.Duration) {
	if c == nil {
		return
	}
	c.finished.Add(1)
	switch status {
	case "success":
		c.succeeded.Add(1)
	case "failed":
		c.failed.Add(1)
	case "canceled":
		c.canceled.Add(1)
	case "interrupted":
		c.interrupted.Add(1)
	}
	if duration > 0 {
		c.mu.Lock()
		c.durationSum += duration.Seconds()
		c.durationCnt++
		c.mu.Unlock()
	}
}

func (c *Collector) SetQueueDepth(depth int) {
	if c != nil {
		c.queueDepth.Store(int64(depth))
	}
}

func (c *Collector) averageDuration() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.durationCnt == 0 {
		return 0
	}
	return c.durationSum / float64(c.durationCnt)
}

func (c *Collector) RenderPrometheus() string {
	if c == nil {
		return ""
	}
	return fmt.Sprintf(
		"# HELP runner_server_runs_submitted_total Total submitted runs\n"+
			"# TYPE runner_server_runs_submitted_total counter\n"+
			"runner_server_runs_submitted_total %d\n"+
			"# HELP runner_server_runs_started_total Total started runs\n"+
			"# TYPE runner_server_runs_started_total counter\n"+
			"runner_server_runs_started_total %d\n"+
			"# HELP runner_server_runs_finished_total Total finished runs\n"+
			"# TYPE runner_server_runs_finished_total counter\n"+
			"runner_server_runs_finished_total %d\n"+
			"# HELP runner_server_runs_success_total Total successful runs\n"+
			"# TYPE runner_server_runs_success_total counter\n"+
			"runner_server_runs_success_total %d\n"+
			"# HELP runner_server_runs_failed_total Total failed runs\n"+
			"# TYPE runner_server_runs_failed_total counter\n"+
			"runner_server_runs_failed_total %d\n"+
			"# HELP runner_server_runs_canceled_total Total canceled runs\n"+
			"# TYPE runner_server_runs_canceled_total counter\n"+
			"runner_server_runs_canceled_total %d\n"+
			"# HELP runner_server_runs_interrupted_total Total interrupted runs\n"+
			"# TYPE runner_server_runs_interrupted_total counter\n"+
			"runner_server_runs_interrupted_total %d\n"+
			"# HELP runner_server_queue_depth Current queue depth\n"+
			"# TYPE runner_server_queue_depth gauge\n"+
			"runner_server_queue_depth %d\n"+
			"# HELP runner_server_run_duration_seconds_avg Average run duration in seconds\n"+
			"# TYPE runner_server_run_duration_seconds_avg gauge\n"+
			"runner_server_run_duration_seconds_avg %.6f\n",
		c.submitted.Load(),
		c.started.Load(),
		c.finished.Load(),
		c.succeeded.Load(),
		c.failed.Load(),
		c.canceled.Load(),
		c.interrupted.Load(),
		c.queueDepth.Load(),
		c.averageDuration(),
	)
}

func (c *Collector) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte(c.RenderPrometheus()))
	})
}
