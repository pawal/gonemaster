package analysis

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"golang.org/x/sync/errgroup"

	serverpkg "codeberg.org/pawal/gonemaster/server"
)

// enrichmentConcurrency caps the number of in-flight external enricher
// lookups per run. Cymru / whois backends are DNS- or TCP-bound and don't
// benefit from unlimited fanout; 16 is comfortable for a single run
// without hammering upstream servers.
const enrichmentConcurrency = 16

var (
	ErrRunNotFound = errors.New("analysis projector run not found")
)

const projectorVersion = "v1"

// WriteStore is the subset of the store surface used to persist one run's
// projected facts. Aliased to serverpkg.AnalysisWriteStore so this package
// does not have to re-declare the same method set; the alias also keeps
// server free of any import on this package (which would cycle).
type WriteStore = serverpkg.AnalysisWriteStore

// Store is the minimal backing store surface needed by the Phase 2 projector
// loading and cohort-resolution logic.
type Store interface {
	GetRun(id string) (serverpkg.Run, bool)
	QueryEntries(filter serverpkg.EntryFilter) serverpkg.EntryList
	GetDomainTags(domainID int64) []string
	ListAnalysisCohorts() []serverpkg.AnalysisCohort
	WriteStore
}

// txCapableStore is the optional capability a Store may implement to let
// ProjectLoaded batch one run's writes into a single transaction. When the
// backing store satisfies this interface the projector runs the whole set
// of upserts + replace-blocks for one run as one atomic commit; otherwise
// it falls back to running the writes against the plain Store.
type txCapableStore interface {
	WithAnalysisWriteTx(fn func(WriteStore) error) error
}

// Projector loads completed runs and resolves which analysis-enabled cohorts
// should receive projected facts for those runs.
type Projector struct {
	store    Store
	enricher Enricher
}

// SetEnricher installs an optional per-address / per-ASN enrichment source.
// Enrichment runs during ProjectLoaded and fills in fields the engine logs
// do not carry (per-address ASN + prefix, per-ASN label). Passing nil
// disables enrichment.
func (p *Projector) SetEnricher(enricher Enricher) {
	if p == nil {
		return
	}
	p.enricher = enricher
}

// RunInput is the fully loaded source material for projecting one completed run.
type RunInput struct {
	Run               serverpkg.Run
	Entries           []serverpkg.Entry
	DomainTags        []string
	MatchingCohorts   []serverpkg.AnalysisCohort
	NameserverTimings []serverpkg.NameserverTiming
}

// NewProjector creates a projector backed by the given store.
func NewProjector(store Store) *Projector {
	return &Projector{store: store}
}

// LoadCompletedRun loads the completed run, all of its entries, its domain tags,
// and the analysis-enabled cohorts that match those tags. Callers that are
// looping over many runs should use LoadCompletedRunWithCatalog and hoist the
// ListAnalysisCohorts call out of the hot loop.
func (p *Projector) LoadCompletedRun(runID string) (RunInput, error) {
	return p.LoadCompletedRunWithCatalog(runID, p.store.ListAnalysisCohorts())
}

// LoadCompletedRunWithCatalog is LoadCompletedRun but uses a caller-supplied
// cohort catalog instead of fetching it. Rebuild loops reuse one catalog
// across all runs — that cohort list does not change during a rebuild, and
// re-reading it per run dominates the query count on large tags.
func (p *Projector) LoadCompletedRunWithCatalog(runID string, catalog []serverpkg.AnalysisCohort) (RunInput, error) {
	run, ok := p.store.GetRun(runID)
	if !ok {
		return RunInput{}, fmt.Errorf("%w: %s", ErrRunNotFound, runID)
	}

	limit := run.EntryCount
	if limit <= 0 {
		limit = 1
	}
	entryList := p.store.QueryEntries(serverpkg.EntryFilter{
		RunID:  run.ID,
		Limit:  limit,
		Offset: 0,
	})
	tags := p.store.GetDomainTags(run.DomainID)
	cohorts := MatchAnalysisEnabledCohorts(tags, catalog)

	return RunInput{
		Run:               run,
		Entries:           entryList.Items,
		DomainTags:        tags,
		MatchingCohorts:   cohorts,
		NameserverTimings: run.NameserverTimings,
	}, nil
}

type extractedEndpoint struct {
	nameserver string
	address    string
	role       string
	source     string
	family     string
	avgMS      float64
	minMS      float64
	maxMS      float64
	queryCount int
}

type extractedAddressFact struct {
	address      string
	family       string
	prefix       string
	prefixFamily string
	asn          *int64
	status       string
	source       string
}

// ProjectRun loads a completed run and projects its facts. Callers that already
// hold a RunInput should use ProjectLoaded to avoid reloading the run.
func (p *Projector) ProjectRun(runID string) error {
	input, err := p.LoadCompletedRun(runID)
	if err != nil {
		return err
	}
	return p.ProjectLoaded(input)
}

