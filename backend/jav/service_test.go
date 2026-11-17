package jav

import (
	"bytes"
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"muse-app/backend/database"
	"muse-app/backend/video"
)

type fakeProvider struct{}

func (fakeProvider) Name() string { return "fake" }
func (fakeProvider) Lookup(context.Context, string) (*Metadata, error) {
	return &Metadata{Title: "Offline fixture", Actors: []string{"A"}}, nil
}

type recordingProvider struct {
	lookupCode string
}

func (p *recordingProvider) Name() string { return "recording" }
func (p *recordingProvider) Lookup(_ context.Context, code string) (*Metadata, error) {
	p.lookupCode = code
	return &Metadata{Title: "Recorded fixture"}, nil
}

type sequenceProvider struct {
	results map[string]sequenceResult
	calls   []string
}

type sequenceResult struct {
	metadata *Metadata
	err      error
}

func (p *sequenceProvider) Name() string { return "sequence" }
func (p *sequenceProvider) Lookup(_ context.Context, code string) (*Metadata, error) {
	p.calls = append(p.calls, code)
	result, ok := p.results[code]
	if !ok {
		return nil, NewLookupError("not_found", http.StatusNotFound, nil)
	}
	return result.metadata, result.err
}

type retryingProvider struct {
	calls atomic.Int32
}

func (p *retryingProvider) Name() string { return "retrying" }
func (p *retryingProvider) Lookup(context.Context, string) (*Metadata, error) {
	if p.calls.Add(1) == 1 {
		return nil, NewLookupError("not_found", http.StatusNotFound, nil)
	}
	return &Metadata{Code: "IPX-633", Title: "Recovered after retry"}, nil
}

// retryableProvider always reports a transient upstream failure, so a task
// using it can only leave the retry queue by exhausting its budget.
type retryableProvider struct{}

func (retryableProvider) Name() string { return "retryable-fixture" }
func (retryableProvider) Lookup(context.Context, string) (*Metadata, error) {
	return nil, NewLookupError("retryable", http.StatusTooManyRequests, errors.New("upstream temporarily unavailable"))
}

type metadataProvider struct {
	metadata   *Metadata
	lookupErr  error
	fetchCover func(context.Context, string) (*http.Response, error)
}

func (p metadataProvider) Name() string { return "fixture" }

func (p metadataProvider) Lookup(context.Context, string) (*Metadata, error) {
	if p.lookupErr != nil {
		return nil, p.lookupErr
	}
	return p.metadata, nil
}

func (p metadataProvider) FetchCover(ctx context.Context, rawURL string) (*http.Response, error) {
	if p.fetchCover == nil {
		return nil, errors.New("cover fetcher not configured")
	}
	return p.fetchCover(ctx, rawURL)
}

type noCoverFetcherProvider struct {
	metadata *Metadata
}

func (p noCoverFetcherProvider) Name() string { return "default-cover-fixture" }
func (p noCoverFetcherProvider) Lookup(context.Context, string) (*Metadata, error) {
	return p.metadata, nil
}

func newServiceFixture(t *testing.T, provider Provider, coverDir string, fileName string) (*Service, *sql.DB, video.VideoRepository, int64) {
	t.Helper()
	db, err := sql.Open(database.DriverName, "file:"+filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), fileName)
	if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	vr := video.NewSQLiteRepository(db)
	v := &video.Video{Path: path, FileName: fileName, FileSize: 7, Status: video.StatusAvailable, CreatedAt: 1, UpdatedAt: 1}
	if err := vr.Create(v); err != nil {
		t.Fatal(err)
	}
	s := NewService(NewSQLiteRepository(db), vr, provider, coverDir, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), 1)
	t.Cleanup(s.Close)
	return s, db, vr, v.ID
}

func waitForDetail(t *testing.T, s *Service, videoID int64, want func(*Detail) bool) *Detail {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		d, err := s.Get(videoID)
		if err == nil && want(d) {
			return d
		}
		time.Sleep(5 * time.Millisecond)
	}
	d, _ := s.Get(videoID)
	t.Fatalf("task did not reach expected state: %+v", d)
	return nil
}

// seedScrapedTitle makes an existing favorite look like a successful scrape:
// the translation candidate queries only see rows with a task in succeeded.
func seedScrapedTitle(t *testing.T, repo *SQLiteRepository, id int64, code, title string) {
	t.Helper()
	if err := repo.PrepareTask(id, false); err != nil {
		t.Fatalf("prepare task %s: %v", code, err)
	}
	if err := repo.SaveSuccess(id, Metadata{Code: code, Title: title}, "filename", 2); err != nil {
		t.Fatalf("save success %s: %v", code, err)
	}
	if err := repo.SetTaskStatus(id, "", StatusSucceeded, "", "", nil); err != nil {
		t.Fatalf("set task status %s: %v", code, err)
	}
}

func createScrapedFavorite(t *testing.T, repo *SQLiteRepository, vr video.VideoRepository, name, code, title string) int64 {
	t.Helper()
	v := &video.Video{
		Path:      filepath.Join(t.TempDir(), name),
		FileName:  name,
		FileSize:  1,
		Status:    video.StatusAvailable,
		CreatedAt: 1,
		UpdatedAt: 1,
	}
	if err := vr.Create(v); err != nil {
		t.Fatalf("create favorite %q: %v", name, err)
	}
	seedScrapedTitle(t, repo, v.ID, code, title)
	return v.ID
}

//go:embed testdata/cover.webp
var testWebP []byte

func testPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestServiceQueuesLargeBatchWithoutDroppingJobs(t *testing.T) {
	s := &Service{jobs: newJobQueue(), pending: make(map[int64]struct{})}
	const total = 300
	for i := 0; i < total; i++ {
		if !s.enqueueExisting(int64(i+1), false) {
			t.Fatalf("enqueueExisting(%d) failed", i+1)
		}
	}
	if got := len(s.pending); got != total {
		t.Fatalf("pending jobs=%d, want %d", got, total)
	}
	s.jobs.close()
}

func TestServiceUsesNormalizedFilenameCodeForLookup(t *testing.T) {
	tests := []struct {
		name string
		file string
		want string
	}{
		{name: "three digit leading zeros", file: "stko-004.mp4", want: "STKO-004"},
		{name: "leading zeros", file: "sivr00412_1_8k.mp4", want: "SIVR-412"},
		{name: "part suffix", file: "dv-1622B.mp4", want: "DV-1622"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &recordingProvider{}
			s, _, _, id := newServiceFixture(t, provider, "", tt.file)
			s.Enqueue(id)
			d := waitForDetail(t, s, id, func(d *Detail) bool { return d.ScrapeStatus == StatusSucceeded })
			if provider.lookupCode != tt.want {
				t.Fatalf("Lookup code = %q, want %q", provider.lookupCode, tt.want)
			}
			if d.Code != tt.want {
				t.Fatalf("saved code = %q, want %q", d.Code, tt.want)
			}
		})
	}
}

