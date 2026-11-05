package video

import (
	"os"
	"testing"
	"time"
)

// waitForConfirm blocks until a group's confirmation leaves the running state.
// The slot is released after the last file was read and stored, so it is the
// completion barrier — closing the pool is not one, because a shutdown may
// legitimately drop a confirmation that has not been submitted yet.
func waitForConfirm(t *testing.T, svc *Service, hash string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if !svc.isConfirming(hash) {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("confirmation of %s did not finish within 10s", hash)
}

// TestFullHashCoversTheWholeFile is the reason the tier exists: unlike the
// sampled fingerprint, a change in the middle must show up.
func TestFullHashCoversTheWholeFile(t *testing.T) {
	const middle = 64 << 10
	head, tail := pattern(1, fingerprintRegion), pattern(2, fingerprintRegion)
	a := writeFile(t, "a.mp4", head, pattern(3, middle), tail)
	b := writeFile(t, "b.mp4", head, pattern(4, middle), tail)

	fullA, err := FullHash(a)
	if err != nil {
		t.Fatalf("full hash a: %v", err)
	}
	fullB, err := FullHash(b)
	if err != nil {
		t.Fatalf("full hash b: %v", err)
	}
	if fullA.Digest == fullB.Digest {
		t.Error("a middle-only change must change the full hash")
	}
	// The sampled fingerprint cannot tell these two apart, which is what makes
	// the confirmation worth its cost.
	if mustFingerprint(t, a).Digest != mustFingerprint(t, b).Digest {
		t.Fatal("fixture: the partial fingerprints were expected to match here")
	}
}

// TestFullHashMatchesIdenticalBytes keeps the confirmation agreeing with the
// fingerprint when the files really are the same.
func TestFullHashMatchesIdenticalBytes(t *testing.T) {
	content := pattern(9, 3*fingerprintRegion+1234)
	a := writeFile(t, "a.mp4", content)
	b := writeFile(t, "other-name.mkv", content)

	fullA, err := FullHash(a)
	if err != nil {
		t.Fatalf("full hash a: %v", err)
	}
	fullB, err := FullHash(b)
	if err != nil {
		t.Fatalf("full hash b: %v", err)
	}
	if fullA.Digest != fullB.Digest {
		t.Errorf("identical bytes hashed differently:\n%s\n%s", fullA.Digest, fullB.Digest)
	}
	if len(fullA.Digest) != 64 {
		t.Errorf("digest length = %d, want 64 hex chars", len(fullA.Digest))
	}
	if fullA.Size != int64(len(content)) {
		t.Errorf("size = %d, want %d", fullA.Size, len(content))
	}
}

func TestFullHashReportsUnreadableFiles(t *testing.T) {
	if _, err := FullHash(t.TempDir() + string(os.PathSeparator) + "gone.mp4"); err == nil {
		t.Error("a missing file must fail rather than produce a digest")
	}
	if _, err := FullHash(t.TempDir()); err == nil {
		t.Error("a directory must fail rather than produce a digest")
	}
}

// TestConfirmContentDuplicateConfirmsIdenticalFiles walks the whole tier: a group
// of two identical files becomes Confirmed once both were read in full.
func TestConfirmContentDuplicateConfirmsIdenticalFiles(t *testing.T) {
	svc, repo, _, dir := newContentScanFixture(t)
	addFingerprintTarget(t, repo, writeFakeVideo(t, dir, "a.mp4", 4096))
	addFingerprintTarget(t, repo, writeFakeVideo(t, dir, "b.mp4", 4096))
	startScan(t, svc)

	groups, err := svc.ListContentDuplicates(false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(groups) != 1 || groups[0].Confirmed || groups[0].Refuted {
		t.Fatalf("groups before confirmation = %+v, want one unconfirmed group", groups)
	}

	if err := svc.ConfirmContentDuplicate(groups[0].Hash); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	waitForConfirm(t, svc, groups[0].Hash)

	after, err := svc.ListContentDuplicates(false)
	if err != nil {
		t.Fatalf("list after confirmation: %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("groups = %+v, want the pair", after)
	}
	if !after[0].Confirmed {
		t.Errorf("group = %+v, want it confirmed by the full read", after[0])
	}
	if after[0].Refuted {
		t.Error("identical files must not be refuted")
	}
}

// TestConfirmContentDuplicateRefutesASampledMatch is the failure the tier exists
// to catch: the sampled regions match but the files are not the same bytes.
func TestConfirmContentDuplicateRefutesASampledMatch(t *testing.T) {
	svc, repo, _, _ := newContentScanFixture(t)

	// Two files that agree on size, head and tail, and differ in the middle —
	// exactly what a partial fingerprint cannot tell apart.
	head, tail := pattern(41, fingerprintRegion), pattern(42, fingerprintRegion)
	middle := 64 << 10
	a := writeFile(t, "a.mp4", head, pattern(43, middle), tail)
	b := writeFile(t, "b.mp4", head, pattern(44, middle), tail)
	addFingerprintTarget(t, repo, a)
	addFingerprintTarget(t, repo, b)
	startScan(t, svc)

	groups, err := svc.ListContentDuplicates(false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("groups = %+v, want the sampled match to group them", groups)
	}
	hash := groups[0].Hash

	if err := svc.ConfirmContentDuplicate(hash); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	waitForConfirm(t, svc, hash)

	after, err := svc.ListContentDuplicates(false)
	if err != nil {
		t.Fatalf("list after confirmation: %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("groups = %+v, want the group still reported", after)
	}
	if after[0].Confirmed {
		t.Error("different bytes must never be confirmed")
	}
	if !after[0].Refuted {
		t.Error("a full read that disagrees must refute the group")
	}
}

// TestConfirmContentDuplicateReportsProgressAndCoalesces checks the scheduling
// contract the panel relies on: the call returns immediately, the group reports
// itself as confirming, and a second call does not start a second read.
func TestConfirmContentDuplicateReportsProgressAndCoalesces(t *testing.T) {
	svc, repo, fp, dir := newContentScanFixture(t)
	a := addFingerprintTarget(t, repo, writeFakeVideo(t, dir, "a.mp4", 4096))
	addFingerprintTarget(t, repo, writeFakeVideo(t, dir, "b.mp4", 4096))
	startScan(t, svc)

	groups, err := svc.ListContentDuplicates(false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	hash := groups[0].Hash

	reads := 0
	release := make(chan struct{})
	fp.fullHash = func(path string) (FingerprintResult, error) {
		reads++
		<-release
		return FullHash(path)
	}

	if err := svc.ConfirmContentDuplicate(hash); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if !svc.isConfirming(hash) {
		t.Error("the group must report itself as confirming right away")
	}
	// A second request joins the running confirmation.
	if err := svc.ConfirmContentDuplicate(hash); err != nil {
		t.Fatalf("second confirm: %v", err)
	}
	close(release)
	waitForConfirm(t, svc, hash)

	if reads != 2 {
		t.Errorf("full reads = %d, want 2: the second request must join the first", reads)
	}
	if svc.isConfirming(hash) {
		t.Error("the confirmation slot must be released when the reads are done")
	}
	got, err := repo.Get(a.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.HashState.FullHash == "" {
		t.Error("the full hash must be stored on the member")
	}
}
