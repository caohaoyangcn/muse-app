// Package api is a single transport-independent façade over the business
// services (video.Service, group.Service). Both transports — the Wails desktop
// app (app.go) and the LAN HTTP server (server.go) — call exactly these methods,
// so read/write behaviour, validation and orchestration live in one place and
// can never drift between clients.
//
// Web-specific concerns (HTTP Range streaming, thumbnail URLs, the mobile SPA,
// Platform shell actions like open-in-file-manager) deliberately stay in their own
// layers and are not part of this façade.
package api

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"muse-app/backend/cloud"
	"muse-app/backend/group"
	"muse-app/backend/jav"
	"muse-app/backend/video"
	"muse-app/backend/watch"
)

// mosaicRestoredGroup is the automatic collection rule for restored
// (mosaic-removed) works. Its members carry a tag badge in the UI.
const mosaicRestoredGroup = "马赛克破解"

// Recycle-bin retention policy. A trashed favorite is restorable for this long
// by default; the value is a user preference and is clamped to the same range
// everywhere the façade reads it. The default lives here (not in settings) so
// the domain is usable before any preference has been stored.
const (
	DefaultTrashRetentionDays = 14
	MinTrashRetentionDays     = 1
	MaxTrashRetentionDays     = 365
)

// TrashSnapshot is the recycle-bin read model. RetentionDays travels with the
// rows so a client can render "N days left" without a second settings read.
type TrashSnapshot struct {
	Videos        []video.Video `json:"videos"`
	RetentionDays int           `json:"retentionDays"`
}

// Library is the façade that owns the cross-service orchestration shared by
// both transports. It is safe for concurrent use by the desktop bindings and
// the HTTP handlers (the underlying services are themselves concurrency-safe).
type Library struct {
	videos *video.Service
	groups *group.Service
	watch  *watch.Service
	jav    *jav.Service
	cloud  *cloud.Service
	log    *slog.Logger
	// trashRetention reads the configured retention in days. nil falls back to
	// DefaultTrashRetentionDays.
	trashRetention func() int
	// now is the clock behind the retention sweep; tests replace it so the
	// boundary can be described without sleeping.
	now func() int64
}

// New builds the façade over the given services.
func New(videos *video.Service, groups *group.Service, log *slog.Logger) *Library {
	l := &Library{
		videos: videos, groups: groups, log: log,
		now: func() int64 { return time.Now().UnixMilli() },
	}
	// Automatic cleanup of a favorite whose file is gone while its volume is
	// attached enters the recycle bin with the missing reason. Wire the façade's
	// one soft-delete path here, so every caller — desktop, LAN server, tests —
	// gets the same orchestration instead of a bare flag update.
	if videos != nil {
		// Automatic cleanup of a missing file enters the recycle bin with the
		// "missing" reason.
		videos.SetMissingRemover(func(id int64) error {
			return l.trashVideo(id, video.TrashReasonMissing)
		})
	}
	return l
}

// SetJavService wires optional JAV metadata enrichment into the façade.
func (l *Library) SetJavService(svc *jav.Service) { l.jav = svc }

// SetCloudService wires optional WebDAV sources into the façade.
func (l *Library) SetCloudService(svc *cloud.Service) { l.cloud = svc }

// scrapedCodes loads the authoritative 番号 per favorite for the video read
// model. It is deliberately best effort: a scrape result that cannot be read
// must not fail the library itself, so names simply fall back to file names.
func (l *Library) scrapedCodes() map[int64]string {
	if l.jav == nil {
		return nil
	}
	codes, err := l.jav.Codes()
	if err != nil {
		l.log.Warn("load scraped codes failed", "err", err)
		return nil
	}
	return codes
}

// restoredVideoIDs returns the favorites in the 马赛克破解 group, including the
// members of its child groups (group membership is transitive). It is best
// effort, like the scrape read: a failure leaves the badge off rather than
// failing the library.
func (l *Library) restoredVideoIDs() map[int64]struct{} {
	if l.groups == nil {
		return nil
	}
	ids, err := l.groups.VideoIDsInNamedGroup(mosaicRestoredGroup)
	if err != nil {
		l.log.Warn("load restored group members failed", "err", err)
		return nil
	}
	set := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set
}

// applyReadModel fills the fields a Video carries only for reading: the
// authoritative 番号 from the scrape result and the 马赛克破解 tag from group
// membership. Both maps are loaded once per batch by the caller.
func applyReadModel(v *video.Video, codes map[int64]string, restored map[int64]struct{}, sources map[int64]cloud.SourceState, backups map[string]bool) {
	v.JavCode = codes[v.ID]
	_, v.MosaicRestored = restored[v.ID]
	if v.DeletedAt == 0 && v.JavCode != "" {
		v.BackedUp = backups[jav.DuplicateKey(v.JavCode, jav.PartForFile(v.FileName, v.JavCode))]
	}
	if v.SourceID > 0 {
		if source, ok := sources[v.SourceID]; ok {
			v.SourceName = source.Name
			v.SourceOffline = !source.Online
			v.SourceEnabled = source.Enabled
			if source.WebURL != "" && v.DeletedAt == 0 && source.Enabled {
				if fileURL, err := cloud.FileURL(source.WebURL, v.RemotePath); err == nil {
					v.CloudURL = fileURL
				}
			}
		} else if v.ArchiveSourceName != "" {
			v.SourceName = v.ArchiveSourceName
		}
		if v.DeletedAt > 0 {
			// Archived/trashed rows are historical: do not advertise a live
			// source or a playback URL.
			v.SourceOffline = false
			v.CloudURL = ""
		}
	}
}

