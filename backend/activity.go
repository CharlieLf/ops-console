package main

import (
	"bufio"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

type ActivityEvent struct {
	Time   time.Time         `json:"time"`
	Type   string            `json:"type"`
	Action string            `json:"action"`
	Name   string            `json:"name"`
	Detail string            `json:"detail"`
	Attrs  map[string]string `json:"attrs"`
}

type Activity struct {
	mu     sync.RWMutex
	events []ActivityEvent
	max    int
}

func NewActivity(max int) *Activity {
	return &Activity{events: []ActivityEvent{}, max: max}
}

func (a *Activity) List(n int) []ActivityEvent {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if n <= 0 || n > len(a.events) {
		n = len(a.events)
	}
	out := make([]ActivityEvent, n)
	copy(out, a.events[:n])
	return out
}

func (a *Activity) add(ev ActivityEvent) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append([]ActivityEvent{ev}, a.events...)
	if len(a.events) > a.max {
		a.events = a.events[:a.max]
	}
}

func (a *Activity) Run(ctx context.Context, d *Docker) {
	a.seed(ctx, d)
	a.stream(ctx, d)
}

func (a *Activity) seed(ctx context.Context, d *Docker) {
	since := time.Now().Add(-2 * time.Hour).Unix()
	q := url.Values{}
	q.Set("since", strconv.FormatInt(since, 10))
	q.Set("until", strconv.FormatInt(time.Now().Unix(), 10))
	q.Set("filters", `{"type":["container"]}`)
	events, err := d.fetchEvents(ctx, q)
	if err != nil {
		return
	}
	for i := len(events) - 1; i >= 0; i-- {
		a.add(events[i])
	}
}

func (a *Activity) stream(ctx context.Context, d *Docker) {
	q := url.Values{}
	q.Set("filters", `{"type":["container"]}`)
	for {
		if ctx.Err() != nil {
			return
		}
		err := d.streamEvents(ctx, q, func(ev ActivityEvent) { a.add(ev) })
		if ctx.Err() != nil {
			return
		}
		log.Printf("activity: stream ended (%v), reconnecting in 5s", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}

func (d *Docker) fetchEvents(ctx context.Context, q url.Values) ([]ActivityEvent, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker/events?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := d.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return readEventLines(resp.Body)
}

func (d *Docker) streamEvents(ctx context.Context, q url.Values, onEvent func(ActivityEvent)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker/events?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	resp, err := d.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)
	for sc.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		ev, ok := parseEventLine(sc.Bytes())
		if ok {
			onEvent(ev)
		}
	}
	return sc.Err()
}

func readEventLines(r interface{ Read([]byte) (int, error) }) ([]ActivityEvent, error) {
	sc := bufio.NewScanner(r)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)
	var out []ActivityEvent
	for sc.Scan() {
		if ev, ok := parseEventLine(sc.Bytes()); ok {
			out = append(out, ev)
		}
	}
	return out, sc.Err()
}

func parseEventLine(line []byte) (ActivityEvent, bool) {
	var raw struct {
		Type   string `json:"Type"`
		Action string `json:"Action"`
		Actor  struct {
			ID         string            `json:"ID"`
			Attributes map[string]string `json:"Attributes"`
		} `json:"Actor"`
		Time     int64 `json:"time"`
		TimeNano int64 `json:"timeNano"`
	}
	if err := json.Unmarshal(line, &raw); err != nil {
		return ActivityEvent{}, false
	}
	name := raw.Actor.Attributes["name"]
	if name == "" {
		name = shortID(raw.Actor.ID)
	}
	t := time.Unix(0, raw.TimeNano)
	if raw.TimeNano == 0 && raw.Time > 0 {
		t = time.Unix(raw.Time, 0)
	}
	detail := raw.Actor.Attributes["image"]
	if detail == "" {
		detail = raw.Actor.Attributes["signal"]
	}
	return ActivityEvent{
		Time: t.UTC(), Type: raw.Type, Action: raw.Action,
		Name: name, Detail: detail, Attrs: raw.Actor.Attributes,
	}, true
}
