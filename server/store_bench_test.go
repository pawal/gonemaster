package server

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func BenchmarkInMemoryJobStoreMixedContention(b *testing.B) {
	store := NewInMemoryJobStore()
	base := time.Now().UTC().Add(-time.Minute)
	const seedJobs = 256
	for i := 0; i < seedJobs; i++ {
		id := fmt.Sprintf("job-%03d", i)
		_, err := store.Create(Job{
			ID:        id,
			BatchID:   "bench",
			Domain:    fmt.Sprintf("bench-%03d.example", i),
			Status:    JobQueued,
			CreatedAt: base.Add(time.Duration(i) * time.Millisecond),
		})
		if err != nil {
			b.Fatalf("seed create %s: %v", id, err)
		}
	}

	var i atomic.Uint64
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			n := int(i.Add(1) % seedJobs)
			jobID := fmt.Sprintf("job-%03d", n)

			if job, ok := store.Get(jobID); ok {
				job.Status = JobRunning
				job.StartedAt = time.Now().UTC()
				_ = store.Update(job)
			}

			_ = store.SetResult(jobID, JobResult{
				JobID:  jobID,
				Status: JobSucceeded,
				Summary: map[string]any{
					"levels": map[string]int{
						"WARNING": n % 3,
						"ERROR":   n % 2,
					},
				},
			})

			_ = store.List(JobFilter{
				Limit:    25,
				Sort:     JobSortErrorDesc,
				Severity: JobSeverityWarningsPlus,
			})
		}
	})
}
