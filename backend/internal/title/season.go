package title

import (
	"regexp"
	"strings"
)

var explicitCNSeason = regexp.MustCompile(`^(?:第\s*[0-9零〇一二两三四五六七八九十]+\s*[季期]|[0-9]+\s*[季期])$`)
var separatedSeason = regexp.MustCompile(`(?i)\b(season|s)(?:[.-]|[ \t]+)([0-9]{1,2})\b`)

// Normalize explicit release markers before both evidence and episode parsing.
// Keep Unicode Roman numerals intact: folding a bare Ⅰ to I loses its evidence.
func normalizeSeasonMarkers(raw string) string {
	raw = strings.Map(func(r rune) rune {
		if (r >= '０' && r <= '９') || (r >= 'Ａ' && r <= 'Ｚ') || (r >= 'ａ' && r <= 'ｚ') {
			return r - 0xfee0
		}
		return r
	}, raw)
	raw = strings.ReplaceAll(raw, "_", " ")
	return separatedSeason.ReplaceAllString(raw, "${1}${2}")
}

// Unknown (0) and conflicting (-1) seasons must not inherit archiving's default
// Season 01. Part/Cour is deliberately distinct from season.
func ResourceSeason(title string) int {
	title = normalizeSeasonMarkers(title)
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

// ValidSeasonFilter accepts all seasons (0), unknown only (-1), or a season.
func ValidSeasonFilter(season int) bool { return season >= -1 && season <= 999 }

// AllowsSeason applies the subscription season rule to release evidence.
func AllowsSeason(title string, season int, allowUnknown bool) bool {
	if season == 0 {
		return true
	}
	actual := ResourceSeason(title)
	if season == -1 {
		return actual == 0
	}
	return actual == season || (actual == 0 && allowUnknown)
}
