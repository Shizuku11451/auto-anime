package title

import "testing"

// 标题分析：系列名只留作品名，季与集分开，技术标签不进名字。
func TestAnalyzeTitleSplitsNameSeasonEpisode(t *testing.T) {
	cases := []struct {
		raw     string
		title   string
		season  int
		episode string
		group   string
		quality string
	}{
		{raw: "[Group] Example Ｓ０２Ｅ０３ [1080p]", title: "Example", season: 2, episode: "03", group: "Group", quality: "1080P"},
		{raw: "[Group] Example Season-02 [03][1080p]", title: "Example", season: 2, episode: "03", group: "Group", quality: "1080P"},
		{raw: "[Group] Example S 02 [03][1080p]", title: "Example", season: 2, episode: "03", group: "Group", quality: "1080P"},
		{raw: "[Group] Example Second Season [03][1080p]", title: "Example", season: 2, episode: "03", group: "Group", quality: "1080P"},
		{raw: "[Group] Example 3rdSeason [03][1080p]", title: "Example", season: 3, episode: "03", group: "Group", quality: "1080P"},
		{raw: "[Group_Name] Example_S02_03_1080p", title: "Example", season: 2, episode: "03", group: "Group_Name", quality: "1080P"},
		{
			raw:     "[LoliHouse] 无职转生 3期 / Mushoku Tensei S3 - 10 [WebRip 1080p HEVC-10bit AAC][简繁内封字幕]",
			title:   "无职转生",
			season:  3,
			episode: "10",
			group:   "LoliHouse",
			quality: "1080P",
		},
		{
			raw:     "【喵萌奶茶屋】★10月新番★[无职转生 第三季][01][1080p][简日双语]",
			title:   "无职转生",
			season:  3,
			episode: "01",
			group:   "喵萌奶茶屋",
			quality: "1080P",
		},
		{
			// SxxEyy 只读季号，集数不能被季解析吃掉。
			raw:     "[Nix-Raws] 与你相恋到生命尽头 / Kimi ga Shinu made Koi wo Shitai S01E09 [CR WEB-DL 1080p AVC AAC][简繁内封]",
			title:   "与你相恋到生命尽头",
			season:  1,
			episode: "09",
			group:   "Nix-Raws",
			quality: "1080P",
		},
		{
			raw:     "[桜都字幕组] 无职转生 第三季 ～到了异世界就拿出真本事～ / Mushoku Tensei III [10][1080P][简体内嵌]",
			title:   "无职转生 ～到了异世界就拿出真本事～",
			season:  3,
			episode: "10",
			group:   "桜都字幕组",
			quality: "1080P",
		},
		{
			raw:     "Show Season 2 - 05 [BDRip][1080p]",
			title:   "Show",
			season:  2,
			episode: "05",
			quality: "1080P",
		},
		{
			raw:     "[字幕组] 夏日番外篇 第08话 [720p]",
			title:   "夏日番外篇",
			episode: "08",
			group:   "字幕组",
			quality: "720P",
		},
		{
			// 番剧名写在第一个方括号里：不吃它，靠重建解析拿回名字。
			raw:     "[咒术回战][01][1080p]",
			title:   "咒术回战",
			episode: "01",
			quality: "1080P",
		},
		{
			raw:     "[无职转生] 第01话",
			title:   "无职转生",
			episode: "01",
		},
		{
			// 作品名里含技术词（重制）：两侧都是汉字时不当标签删。
			raw:     "[LoliHouse] 我们的重制人生 - 01 [1080p][简体内嵌]",
			title:   "我们的重制人生",
			episode: "01",
			group:   "LoliHouse",
			quality: "1080P",
		},
		{
			// Unicode 罗马数字季标记也要读出季号，并从名字里去掉。
			raw:     "無職転生Ⅱ 第01话",
			title:   "無職転生",
			season:  2,
			episode: "01",
		},
		{
			// 括号里的名字含技术词、被后续标签吃空：退回括号里的名字。
			raw:     "[我们的重制人生][01][1080p]",
			title:   "我们的重制人生",
			episode: "01",
			quality: "1080P",
		},
		{
			// 括号里没有汉字（[1080p][x265]）时不认成名字，用原始标题兜底。
			raw:   "[1080p][x265]",
			title: "[1080p][x265]",
		},
		{
			// 标题整体都是标签时也要留下非空系列名。
			raw:     "[组] [1080p][x265][10bit][AAC]",
			title:   "组",
			quality: "1080P",
		},
	}
	for _, c := range cases {
		info := Extract(c.raw)
		if info.Title != c.title || info.Season != c.season || info.Episode != c.episode || info.Group != c.group || info.Quality != c.quality {
			t.Errorf("analyze %q:\n got  %+v\n want title=%q season=%d episode=%q group=%q quality=%q",
				c.raw, info, c.title, c.season, c.episode, c.group, c.quality)
		}
	}
}