// ProjectLoaded extracts and persists all currently-implemented analysis facts
// for one completed run using pre-loaded input. Re-running the same run is
// idempotent because the backing store helpers upsert entities and replace
// per-run/per-cohort fact rows. When the backing store supports it, the
// whole run's writes are batched into a single transaction so the projector
// pays one commit per run instead of one per upsert.
//
// Enrichment (DNS / whois lookups) runs *before* the transaction opens so
// the DB connection is never held open waiting on an external network
// call. That keeps the write transaction short and non-blocking for
// concurrent work on the same connection pool.
func (p *Projector) ProjectLoaded(input RunInput) error {
	if len(input.MatchingCohorts) == 0 {
		return nil
	}
	prepared := p.prepareRun(input)
	if tx, ok := p.store.(txCapableStore); ok {
		return tx.WithAnalysisWriteTx(func(writer WriteStore) error {
			return p.writePrepared(prepared, writer)
		})
	}
	return p.writePrepared(prepared, p.store)
}

// preparedRun is the output of the extract + enrich phase, ready to be
// written inside one transaction. Computed outside the tx so the tx only
// covers database work.
type preparedRun struct {
	input        RunInput
	endpoints    []extractedEndpoint
	addressFacts []extractedAddressFact
	domainASNs   []extractedDomainASN
	asnLabels    map[int64]string
	tagSummaries []tagSummary
	domainFacts  []extractedDomainFact
}

// tagSummary is a per-(tag, testcase) aggregate for one run. Rendered by
// writePrepared into serverpkg.AnalysisRunTagSummary rows — one per
// matching cohort.
type tagSummary struct {
	tag         string
	module      string
	testcase    string
	level       string
	occurrences int
}

// extractTagSummaries groups run entries by (tag, testcase) so the
// landing page's top-findings panel can be served from a materialized
// table instead of rescanning the entries table per domain on every
// request. Level is the worst severity observed for the bucket.
func extractTagSummaries(input RunInput) []tagSummary {
	type key struct {
		tag      string
		testcase string
	}
	buckets := map[key]*tagSummary{}
	for _, entry := range input.Entries {
		tag := strings.TrimSpace(entry.Tag)
		if tag == "" {
			continue
		}
		k := key{tag: tag, testcase: entry.Testcase}
		b, ok := buckets[k]
		if !ok {
			b = &tagSummary{
				tag:      tag,
				module:   entry.Module,
				testcase: entry.Testcase,
				level:    entry.Level,
			}
			buckets[k] = b
		}
		if severityRank(entry.Level) > severityRank(b.level) {
			b.level = entry.Level
		}
		b.occurrences++
	}
	out := make([]tagSummary, 0, len(buckets))
	for _, b := range buckets {
		out = append(out, *b)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].tag != out[j].tag {
			return out[i].tag < out[j].tag
		}
		return out[i].testcase < out[j].testcase
	})
	return out
}

// severityRank mirrors the helper used by handlers_public_analysis_*;
// kept private here so the projector can fold per-entry levels into a
// per-bucket worst-seen level without importing the server package.
func severityRank(level string) int {
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case "CRITICAL":
		return 5
	case "ERROR":
		return 4
	case "WARNING":
		return 3
	case "NOTICE":
		return 2
	case "INFO":
		return 1
	}
	return 0
}

// prepareRun does all the pure-Go extraction and all the external
// enrichment lookups for one run. Must run outside any DB transaction.
func (p *Projector) prepareRun(input RunInput) preparedRun {
	endpoints := p.extractNameserverEndpoints(input)
	addressFacts := p.extractAddressFacts(input)
	addressFacts = p.applyEnrichment(endpoints, addressFacts)
	domainASNs := p.extractDomainASNs(input)
	// The aggregate IPV4_/IPV6_*_ASN engine entries carry every ASN the
	// delegation chain touched — including parent-side registry ASNs
	// that have no authoritative address in this cohort. Drop those so
	// the ASN list never shows ASNs with zero address / nameserver
	// linkage. This filter only kicks in when address-level enrichment
	// is populated; otherwise we still need the aggregate for coverage.
	domainASNs = restrictDomainASNsToAuthoritative(domainASNs, addressFacts)
	asnLabels := p.resolveASNLabels(addressFacts, domainASNs)
	tagSummaries := extractTagSummaries(input)
	domainFacts := extractDomainFacts(input)
	return preparedRun{
		input:        input,
		endpoints:    endpoints,
		addressFacts: addressFacts,
		domainASNs:   domainASNs,
		asnLabels:    asnLabels,
		tagSummaries: tagSummaries,
		domainFacts:  domainFacts,
	}
}

