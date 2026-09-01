package main

import (
	"context"
	"errors"
	"path"
	"strings"
	"sync"
	"time"
)

// ErrPruneBusy is returned when a prune is already in flight.
var ErrPruneBusy = errors.New("a prune run is already in progress")

// keepLabel marks any Docker object that must survive every prune.
const keepLabel = "ops-console.keep"

type Pruner struct {
	docker *Docker
	store  *Store
	mu     sync.Mutex
	busy   bool
}

func NewPruner(d *Docker, s *Store) *Pruner {
	return &Pruner{docker: d, store: s}
}

func (p *Pruner) Busy() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.busy
}

// protected reports whether ref is shielded by a keep pattern. Patterns are
// globs; a pattern without wildcards also matches as a substring so that
// "monitoring" protects "monitoring_beszel_data".
func protected(ref string, patterns []string) (bool, string) {
	lower := strings.ToLower(ref)
	for _, raw := range patterns {
		pattern := strings.ToLower(strings.TrimSpace(raw))
		if pattern == "" {
			continue
		}
		if strings.ContainsAny(pattern, "*?[") {
			if ok, err := path.Match(pattern, lower); err == nil && ok {
				return true, raw
			}
			continue
		}
		if strings.Contains(lower, pattern) {
			return true, raw
		}
	}
	return false, ""
}

// attachedNetworks lists networks that still have a container endpoint,
// including stopped containers that would reconnect on start.
func attachedNetworks(containers []Container) map[string]bool {
	out := map[string]bool{}
	for _, c := range containers {
		for name := range c.NetworkSettings.Networks {
			out[name] = true
		}
	}
	return out
}

func hasKeepLabel(labels map[string]string) bool {
	v, ok := labels[keepLabel]
	return ok && v != "false"
}

func (p *Pruner) Run(ctx context.Context, cfg PruneConfig, trigger string, dryRun bool) (PruneRun, error) {
	p.mu.Lock()
	if p.busy {
		p.mu.Unlock()
		return PruneRun{}, ErrPruneBusy
	}
	p.busy = true
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		p.busy = false
		p.mu.Unlock()
	}()

	cfg.Normalize()
	run := PruneRun{
		ID:        newID(),
		Trigger:   trigger,
		DryRun:    dryRun,
		StartedAt: time.Now().UTC(),
		Items:     []PruneItem{},
		Errors:    []string{},
		Targets:   cfg.Targets,
	}
	cutoff := time.Now().Add(-time.Duration(cfg.MinAgeHours) * time.Hour)

	containers, err := p.docker.Containers(ctx)
	if err != nil {
		return PruneRun{}, err
	}

	if cfg.Targets.StoppedContainers {
		p.pruneContainers(ctx, &run, cfg, containers, cutoff, dryRun)
	}
	if cfg.Targets.DanglingImages || cfg.Targets.UnusedImages {
		p.pruneImages(ctx, &run, cfg, containers, cutoff, dryRun)
	}
	if cfg.Targets.Volumes {
		p.pruneVolumes(ctx, &run, cfg, containers, cutoff, dryRun)
	}
	if cfg.Targets.Networks {
		p.pruneNetworks(ctx, &run, cfg, containers, cutoff, dryRun)
	}
	if cfg.Targets.BuildCache {
		p.pruneBuildCache(ctx, &run, cfg, dryRun)
	}

	run.FinishedAt = time.Now().UTC()
	for _, item := range run.Items {
		if item.Removed || dryRun {
			run.Reclaimed += item.Size
		}
	}
	if err := p.store.RecordRun(run); err != nil {
		run.Errors = append(run.Errors, "could not persist run: "+err.Error())
	}
	return run, nil
}

func (p *Pruner) add(run *PruneRun, kind, ref string, size int64, removed bool, reason string) {
	run.Items = append(run.Items, PruneItem{Kind: kind, Ref: ref, Size: size, Removed: removed, Reason: reason})
}

func (p *Pruner) pruneContainers(ctx context.Context, run *PruneRun, cfg PruneConfig, containers []Container, cutoff time.Time, dryRun bool) {
	for _, c := range containers {
		// "created" containers may be waiting to start, so only reap terminal states.
		if c.State != "exited" && c.State != "dead" {
			continue
		}
		if time.Unix(c.Created, 0).After(cutoff) {
			continue
		}
		if hasKeepLabel(c.Labels) {
			continue
		}
		if ok, pattern := protected(c.Name(), cfg.KeepPatterns); ok {
			p.add(run, "container", c.Name(), 0, false, "kept by pattern "+pattern)
			continue
		}
		if dryRun {
			p.add(run, "container", c.Name(), 0, false, "would remove")
			continue
		}
		if err := p.docker.RemoveContainer(ctx, c.ID); err != nil {
			p.add(run, "container", c.Name(), 0, false, err.Error())
			run.Errors = append(run.Errors, err.Error())
			continue
		}
		p.add(run, "container", c.Name(), 0, true, "")
	}
}

