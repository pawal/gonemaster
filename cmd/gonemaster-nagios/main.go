package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/i18n"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/profile"
)

// stringSliceFlag is a repeatable string flag.
type stringSliceFlag []string

func (s *stringSliceFlag) String() string { return strings.Join(*s, ", ") }
func (s *stringSliceFlag) Set(v string) error {
	*s = append(*s, v)
	return nil
}

type countFlag int

// String returns the current verbosity count.
func (c *countFlag) String() string {
	return fmt.Sprintf("%d", *c)
}

// Set increments the verbosity count.
func (c *countFlag) Set(_ string) error {
	*c += 1
	return nil
}

// IsBoolFlag makes the flag usable as a boolean-style repeated flag.
func (c *countFlag) IsBoolFlag() bool {
	return true
}

type nagiosStatus struct {
	text string
	code int
}

type severityThresholds struct {
	warningValue  int
	criticalValue int
}

var runEngine = engine.Run

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, out io.Writer, errOut io.Writer) int {
	var domain string
	var module string
	var testcase string
	var profilePath string
	var warningLevel string
	var criticalLevel string
	var timeoutSeconds int
	var timeoutSet bool
	var noIPv4 bool
	var noIPv6 bool
	var disableIPv4 bool
	var disableIPv6 bool
	var forceIPv6 bool
	var sourceAddr4 string
	var sourceAddr6 string
	var sourceAddr4Set bool
	var sourceAddr6Set bool
	var nsFlags stringSliceFlag
	var dsFlags stringSliceFlag
	var rrsigWarnDays int
	var showVersion bool
	var showHelp bool
	var verbose countFlag

	fs := flag.NewFlagSet("gonemaster-nagios", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.Usage = func() {
		fmt.Fprintf(errOut, "Usage: %s -H DOMAIN [options]\n", fs.Name())
		fmt.Fprintln(errOut, "")
		fmt.Fprintln(errOut, "Core Nagios options:")
		fmt.Fprintln(errOut, "  -H, --hostname    Zone name to test (preferred)")
		fmt.Fprintln(errOut, "  -d, --domain      Zone name to test (DNS-specific alias)")
		fmt.Fprintln(errOut, "  -w, --warning     Severity that becomes WARNING (default WARNING)")
		fmt.Fprintln(errOut, "  -c, --critical    Severity that becomes CRITICAL (default ERROR)")
		fmt.Fprintln(errOut, "  -t, --timeout     Plugin runtime deadline in seconds")
		fmt.Fprintln(errOut, "  -v, --verbose     Increase verbosity (repeatable)")
		fmt.Fprintln(errOut, "  -V, --version     Print version and exit")
		fmt.Fprintln(errOut, "  -h, --help        Show help")
		fmt.Fprintln(errOut, "")
		fmt.Fprintln(errOut, "Gonemaster options:")
		fmt.Fprintln(errOut, "  --module          Run a single module")
		fmt.Fprintln(errOut, "  --testcase        Run a single testcase")
		fmt.Fprintln(errOut, "  --profile         Profile JSON/YAML path")
		fmt.Fprintln(errOut, "  --no-ipv4         Disable IPv4 queries")
		fmt.Fprintln(errOut, "  --no-ipv6         Disable IPv6 queries")
		fmt.Fprintln(errOut, "  --force-ipv6      Force IPv6 queries")
		fmt.Fprintln(errOut, "  --source-addr4    Override resolver.source4 (IPv4 source address)")
		fmt.Fprintln(errOut, "  --source-addr6    Override resolver.source6 (IPv6 source address)")
		fmt.Fprintln(errOut, "  --ns              Undelegated nameserver: name or name/ip (repeatable)")
		fmt.Fprintln(errOut, "  --ds              Undelegated DS record: keytag,algo,digtype,digest (repeatable)")
		fmt.Fprintln(errOut, "  --rrsig-warn-days Warn if any apex RRSIG expires within N days (requires --testcase dnssec04 or --module dnssec)")
		fmt.Fprintln(errOut, "")
		fmt.Fprintln(errOut, "Compatibility aliases:")
		fmt.Fprintln(errOut, "  --ipv6            Deprecated alias for --force-ipv6")
		fmt.Fprintln(errOut, "  --disable-ipv4    Deprecated alias for --no-ipv4")
		fmt.Fprintln(errOut, "  --disable-ipv6    Deprecated alias for --no-ipv6")
		fmt.Fprintln(errOut, "  --sourceaddr4     Deprecated alias for --source-addr4")
		fmt.Fprintln(errOut, "  --sourceaddr6     Deprecated alias for --source-addr6")
		fmt.Fprintln(errOut, "")
		fmt.Fprintf(errOut, "Severity values: %s\n", strings.Join(severityLevels(), ", "))
		fmt.Fprintln(errOut, "")
		fmt.Fprintln(errOut, "Exit codes:")
		fmt.Fprintln(errOut, "  0 OK")
		fmt.Fprintln(errOut, "  1 WARNING")
		fmt.Fprintln(errOut, "  2 CRITICAL")
		fmt.Fprintln(errOut, "  3 UNKNOWN")
	}

	fs.StringVar(&domain, "hostname", "", "Zone name to test (preferred)")
	fs.StringVar(&domain, "H", "", "Zone name to test (preferred)")
	fs.StringVar(&domain, "domain", "", "Zone name to test (required)")
	fs.StringVar(&domain, "d", "", "Zone name to test (required)")
	fs.StringVar(&warningLevel, "warning", "WARNING", "Severity that becomes WARNING (optional)")
	fs.StringVar(&warningLevel, "w", "WARNING", "Severity that becomes WARNING (optional)")
	fs.StringVar(&criticalLevel, "critical", "ERROR", "Severity that becomes CRITICAL (optional)")
	fs.StringVar(&criticalLevel, "c", "ERROR", "Severity that becomes CRITICAL (optional)")
	fs.IntVar(&timeoutSeconds, "timeout", 0, "Plugin runtime deadline in seconds (optional)")
	fs.IntVar(&timeoutSeconds, "t", 0, "Plugin runtime deadline in seconds (optional)")
	fs.StringVar(&module, "module", "", "Run a single module (optional)")
	fs.StringVar(&testcase, "testcase", "", "Run a single testcase (optional)")
	fs.StringVar(&profilePath, "profile", "", "Profile JSON/YAML path (optional)")
	fs.BoolVar(&noIPv4, "no-ipv4", false, "Disable IPv4 queries (optional)")
	fs.BoolVar(&noIPv6, "no-ipv6", false, "Disable IPv6 queries (optional)")
	fs.BoolVar(&disableIPv4, "disable-ipv4", false, "Disable IPv4 queries (optional)")
	fs.BoolVar(&disableIPv6, "disable-ipv6", false, "Disable IPv6 queries (optional)")
	fs.BoolVar(&forceIPv6, "force-ipv6", false, "Force IPv6 queries (optional)")
	fs.BoolVar(&forceIPv6, "ipv6", false, "Force IPv6 queries (optional)")
	fs.StringVar(&sourceAddr4, "source-addr4", "", "Override resolver.source4 (IPv4 source address) (optional)")
	fs.StringVar(&sourceAddr6, "source-addr6", "", "Override resolver.source6 (IPv6 source address) (optional)")
	fs.StringVar(&sourceAddr4, "sourceaddr4", "", "Override resolver.source4 (IPv4 source address) (optional)")
	fs.StringVar(&sourceAddr6, "sourceaddr6", "", "Override resolver.source6 (IPv6 source address) (optional)")
	fs.Var(&nsFlags, "ns", "Undelegated nameserver: name or name/ip (repeatable)")
	fs.Var(&dsFlags, "ds", "Undelegated DS record: keytag,algo,digtype,digest (repeatable)")
	fs.IntVar(&rrsigWarnDays, "rrsig-warn-days", 0, "Warn if any apex RRSIG expires within N days")
	fs.Var(&verbose, "verbose", "Increase verbosity (repeatable)")
	fs.Var(&verbose, "v", "Increase verbosity (repeatable)")
	fs.BoolVar(&showVersion, "version", false, "Print version and exit")
	fs.BoolVar(&showVersion, "V", false, "Print version and exit")
	fs.BoolVar(&showHelp, "help", false, "Show help")
	fs.BoolVar(&showHelp, "h", false, "Show help")

	if err := fs.Parse(expandVerboseArgs(args)); err != nil {
		return 3
	}
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "timeout", "t":
			timeoutSet = true
		case "source-addr4", "sourceaddr4":
			sourceAddr4Set = true
		case "source-addr6", "sourceaddr6":
			sourceAddr6Set = true
		}
	})

	if showHelp {
		fs.Usage()
		return 0
	}

	if showVersion {
		fmt.Fprintf(out, "Gonemaster version %s\n", engine.VersionFull())
		fmt.Fprintf(out, "Miekg DNS version %s\n", moduleVersion("codeberg.org/miekg/dns"))
		return 0
	}

	if strings.TrimSpace(domain) == "" {
		fs.Usage()
		return 3
	}

	thresholds, thresholdErr := parseSeverityThresholds(warningLevel, criticalLevel)
	if thresholdErr != nil {
		fmt.Fprintln(errOut, thresholdErr.Error())
		return 3
	}
	if timeoutSet && timeoutSeconds < 1 {
		fmt.Fprintln(errOut, "--timeout must be >= 1")
		return 3
	}

	var ipv4Override *bool
	if noIPv4 || disableIPv4 {
		value := false
		ipv4Override = &value
	}
	var ipv6Override *bool
	if noIPv6 || disableIPv6 {
		value := false
		ipv6Override = &value
	}
	if forceIPv6 {
		value := true
		ipv6Override = &value
	}
	if (noIPv6 || disableIPv6) && forceIPv6 {
		fmt.Fprintln(errOut, "--no-ipv6/--disable-ipv6 cannot be combined with --force-ipv6/--ipv6")
		return 3
	}
	var sourceAddr4Override *string
	if sourceAddr4Set {
		value := strings.TrimSpace(sourceAddr4)
		parsed := net.ParseIP(value)
		if parsed == nil || parsed.To4() == nil {
			fmt.Fprintln(errOut, "--source-addr4 must be a valid IPv4 address")
			return 3
		}
		sourceAddr4Override = &value
	}
	var sourceAddr6Override *string
	if sourceAddr6Set {
		value := strings.TrimSpace(sourceAddr6)
		parsed := net.ParseIP(value)
		if parsed == nil || parsed.To4() != nil {
			fmt.Fprintln(errOut, "--source-addr6 must be a valid IPv6 address")
			return 3
		}
		sourceAddr6Override = &value
	}

	var undelegatedNS []engine.UndelegatedNameserver
	for _, spec := range nsFlags {
		ns, err := engine.ParseUndelegatedNameserver(spec)
		if err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 3
		}
		undelegatedNS = append(undelegatedNS, ns)
	}
	var undelegatedDS []engine.UndelegatedDSInfo
	for _, spec := range dsFlags {
		ds, err := engine.ParseUndelegatedDS(spec)
		if err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 3
		}
		undelegatedDS = append(undelegatedDS, ds)
	}
	if len(undelegatedDS) > 0 && len(undelegatedNS) == 0 {
		fmt.Fprintln(errOut, "--ds requires --ns")
		return 3
	}
	if rrsigWarnDays < 0 {
		fmt.Fprintln(errOut, "--rrsig-warn-days must be >= 0")
		return 3
	}
	if rrsigWarnDays > 0 && testcase != "dnssec04" && module != "dnssec" && module != "" {
		fmt.Fprintf(errOut, "--rrsig-warn-days only affects dnssec04; consider --testcase dnssec04 or --module dnssec\n")
	}
	mergedProfilePath, profileCleanup, profileErr := buildMergedProfile(profilePath, rrsigWarnDays)
	if profileErr != nil {
		fmt.Fprintln(errOut, profileErr.Error())
		return 3
	}
	if profileCleanup != nil {
		defer profileCleanup()
	}

	var runCtx context.Context
	var cancel context.CancelFunc
	if timeoutSet {
		runCtx, cancel = context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
		defer cancel()
	}

	req := engine.RunRequest{
		Domain:                 domain,
		Module:                 module,
		Testcase:               testcase,
		Profile:                mergedProfilePath,
		IPv4:                   ipv4Override,
		IPv6:                   ipv6Override,
		SourceAddr4:            sourceAddr4Override,
		SourceAddr6:            sourceAddr6Override,
		UndelegatedNameservers: undelegatedNS,
		UndelegatedDSInfo:      undelegatedDS,
	}
	if runCtx != nil {
		req.Context = runCtx
	}

	entries, err := runEngine(req)
	timedOut := timeoutSet && (errors.Is(err, context.DeadlineExceeded) || (runCtx != nil && errors.Is(runCtx.Err(), context.DeadlineExceeded)))
	if err != nil {
		if timedOut {
			fmt.Fprintf(out, "ZONE UNKNOWN - plugin timed out after %ds\n", timeoutSeconds)
			return 3
		}
		fmt.Fprintf(out, "ZONE UNKNOWN - %s\n", err.Error())
		return 3
	}
	if timedOut {
		fmt.Fprintf(out, "ZONE UNKNOWN - plugin timed out after %ds\n", timeoutSeconds)
		return 3
	}

	status := statusForLevel(maxLevel(entries), thresholds)

	fmt.Fprintf(out, "ZONE %s\n", status.text)
	if verbose > 0 {
		for _, entry := range entries {
			if entry.Level == "" || !shouldPrintVerbose(entry.Level, int(verbose)) {
				continue
			}
			message := translatedMessage(entry)
			if message == "" {
				continue
			}
			fmt.Fprintln(out, message)
		}
	}

	return status.code
}

