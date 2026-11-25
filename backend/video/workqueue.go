package video

import "sync"

// workQueue is an in-memory FIFO queue with one blocking consumer and no
// admission limit.
//
// It deliberately knows nothing about the items: the durable truth for 精彩时刻 is
// the video_highlights table, so this queue only carries ids and is allowed to be
// lost at any moment. That is also why there is no capacity or drop policy — a
// library-wide burst must not silently discard work the database says exists.
//
// (It mirrors backend/jav's queue of the same name; the two packages keep their
// own copy because the type is unexported there and the queue is a dozen lines.)
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

// push appends an item, reporting false when the queue has been closed. A false
// answer means "nobody will consume this", which callers treat as "the row stays
// queued for the next start".
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

// pop waits for the next item. It reports false once the queue is closed and
// empty, which is how the worker ends.
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

// close ends the queue for producers and wakes the consumer. Items already
// queued are dropped on purpose: a closing window must not start another ffmpeg
// run, and their rows are recovered by the next start.
func (q *workQueue[T]) close() {
	q.mu.Lock()
	q.closed = true
	q.cond.Broadcast()
	q.mu.Unlock()
}
