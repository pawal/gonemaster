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

func (s *countingJobStore) GetDomain(id int64) (Domain, bool) {
	return s.inner.GetDomain(id)
}

func (s *countingJobStore) GetDomainByName(name string) (Domain, bool) {
	return s.inner.GetDomainByName(name)
}

func (s *countingJobStore) GetDomainNamesByIDs(ids []int64) map[int64]string {
	return s.inner.GetDomainNamesByIDs(ids)
}

func (s *countingJobStore) ListDomains(filter DomainFilter) DomainList {
	return s.inner.ListDomains(filter)
}

func (s *countingJobStore) UpdateDomainLatest(domainID int64, runID string, finishedAt time.Time, status, level string) error {
	return s.inner.UpdateDomainLatest(domainID, runID, finishedAt, status, level)
}

func (s *countingJobStore) CreateTag(name, description string) error {
	return s.inner.CreateTag(name, description)
}

func (s *countingJobStore) GetTag(name string) (Tag, bool) {
	return s.inner.GetTag(name)
}

func (s *countingJobStore) UpdateTag(name, description string) error {
	return s.inner.UpdateTag(name, description)
}

func (s *countingJobStore) SetTagDefaultProfile(name string, profileID *int64) error {
	return s.inner.SetTagDefaultProfile(name, profileID)
}

func (s *countingJobStore) DeleteTag(name string) error {
	return s.inner.DeleteTag(name)
}

func (s *countingJobStore) ListTags(limit, offset int) []Tag {
	return s.inner.ListTags(limit, offset)
}

func (s *countingJobStore) TagDomains(tag string, domainIDs []int64) error {
	return s.inner.TagDomains(tag, domainIDs)
}

func (s *countingJobStore) UntagDomains(tag string, domainIDs []int64) error {
	return s.inner.UntagDomains(tag, domainIDs)
}

func (s *countingJobStore) GetDomainTags(domainID int64) []string {
	return s.inner.GetDomainTags(domainID)
}

func (s *countingJobStore) ListDomainsByTag(tag string, filter DomainFilter) DomainList {
	return s.inner.ListDomainsByTag(tag, filter)
}

func (s *countingJobStore) GetTagSummary(tag string) (TagSummary, bool) {
	return s.inner.GetTagSummary(tag)
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

func (s *countingJobStore) ListRunsByDomain(domainID int64, limit, offset int) RunList {
	return s.inner.ListRunsByDomain(domainID, limit, offset)
}

func (s *countingJobStore) QueryEntries(filter EntryFilter) EntryList {
	return s.inner.QueryEntries(filter)
}

func (s *countingJobStore) CreateBatch(batch Batch) error {
	return s.inner.CreateBatch(batch)
}

func (s *countingJobStore) GetBatch(id string) (Batch, bool) {
	return s.inner.GetBatch(id)
}

func (s *countingJobStore) SetBatchSnapshotIntent(batchID string, intent bool) error {
	return s.inner.SetBatchSnapshotIntent(batchID, intent)
}

func (s *countingJobStore) ListBatchesByTag(tag string, limit, offset int) BatchList {
	return s.inner.ListBatchesByTag(tag, limit, offset)
}

func (s *countingJobStore) ListBatches(tagLike string, limit, offset int) BatchList {
	return s.inner.ListBatches(tagLike, limit, offset)
}

func (s *countingJobStore) BatchDeletePreviewStats(batchID string) (BatchDeletePreview, error) {
	return s.inner.BatchDeletePreviewStats(batchID)
}

func (s *countingJobStore) DeleteBatch(batchID string) ([]int64, error) {
	return s.inner.DeleteBatch(batchID)
}

func (s *countingJobStore) BatchHasRuns(batchID string) bool {
	return s.inner.BatchHasRuns(batchID)
}

func (s *countingJobStore) CreateProfile(p StoredProfile) (StoredProfile, error) {
	return s.inner.CreateProfile(p)
}
func (s *countingJobStore) GetProfile(id int64) (StoredProfile, bool) {
	return s.inner.GetProfile(id)
}
func (s *countingJobStore) GetProfileByName(name string) (StoredProfile, bool) {
	return s.inner.GetProfileByName(name)
}
func (s *countingJobStore) UpdateProfile(p StoredProfile) error {
	return s.inner.UpdateProfile(p)
}
func (s *countingJobStore) DeleteProfile(id int64) error {
	return s.inner.DeleteProfile(id)
}
func (s *countingJobStore) ListProfiles() []StoredProfile {
	return s.inner.ListProfiles()
}

func (s *countingJobStore) GetSetting(key string) (string, bool) {
	return s.inner.GetSetting(key)
}
func (s *countingJobStore) SetSetting(key, value string) error {
	return s.inner.SetSetting(key, value)
}
func (s *countingJobStore) DeleteSetting(key string) error {
	return s.inner.DeleteSetting(key)
}
func (s *countingJobStore) ListSettings() map[string]string {
	return s.inner.ListSettings()
}

func (s *countingJobStore) PurgeOlderThan(cutoff time.Time) (int64, error) {
	return s.inner.PurgeOlderThan(cutoff)
}

func (s *countingJobStore) ListAnalysisCohorts() []AnalysisCohort {
	return s.inner.ListAnalysisCohorts()
}
func (s *countingJobStore) GetAnalysisCohort(id int64) (AnalysisCohort, bool) {
	return s.inner.GetAnalysisCohort(id)
}
func (s *countingJobStore) GetAnalysisCohortBySource(sourceType, sourceTag string) (AnalysisCohort, bool) {
	return s.inner.GetAnalysisCohortBySource(sourceType, sourceTag)
}
func (s *countingJobStore) UpsertAnalysisCohort(cohort AnalysisCohort) (AnalysisCohort, error) {
	return s.inner.UpsertAnalysisCohort(cohort)
}
func (s *countingJobStore) DeleteAnalysisCohort(id int64) error {
	return s.inner.DeleteAnalysisCohort(id)
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
