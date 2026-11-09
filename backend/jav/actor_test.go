package jav

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"

	"muse-app/backend/database"
	"muse-app/backend/video"
)

// newActorFixture creates a migrated database with the given videos, each
// carrying a scraped cast list. The videos' created_at values are ascending so
// the expected newest-first order is unambiguous.
func newActorFixture(t *testing.T, casts [][]string) (*Service, *SQLiteRepository, []int64) {
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

	vr := video.NewSQLiteRepository(db)
	repo := NewSQLiteRepository(db)
	ids := make([]int64, 0, len(casts))
	for i, cast := range casts {
		path := filepath.Join(t.TempDir(), "v"+string(rune('a'+i))+".mp4")
		v := &video.Video{
			Path:      path,
			FileName:  filepath.Base(path),
			FileSize:  1,
			Status:    video.StatusAvailable,
			CreatedAt: int64(i + 1),
			UpdatedAt: int64(i + 1),
		}
		if err := vr.Create(v); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, v.ID)
		if err := repo.PrepareTask(v.ID, false); err != nil {
			t.Fatal(err)
		}
		if err := repo.SaveSuccess(v.ID, Metadata{Code: "CODE-" + v.FileName, Actors: cast}, "filename", 1); err != nil {
			t.Fatal(err)
		}
	}
	svc := NewService(repo, vr, nil, "", nil, nil, 1)
	t.Cleanup(svc.Close)
	return svc, repo, ids
}

func TestListActorsAggregatesAndOrders(t *testing.T) {
	svc, _, _ := newActorFixture(t, [][]string{
		{"三上悠亚", "Bridge"},
		{"三上悠亚"},
		{"Bridge"},
		{"Bridge"},
		{},
	})

	got, err := svc.ListActors()
	if err != nil {
		t.Fatal(err)
	}
	want := []ActorSummary{
		{Name: "Bridge", Count: 3},
		{Name: "三上悠亚", Count: 2},
	}
	if len(got) != len(want) {
		t.Fatalf("actors=%+v, want %+v", got, want)
	}
	for i := range want {
		if got[i].Name != want[i].Name || got[i].Count != want[i].Count || len(got[i].Aliases) != 0 {
			t.Fatalf("actor[%d]=%+v, want %+v (all: %+v)", i, got[i], want[i], got)
		}
	}
}

func TestListActorsToleratesInvalidJSON(t *testing.T) {
	svc, repo, _ := newActorFixture(t, [][]string{{"A"}})
	if _, err := repo.db.Exec(`UPDATE video_jav SET actors_json='not-json'`); err != nil {
		t.Fatal(err)
	}
	got, err := svc.ListActors()
	if err != nil {
		t.Fatalf("invalid JSON must not fail the aggregate: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("actors=%+v, want none", got)
	}
}

// ActorsByVideo is the shared read model's per-favorite cast. Merged stage
// names collapse to their canonical name, repeats disappear, names are sorted,
// and a favorite without a scraped cast stays out of the map.
func TestActorsByVideoResolvesAliasesAndSorts(t *testing.T) {
	svc, _, ids := newActorFixture(t, [][]string{
		{"B", "C"},
		{"A"},
		{},
	})
	if err := svc.MergeActors("B", "A"); err != nil {
		t.Fatal(err)
	}

	got, err := svc.ActorsByVideo()
	if err != nil {
		t.Fatal(err)
	}
	want := map[int64][]string{
		ids[0]: {"A", "C"},
		ids[1]: {"A"},
	}
	if len(got) != len(want) {
		t.Fatalf("actors by video = %+v, want %+v", got, want)
	}
	for id, cast := range want {
		if !reflect.DeepEqual(got[id], cast) {
			t.Errorf("video %d cast = %v, want %v", id, got[id], cast)
		}
	}
	if _, ok := got[ids[2]]; ok {
		t.Errorf("video without a cast must be absent: %+v", got)
	}
}

func TestVideoIDsByActorReturnsNewestFirst(t *testing.T) {
	svc, _, ids := newActorFixture(t, [][]string{
		{"A"},
		{"A", "B"},
		{"B"},
		{"A"},
	})

	got, err := svc.VideoIDsByActor("A")
	if err != nil {
		t.Fatal(err)
	}
	// ids[3] and ids[1] and ids[0] have A; newest (largest created_at) first.
	want := []int64{ids[3], ids[1], ids[0]}
	if len(got) != len(want) {
		t.Fatalf("ids=%v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ids[%d]=%d, want %d (all: %v)", i, got[i], want[i], got)
		}
	}
}

func TestVideoIDsByActorUnknownReturnsEmpty(t *testing.T) {
	svc, _, _ := newActorFixture(t, [][]string{{"A"}})
	got, err := svc.VideoIDsByActor("nobody")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("ids=%v, want none", got)
	}
}

// Merging folds the source's films into the target and records the source as an
// alias, so the index shows one actor with the combined count.
func TestMergeActorsFoldsCountAndListsAlias(t *testing.T) {
	svc, _, _ := newActorFixture(t, [][]string{
		{"A"},
		{"A"},
		{"B"},
	})

	if err := svc.MergeActors("B", "A"); err != nil {
		t.Fatal(err)
	}
	got, err := svc.ListActors()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("actors=%+v, want a single merged entry", got)
	}
	if got[0].Name != "A" || got[0].Count != 3 {
		t.Fatalf("merged actor=%+v, want {A 3}", got[0])
	}
	if len(got[0].Aliases) != 1 || got[0].Aliases[0] != "B" {
		t.Fatalf("aliases=%v, want [B]", got[0].Aliases)
	}
}

