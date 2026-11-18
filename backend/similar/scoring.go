package similar

import (
	"math"
	"sort"
	"strings"
	"time"
)

// Signal weights. A shared actress is worth 0.28, a shared label only 0.08.
//
// The title channel is deliberately the smallest of the content signals, and it
// is scaled down further by tag agreement (see compare): a title phrase and a
// genre tag often describe the same thing (「真正中出し」/「中出」), so letting
// both speak at full weight would count one piece of evidence twice. When the
// tags already agree the title channel falls silent; when the tags cannot tell
// two films apart, it is the only thing that can.
//
// The weights are deliberately NOT renormalised over the signals a particular
// query happens to have. Renormalising would make a film sharing only the label
// score 1.0 (the only "available" signal, so it fills the whole denominator),
// which is exactly the case that must stay weak. A missing signal is lost
// evidence, not neutral evidence.
const (
	weightActor  = 0.28
	weightTag    = 0.24
	weightSeries = 0.18
	weightTitle  = 0.20
	weightStudio = 0.08
	weightTime   = 0.06
)

const (
	// seriesSame is an exact match on the provider's 系列; seriesPrefix is the
	// weaker "same 番号 prefix" match (one label's output line).
	seriesSame   = 1.0
	seriesPrefix = 0.6
	// recencyTauDays is the time constant of the release-date prior: films
	// three years apart keep ~37% of the boost, ten years apart ~4%.
	// It is a hint about production era, never a reason on its own.
	recencyTauDays = 1095
	secondsPerDay  = 86400

	// mmrLambda trades relevance against variety when the row is filled.
	mmrLambda = 0.7
	// maxSameActor and maxSameSeries keep one performer or one series from
	// filling the row. They are preferences: the row is refilled without them
	// when the library cannot satisfy them.
	maxSameActor  = 4
	maxSameSeries = 3

	// tagReasonThreshold is how much tag cosine earns a 「同题材」 reason.
	tagReasonThreshold = 0.5
	maxReasons         = 2
)

// document is one favorite's prepared, comparable form.
type document struct {
	id         int64
	code       string
	series     string
	prefix     string
	studio     string
	release    *int64
	uncensored *bool
	actors     map[string]float64 // IDF-weighted, unit length
	actorSet   map[string]struct{}
	tags       map[string]float64 // IDF-weighted, unit length; format tags dropped
	titleVec   map[string]float64 // rare title bigrams, IDF-weighted, unit length
	titleText  string             // original title, for naming the shared phrase
	hasActors  bool
}

// Corpus is the whole library scored against itself: it owns the inverse
// document frequencies that make a rare tag («黑人演員») count more than a
// ubiquitous one. Build it once per read and rank as many queries as needed.
type Corpus struct {
	docs  []document
	index map[int64]int
}

