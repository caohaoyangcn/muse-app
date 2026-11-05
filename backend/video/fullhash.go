package video

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// fullHashBufferSize keeps the whole-file read to a handful of syscalls without
// holding a large buffer while several files are read.
const fullHashBufferSize = 256 << 10

// FullHash reads the entire file and returns its SHA-256, together with the
// on-disk facts it was read from.
//
// This is the expensive tier of the content-dedup verdict: a fingerprint reads
// two megabytes, this reads all of a file that is usually several gigabytes. It
// exists to answer the only question a partial hash cannot — whether two files
// whose sampled regions match are in fact identical everywhere — and it is
// therefore only run when the user asks for it, one file at a time.
//
// Like Fingerprint, a file that changes while it is being read is an error
// rather than a result: a confirmation that describes neither version of the
// file is worse than no confirmation, because it is what a user would delete a
// file on.
func FullHash(path string) (FingerprintResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return FingerprintResult{}, fmt.Errorf("open for full hash: %w", err)
	}
	defer func() { _ = f.Close() }()

	before, err := f.Stat()
	if err != nil {
		return FingerprintResult{}, fmt.Errorf("stat for full hash: %w", err)
	}
	if before.IsDir() {
		return FingerprintResult{}, fmt.Errorf("full hash %s: is a directory", path)
	}

	h := sha256.New()
	// Reading through the file's own Reader (rather than the path) keeps the
	// digest tied to the handle whose facts we record.
	if _, err := io.CopyBuffer(h, f, make([]byte, fullHashBufferSize)); err != nil {
		return FingerprintResult{}, fmt.Errorf("full hash %s: %w", path, err)
	}

	after, err := f.Stat()
	if err != nil {
		return FingerprintResult{}, fmt.Errorf("stat after full hash: %w", err)
	}
	if after.Size() != before.Size() || after.ModTime() != before.ModTime() {
		return FingerprintResult{}, fmt.Errorf("full hash %s: file changed while reading", path)
	}

	return FingerprintResult{
		Digest: hex.EncodeToString(h.Sum(nil)),
		Size:   before.Size(),
		Mtime:  before.ModTime().UnixNano(),
	}, nil
}
