package main

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublicProcessHelper(t *testing.T) {
	if os.Getenv("OWNED_IMAGE_TEST_HELPER") != "1" {
		return
	}
	for i, a := range os.Args {
		if a == "--" {
			os.Args = append([]string{"owned-image"}, os.Args[i+1:]...)
			main()
			return
		}
	}
	os.Exit(99)
}
func process(t *testing.T, args ...string) (int, []byte, string) {
	t.Helper()
	cmd := exec.Command(os.Args[0], append([]string{"-test.run=^TestPublicProcessHelper$", "--"}, args...)...)
	cmd.Env = append(os.Environ(), "OWNED_IMAGE_TEST_HELPER=1")
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	e := cmd.Run()
	code := 0
	if e != nil {
		var exit *exec.ExitError
		if !errors.As(e, &exit) {
			t.Fatal(e)
		}
		code = exit.ExitCode()
	}
	return code, out.Bytes(), stderr.String()
}
func TestPublicProcess(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "binary")
	m := filepath.Join(d, "marker")
	sentinel := filepath.Join(d, "sentinel")
	b := make([]byte, 120)
	copy(b, "\x7fELF")
	b[4] = 2
	b[5] = 1
	b[6] = 1
	binary.LittleEndian.PutUint16(b[16:], 2)
	binary.LittleEndian.PutUint16(b[18:], 62)
	binary.LittleEndian.PutUint32(b[20:], 1)
	binary.LittleEndian.PutUint64(b[32:], 64)
	binary.LittleEndian.PutUint16(b[52:], 64)
	binary.LittleEndian.PutUint16(b[54:], 56)
	binary.LittleEndian.PutUint16(b[56:], 1)
	binary.LittleEndian.PutUint32(b[64:], 1)
	binary.LittleEndian.PutUint64(b[96:], 120)
	binary.LittleEndian.PutUint64(b[104:], 120)
	initial := map[string][]byte{p: b, m: []byte("ci-trust owned-smoke-v1 root marker\n"), sentinel: []byte("unchanged")}
	for p, b := range initial {
		if e := os.WriteFile(p, b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	code, a, stderr := process(t, "pack", "--binary", p, "--marker", m)
	if code != 0 || stderr != "" || len(a) == 0 {
		t.Fatalf("pack: %d %q", code, stderr)
	}
	code, again, stderr := process(t, "pack", "--marker", m, "--binary", p)
	if code != 0 || stderr != "" || !bytes.Equal(a, again) {
		t.Fatal("nondeterministic public pack")
	}
	ap := filepath.Join(d, "archive")
	if e := os.WriteFile(ap, a, 0600); e != nil {
		t.Fatal(e)
	}
	initial[ap] = a
	code, out, stderr := process(t, "inspect", "--archive", ap, "--binary", p, "--marker", m)
	var got map[string]string
	if code != 0 || stderr != "" || json.Unmarshal(out, &got) != nil || len(got) != 6 || got["scope"] != "preparation-only" || out[len(out)-1] != '\n' || bytes.Contains(out, []byte{'\r'}) {
		t.Fatalf("inspect: %d %q %q", code, out, stderr)
	}
	sum := func(b []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(b)) }
	var imageID, layerID string
	tr := tar.NewReader(bytes.NewReader(a))
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			t.Fatal(e)
		}
		raw, e := io.ReadAll(tr)
		if e != nil {
			t.Fatal(e)
		}
		if h.Name == "layer.tar" {
			layerID = sum(raw)
		} else if strings.HasSuffix(h.Name, ".json") && h.Name != "manifest.json" {
			imageID = sum(raw)
		}
	}
	expected := fmt.Sprintf("{\"image_id\":%q,\"archive_sha256\":%q,\"binary_sha256\":%q,\"marker_sha256\":%q,\"layer_diff_id\":%q,\"scope\":\"preparation-only\"}\n", imageID, sum(a), sum(b), sum(initial[m]), layerID)
	if string(out) != expected {
		t.Fatalf("metadata: got %q want %q", out, expected)
	}
	for _, tt := range []struct {
		args     []string
		code     int
		out, err string
	}{{[]string{"--help"}, 0, usage, ""}, {[]string{"inspect", "--help"}, 0, usage, ""}, {[]string{"unknown"}, 2, "", usage}, {[]string{"pack", "--binary", p, "--binary", p, "--marker", m}, 2, "", usage}, {[]string{"pack", "--binary", sentinel, "--marker", m}, 1, "", "owned-image: invalid input\n"}, {[]string{"pack", "--binary", d, "--marker", m}, 1, "", "owned-image: invalid input\n"}, {[]string{"inspect", "--archive", sentinel, "--binary", p, "--marker", m}, 1, "", "owned-image: invalid input\n"}} {
		code, out, stderr := process(t, tt.args...)
		if code != tt.code || string(out) != tt.out || stderr != tt.err {
			t.Fatalf("%v: %d %q %q", tt.args, code, out, stderr)
		}
		entries, e := os.ReadDir(d)
		if e != nil || len(entries) != len(initial) {
			t.Fatal("inventory changed", e)
		}
		for p, b := range initial {
			after, e := os.ReadFile(p)
			if e != nil || !bytes.Equal(after, b) {
				t.Fatal("input changed", p, e)
			}
		}
	}
}