// writePrepared executes the DB writes for a prepared run. All enrichment
// has already happened in prepareRun, so this runs entirely against the
// transaction with no outbound network calls.
func (p *Projector) writePrepared(pr preparedRun, w WriteStore) error {
	input := pr.input
	endpoints := pr.endpoints
	addressFacts := pr.addressFacts
	domainASNs := pr.domainASNs
	asnLabels := pr.asnLabels
	tagSummaries := pr.tagSummaries
	domainFacts := pr.domainFacts

	nameserverIDs := map[string]int64{}
	addressIDs := map[string]int64{}
	prefixIDs := map[string]int64{}

	for _, endpoint := range endpoints {
		ns, err := w.UpsertAnalysisNameserver(endpoint.nameserver, input.Run.FinishedAt)
		if err != nil {
			return fmt.Errorf("upsert nameserver %q: %w", endpoint.nameserver, err)
		}
		nameserverIDs[endpoint.nameserver] = ns.ID
		if endpoint.address == "" {
			// Synthetic delegation-only endpoint: the NS name is part of
			// the zone's authoritative set but the engine never produced
			// an address for it. Leave AddressID at the zero sentinel so
			// the fact row records the nameserver membership without a
			// spurious analysis_addresses entry for "".
			continue
		}
		addr, err := w.UpsertAnalysisAddress(endpoint.address, endpoint.family, input.Run.FinishedAt)
		if err != nil {
			return fmt.Errorf("upsert address %q: %w", endpoint.address, err)
		}
		addressIDs[endpoint.address] = addr.ID
	}
	for _, fact := range addressFacts {
		addr, err := w.UpsertAnalysisAddress(fact.address, fact.family, input.Run.FinishedAt)
		if err != nil {
			return fmt.Errorf("upsert address fact %q: %w", fact.address, err)
		}
		addressIDs[fact.address] = addr.ID
		if fact.prefix != "" {
			prefix, err := w.UpsertAnalysisPrefix(fact.prefix, fact.prefixFamily, input.Run.FinishedAt)
			if err != nil {
				return fmt.Errorf("upsert prefix %q: %w", fact.prefix, err)
			}
			prefixIDs[fact.prefix] = prefix.ID
		}
		if fact.asn != nil {
			if _, err := w.UpsertAnalysisASN(*fact.asn, asnLabels[*fact.asn], input.Run.FinishedAt); err != nil {
				return fmt.Errorf("upsert asn %d: %w", *fact.asn, err)
			}
		}
	}
	for _, da := range domainASNs {
		if _, err := w.UpsertAnalysisASN(da.asn, asnLabels[da.asn], input.Run.FinishedAt); err != nil {
			return fmt.Errorf("upsert asn %d: %w", da.asn, err)
		}
	}

	summary := deriveRunDomainSummary(input, endpoints, addressFacts)

	for _, cohort := range input.MatchingCohorts {
		nsRows := make([]serverpkg.AnalysisRunNameserverEndpoint, 0, len(endpoints))
		for _, endpoint := range endpoints {
			nsRows = append(nsRows, serverpkg.AnalysisRunNameserverEndpoint{
				CohortID:     cohort.ID,
				RunID:        input.Run.ID,
				DomainID:     input.Run.DomainID,
				NameserverID: nameserverIDs[endpoint.nameserver],
				AddressID:    addressIDs[endpoint.address],
				Role:         endpoint.role,
				Source:       endpoint.source,
				Family:       endpoint.family,
				AvgMS:        endpoint.avgMS,
				MinMS:        endpoint.minMS,
				MaxMS:        endpoint.maxMS,
				QueryCount:   endpoint.queryCount,
			})
		}
		if err := w.ReplaceAnalysisRunNSEndpoints(cohort.ID, input.Run.ID, nsRows); err != nil {
			return fmt.Errorf("replace nameserver endpoints for cohort %d: %w", cohort.ID, err)
		}

		addrRows := make([]serverpkg.AnalysisRunAddressASN, 0, len(addressFacts))
		for _, fact := range addressFacts {
			var prefixID *int64
			if fact.prefix != "" {
				v := prefixIDs[fact.prefix]
				prefixID = &v
			}
			addrRows = append(addrRows, serverpkg.AnalysisRunAddressASN{
				CohortID:     cohort.ID,
				RunID:        input.Run.ID,
				DomainID:     input.Run.DomainID,
				AddressID:    addressIDs[fact.address],
				PrefixID:     prefixID,
				ASN:          fact.asn,
				LookupStatus: fact.status,
				Source:       fact.source,
			})
		}
		if err := w.ReplaceAnalysisRunAddressASNs(cohort.ID, input.Run.ID, addrRows); err != nil {
			return fmt.Errorf("replace address facts for cohort %d: %w", cohort.ID, err)
		}

		domainASNRows := make([]serverpkg.AnalysisRunDomainASN, 0, len(domainASNs))
		for _, da := range domainASNs {
			domainASNRows = append(domainASNRows, serverpkg.AnalysisRunDomainASN{
				CohortID: cohort.ID,
				RunID:    input.Run.ID,
				DomainID: input.Run.DomainID,
				ASN:      da.asn,
				Family:   da.family,
				Source:   da.source,
			})
		}
		if err := w.ReplaceAnalysisRunDomainASNs(cohort.ID, input.Run.ID, domainASNRows); err != nil {
			return fmt.Errorf("replace domain asns for cohort %d: %w", cohort.ID, err)
		}

		tagRows := make([]serverpkg.AnalysisRunTagSummary, 0, len(tagSummaries))
		for _, ts := range tagSummaries {
			tagRows = append(tagRows, serverpkg.AnalysisRunTagSummary{
				CohortID:        cohort.ID,
				RunID:           input.Run.ID,
				DomainID:        input.Run.DomainID,
				Tag:             ts.tag,
				Module:          ts.module,
				Testcase:        ts.testcase,
				Level:           ts.level,
				OccurrenceCount: ts.occurrences,
			})
		}
		if err := w.ReplaceAnalysisRunTagSummaries(cohort.ID, input.Run.ID, tagRows); err != nil {
			return fmt.Errorf("replace tag summaries for cohort %d: %w", cohort.ID, err)
		}

		factRows := buildDomainFactRows(cohort.ID, input.Run.ID, input.Run.DomainID, domainFacts)
		if err := w.ReplaceAnalysisRunDomainFacts(cohort.ID, input.Run.ID, factRows); err != nil {
			return fmt.Errorf("replace domain facts for cohort %d: %w", cohort.ID, err)
		}

		summary.CohortID = cohort.ID
		if err := w.UpsertAnalysisRunDomainSummary(summary); err != nil {
			return fmt.Errorf("upsert summary for cohort %d: %w", cohort.ID, err)
		}
		if err := w.SetAnalysisProjectionState(serverpkg.AnalysisProjectionState{
			CohortID:         cohort.ID,
			RunID:            input.Run.ID,
			ProjectorVersion: projectorVersion,
			Status:           serverpkg.AnalysisMaterializationReady,
			ProjectedAt:      input.Run.FinishedAt,
		}); err != nil {
			return fmt.Errorf("set projection state for cohort %d: %w", cohort.ID, err)
		}
	}

	return nil
}

