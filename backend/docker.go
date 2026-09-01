package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Docker talks to the Engine API over the unix socket. The official SDK is a
// heavy dependency for the handful of endpoints this tool needs, so requests
// are issued directly against the socket instead.
type Docker struct {
	http *http.Client
	sock string
}

func NewDocker(sock string) *Docker {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	return &Docker{
		sock: sock,
		http: &http.Client{
			Timeout: 120 * time.Second,
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return dialer.DialContext(ctx, "unix", sock)
				},
			},
		},
	}
}

func (d *Docker) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://docker"+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := d.http.Do(req)
	if err != nil {
		return fmt.Errorf("docker %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		msg := strings.TrimSpace(string(payload))
		var apiErr struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(payload, &apiErr) == nil && apiErr.Message != "" {
			msg = apiErr.Message
		}
		return fmt.Errorf("docker %s %s: %s: %s", method, path, resp.Status, msg)
	}
	if out == nil || len(payload) == 0 {
		return nil
	}
	return json.Unmarshal(payload, out)
}

func filterQuery(filters map[string][]string) string {
	if len(filters) == 0 {
		return ""
	}
	buf, err := json.Marshal(filters)
	if err != nil {
		return ""
	}
	return "filters=" + url.QueryEscape(string(buf))
}

type Port struct {
	IP          string `json:"IP"`
	PrivatePort int    `json:"PrivatePort"`
	PublicPort  int    `json:"PublicPort"`
	Type        string `json:"Type"`
}

type Mount struct {
	Type        string `json:"Type"`
	Name        string `json:"Name"`
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
	RW          bool   `json:"RW"`
}

type Container struct {
	ID              string            `json:"Id"`
	Names           []string          `json:"Names"`
	Image           string            `json:"Image"`
	ImageID         string            `json:"ImageID"`
	State           string            `json:"State"`
	Status          string            `json:"Status"`
	Created         int64             `json:"Created"`
	Labels          map[string]string `json:"Labels"`
	Ports           []Port            `json:"Ports"`
	Mounts          []Mount           `json:"Mounts"`
	NetworkSettings struct {
		Networks map[string]struct {
			IPAddress string `json:"IPAddress"`
		} `json:"Networks"`
	} `json:"NetworkSettings"`
}

func (c Container) Name() string {
	if len(c.Names) == 0 {
		return c.ID
	}
	return strings.TrimPrefix(c.Names[0], "/")
}

type ContainerInspect struct {
	ID    string `json:"Id"`
	Name  string `json:"Name"`
	State struct {
		Status     string `json:"Status"`
		Running    bool   `json:"Running"`
		StartedAt  string `json:"StartedAt"`
		FinishedAt string `json:"FinishedAt"`
		ExitCode   int    `json:"ExitCode"`
		Health     *struct {
			Status       string `json:"Status"`
			FailingStreak int   `json:"FailingStreak"`
		} `json:"Health"`
	} `json:"State"`
	RestartCount int `json:"RestartCount"`
	HostConfig   struct {
		Privileged    bool     `json:"Privileged"`
		NetworkMode   string   `json:"NetworkMode"`
		Binds         []string `json:"Binds"`
		RestartPolicy struct {
			Name string `json:"Name"`
		} `json:"RestartPolicy"`
		Memory    int64 `json:"Memory"`
		LogConfig struct {
			Type   string            `json:"Type"`
			Config map[string]string `json:"Config"`
		} `json:"LogConfig"`
	} `json:"HostConfig"`
	Config struct {
		Image       string            `json:"Image"`
		Labels      map[string]string `json:"Labels"`
		Healthcheck *struct {
			Test []string `json:"Test"`
		} `json:"Healthcheck"`
	} `json:"Config"`
	Mounts []Mount `json:"Mounts"`
}

