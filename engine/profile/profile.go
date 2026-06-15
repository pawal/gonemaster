package profile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"slices"

	"codeberg.org/pawal/gonemaster/share"
)

var defaultProfileJSON = share.ProfileJSON

// Profile holds the engine profile configuration and tracks which properties are set.
type Profile struct {
	// Resolver contains resolver-specific profile settings.
	Resolver ResolverSettings `json:"resolver"`
	// Net controls IPv4 and IPv6 enablement.
	Net NetSettings `json:"net"`
	// NoNetwork blocks network access during a run when true.
	NoNetwork bool `json:"no_network"`
	// Cache stores cache-related profile settings keyed by cache area.
	Cache map[string]map[string]any `json:"cache"`
	// ASNDB configures ASN lookup behavior and upstream sources.
	ASNDB ASNDBSettings `json:"asn_db"`
	// Badkeys configures badkeys blocklist loading.
	Badkeys BadkeysSettings `json:"badkeys"`
	// LogFilter defines per-module/tag log filtering rules.
	LogFilter map[string]map[string][]LogFilterRule `json:"logfilter"`
	// TestLevels overrides log levels per module and tag.
	TestLevels map[string]map[string]string `json:"test_levels"`
	// TestCases limits the enabled testcase set when non-empty.
	TestCases []any `json:"test_cases"`
	// TestCasesVars holds testcase-specific numeric tunables.
	TestCasesVars TestCasesVars `json:"test_cases_vars"`

	set map[string]bool
}

// ResolverSettings holds resolver-specific profile settings.
type ResolverSettings struct {
	// Defaults contains default resolver behavior for queries.
	Defaults ResolverDefaults `json:"defaults"`
	// Source4 selects the IPv4 source address for outbound queries.
	Source4 string `json:"source4"`
	// Source6 selects the IPv6 source address for outbound queries.
	Source6 string `json:"source6"`
}

// ResolverDefaults mirrors resolver defaults from the profile.
type ResolverDefaults struct {
	// Debug enables resolver debug behavior.
	Debug bool `json:"debug"`
	// IgnTC ignores TC and avoids retrying over TCP when true.
	IgnTC bool `json:"igntc"`
	// Fallback enables UDP-to-TCP fallback on truncation.
	Fallback bool `json:"fallback"`
	// Recurse sets the RD bit on outbound queries.
	Recurse bool `json:"recurse"`
	// Retrans sets the retransmission interval in seconds.
	Retrans int `json:"retrans"`
	// Retry sets the number of retry attempts.
	Retry int `json:"retry"`
	// Parallel sets the number of parallel resolver workers.
	Parallel int `json:"parallel"`
	// Unordered allows unordered resolver result handling.
	Unordered bool `json:"unordered"`
	// UseVC forces TCP queries by default.
	UseVC bool `json:"usevc"`
	// Timeout sets the per-query timeout in seconds.
	Timeout int `json:"timeout"`
	// ErrorCacheTTL sets the duration (seconds) to skip queries after network errors.
	ErrorCacheTTL int `json:"error_cache_ttl"`
	// PositiveCacheTTL sets the duration (seconds) to cache positive responses.
	PositiveCacheTTL int `json:"positive_cache_ttl"`
	// NegativeCacheTTL sets the duration (seconds) to cache negative responses.
	NegativeCacheTTL int `json:"negative_cache_ttl"`
	// FastFailTimeoutCount sets how many consecutive timeout-pattern failures
	// cause a nameserver to be skipped for the remainder of the job. 0 disables.
	FastFailTimeoutCount int `json:"fast_fail_timeout_count"`
	// NameserverConcurrency limits concurrent queries per nameserver address. 0 disables.
	NameserverConcurrency int `json:"nameserver_concurrency"`
	// NameserverMaxTotalMS caps cumulative milliseconds spent per nameserver
	// address per run; the address is skipped once exceeded. 0 disables.
	NameserverMaxTotalMS int `json:"nameserver_max_total_ms"`
}

// NetSettings holds IP stack enablement flags.
type NetSettings struct {
	// IPv4 enables IPv4 transport when true.
	IPv4 bool `json:"ipv4"`
	// IPv6 enables IPv6 transport when true.
	IPv6 bool `json:"ipv6"`
}

