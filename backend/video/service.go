package video

import (
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"log/slog"
	"os"
	pathpkg "path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"muse-app/backend/platform"
	"muse-app/backend/thumbnail"
)

// Service holds the favorite business logic. It is bound to Wails by app.go.
type Service struct {
	repo      VideoRepository
	processor *Processor
	dataDir   string
	log       *slog.Logger
	emit      func(name string, data ...any)
	// fingerprinter is optional: a Service without one never hashes anything
	// (the desktop app always wires it, some tests do not).
	fingerprinter *Fingerprinter
	// scan holds the state of the library-wide content scan.
	scan contentScanState
	// fileScan holds the state of the 文件检测 (file-existence) pass, shared by
	// the startup check and the manual scan.
	fileScan fileScanState
	// confirm holds the full-file confirmations currently running, keyed by the
	// digest of the group being confirmed.
	confirm confirmState
	// sweepReq coalesces the image sweeps scheduled by removals.
	sweepReq imageSweepRequests
	// fingerprintFile and stat are the seams behind relocation matching and
	// every other disk question the service asks (see relocate.go and
	// volume.go): tests replace them to describe a file and a path's state
	// without touching a disk.
	fingerprintFile func(path string) (FingerprintResult, error)
	stat            func(path string) (os.FileInfo, error)
	// missingRemover is the full removal path (record, thumbnail, group
	// memberships, scraped metadata). The façade wires it once at startup; nil
	// falls back to Remove, which is everything the video layer owns.
	missingRemover func(id int64) error
	// volumeIdentifiable reports whether a path's volume root names a disk the
	// app can tell apart from the filesystem root. On Windows a drive letter or
	// UNC share does; on Unix every path lives on "/", where an unplugged
	// external drive is indistinguishable from a deleted file, so destructive
	// cleanup is never attempted there. Tests replace it to describe a machine
	// with drive letters.
	volumeIdentifiable func(path string) bool
	// reconcileMu serializes whole-volume reconciliations: startup and the
	// background poll may otherwise decide about the same rows at once.
	reconcileMu sync.Mutex
	// volumes caches which of the volumes the library references are attached
	// (see volume.go).
	volumes *volumeState
}

func NewService(repo VideoRepository, processor *Processor, dataDir string, log *slog.Logger,
	emit func(name string, data ...any)) *Service {
	s := &Service{
		repo: repo, processor: processor, dataDir: dataDir, log: log, emit: emit,
		fingerprintFile: Fingerprint, stat: os.Stat,
		volumeIdentifiable: func(path string) bool {
			return filepath.VolumeName(path) != ""
		},
	}
	// The cache asks through s.stat, not os.Stat directly, so a test that
	// replaces the seam describes the machine for relocation and volume probing
	// alike.
	s.volumes = newVolumeState(func(path string) (os.FileInfo, error) { return s.stat(path) })
	return s
}

// SetMediaPathResolver wires how ffprobe/ffmpeg reach a favorite's bytes. Local
// favorites keep using Path; remote favorites use the loopback media proxy.
func (s *Service) SetMediaPathResolver(fn func(*Video) (string, error)) {
	s.processor.SetMediaPathResolver(fn)
}

// SetFingerprinter wires the background content-fingerprint worker. Called
// during startup, before any favorite can be added.
func (s *Service) SetFingerprinter(f *Fingerprinter) {
	s.fingerprinter = f
	// A file whose disk is not attached must keep its digest: teach the
	// fingerprint worker the difference between "away" and "unreadable".
	f.volumeOffline = s.volumes.offline
}

// SetMissingRemover wires the full removal path used when a favorite's file is
// gone while its volume is attached. The façade passes api.Library.RemoveVideo
// so an automatic cleanup also clears group memberships and scraped metadata;
// leaving it unset falls back to Remove, which is everything the video layer
// owns on its own.
func (s *Service) SetMissingRemover(remove func(id int64) error) {
	s.missingRemover = remove
}

// volumeIdentityKnown reports whether the path lives on a volume the app can
// distinguish from the filesystem root. A nil override keeps the test seam
// optional.
func (s *Service) volumeIdentityKnown(path string) bool {
	if s.volumeIdentifiable != nil {
		return s.volumeIdentifiable(path)
	}
	return filepath.VolumeName(path) != ""
}

