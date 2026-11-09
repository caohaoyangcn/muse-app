package jav

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"muse-app/backend/video"
)

const (
	retryScanInterval = 30 * time.Second
)

// retryCooldowns is the wait before each retry, indexed by the attempt that
// just failed. Its length also caps how many retries the service will schedule,
// so it must stay one shorter than the task's max_attempts (the initial attempt
// plus one cooldown per retry). retryDecision treats running out of cooldowns
// as exhaustion even when max_attempts would still allow another attempt.
var retryCooldowns = [...]time.Duration{
	20 * time.Second,
	40 * time.Second,
	60 * time.Second,
}

type Service struct {
	repo           Repository
	videos         video.VideoRepository
	provider       Provider
	coverDir       string
	avatarDir      string
	emit           func(string, ...any)
	log            *slog.Logger
	jobs           *jobQueue
	mu             sync.Mutex
	pending        map[int64]struct{}
	wg             sync.WaitGroup
	startOnce      sync.Once
	closeOnce      sync.Once
	stop           chan struct{}
	stopped        chan struct{}
	scannerStarted chan struct{}
	closed         bool

	// The actor profile backfill: resolver is optional wiring, fillWake nudges
	// the single worker, fillOnce keeps the worker from being started twice.
	resolver ActorResolver
	fillWake chan struct{}
	fillOnce sync.Once
}

type job struct {
	id    int64
	force bool
}

// Optional provider-side fetch contracts. They intentionally live beside the
// service rather than in Provider so existing providers remain source
// compatible. A provider can expose either a cover-specific fetcher or a
// generic HTTP fetcher; otherwise the service keeps its historical default
// client behavior.
type coverResponseFetcher interface {
	FetchCover(context.Context, string) (*http.Response, error)
}

type coverBytesFetcher interface {
	FetchCover(context.Context, string) ([]byte, error)
}

type coverReaderFetcher interface {
	FetchCover(context.Context, string) (io.ReadCloser, error)
}

type urlResponseFetcher interface {
	Fetch(context.Context, string) (*http.Response, error)
}

type namedURLResponseFetcher interface {
	FetchFor(context.Context, string, string) (*http.Response, error)
}

type urlBytesFetcher interface {
	Fetch(context.Context, string) ([]byte, error)
}

type urlReaderFetcher interface {
	Fetch(context.Context, string) (io.ReadCloser, error)
}

type requestResponseFetcher interface {
	Do(*http.Request) (*http.Response, error)
}

type contextRequestResponseFetcher interface {
	Do(context.Context, *http.Request) (*http.Response, error)
}

type httpClientProvider interface {
	HTTPClient() *http.Client
}

type fetchedCover struct {
	body          io.ReadCloser
	statusCode    int
	contentLength int64
}

func NewService(repo Repository, videos video.VideoRepository, provider Provider, coverDir string, emit func(string, ...any), log *slog.Logger, workers int) *Service {
	if workers < 1 {
		workers = 1
	}
	if log == nil {
		log = slog.Default()
	}
	s := &Service{repo: repo, videos: videos, provider: provider, coverDir: coverDir, emit: emit, log: log, jobs: newJobQueue(), pending: make(map[int64]struct{}), stop: make(chan struct{}), stopped: make(chan struct{}), scannerStarted: make(chan struct{}), fillWake: make(chan struct{}, 1)}
	for i := 0; i < workers; i++ {
		s.wg.Add(1)
		go s.worker()
	}
	return s
}

// SetActorAvatarDir tells the service where downloaded performer avatars live.
// It must be called during wiring, before Start: no worker reads the directory
// until a scrape completes. Leaving it empty keeps profiles without avatars
// instead of failing the scrape.
func (s *Service) SetActorAvatarDir(dir string) {
	s.avatarDir = strings.TrimSpace(dir)
}

// Start recovers persisted tasks and starts the periodic retry scanner.
func (s *Service) Start() {
	s.startOnce.Do(func() {
		close(s.scannerStarted)
		s.recoverPending()
		// A backfill interrupted by a quit left names running with no worker;
		// requeueing them is safe because resolving a profile is idempotent.
		if err := s.repo.ResetActorFills(); err != nil {
			s.log.Warn("actor fill recovery failed", "errorCode", ErrorDatabase, "err", err)
		}
		s.startActorFillWorker()
		s.wakeActorFill()
		go s.scanLoop()
	})
}

func (s *Service) Close() {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		s.jobs.close()

		select {
		case <-s.scannerStarted:
			close(s.stop)
			<-s.stopped
		default:
		}
		s.wg.Wait()
	})
}

func (s *Service) scanLoop() {
	ticker := time.NewTicker(retryScanInterval)
	defer ticker.Stop()
	defer close(s.stopped)
	for {
		select {
		case <-ticker.C:
			s.recoverPending()
			// Names waiting out a retry cooldown become due without any user
			// action, so the same tick that recovers scrapes wakes the backfill.
			s.wakeActorFill()
		case <-s.stop:
			return
		}
	}
}

