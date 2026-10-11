package main

import (
	"sync"
	"testing"
)

func TestKeywordFilterChineseVariants(t *testing.T) {
	traditional := "[綠茶字幕組] 無職轉生 第三季 ～到了異世界就拿出真本事～ [14][1080p][繁日內嵌]"
	for _, tc := range []struct {
		name, title, include, exclude string
		want                          bool
	}{
		{"mixed script", traditional, "无职转生,[绿茶字幕组],1080P,繁日", "", true},
		{"reverse script", "[绿茶字幕组] 无职转生 [1080p][简日内嵌]", "無職轉生,[綠茶字幕組],簡日", "", true},
		{"exclude variants", traditional, "无职转生", "内嵌", false},
		{"exclude reverse", "动画 广告", "動畫", "廣告", false},
		{"missing word retained", traditional, "无职转生,不存在", "", false},
		{"group boundary retained", traditional, "[另一绿茶字幕组]", "", false},
		{"different quality", traditional, "无职转生,720p", "", false},
		{"simplified subtitle is different", traditional, "无职转生,简日", "", false},
		{"simplified Chinese subtitle is different", traditional, "无职转生,简中", "", false},
		{"both subtitle variant characters", "動畫 [簡繁日內封]", "动画,简繁", "", true},
		{"exclude traditional subtitle only", "動畫 [簡日內嵌]", "动画", "繁日", true},
		{"English alias is not inferred", traditional, "Jobless Reincarnation", "", false},
		{"not pinyin matching", traditional, "wuzhizhuansheng", "", false},
		{"empty rules", traditional, " , ", " , ", true},
		{"ASCII case and punctuation", "[Group] Summer S3 [1080p]", "[GROUP],summer,1080P", "广告", true},
		{"arbitrary Chinese outside screenshot", "進擊的巨人 [驚奇字幕組]", "进击的巨人,惊奇字幕组", "", true},
		{"phrase context keeps literal include", "乾坤", "乾", "", true},
		{"phrase context keeps literal exclude", "乾坤", "", "乾", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := allowed(tc.title, tc.include, tc.exclude); got != tc.want {
				t.Fatalf("allowed(%q, %q, %q)=%v want=%v", tc.title, tc.include, tc.exclude, got, tc.want)
			}
		})
	}
}

func TestKeywordFilterConcurrentReads(t *testing.T) {
	filter := newKeywordFilter("无职转生,[绿茶字幕组],1080P,繁日", "广告")
	var workers sync.WaitGroup
	for i := 0; i < 16; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for j := 0; j < 20; j++ {
				if !filter.allows("[綠茶字幕組] 無職轉生 [1080p][繁日]") || filter.allows("[綠茶字幕組] 無職轉生 [1080p][繁日] 廣告") {
					t.Error("concurrent matching changed result")
					return
				}
			}
		}()
	}
	workers.Wait()
}

func TestAnimeRuleUsesChineseVariantMatching(t *testing.T) {
	testDatabase(t)
	if _, err := db.Exec(`INSERT INTO animes(id,title,include_rules,exclude_rules) VALUES(1,'无职转生','无职转生,[绿茶字幕组],繁日','广告')`); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		title string
		id    int64
	}{
		{"[綠茶字幕組] 無職轉生 [1080p][繁日]", 1},
		{"[綠茶字幕組] 無職轉生 [1080p][繁日] 廣告", 0},
		{"[綠茶字幕組] 無職轉生 [1080p][簡日]", 0},
	} {
		if id, _ := matchAnimeRule(tc.title); id != tc.id {
			t.Fatalf("matchAnimeRule(%q)=%d want=%d", tc.title, id, tc.id)
		}
	}
}
