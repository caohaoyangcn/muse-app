package video

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// ContentDuplicateMember is one favorite in a content-duplicate group together
// with what checking it against the disk said.
type ContentDuplicateMember struct {
	Video Video `json:"video"`
	// Missing is true when the file is gone. The row can then be removed
	// outright — there is nothing left on disk to compare or to delete.
	Missing bool `json:"missing"`
	// VolumeOffline is true when the file could not be read because its volume
	// is not attached. That is not a missing file: the favorite is kept, and the
	// panel must not offer its removal as if the bytes were gone.
	VolumeOffline bool `json:"volumeOffline"`
	// NeedsReverify is true when the file no longer matches the facts its
	// fingerprint was taken from. Acting on the group while this is set would
	// mean deleting a file on the strength of a verdict about different bytes.
	NeedsReverify bool `json:"needsReverify"`
}

// ContentDuplicateGroup is a set of favorites that hold byte-identical files.
//
// This is a different question from the 番号-based 查重: that one groups the
// different encodes of one work, this one groups the same bytes stored twice
// under another name or on another drive. The same pair can honestly appear in
// both lists.
type ContentDuplicateGroup struct {
	// Hash is the content fingerprint the group was built from. It is also the
	// dismissal key, because the members are exactly the files with this digest.
	Hash string `json:"hash"`
	// Members are the copies, newest first, matching the library's order.
	Members []ContentDuplicateMember `json:"members"`
	// TotalBytes is the space the copies occupy together.
	TotalBytes int64 `json:"totalBytes"`
	// ReclaimableBytes is what keeping one copy would free. It is computed
	// against the largest member, so it never overstates the saving.
	ReclaimableBytes int64 `json:"reclaimableBytes"`
	// Ignored is true when the user chose to keep all of them for exactly this
	// set of members. A change in membership clears the flag.
	Ignored bool `json:"ignored"`
	// Confirmed is true when every member carries a full-file hash and they all
	// agree: the strongest verdict the app can offer before the user deletes
	// anything.
	Confirmed bool `json:"confirmed"`
	// Refuted is true when every member carries a full-file hash and they do NOT
	// all agree — the partial fingerprints matched but the files are not the same
	// bytes. It is the one outcome that invalidates the group, so the panel
	// refuses to act on it.
	Refuted bool `json:"refuted"`
	// Confirming is true while the full-file confirmation of this group is
	// running, so the panel can show progress instead of an idle button.
	Confirming bool `json:"confirming"`
	// NeedsReverify is true when any member needs re-checking (see
	// ContentDuplicateMember).
	NeedsReverify bool `json:"needsReverify"`
	// Missing counts members whose file is gone.
	Missing int `json:"missing"`
}

// IgnoredContentDuplicate is one persisted dismissal: the digest of the group
// plus the member ids it covered when the user dismissed it.
type IgnoredContentDuplicate struct {
	Key      string
	VideoIDs string
}

// confirmState remembers which groups have a full-file confirmation running.
type confirmState struct {
	mu      sync.Mutex
	running map[string]bool
}

// CheckContentDuplicates pairs each fingerprinted favorite with the disk.
//
// The stored fingerprint is only trustworthy while the file is still the file it
// was taken from, and the app cannot know what happened to a file it does not
// own. So every member is checked here, on the read path where the user is about
// to make a decision, rather than trusting a field that may be arbitrarily old.
func CheckContentDuplicates(videos []Video, stat func(string) (os.FileInfo, error)) []ContentDuplicateMember {
	members := make([]ContentDuplicateMember, 0, len(videos))
	for i := range videos {
		v := videos[i]
		member := ContentDuplicateMember{Video: v}
		info, err := stat(v.Path)
		switch {
		case err != nil:
			member.Missing = true
		case !fingerprintDescribes(&v, info.Size(), info.ModTime().UnixNano()):
			member.NeedsReverify = true
		}
		members = append(members, member)
	}
	return members
}

