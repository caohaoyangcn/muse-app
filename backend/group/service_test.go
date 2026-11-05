package group

import (
	"io"
	"log/slog"
	"testing"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestService(t *testing.T) (*Service, *GroupRepository) {
	t.Helper()
	repo := newTestRepo(t)
	svc := NewService(repo, discardLogger(), nil)
	return svc, &repo
}

func TestServiceCreateValidatesName(t *testing.T) {
	svc, _ := newTestService(t)
	if _, err := svc.Create("   "); err != ErrEmptyName {
		t.Errorf("empty name err = %v, want ErrEmptyName", err)
	}
	if _, err := svc.Create("Action"); err != nil {
		t.Errorf("create: %v", err)
	}
}

func TestServiceRenameAndDelete(t *testing.T) {
	svc, _ := newTestService(t)
	g, err := svc.Create("Action")
	if err != nil {
		t.Fatal(err)
	}
	renamed, err := svc.Rename(g.ID, "Adventure")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Name != "Adventure" {
		t.Errorf("renamed name = %q", renamed.Name)
	}
	if _, err := svc.Rename(g.ID, ""); err != ErrEmptyName {
		t.Errorf("rename empty err = %v, want ErrEmptyName", err)
	}
	if err := svc.Delete(g.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(g.ID); err != ErrNotFound {
		t.Errorf("double delete err = %v, want ErrNotFound", err)
	}
}

func TestServiceEmitsOnChange(t *testing.T) {
	repo := newTestRepo(t)
	got := 0
	svc := NewService(repo, discardLogger(),
		func(name string, _ ...any) { got++ })

	if _, err := svc.Create("A"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Rename(1, "B"); err != nil {
		t.Fatal(err)
	}
	if got != 2 {
		t.Errorf("emit count = %d, want 2", got)
	}
}

// ClearVideoMembers emits group:changed so the sidebar's member-count badges
// refresh when a favorite is removed from the library.
func TestServiceClearVideoMembersEmits(t *testing.T) {
	repo := newTestRepo(t)
	got := 0
	svc := NewService(repo, discardLogger(),
		func(name string, _ ...any) { got++ })

	if err := svc.ClearVideoMembers(999); err != nil {
		t.Fatalf("clear members: %v", err)
	}
	if got != 1 {
		t.Errorf("emit count = %d, want 1", got)
	}
}

// SetParent rejects self-parenting and cycles, and detaches with a nil parent.
func TestServiceSetParentValidates(t *testing.T) {
	svc, _ := newTestService(t)
	a, _ := svc.Create("A")
	b, _ := svc.Create("B")

	// Make B a child of A (the common, valid case).
	if _, err := svc.SetParent(b.ID, &a.ID); err != nil {
		t.Fatalf("setparent B->A: %v", err)
	}
	got, err := svc.SetParent(a.ID, &a.ID)
	if err != ErrSelfParent {
		t.Errorf("self-parent err = %v, want ErrSelfParent", err)
	}
	_ = got

	// Cycle: making A a child of B (B is already A's child) must be rejected.
	if _, err := svc.SetParent(a.ID, &b.ID); err != ErrCycle {
		t.Errorf("cycle err = %v, want ErrCycle", err)
	}

	// Detach B back to top level.
	detached, err := svc.SetParent(b.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if detached.ParentID != nil {
		t.Errorf("detached parent = %v, want nil", detached.ParentID)
	}
}

// VideoIDsInGroup is transitive: a video in a child group is visible in the
// parent group because the parent aggregates all descendant members.
func TestServiceVideoIDsInGroupTransitive(t *testing.T) {
	repo := newTestRepo(t)
	svc := NewService(repo, discardLogger(), nil)

	root, _ := svc.Create("Root")
	child, _ := svc.Create("Child")
	if _, err := svc.SetParent(child.ID, &root.ID); err != nil {
		t.Fatal(err)
	}

	// Add a real video (VideosInGroups joins the videos table, so a fictitious
	// id would be dropped) and make it a member of the child group only.
	videoID := insertVideoRow(t, repo)
	if err := repo.SetMembers(videoID, []int64{child.ID}); err != nil {
		t.Fatal(err)
	}

	childIDs, err := svc.VideoIDsInGroup(child.ID)
	if err != nil {
		t.Fatal(err)
	}
	rootIDs, err := svc.VideoIDsInGroup(root.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(childIDs) != 1 || childIDs[0] != videoID {
		t.Errorf("child ids = %v, want [%d]", childIDs, videoID)
	}
	if len(rootIDs) != 1 || rootIDs[0] != videoID {
		t.Errorf("root ids = %v, want [%d] (transitive from child)", rootIDs, videoID)
	}
}

// VideoIDsInNamedGroup resolves a well-known group by name (case-insensitive
// and trimmed) and returns its visible members. An unknown name is an empty
// set, not an error, so a read model can call it unconditionally.
func TestServiceVideoIDsInNamedGroup(t *testing.T) {
	repo := newTestRepo(t)
	svc := NewService(repo, discardLogger(), nil)

	missing, err := svc.VideoIDsInNamedGroup("马赛克破解")
	if err != nil {
		t.Fatalf("unknown group: %v", err)
	}
	if len(missing) != 0 {
		t.Errorf("unknown group ids = %v, want empty", missing)
	}

	g, err := svc.EnsureGroup("马赛克破解")
	if err != nil {
		t.Fatal(err)
	}
	videoID := insertVideoRow(t, repo)
	if err := svc.AddVideoToGroup(videoID, g.ID); err != nil {
		t.Fatal(err)
	}

	ids, err := svc.VideoIDsInNamedGroup("  马赛克破解  ")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != videoID {
		t.Errorf("named group ids = %v, want [%d]", ids, videoID)
	}
}

// insertVideoRow inserts an available favorite row into the repo's database
// and returns its id. It reaches through GroupRepository to the concrete repo.
func insertVideoRow(t *testing.T, repo GroupRepository) int64 {
	t.Helper()
	sqlRepo, ok := repo.(*sqliteRepository)
	if !ok {
		t.Fatal("expected *sqliteRepository for video insertion")
	}
	at := nowMillis()
	res, err := sqlRepo.db.Exec(
		`INSERT INTO videos (path, file_name, file_size, created_at, updated_at, status)
		 VALUES (?, ?, 1, ?, ?, 'available')`,
		"D:\\Videos\\transitive.mp4", "transitive.mp4", at, at,
	)
	if err != nil {
		t.Fatalf("insert video: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}
