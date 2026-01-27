package profile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	"codeberg.org/pawal/gonemaster/share"
)

var defaultProfileJSON = share.ProfileJSON

// Profile mirrors the Zonemaster profile data and tracks which properties are set.
type Profile struct {
	Resolver      ResolverSettings                      `json:"resolver"`
	Net           NetSettings                           `json:"net"`
	NoNetwork     bool                                  `json:"no_network"`
	Cache         map[string]map[string]any             `json:"cache"`
	ASNDB         ASNDBSettings                         `json:"asn_db"`
	LogFilter     map[string]map[string][]LogFilterRule `json:"logfilter"`
	TestLevels    map[string]map[string]string          `json:"test_levels"`
	TestCases     []any                                 `json:"test_cases"`
	TestCasesVars TestCasesVars                         `json:"test_cases_vars"`

	set map[string]bool
}

// ResolverSettings holds resolver-specific profile settings.
type ResolverSettings struct {
	Defaults ResolverDefaults `json:"defaults"`
	Source4  string           `json:"source4"`
	Source6  string           `json:"source6"`
}

// ResolverDefaults mirrors resolver defaults from the profile.
type ResolverDefaults struct {
	Debug    bool `json:"debug"`
	IgnTC    bool `json:"igntc"`
	Fallback bool `json:"fallback"`
	Recurse  bool `json:"recurse"`
	Retrans  int  `json:"retrans"`
	Retry    int  `json:"retry"`
	UseVC    bool `json:"usevc"`
	Timeout  int  `json:"timeout"`
}

// NetSettings holds IP stack enablement flags.
type NetSettings struct {
	IPv4 bool `json:"ipv4"`
	IPv6 bool `json:"ipv6"`
}

// ASNDBSettings holds ASN lookup defaults.
type ASNDBSettings struct {
	Style   string              `json:"style"`
	Sources map[string][]string `json:"sources"`
}

// TestCasesVars stores per-testcase tunables.
type TestCasesVars struct {
	DNSSEC04 DNSSEC04Vars `json:"dnssec04"`
	Zone02   Zone02Vars   `json:"zone02"`
	Zone04   Zone04Vars   `json:"zone04"`
	Zone05   Zone05Vars   `json:"zone05"`
	Zone06   Zone06Vars   `json:"zone06"`
}

type DNSSEC04Vars struct {
	RemainingShort int `json:"REMAINING_SHORT"`
	RemainingLong  int `json:"REMAINING_LONG"`
	DurationLong   int `json:"DURATION_LONG"`
}

type Zone02Vars struct {
	SOARefreshMinimumValue int `json:"SOA_REFRESH_MINIMUM_VALUE"`
}

type Zone04Vars struct {
	SOARetryMinimumValue int `json:"SOA_RETRY_MINIMUM_VALUE"`
}

type Zone05Vars struct {
	SOAExpireMinimumValue int `json:"SOA_EXPIRE_MINIMUM_VALUE"`
}

type Zone06Vars struct {
	SOADefaultTTLMaximumValue int `json:"SOA_DEFAULT_TTL_MAXIMUM_VALUE"`
	SOADefaultTTLMinimumValue int `json:"SOA_DEFAULT_TTL_MINIMUM_VALUE"`
}

// LogFilterRule mirrors the logfilter rule structure.
type LogFilterRule struct {
	When map[string]any `json:"when"`
	Set  string         `json:"set"`
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
	keys := make([]string, 0, len(propertyDefs))
	for key := range propertyDefs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
