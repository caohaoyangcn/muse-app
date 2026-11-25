package video

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// 精彩时刻 previews (docs/spec/highlight-moments.md §8.5).
//
// "Save" used to be the only way to see what a range would look like: the row was
// queued and the image arrived a moment later. A preview renders the same file up
// front, into preview/, and lets the dialog show the real clip — same ffmpeg
// command, same frame rate — so the user confirms what they will get instead of
// guessing from a time range.
//
// A preview is not a row. It has no status, it is never queued, and the detail
// section never shows it. Saving adopts it: the file moves into highlights/ and a
// ready row starts pointing at it, so the confirmed preview is exactly the saved
// artifact (and the same range is never encoded twice).

// errHighlightPreviewStale means the dialog is holding an image the app cannot
// account for any more — the file was cleaned up, or it was rendered for other
// times. The save then falls back to the normal queue instead of failing.
var errHighlightPreviewStale = errors.New("预览已失效，请重新预览")

// PreviewHighlight renders the request into preview/ and returns what the dialog
// should show. Nothing is recorded: no row, no task counter, no detail entry.
func (s *Service) PreviewHighlight(videoID, startMs, endMs int64) (*HighlightPreview, error) {
	v, err := s.repo.Get(videoID)
	if err != nil {
		return nil, err
	}
	if err := ValidateHighlightTimes(startMs, endMs, v.DurationMs); err != nil {
		return nil, err
	}
	// Preview is a synchronous call from the dialog, so a source that cannot be
	// read has to be reported now rather than through a failed card later.
	media, err := s.highlightSource(v)
	if err != nil {
		return nil, err
	}

	kind := HighlightKindMoment
	if endMs > 0 {
		kind = HighlightKindRange
	}
	rel := highlightPreviewPath(videoID, startMs, endMs, s.highlights.seq.Add(1), kind)
	if err := s.ensurePreviewDir(); err != nil {
		return nil, err
	}
	outPath := filepath.Join(s.dataDir, filepath.FromSlash(rel))

	// One preview at a time: two dialogs would otherwise start two ffmpeg runs
	// and each would feel twice as slow as it is.
	s.highlights.previewMu.Lock()
	fps, genErr := s.generateHighlight(s.previewContext(), v, &Highlight{
		VideoID: videoID, Kind: kind, StartMs: startMs, EndMs: endMs,
	}, media, outPath)
	s.highlights.previewMu.Unlock()
	if genErr != nil {
		_ = os.Remove(outPath)
		if errors.Is(genErr, context.Canceled) {
			return nil, fmt.Errorf("应用正在退出，未生成预览")
		}
		return nil, fmt.Errorf("生成预览失败：%w", genErr)
	}
	size, err := s.highlights.fileSize(outPath)
	if err != nil {
		_ = os.Remove(outPath)
		return nil, fmt.Errorf("读取预览文件失败：%w", err)
	}
	s.log.Info("highlight preview rendered", "video", videoID, "kind", kind,
		"startMs", startMs, "endMs", endMs, "fps", fps, "bytes", size)
	return &HighlightPreview{
		Path:    rel,
		Kind:    kind,
		StartMs: startMs,
		EndMs:   endMs,
		Bytes:   size,
		FPS:     fps,
	}, nil
}

// DiscardHighlightPreview deletes a preview the user closed the dialog on. It is
// best effort and idempotent: the startup purge cleans up whatever survives a
// crash, and only names the preview generator writes are touched.
func (s *Service) DiscardHighlightPreview(previewPath string) error {
	name := filepath.Base(filepath.FromSlash(previewPath))
	if !ValidHighlightPreviewFileName(name) {
		return nil
	}
	if err := os.Remove(filepath.Join(s.dataDir, HighlightsPreviewDir, name)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除预览失败：%w", err)
	}
	return nil
}

