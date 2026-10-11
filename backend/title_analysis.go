package main

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ---- 标题分析 ----
//
// 资源标题里混着字幕组、来源、编码、分辨率、字幕语言与季集标记：
//
//	[LoliHouse] 无职转生 3期 / Mushoku Tensei S3 - 10 [WebRip 1080p HEVC-10bit AAC][简繁内封字幕]
//
// 直接拿来当番剧名会把这些标签写进番剧库和归档目录，所以这里拆成识别与命名需要的字段：
// 干净的系列名（Title）、季（Season）、集（Episode）、字幕组（Group）、分辨率（Quality）。
// 季标记只用来确定季号，不再留在系列名里——季由归档时的 Season XX 目录与 NFO 承载。
// 系列名保留中文名：「中文名 / 原名」这种双语标题只取中文那段，番剧文件夹名更一致。

type animeInfo struct {
	Title   string
	Season  int
	Episode string
	Quality string
	Group   string
}

var (
	qualityPattern   = regexp.MustCompile(`(?i)(2160p|1080p|720p|480p|4k|8k)`)
	groupPattern     = regexp.MustCompile(`^\s*[\[【（(]([^\]】）)]+)[\]】）)]\s*`)
	separatorPattern = regexp.MustCompile(`\s+[-–—]\s*`)

	// tagPattern 与 parenPattern 分开：方括号基本都是标签，圆括号常出现在作品正式名里
	// （「进击的巨人（最终季）」），只有整段都是技术信息时才按标签删。
	tagPattern   = regexp.MustCompile(`[\[【][^\]】]*[\]】]`)
	parenPattern = regexp.MustCompile(`[（(][^）)]*[）)]`)
	// decorationPattern 是新番宣传装饰（★10月新番★ 之类），没有识别价值。
	decorationPattern = regexp.MustCompile(`[★☆♪◆■●▲▼✦]+|\d{1,2}\s*月\s*(?:新番|新番组)|新番`)
	// pureNumberTag 匹配 [01]、[01v2] 这类已经被取走的集数标签。
	pureNumberTag = regexp.MustCompile(`^[0-9]{1,4}(?:v[0-9]{1,2})?$`)
	// hashTag 匹配 [ABCD1234] 这类校验码（CRC32/MD5 等），没有识别价值。
	hashTag = regexp.MustCompile(`^[0-9a-f]{8}$|^[0-9a-f]{16,64}$`)
	// seasonEpisodeTag 匹配 S01、S01E09、E09 这类纯季集标签。
	seasonEpisodeTag = regexp.MustCompile(`^s[0-9]{1,2}(?:e[0-9]{1,3})?$|^e[0-9]{1,3}$`)
	seasonReleaseTag = regexp.MustCompile(`(?i)^s[0-9]{1,2}(?:\s+(?:reseed|fin|end|batch|complete))+$`)
	bitDepthPattern  = regexp.MustCompile(`(?i)\b(?:8|10|12|16)[ -]?bits?\b`)
	// tokenPattern 把标签内容拆成词，短代码（chs、nf、cr）只按整词匹配，避免误伤作品名。
	tokenPattern = regexp.MustCompile(`[^0-9a-zA-Z]+`)
	// bareTokenPattern 找出括号外的英文数字词，用来清理裸写的技术标签。
	bareTokenPattern = regexp.MustCompile(`[0-9A-Za-z][0-9A-Za-z._+\-]*`)
	// asciiRomanSuffix 是「Mushoku Tensei III」这类英文罗马数字季标记（带捕获组以便读出季号）。
	asciiRomanSuffix = regexp.MustCompile(`(?i)\b(ii|iii|iv|vi|vii|viii|ix)\b`)
	// ordinalSeasonPattern 匹配「3rd Season」这种英文序数季标记。
	ordinalSeasonPattern = regexp.MustCompile(`(?i)\b([0-9]{1,2})(?:st|nd|rd|th)\s*season\b`)
	// yearTokenPattern 匹配挡在集数前面的年份（「无职转生 2023 - 01」）：年份既不是集数，
	// 又会被合集区间模式吃掉，不先抹掉就解析不到真正的集数。只抹年份本身，分隔符与集数
	// 原样写回（Go 正则没有前瞻，所以连尾部一起捕获）。
	yearTokenPattern = regexp.MustCompile(`(?:^|[^0-9A-Za-z])((?:19|20)[0-9]{2})(\s*[-~～—–]\s*[0-9]{1,4})`)
	// romanRunePattern 匹配 Unicode 罗马数字季标记（無職転生Ⅲ）。
	romanRunePattern = regexp.MustCompile("[" + romanNumeralRunes + "]")
)

