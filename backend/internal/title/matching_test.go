package title

import "testing"

func TestMetaQueryTitles(t *testing.T) {
	cases := []struct {
		title string
		want  []string
	}{
		{"与你相恋到生命尽头 / Kimi ga Shinu made Koi wo Shitai [WebRip][1080p][简日内嵌]", []string{"与你相恋到生命尽头", "Kimi ga Shinu made Koi wo Shitai"}},
		{"夏日番外篇 [1080p]", []string{"夏日番外篇"}},
		{"无职转生 第三季 ～到了异世界就拿出真本事～ / Mushoku Tensei III [10][1080P]", []string{"无职转生 第三季 ～到了异世界就拿出真本事～", "Mushoku Tensei III"}},
		{"", nil},
	}
	for _, c := range cases {
		got := QueryTitles(c.title)
		if len(got) != len(c.want) {
			t.Fatalf("queries of %q: got %#v want %#v", c.title, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("queries of %q: got %#v want %#v", c.title, got, c.want)
			}
		}
	}
}

func TestMetaAccept(t *testing.T) {
	if AliasesScore(QueryTitles("葬送的芙莉莲"), "葬送のフリーレン", "葬送的芙莉莲") == 0 {
		t.Fatal("exact name_cn should match")
	}
	if AliasesScore(QueryTitles("无职转生 第二季"), "Mushoku Tensei II", "无职转生 第二季") == 0 {
		t.Fatal("season suffix should still match")
	}
	if AliasesScore(QueryTitles("葬送的芙莉莲"), "夏日动画", "夏日动画") > 0 {
		t.Fatal("unrelated subject must not match")
	}
	if AliasesScore(QueryTitles("与"), "与", "与") > 0 {
		t.Fatal("single-character queries must not match")
	}
	if AliasesScore(QueryTitles("  "), "テスト", "测试") > 0 {
		t.Fatal("empty query must not match")
	}
	if AliasesScore(QueryTitles("葬送的芙莉莲 S01 · 1080P"), "葬送のフリーレン", "葬送的芙莉莲") == 0 {
		t.Fatal("season and quality noise should be ignored")
	}
	if AliasesScore(QueryTitles("Kimi-ga-Shinu-made-Koi-wo-Shitai"), "Kimi ga Shinu made Koi wo Shitai", "") == 0 {
		t.Fatal("punctuation differences should match")
	}
}

func TestMetaMatchingRespectsSeasonAndIdentity(t *testing.T) {
	cases := []struct {
		query, name, nameCN string
		want                bool
	}{
		{"无职转生 第三季", "無職転生Ⅲ ～異世界行ったら本気だす～", "无职转生 第三季 ～到了异世界就拿出真本事～", true},
		{"无职转生 第三季", "無職転生Ⅱ ～異世界行ったら本気だす～", "无职转生 第二季 ～到了异世界就拿出真本事～", false},
		{"无职转生 第三季", "無職転生 ～異世界行ったら本気だす～", "无职转生～到了异世界就拿出真本事～", false},
		{"Mushoku Tensei III", "Mushoku Tensei Ⅲ", "", true},
		{"Mushoku Tensei III", "Mushoku Tensei II", "", false},
		{"无职转生 第二季 第2部分", "無職転生Ⅱ ～異世界行ったら本気だす～", "无职转生 第二季 ～到了异世界就拿出真本事～", false},
		{"无职转生 第二季 第2部分", "無職転生Ⅱ ～異世界行ったら本気だす～ 第2クール", "无职转生 第二季 ～到了异世界就拿出真本事～ 第2部分", true},
		{"夏日动画", "夏日动画 第二季", "", false},
		{"魔法少女小圆", "魔法少女育成计划", "", false},
		{"Fate/stay night", "Fate/Grand Order", "", false},
		{"葬送的芙莉莲 S01 · 1080P", "葬送のフリーレン", "葬送的芙莉莲", true},
		{"灰与幻想的格林姆迦尔", "灰と幻想のグリムガル", "灰与幻想之格林姆迦尔", true},
		{"间谍过家家 / SPY X FAMILY", "SPY×FAMILY", "间谍过家家", true},
		{"SPY X FAMILY", "SPY×FAMILY", "间谍过家家", true},
		{"HUNTER X HUNTER", "HUNTER×HUNTER", "全职猎人", true},
		{"That Time I Got Reincarnated as a Slime Season 3", "That Time I Got Reincarnated as a Slime Season 3", "关于我转生变成史莱姆这档事 第三季", true},
		{"间谍过家家 / SPY X FAMILY II", "SPY×FAMILY Season 2", "间谍过家家 第二季", true},
		{"间谍过家家 / SPY X FAMILY II", "SPY×FAMILY", "间谍过家家", false},
	}
	for _, c := range cases {
		t.Run(c.query+"/"+c.name, func(t *testing.T) {
			score := AliasesScore(QueryTitles(c.query), c.name, c.nameCN)
			if got := score > 0; got != c.want {
				t.Fatalf("accept=%v, want %v (score=%d)", got, c.want, score)
			}
		})
	}
}