func TestParseSeasonNumber(t *testing.T) {
	cases := []struct {
		text string
		want int
		ok   bool
	}{
		{"3", 3, true},
		{"12", 12, true},
		{"三", 3, true},
		{"十", 10, true},
		{"十一", 11, true},
		{"二十三", 23, true},
		{"", 0, false},
		{"x", 0, false},
	}
	for _, c := range cases {
		got, ok := ParseSeasonNumber(c.text)
		if got != c.want || ok != c.ok {
			t.Errorf("ParseSeasonNumber(%q) = %d,%v want %d,%v", c.text, got, ok, c.want, c.ok)
		}
	}
}

// 规范番剧名取 Bangumi 中文名（无则日文原名），并剥掉季标记。
func TestStandardSeriesNameStripsSeasonMarkers(t *testing.T) {
	cases := []struct{ nameCN, name, want string }{
		{"无职转生 第三季 ～到了异世界就拿出真本事～", "無職転生Ⅲ ～異世界行ったら本気だす～", "无职转生 ～到了异世界就拿出真本事～"},
		{"", "無職転生Ⅲ ～異世界行ったら本気だす～", "無職転生 ～異世界行ったら本気だす～"},
		{"我推的孩子 第二季", "", "我推的孩子"},
		{"葬送的芙莉莲", "Sousou no Frieren", "葬送的芙莉莲"},
		{"Frieren Season 2", "", "Frieren"},
		{"", "Mushoku Tensei III", "Mushoku Tensei"},
		// 整个名字就是季标记时保留原名，不能规范成空名。
		{"第三季", "", "第三季"},
		{"", "", ""},
	}
	for _, c := range cases {
		if got := StandardSeriesName(c.nameCN, c.name); got != c.want {
			t.Errorf("StandardSeriesName(%q, %q) = %q, want %q", c.nameCN, c.name, got, c.want)
		}
	}
}

// 技术标签清理：括号内外混着的来源、编码、容器、校验码都不能进番剧名，
// 而作品名自带的圆括号、连字符、英文缩写要留住。
func TestAnalyzeTitleStripsNoiseTags(t *testing.T) {
	cases := []struct {
		raw     string
		title   string
		episode string
		quality string
	}{
		{"[SubsPlease] Sousou no Frieren - 28 (1080p) [ABCD1234]", "Sousou no Frieren", "28", "1080P"},
		{"[Moozzi2] 呪術廻戦 - 01 (BD 1920x1080 x265-10Bit FLAC)", "呪術廻戦", "01", ""},
		{"【极影字幕社】★7月新番 摇曳百合 第4季 第01话 GB 1080P MP4", "摇曳百合", "01", "1080P"},
		{"[Skymoon-Raws] 我推的孩子 第二季 / Oshi no Ko S2 - 01 [ViuTV][WEB-DL][1080p][AVC AAC][CHT][MP4]", "我推的孩子", "01", "1080P"},
		{"[ANi] 进击的巨人（最终季） - 01 [1080p]", "进击的巨人（最终季）", "01", "1080P"},
		{"[Group] 进击的巨人（AAC 1080p） - 01", "进击的巨人", "01", "1080P"},
		{"[Muv-Luv] Muv-Luv Alternative [01][1080p]", "Muv-Luv Alternative", "01", "1080P"},
		{"[K-On!] K-ON!! [01][1080p]", "K-ON!!", "01", "1080P"},
		{"[Group] Fate/stay night [01][1080p][BDRip]", "Fate/stay night", "01", "1080P"},
		{"[Group] 剧场版 鬼灭之刃 无限列车篇 [1080p]", "剧场版 鬼灭之刃 无限列车篇", "", "1080P"},
	}
	for _, c := range cases {
		info := Extract(c.raw)
		if info.Title != c.title || info.Episode != c.episode || info.Quality != c.quality {
			t.Errorf("analyze %q:\n got  %+v\n want title=%q episode=%q quality=%q", c.raw, info, c.title, c.episode, c.quality)
		}
	}
}

