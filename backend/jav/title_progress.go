package jav

import (
	"strconv"
	"strings"
	"time"
)

// TitleTranslationStatus is the 后台任务 dashboard's view of the 中文译名 queue.
//
// It is a projection of in-memory state only: the queue itself is not
// persisted (a crash simply leaves the remaining titles for the next bulk pass
// or the startup backfill), and the counters survive the end of a run until the
// next one starts, which is what lets the dashboard show the last result
// without a table.
type TitleTranslationStatus struct {
	Active bool `json:"active"`
	// Total is how many rows this run has accepted; Done and Failed are its
	// outcomes so far. Total only grows while the run is active.
	Total  int `json:"total"`
	Done   int `json:"done"`
	Failed int `json:"failed"`
	// InFlight is how many packed requests are waiting on the provider.
	InFlight int `json:"inFlight"`
	// Current describes the newest request's batch ("IPX-633 等 10 部").
	Current   string `json:"current,omitempty"`
	StartedAt int64  `json:"startedAt,omitempty"`
	UpdatedAt int64  `json:"updatedAt,omitempty"`
}

// titleRunState is the run behind TitleTranslationStatus. Guarded by
// Service.titleMu.
type titleRunState struct {
	Total    int
	Done     int
	Failed   int
	InFlight int
	Current  string
	// StartedAt and UpdatedAt are unix milliseconds.
	StartedAt int64
	UpdatedAt int64
}

// tasksUpdateInterval coalesces the dashboard event. A bulk pass finishing 525
// rows in a burst would otherwise notify the frontend hundreds of times for a
// screen that redraws at most once a second anyway.
const tasksUpdateInterval = time.Second

// TitleTranslationStatus returns a copy of the current (or last) run.
func (s *Service) TitleTranslationStatus() TitleTranslationStatus {
	s.titleMu.Lock()
	defer s.titleMu.Unlock()
	run := s.titleRun
	return TitleTranslationStatus{
		Active:    len(s.titlePending) > 0,
		Total:     run.Total,
		Done:      run.Done,
		Failed:    run.Failed,
		InFlight:  run.InFlight,
		Current:   run.Current,
		StartedAt: run.StartedAt,
		UpdatedAt: run.UpdatedAt,
	}
}

// noteTitleBatchLocked accounts for rows accepted by a producer. idle reports
// whether the queue was empty when the producer started: that starts a new run
// and resets the counters, so "上次补齐" always describes one pass. The caller
// holds titleMu.
func (s *Service) noteTitleBatchLocked(idle bool, added int) {
	now := time.Now().UnixMilli()
	if idle {
		s.titleRun = titleRunState{StartedAt: now, UpdatedAt: now}
	}
	s.titleRun.Total += added
	s.titleRun.UpdatedAt = now
}

// rollbackTitleBatchLocked removes accepted rows that were never queued (the
// queue closes only during shutdown), so the run does not advertise work no
// worker will ever do. The caller holds titleMu.
func (s *Service) rollbackTitleBatchLocked(dropped int) {
	if dropped <= 0 {
		return
	}
	s.titleRun.Total -= dropped
	if s.titleRun.Total < 0 {
		s.titleRun.Total = 0
	}
	s.titleRun.UpdatedAt = time.Now().UnixMilli()
}

// beginTitleRequest marks one packed request as in flight and records what it
// covers, so the dashboard can say "正在翻译 IPX-633 等 10 部".
func (s *Service) beginTitleRequest(label string) {
	s.titleMu.Lock()
	s.titleRun.InFlight++
	if label != "" {
		s.titleRun.Current = label
	}
	s.titleRun.UpdatedAt = time.Now().UnixMilli()
	s.titleMu.Unlock()
}

// endTitleRequest clears the in-flight mark; with several requests running, the
// label stays until the last one returns.
func (s *Service) endTitleRequest() {
	s.titleMu.Lock()
	if s.titleRun.InFlight > 0 {
		s.titleRun.InFlight--
	}
	if s.titleRun.InFlight == 0 {
		s.titleRun.Current = ""
	}
	s.titleMu.Unlock()
}

// finishTitleRunLocked records one row's outcome. A row that is skipped (its
// video record cannot be read) counts as failed so Done+Failed can reach Total.
// The caller holds titleMu.
func (s *Service) finishTitleRunLocked(success bool) {
	if success {
		s.titleRun.Done++
	} else {
		s.titleRun.Failed++
	}
	s.titleRun.UpdatedAt = time.Now().UnixMilli()
}

// scheduleTasksUpdate emits one coalesced "the dashboard changed" event per
// interval, however many rows finish in between. The trailing timer guarantees
// the last result is announced instead of being folded away.
func (s *Service) scheduleTasksUpdate() {
	s.titleMu.Lock()
	if s.tasksEmitScheduled {
		s.titleMu.Unlock()
		return
	}
	s.tasksEmitScheduled = true
	s.titleMu.Unlock()
	time.AfterFunc(tasksUpdateInterval, func() {
		s.titleMu.Lock()
		s.tasksEmitScheduled = false
		s.titleMu.Unlock()
		if s.emit != nil {
			s.emit("tasks:updated")
		}
	})
}

// titleRequestLabel names one packed request for the dashboard: the first
// code when it is known, otherwise the first title, plus how many rows ride
// with it.
func titleRequestLabel(code, title string, count int) string {
	label := strings.TrimSpace(code)
	if label == "" {
		label = strings.TrimSpace(title)
		if runes := []rune(label); len(runes) > 24 {
			label = string(runes[:24])
		}
	}
	if label == "" {
		return ""
	}
	if count > 1 {
		return label + " 等 " + strconv.Itoa(count) + " 部"
	}
	return label
}
