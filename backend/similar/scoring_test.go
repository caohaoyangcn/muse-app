package similar

import (
	"math"
	"strconv"
	"testing"
)

func ptr[T any](value T) *T { return &value }

// ids reduces a ranked row to the favorite ids, which is what the assertions
// care about.
func ids(results []Result) []int64 {
	out := make([]int64, len(results))
	for i, r := range results {
		out[i] = r.VideoID
	}
	return out
}

func equalIDs(got, want []int64) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// The cast is the strongest single cue, then the provider's series, then the
// label a work was released under.
func TestRankOrdersSharedCastAboveSameSeriesAboveSameLabel(t *testing.T) {
	query := Feature{
		VideoID: 1, Code: "AAA-001", Series: "群", Studio: "L1",
		Actors: []string{"甲"}, Tags: []string{"t1"},
	}
	cast := Feature{VideoID: 2, Code: "CCC-001", Actors: []string{"甲"}, Tags: []string{"x"}}
	series := Feature{VideoID: 3, Code: "DDD-001", Series: "群", Actors: []string{"乙"}, Tags: []string{"y"}}
	label := Feature{VideoID: 4, Code: "EEE-001", Studio: "L1", Actors: []string{"丙"}, Tags: []string{"z"}}

	got := NewCorpus([]Feature{query, cast, series, label}).Rank(1, 8)
	if want := []int64{2, 3, 4}; !equalIDs(ids(got), want) {
		t.Fatalf("order = %v, want %v", ids(got), want)
	}
}

// A shared release year is a hint, never evidence: two unrelated films from the
// same month must not be recommended as similar.
func TestRankIgnoresReleaseDateWithoutAnyContentOverlap(t *testing.T) {
	released := ptr(int64(1600000000))
	query := Feature{
		VideoID: 1, Code: "AAA-001", Actors: []string{"甲"}, Tags: []string{"t1"}, ReleaseUnix: released,
	}
	sameEra := Feature{
		VideoID: 2, Code: "BBB-001", Actors: []string{"乙"}, Tags: []string{"t2"}, ReleaseUnix: released,
	}

	if got := NewCorpus([]Feature{query, sameEra}).Rank(1, 8); len(got) != 0 {
		t.Fatalf("rank = %v, want no recommendation from the release date alone", ids(got))
	}
}

// Format tags (無碼破解/單體作品/HDTV…) describe how a work was released, not
// what it is. Two films sharing only those must not look alike.
func TestFormatTagsDoNotMakeFilmsSimilar(t *testing.T) {
	tags := []string{"無碼破解", "單體作品", "HDTV"}
	query := Feature{VideoID: 1, Code: "AAA-001", Actors: []string{"甲"}, Tags: tags}
	other := Feature{VideoID: 2, Code: "BBB-001", Actors: []string{"乙"}, Tags: tags}

	if got := NewCorpus([]Feature{query, other}).Rank(1, 8); len(got) != 0 {
		t.Fatalf("rank = %v, want the format tags ignored", ids(got))
	}
}

// A score is the sum of the evidence present, not a share of the evidence the
// query happens to have: a film matching only the label scores 0.08, it does
// not become a 1.0 match just because the query has nothing else to compare.
func TestScoreIsNotRenormalisedOverTheQuerySignals(t *testing.T) {
	query := Feature{VideoID: 1, Code: "AAA-001", Studio: "L1"}
	sameLabel := Feature{VideoID: 2, Code: "BBB-001", Studio: "L1"}

	got := NewCorpus([]Feature{query, sameLabel}).Rank(1, 8)
	if len(got) != 1 {
		t.Fatalf("rank = %v, want the one label match", ids(got))
	}
	if math.Abs(got[0].Score-weightStudio) > 1e-9 {
		t.Fatalf("score = %v, want %v", got[0].Score, weightStudio)
	}
}