func expandVerboseArgs(args []string) []string {
	expanded := make([]string, 0, len(args))
	for _, arg := range args {
		if len(arg) > 2 && strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") {
			allVerbose := true
			for _, ch := range arg[1:] {
				if ch != 'v' {
					allVerbose = false
					break
				}
			}
			if allVerbose {
				for range arg[1:] {
					expanded = append(expanded, "-v")
				}
				continue
			}
		}
		expanded = append(expanded, arg)
	}
	return expanded
}

func parseSeverityThresholds(warningLevel string, criticalLevel string) (severityThresholds, error) {
	_, warningValue, err := normalizeSeverityLevel(warningLevel, "--warning")
	if err != nil {
		return severityThresholds{}, err
	}
	_, criticalValue, err := normalizeSeverityLevel(criticalLevel, "--critical")
	if err != nil {
		return severityThresholds{}, err
	}
	if warningValue >= criticalValue {
		return severityThresholds{}, fmt.Errorf("--warning must be lower severity than --critical")
	}
	return severityThresholds{
		warningValue:  warningValue,
		criticalValue: criticalValue,
	}, nil
}

func normalizeSeverityLevel(value string, flagName string) (string, int, error) {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	levelValue, ok := logger.Levels()[normalized]
	if !ok {
		return "", 0, fmt.Errorf("%s must be one of %s", flagName, strings.Join(severityLevels(), ", "))
	}
	return normalized, levelValue, nil
}