// cloudSources loads the connection names/health the read model needs. It is
// best effort like the scrape read: a source that cannot be listed must not
// fail the library, it only leaves the badge/name empty.
func (l *Library) backedUpKeys() map[string]bool {
	if l.jav == nil {
		return nil
	}
	keys, err := l.jav.BackedUpKeys()
	if err != nil {
		l.log.Warn("load backed-up work keys failed", "err", err)
		return nil
	}
	return keys
}

func (l *Library) cloudSources() map[int64]cloud.SourceState {
	if l.cloud == nil {
		return nil
	}
	sources, err := l.cloud.SourceStates()
	if err != nil {
		l.log.Warn("load cloud source states failed", "err", err)
		return nil
	}
	return sources
}

// attachReadModel fills the read-model fields on a list of favorites. The two
// projections live outside the videos table, so every read path that hands the
// UI a Video passes through here.
func (l *Library) attachReadModel(videos []video.Video) {
	if len(videos) == 0 {
		return
	}
	codes, restored, sources, backups := l.scrapedCodes(), l.restoredVideoIDs(), l.cloudSources(), l.backedUpKeys()
	for i := range videos {
		applyReadModel(&videos[i], codes, restored, sources, backups)
	}
}

// attachReadModelOne is attachReadModel for a single favorite.
func (l *Library) attachReadModelOne(v *video.Video) {
	if v == nil {
		return
	}
	applyReadModel(v, l.scrapedCodes(), l.restoredVideoIDs(), l.cloudSources(), l.backedUpKeys())
}

// ---- Reads (shared read model for both transports) ----

// ListVideos returns all favorites, most recently added first.
func (l *Library) ListVideos() ([]video.Video, error) {
	videos, err := l.videos.List()
	if err != nil {
		return nil, err
	}
	l.attachReadModel(videos)
	return videos, nil
}

// SearchVideos filters favorites by file name or path.
func (l *Library) SearchVideos(keyword string) ([]video.Video, error) {
	videos, err := l.videos.Search(keyword)
	if err != nil {
		return nil, err
	}
	l.attachReadModel(videos)
	return videos, nil
}

// ListGroups returns all groups in sidebar order, with member counts.
func (l *Library) ListGroups() ([]group.Group, error) {
	return l.groups.List()
}

// VideoIDsInGroup returns the favorite ids in a group (newest first) — the
// small-shaped form the desktop sidebar uses.
func (l *Library) VideoIDsInGroup(groupID int64) ([]int64, error) {
	return l.groups.VideoIDsInGroup(groupID)
}

// VideosInGroup returns the full favorites in a group (newest first), in the
// same shape as ListVideos. Used by the HTTP API so a mobile client needs only
// one source of truth. The orchestration of id→object lives here, shared.
func (l *Library) VideosInGroup(groupID int64) ([]video.Video, error) {
	ids, err := l.groups.VideoIDsInGroup(groupID)
	if err != nil {
		return nil, err
	}
	all, err := l.videos.List()
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]video.Video, len(all))
	for _, v := range all {
		byID[v.ID] = v
	}
	out := make([]video.Video, 0, len(ids))
	for _, id := range ids {
		if v, ok := byID[id]; ok {
			out = append(out, v)
		}
	}
	l.attachReadModel(out)
	return out, nil
}

// Video returns a single favorite by id.
func (l *Library) Video(id int64) (*video.Video, error) {
	v, err := l.videos.Get(id)
	if err != nil {
		return nil, err
	}
	l.attachReadModelOne(v)
	return v, nil
}

// MarkVideoOpened records a playback reported by a transport. The desktop
// records the timestamp inside Open; the LAN client hands playback to an
// external app, so the server never sees it and the client reports it here.
// The value is what the recently-played sort and view read.
func (l *Library) MarkVideoOpened(id int64) error {
	return l.videos.MarkOpened(id)
}

// RefreshVolumes re-observes the volumes the favorites live on (is the external
// drive plugged in?) and returns what it saw. A volume that came back is
// reconciled: files that were only away stop being reported as missing, and
// files confirmed gone while the volume is attached are removed as favorites
// through RemoveVideo (records and generated assets only, never the source
// video). Both transports need this read: the desktop calls it on startup, the
// HTTP API exposes the answer as VolumeOffline on every video.
func (l *Library) RefreshVolumes() []video.VolumeChange {
	return l.videos.RefreshVolumes()
}

// ScanFileStatuses starts a 文件检测 pass: the startup file-existence check,
// on demand, so a file deleted or moved while the app runs does not have to wait
// for the next launch to be noticed. It returns immediately; the pass reports
// progress and its outcome through FileScanStatus (and, on the desktop, through
// the video:filescan events).
func (l *Library) ScanFileStatuses() video.FileScanStatus {
	return l.videos.ScanFileStatuses()
}

// FileScanStatus returns the current or last 文件检测 pass.
func (l *Library) FileScanStatus() video.FileScanStatus {
	return l.videos.FileScanStatus()
}

// VideoGroupIDs returns the ids of the groups a video belongs to.
func (l *Library) VideoGroupIDs(videoID int64) ([]int64, error) {
	return l.groups.VideoGroupIDs(videoID)
}

// ---- Writes (shared write model; the Web front-end simply does not expose
// these, keeping its capability surface intentional) ----

// AddVideo adds a single favorite and schedules background metadata scraping.
func (l *Library) AddVideo(path string) (*video.Video, error) {
	v, err := l.videos.AddVideo(path)
	if err != nil {
		return nil, err
	}
	if l.jav != nil {
		l.jav.Enqueue(v.ID)
	}
	l.attachReadModelOne(v)
	return v, nil
}