func TestServiceTriesNextFilenameCandidateAfterNotFound(t *testing.T) {
	provider := &sequenceProvider{results: map[string]sequenceResult{
		"HHD-800": {err: NewLookupError("not_found", http.StatusNotFound, nil)},
		"ABW-180": {metadata: &Metadata{Code: "ABW-180", Title: "ABW fallback candidate"}},
	}}
	s, _, _, id := newServiceFixture(t, provider, "", "hhd800.com@ABW-180.mp4")
	s.Enqueue(id)
	d := waitForDetail(t, s, id, func(d *Detail) bool { return d.ScrapeStatus == StatusSucceeded })
	if !reflect.DeepEqual(provider.calls, []string{"HHD-800", "ABW-180"}) {
		t.Fatalf("lookup calls = %#v", provider.calls)
	}
	if d.Code != "ABW-180" || d.Title != "ABW fallback candidate" || d.Attempts != 1 {
		t.Fatalf("detail = %+v", d)
	}
}

func TestServiceStopsFilenameCandidatesOnNonNotFoundError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{name: "blocked", err: NewLookupError("blocked", http.StatusForbidden, errors.New("blocked")), want: StatusBlocked},
		{name: "retryable", err: NewLookupError("retryable", http.StatusTooManyRequests, errors.New("rate limited")), want: StatusRetryable},
		{name: "parse", err: NewLookupError("parse", 0, errors.New("bad response")), want: StatusBlocked},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			provider := &sequenceProvider{results: map[string]sequenceResult{
				"HHD-800": {err: tc.err},
				"ABW-180": {metadata: &Metadata{Code: "ABW-180", Title: "must not be used"}},
			}}
			s, _, _, id := newServiceFixture(t, provider, "", "hhd800.com@ABW-180.mp4")
			s.Enqueue(id)
			d := waitForDetail(t, s, id, func(d *Detail) bool { return d.ScrapeStatus == tc.want })
			if !reflect.DeepEqual(provider.calls, []string{"HHD-800"}) {
				t.Fatalf("lookup calls = %#v", provider.calls)
			}
			if d.Code != "" || d.Title != "" {
				t.Fatalf("metadata should not be saved: %+v", d)
			}
		})
	}
}

func TestServiceAllFilenameCandidatesNotFound(t *testing.T) {
	provider := &sequenceProvider{results: map[string]sequenceResult{
		"HHD-800": {err: NewLookupError("not_found", http.StatusNotFound, nil)},
		"ABW-180": {err: NewLookupError("not_found", http.StatusNotFound, nil)},
	}}
	s, _, _, id := newServiceFixture(t, provider, "", "hhd800.com@ABW-180.mp4")
	s.Enqueue(id)
	d := waitForDetail(t, s, id, func(d *Detail) bool { return d.ScrapeStatus == StatusNotFound })
	if !reflect.DeepEqual(provider.calls, []string{"HHD-800", "ABW-180"}) {
		t.Fatalf("lookup calls = %#v", provider.calls)
	}
	if d.ErrorCode != ErrorNotFound || d.Attempts != 1 || d.NextRetryAt != nil {
		t.Fatalf("detail = %+v", d)
	}
}

