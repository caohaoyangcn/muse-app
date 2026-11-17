package jav

import "sync"

// workQueue is an in-memory FIFO queue with bounded worker consumption but no
// artificial admission limit. Persisted rows (scrape tasks, video records) are
// the durable source of truth, so enqueueing must not silently drop work when a
// library-wide pass is larger than a fixed channel capacity.
type workQueue[T any] struct {
	mu     sync.Mutex
	cond   *sync.Cond
	items  []T
	closed bool
}

func newWorkQueue[T any]() *workQueue[T] {
	q := &workQueue[T]{}
	q.cond = sync.NewCond(&q.mu)
	return q
}

func (q *workQueue[T]) push(item T) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return false
	}
	q.items = append(q.items, item)
	q.cond.Signal()
	return true
}

func (q *workQueue[T]) pop() (T, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.items) == 0 && !q.closed {
		q.cond.Wait()
	}
	if len(q.items) == 0 {
		var zero T
		return zero, false
	}
	item := q.items[0]
	var zero T
	q.items[0] = zero
	q.items = q.items[1:]
	return item, true
}

func (q *workQueue[T]) close() {
	q.mu.Lock()
	q.closed = true
	q.cond.Broadcast()
	q.mu.Unlock()
}

// jobQueue is the scrape-job flavour of workQueue: one queued video id plus
// whether the user forced a re-scrape.
type jobQueue = workQueue[job]

func newJobQueue() *jobQueue { return newWorkQueue[job]() }
