package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"runner/engine"
	"runner/logging"
	"runner/scheduler"
	"runner/server/events"
	"runner/server/metrics"
	"runner/server/queue"
	"runner/server/store/eventstore"
	"runner/state"
	"runner/workflow"
)

type RunServiceConfig struct {
	MaxConcurrentRuns  int
	MaxOutputBytes     int
	MetaStore          RunRecordStore
	EventStore         eventstore.Store
	AgentDispatchToken string
}

type RunService struct {
	workflowSvc *WorkflowService
	pre         *Preprocessor
	runStore    state.RunStateStore
	recordStore RunRecordStore
	eventStore  eventstore.Store
	queue       queue.Queue
	events      *events.Hub
	metrics     *metrics.Collector

	maxOutputBytes     int
	agentDispatchToken string

	workerCtx    context.Context
	workerCancel context.CancelFunc
	workerWg     sync.WaitGroup

	mu          sync.RWMutex
	jobs        map[string]*runJob
	metas       map[string]RunMeta
	idempotency map[string]string
}

type runJob struct {
	RunID      string
	Workflow   workflow.Workflow
	Meta       RunMeta
	Canceled   bool
	CancelFunc context.CancelFunc
}

func NewRunService(cfg RunServiceConfig, workflowSvc *WorkflowService, pre *Preprocessor, runStore state.RunStateStore, q queue.Queue, hub *events.Hub, collector *metrics.Collector) *RunService {
	if cfg.MaxConcurrentRuns <= 0 {
		cfg.MaxConcurrentRuns = 1
	}
	if cfg.MaxOutputBytes <= 0 {
		cfg.MaxOutputBytes = 65536
	}
	if runStore == nil {
		runStore = state.NewInMemoryRunStore()
	}
	if cfg.MetaStore == nil {
		cfg.MetaStore = NewInMemoryRunRecordStore()
	}
	if q == nil {
		q = queue.NewMemoryQueue(1)
	}
	if hub == nil {
		hub = events.NewHub()
	}
	if collector == nil {
		collector = metrics.NewCollector()
	}

	workerCtx, workerCancel := context.WithCancel(context.Background())
	s := &RunService{
		workflowSvc:        workflowSvc,
		pre:                pre,
		runStore:           runStore,
		recordStore:        cfg.MetaStore,
		eventStore:         cfg.EventStore,
		queue:              q,
		events:             hub,
		metrics:            collector,
		maxOutputBytes:     cfg.MaxOutputBytes,
		agentDispatchToken: strings.TrimSpace(cfg.AgentDispatchToken),
		workerCtx:          workerCtx,
		workerCancel:       workerCancel,
		jobs:               map[string]*runJob{},
		metas:              map[string]RunMeta{},
		idempotency:        map[string]string{},
	}

	_, _ = runStore.MarkInterruptedRunning(context.Background(), "runner-server restarted")
	s.restoreRecords(context.Background())

	for i := 0; i < cfg.MaxConcurrentRuns; i++ {
		s.workerWg.Add(1)
		go func() {
			defer s.workerWg.Done()
			s.workerLoop()
		}()
	}
	return s
}

func (s *RunService) Close() {
	if s == nil {
		return
	}
	s.workerCancel()
	s.queue.Close()
	s.workerWg.Wait()
}

