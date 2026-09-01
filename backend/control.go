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
		if s.Name != name {
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
	args := []string{"compose", "-p", name}
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