type Image struct {
	ID          string            `json:"Id"`
	RepoTags    []string          `json:"RepoTags"`
	RepoDigests []string          `json:"RepoDigests"`
	Created     int64             `json:"Created"`
	Size        int64             `json:"Size"`
	Labels      map[string]string `json:"Labels"`
	Containers  int64             `json:"Containers"`
}

func (i Image) Dangling() bool {
	for _, t := range i.RepoTags {
		if t != "" && t != "<none>:<none>" {
			return false
		}
	}
	return true
}

func (i Image) Ref() string {
	for _, t := range i.RepoTags {
		if t != "" && t != "<none>:<none>" {
			return t
		}
	}
	return shortID(i.ID)
}

type Volume struct {
	Name       string            `json:"Name"`
	Driver     string            `json:"Driver"`
	Mountpoint string            `json:"Mountpoint"`
	CreatedAt  string            `json:"CreatedAt"`
	Labels     map[string]string `json:"Labels"`
	UsageData  *struct {
		Size     int64 `json:"Size"`
		RefCount int64 `json:"RefCount"`
	} `json:"UsageData"`
}

type Network struct {
	Name       string            `json:"Name"`
	ID         string            `json:"Id"`
	Created    string            `json:"Created"`
	Driver     string            `json:"Driver"`
	Internal   bool              `json:"Internal"`
	Labels     map[string]string `json:"Labels"`
	Containers map[string]struct {
		Name string `json:"Name"`
	} `json:"Containers"`
}

type DiskUsage struct {
	LayersSize int64      `json:"LayersSize"`
	Images     []Image    `json:"Images"`
	Volumes    []Volume   `json:"Volumes"`
	BuildCache []struct {
		ID     string `json:"ID"`
		Type   string `json:"Type"`
		Size   int64  `json:"Size"`
		InUse  bool   `json:"InUse"`
		Shared bool   `json:"Shared"`
	} `json:"BuildCache"`
}

type Info struct {
	Name              string `json:"Name"`
	ServerVersion     string `json:"ServerVersion"`
	OperatingSystem   string `json:"OperatingSystem"`
	KernelVersion     string `json:"KernelVersion"`
	Architecture      string `json:"Architecture"`
	NCPU              int    `json:"NCPU"`
	MemTotal          int64  `json:"MemTotal"`
	Containers        int    `json:"Containers"`
	ContainersRunning int    `json:"ContainersRunning"`
	ContainersStopped int    `json:"ContainersStopped"`
	ContainersPaused  int    `json:"ContainersPaused"`
	Images            int    `json:"Images"`
	Driver            string `json:"Driver"`
}

func (d *Docker) Info(ctx context.Context) (Info, error) {
	var out Info
	err := d.do(ctx, http.MethodGet, "/info", nil, &out)
	return out, err
}

func (d *Docker) Ping(ctx context.Context) error {
	return d.do(ctx, http.MethodGet, "/_ping", nil, nil)
}

func (d *Docker) Containers(ctx context.Context) ([]Container, error) {
	var out []Container
	err := d.do(ctx, http.MethodGet, "/containers/json?all=true", nil, &out)
	return out, err
}

func (d *Docker) InspectContainer(ctx context.Context, id string) (ContainerInspect, error) {
	var out ContainerInspect
	err := d.do(ctx, http.MethodGet, "/containers/"+id+"/json", nil, &out)
	return out, err
}

func (d *Docker) Images(ctx context.Context) ([]Image, error) {
	var out []Image
	err := d.do(ctx, http.MethodGet, "/images/json?all=false", nil, &out)
	return out, err
}

func (d *Docker) Volumes(ctx context.Context) ([]Volume, error) {
	var out struct {
		Volumes []Volume `json:"Volumes"`
	}
	err := d.do(ctx, http.MethodGet, "/volumes", nil, &out)
	return out.Volumes, err
}

func (d *Docker) Networks(ctx context.Context) ([]Network, error) {
	var out []Network
	err := d.do(ctx, http.MethodGet, "/networks", nil, &out)
	return out, err
}