// ASNDBSettings holds ASN lookup defaults.
type ASNDBSettings struct {
	// Style selects the ASN backend implementation.
	Style string `json:"style"`
	// Sources lists lookup endpoints per backend style.
	Sources map[string][]string `json:"sources"`
}

// BadkeysSettings holds badkeys blocklist configuration.
type BadkeysSettings struct {
	// Path points to the badkeys data directory or file set.
	Path string `json:"path"`
}

// TestCasesVars stores per-testcase tunables.
type TestCasesVars struct {
	// DNSSEC04 holds tunables for DNSSEC04.
	DNSSEC04 DNSSEC04Vars `json:"dnssec04"`
	// Zone02 holds tunables for Zone02.
	Zone02 Zone02Vars `json:"zone02"`
	// Zone04 holds tunables for Zone04.
	Zone04 Zone04Vars `json:"zone04"`
	// Zone05 holds tunables for Zone05.
	Zone05 Zone05Vars `json:"zone05"`
	// Zone06 holds tunables for Zone06.
	Zone06 Zone06Vars `json:"zone06"`
	// Zone13 holds tunables for Zone13.
	Zone13 Zone13Vars `json:"zone13"`
}

// DNSSEC04Vars holds profile tunables for DNSSEC04 checks.
type DNSSEC04Vars struct {
	// RemainingShort is the minimum acceptable remaining RRSIG lifetime.
	RemainingShort int `json:"REMAINING_SHORT"`
	// RemainingLong is the threshold for long remaining RRSIG lifetime.
	RemainingLong int `json:"REMAINING_LONG"`
	// DurationLong is the threshold for long total RRSIG validity.
	DurationLong int `json:"DURATION_LONG"`
}

// Zone02Vars holds profile tunables for Zone02 checks.
type Zone02Vars struct {
	// SOARefreshMinimumValue is the minimum acceptable SOA refresh value.
	SOARefreshMinimumValue int `json:"SOA_REFRESH_MINIMUM_VALUE"`
}

// Zone04Vars holds profile tunables for Zone04 checks.
type Zone04Vars struct {
	// SOARetryMinimumValue is the minimum acceptable SOA retry value.
	SOARetryMinimumValue int `json:"SOA_RETRY_MINIMUM_VALUE"`
}

// Zone05Vars holds profile tunables for Zone05 checks.
type Zone05Vars struct {
	// SOAExpireMinimumValue is the minimum acceptable SOA expire value.
	SOAExpireMinimumValue int `json:"SOA_EXPIRE_MINIMUM_VALUE"`
}

// Zone06Vars holds profile tunables for Zone06 checks.
type Zone06Vars struct {
	// SOADefaultTTLMaximumValue is the maximum allowed SOA default TTL.
	SOADefaultTTLMaximumValue int `json:"SOA_DEFAULT_TTL_MAXIMUM_VALUE"`
	// SOADefaultTTLMinimumValue is the minimum allowed SOA default TTL.
	SOADefaultTTLMinimumValue int `json:"SOA_DEFAULT_TTL_MINIMUM_VALUE"`
}

// Zone13Vars holds profile tunables for Zone13 (SPF DNS lookup count) checks.
type Zone13Vars struct {
	// SPFLookupLimit is the maximum permitted SPF DNS lookup count.
	SPFLookupLimit int `json:"SPF_LOOKUP_LIMIT"`
}

// LogFilterRule mirrors the logfilter rule structure.
type LogFilterRule struct {
	// When describes the match condition for the rule.
	When map[string]any `json:"when"`
	// Set names the filter action to apply when the rule matches.
	Set string `json:"set"`
}

var effective = mustDefault()

// New creates a profile with all properties unset.
func New() *Profile {
	return &Profile{set: map[string]bool{}}
}

// Default returns a profile populated from the embedded default JSON.
func Default() (*Profile, error) {
	p := New()
	defaults, err := decodeProfileJSON(defaultProfileJSON)
	if err != nil {
		return nil, fmt.Errorf("load default profile: %w", err)
	}
	if err := p.applyDefaults(defaults); err != nil {
		return nil, err
	}
	return p, nil
}

