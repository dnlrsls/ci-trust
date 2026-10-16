package lintpreparation

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
)

func imageHash(b []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(b)) }

// Decode emitted bytes without Overlay, which would normalize writer defects.
func imageMembers(t *testing.T, data []byte) ([]*tar.Header, map[string][]byte) {
	t.Helper()
	if len(data) == 0 || len(data)%10240 != 0 {
		t.Fatalf("tar length %d is not a Python record multiple", len(data))
	}
	r := tar.NewReader(bytes.NewReader(data))
	hs, bodies := []*tar.Header{}, map[string][]byte{}
	for {
		h, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if _, exists := bodies[h.Name]; exists {
			t.Fatal("duplicate emitted name")
		}
		b, err := io.ReadAll(r)
		if err != nil {
			t.Fatal(err)
		}
		if h.Uid != 0 || h.Gid != 0 || h.ModTime.Unix() != 0 || h.Uname != "" || h.Gname != "" || !h.AccessTime.IsZero() || !h.ChangeTime.IsZero() {
			t.Fatalf("noncanonical metadata: %#v", h)
		}
		hs, bodies[h.Name] = append(hs, h), b
	}
	return hs, bodies
}

func imageFixture() Tree {
	return Tree{
		"work":   {Kind: "dir", Mode: 0555},
		"bin":    {Kind: "file", Mode: 0555, Data: []byte{0, 255, 10}},
		"config": {Kind: "file", Mode: 0444, Data: []byte("x")},
		"link":   {Kind: "link", Mode: 0777, Target: "/config"},
	}
}

func TestImageInventory(t *testing.T) {
	got, err := Inventory(imageFixture())
	if err != nil {
		t.Fatal(err)
	}
	want := []any{
		map[string]any{"path": "bin", "type": "file", "mode": "0o555", "uid": 0, "gid": 0, "size": 3, "sha256": imageHash([]byte{0, 255, 10})},
		map[string]any{"path": "config", "type": "file", "mode": "0o444", "uid": 0, "gid": 0, "size": 1, "sha256": imageHash([]byte("x"))},
		map[string]any{"path": "link", "type": "link", "mode": "0o777", "uid": 0, "gid": 0, "target": "/config"},
		map[string]any{"path": "work", "type": "dir", "mode": "0o555", "uid": 0, "gid": 0},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("inventory=%#v", got)
	}
	if _, err := Canonical(got); err != nil {
		t.Fatal(err)
	}
	empty, err := Inventory(Tree{})
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatal("empty inventory must be []")
	}
	for _, tree := range []Tree{
		{"a": {Kind: "link", Mode: 0777, Target: "missing"}},
		{"a": {Kind: "link", Mode: 0777, Target: "a"}},
	} {
		if _, err := Inventory(tree); err == nil {
			t.Error("unresolvable link accepted")
		}
	}
}

