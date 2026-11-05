package api

import (
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"muse-app/backend/database"
	"muse-app/backend/group"
	"muse-app/backend/jav"
	"muse-app/backend/video"
)

// memVideoRepo is a minimal in-memory VideoRepository.
type memVideoRepo struct {
	all []video.Video
	// contentIgnores stores the content-duplicate dismissals so the facade test
	// can exercise "keep all" without a database.
	contentIgnores map[string]string
}

func (r *memVideoRepo) Create(v *video.Video) error {
	v.ID = int64(len(r.all) + 1)
	r.all = append(r.all, *v)
	return nil
}
func (r *memVideoRepo) Get(id int64) (*video.Video, error) {
	v, err := r.GetAny(id)
	if err != nil {
		return nil, err
	}
	if v.DeletedAt > 0 {
		return nil, video.ErrNotFound
	}
	return v, nil
}
func (r *memVideoRepo) GetAny(id int64) (*video.Video, error) {
	for i := range r.all {
		if r.all[i].ID == id {
			return &r.all[i], nil
		}
	}
	return nil, video.ErrNotFound
}
func (r *memVideoRepo) GetByPath(path string) (*video.Video, error) {
	for i := range r.all {
		if r.all[i].Path == path {
			return &r.all[i], nil
		}
	}
	return nil, video.ErrNotFound
}
func (r *memVideoRepo) GetByRemote(sourceID int64, remotePath string) (*video.Video, error) {
	for i := range r.all {
		if r.all[i].SourceID == sourceID && r.all[i].RemotePath == remotePath {
			return &r.all[i], nil
		}
	}
	return nil, video.ErrNotFound
}
func (r *memVideoRepo) ArchiveBySource(sourceID int64, sourceName string, at int64) (int, error) {
	n := 0
	for i := range r.all {
		if r.all[i].SourceID == sourceID && r.all[i].DeletedAt == 0 {
			r.all[i].DeletedAt = at
			r.all[i].DeleteReason = video.TrashReasonArchived
			r.all[i].ArchiveSourceName = sourceName
			n++
		}
	}
	return n, nil
}
func (r *memVideoRepo) ListArchived() ([]video.Video, error) {
	out := make([]video.Video, 0)
	for i := range r.all {
		if r.all[i].DeletedAt > 0 && r.all[i].DeleteReason == video.TrashReasonArchived {
			out = append(out, r.all[i])
		}
	}
	return out, nil
}
func (r *memVideoRepo) List() ([]video.Video, error) {
	out := make([]video.Video, 0, len(r.all))
	for _, v := range r.all {
		if v.DeletedAt == 0 {
			out = append(out, v)
		}
	}
	return out, nil
}
func (r *memVideoRepo) Search(keyword string) ([]video.Video, error) { return r.List() }
func (r *memVideoRepo) Trash(id int64, reason string, at int64) error {
	v, err := r.GetAny(id)
	if err != nil {
		return err
	}
	if v.DeletedAt == 0 {
		v.DeletedAt, v.DeleteReason = at, reason
	}
	return nil
}
func (r *memVideoRepo) Restore(id int64, at int64) error {
	v, err := r.GetAny(id)
	if err != nil {
		return err
	}
	v.DeletedAt, v.DeleteReason = 0, ""
	return nil
}
func (r *memVideoRepo) ListTrashed() ([]video.Video, error) {
	out := make([]video.Video, 0)
	for _, v := range r.all {
		if v.DeletedAt > 0 {
			out = append(out, v)
		}
	}
	return out, nil
}
func (r *memVideoRepo) Delete(id int64) error {
	out := r.all[:0]
	for _, v := range r.all {
		if v.ID != id {
			out = append(out, v)
		}
	}
	r.all = out
	return nil
}
func (r *memVideoRepo) UpdateStatus(id int64, s string) error { return nil }
func (r *memVideoRepo) UpdateMetadata(id int64, d int64, w, h int, fr float64, br int64, vc, ac string) error {
	return nil
}
func (r *memVideoRepo) UpdateThumbnail(id int64, p string) error               { return nil }
func (r *memVideoRepo) UpdateThumbnailIfAuto(id int64, p string) (bool, error) { return true, nil }
func (r *memVideoRepo) SetScrapedCover(id int64, p string) error               { return nil }
func (r *memVideoRepo) SetScrapedCoverAllowed(id int64, allowed bool) error    { return nil }
func (r *memVideoRepo) ApplyScrapedCoverIfAllowed(id int64, p string) (bool, error) {
	return true, nil
}
func (r *memVideoRepo) SetCoverSource(id int64, source string) error { return nil }
func (r *memVideoRepo) UpdateLastOpened(id int64, t int64) error     { return nil }
func (r *memVideoRepo) ListNeedingMetadata() ([]int64, error)        { return nil, nil }
func (r *memVideoRepo) UpdateInfo(id int64, title, notes string) error {
	v, err := r.Get(id)
	if err != nil {
		return err
	}
	v.Title, v.Notes = title, notes
	return nil
}
func (r *memVideoRepo) SetCustomCover(id int64, at int64) error {
	v, err := r.Get(id)
	if err != nil {
		return err
	}
	v.CustomCoverAt = at
	return nil
}
func (r *memVideoRepo) ClearCustomCover(id int64) error {
	v, err := r.Get(id)
	if err != nil {
		return err
	}
	v.CustomCoverAt = 0
	return nil
}
func (r *memVideoRepo) IncrementLikes(id int64) (int64, error) {
	v, err := r.Get(id)
	if err != nil {
		return 0, err
	}
	v.LikeCount++
	return v.LikeCount, nil
}
func (r *memVideoRepo) ClearLikes(id int64) error {
	v, err := r.Get(id)
	if err != nil {
		return err
	}
	v.LikeCount = 0
	return nil
}
func (r *memVideoRepo) UpdateFingerprint(id int64, hash string, version int, size, mtime, at int64) error {
	v, err := r.Get(id)
	if err != nil {
		return err
	}
	v.ContentHash, v.HashVersion = hash, version
	v.HashState = video.FingerprintState{Size: size, Mtime: mtime, At: at}
	return nil
}
func (r *memVideoRepo) UpdateFingerprintFailure(id int64, message string, at int64) error {
	v, err := r.Get(id)
	if err != nil {
		return err
	}
	v.ContentHash, v.HashVersion = "", 0
	v.HashState.Error = message
	v.HashState.At = at
	return nil
}
func (r *memVideoRepo) UpdateFullHash(id int64, hash string, at int64) error {
	v, err := r.Get(id)
	if err != nil {
		return err
	}
	v.HashState.FullHash, v.HashState.FullHashAt = hash, at
	return nil
}
func (r *memVideoRepo) ListNeedingFingerprint(version int) ([]int64, error) {
	ids := make([]int64, 0, len(r.all))
	for i := range r.all {
		if r.all[i].SourceID == 0 && (r.all[i].ContentHash == "" || r.all[i].HashVersion != version) {
			ids = append(ids, r.all[i].ID)
		}
	}
	return ids, nil
}
func (r *memVideoRepo) ListSameSizePeers(size int64, excludeID int64) ([]int64, error) {
	ids := make([]int64, 0, 4)
	for i := range r.all {
		if r.all[i].SourceID == 0 && r.all[i].ID != excludeID && r.all[i].FileSize == size {
			ids = append(ids, r.all[i].ID)
		}
	}
	return ids, nil
}
func (r *memVideoRepo) ListRelocationCandidates(size int64, version int) ([]video.Video, error) {
	out := make([]video.Video, 0, 2)
	for i := range r.all {
		if r.all[i].SourceID == 0 && r.all[i].FileSize == size && r.all[i].ContentHash != "" && r.all[i].HashVersion == version {
			out = append(out, r.all[i])
		}
	}
	return out, nil
}
func (r *memVideoRepo) RelinkPath(id int64, path, fileName, hash string, version int, size, mtime, at int64) error {
	v, err := r.Get(id)
	if err != nil {
		return err
	}
	v.Path, v.FileName, v.Status, v.FileSize = path, fileName, video.StatusAvailable, size
	v.ContentHash, v.HashVersion = hash, version
	v.HashState = video.FingerprintState{Size: size, Mtime: mtime, At: at}
	return nil
}
func (r *memVideoRepo) ListFingerprinted() ([]video.Video, error) {
	out := make([]video.Video, 0, len(r.all))
	for i := range r.all {
		if r.all[i].SourceID == 0 && r.all[i].ContentHash != "" {
			out = append(out, r.all[i])
		}
	}
	return out, nil
}
func (r *memVideoRepo) ListByContentHash(hash string) ([]video.Video, error) {
	out := make([]video.Video, 0, 2)
	for i := range r.all {
		if r.all[i].SourceID == 0 && r.all[i].ContentHash == hash {
			out = append(out, r.all[i])
		}
	}
	return out, nil
}
func (r *memVideoRepo) ListContentDuplicateIgnores() ([]video.IgnoredContentDuplicate, error) {
	out := make([]video.IgnoredContentDuplicate, 0, len(r.contentIgnores))
	for key, ids := range r.contentIgnores {
		out = append(out, video.IgnoredContentDuplicate{Key: key, VideoIDs: ids})
	}
	return out, nil
}
func (r *memVideoRepo) UpsertContentDuplicateIgnore(key, videoIDs string) error {
	if r.contentIgnores == nil {
		r.contentIgnores = map[string]string{}
	}
	r.contentIgnores[key] = videoIDs
	return nil
}
func (r *memVideoRepo) DeleteContentDuplicateIgnore(key string) error {
	delete(r.contentIgnores, key)
	return nil
}
func (r *memVideoRepo) ListVideoIDs() ([]int64, error) {
	ids := make([]int64, 0, len(r.all))
	for _, v := range r.all {
		ids = append(ids, v.ID)
	}
	return ids, nil
}

