package server

import (
	"context"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/rdata"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/logargs"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
)

type spyJobStore struct {
	mu         sync.Mutex
	inner      *InMemoryJobStore
	progresses []int
}

func newSpyJobStore() *spyJobStore {
	return &spyJobStore{inner: NewInMemoryJobStore()}
}

func (s *spyJobStore) Create(job Job) (Job, error) {
	return s.inner.Create(job)
}

func (s *spyJobStore) Get(id string) (Job, bool) {
	return s.inner.Get(id)
}

func (s *spyJobStore) GetByPublicID(publicID string) (Job, bool) {
	return s.inner.GetByPublicID(publicID)
}

func (s *spyJobStore) Update(job Job) error {
	s.mu.Lock()
	s.progresses = append(s.progresses, job.Progress)
	s.mu.Unlock()
	return s.inner.Update(job)
}

func (s *spyJobStore) List(filter JobFilter) JobList {
	return s.inner.List(filter)
}

func (s *spyJobStore) GraduateJob(job Job, entries []engine.LogEntry) error {
	return s.inner.GraduateJob(job, entries)
}

func (s *spyJobStore) GetResult(jobID string) (JobResult, bool) {
	return s.inner.GetResult(jobID)
}

func (s *spyJobStore) GetOrCreateDomain(name string) (Domain, error) {
	return s.inner.GetOrCreateDomain(name)
}

func (s *spyJobStore) GetDomain(id int64) (Domain, bool) {
	return s.inner.GetDomain(id)
}

func (s *spyJobStore) GetDomainByName(name string) (Domain, bool) {
	return s.inner.GetDomainByName(name)
}

func (s *spyJobStore) ListDomains(filter DomainFilter) DomainList {
	return s.inner.ListDomains(filter)
}

func (s *spyJobStore) UpdateDomainLatest(domainID int64, runID string, finishedAt time.Time, status, level string) error {
	return s.inner.UpdateDomainLatest(domainID, runID, finishedAt, status, level)
}

func (s *spyJobStore) CreateTag(name, description string) error {
	return s.inner.CreateTag(name, description)
}

func (s *spyJobStore) GetTag(name string) (Tag, bool) {
	return s.inner.GetTag(name)
}

func (s *spyJobStore) UpdateTag(name, description string) error {
	return s.inner.UpdateTag(name, description)
}

func (s *spyJobStore) SetTagDefaultProfile(name string, profileID *int64) error {
	return s.inner.SetTagDefaultProfile(name, profileID)
}

func (s *spyJobStore) DeleteTag(name string) error {
	return s.inner.DeleteTag(name)
}

func (s *spyJobStore) ListTags(limit, offset int) []Tag {
	return s.inner.ListTags(limit, offset)
}

func (s *spyJobStore) TagDomains(tag string, domainIDs []int64) error {
	return s.inner.TagDomains(tag, domainIDs)
}

func (s *spyJobStore) UntagDomains(tag string, domainIDs []int64) error {
	return s.inner.UntagDomains(tag, domainIDs)
}

func (s *spyJobStore) GetDomainTags(domainID int64) []string {
	return s.inner.GetDomainTags(domainID)
}

func (s *spyJobStore) ListDomainsByTag(tag string, filter DomainFilter) DomainList {
	return s.inner.ListDomainsByTag(tag, filter)
}

func (s *spyJobStore) GetTagSummary(tag string) (TagSummary, bool) {
	return s.inner.GetTagSummary(tag)
}

func (s *spyJobStore) GetRun(id string) (Run, bool) {
	return s.inner.GetRun(id)
}

func (s *spyJobStore) GetRunByPublicID(publicID string) (Run, bool) {
	return s.inner.GetRunByPublicID(publicID)
}

func (s *spyJobStore) ListRuns(filter RunFilter) RunList {
	return s.inner.ListRuns(filter)
}

func (s *spyJobStore) ListRunsByDomain(domainID int64, limit, offset int) RunList {
	return s.inner.ListRunsByDomain(domainID, limit, offset)
}

func (s *spyJobStore) QueryEntries(filter EntryFilter) EntryList {
	return s.inner.QueryEntries(filter)
}

func (s *spyJobStore) CreateBatch(batch Batch) error {
	return s.inner.CreateBatch(batch)
}

