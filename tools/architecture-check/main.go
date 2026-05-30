// architecture-check enforces ground-truth consistency between
// docs/architecture.md and the codebase. Run from the repo root.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const maxReviewAgeDays = 180

func main() {
	docPath := flag.String("doc", "docs/architecture.md", "path to architecture doc")
	flag.Parse()

	docBytes, err := os.ReadFile(*docPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "architecture-check: read %s: %v\n", *docPath, err)
		os.Exit(2)
	}
	doc := string(docBytes)

	var failures []string
	failures = append(failures, checkBinaries(doc)...)
	failures = append(failures, checkBuildTags(doc)...)
	failures = append(failures, checkDrivers(doc)...)
	failures = append(failures, checkLastReviewed(doc)...)

	if len(failures) > 0 {
		fmt.Fprintf(os.Stderr, "architecture-check: drift detected in %s:\n", *docPath)
		for _, f := range failures {
			fmt.Fprintf(os.Stderr, "  - %s\n", f)
		}
		os.Exit(1)
	}
}

// checkBinaries asserts cmd/<name>/ directories equal the binaries referenced
// in the doc via [cmd/<name>/] markdown links.
func checkBinaries(doc string) []string {
	entries, err := os.ReadDir("cmd")
	if err != nil {
		return []string{fmt.Sprintf("read cmd/: %v", err)}
	}
	fsBins := map[string]bool{}
	for _, e := range entries {
		// cmd/internal/ is Go's convention for non-binary shared code.
		if e.IsDir() && e.Name() != "internal" {
			fsBins[e.Name()] = true
		}
	}

	re := regexp.MustCompile(`\[cmd/([a-z0-9-]+)/\]`)
	matches := re.FindAllStringSubmatch(doc, -1)
	docBins := map[string]bool{}
	for _, m := range matches {
		docBins[m[1]] = true
	}
	if len(docBins) == 0 {
		return []string{"no [cmd/<name>/] link references found in doc; Chapter 2 (Component map) link format may have changed"}
	}

	var failures []string
	for _, name := range sortedKeys(fsBins) {
		if !docBins[name] {
			failures = append(failures, fmt.Sprintf("cmd/%s/ exists but is not referenced in the doc; update Chapter 2 and Chapter 17", name))
		}
	}
	for _, name := range sortedKeys(docBins) {
		if !fsBins[name] {
			failures = append(failures, fmt.Sprintf("doc references cmd/%s/ but the directory does not exist", name))
		}
	}
	return failures
}

// checkBuildTags asserts every //go:build tag found in cmd/*.go is documented
// under "### Build tags" in the doc.
func checkBuildTags(doc string) []string {
	docTags := parseBuildTagsFromDoc(doc)
	if len(docTags) == 0 {
		return []string{"no documented build tags found in doc; Chapter 7 'Build tags' subsection may have changed"}
	}

	srcTags := map[string]bool{}
	err := filepath.WalkDir("cmd", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "package ") {
				break
			}
			if !strings.HasPrefix(line, "//go:build") && !strings.HasPrefix(line, "// +build") {
				continue
			}
			for _, tag := range extractTags(line) {
				srcTags[tag] = true
			}
		}
		return nil
	})
	if err != nil {
		return []string{fmt.Sprintf("walk cmd/: %v", err)}
	}

	var failures []string
	for _, tag := range sortedKeys(srcTags) {
		if !docTags[tag] {
			failures = append(failures, fmt.Sprintf("build tag %q appears in cmd/ but is not documented in Chapter 7", tag))
		}
	}
	return failures
}

func parseBuildTagsFromDoc(doc string) map[string]bool {
	body := sliceSubsection(doc, "### Build tags")
	if body == "" {
		return nil
	}
	re := regexp.MustCompile("- `([a-z_]+)`")
	out := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(body, -1) {
		out[m[1]] = true
	}
	return out
}

