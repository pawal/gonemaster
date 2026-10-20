package specdata

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codeberg.org/pawal/gonemaster/engine"
)

// ImplementedByModule returns implemented testcases grouped by module,
// preserving module and testcase order as provided by engine.AvailableTestcases.
func ImplementedByModule() (map[string][]string, []string, error) {
	items := engine.AvailableTestcases()
	modules := map[string][]string{}
	order := []string{}
	seenModule := map[string]bool{}

	for _, item := range items {
		parts := strings.SplitN(item, ":", 2)
		if len(parts) != 2 {
			return nil, nil, fmt.Errorf("unexpected testcase item format: %q", item)
		}

		module := strings.ToLower(strings.TrimSpace(parts[0]))
		testcase := strings.ToLower(strings.TrimSpace(parts[1]))
		if module == "" || testcase == "" {
			return nil, nil, fmt.Errorf("empty module/testcase from item: %q", item)
		}

		if !seenModule[module] {
			seenModule[module] = true
			order = append(order, module)
		}
		modules[module] = append(modules[module], testcase)
	}

	return modules, order, nil
}

// KnownTagsByModule returns the tags each testcase can emit, grouped by module.
func KnownTagsByModule() map[string]map[string][]string {
	out := map[string]map[string][]string{}
	for _, module := range engine.Modules() {
		tags := make(map[string][]string, len(module.Testcases))
		for _, tc := range module.Testcases {
			tags[tc.ID] = tc.Tags
		}
		out[module.Name] = tags
	}
	return out
}

// TestcaseTitles reads the first Purpose bullet from each spec file.
// specsRoot is the directory containing per-module subdirectories (e.g., "docs/specifications/tests").
// Missing spec files or missing Purpose sections produce an empty string for that testcase.
func TestcaseTitles(specsRoot string, modules map[string][]string) map[string]string {
	titles := make(map[string]string)
	for module, testcases := range modules {
		for _, tc := range testcases {
			path := filepath.Join(specsRoot, module, tc+".md")
			if title, err := readSpecPurpose(path); err == nil {
				titles[tc] = title
			}
		}
	}
	return titles
}

// readSpecPurpose returns the first bullet point from the "## Purpose" section.
func readSpecPurpose(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	inPurpose := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "## Purpose" {
			inPurpose = true
			continue
		}
		if inPurpose {
			if after, ok := strings.CutPrefix(line, "- "); ok {
				return strings.TrimSuffix(after, ":"), nil
			}
			// Stop at the next section heading.
			if strings.HasPrefix(line, "## ") {
				break
			}
		}
	}
	return "", fmt.Errorf("no Purpose bullet in %s", path)
}

// TestLevels returns test_levels from a profile file, keyed by module and tag.
func TestLevels(path string) (map[string]map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p struct {
		TestLevels map[string]map[string]string `json:"test_levels"`
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	return p.TestLevels, nil
}
