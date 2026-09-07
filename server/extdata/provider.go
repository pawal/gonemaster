// Package extdata serves small public reference datasets and per-object
// records fetched from third parties. Lookups read an in-memory cache and
// never block on the network; a background worker does the fetching.
package extdata

import (
	"container/list"
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// State reports what a Lookup could serve.
type State string

const (
	StateFresh       State = "fresh"
	StateStale       State = "stale"
	StatePending     State = "pending"
	StateUnavailable State = "unavailable"
	StateDisabled    State = "disabled"
)

// Item is one cached object plus its provenance.
type Item struct {
	Value     any
	FetchedAt time.Time
	SourceURL string
	// Version is the dataset's own version marker, empty for records.
	Version string
}

// Sources holds the URLs the datasets are fetched from.
type Sources struct {
	IANATLDs      string
	RDAPBootstrap string
}

// DefaultSources returns the upstream IANA locations.
func DefaultSources() Sources {
	return Sources{
		IANATLDs:      "https://data.iana.org/TLD/tlds-alpha-by-domain.txt",
		RDAPBootstrap: "https://data.iana.org/rdap/dns.json",
	}
}

// Config controls the provider. The zero value is disabled.
type Config struct {
	Enabled              bool
	RefreshInterval      time.Duration
	RecordTTL            time.Duration
	NegativeTTL          time.Duration
	Timeout              time.Duration
	MaxRequestsPerMinute int
	MaxCachedRecords     int
	Sources              Sources
	UserAgent            string
	// Transport is injected by tests; nil builds a guarded transport.
	Transport http.RoundTripper
	Logger    *slog.Logger

	now func() time.Time
	// allowInsecure relaxes the HTTPS and address guards for tests.
	allowInsecure bool
}

// DefaultConfig returns the provider defaults, disabled.
func DefaultConfig() Config {
	return Config{
		Enabled:              false,
		RefreshInterval:      24 * time.Hour,
		RecordTTL:            168 * time.Hour,
		NegativeTTL:          time.Hour,
		Timeout:              10 * time.Second,
		MaxRequestsPerMinute: 30,
		MaxCachedRecords:     20000,
		Sources:              DefaultSources(),
	}
}

// queueCapacity bounds the pending fetch backlog. Overflow is dropped; the
// next Lookup re-enqueues.
const queueCapacity = 256

type cacheEntry struct {
	key string
	// pinned entries (datasets) are never evicted.
	pinned   bool
	loaded   bool
	item     Item
	failedAt time.Time
	etag     string
	lastMod  string
}

// Provider caches reference data and refreshes it in the background.
type Provider struct {
	cfg    Config
	client *http.Client
	now    func() time.Time
	logger *slog.Logger
	bucket *tokenBucket

	mu       sync.Mutex
	entries  map[string]*list.Element
	lru      *list.List
	inflight map[string]struct{}
	records  int

	queue chan string

	errMu    sync.Mutex
	lastErr  string
	lastErrT time.Time
}

// New builds a provider from cfg. It is inert until Start is called and a
// nil-safe no-op when cfg.Enabled is false.
func New(cfg Config) *Provider {
	if cfg.RefreshInterval <= 0 {
		cfg.RefreshInterval = 24 * time.Hour
	}
	if cfg.RecordTTL <= 0 {
		cfg.RecordTTL = 168 * time.Hour
	}
	if cfg.NegativeTTL <= 0 {
		cfg.NegativeTTL = time.Hour
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.MaxRequestsPerMinute <= 0 {
		cfg.MaxRequestsPerMinute = 30
	}
	if cfg.MaxCachedRecords <= 0 {
		cfg.MaxCachedRecords = 20000
	}
	if cfg.Sources.IANATLDs == "" {
		cfg.Sources.IANATLDs = DefaultSources().IANATLDs
	}
	if cfg.Sources.RDAPBootstrap == "" {
		cfg.Sources.RDAPBootstrap = DefaultSources().RDAPBootstrap
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "gonemaster"
	}
	if cfg.now == nil {
		cfg.now = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	p := &Provider{
		cfg:      cfg,
		now:      cfg.now,
		logger:   cfg.Logger,
		entries:  map[string]*list.Element{},
		lru:      list.New(),
		inflight: map[string]struct{}{},
		queue:    make(chan string, queueCapacity),
	}
	p.bucket = newTokenBucket(cfg.MaxRequestsPerMinute, time.Minute, cfg.now)
	p.client = newHTTPClient(cfg)
	return p
}

// Enabled reports whether the provider serves anything.
func (p *Provider) Enabled() bool {
	return p != nil && p.cfg.Enabled
}

// Start runs the fetch worker and the dataset refresh ticker until ctx is
// done. It primes both datasets immediately.
func (p *Provider) Start(ctx context.Context) {
	if !p.Enabled() {
		return
	}
	go p.worker(ctx)
	for _, name := range datasetNames() {
		p.enqueue(datasetKey(name))
	}
	go p.refreshLoop(ctx)
}

// Lookup returns the cached item for key and what state it is in. It never
// blocks: a miss or a stale entry enqueues a fetch and returns immediately.
func (p *Provider) Lookup(key string) (Item, State) {
	if !p.Enabled() {
		return Item{}, StateDisabled
	}
	kind, ok := kindOf(key)
	if !ok {
		return Item{}, StateUnavailable
	}
	// A record with no resolvable URL is not worth queueing.
	if kind == kindRecord {
		if _, st := p.recordURL(key); st != urlReady {
			if st == urlPending {
				// Schedules the bootstrap under its own negative TTL.
				p.Lookup(datasetKey(datasetRDAPBootstrap))
				return Item{}, StatePending
			}
			return Item{}, StateUnavailable
		}
	}

	now := p.now()
	p.mu.Lock()
	elem, found := p.entries[key]
	var e *cacheEntry
	if found {
		e = elem.Value.(*cacheEntry)
		p.lru.MoveToFront(elem)
	}
	p.mu.Unlock()

	switch {
	case e == nil:
		p.enqueue(key)
		return Item{}, StatePending
	case !e.loaded:
		if now.Sub(e.failedAt) < p.cfg.NegativeTTL {
			return Item{}, StateUnavailable
		}
		p.enqueue(key)
		return Item{}, StatePending
	case now.Sub(e.item.FetchedAt) >= p.ttlFor(kind):
		// A failing refresh backs off too, so a dead source is not
		// re-contacted on every page view.
		if now.Sub(e.failedAt) >= p.cfg.NegativeTTL {
			p.enqueue(key)
		}
		return e.item, StateStale
	default:
		return e.item, StateFresh
	}
}

// ttlFor returns the age at which an entry of this kind is stale.
func (p *Provider) ttlFor(kind entryKind) time.Duration {
	if kind == kindDataset {
		return p.cfg.RefreshInterval
	}
	return p.cfg.RecordTTL
}

// enqueue schedules a fetch unless one is already in flight for key.
func (p *Provider) enqueue(key string) {
	p.mu.Lock()
	if _, busy := p.inflight[key]; busy {
		p.mu.Unlock()
		return
	}
	p.inflight[key] = struct{}{}
	p.mu.Unlock()

	select {
	case p.queue <- key:
	default:
		// Backlog full: drop and let the next lookup retry.
		p.mu.Lock()
		delete(p.inflight, key)
		p.mu.Unlock()
	}
}

func (p *Provider) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case key := <-p.queue:
			p.fetchKey(ctx, key)
		}
	}
}

