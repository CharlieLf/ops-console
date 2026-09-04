package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type server struct {
	docker    *Docker
	collector *Collector
	store     *Store
	pruner    *Pruner
	scheduler *Scheduler
	checker   *Checker
	control   *Control
	metrics   *Metrics
	activity  *Activity
	alerts    *Alerts
	staticDir string
	apiKey    string
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func main() {
	dataDir := env("DATA_DIR", "/app/data")
	staticDir := env("STATIC_DIR", "/app/static")
	listen := env("LISTEN", ":7070")
	sock := env("DOCKER_SOCKET", "/var/run/docker.sock")
	stacksDir := env("HOST_STACKS_DIR", "/host/stacks")

	store, err := NewStore(dataDir)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	docker := NewDocker(sock)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	if err := docker.Ping(pingCtx); err != nil {
		log.Printf("warning: cannot reach the Docker socket at %s: %v", sock, err)
	}
	cancel()

	collector := NewCollector(docker, stacksDir)
	pruner := NewPruner(docker, store)
	scheduler := NewScheduler(store, pruner)
	control := NewControl(docker, collector, stacksDir)
	metrics := NewMetrics(docker, env("HOST_PROC", "/host/proc"), dataDir)
	activity := NewActivity(200)
	alerts := NewAlerts(collector, metrics)
	s := &server{
		docker:    docker,
		collector: collector,
		store:     store,
		pruner:    pruner,
		scheduler: scheduler,
		checker:   NewChecker(collector, store, stacksDir, dataDir),
		control:   control,
		metrics:   metrics,
		activity:  activity,
		alerts:    alerts,
		staticDir: staticDir,
		apiKey:    os.Getenv("API_KEY"),
	}

	go scheduler.Run(ctx)
	go metrics.Run(ctx)
	go activity.Run(ctx, docker)

	srv := &http.Server{
		Addr:              listen,
		Handler:           s.routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Printf("ops-console listening on %s (docker socket %s)", listen, sock)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server: %v", err)
	}
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/overview", s.handleOverview)
	mux.HandleFunc("GET /api/stacks", s.handleStacks)
	mux.HandleFunc("GET /api/stacks/{name}", s.handleStackGet)
	mux.HandleFunc("GET /api/stacks/{name}/compose", s.handleComposeGet)
	mux.HandleFunc("PUT /api/stacks/{name}/compose", s.handleComposePut)
	mux.HandleFunc("POST /api/stacks/{name}/{action}", s.handleStackAction)
	mux.HandleFunc("POST /api/containers/{id}/{action}", s.handleContainerAction)
	mux.HandleFunc("GET /api/containers/{id}/logs", s.handleContainerLogs)
	mux.HandleFunc("GET /api/metrics", s.handleMetrics)
	mux.HandleFunc("GET /api/activity", s.handleActivity)
	mux.HandleFunc("GET /api/alerts", s.handleAlerts)
	mux.HandleFunc("GET /api/checks", s.handleChecks)
	mux.HandleFunc("GET /api/resources", s.handleResources)
	mux.HandleFunc("GET /api/prune/config", s.handlePruneConfigGet)
	mux.HandleFunc("PUT /api/prune/config", s.handlePruneConfigPut)
	mux.HandleFunc("GET /api/prune/status", s.handlePruneStatus)
	mux.HandleFunc("POST /api/prune/run", s.handlePruneRun)
	mux.HandleFunc("GET /api/prune/history", s.handlePruneHistory)
	mux.HandleFunc("/", s.handleStatic)
	return s.withAuth(mux)
}

func (s *server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.apiKey == "" || !strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api/health" {
			next.ServeHTTP(w, r)
			return
		}
		key := r.Header.Get("X-API-Key")
		if key == "" {
			key = r.URL.Query().Get("key")
		}
		if key != s.apiKey {
			writeErr(w, http.StatusUnauthorized, "invalid API key")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	err := s.docker.Ping(r.Context())
	out := map[string]any{"ok": err == nil, "time": time.Now().UTC()}
	if err != nil {
		out["docker"] = err.Error()
		writeJSON(w, http.StatusServiceUnavailable, out)
		return
	}
	out["docker"] = "reachable"
	writeJSON(w, http.StatusOK, out)
}

type pruneStatus struct {
	Enabled  bool        `json:"enabled"`
	Running  bool        `json:"running"`
	NextRun  *time.Time  `json:"nextRun"`
	LastRun  *time.Time  `json:"lastRun"`
	LastItem *PruneRun   `json:"lastResult"`
	Config   PruneConfig `json:"config"`
}

func (s *server) pruneStatus() pruneStatus {
	cfg := s.store.Config()
	st := pruneStatus{
		Enabled: cfg.Enabled,
		Running: s.pruner.Busy(),
		NextRun: s.scheduler.NextRun(),
		LastRun: s.store.LastRun(),
		Config:  cfg,
	}
	if history := s.store.History(); len(history) > 0 {
		st.LastItem = &history[0]
	}
	return st
}

func (s *server) handleOverview(w http.ResponseWriter, r *http.Request) {
	snap, err := s.collector.Snapshot(r.Context(), r.URL.Query().Get("refresh") == "1")
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	res, err := s.collector.Resources(r.Context(), s.store.Config().KeepPatterns)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	report, err := s.checker.Run(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	disk, _ := diskStat(env("DATA_DIR", "/app/data"))

	projects := map[string]bool{}
	var running, stopped int
	for _, c := range snap.Containers {
		if c.Project != "" {
			projects[c.Project] = true
		}
		if c.State == "running" {
			running++
		} else {
			stopped++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"takenAt": snap.TakenAt,
		"host": map[string]any{
			"name":          snap.Info.Name,
			"dockerVersion": snap.Info.ServerVersion,
			"os":            snap.Info.OperatingSystem,
			"kernel":        snap.Info.KernelVersion,
			"arch":          snap.Info.Architecture,
			"cpus":          snap.Info.NCPU,
			"memTotal":      snap.Info.MemTotal,
			"storageDriver": snap.Info.Driver,
		},
		"disk": disk,
		"counts": map[string]any{
			"stacks":            len(projects),
			"containersRunning": running,
			"containersStopped": stopped,
			"images":            len(snap.Images),
			"volumes":           len(snap.Volumes),
			"networks":          len(snap.Networks),
		},
		"storage":  res.Totals,
		"checks":   report.Summary,
		"topFindings": firstFindings(report.Findings, 5),
		"prune":    s.pruneStatus(),
	})
}

func firstFindings(f []Finding, n int) []Finding {
	if len(f) > n {
		return f[:n]
	}
	if f == nil {
		return []Finding{}
	}
	return f
}

func (s *server) handleStacks(w http.ResponseWriter, r *http.Request) {
	stacks, err := s.collector.Stacks(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"stacks": enrichStacks(stacks, s.metrics.Latest())})
}

func enrichStacks(stacks []StackView, m MetricsSnapshot) []StackView {
	stats := map[string]ContainerStat{}
	for _, st := range m.Containers {
		stats[st.ID] = st
		stats[st.Name] = st
	}
	out := make([]StackView, len(stacks))
	for i, stack := range stacks {
		containers := make([]ContainerView, len(stack.Containers))
		var cpu, mem float64
		var memBytes int64
		for j, c := range stack.Containers {
			cv := c
			if st, ok := stats[c.ID]; ok {
				cv.CPUPct = st.CPUPct
				cv.MemUsage = st.MemUsage
				cv.MemLimit = st.MemLimit
				cv.MemPct = st.MemPct
				if c.State == "running" {
					cpu += st.CPUPct
					memBytes += st.MemUsage
					if st.MemLimit > 0 {
						mem += st.MemPct
					}
				}
			}
			containers[j] = cv
		}
		stack.Containers = containers
		stack.CPUPct = cpu
		stack.MemUsage = memBytes
		if stack.Running > 0 && mem > 0 {
			stack.MemPct = mem / float64(stack.Running)
		}
		out[i] = stack
	}
	return out
}

func (s *server) handleStackGet(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	stacks, err := s.collector.Stacks(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	for _, stack := range enrichStacks(stacks, s.metrics.Latest()) {
		if sameStack(stack.Name, name) {
			writeJSON(w, http.StatusOK, stack)
			return
		}
	}
	writeErr(w, http.StatusNotFound, "stack not found")
}

func (s *server) handleActivity(w http.ResponseWriter, r *http.Request) {
	n := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 && parsed <= 200 {
			n = parsed
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": s.activity.List(n)})
}

func (s *server) handleAlerts(w http.ResponseWriter, r *http.Request) {
	list, err := s.alerts.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	if list == nil {
		list = []Alert{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"alerts": list})
}

func (s *server) handleComposeGet(w http.ResponseWriter, r *http.Request) {
	path, content, err := s.control.ReadCompose(r.Context(), r.PathValue("name"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": path, "content": content})
}

func (s *server) handleComposePut(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid payload")
		return
	}
	path, err := s.control.WriteCompose(r.Context(), r.PathValue("name"), body.Content)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": path})
}

func (s *server) handleStackAction(w http.ResponseWriter, r *http.Request) {
	action := r.PathValue("action")
	res, err := s.control.StackAction(r.Context(), r.PathValue("name"), action)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error(), "result": res})
		return
	}
	_, _ = s.collector.Snapshot(r.Context(), true)
	writeJSON(w, http.StatusOK, res)
}

func (s *server) handleContainerAction(w http.ResponseWriter, r *http.Request) {
	if err := s.control.ContainerAction(r.Context(), r.PathValue("id"), r.PathValue("action")); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	_, _ = s.collector.Snapshot(r.Context(), true)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *server) handleContainerLogs(w http.ResponseWriter, r *http.Request) {
	tail := 200
	if v := r.URL.Query().Get("tail"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 5000 {
			tail = n
		}
	}
	timestamps := r.URL.Query().Get("timestamps") != "0"
	logs, err := s.docker.ContainerLogs(r.Context(), r.PathValue("id"), tail, timestamps)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"logs": logs})
}

