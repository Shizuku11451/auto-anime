package filter

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
		{"fullwidth release", "[Group] ＡＩの遺電子 Ｓ０２Ｅ０３ [１０８０ｐ]", "AIの遺電子,[Group],1080P", "", true},
		{"fullwidth rule", "[Group] AIの遺電子 [1080p]", "ＡＩの遺電子,１０８０Ｐ", "", true},
		{"fullwidth exclude", "动画 ＡＤ", "动画", "ad", false},
		{"fullwidth exclude rule", "动画 Ad", "动画", "ＡＤ", false},
		{"fullwidth group boundary", "[AnotherＧｒｏｕｐ] ＡＩの遺電子 [１０８０ｐ]", "AIの遺電子,[Group],1080P", "", false},
		{"fullwidth different quality", "[Group] ＡＩの遺電子 [７２０ｐ]", "AIの遺電子,[Group],1080P", "", false},
		{"fullwidth missing word", "[Group] ＡＩの遺電子 [１０８０ｐ]", "AIの遺電子,不存在", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Allowed(tc.title, tc.include, tc.exclude); got != tc.want {
				t.Fatalf("Allowed(%q, %q, %q)=%v want=%v", tc.title, tc.include, tc.exclude, got, tc.want)
			}
		})
	}
}

func TestKeywordFilterConcurrentReads(t *testing.T) {
	filter := New("无职转生,[绿茶字幕组],1080P,繁日", "广告")
	var workers sync.WaitGroup
	for i := 0; i < 16; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for j := 0; j < 20; j++ {
				if !filter.Allows("[綠茶字幕組] 無職轉生 [1080p][繁日]") || filter.Allows("[綠茶字幕組] 無職轉生 [1080p][繁日] 廣告") {
					t.Error("concurrent matching changed result")
					return
				}
			}
		}()
	}
	workers.Wait()
}
