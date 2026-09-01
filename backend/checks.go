package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Finding struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	Category string `json:"category"`
	Target   string `json:"target"`
	Title    string `json:"title"`
	Detail   string `json:"detail"`
	Hint     string `json:"hint"`
}

type CheckReport struct {
	Findings []Finding `json:"findings"`
	Summary  struct {
		High int `json:"high"`
		Warn int `json:"warn"`
		Info int `json:"info"`
	} `json:"summary"`
}

const (
	sevHigh = "high"
	sevWarn = "warn"
	sevInfo = "info"
)

type Checker struct {
	collector *Collector
	store     *Store
	stacksDir string
	dataDir   string
}

func NewChecker(c *Collector, s *Store, stacksDir, dataDir string) *Checker {
	return &Checker{collector: c, store: s, stacksDir: stacksDir, dataDir: dataDir}
}

func (ck *Checker) Run(ctx context.Context) (*CheckReport, error) {
	snap, err := ck.collector.Snapshot(ctx, false)
	if err != nil {
		return nil, err
	}
	res, err := ck.collector.Resources(ctx, ck.store.Config().KeepPatterns)
	if err != nil {
		return nil, err
	}
	report := &CheckReport{Findings: []Finding{}}
	add := func(f Finding) { report.Findings = append(report.Findings, f) }

	if disk, err := diskStat(ck.dataDir); err == nil {
		pct := disk.UsedRatio * 100
		switch {
		case pct >= 90:
			add(Finding{ID: "disk-critical", Severity: sevHigh, Category: "host", Target: "filesystem",
				Title: fmt.Sprintf("Disk %.0f%% full", pct),
				Detail: fmt.Sprintf("%s free of %s.", humanBytes(disk.Free), humanBytes(disk.Total)),
				Hint:   "Run a prune and review the largest images and volumes."})
		case pct >= 80:
			add(Finding{ID: "disk-high", Severity: sevWarn, Category: "host", Target: "filesystem",
				Title: fmt.Sprintf("Disk %.0f%% full", pct),
				Detail: fmt.Sprintf("%s free of %s.", humanBytes(disk.Free), humanBytes(disk.Total)),
				Hint:   "Enable scheduled pruning to keep this in check."})
		}
	}

	reclaimable := res.Totals.ImagesReclaimable + res.Totals.VolumesReclaim + res.Totals.BuildCacheReclaim
	if reclaimable > 1<<30 {
		add(Finding{ID: "reclaimable", Severity: sevWarn, Category: "storage", Target: "docker",
			Title: humanBytes(reclaimable) + " of Docker data is reclaimable",
			Detail: fmt.Sprintf("Unused images %s, unused volumes %s, build cache %s.",
				humanBytes(res.Totals.ImagesReclaimable), humanBytes(res.Totals.VolumesReclaim), humanBytes(res.Totals.BuildCacheReclaim)),
			Hint: "Preview a prune run, then enable the schedule."})
	}

	if !ck.store.Config().Enabled {
		add(Finding{ID: "prune-disabled", Severity: sevInfo, Category: "maintenance", Target: "auto-prune",
			Title:  "Automatic pruning is off",
			Detail: "Docker garbage is only cleared when you run a prune by hand.",
			Hint:   "Turn on the schedule from the Auto-prune page."})
	}

	for _, c := range snap.Containers {
		if c.State == "running" && !c.HasHealthcheck {
			add(Finding{ID: "no-healthcheck:" + c.Name, Severity: sevWarn, Category: "reliability", Target: c.Name,
				Title:  "No healthcheck",
				Detail: "Docker cannot tell whether this container is actually serving.",
				Hint:   "Add a HEALTHCHECK to the image or a healthcheck block in Compose."})
		}
		if c.Health == "unhealthy" {
			add(Finding{ID: "unhealthy:" + c.Name, Severity: sevHigh, Category: "reliability", Target: c.Name,
				Title: "Container is unhealthy", Detail: c.Status, Hint: "Check container logs."})
		}
		if c.State == "running" && (c.RestartPolicy == "no" || c.RestartPolicy == "") {
			add(Finding{ID: "no-restart:" + c.Name, Severity: sevWarn, Category: "reliability", Target: c.Name,
				Title:  "No restart policy",
				Detail: "The container will stay down after a crash or host reboot.",
				Hint:   "Set restart: unless-stopped."})
		}
		if c.RestartCount > 3 {
			add(Finding{ID: "restart-loop:" + c.Name, Severity: sevWarn, Category: "reliability", Target: c.Name,
				Title:  fmt.Sprintf("Restarted %d times", c.RestartCount),
				Detail: "Frequent restarts usually mean a crash loop or a failing dependency.",
				Hint:   "Inspect logs and exit codes."})
		}
		if c.Privileged {
			add(Finding{ID: "privileged:" + c.Name, Severity: sevHigh, Category: "security", Target: c.Name,
				Title:  "Runs privileged",
				Detail: "A privileged container can take over the host.",
				Hint:   "Drop privileged and grant only the capabilities needed."})
		}
		for _, m := range c.Mounts {
			if strings.HasSuffix(m.Source, "docker.sock") && m.RW {
				add(Finding{ID: "socket-rw:" + c.Name, Severity: sevHigh, Category: "security", Target: c.Name,
					Title:  "Full access to the Docker socket",
					Detail: "Any process in this container can control the daemon, which is equivalent to root on the host.",
					Hint:   "Front it with a filtering socket proxy. A :ro bind mount does not restrict API calls."})
			}
		}
		for _, p := range c.Ports {
			if !p.Exposed {
				continue
			}
			sev := sevWarn
			if p.Public == 80 || p.Public == 443 {
				sev = sevInfo
			}
			add(Finding{ID: fmt.Sprintf("public-port:%s:%d", c.Name, p.Public), Severity: sev, Category: "security", Target: c.Name,
				Title:  fmt.Sprintf("Port %d published on all interfaces", p.Public),
				Detail: fmt.Sprintf("%s/%d is reachable from any network that can route to this host.", p.Proto, p.Public),
				Hint:   "Bind to 127.0.0.1 or the Tailscale address and front it with the reverse proxy."})
		}
		if strings.HasSuffix(c.Image, ":latest") || !strings.Contains(lastSegment(c.Image), ":") {
			add(Finding{ID: "latest-tag:" + c.Name, Severity: sevInfo, Category: "supply-chain", Target: c.Name,
				Title:  "Floating image tag",
				Detail: c.Image + " does not pin a version, so rebuilds are not reproducible.",
				Hint:   "Pin a version tag or a digest."})
		}
		if c.LogDriver == "json-file" && c.LogMaxSize == "" {
			add(Finding{ID: "unbounded-logs:" + c.Name, Severity: sevWarn, Category: "storage", Target: c.Name,
				Title:  "Log file has no size cap",
				Detail: "json-file logs grow until the disk fills.",
				Hint:   "Set logging options max-size and max-file."})
		}
		if c.Project == "" {
			add(Finding{ID: "no-compose:" + c.Name, Severity: sevInfo, Category: "operations", Target: c.Name,
				Title:  "Not managed by Compose",
				Detail: "This container has no Compose project label, so its definition lives only in the daemon.",
				Hint:   "Move it into a Compose file so it can be recreated."})
		}
		if c.State == "exited" {
			add(Finding{ID: "exited:" + c.Name, Severity: sevInfo, Category: "operations", Target: c.Name,
				Title: "Stopped container still present", Detail: c.Status,
				Hint: "Auto-prune can reap these once they pass the minimum age."})
		}
	}

	for _, f := range ck.undeployedStacks(snap) {
		add(f)
	}

	sort.SliceStable(report.Findings, func(i, j int) bool {
		return severityRank(report.Findings[i].Severity) < severityRank(report.Findings[j].Severity)
	})
	for _, f := range report.Findings {
		switch f.Severity {
		case sevHigh:
			report.Summary.High++
		case sevWarn:
			report.Summary.Warn++
		default:
			report.Summary.Info++
		}
	}
	return report, nil
}