func (s *RunService) Submit(ctx context.Context, req *RunRequest) (*RunResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: empty run request", ErrInvalid)
	}
	if strings.TrimSpace(req.WorkflowName) == "" && strings.TrimSpace(req.WorkflowYAML) == "" {
		return nil, fmt.Errorf("%w: workflow_name or workflow_yaml is required", ErrInvalid)
	}
	if strings.TrimSpace(req.WorkflowName) != "" && strings.TrimSpace(req.WorkflowYAML) != "" {
		return nil, fmt.Errorf("%w: workflow_name and workflow_yaml are mutually exclusive", ErrInvalid)
	}
	if key := strings.TrimSpace(req.IdempotencyKey); key != "" {
		s.mu.RLock()
		existingID, ok := s.idempotency[key]
		existingMeta, metaOK := s.metas[existingID]
		s.mu.RUnlock()
		if ok && metaOK {
			return &RunResponse{
				RunID:        existingID,
				Status:       existingMeta.Status,
				WorkflowName: existingMeta.WorkflowName,
				CreatedAt:    existingMeta.CreatedAt,
			}, nil
		}
	}

	wf, rawYAML, err := s.loadWorkflow(ctx, req)
	if err != nil {
		return nil, err
	}
	if wf.Vars == nil {
		wf.Vars = map[string]any{}
	}
	for key, value := range req.Vars {
		wf.Vars[key] = value
	}
	s.applyDefaultOutputLimit(&wf)
	if s.pre != nil {
		if err := s.pre.Process(ctx, &wf); err != nil {
			return nil, err
		}
	}

	runID := state.NewRunID()
	createdAt := time.Now().UTC()
	meta := RunMeta{
		RunID:          runID,
		WorkflowName:   wf.Name,
		WorkflowYAML:   string(rawYAML),
		Vars:           cloneAnyMap(req.Vars),
		TriggeredBy:    defaultTriggeredBy(req.TriggeredBy),
		IdempotencyKey: strings.TrimSpace(req.IdempotencyKey),
		CreatedAt:      createdAt,
		QueuedAt:       createdAt,
		Status:         state.RunStatusQueued,
		Summary:        buildRunSummary(state.RunStatusQueued, ""),
	}
	if err := s.recordStore.Upsert(ctx, meta); err != nil {
		return nil, err
	}
	s.rememberMeta(meta)

	job := &runJob{
		RunID:    runID,
		Workflow: wf,
		Meta:     meta,
	}

	s.mu.Lock()
	s.jobs[runID] = job
	s.mu.Unlock()

	if err := s.queue.Enqueue(ctx, queue.Job{RunID: runID}); err != nil {
		s.mu.Lock()
		delete(s.jobs, runID)
		s.mu.Unlock()
		s.forgetMeta(meta.RunID, meta.IdempotencyKey)
		_ = s.recordStore.Delete(context.Background(), meta.RunID)
		if err == queue.ErrQueueFull {
			return nil, ErrQueueFull
		}
		return nil, err
	}

	s.metrics.ObserveRunSubmitted()
	s.metrics.SetQueueDepth(s.queue.Len())
	s.publishEvent(runID, events.Event{
		Type:      "run_queued",
		RunID:     runID,
		Workflow:  wf.Name,
		Status:    state.RunStatusQueued,
		Message:   meta.Summary,
		Timestamp: createdAt,
	})
	logging.L().Info("run submitted",
		zap.String("run_id", runID),
		zap.String("workflow", wf.Name),
		zap.String("triggered_by", meta.TriggeredBy),
		zap.String("idempotency_key", meta.IdempotencyKey),
	)
	return &RunResponse{
		RunID:        runID,
		Status:       state.RunStatusQueued,
		WorkflowName: wf.Name,
		CreatedAt:    createdAt,
	}, nil
}

func (s *RunService) Get(ctx context.Context, runID string) (*RunDetail, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, fmt.Errorf("%w: run_id is required", ErrInvalid)
	}

	var runSnapshot *state.RunState
	run, err := s.runStore.GetRun(ctx, runID)
	if err == nil {
		cp := run
		runSnapshot = &cp
	} else if !errors.Is(err, state.ErrRunNotFound) {
		return nil, err
	}

	meta, ok := s.lookupMeta(runID)
	if !ok && runSnapshot == nil {
		return nil, ErrNotFound
	}
	if !ok && runSnapshot != nil {
		meta = synthesizeMetaFromRun(*runSnapshot)
	}
	if runSnapshot != nil {
		meta = enrichMetaWithRun(meta, *runSnapshot)
		s.persistMetaAsync(meta)
	}
	detail := buildRunDetail(meta, runSnapshot)
	return &detail, nil
}