func (p *Provider) refreshLoop(ctx context.Context) {
	ticker := time.NewTicker(p.cfg.RefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, name := range datasetNames() {
				p.enqueue(datasetKey(name))
			}
		}
	}
}

// store records a successful fetch.
func (p *Provider) store(key string, kind entryKind, item Item, etag, lastMod string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	e := p.entryLocked(key, kind)
	e.loaded = true
	e.item = item
	e.failedAt = time.Time{}
	e.etag = etag
	e.lastMod = lastMod
	p.evictLocked()
}

// touch refreshes an entry's fetch time without changing its value, for a
// 304 answer.
func (p *Provider) touch(key string, at time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	elem, ok := p.entries[key]
	if !ok {
		return
	}
	e := elem.Value.(*cacheEntry)
	e.item.FetchedAt = at
}

// fail records a failed fetch; the negative TTL starts now.
func (p *Provider) fail(key string, kind entryKind, err error) {
	now := p.now()
	p.mu.Lock()
	e := p.entryLocked(key, kind)
	e.failedAt = now
	p.evictLocked()
	p.mu.Unlock()

	p.errMu.Lock()
	p.lastErr = key + ": " + err.Error()
	p.lastErrT = now
	p.errMu.Unlock()
	p.logger.Debug("external data fetch failed", "key", key, "err", err)
}

