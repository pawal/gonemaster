package dnssecutil

import "testing"

func TestNSEC3HashesMemoHashesOnce(t *testing.T) {
	calls := 0
	orig := nsec3Name
	nsec3Name = func(s, salt string, iter uint16) string { calls++; return orig(s, salt, iter) }
	t.Cleanup(func() { nsec3Name = orig })

	var h NSEC3Hashes
	for _, name := range []string{"a.example", "A.Example.", "a.example"} {
		h.Hash(name, "AB", 5)
	}
	h.Hash("a.example", "ab", 6)
	if calls != 2 {
		t.Errorf("hash computed %d times, want 2", calls)
	}
}