// memGroupRepo records written memberships and video clears.
type memGroupRepo struct {
	groups    []group.Group
	members   map[int64][]int64 // groupID -> videoIDs
	clearVids []int64
}

func (r *memGroupRepo) Create(name string, pos int, parentID *int64, at int64) (*group.Group, error) {
	// Deterministic increasing id so tests can distinguish created groups.
	next := int64(100)
	for _, g := range r.groups {
		if g.ID >= next {
			next = g.ID + 1
		}
	}
	g := group.Group{ID: next, Name: name, ParentID: parentID, Position: pos, CreatedAt: at, UpdatedAt: at}
	r.groups = append(r.groups, g)
	return &g, nil
}
func (r *memGroupRepo) Get(id int64) (*group.Group, error) {
	return &group.Group{ID: id, Name: "g"}, nil
}
func (r *memGroupRepo) FindByName(name string) (*group.Group, error) {
	for i := range r.groups {
		if strings.EqualFold(r.groups[i].Name, name) {
			return &r.groups[i], nil
		}
	}
	return nil, group.ErrNotFound
}
func (r *memGroupRepo) List() ([]group.Group, error) {
	if r.groups != nil {
		return r.groups, nil
	}
	return nil, nil
}
func (r *memGroupRepo) Rename(id int64, name string, at int64) error { return nil }
func (r *memGroupRepo) Delete(id int64) error                        { return nil }
func (r *memGroupRepo) SetPositions(ids []int64) error               { return nil }
func (r *memGroupRepo) SetParent(id int64, parentID *int64, at int64) error {
	return nil
}
func (r *memGroupRepo) Children(parentID int64) ([]int64, error) { return nil, nil }
func (r *memGroupRepo) VideosInGroups(groupIDs []int64) ([]int64, error) {
	var out []int64
	seen := map[int64]bool{}
	for _, gid := range groupIDs {
		for _, vid := range r.members[gid] {
			if !seen[vid] {
				seen[vid] = true
				out = append(out, vid)
			}
		}
	}
	return out, nil
}
func (r *memGroupRepo) Members(videoID int64) ([]int64, error)      { return nil, nil }
func (r *memGroupRepo) SetMembers(videoID int64, ids []int64) error { return nil }
func (r *memGroupRepo) AddVideoToGroup(videoID, groupID int64) error {
	r.members[groupID] = append(r.members[groupID], videoID)
	return nil
}
func (r *memGroupRepo) VideosInGroup(groupID int64) ([]int64, error) { return r.members[groupID], nil }
func (r *memGroupRepo) ClearVideoMembers(videoID int64) error {
	r.clearVids = append(r.clearVids, videoID)
	return nil
}
func (r *memGroupRepo) GroupNamesForVideos(videoIDs []int64) (map[int64][]string, error) {
	out := make(map[int64][]string, len(videoIDs))
	wanted := make(map[int64]bool, len(videoIDs))
	for _, id := range videoIDs {
		wanted[id] = true
	}
	for _, g := range r.groups {
		for _, vid := range r.members[g.ID] {
			if wanted[vid] {
				out[vid] = append(out[vid], g.Name)
			}
		}
	}
	return out, nil
}

