package video

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// pattern builds n deterministic bytes from seed, so a test can give two files
// a byte-identical head or tail while they differ elsewhere.
func pattern(seed byte, n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = seed + byte(i%251)
	}
	return b
}

// writeFile writes the concatenation of parts and returns its path.
func writeFile(t *testing.T, name string, parts ...[]byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, bytes.Join(parts, nil), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func mustFingerprint(t *testing.T, path string) FingerprintResult {
	t.Helper()
	res, err := Fingerprint(path)
	if err != nil {
		t.Fatalf("fingerprint %s: %v", path, err)
	}
	return res
}

// TestFingerprintIdenticalContentIgnoresPath is the core promise: the same
// bytes under a different name in a different directory are the same file as
// far as dedup is concerned.
func TestFingerprintIdenticalContentIgnoresPath(t *testing.T) {
	content := pattern(7, 3*fingerprintRegion)
	a := writeFile(t, "a.mp4", content)
	b := writeFile(t, "another-name.mkv", content)

	fa, fb := mustFingerprint(t, a), mustFingerprint(t, b)
	if fa.Digest != fb.Digest {
		t.Errorf("identical content hashed differently:\n%s\n%s", fa.Digest, fb.Digest)
	}
	if len(fa.Digest) != 64 {
		t.Errorf("digest length = %d, want 64 hex chars", len(fa.Digest))
	}
}

// TestFingerprintSizeIsPartOfTheDigest covers the case a head+tail-only recipe
// would get wrong: same head, same tail, different length.
func TestFingerprintSizeIsPartOfTheDigest(t *testing.T) {
	head, tail := pattern(1, fingerprintRegion), pattern(2, fingerprintRegion)
	a := writeFile(t, "a.mp4", head, pattern(3, 64<<10), tail)
	b := writeFile(t, "b.mp4", head, pattern(4, 192<<10), tail)

	if mustFingerprint(t, a).Digest == mustFingerprint(t, b).Digest {
		t.Error("files sharing a head and a tail but differing in size must not share a digest")
	}
}

// TestFingerprintReadsBothEnds pins the sampling rule in both directions. The
// tail case is the one a head-only implementation would silently pass: an MKV's
// index lives at the end, so ignoring it would correlate files that differ
// exactly where their structure is described.
func TestFingerprintReadsBothEnds(t *testing.T) {
	const middle = 64 << 10
	head, tail := pattern(10, fingerprintRegion), pattern(20, fingerprintRegion)
	base := writeFile(t, "base.mp4", head, pattern(30, middle), tail)

	changedHead := append([]byte(nil), head...)
	changedHead[0] ^= 0xff
	changedTail := append([]byte(nil), tail...)
	changedTail[len(changedTail)-1] ^= 0xff

	headFile := writeFile(t, "head.mp4", changedHead, pattern(30, middle), tail)
	tailFile := writeFile(t, "tail.mp4", head, pattern(30, middle), changedTail)

	baseDigest := mustFingerprint(t, base).Digest
	if baseDigest == mustFingerprint(t, headFile).Digest {
		t.Error("a different first byte must change the digest")
	}
	if baseDigest == mustFingerprint(t, tailFile).Digest {
		t.Error("a different last byte must change the digest")
	}
}

// TestFingerprintIgnoresTheMiddle documents the deliberate compromise: bytes
// between the two regions are not read, so a change there alone is invisible.
// That is what makes the pass cheap, and it is why a group can be confirmed
// with a full hash before the user acts on it.
func TestFingerprintIgnoresTheMiddle(t *testing.T) {
	const middle = 64 << 10
	head, tail := pattern(40, fingerprintRegion), pattern(50, fingerprintRegion)
	a := writeFile(t, "a.mp4", head, pattern(60, middle), tail)
	b := writeFile(t, "b.mp4", head, pattern(61, middle), tail)

	if mustFingerprint(t, a).Digest != mustFingerprint(t, b).Digest {
		t.Error("a middle-only change must not change the sampled digest")
	}
}

// TestFingerprintShortFilesAreReadWhole covers the size-threshold branch: the
// two regions overlap, so the file has to be hashed once rather than twice over
// shared bytes.
func TestFingerprintShortFilesAreReadWhole(t *testing.T) {
	cases := []struct {
		name string
		size int
	}{
		{"empty", 0},
		{"one-byte", 1},
		{"below-region", fingerprintRegion - 1},
		{"exactly-region", fingerprintRegion},
		{"between-regions", fingerprintRegion + 1},
		{"exactly-two-regions", 2 * fingerprintRegion},
		{"above-two-regions", 2*fingerprintRegion + 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeFile(t, "f.mp4", pattern(70, tc.size))
			res := mustFingerprint(t, path)
			if res.Size != int64(tc.size) {
				t.Errorf("size = %d, want %d", res.Size, tc.size)
			}
			if len(res.Digest) != 64 {
				t.Errorf("digest length = %d, want 64", len(res.Digest))
			}
		})
	}
}

// TestFingerprintReportsOnDiskFacts checks that the facts returned describe the
// file itself, since those are what a later verification compares against.
func TestFingerprintReportsOnDiskFacts(t *testing.T) {
	path := writeFile(t, "a.mp4", pattern(80, 4096))
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	res := mustFingerprint(t, path)
	if res.Size != info.Size() {
		t.Errorf("size = %d, want %d", res.Size, info.Size())
	}
	if res.Mtime != info.ModTime().UnixNano() {
		t.Errorf("mtime = %d, want %d", res.Mtime, info.ModTime().UnixNano())
	}
}

// TestFingerprintMissingFile keeps a detached drive or a deleted file from
// looking like a successfully hashed one.
func TestFingerprintMissingFile(t *testing.T) {
	if _, err := Fingerprint(filepath.Join(t.TempDir(), "gone.mp4")); err == nil {
		t.Error("a missing file must fail rather than produce a digest")
	}
}

// TestFingerprintRejectsDirectories keeps the worker from recording a digest
// for a path that is not a file at all.
func TestFingerprintRejectsDirectories(t *testing.T) {
	if _, err := Fingerprint(t.TempDir()); err == nil {
		t.Error("a directory must fail rather than produce a digest")
	}
}

// TestFingerprintChangesWhenContentIsRewritten is the staleness case: the same
// path with different bytes must not keep the old digest.
func TestFingerprintChangesWhenContentIsRewritten(t *testing.T) {
	path := writeFile(t, "a.mp4", pattern(90, fingerprintRegion+1024))
	before := mustFingerprint(t, path)

	if err := os.WriteFile(path, pattern(91, fingerprintRegion+1024), 0o644); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	after := mustFingerprint(t, path)
	if before.Digest == after.Digest {
		t.Error("rewritten content must change the digest")
	}
}