// NewCorpus prepares every feature for scoring. It is O(features × tags); on a
// few thousand favorites this is a couple of milliseconds, cheap enough that
// the façade rebuilds it instead of caching something that could go stale.
func NewCorpus(features []Feature) *Corpus {
	type sets struct {
		actors map[string]struct{}
		tags   map[string]struct{}
		title  map[string]struct{}
	}
	// The cast vocabulary has to be complete before any title is tokenised: a
	// title that merely repeats a performer's name (or a shared surname) must
	// not earn title similarity, because the cast dimension already covers it.
	names := make([]string, 0, len(features)*2)
	for _, f := range features {
		names = append(names, f.Actors...)
	}
	nameVocab := nameVocabulary(names)

	prepared := make([]sets, len(features))
	titleSets := make([]map[string]struct{}, len(features))
	actorDF := make(map[string]int)
	tagDF := make(map[string]int)
	for i, f := range features {
		prepared[i] = sets{
			actors: make(map[string]struct{}, len(f.Actors)),
			tags:   make(map[string]struct{}, len(f.Tags)),
			title:  toSet(titleTokens(f.Title, nameVocab)),
		}
		titleSets[i] = prepared[i].title
		for _, name := range f.Actors {
			if name = strings.TrimSpace(name); name != "" {
				prepared[i].actors[name] = struct{}{}
			}
		}
		for _, tag := range f.Tags {
			key := normalizeKey(tag)
			if key == "" {
				continue
			}
			if _, format := formatTags[key]; format {
				continue
			}
			prepared[i].tags[key] = struct{}{}
		}
		for name := range prepared[i].actors {
			actorDF[name]++
		}
		for key := range prepared[i].tags {
			tagDF[key]++
		}
	}
	titleFreq := titleDocumentFrequency(titleSets)

	total := float64(len(features))
	idf := func(df int) float64 { return math.Log((total + 1) / (float64(df) + 0.5)) }

	c := &Corpus{
		docs:  make([]document, 0, len(features)),
		index: make(map[int64]int, len(features)),
	}
	for i, f := range features {
		d := document{
			id:         f.VideoID,
			code:       normalizeKey(f.Code),
			series:     normalizeKey(f.Series),
			prefix:     codePrefix(f.Code),
			studio:     normalizeKey(f.Studio),
			release:    f.ReleaseUnix,
			uncensored: f.IsUncensored,
			actorSet:   prepared[i].actors,
			titleText:  f.Title,
		}
		d.actors = weigh(prepared[i].actors, actorDF, idf)
		d.tags = weigh(prepared[i].tags, tagDF, idf)
		// Only the kept bigrams are weighed: a passed-in df of zero would give
		// a dropped (boilerplate) token a huge inverse frequency.
		kept := make(map[string]struct{}, len(prepared[i].title))
		for token := range prepared[i].title {
			if _, ok := titleFreq[token]; ok {
				kept[token] = struct{}{}
			}
		}
		d.titleVec = weigh(kept, titleFreq, idf)
		d.hasActors = len(d.actorSet) > 0
		c.index[d.id] = len(c.docs)
		c.docs = append(c.docs, d)
	}
	return c
}

// weigh turns one document's token set into an L2-normalised IDF weight map,
// i.e. the cosine denominator is folded in here once.
func weigh(set map[string]struct{}, df map[string]int, idf func(int) float64) map[string]float64 {
	if len(set) == 0 {
		return nil
	}
	weights := make(map[string]float64, len(set))
	sum := 0.0
	for key := range set {
		w := idf(df[key])
		weights[key] = w
		sum += w * w
	}
	if sum == 0 {
		return nil
	}
	norm := math.Sqrt(sum)
	for key := range weights {
		weights[key] /= norm
	}
	return weights
}

// candidate is one scored favorite waiting to be ordered.
type candidate struct {
	doc   *document
	score float64
}