func (s *spyJobStore) GetBatch(id string) (Batch, bool) {
	return s.inner.GetBatch(id)
}

func (s *spyJobStore) CreateProfile(p StoredProfile) (StoredProfile, error) {
	return s.inner.CreateProfile(p)
}
func (s *spyJobStore) GetProfile(id int64) (StoredProfile, bool) {
	return s.inner.GetProfile(id)
}
func (s *spyJobStore) GetProfileByName(name string) (StoredProfile, bool) {
	return s.inner.GetProfileByName(name)
}
func (s *spyJobStore) UpdateProfile(p StoredProfile) error {
	return s.inner.UpdateProfile(p)
}
func (s *spyJobStore) DeleteProfile(id int64) error {
	return s.inner.DeleteProfile(id)
}
func (s *spyJobStore) ListProfiles() []StoredProfile {
	return s.inner.ListProfiles()
}

func (s *spyJobStore) GetSetting(key string) (string, bool) {
	return s.inner.GetSetting(key)
}
func (s *spyJobStore) SetSetting(key, value string) error {
	return s.inner.SetSetting(key, value)
}
func (s *spyJobStore) DeleteSetting(key string) error {
	return s.inner.DeleteSetting(key)
}
func (s *spyJobStore) ListSettings() map[string]string {
	return s.inner.ListSettings()
}

func (s *spyJobStore) PurgeOlderThan(cutoff time.Time) (int64, error) {
	return s.inner.PurgeOlderThan(cutoff)
}

func (s *spyJobStore) Progresses() []int {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]int, len(s.progresses))
	copy(out, s.progresses)
	return out
}

func TestProgressUpdatesForMultipleTests(t *testing.T) {
	srv := New(DefaultConfig())
	spy := newSpyJobStore()
	srv.store = spy

	srv.engineRunner = func(_ engine.RunRequest) ([]engine.LogEntry, error) {
		return nil, nil
	}

	job := Job{
		ID:        "job-progress",
		Domain:    "example.com",
		Tests:     []string{"t1", "t2", "t3"},
		Status:    JobQueued,
		CreatedAt: time.Now().UTC(),
		Progress:  0,
	}
	if _, err := srv.store.Create(job); err != nil {
		t.Fatalf("create job: %v", err)
	}

	if err := srv.runJob(job.ID); err != nil {
		t.Fatalf("run job: %v", err)
	}

	progresses := spy.Progresses()
	seen := map[int]bool{}
	for _, value := range progresses {
		seen[value] = true
	}
	if !seen[33] || !seen[67] || !seen[100] {
		t.Fatalf("expected progress updates including 33, 67, 100, got %v", progresses)
	}
}

func TestUpdateJobProgressCoalescesSmallIncrements(t *testing.T) {
	srv := New(DefaultConfig())
	spy := newSpyJobStore()
	srv.store = spy
	srv.progressWriteMinStep = 10
	srv.progressWriteMinInterval = time.Hour

	job := Job{
		ID:        "job-progress-coalesce",
		Domain:    "example.com",
		Status:    JobRunning,
		CreatedAt: time.Now().UTC(),
		Progress:  0,
	}
	if _, err := srv.store.Create(job); err != nil {
		t.Fatalf("create job: %v", err)
	}
	srv.initProgressWriteState(job.ID, 0, time.Now().UTC())
	defer srv.clearProgressWriteState(job.ID)

	for progress := 1; progress < 10; progress++ {
		srv.updateJobProgress(job.ID, progress)
	}
	if got := spy.Progresses(); len(got) != 0 {
		t.Fatalf("expected no persisted progress before threshold, got %v", got)
	}

	srv.updateJobProgress(job.ID, 10)
	progresses := spy.Progresses()
	if len(progresses) != 1 || progresses[0] != 10 {
		t.Fatalf("expected one persisted progress update [10], got %v", progresses)
	}
}

