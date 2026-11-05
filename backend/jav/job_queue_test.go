package jav

import "testing"

func TestJobQueueDoesNotDropLargeRecoveryBatch(t *testing.T) {
	q := newJobQueue()
	const total = 300
	for i := 0; i < total; i++ {
		if !q.push(job{id: int64(i + 1)}) {
			t.Fatalf("push(%d) failed", i)
		}
	}

	for i := 0; i < total; i++ {
		got, ok := q.pop()
		if !ok {
			t.Fatalf("pop(%d) returned closed/empty", i)
		}
		if got.id != int64(i+1) {
			t.Fatalf("pop(%d) id=%d, want %d", i, got.id, i+1)
		}
	}
	q.close()
	if _, ok := q.pop(); ok {
		t.Fatal("pop after close returned a job")
	}
}