// AddHighlightFromPreview turns a confirmed preview into a saved highlight: the
// file moves into highlights/ and the row is recorded as ready, so the image the
// user approved is the image the library keeps.
//
// The preview's file name carries the request it was rendered for, so a preview
// of other times is refused rather than adopted. A missing file is refused the
// same way; the caller then falls back to AddHighlight and the normal queue.
func (s *Service) AddHighlightFromPreview(videoID, startMs, endMs int64, previewPath string) (*Highlight, error) {
	v, err := s.repo.Get(videoID)
	if err != nil {
		return nil, err
	}
	// Re-run the rules: the dialog is not the authority on what may be saved.
	if err := ValidateHighlightTimes(startMs, endMs, v.DurationMs); err != nil {
		return nil, err
	}

	kind := HighlightKindMoment
	if endMs > 0 {
		kind = HighlightKindRange
	}
	name := filepath.Base(filepath.FromSlash(previewPath))
	meta, ok := parseHighlightPreviewName(name)
	if !ok || meta.videoID != videoID || meta.startMs != startMs || meta.endMs != endMs ||
		meta.ext != highlightFileExt(kind) {
		return nil, errHighlightPreviewStale
	}
	src := filepath.Join(s.dataDir, HighlightsPreviewDir, name)
	if info, err := os.Stat(src); err != nil || info.Size() == 0 {
		return nil, errHighlightPreviewStale
	}
	if err := s.ensureHighlightDir(); err != nil {
		return nil, err
	}

	now := nowMillis()
	mediaPath := highlightMediaPath(videoID, s.highlights.seq.Add(1), kind)
	dst := s.highlightFile(mediaPath)
	if err := os.Rename(src, dst); err != nil {
		return nil, fmt.Errorf("保存精彩时刻失败：%w", err)
	}
	h := &Highlight{
		VideoID:    videoID,
		Kind:       kind,
		StartMs:    startMs,
		EndMs:      endMs,
		MediaPath:  mediaPath,
		Status:     HighlightStatusReady,
		CreatedAt:  now,
		UpdatedAt:  now,
		FinishedAt: now,
	}
	if err := s.repo.CreateHighlight(h); err != nil {
		// The file is already in highlights/ with no row to own it; removing it
		// now keeps the sweep from having to.
		_ = os.Remove(dst)
		return nil, err
	}
	s.log.Info("highlight saved from preview", "id", h.ID, "video", videoID, "kind", kind)
	s.emitHighlightUpdated(h)
	return h, nil
}

// previewContext is the context a preview renders under: the service's, so a quit
// cancels it like any other ffmpeg run, and a plain background context before the
// service has been started (tests, and the dialog opened during startup).
func (s *Service) previewContext() context.Context {
	s.highlights.mu.Lock()
	defer s.highlights.mu.Unlock()
	if s.highlights.ctx != nil {
		return s.highlights.ctx
	}
	return context.Background()
}

// ensurePreviewDir creates the preview folder on demand, like ensureHighlightDir.
func (s *Service) ensurePreviewDir() error {
	if err := os.MkdirAll(filepath.Join(s.dataDir, HighlightsPreviewDir), 0o755); err != nil {
		return fmt.Errorf("create preview dir: %w", err)
	}
	return nil
}

// purgeStalePreviews deletes every preview left by a previous process. A preview
// only means something to the dialog that rendered it, so nothing can survive a
// restart; this is what keeps the folder from growing forever after a crash.
func (s *Service) purgeStalePreviews() {
	dir := filepath.Join(s.dataDir, HighlightsPreviewDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			s.log.Warn("read preview dir failed", "dir", dir, "err", err)
		}
		return
	}
	removed := 0
	for _, entry := range entries {
		// Only files the preview generator names; anything else in the folder was
		// not written by this app and is left alone.
		if entry.IsDir() || !ValidHighlightPreviewFileName(entry.Name()) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, entry.Name())); err == nil {
			removed++
		}
	}
	if removed > 0 {
		s.log.Info("stale highlight previews removed", "files", removed)
	}
}
