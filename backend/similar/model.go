// Package similar scores how alike two favorites are and ranks the 「类似影片」
// row a detail page shows.
//
// It is deliberately pure: no database, no clock, no network, no configuration.
// The caller hands in the features of the whole library and gets ranked
// favorite ids back, so the scoring rules stay testable without a scrape or a
// provider. Wiring — reading the scrape results, hydrating the ranked ids back
// into full favorites — belongs to the api façade.
package similar

import "strings"

// Feature is one favorite's scoring input. Every field is a reading of what a
// scrape already stored (video_jav); nothing here is persisted separately.
type Feature struct {
	VideoID int64
	// Code is the authoritative 番号. It identifies the work, so a candidate
	// carrying the query's own code is never a "similar" one.
	Code string
	// Title is the scraped ORIGINAL title, never the Chinese translation: a
	// translation is a derived text, so matching on it would tie the row to a
	// translation model's version. It feeds only the title channel — the
	// marketing phrase a label repeats is finer than the genre tags. Empty means
	// the film does not participate at all (the jav layer drops those rows).
	Title  string
	Series string
	Studio string
	// ReleaseUnix is the provider's release date; nil when unknown.
	ReleaseUnix *int64
	// IsUncensored is the provider's 有码/无码 verdict; nil when unknown.
	// Unknown never splits the row, only a known mismatch does.
	IsUncensored *bool
	// Actors is the canonical cast. 演员合并 is applied by the jav layer before
	// the features get here, so two stage names for one performer already
	// resolve to the same string.
	Actors []string
	// Tags is the scraped genre list. Format tags (单体作品/HDTV/無碼破解…) are
	// dropped by the scorer, see formatTags.
	Tags []string
}

// Result is one ranked recommendation: the favorite plus why it was picked.
// Reasons are display strings, already limited and ordered by contribution.
type Result struct {
	VideoID int64
	Score   float64
	Reasons []string
}

// formatTags are the scraped "tags" that describe how a work was shot,
// released or leaked rather than what happens in it. They must not count as
// style: 「無碼破解」 appears across a whole leaked subset and would otherwise
// become the strongest dimension in the library, while 「單體作品」 covers most
// releases and says nothing about similarity.
//
// This is an explicit list rather than an IDF threshold on purpose: a format
// tag can be rare (「LTX破解」 only exists on restored files) exactly when its
// inverse document frequency is high, so statistics cannot tell these apart.
//
// Keys are normalizeKey values, so both character sets are listed.
var formatTags = map[string]struct{}{
	// 发行形态
	"單體作品": {}, "单体作品": {},
	// 画质与介质
	"hdtv": {}, "高清": {}, "高畫質": {}, "高画质": {}, "4k": {},
	// 马赛克形态
	"數位馬賽克": {}, "数位马赛克": {}, "無碼破解": {}, "无码破解": {}, "ltx破解": {},
	// 发行渠道与特典
	"獨占配信": {}, "独占配信": {}, "生写真": {}, "特典映像": {}, "特典": {},
}

// normalizeKey folds the cosmetic differences between how providers and users
// spell the same label: case, surrounding space, inner space and full-width
// ASCII. It keeps the rest of the text intact so distinct names stay distinct.
func normalizeKey(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(value))
	for _, r := range value {
		switch {
		case r >= 0xFF01 && r <= 0xFF5E: // full-width ASCII → ASCII
			b.WriteRune(r - 0xFEE0)
		case r == 0x3000: // ideographic space
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), "")
}

// codePrefix is the label part of a 番号 (SSIS-762 → ssis). Two works of one
// label share production habits and a release template even when the provider
// left 系列 empty, so it is the fallback series signal.
func codePrefix(code string) string {
	if i := strings.IndexByte(code, '-'); i > 0 {
		return normalizeKey(code[:i])
	}
	return normalizeKey(code)
}
