package jav

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"muse-app/backend/similar"
)

// SimilarRow is one scraped favorite as stored: the JSON lists are still raw
// and the cast still carries the stage names the provider published. It exists
// so the repository owns the query and the service owns the interpretation,
// like the other read projections in this package.
type SimilarRow struct {
	VideoID      int64
	Code         string
	Title        string
	Series       string
	Studio       string
	ReleaseUnix  *int64
	IsUncensored *bool
	ActorsJSON   string
	TagsJSON     string
}

// ListSimilarRows returns every scraped favorite still in the library. Trashed
// rows are excluded by the same deleted_at rule as the rest of the read model;
// a row whose scrape never produced anything is still returned (the scorer
// simply finds nothing to compare it on) so callers do not have to guess which
// half-scraped favorites exist.
func (r *SQLiteRepository) ListSimilarRows() ([]SimilarRow, error) {
	rows, err := r.db.Query(`
		SELECT j.video_id, j.code, j.title, j.series, j.studio, j.release_unix,
		       j.is_uncensored, j.actors_json, j.tags_json
		FROM video_jav j
		JOIN videos v ON v.id = j.video_id AND v.deleted_at IS NULL
		ORDER BY j.video_id`)
	if err != nil {
		return nil, fmt.Errorf("list similar rows: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]SimilarRow, 0)
	for rows.Next() {
		var (
			row        SimilarRow
			release    sql.NullInt64
			uncensored sql.NullInt64
		)
		if err := rows.Scan(&row.VideoID, &row.Code, &row.Title, &row.Series, &row.Studio, &release,
			&uncensored, &row.ActorsJSON, &row.TagsJSON); err != nil {
			return nil, fmt.Errorf("scan similar row: %w", err)
		}
		if release.Valid {
			value := release.Int64
			row.ReleaseUnix = &value
		}
		if uncensored.Valid {
			value := uncensored.Int64 != 0
			row.IsUncensored = &value
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// SimilarFeatures returns the scoring features of every scraped favorite that
// has an original title.
//
// A film without one does not participate at all: the title is the only text
// the title channel can match on, and a favorite the scraper never produced a
// title for cannot be compared on it. (It is also the honest reading of
// "similar": nothing to describe it, nothing to say.)
//
// The cast is resolved exactly like ActorsByVideo does it, so 演员合并 applies
// here too: two stage names for one performer must not look like two different
// people to the recommender. Genre tags are passed through untouched — the
// scorer owns the list of tags that describe format rather than style.
func (s *Service) SimilarFeatures() ([]similar.Feature, error) {
	rows, err := s.repo.ListSimilarRows()
	if err != nil {
		return nil, err
	}
	merges, err := s.repo.ListActorMerges()
	if err != nil {
		return nil, err
	}
	aliases := actorAliasMap(merges)

	out := make([]similar.Feature, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.Title) == "" {
			continue
		}
		out = append(out, similar.Feature{
			VideoID:      row.VideoID,
			Code:         row.Code,
			Title:        row.Title,
			Series:       row.Series,
			Studio:       row.Studio,
			ReleaseUnix:  row.ReleaseUnix,
			IsUncensored: row.IsUncensored,
			Actors:       canonicalActors(parseStringList(row.ActorsJSON), aliases),
			Tags:         parseStringList(row.TagsJSON),
		})
	}
	return out, nil
}

// canonicalActors folds 演员合并 aliases into their canonical name, dropping
// duplicates and empty names, and sorts the result so the feature is stable
// across reads (map iteration order is not).
func canonicalActors(names []string, aliases map[string]string) []string {
	if len(names) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(names))
	out := make([]string, 0, len(names))
	for _, name := range names {
		canonical := resolveActor(name, aliases)
		if canonical == "" {
			continue
		}
		if _, dup := seen[canonical]; dup {
			continue
		}
		seen[canonical] = struct{}{}
		out = append(out, canonical)
	}
	sort.Strings(out)
	return out
}
