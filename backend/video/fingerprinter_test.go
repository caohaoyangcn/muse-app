package video

import (
	"log/slog"
	"os"
	"testing"
)

// newFingerprintFixture wires a real repository with a fingerprint worker whose
// pool the test closes.
func newFingerprintFixture(t *testing.T, workers int) (VideoRepository, *Fingerprinter) {
	t.Helper()
	repo := newTestRepo(t)
	fp := NewFingerprinter(repo, nil, slog.Default(), workers)
	t.Cleanup(fp.Close)
	return repo, fp
}

// addFingerprintTarget records a favorite for a real file, with the size the
// disk reports so the size pass sees a plausible row.
func addFingerprintTarget(t *testing.T, repo VideoRepository, path string) *Video {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	v := sampleVideo(path)
	v.FileSize = info.Size()
	if err := repo.Create(v); err != nil {
		t.Fatalf("create favorite for %s: %v", path, err)
	}
	return v
}

func TestFingerprinterStoresFingerprint(t *testing.T) {
	repo, fp := newFingerprintFixture(t, 1)
	path := writeFile(t, "a.mp4", pattern(3, 4096))
	v := addFingerprintTarget(t, repo, path)

	out := fp.Process(v.ID)
	if out.Status != FingerprintHashed {
		t.Fatalf("status = %v (message %q), want hashed", out.Status, out.Message)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	got, err := repo.Get(v.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ContentHash != out.Digest || got.ContentHash == "" {
		t.Errorf("stored digest = %q, want %q", got.ContentHash, out.Digest)
	}
	if got.HashVersion != FingerprintVersion {
		t.Errorf("hash version = %d, want %d", got.HashVersion, FingerprintVersion)
	}
	if got.HashState.Size != info.Size() || got.HashState.Mtime != info.ModTime().UnixNano() {
		t.Errorf("stored facts = %+v, want size %d mtime %d", got.HashState, info.Size(), info.ModTime().UnixNano())
	}
	if got.HashState.Error != "" {
		t.Errorf("hash error = %q, want empty", got.HashState.Error)
	}
	// Hashing repairs the recorded size from the fact it hashed.
	if got.FileSize != info.Size() {
		t.Errorf("file size = %d, want %d", got.FileSize, info.Size())
	}
}

// TestFingerprinterSkipsFileItAlreadyDescribes is what makes re-enqueuing a file
// free: a stored fingerprint that still matches the disk is not recomputed.
func TestFingerprinterSkipsFileItAlreadyDescribes(t *testing.T) {
	repo, fp := newFingerprintFixture(t, 1)
	path := writeFile(t, "a.mp4", pattern(4, 4096))
	v := addFingerprintTarget(t, repo, path)
	if out := fp.Process(v.ID); out.Status != FingerprintHashed {
		t.Fatalf("first pass status = %v, want hashed", out.Status)
	}

	reads := 0
	fp.compute = func(p string) (FingerprintResult, error) {
		reads++
		return Fingerprint(p)
	}
	out := fp.Process(v.ID)
	if out.Status != FingerprintSkipped {
		t.Errorf("status = %v, want skipped", out.Status)
	}
	if reads != 0 {
		t.Errorf("compute ran %d times, want 0: a current fingerprint must not be recomputed", reads)
	}
}

func TestFingerprinterRecomputesWhenTheFileChanges(t *testing.T) {
	repo, fp := newFingerprintFixture(t, 1)
	path := writeFile(t, "a.mp4", pattern(5, 4096))
	v := addFingerprintTarget(t, repo, path)
	first := fp.Process(v.ID)
	if first.Status != FingerprintHashed {
		t.Fatalf("first pass status = %v, want hashed", first.Status)
	}

	if err := os.WriteFile(path, pattern(6, 8192), 0o644); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	second := fp.Process(v.ID)
	if second.Status != FingerprintHashed {
		t.Fatalf("status after rewrite = %v, want hashed", second.Status)
	}
	if second.Digest == first.Digest {
		t.Error("rewritten content must not keep the old digest")
	}
	got, err := repo.Get(v.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.FileSize != 8192 {
		t.Errorf("file size = %d, want the rewritten 8192", got.FileSize)
	}
}

// TestFingerprinterClearsFingerprintOfAFileThatDisappears is the safety
// property: a file nobody can read must not keep a verdict that could put it in
// a duplicate group.
func TestFingerprinterClearsFingerprintOfAFileThatDisappears(t *testing.T) {
	repo, fp := newFingerprintFixture(t, 1)
	path := writeFile(t, "a.mp4", pattern(7, 4096))
	v := addFingerprintTarget(t, repo, path)
	if out := fp.Process(v.ID); out.Status != FingerprintHashed {
		t.Fatalf("status = %v, want hashed", out.Status)
	}

	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}
	out := fp.Process(v.ID)
	if out.Status != FingerprintFailed {
		t.Fatalf("status for a deleted file = %v, want failed", out.Status)
	}
	if out.Message == "" {
		t.Error("a failure must carry its reason")
	}
	got, err := repo.Get(v.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ContentHash != "" {
		t.Errorf("digest = %q, want it cleared", got.ContentHash)
	}
	if got.HashState.Error == "" {
		t.Error("the failure must be recorded on the row")
	}
}

// TestFingerprinterKeepsTheDigestOfADetachedVolume is the other half of that
// rule: a drive that is unplugged is not a file that disappeared, and its digest
// is the memory that lets the library recognize these bytes when the drive comes
// back. Only the unreadable file's verdict is withdrawn.
func TestFingerprinterKeepsTheDigestOfADetachedVolume(t *testing.T) {
	repo, fp := newFingerprintFixture(t, 1)
	path := writeFile(t, "a.mp4", pattern(17, 4096))
	v := addFingerprintTarget(t, repo, path)
	first := fp.Process(v.ID)
	if first.Status != FingerprintHashed {
		t.Fatalf("status = %v, want hashed", first.Status)
	}

	fp.stat = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
	fp.volumeOffline = func(string) bool { return true }

	out := fp.Process(v.ID)
	if out.Status != FingerprintFailed {
		t.Errorf("status = %v, want failed", out.Status)
	}
	got, err := repo.Get(v.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ContentHash != first.Digest {
		t.Errorf("digest = %q, want the kept %q", got.ContentHash, first.Digest)
	}
	if got.HashState.Error != "" {
		t.Errorf("hash error = %q, want none: the drive is away, not the file", got.HashState.Error)
	}
}

func TestFingerprinterQueueDrainsBeforeClose(t *testing.T) {
	repo, fp := newFingerprintFixture(t, 1)
	path := writeFile(t, "a.mp4", pattern(8, 4096))
	v := addFingerprintTarget(t, repo, path)

	fp.Enqueue(v.ID)
	fp.Close()

	got, err := repo.Get(v.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ContentHash == "" {
		t.Error("Close must drain the jobs already queued")
	}
}

func TestFingerprinterEnqueueAfterCloseIsIgnored(t *testing.T) {
	repo, fp := newFingerprintFixture(t, 1)
	path := writeFile(t, "a.mp4", pattern(9, 4096))
	v := addFingerprintTarget(t, repo, path)

	fp.Close()
	// Must neither panic on the closed channel nor queue anything.
	fp.Enqueue(v.ID)

	got, err := repo.Get(v.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ContentHash != "" {
		t.Error("a job enqueued after Close must not run")
	}
}

// TestFingerprintDescribesRequiresEveryFact pins the staleness rule: a digest is
// only current while the recipe and both on-disk facts still agree.
func TestFingerprintDescribesRequiresEveryFact(t *testing.T) {
	current := &Video{
		ContentHash: "abc",
		HashVersion: FingerprintVersion,
		HashState:   FingerprintState{Size: 10, Mtime: 20},
	}
	if !fingerprintDescribes(current, 10, 20) {
		t.Error("matching facts must count as current")
	}
	cases := []struct {
		name   string
		mutate func(*Video)
		size   int64
		mtime  int64
	}{
		{"no digest", func(v *Video) { v.ContentHash = "" }, 10, 20},
		{"older recipe", func(v *Video) { v.HashVersion = FingerprintVersion - 1 }, 10, 20},
		{"grown file", func(*Video) {}, 11, 20},
		{"rewritten file", func(*Video) {}, 10, 21},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := *current
			tc.mutate(&v)
			if fingerprintDescribes(&v, tc.size, tc.mtime) {
				t.Error("stale facts must not count as current")
			}
		})
	}
}

// TestFingerprintOutcomeOfAnUnknownVideo keeps an id removed while queued from
// being reported as a successfully hashed file.
func TestFingerprintOutcomeOfAnUnknownVideo(t *testing.T) {
	_, fp := newFingerprintFixture(t, 1)
	out := fp.Process(4242)
	if out.Status != FingerprintFailed {
		t.Errorf("status = %v, want failed", out.Status)
	}
	if out.ID != 4242 {
		t.Errorf("id = %d, want 4242", out.ID)
	}
}
