package jav

import (
	"context"
	"fmt"
	"time"
)

// ActorResolver turns a scraped actor name into that performer's profile. It is
// the backfill path: metadata lookups already carry profiles for the providers
// that publish them, so a resolver is only needed for names that were scraped
// before profiles were recorded, or by a provider that returns names alone.
type ActorResolver interface {
	ResolveActor(ctx context.Context, name string) (*ActorProfile, error)
}

// actorFillRetryCooldown is the wait before retrying one actor after a
// transient failure. The backfill is a background nicety, so a slow retry costs
// nothing; the attempt counter is what eventually gives up.
const actorFillRetryCooldown = 60 * time.Second

// SetActorResolver wires the actor profile backfill. It must be called during
// wiring, before Start. Without a resolver the backfill reports itself as
// unavailable instead of failing every actor.
func (s *Service) SetActorResolver(resolver ActorResolver) {
	s.resolver = resolver
}

// StartActorFill enqueues every actor in the index that has no profile yet and
// wakes the backfill worker. It returns how many names were newly queued; a
// name that is already queued, already being resolved, or that the provider does
// not know is left alone, so pressing the button twice is harmless.
func (s *Service) StartActorFill() (int, error) {
	if s.resolver == nil {
		return 0, fmt.Errorf("actor profile provider not configured")
	}
	actors, err := s.aggregateActors()
	if err != nil {
		return 0, err
	}
	profiles, err := s.profilesByName()
	if err != nil {
		return 0, err
	}
	names := make([]string, 0, len(actors))
	for _, actor := range actors {
		if profiles[actor.Name] != nil {
			continue
		}
		merged := false
		for _, alias := range actor.Aliases {
			if profiles[alias] != nil {
				merged = true
				break
			}
		}
		if merged {
			continue
		}
		names = append(names, actor.Name)
	}
	queued, err := s.repo.EnqueueActorFills(names)
	if err != nil {
		return 0, err
	}
	s.wakeActorFill()
	return queued, nil
}

// ActorFillStatus summarises the backfill queue for the actor index.
func (s *Service) ActorFillStatus() (ActorFillStatus, error) {
	tasks, err := s.repo.ListActorFillTasks()
	if err != nil {
		return ActorFillStatus{}, err
	}
	status := ActorFillStatus{Total: len(tasks)}
	for _, task := range tasks {
		switch task.Status {
		case StatusQueued:
			status.Queued++
		case StatusRunning:
			status.Running++
			if status.Current == "" {
				status.Current = task.Name
			}
		case StatusSucceeded:
			status.Succeeded++
		case StatusNotFound:
			status.NotFound++
		default:
			status.Failed++
		}
	}
	status.Active = status.Queued > 0 || status.Running > 0
	return status, nil
}

// wakeActorFill nudges the worker without blocking: a wake already pending is
// as good as this one, because the worker drains the whole due queue each time.
func (s *Service) wakeActorFill() {
	select {
	case s.fillWake <- struct{}{}:
	default:
	}
}

// startActorFillWorker is only called from Start, which is what makes the
// worker's shutdown reachable: Close closes stop only after Start has run.
func (s *Service) startActorFillWorker() {
	s.fillOnce.Do(func() {
		s.wg.Add(1)
		go s.actorFillWorker()
	})
}

func (s *Service) actorFillWorker() {
	defer s.wg.Done()
	for {
		select {
		case <-s.stop:
			return
		case <-s.fillWake:
		}
		s.drainActorFill()
	}
}

// drainActorFill resolves every due name, one at a time. Names waiting out a
// retry cooldown are simply not due yet; the periodic scan wakes the worker
// again when they are.
func (s *Service) drainActorFill() {
	if s.resolver == nil {
		return
	}
	for {
		select {
		case <-s.stop:
			return
		default:
		}
		task, claimed, err := s.repo.ClaimActorFill(time.Now().UnixMilli())
		if err != nil {
			s.log.Error("actor fill claim failed", "errorCode", ErrorDatabase, "err", err)
			return
		}
		if !claimed {
			return
		}
		s.resolveActorFill(task)
	}
}

// resolveActorFill resolves one claimed name and records the outcome. The
// requested spelling is the profile key — it is what the cast lists and the
// actor index contain — and the provider's own spelling becomes an alias.
func (s *Service) resolveActorFill(task *ActorFillTask) {
	now := time.Now().UnixMilli()
	profile, err := s.resolver.ResolveActor(context.Background(), task.Name)
	if err == nil && profile != nil {
		profile.FetchedAt = &now
		if saveErr := s.persistActorProfiles([]ActorProfile{*profile}, now); saveErr != nil {
			s.finishActorFill(task.Name, StatusFailed, ErrorDatabase, saveErr.Error(), now)
			return
		}
		s.finishActorFill(task.Name, StatusSucceeded, "", "", now)
		s.log.Info("actor profile resolved", "actor", task.Name, "provider", profile.Provider)
		return
	}
	status, code, message, next := actorFillFailure(err, task)
	if next != nil {
		// Retry after a cooldown: the periodic scan wakes the worker when due.
		if setErr := s.repo.SetActorFillResult(task.Name, status, code, message, next, nil); setErr != nil {
			s.log.Error("actor fill retry not saved", "actor", task.Name, "errorCode", ErrorDatabase, "err", setErr)
		}
		s.emitActorUpdate(task.Name, status)
		return
	}
	s.finishActorFill(task.Name, status, code, message, now)
}

// emitActorUpdate announces that the actor index's enrichment changed. Like
// every scrape update it is a plain invalidation, so a nil emitter (tests, and
// wiring that runs before Wails starts) is simply silent.
func (s *Service) emitActorUpdate(name, status string) {
	if s.emit == nil {
		return
	}
	s.emit("jav:actors:updated", map[string]any{"name": name, "status": status})
}

// actorFillFailure maps a resolver error onto the task states. Not found is
// terminal: retrying a name the provider does not know would keep the queue
// busy forever. Transient failures retry with a cooldown until the attempt
// budget runs out.
func actorFillFailure(err error, task *ActorFillTask) (status, errorCode, message string, nextRetryAt *int64) {
	if err == nil {
		return StatusNotFound, ErrorNotFound, "provider returned no profile", nil
	}
	message = err.Error()
	kind := lookupErrorKind(err)
	switch kind {
	case "not_found":
		return StatusNotFound, ErrorNotFound, "provider does not know this actor", nil
	case "blocked":
		return StatusFailed, ErrorBlocked, message, nil
	case "parse":
		return StatusFailed, ErrorParse, message, nil
	case "retryable", "network":
		if task.Attempts >= task.MaxAttempts {
			return StatusFailed, ErrorNetwork, message, nil
		}
		next := time.Now().UnixMilli() + actorFillRetryCooldown.Milliseconds()
		return StatusQueued, ErrorNetwork, message, &next
	default:
		return StatusFailed, ErrorActorProfile, message, nil
	}
}

// finishActorFill records a terminal outcome.
func (s *Service) finishActorFill(name, status, errorCode, lastError string, now int64) {
	if err := s.repo.SetActorFillResult(name, status, errorCode, lastError, nil, &now); err != nil {
		s.log.Error("actor fill result not saved", "actor", name, "errorCode", ErrorDatabase, "err", err)
	}
	s.emitActorUpdate(name, status)
}
