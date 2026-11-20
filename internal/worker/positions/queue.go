package positions

import (
	"context"
	"slices"
	"sync"

	"go.opentelemetry.io/otel/trace"

	"github.com/saywich/tripgo/internal/domain/trip"
)

type enqueueResult string

const (
	queued    enqueueResult = "queued"
	duplicate enqueueResult = "duplicate"
	dropped   enqueueResult = "dropped"
)

type task struct {
	trip    trip.StaleTrip
	source  trace.SpanContext
	started bool
}

type queue struct {
	mu      sync.Mutex
	pending map[trip.ID]*task
	buffer  buffer
	ready   chan struct{}
	closed  chan struct{}
}

func newQueue(capacity int, priority bool) *queue {
	var tasks buffer = make(fifo, capacity)
	if priority {
		tasks = &priorityBuffer{tasks: make([]*task, 0, capacity)}
	}

	return &queue{
		pending: make(map[trip.ID]*task),
		buffer:  tasks,
		ready:   make(chan struct{}, 1),
		closed:  make(chan struct{}),
	}
}

func (q *queue) offer(t *task) (enqueueResult, *task) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.pending[t.trip.ID] != nil {
		return duplicate, nil
	}

	ok, evicted := q.buffer.offer(t)
	if !ok {
		return dropped, nil
	}

	if evicted != nil {
		delete(q.pending, evicted.trip.ID)
	}

	q.pending[t.trip.ID] = t
	q.wake()
	
	return queued, evicted
}

func (q *queue) wake() {
	select {
	case q.ready <- struct{}{}:
	default:
	}
}

func (q *queue) close() {
	q.mu.Lock()
	defer q.mu.Unlock()

	close(q.closed)
}

func (q *queue) next(ctx context.Context) *task {
	for ctx.Err() == nil {
		if t := q.take(); t != nil {
			return t
		}

		select {
		case <-ctx.Done():
			return nil
		case <-q.closed:
			return q.take()
		case <-q.ready:
		}
	}

	return nil
}

func (q *queue) take() *task {
	q.mu.Lock()
	defer q.mu.Unlock()

	for q.buffer.len() > 0 {
		t := q.buffer.pop()
		if q.pending[t.trip.ID] == t {
			t.started = true

			if q.buffer.len() > 0 {
				q.wake()
			}

			return t
		}
	}

	return nil
}

func (q *queue) cancel(id trip.ID) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if t := q.pending[id]; t != nil && !t.started {
		q.buffer.remove(t)
		delete(q.pending, id)
	}
}

func (q *queue) finish(t *task) {
	q.mu.Lock()
	defer q.mu.Unlock()

	delete(q.pending, t.trip.ID)
}

func (q *queue) size() int64 {
	q.mu.Lock()
	defer q.mu.Unlock()

	return int64(q.buffer.len())
}

type buffer interface {
	offer(*task) (bool, *task)
	pop() *task
	remove(*task)
	len() int
}

type fifo chan *task

func (f fifo) offer(t *task) (bool, *task) {
	select {
	case f <- t:
		return true, nil
	default:
		return false, nil
	}
}

func (f fifo) pop() *task   { return <-f }
func (f fifo) remove(*task) {}
func (f fifo) len() int     { return len(f) }

type priorityBuffer struct {
	tasks []*task
}

func (p *priorityBuffer) offer(t *task) (bool, *task) {
	var evicted *task
	if len(p.tasks) == cap(p.tasks) {
		last := len(p.tasks) - 1
		if !t.trip.Since.Before(p.tasks[last].trip.Since) {
			return false, nil
		}

		evicted = p.tasks[last]
		p.tasks = p.tasks[:last]
	}

	at, _ := slices.BinarySearchFunc(p.tasks, t, func(left, right *task) int {
		return left.trip.Since.Compare(right.trip.Since)
	})

	p.tasks = slices.Insert(p.tasks, at, t)
	return true, evicted
}

func (p *priorityBuffer) pop() *task {
	t := p.tasks[0]
	p.tasks = slices.Delete(p.tasks, 0, 1)
	return t
}

func (p *priorityBuffer) remove(t *task) {
	if at := slices.Index(p.tasks, t); at >= 0 {
		p.tasks = slices.Delete(p.tasks, at, at+1)
	}
}

func (p *priorityBuffer) len() int { return len(p.tasks) }