func (s *RunService) List(ctx context.Context, filter RunFilter) ([]*RunMeta, error) {
	items, err := s.recordStore.List(ctx, RunFilter{})
	if err != nil {
		return nil, err
	}
	merged := map[string]RunMeta{}
	for _, item := range items {
		merged[item.RunID] = cloneRunMeta(item)
	}

	storeRuns, err := s.runStore.ListRuns(ctx, state.ListFilter{})
	if err != nil {
		return nil, err
	}
	for _, run := range storeRuns {
		meta := merged[run.RunID]
		if meta.RunID == "" {
			meta = synthesizeMetaFromRun(run)
		}
		meta = enrichMetaWithRun(meta, run)
		merged[run.RunID] = meta
		s.persistMetaAsync(meta)
	}

	selected := filterAndSortRunMetas(mapValues(merged), filter)
	out := make([]*RunMeta, 0, len(selected))
	for i := range selected {
		item := selected[i]
		out = append(out, &item)
	}
	return out, nil
}

func (s *RunService) Cancel(ctx context.Context, runID string) error {
	runID = strings.TrimSpace(runID)
	s.mu.Lock()
	job, ok := s.jobs[runID]
	if !ok {
		s.mu.Unlock()
		if _, err := s.runStore.GetRun(ctx, runID); err == nil {
			return nil
		}
		if _, metaOK := s.lookupMeta(runID); metaOK {
			return nil
		}
		return ErrNotFound
	}
	if job.CancelFunc == nil {
		job.Canceled = true
		meta := s.metas[runID]
		meta.Status = state.RunStatusCanceled
		meta.Message = "canceled before start"
		meta.Summary = buildRunSummary(meta.Status, meta.Message)
		meta.FinishedAt = time.Now().UTC()
		s.metas[runID] = meta
		s.mu.Unlock()
		s.persistMetaAsync(meta)
		s.publishEvent(runID, events.Event{
			Type:      "run_finish",
			RunID:     runID,
			Workflow:  meta.WorkflowName,
			Status:    state.RunStatusCanceled,
			Message:   meta.Message,
			Timestamp: time.Now().UTC(),
		})
		return nil
	}
	cancel := job.CancelFunc
	s.mu.Unlock()
	cancel()
	return nil
}

func (s *RunService) Subscribe(_ context.Context, runID string) (<-chan events.Event, func(), error) {
	sub, ok := s.events.Subscribe(strings.TrimSpace(runID))
	if !ok {
		return nil, nil, ErrNotFound
	}
	cancel := func() {
		s.events.Unsubscribe(sub)
	}
	return sub.C, cancel, nil
}

func (s *RunService) History(ctx context.Context, runID string) ([]events.Event, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, fmt.Errorf("%w: run_id is required", ErrInvalid)
	}
	if s.eventStore != nil {
		items, err := s.eventStore.List(ctx, runID)
		if err != nil {
			return nil, err
		}
		if len(items) > 0 {
			return items, nil
		}
	}
	detail, err := s.Get(ctx, runID)
	if err != nil {
		return nil, err
	}
	return synthesizeRunHistory(*detail), nil
}