// List returns all favorites, most recently added first (spec-v1.md §17).
func (s *Service) List() ([]Video, error) {
	videos, err := s.repo.List()
	if err != nil {
		return nil, err
	}
	s.volumes.stamp(videos)
	return videos, nil
}

// Search matches keyword against file name and full path (spec-v1.md §18).
func (s *Service) Search(keyword string) ([]Video, error) {
	videos, err := s.repo.Search(keyword)
	if err != nil {
		return nil, err
	}
	s.volumes.stamp(videos)
	return videos, nil
}

// Get returns a single favorite.
func (s *Service) Get(id int64) (*Video, error) {
	v, err := s.repo.Get(id)
	if err != nil {
		return nil, err
	}
	s.volumes.stampVideo(v)
	return v, nil
}

// AddVideo validates, deduplicates and records a new favorite. Metadata and
// thumbnail are produced asynchronously so this returns fast (spec-v1.md §12,
// §32; spec-v1.1.md §11).
func (s *Service) AddVideo(path string) (*Video, error) {
	v, _, err := s.addVideo(path)
	return v, err
}

// addVideo is the single add path. relocated reports that the file re-pointed an
// existing favorite instead of creating one, which is what batch callers report
// back to the user (see relocate.go).
func (s *Service) addVideo(path string) (*Video, bool, error) {
	normalized, info, err := NormalizeAndValidate(path)
	if err != nil {
		return nil, false, err
	}
	if existing, err := s.repo.GetByPath(normalized); err == nil {
		// The path is already recorded. When that favorite is marked missing the
		// file has come back where the app last saw it, so the add revives the
		// favorite instead of refusing the file the user is asking to add.
		if existing.Status == StatusMissing {
			return s.reviveFavorite(existing), false, nil
		}
		return nil, false, ErrAlreadyAdded
	} else if err != ErrNotFound {
		return nil, false, err
	}
	// The file may be one the library already knows under a path that no longer
	// exists: the same bytes after a move. The reference follows the file; the
	// id, and everything the user attached to it, stays.
	if relinked := s.relinkOnAdd(normalized, info); relinked != nil {
		return relinked, true, nil
	}

	now := nowMillis()
	v := &Video{
		Path:      normalized,
		FileName:  filepath.Base(normalized),
		FileSize:  info.Size(),
		CreatedAt: now,
		UpdatedAt: now,
		Status:    StatusAvailable,
	}
	if err := s.repo.Create(v); err != nil {
		return nil, false, err
	}
	s.log.Info("video added", "id", v.ID, "path", v.Path)
	s.processor.Enqueue(v.ID)
	s.enqueueFingerprint(v)
	return v, false, nil
}

// reviveFavorite marks a favorite available again after its file reappeared at
// the path the record already holds, and re-verifies what the app knows about the
// file: the bytes behind a path that was empty can be different ones, so
// metadata, cover and fingerprint are all re-derived.
func (s *Service) reviveFavorite(v *Video) *Video {
	s.markStatus(v, StatusAvailable)
	s.processor.Enqueue(v.ID)
	s.enqueueFingerprint(v)
	s.log.Info("favorite revived at its recorded path", "id", v.ID, "path", v.Path)
	return v
}

// enqueueFingerprint schedules the new favorite for fingerprinting, along with
// the favorites it could be a copy of.
//
// The peers are the point: a scan prunes files whose size is unique, so those
// keep no fingerprint until a second file of that size shows up. Without them,
// adding a copy of a pruned file would compare its digest against nothing.
func (s *Service) enqueueFingerprint(v *Video) {
	if s.fingerprinter == nil {
		return
	}
	s.fingerprinter.Enqueue(v.ID)
	peers, err := s.repo.ListSameSizePeers(v.FileSize, v.ID)
	if err != nil {
		s.log.Warn("list same-size peers failed", "id", v.ID, "err", err)
		return
	}
	for _, id := range peers {
		s.fingerprinter.Enqueue(id)
	}
}

