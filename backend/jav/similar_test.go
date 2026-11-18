package jav

import (
	"io"
	"log/slog"
	"path/filepath"
	"reflect"
	"testing"

	"muse-app/backend/database"
	"muse-app/backend/video"
)

// newSimilarService builds a jav service over a real database and hands back
// both repositories as their interfaces, so a test can seed favorites, scrape
// results and 演员合并 the same way the production wiring does.
func newSimilarService(t *testing.T) (*Service, Repository, video.VideoRepository) {
	t.Helper()
	db, _, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	javRepo := NewSQLiteRepository(db)
	videoRepo := video.NewSQLiteRepository(db)
	discard := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(javRepo, videoRepo, nil, "", nil, discard, 1)
	t.Cleanup(svc.Close)
	return svc, javRepo, videoRepo
}

func createSimilarVideo(t *testing.T, videos video.VideoRepository, repo Repository, name string) int64 {
	t.Helper()
	v := &video.Video{
		Path:      filepath.Join(t.TempDir(), name),
		FileName:  name,
		FileSize:  1,
		CreatedAt: 1,
		UpdatedAt: 1,
		Status:    video.StatusAvailable,
	}
	if err := videos.Create(v); err != nil {
		t.Fatalf("create favorite %q: %v", name, err)
	}
	if err := repo.PrepareTask(v.ID, false); err != nil {
		t.Fatalf("prepare task %q: %v", name, err)
	}
	return v.ID
}

// The features the scorer receives must be the scraped ones, with 演员合并
// already applied: the two stage names of one performer collapse into one, or
// the recommender would count the same person twice.
func TestSimilarFeaturesAppliesActorMerges(t *testing.T) {
	svc, repo, videos := newSimilarService(t)
	id := createSimilarVideo(t, videos, repo, "a.mp4")

	released := int64(1600000000)
	uncensored := true
	if err := repo.SaveSuccess(id, Metadata{
		Code:         "IPX-633",
		Title:        "となりの人妻",
		Series:       "群",
		Studio:       "L1",
		ReleaseUnix:  &released,
		IsUncensored: &uncensored,
		Actors:       []string{"B", "A"},
		Tags:         []string{"题材", "無碼破解"},
	}, "filename", 2); err != nil {
		t.Fatalf("save success: %v", err)
	}
	if err := repo.UpsertActorMerge("B", "A"); err != nil {
		t.Fatalf("merge actors: %v", err)
	}

	features, err := svc.SimilarFeatures()
	if err != nil {
		t.Fatalf("similar features: %v", err)
	}
	if len(features) != 1 {
		t.Fatalf("got %d features, want 1", len(features))
	}
	got := features[0]
	if got.VideoID != id || got.Code != "IPX-633" || got.Series != "群" || got.Studio != "L1" {
		t.Errorf("feature = %+v", got)
	}
	if got.ReleaseUnix == nil || *got.ReleaseUnix != released {
		t.Errorf("release = %v, want %d", got.ReleaseUnix, released)
	}
	if got.IsUncensored == nil || !*got.IsUncensored {
		t.Errorf("uncensored = %v, want true", got.IsUncensored)
	}
	if want := []string{"A"}; !reflect.DeepEqual(got.Actors, want) {
		t.Errorf("cast = %v, want %v (merged and deduplicated)", got.Actors, want)
	}
	// The format tag survives here: deciding which tags describe style rather
	// than format is the scorer's job, not the reader's.
	if want := []string{"题材", "無碼破解"}; !reflect.DeepEqual(got.Tags, want) {
		t.Errorf("tags = %v, want %v", got.Tags, want)
	}
}

// A favorite the scraper produced no title for does not take part at all: the
// original title is the title channel's only text, so there is nothing to
// compare it on.
func TestSimilarFeaturesSkipsFavoritesWithoutATitle(t *testing.T) {
	svc, repo, videos := newSimilarService(t)
	titled := createSimilarVideo(t, videos, repo, "titled.mp4")
	untitled := createSimilarVideo(t, videos, repo, "untitled.mp4")
	if err := repo.SaveSuccess(titled, Metadata{Code: "AAA-001", Title: "となりの人妻"}, "filename", 2); err != nil {
		t.Fatalf("save titled: %v", err)
	}
	if err := repo.SaveSuccess(untitled, Metadata{Code: "BBB-001"}, "filename", 2); err != nil {
		t.Fatalf("save untitled: %v", err)
	}

	features, err := svc.SimilarFeatures()
	if err != nil {
		t.Fatalf("similar features: %v", err)
	}
	if len(features) != 1 || features[0].VideoID != titled {
		t.Fatalf("features = %+v, want only the titled favorite", features)
	}
}

// A favorite in the recycle bin must not come back as a recommendation.
func TestSimilarFeaturesSkipsTrashedFavorites(t *testing.T) {
	svc, repo, videos := newSimilarService(t)
	kept := createSimilarVideo(t, videos, repo, "kept.mp4")
	trashed := createSimilarVideo(t, videos, repo, "trashed.mp4")
	for _, id := range []int64{kept, trashed} {
		if err := repo.SaveSuccess(id, Metadata{Code: "AAA-001", Title: "作品"}, "filename", 2); err != nil {
			t.Fatalf("save success: %v", err)
		}
	}
	if err := videos.Trash(trashed, video.TrashReasonUser, 1); err != nil {
		t.Fatalf("trash: %v", err)
	}

	features, err := svc.SimilarFeatures()
	if err != nil {
		t.Fatalf("similar features: %v", err)
	}
	if len(features) != 1 || features[0].VideoID != kept {
		t.Fatalf("features = %+v, want only %d", features, kept)
	}
}
