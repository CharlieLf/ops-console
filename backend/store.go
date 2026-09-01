package main

import (
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const maxHistory = 60

// PruneTargets selects which classes of Docker objects a prune run touches.
type PruneTargets struct {
	BuildCache        bool `json:"buildCache"`
	DanglingImages    bool `json:"danglingImages"`
	UnusedImages      bool `json:"unusedImages"`
	StoppedContainers bool `json:"stoppedContainers"`
	Networks          bool `json:"networks"`
	Volumes           bool `json:"volumes"`
}

type PruneConfig struct {
	Enabled bool `json:"enabled"`
	// Mode is "interval", "daily" or "weekly".
	Mode          string `json:"mode"`
	IntervalHours int    `json:"intervalHours"`
	TimeOfDay     string `json:"timeOfDay"`
	Weekday       int    `json:"weekday"`
	Timezone      string `json:"timezone"`

	Targets     PruneTargets `json:"targets"`
	MinAgeHours int          `json:"minAgeHours"`
	// KeepPatterns are glob patterns matched against image refs and volume
	// names. Anything matching is never removed.
	KeepPatterns []string `json:"keepPatterns"`
	// DryRun makes scheduled runs report what they would remove without
	// removing anything.
	DryRun bool `json:"dryRun"`
}

func defaultConfig() PruneConfig {
	return PruneConfig{
		Enabled:       false,
		Mode:          "daily",
		IntervalHours: 24,
		TimeOfDay:     "03:30",
		Weekday:       0,
		Timezone:      "Asia/Jakarta",
		Targets: PruneTargets{
			BuildCache:        true,
			DanglingImages:    true,
			UnusedImages:      false,
			StoppedContainers: true,
			Networks:          true,
			Volumes:           false,
		},
		MinAgeHours: 72,
		// The shared reverse-proxy network must survive a moment when every
		// stack happens to be down.
		KeepPatterns: []string{"proxy"},
		DryRun:       false,
	}
}

// Normalize repairs out-of-range values so a bad payload can never produce a
// scheduler that fires constantly or never.
func (c *PruneConfig) Normalize() {
	switch c.Mode {
	case "interval", "daily", "weekly":
	default:
		c.Mode = "daily"
	}
	if c.IntervalHours < 1 {
		c.IntervalHours = 1
	}
	if c.IntervalHours > 24*30 {
		c.IntervalHours = 24 * 30
	}
	if _, _, ok := parseHHMM(c.TimeOfDay); !ok {
		c.TimeOfDay = "03:30"
	}
	if c.Weekday < 0 || c.Weekday > 6 {
		c.Weekday = 0
	}
	if c.Timezone == "" {
		c.Timezone = "UTC"
	}
	if _, err := time.LoadLocation(c.Timezone); err != nil {
		c.Timezone = "UTC"
	}
	if c.MinAgeHours < 0 {
		c.MinAgeHours = 0
	}
	if c.KeepPatterns == nil {
		c.KeepPatterns = []string{}
	}
}

func (c PruneConfig) Location() *time.Location {
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

type PruneItem struct {
	Kind    string `json:"kind"`
	Ref     string `json:"ref"`
	Size    int64  `json:"size"`
	Removed bool   `json:"removed"`
	Reason  string `json:"reason,omitempty"`
}

type PruneRun struct {
	ID         string      `json:"id"`
	Trigger    string      `json:"trigger"`
	DryRun     bool        `json:"dryRun"`
	StartedAt  time.Time   `json:"startedAt"`
	FinishedAt time.Time   `json:"finishedAt"`
	Reclaimed  int64       `json:"reclaimed"`
	Items      []PruneItem `json:"items"`
	Errors     []string    `json:"errors"`
	Targets    PruneTargets `json:"targets"`
}

type state struct {
	Config  PruneConfig `json:"config"`
	History []PruneRun  `json:"history"`
	LastRun *time.Time  `json:"lastRun"`
}

type Store struct {
	mu   sync.RWMutex
	path string
	st   state
}

func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &Store{
		path: filepath.Join(dir, "ops-console.json"),
		st:   state{Config: defaultConfig(), History: []PruneRun{}},
	}
	buf, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, s.persist()
		}
		return nil, err
	}
	if err := json.Unmarshal(buf, &s.st); err != nil {
		return nil, err
	}
	s.st.Config.Normalize()
	if s.st.History == nil {
		s.st.History = []PruneRun{}
	}
	return s, nil
}

func (s *Store) persist() error {
	buf, err := json.MarshalIndent(s.st, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, buf, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) Config() PruneConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.st.Config
}

func (s *Store) SetConfig(c PruneConfig) (PruneConfig, error) {
	c.Normalize()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.st.Config = c
	return c, s.persist()
}

func (s *Store) LastRun() *time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.st.LastRun
}

func (s *Store) History() []PruneRun {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]PruneRun, len(s.st.History))
	copy(out, s.st.History)
	return out
}

// RecordRun prepends a run to the history. Only real (non dry) runs advance the
// schedule clock, so a preview never delays the next scheduled prune.
func (s *Store) RecordRun(run PruneRun) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.st.History = append([]PruneRun{run}, s.st.History...)
	if len(s.st.History) > maxHistory {
		s.st.History = s.st.History[:maxHistory]
	}
	if !run.DryRun {
		t := run.FinishedAt
		s.st.LastRun = &t
	}
	return s.persist()
}

func newID() string {
	const alphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
	b := make([]byte, 10)
	if _, err := rand.Read(b); err != nil {
		return time.Now().UTC().Format("20060102150405")
	}
	out := make([]byte, len(b))
	for i := range b {
		out[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(out)
}
