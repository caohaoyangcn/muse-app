package video

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeVideoAt writes content to dir/name (creating dir) and returns the path.
func writeVideoAt(t *testing.T, dir, name string, content []byte) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// seedMissingFavorite records a favorite for the file at path, gives it the
// fingerprint of the bytes it holds, then takes the file away and re-checks its
// status. That is exactly the state a move leaves in the library: a row whose
// digest still remembers bytes that are no longer at its path.
func seedMissingFavorite(t *testing.T, svc *Service, path string) *Video {
	t.Helper()
	v, err := svc.AddVideo(path)
	if err != nil {
		t.Fatalf("seed add: %v", err)
	}
	res := mustFingerprint(t, path)
	if err := svc.repo.UpdateFingerprint(v.ID, res.Digest, FingerprintVersion, res.Size, res.Mtime, nowMillis()); err != nil {
		t.Fatalf("seed fingerprint: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("seed remove %s: %v", path, err)
	}
	gone, err := svc.RefreshStatus(v.ID)
	if err != nil {
		t.Fatalf("seed refresh: %v", err)
	}
	if gone.Status != StatusMissing {
		t.Fatalf("seed status = %q, want %q", gone.Status, StatusMissing)
	}
	if gone.ContentHash == "" {
		t.Fatal("seed lost the digest: a missing favorite must keep the memory of its bytes")
	}
	return gone
}

func favoriteCount(t *testing.T, svc *Service) int {
	t.Helper()
	list, err := svc.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	return len(list)
}

// TestAddVideoRelinksMovedFile is the feature: the same bytes under a new path
// re-point the favorite that was missing them, instead of becoming a second
// favorite for one file.
func TestAddVideoRelinksMovedFile(t *testing.T) {
	svc, dir := newTestService(t)
	content := pattern(11, 8192)
	old := seedMissingFavorite(t, svc, writeVideoAt(t, filepath.Join(dir, "old"), "movie.mp4", content))
	moved := writeVideoAt(t, filepath.Join(dir, "new"), "movie.mp4", content)

	got, err := svc.AddVideo(moved)
	if err != nil {
		t.Fatalf("add moved file: %v", err)
	}
	if got.ID != old.ID {
		t.Errorf("id = %d, want the missing favorite's %d", got.ID, old.ID)
	}
	if got.Path != moved || got.FileName != "movie.mp4" {
		t.Errorf("path/fileName = %q/%q, want %q/movie.mp4", got.Path, got.FileName, moved)
	}
	if got.Status != StatusAvailable {
		t.Errorf("status = %q, want %q", got.Status, StatusAvailable)
	}
	if got.ContentHash != old.ContentHash {
		t.Errorf("digest = %q, want the matched %q", got.ContentHash, old.ContentHash)
	}
	if got.HashState.Error != "" {
		t.Errorf("hash error = %q, want it cleared", got.HashState.Error)
	}
	// The facts on the row have to describe the file the favorite now points at,
	// or the next verification would distrust a digest that is still correct.
	info, err := os.Stat(moved)
	if err != nil {
		t.Fatalf("stat moved: %v", err)
	}
	if got.HashState.Size != info.Size() || got.HashState.Mtime != info.ModTime().UnixNano() {
		t.Errorf("stored facts = %+v, want size %d mtime %d", got.HashState, info.Size(), info.ModTime().UnixNano())
	}
	if got.FileSize != info.Size() {
		t.Errorf("file size = %d, want %d", got.FileSize, info.Size())
	}
	if n := favoriteCount(t, svc); n != 1 {
		t.Errorf("favorites = %d, want 1: a move must not duplicate the favorite", n)
	}
}

// TestAddVideoKeepsADifferentFileApart pins the other half: a file of the same
// size whose bytes differ is not the missing one and becomes its own favorite.
func TestAddVideoKeepsADifferentFileApart(t *testing.T) {
	svc, dir := newTestService(t)
	old := seedMissingFavorite(t, svc, writeVideoAt(t, filepath.Join(dir, "old"), "movie.mp4", pattern(23, 8192)))
	other := writeVideoAt(t, filepath.Join(dir, "new"), "other.mp4", pattern(24, 8192))

	got, err := svc.AddVideo(other)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if got.ID == old.ID {
		t.Error("different bytes must not re-point the missing favorite")
	}
	if got.ContentHash == old.ContentHash {
		t.Error("test premise broken: the two files must hash differently")
	}
	still, err := svc.Get(old.ID)
	if err != nil {
		t.Fatalf("get missing favorite: %v", err)
	}
	if still.Status != StatusMissing || still.Path != old.Path {
		t.Errorf("missing favorite changed: %+v", still)
	}
	if n := favoriteCount(t, svc); n != 2 {
		t.Errorf("favorites = %d, want 2", n)
	}
}

// TestRelinkKeepsWhatTheUserAttached is why the favorite is re-pointed at all: the
// id is what groups, likes and edits hang off, so recognizing the bytes has to
// preserve every one of them.
func TestRelinkKeepsWhatTheUserAttached(t *testing.T) {
	svc, dir := newTestService(t)
	content := pattern(31, 5000)
	old := seedMissingFavorite(t, svc, writeVideoAt(t, filepath.Join(dir, "old"), "movie.mp4", content))
	if _, err := svc.UpdateInfo(old.ID, "我的标题", "备注"); err != nil {
		t.Fatalf("update info: %v", err)
	}
	if _, err := svc.LikeVideo(old.ID); err != nil {
		t.Fatalf("like: %v", err)
	}

	moved := writeVideoAt(t, filepath.Join(dir, "new"), "movie.mp4", content)
	got, err := svc.AddVideo(moved)
	if err != nil {
		t.Fatalf("add moved file: %v", err)
	}
	if got.Title != "我的标题" || got.Notes != "备注" || got.LikeCount != 1 {
		t.Errorf("user data lost: title %q, notes %q, likes %d", got.Title, got.Notes, got.LikeCount)
	}
}

// TestAddVideoRelinksAFileTheWatchSawFirst covers the race the folder watch
// always runs into: it imports a file about a second after it lands, long before
// anything re-checks the paths that were left behind. The favorite is still
// marked available, but its file is gone and its volume is not.
func TestAddVideoRelinksAFileTheWatchSawFirst(t *testing.T) {
	svc, dir := newTestService(t)
	content := pattern(37, 6000)
	old := writeVideoAt(t, filepath.Join(dir, "a"), "movie.mp4", content)
	v := sampleVideo(old)
	v.FileSize = int64(len(content))
	if err := svc.repo.Create(v); err != nil {
		t.Fatalf("create: %v", err)
	}
	res := mustFingerprint(t, old)
	if err := svc.repo.UpdateFingerprint(v.ID, res.Digest, FingerprintVersion, res.Size, res.Mtime, nowMillis()); err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	if err := os.Remove(old); err != nil {
		t.Fatalf("remove: %v", err)
	}
	moved := writeVideoAt(t, filepath.Join(dir, "b"), "movie.mp4", content)

	got, err := svc.AddVideo(moved)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if got.ID != v.ID {
		t.Errorf("id = %d, want %d: the stale reference has to be re-pointed", got.ID, v.ID)
	}
	if got.Status != StatusAvailable {
		t.Errorf("status = %q, want %q", got.Status, StatusAvailable)
	}
	if n := favoriteCount(t, svc); n != 1 {
		t.Errorf("favorites = %d, want 1", n)
	}
}

// TestAddVideoDoesNotRelinkOffAMissingVolume is the detached-drive guard: a path
// that is unreachable because its volume is not attached is not a file that
// moved, and re-pointing it would silently detach a favorite from a drive that is
// coming back.
func TestAddVideoDoesNotRelinkOffAMissingVolume(t *testing.T) {
	svc, dir := newTestService(t)
	content := pattern(41, 4096)

	// The favorite's bytes are recorded, but its path lives on a volume that is
	// not there: the fake disk reports the volume root as missing too.
	live := writeVideoAt(t, filepath.Join(dir, "gone"), "movie.mp4", content)
	res := mustFingerprint(t, live)
	absent := filepath.Join(dir, "unplugged", "movie.mp4")
	v := sampleVideo(absent)
	v.FileSize = res.Size
	if err := svc.repo.Create(v); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.repo.UpdateFingerprint(v.ID, res.Digest, FingerprintVersion, res.Size, res.Mtime, nowMillis()); err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	if err := os.Remove(live); err != nil {
		t.Fatalf("remove: %v", err)
	}

	reads := 0
	svc.fingerprintFile = func(p string) (FingerprintResult, error) {
		reads++
		return Fingerprint(p)
	}
	volume := filepath.VolumeName(absent) + string(os.PathSeparator)
	svc.stat = func(p string) (os.FileInfo, error) {
		if strings.EqualFold(p, absent) || strings.EqualFold(p, volume) {
			return nil, os.ErrNotExist
		}
		return os.Stat(p)
	}

	incoming := writeVideoAt(t, filepath.Join(dir, "incoming"), "movie.mp4", content)
	got, err := svc.AddVideo(incoming)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if got.ID == v.ID {
		t.Error("a favorite on a volume that is not attached must not be re-pointed")
	}
	if reads != 0 {
		t.Errorf("fingerprinted the added file %d times, want 0: with no candidate there is nothing to compare against", reads)
	}
	if n := favoriteCount(t, svc); n != 2 {
		t.Errorf("favorites = %d, want 2", n)
	}
}

// TestAddVideoCannotRelinkWithoutAFingerprint documents the precondition: the
// memory of the bytes is the digest, so a favorite that never had one cannot be
// recognized. It becomes a normal add instead of a wrong re-point.
func TestAddVideoCannotRelinkWithoutAFingerprint(t *testing.T) {
	svc, dir := newTestService(t)
	content := pattern(43, 4096)
	absent := filepath.Join(dir, "old", "movie.mp4")
	v := sampleVideo(absent)
	v.FileSize = int64(len(content))
	if err := svc.repo.Create(v); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.repo.UpdateStatus(v.ID, StatusMissing); err != nil {
		t.Fatalf("status: %v", err)
	}

	got, err := svc.AddVideo(writeVideoAt(t, filepath.Join(dir, "new"), "movie.mp4", content))
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if got.ID == v.ID {
		t.Error("a favorite without a fingerprint must not be re-pointed")
	}
	if n := favoriteCount(t, svc); n != 2 {
		t.Errorf("favorites = %d, want 2", n)
	}
}

// TestAddVideoHashesNothingOnAHealthyLibrary pins the cost of the feature: with
// no favorite of the same size whose file is gone, adding a file reads no bytes
// beyond what the import already did.
func TestAddVideoHashesNothingOnAHealthyLibrary(t *testing.T) {
	svc, dir := newTestService(t)
	seedMissingFavorite(t, svc, writeVideoAt(t, filepath.Join(dir, "old"), "movie.mp4", pattern(47, 4096)))

	reads := 0
	svc.fingerprintFile = func(p string) (FingerprintResult, error) {
		reads++
		return Fingerprint(p)
	}
	// Same size as the missing favorite would be the one candidate that costs a
	// read, so this files differs in size on purpose: nothing can match it.
	if _, err := svc.AddVideo(writeVideoAt(t, filepath.Join(dir, "new"), "other.mp4", pattern(48, 999))); err != nil {
		t.Fatalf("add: %v", err)
	}
	if reads != 0 {
		t.Errorf("fingerprinted the added file %d times, want 0", reads)
	}
}

// TestAddVideoRevivesAFavoriteWhoseFileCameBack covers the same-path half of a
// returning file: the record already holds that path, so nothing has to be
// re-pointed, only believed again.
func TestAddVideoRevivesAFavoriteWhoseFileCameBack(t *testing.T) {
	svc, dir := newTestService(t)
	content := pattern(53, 4096)
	path := filepath.Join(dir, "movie.mp4")
	old := seedMissingFavorite(t, svc, writeVideoAt(t, dir, "movie.mp4", content))

	writeVideoAt(t, dir, "movie.mp4", content)
	got, err := svc.AddVideo(path)
	if err != nil {
		t.Fatalf("re-add: %v", err)
	}
	if got.ID != old.ID {
		t.Errorf("id = %d, want the recorded favorite %d", got.ID, old.ID)
	}
	if got.Status != StatusAvailable {
		t.Errorf("status = %q, want %q", got.Status, StatusAvailable)
	}
	if n := favoriteCount(t, svc); n != 1 {
		t.Errorf("favorites = %d, want 1", n)
	}
}

// TestAddVideosReportsRelocation keeps the batch caller honest: it has to be able
// to tell a reconnection from an addition, because the UI says different things.
func TestAddVideosReportsRelocation(t *testing.T) {
	svc, dir := newTestService(t)
	content := pattern(59, 7000)
	old := seedMissingFavorite(t, svc, writeVideoAt(t, filepath.Join(dir, "old"), "movie.mp4", content))
	moved := writeVideoAt(t, filepath.Join(dir, "new"), "movie.mp4", content)
	fresh := writeVideoAt(t, filepath.Join(dir, "fresh"), "fresh.mp4", pattern(60, 500))

	results := svc.AddVideos([]string{moved, fresh})
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	if !results[0].Success || !results[0].Relocated || results[0].VideoID != old.ID {
		t.Errorf("moved file result = %+v, want success + relocated + id %d", results[0], old.ID)
	}
	if !results[1].Success || results[1].Relocated {
		t.Errorf("fresh file result = %+v, want success without relocation", results[1])
	}
}

// TestPickRelinkTarget pins the choice when the library holds the same bytes more
// than once: the matching file name decides, and a tie is left to a new favorite
// rather than guessed at.
func TestPickRelinkTarget(t *testing.T) {
	candidate := func(id int64, fileName, digest string) *Video {
		return &Video{ID: id, FileName: fileName, ContentHash: digest}
	}
	cases := []struct {
		name        string
		candidates  []*Video
		digest      string
		fileName    string
		wantID      int64
		wantMatches int
	}{
		{
			name:        "no candidate holds those bytes",
			candidates:  []*Video{candidate(1, "a.mp4", "other")},
			digest:      "mine",
			fileName:    "a.mp4",
			wantID:      0,
			wantMatches: 0,
		},
		{
			name:        "one candidate",
			candidates:  []*Video{candidate(1, "a.mp4", "mine"), candidate(2, "b.mp4", "other")},
			digest:      "mine",
			fileName:    "renamed.mp4",
			wantID:      1,
			wantMatches: 1,
		},
		{
			name:        "several candidates, the name picks one",
			candidates:  []*Video{candidate(1, "other.mp4", "mine"), candidate(2, "a.mp4", "mine")},
			digest:      "mine",
			fileName:    "a.mp4",
			wantID:      2,
			wantMatches: 2,
		},
		{
			name:        "several candidates, none named alike",
			candidates:  []*Video{candidate(1, "x.mp4", "mine"), candidate(2, "y.mp4", "mine")},
			digest:      "mine",
			fileName:    "a.mp4",
			wantID:      0,
			wantMatches: 2,
		},
		{
			name:        "several candidates sharing the name",
			candidates:  []*Video{candidate(1, "a.mp4", "mine"), candidate(2, "a.mp4", "mine")},
			digest:      "mine",
			fileName:    "a.mp4",
			wantID:      0,
			wantMatches: 2,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, matches := pickRelinkTarget(tc.candidates, tc.digest, tc.fileName)
			if matches != tc.wantMatches {
				t.Errorf("matches = %d, want %d", matches, tc.wantMatches)
			}
			if tc.wantID == 0 {
				if got != nil {
					t.Errorf("target = %+v, want none", got)
				}
				return
			}
			if got == nil || got.ID != tc.wantID {
				t.Errorf("target = %+v, want id %d", got, tc.wantID)
			}
		})
	}
}