func TestServiceProcessesQueuedVideoWithoutNetwork(t *testing.T) {
	db, err := sql.Open(database.DriverName, "file:"+filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer func() { _ = db.Close() }()
	if _, err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "IPX-633.mp4")
	if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	vr := video.NewSQLiteRepository(db)
	v := &video.Video{Path: path, FileName: filepath.Base(path), FileSize: 7, Status: video.StatusAvailable, CreatedAt: 1, UpdatedAt: 1}
	if err := vr.Create(v); err != nil {
		t.Fatal(err)
	}
	s := NewService(NewSQLiteRepository(db), vr, fakeProvider{}, "", nil, slog.New(slog.NewTextHandler(io.Discard, nil)), 1)
	defer s.Close()
	s.Enqueue(v.ID)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		d, err := s.Get(v.ID)
		if err == nil && d.ScrapeStatus == StatusSucceeded {
			if d.Code != "IPX-633" || d.Title != "Offline fixture" {
				t.Fatalf("detail = %+v", d)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	d, _ := s.Get(v.ID)
	t.Fatalf("queue did not finish: %+v", d)
}

func TestServiceRecoversExpiredRunningTask(t *testing.T) {
	db, err := sql.Open(database.DriverName, "file:"+filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer func() { _ = db.Close() }()
	if _, err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "IPX-633.mp4")
	if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	vr := video.NewSQLiteRepository(db)
	v := &video.Video{Path: path, FileName: filepath.Base(path), FileSize: 7, Status: video.StatusAvailable, CreatedAt: 1, UpdatedAt: 1}
	if err := vr.Create(v); err != nil {
		t.Fatal(err)
	}
	repo := NewSQLiteRepository(db)
	if err := repo.PrepareTask(v.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE jav_scrape_tasks SET status='running', lease_until=1 WHERE video_id=?`, v.ID); err != nil {
		t.Fatal(err)
	}
	s := NewService(repo, vr, fakeProvider{}, "", nil, slog.New(slog.NewTextHandler(io.Discard, nil)), 1)
	s.Start()
	defer s.Close()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		d, err := s.Get(v.ID)
		if err == nil && d.ScrapeStatus == StatusSucceeded {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	d, _ := s.Get(v.ID)
	t.Fatalf("recovered task did not finish: %+v", d)
}

func TestServiceMapsProviderAuthToBlockedProviderAuth(t *testing.T) {
	s, _, _, id := newServiceFixture(t, metadataProvider{
		lookupErr: NewLookupError("provider-auth", http.StatusForbidden, errors.New("age verification required")),
	}, "", "IPX-633.mp4")
	s.Enqueue(id)
	d := waitForDetail(t, s, id, func(d *Detail) bool { return d.ScrapeStatus == StatusBlocked })
	if d.ErrorCode != ErrorProviderAuth {
		t.Fatalf("error code = %q, want %q", d.ErrorCode, ErrorProviderAuth)
	}
	if d.LastError != "provider-auth: age verification required" {
		t.Fatalf("last error = %q", d.LastError)
	}
	task, err := s.GetTask(id)
	if err != nil || task.Status != StatusBlocked || task.Attempts != 1 {
		t.Fatalf("task = %+v, err = %v", task, err)
	}
}

func TestServiceRejectsMetadataCodeMismatchWithoutSavingSuccess(t *testing.T) {
	s, _, _, id := newServiceFixture(t, metadataProvider{metadata: &Metadata{
		Code: "WRONG-999", Title: "Must not save", Provider: "fixture",
	}}, "", "IPX-633.mp4")
	s.Enqueue(id)
	d := waitForDetail(t, s, id, func(d *Detail) bool { return d.ScrapeStatus == StatusBlocked })
	if d.ErrorCode != ErrorParse {
		t.Fatalf("error code = %q, want %q", d.ErrorCode, ErrorParse)
	}
	if d.Code != "" || d.Title != "" || d.Provider != "" || d.FetchedAt != nil {
		t.Fatalf("mismatched metadata was saved: %+v", d)
	}
}

func TestServiceConvertsWebPCoverToJPEG(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/webp")
		_, _ = w.Write(testWebP)
	}))
	defer server.Close()

	coverDir := t.TempDir()
	s, _, vr, id := newServiceFixture(t, noCoverFetcherProvider{metadata: &Metadata{
		Code: "IPX-633", Title: "WebP cover", CoverURL: server.URL + "/cover.webp",
	}}, coverDir, "IPX-633.mp4")
	s.Enqueue(id)
	d := waitForDetail(t, s, id, func(d *Detail) bool { return d.ScrapeStatus == StatusSucceeded })
	if d.CoverStatus != CoverSucceeded {
		t.Fatalf("cover status = %q, want %q", d.CoverStatus, CoverSucceeded)
	}
	v, err := vr.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if v.ScrapedCoverPath != fmt.Sprintf("covers/jav/%d.jpg", id) {
		t.Fatalf("scraped cover path = %q", v.ScrapedCoverPath)
	}
	cover, err := os.Open(filepath.Join(coverDir, fmt.Sprintf("%d.jpg", id)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cover.Close() }()
	if _, err := jpeg.Decode(cover); err != nil {
		t.Fatalf("saved cover is not JPEG: %v", err)
	}
}

func TestServiceUsesProviderCoverFetcherAndReplacesCustomCoverByDefault(t *testing.T) {
	coverBytes := testPNG(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if got := r.Header.Get("X-Shared-Fetch"); got != "true" {
			t.Errorf("shared fetch header = %q, want true", got)
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(coverBytes)
	}))
	defer server.Close()

	provider := metadataProvider{metadata: &Metadata{
		Code: "IPX-633", Title: "With cover", CoverURL: server.URL + "/cover.jpg", Provider: "fixture",
	}, fetchCover: func(ctx context.Context, rawURL string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("X-Shared-Fetch", "true")
		return http.DefaultClient.Do(req)
	}}
	coverDir := t.TempDir()
	s, _, vr, id := newServiceFixture(t, provider, coverDir, "IPX-633.mp4")
	if err := vr.UpdateThumbnail(id, "thumbnails/custom.jpg"); err != nil {
		t.Fatal(err)
	}
	if err := vr.SetCoverSource(id, "manual"); err != nil {
		t.Fatal(err)
	}
	if err := vr.SetCustomCover(id, time.Now().UnixMilli()); err != nil {
		t.Fatal(err)
	}

	s.Enqueue(id)
	d := waitForDetail(t, s, id, func(d *Detail) bool { return d.ScrapeStatus == StatusSucceeded })
	if requests.Load() != 1 {
		t.Fatalf("cover requests = %d, want 1", requests.Load())
	}
	if d.CoverStatus != CoverSucceeded {
		t.Fatalf("cover status = %q, want %q", d.CoverStatus, CoverSucceeded)
	}
	v, err := vr.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if v.ThumbnailPath != v.ScrapedCoverPath || v.CoverSource != "scraped" || v.CustomCoverAt != 0 {
		t.Fatalf("scraped cover was not applied by default: %+v", v)
	}
	if v.ScrapedCoverPath == "" || !strings.HasSuffix(v.ScrapedCoverPath, ".jpg") {
		t.Fatalf("scraped cover path = %q", v.ScrapedCoverPath)
	}
	if _, err := os.Stat(filepath.Join(coverDir, fmt.Sprintf("%d.jpg", id))); err != nil {
		t.Fatalf("cover file missing: %v", err)
	}
}

type namedCoverProvider struct {
	metadata *Metadata
	fetch    func(context.Context, string, string) (*http.Response, error)
}

func (p namedCoverProvider) Name() string { return "javbus-avmoo" }
func (p namedCoverProvider) Lookup(context.Context, string) (*Metadata, error) {
	return p.metadata, nil
}
func (p namedCoverProvider) FetchFor(ctx context.Context, providerName, rawURL string) (*http.Response, error) {
	return p.fetch(ctx, providerName, rawURL)
}

func TestServiceRoutesCoverUsingMetadataProvider(t *testing.T) {
	coverBytes := testPNG(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Cover-Provider") != "avmoo" {
			t.Errorf("cover provider header = %q, want avmoo", r.Header.Get("X-Cover-Provider"))
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(coverBytes)
	}))
	defer server.Close()

	provider := namedCoverProvider{
		metadata: &Metadata{Code: "IPX-633", Title: "Avmoo metadata", CoverURL: server.URL + "/cover.jpg", Provider: "avmoo"},
		fetch: func(ctx context.Context, providerName, rawURL string) (*http.Response, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
			if err != nil {
				return nil, err
			}
			req.Header.Set("X-Cover-Provider", providerName)
			return http.DefaultClient.Do(req)
		},
	}
	s, _, vr, id := newServiceFixture(t, provider, t.TempDir(), "IPX-633.mp4")
	s.Enqueue(id)
	d := waitForDetail(t, s, id, func(d *Detail) bool { return d.ScrapeStatus == StatusSucceeded })
	if d.Provider != "avmoo" || d.CoverStatus != CoverSucceeded {
		t.Fatalf("detail = %+v", d)
	}
	v, err := vr.Get(id)
	if err != nil || v.ScrapedCoverPath == "" {
		t.Fatalf("video cover = %+v, err=%v", v, err)
	}
}

func TestServiceKeepsCustomCoverWhenScrapedCoverIsDisabled(t *testing.T) {
	coverBytes := testPNG(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(coverBytes)
	}))
	defer server.Close()

	provider := metadataProvider{metadata: &Metadata{
		Code: "IPX-633", Title: "Opt out", CoverURL: server.URL + "/cover.jpg", Provider: "fixture",
	}, fetchCover: func(ctx context.Context, rawURL string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, err
		}
		return http.DefaultClient.Do(req)
	}}

	coverDir := t.TempDir()
	s, _, vr, id := newServiceFixture(t, provider, coverDir, "IPX-633.mp4")
	if err := vr.UpdateThumbnail(id, "thumbnails/custom.jpg"); err != nil {
		t.Fatal(err)
	}
	if err := vr.SetCoverSource(id, "custom_frame"); err != nil {
		t.Fatal(err)
	}
	if err := vr.SetCustomCover(id, time.Now().UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if err := vr.SetScrapedCoverAllowed(id, false); err != nil {
		t.Fatal(err)
	}

	s.Enqueue(id)
	waitForDetail(t, s, id, func(d *Detail) bool { return d.ScrapeStatus == StatusSucceeded })
	v, err := vr.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if v.ThumbnailPath != "thumbnails/custom.jpg" || v.CoverSource != "custom_frame" || v.CustomCoverAt == 0 {
		t.Fatalf("custom cover was overwritten despite opt-out: %+v", v)
	}
	if v.ScrapedCoverPath == "" {
		t.Fatal("scraped cover was not retained as a backup")
	}
	if err := vr.SetScrapedCoverAllowed(id, true); err != nil {
		t.Fatal(err)
	}
	v, err = vr.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if v.ThumbnailPath != v.ScrapedCoverPath || v.CoverSource != "scraped" || v.CustomCoverAt != 0 {
		t.Fatalf("enabling scraped cover did not apply the backup: %+v", v)
	}
}

func TestServiceDefaultCoverFetcherRemainsAvailable(t *testing.T) {
	coverBytes := testPNG(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(coverBytes)
	}))
	defer server.Close()

	coverDir := t.TempDir()
	s, _, vr, id := newServiceFixture(t, noCoverFetcherProvider{metadata: &Metadata{
		Code: "IPX-633", Title: "Default fetch", CoverURL: server.URL + "/cover.jpg",
	}}, coverDir, "IPX-633.mp4")
	s.Enqueue(id)
	waitForDetail(t, s, id, func(d *Detail) bool { return d.ScrapeStatus == StatusSucceeded })
	v, err := vr.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if v.ScrapedCoverPath == "" {
		t.Fatal("default cover fetch did not save a cover")
	}
}

func TestServiceOverrideSavesAndSchedulesMode(t *testing.T) {
	s, _, _, id := newServiceFixture(t, fakeProvider{}, "", "IPX-633.mp4")

	if err := s.SetOverride(id, OverrideManual, " smd-999 "); err != nil {
		t.Fatalf("set manual override: %v", err)
	}
	d := waitForDetail(t, s, id, func(d *Detail) bool { return d.ScrapeStatus == StatusSucceeded })
	if d.Code != "SMD-999" || d.OverrideMode != OverrideManual || d.CodeSource != "manual" {
		t.Fatalf("manual override detail = %+v", d)
	}

	if err := s.SetOverride(id, OverrideSkip, ""); err != nil {
		t.Fatalf("set skip override: %v", err)
	}
	task, err := s.GetTask(id)
	if err != nil {
		t.Fatalf("get disabled task: %v", err)
	}
	if task.Status != StatusDisabled {
		t.Fatalf("disabled task status = %q, want %q", task.Status, StatusDisabled)
	}

	if err := s.SetOverride(id, OverrideAuto, ""); err != nil {
		t.Fatalf("set auto override: %v", err)
	}
	d = waitForDetail(t, s, id, func(d *Detail) bool {
		return d.ScrapeStatus == StatusSucceeded && d.Code == "IPX-633"
	})
	if d.OverrideMode != OverrideAuto || d.CodeSource != "filename" {
		t.Fatalf("auto override detail = %+v", d)
	}
}

func TestServiceEnqueueVideosSkipsSuccessfulAndDisabledTasks(t *testing.T) {
	s, _, _, id := newServiceFixture(t, fakeProvider{}, "", "IPX-633.mp4")

	s.Enqueue(id)
	waitForDetail(t, s, id, func(d *Detail) bool { return d.ScrapeStatus == StatusSucceeded })

	queued, err := s.EnqueueVideos([]int64{id}, true)
	if err != nil {
		t.Fatalf("enqueue unprocessed: %v", err)
	}
	if queued != 0 {
		t.Fatalf("unprocessed queued = %d, want 0 for succeeded task", queued)
	}

	queued, err = s.EnqueueVideos([]int64{id}, false)
	if err != nil {
		t.Fatalf("enqueue force: %v", err)
	}
	if queued != 1 {
		t.Fatalf("force queued = %d, want 1", queued)
	}

	if err := s.SetOverride(id, OverrideSkip, ""); err != nil {
		t.Fatalf("set skip: %v", err)
	}
	queued, err = s.EnqueueVideos([]int64{id}, false)
	if err != nil {
		t.Fatalf("enqueue skipped: %v", err)
	}
	if queued != 0 {
		t.Fatalf("skipped queued = %d, want 0", queued)
	}
}

func TestServiceEnqueueVideosRetriesNotFoundTasksWhenOnlyUnprocessed(t *testing.T) {
	provider := &retryingProvider{}
	s, _, _, id := newServiceFixture(t, provider, "", "IPX-633.mp4")

	s.Enqueue(id)
	waitForDetail(t, s, id, func(d *Detail) bool { return d.ScrapeStatus == StatusNotFound })

	queued, err := s.EnqueueVideos([]int64{id}, true)
	if err != nil {
		t.Fatalf("enqueue unprocessed: %v", err)
	}
	if queued != 1 {
		t.Fatalf("not-found task queued = %d, want 1", queued)
	}

	d := waitForDetail(t, s, id, func(d *Detail) bool { return d.ScrapeStatus == StatusSucceeded })
	if provider.calls.Load() != 2 {
		t.Fatalf("provider calls = %d, want 2", provider.calls.Load())
	}
	if d.Code != "IPX-633" || d.Title != "Recovered after retry" || d.Attempts != 1 {
		t.Fatalf("retried detail = %+v", d)
	}
}

func TestRepositoryListsTaskOverviewsInCreationOrder(t *testing.T) {
	s, db, vr, firstID := newServiceFixture(t, fakeProvider{}, "", "first.mp4")
	defer s.Close()

	secondPath := filepath.Join(t.TempDir(), "second.mp4")
	if err := os.WriteFile(secondPath, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	second := &video.Video{
		Path: secondPath, FileName: "second.mp4", FileSize: 7,
		Status: video.StatusAvailable, CreatedAt: 2, UpdatedAt: 2,
	}
	if err := vr.Create(second); err != nil {
		t.Fatal(err)
	}
	repo := NewSQLiteRepository(db)
	if err := repo.PrepareTask(firstID, false); err != nil {
		t.Fatal(err)
	}
	if err := repo.PrepareTask(second.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE jav_scrape_tasks SET status=?, created_at=?, updated_at=? WHERE video_id=?`, StatusSucceeded, 100, 100, firstID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE jav_scrape_tasks SET status=?, error_code=?, last_error=?, attempts=?, created_at=?, updated_at=? WHERE video_id=?`, StatusBlocked, ErrorBlocked, "blocked by provider", 2, 200, 200, second.ID); err != nil {
		t.Fatal(err)
	}

	overviews, err := repo.ListTaskOverviews()
	if err != nil {
		t.Fatal(err)
	}
	if len(overviews) != 2 {
		t.Fatalf("overview count = %d, want 2", len(overviews))
	}
	if overviews[0].VideoID != second.ID || overviews[1].VideoID != firstID {
		t.Fatalf("overview order = [%d, %d], want [%d, %d]", overviews[0].VideoID, overviews[1].VideoID, second.ID, firstID)
	}
	if overviews[0].Category != TaskCategoryFailed || overviews[0].ErrorCode != ErrorBlocked || overviews[0].Attempts != 2 {
		t.Fatalf("failed overview = %+v", overviews[0])
	}
	if overviews[1].Category != TaskCategorySucceeded || overviews[1].Video.FileName != "first.mp4" {
		t.Fatalf("successful overview = %+v", overviews[1])
	}

	if err := repo.PrepareTask(firstID, true); err != nil {
		t.Fatal(err)
	}
	var taskCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM jav_scrape_tasks WHERE video_id=?`, firstID).Scan(&taskCount); err != nil {
		t.Fatal(err)
	}
	if taskCount != 1 {
		t.Fatalf("task count for one video = %d, want 1", taskCount)
	}
	var createdAt int64
	if err := db.QueryRow(`SELECT created_at FROM jav_scrape_tasks WHERE video_id=?`, firstID).Scan(&createdAt); err != nil {
		t.Fatal(err)
	}
	if createdAt <= 100 {
		t.Fatalf("force requeue created_at = %d, want newer than 100", createdAt)
	}
}

func TestTaskCategoryMapsWorkerStates(t *testing.T) {
	for _, test := range []struct {
		status string
		want   string
	}{
		{StatusQueued, TaskCategoryQueued},
		{StatusRunning, TaskCategoryQueued},
		{StatusRetryable, TaskCategoryQueued},
		{StatusSucceeded, TaskCategorySucceeded},
		{StatusNoCode, TaskCategoryFailed},
		{StatusNotFound, TaskCategoryFailed},
		{StatusBlocked, TaskCategoryFailed},
		{StatusDisabled, TaskCategoryFailed},
		{StatusFailed, TaskCategoryFailed},
		{"future_terminal_state", TaskCategoryFailed},
	} {
		if got := TaskCategory(test.status); got != test.want {
			t.Errorf("TaskCategory(%q) = %q, want %q", test.status, got, test.want)
		}
	}
}

func TestRetryCooldown(t *testing.T) {
	tests := []struct {
		attempts int
		want     time.Duration
	}{
		{attempts: 1, want: 20 * time.Second},
		{attempts: 2, want: 40 * time.Second},
		{attempts: 3, want: 60 * time.Second},
	}
	for _, tt := range tests {
		got, ok := retryCooldown(tt.attempts)
		if !ok || got != tt.want {
			t.Errorf("retryCooldown(%d) = (%s, %t), want (%s, true)", tt.attempts, got, ok, tt.want)
		}
	}

	for _, attempts := range []int{0, 4} {
		if got, ok := retryCooldown(attempts); ok || got != 0 {
			t.Errorf("retryCooldown(%d) = (%s, %t), want (0s, false)", attempts, got, ok)
		}
	}
}

// TestRetryDecisionTerminatesExhaustedRetries pins the rule that gives a
// retryable failure an exit. Retryable is transient: once the task has no
// cooldown left, or no attempt left, it has to become terminal, otherwise it
// stays in the queue forever (ClaimTask refuses it, the recovery scan keeps
// matching it).
func TestRetryDecisionTerminatesExhaustedRetries(t *testing.T) {
	tests := []struct {
		name        string
		status      string
		attempts    int
		maxAttempts int
		want        string
		wantNext    bool
	}{
		{name: "initial failure waits out its cooldown", status: StatusRetryable, attempts: 1, maxAttempts: 4, want: StatusRetryable, wantNext: true},
		{name: "second failure waits out its cooldown", status: StatusRetryable, attempts: 2, maxAttempts: 4, want: StatusRetryable, wantNext: true},
		{name: "third failure waits out its cooldown", status: StatusRetryable, attempts: 3, maxAttempts: 4, want: StatusRetryable, wantNext: true},
		{name: "last attempt of the budget terminates", status: StatusRetryable, attempts: 4, maxAttempts: 4, want: StatusFailed},
		{name: "shorter stored budget terminates", status: StatusRetryable, attempts: 3, maxAttempts: 3, want: StatusFailed},
		{name: "running out of cooldowns terminates", status: StatusRetryable, attempts: 4, maxAttempts: 9, want: StatusFailed},
		{name: "not found stays terminal", status: StatusNotFound, attempts: 1, maxAttempts: 4, want: StatusNotFound},
		{name: "blocked stays terminal", status: StatusBlocked, attempts: 1, maxAttempts: 4, want: StatusBlocked},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, next := retryDecision(tt.status, tt.attempts, tt.maxAttempts)
			if got != tt.want {
				t.Errorf("retryDecision(%q, %d, %d) status = %q, want %q", tt.status, tt.attempts, tt.maxAttempts, got, tt.want)
			}
			if tt.wantNext != (next != nil) {
				t.Errorf("retryDecision(%q, %d, %d) next = %v, want scheduled=%t", tt.status, tt.attempts, tt.maxAttempts, next, tt.wantNext)
			}
		})
	}
}