// AddVideos adds each path independently; one failure never blocks the rest
// (spec-v1.1.md §7, §8, §13).
func (s *Service) AddVideos(paths []string) []AddVideoResult {
	results := make([]AddVideoResult, 0, len(paths))
	for _, p := range paths {
		result := AddVideoResult{Path: p}
		v, relocated, err := s.addVideo(p)
		if err != nil {
			result.Error = UserMessage(err)
			s.log.Warn("add video failed", "path", p, "err", err)
		} else {
			result.Success = true
			result.VideoID = v.ID
			result.Relocated = relocated
		}
		results = append(results, result)
	}
	return results
}

// RemotePathKey builds the synthetic, stable path stored in videos.path for a
// WebDAV favorite. It is case-sensitive and cannot collide with a local path:
// the real remote path lives in remote_path, and the local UNIQUE COLLATE NOCASE
// path index must never decide that two remote spellings are one file.
func RemotePathKey(sourceID int64, remotePath string) string {
	sum := sha1.Sum([]byte(remotePath))
	return fmt.Sprintf("cloud://%d/%x", sourceID, sum)
}

// AddRemoteVideo records a favorite that lives on a WebDAV connection. It
// deliberately does not stat the path: the caller has already done a PROPFIND
// and passes the name/size it observed, so adding a remote file costs no second
// round trip. created is false when the favorite already existed and was
// revived after being marked missing.
func (s *Service) AddRemoteVideo(sourceID int64, remotePath, name string, size int64) (*Video, bool, error) {
	if sourceID <= 0 {
		return nil, false, ErrInvalidRemotePath
	}
	if remotePath == "" || !strings.HasPrefix(remotePath, "/") {
		return nil, false, ErrInvalidRemotePath
	}
	if name == "" {
		name = pathpkg.Base(remotePath)
	}
	if !IsSupportedVideoFile(name) {
		return nil, false, ErrUnsupportedType
	}
	if existing, err := s.repo.GetByRemote(sourceID, remotePath); err == nil {
		if existing.Status == StatusMissing {
			return s.reviveFavorite(existing), false, nil
		}
		return nil, false, ErrAlreadyAdded
	} else if err != ErrNotFound {
		return nil, false, err
	}

	now := nowMillis()
	v := &Video{
		Path:       RemotePathKey(sourceID, remotePath),
		FileName:   name,
		FileSize:   size,
		CreatedAt:  now,
		UpdatedAt:  now,
		Status:     StatusAvailable,
		SourceID:   sourceID,
		RemotePath: remotePath,
	}
	if err := s.repo.Create(v); err != nil {
		return nil, false, err
	}
	s.log.Info("remote video added", "id", v.ID, "source", sourceID, "path", remotePath)
	s.processor.Enqueue(v.ID)
	return v, true, nil
}

// Remove deletes only the database record and the generated thumbnail file.
// The original video is never touched (spec-v1.md §13, §33).
//
// The file not existing is never a reason to refuse: a favorite is a reference,
// and a reference to a file the user already deleted is exactly the one they
// most want gone. Removal is driven by the record, not by the disk.
func (s *Service) Remove(id int64) error {
	v, err := s.repo.Get(id)
	if errors.Is(err, ErrNotFound) {
		// The row is already gone, which is the outcome the caller asked for.
		// This is not a corner case: a card goes stale whenever the record was
		// removed elsewhere (a second window, the LAN client, a duplicate panel),
		// and reporting "找不到视频" there left the user with a card that could
		// never be dismissed. Answering success lets the client drop it.
		s.removeUnreferencedThumbnail(id)
		// The stale card may be one of many; reconcile the folders so a sweep
		// also cleans up after whatever removed the record originally.
		s.sweepImagesAfterRemove(id)
		s.emitRemoved(id)
		s.log.Debug("video already removed", "id", id)
		return nil
	}
	if err != nil {
		return err
	}
	if err := s.repo.Delete(id); err != nil {
		return err
	}
	s.removeThumbnail(v)
	// A worker that was downloading this favorite's cover may write it back
	// after the deletion (see removeThumbnail): asset cleanup is therefore a
	// reconciliation against the database, not a single unlink.
	s.sweepImagesAfterRemove(id)
	s.emitRemoved(id)
	s.log.Info("video removed", "id", id, "path", v.Path)
	return nil
}

