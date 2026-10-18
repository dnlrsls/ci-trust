package runner

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dnlrsls/ci-trust/internal/lintimage"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"
)

func lintImageDigest(b []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(b)) }
func lintImageFixture(t *testing.T) ([]byte, []byte, lintimage.Identities) {
	t.Helper()
	var l bytes.Buffer
	w := tar.NewWriter(&l)
	w.Close()
	layer := l.Bytes()
	config, _ := json.Marshal(map[string]any{"architecture": "amd64", "os": "linux", "config": map[string]any{"User": "65532:65532", "WorkingDir": "/work", "Env": []any{}, "Entrypoint": []any{}, "Cmd": []any{}}, "rootfs": map[string]any{"type": "layers", "diff_ids": []string{lintImageDigest(layer)}}, "history": []any{}})
	ids := lintimage.Identities{ImageID: lintImageDigest(config), LayerDiffID: lintImageDigest(layer)}
	manifest, _ := json.Marshal([]any{map[string]any{"Config": ids.ImageID[7:] + ".json", "RepoTags": []any{}, "Layers": []string{"layer.tar"}}})
	var b bytes.Buffer
	w = tar.NewWriter(&b)
	for _, e := range []struct {
		name string
		data []byte
	}{{ids.ImageID[7:] + ".json", config}, {"layer.tar", layer}, {"manifest.json", manifest}} {
		if e := w.WriteHeader(&tar.Header{Name: e.name, Mode: 0444, Size: int64(len(e.data)), Typeflag: tar.TypeReg, ModTime: time.Unix(0, 0), Format: tar.FormatUSTAR}); e != nil {
			t.Fatal(e)
		}
		w.Write(e.data)
	}
	if e := w.Close(); e != nil {
		t.Fatal(e)
	}
	ids.ArchiveSHA256 = lintImageDigest(b.Bytes())
	return b.Bytes(), []byte("[]"), ids
}

type lintLoadClient struct {
	ids    lintimage.Identities
	calls  []string
	loaded []byte
	mode   string
	onInfo func()
}

func (f *lintLoadClient) load(ctx context.Context, b []byte) (string, error) {
	f.calls = append(f.calls, "load")
	f.loaded = bytes.Clone(b)
	if _, ok := ctx.Deadline(); !ok {
		return "", errors.New("unbounded")
	}
	if f.mode == "load-error" {
		return "untrusted success stdout", errors.New("ambiguous load")
	}
	return "untrusted load stdout", nil
}
func (f *lintLoadClient) call(ctx context.Context, w io.Writer, args ...string) (string, error) {
	f.calls = append(f.calls, strings.Join(args, " "))
	if args[0] == "info" {
		if f.onInfo != nil {
			f.onInfo()
		}
		if f.mode == "daemon" {
			return "{}", nil
		}
		return (&ownedClient{}).call(ctx, w, args...)
	}
	if !reflect.DeepEqual(args, []string{"image", "inspect", f.ids.ImageID}) {
		return "", errors.New("unowned operation")
	}
	if f.mode == "inspect-error" {
		return "", errors.New("unverified inspection")
	}
	config := map[string]any{"User": "65532:65532", "WorkingDir": "/work", "Env": []string{}, "Entrypoint": []string{}, "Cmd": []string{}}
	row := map[string]any{"Id": f.ids.ImageID, "Os": "linux", "Architecture": "amd64", "RepoTags": []string{}, "Config": config, "RootFS": map[string]any{"Type": "layers", "Layers": []string{f.ids.LayerDiffID}}}
	switch f.mode {
	case "identity":
		row["Id"] = "sha256:" + strings.Repeat("0", 64)
	case "layer":
		row["RootFS"] = map[string]any{"Type": "layers", "Layers": []string{"sha256:" + strings.Repeat("0", 64)}}
	case "user":
		config["User"] = "root"
	case "workdir":
		config["WorkingDir"] = "/"
	case "shell":
		config["Shell"] = []string{"unowned"}
	case "unknown":
		config["Unowned"] = false
	case "tags":
		row["RepoTags"] = []string{"unowned:tag"}
	case "platform":
		row["Architecture"] = "arm64"
	}
	b, _ := json.Marshal([]any{row})
	return string(b), nil
}
func TestLintImageLoadGateAndOwnedSnapshot(t *testing.T) {
	var nilRuntime *LocalDocker
	if e := nilRuntime.LoadLintImage(nil, nil, nil); e == nil {
		t.Error("compiled gate granted load")
	}
	a, inv, ids := lintImageFixture(t)
	before := bytes.Clone(a)
	f := &lintLoadClient{ids: ids}
	d := &LocalDocker{client: f, host: "linux"}
	f.onInfo = func() { a[0] ^= 1; inv[0] = '!' }
	if e := d.loadLintImage(context.Background(), a, inv, ids); e != nil {
		t.Error("trusted fixture load rejected", e)
	}
	if !bytes.Equal(f.loaded, before) || !reflect.DeepEqual(f.calls, []string{"info --format {{json .}}", "load", "image inspect " + ids.ImageID}) {
		t.Error("load used mutable bytes or unowned operation", f.calls)
	}
	count := len(f.calls)
	if e := d.loadLintImage(context.Background(), before, []byte("[]"), ids); e == nil || len(f.calls) != count {
		t.Error("load retried")
	}
}
func TestLintImageLoadRejectsBeforeEffectAndNeverRetries(t *testing.T) {
	for _, rule := range []string{"archive", "inventory", "host", "used", "cancelled", "daemon", "load-error", "inspect-error", "identity", "layer", "user", "workdir", "shell", "unknown", "tags", "platform"} {
		t.Run(rule, func(t *testing.T) {
			a, inv, ids := lintImageFixture(t)
			f := &lintLoadClient{ids: ids, mode: rule}
			d := &LocalDocker{client: f, host: "linux"}
			ctx := context.Background()
			switch rule {
			case "archive":
				a[0] ^= 1
			case "inventory":
				inv = []byte("{}")
			case "host":
				d.host = "windows"
			case "used":
				d.attempted = true
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if e := d.loadLintImage(ctx, a, inv, ids); e == nil {
				t.Error("unverified load accepted")
			}
			if rule == "archive" || rule == "inventory" || rule == "host" || rule == "used" || rule == "cancelled" {
				if len(f.calls) != 0 {
					t.Error("invalid input reached Docker")
				}
			}
			if rule != "archive" && rule != "inventory" && rule != "host" && rule != "used" && rule != "cancelled" && rule != "daemon" {
				count := len(f.calls)
				f.mode = ""
				if e := d.loadLintImage(context.Background(), a, inv, ids); e == nil || len(f.calls) != count {
					t.Error("ambiguous load retried")
				}
			}
		})
	}
}