func newFacade(t *testing.T) (*Library, *memVideoRepo, *memGroupRepo) {
	t.Helper()
	discard := slog.New(slog.NewTextHandler(io.Discard, nil))
	vr := &memVideoRepo{all: []video.Video{
		{ID: 10, Path: filepath.Join(t.TempDir(), "a.mp4"), FileName: "a.mp4", Status: video.StatusAvailable},
		{ID: 20, Path: filepath.Join(t.TempDir(), "b.mp4"), FileName: "b.mp4", Status: video.StatusMissing},
	}}
	gr := &memGroupRepo{members: map[int64][]int64{1: {20, 10}}} // group 1 holds videos [20,10]
	lib := New(
		video.NewService(vr, nil, t.TempDir(), discard, nil),
		group.NewService(gr, discard, nil),
		discard,
	)
	return lib, vr, gr
}

// VideosInGroup resolves ids → full objects in the group's order (newest first).
func TestVideosInGroupResolvesObjects(t *testing.T) {
	lib, _, _ := newFacade(t)
	got, err := lib.VideosInGroup(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d videos, want 2", len(got))
	}
	if got[0].ID != 20 || got[1].ID != 10 {
		t.Errorf("order = [%d,%d], want [20,10] (group order preserved)", got[0].ID, got[1].ID)
	}
	if got[0].FileName != "b.mp4" {
		t.Errorf("resolved object mismatch: %q", got[0].FileName)
	}
}

// RemoveVideo is now a soft delete: the record moves to the recycle bin and
// keeps its group memberships, so restoring it puts it back where it was.
func TestRemoveVideoMovesToTrashKeepsMembers(t *testing.T) {
	lib, vr, gr := newFacade(t)
	if err := lib.RemoveVideo(10); err != nil {
		t.Fatal(err)
	}
	if len(gr.clearVids) != 0 {
		t.Errorf("trashing must not clear memberships, got %v", gr.clearVids)
	}
	if v, err := vr.GetAny(10); err != nil || v.DeletedAt == 0 || v.DeleteReason != video.TrashReasonUser {
		t.Fatalf("trashed row = %+v, err=%v", v, err)
	}
	if _, err := vr.Get(10); !errors.Is(err, video.ErrNotFound) {
		t.Errorf("trashed row must not be visible to Get: %v", err)
	}
	trash, err := lib.ListTrash()
	if err != nil {
		t.Fatal(err)
	}
	if len(trash.Videos) != 1 || trash.Videos[0].ID != 10 {
		t.Errorf("trash = %+v, want video 10", trash)
	}
	if trash.RetentionDays != DefaultTrashRetentionDays {
		t.Errorf("retention = %d, want %d", trash.RetentionDays, DefaultTrashRetentionDays)
	}
}

// A stale list (the record was already removed) must not surface an error.
func TestRemoveVideoAlreadyGone(t *testing.T) {
	lib, _, _ := newFacade(t)
	if err := lib.RemoveVideo(10); err != nil {
		t.Fatalf("first remove: %v", err)
	}
	if err := lib.RemoveVideo(10); err != nil {
		t.Errorf("removing an already trashed favorite must succeed, got %v", err)
	}
	if err := lib.RemoveVideo(999); err != nil {
		t.Errorf("removing an unknown favorite must succeed, got %v", err)
	}
}

// Permanent deletion is the only path that clears memberships and scraped
// metadata, and it stays idempotent for a stale card.
func TestDeleteVideoPermanentlyClearsRelations(t *testing.T) {
	lib, _, gr := newFacade(t)
	if err := lib.DeleteVideoPermanently(10); err != nil {
		t.Fatalf("permanent delete: %v", err)
	}
	if len(gr.clearVids) != 1 || gr.clearVids[0] != 10 {
		t.Errorf("cleared videos = %v, want [10]", gr.clearVids)
	}
	if err := lib.DeleteVideoPermanently(999); err != nil {
		t.Errorf("deleting an unknown favorite must succeed, got %v", err)
	}
	if len(gr.clearVids) != 2 || gr.clearVids[1] != 999 {
		t.Errorf("group memberships should still be cleared for a stale removal, got %v", gr.clearVids)
	}
}