// undeployedStacks finds Compose files on disk with no running project, which
// is the usual sign of a stack that was stopped and forgotten.
func (ck *Checker) undeployedStacks(snap *Snapshot) []Finding {
	if ck.stacksDir == "" {
		return nil
	}
	entries, err := os.ReadDir(ck.stacksDir)
	if err != nil {
		return nil
	}
	deployed := map[string]bool{}
	for _, c := range snap.Containers {
		if c.Project != "" {
			deployed[strings.ToLower(c.Project)] = true
		}
	}
	var out []Finding
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if deployed[strings.ToLower(e.Name())] {
			continue
		}
		var found string
		for _, name := range []string{"compose.yaml", "compose.yml", "docker-compose.yml", "docker-compose.yaml"} {
			p := filepath.Join(ck.stacksDir, e.Name(), name)
			if _, err := os.Stat(p); err == nil {
				found = name
				break
			}
		}
		if found == "" {
			continue
		}
		out = append(out, Finding{
			ID: "undeployed:" + e.Name(), Severity: sevInfo, Category: "operations", Target: e.Name(),
			Title:  "Compose file on disk with nothing running",
			Detail: fmt.Sprintf("%s/%s defines a stack but no container carries that project label.", e.Name(), found),
			Hint:   "Either bring it up or archive the directory.",
		})
	}
	return out
}

func severityRank(s string) int {
	switch s {
	case sevHigh:
		return 0
	case sevWarn:
		return 1
	default:
		return 2
	}
}

func lastSegment(ref string) string {
	if i := strings.LastIndex(ref, "/"); i >= 0 {
		return ref[i+1:]
	}
	return ref
}
