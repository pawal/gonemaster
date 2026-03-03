package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var placeholderPattern = regexp.MustCompile(`\{[A-Za-z0-9_]+\}`)

var legacyPlaceholders = map[string]bool{
	"nsname":     true,
	"ns_ip":      true,
	"ns_list":    true,
	"ns_ip_list": true,
	"asn_list":   true,
	"type":       true,
	"class":      true,
	"rrtype":     true,
	"server":     true,
	"ip":         true,
}

// Keep allowlist support for deliberate short-lived exceptions, but the
// target state is an empty allowlist.
var legacyPlaceholderAllowlist = map[string]map[string]bool{}

type entry struct {
	file      string
	startLine int
	context   string
	msgID     string
	msgStr    string
}

func main() {
	files, err := resolveFiles(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "no .po files found")
		os.Exit(1)
	}

	var mismatches int
	var legacyViolations int
	for _, path := range files {
		entries, err := parsePO(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			os.Exit(1)
		}
		for _, e := range entries {
			missing, extra := comparePlaceholders(e.msgID, e.msgStr)
			if len(missing) == 0 && len(extra) == 0 {
				// Continue checking legacy keys even when parity is clean.
			} else {
				mismatches++
				ctx := e.context
				if ctx == "" {
					ctx = "<none>"
				}
				fmt.Printf("%s:%d: msgctxt=%s", e.file, e.startLine, ctx)
				if len(missing) > 0 {
					fmt.Printf(" missing=%s", strings.Join(missing, ","))
				}
				if len(extra) > 0 {
					fmt.Printf(" extra=%s", strings.Join(extra, ","))
				}
				fmt.Println()
			}
			for _, key := range legacyKeysForEntry(e) {
				legacyViolations++
				ctx := e.context
				if ctx == "" {
					ctx = "<none>"
				}
				fmt.Printf("%s:%d: msgctxt=%s legacy=%s\n", e.file, e.startLine, ctx, key)
			}
		}
	}

	if mismatches > 0 {
		fmt.Fprintf(os.Stderr, "placeholder parity check failed: %d mismatch(es)\n", mismatches)
	}
	if legacyViolations > 0 {
		fmt.Fprintf(os.Stderr, "legacy placeholder check failed: %d violation(s)\n", legacyViolations)
	}
	if mismatches > 0 || legacyViolations > 0 {
		os.Exit(1)
	}
}

func resolveFiles(args []string) ([]string, error) {
	if len(args) == 0 {
		args = []string{"share/lang/*.po"}
	}
	seen := map[string]bool{}
	var files []string
	for _, arg := range args {
		if strings.ContainsAny(arg, "*?[") {
			matches, err := filepath.Glob(arg)
			if err != nil {
				return nil, fmt.Errorf("invalid glob %q: %w", arg, err)
			}
			for _, m := range matches {
				if strings.HasSuffix(m, ".po") && !seen[m] {
					seen[m] = true
					files = append(files, m)
				}
			}
			continue
		}
		if !strings.HasSuffix(arg, ".po") {
			continue
		}
		if !seen[arg] {
			seen[arg] = true
			files = append(files, arg)
		}
	}
	sort.Strings(files)
	return files, nil
}

func parsePO(path string) ([]entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var (
		out          []entry
		cur          entry
		field        string
		entryStarted bool
		lineNo       int
	)
	cur.file = path

	flush := func() {
		if !entryStarted {
			return
		}
		// Keep header entry too; parity check naturally passes unless placeholders differ.
		out = append(out, cur)
		cur = entry{file: path}
		field = ""
		entryStarted = false
	}

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lineNo++
		line := sc.Text()

		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		if !entryStarted {
			entryStarted = true
			cur.startLine = lineNo
		}

		switch {
		case strings.HasPrefix(line, "#"):
			// Comment line.
			continue
		case strings.HasPrefix(line, "msgctxt "):
			s, err := parseFieldValue(line, "msgctxt ")
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", lineNo, err)
			}
			cur.context = s
			field = "context"
		case strings.HasPrefix(line, "msgid "):
			s, err := parseFieldValue(line, "msgid ")
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", lineNo, err)
			}
			cur.msgID = s
			field = "msgid"
		case strings.HasPrefix(line, "msgstr "):
			s, err := parseFieldValue(line, "msgstr ")
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", lineNo, err)
			}
			cur.msgStr = s
			field = "msgstr"
		case strings.HasPrefix(line, "\""):
			s, err := strconv.Unquote(line)
			if err != nil {
				return nil, fmt.Errorf("line %d: invalid quoted string: %w", lineNo, err)
			}
			switch field {
			case "context":
				cur.context += s
			case "msgid":
				cur.msgID += s
			case "msgstr":
				cur.msgStr += s
			}
		default:
			field = ""
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	flush()

	return out, nil
}

func parseFieldValue(line string, prefix string) (string, error) {
	raw := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	s, err := strconv.Unquote(raw)
	if err != nil {
		return "", fmt.Errorf("invalid %s string: %w", strings.TrimSpace(prefix), err)
	}
	return s, nil
}

func comparePlaceholders(msgID string, msgStr string) ([]string, []string) {
	idSet := placeholderSet(msgID)
	strSet := placeholderSet(msgStr)

	var missing []string
	for key := range idSet {
		if !strSet[key] {
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)

	var extra []string
	for key := range strSet {
		if !idSet[key] {
			extra = append(extra, key)
		}
	}
	sort.Strings(extra)

	return missing, extra
}

func placeholderSet(s string) map[string]bool {
	out := map[string]bool{}
	for _, token := range placeholderPattern.FindAllString(s, -1) {
		key := strings.TrimSuffix(strings.TrimPrefix(token, "{"), "}")
		if key != "" {
			out[key] = true
		}
	}
	return out
}

func legacyKeysForEntry(e entry) []string {
	idSet := placeholderSet(e.msgID)
	allowed := legacyPlaceholderAllowlist[e.context]
	var legacy []string
	for key := range idSet {
		if !legacyPlaceholders[key] {
			continue
		}
		if allowed != nil && allowed[key] {
			continue
		}
		legacy = append(legacy, key)
	}
	sort.Strings(legacy)
	return legacy
}