// SuggestGroupsForVideos matches existing groups whose name is a case-insensitive
// substring of the file base name, on a per-file basis.
func TestSuggestGroupsForVideos(t *testing.T) {
	discard := slog.New(slog.NewTextHandler(io.Discard, nil))
	vr := &memVideoRepo{}
	gr := &memGroupRepo{
		groups: []group.Group{
			{ID: 1, Name: "4K"},
			{ID: 2, Name: "动漫"},
			{ID: 3, Name: "每日"},
		},
		members: map[int64][]int64{},
	}
	lib := New(
		video.NewService(vr, nil, t.TempDir(), discard, nil),
		group.NewService(gr, discard, nil),
		discard,
	)

	got := lib.SuggestGroupsForVideos([]string{
		`D:\videos\每日总结-watch.mp4`,   // base contains "每日" → 每日
		`D:\videos\我的动漫合集.mp4`,       // base contains "动漫" → 动漫
		`D:\videos\movie.4K.xxx.mkv`, // base contains "4k" (case-insensitive) → 4K
		"D:/videos/movie.mkv",        // no restored
	})
	if len(got) != 4 {
		t.Fatalf("got %d suggestions, want 4", len(got))
	}
	// File 0 → 每日 (group 3)
	if len(got[0].Suggested) != 1 || got[0].Suggested[0].GroupID != 3 {
		t.Errorf("file0 suggested = %+v, want only group 3", got[0].Suggested)
	}
	// File 1 → 动漫 (group 2)
	if len(got[1].Suggested) != 1 || got[1].Suggested[0].GroupID != 2 {
		t.Errorf("file1 suggested = %+v, want only group 2", got[1].Suggested)
	}
	// File 2 → 4K (group 1)
	if len(got[2].Suggested) != 1 || got[2].Suggested[0].GroupID != 1 {
		t.Errorf("file2 suggested = %+v, want only group 1", got[2].Suggested)
	}
	// File 3 → none
	if len(got[3].Suggested) != 0 {
		t.Errorf("file3 suggested = %+v, want none", got[3].Suggested)
	}
	if got[3].Suggested == nil {
		t.Error("file3 suggested is nil, want an empty array for frontend consumers")
	}
}

// TestSuggestRestoredGroupAutoCreates verifies that a file whose name contains
// "restored" is suggested for the "马赛克破解" group, creating it on first use.
func TestSuggestRestoredGroupAutoCreates(t *testing.T) {
	discard := slog.New(slog.NewTextHandler(io.Discard, nil))
	vr := &memVideoRepo{}
	gr := &memGroupRepo{
		groups:  []group.Group{{ID: 1, Name: "每日"}},
		members: map[int64][]int64{},
	}
	lib := New(
		video.NewService(vr, nil, t.TempDir(), discard, nil),
		group.NewService(gr, discard, nil),
		discard,
	)

	got := lib.SuggestGroupsForVideos([]string{
		"D:/videos/Movie.Restored.4K.mp4", // contains restored (case-insensitive)
		"D:/videos/movie.mkv",             // no restored
	})
	if len(got) != 2 {
		t.Fatalf("got %d suggestions, want 2", len(got))
	}
	// File 0 → 马赛克破解 (created on first use).
	if len(got[0].Suggested) != 1 || got[0].Suggested[0].Name != "马赛克破解" {
		t.Fatalf("file0 suggested = %+v, want 马赛克破解", got[0].Suggested)
	}
	// The group was actually created (not just suggested).
	if _, err := gr.FindByName("马赛克破解"); err != nil {
		t.Errorf("马赛克破解 group should have been created, got err %v", err)
	}
	// File 1 → none.
	if len(got[1].Suggested) != 0 {
		t.Errorf("file1 suggested = %+v, want none", got[1].Suggested)
	}

	// Second call reuses the existing group (no duplicate creation).
	countBefore := len(gr.groups)
	got2 := lib.SuggestGroupsForVideos([]string{"D:/videos/restored2.mp4"})
	if len(got2) != 1 || len(got2[0].Suggested) != 1 {
		t.Fatalf("second call suggested = %+v", got2)
	}
	if len(gr.groups) != countBefore {
		t.Errorf("EnsureGroup should reuse existing group, groups = %d", len(gr.groups))
	}
}

// EnsureVideoIn4KGroup auto-assigns a confirmed 4K video to the "4K" group.
func TestEnsureVideoIn4KGroupAutoAssigns(t *testing.T) {
	discard := slog.New(slog.NewTextHandler(io.Discard, nil))
	vr := &memVideoRepo{all: []video.Video{
		{ID: 7, Path: filepath.Join(t.TempDir(), "k.mp4"), FileName: "k.mp4", Width: 3840, Height: 2160},
		{ID: 8, Path: filepath.Join(t.TempDir(), "f.mp4"), FileName: "f.mp4", Width: 1920, Height: 1080},
	}}
	gr := &memGroupRepo{members: map[int64][]int64{}}
	lib := New(
		video.NewService(vr, nil, t.TempDir(), discard, nil),
		group.NewService(gr, discard, nil),
		discard,
	)

	lib.EnsureVideoIn4KGroup(7) // 4K → added to "4K"
	lib.EnsureVideoIn4KGroup(8) // 1080p → skipped
	g4k, err := gr.FindByName("4K")
	if err != nil {
		t.Fatalf("4K group not created: %v", err)
	}
	if len(gr.members[g4k.ID]) != 1 || gr.members[g4k.ID][0] != 7 {
		t.Errorf("members[%d] = %v, want [7]", g4k.ID, gr.members[g4k.ID])
	}
}

