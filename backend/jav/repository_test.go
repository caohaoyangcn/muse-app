package jav

import (
	"path/filepath"
	"reflect"
	"testing"

	"muse-app/backend/database"
	"muse-app/backend/video"
)

// ListCodes feeds the shared video read model, so it must return exactly the
// saved scrape results: a code is present only after a successful scrape and
// never for a video the user chose to skip.
func TestListCodesReturnsAuthoritativeResults(t *testing.T) {
	db, _, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo := NewSQLiteRepository(db)
	videos := video.NewSQLiteRepository(db)
	create := func(name string) int64 {
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

	scraped := create("a.mp4")
	if err := repo.SaveSuccess(scraped, Metadata{Code: "  IPX-633  "}, "filename", 2); err != nil {
		t.Fatalf("save success: %v", err)
	}
	unscraped := create("b.mp4")
	skipped := create("c.mp4")
	if err := repo.SaveSuccess(skipped, Metadata{Code: "ABW-180"}, "filename", 2); err != nil {
		t.Fatalf("save skipped code: %v", err)
	}
	if err := repo.SetOverride(skipped, OverrideSkip, ""); err != nil {
		t.Fatalf("skip video: %v", err)
	}

	codes, err := repo.ListCodes()
	if err != nil {
		t.Fatalf("list codes: %v", err)
	}
	if got := codes[scraped]; got != "IPX-633" {
		t.Errorf("scraped code = %q, want trimmed IPX-633", got)
	}
	if _, ok := codes[unscraped]; ok {
		t.Error("a video with no scrape result must not carry a code")
	}
	if _, ok := codes[skipped]; ok {
		t.Error("a skipped video must not carry a code")
	}
}

// TestTrashedVideosAreHiddenFromJavReads pins the recycle-bin boundary for the
// scrape domain: a trashed favorite keeps its metadata for a restore, but must
// not appear in the task board, duplicate worklist, actor index or retry scan.
func TestTrashedVideosAreHiddenFromJavReads(t *testing.T) {
	db, _, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := NewSQLiteRepository(db)
	videos := video.NewSQLiteRepository(db)
	create := func(name string) *video.Video {
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
		if err := repo.SaveSuccess(v.ID, Metadata{Code: "ABC-123", Actors: []string{"演员甲"}}, "filename", 2); err != nil {
			t.Fatalf("save success %q: %v", name, err)
		}
		return v
	}
	active := create("active.mp4")
	trashed := create("trashed.mp4")
	if err := videos.Trash(trashed.ID, video.TrashReasonUser, 1000); err != nil {
		t.Fatalf("trash: %v", err)
	}

	codes, err := repo.ListCodes()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := codes[trashed.ID]; ok {
		t.Error("trashed favorite still contributes a code")
	}
	if _, ok := codes[active.ID]; !ok {
		t.Error("active favorite lost its code")
	}

	overviews, err := repo.ListTaskOverviews()
	if err != nil {
		t.Fatal(err)
	}
	if len(overviews) != 1 || overviews[0].Video.ID != active.ID {
		t.Errorf("task overviews = %+v, want only the active video", overviews)
	}

	duplicates, err := repo.ListDuplicateCandidates()
	if err != nil {
		t.Fatal(err)
	}
	if len(duplicates) != 1 || duplicates[0].Video.ID != active.ID {
		t.Errorf("duplicate candidates = %+v, want only the active video", duplicates)
	}

	refs, err := repo.ListActorFilmRefs()
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].VideoID != active.ID {
		t.Errorf("actor refs = %+v, want only the active video", refs)
	}

	ids, err := repo.VideoIDsByActorNames([]string{"演员甲"})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != active.ID {
		t.Errorf("actor videos = %v, want [%d]", ids, active.ID)
	}

	pending, err := repo.RecoverPending(1000)
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range pending {
		if task.VideoID == trashed.ID {
			t.Errorf("trashed favorite is still in the retry scan: %+v", task)
		}
	}
}