func (s *RunService) loadWorkflow(ctx context.Context, req *RunRequest) (workflow.Workflow, []byte, error) {
	if strings.TrimSpace(req.WorkflowYAML) != "" {
		raw := []byte(req.WorkflowYAML)
		wf, err := workflow.Load(raw)
		if err != nil {
			return workflow.Workflow{}, nil, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
		if err := wf.Validate(); err != nil {
			return workflow.Workflow{}, nil, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
		return wf, raw, nil
	}
	if s.workflowSvc == nil {
		return workflow.Workflow{}, nil, fmt.Errorf("%w: workflow service is not configured", ErrUnavailable)
	}
	record, err := s.workflowSvc.Get(ctx, req.WorkflowName)
	if err != nil {
		return workflow.Workflow{}, nil, err
	}
	wf, err := workflow.Load(record.RawYAML)
	if err != nil {
		return workflow.Workflow{}, nil, err
	}
	if err := wf.Validate(); err != nil {
		return workflow.Workflow{}, nil, err
	}
	return wf, append([]byte{}, record.RawYAML...), nil
}

func (s *RunService) workerLoop() {
	for {
		job, err := s.queue.Dequeue(s.workerCtx)
		if err != nil {
			return
		}
		s.metrics.SetQueueDepth(s.queue.Len())
		s.processJob(job.RunID)
	}
}

func (s *RunService) processJob(runID string) {
	s.mu.Lock()
	job, ok := s.jobs[runID]
	if !ok {
		s.mu.Unlock()
		return
	}
	if job.Canceled {
		now := time.Now().UTC()
		meta := s.metas[runID]
		meta.Status = state.RunStatusCanceled
		meta.FinishedAt = now
		if meta.Message == "" {
			meta.Message = "canceled before start"
		}
		meta.Summary = buildRunSummary(meta.Status, meta.Message)
		s.metas[runID] = meta
		delete(s.jobs, runID)
		s.mu.Unlock()

		_ = s.runStore.CreateRun(context.Background(), state.RunState{
			RunID:        runID,
			WorkflowName: meta.WorkflowName,
			Status:       state.RunStatusCanceled,
			Message:      meta.Message,
			StartedAt:    meta.CreatedAt,
			FinishedAt:   now,
			UpdatedAt:    now,
			Version:      1,
		})
		s.persistMetaAsync(meta)
		s.metrics.ObserveRunFinished(state.RunStatusCanceled, 0)
		s.publishEvent(runID, events.Event{
			Type:      "run_finish",
			RunID:     runID,
			Workflow:  meta.WorkflowName,
			Status:    state.RunStatusCanceled,
			Message:   meta.Message,
			Timestamp: now,
		})
		return
	}

	ctx, cancel := context.WithCancel(s.workerCtx)
	job.CancelFunc = cancel
	meta := s.metas[runID]
	meta.Status = state.RunStatusRunning
	meta.StartedAt = time.Now().UTC()
	meta.Summary = buildRunSummary(meta.Status, "")
	s.metas[runID] = meta
	s.mu.Unlock()

	s.persistMetaAsync(meta)

	start := time.Now().UTC()
	s.metrics.ObserveRunStarted()
	s.publishEvent(runID, events.Event{
		Type:      "run_start",
		RunID:     runID,
		Workflow:  job.Workflow.Name,
		Status:    state.RunStatusRunning,
		Message:   meta.Summary,
		Timestamp: start,
	})
	logging.L().Info("run started",
		zap.String("run_id", runID),
		zap.String("workflow", job.Workflow.Name),
	)

	eng := engine.New()
	if strings.TrimSpace(s.agentDispatchToken) != "" {
		dispatcher := scheduler.NewHybridDispatcher(eng.Registry)
		dispatcher.Token = s.agentDispatchToken
		eng.SetDispatcher(dispatcher)
	}
	eng.RunStore = s.runStore
	recorder := &runEventRecorder{
		runID:    runID,
		workflow: job.Workflow.Name,
		publish: func(evt events.Event) {
			s.publishEvent(runID, evt)
		},
	}
	runCtx := engine.WithRecorder(ctx, recorder)
	snapshot, execErr := eng.ApplyWithRun(runCtx, job.Workflow, engine.RunOptions{
		RunID: runID,
		Store: s.runStore,
	})
	finished := time.Now().UTC()
	status := snapshot.Status
	if strings.TrimSpace(status) == "" {
		if execErr != nil && ctx.Err() != nil {
			status = state.RunStatusCanceled
		} else if execErr != nil {
			status = state.RunStatusFailed
		} else {
			status = state.RunStatusSuccess
		}
	}
	message := snapshot.Message
	if message == "" && execErr != nil {
		message = execErr.Error()
	}

	s.mu.Lock()
	meta = s.metas[runID]
	meta.Status = status
	meta.Message = message
	meta.FinishedAt = finished
	meta.Summary = buildRunSummary(status, message)
	s.metas[runID] = meta
	delete(s.jobs, runID)
	s.mu.Unlock()
	cancel()

	s.persistMetaAsync(enrichMetaWithRun(meta, snapshot))
	s.metrics.ObserveRunFinished(status, finished.Sub(start))
	s.publishEvent(runID, events.Event{
		Type:      "run_finish",
		RunID:     runID,
		Workflow:  job.Workflow.Name,
		Status:    status,
		Message:   message,
		Timestamp: finished,
	})
	logging.L().Info("run finished",
		zap.String("run_id", runID),
		zap.String("workflow", job.Workflow.Name),
		zap.String("status", status),
		zap.String("message", message),
		zap.String("failed_step", latestFailedStep(snapshot)),
		zap.String("failed_host", latestFailedHost(snapshot)),
		zap.String("failed_address", latestFailedResolvedAddress(snapshot)),
	)
}

func (s *RunService) applyDefaultOutputLimit(wf *workflow.Workflow) {
	if wf == nil {
		return
	}
	for i := range wf.Steps {
		step := &wf.Steps[i]
		if step.Args == nil {
			step.Args = map[string]any{}
		}
		if _, ok := step.Args["max_output_bytes"]; !ok {
			step.Args["max_output_bytes"] = s.maxOutputBytes
		}
	}
	for i := range wf.Handlers {
		handler := &wf.Handlers[i]
		if handler.Args == nil {
			handler.Args = map[string]any{}
		}
		if _, ok := handler.Args["max_output_bytes"]; !ok {
			handler.Args["max_output_bytes"] = s.maxOutputBytes
		}
	}
}

func (s *RunService) restoreRecords(ctx context.Context) {
	items, err := s.recordStore.List(ctx, RunFilter{})
	if err == nil {
		for _, item := range items {
			s.rememberMeta(item)
		}
	}

	storeRuns, err := s.runStore.ListRuns(ctx, state.ListFilter{})
	if err != nil {
		return
	}
	for _, run := range storeRuns {
		meta, ok := s.lookupMeta(run.RunID)
		if !ok {
			meta = synthesizeMetaFromRun(run)
		}
		meta = enrichMetaWithRun(meta, run)
		s.rememberMeta(meta)
		_ = s.recordStore.Upsert(ctx, meta)
	}
}

func (s *RunService) lookupMeta(runID string) (RunMeta, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	meta, ok := s.metas[strings.TrimSpace(runID)]
	return meta, ok
}

func (s *RunService) rememberMeta(meta RunMeta) {
	s.mu.Lock()
	defer s.mu.Unlock()
	meta = cloneRunMeta(meta)
	if meta.Summary == "" {
		meta.Summary = buildRunSummary(meta.Status, meta.Message)
	}
	s.metas[meta.RunID] = meta
	if meta.IdempotencyKey != "" {
		s.idempotency[meta.IdempotencyKey] = meta.RunID
	}
}

func (s *RunService) forgetMeta(runID, idempotencyKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.metas, strings.TrimSpace(runID))
	if strings.TrimSpace(idempotencyKey) != "" {
		delete(s.idempotency, strings.TrimSpace(idempotencyKey))
	}
}

func (s *RunService) persistMetaAsync(meta RunMeta) {
	s.rememberMeta(meta)
	if err := s.recordStore.Upsert(context.Background(), meta); err != nil {
		logging.L().Warn("persist run meta failed",
			zap.String("run_id", meta.RunID),
			zap.Error(err),
		)
	}
}

func (s *RunService) publishEvent(runID string, evt events.Event) {
	if s.eventStore != nil {
		if err := s.eventStore.Append(context.Background(), evt); err != nil {
			logging.L().Warn("append run event failed",
				zap.String("run_id", runID),
				zap.Error(err),
			)
		}
	}
	s.events.Publish(runID, evt)
}

func buildRunDetail(meta RunMeta, run *state.RunState) RunDetail {
	detail := RunDetail{
		RunMeta: cloneRunMeta(meta),
	}
	if detail.TriggeredBy == "" {
		detail.TriggeredBy = "system"
	}
	if detail.Summary == "" {
		detail.Summary = buildRunSummary(detail.Status, detail.Message)
	}
	if run == nil {
		return detail
	}
	detail.WorkflowVersion = run.WorkflowVersion
	detail.LastError = run.LastError
	detail.InterruptedReason = run.InterruptedReason
	detail.LastNotifyError = run.LastNotifyError
	detail.Version = run.Version
	detail.UpdatedAt = run.UpdatedAt
	detail.Args = cloneAnyMap(run.Args)
	detail.Steps = append([]state.StepState{}, run.Steps...)
	if len(run.Resources) > 0 {
		detail.Resources = make(map[string]state.ResourceState, len(run.Resources))
		for key, value := range run.Resources {
			detail.Resources[key] = value
		}
	}
	return detail
}

func synthesizeMetaFromRun(run state.RunState) RunMeta {
	createdAt := run.StartedAt
	if createdAt.IsZero() {
		createdAt = run.UpdatedAt
	}
	return RunMeta{
		RunID:        run.RunID,
		WorkflowName: run.WorkflowName,
		CreatedAt:    createdAt,
		QueuedAt:     createdAt,
		StartedAt:    run.StartedAt,
		FinishedAt:   run.FinishedAt,
		Status:       run.Status,
		Message:      run.Message,
		Summary:      buildRunSummary(run.Status, run.Message),
	}
}

func enrichMetaWithRun(meta RunMeta, run state.RunState) RunMeta {
	if meta.RunID == "" {
		meta = synthesizeMetaFromRun(run)
	}
	if meta.WorkflowName == "" {
		meta.WorkflowName = run.WorkflowName
	}
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = run.StartedAt
	}
	if meta.QueuedAt.IsZero() {
		meta.QueuedAt = meta.CreatedAt
	}
	meta.Status = run.Status
	meta.Message = run.Message
	if !run.StartedAt.IsZero() {
		meta.StartedAt = run.StartedAt
	}
	if !run.FinishedAt.IsZero() {
		meta.FinishedAt = run.FinishedAt
	}
	meta.Summary = buildRunSummary(meta.Status, meta.Message)
	return meta
}

