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

// checkFixture is one favorite in the code-check fixture: a file name, the
// scraped 番号 ("" for a favorite the scraper has not reached yet) and whether
// it should end up in the recycle bin.
type checkFixture struct {
	fileName string
	code     string
	trashed  bool
}

// newCheckFacade builds a SQLite-backed façade with real favorites and real
// scrape results, so "does the library already hold this work?" is answered by
// the same queries production uses.
func newCheckFacade(t *testing.T, fixtures []checkFixture) (*Library, map[string]int64) {
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

	ids := make(map[string]int64, len(fixtures))
	for i, fixture := range fixtures {
		v := &video.Video{
			Path:      filepath.Join(t.TempDir(), fixture.fileName),
			FileName:  fixture.fileName,
			FileSize:  int64(1_000_000 * (i + 1)),
			Width:     1920,
			Height:    1080,
			Status:    video.StatusAvailable,
			CreatedAt: int64(i + 1),
			UpdatedAt: int64(i + 1),
		}
		if err := vr.Create(v); err != nil {
			t.Fatal(err)
		}
		ids[fixture.fileName] = v.ID
		if fixture.code != "" {
			if err := javRepo.PrepareTask(v.ID, false); err != nil {
				t.Fatal(err)
			}
			if err := javRepo.SaveSuccess(v.ID, jav.Metadata{Code: fixture.code, Actors: []string{"Actor"}}, "filename", 1); err != nil {
				t.Fatal(err)
			}
		}
		if fixture.trashed {
			if err := vr.Trash(v.ID, video.TrashReasonUser, 1_000); err != nil {
				t.Fatal(err)
			}
		}
	}
	return lib, ids
}

// matchIDs flattens one result's matches into their video ids.
func matchIDs(check jav.JavCodeCheck) []int64 {
	ids := make([]int64, 0, len(check.Matches))
	for _, match := range check.Matches {
		ids = append(ids, match.VideoID)
	}
	return ids
}

// The extension asks about the cards it is rendering; the facade has to answer
// for a scraped favorite, for one the scraper has not reached yet (its file
// name is the only identity it has), for episodes, for binned copies and for
// works it does not hold at all.
func TestMatchJavCodes(t *testing.T) {
	lib, ids := newCheckFacade(t, []checkFixture{
		{fileName: "abp-576.mp4", code: "ABP-576"},
		{fileName: "ABP-576 1080p.mkv", code: "ABP-576"},
		{fileName: "ipx-633.mp4"},          // never scraped: matched by name
		{fileName: "ABC-123-1.mp4"},        // episode 1, never scraped
		{fileName: "ABC-123-2.mp4"},        // episode 2, never scraped
		{fileName: "FC2-PPV-4970405.mkv", code: "FC2-4970405"},
		{fileName: "ssis-001.mp4", code: "SSIS-001", trashed: true},
		{fileName: "unrelated.mp4", code: "MIDE-999"},
	})

	results, err := lib.MatchJavCodes([]string{
		"abp-576", " IPX-633 ", "ABC-123-1", "ABC-123-2", "FC2-PPV-4970405",
		"SSIS-001", "MIAA-000", "ABP-576",
	})
	if err != nil {
		t.Fatal(err)
	}

	// The request order is kept and the repeated code collapses into its first
	// slot, so a client can pair results with cards positionally.
	wantCodes := []string{
		"ABP-576", "IPX-633", "ABC-123-1", "ABC-123-2", "FC2-4970405",
		"SSIS-001", "MIAA-000",
	}
	if len(results) != len(wantCodes) {
		t.Fatalf("got %d results, want %d: %#v", len(results), len(wantCodes), results)
	}
	for i, want := range wantCodes {
		if results[i].Code != want {
			t.Errorf("result[%d].Code = %q, want %q", i, results[i].Code, want)
		}
	}

	// Both copies of the scraped work are reported, newest first.
	wantABP := []int64{ids["ABP-576 1080p.mkv"], ids["abp-576.mp4"]}
	if got := matchIDs(results[0]); len(got) != len(wantABP) || got[0] != wantABP[0] || got[1] != wantABP[1] {
		t.Errorf("ABP-576 matches = %v, want %v", got, wantABP)
	}
	if match := results[0].Matches[0]; match.Label != "abp-576.mp4" && match.JavCode != "ABP-576" {
		t.Errorf("match label %q / code %q", match.Label, match.JavCode)
	}

	// An unscraped favorite is still found, through the code its name spells.
	if got := matchIDs(results[1]); len(got) != 1 || got[0] != ids["ipx-633.mp4"] {
		t.Errorf("IPX-633 matches = %v, want the unscraped copy", got)
	}
	// Episodes share a base 番号 but are separate works, exactly as 查重 treats
	// them: only the episode JavDB names is reported, and the bare work is not.
	if got := matchIDs(results[2]); len(got) != 1 || got[0] != ids["ABC-123-1.mp4"] {
		t.Errorf("ABC-123-1 matches = %v, want only episode 1", got)
	}
	if got := matchIDs(results[3]); len(got) != 1 || got[0] != ids["ABC-123-2.mp4"] {
		t.Errorf("ABC-123-2 matches = %v, want only episode 2", got)
	}

	// JavDB writes FC2 as FC2-PPV-…, Muse stores FC2-….
	if got := matchIDs(results[4]); len(got) != 1 || got[0] != ids["FC2-PPV-4970405.mkv"] {
		t.Errorf("FC2 matches = %v, want the FC2 copy", got)
	}

	// A binned favorite is reported as binned, not hidden.
	if len(results[5].Matches) != 1 || !results[5].Matches[0].InTrash {
		t.Errorf("SSIS-001 matches = %#v, want one trashed match", results[5].Matches)
	}

	// A work the library does not hold yields an empty array, never null.
	if results[6].Matches == nil {
		t.Fatal("matches must be an empty array, not nil")
	}
	if len(results[6].Matches) != 0 {
		t.Errorf("MIAA-000 matches = %#v, want none", results[6].Matches)
	}
}