func (p *Pruner) pruneImages(ctx context.Context, run *PruneRun, cfg PruneConfig, containers []Container, cutoff time.Time, dryRun bool) {
	images, err := p.docker.Images(ctx)
	if err != nil {
		run.Errors = append(run.Errors, err.Error())
		return
	}
	inUse := map[string]bool{}
	for _, c := range containers {
		inUse[c.ImageID] = true
		inUse[c.Image] = true
	}
	for _, img := range images {
		if inUse[img.ID] || inUse[img.Ref()] {
			continue
		}
		dangling := img.Dangling()
		if dangling && !cfg.Targets.DanglingImages {
			continue
		}
		if !dangling && !cfg.Targets.UnusedImages {
			continue
		}
		if time.Unix(img.Created, 0).After(cutoff) {
			continue
		}
		if hasKeepLabel(img.Labels) {
			continue
		}
		ref := img.Ref()
		if ok, pattern := protected(ref, cfg.KeepPatterns); ok {
			p.add(run, "image", ref, img.Size, false, "kept by pattern "+pattern)
			continue
		}
		if dryRun {
			p.add(run, "image", ref, img.Size, false, "would remove")
			continue
		}
		if err := p.docker.RemoveImage(ctx, img.ID); err != nil {
			p.add(run, "image", ref, img.Size, false, err.Error())
			run.Errors = append(run.Errors, err.Error())
			continue
		}
		p.add(run, "image", ref, img.Size, true, "")
	}
}

func (p *Pruner) pruneVolumes(ctx context.Context, run *PruneRun, cfg PruneConfig, containers []Container, cutoff time.Time, dryRun bool) {
	volumes, err := p.docker.Volumes(ctx)
	if err != nil {
		run.Errors = append(run.Errors, err.Error())
		return
	}
	sizes := map[string]int64{}
	if du, err := p.docker.DiskUsage(ctx); err == nil {
		for _, v := range du.Volumes {
			if v.UsageData != nil {
				sizes[v.Name] = v.UsageData.Size
			}
		}
	}
	attached := map[string]bool{}
	for _, c := range containers {
		for _, m := range c.Mounts {
			if m.Type == "volume" && m.Name != "" {
				attached[m.Name] = true
			}
		}
	}
	for _, v := range volumes {
		if attached[v.Name] {
			continue
		}
		if created, err := time.Parse(time.RFC3339, v.CreatedAt); err == nil && created.After(cutoff) {
			continue
		}
		if hasKeepLabel(v.Labels) {
			continue
		}
		if ok, pattern := protected(v.Name, cfg.KeepPatterns); ok {
			p.add(run, "volume", v.Name, sizes[v.Name], false, "kept by pattern "+pattern)
			continue
		}
		if dryRun {
			p.add(run, "volume", v.Name, sizes[v.Name], false, "would remove")
			continue
		}
		if err := p.docker.RemoveVolume(ctx, v.Name); err != nil {
			p.add(run, "volume", v.Name, sizes[v.Name], false, err.Error())
			run.Errors = append(run.Errors, err.Error())
			continue
		}
		p.add(run, "volume", v.Name, sizes[v.Name], true, "")
	}
}

func (p *Pruner) pruneNetworks(ctx context.Context, run *PruneRun, cfg PruneConfig, containers []Container, cutoff time.Time, dryRun bool) {
	networks, err := p.docker.Networks(ctx)
	if err != nil {
		run.Errors = append(run.Errors, err.Error())
		return
	}
	attached := attachedNetworks(containers)
	for _, n := range networks {
		switch n.Name {
		case "bridge", "host", "none":
			continue
		}
		// The network list endpoint leaves Containers empty, so endpoints have
		// to be derived from the containers themselves.
		if attached[n.Name] || len(n.Containers) > 0 {
			continue
		}
		if created, err := time.Parse(time.RFC3339Nano, n.Created); err == nil && created.After(cutoff) {
			continue
		}
		if hasKeepLabel(n.Labels) {
			continue
		}
		if ok, pattern := protected(n.Name, cfg.KeepPatterns); ok {
			p.add(run, "network", n.Name, 0, false, "kept by pattern "+pattern)
			continue
		}
		if dryRun {
			p.add(run, "network", n.Name, 0, false, "would remove")
			continue
		}
		if err := p.docker.RemoveNetwork(ctx, n.ID); err != nil {
			p.add(run, "network", n.Name, 0, false, err.Error())
			run.Errors = append(run.Errors, err.Error())
			continue
		}
		p.add(run, "network", n.Name, 0, true, "")
	}
}

func (p *Pruner) pruneBuildCache(ctx context.Context, run *PruneRun, cfg PruneConfig, dryRun bool) {
	if dryRun {
		du, err := p.docker.DiskUsage(ctx)
		if err != nil {
			run.Errors = append(run.Errors, err.Error())
			return
		}
		var size int64
		var count int
		for _, c := range du.BuildCache {
			if c.InUse || c.Shared {
				continue
			}
			size += c.Size
			count++
		}
		if count == 0 {
			return
		}
		p.add(run, "build-cache", pluralize(count, "cache record"), size, false, "would remove")
		return
	}
	res, err := p.docker.PruneBuildCache(ctx, false, time.Duration(cfg.MinAgeHours)*time.Hour)
	if err != nil {
		run.Errors = append(run.Errors, err.Error())
		return
	}
	if len(res.CachesDeleted) == 0 && res.SpaceReclaimed == 0 {
		return
	}
	p.add(run, "build-cache", pluralize(len(res.CachesDeleted), "cache record"), res.SpaceReclaimed, true, "")
}
