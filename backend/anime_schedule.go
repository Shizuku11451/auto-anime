package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
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

// resolveBroadcastWeekday 先用条目的放送星期字段；缺失或多日时退回每日放送表，
// 放送表只列当季在播条目，每部只出现在一天。
func resolveBroadcastWeekday(subject *bangumiSubject) int {
	if day := subjectBroadcastWeekday(subject); day != 0 {
		return day
	}
	return calendarWeekdayLookup(int64(subject.ID))
}

// 测试替换为离线实现，避免依赖真实放送表。
var calendarWeekdayLookup = calendarWeekday

var bangumiCalendarCache struct {
	sync.Mutex
	api     string
	fetched time.Time
	days    map[int64]int
}

const bangumiCalendarTTL = 6 * time.Hour

// 放送表获取失败只是少一个补充来源，不影响资料保存；失败后同样等待一段时间再试。
func calendarWeekday(subjectID int64) int {
	if subjectID <= 0 {
		return 0
	}
	c := &bangumiCalendarCache
	c.Lock()
	defer c.Unlock()
	if c.api != bangumiAPI || time.Since(c.fetched) > bangumiCalendarTTL {
		days, err := fetchBangumiCalendar()
		if err != nil {
			log.Printf("Bangumi 放送表获取失败: %v", err)
		}
		c.api, c.fetched, c.days = bangumiAPI, time.Now(), days
	}
	return c.days[subjectID]
}

func fetchBangumiCalendar() (map[int64]int, error) {
	req, err := http.NewRequest("GET", bangumiAPI+"/calendar", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", metaUserAgent)
	res, err := metaClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, (4<<20)+1))
	if err != nil || len(data) > 4<<20 {
		return nil, fmt.Errorf("响应无效或超过 4 MB")
	}
	var calendar []struct {
		Weekday struct {
			ID int `json:"id"`
		} `json:"weekday"`
		Items []struct {
			ID int64 `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &calendar); err != nil {
		return nil, err
	}
	days := map[int64]int{}
	for _, day := range calendar {
		if day.Weekday.ID < 1 || day.Weekday.ID > 7 {
			continue
		}
		for _, item := range day.Items {
			if prev, ok := days[item.ID]; ok && prev != day.Weekday.ID {
				days[item.ID] = 0
			} else if item.ID > 0 {
				days[item.ID] = day.Weekday.ID
			}
		}
	}
	return days, nil
}

func broadcastCacheFresh(checked string) bool {
	at, err := time.Parse(time.RFC3339, checked)
	return err == nil && at.After(time.Now().Add(-7*24*time.Hour))
}

// 未获取到星期的结果一天后重查：新番资料常在开播后才补全。
func broadcastWeekdayFresh(checked string, day int) bool {
	if day != 0 {
		return broadcastCacheFresh(checked)
	}
	at, err := time.Parse(time.RFC3339, checked)
	return err == nil && at.After(time.Now().Add(-24*time.Hour))
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
	var day int
	if err := db.QueryRow(`SELECT COALESCE(subject_id,0),COALESCE(source,''),COALESCE(broadcast_subject_id,0),COALESCE(broadcast_checked_at,''),COALESCE(episode_subject_id,0),COALESCE(broadcast_weekday,0) FROM anime_meta WHERE anime_title=?`, title).
		Scan(&subjectID, &source, &cachedID, &checked, &episodeID, &day); err != nil {
		return
	}
	if subjectID <= 0 || (source != "bangumi" && source != "manual") || (expected.BangumiID > 0 && expected.BangumiID != subjectID) {
		return
	}
	if cachedID == subjectID && episodeID == subjectID && broadcastWeekdayFresh(checked, day) {
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
		WHERE anime_title=? AND subject_id=? AND source=?`, resolveBroadcastWeekday(subject), subjectID, time.Now().UTC().Format(time.RFC3339), total, season, subjectID, title, subjectID, source)
	if err != nil {
		log.Printf("更新日保存失败 %q: %v", title, err)
	}
}