func TestUpdateJobProgressAlwaysPersistsTerminal100(t *testing.T) {
	srv := New(DefaultConfig())
	spy := newSpyJobStore()
	srv.store = spy
	srv.progressWriteMinStep = 200
	srv.progressWriteMinInterval = time.Hour

	job := Job{
		ID:        "job-progress-terminal",
		Domain:    "example.com",
		Status:    JobRunning,
		CreatedAt: time.Now().UTC(),
		Progress:  0,
	}
	if _, err := srv.store.Create(job); err != nil {
		t.Fatalf("create job: %v", err)
	}
	srv.initProgressWriteState(job.ID, 0, time.Now().UTC())
	defer srv.clearProgressWriteState(job.ID)

	srv.updateJobProgress(job.ID, 99)
	if got := spy.Progresses(); len(got) != 0 {
		t.Fatalf("expected no persisted progress before terminal update, got %v", got)
	}

	srv.updateJobProgress(job.ID, 100)
	progresses := spy.Progresses()
	if len(progresses) != 1 || progresses[0] != 100 {
		t.Fatalf("expected terminal progress update [100], got %v", progresses)
	}
	jobAfter, ok := srv.store.Get(job.ID)
	if !ok {
		t.Fatalf("expected stored job")
	}
	if jobAfter.Progress != 100 {
		t.Fatalf("expected stored progress 100, got %d", jobAfter.Progress)
	}
}

func TestRunEngineForJobParallel(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxConcurrentJobs = 0
	srv := New(cfg)

	started := make(chan struct{}, 2)
	release := make(chan struct{})
	srv.engineRunner = func(_ engine.RunRequest) ([]engine.LogEntry, error) {
		started <- struct{}{}
		<-release
		return nil, nil
	}

	job1 := Job{ID: "job-par-1", Domain: "example.com", Tests: []string{"basic01"}}
	job2 := Job{ID: "job-par-2", Domain: "example.net", Tests: []string{"basic01"}}

	errs := make(chan error, 2)
	go func() {
		_, _, _, _, err := srv.runEngineForJob(job1, context.Background())
		errs <- err
	}()
	go func() {
		_, _, _, _, err := srv.runEngineForJob(job2, context.Background())
		errs <- err
	}()

	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(250 * time.Millisecond):
			t.Fatalf("expected both runs to start in parallel, got %d", i)
		}
	}

	close(release)
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
}

func TestRunEngineForJobLimiter(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxConcurrentJobs = 1
	srv := New(cfg)

	started := make(chan struct{}, 2)
	release := make(chan struct{})
	srv.engineRunner = func(_ engine.RunRequest) ([]engine.LogEntry, error) {
		started <- struct{}{}
		<-release
		return nil, nil
	}

	job1 := Job{ID: "job-cap-1", Domain: "example.com", Tests: []string{"basic01"}}
	job2 := Job{ID: "job-cap-2", Domain: "example.net", Tests: []string{"basic01"}}

	errs := make(chan error, 2)
	go func() {
		_, _, _, _, err := srv.runEngineForJob(job1, context.Background())
		errs <- err
	}()
	go func() {
		_, _, _, _, err := srv.runEngineForJob(job2, context.Background())
		errs <- err
	}()

	select {
	case <-started:
	case <-time.After(250 * time.Millisecond):
		t.Fatalf("expected first run to start")
	}
	select {
	case <-started:
		t.Fatalf("expected second run to be blocked by limiter")
	case <-time.After(200 * time.Millisecond):
	}

	close(release)
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
}

