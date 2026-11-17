package jav

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// TitleTranslator turns one scraped title into a Simplified-Chinese title.
//
// Configured reports whether a *newly scraped* title should be translated
// automatically (provider complete, auto-translate not paused). Available
// reports whether a manual request can run at all, which stays true while
// automatic translation is paused; the scrape flow uses Configured, the detail
// button and the bulk action use Available.
//
// The implementation lives outside this package (backend/llm) and is wired once
// during startup; it reads the settings on every call, so editing the provider
// takes effect without restarting the scrape worker.
type TitleTranslator interface {
	TranslateTitle(ctx context.Context, title string) (string, error)
	Configured() bool
	Available() bool
}

// titleTranslationTimeout bounds one queued translation. The provider client
// has its own shorter request timeout; this is the outer guard for a
// misbehaving implementation, and it is never on the scrape's critical path.
const titleTranslationTimeout = 2 * time.Minute

// SetTitleTranslator wires 中文译名 translation. It must be called during
// wiring, before Start. Without a translator the feature simply stays off:
// scrapes succeed, and only the bulk/detail actions report that no provider is
// configured.
func (s *Service) SetTitleTranslator(translator TitleTranslator) {
	s.translator = translator
}

// TranslateTitle queues one favorite for translation. It is idempotent: a
// video already queued or running is left alone. The work happens on the
// translation worker, so the call returns before the provider answers; the
// detail panel sees the resulting state through the normal scrape update
// event.
func (s *Service) TranslateTitle(videoID int64) error {
	if s.translator == nil || !s.translator.Available() {
		return fmt.Errorf("尚未配置 LLM 片名翻译，请先在「设置 → 片名翻译」中填写接口地址、模型与 API Key")
	}
	d, err := s.repo.Get(videoID)
	if err != nil {
		return fmt.Errorf("读取刮削记录失败：%w", err)
	}
	if strings.TrimSpace(d.Title) == "" {
		return fmt.Errorf("该视频还没有可翻译的刮削标题")
	}
	s.enqueueTitleTranslation(videoID, true)
	return nil
}

// TranslateMissingTitles queues every successfully scraped favorite without a
// Chinese title, including earlier failures so the action doubles as a retry
// after a provider fix. It returns how many were newly queued.
func (s *Service) TranslateMissingTitles() (int, error) {
	if s.translator == nil || !s.translator.Available() {
		return 0, fmt.Errorf("尚未配置 LLM 片名翻译，请先在「设置 → 片名翻译」中填写接口地址、模型与 API Key")
	}
	ids, err := s.repo.ListTitleTranslationCandidates()
	if err != nil {
		return 0, err
	}
	return s.enqueueTitleTranslations(ids), nil
}

// RetranslateAllTitles queues every successfully scraped favorite with a
// title, including the rows that already have a Chinese title. It is the
// 重新翻译全部译名 action: after editing the prompt or switching providers the
// user can refresh the whole library without clearing the column by hand.
//
// The stored translation stays visible until the replacement lands, and a
// failed refresh keeps it (see translateTitle), so one bad batch never wipes a
// readable library.
func (s *Service) RetranslateAllTitles() (int, error) {
	if s.translator == nil || !s.translator.Available() {
		return 0, fmt.Errorf("尚未配置 LLM 片名翻译，请先在「设置 → 片名翻译」中填写接口地址、模型与 API Key")
	}
	ids, err := s.repo.ListTitleRetranslationCandidates()
	if err != nil {
		return 0, err
	}
	return s.enqueueTitleTranslations(ids), nil
}

// enqueueTitleTranslations adds a batch to the queue and returns how many were
// newly accepted. notify stays false: a library-wide action would otherwise
// emit one read invalidating event per row before a single translation
// completed. The worker emits when each result lands.
func (s *Service) enqueueTitleTranslations(ids []int64) int {
	queued := 0
	for _, id := range ids {
		if s.enqueueTitleTranslation(id, false) {
			queued++
		}
	}
	return queued
}

// SetTitleResumeOnStart wires the startup backfill switch. It must be called
// during wiring, before Start, and is read lazily so a preference edited in the
// settings page applies to the next launch. A settings read error counts as
// "off": an unreadable preference must never block startup.
func (s *Service) SetTitleResumeOnStart(enabled func() (bool, error)) {
	s.titleResume = enabled
}

// resumeMissingTitles runs the startup backfill once. It is fire-and-forget:
// the worker drains the queue in the background, so a slow provider never
// delays the window. Titles that failed before are included, exactly like the
// manual bulk action, because that action doubles as the retry path after a
// provider fix.
func (s *Service) resumeMissingTitles() {
	if s.titleResume == nil {
		return
	}
	enabled, err := s.titleResume()
	if err != nil {
		s.log.Warn("jav title translation resume check failed", "errorCode", ErrorDatabase, "err", err)
		return
	}
	if !enabled {
		return
	}
	queued, err := s.TranslateMissingTitles()
	if err != nil {
		s.log.Warn("jav title translation resume skipped", "errorCode", ErrorTitleTranslation, "err", err)
		return
	}
	if queued > 0 {
		s.log.Info("jav title translations resumed", "queued", queued)
	}
}