// AutoTagVideos silently applies the automatic group rules to freshly imported
// videos: restored files land in "马赛克破解" (auto-created), name-matched
// files land in existing groups, and failures are skipped.
func TestAutoTagVideos(t *testing.T) {
	discard := slog.New(slog.NewTextHandler(io.Discard, nil))
	vr := &memVideoRepo{}
	gr := &memGroupRepo{
		groups: []group.Group{
			{ID: 1, Name: "动漫"},
			{ID: 3, Name: "每日"},
		},
		members: map[int64][]int64{},
	}
	lib := New(
		video.NewService(vr, nil, t.TempDir(), discard, nil),
		group.NewService(gr, discard, nil),
		discard,
	)

	lib.AutoTagVideos([]video.AddVideoResult{
		{Path: `D:\videos\movie.Restored.mp4`, Success: true, VideoID: 10}, // → 马赛克破解（自动建组）
		{Path: `D:\videos\每日总结.mp4`, Success: true, VideoID: 11},           // → 每日
		{Path: `D:\videos\plain.mkv`, Success: true, VideoID: 12},          // name matches nothing
		{Path: `D:\videos\broken.mp4`, Success: false, VideoID: 0},         // failed add → skipped
	})

	restored, err := gr.FindByName("马赛克破解")
	if err != nil {
		t.Fatalf("马赛克破解 group not auto-created: %v", err)
	}
	// File 10 → restored group (auto-created).
	if len(gr.members[restored.ID]) != 1 || gr.members[restored.ID][0] != 10 {
		t.Errorf("restored members = %v, want [10]", gr.members[restored.ID])
	}
	// File 11 → existing "每日" (group 3).
	if len(gr.members[3]) != 1 || gr.members[3][0] != 11 {
		t.Errorf("每日 members = %v, want [11]", gr.members[3])
	}
	// File 12 (no match) and the failed file must not be grouped anywhere.
	for gid, vids := range gr.members {
		for _, vid := range vids {
			if vid == 12 || vid == 0 {
				t.Errorf("unexpected membership: group %d holds video %d", gid, vid)
			}
		}
	}
}

// AutoTagVideos on an empty batch must not create any groups.
func TestAutoTagVideosEmptyBatch(t *testing.T) {
	discard := slog.New(slog.NewTextHandler(io.Discard, nil))
	gr := &memGroupRepo{members: map[int64][]int64{}}
	lib := New(
		video.NewService(&memVideoRepo{}, nil, t.TempDir(), discard, nil),
		group.NewService(gr, discard, nil),
		discard,
	)

	lib.AutoTagVideos(nil)
	if len(gr.groups) != 0 {
		t.Errorf("AutoTagVideos(nil) created groups: %+v", gr.groups)
	}
}

