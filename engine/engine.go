package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"codeberg.org/pawal/gonemaster/engine/logger"
	ns "codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/recursor"
	address "codeberg.org/pawal/gonemaster/engine/test/address"
	"codeberg.org/pawal/gonemaster/engine/test/basic"
	"codeberg.org/pawal/gonemaster/engine/test/connectivity"
	consistency "codeberg.org/pawal/gonemaster/engine/test/consistency"
	delegation "codeberg.org/pawal/gonemaster/engine/test/delegation"
	"codeberg.org/pawal/gonemaster/engine/test/dnssec"
	nameserver "codeberg.org/pawal/gonemaster/engine/test/nameserver"
	syntax "codeberg.org/pawal/gonemaster/engine/test/syntax"
	zonetest "codeberg.org/pawal/gonemaster/engine/test/zone"
	"codeberg.org/pawal/gonemaster/engine/transport"
	"codeberg.org/pawal/gonemaster/engine/util"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

// RunRequest defines a single test execution request.
type RunRequest struct {
	// Domain is the target zone name to test.
	Domain string
	// UndelegatedNameservers contains optional pre-delegation NS/glue input.
	UndelegatedNameservers []UndelegatedNameserver
	// UndelegatedDSInfo contains optional pre-delegation DS input.
	UndelegatedDSInfo []UndelegatedDSInfo
	// Module limits execution to a module (for example "basic").
	Module string
	// Testcase limits execution to a single testcase (for example "basic02").
	Testcase string
	// Profile is an optional path to a profile file that overrides defaults.
	Profile string
	// MinLevel controls minimum emitted output level (for example "INFO").
	MinLevel string
	// IPv4 overrides net.ipv4 when non-nil.
	IPv4 *bool
	// IPv6 overrides net.ipv6 when non-nil.
	IPv6 *bool
	// Parallel overrides resolver.defaults.parallel when non-nil.
	Parallel *int
	// Unordered overrides resolver.defaults.unordered when non-nil.
	Unordered *bool
	// ErrorCacheTTL sets resolver.defaults.error_cache_ttl in seconds.
	ErrorCacheTTL *int
	// Timeout sets resolver.defaults.timeout in seconds.
	Timeout *int
	// Retry sets resolver.defaults.retry (number of retries).
	Retry *int
	// Retrans sets resolver.defaults.retrans in seconds.
	Retrans *int
	// Fallback sets resolver.defaults.fallback.
	Fallback *bool
	// SourceAddr4 sets resolver.source4 (IPv4 source address).
	SourceAddr4 *string
	// SourceAddr6 sets resolver.source6 (IPv6 source address).
	SourceAddr6 *string
	// PositiveCacheTTL sets resolver.defaults.positive_cache_ttl in seconds.
	PositiveCacheTTL *int
	// NegativeCacheTTL sets resolver.defaults.negative_cache_ttl in seconds.
	NegativeCacheTTL *int
	// BadkeysPath overrides badkeys.path when non-nil.
	BadkeysPath *string
	// NameserverCache optionally provides the per-run nameserver cache store.
	NameserverCache *ns.CacheStore
	// LogCallback receives each log entry as it is created.
	LogCallback func(*logger.Entry) error
	// Context controls cancellation and timeouts for the run.
	Context context.Context
	// Runner, when set, supplies the per-run container to Run.
	Runner *Runner
}

// LogEntry mirrors the JSON output produced by the Perl logger.
type LogEntry struct {
	// Timestamp is seconds since run start.
	Timestamp float64 `json:"timestamp"`
	// Module is the logical module name (for example "Basic").
	Module string `json:"module"`
	// Testcase is the testcase identifier (for example "Basic02").
	Testcase string `json:"testcase"`
	// Tag is the emitted event tag.
	Tag string `json:"tag"`
	// Level is the normalized severity level.
	Level string `json:"level"`
	// Args carries event-specific fields.
	Args map[string]any `json:"args,omitempty"`
}

