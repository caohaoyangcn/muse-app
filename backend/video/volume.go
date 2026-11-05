package video

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// A favorite references a file, and a reference can stop resolving in two
// different ways (spec-v1.md §20, §41):
//
//   - the file is gone while its volume is still attached — a deleted or moved
//     file. The status column records that ("missing"), and the favorite stays
//     out of reach until the file comes back.
//   - the volume itself is not attached — an external drive the user unplugged.
//     The file is not lost, it is on a disk that is not plugged in. Calling that
//     "missing" would be wrong, and after one startup refresh it would be
//     indistinguishable from a deleted file.
//
// This file answers the second question. Volume presence is a property of the
// machine and not of the favorite, so it is never persisted: it is observed,
// cached here and stamped onto the read model (Video.VolumeOffline). Reads never
// touch the disk — the cache is refreshed by a background poll and by explicit
// probes — so an unresponsive network share can never stall the library grid.
//
// See docs/spec/volume-offline.md.

// volumePollEvery is how often WatchVolumes re-checks the volumes the library
// references: fast enough that plugging a drive back in clears the warning
// without a restart, slow enough that the polls are free.
const volumePollEvery = 5 * time.Second

// VolumeChange is one volume as a probe just observed it.
type VolumeChange struct {
	// Root is the mount root: "D:\", "\\server\share\" or "/".
	Root string `json:"root"`
	// Online reports whether the volume is attached.
	Online bool `json:"online"`
	// Changed reports that this observation differs from the previous one. The
	// first observation of a volume is not a change: there was nothing to
	// compare it against, and startup reports the state through the read model.
	Changed bool `json:"changed"`
}

// VolumeRoot returns the mount point path lives on: the drive or UNC share on
// Windows ("D:\", "\\server\share\"), the filesystem root elsewhere.
//
// Recorded paths are absolute (NormalizeAndValidate), so a path that names no
// volume of its own lives on the root — which is what makes the volume test mean
// the same thing on every platform.
func VolumeRoot(path string) string {
	volume := filepath.VolumeName(path)
	if volume == "" {
		return string(os.PathSeparator)
	}
	return volume + string(os.PathSeparator)
}

// volumeKey is the canonical map key for a mount root. Volume names are
// case-insensitive on the platforms that have them.
func volumeKey(root string) string {
	return strings.ToLower(root)
}

// volumeState caches the presence of every volume the library references.
//
// It is the "is the disk plugged in?" half of a reference, kept apart from the
// status column on purpose: unplugging a drive must not rewrite the library.
type volumeState struct {
	// stat is the disk question, injected so tests can describe a machine
	// without one.
	stat func(string) (os.FileInfo, error)

	// probeMu serializes probes. Many callers may ask for a refresh, but the
	// disk is asked once, and readers never take this lock.
	probeMu sync.Mutex

	mu      sync.Mutex
	roots   map[string]string // key -> root as first recorded
	mounted map[string]bool   // key -> last observation; absent until first probe
}

// newVolumeState builds the cache over a disk probe.
func newVolumeState(stat func(string) (os.FileInfo, error)) *volumeState {
	return &volumeState{
		stat:    stat,
		roots:   map[string]string{},
		mounted: map[string]bool{},
	}
}

// stamp fills VolumeRoot and VolumeOffline on videos and remembers the volumes
// they reference, so the next probe covers them.
//
// A volume that has not been probed yet is reported as attached: a warning must
// never be invented, and the probe that follows a startup corrects this.
func (v *volumeState) stamp(videos []Video) {
	v.mu.Lock()
	defer v.mu.Unlock()
	for i := range videos {
		v.stampLocked(&videos[i])
	}
}

// stampVideo is stamp for one record (Get and the single-record reads).
func (v *volumeState) stampVideo(video *Video) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.stampLocked(video)
}

func (v *volumeState) stampLocked(video *Video) {
	if video.Path == "" || video.SourceID > 0 {
		// A remote favorite has no local volume. Its synthetic cloud:// path
		// must never be probed as if it were a disk.
		video.VolumeRoot = ""
		video.VolumeOffline = false
		return
	}
	root := VolumeRoot(video.Path)
	key := volumeKey(root)
	if _, ok := v.roots[key]; !ok {
		v.roots[key] = root
	}
	video.VolumeRoot = root
	online, known := v.mounted[key]
	video.VolumeOffline = known && !online
}

