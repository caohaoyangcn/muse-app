package jav

import "sync"

// jobQueue is an in-memory FIFO queue with bounded worker consumption but no
// artificial admission limit. Persisted task rows are the durable source of
// truth, so enqueueing must not silently drop work when a library-wide scrape
// is larger than a fixed channel capacity.
type jobQueue struct {
	mu     sync.Mutex
	cond   *sync.Cond
	jobs   []job
	closed bool
}

func newJobQueue() *jobQueue {
	q := &jobQueue{}
	q.cond = sync.NewCond(&q.mu)
	return q
}

func (q *jobQueue) push(j job) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return false
	}
	q.jobs = append(q.jobs, j)
	q.cond.Signal()
	return true
}

func (q *jobQueue) pop() (job, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.jobs) == 0 && !q.closed {
		q.cond.Wait()
	}
	if len(q.jobs) == 0 {
		return job{}, false
	}
	j := q.jobs[0]
	q.jobs[0] = job{}
	q.jobs = q.jobs[1:]
	return j, true
}

func (q *jobQueue) close() {
	q.mu.Lock()
	q.closed = true
	q.cond.Broadcast()
	q.mu.Unlock()
}
