package video

import "errors"

// The recycle bin is a soft-delete flag on the favorite row, not a copy of the
// library and not a filesystem trash (spec-v1.md §41: favorites reference
// files; the app never touches the source media).
//
// Keeping the row in place is the whole design. The favorite's id names its
// generated thumbnail and cover, its group memberships live in another table,
// and its scraped metadata hangs off video_jav — all of them keep working while
// the row is trashed. Restoring is therefore one flag change, not a rebuild,
// and a trashed row keeps its unique path so re-adding the same file restores
// the record the user already owns.

// Trash moves an active favorite to the recycle bin. It is deliberately
// idempotent: a stale card whose record is already trashed (or already purged)
// must still disappear instead of failing the caller.
func (s *Service) Trash(id int64, reason string) error {
	if reason == "" {
		reason = TrashReasonUser
	}
	v, err := s.repo.GetAny(id)
	if errors.Is(err, ErrNotFound) {
		// The record is already gone. The caller's card is stale either way;
		// announce the removal so every window can drop it.
		s.emitTrashed(id, reason)
		return nil
	}
	if err != nil {
		return err
	}
	if v.DeletedAt > 0 {
		return nil
	}
	at := nowMillis()
	if err := s.repo.Trash(id, reason, at); err != nil {
		return err
	}
	s.log.Info("video moved to trash", "id", id, "reason", reason, "path", v.Path)
	s.emitTrashed(id, reason)
	return nil
}

func (s *Service) emitTrashed(id int64, reason string) {
	if s.emit != nil {
		s.emit("video:trashed", map[string]any{"id": id, "reason": reason})
	}
}

// Restore turns a trashed favorite back into an active one. The record keeps its
// id and every relation; only its file status is re-established from the disk,
// because the file may be gone, may have come back, or may live on a volume that
// is not attached right now.
func (s *Service) Restore(id int64) error {
	v, err := s.repo.GetAny(id)
	if err != nil {
		return err
	}
	if v.DeletedAt == 0 {
		return nil
	}
	if err := s.repo.Restore(id, nowMillis()); err != nil {
		return err
	}
	v.DeletedAt = 0
	v.DeleteReason = ""

	if v.IsRemote() {
		// A remote favorite's file is not on this machine. Probing the WebDAV
		// source synchronously would make "restore" wait on the network (and
		// fail when the user is offline), so the stored status is kept and the
		// background processor refreshes what it can.
		if s.processor != nil && (s.remoteOnline == nil || s.remoteOnline(v)) {
			s.processor.Enqueue(v.ID)
		}
	} else if root := s.volumeDetached(v.Path); root == "" {
		if _, statErr := s.stat(v.Path); statErr == nil {
			s.markStatus(v, StatusAvailable)
		} else {
			s.markStatus(v, StatusMissing)
		}
		if v.Status == StatusAvailable && s.processor != nil && s.needsProcessing(*v) {
			s.processor.Enqueue(v.ID)
		}
		if v.Status == StatusAvailable {
			s.enqueueFingerprint(v)
		}
	}

	s.log.Info("video restored from trash", "id", id, "status", v.Status)
	if s.emit != nil {
		s.emit("video:restored", map[string]any{"id": id})
	}
	return nil
}

// ArchiveBySource moves every active favorite of a cloud source into the
// recycle bin with reason archived. It is idempotent and returns how many rows
// were archived this time.
func (s *Service) ArchiveBySource(sourceID int64, sourceName string) (int, error) {
	if sourceID <= 0 {
		return 0, ErrInvalidRemotePath
	}
	at := nowMillis()
	n, err := s.repo.ArchiveBySource(sourceID, sourceName, at)
	if err != nil {
		return 0, err
	}
	if n > 0 {
		s.log.Info("cloud source archived", "source", sourceID, "name", sourceName, "count", n)
		if s.emit != nil {
			s.emit("cloud:source:archived", map[string]any{
				"sourceId": sourceID,
				"name":     sourceName,
				"count":    n,
			})
		}
	}
	return n, nil
}

// ListArchived returns archived favorites, newest archive first. It is the
// long-lived part of the recycle bin; normal trash uses ListTrash.
func (s *Service) ListArchived() ([]Video, error) {
	return s.repo.ListArchived()
}

// GetAny returns a row regardless of its recycle-bin state.
func (s *Service) GetAny(id int64) (*Video, error) {
	return s.repo.GetAny(id)
}

// ListTrash returns every trashed favorite, newest removal first. It is a pure
// database read: a drive being unplugged is not a reason to hide the trash, and
// the view must not wait on a disk.
func (s *Service) ListTrash() ([]Video, error) {
	return s.repo.ListTrashed()
}

// restoreTrashed revives a trashed favorite that is being re-added at the path
// it already holds. The caller has already established that the file exists
// (NormalizeAndValidate) or that the remote PROPFIND succeeded.
func (s *Service) restoreTrashed(v *Video) *Video {
	if err := s.repo.Restore(v.ID, nowMillis()); err != nil {
		s.log.Error("restore trashed favorite on add failed", "id", v.ID, "err", err)
		return v
	}
	v.DeletedAt = 0
	v.DeleteReason = ""
	s.markStatus(v, StatusAvailable)
	if s.processor != nil {
		s.processor.Enqueue(v.ID)
	}
	if !v.IsRemote() {
		s.enqueueFingerprint(v)
	}
	if s.emit != nil {
		s.emit("video:restored", map[string]any{"id": v.ID})
	}
	s.log.Info("trashed favorite restored by adding its file", "id", v.ID, "path", v.Path)
	return v
}