func (s *Service) recoverPending() {
	tasks, err := s.repo.RecoverPending(time.Now().UnixMilli())
	if err != nil {
		s.log.Error("jav task recovery failed", "errorCode", ErrorDatabase, "err", err)
		return
	}
	for _, task := range tasks {
		s.enqueueExisting(task.VideoID, task.Force)
		s.log.Info("jav task recovered", "taskId", task.ID, "videoId", task.VideoID, "status", task.Status, "attempts", task.Attempts)
	}
}

func (s *Service) Get(videoID int64) (*Detail, error)         { return s.repo.Get(videoID) }
func (s *Service) GetTask(videoID int64) (*Task, error)       { return s.repo.GetTask(videoID) }
func (s *Service) ListTaskOverviews() ([]TaskOverview, error) { return s.repo.ListTaskOverviews() }

// Codes returns the authoritative 番号 per favorite for the shared video read
// model. A favorite with no saved code is absent from the map.
func (s *Service) Codes() (map[int64]string, error) { return s.repo.ListCodes() }

// ActorsByVideo returns the canonical cast per favorite for the shared video
// read model. Merged stage names (演员合并) resolve to their canonical name, so
// an automatically grouped workspace tab and the actor index agree on who the
// actor is. Names are deduplicated and sorted so the projection is stable; a
// favorite with no scraped cast is absent from the map.
func (s *Service) ActorsByVideo() (map[int64][]string, error) {
	refs, err := s.repo.ListActorFilmRefs()
	if err != nil {
		return nil, err
	}
	merges, err := s.repo.ListActorMerges()
	if err != nil {
		return nil, err
	}
	aliases := actorAliasMap(merges)

	sets := make(map[int64]map[string]struct{})
	for _, ref := range refs {
		canonical := resolveActor(ref.Name, aliases)
		if canonical == "" {
			continue
		}
		if sets[ref.VideoID] == nil {
			sets[ref.VideoID] = make(map[string]struct{})
		}
		sets[ref.VideoID][canonical] = struct{}{}
	}
	out := make(map[int64][]string, len(sets))
	for videoID, names := range sets {
		cast := make([]string, 0, len(names))
		for name := range names {
			cast = append(cast, name)
		}
		sort.Strings(cast)
		out[videoID] = cast
	}
	return out, nil
}

// ListActors returns the aggregated cast index: one entry per actor with the
// number of favorites they appear in, derived from the existing scrape results.
// Names that have been merged (演员合并) are folded into their canonical entry,
// which carries the merged names in Aliases and a count that includes them.
func (s *Service) ListActors() ([]ActorSummary, error) {
	actors, err := s.aggregateActors()
	if err != nil {
		return nil, err
	}
	if err := s.attachActorProfiles(actors); err != nil {
		return nil, err
	}
	return actors, nil
}

// ListDuplicates returns the recognized works (番号) that the library holds more
// than one copy of — the 查重 worklist. Copies are grouped by 番号 plus the
// part marker on their file name, so "ABC-123-1.mp4" and "ABC-123-2.mp4" are
// two episodes, not one duplicated work. Groups the user chose to keep whole
// (保留两者) are omitted unless includeIgnored is set; a group whose membership
// changed since it was dismissed counts as new and is always returned.
// Groups are ordered by their newest copy, matching the library's default
// order.
func (s *Service) ListDuplicates(includeIgnored bool) ([]DuplicateGroup, error) {
	candidates, err := s.repo.ListDuplicateCandidates()
	if err != nil {
		return nil, err
	}
	ignores, err := s.repo.ListDuplicateIgnores()
	if err != nil {
		return nil, err
	}
	dismissed := make(map[string]string, len(ignores))
	for _, ignored := range ignores {
		dismissed[ignored.Key] = ignored.VideoIDs
	}

	byKey := make(map[string]*DuplicateGroup)
	keys := make([]string, 0)
	for _, candidate := range candidates {
		part := PartForFile(candidate.Video.FileName, candidate.Code)
		tier := DuplicateTierLocal
		if candidate.Video.SourceID > 0 {
			tier = DuplicateTierCloud
		}
		key := DuplicateGroupKey(candidate.Code, part, tier)
		group, ok := byKey[key]
		if !ok {
			group = &DuplicateGroup{Key: key, Code: normalizeCode(candidate.Code), Part: part, Tier: tier}
			byKey[key] = group
			keys = append(keys, key)
		}
		group.Videos = append(group.Videos, candidate.Video)
	}

	out := make([]DuplicateGroup, 0, len(keys))
	for _, key := range keys {
		group := byKey[key]
		if len(group.Videos) < 2 {
			continue // one copy is not a duplicate
		}
		ids := make([]int64, 0, len(group.Videos))
		for _, v := range group.Videos {
			ids = append(ids, v.ID)
		}
		group.Ignored = dismissed[key] == duplicateSignature(ids)
		if group.Ignored && !includeIgnored {
			continue
		}
		out = append(out, *group)
	}
	return out, nil
}