// The merged film list spans every alias, not just the canonical name.
func TestVideosByActorIncludesAliases(t *testing.T) {
	svc, _, ids := newActorFixture(t, [][]string{
		{"A"},
		{"B"},
		{"C"},
	})
	if err := svc.MergeActors("B", "A"); err != nil {
		t.Fatal(err)
	}
	if err := svc.MergeActors("C", "A"); err != nil {
		t.Fatal(err)
	}
	got, err := svc.VideoIDsByActor("A")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("ids=%v, want all three films", got)
	}
	// Looking up a merged alias resolves to the same canonical group.
	viaB, err := svc.VideoIDsByActor("B")
	if err != nil {
		t.Fatal(err)
	}
	if len(viaB) != 3 {
		t.Fatalf("looking up alias B gave %v, want all three films", viaB)
	}
	_ = ids
}

// Merging a group into a new target moves the whole group: an alias that already
// pointed at the source follows it onto the target.
func TestMergeActorsMovesWholeGroup(t *testing.T) {
	svc, _, _ := newActorFixture(t, [][]string{{"A"}, {"B"}, {"C"}})
	if err := svc.MergeActors("B", "A"); err != nil { // B -> A
		t.Fatal(err)
	}
	if err := svc.MergeActors("A", "C"); err != nil { // A (group {A,B}) -> C
		t.Fatal(err)
	}
	got, err := svc.ListActors()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("actors=%+v, want one merged entry", got)
	}
	if got[0].Name != "C" || got[0].Count != 3 {
		t.Fatalf("merged actor=%+v, want {C 3}", got[0])
	}
	if len(got[0].Aliases) != 2 {
		t.Fatalf("aliases=%v, want two names", got[0].Aliases)
	}
}

// Merging a name into itself, or into a group it already belongs to, is a no-op.
func TestMergeActorsIdempotent(t *testing.T) {
	svc, _, _ := newActorFixture(t, [][]string{{"A"}, {"B"}})
	if err := svc.MergeActors("A", "A"); err != nil {
		t.Fatal(err)
	}
	if err := svc.MergeActors("B", "A"); err != nil {
		t.Fatal(err)
	}
	if err := svc.MergeActors("B", "A"); err != nil {
		t.Fatal(err)
	}
	got, err := svc.ListActors()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Count != 2 {
		t.Fatalf("actors=%+v, want one entry with count 2", got)
	}
}

// Merging an unknown name still records the mapping — useful when a stage name
// appears only in a merge, never directly in a scraped cast list.
func TestMergeActorsAcceptsUnscrapedName(t *testing.T) {
	svc, _, _ := newActorFixture(t, [][]string{{"A"}})
	if err := svc.MergeActors("stage-name", "A"); err != nil {
		t.Fatal(err)
	}
	got, err := svc.ListActors()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "A" {
		t.Fatalf("actors=%+v, want a single A entry", got)
	}
	if len(got[0].Aliases) != 1 || got[0].Aliases[0] != "stage-name" {
		t.Fatalf("aliases=%v, want [stage-name]", got[0].Aliases)
	}
}

// Empty names are rejected rather than silently creating a mapping.
func TestMergeActorsRejectsEmptyNames(t *testing.T) {
	svc, _, _ := newActorFixture(t, [][]string{{"A"}})
	if err := svc.MergeActors("", "A"); err == nil {
		t.Fatal("empty source must be rejected")
	}
	if err := svc.MergeActors("A", "  "); err == nil {
		t.Fatal("blank target must be rejected")
	}
	if err := svc.UnmergeActor(""); err == nil {
		t.Fatal("empty alias must be rejected")
	}
}

// A film that lists both stage names must count once after they are merged,
// and the index count must equal the size of that actor's film list.
func TestMergeActorsDeduplicatesSharedFilms(t *testing.T) {
	svc, _, _ := newActorFixture(t, [][]string{
		{"A", "B"}, // same performer listed under both names on one film
		{"A"},
		{"B"},
	})
	if err := svc.MergeActors("B", "A"); err != nil {
		t.Fatal(err)
	}
	got, err := svc.ListActors()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("actors=%+v, want a single merged entry", got)
	}
	if got[0].Count != 3 {
		t.Fatalf("merged count=%d, want 3 distinct films (not double-counted)", got[0].Count)
	}
	films, err := svc.VideoIDsByActor("A")
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Count != len(films) {
		t.Fatalf("index count=%d but film list has %d entries; they must agree",
			got[0].Count, len(films))
	}
}

func TestUnmergeActorRestoresIndependentActor(t *testing.T) {
	svc, _, _ := newActorFixture(t, [][]string{{"A"}, {"A"}, {"B"}})
	if err := svc.MergeActors("B", "A"); err != nil {
		t.Fatal(err)
	}
	if err := svc.UnmergeActor("B"); err != nil {
		t.Fatal(err)
	}
	got, err := svc.ListActors()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("actors=%+v, want A and B separately", got)
	}
	counts := map[string]int{}
	for _, a := range got {
		counts[a.Name] = a.Count
		if len(a.Aliases) != 0 {
			t.Fatalf("actor %q still has aliases %v", a.Name, a.Aliases)
		}
	}
	if counts["A"] != 2 || counts["B"] != 1 {
		t.Fatalf("counts=%v, want A=2 B=1", counts)
	}
}