// FromJSON creates a profile from a JSON string.
func FromJSON(text string) (*Profile, error) {
	data, err := decodeProfileJSON([]byte(text))
	if err != nil {
		return nil, fmt.Errorf("load json profile: %w", err)
	}
	return fromDataMap(data)
}

// FromYAML creates a profile from a YAML string.
// This implementation supports JSON-compatible YAML input.
func FromYAML(text string) (*Profile, error) {
	return FromJSON(text)
}

// ToJSON serializes the profile into JSON.
func (p *Profile) ToJSON() (string, error) {
	if p == nil {
		return "{}", nil
	}
	payload := p.toMap()
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// Effective returns the current effective profile.
func Effective() *Profile {
	return effective
}

// SetEffective overrides the global effective profile.
// Passing nil resets to defaults.
func SetEffective(p *Profile) {
	if p == nil {
		effective = mustDefault()
		return
	}
	effective = p
}

// ResetEffective resets the effective profile to defaults.
func ResetEffective() {
	effective = mustDefault()
}

// AllProperties returns the list of recognized property names.
func AllProperties() []string {
	return propertyNames()
}

// Get returns the value of a property when set, or nil otherwise.
func (p *Profile) Get(path string) (any, error) {
	def, ok := propertyDefs[path]
	if !ok {
		return nil, fmt.Errorf("unknown property %q", path)
	}
	if p == nil || !p.isSet(path) {
		return nil, nil
	}
	return deepCopy(def.getter(p)), nil
}

// Set updates a property using the direct setter rules.
func (p *Profile) Set(path string, value any) error {
	return p.setWithSource(path, value, sourceDirect)
}

// Merge applies all set properties from another profile.
func (p *Profile) Merge(other *Profile) error {
	if other == nil {
		return fmt.Errorf("merge with nil profile")
	}
	for _, name := range propertyNames() {
		if !other.isSet(name) {
			continue
		}
		value := deepCopy(propertyDefs[name].getter(other))
		if err := p.setWithSource(name, value, sourceJSON); err != nil {
			return err
		}
	}
	return nil
}

func (p *Profile) ensureSetMap() {
	if p.set == nil {
		p.set = map[string]bool{}
	}
}

func (p *Profile) isSet(path string) bool {
	if p == nil || p.set == nil {
		return false
	}
	return p.set[path]
}

func (p *Profile) markSet(path string) {
	p.ensureSetMap()
	p.set[path] = true
}

func (p *Profile) applyDefaults(defaults map[string]any) error {
	for _, name := range propertyNames() {
		if value, ok := getNestedValue(defaults, name); ok {
			if err := p.setWithSource(name, value, sourceJSON); err != nil {
				return err
			}
		}
	}
	for _, name := range propertyNames() {
		def := propertyDefs[name]
		if p.isSet(name) || !def.hasDefault {
			continue
		}
		if err := p.setWithSource(name, deepCopy(def.defaultValue), sourceDirect); err != nil {
			return err
		}
	}
	return nil
}

func (p *Profile) toMap() map[string]any {
	out := map[string]any{}
	for _, name := range propertyNames() {
		if !p.isSet(name) {
			continue
		}
		value := propertyDefs[name].getter(p)
		setNestedValue(out, name, deepCopy(value))
	}
	return out
}

func fromDataMap(data map[string]any) (*Profile, error) {
	paths := map[string]bool{}
	collectPaths(paths, data, nil)
	p := New()
	for path := range paths {
		if _, ok := propertyDefs[path]; !ok {
			return nil, fmt.Errorf("unknown property %q", path)
		}
		value, ok := getNestedValue(data, path)
		if !ok {
			continue
		}
		if err := p.setWithSource(path, value, sourceJSON); err != nil {
			return nil, err
		}
	}
	return p, nil
}

func decodeProfileJSON(data []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var out map[string]any
	if err := decoder.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func mustDefault() *Profile {
	p, err := Default()
	if err != nil {
		panic(err)
	}
	return p
}

func propertyNames() []string {
	keys := slices.Sorted(maps.Keys(propertyDefs))
	return keys
}