// 合集区间不是单集：不能冒充某一集写进 Season 目录，也不能把系列名啃掉一块。
func TestAnalyzeTitleIgnoresEpisodeRange(t *testing.T) {
	cases := []struct {
		raw     string
		title   string
		episode string
	}{
		{"【FSD】Lycoris Recoil / 莉可丽丝 [01-12合集][1080p]", "莉可丽丝", ""},
		{"[ANi] 咒术回战 - 01~24 [1080P]", "咒术回战", ""},
		{"[Group] 我推的孩子 第01-12话 合集 [1080p]", "我推的孩子", ""},
		{"[Group] 我推的孩子 [12][1080p]", "我推的孩子", "12"},
	}
	for _, c := range cases {
		info := Extract(c.raw)
		if info.Title != c.title || info.Episode != c.episode {
			t.Errorf("analyze %q:\n got  %+v\n want title=%q episode=%q", c.raw, info, c.title, c.episode)
		}
	}
}

// 季号变体与 4 位集数：英文序数、罗马数字都读成季，四位集数不因长度被漏掉。
func TestAnalyzeTitleReadsSeasonAndLongEpisode(t *testing.T) {
	cases := []struct {
		raw     string
		title   string
		season  int
		episode string
	}{
		{"[sakurato.sub] Tensei shitara Slime Datta Ken 3rd Season - 01 [1080P]", "Tensei shitara Slime Datta Ken", 3, "01"},
		{"[VCB-Studio] 无职转生 II [01][1080p]", "无职转生", 2, "01"},
		{"[DMG][One Piece][1100][1080P][HEVC]", "One Piece", 0, "1100"},
		{"[Group] 某番 [1][1080p]", "某番", 0, "1"},
		// 4 位数像年份时不当集数（保守取舍：宁可不识别，也不把年份写成集号）。
		{"[Group] One Piece - 1999 [1080p]", "One Piece 1999", 0, ""},
		// 年份挡在集数前面时先抹掉年份，集数才识别得到（原来会被合集区间整段吃掉）。
		{"[Group] 无职转生 2023 - 01 [1080p]", "无职转生", 0, "01"},
	}
	for _, c := range cases {
		info := Extract(c.raw)
		if info.Title != c.title || info.Season != c.season || info.Episode != c.episode {
			t.Errorf("analyze %q:\n got  %+v\n want title=%q season=%d episode=%q", c.raw, info, c.title, c.season, c.episode)
		}
	}
}

// 双语标题取中文那段做番剧名，跨字幕组更容易归并成同一部番。
func TestAnalyzeTitlePrefersChineseName(t *testing.T) {
	cases := []struct{ raw, title string }{
		{"[北宇治字幕组] 药屋少女的呢喃 / Kusuriya no Hitorigoto [13][1080p]", "药屋少女的呢喃"},
		{"[LoliHouse] 孤独摇滚！ / Bocchi the Rock! - 01 [1080p]", "孤独摇滚！"},
		// 日文原名带假名，不当中文名。
		{"[Group] 葬送的芙莉莲 / 葬送のフリーレン [01][1080p]", "葬送的芙莉莲"},
		// 两段都不是中文时保留原样，不丢名字。
		{"[Group] Sousou no Frieren / Frieren: Beyond Journey's End [01][1080p]", "Sousou no Frieren / Frieren: Beyond Journey's End"},
		// 只有一段（或斜杠不带空格）时不拆。
		{"[Group] 呪術廻戦 [01][1080p]", "呪術廻戦"},
		{"[Group] Fate/stay night [01][1080p]", "Fate/stay night"},
	}
	for _, c := range cases {
		if info := Extract(c.raw); info.Title != c.title {
			t.Errorf("analyze %q: title = %q, want %q", c.raw, info.Title, c.title)
		}
	}
}
