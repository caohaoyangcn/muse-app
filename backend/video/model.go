// Package video contains the favorite model, persistence and business logic.
package video

// Status values for the videos table.
const (
	StatusAvailable = "available"
	StatusMissing   = "missing"
)

// Reasons a favorite was moved to the recycle bin. user means the user removed
// it from the library; missing means automatic cleanup found the file confirmed
// gone on an attached volume. Both keep the record restorable for the retention
// period (see docs/spec/trash.md).
const (
	TrashReasonUser     = "user"
	TrashReasonMissing  = "missing"
	TrashReasonArchived = "archived"
)

// Video is a single favorite. It only references the file on disk;
// the app never owns the media itself (spec-v1.md §6, §41).
type Video struct {
	ID         int64   `json:"id"`
	Path       string  `json:"path"`
	FileName   string  `json:"fileName"`
	FileSize   int64   `json:"fileSize"`
	DurationMs int64   `json:"durationMs"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	FrameRate  float64 `json:"frameRate"` // frames per second; 0 when unknown
	Bitrate    int64   `json:"bitrate"`   // bits per second; 0 when unknown
	// VideoCodec / AudioCodec are the container's stream codec names (ffprobe
	// codec_name), e.g. "h264"/"hevc" and "aac"/"ac3". They are displayed in the
	// library and passed through to clients that choose their own playback stack.
	// An empty AudioCodec is normal (no audio track);
	// empty VideoCodec means "not probed yet".
	VideoCodec    string `json:"videoCodec"`
	AudioCodec    string `json:"audioCodec"`
	ThumbnailPath string `json:"thumbnailPath"` // relative to data dir, e.g. "thumbnails/12.jpg"; empty while generating/failed
	CreatedAt     int64  `json:"createdAt"`
	UpdatedAt     int64  `json:"updatedAt"`
	LastOpenedAt  int64  `json:"lastOpenedAt"`
	Status        string `json:"status"`
	// VolumeOffline reports that the volume holding Path is not attached right
	// now (the user unplugged an external drive). The favorite is not missing:
	// its file is on a disk that is not plugged in, so it keeps its thumbnail,
	// metadata and place in the grid, and only carries a warning. Observed per
	// volume, never persisted — see volume.go.
	VolumeOffline bool `json:"volumeOffline"`
	// VolumeRoot is the mount point Path lives on ("D:\", "\\server\share\",
	// "/"). Read paths fill it; it names the disk VolumeOffline is about.
	VolumeRoot string `json:"volumeRoot"`
	// Editable info (spec §41 reserved fields). Display uses Title when set,
	// then JavCode, falling back to FileName.
	Title string `json:"title"`
	Notes string `json:"notes"`
	// JavCode is the authoritative 番号 the scraper recorded for this favorite
	// (video_jav.code). It is a read-model projection filled by the shared
	// façade, never a column on videos: empty until a scrape has produced a
	// code. Once set, the UI presents the favorite under it instead of the file
	// name, so a messy file name never stands in for the work's identity.
	JavCode string `json:"javCode"`
	// Actors is the canonical cast (演员) the scraper recorded for this favorite,
	// with 演员合并 aliases folded into their canonical name. Like JavCode it is a
	// read-model projection filled by the shared façade from
	// video_jav.actors_json, never a column on videos; empty until a scrape has
	// produced a cast. The workspace uses it to group open video tabs by actor.
	Actors []string `json:"actors"`
	// MosaicRestored reports membership in the 马赛克破解 group — the collection
	// rule that tags restored/mosaic-removed works. It is a read-model
	// projection filled by the shared façade (transitive membership included),
	// never a column on videos; the UI shows it as a tag beside the name.
	MosaicRestored bool `json:"mosaicRestored"`
	// CustomCoverAt is the unix-milli moment the user picked a cover frame.
	// 0 means the current cover is not a user-picked frame. A user-picked
	// frame can still be replaced by a scraped cover when AllowScrapedCover is
	// true.
	CustomCoverAt int64 `json:"customCoverAt"`
	// LikeCount is how many times the user has liked this favorite. Repeated
	// likes stack (spec: likes are a property of the favorite, not the file).
	LikeCount         int64  `json:"likeCount"`
	CoverSource       string `json:"coverSource"`
	ScrapedCoverPath  string `json:"scrapedCoverPath"`
	AllowScrapedCover bool   `json:"allowScrapedCover"`
	// ContentHash is the partial-hash fingerprint of the file (see
	// docs/spec/content-hash-dedup.md): a digest over the file's size plus its
	// first and last megabyte. It identifies byte-identical copies without
	// reading whole multi-gigabyte files. Empty means "not computed yet".
	ContentHash string `json:"contentHash"`
	// HashVersion is the recipe that produced ContentHash; a value below
	// FingerprintVersion is stale and has to be recomputed.
	HashVersion int `json:"hashVersion"`
	// HashState is the verification bookkeeping behind the fingerprint: the
	// on-disk facts it was computed from, the last failure, and the optional
	// full-file confirmation. Deliberately not serialized — clients only need to
	// know whether a fingerprint exists, not how it was verified.
	HashState FingerprintState `json:"-"`

	// SourceID is the cloud_connections row this favorite lives on. 0 means a
	// local file; >0 means a read-only WebDAV source. Remote videos never
	// participate in local disk scanning, relocation or content fingerprints.
	SourceID int64 `json:"sourceId"`
	// RemotePath is the absolute, percent-decoded WebDAV path. It is the
	// authoritative location for a remote favorite; Path only carries a
	// synthetic stable key (see remotePathKey) and must not be opened.
	RemotePath string `json:"remotePath"`
	// SourceName is a read-model projection: the connection's display name.
	// Empty for local favorites.
	SourceName string `json:"sourceName"`
	// SourceOffline is a read-model projection: the connection failed its most
	// recent health probe. It is never persisted on the video row.
	SourceOffline bool `json:"sourceOffline"`
	// SourceEnabled is a read-model projection: the cloud connection is still
	// active. Archived rows whose source was deleted report false.
	SourceEnabled bool `json:"sourceEnabled"`
	// BackedUp is a read-model projection: the same work (番号 + part) has an
	// active available copy on both a local disk and a cloud source. It is a
	// work-level backup marker, not a byte-identical claim.
	BackedUp bool `json:"backedUp"`
	// CloudURL is the OpenList file page for a remote favorite. Playback is
	// delegated to OpenList, so the desktop and mobile clients only need this
	// URL; it is empty for local videos and when no web root is configured.
	CloudURL string `json:"cloudUrl"`

	// DeletedAt is the unix-milli moment the favorite was moved to the recycle
	// bin; 0 means it is active. A trashed row keeps its id, path and every
	// relation, so restoring it is a flag change rather than a rebuild.
	DeletedAt int64 `json:"deletedAt"`
	// DeleteReason is TrashReasonUser, TrashReasonMissing or TrashReasonArchived
	// for a trashed/archived row, empty while active. It only explains the move;
	// it never changes what the row owns.
	DeleteReason string `json:"deleteReason"`
	// ArchiveSourceName is the source-name snapshot written when a cloud source
	// is disabled. It keeps the archive readable after the connection row is
	// deleted.
	ArchiveSourceName string `json:"archiveSourceName"`
	// TrashGroupNames is a read-model projection filled only by the recycle-bin
	// read: the groups the favorite belonged to when it was moved to the trash.
	// Memberships are deliberately retained, so a restore shows the same groups
	// again; the field is never persisted on the row.
	TrashGroupNames []string `json:"trashGroupNames"`
}

// IsRemote reports whether the favorite lives on a cloud connection.
func (v *Video) IsRemote() bool { return v != nil && v.SourceID > 0 }

// FingerprintState is the bookkeeping behind a content fingerprint.
//
// Size and Mtime are what make a fingerprint trustworthy. The database's
// file_size is only written when a favorite is imported, so a file replaced
// outside the app would otherwise keep a fingerprint that still correlates with
// another copy — and the duplicate panel would offer to delete a file based on a
// verdict about different bytes. Any mismatch against a fresh stat means the
// verdict has to be re-established before it can be acted on.
type FingerprintState struct {
	// Size is the file size the fingerprint was computed from.
	Size int64
	// Mtime is the file modification time (unix nanoseconds) at that moment.
	// Nanoseconds, not seconds: a replacement within the same second would
	// otherwise look unchanged.
	Mtime int64
	// At is when the fingerprint was computed (unix millis).
	At int64
	// Error is the last failure reason; empty when a valid fingerprint exists.
	Error string
	// FullHash is the optional full-file SHA-256 confirmation (empty until the
	// user asks for one).
	FullHash string
	// FullHashAt is when FullHash was computed (unix millis).
	FullHashAt int64
}

// AddVideoResult reports the per-file outcome of a batch add (spec-v1.1.md §7).
type AddVideoResult struct {
	Path    string `json:"path"`
	Success bool   `json:"success"`
	VideoID int64  `json:"videoId,omitempty"`
	Error   string `json:"error,omitempty"`
	// Relocated is true when the file did not become a new favorite but
	// re-pointed an existing one whose file had moved: the library already held
	// these bytes under a path that no longer exists. The favorite kept its id,
	// so the UI reports a reconnection rather than an addition.
	Relocated bool `json:"relocated,omitempty"`
}
