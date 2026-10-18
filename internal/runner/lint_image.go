package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/dnlrsls/ci-trust/internal/lintcatalog"
	"github.com/dnlrsls/ci-trust/internal/lintimage"
	"time"
)

var invalidLintImage = errors.New("owned lint image load or observation unverified")

// LoadLintImage is a trusted composition boundary, NOT human authentication.
// Call only after exact authenticated lint review. The compiled gate precedes
// all dependencies; no caller report, policy flag or expected IDs are accepted.
// The untagged image is retained: concurrent loads preclude deletion ownership.
func (d *LocalDocker) LoadLintImage(ctx context.Context, archive, inventory []byte) error {
	if lintcatalog.BlockedReason() != "" {
		return invalidLintImage
	}
	c := lintcatalog.Current()
	return d.loadLintImage(ctx, archive, inventory, lintimage.Identities{ImageID: c.Image, LayerDiffID: c.Layer, ArchiveSHA256: c.Archive})
}

// Private trusted test composition may supply comparison identities, never a
// production admission override. Independent inspection still always runs.
func (d *LocalDocker) loadLintImage(ctx context.Context, archive, inventory []byte, want lintimage.Identities) error {
	if d == nil || ctx == nil || int64(len(archive)) > lintimage.MaxArchiveBytes || len(inventory) > 16<<20 {
		return invalidLintImage
	}
	if e := d.admit(ctx); e != nil {
		return e
	}
	defer d.mu.Unlock()
	if d.host != "linux" || d.client == nil || d.attempted || d.loadAttempted {
		return invalidLintImage
	}
	d.ready = false
	// Inspect and transmit the same privately owned bytes, not mutable caller data.
	archive = bytes.Clone(archive)
	inventory = bytes.Clone(inventory)
	observed, e := lintimage.InspectInventory(bytes.NewReader(archive), bytes.NewReader(inventory), want)
	if e != nil || ctx.Err() != nil || observed.Identities != want || observed.RuntimeAdmitted || observed.Verdict != "INVENTORY_CONSISTENT_NOT_ADMITTED" {
		return invalidLintImage
	}
	loader, ok := d.client.(interface {
		load(context.Context, []byte) (string, error)
	})
	if !ok {
		return invalidLintImage
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if d.checkDaemon(ctx) != nil {
		return invalidLintImage
	}
	d.loadAttempted = true
	if _, e = loader.load(ctx, archive); e != nil || ctx.Err() != nil {
		return invalidLintImage
	}
	s, e := d.client.call(ctx, nil, "image", "inspect", want.ImageID)
	if e != nil || ctx.Err() != nil || !matchesLintImage(s, want) {
		return invalidLintImage
	}
	d.loadVerified = true
	d.loadedImage = want.ImageID
	return nil
}

func matchesLintImage(s string, want lintimage.Identities) bool {
	var rows []struct {
		ID                    string `json:"Id"`
		OS                    string `json:"Os"`
		Architecture, Variant string
		OSVersion             string   `json:"OsVersion"`
		OSFeatures            []string `json:"OsFeatures"`
		RepoTags              []string
		Config                map[string]json.RawMessage
		RootFS                struct {
			Type   string
			Layers []string
		}
	}
	if json.Unmarshal([]byte(s), &rows) != nil || len(rows) != 1 {
		return false
	}
	r := rows[0]
	if r.ID != want.ImageID || r.OS != "linux" || r.Architecture != "amd64" || r.Variant != "" || r.OSVersion != "" || len(r.OSFeatures) != 0 || len(r.RepoTags) != 0 || r.RootFS.Type != "layers" || len(r.RootFS.Layers) != 1 || r.RootFS.Layers[0] != want.LayerDiffID || r.Config == nil {
		return false
	}
	var user, work string
	if json.Unmarshal(r.Config["User"], &user) != nil || user != "65532:65532" || json.Unmarshal(r.Config["WorkingDir"], &work) != nil || work != "/work" {
		return false
	}
	// Separate lint defaults: never relax smoke's empty-config observer.
	for key, raw := range r.Config {
		switch key {
		case "User", "WorkingDir":
		case "Hostname", "Domainname", "Image", "StopSignal":
			var v string
			if string(raw) == "null" || json.Unmarshal(raw, &v) != nil || v != "" {
				return false
			}
		case "AttachStdin", "AttachStdout", "AttachStderr", "Tty", "OpenStdin", "StdinOnce", "ArgsEscaped":
			var v bool
			if string(raw) == "null" || json.Unmarshal(raw, &v) != nil || v {
				return false
			}
		case "Env", "Cmd", "Entrypoint", "OnBuild", "Shell":
			var v []string
			if json.Unmarshal(raw, &v) != nil || len(v) != 0 {
				return false
			}
		case "ExposedPorts", "Volumes", "Labels":
			var v map[string]json.RawMessage
			if json.Unmarshal(raw, &v) != nil || len(v) != 0 {
				return false
			}
		case "Healthcheck", "StopTimeout":
			if string(raw) != "null" {
				return false
			}
		default:
			return false
		}
	}
	return true
}
