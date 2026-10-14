package filter

import (
	"strings"
	"unicode"

	"github.com/longbridgeapp/opencc"
)

// Dictionaries are embedded in the Go dependency; runtime matching is offline.
var keywordConverter = func() *opencc.OpenCC {
	converter, err := opencc.New("t2s")
	if err != nil {
		panic("初始化简繁关键词匹配失败：" + err.Error())
	}
	return converter
}()

func normalizeKeywordText(text string) string {
	text = strings.ToLower(text)
	if strings.IndexFunc(text, func(r rune) bool { return unicode.Is(unicode.Han, r) }) < 0 {
		return text
	}
	converted, err := keywordConverter.Convert(text)
	if err != nil {
		panic("简繁关键词匹配失败：" + err.Error())
	}
	return converted
}

// Filter holds immutable include and exclude terms for repeated matching.
type Filter struct {
	include, exclude []keywordTerm
}

type keywordTerm struct {
	raw, normalized string
}

// New parses comma-separated rules without changing their original meaning.
func New(include, exclude string) Filter {
	words := func(raw string) []keywordTerm {
		var result []keywordTerm
		for _, word := range strings.Split(raw, ",") {
			if word = strings.TrimSpace(word); word != "" {
				result = append(result, keywordTerm{raw: strings.ToLower(word), normalized: normalizeKeywordText(word)})
			}
		}
		return result
	}
	return Filter{include: words(include), exclude: words(exclude)}
}

// Allows requires every include term and rejects any matching exclude term.
func (filter Filter) Allows(title string) bool {
	if len(filter.include) == 0 && len(filter.exclude) == 0 {
		return true
	}
	raw := strings.ToLower(title)
	title = normalizeKeywordText(raw)
	// Phrase conversions can depend on surrounding characters. Preserve exact
	// substring matches as well so normalization never removes a prior match.
	matches := func(word keywordTerm) bool {
		return strings.Contains(raw, word.raw) || strings.Contains(title, word.normalized)
	}
	for _, word := range filter.include {
		if !matches(word) {
			return false
		}
	}
	for _, word := range filter.exclude {
		if matches(word) {
			return false
		}
	}
	return true
}

// Allowed matches one title against comma-separated rules.
func Allowed(title, include, exclude string) bool {
	return New(include, exclude).Allows(title)
}
