package main

import (
	"context"
	"log"
	"strconv"
	"strings"
	"time"
)

func parseHHMM(s string) (int, int, bool) {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) != 2 {
		return 0, 0, false
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil || h < 0 || h > 23 {
		return 0, 0, false
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil || m < 0 || m > 59 {
		return 0, 0, false
	}
	return h, m, true
}

// nextAfter returns the first scheduled instant strictly after t.
func nextAfter(c PruneConfig, t time.Time) time.Time {
	loc := c.Location()
	h, m, ok := parseHHMM(c.TimeOfDay)
	if !ok {
		h, m = 3, 30
	}
	switch c.Mode {
	case "interval":
		return t.Add(time.Duration(c.IntervalHours) * time.Hour)
	case "weekly":
		local := t.In(loc)
		candidate := time.Date(local.Year(), local.Month(), local.Day(), h, m, 0, 0, loc)
		for i := 0; i < 8; i++ {
			if int(candidate.Weekday()) == c.Weekday && candidate.After(t) {
				return candidate
			}
			candidate = candidate.AddDate(0, 0, 1)
		}
		return candidate
	default:
		local := t.In(loc)
		candidate := time.Date(local.Year(), local.Month(), local.Day(), h, m, 0, 0, loc)
		if !candidate.After(t) {
			candidate = candidate.AddDate(0, 0, 1)
		}
		return candidate
	}
}

type Scheduler struct {
	store  *Store
	pruner *Pruner
	// anchor is the reference point when no run has happened yet. It resets on
	// restart so a crash-loop can never trigger repeated prunes.
	anchor time.Time
}

func NewScheduler(store *Store, pruner *Pruner) *Scheduler {
	return &Scheduler{store: store, pruner: pruner, anchor: time.Now()}
}

func (s *Scheduler) anchorTime() time.Time {
	if last := s.store.LastRun(); last != nil && last.After(s.anchor) {
		return *last
	}
	return s.anchor
}

// NextRun reports when the next scheduled prune fires, or nil when disabled.
func (s *Scheduler) NextRun() *time.Time {
	cfg := s.store.Config()
	if !cfg.Enabled {
		return nil
	}
	next := nextAfter(cfg, s.anchorTime())
	return &next
}

func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			cfg := s.store.Config()
			if !cfg.Enabled {
				continue
			}
			if nextAfter(cfg, s.anchorTime()).After(now) {
				continue
			}
			run, err := s.pruner.Run(ctx, cfg, "schedule", cfg.DryRun)
			if err != nil {
				// Busy means a manual run is in flight; retry on the next tick.
				if err == ErrPruneBusy {
					continue
				}
				log.Printf("scheduled prune failed: %v", err)
				s.anchor = time.Now()
				continue
			}
			log.Printf("scheduled prune %s finished: %d items, %s reclaimed (dryRun=%v)",
				run.ID, len(run.Items), humanBytes(run.Reclaimed), run.DryRun)
			if run.DryRun {
				// Dry runs do not advance LastRun, so move the anchor manually.
				s.anchor = time.Now()
			}
		}
	}
}
