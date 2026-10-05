package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"codeberg.org/pawal/gonemaster/tools/specifications/internal/specdata"
)

var (
	tagPattern  = regexp.MustCompile(`^[A-Z0-9_]+$`)
	callPattern = regexp.MustCompile(`append[A-Za-z]*Log\([^\"]*\"([A-Z0-9_]+)\"\s*,`)
)

type validationState struct {
	errors   []string
	warnings []string
}

func (v *validationState) addError(format string, args ...any) {
	v.errors = append(v.errors, fmt.Sprintf(format, args...))
}

func (v *validationState) addWarning(format string, args ...any) {
	v.warnings = append(v.warnings, fmt.Sprintf(format, args...))
}

func main() {
	var (
		specsRoot      string
		profilePath    string
		scanAppendLogs bool
	)

	flag.StringVar(&specsRoot, "specs-root", "docs/specifications/tests", "Path to canonical testcase specifications")
	flag.StringVar(&profilePath, "profile", "share/profile.json", "Path to profile.json")
	flag.BoolVar(&scanAppendLogs, "scan-append-log", false, "Scan append*Log tag literals and report tags missing from metadata")
	flag.Parse()

	implementedByModule, moduleOrder, err := specdata.ImplementedByModule()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "load implemented testcase inventory: %v\n", err)
		os.Exit(1)
	}
	knownTagsByModule := specdata.KnownTagsByModule()
	testLevels, err := specdata.TestLevels(profilePath)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "load profile levels: %v\n", err)
		os.Exit(1)
	}

	state := &validationState{}

	implementedKeys := map[string]string{}
	implementedTotal := 0
	for _, module := range moduleOrder {
		for _, testcase := range implementedByModule[module] {
			key := testcaseKey(module, testcase)
			expectedPath := filepath.Join(specsRoot, module, testcase+".md")
			implementedKeys[key] = expectedPath
			implementedTotal++
			if _, err := os.Stat(expectedPath); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					state.addError("missing canonical testcase spec: %s", expectedPath)
				} else {
					state.addError("stat canonical testcase spec %s: %v", expectedPath, err)
				}
			}
		}
	}

	specFiles, err := collectSpecFiles(specsRoot)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "collect spec files: %v\n", err)
		os.Exit(1)
	}

	knownGlobalTags := map[string]bool{}
	for _, module := range sortedModuleKeys(knownTagsByModule) {
		for _, testcase := range sortedTestcaseKeys(knownTagsByModule[module]) {
			for _, tag := range knownTagsByModule[module][testcase] {
				knownGlobalTags[tag] = true
			}
		}
	}

	for _, filePath := range specFiles {
		module, testcase, err := pathToModuleTestcase(specsRoot, filePath)
		if err != nil {
			state.addError("invalid testcase spec path %s: %v", filePath, err)
			continue
		}

		key := testcaseKey(module, testcase)
		if _, ok := implementedKeys[key]; !ok {
			state.addError("canonical spec has no matching implemented testcase: %s", filePath)
			continue
		}

		emittedTags, err := parseEmittedTags(filePath)
		if err != nil {
			state.addError("parse emitted tags in %s: %v", filePath, err)
			continue
		}
		if len(emittedTags) == 0 {
			state.addError("canonical spec has no emitted tags listed: %s", filePath)
			continue
		}

		knownTags := knownTagsByModule[module][testcase]
		if len(knownTags) == 0 {
			state.addError("no metadata tags found for %s/%s", module, testcase)
			continue
		}

		knownSet := map[string]bool{}
		for _, tag := range knownTags {
			knownSet[tag] = true
		}

		for tag := range emittedTags {
			if !knownSet[tag] {
				state.addError("unknown tag in canonical spec %s: %s", filePath, tag)
			}
		}
		for _, tag := range knownTags {
			if !emittedTags[tag] {
				state.addError("metadata tag missing from canonical spec %s: %s", filePath, tag)
			}
		}

		specLevels, err := parseSeverityLevels(filePath)
		if err != nil {
			state.addError("parse severity levels in %s: %v", filePath, err)
			continue
		}
		for _, line := range severityMismatches(filePath, module, specLevels, testLevels) {
			state.addError("%s", line)
		}
	}

	if scanAppendLogs {
		scannedTags, scanErr := scanAppendLogTags("engine/test")
		if scanErr != nil {
			state.addError("scan append*Log tags: %v", scanErr)
		} else {
			for tag, locations := range scannedTags {
				if knownGlobalTags[tag] {
					continue
				}
				sort.Strings(locations)
				state.addWarning("append*Log tag not found in metadata: %s (locations: %s)", tag, strings.Join(locations, ", "))
			}
		}
	}

	if len(state.warnings) > 0 {
		for _, line := range state.warnings {
			_, _ = fmt.Fprintf(os.Stderr, "warning: %s\n", line)
		}
	}

	if len(state.errors) > 0 {
		for _, line := range state.errors {
			_, _ = fmt.Fprintf(os.Stderr, "error: %s\n", line)
		}
		_, _ = fmt.Fprintf(os.Stderr, "validation failed: %d error(s), %d warning(s)\n", len(state.errors), len(state.warnings))
		os.Exit(1)
	}

	_, _ = fmt.Fprintf(os.Stdout, "validation passed: implemented=%d specs=%d warnings=%d\n", implementedTotal, len(specFiles), len(state.warnings))
}

