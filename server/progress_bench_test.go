package server

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func BenchmarkProgressWriteContention(b *testing.B) {
	benchmarkProgressWriteContention(b, "no_coalescing", 1, 0)
	benchmarkProgressWriteContention(b, "coalesced_step_5", 5, time.Hour)
}

func benchmarkProgressWriteContention(b *testing.B, name string, minStep int, minInterval time.Duration) {
	b.Run(name, func(b *testing.B) {
		srv := newTestServer(b)
		store := newFakeJobStore()
		srv.store = store
		srv.progressWriteMinStep = minStep
		srv.progressWriteMinInterval = minInterval

		var workerSeq atomic.Int64
		var runs atomic.Int64

		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			workerID := workerSeq.Add(1)
			jobID := fmt.Sprintf("bench-progress-worker-%d", workerID)
			_, _ = store.Create(Job{
				ID:        jobID,
				Domain:    "example.com",
				Status:    JobRunning,
				CreatedAt: time.Now().UTC(),
			})
			for pb.Next() {
				job, _ := store.Get(jobID)
				job.Progress = 0
				_ = store.Update(job)
				srv.initProgressWriteState(jobID, 0, time.Now().UTC())
				for progress := 1; progress <= 100; progress++ {
					srv.updateJobProgress(jobID, progress)
				}
				runs.Add(1)
			}
		})
		b.StopTimer()

		runCount := runs.Load()
		if runCount == 0 {
			return
		}
		// Subtract the explicit reset Update done once per benchmark run.
		progressUpdates := store.UpdateCount() - runCount
		b.ReportMetric(float64(progressUpdates)/float64(runCount), "progress_updates/run")
	})
}