// offline reports whether the volume holding path was observed as detached. An
// unprobed volume reports false, for the reason stamp does.
func (v *volumeState) offline(path string) bool {
	if path == "" {
		return false
	}
	key := volumeKey(VolumeRoot(path))
	v.mu.Lock()
	defer v.mu.Unlock()
	online, known := v.mounted[key]
	return known && !online
}

// rootOf returns the recorded root for path, or "" when the path names none.
func (v *volumeState) rootOf(path string) string {
	if path == "" {
		return ""
	}
	return VolumeRoot(path)
}

// record stores one volume's presence observed outside a probe (a single-file
// operation that just asked the disk), so what one card learned is what every
// card shows.
func (v *volumeState) record(path string, online bool) {
	if path == "" {
		return
	}
	root := VolumeRoot(path)
	key := volumeKey(root)
	v.mu.Lock()
	defer v.mu.Unlock()
	if _, ok := v.roots[key]; !ok {
		v.roots[key] = root
	}
	v.mounted[key] = online
}

// probe asks the disk about every volume the library references and reports
// what it saw. It is the only place a volume is probed.
//
// The disk is asked outside the state lock: a probe may wait on an unresponsive
// network share, and stamping a list must not wait with it.
func (v *volumeState) probe() []VolumeChange {
	v.probeMu.Lock()
	defer v.probeMu.Unlock()

	v.mu.Lock()
	roots := make(map[string]string, len(v.roots))
	for key, root := range v.roots {
		roots[key] = root
	}
	v.mu.Unlock()

	observed := make(map[string]bool, len(roots))
	for key, root := range roots {
		_, err := v.stat(root)
		observed[key] = err == nil
	}

	changes := make([]VolumeChange, 0, len(observed))
	v.mu.Lock()
	for key, online := range observed {
		before, known := v.mounted[key]
		v.mounted[key] = online
		changes = append(changes, VolumeChange{
			Root:    roots[key],
			Online:  online,
			Changed: known && before != online,
		})
	}
	v.mu.Unlock()

	// A stable order keeps events and tests independent of map iteration.
	sort.Slice(changes, func(i, j int) bool { return changes[i].Root < changes[j].Root })
	return changes
}

// RefreshVolumes re-observes every volume the library references and returns
// what it saw. A volume that came back is reconciled as a whole: a file that was
// only unreachable is revived, and a file that is really gone is removed as a
// favorite (see reconcileVolume). Both read paths and the background poll call
// this, so the transition behaves the same everywhere.
func (s *Service) RefreshVolumes() []VolumeChange {
	changes := s.volumes.probe()
	for _, change := range changes {
		if change.Changed && change.Online {
			s.reconcileVolume(change.Root, false, nil)
		}
	}
	return changes
}

// WatchVolumes polls volume presence in the background and reports each
// attachment and detachment ("video:volume"). The returned func stops the poll;
// it does not wait for an outstanding probe, because a probe can be waiting on a
// network share and quitting must not hang on it.
func (s *Service) WatchVolumes(every time.Duration) (stop func()) {
	if every <= 0 {
		every = volumePollEvery
	}
	done := make(chan struct{})
	var once sync.Once
	go func() {
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				for _, change := range s.RefreshVolumes() {
					if !change.Changed {
						continue
					}
					s.log.Info("volume presence changed", "root", change.Root, "online", change.Online)
					if s.emit != nil {
						s.emit("video:volume", map[string]any{
							"root":   change.Root,
							"online": change.Online,
						})
					}
				}
			}
		}
	}()
	return func() { once.Do(func() { close(done) }) }
}