// ErrNotImplemented indicates an unknown or unsupported module/testcase
// selection in a run request.
var ErrNotImplemented = errors.New("engine not implemented")

// Version is the semantic version for this build.
var Version = "1.1.0"

// Commit is optionally set at build time using -ldflags.
var Commit = ""

// BuildDate is optionally set at build time using -ldflags.
var BuildDate = ""

// VersionString returns the version string for user-facing output.
func VersionString() string {
	v := strings.TrimSpace(Version)
	if v == "" {
		v = "dev"
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	return v
}

// VersionFull returns the version string with optional build metadata.
func VersionFull() string {
	short := VersionString()
	var details []string
	if strings.TrimSpace(Commit) != "" {
		details = append(details, strings.TrimSpace(Commit))
	}
	if strings.TrimSpace(BuildDate) != "" {
		details = append(details, strings.TrimSpace(BuildDate))
	}
	if len(details) == 0 {
		return short
	}
	return fmt.Sprintf("%s (%s)", short, strings.Join(details, " "))
}

var basicTests = map[string]func(context.Context, *zone.Zone) ([]*logger.Entry, error){
	"basic01": basic.Basic01,
	"basic02": basic.Basic02,
	"basic03": basic.Basic03,
}

var syntaxTests = map[string]func(context.Context, *zone.Zone) ([]*logger.Entry, error){
	"syntax01": syntax.Syntax01,
	"syntax02": syntax.Syntax02,
	"syntax03": syntax.Syntax03,
	"syntax04": syntax.Syntax04,
	"syntax05": syntax.Syntax05,
	"syntax06": syntax.Syntax06,
	"syntax07": syntax.Syntax07,
	"syntax08": syntax.Syntax08,
}

var addressTests = map[string]func(context.Context, *zone.Zone) ([]*logger.Entry, error){
	"address01": address.Address01,
	"address02": address.Address02,
	"address03": address.Address03,
}

var connectivityTests = map[string]func(context.Context, *zone.Zone) ([]*logger.Entry, error){
	"connectivity01": connectivity.Connectivity01,
	"connectivity02": connectivity.Connectivity02,
	"connectivity03": connectivity.Connectivity03,
	"connectivity04": connectivity.Connectivity04,
}

var consistencyTests = map[string]func(context.Context, *zone.Zone) ([]*logger.Entry, error){
	"consistency01": consistency.Consistency01,
	"consistency02": consistency.Consistency02,
	"consistency03": consistency.Consistency03,
	"consistency04": consistency.Consistency04,
	"consistency05": consistency.Consistency05,
	"consistency06": consistency.Consistency06,
}

var delegationTests = map[string]func(context.Context, *zone.Zone) ([]*logger.Entry, error){
	"delegation01": delegation.Delegation01,
	"delegation02": delegation.Delegation02,
	"delegation03": delegation.Delegation03,
	"delegation04": delegation.Delegation04,
	"delegation05": delegation.Delegation05,
	"delegation06": delegation.Delegation06,
	"delegation07": delegation.Delegation07,
}

var dnssecTests = map[string]func(context.Context, *zone.Zone) ([]*logger.Entry, error){
	"dnssec01": dnssec.DNSSEC01,
	"dnssec02": dnssec.DNSSEC02,
	"dnssec03": dnssec.DNSSEC03,
	"dnssec04": dnssec.DNSSEC04,
	"dnssec05": dnssec.DNSSEC05,
	"dnssec06": dnssec.DNSSEC06,
	"dnssec07": dnssec.DNSSEC07,
	"dnssec08": dnssec.DNSSEC08,
	"dnssec09": dnssec.DNSSEC09,
	"dnssec10": dnssec.DNSSEC10,
	"dnssec11": dnssec.DNSSEC11,
	"dnssec13": dnssec.DNSSEC13,
	"dnssec14": dnssec.DNSSEC14,
	"dnssec15": dnssec.DNSSEC15,
	"dnssec16": dnssec.DNSSEC16,
	"dnssec17": dnssec.DNSSEC17,
	"dnssec18": dnssec.DNSSEC18,
	"dnssec19": dnssec.DNSSEC19,
}

var zoneTests = map[string]func(context.Context, *zone.Zone) ([]*logger.Entry, error){
	"zone01": zonetest.Zone01,
	"zone02": zonetest.Zone02,
	"zone03": zonetest.Zone03,
	"zone04": zonetest.Zone04,
	"zone05": zonetest.Zone05,
	"zone06": zonetest.Zone06,
	"zone07": zonetest.Zone07,
	"zone08": zonetest.Zone08,
	"zone09": zonetest.Zone09,
	"zone10": zonetest.Zone10,
	"zone11": zonetest.Zone11,
	"zone12": zonetest.Zone12,
}

var nameserverTests = map[string]func(context.Context, *zone.Zone) ([]*logger.Entry, error){
	"nameserver01": nameserver.Nameserver01,
	"nameserver02": nameserver.Nameserver02,
	"nameserver03": nameserver.Nameserver03,
	"nameserver04": nameserver.Nameserver04,
	"nameserver05": nameserver.Nameserver05,
	"nameserver06": nameserver.Nameserver06,
	"nameserver07": nameserver.Nameserver07,
	"nameserver08": nameserver.Nameserver08,
	"nameserver09": nameserver.Nameserver09,
	"nameserver10": nameserver.Nameserver10,
	"nameserver11": nameserver.Nameserver11,
	"nameserver12": nameserver.Nameserver12,
	"nameserver13": nameserver.Nameserver13,
	"nameserver15": nameserver.Nameserver15,
	"nameserver16": nameserver.Nameserver16,
}

func dnssecTestcaseNames() []string {
	names := make([]string, 0, len(dnssecTests))
	for name := range dnssecTests {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		prefixI, numI, okI := splitTestcaseNumber(names[i])
		prefixJ, numJ, okJ := splitTestcaseNumber(names[j])
		if prefixI == prefixJ && okI && okJ && numI != numJ {
			return numI < numJ
		}
		return names[i] < names[j]
	})
	return names
}