// Rank returns up to limit recommendations for queryID, best first. An unknown
// id, an empty library or a non-positive limit yields no results instead of an
// error: a favorite without a scrape result simply has nothing to match on.
func (c *Corpus) Rank(queryID int64, limit int) []Result {
	qi, known := c.index[queryID]
	if !known || limit <= 0 {
		return nil
	}
	query := &c.docs[qi]

	// primary honours the 有码/无码 partition; secondary holds the films the
	// partition excluded and is only tapped when the row cannot be filled.
	primary := make([]candidate, 0, len(c.docs))
	secondary := make([]candidate, 0, len(c.docs)/8+1)
	for i := range c.docs {
		doc := &c.docs[i]
		if doc.id == query.id {
			continue
		}
		// Another copy, or another part, of the queried work is not a
		// neighbour: 「类似影片」 must not answer with the same 番号.
		if query.code != "" && doc.code == query.code {
			continue
		}
		m := compare(query, doc)
		if !m.gate() {
			continue
		}
		score := m.score()
		if score <= 0 {
			continue
		}
		cand := candidate{doc: doc, score: score}
		if sameCensoring(query, doc) {
			primary = append(primary, cand)
		} else {
			secondary = append(secondary, cand)
		}
	}
	sortCandidates(primary)
	sortCandidates(secondary)
	// The MMR pass is O(shortlist × limit), not O(library × limit): the row is
	// a handful of entries, so diversifying across the whole library is wasted
	// work. The slack keeps the per-actor/per-series caps satisfiable when one
	// performer dominates the top of the list.
	primary = shortlist(primary, limit)
	secondary = shortlist(secondary, limit)

	selected := make([]Result, 0, limit)
	picked := make([]bool, len(primary))
	actorLoad := make(map[string]int, maxSameActor*2)
	seriesLoad := make(map[string]int, maxSameSeries*2)

	// Greedy maximum-marginal-relevance pass. It decides WHO gets a slot, not
	// the order they are shown in: the point is to stop one performer or one
	// series from taking the row, and a reader expects the first card to be the
	// strongest match, not the most "diverse" one.
	for len(selected) < limit {
		best, bestValue := -1, math.Inf(-1)
		for i := range primary {
			if picked[i] {
				continue
			}
			doc := primary[i].doc
			if !capsAllow(actorLoad, seriesLoad, doc) {
				continue
			}
			penalty := 0.0
			for j := range primary {
				if !picked[j] {
					continue
				}
				if v := diversity(doc, primary[j].doc); v > penalty {
					penalty = v
				}
			}
			value := mmrLambda*primary[i].score - (1-mmrLambda)*penalty
			if value > bestValue {
				bestValue, best = value, i
			}
		}
		if best < 0 {
			break
		}
		picked[best] = true
		recordLoad(actorLoad, seriesLoad, primary[best].doc)
		selected = append(selected, result(query, primary[best]))
	}

	// The caps are a preference, not a quota: a small or lopsided library still
	// gets a full row.
	for i := range primary {
		if len(selected) >= limit {
			break
		}
		if picked[i] {
			continue
		}
		picked[i] = true
		selected = append(selected, result(query, primary[i]))
	}
	// Only now, with every same-censoring film used, relax the partition.
	for i := range secondary {
		if len(selected) >= limit {
			break
		}
		selected = append(selected, result(query, secondary[i]))
	}

	// Display in relevance order. Diversity has already done its job by
	// choosing the set; leaving the row in MMR order put a 0.586 film behind a
	// 0.245 one, which reads as a bug to anyone looking at the row.
	sort.SliceStable(selected, func(i, j int) bool {
		if selected[i].Score != selected[j].Score {
			return selected[i].Score > selected[j].Score
		}
		return selected[i].VideoID < selected[j].VideoID
	})
	return selected
}

func result(query *document, cand candidate) Result {
	return Result{
		VideoID: cand.doc.id,
		Score:   cand.score,
		Reasons: explain(query, cand.doc),
	}
}

// match is one pair's similarity broken down per signal, each in [0,1].
type match struct {
	actor, tag, series, title, studio, recency float64
	seriesExact                                bool
	prefixOnly                                 bool
}

// titleWeight is weightTitle scaled down by tag agreement, so a shared marketing
// phrase cannot be counted a second time as a shared genre. The two describe the
// same evidence (「真正中出し」/「中出」): when the tags already agree the title
// channel says nothing, and when they disagree it carries the whole 0.12.
func (m match) titleWeight() float64 { return weightTitle * (1 - m.tag) }

// gate reports whether the two films share any content signal at all. Time is
// excluded on purpose: two unrelated films released in the same month are not
// similar, and letting recency alone through would fill the row with whatever
// the query's era happened to be. The title counts as content: the phrase a
// label repeats is a real subject signal, and it is the only one available when
// the tags cannot separate two films.
func (m match) gate() bool {
	return m.actor > 0 || m.tag > 0 || m.series > 0 || m.title > 0 || m.studio > 0
}

func (m match) score() float64 {
	return weightActor*m.actor + weightTag*m.tag + weightSeries*m.series +
		m.titleWeight()*m.title + weightStudio*m.studio + weightTime*m.recency
}

func compare(q, d *document) match {
	m := match{}
	if q.hasActors && d.hasActors {
		m.actor = cosine(q.actors, d.actors)
	}
	if len(q.tags) > 0 && len(d.tags) > 0 {
		m.tag = cosine(q.tags, d.tags)
	}
	switch {
	case q.series != "" && q.series == d.series:
		m.series, m.seriesExact = seriesSame, true
	case q.prefix != "" && q.prefix == d.prefix:
		m.series, m.prefixOnly = seriesPrefix, true
	}
	if len(q.titleVec) > 0 && len(d.titleVec) > 0 {
		m.title = cosine(q.titleVec, d.titleVec)
	}
	if q.studio != "" && q.studio == d.studio {
		m.studio = 1
	}
	if q.release != nil && d.release != nil {
		days := math.Abs(float64(*q.release-*d.release)) / secondsPerDay
		m.recency = math.Exp(-days / recencyTauDays)
	}
	return m
}

