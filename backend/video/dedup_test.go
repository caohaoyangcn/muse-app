package video

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeFileInfo lets a test state what the disk would report without one.
type fakeFileInfo struct {
	size  int64
	mtime time.Time
}

func (f fakeFileInfo) Name() string       { return "f.mp4" }
func (f fakeFileInfo) Size() int64        { return f.size }
func (f fakeFileInfo) Mode() os.FileMode  { return 0 }
func (f fakeFileInfo) ModTime() time.Time { return f.mtime }
func (f fakeFileInfo) IsDir() bool        { return false }
func (f fakeFileInfo) Sys() any           { return nil }

func fingerprinted(id int64, hash string, createdAt int64) Video {
	return Video{
		ID:          id,
		Path:        "C:\\v\\f.mp4",
		FileName:    "f.mp4",
		FileSize:    100,
		CreatedAt:   createdAt,
		Status:      StatusAvailable,
		ContentHash: hash,
		HashVersion: FingerprintVersion,
	}
}

func TestGroupContentDuplicatesGroupsSharedDigests(t *testing.T) {
	members := []ContentDuplicateMember{
		{Video: fingerprinted(1, "aaa", 10)},
		{Video: fingerprinted(2, "bbb", 20)},
		{Video: fingerprinted(3, "aaa", 30)},
	}
	groups := GroupContentDuplicates(members, nil)
	if len(groups) != 1 {
		t.Fatalf("groups = %+v, want one (the two files sharing aaa)", groups)
	}
	group := groups[0]
	if group.Hash != "aaa" {
		t.Errorf("hash = %q, want aaa", group.Hash)
	}
	if len(group.Members) != 2 {
		t.Fatalf("members = %d, want 2", len(group.Members))
	}
	// Newest first, matching the library's order.
	if group.Members[0].Video.ID != 3 || group.Members[1].Video.ID != 1 {
		t.Errorf("member order = %d,%d, want 3,1", group.Members[0].Video.ID, group.Members[1].Video.ID)
	}
}

func TestGroupContentDuplicatesSkipsSingleCopies(t *testing.T) {
	groups := GroupContentDuplicates([]ContentDuplicateMember{{Video: fingerprinted(1, "aaa", 10)}}, nil)
	if len(groups) != 0 {
		t.Errorf("groups = %+v, want none: one copy is not a duplicate", groups)
	}
}

func TestGroupContentDuplicatesMatchesDismissalToTheMemberSet(t *testing.T) {
	members := []ContentDuplicateMember{
		{Video: fingerprinted(1, "aaa", 10)},
		{Video: fingerprinted(2, "aaa", 20)},
	}

	matched := GroupContentDuplicates(members, map[string]string{
		"aaa": contentDuplicateSignature([]int64{1, 2}),
	})
	if !matched[0].Ignored {
		t.Error("a dismissal of exactly these members must count")
	}

	// A third copy changes the set, so the old decision no longer applies.
	withThird := append(members, ContentDuplicateMember{Video: fingerprinted(3, "aaa", 30)})
	unmatched := GroupContentDuplicates(withThird, map[string]string{
		"aaa": contentDuplicateSignature([]int64{1, 2}),
	})
	if unmatched[0].Ignored {
		t.Error("a dismissal must expire when the membership changes")
	}
}

func TestGroupContentDuplicatesCountsSpace(t *testing.T) {
	a := fingerprinted(1, "aaa", 10)
	a.FileSize = 100
	b := fingerprinted(2, "aaa", 20)
	b.FileSize = 300 // a stale row: the same digest cannot really have two sizes
	groups := GroupContentDuplicates([]ContentDuplicateMember{{Video: a}, {Video: b}}, nil)
	if groups[0].TotalBytes != 400 {
		t.Errorf("total = %d, want 400", groups[0].TotalBytes)
	}
	// Keeping the largest copy frees the rest, so the figure never overstates
	// what deleting a duplicate would save.
	if groups[0].ReclaimableBytes != 100 {
		t.Errorf("reclaimable = %d, want 100", groups[0].ReclaimableBytes)
	}
}