// AddVideos adds a batch of paths, one independent result per file. Every
// successfully imported video is scheduled for background metadata scraping.
func (l *Library) AddVideos(paths []string) []video.AddVideoResult {
	results := l.videos.AddVideos(paths)
	for _, r := range results {
		if r.Success {
			if l.jav != nil {
				l.jav.Enqueue(r.VideoID)
			}
		}
	}
	return results
}

// RemoveVideo moves a favorite to the recycle bin. It keeps the record, its
// generated images, group memberships and scraped metadata, so a restore is a
// flag change.
//
// A record that is already gone removes cleanly rather than erroring: both
// clients report the outcome by reloading, so a list that went stale — a
// removed duplicate, another window, the LAN client — still ends up consistent
// instead of leaving a card that can never be dismissed.
func (l *Library) RemoveVideo(id int64) error {
	return l.trashVideo(id, video.TrashReasonUser)
}

// trashVideo is the one soft-delete orchestration. The reason only explains why
// the favorite left the library; both paths keep the same record.
func (l *Library) trashVideo(id int64, reason string) error {
	return l.videos.Trash(id, reason)
}

// RestoreVideo brings a trashed favorite back into the library. The record keeps
// its id, groups, likes and scraped metadata; the video layer re-establishes its
// file status from the disk.
func (l *Library) RestoreVideo(id int64) error {
	v, err := l.videos.GetAny(id)
	if err != nil {
		return err
	}
	if v.IsRemote() {
		if l.cloud == nil {
			return fmt.Errorf("网盘服务未初始化")
		}
		conn, err := l.cloud.Connection(v.SourceID)
		if err != nil {
			return fmt.Errorf("原网盘连接已删除，无法恢复这条收藏")
		}
		if !conn.Enabled {
			return fmt.Errorf("请先在设置中启用该网盘，再恢复这条收藏")
		}
	}
	return l.videos.Restore(id)
}

// DeleteVideoPermanently is the only path that removes the database row and
// everything generated for it. It never touches the source video or the WebDAV
// file.
func (l *Library) DeleteVideoPermanently(id int64) error {
	if err := l.videos.Remove(id); err != nil {
		return err
	}
	if err := l.groups.ClearVideoMembers(id); err != nil {
		l.log.Warn("clear video group members failed", "id", id, "err", err)
	}
	if l.jav != nil {
		_ = l.jav.Delete(id)
	}
	return nil
}

// ListTrash returns the recycle bin, newest removal first, together with the
// current retention. Group names are a best-effort projection: a membership
// read failure must not hide the trash.
func (l *Library) ListTrash() (TrashSnapshot, error) {
	videos, err := l.videos.ListTrash()
	if err != nil {
		return TrashSnapshot{}, err
	}
	// A trashed favorite still shows under its scraped 番号 (or source name for
	// a cloud favorite), exactly as it did in the library.
	l.attachReadModel(videos)
	for i := range videos {
		// JSON null would make the client's group list non-iterable; an empty
		// array is the contract for "no groups".
		videos[i].TrashGroupNames = []string{}
	}
	snapshot := TrashSnapshot{
		Videos:        videos,
		RetentionDays: l.TrashRetentionDays(),
	}
	if len(videos) == 0 || l.groups == nil {
		return snapshot, nil
	}
	ids := make([]int64, 0, len(videos))
	for i := range videos {
		ids = append(ids, videos[i].ID)
	}
	names, err := l.groups.GroupNamesForVideos(ids)
	if err != nil {
		l.log.Warn("load trash group names failed", "err", err)
		return snapshot, nil
	}
	for i := range snapshot.Videos {
		snapshot.Videos[i].TrashGroupNames = names[snapshot.Videos[i].ID]
	}
	return snapshot, nil
}

// EmptyTrash permanently deletes every trashed favorite. Per-item failures are
// logged and left for the next sweep rather than stopping the whole bin.
func (l *Library) EmptyTrash() (int, error) {
	videos, err := l.videos.ListTrash()
	if err != nil {
		return 0, err
	}
	purged := 0
	for _, v := range videos {
		if err := l.DeleteVideoPermanently(v.ID); err != nil {
			l.log.Warn("empty trash: delete failed", "id", v.ID, "err", err)
			continue
		}
		purged++
	}
	if purged > 0 {
		l.log.Info("trash emptied", "count", purged)
	}
	return purged, nil
}

// TrashRetentionDays returns the effective retention, clamped to the supported
// range. A broken provider or an out-of-range preference falls back to the
// default rather than shortening or extending the policy silently.
func (l *Library) TrashRetentionDays() int {
	days := DefaultTrashRetentionDays
	if l.trashRetention != nil {
		days = l.trashRetention()
	}
	if !ValidTrashRetentionDays(days) {
		return DefaultTrashRetentionDays
	}
	return days
}

// ValidTrashRetentionDays reports whether days is an allowed retention.
func ValidTrashRetentionDays(days int) bool {
	return days >= MinTrashRetentionDays && days <= MaxTrashRetentionDays
}

// SetTrashRetention installs the settings reader behind the retention policy.
// It is called once at startup, before the sweep can run.
func (l *Library) SetTrashRetention(read func() int) {
	l.trashRetention = read
}

// PurgeExpiredTrash permanently deletes the trashed favorites whose retention
// has elapsed. It is safe to call repeatedly and while the app is running: the
// list is re-read from the database and each deletion is idempotent.
func (l *Library) PurgeExpiredTrash() (int, error) {
	videos, err := l.videos.ListTrash()
	if err != nil {
		return 0, err
	}
	now := l.now()
	cutoff := now - int64(l.TrashRetentionDays())*24*60*60*1000
	purged := 0
	for _, v := range videos {
		if v.DeletedAt <= 0 || v.DeletedAt > cutoff {
			continue
		}
		if err := l.DeleteVideoPermanently(v.ID); err != nil {
			l.log.Warn("trash purge: delete failed", "id", v.ID, "err", err)
			continue
		}
		purged++
	}
	if purged > 0 {
		l.log.Info("trash purged expired favorites", "count", purged, "retentionDays", l.TrashRetentionDays())
	}
	return purged, nil
}