// Remedies the row must never answer with: the work itself (another copy or
// part) and a favorite that left the library.
func TestRankSkipsTheSameWorkAndUnknownIDs(t *testing.T) {
	query := Feature{VideoID: 1, Code: "AAA-001", Actors: []string{"甲"}, Tags: []string{"t1"}}
	copyOfQuery := Feature{VideoID: 2, Code: "AAA-001", Actors: []string{"甲"}, Tags: []string{"t1"}}
	neighbour := Feature{VideoID: 3, Code: "BBB-001", Actors: []string{"甲"}}

	corpus := NewCorpus([]Feature{query, copyOfQuery, neighbour})
	if got := ids(corpus.Rank(1, 8)); !equalIDs(got, []int64{3}) {
		t.Fatalf("rank = %v, want only the other work", got)
	}
	if got := corpus.Rank(999, 8); len(got) != 0 {
		t.Fatalf("unknown id rank = %v, want empty", ids(got))
	}
}

// 有码 and 无码 do not belong in one row. The partition wins over a much
// stronger match, and only yields when there is nothing on the right side.
func TestRankKeepsCensoredAndUncensoredApart(t *testing.T) {
	censored := false
	uncensored := true
	query := Feature{
		VideoID: 1, Code: "AAA-001", Actors: []string{"甲"}, Tags: []string{"t1"}, IsUncensored: &censored,
	}
	strongOpposite := Feature{
		VideoID: 2, Code: "BBB-001", Actors: []string{"甲"}, Tags: []string{"t1"}, IsUncensored: &uncensored,
	}
	weakSame := Feature{
		VideoID: 3, Code: "CCC-001", Tags: []string{"t1"}, IsUncensored: &censored,
	}

	got := NewCorpus([]Feature{query, strongOpposite, weakSame}).Rank(1, 1)
	if len(got) != 1 || got[0].VideoID != 3 {
		t.Fatalf("rank = %v, want the weaker same-censoring film (3)", ids(got))
	}

	// With nothing on the same side, the row falls back instead of staying empty.
	fallback := NewCorpus([]Feature{query, strongOpposite}).Rank(1, 1)
	if len(fallback) != 1 || fallback[0].VideoID != 2 {
		t.Fatalf("fallback = %v, want the opposite-censoring film (2)", ids(fallback))
	}
}

// The per-actor cap keeps one performer from filling the row: once four films
// of the query's actress are in, the next slots go to the other candidates
// even though they score lower.
func TestRowLetsOtherCandidatesInAfterFourFilmsOfOneActress(t *testing.T) {
	query := Feature{
		VideoID: 1, Code: "AAA-001", Series: "群", Actors: []string{"甲"}, Tags: []string{"x", "y"},
	}
	features := []Feature{query}
	clones := make([]int64, 0, 8)
	for i := 0; i < 8; i++ {
		id := int64(10 + i)
		clones = append(clones, id)
		features = append(features, Feature{
			VideoID: id, Code: "BBB-00" + string(rune('1'+i)),
			Actors: []string{"甲"}, Tags: []string{"x", "y"},
		})
	}
	other := []int64{30, 31}
	for _, id := range other {
		features = append(features, Feature{
			VideoID: id, Code: "CCC-0" + string(rune('1'+id%10)), Series: "群", Actors: []string{"乙"},
		})
	}

	got := ids(NewCorpus(features).Rank(1, 8))
	if len(got) != 8 {
		t.Fatalf("row = %v, want 8 entries", got)
	}
	// The cap is a preference that yields to a full row, so the clones do come
	// back at the end — but only after both other candidates were used.
	seen := 0
	for i, id := range got {
		if contains(clones, id) {
			seen++
			if seen > maxSameActor && indexOf(got, other[0]) > i {
				t.Fatalf("row = %v, want the other candidates before clone %d", got, seen)
			}
			if seen > maxSameActor && indexOf(got, other[1]) > i {
				t.Fatalf("row = %v, want the other candidates before clone %d", got, seen)
			}
		}
	}
}

func contains(set []int64, value int64) bool {
	for _, candidate := range set {
		if candidate == value {
			return true
		}
	}
	return false
}

