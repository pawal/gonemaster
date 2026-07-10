package server

import (
	"testing"
	"time"
)

func TestSQLStoreDNSSECChainRoundTrip(t *testing.T) {
	const chain = `{"version":1,"zone":"example.com","status":"secure"}`
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
			now := time.Now().UTC().Truncate(time.Microsecond)

			job := Job{ID: "chain-job", Domain: "example.com", Status: JobSucceeded, CreatedAt: now}
			if _, err := s.Create(job); err != nil {
				t.Fatalf("Create: %v", err)
			}
			job.DNSSECChainJSON = chain
			graduateSQLJob(t, s, job, nil)

			got, ok := s.GetRunDNSSECChain(job.ID)
			if !ok {
				t.Fatal("GetRunDNSSECChain: not found")
			}
			if got != chain {
				t.Fatalf("chain = %q, want %q", got, chain)
			}

			result, ok := s.GetResult(job.ID)
			if !ok {
				t.Fatal("GetResult: not found")
			}
			if !result.HasDNSSECChain {
				t.Error("expected has_dnssec_chain marker true")
			}

			// A job graduated without a blob leaves no row and no marker.
			plain := Job{ID: "plain-job", Domain: "plain.example", Status: JobSucceeded, CreatedAt: now}
			if _, err := s.Create(plain); err != nil {
				t.Fatalf("Create plain: %v", err)
			}
			graduateSQLJob(t, s, plain, nil)
			if _, ok := s.GetRunDNSSECChain(plain.ID); ok {
				t.Error("expected no chain row for plain job")
			}
			plainResult, _ := s.GetResult(plain.ID)
			if plainResult.HasDNSSECChain {
				t.Error("expected marker false for plain job")
			}
		})
	}
}

func TestSQLStorePurgeRemovesDNSSECChain(t *testing.T) {
	const chain = `{"version":1,"zone":"old.example","status":"secure"}`
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
			old := time.Now().UTC().Add(-72 * time.Hour).Truncate(time.Microsecond)

			job := Job{ID: "old-chain-job", Domain: "old.example", Status: JobSucceeded, CreatedAt: old}
			if _, err := s.Create(job); err != nil {
				t.Fatalf("Create: %v", err)
			}
			job.FinishedAt = old.Add(time.Second)
			job.DNSSECChainJSON = chain
			graduateSQLJob(t, s, job, nil)

			if _, ok := s.GetRunDNSSECChain(job.ID); !ok {
				t.Fatal("expected chain before purge")
			}
			n, err := s.PurgeOlderThan(time.Now().UTC().Add(-24 * time.Hour))
			if err != nil {
				t.Fatalf("PurgeOlderThan: %v", err)
			}
			if n < 1 {
				t.Fatalf("purged %d runs, want >= 1", n)
			}
			if _, ok := s.GetRunDNSSECChain(job.ID); ok {
				t.Error("expected chain removed after purge")
			}
		})
	}
}