func collectSpecFiles(root string) ([]string, error) {
	files := []string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}
		if strings.EqualFold(filepath.Base(path), "README.md") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func pathToModuleTestcase(root string, path string) (string, string, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", "", err
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) != 2 {
		return "", "", fmt.Errorf("expected path format <module>/<testcase>.md, got %q", rel)
	}
	module := strings.ToLower(strings.TrimSpace(parts[0]))
	testcase := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(parts[1])), ".md")
	if module == "" || testcase == "" {
		return "", "", fmt.Errorf("empty module/testcase in %q", rel)
	}
	return module, testcase, nil
}

func testcaseKey(module string, testcase string) string {
	return module + "/" + testcase
}

func parseEmittedTags(path string) (map[string]bool, error) {
	rows, err := tableRows(path, "## emitted tags")
	if err != nil {
		return nil, err
	}
	tags := map[string]bool{}
	for _, cells := range rows {
		if tag := normalizeTagCell(cells[0]); tag != "" {
			tags[tag] = true
		}
	}
	return tags, nil
}

// parseSeverityLevels returns the level per tag from the severity table.
func parseSeverityLevels(path string) (map[string]string, error) {
	rows, err := tableRows(path, "## severity levels per tag")
	if err != nil {
		return nil, err
	}
	levels := map[string]string{}
	for _, cells := range rows {
		tag := normalizeTagCell(cells[0])
		if tag == "" || len(cells) < 2 {
			continue
		}
		levels[tag] = strings.ToUpper(strings.Trim(cells[1], "` "))
	}
	return levels, nil
}

// severityMismatches lists spec levels absent from or unequal to the profile.
func severityMismatches(path string, module string, specLevels map[string]string, testLevels map[string]map[string]string) []string {
	section := strings.ToUpper(module)
	var out []string
	for _, tag := range slices.Sorted(maps.Keys(specLevels)) {
		want, ok := testLevels[section][tag]
		switch {
		case !ok:
			out = append(out, fmt.Sprintf("severity level missing from share/profile.json test_levels.%s: %s (%s)", section, tag, path))
		case !strings.EqualFold(want, specLevels[tag]):
			out = append(out, fmt.Sprintf("severity level mismatch in %s: %s is %s, share/profile.json has %s", path, tag, specLevels[tag], strings.ToUpper(want)))
		}
	}
	return out
}

// tableRows returns the trimmed cells of each table row under the heading.
func tableRows(path string, heading string) ([][]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var rows [][]string
	inSection := false

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "## ") {
			if strings.HasPrefix(strings.ToLower(line), heading) {
				inSection = true
				continue
			}
			if inSection {
				break
			}
			continue
		}

		if !inSection || !strings.HasPrefix(line, "|") {
			continue
		}

		parts := strings.Split(strings.TrimSuffix(line, "|"), "|")
		if len(parts) < 2 {
			continue
		}
		cells := make([]string, 0, len(parts)-1)
		for _, part := range parts[1:] {
			cells = append(cells, strings.TrimSpace(part))
		}
		if cells[0] == "" || strings.HasPrefix(cells[0], "---") {
			continue
		}
		rows = append(rows, cells)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return rows, nil
}

func normalizeTagCell(cell string) string {
	cell = strings.TrimSpace(cell)
	if strings.HasPrefix(cell, "[") {
		if idx := strings.Index(cell, "]"); idx > 1 {
			cell = cell[1:idx]
		}
	}
	cell = strings.Trim(cell, "`")
	cell = strings.TrimSpace(cell)
	if strings.HasPrefix(cell, "<") && strings.HasSuffix(cell, ">") {
		return ""
	}
	if !tagPattern.MatchString(cell) {
		return ""
	}
	return cell
}

func scanAppendLogTags(root string) (map[string][]string, error) {
	out := map[string][]string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := scanner.Text()
			matches := callPattern.FindAllStringSubmatch(line, -1)
			for _, match := range matches {
				if len(match) < 2 {
					continue
				}
				tag := match[1]
				out[tag] = append(out[tag], fmt.Sprintf("%s:%d", path, lineNo))
			}
		}
		if err := scanner.Err(); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func sortedModuleKeys(modules map[string]map[string][]string) []string {
	keys := slices.Sorted(maps.Keys(modules))
	return keys
}

func sortedTestcaseKeys(testcases map[string][]string) []string {
	keys := slices.Sorted(maps.Keys(testcases))
	return keys
}