// BackedUpKeys returns the work keys that exist as an active, available copy on
// both a local disk and an enabled cloud source. The read model uses it for the
// 已备份 badge. It is deliberately work-level (番号 + part), not byte-level.
func (s *Service) BackedUpKeys() (map[string]bool, error) {
	candidates, err := s.repo.ListDuplicateCandidates()
	if err != nil {
		return nil, err
	}
	type tiers struct{ local, cloud bool }
	byKey := make(map[string]tiers)
	for _, candidate := range candidates {
		if candidate.Video.Status != video.StatusAvailable {
			continue
		}
		part := PartForFile(candidate.Video.FileName, candidate.Code)
		key := DuplicateKey(candidate.Code, part)
		t := byKey[key]
		if candidate.Video.SourceID > 0 {
			t.cloud = true
		} else {
			t.local = true
		}
		byKey[key] = t
	}
	out := make(map[string]bool)
	for key, t := range byKey {
		if t.local && t.cloud {
			out[key] = true
		}
	}
	return out, nil
}

// IgnoreDuplicate records that the user chose to keep every copy in the group
// identified by key (等价于忽略重复提示), so the prompt stays out of the way
// until the group's membership changes.
func (s *Service) IgnoreDuplicate(key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("duplicate key cannot be empty")
	}
	ids, err := s.duplicateMemberIDs(key)
	if err != nil {
		return err
	}
	if len(ids) < 2 {
		// The copies the key described are gone, so there is nothing to
		// dismiss; storing one now would silence a future, unrelated group.
		return s.repo.DeleteDuplicateIgnore(key)
	}
	return s.repo.UpsertDuplicateIgnore(key, duplicateSignature(ids))
}

// duplicateMemberIDs returns the ids of the copies currently in the group
// identified by key, newest first.
func (s *Service) duplicateMemberIDs(key string) ([]int64, error) {
	candidates, err := s.repo.ListDuplicateCandidates()
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, 2)
	for _, candidate := range candidates {
		if DuplicateKey(candidate.Code, PartForFile(candidate.Video.FileName, candidate.Code)) == key {
			ids = append(ids, candidate.Video.ID)
		}
	}
	return ids, nil
}

// VideoIDsByActor returns the favorites whose scraped cast includes the actor
// named name — directly or under any alias merged into it.
func (s *Service) VideoIDsByActor(name string) ([]int64, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return []int64{}, nil
	}
	merges, err := s.repo.ListActorMerges()
	if err != nil {
		return nil, err
	}
	canonical := resolveActor(name, actorAliasMap(merges))
	names := actorGroupNames(canonical, merges)
	return s.repo.VideoIDsByActorNames(names)
}

// MergeActors merges the actor named source into the actor named target, so the
// two stage names are shown as one. Any names already merged into source follow
// it onto target (the whole group moves). Merging a name into itself, or two
// names that already resolve together, is a no-op.
func (s *Service) MergeActors(source, target string) error {
	source = strings.TrimSpace(source)
	target = strings.TrimSpace(target)
	if source == "" || target == "" {
		return fmt.Errorf("actor name cannot be empty")
	}
	merges, err := s.repo.ListActorMerges()
	if err != nil {
		return err
	}
	aliases := actorAliasMap(merges)
	sourceCanonical := resolveActor(source, aliases)
	targetCanonical := resolveActor(target, aliases)
	if sourceCanonical == targetCanonical {
		return nil // already the same actor
	}
	// Move source's whole group onto target: repoint every alias that currently
	// resolves to source, then source itself.
	for _, m := range merges {
		if resolveActor(m.Alias, aliases) == sourceCanonical {
			if err := s.repo.UpsertActorMerge(m.Alias, targetCanonical); err != nil {
				return err
			}
		}
	}
	if sourceCanonical != targetCanonical {
		if err := s.repo.UpsertActorMerge(sourceCanonical, targetCanonical); err != nil {
			return err
		}
	}
	return nil
}

// UnmergeActor removes the merge relationship that maps alias onto its
// canonical actor, restoring alias as an independent actor.
func (s *Service) UnmergeActor(alias string) error {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return fmt.Errorf("actor name cannot be empty")
	}
	return s.repo.DeleteActorMerge(alias)
}