// emitRemoved announces that a favorite left the library. It fires on the
// "already gone" path too: the caller's card is stale either way, and another
// window (or the phone) may still be showing it.
func (s *Service) emitRemoved(id int64) {
	if s.emit != nil {
		s.emit("video:removed", map[string]any{"id": id})
	}
}

// removeThumbnail deletes the generated cover of a record that has just been
// deleted. It reads the recorded path and falls back to the id-shaped file name
// the generator uses, so a record whose cover was replaced (a scraped or custom
// frame) cannot leave an orphan behind either.
//
// This is the fast path, not the guarantee: a cover that is being downloaded at
// this moment can land after these unlinks, and a record deleted by an older
// build was never cleaned up at all. RemoveOrphanedImages reconciles the folders
// against the database and is what actually keeps them bounded.
func (s *Service) removeThumbnail(v *Video) {
	if v.ThumbnailPath != "" {
		s.removeThumbnailFile(filepath.FromSlash(v.ThumbnailPath))
	}
	s.removeThumbnailFile(filepath.Join(thumbnail.RelativeDir, strconv.FormatInt(v.ID, 10)+".jpg"))
}

// removeUnreferencedThumbnail cleans up after a record that was already gone, so
// a stale removal still leaves nothing behind. Nothing describes it any more, so
// the image is named by id.
func (s *Service) removeUnreferencedThumbnail(id int64) {
	s.removeThumbnailFile(filepath.Join(thumbnail.RelativeDir, strconv.FormatInt(id, 10)+".jpg"))
}

// removeThumbnailFile deletes one generated image below the data dir. The path
// is reduced to its base name on purpose: the column holds a relative path, and
// a value that is not one must never point outside the thumbnail folder.
func (s *Service) removeThumbnailFile(rel string) {
	thumbFile := filepath.Join(s.dataDir, thumbnail.RelativeDir, filepath.Base(rel))
	if err := os.Remove(thumbFile); err != nil && !os.IsNotExist(err) {
		s.log.Warn("remove thumbnail failed", "file", thumbFile, "err", err)
	}
}

// Open launches the system default player (spec-v1.md §14), checking the file
// first and marking it missing when it is gone (§20). A file that is unreachable
// because its volume is not attached is not missing: the favorite is left alone
// and the caller is told which disk to plug in.
func (s *Service) Open(id int64) error {
	v, err := s.repo.Get(id)
	if err != nil {
		return err
	}
	if _, err := s.stat(v.Path); err != nil {
		if root := s.volumeDetached(v.Path); root != "" {
			return fmt.Errorf("磁盘未接入：%s\n%s", root, v.Path)
		}
		s.markStatus(v, StatusMissing)
		return fmt.Errorf("找不到文件。\n%s", v.Path)
	}
	s.volumes.record(v.Path, true)
	s.markStatus(v, StatusAvailable)
	if err := platform.OpenWithDefaultApp(v.Path); err != nil {
		return fmt.Errorf("打开视频失败：%w", err)
	}
	if err := s.repo.UpdateLastOpened(v.ID, nowMillis()); err != nil {
		s.log.Warn("update last opened failed", "id", v.ID, "err", err)
	}
	return nil
}

// OpenContainingFolder reveals the file in the system file manager (spec-v1.md §15).
func (s *Service) OpenContainingFolder(id int64) error {
	v, err := s.repo.Get(id)
	if err != nil {
		return err
	}
	if _, err := s.stat(v.Path); err != nil {
		if root := s.volumeDetached(v.Path); root != "" {
			return fmt.Errorf("磁盘未接入：%s\n%s", root, v.Path)
		}
		s.markStatus(v, StatusMissing)
		return fmt.Errorf("找不到文件。\n%s", v.Path)
	}
	s.volumes.record(v.Path, true)
	return platform.OpenContainingFolder(v.Path)
}

// RefreshStatus re-checks one file's existence and returns the updated record.
// A file on a volume that is not attached is reported as such and keeps its
// status: it is not missing, it is away.
func (s *Service) RefreshStatus(id int64) (*Video, error) {
	v, err := s.repo.Get(id)
	if err != nil {
		return nil, err
	}
	v.VolumeRoot = VolumeRoot(v.Path)
	if root := s.volumeDetached(v.Path); root != "" {
		v.VolumeOffline = true
		return v, nil
	}
	v.VolumeOffline = false
	status := StatusAvailable
	if _, err := s.stat(v.Path); err != nil {
		status = StatusMissing
	}
	s.markStatus(v, status)
	v.Status = status
	return v, nil
}

