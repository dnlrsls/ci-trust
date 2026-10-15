package ownedimage

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func fixture(t *testing.T) (string, string) {
	t.Helper()
	d := t.TempDir()
	b := make([]byte, 120)
	copy(b, "\x7fELF")
	b[4] = 2
	b[5] = 1
	b[6] = 1
	binary.LittleEndian.PutUint16(b[16:], uint16(elf.ET_EXEC))
	binary.LittleEndian.PutUint16(b[18:], uint16(elf.EM_X86_64))
	binary.LittleEndian.PutUint32(b[20:], 1)
	binary.LittleEndian.PutUint64(b[32:], 64)
	binary.LittleEndian.PutUint16(b[52:], 64)
	binary.LittleEndian.PutUint16(b[54:], 56)
	binary.LittleEndian.PutUint16(b[56:], 1)
	binary.LittleEndian.PutUint32(b[64:], 1)
	binary.LittleEndian.PutUint32(b[68:], 5)
	binary.LittleEndian.PutUint64(b[96:], 120)
	binary.LittleEndian.PutUint64(b[104:], 120)
	p := filepath.Join(d, "probe")
	m := filepath.Join(d, "marker")
	if err := os.WriteFile(p, b, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m, []byte(Marker), 0600); err != nil {
		t.Fatal(err)
	}
	return p, m
}
func TestPackInspect(t *testing.T) {
	p, m := fixture(t)
	a, err := Pack(p, m)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Pack(p, m)
	if err != nil || !bytes.Equal(a, b) {
		t.Fatal("not deterministic", err)
	}
	meta, err := Inspect(a, p, m)
	if err != nil || meta.Scope != "preparation-only" {
		t.Fatal(meta, err)
	}
	a[0] ^= 1
	if _, err := Inspect(a, p, m); err == nil {
		t.Fatal("accepted tamper")
	}
}
