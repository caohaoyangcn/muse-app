package jav

import "strings"

// ActorProfile returns the stored profile for one actor, resolving merged stage
// names: asking for an alias answers with the profile of the actor that alias
// was merged into, and vice versa. No profile is not an error — most actors in
// a library scraped before profiles were recorded simply have none yet.
func (s *Service) ActorProfile(name string) (*ActorProfile, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, nil
	}
	merges, err := s.repo.ListActorMerges()
	if err != nil {
		return nil, err
	}
	names := actorGroupNames(resolveActor(name, actorAliasMap(merges)), merges)
	profiles, err := s.profilesByName()
	if err != nil {
		return nil, err
	}
	for _, candidate := range names {
		if profile := profiles[candidate]; profile != nil {
			return profile, nil
		}
	}
	return nil, nil
}

// attachActorProfiles decorates the aggregated index with profiles. The
// canonical name wins; a merged alias that carries the only profile still
// supplies the avatar, so merging two spellings never loses the picture.
func (s *Service) attachActorProfiles(actors []ActorSummary) error {
	if len(actors) == 0 {
		return nil
	}
	profiles, err := s.profilesByName()
	if err != nil {
		return err
	}
	for i := range actors {
		profile := profiles[actors[i].Name]
		if profile == nil {
			for _, alias := range actors[i].Aliases {
				if candidate := profiles[alias]; candidate != nil {
					profile = candidate
					break
				}
			}
		}
		actors[i].Profile = profile
	}
	return nil
}

// profilesByName indexes the stored profiles. The returned pointers are fresh
// copies: callers decorate their own read models, and a shared pointer would let
// one view's decoration leak into another's.
func (s *Service) profilesByName() (map[string]*ActorProfile, error) {
	stored, err := s.repo.ListActorProfiles()
	if err != nil {
		return nil, err
	}
	out := make(map[string]*ActorProfile, len(stored))
	for i := range stored {
		profile := stored[i]
		out[profile.Name] = &profile
	}
	return out, nil
}