// enqueueTitleTranslation adds one video to the translation queue. notify
// emits the pending state immediately, which the detail panel needs for a
// user-initiated single translation; bulk enqueueing suppresses it.
func (s *Service) enqueueTitleTranslation(videoID int64, notify bool) bool {
	s.titleMu.Lock()
	if _, exists := s.titlePending[videoID]; exists {
		s.titleMu.Unlock()
		return false
	}
	s.titlePending[videoID] = struct{}{}
	s.titleMu.Unlock()

	if !s.titleJobs.push(videoID) {
		s.titleMu.Lock()
		delete(s.titlePending, videoID)
		s.titleMu.Unlock()
		return false
	}
	if notify {
		s.emitUpdate(videoID)
	}
	return true
}

// titleTranslationPending reports whether a translation for this video is
// queued or running. Get overlays it as the pending status so the UI can show
// 翻译中… without persisting a state that a crash would strand.
func (s *Service) titleTranslationPending(videoID int64) bool {
	s.titleMu.Lock()
	_, pending := s.titlePending[videoID]
	s.titleMu.Unlock()
	return pending
}

// titleWorker consumes the translation queue one at a time: providers are rate
// limited, and a title translation is a small, non-urgent background task.
func (s *Service) titleWorker() {
	defer s.wg.Done()
	for {
		videoID, ok := s.titleJobs.pop()
		if !ok {
			return
		}
		s.translateTitle(videoID)
		s.titleMu.Lock()
		delete(s.titlePending, videoID)
		s.titleMu.Unlock()
	}
}

func (s *Service) translateTitle(videoID int64) {
	d, err := s.repo.Get(videoID)
	if err != nil {
		s.log.Warn("jav title translation skipped", "videoId", videoID, "errorCode", ErrorTitleTranslation, "err", err)
		return
	}
	title := strings.TrimSpace(d.Title)
	if title == "" {
		s.finishTitleTranslation(videoID, "", "标题为空，无法翻译")
		return
	}
	if s.translator == nil {
		s.finishTitleTranslation(videoID, "", "片名翻译未配置")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), titleTranslationTimeout)
	defer cancel()
	previousZh := strings.TrimSpace(d.TitleZh)
	translated, err := s.translator.TranslateTitle(ctx, title)
	if err != nil {
		s.log.Warn("jav title translation failed", "videoId", videoID, "errorCode", ErrorTitleTranslation, "err", err)
		if previousZh != "" {
			// A refresh (重新翻译) failed: keep the readable translation the row
			// already had, but record why the refresh failed so the detail panel
			// can show the error next to the kept title.
			s.finishTitleTranslationFailure(videoID, err.Error())
			return
		}
		s.finishTitleTranslation(videoID, "", err.Error())
		return
	}
	s.log.Info("jav title translated", "videoId", videoID, "titleZh", translated)
	s.finishTitleTranslation(videoID, translated, "")
}

// finishTitleTranslationFailure ends one failed refresh without clearing the
// stored translation. The error column is updated, so the next Get reports
// failed while TitleZh still carries the previous readable title. Like
// finishTitleTranslation, it clears the pending overlay before emitting.
func (s *Service) finishTitleTranslationFailure(videoID int64, failure string) {
	if err := s.repo.SetTitleTranslationFailure(videoID, failure); err != nil {
		s.log.Error("jav title translation failure save failed", "videoId", videoID, "errorCode", ErrorDatabase, "err", err)
	}
	s.titleMu.Lock()
	delete(s.titlePending, videoID)
	s.titleMu.Unlock()
	s.emitUpdate(videoID)
}

// finishTitleTranslation stores one attempt and announces the result. It
// clears the in-memory pending mark *before* emitting: the detail panel reads
// the row as soon as the event arrives, so the pending overlay must already be
// gone or a finished translation would still render as 翻译中….
//
// A translation failure is only recorded here (or in
// finishTitleTranslationFailure when an existing title is kept); the scrape
// itself is never touched, because a translation is enrichment and not a
// reason to fail a work.
func (s *Service) finishTitleTranslation(videoID int64, titleZh, failure string) {
	if err := s.repo.SetTitleTranslation(videoID, titleZh, failure); err != nil {
		s.log.Error("jav title translation save failed", "videoId", videoID, "errorCode", ErrorDatabase, "err", err)
	}
	s.titleMu.Lock()
	delete(s.titlePending, videoID)
	s.titleMu.Unlock()
	s.emitUpdate(videoID)
}
