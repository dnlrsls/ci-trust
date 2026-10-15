package discovery

import "testing"

func TestDefaultOptionsAreFinite(t *testing.T) {
	want := Options{MaxDepth: 16, MaxEntries: 4096, MaxFileBytes: 1 << 20, MaxTotalBytes: 8 << 20, MaxReferences: 256, MaxScriptDepth: 16}
	if got := DefaultOptions(); got != want {
		t.Fatalf("default limits = %+v; want %+v", got, want)
	}
}
