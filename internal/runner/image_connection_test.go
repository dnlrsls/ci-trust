package runner

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/dnlrsls/ci-trust/internal/ownedimage"
)

type loadedOwnedClient struct {
	ownedClient
	meta  ownedimage.Metadata
	loads int
}

func (f *loadedOwnedClient) load(ctx context.Context, archive []byte) (string, error) {
	f.loads++
	return "load output ignored", ctx.Err()
}
func (f *loadedOwnedClient) call(ctx context.Context, w io.Writer, args ...string) (string, error) {
	if args[0] == "image" {
		return imageInspection(f.meta, map[string]any{}), nil
	}
	s, err := f.ownedClient.call(ctx, w, args...)
	if args[0] == "inspect" {
		s = strings.ReplaceAll(s, owned().Image, f.meta.ImageID)
	}
	return s, err
}

func TestVerifiedLoadConnectsToExistingRunnerAndExactCleanup(t *testing.T) {
	a, binary, marker, meta := imageFixture(t)
	client := &loadedOwnedClient{meta: meta}
	d := &LocalDocker{client: client, host: "linux"}
	if err := d.LoadOwnedImage(context.Background(), a, binary, marker, meta); err != nil {
		t.Fatal(err)
	}
	p := owned()
	p.Image = meta.ImageID
	r, err := NewOwned(d, p, func(context.Context) (string, error) { return p.Snapshot, nil })
	if err != nil {
		t.Fatal(err)
	}
	result := r.Execute(context.Background(), p, Consent{Approved: true, Digest: p.Digest()})
	// A nonzero tool exit remains a completed tool result, not a harness PASS.
	if result.Outcome != Completed || result.ExitCode != 7 || !result.CleanupVerified || !client.removed || client.loads != 1 {
		t.Fatalf("result=%+v loads=%d", result, client.loads)
	}
}
