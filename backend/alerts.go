package main

import (
	"context"
	"fmt"
)

type Alert struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	Kind     string `json:"kind"`
	Target   string `json:"target"`
	Title    string `json:"title"`
	Detail   string `json:"detail"`
}

type Alerts struct {
	collector *Collector
	metrics   *Metrics
}

func NewAlerts(c *Collector, m *Metrics) *Alerts {
	return &Alerts{collector: c, metrics: m}
}

func (a *Alerts) List(ctx context.Context) ([]Alert, error) {
	snap, err := a.collector.Snapshot(ctx, false)
	if err != nil {
		return nil, err
	}
	m := a.metrics.Latest()
	stats := map[string]ContainerStat{}
	for _, st := range m.Containers {
		stats[st.ID] = st
		stats[st.Name] = st
	}

	var out []Alert
	for _, c := range snap.Containers {
		if c.Health == "unhealthy" {
			out = append(out, Alert{
				ID: "unhealthy:" + c.Name, Severity: "high", Kind: "health",
				Target: c.Name, Title: "Container unhealthy", Detail: c.Status,
			})
		}
		if c.RestartCount > 5 {
			out = append(out, Alert{
				ID: "restarts:" + c.Name, Severity: "warn", Kind: "reliability",
				Target: c.Name, Title: fmt.Sprintf("%d restarts", c.RestartCount),
				Detail: "Possible crash loop",
			})
		}
		st, ok := stats[c.ID]
		if !ok {
			st = stats[c.Name]
		}
		if ok && c.State == "running" {
			if st.CPUPct >= 80 {
				out = append(out, Alert{
					ID: "cpu:" + c.Name, Severity: "warn", Kind: "resources",
					Target: c.Name, Title: fmt.Sprintf("High CPU %.0f%%", st.CPUPct),
					Detail: c.Project,
				})
			}
			if st.MemPct >= 85 {
				out = append(out, Alert{
					ID: "mem:" + c.Name, Severity: "warn", Kind: "resources",
					Target: c.Name, Title: fmt.Sprintf("High memory %.0f%%", st.MemPct),
					Detail: humanBytes(st.MemUsage),
				})
			}
		}
	}
	if m.Host.CPUPct >= 90 {
		out = append(out, Alert{
			ID: "host-cpu", Severity: "warn", Kind: "host",
			Target: "host", Title: fmt.Sprintf("Host CPU %.0f%%", m.Host.CPUPct),
		})
	}
	if m.Host.MemPct >= 90 {
		out = append(out, Alert{
			ID: "host-mem", Severity: "warn", Kind: "host",
			Target: "host", Title: fmt.Sprintf("Host memory %.0f%%", m.Host.MemPct),
			Detail: humanBytes(m.Host.MemUsed) + " used",
		})
	}
	if m.Disk.UsedRatio >= 0.9 {
		out = append(out, Alert{
			ID: "host-disk", Severity: "high", Kind: "host",
			Target: "host", Title: fmt.Sprintf("Disk %.0f%% full", m.Disk.UsedRatio*100),
			Detail: humanBytes(m.Disk.Free) + " free",
		})
	}
	return out, nil
}
