package main

import (
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"runtime/debug"
	"strings"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/i18n"
	"codeberg.org/pawal/gonemaster/engine/logger"
)

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

var nagiosCodes = map[string]nagiosStatus{
	"DEBUG3":   {text: "OK", code: 0},
	"DEBUG2":   {text: "OK", code: 0},
	"DEBUG":    {text: "OK", code: 0},
	"INFO":     {text: "OK", code: 0},
	"NOTICE":   {text: "OK", code: 0},
	"WARNING":  {text: "WARNING", code: 1},
	"ERROR":    {text: "CRITICAL", code: 2},
	"CRITICAL": {text: "CRITICAL", code: 2},
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
	var noIPv4 bool
	var noIPv6 bool
	var disableIPv4 bool
	var disableIPv6 bool
	var forceIPv6 bool
	var sourceAddr4 string
	var sourceAddr6 string
	var sourceAddr4Set bool
	var sourceAddr6Set bool
	var showVersion bool
	var showHelp bool
	var verbose countFlag

	fs := flag.NewFlagSet("gonemaster-nagios", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.Usage = func() {
		fmt.Fprintf(errOut, "Usage: %s -d DOMAIN [-v|-vv|-vvv] [--module MODULE] [--testcase TESTCASE] [--profile PATH] [--no-ipv4|--disable-ipv4] [--no-ipv6|--disable-ipv6|--ipv6] [--sourceaddr4 IPADDR] [--sourceaddr6 IPADDR] [--version]\n", fs.Name())
		fmt.Fprintln(errOut, "")
		fmt.Fprintln(errOut, "Options:")
		fmt.Fprintln(errOut, "  -d, --domain   Zone name to test (required)")
		fmt.Fprintln(errOut, "  --module       Run a single module (optional)")
		fmt.Fprintln(errOut, "  --testcase     Run a single testcase (optional)")
		fmt.Fprintln(errOut, "  --profile      Profile JSON/YAML path (optional)")
		fmt.Fprintln(errOut, "  --no-ipv4      Disable IPv4 queries (optional)")
		fmt.Fprintln(errOut, "  --no-ipv6      Disable IPv6 queries (optional)")
		fmt.Fprintln(errOut, "  --disable-ipv4 Disable IPv4 queries (optional)")
		fmt.Fprintln(errOut, "  --disable-ipv6 Disable IPv6 queries (optional)")
		fmt.Fprintln(errOut, "  --ipv6         Force IPv6 queries (optional)")
		fmt.Fprintln(errOut, "  --sourceaddr4  Override resolver.source4 (IPv4 source address) (optional)")
		fmt.Fprintln(errOut, "  --sourceaddr6  Override resolver.source6 (IPv6 source address) (optional)")
		fmt.Fprintln(errOut, "  -v, --verbose  Increase verbosity (repeatable)")
		fmt.Fprintln(errOut, "  -V, --version  Print version and exit")
		fmt.Fprintln(errOut, "  -h, --help     Show help")
		fmt.Fprintln(errOut, "")
		fmt.Fprintln(errOut, "Exit codes:")
		fmt.Fprintln(errOut, "  0 OK")
		fmt.Fprintln(errOut, "  1 WARNING")
		fmt.Fprintln(errOut, "  2 CRITICAL")
		fmt.Fprintln(errOut, "  3 UNKNOWN")
	}

	fs.StringVar(&domain, "domain", "", "Zone name to test (required)")
	fs.StringVar(&domain, "d", "", "Zone name to test (required)")
	fs.StringVar(&module, "module", "", "Run a single module (optional)")
	fs.StringVar(&testcase, "testcase", "", "Run a single testcase (optional)")
	fs.StringVar(&profilePath, "profile", "", "Profile JSON/YAML path (optional)")
	fs.BoolVar(&noIPv4, "no-ipv4", false, "Disable IPv4 queries (optional)")
	fs.BoolVar(&noIPv6, "no-ipv6", false, "Disable IPv6 queries (optional)")
	fs.BoolVar(&disableIPv4, "disable-ipv4", false, "Disable IPv4 queries (optional)")
	fs.BoolVar(&disableIPv6, "disable-ipv6", false, "Disable IPv6 queries (optional)")
	fs.BoolVar(&forceIPv6, "ipv6", false, "Force IPv6 queries (optional)")
	fs.StringVar(&sourceAddr4, "sourceaddr4", "", "Override resolver.source4 (IPv4 source address) (optional)")
	fs.StringVar(&sourceAddr6, "sourceaddr6", "", "Override resolver.source6 (IPv6 source address) (optional)")
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
		case "sourceaddr4":
			sourceAddr4Set = true
		case "sourceaddr6":
			sourceAddr6Set = true
		}
	})

	if showHelp {
		fs.Usage()
		return 0
	}

	if showVersion {
		fmt.Fprintf(out, "Gonemaster version %s\n", engine.VersionFull())
		fmt.Fprintf(out, "Miekg DNS version %s\n", moduleVersion("github.com/miekg/dns"))
		return 0
	}

	if strings.TrimSpace(domain) == "" {
		fs.Usage()
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
		fmt.Fprintln(errOut, "--no-ipv6/--disable-ipv6 cannot be combined with --ipv6")
		return 3
	}
	var sourceAddr4Override *string
	if sourceAddr4Set {
		value := strings.TrimSpace(sourceAddr4)
		parsed := net.ParseIP(value)
		if parsed == nil || parsed.To4() == nil {
			fmt.Fprintln(errOut, "--sourceaddr4 must be a valid IPv4 address")
			return 3
		}
		sourceAddr4Override = &value
	}
	var sourceAddr6Override *string
	if sourceAddr6Set {
		value := strings.TrimSpace(sourceAddr6)
		parsed := net.ParseIP(value)
		if parsed == nil || parsed.To4() != nil {
			fmt.Fprintln(errOut, "--sourceaddr6 must be a valid IPv6 address")
			return 3
		}
		sourceAddr6Override = &value
	}

	entries, err := runEngine(engine.RunRequest{
		Domain:      domain,
		Module:      module,
		Testcase:    testcase,
		Profile:     profilePath,
		IPv4:        ipv4Override,
		IPv6:        ipv6Override,
		SourceAddr4: sourceAddr4Override,
		SourceAddr6: sourceAddr6Override,
	})
	if err != nil {
		fmt.Fprintf(out, "ZONE UNKNOWN - %s\n", err.Error())
		return 3
	}

	_, status := maxLevel(entries)
	if status.text == "" {
		status = nagiosStatus{text: "UNKNOWN", code: 3}
	}

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

func maxLevel(entries []engine.LogEntry) (string, nagiosStatus) {
	levels := logger.Levels()
	maxLevel := "INFO"
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
	return maxLevel, nagiosCodes[maxLevel]
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