func buildRunSummary(status, message string) string {
	if strings.TrimSpace(message) != "" {
		return strings.TrimSpace(message)
	}
	switch strings.TrimSpace(status) {
	case state.RunStatusQueued:
		return "任务已进入队列"
	case state.RunStatusRunning:
		return "任务正在执行"
	case state.RunStatusSuccess:
		return "任务执行成功"
	case state.RunStatusFailed:
		return "任务执行失败"
	case state.RunStatusCanceled:
		return "任务已取消"
	case state.RunStatusInterrupted:
		return "任务因服务重启中断"
	default:
		return ""
	}
}

func defaultTriggeredBy(triggeredBy string) string {
	trimmed := strings.TrimSpace(triggeredBy)
	if trimmed == "" {
		return "system"
	}
	return trimmed
}

func synthesizeRunHistory(run RunDetail) []events.Event {
	timestamp := run.CreatedAt
	if timestamp.IsZero() {
		timestamp = run.StartedAt
	}
	if timestamp.IsZero() {
		timestamp = run.UpdatedAt
	}
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}

	items := make([]events.Event, 0, len(run.Steps)*3+2)
	if run.Status == state.RunStatusQueued {
		items = append(items, events.Event{
			Type:      "run_queued",
			RunID:     run.RunID,
			Workflow:  run.WorkflowName,
			Status:    run.Status,
			Message:   run.Summary,
			Timestamp: timestamp,
		})
		return items
	}

	items = append(items, events.Event{
		Type:      "run_start",
		RunID:     run.RunID,
		Workflow:  run.WorkflowName,
		Status:    run.Status,
		Message:   "run accepted by scheduler",
		Timestamp: timestamp,
	})

	for _, step := range run.Steps {
		stepStart := step.StartedAt
		if stepStart.IsZero() {
			stepStart = timestamp
		}
		items = append(items, events.Event{
			Type:      "step_start",
			RunID:     run.RunID,
			Workflow:  run.WorkflowName,
			Step:      step.Name,
			Status:    step.Status,
			Message:   step.Message,
			Timestamp: stepStart,
		})

		for hostName, host := range step.Hosts {
			hostTime := host.FinishedAt
			if hostTime.IsZero() {
				hostTime = host.StartedAt
			}
			if hostTime.IsZero() {
				hostTime = stepStart
			}
			items = append(items, events.Event{
				Type:      "host_result",
				RunID:     run.RunID,
				Workflow:  run.WorkflowName,
				Step:      step.Name,
				Host:      hostName,
				Status:    host.Status,
				Message:   host.Message,
				Output:    host.Output,
				Timestamp: hostTime,
			})
		}

		stepFinish := step.FinishedAt
		if stepFinish.IsZero() {
			stepFinish = stepStart
		}
		items = append(items, events.Event{
			Type:      "step_finish",
			RunID:     run.RunID,
			Workflow:  run.WorkflowName,
			Step:      step.Name,
			Status:    step.Status,
			Message:   step.Message,
			Timestamp: stepFinish,
		})
	}

	finishedAt := run.FinishedAt
	if finishedAt.IsZero() {
		finishedAt = run.UpdatedAt
	}
	if finishedAt.IsZero() {
		finishedAt = timestamp
	}
	items = append(items, events.Event{
		Type:      "run_finish",
		RunID:     run.RunID,
		Workflow:  run.WorkflowName,
		Status:    run.Status,
		Message:   run.Message,
		Timestamp: finishedAt,
	})
	return items
}

func cloneAnyMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

type runEventRecorder struct {
	runID    string
	workflow string
	publish  func(events.Event)
}

func (r *runEventRecorder) StepStart(step workflow.Step, _ []workflow.HostSpec) {
	if r.publish == nil {
		return
	}
	r.publish(events.Event{
		Type:      "step_start",
		RunID:     r.runID,
		Workflow:  r.workflow,
		Step:      step.Name,
		Status:    state.RunStatusRunning,
		Message:   fmt.Sprintf("step %s started", strings.TrimSpace(step.Name)),
		Timestamp: time.Now().UTC(),
	})
}

func (r *runEventRecorder) StepFinish(step workflow.Step, status string) {
	if r.publish == nil {
		return
	}
	r.publish(events.Event{
		Type:      "step_finish",
		RunID:     r.runID,
		Workflow:  r.workflow,
		Step:      step.Name,
		Status:    strings.TrimSpace(status),
		Message:   fmt.Sprintf("step %s finished with status=%s", strings.TrimSpace(step.Name), strings.TrimSpace(status)),
		Timestamp: time.Now().UTC(),
	})
}

func (r *runEventRecorder) HostResult(step workflow.Step, host workflow.HostSpec, result scheduler.Result) {
	if r.publish == nil {
		return
	}
	message := strings.TrimSpace(result.Error)
	if message == "" {
		message = fmt.Sprintf("host %s finished with status=%s", strings.TrimSpace(host.Name), strings.TrimSpace(result.Status))
	}
	if address := extractRunnerDebugString(result.Output, "resolved_address"); address != "" {
		message = fmt.Sprintf("%s (address=%s)", message, address)
	}
	r.publish(events.Event{
		Type:      "host_result",
		RunID:     r.runID,
		Workflow:  r.workflow,
		Step:      step.Name,
		Host:      host.Name,
		Status:    strings.TrimSpace(result.Status),
		Message:   message,
		Output:    cloneAnyMap(result.Output),
		Timestamp: time.Now().UTC(),
	})
}

