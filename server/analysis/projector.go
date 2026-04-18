package analysis

import (
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"time"

	serverpkg "codeberg.org/pawal/gonemaster/server"
)

var (
	ErrRunNotFound = errors.New("analysis projector run not found")
)

const projectorVersion = "v1"

// Store is the minimal backing store surface needed by the Phase 2 projector
// loading and cohort-resolution logic.
type Store interface {
	GetRun(id string) (serverpkg.Run, bool)
	QueryEntries(filter serverpkg.EntryFilter) serverpkg.EntryList
	GetDomainTags(domainID int64) []string
	ListAnalysisCohorts() []serverpkg.AnalysisCohort
	UpsertAnalysisNameserver(name string, seenAt time.Time) (serverpkg.AnalysisNameserver, error)
	UpsertAnalysisAddress(address, family string, seenAt time.Time) (serverpkg.AnalysisAddress, error)
	UpsertAnalysisPrefix(prefix, family string, seenAt time.Time) (serverpkg.AnalysisPrefix, error)
	UpsertAnalysisASN(asn int64, label string, seenAt time.Time) (serverpkg.AnalysisASN, error)
	ReplaceAnalysisRunNSEndpoints(cohortID int64, runID string, items []serverpkg.AnalysisRunNameserverEndpoint) error
	ReplaceAnalysisRunAddressASNs(cohortID int64, runID string, items []serverpkg.AnalysisRunAddressASN) error
	ReplaceAnalysisRunDomainASNs(cohortID int64, runID string, items []serverpkg.AnalysisRunDomainASN) error
	UpsertAnalysisRunDomainSummary(item serverpkg.AnalysisRunDomainSummary) error
	SetAnalysisProjectionState(item serverpkg.AnalysisProjectionState) error
}

