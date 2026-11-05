package jav

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// fakeActorResolver answers from a fixed table. A name that is not listed
// reports not_found, like a provider that does not know the performer.
type fakeActorResolver struct {
	profiles map[string]*ActorProfile
	errs     map[string]error
	calls    atomic.Int32
}

func (r *fakeActorResolver) ResolveActor(_ context.Context, name string) (*ActorProfile, error) {
	r.calls.Add(1)
	if err, ok := r.errs[name]; ok {
		return nil, err
	}
	if profile, ok := r.profiles[name]; ok {
		return profile, nil
	}
	return nil, NewLookupError("not_found", 404, nil)
}

func waitForFill(t *testing.T, s *Service, want func(ActorFillStatus) bool) ActorFillStatus {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		status, err := s.ActorFillStatus()
		if err != nil {
			t.Fatal(err)
		}
		if want(status) {
			return status
		}
		time.Sleep(5 * time.Millisecond)
	}
	status, _ := s.ActorFillStatus()
	t.Fatalf("backfill did not reach the expected state: %+v", status)
	return status
}

// The backfill resolves the names that have no profile and leaves the ones the
// provider does not know in a terminal state instead of retrying them forever.
func TestStartActorFillResolvesMissingProfiles(t *testing.T) {
	svc, _, _ := newActorFixture(t, [][]string{{"A"}, {"B"}, {"C"}})
	svc.SetActorResolver(&fakeActorResolver{profiles: map[string]*ActorProfile{
		"A": {Name: "A", Provider: "fixture", StarID: "star-a", Birthday: "1998-03-31"},
		"C": {Name: "C", Provider: "fixture", StarID: "star-c"},
	}})
	svc.Start()

	queued, err := svc.StartActorFill()
	if err != nil {
		t.Fatal(err)
	}
	if queued != 3 {
		t.Fatalf("queued=%d, want 3", queued)
	}
	status := waitForFill(t, svc, func(s ActorFillStatus) bool {
		return s.Succeeded == 2 && s.NotFound == 1 && !s.Active
	})
	if status.Total != 3 || status.Failed != 0 {
		t.Fatalf("status=%+v", status)
	}

	profile, err := svc.ActorProfile("A")
	if err != nil {
		t.Fatal(err)
	}
	if profile == nil || profile.StarID != "star-a" || profile.Birthday != "1998-03-31" {
		t.Fatalf("profile A=%+v", profile)
	}
	if profile, err := svc.ActorProfile("B"); err != nil || profile != nil {
		t.Fatalf("profile B=%+v err=%v, want none", profile, err)
	}

	// Everything is resolved or terminally unknown, so a second press queues
	// nothing and does not resurrect the not-found name.
	again, err := svc.StartActorFill()
	if err != nil {
		t.Fatal(err)
	}
	if again != 0 {
		t.Fatalf("second run queued %d names, want 0", again)
	}
}

// A transient failure returns the name to the queue with a cooldown, so the run
// stays active instead of giving up on the first network hiccup.
func TestStartActorFillRetriesTransientFailures(t *testing.T) {
	svc, _, _ := newActorFixture(t, [][]string{{"A"}})
	svc.SetActorResolver(&fakeActorResolver{errs: map[string]error{
		"A": NewLookupError("retryable", 429, errors.New("rate limited")),
	}})
	svc.Start()

	if _, err := svc.StartActorFill(); err != nil {
		t.Fatal(err)
	}
	var tasks []ActorFillTask
	var err error
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		tasks, err = svc.repo.ListActorFillTasks()
		if err != nil {
			t.Fatal(err)
		}
		if len(tasks) == 1 && tasks[0].Attempts == 1 && tasks[0].NextRetryAt != nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if len(tasks) != 1 || tasks[0].Attempts != 1 || tasks[0].NextRetryAt == nil {
		t.Fatalf("task=%+v, want one cooldown retry", tasks)
	}
	status, err := svc.ActorFillStatus()
	if err != nil {
		t.Fatal(err)
	}
	if !status.Active || status.Queued != 1 || status.Running != 0 {
		t.Fatalf("a cooldown retry must keep the run active: %+v", status)
	}
}

// The attempt budget is what ends a run of transient failures.
func TestActorFillFailureGivesUpAfterBudget(t *testing.T) {
	err := NewLookupError("retryable", 429, errors.New("rate limited"))
	status, code, _, next := actorFillFailure(err, &ActorFillTask{Attempts: 3, MaxAttempts: 3})
	if status != StatusFailed || code != ErrorNetwork || next != nil {
		t.Fatalf("status=%q code=%q next=%v, want a terminal failure", status, code, next)
	}

	status, code, _, next = actorFillFailure(NewLookupError("not_found", 404, nil), &ActorFillTask{Attempts: 1, MaxAttempts: 3})
	if status != StatusNotFound || code != ErrorNotFound || next != nil {
		t.Fatalf("status=%q code=%q next=%v, want terminal not found", status, code, next)
	}
}

// Without a resolver the backfill reports itself as unavailable rather than
// failing every name.
func TestStartActorFillWithoutResolver(t *testing.T) {
	svc, _, _ := newActorFixture(t, [][]string{{"A"}})
	if _, err := svc.StartActorFill(); err == nil {
		t.Fatal("a missing resolver must be reported")
	}
}