// aggregateActors folds the raw (name, film) pairs through the alias map. Films
// are collected per canonical actor as a set, so a favorite that lists the same
// performer under two merged stage names counts once, and the count always
// equals the size of that actor's film list.
func (s *Service) aggregateActors() ([]ActorSummary, error) {
	refs, err := s.repo.ListActorFilmRefs()
	if err != nil {
		return nil, err
	}
	merges, err := s.repo.ListActorMerges()
	if err != nil {
		return nil, err
	}
	aliases := actorAliasMap(merges)

	// filmsByCanonical is a set of film ids per actor; members lists every
	// merged name that belongs to that canonical (so the UI can show the
	// aliases). A canonical name that only exists as a merge target (never
	// scraped directly) still gets an entry, with an empty film set.
	filmsByCanonical := make(map[string]map[int64]struct{})
	members := make(map[string][]string)
	for _, m := range merges {
		canonical := resolveActor(m.Alias, aliases)
		if filmsByCanonical[canonical] == nil {
			filmsByCanonical[canonical] = make(map[int64]struct{})
		}
		members[canonical] = append(members[canonical], m.Alias)
	}
	for _, ref := range refs {
		canonical := resolveActor(ref.Name, aliases)
		if filmsByCanonical[canonical] == nil {
			filmsByCanonical[canonical] = make(map[int64]struct{})
		}
		filmsByCanonical[canonical][ref.VideoID] = struct{}{}
		members[canonical] = append(members[canonical], ref.Name)
	}

	out := make([]ActorSummary, 0, len(filmsByCanonical))
	for canonical, films := range filmsByCanonical {
		summary := ActorSummary{Name: canonical, Count: len(films)}
		seen := make(map[string]struct{})
		for _, member := range members[canonical] {
			if member == canonical {
				continue
			}
			if _, dup := seen[member]; dup {
				continue
			}
			seen[member] = struct{}{}
			summary.Aliases = append(summary.Aliases, member)
		}
		sort.Strings(summary.Aliases)
		out = append(out, summary)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// actorAliasMap indexes merges by alias for resolution.
func actorAliasMap(merges []ActorMerge) map[string]string {
	aliases := make(map[string]string, len(merges))
	for _, m := range merges {
		aliases[m.Alias] = m.Canonical
	}
	return aliases
}

// resolveActor follows the alias chain to the canonical name. The hop limit
// keeps a corrupt cyclic mapping from looping forever.
func resolveActor(name string, aliases map[string]string) string {
	const maxHops = 32
	seen := make(map[string]struct{}, 4)
	for i := 0; i < maxHops; i++ {
		next, ok := aliases[name]
		if !ok || next == name {
			return name
		}
		if _, dup := seen[next]; dup {
			return name // cycle: stop at the current name
		}
		seen[next] = struct{}{}
		name = next
	}
	return name
}

// actorGroupNames returns the canonical name plus every alias that resolves to
// it, i.e. every scraped spelling that should be treated as one actor.
func actorGroupNames(canonical string, merges []ActorMerge) []string {
	names := []string{canonical}
	aliases := actorAliasMap(merges)
	for _, m := range merges {
		if resolveActor(m.Alias, aliases) == canonical {
			names = append(names, m.Alias)
		}
	}
	return names
}

func (s *Service) Enqueue(videoID int64)      { s.enqueue(videoID, false) }
func (s *Service) EnqueueForce(videoID int64) { s.enqueue(videoID, true) }

// EnqueueVideos schedules a library-wide scrape pass. When onlyUnprocessed is
// true, successful tasks and explicitly skipped videos are left untouched;
// failed and never-started tasks, including not-found tasks, are retried from
// a clean state. Force mode resets every non-skipped task, including
// successful and not-found tasks.
func (s *Service) EnqueueVideos(videoIDs []int64, onlyUnprocessed bool) (int, error) {
	queued := 0
	for _, videoID := range videoIDs {
		detail, err := s.repo.Get(videoID)
		if err != nil && err != sql.ErrNoRows {
			return queued, err
		}
		if detail != nil {
			if detail.OverrideMode == OverrideSkip {
				continue
			}
			if onlyUnprocessed && detail.ScrapeStatus == StatusSucceeded {
				continue
			}
		}
		if err := s.repo.PrepareTask(videoID, true); err != nil {
			return queued, err
		}
		if s.enqueueExisting(videoID, true) {
			queued++
			s.emitUpdate(videoID)
		}
	}
	return queued, nil
}

func (s *Service) enqueue(videoID int64, force bool) {
	if err := s.repo.PrepareTask(videoID, force); err != nil {
		s.log.Error("jav task prepare failed", "videoId", videoID, "errorCode", ErrorDatabase, "err", err)
		return
	}
	if s.enqueueExisting(videoID, force) {
		s.emitUpdate(videoID)
	}
}

func (s *Service) enqueueExisting(videoID int64, force bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	if _, ok := s.pending[videoID]; ok {
		return false
	}
	s.pending[videoID] = struct{}{}
	if !s.jobs.push(job{id: videoID, force: force}) {
		delete(s.pending, videoID)
		return false
	}
	return true
}

func (s *Service) worker() {
	defer s.wg.Done()
	for {
		j, ok := s.jobs.pop()
		if !ok {
			return
		}
		s.process(j)
		s.mu.Lock()
		delete(s.pending, j.id)
		s.mu.Unlock()
	}
}

func (s *Service) process(j job) {
	d, err := s.repo.Get(j.id)
	if err != nil {
		s.log.Warn("jav task detail unavailable", "videoId", j.id, "errorCode", ErrorDatabase, "err", err)
		return
	}
	if d.OverrideMode == OverrideSkip {
		s.finishWithoutLease(j.id, StatusDisabled, "", "")
		return
	}
	if d.ScrapeStatus == StatusSucceeded && !j.force {
		return
	}
	v, err := s.videos.Get(j.id)
	if err != nil {
		s.finishWithoutLease(j.id, StatusRetryable, ErrorVideoMissing, err.Error())
		return
	}
	code, source := d.Code, "manual"
	codes := []string{strings.TrimSpace(code)}
	if d.OverrideMode != OverrideManual || strings.TrimSpace(code) == "" {
		codes = ExtractCodes(v.FileName)
		if len(codes) == 0 {
			s.finishWithoutLease(j.id, StatusNoCode, ErrorNoCode, "文件名中没有识别到番号")
			return
		}
		code, source = codes[0], "filename"
	}
	token := fmt.Sprintf("%d-%d", time.Now().UnixNano(), j.id)
	now := time.Now().UnixMilli()
	claimed, ok, err := s.repo.ClaimTask(j.id, token, now, now+10*time.Minute.Milliseconds())
	if err != nil {
		s.log.Error("jav task claim failed", "videoId", j.id, "errorCode", ErrorDatabase, "err", err)
		return
	}
	if !ok {
		return
	}
	s.emitUpdate(j.id)
	if s.provider == nil {
		s.failTask(j.id, token, claimed.Attempts, StatusBlocked, ErrorProviderMissing, "provider not configured", nil)
		return
	}
	var m *Metadata
	var lookupErr error
	for index, candidate := range codes {
		code = candidate
		s.log.Info("jav lookup start", "videoId", j.id, "code", code, "candidateIndex", index, "candidateCount", len(codes), "provider", s.provider.Name())
		m, lookupErr = s.provider.Lookup(context.Background(), code)
		if lookupErr == nil {
			providerName := ""
			if m != nil {
				providerName = m.Provider
			}
			s.log.Info("jav lookup success", "videoId", j.id, "code", code, "metadataProvider", providerName)
			break
		}
		s.log.Warn("jav lookup result", "videoId", j.id, "code", code, "errorKind", lookupErrorKind(lookupErr), "provider", s.provider.Name())
		if !isNotFoundLookupError(lookupErr) || index == len(codes)-1 {
			s.handleLookupError(j.id, token, claimed, code, lookupErr)
			return
		}
		s.log.Info("jav candidate not found, trying next", "videoId", j.id, "code", code, "nextCode", codes[index+1], "candidateIndex", index+1)
	}
	if lookupErr != nil {
		s.handleLookupError(j.id, token, claimed, code, lookupErr)
		return
	}
	if m == nil {
		s.handleLookupError(j.id, token, claimed, code, NewLookupError("parse", 0, errors.New("provider returned nil metadata")))
		return
	}
	if strings.TrimSpace(m.Code) == "" {
		m.Code = code
	} else if normalizeCode(m.Code) != normalizeCode(code) {
		s.handleLookupError(j.id, token, claimed, code, NewLookupError("parse", 0, fmt.Errorf("provider returned code %q for requested code %q", m.Code, code)))
		return
	}
	if m.Provider == "" {
		m.Provider = s.provider.Name()
	}
	if err := s.repo.SaveSuccess(j.id, *m, source, time.Now().UnixMilli()); err != nil {
		// A transient write failure is retryable, but it must still get a
		// cooldown: a retryable task with no next_retry_at is never picked up
		// by the recovery scan, so it would wait in the queue forever.
		status, next := retryDecision(StatusRetryable, claimed.Attempts, claimed.MaxAttempts)
		s.failTask(j.id, token, claimed.Attempts, status, ErrorDatabase, err.Error(), next)
		return
	}
	// The profiles ride along with the metadata: a failure here is an enrichment
	// miss, never a reason to fail a scrape that produced a valid work.
	_ = s.persistActorProfiles(m.ActorProfiles, time.Now().UnixMilli())
	if err := s.processCover(j.id, m.CoverURL, m.Provider); err != nil {
		s.log.Warn("jav cover download failed", "videoId", j.id, "errorCode", ErrorCoverDownload, "err", err)
	}
	if err := s.repo.SetTaskStatus(j.id, token, StatusSucceeded, "", "", nil); err != nil {
		s.log.Error("jav task completion failed", "videoId", j.id, "errorCode", ErrorDatabase, "err", err)
		return
	}
	s.log.Info("jav task succeeded", "videoId", j.id, "code", m.Code, "provider", m.Provider, "attempts", claimed.Attempts)
	s.emitUpdate(j.id)
}

func (s *Service) processCover(videoID int64, rawURL, providerName string) error {
	if rawURL == "" || s.coverDir == "" {
		return s.repo.SetCover(videoID, CoverNone)
	}
	path, err := s.downloadCover(rawURL, videoID, providerName)
	if err != nil {
		_ = s.repo.SetCover(videoID, CoverFailed)
		return err
	}
	if err := s.repo.SetCover(videoID, CoverSucceeded); err != nil {
		return err
	}
	if err := s.videos.SetScrapedCover(videoID, path); err != nil {
		return err
	}
	if _, err := s.videos.ApplyScrapedCoverIfAllowed(videoID, path); err != nil {
		return err
	}
	return nil
}

func lookupErrorKind(err error) string {
	var lookupErr *LookupError
	if errors.As(err, &lookupErr) {
		return lookupErr.Kind
	}
	return "unknown"
}

func isNotFoundLookupError(err error) bool {
	var lookupErr *LookupError
	if !errors.As(err, &lookupErr) {
		return false
	}
	kind := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(lookupErr.Kind, "_", "-")))
	return kind == "not-found"
}

