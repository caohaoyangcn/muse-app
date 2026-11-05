package group

import (
	"errors"
	"log/slog"
	"strings"
	"time"
)

// Service holds the grouping business logic. It is bound to Wails by app.go
// and emits events so the frontend can stay live without polling.
type Service struct {
	repo GroupRepository
	log  *slog.Logger
	emit func(name string, data ...any)
}

func NewService(repo GroupRepository, log *slog.Logger, emit func(name string, data ...any)) *Service {
	return &Service{repo: repo, log: log, emit: emit}
}

// List returns all groups in display (position) order.
func (s *Service) List() ([]Group, error) {
	return s.repo.List()
}

// Create makes a new group at the end of the list and returns it.
func (s *Service) Create(name string) (*Group, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrEmptyName
	}
	groups, err := s.repo.List()
	if err != nil {
		return nil, err
	}
	g, err := s.repo.Create(name, len(groups), nil, nowMillis())
	if err != nil {
		return nil, err
	}
	s.log.Info("group created", "id", g.ID, "name", g.Name)
	s.emitChanged()
	return g, nil
}

// EnsureGroup returns the existing group with the given name (ignoring case) or
// creates it when it does not exist (spec: auto-grouping must create the group
// on first use). It never creates a duplicate for the same name.
func (s *Service) EnsureGroup(name string) (*Group, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrEmptyName
	}
	if g, err := s.repo.FindByName(name); err == nil {
		return g, nil
	} else if err != ErrNotFound {
		return nil, err
	}
	return s.Create(name)
}

// Rename updates a group's name.
func (s *Service) Rename(id int64, name string) (*Group, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrEmptyName
	}
	if _, err := s.repo.Get(id); err != nil {
		return nil, err
	}
	if err := s.repo.Rename(id, name, nowMillis()); err != nil {
		return nil, err
	}
	s.log.Info("group renamed", "id", id, "name", name)
	s.emitChanged()
	return s.repo.Get(id)
}

// Delete removes a group and its memberships (videos themselves are kept). Its
// direct children are detached to the top level rather than lost.
func (s *Service) Delete(id int64) error {
	if _, err := s.repo.Get(id); err != nil {
		return err
	}
	if err := s.repo.Delete(id); err != nil {
		return err
	}
	s.log.Info("group deleted", "id", id)
	s.emitChanged()
	return nil
}

// SetParent makes the group with id a child of parentID, or detaches it to the
// top level when parentID is nil. It rejects self-parenting and any change that
// would create a cycle (a group can never be an ancestor of its own parent). With
// parentID non-nil the target must exist.
func (s *Service) SetParent(id int64, parentID *int64) (*Group, error) {
	if _, err := s.repo.Get(id); err != nil {
		return nil, err
	}
	if parentID != nil {
		if *parentID == id {
			return nil, ErrSelfParent
		}
		if _, err := s.repo.Get(*parentID); err != nil {
			return nil, err
		}
		// Walk up: if parentID is currently a descendant of id, linking it as a
		// parent would create a cycle.
		ancestors, err := s.ancestors(*parentID)
		if err != nil {
			return nil, err
		}
		if _, contained := ancestors[id]; contained {
			return nil, ErrCycle
		}
	}
	if err := s.repo.SetParent(id, parentID, nowMillis()); err != nil {
		return nil, err
	}
	s.log.Info("group parent set", "id", id, "parentId", parentID)
	s.emitChanged()
	return s.repo.Get(id)
}

// ancestors returns the set of all ancestor ids of the given group, derived from
// the persisted parent relationships.
func (s *Service) ancestors(id int64) (map[int64]struct{}, error) {
	set := make(map[int64]struct{})
	groups, err := s.repo.List()
	if err != nil {
		return nil, err
	}
	parentOf := make(map[int64]int64, len(groups))
	for i := range groups {
		if p := groups[i].ParentID; p != nil {
			parentOf[groups[i].ID] = *p
		}
	}
	cur := id
	for {
		p, ok := parentOf[cur]
		if !ok {
			break
		}
		set[p] = struct{}{}
		cur = p
	}
	return set, nil
}