func TestImagePacking(t *testing.T) {
	tree := imageFixture()
	before, _ := Canonical(mustInventory(t, tree))
	image, ids, err := PackImage(tree)
	if err != nil {
		t.Fatal(err)
	}
	hs, outer := imageMembers(t, image)
	if len(hs) != 3 {
		t.Fatalf("outer entries=%d", len(hs))
	}
	layer := outer["layer.tar"]
	configName := strings.TrimPrefix(ids.ImageID, "sha256:") + ".json"
	wantNames := []string{configName, "layer.tar", "manifest.json"}
	for i, h := range hs {
		if h.Name != wantNames[i] || h.Mode != 0444 || h.Typeflag != tar.TypeReg || len(h.PAXRecords) != 0 {
			t.Fatalf("outer header=%#v", h)
		}
	}
	wantConfig := `{"architecture":"amd64","config":{"Cmd":[],"Entrypoint":[],"Env":[],"User":"65532:65532","WorkingDir":"/work"},"history":[],"os":"linux","rootfs":{"diff_ids":["` + imageHash(layer) + `"],"type":"layers"}}`
	if string(outer[configName]) != wantConfig {
		t.Fatalf("config=%s", outer[configName])
	}
	wantManifest := `[{"Config":"` + configName + `","Layers":["layer.tar"],"RepoTags":[]}]`
	if string(outer["manifest.json"]) != wantManifest {
		t.Fatalf("manifest=%s", outer["manifest.json"])
	}
	wantIDs := ImageIdentities{imageHash(outer[configName]), imageHash(layer), imageHash(image)}
	if ids != wantIDs {
		t.Fatalf("identities=%#v want %#v", ids, wantIDs)
	}
	encoded, _ := json.Marshal(ids)
	var keys map[string]string
	if err := json.Unmarshal(encoded, &keys); err != nil || len(keys) != 3 || keys["image_id"] != ids.ImageID || keys["layer_diff_id"] != ids.LayerDiffID || keys["archive_sha256"] != ids.ArchiveSHA256 {
		t.Fatalf("identity schema=%s", encoded)
	}
	lhs, bodies := imageMembers(t, layer)
	if len(lhs) != 4 {
		t.Fatalf("layer entries=%d", len(lhs))
	}
	for i, name := range []string{"bin", "config", "link", "work/"} {
		h, n := lhs[i], tree[strings.TrimSuffix(name, "/")]
		kind := map[string]byte{"dir": tar.TypeDir, "file": tar.TypeReg, "link": tar.TypeSymlink}[n.Kind]
		if h.Name != name || h.Mode != n.Mode || h.Typeflag != kind || h.Linkname != n.Target || len(h.PAXRecords) != 0 || !bytes.Equal(bodies[name], n.Data) {
			t.Fatalf("layer header/body mismatch: %#v", h)
		}
	}
	again, next, err := PackImage(tree)
	if err != nil || !bytes.Equal(image, again) || ids != next {
		t.Fatal("same-process serialization differs")
	}
	after, _ := Canonical(mustInventory(t, tree))
	if !bytes.Equal(before, after) {
		t.Fatal("packing mutated tree")
	}
	// Python pads after two terminal zero blocks to a 10240-byte record.
	if len(layer) != 10240 || len(image) != 20480 || !bytes.Equal(layer[3072:], make([]byte, 10240-3072)) {
		t.Fatal("incorrect terminal blocks or record padding")
	}
}

func mustInventory(t *testing.T, tree Tree) []any {
	t.Helper()
	v, err := Inventory(tree)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestImagePAXAndSGID(t *testing.T) {
	long := strings.Repeat("a", 101)
	target := strings.Repeat("b", 101)
	tree := Tree{long: {Kind: "link", Mode: 0777, Target: target}, target: {Kind: "file", Mode: 0444}, "é": {Kind: "file", Mode: 0444}}
	if err := Overlay(tree, layer(t, &tar.Header{Name: "work", Typeflag: tar.TypeDir, Mode: 02775})); err != nil {
		t.Fatal(err)
	}
	image, _, err := PackImage(tree)
	if err != nil {
		t.Fatal(err)
	}
	_, outer := imageMembers(t, image)
	hs, _ := imageMembers(t, outer["layer.tar"])
	if len(hs) != 4 {
		t.Fatalf("headers=%d", len(hs))
	}
	if hs[0].Name != long || hs[0].Linkname != target || hs[0].PAXRecords["path"] != long || hs[0].PAXRecords["linkpath"] != target {
		t.Fatalf("long PAX=%#v", hs[0])
	}
	if hs[2].Name != "work/" || hs[2].Mode != 0555 || len(hs[2].PAXRecords) != 0 {
		t.Fatalf("SGID leaked: %#v", hs[2])
	}
	if hs[3].PAXRecords["path"] != "é" {
		t.Fatal("non-ASCII path missing PAX")
	}
	again, _, err := PackImage(tree)
	if err != nil || !bytes.Equal(image, again) {
		t.Fatal("PAX serialization differs between calls")
	}
	// Python's PAX pseudo-name and insertion order differ from Go's defaults.
	raw := outer["layer.tar"]
	if string(bytes.TrimRight(raw[:100], "\x00")) != "././@PaxHeader" || raw[156] != tar.TypeXHeader {
		t.Fatal("non-Python PAX header")
	}
	wantPAX := "111 path=" + long + "\n115 linkpath=" + target + "\n"
	if !bytes.HasPrefix(raw[512:], []byte(wantPAX)) || string(raw[100:124]) != strings.Repeat("0000000\x00", 3) || string(raw[136:148]) != "00000000000\x00" {
		t.Fatal("PAX payload order, byte lengths or metadata differ")
	}
}