// GroupContentDuplicates builds the worklist: one group per fingerprint shared
// by two or more favorites, ordered by their newest member.
func GroupContentDuplicates(members []ContentDuplicateMember, dismissals map[string]string) []ContentDuplicateGroup {
	byHash := make(map[string][]ContentDuplicateMember)
	order := make([]string, 0, len(members))
	for _, member := range members {
		hash := member.Video.ContentHash
		if hash == "" {
			continue
		}
		if _, seen := byHash[hash]; !seen {
			order = append(order, hash)
		}
		byHash[hash] = append(byHash[hash], member)
	}

	groups := make([]ContentDuplicateGroup, 0, len(order))
	for _, hash := range order {
		copies := byHash[hash]
		if len(copies) < 2 {
			continue // one copy is not a duplicate
		}
		sort.SliceStable(copies, func(i, j int) bool {
			return libraryOrderBefore(copies[i].Video, copies[j].Video)
		})
		group := ContentDuplicateGroup{Hash: hash, Members: copies}

		ids := make([]int64, 0, len(copies))
		largest := int64(0)
		hashed := 0
		agree := true
		fullHash := copies[0].Video.HashState.FullHash
		for _, member := range copies {
			ids = append(ids, member.Video.ID)
			group.TotalBytes += member.Video.FileSize
			if member.Video.FileSize > largest {
				largest = member.Video.FileSize
			}
			if member.NeedsReverify {
				group.NeedsReverify = true
			}
			if member.Missing {
				group.Missing++
			}
			memberFull := member.Video.HashState.FullHash
			if memberFull == "" {
				continue
			}
			hashed++
			if memberFull != fullHash {
				agree = false
			}
		}
		group.ReclaimableBytes = group.TotalBytes - largest
		group.Ignored = dismissals[hash] == contentDuplicateSignature(ids)
		// A confirmation only exists once every member has been read in full.
		// Until then the partial verdict stands and the group is simply not
		// confirmed — the panel says so rather than implying more.
		if hashed == len(copies) {
			group.Confirmed = agree
			// Every file was read end to end and the bytes differ: the sampled
			// regions matched by coincidence, so the group is not a duplicate.
			group.Refuted = !agree
		}
		groups = append(groups, group)
	}

	sort.SliceStable(groups, func(i, j int) bool {
		return libraryOrderBefore(groups[i].Members[0].Video, groups[j].Members[0].Video)
	})
	return groups
}

