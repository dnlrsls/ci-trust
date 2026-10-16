package runner

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/dnlrsls/ci-trust/internal/ownedimage"
)

// An inert ELF table fixture: packaged and parsed only, never executed.
func imageFixture(t *testing.T) ([]byte, string, string, ownedimage.Metadata) {
	t.Helper()
	b := make([]byte, 120)
	copy(b, "\x7fELF\x02\x01\x01")
	binary.LittleEndian.PutUint16(b[16:], 2)
	binary.LittleEndian.PutUint16(b[18:], 62)
	binary.LittleEndian.PutUint32(b[20:], 1)
	binary.LittleEndian.PutUint64(b[32:], 64)
	binary.LittleEndian.PutUint16(b[52:], 64)
	binary.LittleEndian.PutUint16(b[54:], 56)
	binary.LittleEndian.PutUint16(b[56:], 1)
	binary.LittleEndian.PutUint32(b[64:], 1)
	binary.LittleEndian.PutUint32(b[68:], 5)
	binary.LittleEndian.PutUint64(b[96:], 120)
	binary.LittleEndian.PutUint64(b[104:], 120)
	root := t.TempDir()
	probe, marker := filepath.Join(root, "probe"), filepath.Join(root, "marker")
	if err := os.WriteFile(probe, b, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, []byte(ownedimage.Marker), 0600); err != nil {
		t.Fatal(err)
	}
	a, err := ownedimage.Pack(probe, marker)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := ownedimage.Inspect(a, probe, marker)
	if err != nil {
		t.Fatal(err)
	}
	return a, probe, marker, meta
}

type imageClient struct {
	meta   ownedimage.Metadata
	calls  []string
	loaded []byte
	mode   string
}

func imageInspection(m ownedimage.Metadata, config any) string {
	b, _ := json.Marshal([]any{map[string]any{"Id": m.ImageID, "Os": "linux", "Architecture": "amd64", "Config": config, "RootFS": map[string]any{"Type": "layers", "Layers": []string{m.LayerDiffID}}}})
	return string(b)
}

func (f *imageClient) load(ctx context.Context, a []byte) (string, error) {
	f.calls = append(f.calls, "load")
	f.loaded = bytes.Clone(a)
	if _, ok := ctx.Deadline(); !ok {
		return "", errors.New("unbounded load")
	}
	if f.mode == "load-failure" {
		return "looks successful", errors.New("ambiguous transport")
	}
	if f.mode == "cancel" {
		<-ctx.Done()
		return "", ctx.Err()
	}
	return "arbitrary load stdout is not evidence", nil
}

func (f *imageClient) call(ctx context.Context, _ io.Writer, args ...string) (string, error) {
	f.calls = append(f.calls, strings.Join(args, " "))
	if args[0] == "info" {
		if f.mode == "daemon" {
			return `{}`, nil
		}
		return (&ownedClient{}).call(ctx, nil, args...)
	}
	if !reflect.DeepEqual(args, []string{"image", "inspect", f.meta.ImageID}) {
		return "", errors.New("unexpected image command")
	}
	if f.mode == "inspect-error" {
		return "", errors.New("inspect unavailable")
	}
	m := f.meta
	if f.mode == "identity" {
		m.ImageID = "sha256:" + strings.Repeat("0", 64)
	}
	if f.mode == "layer" {
		m.LayerDiffID = "sha256:" + strings.Repeat("0", 64)
	}
	config := map[string]any{}
	if f.mode == "config" {
		config["Shell"] = []string{"unreviewed"}
	}
	return imageInspection(m, config), nil
}

func TestLoadOwnedImageRejectsUnverifiedInputsBeforeDocker(t *testing.T) {
	for _, name := range []string{"archive", "binary", "marker", "metadata", "host", "used", "context"} {
		t.Run(name, func(t *testing.T) {
			a, binary, marker, meta := imageFixture(t)
			f := &imageClient{meta: meta}
			d := &LocalDocker{client: f, host: "linux"}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch name {
			case "archive":
				a[0] ^= 1
			case "binary":
				binary = filepath.Join(t.TempDir(), "absent")
			case "marker":
				marker = binary
			case "metadata":
				meta.BinarySHA256 = "sha256:" + strings.Repeat("0", 64)
			case "host":
				d.host = "windows"
			case "used":
				d.attempted = true
			case "context":
				cancel()
			}
			if err := d.LoadOwnedImage(ctx, a, binary, marker, meta); err == nil || len(f.calls) != 0 {
				t.Fatalf("invalid input reached Docker: calls=%v err=%v", f.calls, err)
			}
		})
	}
}

