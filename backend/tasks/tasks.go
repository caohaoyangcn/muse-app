// Package tasks defines the read model behind the 后台任务 dashboard.
//
// It deliberately depends on no domain package: each task owner (the scraper,
// the translation queue, the library scans, the actor backfill) keeps its own
// state and its own executor, and api.Library maps those statuses into this
// shape. The dashboard is therefore a projection, not a second scheduler —
// which is why a scrape queue and a fingerprint pass can share one screen
// without pretending to share one execution model.
package tasks

import "sort"

// Kind groups rows by what the work does.
type Kind string

const (
	// KindBatch is a queue of independent items, worked through until empty.
	KindBatch Kind = "batch"
	// KindScan is one pass over the library that ends by itself.
	KindScan Kind = "scan"
)

// State is the display state of one row. StateFailed is reserved for a pass
// that could not run at all; a batch that finished with some failed items is
// still done, with Failed > 0, because the run itself completed.
type State string

const (
	StateRunning State = "running"
	StateDone    State = "done"
	StateFailed  State = "failed"
)

// Task is one row.
//
// Total == 0 means the total is genuinely unknown — a scrape queue has no batch
// boundary, and a scan's second phase is not sized until the first one ends —
// so the UI shows counts without a percentage instead of inventing a
// denominator that would change under the user.
type Task struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Kind  Kind   `json:"kind"`
	State State  `json:"state"`
	// Done and Total drive the progress bar; Failed is reported separately.
	Done   int `json:"done"`
	Total  int `json:"total"`
	Failed int `json:"failed"`
	// Unit is the noun after the counts: 条, 部, 个文件, 位演员.
	Unit string `json:"unit"`
	// Current is the item or batch being worked on right now, when one exists.
	Current string `json:"current,omitempty"`
	// Message carries the phase or the queue breakdown ("刮削中 1 · 待重试 2").
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
	// StartedAt and UpdatedAt are unix milliseconds.
	StartedAt int64 `json:"startedAt,omitempty"`
	UpdatedAt int64 `json:"updatedAt,omitempty"`
	// Detail is the view this row drills into ("scrapeTasks", "duplicates", …).
	Detail string `json:"detail,omitempty"`
}

// Snapshot is the whole dashboard, already ordered for display.
type Snapshot struct {
	Tasks []Task `json:"tasks"`
}

// Sort puts running rows first (oldest first, so a long pass stays at the top
// where the user found it) and finished rows after them, most recent first.
func Sort(list []Task) {
	sort.SliceStable(list, func(i, j int) bool {
		runningI := list[i].State == StateRunning
		runningJ := list[j].State == StateRunning
		if runningI != runningJ {
			return runningI
		}
		if runningI {
			return list[i].StartedAt < list[j].StartedAt
		}
		return list[i].UpdatedAt > list[j].UpdatedAt
	})
}