// Projector loads completed runs and resolves which analysis-enabled cohorts
// should receive projected facts for those runs.
type Projector struct {
	store Store
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
// and the analysis-enabled cohorts that match those tags.
func (p *Projector) LoadCompletedRun(runID string) (RunInput, error) {
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
	cohorts := MatchAnalysisEnabledCohorts(tags, p.store.ListAnalysisCohorts())

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
// per-run/per-cohort fact rows.
func (p *Projector) ProjectLoaded(input RunInput) error {
	if len(input.MatchingCohorts) == 0 {
		return nil
	}

	endpoints := p.extractNameserverEndpoints(input)
	addressFacts := p.extractAddressFacts(input)
	domainASNs := p.extractDomainASNs(input)

	nameserverIDs := map[string]int64{}
	addressIDs := map[string]int64{}
	prefixIDs := map[string]int64{}

	for _, endpoint := range endpoints {
		ns, err := p.store.UpsertAnalysisNameserver(endpoint.nameserver, input.Run.FinishedAt)
		if err != nil {
			return fmt.Errorf("upsert nameserver %q: %w", endpoint.nameserver, err)
		}
		addr, err := p.store.UpsertAnalysisAddress(endpoint.address, endpoint.family, input.Run.FinishedAt)
		if err != nil {
			return fmt.Errorf("upsert address %q: %w", endpoint.address, err)
		}
		nameserverIDs[endpoint.nameserver] = ns.ID
		addressIDs[endpoint.address] = addr.ID
	}
	for _, fact := range addressFacts {
		addr, err := p.store.UpsertAnalysisAddress(fact.address, fact.family, input.Run.FinishedAt)
		if err != nil {
			return fmt.Errorf("upsert address fact %q: %w", fact.address, err)
		}
		addressIDs[fact.address] = addr.ID
		if fact.prefix != "" {
			prefix, err := p.store.UpsertAnalysisPrefix(fact.prefix, fact.prefixFamily, input.Run.FinishedAt)
			if err != nil {
				return fmt.Errorf("upsert prefix %q: %w", fact.prefix, err)
			}
			prefixIDs[fact.prefix] = prefix.ID
		}
		if fact.asn != nil {
			if _, err := p.store.UpsertAnalysisASN(*fact.asn, "", input.Run.FinishedAt); err != nil {
				return fmt.Errorf("upsert asn %d: %w", *fact.asn, err)
			}
		}
	}
	for _, da := range domainASNs {
		if _, err := p.store.UpsertAnalysisASN(da.asn, "", input.Run.FinishedAt); err != nil {
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
		if err := p.store.ReplaceAnalysisRunNSEndpoints(cohort.ID, input.Run.ID, nsRows); err != nil {
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
		if err := p.store.ReplaceAnalysisRunAddressASNs(cohort.ID, input.Run.ID, addrRows); err != nil {
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
		if err := p.store.ReplaceAnalysisRunDomainASNs(cohort.ID, input.Run.ID, domainASNRows); err != nil {
			return fmt.Errorf("replace domain asns for cohort %d: %w", cohort.ID, err)
		}

		summary.CohortID = cohort.ID
		if err := p.store.UpsertAnalysisRunDomainSummary(summary); err != nil {
			return fmt.Errorf("upsert summary for cohort %d: %w", cohort.ID, err)
		}
		if err := p.store.SetAnalysisProjectionState(serverpkg.AnalysisProjectionState{
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
	endpointAddresses := map[string]struct{}{}
	asns := map[int64]struct{}{}
	prefixes := map[string]struct{}{}

	for _, endpoint := range endpoints {
		nameservers[endpoint.nameserver] = struct{}{}
		endpointAddresses[endpoint.address] = struct{}{}
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
		EndpointCount:   len(endpointAddresses),
		ASNCount:        len(asns),
		PrefixCount:     len(prefixes),
		WorstLevel:      input.Run.WorstLevel,
	}
}

func (p *Projector) extractNameserverEndpoints(input RunInput) []extractedEndpoint {
	seen := map[string]extractedEndpoint{}
	add := func(item extractedEndpoint) {
		if item.nameserver == "" || item.address == "" {
			return
		}
		if item.family == "" {
			item.family = familyForAddress(item.address)
		}
		if item.family == "" {
			return
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

	// nameserver_timings is the gonemaster engine's own record of the
	// authoritative nameservers it actually queried for the tested zone, so
	// we treat it as ground truth for "this (ns, address) is authoritative
	// for the domain". When it's populated, any (ns, address) NOT in that
	// set is treated as parent-side (delegation / DS queries, etc.) and
	// downgraded to role="parent" — those rows are filtered out of the
	// public views, so the domain detail page shows only the zone's own
	// servers.
	timingSet := make(map[string]struct{}, len(input.NameserverTimings))
	for _, timing := range input.NameserverTimings {
		ns := normalizeNameserverName(timing.Nameserver)
		addr := strings.TrimSpace(timing.Address)
		if ns == "" || addr == "" {
			continue
		}
		timingSet[ns+"|"+addr] = struct{}{}
	}
	haveTimings := len(timingSet) > 0
	classify := func(candidateRole, ns, addr string) string {
		if !haveTimings {
			return candidateRole
		}
		if _, ok := timingSet[ns+"|"+addr]; ok {
			return "authoritative"
		}
		return "parent"
	}

	for _, timing := range input.NameserverTimings {
		add(extractedEndpoint{
			nameserver: normalizeNameserverName(timing.Nameserver),
			address:    strings.TrimSpace(timing.Address),
			role:       "authoritative",
			source:     "timings",
			family:     familyForAddress(timing.Address),
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
				role:       classify(roleForSource("entry"), ns, addr),
				source:     "entry",
			})
		}
		for _, sourceKey := range []string{"servers", "parent_servers", "child_servers", "zone_servers", "ns_set_servers"} {
			for _, endpoint := range endpointsFromArgs(entry.Args[sourceKey]) {
				endpoint.role = classify(roleForSource(sourceKey), endpoint.nameserver, endpoint.address)
				endpoint.source = sourceKey
				add(endpoint)
			}
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

func roleForSource(source string) string {
	switch source {
	case "parent_servers":
		return "parent"
	case "child_servers", "zone_servers", "servers", "ns_set_servers", "timings", "entry":
		return "authoritative"
	default:
		return ""
	}
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