// contentDuplicateSignature renders a group's member ids as a stable string, so
// a dismissal can be matched against the set it was made for. Same shape as the
// jav 查重 dismissal, because the semantics are the same: a changed membership
// means the old decision no longer applies.
func contentDuplicateSignature(ids []int64) string {
	sorted := append([]int64(nil), ids...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	parts := make([]string, 0, len(sorted))
	for _, id := range sorted {
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	return strings.Join(parts, ",")
}

// libraryOrderBefore reports whether a comes before b in the library's default
// order (newest first, id as the tiebreak).
func libraryOrderBefore(a, b Video) bool {
	if a.CreatedAt != b.CreatedAt {
		return a.CreatedAt > b.CreatedAt
	}
	return a.ID > b.ID
}

// markDetachedMembers reclassifies "gone" as "away" for the members whose volume
// is not attached. An unplugged drive is not a missing file, and presenting it as
// one would invite the user to clean up a favorite whose bytes are safe on a disk
// that is simply not plugged in.
func markDetachedMembers(members []ContentDuplicateMember, offline func(path string) bool) {
	for i := range members {
		if members[i].Missing && offline(members[i].Video.Path) {
			members[i].Missing = false
			members[i].VolumeOffline = true
		}
	}
}

// ListContentDuplicates returns the favorites that hold byte-identical files —
// the content dedup worklist. Groups the user chose to keep whole (保留全部) are
// omitted unless includeIgnored is set, and a group whose membership changed
// counts as new and is always returned.
func (s *Service) ListContentDuplicates(includeIgnored bool) ([]ContentDuplicateGroup, error) {
	videos, err := s.repo.ListFingerprinted()
	if err != nil {
		return nil, err
	}
	// The panel stats every member, so it must also know which of the failures
	// mean "the drive is away" rather than "the file is gone".
	s.volumes.stamp(videos)
	s.volumes.probe()
	members := CheckContentDuplicates(videos, s.stat)
	markDetachedMembers(members, s.volumes.offline)
	// The panel is the moment the user learns a file moved or vanished, so the
	// verdict it just computed is written back: the library grid then agrees
	// with what the panel shows.
	s.syncContentStatuses(members)

	ignores, err := s.repo.ListContentDuplicateIgnores()
	if err != nil {
		return nil, err
	}
	dismissals := make(map[string]string, len(ignores))
	for _, ignored := range ignores {
		dismissals[ignored.Key] = ignored.VideoIDs
	}

	groups := GroupContentDuplicates(members, dismissals)
	// A confirmation in progress is reported on the group, so the panel shows
	// progress instead of offering the action again.
	for i := range groups {
		groups[i].Confirming = s.isConfirming(groups[i].Hash)
	}
	if includeIgnored {
		return groups, nil
	}
	visible := make([]ContentDuplicateGroup, 0, len(groups))
	for _, group := range groups {
		if group.Ignored {
			continue
		}
		visible = append(visible, group)
	}
	return visible, nil
}

// syncContentStatuses persists the file-existence verdict of a worklist read,
// so the library grid and the panel do not disagree. markStatus skips rows whose
// status did not change.
func (s *Service) syncContentStatuses(members []ContentDuplicateMember) {
	for i := range members {
		status := StatusAvailable
		if members[i].Missing {
			status = StatusMissing
		}
		s.markStatus(&members[i].Video, status)
	}
}

// IgnoreContentDuplicate records that the user chose to keep every member of the
// group with this digest (保留全部), so the prompt stays out of the way until the
// membership changes.
func (s *Service) IgnoreContentDuplicate(hash string) error {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return fmt.Errorf("content duplicate hash cannot be empty")
	}
	copies, err := s.repo.ListByContentHash(hash)
	if err != nil {
		return err
	}
	if len(copies) < 2 {
		// The copies the digest described are gone, so there is nothing to
		// dismiss; storing one now would silence a future, unrelated group.
		return s.repo.DeleteContentDuplicateIgnore(hash)
	}
	ids := make([]int64, 0, len(copies))
	for _, v := range copies {
		ids = append(ids, v.ID)
	}
	return s.repo.UpsertContentDuplicateIgnore(hash, contentDuplicateSignature(ids))
}

// ReverifyContentDuplicate re-checks a group's members by re-reading their
// fingerprints. It is how a group whose files changed (or whose drive was
// plugged back in) becomes actionable again.
func (s *Service) ReverifyContentDuplicate(hash string) error {
	if s.fingerprinter == nil {
		return ErrFingerprinterUnavailable
	}
	copies, err := s.repo.ListByContentHash(hash)
	if err != nil {
		return err
	}
	for _, v := range copies {
		s.fingerprinter.Enqueue(v.ID)
	}
	return nil
}

// ConfirmContentDuplicate reads every member of a group in full and stores the
// digest, which turns the sampled verdict into a byte-for-byte one. It returns
// as soon as the work is scheduled; the group reports Confirming while it runs
// and Confirmed (or Refuted, when the files turn out to differ) afterwards.
//
// This is the only path that reads whole files, so it is always user-initiated
// and runs one file at a time.
func (s *Service) ConfirmContentDuplicate(hash string) error {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return fmt.Errorf("content duplicate hash cannot be empty")
	}
	if s.fingerprinter == nil {
		return ErrFingerprinterUnavailable
	}
	copies, err := s.repo.ListByContentHash(hash)
	if err != nil {
		return err
	}
	if len(copies) < 2 {
		// Nothing to confirm: one copy is not a duplicate group.
		return nil
	}
	if !s.beginConfirm(hash) {
		// Already running; the caller can watch the group instead.
		return nil
	}

	ids := make([]int64, 0, len(copies))
	for _, v := range copies {
		ids = append(ids, v.ID)
	}
	go func() {
		defer s.endConfirm(hash)
		done, failed := 0, 0
		s.fingerprinter.ConfirmAll(ids, func(out FullHashOutcome) {
			done++
			if !out.Succeeded() {
				failed++
			}
			if s.emit != nil {
				s.emit("fingerprint:confirm:progress", map[string]any{
					"hash": hash, "done": done, "total": len(ids), "failed": failed,
				})
			}
		})
		s.log.Info("content confirmation finished", "hash", hash, "members", len(ids), "failed", failed)
		if s.emit != nil {
			s.emit("fingerprint:confirm:done", map[string]any{"hash": hash, "failed": failed})
		}
	}()
	return nil
}

// beginConfirm claims the confirmation slot for a group, reporting false when
// one is already running for it.
func (s *Service) beginConfirm(hash string) bool {
	s.confirm.mu.Lock()
	defer s.confirm.mu.Unlock()
	if s.confirm.running == nil {
		s.confirm.running = map[string]bool{}
	}
	if s.confirm.running[hash] {
		return false
	}
	s.confirm.running[hash] = true
	return true
}

// endConfirm releases the confirmation slot.
func (s *Service) endConfirm(hash string) {
	s.confirm.mu.Lock()
	defer s.confirm.mu.Unlock()
	delete(s.confirm.running, hash)
}

// isConfirming reports whether a group's confirmation is running.
func (s *Service) isConfirming(hash string) bool {
	s.confirm.mu.Lock()
	defer s.confirm.mu.Unlock()
	return s.confirm.running[hash]
}