func maxLevel(entries []engine.LogEntry) string {
	levels := logger.Levels()
	maxLevel := lowestSeverityLevel(levels)
	maxValue := levels[maxLevel]
	for _, entry := range entries {
		level := strings.ToUpper(strings.TrimSpace(entry.Level))
		value, ok := levels[level]
		if !ok {
			continue
		}
		if value > maxValue {
			maxValue = value
			maxLevel = level
		}
	}
	return maxLevel
}

func statusForLevel(level string, thresholds severityThresholds) nagiosStatus {
	levelValue, ok := logger.Levels()[strings.ToUpper(strings.TrimSpace(level))]
	if !ok {
		return nagiosStatus{text: "UNKNOWN", code: 3}
	}
	switch {
	case levelValue >= thresholds.criticalValue:
		return nagiosStatus{text: "CRITICAL", code: 2}
	case levelValue >= thresholds.warningValue:
		return nagiosStatus{text: "WARNING", code: 1}
	default:
		return nagiosStatus{text: "OK", code: 0}
	}
}

func severityLevels() []string {
	levels := logger.Levels()
	names := make([]string, 0, len(levels))
	for name := range levels {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		left := levels[names[i]]
		right := levels[names[j]]
		if left == right {
			return names[i] < names[j]
		}
		return left < right
	})
	return names
}

