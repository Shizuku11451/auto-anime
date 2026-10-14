package title

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// MaxMetaQueries bounds metadata search expansion.
const MaxMetaQueries = 6

var (
	metaBrackets          = regexp.MustCompile(`[\[\]【】()（）]`)
	metaAliasSeparator    = regexp.MustCompile(`\s+[/／|]\s+`)
	metaReleaseTag        = regexp.MustCompile(`(?i)^(?:[0-9]{1,2}月(?:新番|番)?|[0-9]{4}(?:年[0-9]{1,2}月新番)?|[0-9]{1,3}(?:v[0-9]+)?(?:[-~][0-9]{1,3})?(?:话|話|集|end)?|[a-f0-9]{8})$`)
	metaTechnicalTag      = regexp.MustCompile(`(?i)(?:[0-9]{3,4}[pi]|[248]k|web[- ._]?dl|webrip|bdrip|bluray|blu-ray|hdtv|[xh][ .]?26[45]|hevc|avc|av1|[0-9]{1,2}bit|aac|flac|ac3|eac3|opus|mp4|mkv|webm|gb|big5|chs|cht|jpn|jp|[简簡繁体體中日英双雙语語内內嵌封外挂掛字幕文]+)`)
	metaInlineTechnical   = regexp.MustCompile(`(?i)(?:^|[\s·_])(?:[0-9]{3,4}[pi]|[248]k|web[- ._]?dl|webrip|bdrip|[xh]26[45]|hevc|avc|aac|flac|mp4|mkv|[0-9]{1,2}bit)\b`)
	metaCNSeason          = regexp.MustCompile(`(?:第\s*)?([0-9零〇一二两三四五六七八九十]{1,3})\s*[季期]`)
	metaENSeason          = regexp.MustCompile(`(?i)\b(?:season\s*|s)([0-9]{1,2})\b`)
	metaOrdinalSeason     = regexp.MustCompile(`(?i)\b([0-9]{1,2}(?:st|nd|rd|th)|first|second|third|fourth|fifth|sixth|seventh|eighth|ninth|tenth)\s+season\b`)
	metaRomanSeason       = regexp.MustCompile(`(?:^|[^a-zA-Z0-9])([IVX]{1,5})(?:$|[^a-zA-Z0-9])`)
	metaCNPart            = regexp.MustCompile(`(?:第\s*)?([0-9零〇一二两三四五六七八九十]{1,3})\s*(?:部分|クール)`)
	metaENPart            = regexp.MustCompile(`(?i)\b(?:part|cour)\s*([0-9]{1,2})\b`)
	metaSubtitleSeparator = regexp.MustCompile(`[~～:：]`)
	metaRomanNumbers      = strings.NewReplacer("Ⅰ", "I", "Ⅱ", "II", "Ⅲ", "III", "Ⅳ", "IV", "Ⅴ", "V", "Ⅵ", "VI", "Ⅶ", "VII", "Ⅷ", "VIII", "Ⅸ", "IX", "Ⅹ", "X")
)