// TestTitleTranslationStorage verifies the two 中文译名 columns and the bulk
// work list: only successfully scraped, non-trashed, untranslated works are
// candidates, and a re-scrape clears a translation that no longer matches.
func TestTitleTranslationStorage(t *testing.T) {
	db, _, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo := NewSQLiteRepository(db)
	videos := video.NewSQLiteRepository(db)
	create := func(name, title, status string) int64 {
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
		if err := repo.SaveSuccess(v.ID, Metadata{Code: "IPX-633", Title: title}, "filename", 2); err != nil {
			t.Fatalf("save success %q: %v", name, err)
		}
		if err := repo.SetTaskStatus(v.ID, "", status, "", "", nil); err != nil {
			t.Fatalf("set task status %q: %v", name, err)
		}
		return v.ID
	}

	untranslated := create("a.mp4", "元タイトル A", StatusSucceeded)
	translated := create("b.mp4", "元タイトル B", StatusSucceeded)
	if err := repo.SetTitleTranslation(translated, "乙的中文名", ""); err != nil {
		t.Fatalf("save translation: %v", err)
	}
	_ = create("c.mp4", "元タイトル C", StatusFailed)
	trashed := create("d.mp4", "元タイトル D", StatusSucceeded)
	if _, err := db.Exec(`UPDATE videos SET deleted_at = 1 WHERE id = ?`, trashed); err != nil {
		t.Fatalf("trash favorite: %v", err)
	}
	emptyTitle := create("e.mp4", "   ", StatusSucceeded)

	candidates, err := repo.ListTitleTranslationCandidates()
	if err != nil {
		t.Fatalf("list candidates: %v", err)
	}
	if !reflect.DeepEqual(candidates, []int64{untranslated}) {
		t.Fatalf("candidates = %#v, want only the untranslated succeeded favorite", candidates)
	}
	_ = emptyTitle

	// Success and failure columns are mutually exclusive, so a later success
	// clears an earlier failure message and vice versa.
	if err := repo.SetTitleTranslation(untranslated, "", "LLM 鉴权失败"); err != nil {
		t.Fatalf("save failure: %v", err)
	}
	d, err := repo.Get(untranslated)
	if err != nil {
		t.Fatalf("get failed detail: %v", err)
	}
	if d.TitleZh != "" || d.TitleZhError != "LLM 鉴权失败" || d.TitleZhStatus != TitleZhFailed {
		t.Fatalf("failed detail = (%q, %q, %q)", d.TitleZh, d.TitleZhError, d.TitleZhStatus)
	}
	if err := repo.SetTitleTranslation(untranslated, "甲的中文名", ""); err != nil {
		t.Fatalf("save late success: %v", err)
	}
	d, err = repo.Get(untranslated)
	if err != nil {
		t.Fatalf("get translated detail: %v", err)
	}
	if d.TitleZh != "甲的中文名" || d.TitleZhError != "" || d.TitleZhStatus != TitleZhSucceeded {
		t.Fatalf("translated detail = (%q, %q, %q)", d.TitleZh, d.TitleZhError, d.TitleZhStatus)
	}

	// A forced re-scrape replaces the title, so the old translation is cleared.
	if err := repo.SaveSuccess(untranslated, Metadata{Code: "IPX-633", Title: "新しいタイトル"}, "filename", 3); err != nil {
		t.Fatalf("re-save success: %v", err)
	}
	d, err = repo.Get(untranslated)
	if err != nil {
		t.Fatalf("get re-scraped detail: %v", err)
	}
	if d.TitleZh != "" || d.TitleZhError != "" || d.TitleZhStatus != TitleZhNone {
		t.Fatalf("re-scraped translation = (%q, %q, %q), want none", d.TitleZh, d.TitleZhError, d.TitleZhStatus)
	}
}

// TestListTitlesAndSearchTitlesParams cover the read-model projections behind
// the 【番号】译名 card title: the original/translated pair, and the keyword
// search that lets a user find a card by either half even though the file name
// says nothing about it.
func TestListTitlesAndSearchTitlesParams(t *testing.T) {
	db, _, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo := NewSQLiteRepository(db)
	videos := video.NewSQLiteRepository(db)
	create := func(name string) int64 {
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

	translated := create("a.mp4")
	if err := repo.SaveSuccess(translated, Metadata{Code: "IPX-633", Title: "となりの人妻"}, "filename", 2); err != nil {
		t.Fatalf("save translated work: %v", err)
	}
	if err := repo.SetTitleTranslation(translated, "邻家的人妻", ""); err != nil {
		t.Fatalf("save translation: %v", err)
	}

	untranslated := create("b.mp4")
	if err := repo.SaveSuccess(untranslated, Metadata{Code: "ABW-180", Title: "あの娘"}, "filename", 2); err != nil {
		t.Fatalf("save untranslated work: %v", err)
	}
	// A failed task still has a scraped title; the read model keeps it so the
	// original title stands in until a later translation succeeds.
	if err := repo.SetTaskStatus(untranslated, "", StatusFailed, "", "", nil); err != nil {
		t.Fatalf("fail task: %v", err)
	}

	unscraped := create("c.mp4")
	skipped := create("d.mp4")
	if err := repo.SaveSuccess(skipped, Metadata{Code: "SSIS-001", Title: "スキップ"}, "filename", 2); err != nil {
		t.Fatalf("save skipped work: %v", err)
	}
	if err := repo.SetOverride(skipped, OverrideSkip, ""); err != nil {
		t.Fatalf("skip work: %v", err)
	}
	trashed := create("e.mp4")
	if err := repo.SaveSuccess(trashed, Metadata{Code: "TRASH-1", Title: "ゴミ"}, "filename", 2); err != nil {
		t.Fatalf("save trashed work: %v", err)
	}
	if err := videos.Trash(trashed, video.TrashReasonUser, 1000); err != nil {
		t.Fatalf("trash favorite: %v", err)
	}

	titles, err := repo.ListTitles()
	if err != nil {
		t.Fatalf("list titles: %v", err)
	}
	if got := titles[translated]; got.Title != "となりの人妻" || got.TitleZh != "邻家的人妻" {
		t.Errorf("translated pair = %+v", got)
	}
	if got := titles[untranslated]; got.Title != "あの娘" || got.TitleZh != "" {
		t.Errorf("untranslated pair = %+v", got)
	}
	if _, ok := titles[unscraped]; ok {
		t.Error("an unscraped favorite must not carry a scraped title")
	}
	if _, ok := titles[skipped]; ok {
		t.Error("a skipped favorite must not carry a scraped title")
	}
	if _, ok := titles[trashed]; ok {
		t.Error("a trashed favorite must not carry a scraped title")
	}

	for _, tc := range []struct {
		name    string
		keyword string
		want    []int64
	}{
		{name: "translation", keyword: "邻家", want: []int64{translated}},
		{name: "original title", keyword: "あの娘", want: []int64{untranslated}},
		{name: "code is case-insensitive", keyword: "ipx-633", want: []int64{translated}},
		{name: "code", keyword: "ABW", want: []int64{untranslated}},
		{name: "skipped and trashed are excluded", keyword: "SSIS", want: []int64{}},
		{name: "wildcards are literal", keyword: "%", want: []int64{}},
		{name: "blank", keyword: "   ", want: []int64{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := repo.SearchTitleIDs(tc.keyword)
			if err != nil {
				t.Fatalf("search: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("SearchTitleIDs(%q) = %#v, want %#v", tc.keyword, got, tc.want)
			}
		})
	}
}
