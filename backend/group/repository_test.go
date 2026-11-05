package group

import (
	"testing"

	"muse-app/backend/database"
)

func newTestRepo(t *testing.T) GroupRepository {
	t.Helper()
	db, _, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewSQLiteRepository(db)
}

func TestGroupCRUD(t *testing.T) {
	repo := newTestRepo(t)

	g1, err := repo.Create("Action", 0, nil, 1000)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if g1.ID == 0 || g1.Name != "Action" || g1.Position != 0 {
		t.Fatalf("got %+v", g1)
	}

	got, err := repo.Get(g1.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "Action" || got.VideoCount != 0 {
		t.Errorf("got %+v", got)
	}

	if _, err := repo.Get(999); err != ErrNotFound {
		t.Errorf("get missing: err = %v, want ErrNotFound", err)
	}

	if err := repo.Rename(g1.ID, "Adventure", 2000); err != nil {
		t.Fatalf("rename: %v", err)
	}
	got, _ = repo.Get(g1.ID)
	if got.Name != "Adventure" || got.UpdatedAt != 2000 {
		t.Errorf("rename not persisted: %+v", got)
	}

	if err := repo.Delete(g1.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := repo.Get(g1.ID); err != ErrNotFound {
		t.Errorf("after delete: err = %v, want ErrNotFound", err)
	}
}

func TestGroupListOrderAndReorder(t *testing.T) {
	repo := newTestRepo(t)
	a, _ := repo.Create("A", 0, nil, 1)
	b, _ := repo.Create("B", 1, nil, 1)
	c, _ := repo.Create("C", 2, nil, 1)

	all, err := repo.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 || all[0].ID != a.ID || all[1].ID != b.ID || all[2].ID != c.ID {
		t.Errorf("initial order unexpected: %+v", all)
	}

	// Drag C to the top: C, A, B.
	if err := repo.SetPositions([]int64{c.ID, a.ID, b.ID}); err != nil {
		t.Fatal(err)
	}
	all, _ = repo.List()
	if all[0].ID != c.ID || all[1].ID != a.ID || all[2].ID != b.ID {
		t.Errorf("reordered order unexpected: %+v", all)
	}
}

func TestGroupMembers(t *testing.T) {
	db, _, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := &sqliteRepository{db: db}

	// Videos must exist as favorites for VideosInGroup to list them (it joins
	// against the videos table so vanished favorites don't leak into a group).
	insertVideo := func(name string) int64 {
		res, err := db.Exec(
			`INSERT INTO videos (path, file_name, file_size, created_at, updated_at, status)
			 VALUES (?, ?, 1, ?, ?, 'available')`,
			"D:\\Videos\\"+name, name, nowMillis(), nowMillis(),
		)
		if err != nil {
			t.Fatalf("insert video: %v", err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	v10 := insertVideo("v10.mp4")
	v20 := insertVideo("v20.mp4")

	g1, _ := repo.Create("G1", 0, nil, 1)
	g2, _ := repo.Create("G2", 1, nil, 1)

	// Simulate a removed favorite whose membership was not cleaned: pointing at
	// a non-existent video id must not surface in VideosInGroup.
	_ = repo.SetMembers(v10, []int64{g1.ID, g2.ID})
	_ = repo.SetMembers(99999, []int64{g1.ID, g2.ID})

	got, err := repo.Members(v10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("members = %v", got)
	}

	// Replace: video v10 moves to G2 only.
	if err := repo.SetMembers(v10, []int64{g2.ID}); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.Members(v10)
	if len(got) != 1 || got[0] != g2.ID {
		t.Errorf("after replace members = %v", got)
	}

	// Add a second real video to G2 and check it lists back (newest first).
	if err := repo.SetMembers(v20, []int64{g1.ID, g2.ID}); err != nil {
		t.Fatal(err)
	}
	ids, err := repo.VideosInGroup(g2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids[0] != v20 || ids[1] != v10 {
		t.Errorf("group g2 video ids = %v", ids)
	}

	if err := repo.ClearVideoMembers(v10); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.Members(v10)
	if len(got) != 0 {
		t.Errorf("members after clear = %v", got)
	}
}

func TestGroupListIncludesVideoCount(t *testing.T) {
	db, _, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := NewSQLiteRepository(db)
	g, _ := repo.Create("G", 0, nil, 1)
	if _, err := db.Exec(
		`INSERT INTO videos (path, file_name, file_size, created_at, updated_at, status)
		 VALUES ('D:\\Videos\\v1.mp4','v1.mp4',1,1,1,'available'),
		        ('D:\\Videos\\v2.mp4','v2.mp4',1,1,1,'available')`); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetMembers(1, []int64{g.ID}); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetMembers(2, []int64{g.ID}); err != nil {
		t.Fatal(err)
	}
	list, err := repo.List()
	if err != nil {
		t.Fatal(err)
	}
	if list[0].VideoCount != 2 {
		t.Errorf("video count = %d, want 2", list[0].VideoCount)
	}
}

// Parent/child hierarchy: List() reports transitive counts and parent ids, and
// deleting a parent detaches its children instead of losing them.
func TestGroupParentChild(t *testing.T) {
	db, _, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := NewSQLiteRepository(db)
	if _, err := db.Exec(
		`INSERT INTO videos (path, file_name, file_size, created_at, updated_at, status)
		 VALUES ('D:\\Videos\\v7.mp4','v7.mp4',1,1,1,'available')`); err != nil {
		t.Fatal(err)
	}
	root, _ := repo.Create("Root", 0, nil, 1)
	child, _ := repo.Create("Child", 1, &root.ID, 1)
	grand, _ := repo.Create("Grand", 2, &child.ID, 1)

	list, err := repo.List()
	if err != nil {
		t.Fatal(err)
	}
	byID := map[int64]Group{}
	for _, g := range list {
		byID[g.ID] = g
	}
	if byID[child.ID].ParentID == nil || *byID[child.ID].ParentID != root.ID {
		t.Errorf("child parent = %v, want root", byID[child.ID].ParentID)
	}
	if byID[grand.ID].ParentID == nil || *byID[grand.ID].ParentID != child.ID {
		t.Errorf("grand parent = %v, want child", byID[grand.ID].ParentID)
	}

	// Assign a video to the grandchild; the transitive count should be visible
	// up the whole ancestor chain via List().
	if err := repo.SetMembers(1, []int64{grand.ID}); err != nil {
		t.Fatal(err)
	}
	list, _ = repo.List()
	byID = map[int64]Group{}
	for _, g := range list {
		byID[g.ID] = g
	}
	if byID[root.ID].VideoCount != 1 || byID[child.ID].VideoCount != 1 {
		t.Errorf("transitive counts root=%d child=%d, want 1/1",
			byID[root.ID].VideoCount, byID[child.ID].VideoCount)
	}

	children, err := repo.Children(root.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(children) != 1 || children[0] != child.ID {
		t.Errorf("children = %v, want [child]", children)
	}

	// Deleting the middle child must detach the grandchild to top level.
	if err := repo.Delete(child.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.Get(grand.ID)
	if got.ParentID != nil {
		t.Errorf("grand parent after delete = %v, want nil (detached)", got.ParentID)
	}
}

// VideosInGroups aggregates across descendant groups and deduplicates a video
// that is a member of several groups.
func TestGroupVideosInGroupsTransitive(t *testing.T) {
	db, _, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := &sqliteRepository{db: db}
	insert := func(name string) int64 {
		res, err := db.Exec(
			`INSERT INTO videos (path, file_name, file_size, created_at, updated_at, status)
			 VALUES (?, ?, 1, ?, ?, 'available')`,
			"D:\\Videos\\"+name, name, nowMillis(), nowMillis(),
		)
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	root, _ := repo.Create("Root", 0, nil, 1)
	child, _ := repo.Create("Child", 1, &root.ID, 1)

	vA := insert("a.mp4") // belongs to child (and transitively root)
	vB := insert("b.mp4") // belongs to root directly
	_ = repo.SetMembers(vA, []int64{child.ID})
	_ = repo.SetMembers(vB, []int64{root.ID})
	_ = repo.SetMembers(vA, []int64{root.ID, child.ID}) // dup in both -> dedup

	got, err := repo.VideosInGroups([]int64{root.ID, child.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d videos, want 2 (deduplicated): %v", len(got), got)
	}
	seen := map[int64]bool{}
	for _, id := range got {
		seen[id] = true
	}
	if !seen[vA] || !seen[vB] {
		t.Errorf("transitive set = %v, want both %d and %d", got, vA, vB)
	}
}

// TestGroupViewsExcludeTrashedFavorites is the recycle-bin contract for groups:
// memberships survive while a favorite is trashed, but it must not appear in a
// group view or count as a member until it is restored.
func TestGroupViewsExcludeTrashedFavorites(t *testing.T) {
	db, _, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := NewSQLiteRepository(db)
	res, err := db.Exec(
		`INSERT INTO videos (path, file_name, file_size, created_at, updated_at, status)
		 VALUES ('D:\\Videos\\a.mp4','a.mp4',1,1,1,'available'),
		        ('D:\\Videos\\b.mp4','b.mp4',1,1,1,'available')`)
	if err != nil {
		t.Fatal(err)
	}
	_ = res
	g, err := repo.Create("G", 0, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SetMembers(1, []int64{g.ID}); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetMembers(2, []int64{g.ID}); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM video_group_members`).Scan(new(int)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE videos SET deleted_at=1000, delete_reason='user' WHERE id=2`); err != nil {
		t.Fatal(err)
	}

	list, err := repo.List()
	if err != nil {
		t.Fatal(err)
	}
	if list[0].VideoCount != 1 {
		t.Errorf("count with one trashed member = %d, want 1", list[0].VideoCount)
	}
	ids, err := repo.VideosInGroup(g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != 1 {
		t.Errorf("group videos = %v, want [1]", ids)
	}
	// Membership is retained, so the recycle bin can still name the group.
	names, err := repo.GroupNamesForVideos([]int64{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(names[2]) != 1 || names[2][0] != "G" {
		t.Errorf("trashed favorite groups = %v, want [G]", names[2])
	}
}