// StartTrashPurge runs the retention sweep now and then on every interval.
// Retroactive changes to the retention are therefore picked up on the next
// tick; App.SetTrashRetentionDays additionally triggers one immediate pass.
// A non-positive interval uses one hour.
func (l *Library) StartTrashPurge(every time.Duration) (stop func()) {
	if every <= 0 {
		every = time.Hour
	}
	done := make(chan struct{})
	var once sync.Once
	go func() {
		if n, err := l.PurgeExpiredTrash(); err != nil {
			l.log.Warn("initial trash purge failed", "err", err)
		} else if n > 0 {
			l.log.Info("initial trash purge removed entries", "count", n)
		}
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if n, err := l.PurgeExpiredTrash(); err != nil {
					l.log.Warn("trash purge failed", "err", err)
				} else if n > 0 {
					l.log.Info("trash purge removed entries", "count", n)
				}
			}
		}
	}()
	return func() { once.Do(func() { close(done) }) }
}

// RemoveOrphanedImages deletes the generated thumbnails and scraped covers that
// no favorite refers to any more. It is the shared maintenance entry point: the
// desktop calls it in the background at startup, and the HTTP API exposes it so a
// headless LAN deployment can sweep the same folders.
func (l *Library) RemoveOrphanedImages() (video.ImageSweepResult, error) {
	return l.videos.RemoveOrphanedImages()
}

// VideoJav returns the scraped metadata for one favorite.
func (l *Library) VideoJav(videoID int64) (*jav.Detail, error) {
	if l.jav == nil {
		return nil, fmt.Errorf("jav service not configured")
	}
	return l.jav.Get(videoID)
}

func (l *Library) VideoJavTask(videoID int64) (*jav.Task, error) {
	if l.jav == nil {
		return nil, fmt.Errorf("jav service not configured")
	}
	return l.jav.GetTask(videoID)
}

// ListJavTaskOverviews returns the latest persisted scrape task for every
// video, ordered by task creation time (newest first).
func (l *Library) ListJavTaskOverviews() ([]jav.TaskOverview, error) {
	if l.jav == nil {
		return nil, fmt.Errorf("jav service not configured")
	}
	overviews, err := l.jav.ListTaskOverviews()
	if err != nil {
		return nil, err
	}
	codes, restored, sources, backups := l.scrapedCodes(), l.restoredVideoIDs(), l.cloudSources(), l.backedUpKeys()
	for i := range overviews {
		applyReadModel(&overviews[i].Video, codes, restored, sources, backups)
	}
	return overviews, nil
}

// ListJavDuplicates returns the recognized works (番号) the library holds more
// than one copy of — the 查重 worklist. Copies sharing a 番号 but carrying
// different part markers ("ABC-123-1" vs "ABC-123-2") are separate episodes,
// not duplicates, and are left out. Groups the user chose to keep whole are
// omitted unless includeIgnored is set.
//
// The read reconciles the versions it is about to show: a file confirmed gone
// on an attached volume is removed as a favorite (through RemoveVideo, with the
// same volume-trust guard as startup), and the worklist is rebuilt. The panel is
// where the user learns that a copy is gone, so it must not keep offering to
// "keep" a version whose file no longer exists; versions that cannot be removed
// safely (detached drive, unreadable file, untrusted volume) stay listed.
func (l *Library) ListJavDuplicates(includeIgnored bool) ([]jav.DuplicateGroup, error) {
	if l.jav == nil {
		return nil, fmt.Errorf("jav service not configured")
	}
	groups, err := l.jav.ListDuplicates(includeIgnored)
	if err != nil {
		return nil, err
	}
	removed, err := l.videos.ReconcileMissing(duplicateMemberIDs(groups))
	if err != nil {
		l.log.Warn("reconcile missing duplicate versions failed", "err", err)
		l.attachDuplicateReadModel(groups)
		return groups, nil
	}
	if len(removed) > 0 {
		// Membership changed, so group sizes and dismissal signatures must be
		// recomputed. Rebuilding also drops a group that is no longer a
		// duplicate.
		groups, err = l.jav.ListDuplicates(includeIgnored)
		if err != nil {
			return nil, err
		}
	}
	l.attachDuplicateReadModel(groups)
	return groups, nil
}

// attachDuplicateReadModel fills the read-model fields on every version in a
// 番号 worklist.
func (l *Library) attachDuplicateReadModel(groups []jav.DuplicateGroup) {
	if len(groups) == 0 {
		return
	}
	codes, restored, sources, backups := l.scrapedCodes(), l.restoredVideoIDs(), l.cloudSources(), l.backedUpKeys()
	for i := range groups {
		for j := range groups[i].Videos {
			applyReadModel(&groups[i].Videos[j], codes, restored, sources, backups)
		}
	}
}

// duplicateMemberIDs collects every video id the worklist shows, once.
func duplicateMemberIDs(groups []jav.DuplicateGroup) []int64 {
	ids := make([]int64, 0)
	seen := make(map[int64]bool)
	for _, group := range groups {
		for _, v := range group.Videos {
			if seen[v.ID] {
				continue
			}
			seen[v.ID] = true
			ids = append(ids, v.ID)
		}
	}
	return ids
}

// IgnoreJavDuplicate keeps every copy in one duplicate group, silencing its
// 查重 prompt until the group's membership changes (等价于忽略重复提示).
func (l *Library) IgnoreJavDuplicate(key string) error {
	if l.jav == nil {
		return fmt.Errorf("jav service not configured")
	}
	return l.jav.IgnoreDuplicate(key)
}

