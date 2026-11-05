package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"muse-app/backend/jav"
)

func TestAvmooLookupParsesMetadataAndReusesSession(t *testing.T) {
	var mu sync.Mutex
	var sessionRequests, searchRequests, detailRequests int
	var gotSearch []any
	var gotDetail []any
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/cn/search/IPX-228":
			sessionRequests++
			w.Header().Add("Set-Cookie", "avmoo_session=session-secret; Path=/")
			_, _ = fmt.Fprint(w, `<html><head><meta name="csrf-token" content="csrf-secret"></head></html>`)
		case "/jav/data/api/search":
			searchRequests++
			if r.Method != http.MethodPost || r.Header.Get("X-CSRF-Token") != "csrf-secret" || r.Header.Get("Cookie") != "avmoo_session=session-secret" || r.Header.Get("Accept-Language") != avmooAcceptLanguage {
				t.Errorf("search headers/method invalid: method=%s headers=%v", r.Method, r.Header)
			}
			if err := json.NewDecoder(r.Body).Decode(&gotSearch); err != nil {
				t.Errorf("decode search body: %v", err)
			}
			writeAvmooEnvelope(t, w, []map[string]any{
				{"movieId": "wrong", "movieFanHao": "IPX-228R"},
				{"movieId": "movie-228", "movieFanHao": "IPX-228"},
			})
		case "/jav/data/api/getMovie":
			detailRequests++
			if err := json.NewDecoder(r.Body).Decode(&gotDetail); err != nil {
				t.Errorf("decode detail body: %v", err)
			}
			writeAvmooEnvelope(t, w, map[string]any{
				"movieId": "movie-228", "movieFanHao": "IPX-228", "title": "API title",
				"title_tw": "Traditional title", "releaseDate": "2024-01-02", "length": 123,
				"posterLarge": server.URL + "/cover.jpg",
				"studio":      map[string]any{"studioName": "Studio X"},
				"series":      map[string]any{"seriesName": "Series X"},
				"genre":       []map[string]any{{"genreName": "Drama"}, {"genreName": "Drama"}, {"genreName_tw": "Action"}},
				"star":        []map[string]any{{"starName": "Alice"}, {"starName": "Alice"}, {"starName_cn": "Bob"}},
			})
		case "/cover.jpg":
			if cookie := r.Header.Get("Cookie"); cookie != "" {
				t.Errorf("cover request leaked session cookie %q", cookie)
			}
			if r.Header.Get("User-Agent") == "" || r.Header.Get("Accept-Language") == "" {
				t.Errorf("cover request missing browser headers: %v", r.Header)
			}
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(testAvmooPNG(t))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := newAvmooTestProvider(server)
	first, err := provider.Lookup(context.Background(), "ipx-228")
	if err != nil {
		t.Fatal(err)
	}
	second, err := provider.Lookup(context.Background(), "IPX-228")
	if err != nil {
		t.Fatal(err)
	}
	for _, metadata := range []*jav.Metadata{first, second} {
		if metadata.Code != "IPX-228" || metadata.Title != "API title" || metadata.Studio != "Studio X" || metadata.Series != "Series X" {
			t.Fatalf("metadata identity = %#v", metadata)
		}
		if metadata.DurationMin == nil || *metadata.DurationMin != 123 || metadata.ReleaseUnix == nil {
			t.Fatalf("metadata date/duration = %#v", metadata)
		}
		if metadata.Provider != "avmoo" || metadata.IsUncensored == nil || *metadata.IsUncensored {
			t.Fatalf("metadata provider/uncensored = %#v", metadata)
		}
		if !equalStrings(metadata.Tags, []string{"Drama", "Action"}) || !equalStrings(metadata.Actors, []string{"Alice", "Bob"}) {
			t.Fatalf("metadata lists = %#v", metadata)
		}
	}
	if sessionRequests != 1 || searchRequests != 2 || detailRequests != 2 {
		t.Fatalf("request counts session=%d search=%d detail=%d", sessionRequests, searchRequests, detailRequests)
	}
	if len(gotSearch) != 3 || len(gotDetail) != 2 {
		t.Fatalf("payloads search=%#v detail=%#v", gotSearch, gotDetail)
	}
	response, err := provider.Fetch(context.Background(), server.URL+"/cover.jpg")
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
}

