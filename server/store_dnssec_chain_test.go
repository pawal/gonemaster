package server

import (
	"testing"
	"time"
)

const testChainJSON = `{"version":1,"zone":"example.com","status":"secure"}`

func TestInMemoryStoreDNSSECChainRoundTrip(t *testing.T) {
	store := NewInMemoryJobStore()
	now := time.Now().UTC()
	job := Job{ID: "job-chain", Domain: "example.com", Status: JobQueued, CreatedAt: now}
	created, err := store.Create(job)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	created.DNSSECChainJSON = testChainJSON
	graduateTestJob(t, store, created, nil)

	chain, ok := store.GetRunDNSSECChain(created.ID)
	if !ok {
		t.Fatal("expected stored chain")
	}
	if chain != testChainJSON {
		t.Fatalf("chain = %q, want %q", chain, testChainJSON)
	}

	result, ok := store.GetResult(created.ID)
	if !ok {
		t.Fatal("expected result")
	}
	if !result.HasDNSSECChain {
		t.Error("expected has_dnssec_chain marker to be true")
	}
}

func TestInMemoryStoreDNSSECChainAbsent(t *testing.T) {
	store := NewInMemoryJobStore()
	now := time.Now().UTC()
	job := Job{ID: "job-nochain", Domain: "example.com", Status: JobQueued, CreatedAt: now}
	created, err := store.Create(job)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// No DNSSECChainJSON set.
	graduateTestJob(t, store, created, nil)

	if _, ok := store.GetRunDNSSECChain(created.ID); ok {
		t.Error("expected no stored chain")
	}
	result, ok := store.GetResult(created.ID)
	if !ok {
		t.Fatal("expected result")
	}
	if result.HasDNSSECChain {
		t.Error("expected has_dnssec_chain marker to be false")
	}
}

func TestInMemoryStorePurgeRemovesDNSSECChain(t *testing.T) {
	store := NewInMemoryJobStore()
	old := time.Now().UTC().Add(-48 * time.Hour)
	job := Job{ID: "job-old-chain", Domain: "example.com", Status: JobQueued, CreatedAt: old}
	created, err := store.Create(job)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	created.Status = JobSucceeded
	created.FinishedAt = old.Add(time.Second)
	created.DNSSECChainJSON = testChainJSON
	graduateTestJob(t, store, created, nil)

	if _, ok := store.GetRunDNSSECChain(created.ID); !ok {
		t.Fatal("expected chain before purge")
	}
	n, err := store.PurgeOlderThan(time.Now().UTC().Add(-24 * time.Hour))
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if n != 1 {
		t.Fatalf("purged %d runs, want 1", n)
	}
	if _, ok := store.GetRunDNSSECChain(created.ID); ok {
		t.Error("expected chain removed after purge")
	}
}
