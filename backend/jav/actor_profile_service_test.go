package jav

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// pngResponse is the smallest valid image body the avatar and cover writers
// accept; the exact pixels do not matter, only that decoding succeeds.
func pngResponse(t *testing.T) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return &http.Response{
		StatusCode:    http.StatusOK,
		ContentLength: int64(buf.Len()),
		Body:          io.NopCloser(bytes.NewReader(buf.Bytes())),
	}
}

// A scrape saves the profiles it received and caches their avatars next to the
// covers, so the actor index can show a picture without hitting the provider.
func TestServiceSavesActorProfilesAndAvatars(t *testing.T) {
	avatarDir := t.TempDir()
	provider := metadataProvider{
		metadata: &Metadata{
			Code:   "IPX-633",
			Title:  "Profiled",
			Actors: []string{"Alice"},
			ActorProfiles: []ActorProfile{{
				Name:      "Alice",
				Provider:  "fixture",
				StarID:    "star1",
				AvatarURL: "https://example.invalid/star1.jpg",
				Birthday:  "1998-03-31",
			}},
		},
		fetchCover: func(context.Context, string) (*http.Response, error) {
			return pngResponse(t), nil
		},
	}
	s, _, _, id := newServiceFixture(t, provider, t.TempDir(), "IPX-633.mp4")
	s.SetActorAvatarDir(avatarDir)
	s.Enqueue(id)
	waitForDetail(t, s, id, func(d *Detail) bool { return d.ScrapeStatus == StatusSucceeded })

	profile, err := s.ActorProfile("Alice")
	if err != nil {
		t.Fatal(err)
	}
	if profile == nil || profile.StarID != "star1" || profile.Birthday != "1998-03-31" {
		t.Fatalf("profile=%+v", profile)
	}
	if profile.AvatarPath != "avatars/star1.jpg" {
		t.Fatalf("avatar path = %q, want avatars/star1.jpg", profile.AvatarPath)
	}
	if _, err := os.Stat(filepath.Join(avatarDir, "star1.jpg")); err != nil {
		t.Fatalf("avatar file missing: %v", err)
	}

	actors, err := s.ListActors()
	if err != nil {
		t.Fatal(err)
	}
	if len(actors) != 1 || actors[0].Profile == nil || actors[0].Profile.AvatarPath == "" {
		t.Fatalf("actor index lacks the profile: %+v", actors)
	}
}

// A failed avatar download is an enrichment miss: the facts stay stored, the
// scrape still succeeds, and no half-written file is left behind.
func TestServiceKeepsProfileWhenAvatarDownloadFails(t *testing.T) {
	avatarDir := t.TempDir()
	provider := metadataProvider{
		metadata: &Metadata{
			Code:   "IPX-633",
			Actors: []string{"Alice"},
			ActorProfiles: []ActorProfile{{
				Name:      "Alice",
				Provider:  "fixture",
				StarID:    "star1",
				AvatarURL: "https://example.invalid/star1.jpg",
			}},
		},
		fetchCover: func(context.Context, string) (*http.Response, error) {
			return nil, errors.New("avatar host unreachable")
		},
	}
	s, _, _, id := newServiceFixture(t, provider, t.TempDir(), "IPX-633.mp4")
	s.SetActorAvatarDir(avatarDir)
	s.Enqueue(id)
	waitForDetail(t, s, id, func(d *Detail) bool { return d.ScrapeStatus == StatusSucceeded })

	profile, err := s.ActorProfile("Alice")
	if err != nil {
		t.Fatal(err)
	}
	if profile == nil || profile.StarID != "star1" {
		t.Fatalf("profile lost after a failed avatar download: %+v", profile)
	}
	if profile.AvatarPath != "" {
		t.Fatalf("avatar path = %q, want empty", profile.AvatarPath)
	}
	entries, err := os.ReadDir(avatarDir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("failed download left files behind: %v", entries)
	}
}

// Without an avatar directory the profile is still recorded: the directory is
// optional wiring, not a precondition for the scrape.
func TestServiceStoresProfilesWithoutAvatarDir(t *testing.T) {
	provider := metadataProvider{metadata: &Metadata{
		Code:   "IPX-633",
		Actors: []string{"Alice"},
		ActorProfiles: []ActorProfile{{
			Name:      "Alice",
			StarID:    "star1",
			AvatarURL: "https://example.invalid/star1.jpg",
		}},
	}}
	s, _, _, id := newServiceFixture(t, provider, t.TempDir(), "IPX-633.mp4")
	s.Enqueue(id)
	waitForDetail(t, s, id, func(d *Detail) bool { return d.ScrapeStatus == StatusSucceeded })

	profile, err := s.ActorProfile("Alice")
	if err != nil {
		t.Fatal(err)
	}
	if profile == nil || profile.AvatarPath != "" {
		t.Fatalf("profile=%+v, want stored without an avatar", profile)
	}
}
