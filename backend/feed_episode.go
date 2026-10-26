package main

import (
	"strconv"

	keywordfilter "auto-anime/internal/filter"
	titleparse "auto-anime/internal/title"
)

func validEpisodeOffset(offset *int) bool {
	return offset == nil || (*offset >= -9999 && *offset <= 9999)
}

func filteredFeedItems(entries []feedEntry, include, exclude string, season int, unknown bool) []feedItem {
	filter := keywordfilter.New(include, exclude)
	var out []feedItem
	for _, item := range feedItemsFromEntries(entries) {
		if item.Title != "" && item.Link != "" && filter.Allows(item.Title) && titleparse.AllowsSeason(item.Title, season, unknown) {
			out = append(out, item)
		}
	}
	return out
}

// A feed spanning different series/seasons cannot safely share one offset.
func inferEpisodeOffset(items []feedItem) *int {
	minimum := 10000
	series, season := "", 0
	for _, item := range items {
		info := titleparse.Extract(item.Title)
		n, err := strconv.Atoi(info.Episode)
		if err != nil || n < 0 || n > 9999 || info.Title == "" {
			continue
		}
		if series != "" && (series != info.Title || season != info.Season) {
			return nil
		}
		series, season = info.Title, info.Season
		minimum = min(minimum, n)
	}
	if minimum == 10000 {
		return nil
	}
	offset := 1 - minimum
	return &offset
}

func offsetEpisode(episode string, offset int) string {
	n, err := strconv.Atoi(episode)
	if offset == 0 || err != nil || n+offset < 1 || n+offset > 9999 {
		return episode
	}
	return strconv.Itoa(n + offset)
}