// BackfillMetadata re-probes every favorite that has no duration or frame rate
// (typically rows created before those fields existed). It enqueues them on the
// background processor so ffprobe re-extracts the missing metadata. Safe to call
// on a fresh library (finds nothing). Called from startup when a schema
// migration introduced a metadata field.
func (s *Service) BackfillMetadata() int {
	ids, err := s.repo.ListNeedingMetadata()
	if err != nil {
		s.log.Error("list videos needing metadata failed", "err", err)
		return 0
	}
	for _, id := range ids {
		s.processor.Enqueue(id)
	}
	if len(ids) > 0 {
		s.log.Info("scheduled metadata backfill", "count", len(ids))
	}
	return len(ids)
}

// BackfillProcessing retries videos that still need metadata (including the
// codec names) or an automatic cover. A transient media-read failure can leave
// metadata populated while the thumbnail is missing, so metadata-only backfill
// is not sufficient. Run on every startup: it is idempotent.
func (s *Service) BackfillProcessing() int {
	videos, err := s.repo.List()
	if err != nil {
		s.log.Error("list videos needing processing failed", "err", err)
		return 0
	}
	count := 0
	for _, v := range videos {
		if !s.needsProcessing(v) {
			continue
		}
		s.processor.Enqueue(v.ID)
		count++
	}
	if count > 0 {
		s.log.Info("scheduled video processing backfill", "count", count)
	}
	return count
}

// needsProcessing reports whether the background processor still has work to do
// for a favorite: metadata it never extracted, or an automatic cover it never
// produced. Shared by the startup backfill and relocation matching, so the two
// cannot drift about what "already processed" means.
func (s *Service) needsProcessing(v Video) bool {
	needsMetadata := v.DurationMs <= 0 || v.FrameRate <= 0 || v.Bitrate <= 0 || v.VideoCodec == ""
	needsThumbnail := v.CustomCoverAt == 0 &&
		(v.CoverSource == "" || v.CoverSource == "auto_frame") &&
		!s.hasAutomaticThumbnail(v)
	return needsMetadata || needsThumbnail
}

func (s *Service) hasAutomaticThumbnail(v Video) bool {
	if v.ThumbnailPath == "" {
		return false
	}
	name := filepath.Base(filepath.FromSlash(v.ThumbnailPath))
	if name == "." || name == string(filepath.Separator) {
		return false
	}
	_, err := os.Stat(filepath.Join(s.dataDir, thumbnail.RelativeDir, name))
	return err == nil
}

func (s *Service) markStatus(v *Video, status string) {
	if v.Status == status {
		return
	}
	if err := s.repo.UpdateStatus(v.ID, status); err != nil {
		s.log.Error("update status failed", "id", v.ID, "err", err)
		return
	}
	v.Status = status
	s.log.Info("status changed", "id", v.ID, "status", status, "path", v.Path)
	if s.emit != nil {
		s.emit("video:status:changed", map[string]any{"id": v.ID, "status": status})
	}
}

// ---- Video info editing (编辑影片信息) ----

// UpdateInfo saves the user's custom title and notes. file_name stays
// immutable; an empty title means "display the file name".
func (s *Service) UpdateInfo(id int64, title, notes string) (*Video, error) {
	if _, err := s.repo.Get(id); err != nil {
		return nil, err
	}
	if err := s.repo.UpdateInfo(id, strings.TrimSpace(title), strings.TrimSpace(notes)); err != nil {
		return nil, err
	}
	s.log.Info("video info updated", "id", id)
	return s.repo.Get(id)
}

// LikeVideo records one like for a favorite. Likes stack — every call adds one
// to the counter — and returns the video with the updated count.
func (s *Service) LikeVideo(id int64) (*Video, error) {
	count, err := s.repo.IncrementLikes(id)
	if err != nil {
		return nil, err
	}
	s.log.Info("video liked", "id", id, "count", count)
	if s.emit != nil {
		s.emit("video:liked", map[string]any{"id": id, "likeCount": count})
	}
	return s.repo.Get(id)
}