func lowestSeverityLevel(levels map[string]int) string {
	names := make([]string, 0, len(levels))
	for name := range levels {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		left := levels[names[i]]
		right := levels[names[j]]
		if left == right {
			return names[i] < names[j]
		}
		return left < right
	})
	if len(names) == 0 {
		return "DEBUG3"
	}
	return names[0]
}

func shouldPrintVerbose(level string, verbose int) bool {
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case "INFO":
		return verbose >= 3
	case "NOTICE":
		return verbose >= 2
	case "WARNING", "ERROR", "CRITICAL":
		return verbose >= 1
	default:
		return false
	}
}

func translatedMessage(entry engine.LogEntry) string {
	message, found := i18n.TranslateWithStatus("", entry.Module, entry.Tag, entry.Args)
	if found {
		return strings.TrimSpace(message)
	}
	tmp, err := logger.NewEntry(entry.Tag, entry.Args, entry.Testcase, entry.Module)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(tmp.String())
}

func moduleVersion(path string) string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info == nil {
		return "unknown"
	}
	if info.Main.Path == path {
		return normalizeVersion(info.Main.Version)
	}
	for _, dep := range info.Deps {
		if dep == nil || dep.Path != path {
			continue
		}
		if dep.Replace != nil {
			if dep.Replace.Version != "" {
				return normalizeVersion(dep.Replace.Version)
			}
			return dep.Replace.Path
		}
		return normalizeVersion(dep.Version)
	}
	return "unknown"
}