func splitTestcaseNumber(name string) (string, int, bool) {
	idx := len(name)
	for idx > 0 {
		ch := name[idx-1]
		if ch < '0' || ch > '9' {
			break
		}
		idx--
	}
	if idx == len(name) {
		return name, 0, false
	}
	num, err := strconv.Atoi(name[idx:])
	if err != nil {
		return name, 0, false
	}
	return name[:idx], num, true
}

func normalizeRequest(req RunRequest) (string, string, error) {
	module := strings.ToLower(strings.TrimSpace(req.Module))
	testcase := strings.ToLower(strings.TrimSpace(req.Testcase))

	if module != "" && moduleTestcases[module] == nil {
		return "", "", ErrNotImplemented
	}
	if testcase != "" {
		testModule := testcaseModule(testcase)
		if testModule == "" {
			return "", "", ErrNotImplemented
		}
		if module != "" && module != testModule {
			return "", "", ErrNotImplemented
		}
	}
	return module, testcase, nil
}

func buildProfile(req RunRequest, module string, testcase string) (*profile.Profile, bool, error) {
	p, err := profile.Default()
	if err != nil {
		return nil, false, err
	}
	if req.Profile != "" {
		data, err := os.ReadFile(req.Profile)
		if err != nil {
			return nil, false, err
		}
		override, err := profile.FromYAML(string(data))
		if err != nil {
			return nil, false, err
		}
		if err := p.Merge(override); err != nil {
			return nil, false, err
		}
	}

	if req.IPv4 != nil {
		if err := p.Set("net.ipv4", *req.IPv4); err != nil {
			return nil, false, err
		}
	}
	if req.IPv6 != nil {
		if err := p.Set("net.ipv6", *req.IPv6); err != nil {
			return nil, false, err
		}
	}
	if req.Parallel != nil {
		if err := p.Set("resolver.defaults.parallel", *req.Parallel); err != nil {
			return nil, false, err
		}
	}
	if req.Unordered != nil {
		if err := p.Set("resolver.defaults.unordered", *req.Unordered); err != nil {
			return nil, false, err
		}
	}
	if req.ErrorCacheTTL != nil {
		if err := p.Set("resolver.defaults.error_cache_ttl", *req.ErrorCacheTTL); err != nil {
			return nil, false, err
		}
	}
	if req.Timeout != nil {
		if err := p.Set("resolver.defaults.timeout", *req.Timeout); err != nil {
			return nil, false, err
		}
	}
	if req.Retry != nil {
		if err := p.Set("resolver.defaults.retry", *req.Retry); err != nil {
			return nil, false, err
		}
	}
	if req.Retrans != nil {
		if err := p.Set("resolver.defaults.retrans", *req.Retrans); err != nil {
			return nil, false, err
		}
	}
	if req.Fallback != nil {
		if err := p.Set("resolver.defaults.fallback", *req.Fallback); err != nil {
			return nil, false, err
		}
	}
	if req.SourceAddr4 != nil {
		if err := p.Set("resolver.source4", *req.SourceAddr4); err != nil {
			return nil, false, err
		}
	}
	if req.SourceAddr6 != nil {
		if err := p.Set("resolver.source6", *req.SourceAddr6); err != nil {
			return nil, false, err
		}
	}
	if req.PositiveCacheTTL != nil {
		if err := p.Set("resolver.defaults.positive_cache_ttl", *req.PositiveCacheTTL); err != nil {
			return nil, false, err
		}
	}
	if req.NegativeCacheTTL != nil {
		if err := p.Set("resolver.defaults.negative_cache_ttl", *req.NegativeCacheTTL); err != nil {
			return nil, false, err
		}
	}
	if req.BadkeysPath != nil {
		if err := p.Set("badkeys.path", *req.BadkeysPath); err != nil {
			return nil, false, err
		}
	}
	autoDisabledIPv6 := shouldAutoDisableIPv6(req, p.Net.IPv6)
	if autoDisabledIPv6 {
		if err := p.Set("net.ipv6", false); err != nil {
			return nil, false, err
		}
	}

	if testcase == "" {
		switch module {
		case "basic":
			_ = p.Set("test_cases", toAnySlice(moduleTestcases[module]))
		case "syntax":
			_ = p.Set("test_cases", toAnySlice(moduleTestcases[module]))
		case "address":
			_ = p.Set("test_cases", toAnySlice(moduleTestcases[module]))
		case "connectivity":
			_ = p.Set("test_cases", toAnySlice(moduleTestcases[module]))
		case "dnssec":
			_ = p.Set("test_cases", toAnySlice(moduleTestcases[module]))
		case "delegation":
			_ = p.Set("test_cases", toAnySlice(moduleTestcases[module]))
		case "nameserver":
			_ = p.Set("test_cases", toAnySlice(moduleTestcases[module]))
		case "zone":
			_ = p.Set("test_cases", toAnySlice(moduleTestcases[module]))
		}
	}

	return p, autoDisabledIPv6, nil
}