func extractTags(line string) []string {
	line = strings.TrimPrefix(line, "//go:build")
	line = strings.TrimPrefix(line, "// +build")
	for _, op := range []string{"&&", "||", "(", ")", "!"} {
		line = strings.ReplaceAll(line, op, " ")
	}
	var out []string
	for _, f := range strings.Fields(line) {
		for _, p := range strings.Split(f, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

// checkDrivers asserts the persistence backends documented in Chapter 5 match
// the driver registration files in cmd/gonemaster-server/. The "memory"
// backend is in-process and has no registration file.
func checkDrivers(doc string) []string {
	docDrivers := parseDriversFromDoc(doc)
	if len(docDrivers) == 0 {
		return []string{"no documented persistence backends found in doc; Chapter 5 backend table may have changed"}
	}

	infrastructure := map[string]bool{
		"main.go":      true,
		"doc.go":       true,
		"envconfig.go": true,
		"auth_cmd.go":  true,
	}
	srcDrivers := map[string]bool{}
	entries, err := os.ReadDir("cmd/gonemaster-server")
	if err != nil {
		return []string{fmt.Sprintf("read cmd/gonemaster-server/: %v", err)}
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if infrastructure[name] {
			continue
		}
		srcDrivers[strings.TrimSuffix(name, ".go")] = true
	}

	var failures []string
	for _, name := range sortedKeys(docDrivers) {
		if name == "memory" {
			continue
		}
		if !srcDrivers[name] {
			failures = append(failures, fmt.Sprintf("Chapter 5 documents %q backend but cmd/gonemaster-server/%s.go does not exist", name, name))
		}
	}
	for _, name := range sortedKeys(srcDrivers) {
		if !docDrivers[name] {
			failures = append(failures, fmt.Sprintf("cmd/gonemaster-server/%s.go exists but %q is not in Chapter 5's backend table (or it is a non-driver file; if so add it to the architecture-check infrastructure list)", name, name))
		}
	}
	return failures
}

func parseDriversFromDoc(doc string) map[string]bool {
	body := sliceSubsection(doc, "## 5. Persistence backends")
	if body == "" {
		return nil
	}
	re := regexp.MustCompile("(?m)^\\| `([a-z]+)` \\|")
	out := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(body, -1) {
		out[m[1]] = true
	}
	return out
}

// checkLastReviewed asserts the doc has a "Last reviewed: YYYY-MM-DD" line
// no older than maxReviewAgeDays days.
func checkLastReviewed(doc string) []string {
	re := regexp.MustCompile(`Last reviewed:\s*(\d{4}-\d{2}-\d{2})`)
	m := re.FindStringSubmatch(doc)
	if m == nil {
		return []string{"missing `Last reviewed: YYYY-MM-DD` line"}
	}
	t, err := time.Parse("2006-01-02", m[1])
	if err != nil {
		return []string{fmt.Sprintf("Last reviewed value %q is not a valid YYYY-MM-DD date", m[1])}
	}
	ageDays := int(time.Since(t).Hours() / 24)
	if ageDays > maxReviewAgeDays {
		return []string{fmt.Sprintf("Last reviewed: %s is %d days old; budget is %d days. Re-read the document and bump the date.", m[1], ageDays, maxReviewAgeDays)}
	}
	return nil
}

// sliceSubsection returns the text from `heading` up to the next heading of
// the same or higher level, or "" if `heading` is not found.
func sliceSubsection(doc, heading string) string {
	idx := strings.Index(doc, heading)
	if idx == -1 {
		return ""
	}
	level := 0
	for level < len(heading) && heading[level] == '#' {
		level++
	}
	prefix := strings.Repeat("#", level) + " "
	rest := doc[idx+len(heading):]
	for i := 0; i < len(rest); i++ {
		if i == 0 || rest[i-1] == '\n' {
			if strings.HasPrefix(rest[i:], prefix) {
				return doc[idx : idx+len(heading)+i]
			}
			for l := 1; l < level; l++ {
				if strings.HasPrefix(rest[i:], strings.Repeat("#", l)+" ") {
					return doc[idx : idx+len(heading)+i]
				}
			}
		}
	}
	return doc[idx:]
}

func sortedKeys(s map[string]bool) []string {
	out := make([]string, 0, len(s))
	for k := range s {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
