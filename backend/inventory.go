package main

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	labelProject     = "com.docker.compose.project"
	labelService     = "com.docker.compose.service"
	labelConfigFiles = "com.docker.compose.project.config_files"
	labelWorkingDir  = "com.docker.compose.project.working_dir"
)

type PortView struct {
	IP      string `json:"ip"`
	Public  int    `json:"public"`
	Private int    `json:"private"`
	Proto   string `json:"proto"`
	Exposed bool   `json:"exposed"`
}

type MountView struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	RW          bool   `json:"rw"`
}

type ContainerView struct {
	ID             string      `json:"id"`
	Name           string      `json:"name"`
	Image          string      `json:"image"`
	Project        string      `json:"project"`
	Service        string      `json:"service"`
	State          string      `json:"state"`
	Status         string      `json:"status"`
	Health         string      `json:"health"`
	HasHealthcheck bool        `json:"hasHealthcheck"`
	RestartPolicy  string      `json:"restartPolicy"`
	RestartCount   int         `json:"restartCount"`
	Privileged     bool        `json:"privileged"`
	DockerSocket   bool        `json:"dockerSocket"`
	LogDriver      string      `json:"logDriver"`
	LogMaxSize     string      `json:"logMaxSize"`
	CreatedAt      time.Time   `json:"createdAt"`
	CPUPct         float64     `json:"cpuPct,omitempty"`
	MemUsage       int64       `json:"memUsage,omitempty"`
	MemLimit       int64       `json:"memLimit,omitempty"`
	MemPct         float64     `json:"memPct,omitempty"`
	Ports          []PortView  `json:"ports"`
	Networks       []string    `json:"networks"`
	Mounts         []MountView `json:"mounts"`
}

type StackView struct {
	Name        string          `json:"name"`
	WorkingDir  string          `json:"workingDir"`
	ConfigFiles []string        `json:"configFiles"`
	Running     int             `json:"running"`
	Total       int             `json:"total"`
	CPUPct      float64         `json:"cpuPct,omitempty"`
	MemUsage    int64           `json:"memUsage,omitempty"`
	MemPct      float64         `json:"memPct,omitempty"`
	Deployed    bool            `json:"deployed"`
	Networks    []string        `json:"networks"`
	Volumes     []string        `json:"volumes"`
	Containers  []ContainerView `json:"containers"`
}

// Snapshot is one consistent view of the daemon, cached briefly so that a page
// with several widgets does not hammer the socket.
type Snapshot struct {
	TakenAt    time.Time       `json:"takenAt"`
	Info       Info            `json:"info"`
	Containers []ContainerView `json:"containers"`
	Images     []Image         `json:"-"`
	Volumes    []Volume        `json:"-"`
	Networks   []Network       `json:"-"`
	raw        []Container
}

type Collector struct {
	docker    *Docker
	stacksDir string

	mu       sync.Mutex
	snap     *Snapshot
	snapAt   time.Time
	du       *DiskUsage
	duAt     time.Time
	snapTTL  time.Duration
	duTTL    time.Duration
}

func NewCollector(d *Docker, stacksDir string) *Collector {
	return &Collector{docker: d, stacksDir: stacksDir, snapTTL: 5 * time.Second, duTTL: 60 * time.Second}
}

func (c *Collector) Snapshot(ctx context.Context, force bool) (*Snapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !force && c.snap != nil && time.Since(c.snapAt) < c.snapTTL {
		return c.snap, nil
	}
	info, err := c.docker.Info(ctx)
	if err != nil {
		return nil, err
	}
	containers, err := c.docker.Containers(ctx)
	if err != nil {
		return nil, err
	}
	images, err := c.docker.Images(ctx)
	if err != nil {
		return nil, err
	}
	volumes, err := c.docker.Volumes(ctx)
	if err != nil {
		return nil, err
	}
	networks, err := c.docker.Networks(ctx)
	if err != nil {
		return nil, err
	}

	views := make([]ContainerView, 0, len(containers))
	for _, ct := range containers {
		views = append(views, c.view(ctx, ct))
	}
	sort.Slice(views, func(i, j int) bool { return views[i].Name < views[j].Name })

	c.snap = &Snapshot{
		TakenAt:    time.Now().UTC(),
		Info:       info,
		Containers: views,
		Images:     images,
		Volumes:    volumes,
		Networks:   networks,
		raw:        containers,
	}
	c.snapAt = time.Now()
	return c.snap, nil
}