func TestGroupContentDuplicatesConfirmsOnlyWhenFullHashesAgree(t *testing.T) {
	withFull := func(id int64, full string) ContentDuplicateMember {
		v := fingerprinted(id, "aaa", id)
		v.HashState.FullHash = full
		return ContentDuplicateMember{Video: v}
	}
	agree := GroupContentDuplicates([]ContentDuplicateMember{
		withFull(1, "same"), withFull(2, "same"),
	}, nil)
	if !agree[0].Confirmed {
		t.Error("agreeing full hashes must confirm the group")
	}
	disagree := GroupContentDuplicates([]ContentDuplicateMember{
		withFull(1, "same"), withFull(2, "other"),
	}, nil)
	if disagree[0].Confirmed {
		t.Error("disagreeing full hashes must not confirm the group")
	}
	missing := GroupContentDuplicates([]ContentDuplicateMember{
		withFull(1, "same"), {Video: fingerprinted(2, "aaa", 2)},
	}, nil)
	if missing[0].Confirmed {
		t.Error("a member without a full hash must not confirm the group")
	}
}

func TestCheckContentDuplicatesFlagsMissingAndStaleFiles(t *testing.T) {
	current := fingerprinted(1, "aaa", 10)
	current.HashState = FingerprintState{Size: 100, Mtime: 500}
	stale := fingerprinted(2, "aaa", 20)
	stale.HashState = FingerprintState{Size: 100, Mtime: 500}

	stat := func(path string) (os.FileInfo, error) {
		switch path {
		case "C:\\v\\f.mp4":
			// Current for the first member only: both rows share a path here, so
			// the check has to be driven by the facts.
			return fakeFileInfo{size: 100, mtime: time.Unix(0, 500)}, nil
		default:
			return nil, os.ErrNotExist
		}
	}
	members := CheckContentDuplicates([]Video{current, stale}, stat)
	if members[0].Missing || members[0].NeedsReverify {
		t.Errorf("matching facts must be neither missing nor stale: %+v", members[0])
	}
	if members[1].NeedsReverify {
		t.Errorf("same facts must not be stale: %+v", members[1])
	}

	changed := CheckContentDuplicates([]Video{current}, func(string) (os.FileInfo, error) {
		return fakeFileInfo{size: 100, mtime: time.Unix(0, 501)}, nil
	})
	if !changed[0].NeedsReverify {
		t.Error("a changed mtime must ask for re-verification")
	}

	gone := CheckContentDuplicates([]Video{current}, func(string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	})
	if !gone[0].Missing {
		t.Error("an unstattable file must be reported as missing")
	}
}

// TestMarkDetachedMembersSeparatesAwayFromGone pins the reclassification the
// panel depends on: only a member whose volume is not attached turns from
// "missing" into "away", and it stays removable-as-record but is not presented as
// a lost file.
func TestMarkDetachedMembersSeparatesAwayFromGone(t *testing.T) {
	away := fingerprinted(1, "aaa", 10)
	gone := fingerprinted(2, "aaa", 20)
	gone.Path = `D:\v\f.mp4`
	members := []ContentDuplicateMember{
		{Video: away, Missing: true},
		{Video: gone, Missing: true},
	}

	markDetachedMembers(members, func(path string) bool {
		return strings.EqualFold(path, away.Path)
	})

	if members[0].Missing || !members[0].VolumeOffline {
		t.Errorf("detached member = %+v, want away (offline, not missing)", members[0])
	}
	if !members[1].Missing || members[1].VolumeOffline {
		t.Errorf("gone member = %+v, want missing", members[1])
	}
}