// Among equally relevant films, one that repeats what is already in the row
// loses to one that adds something new.
func TestRowPrefersANewSubjectOverARepeat(t *testing.T) {
	query := Feature{VideoID: 1, Code: "AAA-001", Tags: []string{"x", "y"}}
	first := Feature{VideoID: 2, Code: "BBB-001", Tags: []string{"x"}}
	repeat := Feature{VideoID: 3, Code: "CCC-001", Tags: []string{"x"}}
	fresh := Feature{VideoID: 4, Code: "DDD-001", Tags: []string{"y"}}
	// A second "y" film keeps both tags equally common, so only the repetition
	// can decide the order.
	otherFresh := Feature{VideoID: 5, Code: "EEE-001", Tags: []string{"y"}}

	got := ids(NewCorpus([]Feature{query, first, repeat, fresh, otherFresh}).Rank(1, 8))
	if got[0] != 2 {
		t.Fatalf("row = %v, want the first pick to be 2", got)
	}
	if indexOf(got, 4) > indexOf(got, 3) {
		t.Fatalf("row = %v, want the new subject (4) before the repeat (3)", got)
	}
}

// A rare tag says more about a film than a ubiquitous one, so sharing 「黑人演員」
// must outrank sharing 「巨乳」.
func TestRareTagBeatsCommonTag(t *testing.T) {
	query := Feature{VideoID: 1, Code: "AAA-001", Tags: []string{"common", "rare"}}
	rare := Feature{VideoID: 2, Code: "BBB-001", Tags: []string{"rare"}}
	common := Feature{VideoID: 3, Code: "CCC-001", Tags: []string{"common"}}

	features := []Feature{query, rare, common}
	for i := 0; i < 8; i++ {
		features = append(features, Feature{
			VideoID: int64(10 + i), Code: "FFF-00" + string(rune('1'+i)), Tags: []string{"common"},
		})
	}

	got := ids(NewCorpus(features).Rank(1, 8))
	rareAt, commonAt := indexOf(got, 2), indexOf(got, 3)
	if rareAt < 0 || commonAt < 0 {
		t.Fatalf("row = %v, want both films ranked", got)
	}
	if rareAt > commonAt {
		t.Fatalf("row = %v, want the rare-tag match first", got)
	}
}

func indexOf(list []int64, value int64) int {
	for i, candidate := range list {
		if candidate == value {
			return i
		}
	}
	return -1
}

func TestRankLimitBounds(t *testing.T) {
	query := Feature{VideoID: 1, Code: "AAA-001", Actors: []string{"甲"}}
	other := Feature{VideoID: 2, Code: "BBB-001", Actors: []string{"甲"}}
	corpus := NewCorpus([]Feature{query, other})

	for _, limit := range []int{0, -5} {
		if got := corpus.Rank(1, limit); len(got) != 0 {
			t.Fatalf("Rank(limit=%d) = %v, want empty", limit, ids(got))
		}
	}
	if got := corpus.Rank(1, 100); len(got) != 1 {
		t.Fatalf("Rank(limit=100) = %v, want the single candidate", ids(got))
	}
}

func TestRankOfAnEmptyCorpusIsEmpty(t *testing.T) {
	if got := NewCorpus(nil).Rank(1, 8); len(got) != 0 {
		t.Fatalf("rank = %v, want empty", ids(got))
	}
}

// BenchmarkRankLibrary measures the whole scoring path the façade runs per
// read: build the corpus over a library-sized set, then rank one query. The
// target in the spec is 30 ms for a few thousand favorites.
func BenchmarkRankLibrary(b *testing.B) {
	const films = 3000
	features := make([]Feature, 0, films)
	for i := 0; i < films; i++ {
		features = append(features, Feature{
			VideoID:     int64(i + 1),
			Code:        "ABC-" + strconv.Itoa(i),
			Title:       "作品" + strconv.Itoa(i%300) + " 中出し解禁 美人妻の誘惑 " + strconv.Itoa(i%97),
			Series:      "系列" + strconv.Itoa(i%37),
			Studio:      "厂牌" + strconv.Itoa(i%12),
			Actors:      []string{"演员" + strconv.Itoa(i%140)},
			Tags:        []string{"t" + strconv.Itoa(i%42), "t" + strconv.Itoa((i*7)%42), "t" + strconv.Itoa((i*13)%42)},
			ReleaseUnix: ptr(int64(1400000000 + i*86400)),
		})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewCorpus(features).Rank(1, 8)
	}
}
