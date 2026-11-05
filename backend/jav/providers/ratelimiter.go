package providers

import (
	"context"
	"sync"
	"time"
)

const (
	javBusRequestsPerMinute = 5
	javBusRequestInterval   = time.Minute / javBusRequestsPerMinute
)

type requestLimiter interface {
	Wait(context.Context) error
}

type intervalLimiter struct {
	mu       sync.Mutex
	interval time.Duration
	next     time.Time
}

func newIntervalLimiter(interval time.Duration) *intervalLimiter {
	return &intervalLimiter{interval: interval}
}

func (l *intervalLimiter) Wait(ctx context.Context) error {
	if l == nil || l.interval <= 0 {
		return nil
	}

	now := time.Now()
	l.mu.Lock()
	scheduled := now
	if l.next.After(scheduled) {
		scheduled = l.next
	}
	l.next = scheduled.Add(l.interval)
	l.mu.Unlock()

	if delay := time.Until(scheduled); delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

var globalJavBusRequestLimiter requestLimiter = newIntervalLimiter(javBusRequestInterval)
