package ownedprobe

import (
	"os"
	"syscall"
	"testing"
)

// Model a real full tmpfs: the limit file can fill it exactly, after which a
// separate legacy scratch write cannot succeed. All descriptors remain inert.
func TestFullScratchQuotaDoesNotInvalidateReadOnlyGuard(t *testing.T) {
	s, m, q, _ := dynamicAdapter()
	open := s.ops.open
	legacyAfterFull := 0
	s.ops.open = func(path string, flags int, mode os.FileMode) (file, error) {
		if path == scratchPath && flags&os.O_EXCL != 0 && q.budget == 0 {
			legacyAfterFull++
			return nil, syscall.ENOSPC
		}
		return open(path, flags, mode)
	}
	if !s.scratchLimit() {
		t.Fatalf("quota proof blocked by guard allocation: native attempts=%d legacy-after-full=%d", q.attempted, legacyAfterFull)
	}
	if q.attempted != 8*1024*1024+1 || legacyAfterFull != 0 || q.closes != 1 || m.files[limitScratchPath] != nil || m.files[scratchPath] != nil {
		t.Fatalf("attempts=%d guardallocations=%d closes=%d ownedfilesleft=%v/%v", q.attempted, legacyAfterFull, q.closes, m.files[limitScratchPath] != nil, m.files[scratchPath] != nil)
	}
}
