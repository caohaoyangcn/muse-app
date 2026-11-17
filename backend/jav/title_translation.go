package jav

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// TitleTranslator turns scraped titles into Simplified-Chinese titles.
//
// Configured reports whether a *newly scraped* title should be translated
// automatically (provider complete, auto-translate not paused). Available
// reports whether a manual request can run at all, which stays true while
// automatic translation is paused; the scrape flow uses Configured, the detail
// button and the bulk action use Available.
//
// TranslateTitles is the packed path: the worker amortizes the large system
// prompt over several titles in one request. Both returned slices have the same
// length as titles; a slot's error means that one title failed. TranslateTitle
// stays for the single-title smoke test and any one-off caller.
//
// The implementation lives outside this package (backend/llm) and is wired once
// during startup; it reads the settings on every call, so editing the provider
// takes effect without restarting the scrape worker.
type TitleTranslator interface {
	TranslateTitle(ctx context.Context, title string) (string, error)
	TranslateTitles(ctx context.Context, titles []string) ([]string, []error)
	Configured() bool
	Available() bool
}

// titleTranslationTimeout bounds one queued request. The provider client has
// its own shorter request timeout; this is the outer guard for a misbehaving
// implementation, and it is never on the scrape's critical path.
const titleTranslationTimeout = 2 * time.Minute

// titleTranslationBatchSize is how many titles one request packs. Ten keeps the
// model's numbered output stable and cuts the per-title system-prompt overhead
// by an order of magnitude; a larger pack risks a truncated reply and a long
// retry tail.
const titleTranslationBatchSize = 10

// titleTranslationConcurrency is how many packed requests may be in flight at
// once. Five keeps a bulk pass moving while staying far below the burst a
// provider would throttle; the single worker used to leave most of the window
// idle because each request spends its time waiting on the model.
const titleTranslationConcurrency = 5

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
// failed refresh keeps it (see translateTitleBatch), so one bad batch never
// wipes a readable library.
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

