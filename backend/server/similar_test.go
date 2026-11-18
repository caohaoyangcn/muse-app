package server

import (
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"muse-app/backend/api"
	"muse-app/backend/database"
	"muse-app/backend/group"
	"muse-app/backend/jav"
	"muse-app/backend/video"
)

// newSimilarRoutes builds the LAN routes over a real library so the 「类似影片」
// endpoint is exercised against the queries production runs, without binding a
// listener.
func newSimilarRoutes(t *testing.T) (http.Handler, func(name string, meta jav.Metadata) int64) {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := sql.Open(database.DriverName, "file:"+filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}

	vr := video.NewSQLiteRepository(db)
	lib := api.New(video.NewService(vr, nil, t.TempDir(), log, nil), group.NewService(&stubGroupRepo{}, log, nil), log)
	javRepo := jav.NewSQLiteRepository(db)
	javSvc := jav.NewService(javRepo, vr, nil, t.TempDir(), nil, log, 1)
	t.Cleanup(javSvc.Close)
	lib.SetJavService(javSvc)

	add := func(name string, meta jav.Metadata) int64 {
		t.Helper()
		if strings.TrimSpace(meta.Title) == "" {
			meta.Title = name
		}
		v := &video.Video{
			Path:     filepath.Join(t.TempDir(), name),
			FileName: name,
			Status:   video.StatusAvailable,
		}
		if err := vr.Create(v); err != nil {
			t.Fatal(err)
		}
		if err := javRepo.PrepareTask(v.ID, false); err != nil {
			t.Fatal(err)
		}
		if err := javRepo.SaveSuccess(v.ID, meta, "filename", 1); err != nil {
			t.Fatal(err)
		}
		return v.ID
	}
	return New(lib, t.TempDir(), log).routes(), add
}

// similarPayload mirrors what a browser receives: the favorite, its score and
// the reasons, under the lower-case JSON names the shared contract declares.
type similarPayload struct {
	Video   video.Video `json:"video"`
	Score   float64     `json:"score"`
	Reasons []string    `json:"reasons"`
}

func getSimilar(t *testing.T, h http.Handler, path string) (*httptest.ResponseRecorder, []similarPayload) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200 (body %s)", path, rec.Code, rec.Body.String())
	}
	var got []similarPayload
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("GET %s body %q: %v", path, rec.Body.String(), err)
	}
	return rec, got
}

// The row reaches the phone with the same shape the desktop binding returns,
// including the read model (番号) and the reasons behind each pick.
func TestSimilarRouteReturnsTheRow(t *testing.T) {
	h, add := newSimilarRoutes(t)
	query := add("a.mp4", jav.Metadata{Code: "AAA-001", Actors: []string{"Actor A"}})
	neighbour := add("b.mp4", jav.Metadata{Code: "BBB-001", Actors: []string{"Actor A"}})

	_, got := getSimilar(t, h, "/api/videos/"+strconv.FormatInt(query, 10)+"/similar?limit=4")
	if len(got) != 1 {
		t.Fatalf("row = %+v, want one recommendation", got)
	}
	if got[0].Video.ID != neighbour {
		t.Fatalf("recommended %d, want %d", got[0].Video.ID, neighbour)
	}
	if got[0].Video.JavCode != "BBB-001" {
		t.Errorf("recommendation lost the read model: %+v", got[0].Video)
	}
	if len(got[0].Reasons) == 0 || got[0].Reasons[0] != "同演员" {
		t.Errorf("reasons = %v, want 同演员 first", got[0].Reasons)
	}
}

// An unknown favorite answers 200 with an empty array: the phone renders the
// same "nothing to show" state instead of an error page.
func TestSimilarRouteAnswersEmptyArrayForAnUnknownFavorite(t *testing.T) {
	h, _ := newSimilarRoutes(t)

	rec, got := getSimilar(t, h, "/api/videos/999/similar")
	if len(got) != 0 {
		t.Fatalf("row = %+v, want empty", got)
	}
	if body := rec.Body.String(); body != "[]\n" && body != "[]" {
		t.Errorf("body = %q, want an empty JSON array (not null)", body)
	}
}

// A malformed limit is not worth failing a read over: it falls back to the
// façade default.
func TestSimilarRouteIgnoresAMalformedLimit(t *testing.T) {
	h, add := newSimilarRoutes(t)
	query := add("a.mp4", jav.Metadata{Code: "AAA-001", Actors: []string{"Actor A"}})
	add("b.mp4", jav.Metadata{Code: "BBB-001", Actors: []string{"Actor A"}})

	if _, got := getSimilar(t, h, "/api/videos/"+strconv.FormatInt(query, 10)+"/similar?limit=lots"); len(got) != 1 {
		t.Fatalf("row = %+v, want the default row", got)
	}
}
