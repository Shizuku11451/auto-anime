package main

import (
	"encoding/json"
	"log"
	"regexp"
	"strings"
	"time"
)

var broadcastDayPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(?:星期|礼拜|禮拜|周|週)一|月曜(?:日)?|\bmonday\b`),
	regexp.MustCompile(`(?i)(?:星期|礼拜|禮拜|周|週)二|火曜(?:日)?|\btuesday\b`),
	regexp.MustCompile(`(?i)(?:星期|礼拜|禮拜|周|週)三|水曜(?:日)?|\bwednesday\b`),
	regexp.MustCompile(`(?i)(?:星期|礼拜|禮拜|周|週)四|木曜(?:日)?|\bthursday\b`),
	regexp.MustCompile(`(?i)(?:星期|礼拜|禮拜|周|週)五|金曜(?:日)?|\bfriday\b`),
	regexp.MustCompile(`(?i)(?:星期|礼拜|禮拜|周|週)六|土曜(?:日)?|\bsaturday\b`),
	regexp.MustCompile(`(?i)(?:星期|礼拜|禮拜|周|週)[日天]|日曜(?:日)?|\bsunday\b`),
}

// 只采用明确的星期字段。首播日期不保证后续周更，多日或不定期不强行归到一天。
func subjectBroadcastWeekday(subject *bangumiSubject) int {
	var fields []struct {
		Key   string          `json:"key"`
		Value json.RawMessage `json:"value"`
	}
	if json.Unmarshal(subject.Infobox, &fields) != nil {
		return 0
	}
	day := 0
	for _, field := range fields {
		switch strings.TrimSpace(field.Key) {
		case "放送星期", "放送曜日", "播出星期":
		default:
			continue
		}
		var value string
		if json.Unmarshal(field.Value, &value) != nil {
			var values []struct {
				Value string `json:"v"`
			}
			if json.Unmarshal(field.Value, &values) != nil {
				return 0
			}
			for _, v := range values {
				value += " " + v.Value
			}
		}
		for _, uncertain := range []string{"不定", "未定", "待定", "每天", "每日", "、", "/", "至", "到", "～", "〜", "~", "-", "及", "和", "或", "&"} {
			if strings.Contains(value, uncertain) {
				return 0
			}
		}
		matched := 0
		for i, pattern := range broadcastDayPatterns {
			if pattern.MatchString(value) {
				if matched != 0 || (day != 0 && day != i+1) {
					return 0
				}
				matched = i + 1
			}
		}
		if matched == 0 {
			return 0
		}
		day = matched
	}
	return day
}

func broadcastCacheFresh(checked string) bool {
	at, err := time.Parse(time.RFC3339, checked)
	return err == nil && at.After(time.Now().Add(-7*24*time.Hour))
}

// 旧资料无需重新搜索或下载封面；单独补全星期。按番剧 ID 跟随官方名规范化。
func lookupMetaWithSchedule(title string) {
	var id int64
	db.QueryRow("SELECT id FROM animes WHERE title=?", title).Scan(&id)
	lookupAndCacheMeta(title)
	if id > 0 {
		if err := db.QueryRow("SELECT title FROM animes WHERE id=?", id).Scan(&title); err != nil {
			return
		}
	}
	refreshBroadcastWeekday(title)
	refreshFeedEpisodeTotals(title)
}

func refreshBroadcastWeekday(title string) {
	if !bangumiEnabled {
		return
	}
	expected, err := readMetaLookupState(db, title)
	if err != nil || expected.ID == 0 {
		return
	}
	var subjectID, cachedID, episodeID int64
	var source, checked string
	if err := db.QueryRow(`SELECT COALESCE(subject_id,0),COALESCE(source,''),COALESCE(broadcast_subject_id,0),COALESCE(broadcast_checked_at,''),COALESCE(episode_subject_id,0) FROM anime_meta WHERE anime_title=?`, title).
		Scan(&subjectID, &source, &cachedID, &checked, &episodeID); err != nil {
		return
	}
	if subjectID <= 0 || (source != "bangumi" && source != "manual") || (expected.BangumiID > 0 && expected.BangumiID != subjectID) {
		return
	}
	if cachedID == subjectID && episodeID == subjectID && broadcastCacheFresh(checked) {
		return
	}
	subject, err := fetchBangumiSubject(subjectID)
	if err != nil {
		log.Printf("更新日获取失败 %q: %v", title, err)
		return
	}
	if int64(subject.ID) != subjectID {
		return
	}
	// 元数据绑定、重识别、改名或手动编辑期间的迟到响应不能覆盖当前记录。
	total, season := subjectEpisodes(subject)
	_, err = writeMetaResult(title, &expected, `UPDATE anime_meta SET broadcast_weekday=?,broadcast_subject_id=?,broadcast_checked_at=?,episode_total=?,episode_season=?,episode_subject_id=?
		WHERE anime_title=? AND subject_id=? AND source=?`, subjectBroadcastWeekday(subject), subjectID, time.Now().UTC().Format(time.RFC3339), total, season, subjectID, title, subjectID, source)
	if err != nil {
		log.Printf("更新日保存失败 %q: %v", title, err)
	}
}