// enqueueTitleTranslations packs a bulk pass into fixed-size requests and
// returns how many ids were newly accepted. Every id is reserved before any
// chunk is pushed, so the pending overlay is truthful while the producer is
// still handing work to the worker.
func (s *Service) enqueueTitleTranslations(ids []int64) int {
	s.titleMu.Lock()
	accepted := make([]int64, 0, len(ids))
	for _, id := range ids {
		if _, exists := s.titlePending[id]; exists {
			continue
		}
		s.titlePending[id] = struct{}{}
		accepted = append(accepted, id)
	}
	s.titleMu.Unlock()

	pushed := 0
	for start := 0; start < len(accepted); start += titleTranslationBatchSize {
		end := start + titleTranslationBatchSize
		if end > len(accepted) {
			end = len(accepted)
		}
		chunk := append([]int64(nil), accepted[start:end]...)
		if !s.titleJobs.push(chunk) {
			// The queue only closes during shutdown. Roll back every id that
			// will never be processed so a later Get cannot report 翻译中….
			s.titleMu.Lock()
			for _, id := range accepted[start:] {
				delete(s.titlePending, id)
			}
			s.titleMu.Unlock()
			return pushed
		}
		pushed += len(chunk)
	}
	return pushed
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

// enqueueTitleTranslation adds one video to the translation queue as a
// one-item batch. notify emits the pending state immediately, which the detail
// panel needs for a user-initiated single translation; bulk enqueueing
// suppresses it.
func (s *Service) enqueueTitleTranslation(videoID int64, notify bool) bool {
	s.titleMu.Lock()
	if _, exists := s.titlePending[videoID]; exists {
		s.titleMu.Unlock()
		return false
	}
	s.titlePending[videoID] = struct{}{}
	s.titleMu.Unlock()

	if !s.titleJobs.push([]int64{videoID}) {
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

// titleWorker consumes packed batches. Several workers share the queue so the
// provider wait time overlaps; each worker still processes one request at a
// time, capping the service at titleTranslationConcurrency in-flight requests.
func (s *Service) titleWorker() {
	defer s.wg.Done()
	for {
		videoIDs, ok := s.titleJobs.pop()
		if !ok {
			return
		}
		s.translateTitleBatch(videoIDs)
	}
}

// translateTitleBatch translates one packed batch. Each row is re-read just
// before the request, so a re-scrape that cleared a translation is not
// overwritten with a stale title; the previous translation is remembered so a
// failed refresh can keep it.
func (s *Service) translateTitleBatch(videoIDs []int64) {
	type titleJob struct {
		videoID    int64
		title      string
		previousZh string
	}
	jobs := make([]titleJob, 0, len(videoIDs))
	for _, videoID := range videoIDs {
		d, err := s.repo.Get(videoID)
		if err != nil {
			s.log.Warn("jav title translation skipped", "videoId", videoID, "errorCode", ErrorTitleTranslation, "err", err)
			s.dropTitleTranslation(videoID)
			continue
		}
		title := strings.TrimSpace(d.Title)
		if title == "" {
			s.finishTitleTranslation(videoID, "", "标题为空，无法翻译")
			continue
		}
		jobs = append(jobs, titleJob{
			videoID:    videoID,
			title:      title,
			previousZh: strings.TrimSpace(d.TitleZh),
		})
	}
	if len(jobs) == 0 {
		return
	}
	if s.translator == nil {
		for _, job := range jobs {
			s.finishTitleTranslation(job.videoID, "", "片名翻译未配置")
		}
		return
	}

	titles := make([]string, len(jobs))
	for i, job := range jobs {
		titles[i] = job.title
	}
	ctx, cancel := context.WithTimeout(context.Background(), titleTranslationTimeout)
	defer cancel()
	translations, errs := s.translator.TranslateTitles(ctx, titles)
	if len(translations) != len(jobs) || len(errs) != len(jobs) {
		// A broken translator must not leave every row stuck in 翻译中….
		for _, job := range jobs {
			s.finishTitleTranslationFailure(job.videoID, "片名翻译返回了长度不匹配的结果")
		}
		return
	}

	translated := 0
	for i := range jobs {
		if errs[i] == nil && strings.TrimSpace(translations[i]) != "" {
			translated++
		}
	}
	s.log.Info("jav title batch finished", "requested", len(jobs), "translated", translated)
	for i, job := range jobs {
		s.finishOneTitleTranslation(job.videoID, translations[i], errs[i], job.previousZh)
	}
}

// finishOneTitleTranslation stores one slot of a packed reply. A failed refresh
// keeps the old readable title and records the error next to it; a first-time
// failure leaves title_zh empty and the error explains why.
func (s *Service) finishOneTitleTranslation(videoID int64, translated string, err error, previousZh string) {
	if err != nil {
		s.log.Warn("jav title translation failed", "videoId", videoID, "errorCode", ErrorTitleTranslation, "err", err)
		if previousZh != "" {
			s.finishTitleTranslationFailure(videoID, err.Error())
			return
		}
		s.finishTitleTranslation(videoID, "", err.Error())
		return
	}
	translated = strings.TrimSpace(translated)
	if translated == "" {
		s.log.Warn("jav title translation failed", "videoId", videoID, "errorCode", ErrorTitleTranslation, "err", "LLM 返回了空的翻译结果")
		if previousZh != "" {
			s.finishTitleTranslationFailure(videoID, "LLM 返回了空的翻译结果")
			return
		}
		s.finishTitleTranslation(videoID, "", "LLM 返回了空的翻译结果")
		return
	}
	s.log.Info("jav title translated", "videoId", videoID, "titleZh", translated)
	s.finishTitleTranslation(videoID, translated, "")
}

// dropTitleTranslation clears the pending overlay for a row that cannot be
// read anymore, so it does not stay 翻译中… until the next event.
func (s *Service) dropTitleTranslation(videoID int64) {
	s.titleMu.Lock()
	delete(s.titlePending, videoID)
	s.titleMu.Unlock()
	s.emitUpdate(videoID)
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
