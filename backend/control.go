package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Control runs stack/container actions and reads/writes compose files under the
// host stacks directory. Compose CLI talks to the same docker.sock.
type Control struct {
	docker    *Docker
	collector *Collector
	stacksDir string
	compose   string
}

func NewControl(d *Docker, collector *Collector, stacksDir string) *Control {
	bin := "docker"
	if p, err := exec.LookPath("docker"); err == nil {
		bin = p
	}
	return &Control{docker: d, collector: collector, stacksDir: filepath.Clean(stacksDir), compose: bin}
}

// composeProjectName makes a name legal for `docker compose -p`.
// Docker requires lowercase alphanumerics, hyphens, underscores, starting with a letter or number.
func composeProjectName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		}
	}
	out := b.String()
	if out == "" {
		return "stack"
	}
	if out[0] == '-' || out[0] == '_' {
		out = "p" + out
	}
	return out
}

func sameStack(a, b string) bool {
	return composeProjectName(a) == composeProjectName(b)
}

// resolveStackDir finds a stack folder when the compose project name differs
// from the directory name (e.g. project "scaleo" vs folder "Scaleo").
func resolveStackDir(stacksDir, name string) string {
	entries, err := os.ReadDir(stacksDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() && strings.EqualFold(e.Name(), name) {
			return filepath.Join(stacksDir, e.Name())
		}
	}
	return ""
}

func (c *Control) resolveStack(ctx context.Context, name string) (workingDir string, files []string, err error) {
	stacks, err := c.collector.Stacks(ctx)
	if err != nil {
		return "", nil, err
	}
	for _, s := range stacks {
		if !sameStack(s.Name, name) {
			continue
		}
		files = append([]string{}, s.ConfigFiles...)
		workingDir = s.WorkingDir
		if workingDir == "" && len(files) > 0 {
			workingDir = filepath.Dir(files[0])
		}
		if len(files) > 0 {
			return workingDir, files, nil
		}
		break
	}
	dir := filepath.Join(c.stacksDir, name)
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		if resolved := resolveStackDir(c.stacksDir, name); resolved != "" {
			dir = resolved
		}
	}
	candidates := []string{
		filepath.Join(dir, "docker-compose.yml"),
		filepath.Join(dir, "docker-compose.yaml"),
		filepath.Join(dir, "compose.yml"),
		filepath.Join(dir, "compose.yaml"),
	}
	for _, f := range candidates {
		if st, e := os.Stat(f); e == nil && !st.IsDir() {
			return dir, []string{f}, nil
		}
	}
	return "", nil, fmt.Errorf("stack %q not found", name)
}

func (c *Control) underStacks(path string) (string, error) {
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) {
		clean = filepath.Join(c.stacksDir, clean)
	}
	rel, err := filepath.Rel(c.stacksDir, clean)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path outside stacks dir")
	}
	return clean, nil
}

func (c *Control) ReadCompose(ctx context.Context, stack string) (path string, content string, err error) {
	_, files, err := c.resolveStack(ctx, stack)
	if err != nil {
		return "", "", err
	}
	if len(files) == 0 {
		return "", "", fmt.Errorf("no compose file for stack %q", stack)
	}
	path, err = c.underStacks(files[0])
	if err != nil {
		return "", "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	return path, string(data), nil
}

func (c *Control) WriteCompose(ctx context.Context, stack, content string) (string, error) {
	path, _, err := c.ReadCompose(ctx, stack)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("compose content empty")
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return path, nil
}

type ActionResult struct {
	OK      bool   `json:"ok"`
	Command string `json:"command"`
	Output  string `json:"output"`
}

func (c *Control) StackAction(ctx context.Context, name, action string) (ActionResult, error) {
	dir, files, err := c.resolveStack(ctx, name)
	if err != nil {
		return ActionResult{}, err
	}
	if len(files) == 0 {
		return ActionResult{}, fmt.Errorf("no compose file for stack %q", name)
	}
	args := []string{"compose"}
	// Include the mobile profile on down even if the flag is off, so flipping
	// DEPLOY_MOBILE=false then Down still stops the web container.
	if action == "down" || stackWantsMobile(dir) {
		args = append(args, "--profile", "mobile")
	}
	args = append(args, "-p", composeProjectName(name))
	for _, f := range files {
		args = append(args, "-f", f)
	}
	switch action {
	case "up":
		args = append(args, "up", "-d", "--remove-orphans")
	case "down":
		args = append(args, "down", "--remove-orphans")
	case "restart":
		args = append(args, "restart")
	case "rebuild":
		args = append(args, "up", "-d", "--build", "--remove-orphans")
	case "pull":
		args = append(args, "pull")
	default:
		return ActionResult{}, fmt.Errorf("unknown action %q", action)
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.compose, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "DOCKER_HOST=unix:///var/run/docker.sock")
	if stackWantsMobile(dir) {
		cmd.Env = append(cmd.Env, "COMPOSE_PROFILES=mobile")
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	runErr := cmd.Run()
	out := strings.TrimSpace(buf.String())
	res := ActionResult{OK: runErr == nil, Command: "docker " + strings.Join(args, " "), Output: out}
	if runErr != nil {
		if out == "" {
			out = runErr.Error()
		}
		res.Output = out
		return res, fmt.Errorf("%s", out)
	}
	return res, nil
}

func (c *Control) ContainerAction(ctx context.Context, id, action string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("container id required")
	}
	switch action {
	case "start":
		return c.docker.StartContainer(ctx, id)
	case "stop":
		return c.docker.StopContainer(ctx, id, 10)
	case "restart":
		return c.docker.RestartContainer(ctx, id, 10)
	default:
		return fmt.Errorf("unknown action %q", action)
	}
}

// stackWantsMobile reads the stack's .env. DEPLOY_MOBILE=true (or COMPOSE_PROFILES
// containing "mobile") means Up/Rebuild should start the Expo web service.
func stackWantsMobile(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		switch key {
		case "DEPLOY_MOBILE":
			switch strings.ToLower(val) {
			case "1", "true", "yes", "on", "mobile":
				return true
			}
		case "COMPOSE_PROFILES":
			for _, p := range strings.Split(val, ",") {
				if strings.TrimSpace(p) == "mobile" {
					return true
				}
			}
		}
	}
	return false
}