func (d *Docker) DiskUsage(ctx context.Context) (DiskUsage, error) {
	var out DiskUsage
	err := d.do(ctx, http.MethodGet, "/system/df", nil, &out)
	return out, err
}

type containersPruneResult struct {
	ContainersDeleted []string `json:"ContainersDeleted"`
	SpaceReclaimed    int64    `json:"SpaceReclaimed"`
}

func (d *Docker) PruneContainers(ctx context.Context, filters map[string][]string) (containersPruneResult, error) {
	var out containersPruneResult
	err := d.do(ctx, http.MethodPost, "/containers/prune?"+filterQuery(filters), nil, &out)
	return out, err
}

type networksPruneResult struct {
	NetworksDeleted []string `json:"NetworksDeleted"`
}

func (d *Docker) PruneNetworks(ctx context.Context, filters map[string][]string) (networksPruneResult, error) {
	var out networksPruneResult
	err := d.do(ctx, http.MethodPost, "/networks/prune?"+filterQuery(filters), nil, &out)
	return out, err
}

type buildPruneResult struct {
	CachesDeleted  []string `json:"CachesDeleted"`
	SpaceReclaimed int64    `json:"SpaceReclaimed"`
}

func (d *Docker) PruneBuildCache(ctx context.Context, all bool, until time.Duration) (buildPruneResult, error) {
	q := url.Values{}
	if all {
		q.Set("all", "true")
	}
	if until > 0 {
		q.Set("filters", fmt.Sprintf(`{"until":[%q]}`, fmt.Sprintf("%dh", int(until.Hours()))))
	}
	var out buildPruneResult
	err := d.do(ctx, http.MethodPost, "/build/prune?"+q.Encode(), nil, &out)
	return out, err
}

func (d *Docker) RemoveContainer(ctx context.Context, id string) error {
	return d.do(ctx, http.MethodDelete, "/containers/"+id+"?v=false&force=false", nil, nil)
}

func (d *Docker) RemoveNetwork(ctx context.Context, id string) error {
	return d.do(ctx, http.MethodDelete, "/networks/"+id, nil, nil)
}

func (d *Docker) RemoveImage(ctx context.Context, id string) error {
	return d.do(ctx, http.MethodDelete, "/images/"+id+"?force=false&noprune=false", nil, nil)
}

func (d *Docker) RemoveVolume(ctx context.Context, name string) error {
	return d.do(ctx, http.MethodDelete, "/volumes/"+url.PathEscape(name), nil, nil)
}

func shortID(id string) string {
	id = strings.TrimPrefix(id, "sha256:")
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func (d *Docker) StartContainer(ctx context.Context, id string) error {
	return d.do(ctx, http.MethodPost, "/containers/"+id+"/start", nil, nil)
}

func (d *Docker) StopContainer(ctx context.Context, id string, timeoutSec int) error {
	if timeoutSec <= 0 {
		timeoutSec = 10
	}
	return d.do(ctx, http.MethodPost, fmt.Sprintf("/containers/%s/stop?t=%d", id, timeoutSec), nil, nil)
}

func (d *Docker) RestartContainer(ctx context.Context, id string, timeoutSec int) error {
	if timeoutSec <= 0 {
		timeoutSec = 10
	}
	return d.do(ctx, http.MethodPost, fmt.Sprintf("/containers/%s/restart?t=%d", id, timeoutSec), nil, nil)
}

// ContainerLogs returns the last n lines of stdout/stderr (no follow).
func (d *Docker) ContainerLogs(ctx context.Context, id string, tail int) (string, error) {
	if tail <= 0 {
		tail = 200
	}
	path := fmt.Sprintf("/containers/%s/logs?stdout=1&stderr=1&timestamps=1&tail=%d", id, tail)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker"+path, nil)
	if err != nil {
		return "", err
	}
	resp, err := d.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("docker logs: %s: %s", resp.Status, strings.TrimSpace(string(payload)))
	}
	return decodeDockerLogs(payload), nil
}