// ListContentDuplicates returns the favorites that hold byte-identical files —
// the content dedup worklist. It answers a different question than the 番号
// worklist above: that one groups the different encodes of one work, this one
// groups the same bytes stored twice under another name or on another drive, so
// the same pair may honestly appear in both. Groups the user chose to keep whole
// (保留全部) are omitted unless includeIgnored is set.
func (l *Library) ListContentDuplicates(includeIgnored bool) ([]video.ContentDuplicateGroup, error) {
	groups, err := l.videos.ListContentDuplicates(includeIgnored)
	if err != nil {
		return nil, err
	}
	if len(groups) > 0 {
		codes, restored, sources, backups := l.scrapedCodes(), l.restoredVideoIDs(), l.cloudSources(), l.backedUpKeys()
		for i := range groups {
			for j := range groups[i].Members {
				applyReadModel(&groups[i].Members[j].Video, codes, restored, sources, backups)
			}
		}
	}
	return groups, nil
}

// IgnoreContentDuplicate keeps every member of one content-duplicate group
// (保留全部), silencing it until the group's membership changes.
func (l *Library) IgnoreContentDuplicate(hash string) error {
	return l.videos.IgnoreContentDuplicate(hash)
}

// ReverifyContentDuplicate re-reads the fingerprints of a group's members, which
// is how a group whose files changed becomes actionable again.
func (l *Library) ReverifyContentDuplicate(hash string) error {
	return l.videos.ReverifyContentDuplicate(hash)
}

// ConfirmContentDuplicate reads every member of a group in full, turning the
// sampled verdict into a byte-for-byte one before the user acts on it.
func (l *Library) ConfirmContentDuplicate(hash string) error {
	return l.videos.ConfirmContentDuplicate(hash)
}

// ScanContentDuplicates starts the library-wide fingerprint pass and returns its
// initial status; calling it while a pass runs joins that pass.
func (l *Library) ScanContentDuplicates() (video.ContentScanStatus, error) {
	return l.videos.ScanContentDuplicates()
}

// ContentScanStatus returns the current or last fingerprint pass.
func (l *Library) ContentScanStatus() video.ContentScanStatus {
	return l.videos.ContentScanStatus()
}

// ListActors returns the aggregated cast index (演员): one entry per actor with
// the number of favorites they appear in, derived from the scraped metadata.
func (l *Library) ListActors() ([]jav.ActorSummary, error) {
	if l.jav == nil {
		return nil, fmt.Errorf("jav service not configured")
	}
	return l.jav.ListActors()
}

// VideosByActor returns the full favorites whose scraped cast includes name, in
// the same shape and order as ListVideos so the grid can render it directly.
// Merged stage names (演员合并) are included, so one actor's aliases resolve to
// a single film list.
func (l *Library) VideosByActor(name string) ([]video.Video, error) {
	if l.jav == nil {
		return nil, fmt.Errorf("jav service not configured")
	}
	ids, err := l.jav.VideoIDsByActor(name)
	if err != nil {
		return nil, err
	}
	all, err := l.videos.List()
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]video.Video, len(all))
	for _, v := range all {
		byID[v.ID] = v
	}
	out := make([]video.Video, 0, len(ids))
	for _, id := range ids {
		if v, ok := byID[id]; ok {
			out = append(out, v)
		}
	}
	l.attachReadModel(out)
	return out, nil
}

// MergeActors merges source into target so the two stage names are shown and
// browsed as one actor (演员合并).
func (l *Library) MergeActors(source, target string) error {
	if l.jav == nil {
		return fmt.Errorf("jav service not configured")
	}
	return l.jav.MergeActors(source, target)
}

// UnmergeActor splits alias back out into its own actor (解除合并).
func (l *Library) UnmergeActor(alias string) error {
	if l.jav == nil {
		return fmt.Errorf("jav service not configured")
	}
	return l.jav.UnmergeActor(alias)
}

// ActorProfile returns one actor's scraped profile (头像 / 生日 / 三围 …),
// resolving merged aliases. No profile is not an error: the actor may simply
// not have been enriched yet.
func (l *Library) ActorProfile(name string) (*jav.ActorProfile, error) {
	if l.jav == nil {
		return nil, fmt.Errorf("jav service not configured")
	}
	return l.jav.ActorProfile(name)
}

// StartActorFill queues a profile backfill (补全演员信息) for every actor that
// has none yet and returns how many names were newly queued. It is deliberately
// fire-and-forget: the run happens in the background and the actor index follows
// its progress through ActorFillStatus.
func (l *Library) StartActorFill() (int, error) {
	if l.jav == nil {
		return 0, fmt.Errorf("jav service not configured")
	}
	return l.jav.StartActorFill()
}

// ActorFillStatus reports the background profile backfill's progress.
func (l *Library) ActorFillStatus() (jav.ActorFillStatus, error) {
	if l.jav == nil {
		return jav.ActorFillStatus{}, fmt.Errorf("jav service not configured")
	}
	return l.jav.ActorFillStatus()
}

func (l *Library) ScrapeVideoJav(videoID int64, force bool) error {
	if l.jav == nil {
		return fmt.Errorf("jav service not configured")
	}
	if force {
		l.jav.EnqueueForce(videoID)
	} else {
		l.jav.Enqueue(videoID)
	}
	return nil
}

