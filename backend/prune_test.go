package main

import (
	"testing"
	"time"
)

func TestProtected(t *testing.T) {
	patterns := []string{"proxy", "monitoring_*", "postgres:1?"}
	cases := []struct {
		ref  string
		want bool
	}{
		{"proxy", true},
		{"npm_proxy_data", true},
		{"monitoring_beszel_data", true},
		{"monitor_data", false},
		{"postgres:16", true},
		{"postgres:9", false},
		{"redis:7", false},
	}
	for _, tc := range cases {
		if got, _ := protected(tc.ref, patterns); got != tc.want {
			t.Errorf("protected(%q) = %v, want %v", tc.ref, got, tc.want)
		}
	}
}

func TestProtectedIgnoresEmptyPatterns(t *testing.T) {
	if got, _ := protected("anything", []string{"", "   "}); got {
		t.Fatal("blank patterns must not protect everything")
	}
}

// The network list endpoint returns an empty Containers map, so attachment has
// to come from the containers themselves.
func TestAttachedNetworks(t *testing.T) {
	var c Container
	c.NetworkSettings.Networks = map[string]struct {
		IPAddress string `json:"IPAddress"`
	}{"proxy": {IPAddress: "172.18.0.2"}}

	attached := attachedNetworks([]Container{c})
	if !attached["proxy"] {
		t.Fatal("proxy must be seen as attached")
	}
	if attached["orphan"] {
		t.Fatal("unrelated network must not be attached")
	}
}

func TestNextAfterDaily(t *testing.T) {
	cfg := defaultConfig()
	cfg.Mode = "daily"
	cfg.TimeOfDay = "03:30"
	cfg.Timezone = "UTC"

	base := time.Date(2026, 8, 27, 1, 0, 0, 0, time.UTC)
	next := nextAfter(cfg, base)
	if !next.Equal(time.Date(2026, 8, 27, 3, 30, 0, 0, time.UTC)) {
		t.Fatalf("got %s", next)
	}

	base = time.Date(2026, 8, 27, 4, 0, 0, 0, time.UTC)
	next = nextAfter(cfg, base)
	if !next.Equal(time.Date(2026, 8, 28, 3, 30, 0, 0, time.UTC)) {
		t.Fatalf("got %s", next)
	}
}

func TestNextAfterWeekly(t *testing.T) {
	cfg := defaultConfig()
	cfg.Mode = "weekly"
	cfg.TimeOfDay = "02:00"
	cfg.Timezone = "UTC"
	cfg.Weekday = int(time.Sunday)

	// Thursday 2026-08-27 -> next Sunday is 2026-08-30.
	next := nextAfter(cfg, time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC))
	if !next.Equal(time.Date(2026, 8, 30, 2, 0, 0, 0, time.UTC)) {
		t.Fatalf("got %s", next)
	}
}

func TestNextAfterInterval(t *testing.T) {
	cfg := defaultConfig()
	cfg.Mode = "interval"
	cfg.IntervalHours = 6
	base := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	if got := nextAfter(cfg, base); !got.Equal(base.Add(6 * time.Hour)) {
		t.Fatalf("got %s", got)
	}
}

func TestNormalizeRejectsGarbage(t *testing.T) {
	cfg := PruneConfig{Mode: "hourly", IntervalHours: 0, TimeOfDay: "99:99", Weekday: 12, Timezone: "Mars/Olympus", MinAgeHours: -5}
	cfg.Normalize()
	if cfg.Mode != "daily" || cfg.IntervalHours != 1 || cfg.TimeOfDay != "03:30" || cfg.Weekday != 0 || cfg.Timezone != "UTC" || cfg.MinAgeHours != 0 {
		t.Fatalf("normalize left bad values: %+v", cfg)
	}
}

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{0: "0 B", 999: "999 B", 1024: "1.0 KB", 1536: "1.5 KB", 5 << 30: "5.0 GB"}
	for in, want := range cases {
		if got := humanBytes(in); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", in, got, want)
		}
	}
}
