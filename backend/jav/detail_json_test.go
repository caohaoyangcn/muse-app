package jav

import (
	"encoding/json"
	"testing"
)

// The web UI reads detail.actors.length and detail.tags.length while rendering
// the detail panel, so an empty list must arrive as [] rather than null. A nil
// slice marshals to null, which is exactly how a scraped work with no actresses
// crashes the panel.
func TestDetailJSONSerializesEmptyActorsAndTagsAsArrays(t *testing.T) {
	s, _, _, id := newServiceFixture(t,
		metadataProvider{metadata: &Metadata{Code: "SDAM-8302", Title: "No actresses"}},
		"", "SDAM-8302.mp4")
	s.Enqueue(id)
	d := waitForDetail(t, s, id, func(d *Detail) bool { return d.ScrapeStatus == StatusSucceeded })

	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"actors", "tags"} {
		if got := string(payload[key]); got != "[]" {
			t.Fatalf("%s = %s, want [] (payload: %s)", key, got, raw)
		}
	}
}