// ScrapeVideosJav schedules a library-wide asynchronous scrape pass. force
// resets successful tasks too; explicitly skipped videos remain skipped.
func (l *Library) ScrapeVideosJav(force bool) (int, error) {
	if l.jav == nil {
		return 0, fmt.Errorf("jav service not configured")
	}
	videos, err := l.videos.List()
	if err != nil {
		return 0, err
	}
	ids := make([]int64, 0, len(videos))
	for _, v := range videos {
		ids = append(ids, v.ID)
	}
	return l.jav.EnqueueVideos(ids, !force)
}

func (l *Library) SetVideoJavOverride(videoID int64, mode, code string) error {
	if l.jav == nil {
		return fmt.Errorf("jav service not configured")
	}
	return l.jav.SetOverride(videoID, mode, code)
}

// CreateGroup makes a new empty group.
func (l *Library) CreateGroup(name string) (*group.Group, error) {
	return l.groups.Create(name)
}

// RenameGroup updates a group's name.
func (l *Library) RenameGroup(id int64, name string) (*group.Group, error) {
	return l.groups.Rename(id, name)
}

// DeleteGroup removes a group and its memberships (videos are kept).
func (l *Library) DeleteGroup(id int64) error {
	return l.groups.Delete(id)
}

// SetGroupParent makes one group a child of another (or detaches it to the top
// level when parentID is nil). It rejects self-parenting and cycles.
func (l *Library) SetGroupParent(id int64, parentID *int64) (*group.Group, error) {
	return l.groups.SetParent(id, parentID)
}

// ReorderGroups persists the sidebar drag & drop ordering.
func (l *Library) ReorderGroups(ids []int64) error {
	return l.groups.Reorder(ids)
}

// SetVideoGroups assigns a video to the given groups (replacing previous ones).
func (l *Library) SetVideoGroups(videoID int64, groupIDs []int64) error {
	return l.groups.SetVideoGroups(videoID, groupIDs)
}

// AddVideoToGroup adds one video to one group (idempotent).
func (l *Library) AddVideoToGroup(videoID, groupID int64) error {
	return l.groups.AddVideoToGroup(videoID, groupID)
}

// ---- Video info editing (编辑影片信息) ----
//
// Thin passthroughs: no cross-service orchestration is needed today, but the
// methods live on the façade so both transports (and future callers) keep one
// entry point per the repo convention.

// UpdateVideoInfo saves a custom title/notes for a favorite. An empty title
// means the UI keeps showing the scraped 番号, or the file name without one.
func (l *Library) UpdateVideoInfo(id int64, title, notes string) (*video.Video, error) {
	v, err := l.videos.UpdateInfo(id, title, notes)
	if err != nil {
		return nil, err
	}
	l.attachReadModelOne(v)
	return v, nil
}

// LikeVideo adds one like to a favorite's counter (repeated likes stack) and
// returns the updated record.
func (l *Library) LikeVideo(id int64) (*video.Video, error) {
	v, err := l.videos.LikeVideo(id)
	if err != nil {
		return nil, err
	}
	l.attachReadModelOne(v)
	return v, nil
}

// ClearVideoLikes resets a favorite's like counter to zero ("清空点赞数").
func (l *Library) ClearVideoLikes(id int64) (*video.Video, error) {
	v, err := l.videos.ClearVideoLikes(id)
	if err != nil {
		return nil, err
	}
	l.attachReadModelOne(v)
	return v, nil
}

// SetVideoCover extracts the frame at atSec (seconds) as the favorite's cover
// and marks it custom, protecting it from automatic regeneration.
func (l *Library) SetVideoCover(id int64, atSec float64) (*video.Video, error) {
	v, err := l.videos.SetCover(id, atSec)
	if err != nil {
		return nil, err
	}
	l.attachReadModelOne(v)
	return v, nil
}

// ResetVideoCover reverts the cover to the automatically generated thumbnail.
func (l *Library) ResetVideoCover(id int64) (*video.Video, error) {
	v, err := l.videos.ResetCover(id)
	if err != nil {
		return nil, err
	}
	l.attachReadModelOne(v)
	return v, nil
}

// SetVideoScrapedCoverAllowed stores the per-video preference for allowing a
// scraped cover to replace the current cover.
func (l *Library) SetVideoScrapedCoverAllowed(id int64, allowed bool) (*video.Video, error) {
	v, err := l.videos.SetScrapedCoverAllowed(id, allowed)
	if err != nil {
		return nil, err
	}
	l.attachReadModelOne(v)
	return v, nil
}

// ---- Cloud sources (WebDAV) ----

// ListCloudConnections returns the configured sources. It never touches the
// network; the settings page can open instantly and the list shows the last
// observed health.
func (l *Library) ListCloudConnections() ([]cloud.ConnectionView, error) {
	if l.cloud == nil {
		return []cloud.ConnectionView{}, nil
	}
	return l.cloud.ListConnections()
}

// SaveCloudConnection creates or updates a source. The password is protected
// before it reaches SQLite; the returned view never contains it.
func (l *Library) SaveCloudConnection(input cloud.ConnectionInput) (*cloud.ConnectionView, error) {
	if l.cloud == nil {
		return nil, fmt.Errorf("网盘服务未初始化")
	}
	return l.cloud.SaveConnection(input)
}

// DeleteCloudConnection removes a source and refuses while favorites still
// point at it.
func (l *Library) DeleteCloudConnection(id int64) error {
	if l.cloud == nil {
		return fmt.Errorf("网盘服务未初始化")
	}
	return l.cloud.DeleteConnection(id)
}

// TestCloudConnection probes the form values (or the stored row) and reports a
// readable result instead of throwing for the expected failures.
func (l *Library) TestCloudConnection(input cloud.ConnectionInput) (cloud.TestResult, error) {
	if l.cloud == nil {
		return cloud.TestResult{Error: "网盘服务未初始化"}, nil
	}
	return l.cloud.TestConnection(input)
}