// entryLocked returns the entry for key, creating it if needed.
func (p *Provider) entryLocked(key string, kind entryKind) *cacheEntry {
	if elem, ok := p.entries[key]; ok {
		p.lru.MoveToFront(elem)
		return elem.Value.(*cacheEntry)
	}
	e := &cacheEntry{key: key, pinned: kind == kindDataset}
	p.entries[key] = p.lru.PushFront(e)
	if !e.pinned {
		p.records++
	}
	return e
}

// evictLocked drops least-recently-used records past the cache cap.
func (p *Provider) evictLocked() {
	for p.records > p.cfg.MaxCachedRecords {
		elem := p.lru.Back()
		for elem != nil && elem.Value.(*cacheEntry).pinned {
			elem = elem.Prev()
		}
		if elem == nil {
			return
		}
		e := elem.Value.(*cacheEntry)
		p.lru.Remove(elem)
		delete(p.entries, e.key)
		p.records--
	}
}

// cachedValue returns a loaded entry's value without scheduling a fetch.
func (p *Provider) cachedValue(key string) (any, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	elem, ok := p.entries[key]
	if !ok {
		return nil, false
	}
	e := elem.Value.(*cacheEntry)
	if !e.loaded {
		return nil, false
	}
	return e.item.Value, true
}

// conditionsFor returns the validators to send on a refresh.
func (p *Provider) conditionsFor(key string) (etag, lastMod string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	elem, ok := p.entries[key]
	if !ok {
		return "", ""
	}
	e := elem.Value.(*cacheEntry)
	if !e.loaded {
		return "", ""
	}
	return e.etag, e.lastMod
}

// DatasetStatus is one dataset's cache state.
type DatasetStatus struct {
	Name      string     `json:"name"`
	State     string     `json:"state"`
	FetchedAt *time.Time `json:"fetched_at,omitempty"`
	Version   string     `json:"version,omitempty"`
	Entries   int        `json:"entries,omitempty"`
}

// Status is the operator-facing view of the provider.
type Status struct {
	Enabled       bool            `json:"enabled"`
	Datasets      []DatasetStatus `json:"datasets,omitempty"`
	CachedRecords int             `json:"cached_records"`
	QueueDepth    int             `json:"queue_depth"`
	LastError     string          `json:"last_error,omitempty"`
	LastErrorAt   *time.Time      `json:"last_error_at,omitempty"`
}

// Status reports dataset freshness, cache size and the last fetch error.
func (p *Provider) Status() Status {
	if p == nil {
		return Status{}
	}
	if !p.cfg.Enabled {
		return Status{Enabled: false}
	}
	out := Status{Enabled: true, QueueDepth: len(p.queue)}
	for _, name := range datasetNames() {
		item, state := p.Lookup(datasetKey(name))
		ds := DatasetStatus{Name: name, State: string(state), Version: item.Version}
		if !item.FetchedAt.IsZero() {
			at := item.FetchedAt
			ds.FetchedAt = &at
			ds.Entries = datasetSize(item.Value)
		}
		out.Datasets = append(out.Datasets, ds)
	}
	p.mu.Lock()
	out.CachedRecords = p.records
	p.mu.Unlock()
	p.errMu.Lock()
	out.LastError = p.lastErr
	if !p.lastErrT.IsZero() {
		at := p.lastErrT
		out.LastErrorAt = &at
	}
	p.errMu.Unlock()
	return out
}

type entryKind int

const (
	kindDataset entryKind = iota
	kindRecord
)

const (
	datasetPrefix = "dataset:"
	recordPrefix  = "rdap_domain:"
)

func datasetKey(name string) string { return datasetPrefix + name }

// RDAPDomainKey is the cache key for one domain's RDAP record.
func RDAPDomainKey(domain string) string { return recordPrefix + normalizeDomain(domain) }

// kindOf classifies a cache key.
func kindOf(key string) (entryKind, bool) {
	switch {
	case strings.HasPrefix(key, datasetPrefix):
		if _, ok := datasetByName(strings.TrimPrefix(key, datasetPrefix)); !ok {
			return 0, false
		}
		return kindDataset, true
	case strings.HasPrefix(key, recordPrefix):
		if strings.TrimPrefix(key, recordPrefix) == "" {
			return 0, false
		}
		return kindRecord, true
	}
	return 0, false
}