// Every movie payload already carries its cast's profiles, so one lookup
// records the avatar, the measurements and the stable performer id without any
// extra request.
func TestAvmooLookupMapsStarProfiles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cn/search/IPX-633":
			w.Header().Add("Set-Cookie", "session=secret; Path=/")
			_, _ = fmt.Fprint(w, `<meta name="csrf-token" content="token">`)
		case "/jav/data/api/search":
			writeAvmooEnvelope(t, w, []map[string]any{{"movieId": "movie-633", "movieFanHao": "IPX-633"}})
		case "/jav/data/api/getMovie":
			writeAvmooEnvelope(t, w, map[string]any{
				"movieId": "movie-633", "movieFanHao": "IPX-633", "title": "IPX-633",
				"star": []map[string]any{{
					"starId":          "nxyzzbk",
					"starName":        "明里つむぎ",
					"starName_ja":     "明里つむぎ",
					"starName_en":     "Akari Tsumugi",
					"avatarUrl":       "https://jp.netcdn.space/mono/actjpgs/akari_tumugi.jpg",
					"birthday":        "1998-03-31",
					"bloodType":       "O",
					"hometown":        "神奈川県",
					"hobby":           "アイドル研究",
					"movieCount":      470,
					"lastReleaseDate": "2026-10-30",
					"size":            map[string]any{"T": "155", "B": "80", "C": "B", "W": "58", "H": "83"},
				}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	metadata, err := newAvmooTestProvider(server).Lookup(context.Background(), "IPX-633")
	if err != nil {
		t.Fatal(err)
	}
	if !equalStrings(metadata.Actors, []string{"明里つむぎ"}) {
		t.Fatalf("actors=%v", metadata.Actors)
	}
	if len(metadata.ActorProfiles) != 1 {
		t.Fatalf("profiles=%+v, want one", metadata.ActorProfiles)
	}
	profile := metadata.ActorProfiles[0]
	if profile.Name != "明里つむぎ" || profile.StarID != "nxyzzbk" || profile.Provider != "avmoo" {
		t.Fatalf("profile identity=%+v", profile)
	}
	if profile.AvatarURL == "" || profile.Birthday != "1998-03-31" || profile.Cup != "B" || profile.BloodType != "O" {
		t.Fatalf("profile facts=%+v", profile)
	}
	if profile.HeightCM == nil || *profile.HeightCM != 155 ||
		profile.BustCM == nil || *profile.BustCM != 80 ||
		profile.WaistCM == nil || *profile.WaistCM != 58 ||
		profile.HipCM == nil || *profile.HipCM != 83 {
		t.Fatalf("profile measurements=%+v", profile)
	}
	if profile.MovieCount == nil || *profile.MovieCount != 470 {
		t.Fatalf("profile movie count=%+v", profile)
	}
	if profile.LastReleaseUnix == nil {
		t.Fatal("profile last release date missing")
	}
	if !equalStrings(profile.Aliases, []string{"Akari Tsumugi"}) {
		t.Fatalf("profile aliases=%v", profile.Aliases)
	}
}

// A cast list without profiles (a name-only payload) stays exactly as before:
// names only, no empty profile rows.
func TestAvmooLookupKeepsNameOnlyCastWithoutProfiles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cn/search/IPX-228":
			w.Header().Add("Set-Cookie", "session=secret; Path=/")
			_, _ = fmt.Fprint(w, `<meta name="csrf-token" content="token">`)
		case "/jav/data/api/search":
			writeAvmooEnvelope(t, w, []map[string]any{{"movieId": "movie", "movieFanHao": "IPX-228"}})
		case "/jav/data/api/getMovie":
			writeAvmooEnvelope(t, w, map[string]any{
				"movieId": "movie", "movieFanHao": "IPX-228", "title": "t",
				"star": []map[string]any{{"starName": "Alice"}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	metadata, err := newAvmooTestProvider(server).Lookup(context.Background(), "IPX-228")
	if err != nil {
		t.Fatal(err)
	}
	if !equalStrings(metadata.Actors, []string{"Alice"}) {
		t.Fatalf("actors=%v", metadata.Actors)
	}
	if len(metadata.ActorProfiles) != 0 {
		t.Fatalf("profiles=%+v, want none for a name-only cast", metadata.ActorProfiles)
	}
}

// The backfill path: a performer the library only has a name for is resolved
// through their works, and the profile is keyed by the library's spelling so it
// joins the actor index instead of creating a second entry.
func TestAvmooResolveActorKeysTheRequestedName(t *testing.T) {
	var probes []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/cn/search/"):
			w.Header().Add("Set-Cookie", "session=secret; Path=/")
			_, _ = fmt.Fprint(w, `<meta name="csrf-token" content="token">`)
		case r.URL.Path == "/jav/data/api/search":
			var payload []any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode search body: %v", err)
			}
			writeAvmooEnvelope(t, w, []map[string]any{
				{"movieId": "m1", "movieFanHao": "AAA-001"},
				{"movieId": "m2", "movieFanHao": "BBB-002"},
			})
		case r.URL.Path == "/jav/data/api/getMovie":
			var payload []any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode movie body: %v", err)
			}
			movieID, _ := payload[0].(string)
			probes = append(probes, movieID)
			star := map[string]any{"starId": "other", "starName": "別の人"}
			if movieID == "m2" {
				star = map[string]any{
					"starId":    "nxyzzbk",
					"starName":  "明里つむぎ",
					"avatarUrl": "https://jp.netcdn.space/mono/actjpgs/akari_tumugi.jpg",
					"birthday":  "1998-03-31",
					"size":      map[string]any{"T": "155"},
				}
			}
			writeAvmooEnvelope(t, w, map[string]any{"movieId": movieID, "star": []map[string]any{star}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	// The library's spelling carries a space the provider's does not.
	profile, err := newAvmooTestProvider(server).ResolveActor(context.Background(), "明里 つむぎ")
	if err != nil {
		t.Fatal(err)
	}
	if profile.Name != "明里 つむぎ" {
		t.Fatalf("profile name = %q, want the requested spelling", profile.Name)
	}
	if profile.StarID != "nxyzzbk" || profile.Birthday != "1998-03-31" || profile.AvatarURL == "" {
		t.Fatalf("profile=%+v", profile)
	}
	if !equalStrings(profile.Aliases, []string{"明里つむぎ"}) {
		t.Fatalf("aliases=%v, want the provider's spelling", profile.Aliases)
	}
	if !equalStrings(probes, []string{"m1", "m2"}) {
		t.Fatalf("probes=%v, want both works inspected", probes)
	}
}

// A name no cast list mentions is a terminal not_found, which is what keeps the
// backfill queue from retrying it forever.
func TestAvmooResolveActorNotFound(t *testing.T) {
	var probes int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/cn/search/"):
			w.Header().Add("Set-Cookie", "session=secret; Path=/")
			_, _ = fmt.Fprint(w, `<meta name="csrf-token" content="token">`)
		case r.URL.Path == "/jav/data/api/search":
			results := make([]map[string]any, 0, avmooActorMovieProbes+2)
			for i := 0; i < avmooActorMovieProbes+2; i++ {
				results = append(results, map[string]any{"movieId": fmt.Sprintf("m%d", i)})
			}
			writeAvmooEnvelope(t, w, results)
		case r.URL.Path == "/jav/data/api/getMovie":
			probes++
			writeAvmooEnvelope(t, w, map[string]any{
				"star": []map[string]any{{"starId": "other", "starName": "別の人"}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	_, err := newAvmooTestProvider(server).ResolveActor(context.Background(), "知らない人")
	assertLookupKind(t, err, "not_found")
	if probes > avmooActorMovieProbes {
		t.Fatalf("probed %d works, want at most %d", probes, avmooActorMovieProbes)
	}
}

func TestAvmooLookupRequiresExactCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cn/search/IPX-228":
			w.Header().Add("Set-Cookie", "session=secret; Path=/")
			_, _ = fmt.Fprint(w, `<meta name="csrf-token" content="token">`)
		case "/jav/data/api/search":
			writeAvmooEnvelope(t, w, []map[string]any{{"movieId": "wrong", "movieFanHao": "IPX-228R"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	_, err := newAvmooTestProvider(server).Lookup(context.Background(), "IPX-228")
	assertLookupKind(t, err, "not_found")
}

func TestAvmooRefreshesSessionOnUnauthorized(t *testing.T) {
	var sessionRequests, searchRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cn/search/IPX-228":
			sessionRequests++
			w.Header().Add("Set-Cookie", fmt.Sprintf("session=secret-%d; Path=/", sessionRequests))
			_, _ = fmt.Fprintf(w, `<meta name="csrf-token" content="token-%d">`, sessionRequests)
		case "/jav/data/api/search":
			searchRequests++
			if searchRequests == 1 {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			writeAvmooEnvelope(t, w, []map[string]any{{"movieId": "movie", "movieFanHao": "IPX-228"}})
		case "/jav/data/api/getMovie":
			writeAvmooEnvelope(t, w, map[string]any{"movieId": "movie", "movieFanHao": "IPX-228", "title": "refreshed"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	metadata, err := newAvmooTestProvider(server).Lookup(context.Background(), "IPX-228")
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Title != "refreshed" || sessionRequests != 2 || searchRequests != 2 {
		t.Fatalf("metadata=%#v sessionRequests=%d searchRequests=%d", metadata, sessionRequests, searchRequests)
	}
}

func TestAvmooRetryableStatusDoesNotExposeSecrets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cn/search/IPX-228":
			w.Header().Add("Set-Cookie", "session=secret-cookie; Path=/")
			_, _ = fmt.Fprint(w, `<meta name="csrf-token" content="secret-token">`)
		case "/jav/data/api/search":
			w.WriteHeader(http.StatusTooManyRequests)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := newAvmooTestProvider(server).Lookup(ctx, "IPX-228")
	assertLookupKind(t, err, "retryable")
	if strings.Contains(err.Error(), "secret-cookie") || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("error exposed session secret: %v", err)
	}
}

func TestAvmooClassifiesNetworkError(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("dial failed")
	})}
	_, err := NewAvmooWithConfig(client, "http://avmoo.invalid", AvmooConfig{Timeout: time.Second}).Lookup(context.Background(), "IPX-228")
	assertLookupKind(t, err, "network")
}

func newAvmooTestProvider(server *httptest.Server) *Avmoo {
	return newAvmooWithLimiter(server.Client(), server.URL, AvmooConfig{Timeout: time.Second}, newIntervalLimiter(0))
}

func writeAvmooEnvelope(t *testing.T, w http.ResponseWriter, data any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "message": "ok", "data": data})
}

func testAvmooPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func equalStrings(got, want []string) bool {
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