func (c *Collector) view(ctx context.Context, ct Container) ContainerView {
	v := ContainerView{
		ID:        ct.ID,
		Name:      ct.Name(),
		Image:     ct.Image,
		Project:   ct.Labels[labelProject],
		Service:   ct.Labels[labelService],
		State:     ct.State,
		Status:    ct.Status,
		CreatedAt: time.Unix(ct.Created, 0).UTC(),
		Ports:     []PortView{},
		Networks:  []string{},
		Mounts:    []MountView{},
	}
	seen := map[string]bool{}
	for _, p := range ct.Ports {
		if p.PublicPort == 0 {
			continue
		}
		exposed := p.IP == "" || p.IP == "0.0.0.0" || p.IP == "::"
		ip := p.IP
		if exposed {
			// A wildcard binding is reported once per address family; collapse
			// those into a single row.
			ip = "0.0.0.0"
		}
		key := ip + ":" + p.Type + ":" + itoa(p.PublicPort) + ":" + itoa(p.PrivatePort)
		if seen[key] {
			continue
		}
		seen[key] = true
		v.Ports = append(v.Ports, PortView{
			IP:      ip,
			Public:  p.PublicPort,
			Private: p.PrivatePort,
			Proto:   p.Type,
			Exposed: exposed,
		})
	}
	for name := range ct.NetworkSettings.Networks {
		v.Networks = append(v.Networks, name)
	}
	sort.Strings(v.Networks)
	for _, m := range ct.Mounts {
		v.Mounts = append(v.Mounts, MountView{Type: m.Type, Name: m.Name, Source: m.Source, Destination: m.Destination, RW: m.RW})
		if strings.HasSuffix(m.Source, "docker.sock") {
			v.DockerSocket = true
		}
	}

	insp, err := c.docker.InspectContainer(ctx, ct.ID)
	if err != nil {
		return v
	}
	v.RestartCount = insp.RestartCount
	v.RestartPolicy = insp.HostConfig.RestartPolicy.Name
	if v.RestartPolicy == "" {
		v.RestartPolicy = "no"
	}
	v.Privileged = insp.HostConfig.Privileged
	v.LogDriver = insp.HostConfig.LogConfig.Type
	v.LogMaxSize = insp.HostConfig.LogConfig.Config["max-size"]
	if insp.Config.Healthcheck != nil && len(insp.Config.Healthcheck.Test) > 0 &&
		!strings.EqualFold(insp.Config.Healthcheck.Test[0], "NONE") {
		v.HasHealthcheck = true
	}
	if insp.State.Health != nil {
		v.Health = insp.State.Health.Status
	}
	if insp.Config.Image != "" {
		v.Image = insp.Config.Image
	}
	return v
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// Stacks groups containers by Compose project. Containers started outside
// Compose are collected under a synthetic "(standalone)" stack.
func (c *Collector) Stacks(ctx context.Context) ([]StackView, error) {
	snap, err := c.Snapshot(ctx, false)
	if err != nil {
		return nil, err
	}
	byProject := map[string]*StackView{}
	for _, cv := range snap.Containers {
		name := cv.Project
		if name == "" {
			name = "(standalone)"
		}
		stack, ok := byProject[name]
		if !ok {
			stack = &StackView{Name: name, ConfigFiles: []string{}, Networks: []string{}, Volumes: []string{}, Containers: []ContainerView{}}
			byProject[name] = stack
		}
		stack.Containers = append(stack.Containers, cv)
		stack.Total++
		stack.Deployed = true
		if cv.State == "running" {
			stack.Running++
		}
	}
	for _, stack := range byProject {
		sortContainers(stack.Containers)
	}
	for _, ct := range snap.raw {
		stack := byProject[ct.Labels[labelProject]]
		if stack == nil {
			continue
		}
		if dir := ct.Labels[labelWorkingDir]; dir != "" {
			stack.WorkingDir = dir
		}
		if files := ct.Labels[labelConfigFiles]; files != "" {
			for _, f := range strings.Split(files, ",") {
				stack.ConfigFiles = appendUnique(stack.ConfigFiles, strings.TrimSpace(f))
			}
		}
	}
	for _, stack := range byProject {
		for _, cv := range stack.Containers {
			for _, n := range cv.Networks {
				stack.Networks = appendUnique(stack.Networks, n)
			}
			for _, m := range cv.Mounts {
				if m.Type == "volume" && m.Name != "" {
					stack.Volumes = appendUnique(stack.Volumes, m.Name)
				}
			}
		}
		sort.Strings(stack.Networks)
		sort.Strings(stack.Volumes)
	}
	for _, disk := range c.discoverDiskStacks() {
		if stackOnDiskAlready(byProject, disk) {
			continue
		}
		byProject[disk.Name] = &disk
	}
	out := make([]StackView, 0, len(byProject))
	for _, s := range byProject {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

var composeFilenames = []string{
	"compose.yaml", "compose.yml", "docker-compose.yml", "docker-compose.yaml",
}

func sortContainers(list []ContainerView) {
	sort.SliceStable(list, func(i, j int) bool {
		rank := func(s string) int {
			switch s {
			case "running":
				return 0
			case "restarting", "paused":
				return 1
			case "exited", "dead", "created":
				return 2
			default:
				return 3
			}
		}
		ri, rj := rank(list[i].State), rank(list[j].State)
		if ri != rj {
			return ri < rj
		}
		return list[i].Name < list[j].Name
	})
}

func stackOnDiskAlready(byProject map[string]*StackView, disk StackView) bool {
	lower := strings.ToLower(disk.Name)
	for _, s := range byProject {
		if strings.ToLower(s.Name) == lower {
			return true
		}
		if disk.WorkingDir != "" && strings.EqualFold(s.WorkingDir, disk.WorkingDir) {
			return true
		}
	}
	return false
}

func (c *Collector) discoverDiskStacks() []StackView {
	if c.stacksDir == "" {
		return nil
	}
	entries, err := os.ReadDir(c.stacksDir)
	if err != nil {
		return nil
	}
	var out []StackView
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(c.stacksDir, e.Name())
		var files []string
		for _, name := range composeFilenames {
			p := filepath.Join(dir, name)
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				files = append(files, p)
			}
		}
		if len(files) == 0 {
			continue
		}
		out = append(out, StackView{
			Name:        e.Name(),
			WorkingDir:  dir,
			ConfigFiles: files,
			Deployed:    false,
			Containers:  []ContainerView{},
			Networks:    []string{},
			Volumes:     []string{},
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func appendUnique(list []string, v string) []string {
	if v == "" {
		return list
	}
	for _, existing := range list {
		if existing == v {
			return list
		}
	}
	return append(list, v)
}

func (c *Collector) diskUsage(ctx context.Context, force bool) (*DiskUsage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !force && c.du != nil && time.Since(c.duAt) < c.duTTL {
		return c.du, nil
	}
	du, err := c.docker.DiskUsage(ctx)
	if err != nil {
		return nil, err
	}
	c.du = &du
	c.duAt = time.Now()
	return c.du, nil
}

type ResourceRow struct {
	Name      string    `json:"name"`
	Ref       string    `json:"ref"`
	Size      int64     `json:"size"`
	InUse     bool      `json:"inUse"`
	CreatedAt time.Time `json:"createdAt"`
	Extra     string    `json:"extra"`
	Protected bool      `json:"protected"`
}

type Resources struct {
	Images   []ResourceRow `json:"images"`
	Volumes  []ResourceRow `json:"volumes"`
	Networks []ResourceRow `json:"networks"`
	Totals   struct {
		ImagesSize        int64 `json:"imagesSize"`
		ImagesReclaimable int64 `json:"imagesReclaimable"`
		VolumesSize       int64 `json:"volumesSize"`
		VolumesReclaim    int64 `json:"volumesReclaimable"`
		BuildCacheSize    int64 `json:"buildCacheSize"`
		BuildCacheReclaim int64 `json:"buildCacheReclaimable"`
	} `json:"totals"`
}

func (c *Collector) Resources(ctx context.Context, keepPatterns []string) (*Resources, error) {
	snap, err := c.Snapshot(ctx, false)
	if err != nil {
		return nil, err
	}
	du, err := c.diskUsage(ctx, false)
	if err != nil {
		return nil, err
	}
	res := &Resources{Images: []ResourceRow{}, Volumes: []ResourceRow{}, Networks: []ResourceRow{}}

	usedImages := map[string]bool{}
	attachedVolumes := map[string]bool{}
	for _, ct := range snap.raw {
		usedImages[ct.ImageID] = true
		usedImages[ct.Image] = true
		for _, m := range ct.Mounts {
			if m.Type == "volume" && m.Name != "" {
				attachedVolumes[m.Name] = true
			}
		}
	}
	for _, img := range snap.Images {
		inUse := usedImages[img.ID] || usedImages[img.Ref()]
		keep, _ := protected(img.Ref(), keepPatterns)
		kind := "tagged"
		if img.Dangling() {
			kind = "dangling"
		}
		res.Images = append(res.Images, ResourceRow{
			Name: img.Ref(), Ref: shortID(img.ID), Size: img.Size, InUse: inUse,
			CreatedAt: time.Unix(img.Created, 0).UTC(), Extra: kind,
			Protected: keep || hasKeepLabel(img.Labels),
		})
		res.Totals.ImagesSize += img.Size
		if !inUse {
			res.Totals.ImagesReclaimable += img.Size
		}
	}
	sizes := map[string]int64{}
	for _, v := range du.Volumes {
		if v.UsageData != nil {
			sizes[v.Name] = v.UsageData.Size
		}
	}
	for _, v := range snap.Volumes {
		inUse := attachedVolumes[v.Name]
		keep, _ := protected(v.Name, keepPatterns)
		created, _ := time.Parse(time.RFC3339, v.CreatedAt)
		res.Volumes = append(res.Volumes, ResourceRow{
			Name: v.Name, Ref: v.Driver, Size: sizes[v.Name], InUse: inUse,
			CreatedAt: created.UTC(), Extra: v.Mountpoint,
			Protected: keep || hasKeepLabel(v.Labels),
		})
		res.Totals.VolumesSize += sizes[v.Name]
		if !inUse {
			res.Totals.VolumesReclaim += sizes[v.Name]
		}
	}
	endpoints := map[string]int{}
	for _, ct := range snap.raw {
		for name := range ct.NetworkSettings.Networks {
			endpoints[name]++
		}
	}
	for _, n := range snap.Networks {
		keep, _ := protected(n.Name, keepPatterns)
		created, _ := time.Parse(time.RFC3339Nano, n.Created)
		predefined := n.Name == "bridge" || n.Name == "host" || n.Name == "none"
		count := endpoints[n.Name]
		if count == 0 {
			count = len(n.Containers)
		}
		res.Networks = append(res.Networks, ResourceRow{
			Name: n.Name, Ref: n.Driver, InUse: count > 0 || predefined,
			CreatedAt: created.UTC(), Extra: pluralize(count, "container"),
			Protected: keep || predefined || hasKeepLabel(n.Labels),
		})
	}
	for _, bc := range du.BuildCache {
		res.Totals.BuildCacheSize += bc.Size
		if !bc.InUse && !bc.Shared {
			res.Totals.BuildCacheReclaim += bc.Size
		}
	}
	sortRowsBySize(res.Images)
	sortRowsBySize(res.Volumes)
	sort.Slice(res.Networks, func(i, j int) bool { return res.Networks[i].Name < res.Networks[j].Name })
	return res, nil
}

func sortRowsBySize(rows []ResourceRow) {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Size != rows[j].Size {
			return rows[i].Size > rows[j].Size
		}
		return rows[i].Name < rows[j].Name
	})
}