func latestFailedStep(run state.RunState) string {
	for i := len(run.Steps) - 1; i >= 0; i-- {
		step := run.Steps[i]
		if strings.EqualFold(strings.TrimSpace(step.Status), state.RunStatusFailed) {
			return step.Name
		}
		for _, host := range step.Hosts {
			if strings.EqualFold(strings.TrimSpace(host.Status), state.RunStatusFailed) {
				return step.Name
			}
		}
	}
	return ""
}

func latestFailedHost(run state.RunState) string {
	for i := len(run.Steps) - 1; i >= 0; i-- {
		step := run.Steps[i]
		for hostName, host := range step.Hosts {
			if strings.EqualFold(strings.TrimSpace(host.Status), state.RunStatusFailed) {
				return hostName
			}
		}
	}
	return ""
}

func latestFailedResolvedAddress(run state.RunState) string {
	for i := len(run.Steps) - 1; i >= 0; i-- {
		step := run.Steps[i]
		for _, host := range step.Hosts {
			if !strings.EqualFold(strings.TrimSpace(host.Status), state.RunStatusFailed) {
				continue
			}
			if address := extractRunnerDebugString(host.Output, "resolved_address"); address != "" {
				return address
			}
		}
	}
	return ""
}

func extractRunnerDebugString(output map[string]any, key string) string {
	if len(output) == 0 {
		return ""
	}
	raw, ok := output["runner_debug"]
	if !ok || raw == nil {
		return ""
	}
	debug, ok := raw.(map[string]any)
	if !ok {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(debug[key]))
}