// reconcileVolume re-answers "is the file there?" for every favorite on one
// attached volume. It is the single place where a volume's return is reconciled:
// files that came back are revived, files that are confirmed gone are removed as
// favorites, and files that are merely unreadable stay visible as missing.
//
// Removal is the whole point of the feature and must never be based on "stat
// failed":
//
//   - the volume must be online (the caller only gets here for an attached
//     volume, and the offline check below defends against a drive going away
//     mid-pass);
//   - the path must live on a volume the app can tell apart from the filesystem
//     root (a drive letter or UNC share). On Unix every path lives on "/", where
//     an unplugged external drive is indistinguishable from a deleted file;
//   - the error must be ErrNotExist, not a permission or I/O failure;
//   - a volume where none of the favorites is present is not trusted (possible
//     drive-letter reuse), so nothing is removed and the rows stay visible.
//
// announce controls whether status changes are announced per row. Startup needs
// that (the first screen may already be drawn); a reattach does not, because the
// volume event that triggered it is the one refresh every view needs.
//
// checked, when not nil, is called once for every file whose existence was asked
// about. The manual 文件检测 pass uses it to report how far it has got; the
// background poll has nobody waiting on it and passes nil.
func (s *Service) reconcileVolume(root string, announce bool, checked func()) (revived, removed, skipped int) {
	s.reconcileMu.Lock()
	defer s.reconcileMu.Unlock()

	// The drive may have been unplugged between the probe and this pass.
	if s.volumes.offline(root) {
		return 0, 0, 0
	}

	videos, err := s.repo.List()
	if err != nil {
		s.log.Warn("reconcile volume: list favorites failed", "root", root, "err", err)
		return 0, 0, 0
	}
	key := volumeKey(root)
	onVolume := make([]Video, 0, len(videos))
	for _, v := range videos {
		if v.IsRemote() {
			continue // a cloud:// key has no local volume to reconcile
		}
		if volumeKey(VolumeRoot(v.Path)) == key {
			onVolume = append(onVolume, v)
		}
	}
	if len(onVolume) == 0 {
		return 0, 0, 0
	}

	setStatus := s.markStatus
	if !announce {
		setStatus = s.markStatusQuiet
	}

	present := 0
	candidates := make([]*Video, 0, len(onVolume))
	for i := range onVolume {
		v := &onVolume[i]
		_, statErr := s.stat(v.Path)
		if checked != nil {
			checked()
		}
		if statErr == nil {
			present++
			if v.Status != StatusAvailable {
				setStatus(v, StatusAvailable)
				revived++
			}
			continue
		}
		// Only a confirmed-absent file on an identifiable volume is a cleanup
		// candidate. An unreadable file is not a deleted file.
		if errors.Is(statErr, os.ErrNotExist) && s.volumeIdentityKnown(v.Path) {
			candidates = append(candidates, v)
			continue
		}
		if v.Status != StatusMissing {
			setStatus(v, StatusMissing)
		}
	}
	if len(candidates) == 0 {
		return revived, 0, 0
	}
	removedIDs, skipped := s.cleanupMissing(root, len(onVolume), present, candidates, setStatus)
	return revived, len(removedIDs), skipped
}

// cleanupMissing is the one place a candidate is actually removed. The caller
// has already established that these paths are confirmed absent on an attached,
// identifiable volume; this function adds the last guard and performs the
// removal through the injected full-removal path.
//
// total is every favorite on the volume; present is how many of them (candidates
// are absent by definition) stat successfully. When none is present and the
// volume holds more than one favorite, the volume is not trusted: that is what a
// reused drive letter (or a mount that is not ready yet) looks like, and wiping
// the whole volume's favorites would be the worst possible response.
func (s *Service) cleanupMissing(root string, total, present int, candidates []*Video,
	setStatus func(*Video, string)) (removed []int64, skipped int) {
	if len(candidates) == 0 {
		return nil, 0
	}
	if present == 0 && total > 1 {
		for _, v := range candidates {
			if v.Status != StatusMissing {
				setStatus(v, StatusMissing)
			}
		}
		s.log.Warn("auto-removal skipped: no favorite on the volume is present",
			"root", root, "missing", len(candidates))
		if s.emit != nil {
			s.emit("video:missing:swept", map[string]any{
				"root": root, "removed": 0, "skipped": len(candidates),
			})
		}
		return nil, len(candidates)
	}

	for _, v := range candidates {
		remove := s.missingRemover
		if remove == nil {
			remove = s.Remove
		}
		if err := remove(v.ID); err != nil {
			s.log.Error("auto-remove missing favorite failed",
				"id", v.ID, "path", v.Path, "err", err)
			if v.Status != StatusMissing {
				setStatus(v, StatusMissing)
			}
			continue
		}
		removed = append(removed, v.ID)
	}
	if len(removed) > 0 {
		s.log.Info("missing favorites auto-removed", "root", root, "count", len(removed))
		if s.emit != nil {
			s.emit("video:missing:swept", map[string]any{
				"root": root, "removed": len(removed), "skipped": 0,
			})
		}
	}
	return removed, 0
}

