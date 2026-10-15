package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

type dockerClient interface {
	call(context.Context, io.Writer, ...string) (string, error)
}

// LocalDocker implements only an existing local Linux Docker daemon. Construct it
// in trusted application composition, never from target or agent arguments.
// Metadata checks are prerequisites, not independent enforcement proof.
type LocalDocker struct {
	mu              sync.Mutex
	client          dockerClient
	host            string
	plan            Plan
	ready           bool
	name, id, owner string
	attempted       bool
}

// NewLocalDocker requires a trusted absolute executable and an existing empty
// private client-config directory. It performs no runtime access or provisioning.
// The caller must keep both paths immutable and private throughout execution.
func NewLocalDocker(executable, config string) (*LocalDocker, error) {
	if !filepath.IsAbs(executable) || !filepath.IsAbs(config) {
		return nil, errors.New("absolute trusted paths required")
	}
	fi, err := os.Lstat(config)
	if err != nil || !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 || fi.Mode().Perm()&0077 != 0 {
		return nil, errors.New("private config directory required")
	}
	entries, err := os.ReadDir(config)
	if err != nil || len(entries) != 0 {
		return nil, errors.New("empty config required")
	}
	return &LocalDocker{client: &executableDocker{executable, config}, host: runtime.GOOS}, nil
}

type executableDocker struct{ path, config string }

// Both metadata streams share one allocation bound. Tool streams instead go to
// Runner's synchronized output bound. Every client has a deadline and WaitDelay.
func (c *executableDocker) call(ctx context.Context, output io.Writer, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	argv := append([]string{"--config", c.config, "--host", "unix:///var/run/docker.sock"}, args...)
	cmd := exec.CommandContext(ctx, c.path, argv...)
	cmd.Env = []string{"HOME=" + c.config, "DOCKER_CONFIG=" + c.config, "LANG=C", "LC_ALL=C"}
	cmd.WaitDelay = 200 * time.Millisecond
	b := &boundedOutput{max: 64 << 10, cancel: cancel}
	if output == nil {
		cmd.Stdout = b
		cmd.Stderr = b
	} else {
		cmd.Stdout = output
		cmd.Stderr = output
	}
	err := cmd.Run()
	s, overflow := b.state()
	if overflow {
		return s, errors.New("metadata output limit")
	}
	if ctx.Err() != nil {
		return s, ctx.Err()
	}
	return s, err
}

// admit rejects concurrent operations immediately rather than waiting behind a
// client call and losing the caller's cancellation deadline.
func (d *LocalDocker) admit(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !d.mu.TryLock() {
		return errors.New("runtime operation already in progress")
	}
	if err := ctx.Err(); err != nil {
		d.mu.Unlock()
		return err
	}
	return nil
}

func (d *LocalDocker) Preflight(ctx context.Context, p Plan) error {
	if err := d.admit(ctx); err != nil {
		return err
	}
	defer d.mu.Unlock()
	if d.attempted {
		return errors.New("runtime already used; construct a fresh instance")
	}
	d.ready = false
	if d.host != "linux" || d.client == nil || !valid(p) {
		return errors.New("unsupported local Linux scope or plan")
	}
	var info struct {
		OSType, CgroupVersion                          string
		MemoryLimit, SwapLimit, PidsLimit, CpuCfsQuota bool
		SecurityOptions, Warnings                      []string
	}
	s, err := d.client.call(ctx, nil, "info", "--format", "{{json .}}")
	if err != nil {
		return fmt.Errorf("daemon metadata: %w", err)
	}
	if json.Unmarshal([]byte(s), &info) != nil || info.OSType != "linux" || info.CgroupVersion != "2" || !info.MemoryLimit || !info.SwapLimit || !info.PidsLimit || !info.CpuCfsQuota || len(info.Warnings) != 0 {
		return errors.New("required limit capabilities unavailable")
	}
	seccomp := false
	for _, v := range info.SecurityOptions {
		if v == "name=seccomp,profile=builtin" {
			seccomp = true
		}
	}
	if !seccomp {
		return errors.New("default seccomp unavailable")
	}
	var images []struct {
		ID     string `json:"Id"`
		OS     string `json:"Os"`
		Config struct {
			Volumes              map[string]json.RawMessage
			Env, Entrypoint, Cmd []string
			Healthcheck          json.RawMessage
		}
	}
	s, err = d.client.call(ctx, nil, "image", "inspect", p.Image)
	if err != nil {
		return fmt.Errorf("local immutable image unavailable: %w", err)
	}
	if json.Unmarshal([]byte(s), &images) != nil || len(images) != 1 || images[0].ID != p.Image || images[0].OS != "linux" || len(images[0].Config.Volumes) != 0 || len(images[0].Config.Env) != 0 || len(images[0].Config.Entrypoint) != 0 || len(images[0].Config.Cmd) != 0 || (len(images[0].Config.Healthcheck) != 0 && string(images[0].Config.Healthcheck) != "null") {
		return errors.New("unsupported image defaults or identity")
	}
	d.plan = p
	d.ready = true
	return nil
}

// Intermediate cut: execution and removal remain unavailable until the complete
// ownership lifecycle and its safety tests land together. No client call occurs.
func (d *LocalDocker) Start(context.Context, string, []string, io.Writer) (int, error) {
	return -1, errors.New("owned lifecycle unavailable in intermediate snapshot")
}
func (d *LocalDocker) Remove(context.Context, string) error {
	return errors.New("owned lifecycle unavailable in intermediate snapshot")
}
