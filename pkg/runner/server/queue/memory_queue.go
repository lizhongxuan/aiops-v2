package queue

import (
	"context"
	"sync/atomic"
)

type MemoryQueue struct {
	ch     chan Job
	closed atomic.Bool
}

func NewMemoryQueue(size int) *MemoryQueue {
	if size <= 0 {
		size = 1
	}
	return &MemoryQueue{
		ch: make(chan Job, size),
	}
}

func (q *MemoryQueue) Enqueue(ctx context.Context, job Job) error {
	if q == nil {
		return ErrQueueClosed
	}
	if q.closed.Load() {
		return ErrQueueClosed
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	select {
	case q.ch <- job:
		return nil
	default:
		return ErrQueueFull
	}
}

func (q *MemoryQueue) Dequeue(ctx context.Context) (Job, error) {
	if q == nil {
		return Job{}, ErrQueueClosed
	}
	select {
	case <-ctx.Done():
		return Job{}, ctx.Err()
	case job, ok := <-q.ch:
		if !ok {
			return Job{}, ErrQueueClosed
		}
		return job, nil
	}
}

func (q *MemoryQueue) Len() int {
	if q == nil {
		return 0
	}
	return len(q.ch)
}

func (q *MemoryQueue) Close() {
	if q == nil {
		return
	}
	if q.closed.CompareAndSwap(false, true) {
		close(q.ch)
	}
}
