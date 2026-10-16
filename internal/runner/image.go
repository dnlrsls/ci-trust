package runner

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/dnlrsls/ci-trust/internal/ownedimage"
)

var invalidOwnedImage = errors.New("owned image load or observation unverified")

// LoadOwnedImage is a trusted composition boundary, NOT human authentication.
// Call it only after an authenticated exact-plan decision. It independently
// inspects the bounded archive against owned binary/marker bytes before Docker
// access, loads through stdin, then observes immutable identity and empty config.
// Load output alone never attests success. A failed/ambiguous load is not retried.
//
// The untagged content-addressed image is retained. A prior absence observation
// cannot prove ownership against concurrent loads; this API deliberately grants
// no image deletion authority. Container removal remains exact-ID/owner-bound.
func (d *LocalDocker) LoadOwnedImage(ctx context.Context, archive []byte, binary, marker string, expected ownedimage.Metadata) error {
	if err := d.admit(ctx); err != nil {
		return err
	}
	defer d.mu.Unlock()
	if d.host != "linux" || d.client == nil || d.attempted || d.loadAttempted {
		return invalidOwnedImage
	}
	d.ready = false
	observed, err := ownedimage.Inspect(archive, binary, marker)
	if err != nil || observed != expected {
		return invalidOwnedImage
	}
	loader, ok := d.client.(interface {
		load(context.Context, []byte) (string, error)
	})
	if !ok {
		return invalidOwnedImage
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := d.checkDaemon(ctx); err != nil {
		return invalidOwnedImage
	}
	d.loadAttempted = true
	if _, err := loader.load(ctx, archive); err != nil || ctx.Err() != nil {
		return invalidOwnedImage
	}
	s, err := d.client.call(ctx, nil, "image", "inspect", expected.ImageID)
	if err != nil || ctx.Err() != nil || !matchesOwnedImage(s, expected) {
		return invalidOwnedImage
	}
	d.loadVerified = true
	d.loadedImage = expected.ImageID
	return nil
}

func matchesOwnedImage(s string, expected ownedimage.Metadata) bool {
	var rows []struct {
		ID           string `json:"Id"`
		OS           string `json:"Os"`
		Architecture string
		Variant      string
		OSVersion    string   `json:"OsVersion"`
		OSFeatures   []string `json:"OsFeatures"`
		RepoTags     []string
		Config       map[string]json.RawMessage
		RootFS       struct {
			Type   string
			Layers []string
		}
	}
	if json.Unmarshal([]byte(s), &rows) != nil || len(rows) != 1 {
		return false
	}
	r := rows[0]
	if r.ID != expected.ImageID || r.OS != "linux" || r.Architecture != "amd64" || r.Variant != "" || r.OSVersion != "" || len(r.OSFeatures) != 0 || len(r.RepoTags) != 0 || r.RootFS.Type != "layers" || len(r.RootFS.Layers) != 1 || r.RootFS.Layers[0] != expected.LayerDiffID || r.Config == nil {
		return false
	}
	// Docker versions may materialize default empty fields omitted in the
	// reviewed {} config. Admit only known, correctly typed, empty defaults.
	// In particular no healthcheck, shell, onbuild, user, working dir or env
	// may silently become part of execution. Unknown fields fail closed.
	for key, raw := range r.Config {
		switch key {
		case "Hostname", "Domainname", "User", "Image", "WorkingDir", "StopSignal":
			var value string
			if string(raw) == "null" || json.Unmarshal(raw, &value) != nil || value != "" {
				return false
			}
		case "AttachStdin", "AttachStdout", "AttachStderr", "Tty", "OpenStdin", "StdinOnce", "ArgsEscaped":
			var value bool
			if string(raw) == "null" || json.Unmarshal(raw, &value) != nil || value {
				return false
			}
		case "Env", "Cmd", "Entrypoint", "OnBuild", "Shell":
			var value []string
			if json.Unmarshal(raw, &value) != nil || len(value) != 0 {
				return false
			}
		case "ExposedPorts", "Volumes", "Labels":
			var value map[string]json.RawMessage
			if json.Unmarshal(raw, &value) != nil || len(value) != 0 {
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