func (s *Service) handleLookupError(videoID int64, token string, task *Task, code string, err error) {
	status, errorCode := StatusRetryable, ErrorNetwork
	if le := new(LookupError); errors.As(err, &le) {
		kind := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(le.Kind, "_", "-")))
		switch kind {
		case "not-found":
			status, errorCode = StatusNotFound, ErrorNotFound
		case "blocked":
			status, errorCode = StatusBlocked, ErrorBlocked
		case "provider-auth", "provider-authentication", "age-verification", "age-verify", "driver-verify", "auth":
			status, errorCode = StatusBlocked, ErrorProviderAuth
		case "retryable":
			errorCode = ErrorRateLimited
		case "parse":
			status, errorCode = StatusBlocked, ErrorParse
		case "http":
			errorCode = ErrorHTTP
		}
	}
	if isProviderAuthError(err) {
		status, errorCode = StatusBlocked, ErrorProviderAuth
	}
	status, next := retryDecision(status, task.Attempts, task.MaxAttempts)
	s.failTask(videoID, token, task.Attempts, status, errorCode, err.Error(), next)
	s.logLookupFailure(videoID, code, task.Attempts, status, errorCode, err, next)
}

// retryDecision returns the status to persist after a failed attempt and, while
// the failure is still retryable, when the next attempt may start.
//
// A retryable failure only stays retryable while the task has both a cooldown
// to wait out and an attempt left. Once either is gone the task is exhausted
// and must terminate as failed: ClaimTask refuses an exhausted task, so leaving
// it retryable would keep it in the queue (and in the recovery scan) forever
// without ever being retried.
func retryDecision(status string, attempts, maxAttempts int) (string, *int64) {
	if status != StatusRetryable {
		return status, nil
	}
	cooldown, ok := retryCooldown(attempts)
	if !ok || attempts >= maxAttempts {
		return StatusFailed, nil
	}
	next := time.Now().Add(cooldown).UnixMilli()
	return StatusRetryable, &next
}