func TestLoadOwnedImageObservesAfterLoadAndNeverDeletes(t *testing.T) {
	for _, mode := range []string{"success", "daemon", "load-failure", "inspect-error", "identity", "layer", "config", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			a, binary, marker, meta := imageFixture(t)
			f := &imageClient{meta: meta, mode: mode}
			d := &LocalDocker{client: f, host: "linux"}
			ctx := context.Background()
			if mode == "cancel" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 30*time.Millisecond)
				defer cancel()
			}
			err := d.LoadOwnedImage(ctx, a, binary, marker, meta)
			if (err == nil) != (mode == "success") {
				t.Fatalf("calls=%v err=%v", f.calls, err)
			}
			want := 3
			if mode == "daemon" {
				want = 1
			}
			if mode == "load-failure" || mode == "cancel" {
				want = 2
			}
			if len(f.calls) != want {
				t.Fatalf("calls=%v", f.calls)
			}
			if mode != "daemon" && !bytes.Equal(a, f.loaded) {
				t.Fatal("load bytes changed")
			}
			if mode == "load-failure" || mode == "inspect-error" || mode == "identity" || mode == "layer" || mode == "config" || mode == "cancel" {
				before := len(f.calls)
				if err := d.Preflight(context.Background(), owned()); err == nil || len(f.calls) != before {
					t.Fatal("failed load bypassed through preflight")
				}
			}
			if mode == "success" {
				before := len(f.calls)
				if err := d.Preflight(context.Background(), owned()); err == nil || len(f.calls) != before {
					t.Fatal("loaded identity rebound to another plan")
				}
				if err := d.LoadOwnedImage(ctx, a, binary, marker, meta); err == nil || len(f.calls) != before {
					t.Fatal("load retried")
				}
			}
		})
	}
}

func TestOwnedImageInspectionRejectsUnexpectedDefaults(t *testing.T) {
	_, _, _, m := imageFixture(t)
	defaults := map[string]any{"User": "", "WorkingDir": "", "Env": nil, "Cmd": []string{}, "Entrypoint": nil, "Volumes": nil, "Healthcheck": nil, "AttachStdout": false}
	if !matchesOwnedImage(imageInspection(m, defaults), m) {
		t.Fatal("empty daemon defaults rejected")
	}
	for key, value := range map[string]any{"User": "root", "WorkingDir": "/other", "Env": []string{"TOKEN=fixture"}, "Shell": []string{"/bin/sh"}, "OnBuild": []string{"RUN anything"}, "Healthcheck": map[string]any{}, "StopTimeout": 0, "Unknown": nil, "Tty": true, "Labels": map[string]string{"foreign": "value"}} {
		t.Run(key, func(t *testing.T) {
			if matchesOwnedImage(imageInspection(m, map[string]any{key: value}), m) {
				t.Fatal("unexpected default admitted")
			}
		})
	}
	for _, value := range []string{"[]", "null", "{}", strings.Replace(imageInspection(m, map[string]any{}), `"amd64"`, `"arm64"`, 1), strings.Replace(imageInspection(m, map[string]any{}), `"linux"`, `"windows"`, 1)} {
		if matchesOwnedImage(value, m) {
			t.Fatal("wrong daemon image accepted")
		}
	}
}

func TestOwnedExecutableImageLoadHasBoundedStdinAndCombinedOutput(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	c := &executableDocker{executable, t.TempDir()}
	for _, mode := range []string{"owned-load-streams", "owned-load-delay", "owned-load-overflow", "owned-load-failure", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			input := mode
			if mode == "cancel" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 100*time.Millisecond)
				defer cancel()
				input = "owned-load-delay"
			}
			start := time.Now()
			s, err := c.load(ctx, []byte(input))
			if mode == "owned-load-delay" && (time.Since(start) < 2200*time.Millisecond || time.Since(start) > 4*time.Second) {
				t.Fatalf("load wall=%v", time.Since(start))
			}
			if mode == "cancel" && time.Since(start) > time.Second {
				t.Fatal("caller deadline ignored")
			}
			if mode == "owned-load-streams" || mode == "owned-load-delay" {
				if err != nil || !strings.Contains(s, "owned load stdout\n") || !strings.Contains(s, "owned load stderr\n") {
					t.Fatalf("streams=%q err=%v", s, err)
				}
			} else if err == nil || len(s) > 64<<10 {
				t.Fatalf("bytes=%d err=%v", len(s), err)
			}
		})
	}
}