func TestOutputFailure(t *testing.T) {
	p, m, a := commandInputs(t)
	for _, args := range [][]string{
		{"--help"}, {"pack", "--help"}, {"inspect", "--help"},
		{"pack", "--binary", p, "--marker", m},
		{"inspect", "--archive", a, "--binary", p, "--marker", m},
	} {
		t.Run(args[0]+strings.Join(args[1:], "_"), func(t *testing.T) {
			var stderr bytes.Buffer
			out := &recordFailWriter{}
			code := run(args, out, &stderr)
			if code != 1 || out.accepted.Len() != 0 || stderr.String() != "owned-image: output failed\n" {
				t.Fatalf("got status=%d stdout=%q stderr=%q", code, out.accepted.Bytes(), stderr.String())
			}
		})
	}
}

type recordFailWriter struct{ accepted bytes.Buffer }

func (*recordFailWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

// commandInputs uses inert invented ELF bytes, never executable probe code.
func commandInputs(t *testing.T) (string, string, string) {
	t.Helper()
	d := t.TempDir()
	p, m, a := filepath.Join(d, "binary"), filepath.Join(d, "marker"), filepath.Join(d, "archive")
	b := make([]byte, 120)
	copy(b, "\x7fELF")
	b[4], b[5], b[6] = 2, 1, 1
	binary.LittleEndian.PutUint16(b[16:], 2)
	binary.LittleEndian.PutUint16(b[18:], 62)
	binary.LittleEndian.PutUint32(b[20:], 1)
	binary.LittleEndian.PutUint64(b[32:], 64)
	binary.LittleEndian.PutUint16(b[52:], 64)
	binary.LittleEndian.PutUint16(b[54:], 56)
	binary.LittleEndian.PutUint16(b[56:], 1)
	binary.LittleEndian.PutUint32(b[64:], 1)
	binary.LittleEndian.PutUint64(b[96:], 120)
	binary.LittleEndian.PutUint64(b[104:], 120)
	for path, data := range map[string][]byte{p: b, m: []byte("ci-trust owned-smoke-v1 root marker\n")} {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	code, archive, stderr := process(t, "pack", "--binary", p, "--marker", m)
	if code != 0 || stderr != "" {
		t.Fatalf("fixture pack: %d %q", code, stderr)
	}
	if err := os.WriteFile(a, archive, 0600); err != nil {
		t.Fatal(err)
	}
	return p, m, a
}

func TestPublicProcessSymlinkRejection(t *testing.T) {
	for _, input := range []string{"binary", "marker", "archive"} {
		t.Run(input, func(t *testing.T) {
			p, m, a := commandInputs(t)
			d := filepath.Dir(p)
			counter := filepath.Join(d, "counter")
			if err := os.WriteFile(counter, []byte("0\n"), 0600); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(d, "input-link")
			target := map[string]string{"binary": p, "marker": m, "archive": a}[input]
			if err := os.Symlink(target, link); err != nil {
				t.Fatalf("filesystem symlink proof unavailable: %v", err)
			}
			before := map[string][]byte{}
			for _, path := range []string{p, m, a, counter} {
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				before[path] = data
			}
			inventory, err := os.ReadDir(d)
			if err != nil {
				t.Fatal(err)
			}
			args := []string{"pack", "--binary", p, "--marker", m}
			switch input {
			case "binary":
				args[2] = link
			case "marker":
				args[4] = link
			case "archive":
				args = []string{"inspect", "--archive", link, "--binary", p, "--marker", m}
			}
			code, stdout, stderr := process(t, args...)
			if code != 1 || len(stdout) != 0 || stderr != "owned-image: invalid input\n" {
				t.Fatalf("got status=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
			afterInventory, err := os.ReadDir(d)
			if err != nil || len(afterInventory) != len(inventory) {
				t.Fatal("inventory changed", err)
			}
			for i := range inventory {
				if inventory[i].Name() != afterInventory[i].Name() {
					t.Fatal("inventory names changed")
				}
			}
			for path, want := range before {
				got, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(got, want) {
					t.Fatal("target/counter changed", path, err)
				}
			}
			info, err := os.Lstat(link)
			if err != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Fatal("link replaced", err)
			}
			gotTarget, err := os.Readlink(link)
			if err != nil || gotTarget != target {
				t.Fatal("link target changed", err)
			}
		})
	}
}
