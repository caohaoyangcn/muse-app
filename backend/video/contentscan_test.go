package video

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"muse-app/backend/database"
)

// newContentScanFixture wires a Service with a real repository and a fingerprint
// worker, so the scan exercises the same path the app uses. An optional emit
// records the progress events a panel would receive.
func newContentScanFixture(t *testing.T, emit ...func(string, ...any)) (*Service, VideoRepository, *Fingerprinter, string) {
	t.Helper()
	dataDir := t.TempDir()
	db, _, err := database.Open(dataDir)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := NewSQLiteRepository(db)
	processor := NewProcessor(repo, "", "", filepath.Join(dataDir, "thumbnails"), nil, slog.Default(), 1)
	var emitFn func(string, ...any)
	if len(emit) > 0 {
		emitFn = emit[0]
	}
	svc := NewService(repo, processor, dataDir, slog.Default(), emitFn)
	fp := NewFingerprinter(repo, nil, slog.Default(), 2)
	t.Cleanup(fp.Close)
	svc.SetFingerprinter(fp)
	return svc, repo, fp, dataDir
}

// TestContentScanReportsPhaseProgress pins the counters the panel shows while a
// pass runs. On a library where almost every file has a size of its own the size
// phase is the whole pass, so it has to report numbers of its own: the comparing
// phase's total does not exist until it ends.
func TestContentScanReportsPhaseProgress(t *testing.T) {
	var mu sync.Mutex
	var events []ContentScanStatus
	svc, repo, _, dir := newContentScanFixture(t, func(name string, data ...any) {
		if name != "fingerprint:scan:progress" || len(data) != 1 {
			return
		}
		status, ok := data[0].(ContentScanStatus)
		if !ok {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		events = append(events, status)
	})

	// Two files share a size and two are unique, so the pass examines four files
	// and every one of them is unidentified: a scan is what gives a favorite its
	// identity, not only what finds duplicates.
	addFingerprintTarget(t, repo, writeFakeVideo(t, dir, "a.mp4", 4096))
	addFingerprintTarget(t, repo, writeFakeVideo(t, dir, "b.mp4", 4096))
	addFingerprintTarget(t, repo, writeFakeVideo(t, dir, "c.mp4", 1000))
	addFingerprintTarget(t, repo, writeFakeVideo(t, dir, "d.mp4", 2000))
	// A favorite on a detached drive is not examined at all, so it must not be
	// counted in the phase total.
	gone := sampleVideo(filepath.Join(dir, "gone", "offline.mp4"))
	gone.Status = StatusMissing
	if err := repo.Create(gone); err != nil {
		t.Fatalf("create offline favorite: %v", err)
	}

	status := startScan(t, svc)

	mu.Lock()
	got := append([]ContentScanStatus(nil), events...)
	mu.Unlock()
	if len(got) == 0 {
		t.Fatal("no progress events were emitted")
	}
	if got[0].Phase != scanPhaseExamining || got[0].Examined != 0 || got[0].ExamineTotal != 4 {
		t.Errorf("first event = %+v, want the size phase starting over 4 files", got[0])
	}
	var sawExaminedAll, sawComparing bool
	for _, e := range got {
		if e.Examined > e.ExamineTotal {
			t.Errorf("examined %d of %d: the counter ran past its total (%+v)", e.Examined, e.ExamineTotal, e)
		}
		switch e.Phase {
		case scanPhaseExamining:
			if e.Examined == 4 && e.ExamineTotal == 4 {
				sawExaminedAll = true
			}
		case scanPhaseComparing:
			sawComparing = true
			if e.Total != 4 {
				t.Errorf("comparing event = %+v, want 4 files to compare", e)
			}
		}
	}
	if !sawExaminedAll {
		t.Errorf("no event reported the size phase finishing: %+v", got)
	}
	if !sawComparing {
		t.Errorf("no event reported the comparing phase: %+v", got)
	}

	if status.Phase != "" || !status.Running == false {
		t.Errorf("final status = %+v, want a stopped pass with no phase", status)
	}
	if status.Examined != 4 || status.ExamineTotal != 4 {
		t.Errorf("final examined = %d/%d, want 4/4", status.Examined, status.ExamineTotal)
	}
	if status.Total != 4 || status.Done != 4 || status.Hashed != 4 {
		t.Errorf("final comparing = %d done of %d, hashed %d, want 4/4 hashed", status.Done, status.Total, status.Hashed)
	}
	if status.Failed != 0 {
		t.Errorf("failed = %d, want 0", status.Failed)
	}
}

// waitForScan blocks until the pass leaves the running state.
func waitForScan(t *testing.T, svc *Service) ContentScanStatus {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if st := svc.ContentScanStatus(); !st.Running {
			return st
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("content scan did not finish within 10s")
	return ContentScanStatus{}
}

// startScan runs one pass and waits for it.
func startScan(t *testing.T, svc *Service) ContentScanStatus {
	t.Helper()
	if _, err := svc.ScanContentDuplicates(); err != nil {
		t.Fatalf("start scan: %v", err)
	}
	return waitForScan(t, svc)
}

// TestContentScanIdentifiesEveryUnidentifiedFile is what makes move detection
// work across an existing library: a scan gives a digest to every favorite that
// has none, including files whose size is unique. A favorite without one cannot
// be recognized after a rename, so the scan must not leave those behind.
func TestContentScanIdentifiesEveryUnidentifiedFile(t *testing.T) {
	svc, repo, _, dir := newContentScanFixture(t)
	a := addFingerprintTarget(t, repo, writeFakeVideo(t, dir, "a.mp4", 1000))
	b := addFingerprintTarget(t, repo, writeFakeVideo(t, dir, "b.mp4", 2000))

	status := startScan(t, svc)
	if status.Total != 2 || status.Hashed != 2 {
		t.Errorf("status = %+v, want 2 unidentified files hashed", status)
	}
	if status.Failed != 0 {
		t.Errorf("failed = %d, want 0", status.Failed)
	}
	for _, id := range []int64{a.ID, b.ID} {
		got, err := repo.Get(id)
		if err != nil {
			t.Fatalf("get %d: %v", id, err)
		}
		if got.ContentHash == "" || got.HashVersion != FingerprintVersion {
			t.Errorf("video %d was left without a fingerprint: %+v", id, got)
		}
	}
}

// TestContentScanFingerprintsSizeCollisions covers T1, and the digest equality
// the whole feature rests on: identical bytes in two files produce one digest.
func TestContentScanFingerprintsSizeCollisions(t *testing.T) {
	svc, repo, _, dir := newContentScanFixture(t)
	// writeFakeVideo writes zeroes, so two files of one size hold equal bytes.
	a := addFingerprintTarget(t, repo, writeFakeVideo(t, dir, "a.mp4", 4096))
	b := addFingerprintTarget(t, repo, writeFakeVideo(t, dir, "b.mp4", 4096))
	c := addFingerprintTarget(t, repo, writeFakeVideo(t, dir, "c.mp4", 100))

	status := startScan(t, svc)
	if status.Total != 3 || status.Hashed != 3 {
		t.Errorf("status = %+v, want 3 candidates hashed", status)
	}

	gotA, err := repo.Get(a.ID)
	if err != nil {
		t.Fatalf("get a: %v", err)
	}
	gotB, err := repo.Get(b.ID)
	if err != nil {
		t.Fatalf("get b: %v", err)
	}
	gotC, err := repo.Get(c.ID)
	if err != nil {
		t.Fatalf("get c: %v", err)
	}
	if gotA.ContentHash == "" || gotB.ContentHash == "" {
		t.Fatal("the size-sharing files must have been fingerprinted")
	}
	if gotA.ContentHash != gotB.ContentHash {
		t.Errorf("identical bytes produced different digests:\n%s\n%s", gotA.ContentHash, gotB.ContentHash)
	}
	if gotC.ContentHash == "" {
		t.Error("a file of unique size must still gain its identity")
	}
}

// TestContentScanTellsEqualSizesApart is the counterpart: sharing a size is only
// a reason to look, not a verdict.
func TestContentScanTellsEqualSizesApart(t *testing.T) {
	svc, repo, _, dir := newContentScanFixture(t)
	a := addFingerprintTarget(t, repo, writeFakeVideo(t, dir, "a.mp4", 4096))
	other := filepath.Join(dir, "b.mp4")
	if err := os.WriteFile(other, pattern(11, 4096), 0o644); err != nil {
		t.Fatalf("write b: %v", err)
	}
	b := addFingerprintTarget(t, repo, other)

	startScan(t, svc)

	gotA, err := repo.Get(a.ID)
	if err != nil {
		t.Fatalf("get a: %v", err)
	}
	gotB, err := repo.Get(b.ID)
	if err != nil {
		t.Fatalf("get b: %v", err)
	}
	if gotA.ContentHash == "" || gotB.ContentHash == "" {
		t.Fatal("both files must have been fingerprinted")
	}
	if gotA.ContentHash == gotB.ContentHash {
		t.Error("different content of the same size must not share a digest")
	}
}

// TestContentScanRescanSkipsCurrentFingerprints keeps a second pass cheap, which
// is what lets the panel offer "重新扫描" without a warning: fingerprints that
// still describe their files are not candidates at all, so nothing is queued for
// them and no file is opened.
func TestContentScanRescanSkipsCurrentFingerprints(t *testing.T) {
	svc, repo, _, dir := newContentScanFixture(t)
	addFingerprintTarget(t, repo, writeFakeVideo(t, dir, "a.mp4", 4096))
	addFingerprintTarget(t, repo, writeFakeVideo(t, dir, "b.mp4", 4096))

	if status := startScan(t, svc); status.Hashed != 2 {
		t.Fatalf("first pass status = %+v, want 2 hashed", status)
	}
	second := startScan(t, svc)
	if second.Total != 0 || second.Hashed != 0 || second.Done != 0 {
		t.Errorf("second pass status = %+v, want nothing left to do", second)
	}
}

// TestContentScanReportsUnreadableFiles is the honest-reporting rule: a favorite
// whose file cannot be read is counted, not quietly treated as duplicate-free.
func TestContentScanReportsUnreadableFiles(t *testing.T) {
	svc, repo, _, dir := newContentScanFixture(t)
	addFingerprintTarget(t, repo, writeFakeVideo(t, dir, "here.mp4", 4096))

	// A favorite on a drive that is not attached: the row exists, the file does
	// not, and its recorded size would have made it a candidate.
	offline := sampleVideo(filepath.Join(dir, "offline", "gone.mp4"))
	offline.FileSize = 4096
	if err := repo.Create(offline); err != nil {
		t.Fatalf("create offline favorite: %v", err)
	}

	status := startScan(t, svc)
	if status.Failed != 1 {
		t.Errorf("failed = %d, want 1", status.Failed)
	}
	got, err := repo.Get(offline.ID)
	if err != nil {
		t.Fatalf("get offline: %v", err)
	}
	if got.HashState.Error == "" {
		t.Error("the unreadable file must carry its failure reason")
	}
	if got.ContentHash != "" {
		t.Error("the unreadable file must hold no fingerprint")
	}
}

// TestContentScanKeepsTheFingerprintOfADetachedVolume is the data-safety half of
// "away is not gone": unplugging a drive must not erase the digests that identify
// its files, because those digests are how a moved file is recognized when the
// drive comes back.
func TestContentScanKeepsTheFingerprintOfADetachedVolume(t *testing.T) {
	svc, repo, fp, dir := newContentScanFixture(t)
	path := writeFakeVideo(t, dir, "a.mp4", 4096)
	v := addFingerprintTarget(t, repo, path)
	if out := fp.Process(v.ID); out.Status != FingerprintHashed {
		t.Fatalf("seed fingerprint: %v (%s)", out.Status, out.Message)
	}
	seeded, err := repo.Get(v.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	// The drive is unplugged: neither the file nor its volume root answers.
	realStat := svc.stat
	svc.stat = func(p string) (os.FileInfo, error) {
		if strings.EqualFold(p, path) || strings.EqualFold(p, VolumeRoot(path)) {
			return nil, os.ErrNotExist
		}
		return realStat(p)
	}

	status := startScan(t, svc)
	if status.Failed != 0 {
		t.Errorf("failed = %d, want 0: an absent drive is not an unreadable file", status.Failed)
	}
	got, err := repo.Get(v.ID)
	if err != nil {
		t.Fatalf("get after scan: %v", err)
	}
	if got.ContentHash != seeded.ContentHash {
		t.Errorf("digest = %q, want the seeded %q", got.ContentHash, seeded.ContentHash)
	}
	if got.HashState.Error != "" {
		t.Errorf("hash error = %q, want none: the file is away, not unreadable", got.HashState.Error)
	}
}

// TestContentScanIsNotRestartedWhileRunning pins the reuse rule with a worker
// blocked inside the read: a second call must join the running pass instead of
// starting another one.
func TestContentScanIsNotRestartedWhileRunning(t *testing.T) {
	svc, repo, fp, dir := newContentScanFixture(t)
	addFingerprintTarget(t, repo, writeFakeVideo(t, dir, "a.mp4", 4096))
	addFingerprintTarget(t, repo, writeFakeVideo(t, dir, "b.mp4", 4096))

	var mu sync.Mutex
	computed := 0
	release := make(chan struct{})
	fp.compute = func(path string) (FingerprintResult, error) {
		mu.Lock()
		computed++
		mu.Unlock()
		<-release
		return Fingerprint(path)
	}

	if _, err := svc.ScanContentDuplicates(); err != nil {
		t.Fatalf("first scan: %v", err)
	}
	if _, err := svc.ScanContentDuplicates(); err != nil {
		t.Fatalf("second scan: %v", err)
	}
	close(release)
	waitForScan(t, svc)

	mu.Lock()
	defer mu.Unlock()
	if computed != 2 {
		t.Errorf("compute ran %d times, want 2: a second call must reuse the running pass", computed)
	}
}

// TestAddVideoFingerprintsSizePeers covers the lazy pass: a favorite imported
// while its size was unique was pruned by the scan, so the arrival of a copy is
// what gives it a fingerprint.
func TestAddVideoFingerprintsSizePeers(t *testing.T) {
	svc, repo, fp, dir := newContentScanFixture(t)
	// Imported earlier, when nothing else had this size: no fingerprint.
	old := addFingerprintTarget(t, repo, writeFakeVideo(t, dir, "old.mp4", 4096))
	if got, err := repo.Get(old.ID); err != nil {
		t.Fatalf("get: %v", err)
	} else if got.ContentHash != "" {
		t.Fatal("fixture: the peer should start without a fingerprint")
	}

	// Its copy arrives.
	copied := filepath.Join(dir, "copy.mp4")
	if err := os.WriteFile(copied, make([]byte, 4096), 0o644); err != nil {
		t.Fatalf("write copy: %v", err)
	}
	added, err := svc.AddVideo(copied)
	if err != nil {
		t.Fatalf("add copy: %v", err)
	}
	// Close drains what the import scheduled.
	fp.Close()

	gotOld, err := repo.Get(old.ID)
	if err != nil {
		t.Fatalf("get old: %v", err)
	}
	gotNew, err := repo.Get(added.ID)
	if err != nil {
		t.Fatalf("get copy: %v", err)
	}
	if gotOld.ContentHash == "" {
		t.Error("the pruned peer must be fingerprinted once a file of its size arrives")
	}
	if gotNew.ContentHash == "" {
		t.Error("the added file must be fingerprinted")
	}
	if gotOld.ContentHash != gotNew.ContentHash {
		t.Errorf("identical bytes produced different digests:\n%s\n%s", gotOld.ContentHash, gotNew.ContentHash)
	}
}

// TestScanContentDuplicatesWithoutWorker keeps a Service built without content
// dedup wired from panicking, and says so instead.
func TestScanContentDuplicatesWithoutWorker(t *testing.T) {
	svc, _ := newTestService(t)
	if _, err := svc.ScanContentDuplicates(); err != ErrFingerprinterUnavailable {
		t.Errorf("error = %v, want %v", err, ErrFingerprinterUnavailable)
	}
}