func normalizeVersion(version string) string {
	if version == "" {
		return "unknown"
	}
	return version
}

// buildMergedProfile returns the profile path to use for the engine request.
// If rrsigWarnDays > 0, it merges a REMAINING_SHORT override into the base
// profile (or an empty profile if basePath is ""), writes a temp file, and
// returns a cleanup function. If neither basePath nor rrsigWarnDays produces
// any change, it returns basePath unchanged with no cleanup.
func buildMergedProfile(basePath string, rrsigWarnDays int) (string, func(), error) {
	if rrsigWarnDays <= 0 {
		return basePath, nil, nil
	}
	base := profile.New()
	if basePath != "" {
		data, err := os.ReadFile(basePath)
		if err != nil {
			return "", nil, fmt.Errorf("--profile: %w", err)
		}
		loaded, err := profile.FromYAML(string(data))
		if err != nil {
			return "", nil, fmt.Errorf("--profile: %w", err)
		}
		base = loaded
	}
	overrideMap := map[string]any{
		"test_cases_vars": map[string]any{
			"dnssec04": map[string]any{
				"REMAINING_SHORT": rrsigWarnDays * 86400,
			},
		},
	}
	payload, err := json.Marshal(overrideMap)
	if err != nil {
		return "", nil, err
	}
	overrideProfile, err := profile.FromJSON(string(payload))
	if err != nil {
		return "", nil, fmt.Errorf("--rrsig-warn-days: %w", err)
	}
	if err := base.Merge(overrideProfile); err != nil {
		return "", nil, fmt.Errorf("--rrsig-warn-days: %w", err)
	}
	merged, err := base.ToJSON()
	if err != nil {
		return "", nil, err
	}
	tmp, err := os.CreateTemp("", "gonemaster-nagios-profile-*.json")
	if err != nil {
		return "", nil, err
	}
	if _, err := tmp.Write([]byte(merged)); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return "", nil, err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return "", nil, err
	}
	return tmp.Name(), func() { _ = os.Remove(tmp.Name()) }, nil
}