// Docker multiplexes stdout/stderr as 8-byte headers when TTY is off.
func decodeDockerLogs(raw []byte) string {
	var b strings.Builder
	i := 0
	for i+8 <= len(raw) {
		size := int(raw[i+4])<<24 | int(raw[i+5])<<16 | int(raw[i+6])<<8 | int(raw[i+7])
		i += 8
		if size < 0 || i+size > len(raw) {
			b.Write(raw[i:])
			break
		}
		b.Write(raw[i : i+size])
		i += size
	}
	if b.Len() == 0 {
		return string(raw)
	}
	return b.String()
}

type containerStatsJSON struct {
	CPUStats struct {
		CPUUsage struct {
			TotalUsage  uint64   `json:"total_usage"`
			PercpuUsage []uint64 `json:"percpu_usage"`
		} `json:"cpu_usage"`
		SystemUsage uint64 `json:"system_cpu_usage"`
		OnlineCPUs  uint32 `json:"online_cpus"`
	} `json:"cpu_stats"`
	PreCPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemUsage uint64 `json:"system_cpu_usage"`
	} `json:"precpu_stats"`
	MemoryStats struct {
		Usage uint64 `json:"usage"`
		Limit uint64 `json:"limit"`
		Stats struct {
			Cache        uint64 `json:"cache"`
			InactiveFile uint64 `json:"inactive_file"`
		} `json:"stats"`
	} `json:"memory_stats"`
	Name     string `json:"name"`
	ID       string `json:"id"`
	NumProcs uint64 `json:"num_procs"`
}

type ContainerStat struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	CPUPct  float64 `json:"cpuPct"`
	MemUsage int64  `json:"memUsage"`
	MemLimit int64  `json:"memLimit"`
	MemPct  float64 `json:"memPct"`
}

func (d *Docker) ContainerStats(ctx context.Context, id string) (ContainerStat, error) {
	var raw containerStatsJSON
	err := d.do(ctx, http.MethodGet, "/containers/"+id+"/stats?stream=0&one-shot=1", nil, &raw)
	if err != nil {
		return ContainerStat{}, err
	}
	return parseContainerStat(raw), nil
}

func parseContainerStat(raw containerStatsJSON) ContainerStat {
	st := ContainerStat{
		ID:   raw.ID,
		Name: strings.TrimPrefix(raw.Name, "/"),
	}
	cpuDelta := float64(raw.CPUStats.CPUUsage.TotalUsage - raw.PreCPUStats.CPUUsage.TotalUsage)
	sysDelta := float64(raw.CPUStats.SystemUsage - raw.PreCPUStats.SystemUsage)
	online := float64(raw.CPUStats.OnlineCPUs)
	if online == 0 {
		online = float64(len(raw.CPUStats.CPUUsage.PercpuUsage))
	}
	if online == 0 {
		online = 1
	}
	if sysDelta > 0 && cpuDelta >= 0 {
		st.CPUPct = (cpuDelta / sysDelta) * online * 100
	}
	// Prefer working set (usage - inactive_file / cache) when available.
	mem := int64(raw.MemoryStats.Usage)
	if raw.MemoryStats.Stats.InactiveFile > 0 && raw.MemoryStats.Stats.InactiveFile < raw.MemoryStats.Usage {
		mem = int64(raw.MemoryStats.Usage - raw.MemoryStats.Stats.InactiveFile)
	} else if raw.MemoryStats.Stats.Cache > 0 && raw.MemoryStats.Stats.Cache < raw.MemoryStats.Usage {
		mem = int64(raw.MemoryStats.Usage - raw.MemoryStats.Stats.Cache)
	}
	st.MemUsage = mem
	st.MemLimit = int64(raw.MemoryStats.Limit)
	if st.MemLimit > 0 {
		st.MemPct = float64(st.MemUsage) / float64(st.MemLimit) * 100
	}
	return st
}
