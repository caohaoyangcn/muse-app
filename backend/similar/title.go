package similar

import (
	"math"
	"strings"
	"unicode"
)

// The title channel reads the scraped original title — never the Chinese
// translation. A translation is a derived text: matching on it would make the
// recommendations change whenever the translation model or prompt changes,
// while the original title is the work's canonical string.
//
// What the channel is for: the marketing phrase a label repeats across a batch
// is a real, finer-grained signal than the genre tags (「杭打ち騎乗位」「共演」
// 「絶対領域」 have no tag equivalent). What it must NOT do is re-encode the
// structured dimensions, so the channel drops:
//
//   - tokens built from actress names (the cast already has its own weight, and
//     a shared surname is the worst kind of false positive: 竹内あい vs 竹内夏希);
//   - tokens that appear across a large share of the library, i.e. the release
//     boilerplate (ブルーレイディスク / 生写真付き / 独占配信);
//   - tokens containing digits (放送日 and part numbers: 寝取られの館11).
//
// Japanese has no word boundaries, so the token is a character bigram. That is
// enough to carry a marketing phrase (杭打ち → 杭打/打ち) and costs no tokenizer
// dependency, no dictionary download and no CGO.
const (
	// titleMaxDFRatio drops release boilerplate: a phrase shared by more than
	// this share of the library says nothing about two particular films.
	titleMaxDFRatio = 0.03
	// titleMinDF keeps only phrases that at least two films can share.
	titleMinDF = 2
	// titleReasonThreshold is how much title cosine earns a 「同关键词」 reason.
	titleReasonThreshold = 0.20
	// maxPhraseRunes bounds the phrase shown as the reason.
	maxPhraseRunes = 12
)

// titleTokens cuts a title into the overlapping bigrams the channel scores.
// nameTokens is the vocabulary built from every scraped actress name; a bigram
// that appears in one of them is a name fragment and is dropped.
func titleTokens(title string, nameTokens map[string]struct{}) []string {
	runes := []rune(strings.ToLower(title))
	if len(runes) < 2 {
		return nil
	}
	tokens := make([]string, 0, len(runes))
	for i := 0; i+1 < len(runes); i++ {
		left, right := runes[i], runes[i+1]
		if !phraseRune(left) || !phraseRune(right) {
			continue
		}
		pair := string(runes[i : i+2])
		if _, isName := nameTokens[pair]; isName {
			continue
		}
		tokens = append(tokens, pair)
	}
	return tokens
}

// phraseRune reports whether a rune can be part of a scored bigram: letters and
// kana/kanji, but not punctuation, space or digits.
func phraseRune(r rune) bool {
	switch {
	case unicode.IsDigit(r), unicode.IsPunct(r), unicode.IsSpace(r):
		return false
	case unicode.IsLetter(r):
		return true
	default:
		// Kana and kanji are letters; a stray symbol (「●」「♡」) is not.
		return false
	}
}

// nameVocabulary collects every bigram that can be built from the scraped
// actress names, so a title that merely repeats a performer's name cannot earn
// title similarity — the cast dimension already covers that, and a shared
// surname is not a shared subject.
func nameVocabulary(names []string) map[string]struct{} {
	vocab := make(map[string]struct{}, len(names)*3)
	for _, name := range names {
		runes := []rune(strings.ToLower(name))
		for i := 0; i+1 < len(runes); i++ {
			vocab[string(runes[i:i+2])] = struct{}{}
		}
	}
	return vocab
}

// titleDocumentFrequency keeps the bigrams that are worth scoring: shared by at
// least two films and not boilerplate. Tokens outside the returned set are
// dropped from every title vector, which is what stops a release suffix from
// creating similarity.
func titleDocumentFrequency(docs []map[string]struct{}) map[string]int {
	df := make(map[string]int)
	for _, tokens := range docs {
		for token := range tokens {
			df[token]++
		}
	}
	limit := int(math.Ceil(float64(len(docs)) * titleMaxDFRatio))
	if limit < titleMinDF {
		limit = titleMinDF
	}
	for token, count := range df {
		if count < titleMinDF || count > limit {
			delete(df, token)
		}
	}
	return df
}

func toSet(tokens []string) map[string]struct{} {
	set := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		set[token] = struct{}{}
	}
	return set
}

// sharedPhrase names what two titles have in common, for the 「同关键词」 reason.
// It grows the heaviest shared bigram into a readable phrase and stops at the
// longest string both titles actually contain, so the row never claims an
// overlap the scorer did not use.
func sharedPhrase(titleA, titleB string, shared map[string]float64) string {
	best, bestWeight := "", -1.0
	for token, weight := range shared {
		if weight > bestWeight || (weight == bestWeight && token < best) {
			best, bestWeight = token, weight
		}
	}
	if best == "" {
		return ""
	}
	a := []rune(strings.ToLower(titleA))
	b := []rune(strings.ToLower(titleB))
	start := indexRunes(a, []rune(best))
	if start < 0 {
		return best
	}
	left, right := start, start+len([]rune(best))
	// Grow right, then left, only when the extended slice still appears in the
	// other title.
	for right < len(a) && right-left < maxPhraseRunes && phraseRune(a[right]) &&
		indexRunes(b, a[left:right+1]) >= 0 {
		right++
	}
	for left > 0 && right-left < maxPhraseRunes && phraseRune(a[left-1]) &&
		indexRunes(b, a[left-1:right]) >= 0 {
		left--
	}
	return string(a[left:right])
}

// titleReason names the phrase two titles share, so a card can say why it is
// there (「同关键词：杭打ち騎乗位」) instead of showing a bare score.
func titleReason(q, d *document) string {
	shared := make(map[string]float64, len(q.titleVec))
	for token, weight := range q.titleVec {
		if other, ok := d.titleVec[token]; ok {
			shared[token] = min(weight, other)
		}
	}
	phrase := sharedPhrase(q.titleText, d.titleText, shared)
	if phrase == "" {
		return "标题用词相近"
	}
	return "同关键词：" + phrase
}

func indexRunes(haystack, needle []rune) int {
	if len(needle) == 0 || len(needle) > len(haystack) {
		return -1
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := range needle {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