func TestMetaRomanMarkersOnlyAtSeasonSuffix(t *testing.T) {
	for _, tc := range []struct {
		title  string
		season int
	}{
		{"SPY X FAMILY", 0}, {"HUNTER X HUNTER", 0},
		{"That Time I Got Reincarnated as a Slime Season 3", 3},
		{"Mushoku Tensei III: Isekai Ittara Honki Dasu", 3},
		{"無職転生Ⅲ ～異世界行ったら本気だす～", 3},
		{"测试动画 [II]", 2}, {"测试动画 II Part 2", 2},
	} {
		_, season, _ := TitleParts(tc.title)
		if season != tc.season {
			t.Errorf("season of %q: got %d want %d", tc.title, season, tc.season)
		}
	}
}

func TestDiscoverSeasonIntent(t *testing.T) {
	for _, tc := range []struct {
		input, base string
		season      int
	}{
		{"无职转生 第一季", "无职转生", 1}, {"无职转生第一季", "无职转生", 1},
		{"Example S01", "Example", 1}, {"Example Season 2", "Example", 2},
		{"Example 3rd Season", "Example", 3}, {"86", "86", 0},
		{"Example Part 2", "Example Part 2", 0}, {"Example S01E02", "Example S01E02", 0},
		{"第一季", "第一季", 0}, {"三月的狮子", "三月的狮子", 0},
	} {
		base, season := DiscoverIntent(tc.input)
		if base != tc.base || season != tc.season {
			t.Fatalf("%q: %q %d", tc.input, base, season)
		}
	}
}

func TestResourceSeasonEvidence(t *testing.T) {
	for _, tc := range []struct {
		title  string
		season int
	}{
		{"[组] 测试动画 [S1 Reseed Fin]", 1}, {"[组] 测试动画 [S01] [01]", 1},
		{"测试动画 第二季 / Example S2 Part 2", 2}, {"测试动画 第一季 / Example S2", -1},
		{"测试动画 Part 2 - 01", 0}, {"测试动画 [2021][01-24合集]", 0},
		{"SPY X FAMILY [01]", 0}, {"测试动画 II", 2},
		{"[VCB-Studio] 无职转生 II [01][1080p]", 2},
		{"[Group] Example II - 01 [1080p]", 2},
		{"[Group] Example ii - 01 [1080p]", 2},
		{"[Group] Example_S02_01_1080p", 2},
		{"[Group] Example_S02E01_1080p", 2},
		{"[Group] 无职转生Ⅱ [01][1080p]", 2},
		{"[Group] Example XII [01][1080p]", 0},
		{"[Group] 测试动画Ⅻ [01][1080p]", 12},
		{"[Group] 测试动画 第二季 / Example III [01][1080p]", -1},
		{"[Group] Example_S02_S03E01_1080p", -1},
		{"[Group] SPY X FAMILY [01][1080p]", 0},
		{"[Group] That Time I Got Reincarnated [01][1080p]", 0},
		{"四季青春 [01]", 0}, {"四季青春 第三季 [01]", 3},
		{"Example S01E02", 1}, {"Example S01E02 / Example S02E02", -1},
	} {
		if got := ResourceSeason(tc.title); got != tc.season {
			t.Fatalf("%s: %d want %d", tc.title, got, tc.season)
		}
		if AllowsSeason(tc.title, 1, false) != (tc.season == 1) {
			t.Fatal("strict filter mismatch")
		}
		if AllowsSeason(tc.title, 1, true) != (tc.season == 1 || tc.season == 0) {
			t.Fatal("unknown opt-in mismatch")
		}
		if tc.season != 0 {
			want := max(tc.season, 0)
			if got := Extract(tc.title).Season; got != want {
				t.Fatalf("filter/archive season mismatch for %q: %d want %d", tc.title, got, want)
			}
		}
	}
	info := Extract("[组] 测试动画 [S1 Reseed Fin] [1080p]")
	if info.Season != 1 || info.Title != "测试动画" {
		t.Fatalf("mixed bracket: %+v", info)
	}
	for _, depth := range []string{"10-bit", "10bit", "10 bit", "8-bit"} {
		info = Extract("[组] 测试动画 / Example " + depth + " 1080p HEVC BDRip [S1 Reseed Fin]")
		if info.Season != 1 || info.Title != "测试动画" || info.Episode != "" {
			t.Fatalf("bit depth became episode: %+v", info)
		}
		info = Extract("[组] 测试动画 - 02 " + depth + " 1080p [S1]")
		if info.Episode != "02" {
			t.Fatalf("real episode lost: %+v", info)
		}
	}
	info = Extract("[7ACG] 测试动画 / Example S01 | 01-24 [简繁字幕] BDrip 1080p AV1 OPUS 2.0(v2)")
	if info.Season != 1 || info.Episode != "" {
		t.Fatalf("batch audio channels became episode: %+v", info)
	}
}
