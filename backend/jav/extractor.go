package jav

import (
	"path/filepath"
	"regexp"
	"strings"
)

// The trailing `(?:[^0-9a-z]|$)` is what keeps a part marker from being read
// out of a longer token: "ABC-123-4K" and "ABC-123 1080p" carry no part, while
// "ABC-123-1" and "ABC-123_2" do. Go's regexp (RE2) has no lookahead, so the
// boundary character is captured as group 4 / group 3 — it can never be a
// letter, and every code candidate starts with one, so no later candidate is
// swallowed by it.
var codePattern = regexp.MustCompile(`(?i)([a-z]{2,6})[-_ ]?([0-9]{2,5})(?:[-_. ]+([0-9]{1,3})([^0-9a-z]|$))?`)
var fc2CodePattern = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])FC2(?:[-_ ]?PPV)?(?:[-_ ]*|\)\()([0-9]{6,10})(?:[-_. ]+([0-9]{1,3})([^0-9a-z]|$))?`)
var fc2MarkerPattern = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])FC2(?:[-_ ]?PPV)?(?:\b|[-_ ])`)

var ignoredPrefixes = map[string]struct{}{
	"VIDEO": {}, "MOVIE": {}, "SAMPLE": {}, "HOLIDAY": {}, "SEASON": {},
	"EPISODE": {}, "RESOLUTION": {}, "PART": {}, "DISC": {}, "VOL": {},
}

// NameCode is one 番号 candidate found in a file name, plus the part marker the
// name attaches to it.
//
// Part is the number of a 上下集 marker written right after the code:
// "ABC-123-1.mp4" and "ABC-123_2.mp4" yield part "1" and "2". It is what keeps
// episodes apart from duplicate copies — two files sharing a code but carrying
// different parts are two works, not the same work twice.
type NameCode struct {
	Code string
	Part string
}

// ExtractNameCodes returns stable, de-duplicated 番号 candidates from a file
// name, in the order the scraper tries them, each with its part marker. A
// candidate only takes a part marker when that marker is not itself part of a
// longer token, so "IPX-633 1080p" and "ABC-123-4K" have none.
//
// Only bare numeric markers count ("abc-123-1", "abc-123_2"). Textual ones like
// "abc-123 part1" are deliberately left alone: reading a part where there is
// none splits one work into phantom episodes and hides a real duplicate
// forever, while the opposite mistake is visible and one click to dismiss.
func ExtractNameCodes(fileName string) []NameCode {
	base := strings.TrimSuffix(filepath.Base(fileName), filepath.Ext(fileName))
	if fc2Match := fc2CodePattern.FindStringSubmatchIndex(base); fc2Match != nil {
		number := strings.TrimLeft(base[fc2Match[2]:fc2Match[3]], "0")
		if number == "" {
			number = "0"
		}
		if len(number) >= 6 && len(number) <= 7 {
			return []NameCode{{Code: "FC2-" + number, Part: partFromMatch(base, fc2Match[4], fc2Match[5], fc2Match[6], fc2Match[7])}}
		}
	}
	if fc2MarkerPattern.MatchString(base) {
		return []NameCode{}
	}
	matches := codePattern.FindAllStringSubmatchIndex(base, -1)
	seen := make(map[string]struct{}, len(matches))
	out := make([]NameCode, 0, len(matches))
	for _, m := range matches {
		prefix := strings.ToUpper(base[m[2]:m[3]])
		if _, ignored := ignoredPrefixes[prefix]; ignored {
			continue
		}
		number := base[m[4]:m[5]]
		if len(number) >= 4 {
			number = strings.TrimLeft(number, "0")
			if number == "" {
				number = "0"
			}
		}
		candidate := NameCode{
			Code: strings.ToUpper(prefix + "-" + number),
			Part: partFromMatch(base, m[6], m[7], m[8], m[9]),
		}
		key := candidate.Code + "\x00" + candidate.Part
		if _, ok := seen[key]; !ok {
			seen[key] = struct{}{}
			out = append(out, candidate)
		}
	}
	return out
}

// partFromMatch reads a captured part marker, returning "" when there is none.
// start/end delimit the marker itself and boundaryEnd the end of the boundary
// that followed it. A marker followed by "." and another digit is rejected:
// "abc-123 5.1ch.mkv" names an audio layout, not episode 5, and treating it as
// one would split a work's copies into phantom episodes.
func partFromMatch(base string, start, end, boundaryStart, boundaryEnd int) string {
	if start < 0 || boundaryStart < 0 {
		return ""
	}
	if boundaryEnd-boundaryStart == 1 && base[boundaryStart] == '.' && boundaryEnd < len(base) && isASCIIDigit(base[boundaryEnd]) {
		return ""
	}
	return normalizePart(base[start:end])
}

func isASCIIDigit(c byte) bool { return c >= '0' && c <= '9' }

// ExtractCodes returns stable, de-duplicated code candidates from a file name.
func ExtractCodes(fileName string) []string {
	candidates := ExtractNameCodes(fileName)
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, candidate.Code)
	}
	return out
}

// normalizePart canonicalises a captured part marker so "01" and "1" name the
// same episode.
func normalizePart(part string) string {
	if part == "" {
		return ""
	}
	trimmed := strings.TrimLeft(part, "0")
	if trimmed == "" {
		return "0"
	}
	return trimmed
}
