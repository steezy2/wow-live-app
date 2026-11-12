package raiderio

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestTimersAndAffixesComeFromTheAPIOnceThenTheCache(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch r.URL.Path {
		case "/mythic-plus/static-data":
			if r.URL.Query().Get("expansion_id") != "11" {
				t.Errorf("expansion_id = %q", r.URL.Query().Get("expansion_id"))
			}
			w.Write([]byte(`{"seasons":[{"dungeons":[{"challenge_mode_id":585,"keystone_timer_seconds":1800}]}]}`))
		case "/mythic-plus/affixes":
			w.Write([]byte(`{"affix_details":[{"id":9,"name":"Tyrannical"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c := Client{HTTP: server.Client(), CacheDir: t.TempDir(), BaseURL: server.URL}
	for range 2 {
		timers, err := c.Timers(context.Background())
		if err != nil || !reflect.DeepEqual(timers, map[int]int64{585: 1_800_000}) {
			t.Fatalf("timers = %v, %v", timers, err)
		}
		affixes, err := c.Affixes(context.Background(), "us")
		if err != nil || affixes[9] != "Tyrannical" {
			t.Fatalf("affixes = %v, %v", affixes, err)
		}
	}
	if calls != 2 {
		t.Fatalf("API calls = %d; want 2 (the second round from the cache)", calls)
	}
}

func TestSeasonDatesByRegion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"seasons":[{"slug":"season-mn-1","starts":{"us":"2026-03-17T15:00:00Z","eu":null},
			"ends":{"us":null}}]}`))
	}))
	defer server.Close()
	c := Client{HTTP: server.Client(), CacheDir: t.TempDir(), BaseURL: server.URL}
	start, end, err := c.SeasonDates(context.Background(), "us")
	if err != nil || start != time.Date(2026, 3, 17, 15, 0, 0, 0, time.UTC) || !end.IsZero() {
		t.Fatalf("SeasonDates(us) = %v, %v, %v; want the start and no end yet", start, end, err)
	}
	if _, _, err := c.SeasonDates(context.Background(), "eu"); err == nil {
		t.Fatal("SeasonDates(eu) worked; want an error for a region without a start")
	}
}
