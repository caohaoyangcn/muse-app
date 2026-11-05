package jav

import "muse-app/backend/video"

const (
	OverrideAuto   = "auto"
	OverrideManual = "manual"
	OverrideSkip   = "skip"

	StatusQueued    = "queued"
	StatusRunning   = "running"
	StatusSucceeded = "succeeded"
	StatusNoCode    = "no_code"
	StatusNotFound  = "not_found"
	StatusRetryable = "retryable"
	StatusBlocked   = "blocked"
	StatusDisabled  = "disabled"
	// StatusFailed is the terminal state of a task that used up its retry
	// budget. Retryable is a transient state, so exhaustion must land here:
	// a retryable task with no attempts left can never be claimed again and
	// would otherwise sit in the queue forever. A forced re-scrape revives it.
	StatusFailed = "failed"

	CoverNone      = "none"
	CoverPending   = "pending"
	CoverSucceeded = "succeeded"
	CoverFailed    = "failed"
)

const (
	ErrorNoCode          = "no_code"
	ErrorNotFound        = "not_found"
	ErrorBlocked         = "blocked"
	ErrorProviderAuth    = "provider_auth"
	ErrorNetwork         = "network"
	ErrorHTTP            = "http"
	ErrorRateLimited     = "rate_limited"
	ErrorParse           = "parse"
	ErrorCoverDownload   = "cover_download"
	ErrorProviderMissing = "provider_unavailable"
	ErrorVideoMissing    = "video_missing"
	ErrorDatabase        = "database"
	// ErrorActorProfile is a resolver failure that is neither a lookup miss nor
	// a storage problem: an unexpected provider response.
	ErrorActorProfile = "actor_profile"
)

type Metadata struct {
	Code         string
	Title        string
	Studio       string
	Series       string
	ReleaseUnix  *int64
	DurationMin  *int
	IsUncensored *bool
	Actors       []string
	// ActorProfiles carries the provider's profile for the cast members it
	// described, keyed by the same name used in Actors. A provider that only
	// returns names (or a scrape that found no profile) leaves it empty.
	ActorProfiles []ActorProfile
	Tags          []string
	CoverURL      string
	Provider      string
}

// ActorProfile is one performer's scraped profile: the avatar and the
// biographical facts the metadata provider publishes next to a work's cast.
// Name is the scraped display name and the key the actor index aggregates on;
// StarID is the provider's stable performer id, which survives stage-name
// changes and decides the avatar file name. Optional numbers stay nil when the
// provider leaves them out (anonymous performers, retiring performers).
type ActorProfile struct {
	Name       string `json:"name"`
	Provider   string `json:"provider,omitempty"`
	StarID     string `json:"starId,omitempty"`
	AvatarURL  string `json:"avatarUrl,omitempty"`
	AvatarPath string `json:"avatarPath,omitempty"`
	Birthday   string `json:"birthday,omitempty"`
	HeightCM   *int   `json:"heightCm,omitempty"`
	BustCM     *int   `json:"bustCm,omitempty"`
	Cup        string `json:"cup,omitempty"`
	WaistCM    *int   `json:"waistCm,omitempty"`
	HipCM      *int   `json:"hipCm,omitempty"`
	BloodType  string `json:"bloodType,omitempty"`
	Hometown   string `json:"hometown,omitempty"`
	Hobby      string `json:"hobby,omitempty"`
	MovieCount *int   `json:"movieCount,omitempty"`
	// LastReleaseUnix is the provider's newest release date for the performer,
	// kept as a hint that an inactive actor has genuinely retired.
	LastReleaseUnix *int64   `json:"lastReleaseUnix,omitempty"`
	Aliases         []string `json:"aliases,omitempty"`
	FetchedAt       *int64   `json:"fetchedAt,omitempty"`
}

// ActorFillTask is one actor name's state in the profile backfill queue.
type ActorFillTask struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	Attempts    int    `json:"attempts"`
	MaxAttempts int    `json:"maxAttempts"`
	ErrorCode   string `json:"errorCode"`
	LastError   string `json:"lastError"`
	NextRetryAt *int64 `json:"nextRetryAt,omitempty"`
	CreatedAt   int64  `json:"createdAt"`
	UpdatedAt   int64  `json:"updatedAt"`
	FinishedAt  *int64 `json:"finishedAt,omitempty"`
}

