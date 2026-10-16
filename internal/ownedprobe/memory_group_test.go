package ownedprobe

import "testing"

// A group OOM policy could kill the parent along with the owned child. Stop
// before resource demand instead of treating parent survival as an assumption.
func TestMemoryGroupOOMPolicyPreventsChildDemand(t *testing.T) {
	for _, value := range []string{"1\n", "invalid"} {
		s, m, _, calls := dynamicAdapter()
		m.files["/sys/fs/cgroup/memory.oom.group"] = &memoryFile{data: []byte(value)}
		got := s.childEffect("memory", "/sys/fs/cgroup/memory.events", "max", "oom_kill")
		if got.Supported || len(*calls) != 0 {
			t.Fatalf("unsafe group policy dispatched child: result=%+v calls=%v", got, *calls)
		}
	}
}