// Reorder applies the frontend's drag & drop ordering: the position fields are
// renumbered to match the given list order.
func (s *Service) Reorder(ids []int64) error {
	if err := s.repo.SetPositions(ids); err != nil {
		return err
	}
	s.log.Info("groups reordered", "count", len(ids))
	s.emitChanged()
	return nil
}

// VideoGroupIDs returns the groups a video currently belongs to.
func (s *Service) VideoGroupIDs(videoID int64) ([]int64, error) {
	return s.repo.Members(videoID)
}

// SetVideoGroups assigns a video to the given groups (replacing previous
// memberships). Empty input removes the video from all groups.
func (s *Service) SetVideoGroups(videoID int64, groupIDs []int64) error {
	if err := s.repo.SetMembers(videoID, groupIDs); err != nil {
		return err
	}
	s.emitChanged()
	s.log.Info("video groups set", "videoId", videoID, "groups", groupIDs)
	return nil
}

// AddVideoToGroup adds one video to one group. It is idempotent: if the video
// is already a member, nothing changes and no error is returned, so dragging
// an already-grouped video over a group has no side effect.
func (s *Service) AddVideoToGroup(videoID int64, groupID int64) error {
	if _, err := s.repo.Get(groupID); err != nil {
		return err
	}
	if err := s.repo.AddVideoToGroup(videoID, groupID); err != nil {
		return err
	}
	s.emitChanged()
	s.log.Info("video added to group", "videoId", videoID, "group", groupID)
	return nil
}

// VideoIDsInGroup returns the favorite ids visible in a group (newest first).
// Membership is transitive: because a video in a child group also belongs to
// the parent, the visible set is the aggregate of the group's own members and
// every descendant's members. A parent therefore shows all its children's
// videos (deduplicated when a video is a member of several groups).
func (s *Service) VideoIDsInGroup(groupID int64) ([]int64, error) {
	if _, err := s.repo.Get(groupID); err != nil {
		return nil, err
	}
	ids, err := s.descendantsAndSelf(groupID)
	if err != nil {
		return nil, err
	}
	return s.repo.VideosInGroups(ids)
}

// VideoIDsInNamedGroup returns the ids visible in the group with the given
// name (case-insensitive), including its descendants, or an empty list when no
// such group exists. Read models use it to ask about a well-known group — the
// restored collection rule, for example — without knowing its id.
func (s *Service) VideoIDsInNamedGroup(name string) ([]int64, error) {
	g, err := s.repo.FindByName(strings.TrimSpace(name))
	if errors.Is(err, ErrNotFound) {
		return []int64{}, nil
	}
	if err != nil {
		return nil, err
	}
	return s.VideoIDsInGroup(g.ID)
}

// descendantsAndSelf returns groupID plus every descendant group id (recursive
// child walk). Viewing a parent therefore aggregates the whole sub-tree.
func (s *Service) descendantsAndSelf(groupID int64) ([]int64, error) {
	groups, err := s.repo.List()
	if err != nil {
		return nil, err
	}
	children := make(map[int64][]int64, len(groups))
	for i := range groups {
		if p := groups[i].ParentID; p != nil {
			children[*p] = append(children[*p], groups[i].ID)
		}
	}
	out := []int64{groupID}
	var walk func(id int64)
	walk = func(id int64) {
		for _, c := range children[id] {
			out = append(out, c)
			walk(c)
		}
	}
	walk(groupID)
	return out, nil
}

// ClearVideoMembers removes a favorite from every group. Called when a video
// is removed from favorites so its memberships do not leak. Emits group:changed
// so the sidebar's member-count badges refresh on the desktop and mobile clients.
func (s *Service) ClearVideoMembers(videoID int64) error {
	if err := s.repo.ClearVideoMembers(videoID); err != nil {
		return err
	}
	s.emitChanged()
	s.log.Info("video members cleared", "videoId", videoID)
	return nil
}

// emitChanged notifies the frontend that the group catalog changed so it can
// refresh the sidebar and any group view.
func (s *Service) emitChanged() {
	if s.emit != nil {
		s.emit("group:changed", map[string]any{})
	}
}

func nowMillis() int64 {
	return time.Now().UnixMilli()
}