// Only known release metadata is discarded. A bracket can contain the entire
// series name, an alias, or a subtitle, rather than just a technical tag.
func metaNoiseTag(s string) bool {
	s = strings.TrimSpace(s)
	lower := strings.ToLower(s)
	if s == "" || metaReleaseTag.MatchString(s) {
		return true
	}
	for _, marker := range []string{"字幕组", "字幕組", "字幕社", "-raws", "-subs"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	remainder := metaTechnicalTag.ReplaceAllString(s, "")
	return strings.Trim(remainder, " \t\r\n-_.+&/·") == ""
}

func metaScript(s string) int {
	script := 0
	for _, r := range s {
		switch {
		case unicode.In(r, unicode.Hiragana, unicode.Katakana):
			return 3
		case unicode.Is(unicode.Han, r):
			script = 2
		case unicode.IsLetter(r) && script == 0:
			script = 1
		}
	}
	return script
}

func QueryTitles(title string) []string {
	// Bound processing of untrusted RSS names, including malformed brackets.
	runes := []rune(title)
	if len(runes) > 512 {
		title = string(runes[:512])
	}
	var out []string
	inside, plainTitle := false, false
	add := func(text string, bracketed bool) {
		for i, part := range metaAliasSeparator.Split(text, -1) {
			part = strings.Join(strings.Fields(part), " ")
			if part == "" || (bracketed && metaNoiseTag(part)) {
				continue
			}
			part = strings.TrimSpace(metaInlineTechnical.ReplaceAllString(part, " "))
			if normalizeMetaText(part) == "" {
				continue
			}
			base, season, split := TitleParts(part)
			marker := (base == "" && (season > 0 || split > 0)) ||
				(metaNumber(part) > 0 && strings.Trim(part, "IVX") == "")
			if len(out) > 0 && i == 0 && (marker ||
				((!bracketed || plainTitle) && metaScript(part) == metaScript(out[len(out)-1]))) {
				out[len(out)-1] += " " + part
			} else if !marker {
				out = append(out, part)
				plainTitle = !bracketed
			}
		}
	}
	last := 0
	for _, loc := range metaBrackets.FindAllStringIndex(title, -1) {
		add(title[last:loc[0]], inside)
		inside = strings.Contains("[【(（", title[loc[0]:loc[1]])
		last = loc[1]
	}
	add(title[last:], inside)
	seen := map[string]bool{}
	unique := out[:0]
	for _, q := range out {
		key := normalizeMetaText(q)
		if !seen[key] && len([]rune(key)) >= 2 {
			seen[key] = true
			unique = append(unique, q)
		}
	}
	return unique
}

// An explicit season/part correction in the configured search name wins over
// a release label; a shortened search name still inherits the original hints.
func LookupAliases(title, searchName string) []string {
	original := QueryTitles(title)
	custom := QueryTitles(searchName)
	season, part := 0, 0
	for _, alias := range custom {
		_, s, p := TitleParts(alias)
		season, part = mergeMetaNumber(season, s), mergeMetaNumber(part, p)
	}
	aliases := append([]string(nil), custom...)
	for _, alias := range original {
		_, s, p := TitleParts(alias)
		if (season > 0 && s > 0 && season != s) || (part > 0 && p > 0 && part != p) {
			continue
		}
		aliases = append(aliases, alias)
	}
	return aliases
}

// Try intact aliases first, then shorter names. Ranking always uses the intact
// aliases and their season/part markers, never the broadened search keyword.
func SearchQueries(aliases []string) []string {
	var queries []string
	seen := map[string]bool{}
	add := func(q string) {
		q = strings.TrimSpace(q)
		key := normalizeMetaText(q)
		if len(queries) < MaxMetaQueries && len([]rune(key)) >= 2 && !seen[key] {
			seen[key] = true
			queries = append(queries, q)
		}
	}
	for _, alias := range aliases {
		add(alias)
	}
	for _, alias := range aliases {
		add(metaSubtitleSeparator.Split(alias, 2)[0])
	}
	for _, alias := range aliases {
		head := metaSubtitleSeparator.Split(alias, 2)[0]
		add(stripMetaMarkers(head))
	}
	return queries
}

func foldMetaSymbols(s string) string {
	s = metaRomanNumbers.Replace(s)
	return strings.Map(func(r rune) rune {
		if r >= '！' && r <= '～' {
			return r - 0xfee0
		}
		return r
	}, s)
}

func normalizeMetaText(s string) string {
	var b strings.Builder
	count := 0
	for _, r := range strings.ToLower(foldMetaSymbols(s)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			count++
			if count == 512 {
				break
			}
		}
	}
	return b.String()
}

func metaNumber(s string) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	if n := map[string]int{"I": 1, "II": 2, "III": 3, "IV": 4, "V": 5, "VI": 6, "VII": 7, "VIII": 8, "IX": 9, "X": 10,
		"first": 1, "second": 2, "third": 3, "fourth": 4, "fifth": 5, "sixth": 6, "seventh": 7, "eighth": 8, "ninth": 9, "tenth": 10}[s]; n > 0 {
		return n
	}
	for _, suffix := range []string{"st", "nd", "rd", "th"} {
		if strings.HasSuffix(s, suffix) {
			n, _ := strconv.Atoi(strings.TrimSuffix(s, suffix))
			return n
		}
	}
	n, tens := 0, 0
	for _, r := range s {
		if r == '十' {
			tens, n = max(n, 1)*10, 0
			continue
		}
		v, ok := map[rune]int{'零': 0, '〇': 0, '一': 1, '二': 2, '两': 2, '三': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9}[r]
		if !ok {
			return 0
		}
		n = n*10 + v
	}
	return tens + n
}

func mergeMetaNumber(a, b int) int {
	if a == 0 {
		return b
	}
	if b != 0 && a != b {
		return -1 // conflicting aliases should not silently pick a season
	}
	return a
}