func retryCooldown(attempts int) (time.Duration, bool) {
	if attempts < 1 || attempts > len(retryCooldowns) {
		return 0, false
	}
	return retryCooldowns[attempts-1], true
}

func (s *Service) failTask(videoID int64, token string, attempts int, status, errorCode, message string, next *int64) {
	if err := s.repo.SetTaskStatus(videoID, token, status, errorCode, message, next); err != nil {
		s.log.Error("jav task failure persistence failed", "videoId", videoID, "errorCode", ErrorDatabase, "err", err)
		return
	}
	s.emitUpdate(videoID)
}

func (s *Service) finishWithoutLease(videoID int64, status, errorCode, message string) {
	if err := s.repo.SetTaskStatus(videoID, "", status, errorCode, message, nil); err != nil {
		s.log.Error("jav task status persistence failed", "videoId", videoID, "errorCode", ErrorDatabase, "err", err)
		return
	}
	if errorCode != "" {
		s.log.Warn("jav task finished without scrape", "videoId", videoID, "status", status, "errorCode", errorCode, "err", message)
	}
	s.emitUpdate(videoID)
}

func (s *Service) logLookupFailure(videoID int64, code string, attempts int, status, errorCode string, err error, next *int64) {
	attrs := []any{"videoId", videoID, "code", code, "provider", s.provider.Name(), "status", status, "errorCode", errorCode, "attempts", attempts, "err", err}
	if next != nil {
		attrs = append(attrs, "nextRetryAt", *next)
	}
	if status == StatusFailed {
		s.log.Error("jav task retry limit reached", attrs...)
		return
	}
	s.log.Warn("jav task lookup failed", attrs...)
}

