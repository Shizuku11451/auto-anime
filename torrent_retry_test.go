package main

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestTorrentRateLimitRetry(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		retry     string
		failures  int
		calls     int
		waits     []time.Duration
		wantError bool
	}{
		{"recovers", 429, "7", 1, 2, []time.Duration{7 * time.Second, time.Second}, false},
		{"bounded", 429, "", 9, 3, []time.Duration{5 * time.Second, 10 * time.Second, time.Second}, true},
		{"long cooldown", 429, "3600", 1, 1, []time.Duration{time.Second}, true},
		{"other errors", 403, "", 1, 1, []time.Duration{time.Second}, true},
		{"http date", 429, time.Now().Add(time.Hour).UTC().Format(http.TimeFormat), 1, 1, []time.Duration{time.Second}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls <= tc.failures {
					w.Header().Set("Retry-After", tc.retry)
					w.WriteHeader(tc.status)
					return
				}
				w.Write([]byte("d4:infodee"))
			}))
			defer srv.Close()
			var waits []time.Duration
			data, err := fetchTorrentWithRetry(srv.URL, func(d time.Duration) { waits = append(waits, d) })
			if (err != nil) != tc.wantError || calls != tc.calls || !reflect.DeepEqual(waits, tc.waits) {
				t.Fatalf("calls=%d waits=%v err=%v", calls, waits, err)
			}
			if err == nil && string(data) != "d4:infodee" {
				t.Fatal("torrent body lost")
			}
			if err != nil && tc.status == 429 && !strings.Contains(err.Error(), "429") {
				t.Fatal(err)
			}
		})
	}
}
