package video

import (
	"os"
	"path/filepath"
	"strings"
)

// This file is the "the file moved" half of favorite identity (收藏 ≠ 文件,
// spec-v1.md §41). It never moves anything: it recognizes that a file the app has
// just been handed holds bytes a favorite already describes under a path that no
// longer exists, and re-points that favorite instead of recording a second one.
//
// Hash identity is what makes the recognition safe. A favorite keeps its
// content_hash after its file disappears, and every duplicate read path filters
// on status = available, so a retained digest is a memory of the bytes rather
// than a live verdict about them. The digest covers a file's size and both ends
// — the same evidence the duplicate panel acts on — so equal digests are the same
// bytes, wherever they now live.

// relinkOnAdd returns the favorite that already describes the file at normalized
// under a path that no longer exists, re-pointed at normalized. A nil result
// means this file is not a moved one and the caller records a new favorite.
//
// The caller has established that normalized is not a recorded path.
func (s *Service) relinkOnAdd(normalized string, info os.FileInfo) *Video {
	candidates, err := s.relinkCandidates(info.Size())
	if err != nil {
		// Best effort: a question the app cannot answer must not block the add.
		s.log.Warn("relink: list candidates failed", "path", normalized, "err", err)
		return nil
	}
	if len(candidates) == 0 {
		return nil
	}

	// The new file is fingerprinted only now. With no favorite of the same size
	// whose file is gone, reading it could not change the outcome, so a healthy
	// library never pays for this.
	facts, err := s.fingerprintFile(normalized)
	if err != nil {
		s.log.Debug("relink: fingerprint of the added file failed", "path", normalized, "err", err)
		return nil
	}
	fileName := filepath.Base(normalized)
	target, matches := pickRelinkTarget(candidates, facts.Digest, fileName)
	if target == nil {
		if matches > 1 {
			s.log.Info("relink: those bytes are already several favorites, recording a new one",
				"path", normalized, "candidates", matches)
		}
		return nil
	}

	now := nowMillis()
	if err := s.repo.RelinkPath(target.ID, normalized, fileName, facts.Digest, FingerprintVersion,
		facts.Size, facts.Mtime, now); err != nil {
		s.log.Warn("relink: re-point favorite failed",
			"id", target.ID, "from", target.Path, "to", normalized, "err", err)
		return nil
	}
	from := target.Path
	applyRelink(target, normalized, fileName, facts, now)
	s.log.Info("favorite relinked to its moved file",
		"id", target.ID, "from", from, "to", target.Path)
	if s.emit != nil {
		s.emit("video:relinked", map[string]any{"id": target.ID, "from": from, "path": target.Path})
	}
	// The bytes are the ones the favorite was recorded with, so its metadata and
	// cover still describe them. Only work that never finished — an import that
	// was interrupted when the file disappeared mid-processing — is picked up.
	if s.needsProcessing(*target) {
		s.processor.Enqueue(target.ID)
	}
	return target
}

// relinkCandidates narrows the same-size favorites down to the ones whose file is
// really gone.
//
// A favorite qualifies when it is already marked missing, or when it is not
// marked missing yet but its file is gone while its volume is still there. The
// second case is the race the folder watch runs into: it imports a file about a
// second after it lands, long before anything re-checks the paths that were left
// behind. A path that is merely unreachable — a drive the user unplugged — is not
// gone and must never be re-pointed.
func (s *Service) relinkCandidates(size int64) ([]*Video, error) {
	rows, err := s.repo.ListRelocationCandidates(size, FingerprintVersion)
	if err != nil {
		return nil, err
	}
	out := make([]*Video, 0, len(rows))
	for i := range rows {
		v := &rows[i]
		// A favorite auto-trashed because its file disappeared is still a
		// relocation candidate: the file may simply have moved. A favorite the
		// user removed by hand while its file was still present is not — adding
		// another copy must not silently undo that decision.
		if v.DeletedAt > 0 && v.DeleteReason != TrashReasonMissing {
			continue
		}
		if v.DeletedAt > 0 || v.Status == StatusMissing || s.pathVanished(v.Path) {
			out = append(out, v)
		}
	}
	return out, nil
}

// pathVanished reports whether a path's file is gone while its volume is still
// mounted: a reference that has gone stale, as opposed to one that is temporarily
// unreachable because the volume holding it is not attached.
func (s *Service) pathVanished(path string) bool {
	if _, err := s.stat(path); err == nil {
		return false
	}
	_, err := s.stat(VolumeRoot(path))
	return err == nil
}

// pickRelinkTarget returns the favorite the file belongs to, given the digest of
// its bytes, and how many candidates held those same bytes.
//
// One match is the answer. When several candidates hold the same bytes — a
// library can honestly hold the same file twice, which is what the duplicate
// panel is for — the candidate recorded under the same file name is the one that
// moved. Anything still ambiguous is a question the app cannot answer, so it
// answers nil and a new favorite is recorded rather than guessing.
func pickRelinkTarget(candidates []*Video, digest, fileName string) (*Video, int) {
	matched := make([]*Video, 0, len(candidates))
	for _, c := range candidates {
		if c.ContentHash == digest {
			matched = append(matched, c)
		}
	}
	if len(matched) == 0 {
		return nil, 0
	}
	if len(matched) == 1 {
		return matched[0], 1
	}
	var named *Video
	for _, c := range matched {
		if !strings.EqualFold(c.FileName, fileName) {
			continue
		}
		if named != nil {
			return nil, len(matched) // two candidates share this file name too
		}
		named = c
	}
	return named, len(matched)
}

// applyRelink mirrors RelinkPath on the in-memory record, so the value handed
// back to the caller describes the row that was just written. full_hash survives:
// it is the full-file confirmation of these very bytes.
func applyRelink(v *Video, path, fileName string, facts FingerprintResult, at int64) {
	v.Path = path
	v.FileName = fileName
	v.Status = StatusAvailable
	v.DeletedAt = 0
	v.DeleteReason = ""
	v.FileSize = facts.Size
	v.ContentHash = facts.Digest
	v.HashVersion = FingerprintVersion
	v.UpdatedAt = at
	v.HashState = FingerprintState{
		Size:       facts.Size,
		Mtime:      facts.Mtime,
		At:         at,
		FullHash:   v.HashState.FullHash,
		FullHashAt: v.HashState.FullHashAt,
	}
}