func (s *server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.metrics.Latest())
}

func (s *server) handleChecks(w http.ResponseWriter, r *http.Request) {
	report, err := s.checker.Run(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (s *server) handleResources(w http.ResponseWriter, r *http.Request) {
	res, err := s.collector.Resources(r.Context(), s.store.Config().KeepPatterns)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *server) handlePruneConfigGet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.store.Config())
}

func (s *server) handlePruneConfigPut(w http.ResponseWriter, r *http.Request) {
	var cfg PruneConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid payload: "+err.Error())
		return
	}
	saved, err := s.store.SetConfig(cfg)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"config": saved, "status": s.pruneStatus()})
}

func (s *server) handlePruneStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.pruneStatus())
}

func (s *server) handlePruneRun(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DryRun   bool         `json:"dryRun"`
		Override *PruneConfig `json:"override"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	cfg := s.store.Config()
	if body.Override != nil {
		cfg = *body.Override
		cfg.Normalize()
	}
	trigger := "manual"
	if body.DryRun {
		trigger = "preview"
	}
	// A real prune can outlive the request; give it its own deadline.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	run, err := s.pruner.Run(ctx, cfg, trigger, body.DryRun)
	if err != nil {
		if errors.Is(err, ErrPruneBusy) {
			writeErr(w, http.StatusConflict, err.Error())
			return
		}
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	// Force the next read to reflect what was just removed.
	_, _ = s.collector.Snapshot(ctx, true)
	if !run.DryRun {
		_, _ = s.collector.diskUsage(ctx, true)
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": run, "status": s.pruneStatus()})
}

func (s *server) handlePruneHistory(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"history": s.store.History()})
}

// handleStatic serves the built SPA and falls back to index.html so that
// client-side routes survive a refresh.
func (s *server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeErr(w, http.StatusNotFound, "unknown endpoint")
		return
	}
	clean := filepath.Clean(r.URL.Path)
	target := filepath.Join(s.staticDir, clean)
	if !strings.HasPrefix(target, filepath.Clean(s.staticDir)) {
		http.NotFound(w, r)
		return
	}
	if info, err := os.Stat(target); err == nil && !info.IsDir() {
		http.ServeFile(w, r, target)
		return
	}
	index := filepath.Join(s.staticDir, "index.html")
	if _, err := os.Stat(index); err != nil {
		writeErr(w, http.StatusNotFound, "ui not built")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, index)
}