// EffectiveProfile returns the profile that would be used for the request.
func EffectiveProfile(req RunRequest) (*profile.Profile, error) {
	if req.Runner != nil && req.Runner.Profile != nil {
		return req.Runner.Profile, nil
	}
	module, testcase, err := normalizeRequest(req)
	if err != nil {
		return nil, err
	}
	p, _, err := buildProfile(req, module, testcase)
	return p, err
}

// RunWithRunner executes a Zonemaster test run using a provided runner.
func RunWithRunner(req RunRequest, runner *Runner) ([]LogEntry, error) {
	if strings.TrimSpace(req.Domain) == "" {
		return nil, fmt.Errorf("domain is required")
	}
	if runner == nil {
		return nil, fmt.Errorf("runner is required")
	}
	if runner.Profile == nil {
		return nil, fmt.Errorf("runner profile is required")
	}
	if runner.Logger == nil {
		return nil, fmt.Errorf("runner logger is required")
	}
	if runner.NameserverCache == nil {
		if req.NameserverCache != nil {
			runner.NameserverCache = req.NameserverCache
		} else {
			runner.NameserverCache = ns.NewCacheStore()
		}
	}

	module, testcase, err := normalizeRequest(req)
	if err != nil {
		return nil, err
	}

	ctx := req.Context
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = profile.WithContext(ctx, runner.Profile)
	ctx = logger.WithContext(ctx, runner.Logger)
	if runner.Limiter != nil {
		ctx = transport.WithLimiter(ctx, runner.Limiter)
	}
	ctx = ns.WithCache(ctx, runner.NameserverCache)
	if runner.StartedAt.IsZero() {
		runner.StartedAt = time.Now()
	}
	if runner.Profile != nil {
		runner.Logger.SetProfile(runner.Profile)
	}
	ctx = WithRunner(ctx, runner)

	if runner.AutoIPv6Disabled {
		if _, err := runner.Logger.AddWithoutCallback("IPV6_DISABLED", map[string]any{"reason": "auto_no_global_ipv6"}, "", ""); err != nil {
			return nil, err
		}
	}
	if _, err := runner.Logger.AddWithoutCallback("GLOBAL_VERSION", map[string]any{"version": VersionString()}, "", ""); err != nil {
		return nil, err
	}

	entries, err := runWithContext(ctx, req, module, testcase)
	if err != nil {
		return nil, err
	}
	return convertEntries(entries, req.MinLevel)
}