func TestRunEngineForJobPassesUndelegatedInputs(t *testing.T) {
	cfg := DefaultConfig()
	srv := New(cfg)

	var captured engine.RunRequest
	srv.engineRunner = func(req engine.RunRequest) ([]engine.LogEntry, error) {
		captured = req
		return nil, nil
	}

	job := Job{
		ID:        "job-undel",
		Domain:    "example.com",
		Status:    JobQueued,
		CreatedAt: time.Now().UTC(),
		UndelegatedNS: []engine.UndelegatedNameserver{
			{Name: "ns1.example.com", IP: "192.0.2.1"},
			{Name: "ns1.example.com", IP: "2001:db8::1"},
		},
		UndelegatedDS: []engine.UndelegatedDSInfo{
			{KeyTag: 12345, Algorithm: 13, DigestType: 2, Digest: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"},
		},
	}

	_, _, _, _, err := srv.runEngineForJob(job, context.Background())
	if err != nil {
		t.Fatalf("runEngineForJob: %v", err)
	}
	if len(captured.UndelegatedNameservers) != 2 {
		t.Fatalf("expected 2 undelegated nameservers, got %d", len(captured.UndelegatedNameservers))
	}
	if captured.UndelegatedNameservers[0].Name != "ns1.example.com" || captured.UndelegatedNameservers[0].IP != "192.0.2.1" {
		t.Fatalf("unexpected first nameserver: %+v", captured.UndelegatedNameservers[0])
	}
	if captured.UndelegatedNameservers[1].Name != "ns1.example.com" || captured.UndelegatedNameservers[1].IP != "2001:db8::1" {
		t.Fatalf("unexpected second nameserver: %+v", captured.UndelegatedNameservers[1])
	}
	if len(captured.UndelegatedDSInfo) != 1 {
		t.Fatalf("expected 1 undelegated DS record, got %d", len(captured.UndelegatedDSInfo))
	}
	if captured.UndelegatedDSInfo[0].KeyTag != 12345 {
		t.Fatalf("unexpected DS key tag: %d", captured.UndelegatedDSInfo[0].KeyTag)
	}
}

func TestRunEngineForJobPassesSourceAddrOverrides(t *testing.T) {
	cfg := DefaultConfig()
	source4 := "192.0.2.70"
	source6 := "2001:db8::70"
	cfg.SourceAddr4 = &source4
	cfg.SourceAddr6 = &source6
	srv := New(cfg)

	var captured engine.RunRequest
	srv.engineRunner = func(req engine.RunRequest) ([]engine.LogEntry, error) {
		captured = req
		return nil, nil
	}

	job := Job{
		ID:        "job-sourceaddr",
		Domain:    "example.com",
		Status:    JobQueued,
		CreatedAt: time.Now().UTC(),
	}

	_, _, _, _, err := srv.runEngineForJob(job, context.Background())
	if err != nil {
		t.Fatalf("runEngineForJob: %v", err)
	}
	if captured.SourceAddr4 == nil || *captured.SourceAddr4 != source4 {
		t.Fatalf("expected SourceAddr4 %q, got %#v", source4, captured.SourceAddr4)
	}
	if captured.SourceAddr6 == nil || *captured.SourceAddr6 != source6 {
		t.Fatalf("expected SourceAddr6 %q, got %#v", source6, captured.SourceAddr6)
	}
}

func TestRunEngineForJobPassesIPDisableFlags(t *testing.T) {
	srv := New(DefaultConfig())

	var captured engine.RunRequest
	srv.engineRunner = func(req engine.RunRequest) ([]engine.LogEntry, error) {
		captured = req
		return nil, nil
	}

	job := Job{
		ID:           "job-ipv6-disabled",
		Domain:       "example.com",
		Status:       JobQueued,
		CreatedAt:    time.Now().UTC(),
		IPv6Disabled: true,
	}

	_, _, _, _, err := srv.runEngineForJob(job, context.Background())
	if err != nil {
		t.Fatalf("runEngineForJob: %v", err)
	}
	if captured.IPv6 == nil {
		t.Fatal("expected captured.IPv6 pointer to be set")
	}
	if *captured.IPv6 {
		t.Fatalf("expected captured.IPv6 == false, got true")
	}
	if captured.IPv4 != nil {
		t.Fatalf("expected captured.IPv4 to stay nil, got %#v", captured.IPv4)
	}

	// IPv4-only path.
	job2 := Job{
		ID:           "job-ipv4-disabled",
		Domain:       "example.com",
		Status:       JobQueued,
		CreatedAt:    time.Now().UTC(),
		IPv4Disabled: true,
	}
	_, _, _, _, err = srv.runEngineForJob(job2, context.Background())
	if err != nil {
		t.Fatalf("runEngineForJob: %v", err)
	}
	if captured.IPv4 == nil || *captured.IPv4 {
		t.Fatalf("expected captured.IPv4 == false, got %#v", captured.IPv4)
	}
}

func TestDNSQueryCounterCallback(t *testing.T) {
	counter := &dnsQueryCounter{}
	entries := []*logger.Entry{
		{Tag: "EXTERNAL_QUERY", Args: map[string]any{logargs.KeyAddress: "192.0.2.10"}},
		{Tag: "external_query", Args: map[string]any{logargs.KeyAddress: "2001:db8::10"}},
		{Tag: "EXTERNAL_QUERY", Args: map[string]any{logargs.KeyAddress: "not-an-ip"}},
		{Tag: "EXTERNAL_RESPONSE", Args: map[string]any{logargs.KeyAddress: "198.51.100.20"}},
	}
	for _, entry := range entries {
		if err := counter.Callback(entry); err != nil {
			t.Fatalf("callback error: %v", err)
		}
	}
	ipv4, ipv6 := counter.Totals()
	if ipv4 != 1 {
		t.Fatalf("ipv4 = %d, want 1", ipv4)
	}
	if ipv6 != 1 {
		t.Fatalf("ipv6 = %d, want 1", ipv6)
	}
}

func TestRunEngineForJobPassesCacheStore(t *testing.T) {
	cfg := DefaultConfig()
	srv := New(cfg)

	var captured engine.RunRequest
	srv.engineRunner = func(req engine.RunRequest) ([]engine.LogEntry, error) {
		captured = req
		return nil, nil
	}

	job := Job{
		ID:        "job-cache",
		Domain:    "example.com",
		Status:    JobQueued,
		CreatedAt: time.Now().UTC(),
	}

	_, stats, _, _, err := srv.runEngineForJob(job, context.Background())
	if err != nil {
		t.Fatalf("runEngineForJob: %v", err)
	}
	if captured.NameserverCache == nil {
		t.Fatalf("expected NameserverCache to be set on RunRequest")
	}
	// With a stub engine that does no real queries, cache stats should be zero.
	if stats.cacheHits != 0 || stats.cacheMisses != 0 || stats.cacheEvictions != 0 {
		t.Fatalf("expected zero cache stats from stub engine, got hits=%d misses=%d evictions=%d",
			stats.cacheHits, stats.cacheMisses, stats.cacheEvictions)
	}
}

func TestRunEngineForJobHotCacheReportsWarmQueryMetrics(t *testing.T) {
	cfg := DefaultConfig()
	cfg.CrossJobHotCache = true
	srv := New(cfg)

	var networkCalls atomic.Int32
	srv.engineRunner = func(req engine.RunRequest) ([]engine.LogEntry, error) {
		ns, err := nameserver.NewWithCache(req.NameserverCache, "ns.example", "192.0.2.60", nil)
		if err != nil {
			return nil, err
		}
		ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
			networkCalls.Add(1)
			msg := new(dns.Msg)
			msg.Rcode = dns.RcodeSuccess
			msg.Answer = []dns.RR{
				&dns.A{
					Hdr: dns.Header{
						Name:  "warm.example.",
						Class: dns.ClassINET,
						TTL:   60,
					},
					A: rdata.A{Addr: netip.MustParseAddr("192.0.2.60")},
				},
			}
			return packet.Packet{Msg: msg}, nil
		})
		_, err = ns.QueryWithOptions(req.Context, "warm.example", "A", nil)
		return nil, err
	}

	jobA := Job{ID: "job-hot-a", Domain: "example.com", Status: JobQueued, CreatedAt: time.Now().UTC()}
	_, statsA, _, _, err := srv.runEngineForJob(jobA, context.Background())
	if err != nil {
		t.Fatalf("first runEngineForJob: %v", err)
	}
	if statsA.cacheHits != 0 || statsA.cacheMisses != 1 {
		t.Fatalf("first run cache stats = hits=%d misses=%d, want hits=0 misses=1", statsA.cacheHits, statsA.cacheMisses)
	}

	jobB := Job{ID: "job-hot-b", Domain: "example.net", Status: JobQueued, CreatedAt: time.Now().UTC()}
	_, statsB, _, _, err := srv.runEngineForJob(jobB, context.Background())
	if err != nil {
		t.Fatalf("second runEngineForJob: %v", err)
	}
	if statsB.cacheHits != 1 || statsB.cacheMisses != 0 {
		t.Fatalf("second run cache stats = hits=%d misses=%d, want hits=1 misses=0", statsB.cacheHits, statsB.cacheMisses)
	}
	if got := networkCalls.Load(); got != 1 {
		t.Fatalf("network calls = %d, want 1", got)
	}
}

