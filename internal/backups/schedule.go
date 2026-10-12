// Package backups schedules container-scoped backups without executing game processes.
package backups

import (
	"time"
	_ "time/tzdata"

	"minecraft-hosting/internal/server"
)

// NextDaily returns the next canonical local daily instant. Repeated times use
// their first occurrence; missing times use the first valid minute after the gap.
func NextDaily(after time.Time, hour, minute int, zone string) (time.Time, error) {
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 || zone == "" || zone == "Local" || len(zone) > 100 {
		return time.Time{}, server.ErrInvalid
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return time.Time{}, server.ErrInvalid
	}
	y, m, d := after.In(loc).Date()
	date := time.Date(y, m, d, 12, 0, 0, 0, time.UTC)
	for day := 0; day < 370; day++ {
		y, m, d = date.Date()
		base := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
		var exact, fallback time.Time
		// The supported IANA offsets fit within +/-24 hours. Enumerating instants
		// avoids time.Date's unspecified choice in gaps and repeated local hours.
		for instant := base.Add(-24 * time.Hour); instant.Before(base.Add(48 * time.Hour)); instant = instant.Add(time.Minute) {
			local := instant.In(loc)
			ly, lm, ld := local.Date()
			if ly != y || lm != m || ld != d {
				continue
			}
			localMinute := local.Hour()*60 + local.Minute()
			wanted := hour*60 + minute
			if localMinute == wanted && exact.IsZero() {
				exact = instant
			}
			if localMinute > wanted && (fallback.IsZero() || localMinute < fallback.In(loc).Hour()*60+fallback.In(loc).Minute()) {
				fallback = instant
			}
		}
		candidate := exact
		if candidate.IsZero() {
			candidate = fallback
		}
		if !candidate.IsZero() && candidate.After(after) {
			return candidate.UTC(), nil
		}
		date = date.AddDate(0, 0, 1)
	}
	return time.Time{}, server.ErrInvalid
}
