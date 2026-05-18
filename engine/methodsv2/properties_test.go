package methodsv2

import (
	"fmt"
	"math/rand"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/recursor"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

// TestGetDelNSNamesAndIPsNoNilNamesInOutput runs several seeded scenarios
// with random nameserver-name sets and asserts that every returned NSItem
// has a non-empty Name. A nil/empty name in the output would be a quiet
// data-loss bug for downstream callers.
func TestGetDelNSNamesAndIPsNoNilNamesInOutput(t *testing.T) {
	letters := []byte("abcdefghijklmnopqrstuvwxyz")
	for trial := 0; trial < 20; trial++ {
		ClearCache()
		nameserver.EmptyCache()

		seed := int64(trial * 11)
		rng := rand.New(rand.NewSource(seed))
		size := 1 + rng.Intn(5)

		ctx, prof, _ := testhelpers.Context(t)
		prof.Net.IPv4 = true
		prof.Net.IPv6 = true

		r := &recursor.Recursor{}
		if err := r.AddFakeAddresses(".", map[string][]string{"a.root": {"192.0.2.1"}}); err != nil {
			t.Fatalf("add root: %v", err)
		}
		glue := map[string][]string{}
		for i := 0; i < size; i++ {
			labelLen := 1 + rng.Intn(4)
			label := make([]byte, labelLen)
			for j := range label {
				label[j] = letters[rng.Intn(len(letters))]
			}
			name := string(label) + ".example.com"
			glue[name] = []string{fmt.Sprintf("192.0.2.%d", 50+i)}
		}
		if err := r.AddFakeAddresses("example.com", glue); err != nil {
			t.Fatalf("trial %d: add zone: %v", trial, err)
		}

		z, err := zone.NewWithRecursor("example.com", r)
		if err != nil {
			t.Fatalf("trial %d: new zone: %v", trial, err)
		}

		items, err := GetDelNSNamesAndIPs(ctx, &z)
		if err != nil {
			t.Fatalf("trial %d (seed=%d): %v", trial, seed, err)
		}
		for i, item := range items {
			if item.Name.String() == "" {
				t.Errorf("trial seed=%d item %d: empty Name", seed, i)
			}
		}
	}
	t.Cleanup(func() {
		ClearCache()
		nameserver.EmptyCache()
	})
}
