// generate-tag-catalog generates per-module tag catalog markdown files under
// docs/specifications/tags/ from live code metadata and share/profile.json.
//
// Usage:
//
//	go run ./tools/specifications/generate-tag-catalog/
//	go run ./tools/specifications/generate-tag-catalog/ --check
//
// --check diffs generated content against on-disk files and exits non-zero if
// they differ (useful for CI drift detection).
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"codeberg.org/pawal/gonemaster/tools/specifications/internal/specdata"
)

// moduleOrder is the canonical output order for module catalog files.
var moduleOrder = []string{
	"address",
	"basic",
	"connectivity",
	"consistency",
	"delegation",
	"dnssec",
	"nameserver",
	"syntax",
	"zone",
}

// moduleDisplayName maps lowercase module key to the display name used in
// profile.json test_levels keys and i18n comment prefixes.
var moduleDisplayName = map[string]string{
	"address":      "ADDRESS",
	"basic":        "BASIC",
	"connectivity": "CONNECTIVITY",
	"consistency":  "CONSISTENCY",
	"delegation":   "DELEGATION",
	"dnssec":       "DNSSEC",
	"nameserver":   "NAMESERVER",
	"syntax":       "SYNTAX",
	"zone":         "ZONE",
}

// moduleMetadataSource maps lowercase module key to the Go source file that
// contains its Metadata() function.
var moduleMetadataSource = map[string]string{
	"address":      "engine/test/address/address.go",
	"basic":        "engine/test/basic/basic.go",
	"connectivity": "engine/test/connectivity/connectivity.go",
	"consistency":  "engine/test/consistency/consistency.go",
	"delegation":   "engine/test/delegation/delegation.go",
	"dnssec":       "engine/test/dnssec/dnssec.go",
	"nameserver":   "engine/test/nameserver/nameserver.go",
	"syntax":       "engine/test/syntax/syntax.go",
	"zone":         "engine/test/zone/zone.go",
}

type profile struct {
	TestLevels map[string]map[string]string `json:"test_levels"`
}

func main() {
	var (
		outputDir   string
		profilePath string
		i18nDir     string
		checkMode   bool
	)
	flag.StringVar(&outputDir, "output", "docs/specifications/tags", "Output directory for tag catalog files")
	flag.StringVar(&profilePath, "profile", "share/profile.json", "Path to profile.json")
	flag.StringVar(&i18nDir, "i18n", "share/lang", "Directory containing *.po locale files")
	flag.BoolVar(&checkMode, "check", false, "Diff generated content against on-disk files; exit non-zero on drift")
	flag.Parse()

	modules := specdata.KnownTagsByModule()

	prof, err := loadProfile(profilePath)
	if err != nil {
		fatalf("load profile: %v", err)
	}

	i18nCoverage, err := loadI18nCoverage(i18nDir)
	if err != nil {
		fatalf("load i18n coverage: %v", err)
	}

	driftFound := false
	for _, moduleName := range moduleOrder {
		testcaseMap, ok := modules[moduleName]
		if !ok {
			continue
		}

		displayName := moduleDisplayName[moduleName]
		moduleLevels := prof.TestLevels[displayName]

		content := generateCatalog(moduleName, displayName, testcaseMap, moduleLevels, i18nCoverage)
		outPath := filepath.Join(outputDir, moduleName+".md")

		if checkMode {
			existing, readErr := os.ReadFile(outPath)
			if readErr != nil {
				_, _ = fmt.Fprintf(os.Stderr, "drift: cannot read %s: %v\n", outPath, readErr)
				driftFound = true
				continue
			}
			if !bytes.Equal(existing, []byte(content)) {
				_, _ = fmt.Fprintf(os.Stderr, "drift: %s differs from generated output\n", outPath)
				driftFound = true
			}
			continue
		}

		if err := os.WriteFile(outPath, []byte(content), 0o644); err != nil {
			fatalf("write %s: %v", outPath, err)
		}
		fmt.Printf("wrote %s\n", outPath)
	}

	if checkMode && driftFound {
		_, _ = fmt.Fprintln(os.Stderr, "tag catalog drift detected; run: go run ./tools/specifications/generate-tag-catalog/")
		os.Exit(1)
	}
	if checkMode {
		fmt.Println("tag catalogs are up to date")
	}
}