func deriveRunDomainSummary(input RunInput, endpoints []extractedEndpoint, addressFacts []extractedAddressFact) serverpkg.AnalysisRunDomainSummary {
	nameservers := map[string]struct{}{}
	type nsAddrKey struct{ ns, addr string }
	endpointPairs := map[nsAddrKey]struct{}{}
	asns := map[int64]struct{}{}
	prefixes := map[string]struct{}{}

	// Skip parent-role endpoints (root/registry servers seen while traversing
	// the delegation chain). They are not the domain's own authoritative
	// nameservers, and every downstream /domains and /nameservers view
	// filters them out — counting them in the summary inflated TLD rows by
	// the 13 root servers and their 26 addresses.
	for _, endpoint := range endpoints {
		if endpoint.role == "parent" {
			continue
		}
		nameservers[endpoint.nameserver] = struct{}{}
		// Synthetic delegation-only endpoints carry no address. The NS
		// still counts toward NameserverCount (so CAN_NOT_BE_RESOLVED
		// nameservers are visible on /domains), but nothing to add to
		// the endpoint-pair tally.
		if endpoint.address != "" {
			endpointPairs[nsAddrKey{endpoint.nameserver, endpoint.address}] = struct{}{}
		}
	}
	for _, fact := range addressFacts {
		if fact.asn != nil {
			asns[*fact.asn] = struct{}{}
		}
		if fact.prefix != "" {
			prefixes[fact.prefix] = struct{}{}
		}
	}

	return serverpkg.AnalysisRunDomainSummary{
		RunID:           input.Run.ID,
		DomainID:        input.Run.DomainID,
		Score:           input.Run.Score,
		Grade:           input.Run.Grade,
		NameserverCount: len(nameservers),
		EndpointCount:   len(endpointPairs),
		ASNCount:        len(asns),
		PrefixCount:     len(prefixes),
		WorstLevel:      input.Run.WorstLevel,
	}
}

