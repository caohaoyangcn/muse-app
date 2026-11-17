package api

import (
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"muse-app/backend/database"
	"muse-app/backend/group"
	"muse-app/backend/jav"
	"muse-app/backend/video"
)

// newSimilarFacade builds a façade over a real SQLite database, so the scrape
// rows, the 演员合并 aliases and the trash rules are exercised end to end, and
// returns the two ways a test seeds a favorite: with a scrape result and without
// one.
func newSimilarFacade(t *testing.T) (*Library, func(name string, meta jav.Metadata) int64, func(name string) int64) {
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

	discard := slog.New(slog.NewTextHandler(io.Discard, nil))
	vr := video.NewSQLiteRepository(db)
	lib := New(
		video.NewService(vr, nil, t.TempDir(), discard, nil),
		group.NewService(&memGroupRepo{members: map[int64][]int64{}}, discard, nil),
		discard,
	)
	javRepo := jav.NewSQLiteRepository(db)
	svc := jav.NewService(javRepo, vr, nil, "", nil, discard, 1)
	t.Cleanup(svc.Close)
	lib.SetJavService(svc)

	createVideo := func(t *testing.T, name string) *video.Video {
		t.Helper()
		v := &video.Video{
			Path:     filepath.Join(t.TempDir(), name),
			FileName: name,
			Status:   video.StatusAvailable,
		}
		if err := vr.Create(v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	addScraped := func(name string, meta jav.Metadata) int64 {
		t.Helper()
		v := createVideo(t, name)
		if err := javRepo.PrepareTask(v.ID, false); err != nil {
			t.Fatal(err)
		}
		if err := javRepo.SaveSuccess(v.ID, meta, "filename", 1); err != nil {
			t.Fatal(err)
		}
		return v.ID
	}
	addBare := func(name string) int64 {
		t.Helper()
		return createVideo(t, name).ID
	}
	return lib, addScraped, addBare
}

func hasReason(result SimilarVideo, reason string) bool {
	for _, got := range result.Reasons {
		if got == reason {
			return true
		}
	}
	return false
}

func resultIDs(results []SimilarVideo) []int64 {
	out := make([]int64, len(results))
	for i, r := range results {
		out[i] = r.Video.ID
	}
	return out
}

// The row is ordered by evidence strength, the favorite itself is never in it,
// and every card arrives with the shared read model already applied.
func TestSimilarVideosRanksByCastThenSeriesThenLabel(t *testing.T) {
	lib, addScraped, _ := newSimilarFacade(t)

	query := addScraped("a.mp4", jav.Metadata{
		Code: "AAA-001", Title: "查询", Series: "群", Studio: "L1",
		Actors: []string{"甲"}, Tags: []string{"t1"},
	})
	cast := addScraped("b.mp4", jav.Metadata{
		Code: "BBB-001", Title: "同演员", Actors: []string{"甲"}, Tags: []string{"t2"},
	})
	series := addScraped("c.mp4", jav.Metadata{
		Code: "CCC-001", Title: "同系列", Series: "群", Actors: []string{"乙"}, Tags: []string{"t3"},
	})
	label := addScraped("d.mp4", jav.Metadata{
		Code: "DDD-001", Title: "同厂牌", Studio: "L1", Actors: []string{"丙"}, Tags: []string{"t4"},
	})

	got, err := lib.SimilarVideos(query, 8)
	if err != nil {
		t.Fatal(err)
	}
	want := []int64{cast, series, label}
	if ids := resultIDs(got); len(ids) != len(want) || ids[0] != want[0] || ids[1] != want[1] || ids[2] != want[2] {
		t.Fatalf("row = %v, want %v", ids, want)
	}
	if !hasReason(got[0], "同演员") {
		t.Errorf("reasons = %v, want 同演员", got[0].Reasons)
	}
	if !hasReason(got[1], "同系列") {
		t.Errorf("reasons = %v, want 同系列", got[1].Reasons)
	}
	// The card carries the same 【番号】译名 projection as every other list.
	if got[0].Video.JavCode != "BBB-001" || got[0].Video.JavTitle != "同演员" {
		t.Errorf("card projection = %+v", got[0].Video)
	}
}

// 另一份同番号的文件不是"相似"，离开收藏的影片也不该出现。
func TestSimilarVideosSkipsTheSameWorkAndTrashedFavorites(t *testing.T) {
	lib, addScraped, _ := newSimilarFacade(t)

	query := addScraped("a.mp4", jav.Metadata{Code: "AAA-001", Actors: []string{"甲"}})
	addScraped("dup.mp4", jav.Metadata{Code: "AAA-001", Actors: []string{"甲"}})
	neighbour := addScraped("n.mp4", jav.Metadata{Code: "CCC-001", Actors: []string{"甲"}})
	trashed := addScraped("t.mp4", jav.Metadata{Code: "DDD-001", Actors: []string{"甲"}})
	if err := lib.RemoveVideo(trashed); err != nil {
		t.Fatal(err)
	}

	got, err := lib.SimilarVideos(query, 8)
	if err != nil {
		t.Fatal(err)
	}
	if ids := resultIDs(got); len(ids) != 1 || ids[0] != neighbour {
		t.Fatalf("row = %v, want only %d", ids, neighbour)
	}
}

// 演员合并 must reach the recommender: two stage names for one performer are
// one signal, not two.
func TestSimilarVideosAppliesActorMerges(t *testing.T) {
	lib, addScraped, _ := newSimilarFacade(t)

	query := addScraped("a.mp4", jav.Metadata{Code: "AAA-001", Actors: []string{"B"}})
	alias := addScraped("b.mp4", jav.Metadata{Code: "BBB-001", Actors: []string{"A"}})

	before, err := lib.SimilarVideos(query, 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 0 {
		t.Fatalf("row before the merge = %v, want none", resultIDs(before))
	}

	if err := lib.MergeActors("B", "A"); err != nil {
		t.Fatal(err)
	}
	after, err := lib.SimilarVideos(query, 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 || after[0].Video.ID != alias {
		t.Fatalf("row after the merge = %v, want %d", resultIDs(after), alias)
	}
	if !hasReason(after[0], "同演员") {
		t.Errorf("reasons = %v, want 同演员", after[0].Reasons)
	}
}

// A favorite without a scrape result has nothing to compare on; the row is
// empty rather than an error, and so is a façade with no jav service at all.
func TestSimilarVideosIsEmptyWithoutScrapeResults(t *testing.T) {
	lib, _, addBare := newSimilarFacade(t)

	bare := addBare("bare.mp4")
	got, err := lib.SimilarVideos(bare, 8)
	if err != nil {
		t.Fatalf("unscraped favorite: %v", err)
	}
	if got == nil {
		t.Fatal("want an empty slice, not nil (JSON would be null)")
	}
	if len(got) != 0 {
		t.Fatalf("row = %v, want empty", resultIDs(got))
	}

	lib.SetJavService(nil)
	got, err = lib.SimilarVideos(bare, 8)
	if err != nil || len(got) != 0 {
		t.Fatalf("without jav service = %v, %v, want empty", resultIDs(got), err)
	}
}

// The limit is clamped in the façade, so a caller asking for everything gets a
// bounded row and an unknown favorite is simply not found.
func TestSimilarVideosClampsTheLimit(t *testing.T) {
	lib, addScraped, _ := newSimilarFacade(t)

	query := addScraped("a.mp4", jav.Metadata{Code: "AAA-001", Actors: []string{"甲"}})
	for _, name := range []string{"b.mp4", "c.mp4", "d.mp4"} {
		addScraped(name, jav.Metadata{Code: "BBB-" + name, Actors: []string{"甲"}})
	}

	for _, limit := range []int{0, -3, 1000} {
		got, err := lib.SimilarVideos(query, limit)
		if err != nil {
			t.Fatalf("limit %d: %v", limit, err)
		}
		if len(got) != 3 {
			t.Fatalf("limit %d row = %v, want all three neighbours", limit, resultIDs(got))
		}
	}

	got, err := lib.SimilarVideos(999999, 8)
	if err != nil {
		t.Fatalf("unknown favorite: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("unknown favorite row = %v, want empty", resultIDs(got))
	}
}