const romanNumeralRunes = "ⅠⅡⅢⅣⅤⅥⅦⅧⅨⅩⅪⅫ"

// 集数标记从最明确排到最含糊，先匹配先赢。第三季、S01 这类季标记不会满足集数条件，
// 1080、年份这类 4 位数也不当集数（方括号里的除外）。
var episodePatterns = []*regexp.Regexp{
	regexp.MustCompile(`第\s*([0-9]{1,4})\s*(?:话|集|話)`),
	regexp.MustCompile(`(?i)(?:^|[\s._\-\[])\s*s[0-9]{1,3}\s*e([0-9]{1,3})(?:[^0-9a-z]|$)`),
	regexp.MustCompile(`[\[【(]\s*([0-9]{1,4})(?:v[0-9]{1,2})?\s*[\]】)]`),
	regexp.MustCompile(`(?:^|[\s._-])([0-9]{1,4})(?:v[0-9]{1,2})?(?:\s*(?:话|集|話)|[^0-9a-z]|$)`),
}

// episodeRangePatterns 匹配「第01-12话」「[01-12合集]」「01~24」这类合集区间：它不是单集，
// 整段从系列名里去掉，也不写 Episode（合集资源留在番剧根目录，不冒充某一集）。
var episodeRangePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)第\s*[0-9]{1,4}\s*[-~～—–至]\s*[0-9]{1,4}\s*(?:话|集|話)?\s*(?:合集|全集|全|end)?`),
	regexp.MustCompile(`(?i)(?:^|[\s._\-\[【(])\s*[0-9]{1,4}\s*[-~～—–]\s*[0-9]{1,4}\s*(?:话|集|話)?\s*(?:合集|全集|全|end)?\s*[\]】)]?`),
}

// seasonPatterns 从最明确到最含糊；命中都要从系列名里去掉，只保留季号。
// 注意不含 SxxEyy：那是季集一体的标记，只用来读季号，整段留给集数解析。
var seasonPatterns = []*regexp.Regexp{
	regexp.MustCompile(`第\s*([0-9一二三四五六七八九十]{1,3})\s*[季期部]`),
	regexp.MustCompile(`(?i)season\s*([0-9]{1,2})(?:[^0-9]|$)`),
	ordinalSeasonPattern,
	// 独立的 S3 才算季标记；后面跟着 E01 的交给 episodeTagPattern。
	regexp.MustCompile(`(?i)(?:^|[\s._\-])s([0-9]{1,2})(?:[^0-9eE]|$)`),
	regexp.MustCompile(`([0-9]{1,2})\s*[季期]`),
}

// episodeTagPattern 匹配 S03E01 这类季集一体标记：只读季号，标记本身由集数解析消费。
var episodeTagPattern = regexp.MustCompile(`(?i)\bs([0-9]{1,2})\s*e[0-9]{1,3}\b`)

// resourceSubstrings 是出现即说明该标签是技术信息的词（够长、不会误伤作品名）。
var resourceSubstrings = []string{
	"webrip", "web-dl", "webdl", "bdrip", "bd-rip", "bdremux", "bluray", "blu-ray", "bdbox",
	"hevc", "x264", "x265", "h264", "h265", "10bit", "8bit", "hi10p", "ma10p", "yuv420p",
	"1080p", "720p", "2160p", "480p", "4k", "8k", "60fps", "120fps",
	"aac", "flac", "opus", "truehd", "eac3", "assx2", "srtx2", "jpsc", "jptc", "big5",
	"remux", "hdtv", "dvdrip", "tvrip", "vostfr", "netflix", "abema", "hidive", "iqiyi", "bilibili", "viutv", "webm",
	"baha", "cr ", " b-global", "bglobal", "unext", "amzn", "disney",
}

// resourceTokens 是只有整词出现才算技术标签的短代码。
var resourceTokens = map[string]bool{
	"chs": true, "cht": true, "sc": true, "tc": true, "gb": true, "cn": true, "jp": true,
	"cr": true, "nf": true, "hd": true, "fhd": true, "uhd": true, "tv": true, "web": true,
	"bd": true, "dvd": true, "mp4": true, "mkv": true, "avi": true, "mov": true,
	"ass": true, "srt": true, "sub": true, "subs": true, "font": true, "fonts": true,
	"avc": true, "aac": true, "dd": true, "ddp": true, "ddp5": true, "ac3": true, "dts": true, "av1": true, "flv": true, "webm": true,
}

// bareResourceTokens 是括号外整词出现才算技术标签的词：这些词够明确，嵌在作品名里的
// 可能性很低，删掉才不会再写进番剧文件夹名。含糊的短词（tv、web、hd）仍只认括号内。
var bareResourceTokens = map[string]bool{
	// 分辨率与编码
	"2160p": true, "1080p": true, "1080i": true, "720p": true, "480p": true, "4k": true, "8k": true,
	"1920x1080": true, "1280x720": true, "3840x2160": true,
	"hevc": true, "avc": true, "av1": true, "x264": true, "x265": true, "h264": true, "h265": true,
	"10bit": true, "8bit": true, "hi10p": true, "ma10p": true, "yuv420p": true,
	// 容器、音轨与字幕格式
	"mp4": true, "mkv": true, "avi": true, "mov": true, "webm": true, "flv": true,
	"aac": true, "flac": true, "opus": true, "ac3": true, "eac3": true, "dts": true, "truehd": true,
	"ass": true, "srt": true, "assx2": true, "srtx2": true,
	// 字幕语言代码
	"chs": true, "cht": true, "gb": true, "big5": true, "gb2312": true, "jpsc": true, "jptc": true,
	// 片源与平台
	"web-dl": true, "webdl": true, "webrip": true, "bdrip": true, "bd-rip": true, "bdremux": true,
	"bluray": true, "blu-ray": true, "remux": true, "hdtv": true, "dvdrip": true,
	"baha": true, "viutv": true, "bglobal": true, "b-global": true, "netflix": true, "amzn": true,
	"disney": true, "unext": true, "hidive": true, "abema": true, "iqiyi": true, "bilibili": true,
	// 版本与帧率
	"v2": true, "v3": true, "60fps": true, "120fps": true,
}

// bareResourcePhrases 是直接写在标题里的中文技术词（`摇曳百合 简体内嵌 1080P`）。只收多字
// 词：单字「简」「繁」等无法与作品名区分，仍只认括号内的。
var bareResourcePhrases = []string{
	"简体", "繁体", "简中", "繁中", "简日", "繁日", "简繁", "双语", "中字", "字幕",
	"内封", "内嵌", "外挂", "合集", "全集", "补档", "字幕组", "字幕組", "字幕社", "压制", "重制",
	// 技术词与汉字连写、又不会被其它词条接上的常见写法（`简体中文`、`中文字幕`）：
	// 两侧紧挨汉字的判断会把这些当成作品名，所以整词收进来。
	"简体中文", "繁体中文", "中文字幕", "日文字幕", "日语字幕", "中日双语", "简繁内封",
	"双语版", "重制版", "合集版",
}

// barePhrasePattern 把裸写中文技术词按最长优先拼成一个模式：连续写在一起的技术词
// （`简体内嵌`）会作为一整段匹配，边界判断才准确。
var barePhrasePattern = regexp.MustCompile(strings.Join(barePhraseAlternatives(), "|"))

// barePhraseAlternatives 长的排前面：正则的 | 是先命中先赢，长词先试才不会被短词
// （重制 / 重制版）切碎。
func barePhraseAlternatives() []string {
	out := append([]string(nil), bareResourcePhrases...)
	sort.SliceStable(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	for i, phrase := range out {
		out[i] = regexp.QuoteMeta(phrase)
	}
	return out
}

// cjkResourceMarkers 是中文技术标签的判定词（按子串匹配）。
var cjkResourceMarkers = []string{
	"简", "繁", "内封", "内嵌", "外挂", "字幕", "双语", "中日", "日中", "日语", "中文", "日文",
	"合集", "全集", "补档", "字幕组", "字幕組", "压制", "重制",
}

// extractAnimeInfo 分析资源标题；见本文件顶部说明。
func extractAnimeInfo(raw string) animeInfo {
	info := analyzeTitle(raw, true)
	if strings.TrimSpace(info.Title) != "" {
		return info
	}
	// 第一个方括号其实是番剧名而不是字幕组（「[无职转生 第二季] 第01话」）时，
	// 吃掉它就会把名字也一起丢掉：不吃它重新解析一次。
	if retry := analyzeTitle(raw, false); strings.TrimSpace(retry.Title) != "" {
		return retry
	}
	// 名字被当成技术标签删掉了（「[我们的重制人生][01][1080p]」）：退回括号里的名字，
	// 并保留已经解析出的季、集与分辨率。
	info.Group = ""
	if name := firstBracketName(raw); name != "" {
		info.Title = name
		return info
	}
	// 标题整体都是标签时也要留下可识别的名字，绝不能是空串。
	info.Title = strings.TrimSpace(raw)
	return info
}

// analyzeTitle 是标题分析的主体：stripGroup 为 false 时不吃开头的方括号。
func analyzeTitle(raw string, stripGroup bool) animeInfo {
	info := animeInfo{Title: strings.TrimSpace(raw)}
	clean := strings.TrimSpace(raw)
	if stripGroup {
		if m := groupPattern.FindStringSubmatch(clean); len(m) > 1 {
			info.Group = strings.TrimSpace(m[1])
			clean = strings.TrimSpace(clean[len(m[0]):])
		}
	}
	if m := qualityPattern.FindString(clean); m != "" {
		info.Quality = strings.ToUpper(m)
	}
	info.Season, clean = takeSeason(clean)
	// Read evidence before cleaning bracket labels such as [S1 Reseed Fin].
	if season := resourceSeason(raw); season > 0 {
		info.Season = season
	} else if season < 0 {
		info.Season = 0
	}
	// 年份先当噪声去掉，否则「2023 - 01」会被合集区间匹配吃掉，集数就丢了。
	clean = maskYears(clean)
	// Bit depth is technical metadata, not an episode (e.g. a season batch
	// labelled 10-bit must not appear as episode 10).
	clean = bitDepthPattern.ReplaceAllString(clean, " ")
	// 合集区间先去掉：否则「[01-12合集]」会被当成第 12 集，还会把系列名啃掉一块。
	hasEpisodeRange := false
	for _, pattern := range episodeRangePatterns {
		hasEpisodeRange = hasEpisodeRange || pattern.MatchString(clean)
		clean = pattern.ReplaceAllString(clean, " ")
	}
	for _, pattern := range episodePatterns {
		if hasEpisodeRange {
			break
		}
		m := pattern.FindStringSubmatch(clean)
		if len(m) <= 1 || looksLikeYear(m[1]) {
			continue
		}
		info.Episode = m[1]
		clean = strings.Replace(clean, m[0], " ", 1)
		break
	}
	clean = stripResourceTags(clean)
	clean = stripParenTags(clean)
	clean = stripBareTokens(clean)
	clean = decorationPattern.ReplaceAllString(clean, " ")
	clean = separatorPattern.ReplaceAllString(clean, " ")
	info.Title = preferChineseSegment(cleanSeriesName(clean))
	return info
}

// firstBracketName 取开头方括号里的名字：只有含汉字才可能是番剧名，
// 纯技术标签（[1080p]、[01]、[ABCD1234]）返回空。
func firstBracketName(raw string) string {
	m := groupPattern.FindStringSubmatch(strings.TrimSpace(raw))
	if len(m) < 2 {
		return ""
	}
	inner := strings.TrimSpace(m[1])
	if inner == "" || !hasHan(inner) {
		return ""
	}
	return inner
}

// hasHan 判断字符串里有没有汉字；假名不算，日文原名也能用。
func hasHan(text string) bool {
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// looksLikeYear 让「One Piece - 1100」这类 4 位集数保留，又不把标题里的年份当集数。
func looksLikeYear(episode string) bool {
	if len(episode) != 4 {
		return false
	}
	n, err := strconv.Atoi(episode)
	return err == nil && n >= 1900 && n <= 2099
}

// maskYears 把挡在集数前面的年份抹成空格（「2023 - 01」里的 2023），分隔符与集数留原样：
// 年份不是集数，但会被合集区间模式匹配掉，导致真正的集数解析不到。
func maskYears(text string) string {
	matches := yearTokenPattern.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return text
	}
	var out strings.Builder
	last := 0
	for _, m := range matches {
		out.WriteString(text[last:m[2]])
		out.WriteString(strings.Repeat(" ", m[3]-m[2]))
		out.WriteString(text[m[3]:m[5]])
		last = m[5]
	}
	out.WriteString(text[last:])
	return out.String()
}

// takeSeason 取标题里的季号：S03E01 只读季号，其它季标记（3期、第三季、S3、3rd Season、II）
// 一并从字符串里去掉。
func takeSeason(raw string) (int, string) {
	season := 0
	clean := raw
	if m := episodeTagPattern.FindStringSubmatch(clean); len(m) > 1 {
		if n, ok := parseSeasonNumber(m[1]); ok && n >= 1 && n <= 999 {
			season = n
		}
	}
	for _, pattern := range seasonPatterns {
		m := pattern.FindStringSubmatch(clean)
		if len(m) > 1 && season == 0 {
			if n, ok := parseSeasonNumber(m[1]); ok && n >= 1 && n <= 999 {
				season = n
			}
		}
		clean = pattern.ReplaceAllString(clean, " ")
	}
	// 罗马数字季标记（Mushoku Tensei III）单独读：它没有阿拉伯数字可用。
	if m := asciiRomanSuffix.FindStringSubmatch(clean); len(m) > 1 {
		if n := romanNumber(m[1]); n > 0 && season == 0 {
			season = n
		}
	}
	clean = asciiRomanSuffix.ReplaceAllString(clean, " ")
	// Unicode 罗马数字季标记（無職転生Ⅲ）同样要读出季号：stripSeasonMarkers 会把它
	// 从官方名里剥掉，这里不读就会让该季的集数落进第 1 季。
	if m := romanRunePattern.FindString(clean); m != "" && season == 0 {
		if n := romanNumber(m); n > 0 {
			season = n
		}
	}
	clean = romanRunePattern.ReplaceAllString(clean, " ")
	return season, clean
}

// romanNumber 解析标题里作为季标记出现的罗马数字：ASCII 只认 ii–ix（单个 I、V、X
// 太容易是作品名的一部分），Unicode Ⅱ–Ⅻ 按字符表读，它们基本只会是季标记。
func romanNumber(text string) int {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "ii":
		return 2
	case "iii":
		return 3
	case "iv":
		return 4
	case "vi":
		return 6
	case "vii":
		return 7
	case "viii":
		return 8
	case "ix":
		return 9
	}
	// Unicode 罗马数字（Ⅱ–Ⅻ）只可能是季标记，按字符表读出序号。
	if runes := []rune(strings.TrimSpace(text)); len(runes) == 1 {
		if idx := strings.IndexRune(romanNumeralRunes, runes[0]); idx >= 0 {
			return utf8.RuneCountInString(romanNumeralRunes[:idx]) + 1
		}
	}
	return 0
}

// parseSeasonNumber 解析阿拉伯数字与「三」「十二」这类中文数字。
func parseSeasonNumber(text string) (int, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, false
	}
	if n, err := strconv.Atoi(text); err == nil {
		return n, true
	}
	total, section, seen := 0, 0, false
	for _, r := range text {
		switch {
		case r == '十':
			if section == 0 {
				section = 1
			}
			total += section * 10
			section, seen = 0, true
		case chineseDigit(r) > 0:
			section, seen = chineseDigit(r), true
		default:
			return 0, false
		}
	}
	if !seen {
		return 0, false
	}
	return total + section, true
}

func chineseDigit(r rune) int {
	switch r {
	case '一':
		return 1
	case '二', '两':
		return 2
	case '三':
		return 3
	case '四':
		return 4
	case '五':
		return 5
	case '六':
		return 6
	case '七':
		return 7
	case '八':
		return 8
	case '九':
		return 9
	}
	return 0
}

// stripResourceTags 丢掉技术标签，并把保留下来的方括号去掉（`[无职转生]` → `无职转生`）。
func stripResourceTags(text string) string {
	return tagPattern.ReplaceAllStringFunc(text, func(segment string) string {
		inner := tagInner(segment)
		if isResourceTag(inner) {
			return " "
		}
		return inner
	})
}

// tagInner 去掉标签两端的分隔符，方括号与圆括号都要能解包。
func tagInner(segment string) string {
	inner := strings.TrimSpace(segment)
	for _, open := range []string{"[", "【", "(", "（"} {
		inner = strings.TrimPrefix(inner, open)
	}
	for _, close := range []string{"]", "】", ")", "）"} {
		inner = strings.TrimSuffix(inner, close)
	}
	return strings.TrimSpace(inner)
}

// stripParenTags 只删整段都是技术信息的圆括号（`(BD 1920x1080 x265-10Bit FLAC)`、`(1080p)`），
// 作品名自带的圆括号（`（最终季）`）原样保留。
func stripParenTags(text string) string {
	return parenPattern.ReplaceAllStringFunc(text, func(segment string) string {
		if isResourceTag(tagInner(segment)) {
			return " "
		}
		return segment
	})
}

// stripBareTokens 删掉没被括号包起来的技术标签（`摇曳百合 GB 1080P MP4 简体内嵌`），
// 只认够明确的词，避免误伤作品名里的英文与中文。
func stripBareTokens(text string) string {
	text = bareTokenPattern.ReplaceAllStringFunc(text, func(token string) string {
		if isBareResourceToken(token) {
			return " "
		}
		return token
	})
	return stripBarePhrases(text)
}

// stripBarePhrases 删掉裸写的中文技术词，但两侧紧挨汉字时保留：连续写在一起的技术词
// （`简体内嵌`、`简日双语`）先合成一段再判断，所以「我们的重制人生」「字幕少女」这类
// 作品名不会被啃掉一块。代价是「无职转生简体内嵌」这种完全没有分隔符的写法不再清理。
func stripBarePhrases(text string) string {
	matches := barePhrasePattern.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return text
	}
	var out strings.Builder
	last := 0
	for start := 0; start < len(matches); {
		begin, end := matches[start][0], matches[start][1]
		// 相邻命中的技术词算一段（简体内嵌 = 简体 + 内嵌）。
		for start++; start < len(matches) && matches[start][0] == end; start++ {
			end = matches[start][1]
		}
		out.WriteString(text[last:begin])
		if hanBefore(text, begin) || hanAfter(text, end) {
			out.WriteString(text[begin:end])
		} else {
			out.WriteString(" ")
		}
		last = end
	}
	out.WriteString(text[last:])
	return out.String()
}

// hanBefore 判断下标之前的字符是不是汉字。
func hanBefore(text string, offset int) bool {
	if offset <= 0 {
		return false
	}
	r, _ := utf8.DecodeLastRuneInString(text[:offset])
	return unicode.Is(unicode.Han, r)
}

// hanAfter 判断下标处的字符是不是汉字。
func hanAfter(text string, offset int) bool {
	if offset >= len(text) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(text[offset:])
	return unicode.Is(unicode.Han, r)
}

// isBareResourceToken 判断括号外的一个词是不是技术标签：整词命中，或按 . _ + - 拆开后
// 每一段都命中（`x265-10bit`、`Ma10p_1080p`）；`Muv-Luv`、`K-On` 这类作品名不会被误删。
func isBareResourceToken(token string) bool {
	word := strings.ToLower(strings.Trim(token, "._+-"))
	if word == "" {
		return false
	}
	if bareResourceTokens[word] {
		return true
	}
	parts := strings.FieldsFunc(word, func(r rune) bool {
		return r == '.' || r == '_' || r == '+' || r == '-'
	})
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts {
		if !bareResourceTokens[part] {
			return false
		}
	}
	return true
}

func isResourceTag(inner string) bool {
	if seasonReleaseTag.MatchString(strings.TrimSpace(inner)) {
		return true
	}
	if inner == "" {
		return true
	}
	lower := strings.ToLower(inner)
	if pureNumberTag.MatchString(lower) || seasonEpisodeTag.MatchString(lower) || hashTag.MatchString(lower) {
		return true
	}
	for _, marker := range cjkResourceMarkers {
		if strings.Contains(inner, marker) {
			return true
		}
	}
	for _, word := range resourceSubstrings {
		if strings.Contains(lower, word) {
			return true
		}
	}
	for _, token := range tokenPattern.Split(lower, -1) {
		if token != "" && resourceTokens[token] {
			return true
		}
	}
	return false
}

// cleanSeriesName 收尾：合并空白、去掉首尾的分隔符。
func cleanSeriesName(text string) string {
	return strings.Trim(collapseSpaces(text), "- _·|/")
}

func collapseSpaces(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// preferChineseSegment 在「中文名 / 原名」这种双语标题里取中文那段：番剧名与文件夹名用
// 中文更一致，不同字幕组之间也更容易归并成同一部番。没有中文段时原样返回，不丢信息。
func preferChineseSegment(title string) string {
	segments := metaAliasSeparator.Split(title, -1)
	if len(segments) < 2 {
		return title
	}
	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		if isChineseName(segment) {
			return segment
		}
	}
	return title
}

// isChineseName 判断一段标题能不能当中文名用：含汉字、且不含日文假名。
// 「無職転生」这类没有假名的日文原名也会被视为可用名（总比把名字弄丢好）。
func isChineseName(text string) bool {
	hasHan := false
	for _, r := range text {
		switch {
		case unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r):
			return false
		case unicode.Is(unicode.Han, r):
			hasHan = true
		}
	}
	return hasHan
}

// ---- 规范番剧名 ----

// standardSeriesName 给媒体库用的规范番剧名：优先 Bangumi 中文名，其次日文原名，
// 都会剥掉季标记（季由 Season 目录与 NFO 承载）。两者都不可用时返回空字符串。
func standardSeriesName(nameCN, name string) string {
	for _, candidate := range []string{nameCN, name} {
		if standard := stripSeasonMarkers(candidate); standard != "" {
			return standard
		}
	}
	return ""
}

// stripSeasonMarkers 去掉名字里的季标记（第三季、3期、Season 3、Ⅲ、III）。
// 整个名字就是季标记时保留原名，避免得到空名。
func stripSeasonMarkers(name string) string {
	original := collapseSpaces(name)
	if original == "" {
		return ""
	}
	cleaned := original
	for _, pattern := range seasonPatterns {
		cleaned = pattern.ReplaceAllString(cleaned, " ")
	}
	cleaned = strings.Map(func(r rune) rune {
		if strings.ContainsRune(romanNumeralRunes, r) {
			return ' '
		}
		return r
	}, cleaned)
	cleaned = asciiRomanSuffix.ReplaceAllString(cleaned, " ")
	cleaned = cleanSeriesName(cleaned)
	if cleaned == "" {
		return original
	}
	return cleaned
}
