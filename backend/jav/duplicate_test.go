package jav

import (
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"muse-app/backend/database"
	"muse-app/backend/video"
)

// newDuplicateFixture seeds a library of (file name, 番号) pairs so the duplicate
// grouping can be exercised against real SQLite rows. An empty 番号 stands for a
// favorite that was never recognized.
func newDuplicateFixture(t *testing.T, entries [][2]string) (*Service, video.VideoRepository) {
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
	for i, entry := range entries {
		v := &video.Video{
			Path:      filepath.Join(t.TempDir(), entry[0]),
			FileName:  entry[0],
			FileSize:  100 + int64(i),
			Status:    video.StatusAvailable,
			CreatedAt: int64(i + 1),
			UpdatedAt: int64(i + 1),
		}
		if err := vr.Create(v); err != nil {
			t.Fatal(err)
		}
		if entry[1] == "" {
			continue
		}
		if err := repo.PrepareTask(v.ID, false); err != nil {
			t.Fatal(err)
		}
		if err := repo.SaveSuccess(v.ID, Metadata{Code: entry[1]}, "filename", 1); err != nil {
			t.Fatal(err)
		}
	}
	s := NewService(repo, vr, nil, "", nil, slog.New(slog.NewTextHandler(io.Discard, nil)), 1)
	t.Cleanup(s.Close)
	return s, vr
}

// groupCodes renders the visible worklist as 番号[/part] → member file names, so
// a test asserts the whole shape at once.
func groupCodes(t *testing.T, groups []DuplicateGroup) map[string][]string {
	t.Helper()
	out := make(map[string][]string, len(groups))
	for _, group := range groups {
		key := group.Code
		if group.Part != "" {
			key += "/" + group.Part
		}
		names := make([]string, 0, len(group.Videos))
		for _, v := range group.Videos {
			names = append(names, v.FileName)
		}
		out[key] = names
	}
	return out
}

// Two copies of one 番号 are one duplicate group, but copies carrying different
// part markers are separate episodes and must not be reported.
func TestListDuplicatesGroupsSameCodeOnly(t *testing.T) {
	s, _ := newDuplicateFixture(t, [][2]string{
		{"abc-123.mp4", "ABC-123"},
		{"abc-123 1080p.mkv", "ABC-123"},
		{"ABC-124.mp4", "ABC-124"},
		{"abc-124-1.mp4", "ABC-124"}, // part 1 of another work
		{"abc-124_2.mp4", "ABC-124"}, // part 2 of the same work
		{"abc-125-1.mp4", "ABC-125"},
		{"abc-125-2.mp4", "ABC-125"},
		{"unrecognized.mp4", ""},
	})

	groups, err := s.ListDuplicates(false)
	if err != nil {
		t.Fatal(err)
	}
	got := groupCodes(t, groups)
	want := map[string][]string{
		"ABC-123": {"abc-123 1080p.mkv", "abc-123.mp4"},
	}
	if len(got) != len(want) {
		t.Fatalf("groups = %#v, want %#v", got, want)
	}
	for key, names := range want {
		if len(got[key]) != len(names) || got[key][0] != names[0] || got[key][1] != names[1] {
			t.Fatalf("group %s = %v, want %v", key, got[key], names)
		}
	}
}

// A part marker is only read where the name really marks one, so resolution and
// quality tags never split a genuine pair of copies.
func TestListDuplicatesIgnoresResolutionSuffixes(t *testing.T) {
	s, _ := newDuplicateFixture(t, [][2]string{
		{"abc-123 1080p.mp4", "ABC-123"},
		{"abc-123 4K.mkv", "ABC-123"},
		{"abc-123-4K.mp4", "ABC-123"},
	})

	groups, err := s.ListDuplicates(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || len(groups[0].Videos) != 3 {
		t.Fatalf("groups = %#v, want one group of 3 copies", groups)
	}
}

// Dismissing a group hides it; adding another copy makes the set new again, so
// the prompt comes back instead of the new file inheriting the old dismissal.
func TestIgnoreDuplicateHidesUntilMembershipChanges(t *testing.T) {
	s, vr := newDuplicateFixture(t, [][2]string{
		{"abc-123.mp4", "ABC-123"},
		{"abc-123 copy.mp4", "ABC-123"},
	})

	groups, err := s.ListDuplicates(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 {
		t.Fatalf("groups = %#v, want 1", groups)
	}
	key := groups[0].Key
	if key != "ABC-123" {
		t.Fatalf("key = %q, want ABC-123", key)
	}

	if err := s.IgnoreDuplicate(key); err != nil {
		t.Fatal(err)
	}
	visible, err := s.ListDuplicates(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(visible) != 0 {
		t.Fatalf("dismissed group still visible: %#v", visible)
	}
	// The panel's "show ignored" switch still finds it, flagged as dismissed.
	all, err := s.ListDuplicates(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || !all[0].Ignored {
		t.Fatalf("includeIgnored = %#v, want the dismissed group marked ignored", all)
	}

	// A third copy changes the membership, so the group is worth a fresh look.
	v := &video.Video{Path: filepath.Join(t.TempDir(), "abc-123 hd.mp4"), FileName: "abc-123 hd.mp4", Status: video.StatusAvailable, CreatedAt: 9, UpdatedAt: 9}
	if err := vr.Create(v); err != nil {
		t.Fatal(err)
	}
	if err := s.repo.PrepareTask(v.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := s.repo.SaveSuccess(v.ID, Metadata{Code: "ABC-123"}, "filename", 1); err != nil {
		t.Fatal(err)
	}
	again, err := s.ListDuplicates(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 1 || again[0].Ignored {
		t.Fatalf("groups after a new copy = %#v, want the group surfaced again", again)
	}
}

// Dismissing a group whose copies are gone must not leave a stale marker behind
// that would silence a future, unrelated group with the same 番号.
func TestIgnoreDuplicateDropsStaleKey(t *testing.T) {
	s, _ := newDuplicateFixture(t, [][2]string{{"abc-123.mp4", "ABC-123"}})

	if err := s.IgnoreDuplicate("ABC-123"); err != nil {
		t.Fatal(err)
	}
	ignores, err := s.repo.ListDuplicateIgnores()
	if err != nil {
		t.Fatal(err)
	}
	if len(ignores) != 0 {
		t.Fatalf("ignores = %#v, want none for a group with fewer than two copies", ignores)
	}
}

func TestListDuplicatesWithoutJavServiceData(t *testing.T) {
	s, _ := newDuplicateFixture(t, [][2]string{{"abc-123.mp4", ""}})
	groups, err := s.ListDuplicates(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 {
		t.Fatalf("groups = %#v, want none", groups)
	}
}
