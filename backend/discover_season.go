package main

import (
	"strconv"
	"strings"

	titleparse "auto-anime/internal/title"
)

// Hints never replace title evidence or enter subscription filters/archival.
// Require two shared episodes in the same RSS, with no competing season.
func inferDiscoverSeasons(items []discoverItem) {
	type evidence struct {
		season   int
		episodes map[int]bool
	}
	type seriesKey struct{ feed, name string }
	type versionKey struct {
		seriesKey
		group, quality string
	}
	known := map[seriesKey]*evidence{}
	unknown := map[versionKey][]int{}
	for i, item := range items {
		items[i].InferredSeason = 0
		name, _, part := titleparse.TitleParts(item.AnimeTitle)
		if name == "" || item.FeedURL == "" || part != 0 {
			continue
		}
		key := seriesKey{item.FeedURL, name}
		if item.Season == 0 {
			version := versionKey{key, strings.ToLower(item.Group), strings.ToLower(item.Quality)}
			unknown[version] = append(unknown[version], i)
			continue
		}
		e := known[key]
		if e == nil {
			e = &evidence{season: item.Season, episodes: map[int]bool{}}
			known[key] = e
		}
		if e.season != item.Season {
			e.season = -1
		}
		if episode, err := strconv.Atoi(item.Episode); err == nil && episode > 0 {
			e.episodes[episode] = true
		}
	}
	for version, indices := range unknown {
		e := known[version.seriesKey]
		if e == nil || e.season <= 0 {
			continue
		}
		shared := map[int]bool{}
		for _, i := range indices {
			if episode, err := strconv.Atoi(items[i].Episode); err == nil && e.episodes[episode] {
				shared[episode] = true
			}
		}
		if len(shared) >= 2 {
			for _, i := range indices {
				items[i].InferredSeason = e.season
			}
		}
	}
}
