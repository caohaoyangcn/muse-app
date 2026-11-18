package similar

import (
	"math"
	"strings"
	"testing"
)

// The whole point of the title channel: a marketing phrase a label repeats is a
// subject signal the genre tags do not carry, so two films that share only it
// still belong in each other's row — and the card can say why.
func TestTitlePhraseConnectsFilmsWithNoOtherOverlap(t *testing.T) {
	query := Feature{
		VideoID: 1, Code: "AAA-001", Title: "淫語と杭打ち騎乗位で朝まで痴女られる",
		Tags: []string{"x"}, Actors: []string{"甲"},
	}
	phraseTwin := Feature{
		VideoID: 2, Code: "BBB-001", Title: "杭打ち騎乗位で痴女り続ける隣人",
		Tags: []string{"y"},
	}
	unrelated := Feature{
		VideoID: 3, Code: "CCC-001", Title: "温泉旅行でまったり過ごす休日",
		Tags: []string{"z"},
	}

	got := NewCorpus([]Feature{query, phraseTwin, unrelated}).Rank(1, 8)
	if len(got) != 1 || got[0].VideoID != 2 {
		t.Fatalf("rank = %v, want only the shared-phrase film", ids(got))
	}
	reason := strings.Join(got[0].Reasons, "|")
	if !strings.Contains(reason, "同关键词") {
		t.Fatalf("reasons = %v, want a 同关键词 reason", got[0].Reasons)
	}
	if !strings.Contains(reason, "杭打ち騎乗位") {
		t.Errorf("reason = %q, want the shared phrase named", reason)
	}
}

// A shared surname is not a shared subject, and the cast dimension already
// scores names. A title that merely repeats a performer's name must not earn
// title similarity.
func TestActressNameInTitleDoesNotCreateSimilarity(t *testing.T) {
	query := Feature{
		VideoID: 1, Code: "AAA-001", Title: "竹内あいの発情ボイン",
		Actors: []string{"竹内あい"},
	}
	namesake := Feature{
		VideoID: 2, Code: "BBB-001", Title: "竹内夏希の初撮り",
		Actors: []string{"竹内夏希"},
	}
	// Another film is needed so the name bigrams have a document frequency
	// worth filtering.
	filler := Feature{VideoID: 3, Code: "CCC-001", Title: "関係のない作品", Actors: []string{"別人"}}

	if got := NewCorpus([]Feature{query, namesake, filler}).Rank(1, 8); len(got) != 0 {
		t.Fatalf("rank = %v, want nothing: the only overlap is a surname", ids(got))
	}
}

// Release boilerplate is not content. On a shelf of blu-ray releases the suffix
// must not become the strongest shared phrase in the library.
func TestReleaseBoilerplateDoesNotCreateSimilarity(t *testing.T) {
	features := []Feature{
		{VideoID: 1, Code: "AAA-001", Title: "淫語と杭打ち騎乗位で痴女られる ブルーレイディスク"},
		{VideoID: 2, Code: "BBB-001", Title: "絶対領域の美少女メイド ブルーレイディスク"},
	}
	for i := 0; i < 38; i++ {
		features = append(features, Feature{
			VideoID: int64(100 + i),
			Code:    "DDD-" + strings.Repeat("0", 1) + string(rune('A'+i%26)) + string(rune('a'+i/26)),
			Title:   "独自タイトル" + string(rune('あ'+i)) + " ブルーレイディスク",
		})
	}

	if got := NewCorpus(features).Rank(1, 8); len(got) != 0 {
		t.Fatalf("rank = %v, want nothing: the shared suffix is boilerplate", ids(got))
	}
}

// The anti-double-counting rule: when the tags already agree, the title channel
// stays silent. Two films with identical tags score the same whether or not
// their titles also happen to read alike.
func TestTagAgreementSilencesTheTitleChannel(t *testing.T) {
	query := Feature{
		VideoID: 1, Code: "AAA-001", Title: "淫語と杭打ち騎乗位で痴女られる",
		Tags: []string{"x", "y"},
	}
	sameTitleAndTags := Feature{
		VideoID: 2, Code: "BBB-001", Title: "淫語と杭打ち騎乗位で痴女られる",
		Tags: []string{"x", "y"},
	}
	sameTagsOnly := Feature{
		VideoID: 3, Code: "CCC-001", Title: "まったく別の題名の作品",
		Tags: []string{"x", "y"},
	}

	got := NewCorpus([]Feature{query, sameTitleAndTags, sameTagsOnly}).Rank(1, 8)
	if len(got) != 2 {
		t.Fatalf("rank = %v, want both tag matches", ids(got))
	}
	// Cosine is float arithmetic, so "silent" means within rounding, not
	// bit-identical.
	if diff := math.Abs(got[0].Score - got[1].Score); diff > 1e-9 {
		t.Fatalf("scores = %v / %v (diff %v), want the same: the title must not be counted twice",
			got[0].Score, got[1].Score, diff)
	}
}

// A title phrase is supporting evidence, not a replacement for the cast: a
// film sharing only the phrase must rank below one sharing only the actress.
func TestTitleEvidenceWeighsLessThanTheCast(t *testing.T) {
	query := Feature{
		VideoID: 1, Code: "AAA-001", Title: "淫語と杭打ち騎乗位で痴女られる",
		Actors: []string{"甲"}, Tags: []string{"x"},
	}
	phraseTwin := Feature{VideoID: 2, Code: "BBB-001", Title: "杭打ち騎乗位で痴女る人妻"}
	castTwin := Feature{VideoID: 3, Code: "CCC-001", Title: "まったく別の題名", Actors: []string{"甲"}}

	got := NewCorpus([]Feature{query, phraseTwin, castTwin}).Rank(1, 8)
	if len(got) != 2 || got[0].VideoID != 3 {
		t.Fatalf("rank = %v, want the cast match first", ids(got))
	}
}

// The reason must quote text both titles actually contain, never a phrase the
// scorer did not use.
func TestSharedPhraseQuotesBothTitles(t *testing.T) {
	shared := map[string]float64{"杭打": 1.0}
	got := sharedPhrase("淫語と杭打ち騎乗位で痴女られる", "杭打ち騎乗位で痴女り続ける", shared)
	if !strings.Contains(got, "杭打") {
		t.Fatalf("phrase = %q, want it to start from the shared bigram", got)
	}
	if !strings.Contains("淫語と杭打ち騎乗位で痴女られる", got) {
		t.Errorf("phrase = %q, want a substring of the query title", got)
	}
	if !strings.Contains("杭打ち騎乗位で痴女り続ける", got) {
		t.Errorf("phrase = %q, want a substring of the candidate title too", got)
	}
}
