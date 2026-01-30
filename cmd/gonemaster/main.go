package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"codeberg.org/pawal/gonemaster/engine"
)

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
	var dumpProfile bool
	var locale string
	var noIPv4 bool
	var noIPv6 bool
	var parallel int
	var parallelSet bool
	var noProgress bool
	var listTests bool
	var showVersion bool

	fs := flag.NewFlagSet("gonemaster", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.Usage = func() {
		fmt.Fprintf(errOut, "Usage: %s --domain DOMAIN [--module MODULE] [--testcase TESTCASE] [--profile PATH] [--min-level LEVEL] [--output PATH] [--raw] [--json] [--dump-profile] [--locale LOCALE] [--no-ipv4] [--no-ipv6] [--parallel N] [--no-progress] [--list-tests] [--version]\n", fs.Name())
		fmt.Fprintln(errOut, "")
		fmt.Fprintln(errOut, "Options:")
		fmt.Fprintln(errOut, "  --domain     Zone name to test (required)")
		fmt.Fprintln(errOut, "  --module     Run a single module (optional)")
		fmt.Fprintln(errOut, "  --testcase   Run a single testcase (optional)")
		fmt.Fprintln(errOut, "  --profile    Profile JSON/YAML path (optional)")
		fmt.Fprintln(errOut, "  --min-level  Minimum log level (optional, default NOTICE)")
		fmt.Fprintln(errOut, "  --output     Write output to file (optional)")
		fmt.Fprintln(errOut, "  --raw        Stream raw log entries as they are produced (optional)")
		fmt.Fprintln(errOut, "  --json       Print JSON output instead of translated output (optional)")
		fmt.Fprintln(errOut, "  --dump-profile  Print effective profile in JSON and exit (optional)")
		fmt.Fprintln(errOut, "  --locale     Locale for translated output (optional)")
		fmt.Fprintln(errOut, "  --no-ipv4    Disable IPv4 queries (optional)")
		fmt.Fprintln(errOut, "  --no-ipv6    Disable IPv6 queries (optional)")
		fmt.Fprintln(errOut, "  --parallel   Override resolver.defaults.parallel (optional)")
		fmt.Fprintln(errOut, "  --no-progress  Disable progress indicator (optional)")
		fmt.Fprintln(errOut, "  --list-tests  List all available test cases (optional)")
		fmt.Fprintln(errOut, "  --version    Print version and exit (optional)")
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
	fs.BoolVar(&dumpProfile, "dump-profile", false, "Print effective profile in JSON and exit (optional)")
	fs.StringVar(&locale, "locale", "", "Locale for translated output (optional)")
	fs.BoolVar(&noIPv4, "no-ipv4", false, "Disable IPv4 queries (optional)")
	fs.BoolVar(&noIPv6, "no-ipv6", false, "Disable IPv6 queries (optional)")
	fs.IntVar(&parallel, "parallel", 0, "Override resolver.defaults.parallel (optional)")
	fs.BoolVar(&noProgress, "no-progress", false, "Disable progress indicator (optional)")
	fs.BoolVar(&listTests, "list-tests", false, "List all available test cases (optional)")
	fs.BoolVar(&showVersion, "version", false, "Print version and exit (optional)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "parallel" {
			parallelSet = true
		}
	})

	if showVersion {
		fmt.Fprintf(out, "Gonemaster version %s\n", engine.VersionFull())
		fmt.Fprintf(out, "Miekg DNS version %s\n", moduleVersion("github.com/miekg/dns"))
		return 0
	}

	if listTests {
		for _, testCase := range engine.AvailableTestcases() {
			fmt.Fprintln(out, testCase)
		}
		return 0
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
	var parallelOverride *int
	if parallelSet {
		if parallel < 1 {
			fmt.Fprintln(errOut, "--parallel must be >= 1")
			return 2
		}
		value := parallel
		parallelOverride = &value
	}

	req := engine.RunRequest{
		Domain:   domain,
		Module:   module,
		Testcase: testcase,
		Profile:  profile,
		MinLevel: minLevel,
		IPv4:     ipv4Override,
		IPv6:     ipv6Override,
		Parallel: parallelOverride,
	}

	if dumpProfile {
		if raw {
			fmt.Fprintln(errOut, "--dump-profile cannot be combined with --raw")
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

	if domain == "" {
		fmt.Fprintln(errOut, "--domain is required")
		return 2
	}

	var rawWriter io.Writer
	humanWriter := out
	humanStreaming := false
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
		humanReporter := newHumanReporter(w, locale, minLevel, showSpinner)
		if humanReporter != nil {
			req.LogCallback = humanReporter.Callback
			defer humanReporter.Finish()
			humanStreaming = true
			if showSpinner {
				stopInterruptHandler = installInterruptHandler(humanReporter.Finish)
			}
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
				req.LogCallback = progress.Callback
			}
		}
	}

	entries, err := engine.Run(req)
	if progress != nil {
		progress.Finish()
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

	if !jsonOutput {
		if !humanStreaming {
			if writeErr := writeHuman(entries, locale, humanWriter); writeErr != nil {
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