// Run executes a Zonemaster test run.
func Run(req RunRequest) ([]LogEntry, error) {
	if req.Runner != nil {
		return RunWithRunner(req, req.Runner)
	}

	module, testcase, err := normalizeRequest(req)
	if err != nil {
		return nil, err
	}

	log := logger.New()
	if req.LogCallback != nil {
		log.Callback = req.LogCallback
	}

	p, autoDisabledIPv6, err := buildProfile(req, module, testcase)
	if err != nil {
		return nil, err
	}
	log.SetProfile(p)

	queryLimit := p.Resolver.Defaults.Parallel
	if p.Resolver.Defaults.Unordered && queryLimit > 1 {
		queryLimit = queryLimit * queryLimit
	}
	limiter := transport.NewLimiter(queryLimit)

	cacheStore := req.NameserverCache
	if cacheStore == nil {
		cacheStore = ns.NewCacheStore()
	}

	runner := &Runner{
		Profile:          p,
		Logger:           log,
		Limiter:          limiter,
		NameserverCache:  cacheStore,
		StartedAt:        time.Now(),
		AutoIPv6Disabled: autoDisabledIPv6,
	}

	return RunWithRunner(req, runner)
}

func runWithContext(ctx context.Context, req RunRequest, module string, testcase string) ([]*logger.Entry, error) {
	normalizedNameservers, normalizedDSInfo, err := NormalizeUndelegatedInputs(req.UndelegatedNameservers, req.UndelegatedDSInfo)
	if err != nil {
		return nil, err
	}
	req.UndelegatedNameservers = normalizedNameservers
	req.UndelegatedDSInfo = normalizedDSInfo

	r, err := recursor.New()
	if err != nil {
		return nil, err
	}
	z, err := zone.NewWithRecursor(req.Domain, r)
	if err != nil {
		return nil, err
	}
	if err := applyUndelegatedDelegation(ctx, r, &z, req.UndelegatedNameservers, req.UndelegatedDSInfo); err != nil {
		return nil, err
	}
	var entries []*logger.Entry
	switch {
	case testcase != "":
		if fn, ok := basicTests[testcase]; ok {
			entries, err = fn(ctx, &z)
		} else if fn, ok := syntaxTests[testcase]; ok {
			entries, err = fn(ctx, &z)
		} else if fn, ok := addressTests[testcase]; ok {
			entries, err = fn(ctx, &z)
		} else if fn, ok := connectivityTests[testcase]; ok {
			entries, err = fn(ctx, &z)
		} else if fn, ok := consistencyTests[testcase]; ok {
			entries, err = fn(ctx, &z)
		} else if fn, ok := dnssecTests[testcase]; ok {
			entries, err = fn(ctx, &z)
		} else if fn, ok := delegationTests[testcase]; ok {
			entries, err = fn(ctx, &z)
		} else if fn, ok := nameserverTests[testcase]; ok {
			entries, err = fn(ctx, &z)
		} else if fn, ok := zoneTests[testcase]; ok {
			entries, err = fn(ctx, &z)
		}
	case module == "basic":
		entries, err = basic.All(ctx, &z)
	case module == "syntax":
		entries, err = syntax.All(ctx, &z)
	case module == "address":
		entries, err = address.AddressAll(ctx, &z)
	case module == "connectivity":
		entries, err = connectivity.All(ctx, &z)
	case module == "consistency":
		entries, err = consistency.All(ctx, &z)
	case module == "dnssec":
		entries, err = dnssec.All(ctx, &z)
	case module == "delegation":
		entries, err = delegation.All(ctx, &z)
	case module == "nameserver":
		entries, err = nameserver.All(ctx, &z)
	case module == "zone":
		entries, err = zonetest.All(ctx, &z)
	case module == "":
		entries, err = basic.All(ctx, &z)
		if err == nil {
			if !basic.CanContinue(ctx, &z, entries) {
				entry, addErr := util.Info(ctx, "CANNOT_CONTINUE", map[string]any{"domain": z.Name.String()})
				if addErr != nil {
					return nil, addErr
				}
				if entry != nil {
					entries = append(entries, entry)
				}
				return entries, nil
			}
			more, err2 := address.AddressAll(ctx, &z)
			if err2 != nil {
				return nil, err2
			}
			entries = append(entries, more...)
			more, err2 = connectivity.All(ctx, &z)
			if err2 != nil {
				return nil, err2
			}
			entries = append(entries, more...)
			more, err2 = consistency.All(ctx, &z)
			if err2 != nil {
				return nil, err2
			}
			entries = append(entries, more...)
			more, err2 = delegation.All(ctx, &z)
			if err2 != nil {
				return nil, err2
			}
			entries = append(entries, more...)
			more, err2 = dnssec.All(ctx, &z)
			if err2 != nil {
				return nil, err2
			}
			entries = append(entries, more...)
			more, err2 = nameserver.All(ctx, &z)
			if err2 != nil {
				return nil, err2
			}
			entries = append(entries, more...)
			more, err2 = syntax.All(ctx, &z)
			if err2 != nil {
				return nil, err2
			}
			entries = append(entries, more...)
			more, err2 = zonetest.All(ctx, &z)
			if err2 != nil {
				return nil, err2
			}
			entries = append(entries, more...)
		}
	default:
		return nil, ErrNotImplemented
	}
	if err != nil {
		return nil, err
	}

	return entries, nil
}

func convertEntries(entries []*logger.Entry, minLevel string) ([]LogEntry, error) {
	threshold := ""
	minLevel = strings.ToUpper(strings.TrimSpace(minLevel))
	if minLevel != "" {
		if _, ok := logger.Levels()[minLevel]; !ok {
			return nil, fmt.Errorf("unknown min level %q", minLevel)
		}
		threshold = minLevel
	}

	var minValue int
	if threshold != "" {
		minValue = logger.Levels()[threshold]
	}

	out := make([]LogEntry, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		level := entry.Level()
		if threshold != "" && entry.NumericLevel() < minValue {
			continue
		}
		out = append(out, LogEntry{
			Timestamp: entry.Timestamp,
			Module:    entry.Module,
			Testcase:  entry.Testcase,
			Tag:       entry.Tag,
			Level:     level,
			Args:      entry.Args,
		})
	}
	return out, nil
}