// ReconcileMissing checks the given favorites against the disk and removes the
// ones confirmed gone on an attached, identifiable volume, applying the same
// volume-trust guard as the startup pass. It is the read-path entry point for a
// view that discovers missing files itself — the 番号 duplicate worklist — so a
// version that vanished while the app was running leaves that worklist at the
// same time it leaves the library.
//
// It returns the ids that were removed. A record on a detached volume is left
// alone: the drive being away is not a missing file. A record whose path is
// merely unreadable is marked missing and kept. Nothing is removed when the
// volume holds no other present favorite.
func (s *Service) ReconcileMissing(ids []int64) (map[int64]bool, error) {
	removed := make(map[int64]bool)
	if len(ids) == 0 {
		return removed, nil
	}

	s.reconcileMu.Lock()
	defer s.reconcileMu.Unlock()

	videos, err := s.repo.List()
	if err != nil {
		return nil, err
	}
	wanted := make(map[int64]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}

	byVolume := make(map[string][]Video)
	for _, v := range videos {
		if v.IsRemote() {
			continue
		}
		root := VolumeRoot(v.Path)
		key := volumeKey(root)
		byVolume[key] = append(byVolume[key], v)
	}

	for _, onVolume := range byVolume {
		if len(onVolume) == 0 {
			continue
		}
		root := VolumeRoot(onVolume[0].Path)
		// Ask the disk directly: the caller is a read that must decide now, and
		// the cached answer may predate the disk being plugged or unplugged.
		if _, err := s.stat(root); err != nil {
			s.volumes.record(root, false)
			continue
		}
		s.volumes.record(root, true)

		candidates := make([]*Video, 0, len(onVolume))
		for i := range onVolume {
			v := &onVolume[i]
			if !wanted[v.ID] {
				continue
			}
			_, statErr := s.stat(v.Path)
			if statErr == nil {
				if v.Status != StatusAvailable {
					s.markStatus(v, StatusAvailable)
				}
				continue
			}
			if errors.Is(statErr, os.ErrNotExist) && s.volumeIdentityKnown(v.Path) {
				candidates = append(candidates, v)
				continue
			}
			if v.Status != StatusMissing {
				s.markStatus(v, StatusMissing)
			}
		}
		if len(candidates) == 0 {
			continue
		}

		// Trust the volume when any favorite that is not a candidate is still
		// present. Stop at the first hit: a healthy volume costs one stat.
		candidateIDs := make(map[int64]bool, len(candidates))
		for _, v := range candidates {
			candidateIDs[v.ID] = true
		}
		present := 0
		for i := range onVolume {
			v := &onVolume[i]
			if candidateIDs[v.ID] {
				continue
			}
			if _, err := s.stat(v.Path); err == nil {
				present = 1
				break
			}
		}

		removedIDs, _ := s.cleanupMissing(root, len(onVolume), present, candidates, s.markStatus)
		for _, id := range removedIDs {
			removed[id] = true
		}
	}
	return removed, nil
}

// markStatusQuiet writes a status change without the per-row event. It is the
// bulk form used when one observation explains many rows at once.
func (s *Service) markStatusQuiet(v *Video, status string) {
	if v.Status == status {
		return
	}
	if err := s.repo.UpdateStatus(v.ID, status); err != nil {
		s.log.Error("update status failed", "id", v.ID, "err", err)
		return
	}
	v.Status = status
}

// volumeDetached reports the root of the volume holding path when that volume is
// not attached, and "" otherwise. It asks the disk rather than the cache because
// its callers are single-file operations about to report a failure to the user,
// where a stale answer is worse than a stat; the answer is recorded either way.
func (s *Service) volumeDetached(path string) string {
	root := s.volumes.rootOf(path)
	if root == "" {
		return ""
	}
	_, err := s.stat(root)
	s.volumes.record(path, err == nil)
	if err == nil {
		return ""
	}
	return root
}
