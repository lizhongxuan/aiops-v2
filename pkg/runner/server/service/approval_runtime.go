package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"runner/executor"
	"runner/state"
	"runner/workflow"
)

const approvalResolveRegistrationWait = 500 * time.Millisecond

type approvalCoordinator struct {
	mu      sync.Mutex
	pending map[string]map[string]*approvalRequest
}

type approvalRequest struct {
	runID    string
	nodeID   string
	decision chan executor.ApprovalDecision
}

type runApprovalRuntime struct {
	coordinator *approvalCoordinator
	runID       string
}

func newApprovalCoordinator() *approvalCoordinator {
	return &approvalCoordinator{pending: map[string]map[string]*approvalRequest{}}
}

func (c *approvalCoordinator) Runtime(runID string) executor.ApprovalRuntime {
	return &runApprovalRuntime{coordinator: c, runID: strings.TrimSpace(runID)}
}

func (r *runApprovalRuntime) WaitForApproval(ctx context.Context, wf workflow.Workflow, node workflow.GraphNodeSpec) (executor.ApprovalDecision, error) {
	if r == nil || r.coordinator == nil {
		return executor.ApprovalDecision{}, fmt.Errorf("approval coordinator is not configured")
	}
	return r.coordinator.wait(ctx, r.runID, wf, node)
}

func (c *approvalCoordinator) wait(ctx context.Context, runID string, _ workflow.Workflow, node workflow.GraphNodeSpec) (executor.ApprovalDecision, error) {
	runID = strings.TrimSpace(runID)
	nodeID := strings.TrimSpace(node.ID)
	if runID == "" || nodeID == "" {
		return executor.ApprovalDecision{}, fmt.Errorf("approval run_id and node_id are required")
	}
	req := &approvalRequest{
		runID:    runID,
		nodeID:   nodeID,
		decision: make(chan executor.ApprovalDecision, 1),
	}
	if err := c.register(req); err != nil {
		return executor.ApprovalDecision{}, err
	}
	defer c.unregister(runID, nodeID, req)

	timeout, err := approvalTimeout(node)
	if err != nil {
		return executor.ApprovalDecision{}, err
	}
	if timeout <= 0 {
		select {
		case decision := <-req.decision:
			return normalizeServiceApprovalDecision(decision), nil
		case <-ctx.Done():
			return executor.ApprovalDecision{}, ctx.Err()
		}
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case decision := <-req.decision:
		return normalizeServiceApprovalDecision(decision), nil
	case <-timer.C:
		return approvalTimeoutDecision(node, timeout), nil
	case <-ctx.Done():
		return executor.ApprovalDecision{}, ctx.Err()
	}
}

func (c *approvalCoordinator) register(req *approvalRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pending == nil {
		c.pending = map[string]map[string]*approvalRequest{}
	}
	if c.pending[req.runID] == nil {
		c.pending[req.runID] = map[string]*approvalRequest{}
	}
	if _, exists := c.pending[req.runID][req.nodeID]; exists {
		return fmt.Errorf("%w: approval node %s/%s is already waiting", ErrConflict, req.runID, req.nodeID)
	}
	c.pending[req.runID][req.nodeID] = req
	return nil
}

func (c *approvalCoordinator) unregister(runID, nodeID string, req *approvalRequest) {
	c.mu.Lock()
	defer c.mu.Unlock()
	nodes := c.pending[runID]
	if nodes == nil {
		return
	}
	if nodes[nodeID] == req {
		delete(nodes, nodeID)
	}
	if len(nodes) == 0 {
		delete(c.pending, runID)
	}
}

func (c *approvalCoordinator) Resolve(ctx context.Context, runID, nodeID string, decision executor.ApprovalDecision) error {
	runID = strings.TrimSpace(runID)
	nodeID = strings.TrimSpace(nodeID)
	if runID == "" || nodeID == "" {
		return fmt.Errorf("%w: run_id and node_id are required", ErrInvalid)
	}
	deadline := time.NewTimer(approvalResolveRegistrationWait)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		req := c.lookup(runID, nodeID)
		if req != nil {
			select {
			case req.decision <- normalizeServiceApprovalDecision(decision):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			default:
				return fmt.Errorf("%w: approval node %s/%s is already resolved", ErrConflict, runID, nodeID)
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("%w: approval node %s/%s is not waiting", ErrNotFound, runID, nodeID)
		case <-ticker.C:
		}
	}
}

func (c *approvalCoordinator) lookup(runID, nodeID string) *approvalRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pending == nil || c.pending[runID] == nil {
		return nil
	}
	return c.pending[runID][nodeID]
}

func (s *RunService) ApproveNode(ctx context.Context, runID, nodeID, actor, comment string) error {
	if s == nil || s.approvals == nil {
		return fmt.Errorf("%w: run service is not configured", ErrUnavailable)
	}
	return s.approvals.Resolve(ctx, runID, nodeID, executor.ApprovalDecision{
		Status:  state.RunStatusSuccess,
		Actor:   strings.TrimSpace(actor),
		Comment: strings.TrimSpace(comment),
	})
}

func (s *RunService) RejectNode(ctx context.Context, runID, nodeID, actor, comment string) error {
	if s == nil || s.approvals == nil {
		return fmt.Errorf("%w: run service is not configured", ErrUnavailable)
	}
	return s.approvals.Resolve(ctx, runID, nodeID, executor.ApprovalDecision{
		Status:  state.RunStatusFailed,
		Actor:   strings.TrimSpace(actor),
		Comment: strings.TrimSpace(comment),
	})
}

func approvalTimeout(node workflow.GraphNodeSpec) (time.Duration, error) {
	if node.Data.Approval == nil || strings.TrimSpace(node.Data.Approval.Timeout) == "" {
		return 0, nil
	}
	timeout, err := time.ParseDuration(strings.TrimSpace(node.Data.Approval.Timeout))
	if err != nil {
		return 0, fmt.Errorf("%w: invalid approval timeout for node %q: %v", ErrInvalid, strings.TrimSpace(node.ID), err)
	}
	return timeout, nil
}

func approvalTimeoutDecision(node workflow.GraphNodeSpec, timeout time.Duration) executor.ApprovalDecision {
	status := state.RunStatusFailed
	if node.Data.Approval != nil {
		switch strings.TrimSpace(strings.ToLower(node.Data.Approval.OnTimeout)) {
		case "approve", "approved", state.RunStatusSuccess:
			status = state.RunStatusSuccess
		}
	}
	return executor.ApprovalDecision{
		Status:  status,
		Actor:   "system",
		Comment: fmt.Sprintf("approval timed out after %s", timeout),
	}
}

func normalizeServiceApprovalDecision(decision executor.ApprovalDecision) executor.ApprovalDecision {
	decision.Status = strings.TrimSpace(strings.ToLower(decision.Status))
	decision.Actor = strings.TrimSpace(decision.Actor)
	decision.Comment = strings.TrimSpace(decision.Comment)
	return decision
}