func (s *Service) emitUpdate(id int64) {
	if s.emit == nil {
		return
	}
	d, err := s.repo.Get(id)
	if err != nil {
		return
	}
	s.emit("jav:scrape:updated", map[string]any{"videoId": id, "scrapeStatus": d.ScrapeStatus, "coverStatus": d.CoverStatus, "errorCode": d.ErrorCode, "lastError": d.LastError, "attempts": d.Attempts, "nextRetryAt": d.NextRetryAt})
}

func (s *Service) SetOverride(videoID int64, mode, code string) error {
	mode = strings.ToLower(strings.TrimSpace(mode))
	code = normalizeCode(code)
	if mode != OverrideAuto && mode != OverrideManual && mode != OverrideSkip {
		return fmt.Errorf("invalid override mode %q", mode)
	}
	if mode == OverrideManual && code == "" {
		return fmt.Errorf("manual JavBus code cannot be empty")
	}
	if err := s.repo.PrepareTask(videoID, false); err != nil {
		return err
	}
	if err := s.repo.SetOverride(videoID, mode, code); err != nil {
		return err
	}
	if mode == OverrideSkip {
		s.emitUpdate(videoID)
		return nil
	}
	if s.enqueueExisting(videoID, false) {
		s.emitUpdate(videoID)
	}
	return nil
}

func (s *Service) Delete(videoID int64) error {
	if err := s.repo.Delete(videoID); err != nil {
		return err
	}
	if s.coverDir != "" {
		if err := os.Remove(filepath.Join(s.coverDir, fmt.Sprintf("%d.jpg", videoID))); err != nil && !os.IsNotExist(err) {
			s.log.Warn("remove jav cover failed", "videoId", videoID, "errorCode", ErrorDatabase, "err", err)
		}
	}
	return nil
}

func normalizeCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

