package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/normalization"
)

var runEngine = engine.Run

type repeatableStringFlag []string
type usageLine struct {
	flag   string
	detail string
}

func (f *repeatableStringFlag) String() string {
	if f == nil || len(*f) == 0 {
		return ""
	}
	return fmt.Sprintf("%v", []string(*f))
}

func (f *repeatableStringFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, out io.Writer, errOut io.Writer) int {
	var domain string
	var module string
	var testcase string
	var profile string
	var minLevel = "NOTICE"
	var output string
	var raw bool
	var jsonOutput bool
	var jsonStream bool
	var dumpProfile bool
	var locale string
	var stopLevel string
	var noIPv4 bool
	var noIPv6 bool
	var forceIPv6 bool
	var parallel int
	var parallelSet bool
	var unordered bool
	var unorderedSet bool
	var ordered bool
	var orderedSet bool
	var errorCacheTTL int
	var errorCacheTTLSet bool
	var timeoutSeconds int
	var timeoutSet bool
	var retryCount int
	var retrySet bool
	var retransSeconds int
	var retransSet bool
	var fallback bool
	var fallbackSet bool
	var noFallback bool
	var noFallbackSet bool
	var sourceAddr4 string
	var sourceAddr4Set bool
	var sourceAddr6 string
	var sourceAddr6Set bool
	var positiveCacheTTL int
	var positiveCacheTTLSet bool
	var negativeCacheTTL int
	var negativeCacheTTLSet bool
	var savePacketCachePath string
	var restorePacketCachePath string
	var noProgress bool
	var count bool
	var listTests bool
	var showVersion bool
	var stopLevelSet bool
	var undelegatedNSSpecs repeatableStringFlag
	var undelegatedDSSpecs repeatableStringFlag

	fs := flag.NewFlagSet("gonemaster", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.Usage = func() {
		fmt.Fprintf(errOut, "Usage: %s [flags] [DOMAIN]\n\n", fs.Name())
		fmt.Fprintln(errOut, "Flags:")
		printUsageGroup(errOut, "Target", []usageLine{
			{flag: "DOMAIN", detail: "Zone name to test (positional alternative to --domain)"},
			{flag: "--domain DOMAIN", detail: "Zone name to test (required for runs if positional DOMAIN is not provided)"},
			{flag: "--module MODULE", detail: "Run a single module"},
			{flag: "--testcase TESTCASE", detail: "Run a single testcase"},
			{flag: "--profile PATH", detail: "Profile JSON/YAML path"},
		})
		printUsageGroup(errOut, "Output", []usageLine{
			{flag: "--min-level LEVEL", detail: "Minimum log level (default NOTICE)"},
			{flag: "--stop-level LEVEL", detail: "Stop the run after first log entry at LEVEL or higher"},
			{flag: "--locale LOCALE", detail: "Locale for translated output"},
			{flag: "--output PATH", detail: "Write output to file"},
			{flag: "--raw", detail: "Stream raw log entries"},
			{flag: "--json", detail: "Print a JSON array of log entries"},
			{flag: "--json-stream", detail: "Stream JSON log entries"},
			{flag: "--count", detail: "Print count summary by level and message tag"},
			{flag: "--no-progress", detail: "Disable progress indicator"},
		})
		printUsageGroup(errOut, "Cache", []usageLine{
			{flag: "--save PATH", detail: "Write DNS packet cache to file after the run"},
			{flag: "--restore PATH", detail: "Prime DNS packet cache from file before the run"},
		})
		printUsageGroup(errOut, "Resolver/Profile Overrides", []usageLine{
			{flag: "--no-ipv4", detail: "Disable IPv4 queries"},
			{flag: "--no-ipv6", detail: "Disable IPv6 queries"},
			{flag: "--ipv6", detail: "Force IPv6 queries"},
			{flag: "--parallel N", detail: "Override resolver.defaults.parallel"},
			{flag: "--unordered", detail: "Allow unordered resolver behavior"},
			{flag: "--ordered", detail: "Force ordered resolver behavior"},
			{flag: "--timeout N", detail: "Override resolver.defaults.timeout (seconds)"},
			{flag: "--retry N", detail: "Override resolver.defaults.retry"},
			{flag: "--retrans N", detail: "Override resolver.defaults.retrans (seconds)"},
			{flag: "--fallback", detail: "Enable TCP fallback on UDP failure"},
			{flag: "--no-fallback", detail: "Disable TCP fallback on UDP failure"},
			{flag: "--sourceaddr4 IPADDR", detail: "Override resolver.source4 (IPv4 source address)"},
			{flag: "--sourceaddr6 IPADDR", detail: "Override resolver.source6 (IPv6 source address)"},
			{flag: "--error-cache-ttl N", detail: "Skip query retry after network errors (seconds)"},
			{flag: "--positive-cache-ttl N", detail: "Cache positive DNS responses (seconds)"},
			{flag: "--negative-cache-ttl N", detail: "Cache negative DNS responses (seconds)"},
		})
		printUsageGroup(errOut, "Undelegated", []usageLine{
			{flag: "--ns NAME[/IP]", detail: "Undelegated nameserver (repeatable)"},
			{flag: "--ds KEYTAG,ALGORITHM,DIGTYPE,DIGEST", detail: "Undelegated DS info (repeatable)"},
		})
		printUsageGroup(errOut, "Utility", []usageLine{
			{flag: "--dump-profile", detail: "Print effective profile in JSON and exit"},
			{flag: "--list-tests", detail: "List all available test cases and exit"},
			{flag: "--version", detail: "Print version information and exit"},
		})
		fmt.Fprintln(errOut, "Undelegated examples:")
		fmt.Fprintln(errOut, "  --ns ns1.example.com/192.0.2.10 --ns ns1.example.com/2001:db8::10")
		fmt.Fprintln(errOut, "  --ds 12345,13,2,0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF")
		fmt.Fprintln(errOut, "")
		fmt.Fprintln(errOut, "Exit codes:")
		fmt.Fprintln(errOut, "  0   Success")
		fmt.Fprintln(errOut, "  2   Usage or runtime error")
		fmt.Fprintln(errOut, "  130 Interrupted (SIGINT/SIGTERM)")
	}
	fs.StringVar(&domain, "domain", "", "Zone name to test (required)")
	fs.StringVar(&module, "module", "", "Run a single module (optional)")
	fs.StringVar(&testcase, "testcase", "", "Run a single testcase (optional)")
	fs.StringVar(&profile, "profile", "", "Profile JSON/YAML path (optional)")
	fs.StringVar(&minLevel, "min-level", "NOTICE", "Minimum log level (optional, default NOTICE)")
	fs.StringVar(&output, "output", "", "Write output to file (optional)")
	fs.BoolVar(&raw, "raw", false, "Stream raw log entries as they are produced (optional)")
	fs.BoolVar(&jsonOutput, "json", false, "Print JSON output instead of translated output (optional)")
	fs.BoolVar(&jsonStream, "json-stream", false, "Stream JSON log entries as they are produced (optional)")
	fs.BoolVar(&dumpProfile, "dump-profile", false, "Print effective profile in JSON and exit (optional)")
	fs.StringVar(&stopLevel, "stop-level", "", "Stop the run after first log entry at this level or higher (optional)")
	fs.StringVar(&locale, "locale", "", "Locale for translated output (optional)")
	fs.BoolVar(&noIPv4, "no-ipv4", false, "Disable IPv4 queries (optional)")
	fs.BoolVar(&noIPv6, "no-ipv6", false, "Disable IPv6 queries (optional)")
	fs.BoolVar(&forceIPv6, "ipv6", false, "Force IPv6 queries (optional)")
	fs.IntVar(&parallel, "parallel", 0, "Override resolver.defaults.parallel (optional)")
	fs.BoolVar(&unordered, "unordered", false, "Allow unordered resolver behavior (optional, override profile)")
	fs.BoolVar(&ordered, "ordered", false, "Force ordered resolver behavior (optional, override profile)")
	fs.IntVar(&timeoutSeconds, "timeout", 0, "Override resolver.defaults.timeout in seconds (optional)")
	fs.IntVar(&retryCount, "retry", 0, "Override resolver.defaults.retry (optional)")
	fs.IntVar(&retransSeconds, "retrans", 0, "Override resolver.defaults.retrans in seconds (optional)")
	fs.BoolVar(&fallback, "fallback", false, "Enable TCP fallback on UDP failure (optional)")
	fs.BoolVar(&noFallback, "no-fallback", false, "Disable TCP fallback on UDP failure (optional)")
	fs.StringVar(&sourceAddr4, "sourceaddr4", "", "Override resolver.source4 (IPv4 source address) (optional)")
	fs.StringVar(&sourceAddr6, "sourceaddr6", "", "Override resolver.source6 (IPv6 source address) (optional)")
	fs.IntVar(&errorCacheTTL, "error-cache-ttl", 0, "Seconds to skip queries after network errors (optional)")
	fs.IntVar(&positiveCacheTTL, "positive-cache-ttl", 0, "Seconds to cache positive DNS responses (optional)")
	fs.IntVar(&negativeCacheTTL, "negative-cache-ttl", 0, "Seconds to cache negative DNS responses (optional)")
	fs.StringVar(&savePacketCachePath, "save", "", "Write DNS packet cache to file after the run (optional)")
	fs.StringVar(&restorePacketCachePath, "restore", "", "Prime DNS packet cache from file before the run (optional)")
	fs.Var(&undelegatedNSSpecs, "ns", "Undelegated nameserver as name[/ip] (repeatable)")
	fs.Var(&undelegatedDSSpecs, "ds", "Undelegated DS as keytag,algorithm,digtype,digest (repeatable)")
	fs.BoolVar(&noProgress, "no-progress", false, "Disable progress indicator (optional)")
	fs.BoolVar(&count, "count", false, "Print count summary by level and message tag (optional)")
	fs.BoolVar(&listTests, "list-tests", false, "List all available test cases (optional)")
	fs.BoolVar(&showVersion, "version", false, "Print version and exit (optional)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	positional := fs.Args()
	if len(positional) > 1 {
		fmt.Fprintln(errOut, "only one positional DOMAIN argument is allowed")
		return 2
	}
	if len(positional) == 1 {
		if strings.TrimSpace(domain) != "" {
			fmt.Fprintln(errOut, "domain provided twice; use either --domain DOMAIN or positional DOMAIN")
			return 2
		}
		domain = strings.TrimSpace(positional[0])
	}
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "parallel" {
			parallelSet = true
		}
		if f.Name == "unordered" {
			unorderedSet = true
		}
		if f.Name == "ordered" {
			orderedSet = true
		}
		if f.Name == "error-cache-ttl" {
			errorCacheTTLSet = true
		}
		if f.Name == "timeout" {
			timeoutSet = true
		}
		if f.Name == "retry" {
			retrySet = true
		}
		if f.Name == "retrans" {
			retransSet = true
		}
		if f.Name == "fallback" {
			fallbackSet = true
		}
		if f.Name == "no-fallback" {
			noFallbackSet = true
		}
		if f.Name == "sourceaddr4" {
			sourceAddr4Set = true
		}
		if f.Name == "sourceaddr6" {
			sourceAddr6Set = true
		}
		if f.Name == "positive-cache-ttl" {
			positiveCacheTTLSet = true
		}
		if f.Name == "negative-cache-ttl" {
			negativeCacheTTLSet = true
		}
		if f.Name == "stop-level" {
			stopLevelSet = true
		}
	})

	hasPacketCacheFlags := strings.TrimSpace(savePacketCachePath) != "" || strings.TrimSpace(restorePacketCachePath) != ""
	if hasPacketCacheFlags && showVersion {
		fmt.Fprintln(errOut, "--save/--restore cannot be combined with --version")
		return 2
	}
	if hasPacketCacheFlags && listTests {
		fmt.Fprintln(errOut, "--save/--restore cannot be combined with --list-tests")
		return 2
	}

	if showVersion {
		fmt.Fprintf(out, "Gonemaster version %s\n", engine.VersionFull())
		fmt.Fprintf(out, "Miekg DNS version %s\n", moduleVersion("codeberg.org/miekg/dns"))
		return 0
	}

	if listTests {
		for _, testCase := range engine.AvailableTestcases() {
			fmt.Fprintln(out, testCase)
		}
		return 0
	}

	normalizedStopLevel := ""
	if stopLevelSet {
		normalized, levelErr := normalizeStopLevel(stopLevel)
		if levelErr != nil {
			fmt.Fprintln(errOut, levelErr.Error())
			return 2
		}
		normalizedStopLevel = normalized
	}

	var ipv4Override *bool
	if noIPv4 {
		value := false
		ipv4Override = &value
	}
	var ipv6Override *bool
	if noIPv6 {
		value := false
		ipv6Override = &value
	}
	if forceIPv6 {
		value := true
		ipv6Override = &value
	}
	var parallelOverride *int
	if parallelSet {
		if parallel < 1 {
			fmt.Fprintln(errOut, "--parallel must be >= 1")
			return 2
		}
		value := parallel
		parallelOverride = &value
	}
	var unorderedOverride *bool
	if unorderedSet {
		value := unordered
		unorderedOverride = &value
	}
	if orderedSet {
		value := false
		unorderedOverride = &value
	}
	var errorCacheOverride *int
	if errorCacheTTLSet {
		if errorCacheTTL < 0 {
			fmt.Fprintln(errOut, "--error-cache-ttl must be >= 0")
			return 2
		}
		value := errorCacheTTL
		errorCacheOverride = &value
	}
	var timeoutOverride *int
	if timeoutSet {
		if timeoutSeconds < 0 {
			fmt.Fprintln(errOut, "--timeout must be >= 0")
			return 2
		}
		value := timeoutSeconds
		timeoutOverride = &value
	}
	var retryOverride *int
	if retrySet {
		if retryCount < 0 {
			fmt.Fprintln(errOut, "--retry must be >= 0")
			return 2
		}
		value := retryCount
		retryOverride = &value
	}
	var retransOverride *int
	if retransSet {
		if retransSeconds < 0 {
			fmt.Fprintln(errOut, "--retrans must be >= 0")
			return 2
		}
		value := retransSeconds
		retransOverride = &value
	}
	var fallbackOverride *bool
	if fallbackSet {
		value := true
		fallbackOverride = &value
	}
	if noFallbackSet {
		value := false
		fallbackOverride = &value
	}
	var sourceAddr4Override *string
	if sourceAddr4Set {
		value := strings.TrimSpace(sourceAddr4)
		parsed := net.ParseIP(value)
		if parsed == nil || parsed.To4() == nil {
			fmt.Fprintln(errOut, "--sourceaddr4 must be a valid IPv4 address")
			return 2
		}
		sourceAddr4Override = &value
	}
	var sourceAddr6Override *string
	if sourceAddr6Set {
		value := strings.TrimSpace(sourceAddr6)
		parsed := net.ParseIP(value)
		if parsed == nil || parsed.To4() != nil {
			fmt.Fprintln(errOut, "--sourceaddr6 must be a valid IPv6 address")
			return 2
		}
		sourceAddr6Override = &value
	}
	var positiveCacheOverride *int
	if positiveCacheTTLSet {
		if positiveCacheTTL < 0 {
			fmt.Fprintln(errOut, "--positive-cache-ttl must be >= 0")
			return 2
		}
		value := positiveCacheTTL
		positiveCacheOverride = &value
	}
	var negativeCacheOverride *int
	if negativeCacheTTLSet {
		if negativeCacheTTL < 0 {
			fmt.Fprintln(errOut, "--negative-cache-ttl must be >= 0")
			return 2
		}
		value := negativeCacheTTL
		negativeCacheOverride = &value
	}
	if orderedSet && unorderedSet {
		fmt.Fprintln(errOut, "--ordered cannot be combined with --unordered")
		return 2
	}
	if fallbackSet && noFallbackSet {
		fmt.Fprintln(errOut, "--fallback cannot be combined with --no-fallback")
		return 2
	}
	if noIPv6 && forceIPv6 {
		fmt.Fprintln(errOut, "--no-ipv6 cannot be combined with --ipv6")
		return 2
	}

	req := engine.RunRequest{
		Domain:           domain,
		Module:           module,
		Testcase:         testcase,
		Profile:          profile,
		MinLevel:         minLevel,
		IPv4:             ipv4Override,
		IPv6:             ipv6Override,
		Parallel:         parallelOverride,
		Unordered:        unorderedOverride,
		ErrorCacheTTL:    errorCacheOverride,
		Timeout:          timeoutOverride,
		Retry:            retryOverride,
		Retrans:          retransOverride,
		Fallback:         fallbackOverride,
		SourceAddr4:      sourceAddr4Override,
		SourceAddr6:      sourceAddr6Override,
		PositiveCacheTTL: positiveCacheOverride,
		NegativeCacheTTL: negativeCacheOverride,
	}
	var stopController *stopLevelController
	var stopCapture *entryCaptureReporter
	if stopLevelSet {
		baseCtx := req.Context
		if baseCtx == nil {
			baseCtx = context.Background()
		}
		runCtx, cancelStop := context.WithCancelCause(baseCtx)
		defer cancelStop(nil)
		req.Context = runCtx
		stopController = newStopLevelController(normalizedStopLevel, cancelStop)
		stopCapture = newEntryCaptureReporter()
	}
	for _, spec := range undelegatedNSSpecs {
		nsItem, parseErr := engine.ParseUndelegatedNameserver(spec)
		if parseErr != nil {
			fmt.Fprintln(errOut, parseErr.Error())
			return 2
		}
		req.UndelegatedNameservers = append(req.UndelegatedNameservers, nsItem)
	}
	for _, spec := range undelegatedDSSpecs {
		dsItem, parseErr := engine.ParseUndelegatedDS(spec)
		if parseErr != nil {
			fmt.Fprintln(errOut, parseErr.Error())
			return 2
		}
		req.UndelegatedDSInfo = append(req.UndelegatedDSInfo, dsItem)
	}

	if dumpProfile {
		if raw || jsonStream || count || strings.TrimSpace(savePacketCachePath) != "" || strings.TrimSpace(restorePacketCachePath) != "" {
			if raw {
				fmt.Fprintln(errOut, "--dump-profile cannot be combined with --raw")
			} else if jsonStream {
				fmt.Fprintln(errOut, "--dump-profile cannot be combined with --json-stream")
			} else if strings.TrimSpace(savePacketCachePath) != "" {
				fmt.Fprintln(errOut, "--dump-profile cannot be combined with --save")
			} else if strings.TrimSpace(restorePacketCachePath) != "" {
				fmt.Fprintln(errOut, "--dump-profile cannot be combined with --restore")
			} else {
				fmt.Fprintln(errOut, "--dump-profile cannot be combined with --count")
			}
			return 2
		}
		effectiveProfile, err := engine.EffectiveProfile(req)
		if err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 2
		}
		payload, err := effectiveProfile.ToJSON()
		if err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 2
		}
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, []byte(payload), "", "  "); err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 2
		}
		w := out
		if output != "" {
			f, fileErr := os.Create(output)
			if fileErr != nil {
				fmt.Fprintln(errOut, fileErr.Error())
				return 2
			}
			defer f.Close()
			w = f
		}
		fmt.Fprintln(w, pretty.String())
		return 0
	}

	if raw && jsonOutput {
		fmt.Fprintln(errOut, "--json cannot be combined with --raw")
		return 2
	}
	if raw && jsonStream {
		fmt.Fprintln(errOut, "--json-stream cannot be combined with --raw")
		return 2
	}
	if jsonOutput && jsonStream {
		fmt.Fprintln(errOut, "--json-stream cannot be combined with --json")
		return 2
	}
	if count && raw {
		fmt.Fprintln(errOut, "--count cannot be combined with --raw")
		return 2
	}
	if count && jsonOutput {
		fmt.Fprintln(errOut, "--count cannot be combined with --json")
		return 2
	}
	if count && jsonStream {
		fmt.Fprintln(errOut, "--count cannot be combined with --json-stream")
		return 2
	}

	if domain == "" {
		fmt.Fprintln(errOut, "--domain is required (or pass DOMAIN as a positional argument)")
		return 2
	}
	if errs, normalized := normalization.NormalizeName(domain); len(errs) > 0 {
		fmt.Fprintln(errOut, errs[0].Message())
		return 2
	} else if normalized != "" {
		domain = normalized
		req.Domain = normalized
	}

	var packetCacheStore *nameserver.CacheStore
	if strings.TrimSpace(savePacketCachePath) != "" || strings.TrimSpace(restorePacketCachePath) != "" {
		packetCacheStore = nameserver.NewCacheStore()
		if strings.TrimSpace(restorePacketCachePath) != "" {
			if restoreErr := packetCacheStore.RestorePacketCache(restorePacketCachePath); restoreErr != nil {
				fmt.Fprintln(errOut, restoreErr.Error())
				return 2
			}
		}
		req.NameserverCache = packetCacheStore
	}

	var rawWriter io.Writer
	humanWriter := out
	humanStreaming := false
	var humanReport *humanReporter
	var countReport *countReporter
	var stopInterruptHandler func()
	if raw {
		rawWriter = out
		if output != "" {
			f, fileErr := os.Create(output)
			if fileErr != nil {
				fmt.Fprintln(errOut, fileErr.Error())
				return 2
			}
			defer f.Close()
			rawWriter = f
		}
		rawReporter := newRawReporter(rawWriter, minLevel)
		if rawReporter != nil {
			req.LogCallback = rawReporter.Callback
		}
	} else if jsonStream {
		jsonWriter := out
		if output != "" {
			f, fileErr := os.Create(output)
			if fileErr != nil {
				fmt.Fprintln(errOut, fileErr.Error())
				return 2
			}
			defer f.Close()
			jsonWriter = f
		}
		jsonReporter := newJSONStreamReporter(jsonWriter, minLevel)
		if jsonReporter != nil {
			req.LogCallback = jsonReporter.Callback
		}
	} else if !jsonOutput {
		w := out
		if output != "" {
			f, fileErr := os.Create(output)
			if fileErr != nil {
				fmt.Fprintln(errOut, fileErr.Error())
				return 2
			}
			defer f.Close()
			w = f
		}
		humanWriter = w
		showSpinner := !noProgress && isTerminalWriter(w)
		humanReport = newHumanReporter(w, locale, minLevel, showSpinner)
		if count {
			countReport = newCountReporter()
		}
		if humanReport != nil {
			req.LogCallback = humanReport.Callback
			if countReport != nil {
				req.LogCallback = composeCallbacks(req.LogCallback, countReport.Callback)
			}
			defer humanReport.Finish()
			humanStreaming = true
			if showSpinner {
				stopInterruptHandler = installInterruptHandler(humanReport.Finish)
			}
		} else if countReport != nil {
			req.LogCallback = countReport.Callback
		}
	}

	if stopInterruptHandler != nil {
		defer stopInterruptHandler()
	}

	var progress *progressReporter
	if !raw && !noProgress && jsonOutput && isTerminalWriter(errOut) {
		planned, planErr := engine.PlannedTestcases(req)
		if planErr == nil {
			progress = newProgressReporter(errOut, planned)
			if progress != nil {
				req.LogCallback = composeCallbacks(req.LogCallback, progress.Callback)
			}
		}
	}
	if stopController != nil && stopCapture != nil {
		req.LogCallback = composeCallbacks(stopController.Callback, stopCapture.Callback, req.LogCallback)
	}

	entries, err := runEngine(req)
	if progress != nil {
		progress.Finish()
	}
	if stopController != nil && stopCapture != nil && stopController.Triggered() && req.Context != nil && errors.Is(context.Cause(req.Context), errStopLevelReached) {
		if err != nil && errors.Is(err, context.Canceled) {
			err = nil
		}
		if jsonOutput {
			filteredEntries, filterErr := stopCapture.FilteredEntries(minLevel)
			if filterErr != nil {
				fmt.Fprintln(errOut, filterErr.Error())
				return 2
			}
			entries = filteredEntries
		}
	}
	if packetCacheStore != nil && strings.TrimSpace(savePacketCachePath) != "" {
		if saveErr := packetCacheStore.SavePacketCache(savePacketCachePath); saveErr != nil {
			fmt.Fprintln(errOut, saveErr.Error())
			return 2
		}
	}
	if !jsonOutput && humanStreaming && err == nil && len(entries) == 0 && humanReport != nil {
		if writeErr := humanReport.PrintLooksOK(); writeErr != nil {
			fmt.Fprintln(errOut, writeErr.Error())
			return 2
		}
	}
	if err != nil && !errors.Is(err, engine.ErrNotImplemented) && !raw {
		fmt.Fprintln(errOut, err.Error())
	}

	if raw {
		if err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 2
		}
		return 0
	}
	if jsonStream {
		if err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 2
		}
		return 0
	}

	if !jsonOutput {
		if !humanStreaming {
			if writeErr := writeHuman(entries, locale, humanWriter); writeErr != nil {
				fmt.Fprintln(errOut, writeErr.Error())
				return 2
			}
		}
		if countReport != nil {
			countLines := countReport.SummaryLines()
			if humanReport != nil {
				for _, line := range countLines {
					if writeErr := humanReport.printLine(line); writeErr != nil {
						fmt.Fprintln(errOut, writeErr.Error())
						return 2
					}
				}
			} else if writeErr := writeLines(humanWriter, countLines); writeErr != nil {
				fmt.Fprintln(errOut, writeErr.Error())
				return 2
			}
		}
		if err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 2
		}
		return 0
	}

	var w io.Writer = out
	if output != "" {
		f, fileErr := os.Create(output)
		if fileErr != nil {
			fmt.Fprintln(errOut, fileErr.Error())
			return 2
		}
		defer f.Close()
		w = f
	}

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if encodeErr := enc.Encode(entries); encodeErr != nil {
		fmt.Fprintln(errOut, encodeErr.Error())
		return 2
	}

	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	return 0
}

func installInterruptHandler(cleanup func()) func() {
	if cleanup == nil {
		return nil
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	done := make(chan struct{})

	go func() {
		select {
		case <-signals:
			cleanup()
			signal.Stop(signals)
			os.Exit(130)
		case <-done:
		}
	}()

	return func() {
		close(done)
		signal.Stop(signals)
	}
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

func writeLines(out io.Writer, lines []string) error {
	for _, line := range lines {
		if _, err := fmt.Fprintln(out, line); err != nil {
			return err
		}
	}
	return nil
}

func printUsageGroup(out io.Writer, title string, lines []usageLine) {
	if len(lines) == 0 {
		return
	}
	fmt.Fprintf(out, "  %s:\n", title)
	for _, line := range lines {
		fmt.Fprintf(out, "    %-42s %s\n", line.flag, line.detail)
	}
	fmt.Fprintln(out, "")
}