func TestRunJobSnapshotsEffectiveProfile(t *testing.T) {
	srv := New(DefaultConfig())
	spy := newSpyJobStore()
	srv.store = spy

	stored, err := srv.store.CreateProfile(StoredProfile{
		Name:   "strict",
		Config: `{"net":{"ipv4":false},"resolver":{"defaults":{"timeout":5}}}`,
	})
	if err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}
	srv.engineRunner = func(_ engine.RunRequest) ([]engine.LogEntry, error) {
		return nil, nil
	}

	job := Job{
		ID:        "job-effective-profile",
		Domain:    "example.com",
		Status:    JobQueued,
		CreatedAt: time.Now().UTC(),
		ProfileID: &stored.ID,
		Overrides: map[string]any{
			"resolver": map[string]any{
				"defaults": map[string]any{
					"timeout": 7,
				},
			},
		},
	}
	if _, err := srv.store.Create(job); err != nil {
		t.Fatalf("create job: %v", err)
	}

	if err := srv.runJob(job.ID); err != nil {
		t.Fatalf("run job: %v", err)
	}

	run, ok := srv.store.GetRun(job.ID)
	if !ok {
		t.Fatal("expected run")
	}
	if run.EffectiveProfile == "" {
		t.Fatal("expected effective_profile snapshot")
	}
	effective, err := profile.FromJSON(run.EffectiveProfile)
	if err != nil {
		t.Fatalf("parse effective profile: %v", err)
	}
	ipv4, err := effective.Get("net.ipv4")
	if err != nil {
		t.Fatalf("get net.ipv4: %v", err)
	}
	if value, ok := ipv4.(bool); !ok || value != false {
		t.Fatalf("expected net.ipv4 false, got %v", ipv4)
	}
	timeout, err := effective.Get("resolver.defaults.timeout")
	if err != nil {
		t.Fatalf("get timeout: %v", err)
	}
	if value, ok := timeout.(int); !ok || value != 7 {
		t.Fatalf("expected timeout 7, got %v", timeout)
	}
}