func (p *Projector) extractNameserverEndpoints(input RunInput) []extractedEndpoint {
	seen := map[string]extractedEndpoint{}
	add := func(item extractedEndpoint) {
		if item.nameserver == "" {
			return
		}
		// Address may be empty when this is a synthetic delegation-only
		// endpoint (NS name appears in the zone's NS set but the engine
		// never produced an address for it — unresolvable, or the
		// address was filtered out). Allow those through so the
		// nameserver shows up on the domain detail; all other sources
		// still require an address.
		if item.address == "" && item.source != "delegation" {
			return
		}
		if item.address != "" {
			if item.family == "" {
				item.family = familyForAddress(item.address)
			}
			if item.family == "" {
				return
			}
		}
		key := item.nameserver + "|" + item.address + "|" + item.role + "|" + item.source
		if existing, ok := seen[key]; ok {
			if existing.queryCount == 0 && item.queryCount > 0 {
				existing.avgMS = item.avgMS
				existing.minMS = item.minMS
				existing.maxMS = item.maxMS
				existing.queryCount = item.queryCount
			}
			seen[key] = existing
			return
		}
		seen[key] = item
	}

	// Authoritative NS name set for the zone under test. Primary source
	// is NameserverTiming: the worker emits one row per delegated target
	// including unreachable / unresolved ones, so reading the names off
	// those rows gives us the complete delegation set. Falls back to the
	// Delegation01 tag parser for legacy runs written before the worker
	// emitted status-bearing rows.
	authoritativeNSSet := map[string]struct{}{}
	for _, timing := range input.NameserverTimings {
		ns := normalizeNameserverName(timing.Nameserver)
		if ns == "" {
			continue
		}
		authoritativeNSSet[ns] = struct{}{}
	}
	if len(authoritativeNSSet) == 0 {
		authoritativeNSSet = delegationNSSet(input.Entries)
	}

	childSide := func(source string) bool {
		switch source {
		case "child_servers", "zone_servers", "ns_set_servers":
			return true
		}
		return false
	}
	classify := func(source, ns, addr string) string {
		if len(authoritativeNSSet) > 0 {
			if _, ok := authoritativeNSSet[ns]; ok {
				return "authoritative"
			}
			return "parent"
		}
		// Last-resort fallback: no timings, no delegation entries —
		// trust only the explicit child-side source keys, everything
		// else defaults to parent so root-server traversal doesn't
		// leak into the authoritative view.
		if childSide(source) {
			return "authoritative"
		}
		return "parent"
	}

	for _, timing := range input.NameserverTimings {
		ns := normalizeNameserverName(timing.Nameserver)
		addr := strings.TrimSpace(timing.Address)
		if ns == "" {
			continue
		}
		if addr == "" {
			// Unresolved target — surface the NS name without an
			// address so the domain detail page still lists it.
			add(extractedEndpoint{
				nameserver: ns,
				address:    "",
				role:       "authoritative",
				source:     "delegation",
			})
			continue
		}
		add(extractedEndpoint{
			nameserver: ns,
			address:    addr,
			role:       "authoritative",
			source:     "timings",
			family:     familyForAddress(addr),
			avgMS:      timing.AvgMS,
			minMS:      timing.MinMS,
			maxMS:      timing.MaxMS,
			queryCount: timing.Count,
		})
	}

	for _, entry := range input.Entries {
		if ns, addr, ok := singularEndpointFromArgs(entry.Args); ok {
			add(extractedEndpoint{
				nameserver: ns,
				address:    addr,
				role:       classify("entry", ns, addr),
				source:     "entry",
			})
		}
		for _, sourceKey := range []string{"servers", "parent_servers", "child_servers", "zone_servers", "ns_set_servers"} {
			for _, endpoint := range endpointsFromArgs(entry.Args[sourceKey]) {
				endpoint.role = classify(sourceKey, endpoint.nameserver, endpoint.address)
				endpoint.source = sourceKey
				add(endpoint)
			}
		}
	}

	// Emit one synthetic endpoint per authoritative NS name that never
	// produced an (ns, addr) pair. These are typically
	// CAN_NOT_BE_RESOLVED cases (no A/AAAA for the NS hostname). The
	// address is empty, the source is "delegation", and downstream
	// readers render them as NS-without-address rows.
	if len(authoritativeNSSet) > 0 {
		covered := map[string]struct{}{}
		for _, ep := range seen {
			if ep.role == "authoritative" && ep.address != "" {
				covered[ep.nameserver] = struct{}{}
			}
		}
		for ns := range authoritativeNSSet {
			if _, done := covered[ns]; done {
				continue
			}
			add(extractedEndpoint{
				nameserver: ns,
				address:    "",
				role:       "authoritative",
				source:     "delegation",
			})
		}
	}

	out := make([]extractedEndpoint, 0, len(seen))
	for _, item := range seen {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].nameserver != out[j].nameserver {
			return out[i].nameserver < out[j].nameserver
		}
		if out[i].address != out[j].address {
			return out[i].address < out[j].address
		}
		if out[i].role != out[j].role {
			return out[i].role < out[j].role
		}
		return out[i].source < out[j].source
	})
	return out
}

func (p *Projector) extractAddressFacts(input RunInput) []extractedAddressFact {
	facts := map[string]extractedAddressFact{}
	upsert := func(address string) *extractedAddressFact {
		family := familyForAddress(address)
		if family == "" {
			return nil
		}
		existing, ok := facts[address]
		if !ok {
			existing = extractedAddressFact{address: address, family: family}
		}
		facts[address] = existing
		ref := facts[address]
		return &ref
	}
	store := func(fact extractedAddressFact) {
		if fact.status == "" {
			fact.status = "unknown"
		}
		facts[fact.address] = fact
	}

	for _, entry := range input.Entries {
		// Per-address entries (DNSSEC key lookups, authoritative queries, etc.)
		// carry the address at the top level. Apply any prefix/ASN args found
		// alongside them to that single address.
		if address := stringArg(entry.Args, "address"); address != "" {
			fact := upsert(address)
			if fact == nil {
				continue
			}
			if prefixes := stringSliceArg(entry.Args, "prefixes"); len(prefixes) > 0 {
				applyPrefix(fact, prefixes[0])
			}
			if asn, status := asnFromArgs(entry.Args); asn != nil {
				fact.asn = asn
				fact.status = status
				fact.source = "entries"
			}
			store(*fact)
		}
		// CN04_*_SAME_PREFIX entries carry a single prefix at the top level
		// and the set of addresses that share it in a nested `servers` list.
		// The gonemaster engine never emits (address, prefix) in one flat row,
		// so unpack the nested structure here. See plans/tld-analysis.md.
		prefixes := stringSliceArg(entry.Args, "prefixes")
		if len(prefixes) == 0 {
			continue
		}
		for _, endpoint := range endpointsFromArgs(entry.Args["servers"]) {
			if endpoint.address == "" {
				continue
			}
			fact := upsert(endpoint.address)
			if fact == nil {
				continue
			}
			applyPrefix(fact, prefixes[0])
			store(*fact)
		}
	}

	out := make([]extractedAddressFact, 0, len(facts))
	for _, item := range facts {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].address < out[j].address })
	return out
}

