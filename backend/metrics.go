package main

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	metricsHistory = 120 // ~30 min at 15s
	metricsEvery   = 15 * time.Second
	statsWorkers   = 6
)

type HostSample struct {
	At      time.Time `json:"at"`
	CPUPct  float64   `json:"cpuPct"`
	MemUsed int64     `json:"memUsed"`
	MemTotal int64    `json:"memTotal"`
	MemPct  float64   `json:"memPct"`
}

type MetricsSnapshot struct {
	TakenAt    time.Time       `json:"takenAt"`
	Host       HostSample      `json:"host"`
	History    []HostSample    `json:"history"`
	Containers []ContainerStat `json:"containers"`
	Disk       DiskStat        `json:"disk"`
}

type Metrics struct {
	docker   *Docker
	hostProc string
	dataDir  string

	mu      sync.Mutex
	last    MetricsSnapshot
	history []HostSample

	prevCPU totalCPU
	haveCPU bool
}

type totalCPU struct {
	idle  uint64
	total uint64
}

func NewMetrics(d *Docker, hostProc, dataDir string) *Metrics {
	return &Metrics{
		docker:   d,
		hostProc: hostProc,
		dataDir:  dataDir,
		history:  make([]HostSample, 0, metricsHistory),
	}
}

func (m *Metrics) Run(ctx context.Context) {
	m.sample(ctx)
	t := time.NewTicker(metricsEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.sample(ctx)
		}
	}
}

func (m *Metrics) Latest() MetricsSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := m.last
	out.History = append([]HostSample(nil), m.history...)
	return out
}

func (m *Metrics) sample(ctx context.Context) {
	host := m.readHost()
	disk, _ := diskStat(m.dataDir)
	containers := m.collectStats(ctx)

	m.mu.Lock()
	defer m.mu.Unlock()
	m.history = append(m.history, host)
	if len(m.history) > metricsHistory {
		m.history = m.history[len(m.history)-metricsHistory:]
	}
	m.last = MetricsSnapshot{
		TakenAt:    time.Now().UTC(),
		Host:       host,
		Containers: containers,
		Disk:       disk,
	}
}

func (m *Metrics) collectStats(ctx context.Context) []ContainerStat {
	list, err := m.docker.Containers(ctx)
	if err != nil {
		return nil
	}
	type item struct {
		id   string
		name string
	}
	var running []item
	for _, c := range list {
		if c.State == "running" {
			running = append(running, item{id: c.ID, name: c.Name()})
		}
	}
	if len(running) == 0 {
		return []ContainerStat{}
	}
	out := make([]ContainerStat, 0, len(running))
	var mu sync.Mutex
	sem := make(chan struct{}, statsWorkers)
	var wg sync.WaitGroup
	for _, it := range running {
		wg.Add(1)
		sem <- struct{}{}
		go func(it item) {
			defer wg.Done()
			defer func() { <-sem }()
			st, err := m.docker.ContainerStats(ctx, it.id)
			if err != nil {
				return
			}
			st.ID = it.id
			st.Name = it.name
			mu.Lock()
			out = append(out, st)
			mu.Unlock()
		}(it)
	}
	wg.Wait()
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].MemUsage > out[i].MemUsage {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func (m *Metrics) readHost() HostSample {
	s := HostSample{At: time.Now().UTC()}
	memTotal, memAvail := readMeminfo(filepath.Join(m.hostProc, "meminfo"))
	if memTotal == 0 {
		if info, err := m.docker.Info(context.Background()); err == nil {
			memTotal = info.MemTotal
		}
	}
	s.MemTotal = memTotal
	if memAvail > 0 && memTotal >= memAvail {
		s.MemUsed = memTotal - memAvail
	}
	if s.MemTotal > 0 {
		s.MemPct = float64(s.MemUsed) / float64(s.MemTotal) * 100
	}
	cur, ok := readCPU(filepath.Join(m.hostProc, "stat"))
	if ok && m.haveCPU {
		idleDelta := float64(cur.idle - m.prevCPU.idle)
		totalDelta := float64(cur.total - m.prevCPU.total)
		if totalDelta > 0 {
			s.CPUPct = (1 - idleDelta/totalDelta) * 100
			if s.CPUPct < 0 {
				s.CPUPct = 0
			}
		}
	}
	if ok {
		m.prevCPU = cur
		m.haveCPU = true
	}
	return s
}

func readMeminfo(path string) (total, available int64) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		v, _ := strconv.ParseInt(fields[1], 10, 64)
		v *= 1024 // kB -> bytes
		switch fields[0] {
		case "MemTotal:":
			total = v
		case "MemAvailable:":
			available = v
		}
	}
	return total, available
}

func readCPU(path string) (totalCPU, bool) {
	f, err := os.Open(path)
	if err != nil {
		return totalCPU{}, false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return totalCPU{}, false
	}
	fields := strings.Fields(sc.Text())
	if len(fields) < 5 || fields[0] != "cpu" {
		return totalCPU{}, false
	}
	var nums []uint64
	for _, f := range fields[1:] {
		n, err := strconv.ParseUint(f, 10, 64)
		if err != nil {
			return totalCPU{}, false
		}
		nums = append(nums, n)
	}
	var sum uint64
	for _, n := range nums {
		sum += n
	}
	idle := nums[3]
	if len(nums) > 4 {
		idle += nums[4] // iowait
	}
	return totalCPU{idle: idle, total: sum}, true
}