// BrowseCloud lists one remote collection (directories first).
func (l *Library) BrowseCloud(id int64, path string) ([]cloud.Entry, error) {
	if l.cloud == nil {
		return nil, fmt.Errorf("网盘服务未初始化")
	}
	return l.cloud.Browse(id, path)
}

// CloudStorageAvailable reports whether a WebDAV password can be protected on
// this platform. Anonymous sources remain usable when it cannot.
func (l *Library) CloudStorageAvailable() bool {
	return l.cloud != nil && l.cloud.StorageAvailable()
}

// AddCloudVideos validates each selected remote path and adds it as a
// read-only favorite. Stat is the authoritative source of the display name and
// size; the browser's snapshot may be stale. One failure never blocks the rest.
func (l *Library) AddCloudVideos(sourceID int64, paths []string) []video.AddVideoResult {
	results := make([]video.AddVideoResult, 0, len(paths))
	for _, p := range paths {
		result := video.AddVideoResult{Path: p}
		if l.cloud == nil {
			result.Error = "网盘服务未初始化"
			results = append(results, result)
			continue
		}
		entry, err := l.cloud.Stat(sourceID, p)
		if err != nil {
			result.Error = cloud.UserMessage(err)
			results = append(results, result)
			continue
		}
		if entry.IsDir {
			result.Error = "文件夹不能直接加入收藏"
			results = append(results, result)
			continue
		}
		v, _, err := l.videos.AddRemoteVideo(sourceID, entry.Path, entry.Name, entry.Size)
		if err != nil {
			result.Error = video.UserMessage(err)
			results = append(results, result)
			continue
		}
		result.Success = true
		result.VideoID = v.ID
		if l.jav != nil {
			l.jav.Enqueue(v.ID)
		}
		results = append(results, result)
	}
	return results
}

// CloudPageURL returns the OpenList file page for a remote favorite. Opening it
// hands playback to OpenList; Muse never streams or stores the video.
func (l *Library) CloudPageURL(videoID int64) (string, error) {
	if l.cloud == nil {
		return "", fmt.Errorf("网盘服务未初始化")
	}
	v, err := l.videos.Get(videoID)
	if err != nil {
		return "", err
	}
	if !v.IsRemote() {
		return "", fmt.Errorf("该视频不在网盘上")
	}
	return l.cloud.FileURL(v.SourceID, v.RemotePath)
}

// ArchiveCloudConnection retires one cloud source and moves its active
// favorites into the recycle bin with reason archived. It is deliberately not
// reversible in bulk: enabling the source again leaves the archive untouched.
func (l *Library) ArchiveCloudConnection(id int64) (int, error) {
	if l.cloud == nil {
		return 0, fmt.Errorf("网盘服务未初始化")
	}
	view, err := l.cloud.Connection(id)
	if err != nil {
		return 0, err
	}
	if !view.Enabled {
		return 0, nil
	}
	if err := l.cloud.DisableConnection(id); err != nil {
		return 0, err
	}
	n, err := l.videos.ArchiveBySource(id, view.Name)
	if err != nil {
		// Do not leave a disabled source with active favorites if the archive
		// update failed; the user's intent is atomic.
		_ = l.cloud.EnableConnection(id)
		return 0, err
	}
	return n, nil
}

// EnableCloudConnection makes a retired source usable again. Archived favorites
// stay archived; the user must restore them manually.
func (l *Library) EnableCloudConnection(id int64) error {
	if l.cloud == nil {
		return fmt.Errorf("网盘服务未初始化")
	}
	return l.cloud.EnableConnection(id)
}

// ListArchived returns the long-lived archived part of the recycle bin.
func (l *Library) ListArchived() ([]video.Video, error) {
	videos, err := l.videos.ListArchived()
	if err != nil {
		return nil, err
	}
	l.attachReadModel(videos)
	// The trash UI shows original group names as context.
	ids := make([]int64, 0, len(videos))
	for i := range videos {
		videos[i].TrashGroupNames = []string{}
		ids = append(ids, videos[i].ID)
	}
	if l.groups != nil && len(ids) > 0 {
		if names, err := l.groups.GroupNamesForVideos(ids); err == nil {
			for i := range videos {
				videos[i].TrashGroupNames = names[videos[i].ID]
			}
		}
	}
	return videos, nil
}

// ClearArchivedVideos permanently deletes every archived row. This is the
// explicit "clear archive" action; normal trash retention never touches it.
func (l *Library) ClearArchivedVideos() (int, error) {
	videos, err := l.videos.ListArchived()
	if err != nil {
		return 0, err
	}
	return l.deleteArchived(videos)
}

// ClearArchivedSource permanently deletes the archive of one source.
func (l *Library) ClearArchivedSource(sourceID int64) (int, error) {
	videos, err := l.videos.ListArchived()
	if err != nil {
		return 0, err
	}
	filtered := videos[:0]
	for _, v := range videos {
		if v.SourceID == sourceID {
			filtered = append(filtered, v)
		}
	}
	return l.deleteArchived(filtered)
}

func (l *Library) deleteArchived(videos []video.Video) (int, error) {
	deleted := 0
	for _, v := range videos {
		if err := l.DeleteVideoPermanently(v.ID); err != nil {
			l.log.Warn("clear archive: delete failed", "id", v.ID, "err", err)
			continue
		}
		deleted++
	}
	return deleted, nil
}

// ---- Group suggestions (auto-detected after adding videos) ----

// GroupSuggestion is one recommended group for a just-added video.
type GroupSuggestion struct {
	GroupID int64  `json:"groupId"`
	Name    string `json:"name"`
}

