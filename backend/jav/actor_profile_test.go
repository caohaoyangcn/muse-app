package jav

import "testing"

func intPtr(v int) *int       { return &v }
func int64Ptr(v int64) *int64 { return &v }

// A later scrape that knows less about a performer must not erase what an
// earlier one recorded: the avatar path and the measurements survive a partial
// refresh that only carries the provider id.
func TestUpsertActorProfilesKeepsFactsALaterScrapeOmits(t *testing.T) {
	_, repo, _ := newActorFixture(t, [][]string{{"A"}})

	if err := repo.UpsertActorProfiles([]ActorProfile{{
		Name:       "A",
		Provider:   "avmoo",
		StarID:     "nxyzzbk",
		AvatarURL:  "https://jp.netcdn.space/mono/actjpgs/akari_tumugi.jpg",
		Birthday:   "1998-03-31",
		HeightCM:   intPtr(155),
		BustCM:     intPtr(80),
		Cup:        "B",
		WaistCM:    intPtr(58),
		HipCM:      intPtr(83),
		BloodType:  "O",
		Hometown:   "神奈川県",
		MovieCount: intPtr(470),
	}}); err != nil {
		t.Fatal(err)
	}

	// Second write: the avatar finished downloading, and the provider repeated
	// only the stable id.
	if err := repo.UpsertActorProfiles([]ActorProfile{{
		Name:       "A",
		StarID:     "nxyzzbk",
		AvatarPath: "avatars/nxyzzbk.jpg",
		FetchedAt:  int64Ptr(1_700_000_000_000),
	}}); err != nil {
		t.Fatal(err)
	}

	profiles, err := repo.ListActorProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 {
		t.Fatalf("profiles=%+v, want one row", profiles)
	}
	got := profiles[0]
	if got.Provider != "avmoo" || got.Birthday != "1998-03-31" {
		t.Fatalf("partial refresh erased the profile: %+v", got)
	}
	if got.HeightCM == nil || *got.HeightCM != 155 || got.Cup != "B" {
		t.Fatalf("measurements lost: %+v", got)
	}
	if got.AvatarPath != "avatars/nxyzzbk.jpg" {
		t.Fatalf("avatar path = %q, want the downloaded copy", got.AvatarPath)
	}
	if got.AvatarURL == "" || got.MovieCount == nil || *got.MovieCount != 470 {
		t.Fatalf("profile facts lost: %+v", got)
	}
}

// The actor index carries the profile, and a merged alias without its own
// profile still supplies the picture to the canonical entry.
func TestListActorsAttachesProfileFromAlias(t *testing.T) {
	svc, repo, _ := newActorFixture(t, [][]string{{"A"}, {"A"}, {"B"}})

	if err := repo.UpsertActorProfiles([]ActorProfile{{
		Name:     "B",
		Provider: "avmoo",
		StarID:   "star-b",
		Birthday: "2000-01-02",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.MergeActors("B", "A"); err != nil {
		t.Fatal(err)
	}

	actors, err := svc.ListActors()
	if err != nil {
		t.Fatal(err)
	}
	if len(actors) != 1 {
		t.Fatalf("actors=%+v, want one merged entry", actors)
	}
	if actors[0].Name != "A" || actors[0].Profile == nil {
		t.Fatalf("merged actor lost its profile: %+v", actors[0])
	}
	if actors[0].Profile.StarID != "star-b" {
		t.Fatalf("profile=%+v, want the alias's profile", actors[0].Profile)
	}
}

// Asking for a profile by either spelling of a merged performer answers with
// the one that was scraped.
func TestActorProfileFollowsMerges(t *testing.T) {
	svc, repo, _ := newActorFixture(t, [][]string{{"A"}, {"B"}})

	if err := repo.UpsertActorProfiles([]ActorProfile{{
		Name:   "B",
		StarID: "star-b",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.MergeActors("B", "A"); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"A", "B"} {
		profile, err := svc.ActorProfile(name)
		if err != nil {
			t.Fatal(err)
		}
		if profile == nil || profile.StarID != "star-b" {
			t.Fatalf("ActorProfile(%q)=%+v, want the merged profile", name, profile)
		}
	}
}

// A malformed alias list on the profile row must not break the actor index.
func TestActorProfileToleratesInvalidAliasJSON(t *testing.T) {
	svc, repo, _ := newActorFixture(t, [][]string{{"A"}})

	if err := repo.UpsertActorProfiles([]ActorProfile{{Name: "A", StarID: "star-a"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.Exec(`UPDATE actor_profiles SET aliases_json='not-json'`); err != nil {
		t.Fatal(err)
	}
	actors, err := svc.ListActors()
	if err != nil {
		t.Fatalf("invalid aliases must not fail the index: %v", err)
	}
	if len(actors) != 1 || actors[0].Profile == nil {
		t.Fatalf("actors=%+v, want the profile without aliases", actors)
	}
	if len(actors[0].Profile.Aliases) != 0 {
		t.Fatalf("aliases=%v, want none", actors[0].Profile.Aliases)
	}
}

// Unknown and empty names answer with no profile rather than an error.
func TestActorProfileUnknownReturnsNil(t *testing.T) {
	svc, _, _ := newActorFixture(t, [][]string{{"A"}})
	for _, name := range []string{"", "  ", "nobody"} {
		profile, err := svc.ActorProfile(name)
		if err != nil {
			t.Fatalf("ActorProfile(%q): %v", name, err)
		}
		if profile != nil {
			t.Fatalf("ActorProfile(%q)=%+v, want nil", name, profile)
		}
	}
}
