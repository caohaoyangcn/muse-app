package video

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
)

// FingerprintVersion is the recipe behind a fingerprint. Bump it whenever the
// digest input changes — the region size, an added region, the tag — because
// rows carrying an older version are recomputed by the next scan.
const FingerprintVersion = 1

// fingerprintRegion is how many bytes are sampled from each end of the file.
// The head carries the container header and the first frames; the tail carries
// what is written after them, which is the MKV index (Cues) or an MP4 whose
// moov atom was not moved to the front. Reading both ends costs two seeks
// instead of a full multi-gigabyte read, and a megabyte of each is far more
// than needed to tell two different works apart.
const fingerprintRegion = 1 << 20

// fingerprintTag salts the digest, so a later recipe cannot produce a value
// that happens to equal one from this recipe.
const fingerprintTag = "muse-fp1"

// fingerprintBufferSize keeps each sampled region to a handful of reads
// instead of one read per 32 KiB.
const fingerprintBufferSize = 256 << 10

// FingerprintResult is what one fingerprint pass observed together with the
// digest it produced.
//
// Digest and the on-disk facts come from the same open handle, so they can
// never disagree about which version of the file they describe — which is the
// point: these facts decide whether the digest may still be trusted.
type FingerprintResult struct {
	// Digest is the hex SHA-256 over size, head and tail.
	Digest string
	// Size is the file size the digest was computed from.
	Size int64
	// Mtime is the file modification time (unix nanoseconds) at that moment.
	Mtime int64
}

// Fingerprint computes the content fingerprint of the file at path: a digest
// over its size plus its first and last fingerprintRegion bytes.
//
// The size is part of the digest input, so two files that happen to share a
// head and a tail but differ in length cannot collide. For a file no longer
// than the two regions the whole file is read once, which is also why the
// digest stays continuous at exactly two regions: there the head and the tail
// are the whole file read in the same order either way.
//
// A file that changes while it is being read is reported as an error rather
// than fingerprinted: recording facts that describe neither version would let a
// later verification trust a digest over bytes that are no longer there.
func Fingerprint(path string) (FingerprintResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return FingerprintResult{}, fmt.Errorf("open for fingerprint: %w", err)
	}
	defer func() { _ = f.Close() }()

	before, err := f.Stat()
	if err != nil {
		return FingerprintResult{}, fmt.Errorf("stat for fingerprint: %w", err)
	}
	if before.IsDir() {
		return FingerprintResult{}, fmt.Errorf("fingerprint %s: is a directory", path)
	}

	h := sha256.New()
	if _, err := io.WriteString(h, fingerprintTag); err != nil {
		return FingerprintResult{}, fmt.Errorf("fingerprint %s: %w", path, err)
	}
	var sizeBuf [8]byte
	binary.LittleEndian.PutUint64(sizeBuf[:], uint64(before.Size()))
	if _, err := h.Write(sizeBuf[:]); err != nil {
		return FingerprintResult{}, fmt.Errorf("fingerprint %s: %w", path, err)
	}
	if err := hashRegions(h, f, before.Size()); err != nil {
		return FingerprintResult{}, fmt.Errorf("fingerprint %s: %w", path, err)
	}

	after, err := f.Stat()
	if err != nil {
		return FingerprintResult{}, fmt.Errorf("stat after fingerprint: %w", err)
	}
	if after.Size() != before.Size() || after.ModTime() != before.ModTime() {
		return FingerprintResult{}, fmt.Errorf("fingerprint %s: file changed while reading", path)
	}

	return FingerprintResult{
		Digest: hex.EncodeToString(h.Sum(nil)),
		Size:   before.Size(),
		Mtime:  before.ModTime().UnixNano(),
	}, nil
}

// hashRegions writes the sampled regions of f into h.
func hashRegions(h hash.Hash, f *os.File, size int64) error {
	buf := make([]byte, fingerprintBufferSize)
	// Short file: one pass over everything, so a clip smaller than the two
	// regions is not hashed twice over bytes the regions would share.
	if size <= 2*fingerprintRegion {
		if _, err := io.CopyBuffer(h, f, buf); err != nil {
			return err
		}
		return nil
	}
	if _, err := io.CopyBuffer(h, io.LimitReader(f, fingerprintRegion), buf); err != nil {
		return err
	}
	if _, err := f.Seek(-fingerprintRegion, io.SeekEnd); err != nil {
		return err
	}
	_, err := io.CopyBuffer(h, io.LimitReader(f, fingerprintRegion), buf)
	return err
}