// TestServiceFailsTaskAfterRetryBudgetExhausted covers the state a task must
// reach when it runs out of retries: a terminal failure the recovery scan
// leaves alone, instead of a permanently pending retryable entry.
func TestServiceFailsTaskAfterRetryBudgetExhausted(t *testing.T) {
	s, db, _, id := newServiceFixture(t, retryableProvider{}, "", "IPX-633.mp4")
	// The task row has to exist before its budget can be tightened, and a
	// one-attempt budget reaches exhaustion without waiting out a cooldown.
	if err := NewSQLiteRepository(db).PrepareTask(id, false); err != nil {
		t.Fatal(err)
	}
	res, err := db.Exec(`UPDATE jav_scrape_tasks SET max_attempts=1 WHERE video_id=?`, id)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := res.RowsAffected(); err != nil || n != 1 {
		t.Fatalf("tightening the retry budget touched %d rows (err=%v), want 1", n, err)
	}
	s.Enqueue(id)
	d := waitForDetail(t, s, id, func(d *Detail) bool { return d.ScrapeStatus == StatusFailed })
	if d.ErrorCode != ErrorRateLimited {
		t.Fatalf("error code = %q, want %q", d.ErrorCode, ErrorRateLimited)
	}

	task, err := s.GetTask(id)
	if err != nil {
		t.Fatal(err)
	}
	if task.Attempts != 1 || task.MaxAttempts != 1 {
		t.Fatalf("task attempts = %d/%d, want 1/1", task.Attempts, task.MaxAttempts)
	}
	if task.NextRetryAt != nil {
		t.Fatalf("exhausted task is still scheduled for retry at %d", *task.NextRetryAt)
	}
	if got := TaskCategory(task.Status); got != TaskCategoryFailed {
		t.Fatalf("exhausted task category = %q, want %q", got, TaskCategoryFailed)
	}

	recovered, err := NewSQLiteRepository(db).RecoverPending(time.Now().UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range recovered {
		if r.VideoID == id {
			t.Fatalf("exhausted task %d is still recovered by the retry scan", id)
		}
	}
}

// TestRepositorySkipsExhaustedRetryableTasksInRecovery pins the scan's filter.
// An exhausted retryable task cannot be claimed, so recovering it would spin at
// the retry scan interval forever without ever making progress — which is what
// produced a queue that only ever grew.
func TestRepositorySkipsExhaustedRetryableTasksInRecovery(t *testing.T) {
	db, err := sql.Open(database.DriverName, "file:"+filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer func() { _ = db.Close() }()
	if _, err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	vr := video.NewSQLiteRepository(db)
	repo := NewSQLiteRepository(db)
	due := time.Now().Add(-time.Hour).UnixMilli()

	ids := make([]int64, 0, 3)
	for _, name := range []string{"exhausted.mp4", "budget-left.mp4", "queued.mp4"} {
		path := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
		v := &video.Video{Path: path, FileName: name, FileSize: 7, Status: video.StatusAvailable, CreatedAt: 1, UpdatedAt: 1}
		if err := vr.Create(v); err != nil {
			t.Fatal(err)
		}
		if err := repo.PrepareTask(v.ID, false); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, v.ID)
	}
	if _, err := db.Exec(`UPDATE jav_scrape_tasks SET status='retryable', attempts=3, max_attempts=3, next_retry_at=? WHERE video_id=?`, due, ids[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE jav_scrape_tasks SET status='retryable', attempts=1, max_attempts=4, next_retry_at=? WHERE video_id=?`, due, ids[1]); err != nil {
		t.Fatal(err)
	}
	// ids[2] keeps the default queued state.

	recovered, err := repo.RecoverPending(time.Now().UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	got := make(map[int64]bool, len(recovered))
	for _, task := range recovered {
		got[task.VideoID] = true
	}
	if got[ids[0]] {
		t.Error("a retryable task with no attempts left must not be recovered")
	}
	if !got[ids[1]] {
		t.Error("a retryable task with an attempt left and an elapsed cooldown must be recovered")
	}
	if !got[ids[2]] {
		t.Error("a queued task must be recovered")
	}
}

// fakeTitleTranslator is a controllable TitleTranslator: configured gates the
// scrape-time trigger, started/release let a test hold a request in flight, and
// result/err describe the provider's answer. batches records every packed call
// so a test can assert that bulk work is amortized instead of sent one by one.
type fakeTitleTranslator struct {
	configured atomic.Bool
	started    chan struct{}
	release    chan struct{}
	once       sync.Once
	mu         sync.Mutex
	titles     []string
	batches    [][]string
	result     string
	err        error
}

func (f *fakeTitleTranslator) Configured() bool { return f.configured.Load() }

func (f *fakeTitleTranslator) Available() bool { return f.configured.Load() }

func (f *fakeTitleTranslator) TranslateTitle(ctx context.Context, title string) (string, error) {
	results, errs := f.translate(ctx, []string{title})
	return results[0], errs[0]
}

func (f *fakeTitleTranslator) TranslateTitles(ctx context.Context, titles []string) ([]string, []error) {
	return f.translate(ctx, titles)
}

func (f *fakeTitleTranslator) translate(ctx context.Context, titles []string) ([]string, []error) {
	f.mu.Lock()
	f.titles = append(f.titles, titles...)
	f.batches = append(f.batches, append([]string(nil), titles...))
	result, err := f.result, f.err
	f.mu.Unlock()

	if f.started != nil && f.release != nil {
		f.once.Do(func() { close(f.started) })
		select {
		case <-f.release:
		case <-ctx.Done():
			results := make([]string, len(titles))
			errs := make([]error, len(titles))
			for i := range errs {
				errs[i] = ctx.Err()
			}
			return results, errs
		}
	}
	results := make([]string, len(titles))
	errs := make([]error, len(titles))
	for i := range titles {
		results[i] = result
		errs[i] = err
	}
	return results, errs
}

func (f *fakeTitleTranslator) translatedTitles() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.titles...)
}

func (f *fakeTitleTranslator) translatedBatches() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]string, len(f.batches))
	for i, batch := range f.batches {
		out[i] = append([]string(nil), batch...)
	}
	return out
}

// blockingBatchTranslator holds every packed request until release and records
// the highest number of concurrent calls, so a test can prove the worker pool
// is bounded at the configured in-flight limit.
type blockingBatchTranslator struct {
	configured atomic.Bool
	release    chan struct{}
	entered    chan struct{}
	active     atomic.Int32
	maxActive  atomic.Int32
}

func (b *blockingBatchTranslator) Configured() bool { return b.configured.Load() }

func (b *blockingBatchTranslator) Available() bool { return b.configured.Load() }

func (b *blockingBatchTranslator) TranslateTitle(ctx context.Context, title string) (string, error) {
	results, errs := b.TranslateTitles(ctx, []string{title})
	return results[0], errs[0]
}

func (b *blockingBatchTranslator) TranslateTitles(ctx context.Context, titles []string) ([]string, []error) {
	current := b.active.Add(1)
	for {
		peak := b.maxActive.Load()
		if current <= peak || b.maxActive.CompareAndSwap(peak, current) {
			break
		}
	}
	select {
	case b.entered <- struct{}{}:
	default:
	}

	select {
	case <-b.release:
	case <-ctx.Done():
	}
	b.active.Add(-1)

	results := make([]string, len(titles))
	errs := make([]error, len(titles))
	for i := range titles {
		results[i] = "译名"
	}
	return results, errs
}

func TestServiceTranslatesTitleAfterScrape(t *testing.T) {
	provider := &sequenceProvider{results: map[string]sequenceResult{
		"IPX-633": {metadata: &Metadata{Code: "IPX-633", Title: "となりの人妻"}},
	}}
	s, _, _, id := newServiceFixture(t, provider, "", "IPX-633.mp4")
	translator := &fakeTitleTranslator{result: "邻家的人妻"}
	translator.configured.Store(true)
	s.SetTitleTranslator(translator)

	s.Enqueue(id)
	d := waitForDetail(t, s, id, func(d *Detail) bool { return d.TitleZhStatus == TitleZhSucceeded })
	if d.TitleZh != "邻家的人妻" {
		t.Fatalf("titleZh = %q, want 邻家的人妻", d.TitleZh)
	}
	if d.TitleZhError != "" {
		t.Fatalf("titleZhError = %q, want empty", d.TitleZhError)
	}
	if d.ScrapeStatus != StatusSucceeded {
		t.Fatalf("scrape status = %q, want succeeded", d.ScrapeStatus)
	}
	if got := translator.translatedTitles(); len(got) != 1 || got[0] != "となりの人妻" {
		t.Fatalf("translated titles = %#v, want the scraped title", got)
	}
}

func TestServiceSkipsTranslationWhenProviderNotConfigured(t *testing.T) {
	s, _, _, id := newServiceFixture(t, fakeProvider{}, "", "IPX-633.mp4")
	translator := &fakeTitleTranslator{result: "不应出现"}
	s.SetTitleTranslator(translator)

	s.Enqueue(id)
	d := waitForDetail(t, s, id, func(d *Detail) bool { return d.ScrapeStatus == StatusSucceeded })
	if d.TitleZh != "" || d.TitleZhStatus != TitleZhNone {
		t.Fatalf("translation = (%q, %q), want none while unconfigured", d.TitleZh, d.TitleZhStatus)
	}
	// Give a stray worker a moment to (incorrectly) run before asserting.
	time.Sleep(20 * time.Millisecond)
	if got := translator.translatedTitles(); len(got) != 0 {
		t.Fatalf("translator was called while unconfigured: %#v", got)
	}
}

func TestServiceRecordsTranslationFailureWithoutFailingScrape(t *testing.T) {
	s, _, _, id := newServiceFixture(t, fakeProvider{}, "", "IPX-633.mp4")
	translator := &fakeTitleTranslator{err: errors.New("LLM 鉴权失败（401）：请检查 API Key 是否有权限")}
	translator.configured.Store(true)
	s.SetTitleTranslator(translator)

	s.Enqueue(id)
	d := waitForDetail(t, s, id, func(d *Detail) bool { return d.TitleZhStatus == TitleZhFailed })
	if d.ScrapeStatus != StatusSucceeded {
		t.Fatalf("scrape status = %q, want succeeded", d.ScrapeStatus)
	}
	if d.TitleZh != "" {
		t.Fatalf("titleZh = %q, want empty on failure", d.TitleZh)
	}
	if !strings.Contains(d.TitleZhError, "API Key") {
		t.Fatalf("titleZhError = %q, want the provider message", d.TitleZhError)
	}
}

func TestServiceBulkTranslatesMissingTitles(t *testing.T) {
	s, _, _, id := newServiceFixture(t, fakeProvider{}, "", "IPX-633.mp4")
	translator := &fakeTitleTranslator{result: "邻家的人妻"}
	s.SetTitleTranslator(translator)

	// Scraped while translation was off: the work lands without a translation.
	s.Enqueue(id)
	waitForDetail(t, s, id, func(d *Detail) bool { return d.ScrapeStatus == StatusSucceeded })

	translator.configured.Store(true)
	queued, err := s.TranslateMissingTitles()
	if err != nil {
		t.Fatalf("translate missing: %v", err)
	}
	if queued != 1 {
		t.Fatalf("queued = %d, want 1", queued)
	}
	// Pressing again while the work is queued must not enqueue it twice.
	if again, err := s.TranslateMissingTitles(); err != nil || again != 0 {
		t.Fatalf("second translate missing = %d, %v; want 0, nil", again, err)
	}
	d := waitForDetail(t, s, id, func(d *Detail) bool { return d.TitleZhStatus == TitleZhSucceeded })
	if d.TitleZh != "邻家的人妻" {
		t.Fatalf("titleZh = %q, want 邻家的人妻", d.TitleZh)
	}
}

// TestServicePacksQueuedTitlesIntoOneRequest proves a bulk pass is amortized:
// three queued favorites reach the translator as one request instead of three
// requests that each pay the full system prompt.
func TestServicePacksQueuedTitlesIntoOneRequest(t *testing.T) {
	s, db, vr, first := newServiceFixture(t, fakeProvider{}, "", "IPX-633.mp4")
	repo := NewSQLiteRepository(db)
	seedScrapedTitle(t, repo, first, "IPX-633", "一つ目")
	ids := []int64{first}
	for _, item := range []struct{ name, code, title string }{
		{"ABW-180.mp4", "ABW-180", "二つ目"},
		{"SONE-364.mp4", "SONE-364", "三つ目"},
	} {
		ids = append(ids, createScrapedFavorite(t, repo, vr, item.name, item.code, item.title))
	}

	translator := &fakeTitleTranslator{result: "译名"}
	translator.configured.Store(true)
	s.SetTitleTranslator(translator)

	queued, err := s.RetranslateAllTitles()
	if err != nil {
		t.Fatalf("retranslate all: %v", err)
	}
	if queued != len(ids) {
		t.Fatalf("queued = %d, want %d", queued, len(ids))
	}
	for _, id := range ids {
		waitForDetail(t, s, id, func(d *Detail) bool { return d.TitleZh == "译名" })
	}
	batches := translator.translatedBatches()
	if len(batches) != 1 {
		t.Fatalf("translator calls = %d, want one packed request: %#v", len(batches), batches)
	}
	if len(batches[0]) != len(ids) {
		t.Fatalf("packed titles = %d, want %d", len(batches[0]), len(ids))
	}
}

// TestServiceKeepsFiveTranslationRequestsInFlight proves the pool overlaps
// provider wait time without exceeding the configured bound: five packed
// requests start, a sixth cannot, and every favorite still lands its title.
func TestServiceKeepsFiveTranslationRequestsInFlight(t *testing.T) {
	s, db, vr, first := newServiceFixture(t, fakeProvider{}, "", "IPX-633.mp4")
	repo := NewSQLiteRepository(db)
	seedScrapedTitle(t, repo, first, "IPX-633", "标题 0")
	ids := []int64{first}
	for i := 1; i < titleTranslationBatchSize*titleTranslationConcurrency; i++ {
		ids = append(ids, createScrapedFavorite(
			t, repo, vr,
			fmt.Sprintf("batch-%02d.mp4", i),
			fmt.Sprintf("T-%03d", i),
			fmt.Sprintf("标题 %d", i),
		))
	}

	probe := &blockingBatchTranslator{
		release: make(chan struct{}),
		entered: make(chan struct{}, titleTranslationConcurrency),
	}
	probe.configured.Store(true)
	s.SetTitleTranslator(probe)
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(probe.release) }) }
	t.Cleanup(release)

	queued, err := s.RetranslateAllTitles()
	if err != nil {
		t.Fatalf("retranslate all: %v", err)
	}
	if queued != len(ids) {
		t.Fatalf("queued = %d, want %d", queued, len(ids))
	}
	for i := 0; i < titleTranslationConcurrency; i++ {
		select {
		case <-probe.entered:
		case <-time.After(2 * time.Second):
			t.Fatalf("only %d translation requests started, want %d", i, titleTranslationConcurrency)
		}
	}
	if got := probe.maxActive.Load(); got != titleTranslationConcurrency {
		t.Fatalf("in-flight requests = %d, want %d", got, titleTranslationConcurrency)
	}
	// All five workers are blocked, so a sixth request cannot be in flight.
	select {
	case <-probe.entered:
		t.Fatal("a sixth translation request started")
	case <-time.After(100 * time.Millisecond):
	}

	release()
	for _, id := range ids {
		waitForDetail(t, s, id, func(d *Detail) bool { return d.TitleZh == "译名" })
	}
}

// TestServiceRetranslatesEveryTitle verifies the 重新翻译全部译名 batch: a row
// that already has a Chinese title is queued again, a second press while it is
// in flight is ignored, and the new result replaces the stored one.
func TestServiceRetranslatesEveryTitle(t *testing.T) {
	s, db, _, id := newServiceFixture(t, fakeProvider{}, "", "IPX-633.mp4")
	translator := &fakeTitleTranslator{
		started: make(chan struct{}),
		release: make(chan struct{}),
		result:  "新译名",
	}
	s.SetTitleTranslator(translator)

	// Scrape while translation is off, then seed a translation so the row looks
	// like one translated under the old prompt.
	s.Enqueue(id)
	waitForDetail(t, s, id, func(d *Detail) bool { return d.ScrapeStatus == StatusSucceeded })
	if err := NewSQLiteRepository(db).SetTitleTranslation(id, "旧译名", ""); err != nil {
		t.Fatalf("seed translation: %v", err)
	}

	translator.configured.Store(true)
	queued, err := s.RetranslateAllTitles()
	if err != nil {
		t.Fatalf("retranslate all: %v", err)
	}
	if queued != 1 {
		t.Fatalf("queued = %d, want 1 even though the row was translated", queued)
	}
	select {
	case <-translator.started:
	case <-time.After(2 * time.Second):
		t.Fatal("retranslation never started")
	}
	// Pressing again while the refresh is running must not enqueue it twice.
	if again, err := s.RetranslateAllTitles(); err != nil || again != 0 {
		t.Fatalf("second retranslate all = %d, %v; want 0, nil", again, err)
	}

	close(translator.release)
	d := waitForDetail(t, s, id, func(d *Detail) bool { return d.TitleZh == "新译名" })
	if d.TitleZh != "新译名" {
		t.Fatalf("titleZh = %q, want the refreshed translation", d.TitleZh)
	}
}

// TestServiceRetranslationFailureKeepsPreviousTitle backs the promise that a
// bad batch is recoverable: when refreshing an existing translation fails, the
// old readable title stays and the failure is recorded next to it.
func TestServiceRetranslationFailureKeepsPreviousTitle(t *testing.T) {
	s, db, _, id := newServiceFixture(t, fakeProvider{}, "", "IPX-633.mp4")
	translator := &fakeTitleTranslator{
		started: make(chan struct{}),
		release: make(chan struct{}),
		err:     errors.New("LLM 请求过于频繁（429）：请稍后再试"),
	}
	s.SetTitleTranslator(translator)

	s.Enqueue(id)
	waitForDetail(t, s, id, func(d *Detail) bool { return d.ScrapeStatus == StatusSucceeded })
	if err := NewSQLiteRepository(db).SetTitleTranslation(id, "旧译名", ""); err != nil {
		t.Fatalf("seed translation: %v", err)
	}

	translator.configured.Store(true)
	if _, err := s.RetranslateAllTitles(); err != nil {
		t.Fatalf("retranslate all: %v", err)
	}
	select {
	case <-translator.started:
	case <-time.After(2 * time.Second):
		t.Fatal("retranslation never started")
	}
	close(translator.release)

	d := waitForDetail(t, s, id, func(d *Detail) bool { return d.TitleZhStatus != TitleZhPending })
	if d.TitleZh != "旧译名" {
		t.Fatalf("titleZh = %q, want the previous translation kept after a failed refresh", d.TitleZh)
	}
	if d.TitleZhStatus != TitleZhFailed {
		t.Fatalf("status = %q, want failed while the kept title reports the refresh error", d.TitleZhStatus)
	}
	if !strings.Contains(d.TitleZhError, "429") {
		t.Fatalf("titleZhError = %q, want the provider failure", d.TitleZhError)
	}
}

// TestServiceResumesMissingTitlesOnStart verifies the opt-in startup backfill:
// a favorite scraped while translation was paused is queued again when Start
// runs, so a batch interrupted by a quit picks up without a manual click.
func TestServiceResumesMissingTitlesOnStart(t *testing.T) {
	s, _, _, id := newServiceFixture(t, fakeProvider{}, "", "IPX-633.mp4")
	translator := &fakeTitleTranslator{result: "邻家的人妻"}
	s.SetTitleTranslator(translator)

	// Scraped while translation was off, so the work has no translation yet.
	s.Enqueue(id)
	waitForDetail(t, s, id, func(d *Detail) bool { return d.ScrapeStatus == StatusSucceeded })

	translator.configured.Store(true)
	s.SetTitleResumeOnStart(func() (bool, error) { return true, nil })
	s.Start()
	d := waitForDetail(t, s, id, func(d *Detail) bool { return d.TitleZhStatus == TitleZhSucceeded })
	if d.TitleZh != "邻家的人妻" {
		t.Fatalf("titleZh = %q, want 邻家的人妻", d.TitleZh)
	}
}

// TestServiceStartSkipsResumeWhenSwitchOff keeps the default quiet: with the
// switch off, opening the app must not spend a provider request.
func TestServiceStartSkipsResumeWhenSwitchOff(t *testing.T) {
	s, _, _, id := newServiceFixture(t, fakeProvider{}, "", "IPX-633.mp4")
	translator := &fakeTitleTranslator{result: "不应出现"}
	s.SetTitleTranslator(translator)

	s.Enqueue(id)
	waitForDetail(t, s, id, func(d *Detail) bool { return d.ScrapeStatus == StatusSucceeded })

	// Configure the provider only after the scrape, so the scrape-time trigger
	// cannot be the one calling the translator.
	translator.configured.Store(true)
	s.SetTitleResumeOnStart(func() (bool, error) { return false, nil })
	s.Start()
	// Give a stray worker a moment to (incorrectly) run before asserting.
	time.Sleep(20 * time.Millisecond)
	if got := translator.translatedTitles(); len(got) != 0 {
		t.Fatalf("translator was called with the switch off: %#v", got)
	}
}

// TestServiceStartToleratesResumeCheckFailure keeps startup independent of the
// settings table: an unreadable preference is logged and treated as off.
func TestServiceStartToleratesResumeCheckFailure(t *testing.T) {
	s, _, _, id := newServiceFixture(t, fakeProvider{}, "", "IPX-633.mp4")
	translator := &fakeTitleTranslator{result: "不应出现"}
	s.SetTitleTranslator(translator)

	s.Enqueue(id)
	waitForDetail(t, s, id, func(d *Detail) bool { return d.ScrapeStatus == StatusSucceeded })

	translator.configured.Store(true)
	s.SetTitleResumeOnStart(func() (bool, error) { return false, errors.New("settings unavailable") })
	s.Start()
	time.Sleep(20 * time.Millisecond)
	if got := translator.translatedTitles(); len(got) != 0 {
		t.Fatalf("translator was called after a failed settings read: %#v", got)
	}
}

func TestServiceReportsPendingTranslationInDetail(t *testing.T) {
	s, _, _, id := newServiceFixture(t, fakeProvider{}, "", "IPX-633.mp4")
	translator := &fakeTitleTranslator{
		started: make(chan struct{}),
		release: make(chan struct{}),
		result:  "邻家的人妻",
	}
	s.SetTitleTranslator(translator)

	s.Enqueue(id)
	waitForDetail(t, s, id, func(d *Detail) bool { return d.ScrapeStatus == StatusSucceeded })

	translator.configured.Store(true)
	if err := s.TranslateTitle(id); err != nil {
		t.Fatalf("translate title: %v", err)
	}
	select {
	case <-translator.started:
	case <-time.After(2 * time.Second):
		t.Fatal("translation never started")
	}
	pending, err := s.Get(id)
	if err != nil {
		t.Fatalf("get pending detail: %v", err)
	}
	if pending.TitleZhStatus != TitleZhPending {
		t.Fatalf("status = %q, want pending while the worker is busy", pending.TitleZhStatus)
	}

	close(translator.release)
	d := waitForDetail(t, s, id, func(d *Detail) bool { return d.TitleZhStatus == TitleZhSucceeded })
	if d.TitleZh != "邻家的人妻" {
		t.Fatalf("titleZh = %q, want 邻家的人妻", d.TitleZh)
	}
}

func TestServiceRescrapeReplacesStaleTranslation(t *testing.T) {
	s, _, _, id := newServiceFixture(t, fakeProvider{}, "", "IPX-633.mp4")
	translator := &fakeTitleTranslator{result: "旧译名"}
	translator.configured.Store(true)
	s.SetTitleTranslator(translator)

	s.Enqueue(id)
	waitForDetail(t, s, id, func(d *Detail) bool { return d.TitleZh == "旧译名" })

	translator.mu.Lock()
	translator.result = "新译名"
	translator.mu.Unlock()
	s.EnqueueForce(id)
	d := waitForDetail(t, s, id, func(d *Detail) bool { return d.TitleZh == "新译名" })
	if d.TitleZh != "新译名" {
		t.Fatalf("titleZh = %q, want the translation of the new scrape", d.TitleZh)
	}
}

func TestServiceTranslateTitleRequiresConfiguredProvider(t *testing.T) {
	s, _, _, id := newServiceFixture(t, fakeProvider{}, "", "IPX-633.mp4")
	if err := s.TranslateTitle(id); err == nil || !strings.Contains(err.Error(), "尚未配置") {
		t.Fatalf("TranslateTitle unconfigured error = %v, want a configuration hint", err)
	}
	if _, err := s.TranslateMissingTitles(); err == nil || !strings.Contains(err.Error(), "尚未配置") {
		t.Fatalf("TranslateMissingTitles unconfigured error = %v, want a configuration hint", err)
	}
	if _, err := s.RetranslateAllTitles(); err == nil || !strings.Contains(err.Error(), "尚未配置") {
		t.Fatalf("RetranslateAllTitles unconfigured error = %v, want a configuration hint", err)
	}
}