func TestRunJobPersistsNameserverTimingsForDelegatedNameserversOnly(t *testing.T) {
	srv := New(DefaultConfig())
	spy := newSpyJobStore()
	srv.store = spy
	srv.delegationLookup = func(_ context.Context, domain string) DelegationInfo {
		if domain != "example.com" {
			t.Fatalf("unexpected domain %q", domain)
		}
		return DelegationInfo{
			Nameservers: []DelegationNS{
				{NS: "ns1.example.com", IP: "192.0.2.10"},
				{NS: "ns2.example.com", IP: "192.0.2.20"},
			},
		}
	}
	srv.engineRunner = func(req engine.RunRequest) ([]engine.LogEntry, error) {
		req.NameserverCache.RecordQueryTime("ns1.example.com/192.0.2.10", 20*time.Millisecond)
		req.NameserverCache.RecordQueryTime("ns1.example.com/192.0.2.10", 40*time.Millisecond)
		req.NameserverCache.RecordQueryTime("ns2.example.com/192.0.2.20", 15*time.Millisecond)
		req.NameserverCache.RecordQueryTime("a.gtld-servers.net/192.5.6.30", 100*time.Millisecond)
		return []engine.LogEntry{{Module: "Nameserver", Testcase: "Nameserver01", Tag: "NO_RESPONSE", Level: "INFO"}}, nil
	}

	job := Job{
		ID:        "job-ns-timings",
		Domain:    "example.com",
		Status:    JobQueued,
		CreatedAt: time.Now().UTC(),
	}
	if _, err := srv.store.Create(job); err != nil {
		t.Fatalf("create job: %v", err)
	}

	if err := srv.runJob(job.ID); err != nil {
		t.Fatalf("runJob: %v", err)
	}

	result, ok := srv.store.GetResult(job.ID)
	if !ok {
		t.Fatal("expected result")
	}
	if len(result.NameserverTimings) != 2 {
		t.Fatalf("nameserver timings len = %d, want 2", len(result.NameserverTimings))
	}
	if result.NameserverTimings[0].Nameserver != "ns1.example.com" {
		t.Fatalf("first nameserver = %q, want ns1.example.com", result.NameserverTimings[0].Nameserver)
	}
	if result.NameserverTimings[0].Count != 2 {
		t.Fatalf("first count = %d, want 2", result.NameserverTimings[0].Count)
	}
	if result.NameserverTimings[0].AvgMS != 30 {
		t.Fatalf("first avg_ms = %v, want 30", result.NameserverTimings[0].AvgMS)
	}
	if result.NameserverTimings[1].Nameserver != "ns2.example.com" {
		t.Fatalf("second nameserver = %q, want ns2.example.com", result.NameserverTimings[1].Nameserver)
	}
}