// ClearVideoLikes resets a favorite's like counter to zero ("清空点赞数").
func (s *Service) ClearVideoLikes(id int64) (*Video, error) {
	v, err := s.repo.Get(id)
	if err != nil {
		return nil, err
	}
	if v.LikeCount == 0 {
		return v, nil
	}
	if err := s.repo.ClearLikes(id); err != nil {
		return nil, err
	}
	s.log.Info("video likes cleared", "id", id, "previous", v.LikeCount)
	if s.emit != nil {
		s.emit("video:liked", map[string]any{"id": id, "likeCount": 0})
	}
	return s.repo.Get(id)
}

// SetCover extracts the frame at atSec with ffmpeg and installs it as the
// video's cover (overwriting thumbnails/<id>.jpg). The timestamp is clamped to
// the video duration; automatic frame processing never overwrites it. Scraped
// cover replacement follows the video's AllowScrapedCover preference.
func (s *Service) SetCover(id int64, atSec float64) (*Video, error) {
	v, err := s.repo.Get(id)
	if err != nil {
		return nil, err
	}
	if atSec < 0 {
		atSec = 0
	}
	if v.DurationMs > 0 {
		if maxSec := float64(v.DurationMs)/1000 - 0.1; atSec > maxSec {
			atSec = maxSec
		}
		if atSec < 0 {
			atSec = 0
		}
	}
	mediaPath, err := s.processor.MediaPath(v)
	if err != nil {
		return nil, err
	}
	thumbDir := filepath.Join(s.dataDir, thumbnail.RelativeDir)
	var rel string
	if v.IsRemote() {
		rel, err = thumbnail.ExtractAtContextWithTimeout(context.Background(), s.processor.FFmpegBin(), mediaPath, atSec, thumbDir, v.ID, 90*time.Second)
	} else {
		rel, err = thumbnail.ExtractAt(s.processor.FFmpegBin(), mediaPath, atSec, thumbDir, v.ID)
	}
	if err != nil {
		return nil, err
	}
	if err := s.repo.UpdateThumbnail(v.ID, rel); err != nil {
		return nil, err
	}
	if err := s.repo.SetCustomCover(v.ID, nowMillis()); err != nil {
		return nil, err
	}
	if err := s.repo.SetCoverSource(v.ID, "custom_frame"); err != nil {
		return nil, err
	}
	s.log.Info("custom cover set", "id", v.ID, "atSec", atSec)
	if s.emit != nil {
		s.emit("thumbnail:generated", map[string]any{"id": v.ID})
	}
	return s.repo.Get(id)
}

// ResetCover reverts to the automatic thumbnail: the custom marker is cleared
// and the background processor regenerates the image (a no-op when the cover
// is already automatic).
func (s *Service) ResetCover(id int64) (*Video, error) {
	v, err := s.repo.Get(id)
	if err != nil {
		return nil, err
	}
	if v.CustomCoverAt == 0 {
		return v, nil
	}
	if err := s.repo.ClearCustomCover(v.ID); err != nil {
		return nil, err
	}
	s.log.Info("custom cover reset", "id", v.ID)
	if v.AllowScrapedCover && v.ScrapedCoverPath != "" {
		if err := s.repo.UpdateThumbnail(v.ID, v.ScrapedCoverPath); err != nil {
			return nil, err
		}
		if err := s.repo.SetCoverSource(v.ID, "scraped"); err != nil {
			return nil, err
		}
	} else {
		if err := s.repo.SetCoverSource(v.ID, "auto_frame"); err != nil {
			return nil, err
		}
		if s.processor != nil {
			s.processor.Enqueue(v.ID)
		}
	}
	return s.repo.Get(id)
}

// SetScrapedCoverAllowed changes whether a video's downloaded scraped cover
// may replace its current cover. Enabling it also applies an already-downloaded
// scraped cover immediately.
func (s *Service) SetScrapedCoverAllowed(id int64, allowed bool) (*Video, error) {
	if _, err := s.repo.Get(id); err != nil {
		return nil, err
	}
	if err := s.repo.SetScrapedCoverAllowed(id, allowed); err != nil {
		return nil, err
	}
	return s.repo.Get(id)
}