// A file name that merely mentions a code inside a longer token must not make
// an unrelated favorite look collected: only the name's first candidate counts,
// so "hhd800.com@ABW-180.mp4" belongs to HHD-800, not to ABW-180.
func TestMatchJavCodesIgnoresLaterNameCandidates(t *testing.T) {
	lib, ids := newCheckFacade(t, []checkFixture{
		{fileName: "hhd800.com@ABW-180.mp4"},
	})

	results, err := lib.MatchJavCodes([]string{"HHD-800"})
	if err != nil {
		t.Fatal(err)
	}
	if got := matchIDs(results[0]); len(got) != 1 || got[0] != ids["hhd800.com@ABW-180.mp4"] {
		t.Fatalf("HHD-800 matches = %v, want the file whose first candidate it is", got)
	}

	results, err = lib.MatchJavCodes([]string{"ABW-180"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results[0].Matches) != 0 {
		t.Fatalf("ABW-180 matches = %#v, want none (only the first candidate counts)", results[0].Matches)
	}
}

// When a page names both the bare work and one of its episodes, each query gets
// its own answer: the bare copy is not reported for the episode and vice versa.
func TestMatchJavCodesKeepsWorkAndEpisodeApart(t *testing.T) {
	lib, ids := newCheckFacade(t, []checkFixture{
		{fileName: "ABC-123.mp4"},
		{fileName: "ABC-123-1.mp4"},
	})

	results, err := lib.MatchJavCodes([]string{"ABC-123", "ABC-123-1"})
	if err != nil {
		t.Fatal(err)
	}
	if got := matchIDs(results[0]); len(got) != 1 || got[0] != ids["ABC-123.mp4"] {
		t.Errorf("ABC-123 matches = %v, want only the bare copy", got)
	}
	if got := matchIDs(results[1]); len(got) != 1 || got[0] != ids["ABC-123-1.mp4"] {
		t.Errorf("ABC-123-1 matches = %v, want only episode 1", got)
	}
}

// Without a jav service the file names are all the library has; the check must
// keep working instead of failing the whole request.
func TestMatchJavCodesWithoutJavService(t *testing.T) {
	lib, ids := newCheckFacade(t, []checkFixture{
		{fileName: "abp-576.mp4"},
	})
	lib.SetJavService(nil)

	results, err := lib.MatchJavCodes([]string{"ABP-576"})
	if err != nil {
		t.Fatal(err)
	}
	if got := matchIDs(results[0]); len(got) != 1 || got[0] != ids["abp-576.mp4"] {
		t.Fatalf("matches = %v, want the file-name match", got)
	}
}

// A batch larger than the documented limit is rejected rather than silently
// truncated, so a client that sends too much learns about it.
func TestMatchJavCodesRejectsOversizedBatch(t *testing.T) {
	lib, _ := newCheckFacade(t, nil)

	codes := make([]string, MaxJavCodeCheckCodes+1)
	for i := range codes {
		codes[i] = "ABP-" + string(rune('A'+i%26)) + string(rune('A'+i/26))
	}
	if _, err := lib.MatchJavCodes(codes); err == nil {
		t.Fatal("want an error for a batch over the limit")
	}
}

// An empty batch is a valid (if pointless) request: no results, no error.
func TestMatchJavCodesEmptyInput(t *testing.T) {
	lib, _ := newCheckFacade(t, []checkFixture{{fileName: "abp-576.mp4", code: "ABP-576"}})

	results, err := lib.MatchJavCodes(nil)
	if err != nil {
		t.Fatal(err)
	}
	if results == nil || len(results) != 0 {
		t.Fatalf("results = %#v, want an empty (non-nil) slice", results)
	}
}
