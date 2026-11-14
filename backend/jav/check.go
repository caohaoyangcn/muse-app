package jav

import "strings"

// This file answers one question for an outside caller (the JavDB browser
// extension): "is this work in the library?". It deliberately owns no
// extraction rules of its own — the 番号 travels in already rendered by the
// caller — it only normalises that value and describes the matches.

// JavCodeCheck is one queried 番号 and the favorites that hold it. Code echoes
// the caller's value so a client can pair results with cards without relying on
// array order or on its own normalisation.
type JavCodeCheck struct {
	Code    string         `json:"code"`
	Matches []JavCodeMatch `json:"matches"`
}

// JavCodeMatch is one favorite holding a queried work. It carries just enough
// for a card popover: what the copy is, how big it is, who is in it and whether
// it is still in the library.
type JavCodeMatch struct {
	Code string `json:"code"`
	// VideoID is the Muse favorite id; the client opens /videos/<id> with it.
	VideoID int64 `json:"videoId"`
	// InTrash reports a favorite in the recycle bin (or the long-lived archive).
	// It is shown as such rather than as a missing favorite, because "we still
	// have the record, it is just binned" is a different answer.
	InTrash  bool   `json:"inTrash"`
	FileName string `json:"fileName"`
	// Label is what the UI names the favorite by: Title, then JavCode, then the
	// file name — the same fallback order every Muse surface uses.
	Label    string   `json:"label"`
	JavCode  string   `json:"javCode"`
	Width    int      `json:"width"`
	Height   int      `json:"height"`
	FileSize int64    `json:"fileSize"`
	Actors   []string `json:"actors"`
	// CoverPath is the scraped cover (relative to the data dir, served at
	// "/covers/jav/<id>") when one was downloaded; empty means the generated
	// thumbnail at "/thumbnails/<id>.jpg" is all there is.
	CoverPath string `json:"coverPath"`
	// SourceName is the cloud connection a remote copy lives on; empty for a
	// local file. VolumeOffline flags a local copy whose disk is unplugged.
	SourceName    string `json:"sourceName"`
	VolumeOffline bool   `json:"volumeOffline"`
	Status        string `json:"status"`
	CreatedAt     int64  `json:"createdAt"`
}

// LookupKey normalises a caller-supplied 番号 so it can be compared with the
// keys stored in the library. It is not an extractor: the caller must pass a
// value that already names a work (an element JavDB rendered), and this only
// removes presentation differences — surrounding whitespace (including the
// non-breaking and ideographic spaces a page may pad with), zero-width
// characters and letter case, plus JavDB's "FC2-PPV-1234567" spelling, which
// Muse stores as "FC2-1234567" because that is how the FC2 provider and the
// file-name extractor key it.
//
// browser-extension/content.js applies the same clean-up before sending, so the
// two sides cannot disagree about what a code string is.
func LookupKey(code string) string {
	key := normalizeCode(stripInvisible(code))
	if rest, ok := strings.CutPrefix(key, "FC2-PPV-"); ok && rest != "" {
		return "FC2-" + rest
	}
	return key
}

// stripInvisible drops the characters that only pad a rendered value: the
// zero-width space/BOM, and whitespace runs — which are collapsed to one plain
// space (so interior separators survive as a single space and surrounding
// padding is trimmed by the caller's normalise step). Interior punctuation is
// left alone: it is part of the page's spelling, and the library stores what
// the scraper wrote.
func stripInvisible(code string) string {
	var builder strings.Builder
	builder.Grow(len(code))
	pendingSpace := false
	for _, r := range code {
		switch r {
		case '\u200b', '\ufeff':
			continue
		case ' ', '\t', '\n', '\r', '\u00a0', '\u3000':
			pendingSpace = builder.Len() > 0
			continue
		}
		if pendingSpace {
			builder.WriteByte(' ')
			pendingSpace = false
		}
		builder.WriteRune(r)
	}
	return builder.String()
}