// ListGroups must pass the parent/child hierarchy through to the web client: the
// ParentID field reaches the LAN SPA so it can render the tree. Also verifies
// that a video in a child group is visible via the parent group's endpooint
// (transitive membership), which the mobile SPA relies on.
func TestListGroupsAndTransitiveVideosAcrossTransport(t *testing.T) {
	discard := slog.New(slog.NewTextHandler(io.Discard, nil))
	pid := int64(1)
	gr := &memGroupRepo{
		groups: []group.Group{
			{ID: 1, Name: "Root", ParentID: nil},
			{ID: 2, Name: "Child", ParentID: &pid},
		},
		members: map[int64][]int64{2: {10}}, // video 10 lives in the child group only
	}
	vr := &memVideoRepo{all: []video.Video{
		{ID: 10, Path: filepath.Join(t.TempDir(), "a.mp4"), FileName: "a.mp4", Status: video.StatusAvailable},
	}}
	lib := New(
		video.NewService(vr, nil, t.TempDir(), discard, nil),
		group.NewService(gr, discard, nil),
		discard,
	)

	got, err := lib.ListGroups()
	if err != nil {
		t.Fatal(err)
	}
	byID := map[int64]group.Group{}
	for _, g := range got {
		byID[g.ID] = g
	}
	if byID[1].ParentID != nil {
		t.Errorf("root parent = %v, want nil", byID[1].ParentID)
	}
	if byID[2].ParentID == nil || *byID[2].ParentID != 1 {
		t.Errorf("child parent = %v, want root (1)", byID[2].ParentID)
	}

	// Viewing the parent group returns the child's video (transitive).
	vids, err := lib.VideosInGroup(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(vids) != 1 || vids[0].ID != 10 {
		t.Errorf("parent group videos = %v, want the child's video", vids)
	}
}

// newActorFacade builds a façade whose jav service has real (SQLite-backed)
// scrape results, so the actor aggregate and its id→object resolution are
// exercised end to end.
func newActorFacade(t *testing.T, casts [][]string) *Library {
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

	for i, cast := range casts {
		v := &video.Video{
			Path:     filepath.Join(t.TempDir(), "v"+string(rune('a'+i))+".mp4"),
			FileName: "v" + string(rune('a'+i)) + ".mp4",
			Status:   video.StatusAvailable,
		}
		if err := vr.Create(v); err != nil {
			t.Fatal(err)
		}
		if err := javRepo.PrepareTask(v.ID, false); err != nil {
			t.Fatal(err)
		}
		if err := javRepo.SaveSuccess(v.ID, jav.Metadata{Code: "C" + v.FileName, Actors: cast}, "filename", 1); err != nil {
			t.Fatal(err)
		}
	}
	return lib
}

// The facade attaches the authoritative 番号 to the favorites it hands both
// transports, so the UI can show the work's identity instead of the file name.
func TestListVideosCarriesScrapedCode(t *testing.T) {
	lib := newActorFacade(t, [][]string{{"A"}, {"A"}})

	videos, err := lib.ListVideos()
	if err != nil {
		t.Fatal(err)
	}
	if len(videos) != 2 {
		t.Fatalf("got %d videos, want 2", len(videos))
	}
	for _, v := range videos {
		if want := "C" + v.FileName; v.JavCode != want {
			t.Errorf("ListVideos code = %q, want %q", v.JavCode, want)
		}
	}

	single, err := lib.Video(videos[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := "C" + single.FileName; single.JavCode != want {
		t.Errorf("Video code = %q, want %q", single.JavCode, want)
	}

	// The scrape board reads its favorites through a separate query; it must
	// carry the same code.
	overviews, err := lib.ListJavTaskOverviews()
	if err != nil {
		t.Fatal(err)
	}
	if len(overviews) != 2 {
		t.Fatalf("got %d task overviews, want 2", len(overviews))
	}
	for _, overview := range overviews {
		if want := "C" + overview.Video.FileName; overview.Video.JavCode != want {
			t.Errorf("task overview code = %q, want %q", overview.Video.JavCode, want)
		}
	}
}

// Favorites in the 马赛克破解 group carry the tag flag both clients render
// beside the name; favorites outside it do not.
func TestListVideosCarriesRestoredBadge(t *testing.T) {
	lib, _, gr := newFacade(t)
	restored := group.Group{ID: 7, Name: "马赛克破解"}
	gr.groups = append(gr.groups, restored)
	gr.members[restored.ID] = []int64{10} // only the first favorite is restored

	videos, err := lib.ListVideos()
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[int64]video.Video, len(videos))
	for _, v := range videos {
		byID[v.ID] = v
	}
	if !byID[10].MosaicRestored {
		t.Error("favorite in 马赛克破解 should carry the badge")
	}
	if byID[20].MosaicRestored {
		t.Error("favorite outside 马赛克破解 must not carry the badge")
	}

	single, err := lib.Video(10)
	if err != nil {
		t.Fatal(err)
	}
	if !single.MosaicRestored {
		t.Error("single-favorite read should carry the badge too")
	}
}

// Without a JAV service the read model is still complete; the code is simply
// absent, which is what makes the UI fall back to the file name.
func TestListVideosWithoutJavServiceLeavesCodeEmpty(t *testing.T) {
	lib, _, _ := newFacade(t)
	videos, err := lib.ListVideos()
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range videos {
		if v.JavCode != "" {
			t.Errorf("code = %q, want empty without a JAV service", v.JavCode)
		}
	}
}

// VideosByActor resolves the actor's video ids back into full favorites in the
// same newest-first shape as ListVideos.
func TestVideosByActorResolvesObjects(t *testing.T) {
	lib := newActorFacade(t, [][]string{{"A"}, {"A", "B"}, {"B"}})

	actors, err := lib.ListActors()
	if err != nil {
		t.Fatal(err)
	}
	want := []jav.ActorSummary{{Name: "A", Count: 2}, {Name: "B", Count: 2}}
	if len(actors) != len(want) ||
		actors[0].Name != want[0].Name || actors[0].Count != want[0].Count ||
		actors[1].Name != want[1].Name || actors[1].Count != want[1].Count {
		t.Fatalf("actors=%+v, want %+v", actors, want)
	}

	got, err := lib.VideosByActor("A")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d videos, want 2", len(got))
	}
	for _, v := range got {
		if v.FileName == "" {
			t.Errorf("actor video not resolved to a full object: %+v", v)
		}
	}
}

// The facade reports a missing jav service rather than panicking.
func TestVideosByActorWithoutJavService(t *testing.T) {
	lib, _, _ := newFacade(t)
	if _, err := lib.VideosByActor("A"); err == nil {
		t.Fatal("want an error when the jav service is not configured")
	}
	if _, err := lib.ListActors(); err == nil {
		t.Fatal("want an error when the jav service is not configured")
	}
	if err := lib.MergeActors("A", "B"); err == nil {
		t.Fatal("want an error when the jav service is not configured")
	}
	if err := lib.UnmergeActor("A"); err == nil {
		t.Fatal("want an error when the jav service is not configured")
	}
}

// Merging through the facade folds one actor's films into the other, and the
// merged film list spans both scraped names.
func TestMergeActorsThroughFacade(t *testing.T) {
	lib := newActorFacade(t, [][]string{{"A"}, {"A"}, {"B"}})
	if err := lib.MergeActors("B", "A"); err != nil {
		t.Fatal(err)
	}
	actors, err := lib.ListActors()
	if err != nil {
		t.Fatal(err)
	}
	if len(actors) != 1 || actors[0].Name != "A" || actors[0].Count != 3 {
		t.Fatalf("actors=%+v, want one entry {A 3}", actors)
	}
	if len(actors[0].Aliases) != 1 || actors[0].Aliases[0] != "B" {
		t.Fatalf("aliases=%v, want [B]", actors[0].Aliases)
	}

	got, err := lib.VideosByActor("B")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("looking up merged alias gave %d films, want 3", len(got))
	}
}