// TestListContentDuplicatesKeepsADetachedVolumeAvailable covers the whole read:
// with the drive unplugged the group is still shown, but no member is called
// missing and the status column is left alone, so plugging the drive back in
// does not require the user to undo a cleanup.
func TestListContentDuplicatesKeepsADetachedVolumeAvailable(t *testing.T) {
	svc, repo, _, dir := newContentScanFixture(t)
	a := addFingerprintTarget(t, repo, writeFakeVideo(t, dir, "a.mp4", 4096))
	b := addFingerprintTarget(t, repo, writeFakeVideo(t, dir, "b.mp4", 4096))
	startScan(t, svc)

	realStat := svc.stat
	svc.stat = func(p string) (os.FileInfo, error) {
		if strings.EqualFold(p, a.Path) || strings.EqualFold(p, b.Path) ||
			strings.EqualFold(p, VolumeRoot(a.Path)) {
			return nil, os.ErrNotExist
		}
		return realStat(p)
	}

	groups, err := svc.ListContentDuplicates(false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(groups) != 1 || groups[0].Missing != 0 {
		t.Fatalf("groups = %+v, want one group with no missing member", groups)
	}
	for _, member := range groups[0].Members {
		if member.Missing || !member.VolumeOffline {
			t.Errorf("member %d = %+v, want away", member.Video.ID, member)
		}
	}
	for _, id := range []int64{a.ID, b.ID} {
		got, err := repo.Get(id)
		if err != nil {
			t.Fatalf("get %d: %v", id, err)
		}
		if got.Status != StatusAvailable {
			t.Errorf("status of %d = %q, want it untouched", id, got.Status)
		}
	}
}

// TestListContentDuplicatesGroupsIdenticalFiles is the whole feature end to end:
// two files with equal bytes are one group, and the digests come from the real
// fingerprint pass.
func TestListContentDuplicatesGroupsIdenticalFiles(t *testing.T) {
	svc, repo, _, dir := newContentScanFixture(t)
	addFingerprintTarget(t, repo, writeFakeVideo(t, dir, "a.mp4", 4096))
	addFingerprintTarget(t, repo, writeFakeVideo(t, dir, "b.mp4", 4096))
	startScan(t, svc)

	groups, err := svc.ListContentDuplicates(false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("groups = %+v, want one", groups)
	}
	group := groups[0]
	if group.Hash == "" || len(group.Members) != 2 {
		t.Fatalf("group = %+v, want two members under a digest", group)
	}
	if group.TotalBytes != 8192 || group.ReclaimableBytes != 4096 {
		t.Errorf("space = %d/%d, want 8192/4096", group.TotalBytes, group.ReclaimableBytes)
	}
	if group.NeedsReverify || group.Missing != 0 || group.Confirmed || group.Ignored {
		t.Errorf("fresh group flags = %+v, want nothing set", group)
	}

	// Different bytes of the same size are not a group.
	other := filepath.Join(dir, "c.mp4")
	if err := os.WriteFile(other, pattern(21, 4096), 0o644); err != nil {
		t.Fatalf("write c: %v", err)
	}
	addFingerprintTarget(t, repo, other)
	if groups, err := svc.ListContentDuplicates(false); err != nil {
		t.Fatalf("list after third file: %v", err)
	} else if len(groups) != 1 {
		t.Errorf("groups = %+v, want the original one only", groups)
	}
}

func TestIgnoreContentDuplicateHidesUntilMembershipChanges(t *testing.T) {
	svc, repo, fp, dir := newContentScanFixture(t)
	addFingerprintTarget(t, repo, writeFakeVideo(t, dir, "a.mp4", 4096))
	addFingerprintTarget(t, repo, writeFakeVideo(t, dir, "b.mp4", 4096))
	startScan(t, svc)

	groups, err := svc.ListContentDuplicates(false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("groups = %+v, want one", groups)
	}
	hash := groups[0].Hash

	if err := svc.IgnoreContentDuplicate(hash); err != nil {
		t.Fatalf("ignore: %v", err)
	}
	visible, err := svc.ListContentDuplicates(false)
	if err != nil {
		t.Fatalf("list after ignore: %v", err)
	}
	if len(visible) != 0 {
		t.Fatalf("groups after ignore = %+v, want none", visible)
	}
	withIgnored, err := svc.ListContentDuplicates(true)
	if err != nil {
		t.Fatalf("list with ignored: %v", err)
	}
	if len(withIgnored) != 1 || !withIgnored[0].Ignored {
		t.Fatalf("ignored groups = %+v, want one marked ignored", withIgnored)
	}

	// A third identical copy changes the membership, so the decision expires.
	third := addFingerprintTarget(t, repo, writeFakeVideo(t, dir, "c.mp4", 4096))
	if out := fp.Process(third.ID); out.Status != FingerprintHashed {
		t.Fatalf("fingerprint third copy: %v", out.Status)
	}
	after, err := svc.ListContentDuplicates(false)
	if err != nil {
		t.Fatalf("list after third copy: %v", err)
	}
	if len(after) != 1 || after[0].Ignored {
		t.Fatalf("groups after a new copy = %+v, want the group back and not ignored", after)
	}
}

func TestIgnoreContentDuplicateDropsDismissalOnceCopiesAreGone(t *testing.T) {
	svc, repo, _, dir := newContentScanFixture(t)
	addFingerprintTarget(t, repo, writeFakeVideo(t, dir, "a.mp4", 4096))
	b := addFingerprintTarget(t, repo, writeFakeVideo(t, dir, "b.mp4", 4096))
	startScan(t, svc)

	groups, err := svc.ListContentDuplicates(false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	hash := groups[0].Hash
	if err := svc.IgnoreContentDuplicate(hash); err != nil {
		t.Fatalf("ignore: %v", err)
	}

	// One copy is removed from the favorites, so the dismissal describes a set
	// that no longer exists.
	if err := repo.Delete(b.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := svc.IgnoreContentDuplicate(hash); err != nil {
		t.Fatalf("ignore with one copy left: %v", err)
	}
	ignores, err := repo.ListContentDuplicateIgnores()
	if err != nil {
		t.Fatalf("list ignores: %v", err)
	}
	if len(ignores) != 0 {
		t.Errorf("ignores = %+v, want the stale dismissal cleared", ignores)
	}
}

func TestListContentDuplicatesFlagsAChangedFile(t *testing.T) {
	svc, repo, _, dir := newContentScanFixture(t)
	a := addFingerprintTarget(t, repo, writeFakeVideo(t, dir, "a.mp4", 4096))
	_ = addFingerprintTarget(t, repo, writeFakeVideo(t, dir, "b.mp4", 4096))
	startScan(t, svc)

	// The file is replaced behind the app's back: the same path, other bytes.
	copyPath := a.Path
	if err := os.WriteFile(copyPath, pattern(31, 8192), 0o644); err != nil {
		t.Fatalf("rewrite: %v", err)
	}

	groups, err := svc.ListContentDuplicates(false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("groups = %+v, want the group to still be reported", groups)
	}
	if !groups[0].NeedsReverify {
		t.Error("the group must ask for re-verification when a member changed")
	}
	found := false
	for _, member := range groups[0].Members {
		if member.Video.ID == a.ID {
			found = member.NeedsReverify
		}
	}
	if !found {
		t.Error("the changed member must be the one flagged")
	}
}

// TestReverifyContentDuplicateMakesAGroupActionableAgain walks the recovery path:
// a member touched outside the app is re-read, and once the fingerprint matches
// again the group needs no re-verification.
func TestReverifyContentDuplicateMakesAGroupActionableAgain(t *testing.T) {
	svc, repo, fp, dir := newContentScanFixture(t)
	addFingerprintTarget(t, repo, writeFakeVideo(t, dir, "a.mp4", 4096))
	b := addFingerprintTarget(t, repo, writeFakeVideo(t, dir, "b.mp4", 4096))
	startScan(t, svc)

	// Touch one copy: the bytes are the same, so the digest will be too, but the
	// stored facts no longer describe the file.
	touched := time.Now().Add(-time.Hour)
	if err := os.Chtimes(b.Path, touched, touched); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	before, err := svc.ListContentDuplicates(false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(before) != 1 || !before[0].NeedsReverify {
		t.Fatalf("groups before reverify = %+v, want one needing re-verification", before)
	}
	hash := before[0].Hash

	if err := svc.ReverifyContentDuplicate(hash); err != nil {
		t.Fatalf("reverify: %v", err)
	}
	fp.Close() // drains the re-read

	after, err := svc.ListContentDuplicates(false)
	if err != nil {
		t.Fatalf("list after reverify: %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("groups after reverify = %+v, want the pair", after)
	}
	if after[0].NeedsReverify {
		t.Error("re-verification must clear the stale flag")
	}
	if after[0].Hash != hash {
		t.Errorf("hash = %q, want the unchanged %q", after[0].Hash, hash)
	}
}

// TestListContentDuplicatesDropsAMissingCopyFromTheWorklist documents the
// consequence of persisting the existence verdict: once a copy is gone the group
// has a single member, and one file is not a duplicate.
func TestListContentDuplicatesDropsAMissingCopyFromTheWorklist(t *testing.T) {
	svc, repo, _, dir := newContentScanFixture(t)
	a := addFingerprintTarget(t, repo, writeFakeVideo(t, dir, "a.mp4", 4096))
	addFingerprintTarget(t, repo, writeFakeVideo(t, dir, "b.mp4", 4096))
	startScan(t, svc)

	if err := os.Remove(a.Path); err != nil {
		t.Fatalf("remove: %v", err)
	}
	first, err := svc.ListContentDuplicates(false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(first) != 1 || first[0].Missing != 1 {
		t.Fatalf("groups = %+v, want one group reporting a missing member", first)
	}
	got, err := repo.Get(a.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != StatusMissing {
		t.Errorf("status = %q, want the missing file recorded", got.Status)
	}

	second, err := svc.ListContentDuplicates(false)
	if err != nil {
		t.Fatalf("second list: %v", err)
	}
	if len(second) != 0 {
		t.Errorf("groups = %+v, want none: one copy left is not a duplicate", second)
	}
}
