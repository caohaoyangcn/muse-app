// Package group contains the favorite-grouping model, persistence and
// business logic. Groups let the user organize favorite videos (分组).
package group

// Group is a named, reorderable bucket that favorites can be assigned to.
// A group may optionally belong to a parent group (分组父子关系). Membership is
// transitive: a video that belongs to a child group also belongs to that
// child's parent (and every ancestor), so the parent's video set is the
// recursive aggregate of its own and all its descendants' videos. A video can
// belong to a parent and a child group independently.
type Group struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Position   int    `json:"position"`
	ParentID   *int64 `json:"parentId"`   // nil = top-level group
	VideoCount int64  `json:"videoCount"` // transitive member count (self + all descendants)
	CreatedAt  int64  `json:"createdAt"`
	UpdatedAt  int64  `json:"updatedAt"`
}