func isProviderAuthError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{"driver-verify", "age verification", "age-verification", "provider-auth", "provider auth"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func (s *Service) fetchCover(rawURL, providerName string) (fetchedCover, error) {
	ctx := context.Background()
	if p, ok := s.provider.(namedURLResponseFetcher); ok {
		res, err := p.FetchFor(ctx, providerName, rawURL)
		return responseCover(res, err)
	}
	switch p := s.provider.(type) {
	case coverResponseFetcher:
		res, err := p.FetchCover(ctx, rawURL)
		return responseCover(res, err)
	case coverBytesFetcher:
		body, err := p.FetchCover(ctx, rawURL)
		return bytesCover(body, err)
	case coverReaderFetcher:
		body, err := p.FetchCover(ctx, rawURL)
		return readerCover(body, err)
	case urlResponseFetcher:
		res, err := p.Fetch(ctx, rawURL)
		return responseCover(res, err)
	case urlBytesFetcher:
		body, err := p.Fetch(ctx, rawURL)
		return bytesCover(body, err)
	case urlReaderFetcher:
		body, err := p.Fetch(ctx, rawURL)
		return readerCover(body, err)
	case contextRequestResponseFetcher:
		req, err := coverRequest(ctx, rawURL)
		if err != nil {
			return fetchedCover{}, err
		}
		res, err := p.Do(ctx, req)
		return responseCover(res, err)
	case requestResponseFetcher:
		req, err := coverRequest(ctx, rawURL)
		if err != nil {
			return fetchedCover{}, err
		}
		res, err := p.Do(req)
		return responseCover(res, err)
	case httpClientProvider:
		client := p.HTTPClient()
		if client != nil {
			req, err := coverRequest(ctx, rawURL)
			if err != nil {
				return fetchedCover{}, err
			}
			res, err := client.Do(req)
			return responseCover(res, err)
		}
	}
	client := &http.Client{Timeout: 20 * time.Second}
	res, err := client.Get(rawURL)
	return responseCover(res, err)
}

func coverRequest(ctx context.Context, rawURL string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 Muse/1.0")
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	return req, nil
}

func responseCover(res *http.Response, err error) (fetchedCover, error) {
	if err != nil {
		return fetchedCover{}, err
	}
	if res == nil {
		return fetchedCover{}, errors.New("cover fetch returned nil response")
	}
	if res.Body == nil {
		return fetchedCover{}, errors.New("cover fetch returned empty body")
	}
	return fetchedCover{body: res.Body, statusCode: res.StatusCode, contentLength: res.ContentLength}, nil
}

func bytesCover(body []byte, err error) (fetchedCover, error) {
	if err != nil {
		return fetchedCover{}, err
	}
	return fetchedCover{body: io.NopCloser(bytes.NewReader(body)), statusCode: http.StatusOK, contentLength: int64(len(body))}, nil
}

func readerCover(body io.ReadCloser, err error) (fetchedCover, error) {
	if err != nil {
		return fetchedCover{}, err
	}
	if body == nil {
		return fetchedCover{}, errors.New("cover fetch returned empty body")
	}
	return fetchedCover{body: body, statusCode: http.StatusOK, contentLength: -1}, nil
}

func (s *Service) downloadCover(rawURL string, id int64, providerName string) (string, error) {
	name := fmt.Sprintf("%d.jpg", id)
	if err := s.saveJPEG(rawURL, providerName, s.coverDir, name); err != nil {
		return "", err
	}
	return filepath.ToSlash(filepath.Join("covers", "jav", name)), nil
}

// avatarNamePattern is the only shape a performer id may take to become a file
// name: avmoo ids are lowercase alphanumerics, and anything else would be a path
// traversal waiting to happen.
var avatarNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// downloadAvatar caches one performer's avatar next to the covers. The file is
// named after the provider's stable performer id, so a stage-name change reuses
// the same picture instead of orphaning the old one.
func (s *Service) downloadAvatar(rawURL, starID, providerName string) (string, error) {
	starID = strings.TrimSpace(starID)
	if !avatarNamePattern.MatchString(starID) {
		return "", fmt.Errorf("actor id %q cannot name an avatar file", starID)
	}
	name := starID + ".jpg"
	if err := s.saveJPEG(rawURL, providerName, s.avatarDir, name); err != nil {
		return "", err
	}
	return filepath.ToSlash(filepath.Join("avatars", name)), nil
}

// persistActorProfiles stores the profiles a lookup returned and then caches
// their avatars. The row is written before the download so a failed download
// still leaves the facts recorded; the avatar path simply stays empty until a
// later pass succeeds. It returns only storage errors: an unreachable avatar
// host is logged and never fails the caller.
func (s *Service) persistActorProfiles(profiles []ActorProfile, fetchedAt int64) error {
	if len(profiles) == 0 {
		return nil
	}
	for i := range profiles {
		if profiles[i].FetchedAt == nil {
			profiles[i].FetchedAt = &fetchedAt
		}
	}
	if err := s.repo.UpsertActorProfiles(profiles); err != nil {
		s.log.Warn("jav actor profiles not saved", "errorCode", ErrorDatabase, "err", err)
		return err
	}
	if s.avatarDir == "" {
		return nil
	}
	for _, profile := range profiles {
		if strings.TrimSpace(profile.AvatarURL) == "" || strings.TrimSpace(profile.StarID) == "" {
			continue
		}
		path, err := s.downloadAvatar(profile.AvatarURL, profile.StarID, profile.Provider)
		if err != nil {
			s.log.Warn("jav actor avatar download failed", "actor", profile.Name, "errorCode", ErrorCoverDownload, "err", err)
			continue
		}
		if err := s.repo.SetActorAvatarPath(profile.Name, path); err != nil {
			s.log.Warn("jav actor avatar path not saved", "actor", profile.Name, "errorCode", ErrorDatabase, "err", err)
		}
	}
	return nil
}

// saveJPEG downloads rawURL through the provider's transport and writes it as a
// JPEG into dir/name, replacing the target atomically. Covers and avatars share
// this path: both are small remote images that must end up as a local file the
// webview and the LAN client can read.
func (s *Service) saveJPEG(rawURL, providerName, dir, name string) error {
	fetched, err := s.fetchCover(rawURL, providerName)
	if err != nil {
		return err
	}
	defer func() { _ = fetched.body.Close() }()
	if fetched.statusCode < 200 || fetched.statusCode >= 300 {
		return fmt.Errorf("image http status %d", fetched.statusCode)
	}
	if fetched.contentLength > 10<<20 {
		return fmt.Errorf("image too large")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".image-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err = io.CopyN(tmp, io.LimitReader(fetched.body, 10<<20+1), 10<<20+1); err != nil && !errors.Is(err, io.EOF) {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	f, err := os.Open(tmpName)
	if err != nil {
		return err
	}
	decoded, _, decodeErr := image.Decode(f)
	closeErr := f.Close()
	if decodeErr != nil {
		return fmt.Errorf("invalid image: %w", decodeErr)
	}
	if closeErr != nil {
		return closeErr
	}

	encoded, err := os.CreateTemp(dir, ".image-jpeg-*")
	if err != nil {
		return err
	}
	encodedName := encoded.Name()
	defer func() { _ = os.Remove(encodedName) }()
	if err := jpeg.Encode(encoded, decoded, &jpeg.Options{Quality: 90}); err != nil {
		_ = encoded.Close()
		return fmt.Errorf("encode image as JPEG: %w", err)
	}
	if err := encoded.Close(); err != nil {
		return err
	}
	return os.Rename(encodedName, filepath.Join(dir, name))
}
