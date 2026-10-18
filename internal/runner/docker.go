package runner

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dnlrsls/ci-trust/internal/lintcatalog"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
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
	loadAttempted   bool
	loadVerified    bool
	loadedImage     string
	oomEvent        *bool
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
	limit := 2 * time.Second
	// Only the fixed attach operation carries tool execution. All metadata,
	// creation and cleanup calls retain the independent two-second cap.
	if len(args) == 3 && args[0] == "start" && args[1] == "--attach" && containerID.MatchString(args[2]) {
		limit = time.Minute
	}
	return c.invoke(ctx, output, nil, limit, args...)
}

// load has no pathname, registry, platform or caller-provided argv. Stdin carries
// only independently inspected archive bytes, never an inherited input stream.
func (c *executableDocker) load(ctx context.Context, archive []byte) (string, error) {
	return c.invoke(ctx, nil, bytes.NewReader(archive), 20*time.Second, "image", "load", "--quiet")
}

func (c *executableDocker) invoke(ctx context.Context, output io.Writer, input io.Reader, limit time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	b := &boundedOutput{max: 64 << 10, cancel: cancel}
	if output != nil {
		err := c.runProcess(ctx, input, output, output, args...)
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", err
	}
	err := c.runProcess(ctx, input, b, b, args...)
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
	if validLint(p) && lintcatalog.BlockedReason() != "" {
		return errors.New("compiled lint runtime is not admitted")
	}
	if err := d.admit(ctx); err != nil {
		return err
	}
	defer d.mu.Unlock()
	if d.attempted {
		return errors.New("runtime already used; construct a fresh instance")
	}
	d.ready = false
	if d.loadAttempted && (!d.loadVerified || p.Image != d.loadedImage) {
		return errors.New("owned image load unverified or plan image changed")
	}
	if d.host != "linux" || d.client == nil || (!valid(p) && !validLint(p)) {
		return errors.New("unsupported local Linux scope or plan")
	}
	if p.Transport == splitStdin {
		if _, ok := d.client.(splitDockerClient); !ok {
			return errors.New("split container transport unavailable")
		}
	}
	if err := d.checkDaemon(ctx); err != nil {
		return err
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
	s, err := d.client.call(ctx, nil, "image", "inspect", p.Image)
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

func (d *LocalDocker) checkDaemon(ctx context.Context) error {
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
	return nil
}

var containerID = regexp.MustCompile(`^[a-f0-9]{64}$`)
var containerName = regexp.MustCompile(`^ci-trust-[0-9]+-[0-9]+$`)

type dockerInspection struct {
	ID     string `json:"Id"`
	Image  string
	Config struct {
		Labels         map[string]string
		Tty, OpenStdin *bool
	}
	State struct {
		Status, StartedAt string
		Error             *string
		ExitCode          *int
		OOMKilled         *bool
	}
}

func (d *LocalDocker) inspect(ctx context.Context, id string) (dockerInspection, error) {
	var rows []dockerInspection
	s, err := d.client.call(ctx, nil, "inspect", id)
	if err != nil {
		return dockerInspection{}, err
	}
	if json.Unmarshal([]byte(s), &rows) != nil || len(rows) != 1 || rows[0].ID != id || rows[0].Image != d.plan.Image || rows[0].Config.Labels["ci-trust.owner"] != d.owner {
		return dockerInspection{}, errors.New("container identity or ownership unverified")
	}
	return rows[0], nil
}
func (d *LocalDocker) Start(ctx context.Context, name string, args []string, w io.Writer) (int, error) {
	return d.start(ctx, name, args, w, "", nil)
}

func (d *LocalDocker) startCaptured(ctx context.Context, name string, args []string, input string) (capturedRun, error) {
	c := capturedRun{ExitCode: -1}
	code, err := d.start(ctx, name, args, nil, input, &c)
	c.ExitCode = code
	return c, err
}

// start shares the ownership, inspection and cleanup authority path with legacy
// smoke. Captured mode is explicitly plan-bound, not selected by arbitrary input.
func (d *LocalDocker) start(ctx context.Context, name string, args []string, w io.Writer, input string, capture *capturedRun) (int, error) {
	if err := d.admit(ctx); err != nil {
		return -1, err
	}
	defer d.mu.Unlock()
	if !d.ready || d.attempted || !containerName.MatchString(name) || !reflect.DeepEqual(args, dockerArgs(d.plan, name)) {
		return -1, errors.New("unowned start vector")
	}
	var split splitDockerClient
	if capture != nil {
		var ok bool
		split, ok = d.client.(splitDockerClient)
		if !ok || d.plan.Transport != splitStdin || input != "" || (!valid(d.plan) && !validLint(d.plan)) {
			return -1, errors.New("unowned or unavailable split transport")
		}
	} else if d.plan.Transport != "" || w == nil {
		return -1, errors.New("legacy transport required")
	}
	// Direct callers receive the same owned wall bound as Runner.Execute;
	// WithTimeout also preserves an earlier caller cancellation deadline.
	ctx, cancel := context.WithTimeout(ctx, d.plan.Limits.Wall)
	defer cancel()
	d.attempted = true
	d.name = name
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return -1, err
	}
	d.owner = hex.EncodeToString(token)
	create := append([]string{"create", "--log-driver=none", "--ipc=none", "--label", "ci-trust.owner=" + d.owner}, args[1:]...)
	s, err := d.client.call(ctx, nil, create...)
	if err != nil {
		return -1, fmt.Errorf("creation ambiguous; no removal authority: %w", err)
	}
	id := strings.TrimSpace(s)
	if !containerID.MatchString(id) {
		return -1, errors.New("creation ID unavailable")
	}
	created, err := d.inspect(ctx, id)
	if err != nil {
		return -1, err
	}
	d.id = id // Removal authority exists only after exact ID, image and secret label match.
	if capture != nil && (created.Config.Tty == nil || *created.Config.Tty || created.Config.OpenStdin == nil || !*created.Config.OpenStdin) {
		return -1, errors.New("split container input or terminal state unverified")
	}
	var proofOutput *boundedOutput
	if d.plan.Config == "owned-proof-v1" {
		proofOutput = &boundedOutput{max: d.plan.Limits.OutputBytes, cancel: cancel}
		w = io.MultiWriter(w, proofOutput)
	}
	var attachErr error
	if capture != nil {
		capture.Stdout, capture.Stderr, attachErr = split.attach(ctx, id, input, d.plan.Limits.OutputBytes, d.plan.Limits.Wall)
	} else {
		_, attachErr = d.client.call(ctx, w, "start", "--attach", id)
	}
	if ctx.Err() != nil {
		return -1, ctx.Err()
	}
	row, err := d.inspect(ctx, id)
	if err != nil {
		return -1, err
	}
	if row.State.OOMKilled != nil {
		event := *row.State.OOMKilled
		d.oomEvent = &event
	}
	started, err := time.Parse(time.RFC3339Nano, row.State.StartedAt)
	if err != nil || started.IsZero() || row.State.Status != "exited" || row.State.Error == nil || *row.State.Error != "" || row.State.OOMKilled == nil || row.State.ExitCode == nil || *row.State.ExitCode < 0 || *row.State.ExitCode > 255 {
		return -1, errors.New("startup, resource or infrastructure failure")
	}
	if *row.State.OOMKilled {
		// The native proof deliberately tests an owned child's cgroup ceiling.
		// Never infer survival from OOMKilled alone or relax legacy behavior.
		if proofOutput == nil || *row.State.ExitCode != 0 || attachErr != nil {
			return -1, errors.New("startup, resource or infrastructure failure")
		}
		output, overflow := proofOutput.state()
		if overflow || !completedOwnedProof(output) {
			return -1, errors.New("owned parent completion after OOM event unverified")
		}
	}
	if attachErr != nil {
		// Matching client/container statuses cannot prove complete transport.
		// Preserve the independently inspected exit without claiming completion.
		return *row.State.ExitCode, fmt.Errorf("attach completion unverified: %w", attachErr)
	}
	return *row.State.ExitCode, nil
}

// OOMEvent returns the independently inspected cgroup OOM flag, not a claim
// about which process was killed. Nil means no final flag was observed.
func (d *LocalDocker) OOMEvent() *bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.oomEvent == nil {
		return nil
	}
	value := *d.oomEvent
	return &value
}

func (d *LocalDocker) Remove(ctx context.Context, name string) error {
	if err := d.admit(ctx); err != nil {
		return err
	}
	defer d.mu.Unlock()
	if !d.attempted || name != d.name || d.id == "" {
		return errors.New("creation or ownership ambiguous; no removal authority")
	}
	if _, err := d.inspect(ctx, d.id); err != nil {
		return err
	}
	_, removeErr := d.client.call(ctx, nil, "rm", "--force", d.id)
	// A successful exact-ID listing is required; generic inspect errors never mean absence.
	s, err := d.client.call(ctx, nil, "ps", "--all", "--no-trunc", "--filter", "id="+d.id, "--format", "{{.ID}}")
	if err != nil {
		return fmt.Errorf("absence unverified: %w", err)
	}
	if len(bytes.TrimSpace([]byte(s))) != 0 {
		return errors.New("owned container still present")
	}
	if removeErr != nil {
		return fmt.Errorf("removal reported infrastructure failure despite absence: %w", removeErr)
	}
	return nil
}