// generateCatalog renders the markdown content for one module's tag catalog.
func generateCatalog(
	moduleName string,
	displayName string,
	testcaseMap map[string][]string,
	moduleLevels map[string]string,
	i18nCoverage map[string]bool,
) string {
	// Build inverted index: tag -> sorted list of testcases that emit it.
	tagToTestcases := map[string][]string{}
	for tc, tags := range testcaseMap {
		for _, tag := range tags {
			tagToTestcases[tag] = append(tagToTestcases[tag], tc)
		}
	}
	allTags := sortedStringKeys(tagToTestcases)
	for tag := range tagToTestcases {
		sort.Strings(tagToTestcases[tag])
	}

	// Compute i18n coverage for this module.
	var missingI18n, staleI18n []string
	tagSet := map[string]bool{}
	for _, tag := range allTags {
		tagSet[tag] = true
		key := displayName + ":" + tag
		if !i18nCoverage[key] {
			missingI18n = append(missingI18n, tag)
		}
	}
	// Find stale i18n entries: covered by PO but absent from current metadata.
	prefix := displayName + ":"
	for key := range i18nCoverage {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		tag := strings.TrimPrefix(key, prefix)
		if !tagSet[tag] {
			staleI18n = append(staleI18n, tag)
		}
	}
	sort.Strings(missingI18n)
	sort.Strings(staleI18n)

	var b strings.Builder

	title := strings.ToUpper(moduleName[:1]) + strings.ToLower(moduleName[1:])
	fmt.Fprintf(&b, "# Tag Catalog: %s\n\n", title)
	fmt.Fprintf(&b, "Module: `%s`  \n", displayName)
	fmt.Fprintf(&b, "Metadata source: `%s`  \n", moduleMetadataSource[moduleName])
	fmt.Fprintf(&b, "Level config: `share/profile.json` (`test_levels.%s`)  \n", displayName)
	fmt.Fprintf(&b, "Generated by: `go run ./tools/specifications/generate-tag-catalog/`\n\n")
	fmt.Fprintf(&b, "_Do not edit by hand - regenerate with the command above._\n\n")

	fmt.Fprintf(&b, "## Tags\n\n")
	fmt.Fprintf(&b, "| Tag | Level | Testcase(s) | i18n |\n")
	fmt.Fprintf(&b, "| --- | --- | --- | --- |\n")

	for _, tag := range allTags {
		level := moduleLevels[tag]
		if level == "" {
			level = "-"
		}
		tcs := testcaseLinksInline(moduleName, tagToTestcases[tag])
		covered := i18nCoverage[displayName+":"+tag]
		i18nStr := "yes"
		if !covered {
			i18nStr = "**no**"
		}
		fmt.Fprintf(&b, "| `%s` | `%s` | %s | %s |\n", tag, level, tcs, i18nStr)
	}

	fmt.Fprintf(&b, "\n")

	// i18n notes section.
	fmt.Fprintf(&b, "## i18n Notes\n\n")

	if len(missingI18n) == 0 && len(staleI18n) == 0 {
		fmt.Fprintf(&b, "All tags have i18n coverage and no stale entries found.\n")
	} else {
		if len(missingI18n) > 0 {
			fmt.Fprintf(&b, "### Tags Missing From Locale Files\n\n")
			fmt.Fprintf(&b, "These tags have no `%s:<TAG>` `msgctxt` entry in any `.po` file.\n", displayName)
			fmt.Fprintf(&b, "They will render as the raw tag name in translated output.\n\n")
			for _, tag := range missingI18n {
				fmt.Fprintf(&b, "- `%s`\n", tag)
			}
			fmt.Fprintf(&b, "\n")
		}
		if len(staleI18n) > 0 {
			fmt.Fprintf(&b, "### Stale Entries In Locale Files\n\n")
			fmt.Fprintf(&b, "These tags appear as `%s:<TAG>` in `.po` files but are absent from\n", displayName)
			fmt.Fprintf(&b, "current code metadata.  The messages may be dead translations.\n\n")
			for _, tag := range staleI18n {
				fmt.Fprintf(&b, "- `%s`\n", tag)
			}
			fmt.Fprintf(&b, "\n")
		}
	}

	return b.String()
}

// testcaseLinksInline renders a comma-separated inline list of testcase links.
func testcaseLinksInline(moduleName string, testcases []string) string {
	parts := make([]string, 0, len(testcases))
	for _, tc := range testcases {
		link := fmt.Sprintf("[%s](../tests/%s/%s.md)", tc, moduleName, tc)
		parts = append(parts, link)
	}
	return strings.Join(parts, ", ")
}

// loadProfile reads and unmarshals share/profile.json.
func loadProfile(path string) (*profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p profile
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// loadI18nCoverage scans all *.po files in dir and returns a set of
// "MODULE:TAG" strings that appear in msgctxt entries. It also accepts legacy
// "#. MODULE:TAG" comment lines for backward compatibility.
func loadI18nCoverage(dir string) (map[string]bool, error) {
	coverage := map[string]bool{}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".po") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if err := scanPOFile(path, coverage); err != nil {
			return nil, fmt.Errorf("scan %s: %w", path, err)
		}
	}

	return coverage, nil
}

// scanPOFile extracts coverage entries from a PO file.
// Preferred form: msgctxt "MODULE:TAG"
// Legacy form:    #. MODULE:TAG
func scanPOFile(path string, out map[string]bool) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Preferred modern source of truth.
		if strings.HasPrefix(line, "msgctxt ") {
			if token, ok := parsePOQuotedValue(line, "msgctxt "); ok {
				addCoverageToken(token, out)
			}
			continue
		}
		// Legacy fallback.
		if !strings.HasPrefix(line, "#. ") {
			continue
		}
		token := strings.TrimPrefix(line, "#. ")
		addCoverageToken(token, out)
	}
	return scanner.Err()
}

func parsePOQuotedValue(line string, prefix string) (string, bool) {
	payload := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	if payload == "" {
		return "", false
	}
	unquoted, err := strconv.Unquote(payload)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(unquoted), true
}

func addCoverageToken(token string, out map[string]bool) {
	token = strings.TrimSpace(token)
	parts := strings.SplitN(token, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return
	}
	out[token] = true
}

func sortedStringKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func fatalf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "generate-tag-catalog: "+format+"\n", args...)
	os.Exit(1)
}