// TestRepositoryRelinkPath pins what the re-point writes: the new path and its
// facts, the favorite available again, and every user-owned column untouched.
func TestRepositoryRelinkPath(t *testing.T) {
	repo := newTestRepo(t)
	v := sampleVideo(`D:\Videos\a.mp4`)
	if err := repo.Create(v); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.UpdateFingerprint(v.ID, "digest", FingerprintVersion, 111, 222, 333); err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	if err := repo.UpdateFullHash(v.ID, "full", 444); err != nil {
		t.Fatalf("full hash: %v", err)
	}
	if err := repo.UpdateStatus(v.ID, StatusMissing); err != nil {
		t.Fatalf("status: %v", err)
	}
	if err := repo.RelinkPath(v.ID, `E:\Moved\a.mp4`, "a.mp4", "digest", FingerprintVersion, 111, 555, 666); err != nil {
		t.Fatalf("relink: %v", err)
	}

	got, err := repo.Get(v.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Path != `E:\Moved\a.mp4` || got.FileName != "a.mp4" {
		t.Errorf("path/fileName = %q/%q", got.Path, got.FileName)
	}
	if got.Status != StatusAvailable {
		t.Errorf("status = %q, want %q", got.Status, StatusAvailable)
	}
	if got.FileSize != 111 {
		t.Errorf("file size = %d, want 111", got.FileSize)
	}
	if got.HashState.Size != 111 || got.HashState.Mtime != 555 || got.HashState.At != 666 {
		t.Errorf("hash facts = %+v, want size 111 mtime 555 at 666", got.HashState)
	}
	if got.HashState.FullHash != "full" {
		t.Errorf("full hash = %q, want it kept: it confirms these very bytes", got.HashState.FullHash)
	}
}

// TestRepositoryListRelocationCandidates pins the cheap filter: same recorded
// size and a fingerprint from the current recipe, whatever the status says.
func TestRepositoryListRelocationCandidates(t *testing.T) {
	repo := newTestRepo(t)
	sized := func(path string, size int64) *Video {
		v := sampleVideo(path)
		v.FileSize = size
		return v
	}
	sameSize := sized(`D:\a.mp4`, 100)
	staleSize := sized(`D:\b.mp4`, 200)
	staleRecipe := sized(`D:\c.mp4`, 100)
	for _, v := range []*Video{sameSize, staleSize, staleRecipe} {
		if err := repo.Create(v); err != nil {
			t.Fatalf("create %s: %v", v.Path, err)
		}
	}
	if err := repo.UpdateFingerprint(sameSize.ID, "d1", FingerprintVersion, 100, 1, 1); err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	if err := repo.UpdateFingerprint(staleSize.ID, "d2", FingerprintVersion, 200, 1, 1); err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	if err := repo.UpdateFingerprint(staleRecipe.ID, "d3", FingerprintVersion-1, 100, 1, 1); err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	// A favorite without a digest can never be a candidate.
	if err := repo.Create(sized(`D:\d.mp4`, 100)); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.ListRelocationCandidates(100, FingerprintVersion)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].ID != sameSize.ID {
		t.Errorf("candidates = %+v, want only %s", got, sameSize.Path)
	}
}
