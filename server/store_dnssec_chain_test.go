package server

import (
	"testing"
	"time"
)

const testChainJSON = `{"version":1,"zone":"example.com","status":"secure"}`

func TestJobStoreDNSSECChainRoundTrip(t *testing.T) {
	forEachStore(t, func(t *testing.T, s JobStore) {
		now := time.Now().UTC().Truncate(time.Microsecond)
		job := Job{ID: "job-chain", Domain: "example.com", Status: JobQueued, CreatedAt: now}
		created, err := s.Create(job)
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		created.DNSSECChainJSON = testChainJSON
		graduate(t, s, created, nil)

		chain, ok, err := s.GetRunDNSSECChain(created.ID)
		if err != nil {
			t.Fatalf("GetRunDNSSECChain: %v", err)
		}
		if !ok {
			t.Fatal("expected stored chain")
		}
		if chain != testChainJSON {
			t.Fatalf("chain = %q, want %q", chain, testChainJSON)
		}

		result, ok := s.GetResult(created.ID)
		if !ok {
			t.Fatal("expected result")
		}
		if !result.HasDNSSECChain {
			t.Error("expected has_dnssec_chain marker to be true")
		}
	})
}

func TestJobStoreDNSSECChainAbsent(t *testing.T) {
	forEachStore(t, func(t *testing.T, s JobStore) {
		now := time.Now().UTC().Truncate(time.Microsecond)
		created, err := s.Create(Job{ID: "job-nochain", Domain: "example.com", Status: JobQueued, CreatedAt: now})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		graduate(t, s, created, nil)

		if _, ok, _ := s.GetRunDNSSECChain(created.ID); ok {
			t.Error("expected no stored chain")
		}
		result, ok := s.GetResult(created.ID)
		if !ok {
			t.Fatal("expected result")
		}
		if result.HasDNSSECChain {
			t.Error("expected has_dnssec_chain marker to be false")
		}
	})
}

func TestJobStorePurgeRemovesDNSSECChain(t *testing.T) {
	forEachStore(t, func(t *testing.T, s JobStore) {
		old := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Microsecond)
		created, err := s.Create(Job{ID: "job-old-chain", Domain: "example.com", Status: JobQueued, CreatedAt: old})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		created.Status = JobSucceeded
		created.FinishedAt = old.Add(time.Second)
		created.DNSSECChainJSON = testChainJSON
		graduate(t, s, created, nil)

		if _, ok, _ := s.GetRunDNSSECChain(created.ID); !ok {
			t.Fatal("expected chain before purge")
		}
		n, err := s.PurgeOlderThan(time.Now().UTC().Add(-24 * time.Hour))
		if err != nil {
			t.Fatalf("purge: %v", err)
		}
		if n != 1 {
			t.Fatalf("purged %d runs, want 1", n)
		}
		if _, ok, _ := s.GetRunDNSSECChain(created.ID); ok {
			t.Error("expected chain removed after purge")
		}
	})
}