// explain names the signals behind a recommendation, strongest first. It is
// only called for the handful of films that make the row.
func explain(q, d *document) []string {
	m := compare(q, d)
	type contribution struct {
		text  string
		value float64
	}
	list := make([]contribution, 0, 5)
	if m.actor > 0 {
		list = append(list, contribution{"同演员", weightActor * m.actor})
	}
	if m.tag >= tagReasonThreshold {
		list = append(list, contribution{"同题材", weightTag * m.tag})
	}
	switch {
	case m.seriesExact:
		list = append(list, contribution{"同系列", weightSeries * m.series})
	case m.prefixOnly:
		list = append(list, contribution{"同作品线", weightSeries * m.series})
	}
	if m.title >= titleReasonThreshold {
		list = append(list, contribution{titleReason(q, d), m.titleWeight() * m.title})
	}
	if m.studio > 0 {
		list = append(list, contribution{"同厂牌", weightStudio * m.studio})
	}
	if sameYear(q.release, d.release) {
		list = append(list, contribution{"同年发行", weightTime})
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].value > list[j].value })
	if len(list) > maxReasons {
		list = list[:maxReasons]
	}
	reasons := make([]string, len(list))
	for i, c := range list {
		reasons[i] = c.text
	}
	return reasons
}

// diversity is how much one film repeats another in the row; the MMR penalty.
// Series is left out because maxSameSeries already bounds it.
func diversity(a, b *document) float64 {
	sim := 0.0
	if a.hasActors && b.hasActors {
		if v := cosine(a.actors, b.actors); v > sim {
			sim = v
		}
	}
	if len(a.tags) > 0 && len(b.tags) > 0 {
		if v := cosine(a.tags, b.tags); v > sim {
			sim = v
		}
	}
	return sim
}

func capsAllow(actorLoad, seriesLoad map[string]int, doc *document) bool {
	for name := range doc.actorSet {
		if actorLoad[name] >= maxSameActor {
			return false
		}
	}
	if key := seriesKey(doc); key != "" && seriesLoad[key] >= maxSameSeries {
		return false
	}
	return true
}

func recordLoad(actorLoad, seriesLoad map[string]int, doc *document) {
	for name := range doc.actorSet {
		actorLoad[name]++
	}
	if key := seriesKey(doc); key != "" {
		seriesLoad[key]++
	}
}

// seriesKey is what the per-series cap counts: the provider's 系列 when it has
// one, otherwise the 番号 prefix (one label's output line).
func seriesKey(doc *document) string {
	if doc.series != "" {
		return doc.series
	}
	return doc.prefix
}

func sameCensoring(a, b *document) bool {
	if a.uncensored == nil || b.uncensored == nil {
		return true
	}
	return *a.uncensored == *b.uncensored
}

func sameYear(a, b *int64) bool {
	if a == nil || b == nil {
		return false
	}
	return time.Unix(*a, 0).UTC().Year() == time.Unix(*b, 0).UTC().Year()
}

func sortCandidates(list []candidate) {
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].score != list[j].score {
			return list[i].score > list[j].score
		}
		return list[i].doc.id < list[j].doc.id
	})
}

// shortlist bounds the diversification work to the candidates that could
// plausibly make the row.
func shortlist(list []candidate, limit int) []candidate {
	max := limit*8 + 24
	if len(list) > max {
		return list[:max]
	}
	return list
}

// cosine of two unit-length weight maps. Vectors are already L2-normalised, so
// the dot product is the cosine; iterating the smaller map keeps it cheap.
func cosine(a, b map[string]float64) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	if len(a) > len(b) {
		a, b = b, a
	}
	sum := 0.0
	for key, weight := range a {
		sum += weight * b[key]
	}
	return sum
}
