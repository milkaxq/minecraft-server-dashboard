package backups

import (
	"testing"
	"time"
)

func TestNextDaily(t *testing.T) {
	cases := []struct {
		name, after, zone, want string
		hour, minute            int
	}{
		{"normal", "2026-10-13T07:00:00Z", "UTC", "2026-10-13T08:15:00Z", 8, 15},
		{"equal moves tomorrow", "2026-10-13T08:15:00Z", "UTC", "2026-10-14T08:15:00Z", 8, 15},
		{"quarter offset", "2026-10-13T00:00:00Z", "Asia/Kathmandu", "2026-10-13T02:15:00Z", 8, 0},
		{"spring gap", "2026-03-28T23:00:00Z", "Europe/Berlin", "2026-03-29T01:00:00Z", 2, 30},
		{"fall earlier", "2026-10-24T23:00:00Z", "Europe/Berlin", "2026-10-25T00:30:00Z", 2, 30},
		{"fall no second", "2026-10-25T00:30:00Z", "Europe/Berlin", "2026-10-26T01:30:00Z", 2, 30},
		{"half-hour gap", "2026-10-03T12:00:00Z", "Australia/Lord_Howe", "2026-10-03T15:30:00Z", 2, 15},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			after, _ := time.Parse(time.RFC3339, c.after)
			want, _ := time.Parse(time.RFC3339, c.want)
			got, err := NextDaily(after, c.hour, c.minute, c.zone)
			if err != nil || !got.Equal(want) {
				t.Fatalf("got %v, %v; want %v", got, err, want)
			}
		})
	}
}
func TestNextDailyInvalid(t *testing.T) {
	for _, c := range []struct {
		hour, minute int
		zone         string
	}{{-1, 0, "UTC"}, {24, 0, "UTC"}, {0, -1, "UTC"}, {0, 60, "UTC"}, {1, 1, "unknown/zone"}, {1, 1, ""}, {1, 1, "Local"}} {
		if _, err := NextDaily(time.Now(), c.hour, c.minute, c.zone); err == nil {
			t.Fatalf("accepted %+v", c)
		}
	}
}
