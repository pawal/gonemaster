package server

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
)

type countingJobStore struct {
	inner       *InMemoryJobStore
	updateCount atomic.Int64
}

func newCountingJobStore() *countingJobStore {
	return &countingJobStore{
		inner: NewInMemoryJobStore(),
	}
}

func (s *countingJobStore) Create(job Job) (Job, error) {
	return s.inner.Create(job)
}

func (s *countingJobStore) Get(id string) (Job, bool) {
	return s.inner.Get(id)
}

func (s *countingJobStore) GetByPublicID(publicID string) (Job, bool) {
	return s.inner.GetByPublicID(publicID)
}

func (s *countingJobStore) Update(job Job) error {
	s.updateCount.Add(1)
	return s.inner.Update(job)
}

func (s *countingJobStore) List(filter JobFilter) JobList {
	return s.inner.List(filter)
}

func (s *countingJobStore) GraduateJob(job Job, entries []engine.LogEntry) error {
	return s.inner.GraduateJob(job, entries)
}

func (s *countingJobStore) GetResult(jobID string) (JobResult, bool) {
	return s.inner.GetResult(jobID)
}

func (s *countingJobStore) GetOrCreateDomain(name string) (Domain, error) {
	return s.inner.GetOrCreateDomain(name)
}

func (s *countingJobStore) ListDomains(filter DomainFilter) DomainList {
	return s.inner.ListDomains(filter)
}

func (s *countingJobStore) CreateTag(name, description string) error {
	return s.inner.CreateTag(name, description)
}

func (s *countingJobStore) ListTags(limit, offset int) []Tag {
	return s.inner.ListTags(limit, offset)
}

func (s *countingJobStore) TagDomains(tag string, domainIDs []int64) error {
	return s.inner.TagDomains(tag, domainIDs)
}

func (s *countingJobStore) GetRun(id string) (Run, bool) {
	return s.inner.GetRun(id)
}

func (s *countingJobStore) GetRunByPublicID(publicID string) (Run, bool) {
	return s.inner.GetRunByPublicID(publicID)
}

func (s *countingJobStore) ListRuns(filter RunFilter) RunList {
	return s.inner.ListRuns(filter)
}

func (s *countingJobStore) CreateBatch(batch Batch) error {
	return s.inner.CreateBatch(batch)
}

func (s *countingJobStore) GetBatch(id string) (Batch, bool) {
	return s.inner.GetBatch(id)
}

func (s *countingJobStore) PurgeOlderThan(cutoff time.Time) (int64, error) {
	return s.inner.PurgeOlderThan(cutoff)
}

func (s *countingJobStore) UpdateCount() int64 {
	return s.updateCount.Load()
}

func BenchmarkProgressWriteContention(b *testing.B) {
	benchmarkProgressWriteContention(b, "no_coalescing", 1, 0)
	benchmarkProgressWriteContention(b, "coalesced_step_5", 5, time.Hour)
}

func benchmarkProgressWriteContention(b *testing.B, name string, minStep int, minInterval time.Duration) {
	b.Run(name, func(b *testing.B) {
		srv := New(DefaultConfig())
		store := newCountingJobStore()
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
