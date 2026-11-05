package jav

import (
	"sort"
	"strconv"
	"strings"

	"muse-app/backend/video"
)

// DuplicateGroup is one set of favorites the app recognized as the same work
// (同番号多版本): the same 番号 with the same part marker, so each member is a
// separate copy of that work rather than another episode of it. This is what
// the 查重 panel offers to resolve — keep one copy, or keep them all.
type DuplicateGroup struct {
	// Key identifies the group (番号 + part). It is opaque to clients and is
	// what a dismissal is stored under.
	Key string `json:"key"`
	// Code is the recognized 番号, as shown in the video details.
	Code string `json:"code"`
	// Part is the 上下集 marker of the file names ("" for an ordinary work).
	Part string `json:"part"`
	// Ignored is true when the user already chose "keep both" for exactly this
	// set of copies. A newly added copy changes the set and clears the flag.
	Ignored bool `json:"ignored"`
	// Videos are the copies, in library order (newest added first).
	Videos []video.Video `json:"videos"`
}

// IgnoredDuplicate is one persisted dismissal: a group key plus the member ids
// it covered when the user dismissed it.
type IgnoredDuplicate struct {
	Key      string
	VideoIDs string
}

// DuplicateCandidate is one scraped favorite eligible for duplicate grouping:
// the 番号 recognized for it plus the favorite itself. The part marker is not
// stored — it is read off the file name at grouping time, so it can never
// contradict the name the user sees.
type DuplicateCandidate struct {
	Code  string
	Video video.Video
}

// DuplicateKey builds the grouping key from a 番号 and its part marker.
func DuplicateKey(code, part string) string {
	code = normalizeCode(code)
	if part == "" {
		return code
	}
	return code + "-" + part
}

// PartForFile returns the part marker the file name attaches to code, or "" when
// the name carries no part marker or does not spell that code at all (a manual
// 番号 override, for instance). It resolves through the same candidates the
// scraper used, so a name holding several codes ("hhd800.com@ABW-180.mp4")
// reports the part of the code that was actually recognized.
func PartForFile(fileName, code string) string {
	code = normalizeCode(code)
	if code == "" {
		return ""
	}
	for _, candidate := range ExtractNameCodes(fileName) {
		if candidate.Code == code {
			return candidate.Part
		}
	}
	return ""
}

// duplicateSignature renders a group's members as a stable string, so a
// dismissal can be matched against the set it was made for.
func duplicateSignature(ids []int64) string {
	sorted := append([]int64(nil), ids...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	parts := make([]string, 0, len(sorted))
	for _, id := range sorted {
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	return strings.Join(parts, ",")
}