// ActorFillStatus is the read model the actor index shows while the backfill
// run is working: how each terminal state is distributed, the name being
// resolved right now, and whether any work is left.
type ActorFillStatus struct {
	Total     int    `json:"total"`
	Queued    int    `json:"queued"`
	Running   int    `json:"running"`
	Succeeded int    `json:"succeeded"`
	NotFound  int    `json:"notFound"`
	Failed    int    `json:"failed"`
	Current   string `json:"current,omitempty"`
	Active    bool   `json:"active"`
}

// ActorSummary is one actor aggregated across the scraped cast lists, with the
// number of favorites they appear in. The aggregate is derived at read time
// from video_jav.actors_json, so it needs no table of its own and can never
// drift from the scrape results that mention the actor.
//
// Aliases lists the other scraped names merged into this one (演员合并): Count
// already includes their films, and Name is the canonical name shown in the UI.
type ActorSummary struct {
	Name    string   `json:"name"`
	Count   int      `json:"count"`
	Aliases []string `json:"aliases,omitempty"`
	// Profile is the scraped profile of the canonical name, or of the first
	// merged alias that has one, so a merged actor keeps its avatar.
	Profile *ActorProfile `json:"profile,omitempty"`
}

// ActorFilmRef is one distinct (scraped name, favorite) pair: the raw material
// the actor index is aggregated from. Keeping the pair (rather than a per-name
// count) is what lets merged aliases be deduplicated, since one favorite can
// list the same performer under two stage names.
type ActorFilmRef struct {
	Name    string
	VideoID int64
}

// ActorMerge is one persisted alias → canonical mapping.
type ActorMerge struct {
	Alias     string `json:"alias"`
	Canonical string `json:"canonical"`
}

type Detail struct {
	VideoID      int64    `json:"videoId"`
	Code         string   `json:"code"`
	Title        string   `json:"title"`
	Studio       string   `json:"studio"`
	Series       string   `json:"series"`
	ReleaseUnix  *int64   `json:"releaseUnix,omitempty"`
	DurationMin  *int     `json:"durationMin,omitempty"`
	IsUncensored *bool    `json:"isUncensored,omitempty"`
	Actors       []string `json:"actors"`
	Tags         []string `json:"tags"`
	CoverURL     string   `json:"coverUrl"`
	CoverStatus  string   `json:"coverStatus"`
	Provider     string   `json:"provider"`
	CodeSource   string   `json:"codeSource"`
	OverrideMode string   `json:"overrideMode"`
	ScrapeStatus string   `json:"scrapeStatus"`
	Attempts     int      `json:"attempts"`
	LastError    string   `json:"lastError"`
	NextRetryAt  *int64   `json:"nextRetryAt,omitempty"`
	FetchedAt    *int64   `json:"fetchedAt,omitempty"`
	ErrorCode    string   `json:"errorCode"`
	ErrorAt      *int64   `json:"errorAt,omitempty"`
}

type Task struct {
	ID          int64  `json:"id"`
	VideoID     int64  `json:"videoId"`
	Status      string `json:"status"`
	Force       bool   `json:"force"`
	Attempts    int    `json:"attempts"`
	MaxAttempts int    `json:"maxAttempts"`
	NextRetryAt *int64 `json:"nextRetryAt,omitempty"`
	LeaseToken  string `json:"-"`
	LeaseUntil  *int64 `json:"leaseUntil,omitempty"`
	ErrorCode   string `json:"errorCode"`
	LastError   string `json:"lastError"`
	ErrorAt     *int64 `json:"errorAt,omitempty"`
	CreatedAt   int64  `json:"createdAt"`
	UpdatedAt   int64  `json:"updatedAt"`
	StartedAt   *int64 `json:"startedAt,omitempty"`
	FinishedAt  *int64 `json:"finishedAt,omitempty"`
}

const (
	TaskCategoryQueued    = "queued"
	TaskCategorySucceeded = "succeeded"
	TaskCategoryFailed    = "failed"
)

// TaskOverview is the read model used by the task status screen. The task
// table stores one current task per video, so this is already the latest task
// for every video rather than a historical attempt list.
type TaskOverview struct {
	Task
	Category string      `json:"category"`
	Video    video.Video `json:"video"`
}

// TaskCategory groups the detailed worker states into the three states shown
// by the UI. StatusRetryable is a transient state and stays in the queued
// bucket; an exhausted task reports StatusFailed and lands in the failed
// bucket. Unknown future terminal states are treated as failures so they
// remain visible instead of silently disappearing from the overview.
func TaskCategory(status string) string {
	switch status {
	case StatusQueued, StatusRunning, StatusRetryable:
		return TaskCategoryQueued
	case StatusSucceeded:
		return TaskCategorySucceeded
	default:
		return TaskCategoryFailed
	}
}