// extractedDomainASN is one (ASN, family) observation the engine emitted for
// the tested domain without pairing to a specific address.
type extractedDomainASN struct {
	asn    int64
	family string
	source string
}

// extractDomainASNs pulls the aggregate ASN sets the engine emits per family
// (IPV4_*_ASN / IPV6_*_ASN tags with an `asns` argument). These tell us which
// ASNs the domain's nameserver set spans without tying each ASN to an
// individual address. Deduplicated by (asn, family).
func (p *Projector) extractDomainASNs(input RunInput) []extractedDomainASN {
	seen := map[[2]string]extractedDomainASN{}
	for _, entry := range input.Entries {
		values := numericSliceArg(entry.Args, "asns")
		if len(values) == 0 {
			continue
		}
		family := familyFromASNTag(entry.Tag)
		for _, asn := range values {
			if asn == 0 {
				continue
			}
			key := [2]string{family, fmt.Sprintf("%d", asn)}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = extractedDomainASN{asn: asn, family: family, source: "entries"}
		}
	}
	out := make([]extractedDomainASN, 0, len(seen))
	for _, v := range seen {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].asn != out[j].asn {
			return out[i].asn < out[j].asn
		}
		return out[i].family < out[j].family
	})
	return out
}

// applyEnrichment asks the enricher for (asn, prefix) for every unique
// address the engine mentioned (via endpoints or existing address facts)
// and merges those answers into addressFacts. Facts that already carry
// engine-derived prefix/asn values are left untouched so projection stays
// deterministic when enrichment is disabled or the backend returns nothing.
func (p *Projector) applyEnrichment(endpoints []extractedEndpoint, addressFacts []extractedAddressFact) []extractedAddressFact {
	if p == nil || p.enricher == nil {
		return addressFacts
	}
	ctx := context.Background()

	// Collect the ordered set of unique addresses we want to enrich:
	// every existing address-fact, plus any authoritative endpoint
	// address that doesn't already have a fact. Parent-role endpoints
	// are skipped — they live outside the cohort's authoritative graph
	// and enriching them would only create orphan ASN/prefix rows.
	byIndex := make(map[string]int, len(addressFacts))
	for i, fact := range addressFacts {
		byIndex[fact.address] = i
	}
	addresses := make([]string, 0, len(addressFacts)+len(endpoints))
	seenAddr := make(map[string]struct{}, len(addressFacts)+len(endpoints))
	for _, fact := range addressFacts {
		if _, ok := seenAddr[fact.address]; ok {
			continue
		}
		seenAddr[fact.address] = struct{}{}
		addresses = append(addresses, fact.address)
	}
	for _, endpoint := range endpoints {
		if endpoint.address == "" || endpoint.role == "parent" {
			continue
		}
		if _, ok := seenAddr[endpoint.address]; ok {
			continue
		}
		seenAddr[endpoint.address] = struct{}{}
		addresses = append(addresses, endpoint.address)
	}

	// Fan the lookups out concurrently — for a cold enricher cache each
	// EnrichAddress is a network round-trip (DNS to cymru or whois TCP).
	// Doing them serially would dominate wall-clock time for a cohort
	// rebuild. Results are written into position-indexed slots so the
	// merge-into-addressFacts step below runs in deterministic order.
	enrichments := make([]*AddressEnrichment, len(addresses))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(enrichmentConcurrency)
	for i, address := range addresses {
		i, address := i, address
		g.Go(func() error {
			info, ok := p.enricher.EnrichAddress(gctx, address)
			if ok {
				enrichments[i] = &info
			}
			return nil
		})
	}
	_ = g.Wait()

	for i, address := range addresses {
		info := enrichments[i]
		if info == nil {
			continue
		}
		idx, exists := byIndex[address]
		if !exists {
			family := info.Family
			if family == "" {
				family = familyForAddress(address)
			}
			addressFacts = append(addressFacts, extractedAddressFact{
				address: address,
				family:  family,
			})
			idx = len(addressFacts) - 1
			byIndex[address] = idx
		}
		// Mutate via slice index rather than a stored pointer — a prior
		// append may have reallocated the backing array, which would
		// orphan any *extractedAddressFact captured before the growth.
		fact := &addressFacts[idx]
		if fact.family == "" {
			if info.Family != "" {
				fact.family = info.Family
			} else {
				fact.family = familyForAddress(address)
			}
		}
		if fact.prefix == "" && info.Prefix != "" {
			fact.prefix = info.Prefix
			fact.prefixFamily = familyForPrefix(info.Prefix)
			if fact.prefixFamily == "" {
				fact.prefixFamily = fact.family
			}
			fact.source = "enricher"
		}
		if fact.asn == nil && info.ASN != nil {
			v := *info.ASN
			fact.asn = &v
			if fact.status == "" || fact.status == "unknown" {
				fact.status = "ok"
			}
			if fact.source == "" {
				fact.source = "enricher"
			}
		}
	}
	sort.Slice(addressFacts, func(i, j int) bool { return addressFacts[i].address < addressFacts[j].address })
	return addressFacts
}