// newDuplicateFacade builds a façade whose favorites carry real file names and
// recognized 番号, so the 查重 worklist is exercised end to end through the
// façade both transports use.
func newDuplicateFacade(t *testing.T, entries [][2]string) (*Library, video.VideoRepository) {
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

	for i, entry := range entries {
		v := &video.Video{
			Path:      filepath.Join(t.TempDir(), entry[0]),
			FileName:  entry[0],
			Status:    video.StatusAvailable,
			CreatedAt: int64(i + 1),
			UpdatedAt: int64(i + 1),
		}
		// The worklist now reconciles its members against the disk, so the
		// fixtures have to describe files that exist. Tests that need a missing
		// version remove the file after the fixture is built.
		if err := os.WriteFile(v.Path, []byte("fixture"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := vr.Create(v); err != nil {
			t.Fatal(err)
		}
		if err := javRepo.PrepareTask(v.ID, false); err != nil {
			t.Fatal(err)
		}
		if err := javRepo.SaveSuccess(v.ID, jav.Metadata{Code: entry[1]}, "filename", 1); err != nil {
			t.Fatal(err)
		}
	}
	return lib, vr
}

// The façade exposes the duplicate worklist and the "keep both" dismissal both
// transports call.
func TestDuplicateWorklistThroughFacade(t *testing.T) {
	lib, _ := newDuplicateFacade(t, [][2]string{
		{"abc-123.mp4", "ABC-123"},
		{"abc-123_1.mp4", "ABC-123"}, // part 1: an episode, so a separate work
		{"abc-123-2.mp4", "ABC-123"}, // part 2 of that episode pair
		{"abc-123 bd.mkv", "ABC-123"},
	})

	groups, err := lib.ListJavDuplicates(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 {
		t.Fatalf("groups = %#v, want one (the two unmarked copies)", groups)
	}
	if groups[0].Key != "ABC-123" || groups[0].Code != "ABC-123" || groups[0].Part != "" {
		t.Fatalf("group = %+v, want key/code ABC-123 with no part", groups[0])
	}
	if len(groups[0].Videos) != 2 || groups[0].Videos[0].FileName != "abc-123 bd.mkv" {
		t.Fatalf("group videos = %#v, want the two unmarked copies newest first", groups[0].Videos)
	}

	if err := lib.IgnoreJavDuplicate(groups[0].Key); err != nil {
		t.Fatal(err)
	}
	visible, err := lib.ListJavDuplicates(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(visible) != 0 {
		t.Fatalf("groups after dismissal = %#v, want none", visible)
	}
}

// TestDuplicateWorklistRemovesAMissingVersion is the read-path behaviour the
// panel relies on: a copy whose file is gone on an attached drive is removed
// before the worklist is returned, so the user is never offered "keep" for a
// file that no longer exists. One copy left means the work is not a duplicate
// any more.
func TestDuplicateWorklistRemovesAMissingVersion(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("automatic cleanup needs an identifiable volume (Windows drive letters)")
	}
	lib, vr := newDuplicateFacade(t, [][2]string{
		{"abc-123.mp4", "ABC-123"},
		{"abc-123 bd.mkv", "ABC-123"},
	})

	all, err := vr.List()
	if err != nil {
		t.Fatal(err)
	}
	var goneID, keptID int64
	var gonePath string
	for _, v := range all {
		if v.FileName == "abc-123.mp4" {
			goneID, gonePath = v.ID, v.Path
			continue
		}
		keptID = v.ID
	}
	if goneID == 0 || keptID == 0 {
		t.Fatalf("fixture videos = %#v", all)
	}
	if err := os.Remove(gonePath); err != nil {
		t.Fatal(err)
	}

	groups, err := lib.ListJavDuplicates(false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(groups) != 0 {
		t.Fatalf("groups = %#v, want none: one copy is not a duplicate", groups)
	}
	if _, err := vr.Get(goneID); !errors.Is(err, video.ErrNotFound) {
		t.Errorf("missing version still recorded: err = %v, want ErrNotFound", err)
	}
	if _, err := vr.Get(keptID); err != nil {
		t.Errorf("the present version must stay: %v", err)
	}
}

// Without a jav service the façade reports the gap instead of panicking.
func TestDuplicatesWithoutJavService(t *testing.T) {
	lib, _, _ := newFacade(t)
	if _, err := lib.ListJavDuplicates(false); err == nil {
		t.Fatal("want an error when the jav service is not configured")
	}
	if err := lib.IgnoreJavDuplicate("ABC-123"); err == nil {
		t.Fatal("want an error when the jav service is not configured")
	}
}

// newContentDuplicateFacade builds a façade over a real database and a real
// directory, so the content worklist is exercised end to end: the fingerprints
// are read from the rows and the files are checked on disk.
func newContentDuplicateFacade(t *testing.T) (*Library, video.VideoRepository, string) {
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
	return lib, vr, t.TempDir()
}

// addIdenticalFavorite writes a file of the same bytes and records it as a
// favorite whose fingerprint is already known.
func addIdenticalFavorite(t *testing.T, repo video.VideoRepository, dir, name, digest string, createdAt int64) *video.Video {
	t.Helper()
	path := filepath.Join(dir, name)
	content := make([]byte, 1024)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", name, err)
	}
	v := &video.Video{
		Path:      path,
		FileName:  name,
		FileSize:  info.Size(),
		Status:    video.StatusAvailable,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}
	if err := repo.Create(v); err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	if err := repo.UpdateFingerprint(v.ID, digest, video.FingerprintVersion, info.Size(), info.ModTime().UnixNano(), createdAt); err != nil {
		t.Fatalf("fingerprint %s: %v", name, err)
	}
	return v
}

// The façade exposes the content worklist and its "keep all" dismissal, which is
// what both transports call.
func TestContentDuplicateWorklistThroughFacade(t *testing.T) {
	lib, repo, dir := newContentDuplicateFacade(t)
	addIdenticalFavorite(t, repo, dir, "a.mp4", "digest-one", 1)
	addIdenticalFavorite(t, repo, dir, "b.mp4", "digest-one", 2)
	addIdenticalFavorite(t, repo, dir, "c.mp4", "digest-two", 3)

	groups, err := lib.ListContentDuplicates(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 {
		t.Fatalf("groups = %#v, want one (the two files sharing digest-one)", groups)
	}
	if groups[0].Hash != "digest-one" || len(groups[0].Members) != 2 {
		t.Fatalf("group = %+v, want digest-one with two members", groups[0])
	}
	if groups[0].Members[0].Video.FileName != "b.mp4" {
		t.Errorf("first member = %q, want the newest copy b.mp4", groups[0].Members[0].Video.FileName)
	}
	if groups[0].ReclaimableBytes != 1024 {
		t.Errorf("reclaimable = %d, want one copy's 1024 bytes", groups[0].ReclaimableBytes)
	}

	if err := lib.IgnoreContentDuplicate(groups[0].Hash); err != nil {
		t.Fatal(err)
	}
	visible, err := lib.ListContentDuplicates(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(visible) != 0 {
		t.Fatalf("groups after dismissal = %#v, want none", visible)
	}
	withIgnored, err := lib.ListContentDuplicates(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(withIgnored) != 1 || !withIgnored[0].Ignored {
		t.Fatalf("groups with ignored = %#v, want the dismissed group", withIgnored)
	}
}

// A façade whose content dedup is not wired reports the gap rather than
// pretending the library is duplicate-free.
func TestContentDuplicatesWithoutFingerprintWorker(t *testing.T) {
	lib, _, _ := newContentDuplicateFacade(t)
	if _, err := lib.ScanContentDuplicates(); err == nil {
		t.Fatal("want an error when no fingerprint worker is wired")
	}
	if err := lib.ReverifyContentDuplicate("digest-one"); err == nil {
		t.Fatal("want an error when no fingerprint worker is wired")
	}
	// Reading the worklist needs no worker: an unhashed library simply has none.
	if groups, err := lib.ListContentDuplicates(false); err != nil || len(groups) != 0 {
		t.Fatalf("groups = %#v, err = %v, want an empty worklist", groups, err)
	}
}

// TestTrashSnapshotCarriesRetentionAndGroups verifies the recycle-bin read
// model: the retention travels with the rows, and the groups a favorite
// belonged to are shown so a restore has no surprises.
func TestTrashSnapshotCarriesRetentionAndGroups(t *testing.T) {
	lib, vr, gr := newFacade(t)
	gr.groups = []group.Group{{ID: 1, Name: "收藏夹"}}
	vr.all[0].DeletedAt = 1000
	vr.all[0].DeleteReason = video.TrashReasonUser
	lib.SetTrashRetention(func() int { return 30 })

	snapshot, err := lib.ListTrash()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.RetentionDays != 30 {
		t.Errorf("retention = %d, want 30", snapshot.RetentionDays)
	}
	if len(snapshot.Videos) != 1 || snapshot.Videos[0].ID != 10 {
		t.Fatalf("videos = %+v, want video 10", snapshot.Videos)
	}
	if len(snapshot.Videos[0].TrashGroupNames) != 1 || snapshot.Videos[0].TrashGroupNames[0] != "收藏夹" {
		t.Errorf("group names = %v, want [收藏夹]", snapshot.Videos[0].TrashGroupNames)
	}
}

// TestTrashRetentionClampsInvalidValues keeps a broken preference from silently
// turning into a one-day policy.
func TestTrashRetentionClampsInvalidValues(t *testing.T) {
	lib, _, _ := newFacade(t)
	for _, days := range []int{0, -5, MaxTrashRetentionDays + 1} {
		lib.SetTrashRetention(func() int { return days })
		if got := lib.TrashRetentionDays(); got != DefaultTrashRetentionDays {
			t.Errorf("retention(%d) = %d, want default %d", days, got, DefaultTrashRetentionDays)
		}
	}
	lib.SetTrashRetention(func() int { return 30 })
	if got := lib.TrashRetentionDays(); got != 30 {
		t.Errorf("valid retention = %d, want 30", got)
	}
}

// TestPurgeExpiredTrashOnlyRemovesPastTheRetention pins the boundary and proves
// the purge hard-deletes what it removes.
func TestPurgeExpiredTrashOnlyRemovesPastTheRetention(t *testing.T) {
	lib, vr, gr := newFacade(t)
	const day = int64(24 * 60 * 60 * 1000)
	lib.now = func() int64 { return 100 * day }
	lib.SetTrashRetention(func() int { return 14 })
	vr.all[0].DeletedAt = 87 * day // exactly 13 days old: kept
	vr.all[0].DeleteReason = video.TrashReasonUser
	vr.all[1].DeletedAt = 80 * day // 20 days old: purged
	vr.all[1].DeleteReason = video.TrashReasonMissing

	purged, err := lib.PurgeExpiredTrash()
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if purged != 1 {
		t.Errorf("purged = %d, want 1", purged)
	}
	if _, err := vr.GetAny(20); !errors.Is(err, video.ErrNotFound) {
		t.Errorf("expired row still present: %v", err)
	}
	if _, err := vr.GetAny(10); err != nil {
		t.Errorf("row inside retention was removed: %v", err)
	}
	if len(gr.clearVids) != 1 || gr.clearVids[0] != 20 {
		t.Errorf("cleared videos = %v, want [20]", gr.clearVids)
	}
}

// TestEmptyTrashPurgesEverythingAndReportsCount covers the explicit action.
func TestEmptyTrashPurgesEverythingAndReportsCount(t *testing.T) {
	lib, vr, _ := newFacade(t)
	vr.all[0].DeletedAt = 1000
	vr.all[1].DeletedAt = 2000
	count, err := lib.EmptyTrash()
	if err != nil {
		t.Fatalf("empty: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
	trash, err := lib.ListTrash()
	if err != nil {
		t.Fatal(err)
	}
	if len(trash.Videos) != 0 {
		t.Errorf("trash still has %d rows", len(trash.Videos))
	}
}