func parseMetaMarkers(s string) (string, int, int) {
	s = foldMetaSymbols(s)
	season, part := 0, 0
	for _, spec := range []struct {
		pattern *regexp.Regexp
		target  *int
	}{
		{metaCNSeason, &season}, {metaENSeason, &season}, {metaOrdinalSeason, &season},
		{metaCNPart, &part}, {metaENPart, &part},
	} {
		s = spec.pattern.ReplaceAllStringFunc(s, func(match string) string {
			m := spec.pattern.FindStringSubmatch(match)
			*spec.target = mergeMetaNumber(*spec.target, metaNumber(strings.ToLower(m[1])))
			return " "
		})
	}
	// A Roman season suffix can precede a subtitle, but a word inside a title
	// (SPY X FAMILY, That Time I Got Reincarnated...) is not a season marker.
	for _, loc := range reverseMetaMatches(metaRomanSeason.FindAllStringSubmatchIndex(s, -1)) {
		if loc[2] == 0 || len([]rune(normalizeMetaText(s[:loc[2]]))) < 2 {
			continue
		}
		tail := strings.TrimLeft(s[loc[3]:], " \t\r\n]】)）")
		if tail != "" && !strings.ContainsRune("~～:：", []rune(tail)[0]) {
			continue
		}
		if n := metaNumber(s[loc[2]:loc[3]]); n > 0 {
			season = mergeMetaNumber(season, n)
			s = s[:loc[2]] + " " + s[loc[3]:]
		}
	}
	return s, season, part
}

func reverseMetaMatches(matches [][]int) [][]int {
	for i, j := 0, len(matches)-1; i < j; i, j = i+1, j-1 {
		matches[i], matches[j] = matches[j], matches[i]
	}
	return matches
}

func stripMetaMarkers(s string) string {
	s, _, _ = parseMetaMarkers(s)
	return strings.Join(strings.Fields(s), " ")
}

// TitleParts returns a normalized name and its season and part evidence.
func TitleParts(s string) (string, int, int) {
	s = metaInlineTechnical.ReplaceAllString(s, " ")
	s, season, part := parseMetaMarkers(s)
	return normalizeMetaText(s), season, part
}

// AliasesScore ranks names only when their season and part evidence agree.
func AliasesScore(aliases []string, name, nameCN string) int {
	qSeason, qPart, cSeason, cPart := 0, 0, 0, 0
	var queries, candidates []string
	for _, alias := range aliases {
		base, season, part := TitleParts(alias)
		queries = append(queries, base)
		qSeason, qPart = mergeMetaNumber(qSeason, season), mergeMetaNumber(qPart, part)
	}
	for _, alias := range []string{nameCN, name} {
		base, season, part := TitleParts(alias)
		candidates = append(candidates, base)
		cSeason, cPart = mergeMetaNumber(cSeason, season), mergeMetaNumber(cPart, part)
	}
	if qSeason < 0 || qPart < 0 || cSeason < 0 || cPart < 0 ||
		max(qSeason, 1) != max(cSeason, 1) || max(qPart, 1) != max(cPart, 1) {
		return 0
	}
	best := 0
	for _, q := range queries {
		for _, c := range candidates {
			ql, cl := len([]rune(q)), len([]rune(c))
			if min(ql, cl) < 2 {
				continue
			}
			if q == c {
				best = max(best, 1000)
				continue
			}
			short, long := q, c
			if ql > cl {
				short, long = c, q
			}
			contained := strings.Contains(long, short)
			prefix := strings.HasPrefix(long, short) && (metaScript(short) >= 2 || len(short) >= 8)
			if contained && min(ql, cl) >= 4 && (prefix || min(ql, cl)*2 >= max(ql, cl)) {
				best = max(best, 800+min(min(ql, cl), 99))
				continue
			}
			// Compare the entire name, not just a shared genre phrase.
			limit := max(1, max(ql, cl)/8)
			if min(ql, cl) >= 6 && max(ql, cl)-min(ql, cl) <= limit {
				if distance := metaEditDistance(q, c); distance <= limit {
					best = max(best, 600-distance*100/max(ql, cl))
				}
			}
		}
	}
	return best
}

func metaEditDistance(a, b string) int {
	ar, br := []rune(a), []rune(b)
	row := make([]int, len(br)+1)
	for j := range row {
		row[j] = j
	}
	for i, r := range ar {
		previous := row[0]
		row[0] = i + 1
		for j, other := range br {
			cost := 1
			if r == other {
				cost = 0
			}
			old := row[j+1]
			row[j+1] = min(row[j]+1, row[j+1]+1, previous+cost)
			previous = old
		}
	}
	return row[len(br)]
}