// resolveASNLabels collects every ASN referenced in this projection and
// asks the enricher for a human-readable label. Cached by the enricher, so
// the cost is one lookup per unique ASN across the entire cohort rebuild.
// Returns an empty map when no enricher is configured — callers must still
// upsert ASNs with an empty label in that case.
func (p *Projector) resolveASNLabels(addressFacts []extractedAddressFact, domainASNs []extractedDomainASN) map[int64]string {
	labels := map[int64]string{}
	if p == nil || p.enricher == nil {
		return labels
	}
	ctx := context.Background()
	seen := map[int64]struct{}{}
	var uniqueASNs []int64
	addASN := func(asn int64) {
		if asn == 0 {
			return
		}
		if _, ok := seen[asn]; ok {
			return
		}
		seen[asn] = struct{}{}
		uniqueASNs = append(uniqueASNs, asn)
	}
	for _, fact := range addressFacts {
		if fact.asn != nil {
			addASN(*fact.asn)
		}
	}
	for _, da := range domainASNs {
		addASN(da.asn)
	}
	if len(uniqueASNs) == 0 {
		return labels
	}

	// Same fanout pattern as address enrichment — ASN label lookups are
	// external (cymru DNS or RIPE whois) and trivially parallelizable.
	resolved := make([]string, len(uniqueASNs))
	hits := make([]bool, len(uniqueASNs))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(enrichmentConcurrency)
	for i, asn := range uniqueASNs {
		i, asn := i, asn
		g.Go(func() error {
			if label, ok := p.enricher.EnrichASNLabel(gctx, asn); ok && label != "" {
				resolved[i] = label
				hits[i] = true
			}
			return nil
		})
	}
	_ = g.Wait()

	for i, asn := range uniqueASNs {
		if hits[i] {
			labels[asn] = resolved[i]
		}
	}
	return labels
}

// restrictDomainASNsToAuthoritative keeps only the aggregate-ASN rows whose
// ASN also shows up as a per-address fact for the same run. That filters
// out parent-side ASNs the engine mentions while traversing delegation
// (e.g. root / TLD registry ASNs) but which own no authoritative address
// in the cohort. If no per-address ASN facts were resolved (enrichment
// disabled or offline), the aggregate list is returned untouched so the
// ASN view still has coverage.
func restrictDomainASNsToAuthoritative(domainASNs []extractedDomainASN, addressFacts []extractedAddressFact) []extractedDomainASN {
	authoritative := map[int64]struct{}{}
	for _, fact := range addressFacts {
		if fact.asn != nil {
			authoritative[*fact.asn] = struct{}{}
		}
	}
	if len(authoritative) == 0 {
		return domainASNs
	}
	out := domainASNs[:0]
	for _, da := range domainASNs {
		if _, ok := authoritative[da.asn]; !ok {
			continue
		}
		out = append(out, da)
	}
	return out
}

// familyFromASNTag maps tags like IPV4_DIFFERENT_ASN / IPV6_ONE_ASN to a
// family label. Returns "" when the tag doesn't carry a family hint.
func familyFromASNTag(tag string) string {
	upper := strings.ToUpper(tag)
	if strings.HasPrefix(upper, "IPV4_") {
		return "ipv4"
	}
	if strings.HasPrefix(upper, "IPV6_") {
		return "ipv6"
	}
	return ""
}

func applyPrefix(fact *extractedAddressFact, prefix string) {
	if fact == nil || prefix == "" || fact.prefix != "" {
		return
	}
	fact.prefix = prefix
	fact.prefixFamily = familyForPrefix(prefix)
	if fact.prefixFamily == "" {
		fact.prefixFamily = fact.family
	}
	if fact.status == "" {
		fact.status = "prefix_only"
	}
	fact.source = "entries"
}

// MatchAnalysisEnabledCohorts returns the tag-backed cohorts that should be
// populated for a run with the given domain tags.
func MatchAnalysisEnabledCohorts(domainTags []string, cohorts []serverpkg.AnalysisCohort) []serverpkg.AnalysisCohort {
	if len(domainTags) == 0 || len(cohorts) == 0 {
		return nil
	}

	tagSet := make(map[string]struct{}, len(domainTags))
	for _, tag := range domainTags {
		tagSet[tag] = struct{}{}
	}

	var matched []serverpkg.AnalysisCohort
	for _, cohort := range cohorts {
		if !cohort.AnalysisEnabled {
			continue
		}
		if cohort.SourceType != "tag" {
			continue
		}
		if _, ok := tagSet[cohort.SourceTag]; !ok {
			continue
		}
		matched = append(matched, cohort)
	}

	sort.Slice(matched, func(i, j int) bool {
		if matched[i].SortOrder != matched[j].SortOrder {
			return matched[i].SortOrder < matched[j].SortOrder
		}
		if matched[i].Label != matched[j].Label {
			return matched[i].Label < matched[j].Label
		}
		return matched[i].SourceTag < matched[j].SourceTag
	})

	return matched
}

func normalizeNameserverName(name string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), ".")
}

func stringArg(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	v, _ := args[key].(string)
	return strings.TrimSpace(v)
}

func singularEndpointFromArgs(args map[string]any) (string, string, bool) {
	ns := normalizeNameserverName(stringArg(args, "ns"))
	addr := stringArg(args, "address")
	if ns == "" || addr == "" {
		return "", "", false
	}
	return ns, addr, true
}

