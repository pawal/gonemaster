package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/profile"
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
	Domain    string
	Module    string
	Testcase  string
	Profile   string
	MinLevel  string
	IPv4      *bool
	IPv6      *bool
	Parallel  *int
	Unordered *bool
	// LogCallback receives each log entry as it is created.
	LogCallback func(*logger.Entry) error
}

// LogEntry mirrors the JSON output produced by the Perl logger.
type LogEntry struct {
	Timestamp float64        `json:"timestamp"`
	Module    string         `json:"module"`
	Testcase  string         `json:"testcase"`
	Tag       string         `json:"tag"`
	Level     string         `json:"level"`
	Args      map[string]any `json:"args,omitempty"`
}

var ErrNotImplemented = errors.New("engine not implemented")

// Version is the semantic version for this build.
var Version = "0.9.9"

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

// EffectiveProfile returns the profile that would be used for the request.
func EffectiveProfile(req RunRequest) (*profile.Profile, error) {
	module := strings.ToLower(strings.TrimSpace(req.Module))
	testcase := strings.ToLower(strings.TrimSpace(req.Testcase))

	if module != "" && moduleTestcases[module] == nil {
		return nil, ErrNotImplemented
	}
	if testcase != "" {
		testModule := testcaseModule(testcase)
		if testModule == "" {
			return nil, ErrNotImplemented
		}
		if module != "" && module != testModule {
			return nil, ErrNotImplemented
		}
	}

	p, err := profile.Default()
	if err != nil {
		return nil, err
	}
	if req.Profile != "" {
		data, err := os.ReadFile(req.Profile)
		if err != nil {
			return nil, err
		}
		override, err := profile.FromYAML(string(data))
		if err != nil {
			return nil, err
		}
		if err := p.Merge(override); err != nil {
			return nil, err
		}
	}

	if req.IPv4 != nil {
		if err := p.Set("net.ipv4", *req.IPv4); err != nil {
			return nil, err
		}
	}
	if req.IPv6 != nil {
		if err := p.Set("net.ipv6", *req.IPv6); err != nil {
			return nil, err
		}
	}
	if req.Parallel != nil {
		if err := p.Set("resolver.defaults.parallel", *req.Parallel); err != nil {
			return nil, err
		}
	}
	if req.Unordered != nil {
		if err := p.Set("resolver.defaults.unordered", *req.Unordered); err != nil {
			return nil, err
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

	return p, nil
}

// Run executes a Zonemaster test run.
func Run(req RunRequest) ([]LogEntry, error) {
	if strings.TrimSpace(req.Domain) == "" {
		return nil, fmt.Errorf("domain is required")
	}

	module := strings.ToLower(strings.TrimSpace(req.Module))
	testcase := strings.ToLower(strings.TrimSpace(req.Testcase))

	if module != "" && moduleTestcases[module] == nil {
		return nil, ErrNotImplemented
	}
	if testcase != "" {
		testModule := testcaseModule(testcase)
		if testModule == "" {
			return nil, ErrNotImplemented
		}
		if module != "" && module != testModule {
			return nil, ErrNotImplemented
		}
	}

	log := logger.New()
	if req.LogCallback != nil {
		log.Callback = req.LogCallback
	}
	util.SetLogger(log)
	defer util.SetLogger(nil)
	logger.StartTimeNow()

	profile.ResetEffective()
	if req.Profile != "" {
		data, err := os.ReadFile(req.Profile)
		if err != nil {
			return nil, err
		}
		override, err := profile.FromYAML(string(data))
		if err != nil {
			return nil, err
		}
		if err := profile.Effective().Merge(override); err != nil {
			return nil, err
		}
	}

	if req.IPv4 != nil {
		if err := profile.Effective().Set("net.ipv4", *req.IPv4); err != nil {
			return nil, err
		}
	}
	if req.IPv6 != nil {
		if err := profile.Effective().Set("net.ipv6", *req.IPv6); err != nil {
			return nil, err
		}
	}
	if req.Parallel != nil {
		if err := profile.Effective().Set("resolver.defaults.parallel", *req.Parallel); err != nil {
			return nil, err
		}
	}
	if req.Unordered != nil {
		if err := profile.Effective().Set("resolver.defaults.unordered", *req.Unordered); err != nil {
			return nil, err
		}
	}

	if testcase == "" {
		switch module {
		case "basic":
			_ = profile.Effective().Set("test_cases", toAnySlice(moduleTestcases[module]))
		case "syntax":
			_ = profile.Effective().Set("test_cases", toAnySlice(moduleTestcases[module]))
		case "address":
			_ = profile.Effective().Set("test_cases", toAnySlice(moduleTestcases[module]))
		case "connectivity":
			_ = profile.Effective().Set("test_cases", toAnySlice(moduleTestcases[module]))
		case "dnssec":
			_ = profile.Effective().Set("test_cases", toAnySlice(moduleTestcases[module]))
		case "delegation":
			_ = profile.Effective().Set("test_cases", toAnySlice(moduleTestcases[module]))
		case "nameserver":
			_ = profile.Effective().Set("test_cases", toAnySlice(moduleTestcases[module]))
		case "zone":
			_ = profile.Effective().Set("test_cases", toAnySlice(moduleTestcases[module]))
		}
	}

	queryLimit := profile.Effective().Resolver.Defaults.Parallel
	if profile.Effective().Resolver.Defaults.Unordered && queryLimit > 1 {
		queryLimit = queryLimit * queryLimit
	}
	transport.SetGlobalQueryLimit(queryLimit)
	logger.ResetConfig()
	if _, err := util.Info("GLOBAL_VERSION", map[string]any{"version": VersionString()}); err != nil {
		return nil, err
	}

	z, err := zone.New(req.Domain)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
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
			if !basic.CanContinue(&z, entries) {
				entry, addErr := util.Info("CANNOT_CONTINUE", map[string]any{"domain": z.Name.String()})
				if addErr != nil {
					return nil, addErr
				}
				if entry != nil {
					entries = append(entries, entry)
				}
				return convertEntries(entries, req.MinLevel)
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

	return convertEntries(entries, req.MinLevel)
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