// VideoGroupSuggestion is the per-file group recommendation shown after adding.
// A file with no matching groups has an empty Suggested slice.
type VideoGroupSuggestion struct {
	Path      string            `json:"path"`
	FileName  string            `json:"fileName"`
	Suggested []GroupSuggestion `json:"suggested"`
}

// matchGroupRules scores a file name against the automatic group rules shared
// by both the suggestion flow and the silent auto-tag flow, so the two can
// never drift. Rules:
//   - "restored" in the name (case-insensitive) → "马赛克破解". The group is
//     ensured (created on first use) as a deliberate side effect.
//   - any existing group's name occurring in the file's base name
//     (case-insensitive substring) → that group.
//
// It skips groups already produced by an earlier rule (dedup by group id) and
// yields suggestions in sidebar (display) order.
func (l *Library) matchGroupRules(fileName string, groups []group.Group, ensureRestored func() *group.Group) []GroupSuggestion {
	base := strings.ToLower(fileName)
	suggested := make([]GroupSuggestion, 0)
	seen := make(map[int64]struct{})
	if strings.Contains(base, "restored") || strings.Contains(base, "ltx") || strings.Contains(base, "LTX") {
		if g := ensureRestored(); g != nil {
			suggested = append(suggested, GroupSuggestion{GroupID: g.ID, Name: g.Name})
			seen[g.ID] = struct{}{}
		} else {
			l.log.Warn("group rules: ensure restored group failed")
		}
	}
	for _, g := range groups {
		if g.Name == "" {
			continue
		}
		if _, dup := seen[g.ID]; dup {
			continue
		}
		if strings.Contains(base, strings.ToLower(g.Name)) {
			suggested = append(suggested, GroupSuggestion{GroupID: g.ID, Name: g.Name})
		}
	}
	return suggested
}

// SuggestGroupsForVideos infers recommended groups for a batch of just-added
// paths purely from the file name (spec: resolution-based grouping is automatic
// and async, so only the group-name substring rule is suggested here). For each
// path, every existing group whose name is a case-insensitive substring of the
// file's base name becomes a suggestion. Suggestions reference real, persisted
// groups — the frontend only needs to confirm them.
func (l *Library) SuggestGroupsForVideos(paths []string) []VideoGroupSuggestion {
	out := make([]VideoGroupSuggestion, 0, len(paths))
	if len(paths) == 0 {
		return out
	}
	groups, err := l.groups.List()
	if err != nil {
		// Best effort: on failure we simply offer no suggestions.
		l.log.Warn("suggest groups: list groups failed", "err", err)
		groups = nil
	}
	ensureRestored := l.ensureRestoredGroup()

	for _, p := range paths {
		fileName := filepath.Base(p)
		suggested := l.matchGroupRules(fileName, groups, ensureRestored)
		// Deterministic display order follows the sidebar (already sorted).
		out = append(out, VideoGroupSuggestion{Path: p, FileName: fileName, Suggested: suggested})
	}
	return out
}

// AutoTagVideos silently applies the automatic group rules to freshly imported
// videos. It is the folder-watch path: scans land here so auto-imported files
// are classified exactly like manual adds (same rules), but without a
// suggestion dialog — membership is applied immediately and idempotently, the
// same way resolution-based 4K grouping already works.
//
// A relocated result is skipped: those bytes were imported once already, and
// with them these same rules, so re-applying them would only undo a membership
// the user has since removed.
func (l *Library) AutoTagVideos(results []video.AddVideoResult) {
	added := make([]string, 0, len(results))
	byPath := make(map[string]int64, len(results))
	for _, r := range results {
		if r.Success && r.VideoID > 0 && !r.Relocated {
			added = append(added, r.Path)
			byPath[r.Path] = r.VideoID
		}
	}
	if len(added) == 0 {
		return
	}
	for _, s := range l.SuggestGroupsForVideos(added) {
		id, ok := byPath[s.Path]
		if !ok {
			continue
		}
		for _, g := range s.Suggested {
			if err := l.groups.AddVideoToGroup(id, g.GroupID); err != nil {
				l.log.Warn("auto-tag: add to group failed", "videoId", id, "path", s.Path, "group", g.Name, "err", err)
				continue
			}
			l.log.Info("auto-tag applied", "videoId", id, "path", s.Path, "group", g.Name)
		}
	}
}

// ensureRestoredGroup lazily ensures the "马赛克破解" group exists (bound once
// per call batch, so a batch of restored files reuses the same group).
func (l *Library) ensureRestoredGroup() func() *group.Group {
	var restoredGroup *group.Group
	ready := false
	return func() *group.Group {
		if !ready {
			restoredGroup, _ = l.groups.EnsureGroup(mosaicRestoredGroup)
			ready = true
		}
		return restoredGroup
	}
}

// EnsureVideoIn4KGroup auto-assigns a just-probed 4K (3840×2160) video to the
// "4K" group, creating that group on first use. It is invoked asynchronously by
// the background processor once the resolution is known, so it never blocks or
// shows a suggestion dialog.
func (l *Library) EnsureVideoIn4KGroup(videoID int64) {
	v, err := l.videos.Get(videoID)
	if err != nil {
		l.log.Warn("auto 4K: video unavailable", "id", videoID, "err", err)
		return
	}
	if v.Width != 3840 || v.Height != 2160 {
		return
	}
	g, err := l.groups.EnsureGroup("4K")
	if err != nil {
		l.log.Warn("auto 4K: ensure group failed", "id", videoID, "err", err)
		return
	}
	if err := l.groups.AddVideoToGroup(videoID, g.ID); err != nil {
		l.log.Warn("auto 4K: add to group failed", "id", videoID, "err", err)
		return
	}
	l.log.Info("4K video auto-grouped", "id", videoID, "group", g.ID)
}
