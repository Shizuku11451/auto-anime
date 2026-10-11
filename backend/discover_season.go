package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Search intent only consumes an explicit trailing marker, never title numbers,
// episode numbers, or Part/Cour labels. The original input remains visible.
var discoverSeasonSuffix = regexp.MustCompile(`(?i)(?:\s*第\s*([0-9一二两三四五六七八九十]{1,3})\s*[季期]|\s+([0-9]{1,2})\s*[季期]|\s+s([0-9]{1,2})|\s+season\s+([0-9]{1,2})|\s+([0-9]{1,2})(?:st|nd|rd|th)\s+season)\s*$`)
var explicitCNSeason = regexp.MustCompile(`^(?:第\s*[0-9零〇一二两三四五六七八九十]+\s*[季期]|[0-9]+\s*[季期])$`)

func discoverIntent(keyword string) (string, int) {
	loc := discoverSeasonSuffix.FindStringSubmatchIndex(keyword)
	if loc == nil {
		return keyword, 0
	}
	base := strings.TrimSpace(keyword[:loc[0]])
	if len([]rune(base)) < 2 {
		return keyword, 0
	}
	for i := 2; i < len(loc); i += 2 {
		if loc[i] >= 0 {
			if n, ok := parseSeasonNumber(keyword[loc[i]:loc[i+1]]); ok && n > 0 && n <= 99 {
				return base, n
			}
		}
	}
	return keyword, 0
}

// Unknown (0) and conflicting (-1) seasons must not inherit archiving's default
// Season 01. Part/Cour is deliberately distinct from season.
func resourceSeason(title string) int {
	// Underscores delimit release tokens just like spaces in archiving's parser.
	title = strings.ReplaceAll(title, "_", " ")
	// Metadata's fuzzy matcher accepts bare Chinese numerals; search evidence
	// must not turn a work named 四季青春 into season 4.
	title = metaCNSeason.ReplaceAllStringFunc(title, func(marker string) string {
		if explicitCNSeason.MatchString(marker) {
			return marker
		}
		return " "
	})
	_, season, _ := parseMetaMarkers(title)
	for _, marker := range episodeTagPattern.FindAllStringSubmatch(title, -1) {
		season = mergeMetaNumber(season, metaNumber(marker[1]))
	}
	// Reuse archiving's unambiguous Roman markers. Metadata matching only
	// accepts a suffix, but releases usually append episodes and technical tags.
	for _, marker := range asciiRomanSuffix.FindAllString(title, -1) {
		season = mergeMetaNumber(season, romanNumber(marker))
	}
	for _, marker := range romanRunePattern.FindAllString(title, -1) {
		season = mergeMetaNumber(season, romanNumber(marker))
	}
	return season
}

func validSeasonFilter(season int) bool { return season >= -1 && season <= 999 }

func allowsSeason(title string, season int, allowUnknown bool) bool {
	if season == 0 {
		return true
	}
	actual := resourceSeason(title)
	if season == -1 {
		return actual == 0
	}
	return actual == season || (actual == 0 && allowUnknown)
}

func discoverQueries(keyword, base string, season int) []string {
	queries := []string{keyword}
	if season == 0 {
		return queries
	}
	for _, q := range []string{base + " S" + strconv.Itoa(season), fmt.Sprintf("%s S%02d", base, season), base} {
		found := false
		for _, previous := range queries {
			found = found || strings.EqualFold(previous, q)
		}
		if !found {
			queries = append(queries, q)
		}
	}
	return queries
}