// delegationNSSet parses the canonical authoritative NS name set from the
// run's Delegation01 entries. Prefers the child-side tag
// (ENOUGH_NS_CHILD / NOT_ENOUGH_NS_CHILD), which reflects what the zone
// itself publishes; falls back to the parent-side
// (ENOUGH_NS_DEL / NOT_ENOUGH_NS_DEL) when no child-side entry was
// emitted. Returns an empty map when the run predates Delegation01
// tagging or the tags carry no usable servers list.
func delegationNSSet(entries []serverpkg.Entry) map[string]struct{} {
	child := map[string]struct{}{}
	parent := map[string]struct{}{}
	for _, entry := range entries {
		var bucket map[string]struct{}
		switch entry.Tag {
		case "ENOUGH_NS_CHILD", "NOT_ENOUGH_NS_CHILD":
			bucket = child
		case "ENOUGH_NS_DEL", "NOT_ENOUGH_NS_DEL":
			bucket = parent
		default:
			continue
		}
		for _, name := range nsNamesFromServersArg(entry.Args["servers"]) {
			bucket[name] = struct{}{}
		}
	}
	if len(child) > 0 {
		return child
	}
	return parent
}

// nsNamesFromServersArg extracts just the `ns` field from a servers=[{ns:...}]
// args value. Used for delegation-level NS lists where no address is
// carried on the entry.
func nsNamesFromServersArg(raw any) []string {
	if raw == nil {
		return nil
	}
	take := func(items []map[string]any) []string {
		out := make([]string, 0, len(items))
		for _, m := range items {
			name := normalizeNameserverName(stringArg(m, "ns"))
			if name != "" {
				out = append(out, name)
			}
		}
		return out
	}
	switch list := raw.(type) {
	case []map[string]any:
		return take(list)
	case []any:
		converted := make([]map[string]any, 0, len(list))
		for _, item := range list {
			if m, ok := item.(map[string]any); ok {
				converted = append(converted, m)
			}
		}
		return take(converted)
	}
	return nil
}

func endpointsFromArgs(raw any) []extractedEndpoint {
	var out []extractedEndpoint
	list, ok := raw.([]any)
	if !ok {
		if typed, ok := raw.([]map[string]any); ok {
			for _, item := range typed {
				ns := normalizeNameserverName(stringArg(item, "ns"))
				addr := stringArg(item, "address")
				if ns == "" || addr == "" {
					continue
				}
				out = append(out, extractedEndpoint{nameserver: ns, address: addr, family: familyForAddress(addr)})
			}
		}
		return out
	}
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		ns := normalizeNameserverName(stringArg(m, "ns"))
		addr := stringArg(m, "address")
		if ns == "" || addr == "" {
			continue
		}
		out = append(out, extractedEndpoint{nameserver: ns, address: addr, family: familyForAddress(addr)})
	}
	return out
}

func familyForAddress(address string) string {
	addr, err := netip.ParseAddr(strings.TrimSpace(address))
	if err != nil {
		return ""
	}
	if addr.Is4() {
		return "ipv4"
	}
	if addr.Is6() {
		return "ipv6"
	}
	return ""
}

func familyForPrefix(prefix string) string {
	p, err := netip.ParsePrefix(strings.TrimSpace(prefix))
	if err != nil {
		return ""
	}
	if p.Addr().Is4() {
		return "ipv4"
	}
	if p.Addr().Is6() {
		return "ipv6"
	}
	return ""
}

func stringSliceArg(args map[string]any, key string) []string {
	if args == nil {
		return nil
	}
	raw, ok := args[key]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		out := make([]string, 0, len(v))
		for _, item := range v {
			item = strings.TrimSpace(item)
			if item != "" {
				out = append(out, item)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			str, ok := item.(string)
			if !ok {
				continue
			}
			str = strings.TrimSpace(str)
			if str != "" {
				out = append(out, str)
			}
		}
		return out
	default:
		return nil
	}
}

func asnFromArgs(args map[string]any) (*int64, string) {
	if args == nil {
		return nil, ""
	}
	if raw, ok := args["asn"]; ok {
		if asn, ok := numericToInt64(raw); ok {
			return &asn, "ok"
		}
	}
	values := numericSliceArg(args, "asns")
	if len(values) == 0 {
		return nil, ""
	}
	if len(values) == 1 {
		return &values[0], "ok"
	}
	return &values[0], "multiple_asns"
}

func numericSliceArg(args map[string]any, key string) []int64 {
	raw, ok := args[key]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []any:
		out := make([]int64, 0, len(v))
		seen := map[int64]struct{}{}
		for _, item := range v {
			n, ok := numericToInt64(item)
			if !ok {
				continue
			}
			if _, ok := seen[n]; ok {
				continue
			}
			seen[n] = struct{}{}
			out = append(out, n)
		}
		sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
		return out
	default:
		if n, ok := numericToInt64(v); ok {
			return []int64{n}
		}
		return nil
	}
}

func numericToInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int64:
		return n, true
	case float64:
		return int64(n), true
	case float32:
		return int64(n), true
	default:
		return 0, false
	}
}
