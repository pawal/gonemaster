package engine

import (
	"strings"

	"codeberg.org/pawal/gonemaster/engine/profile"
)

var moduleTestcases = map[string][]string{
	"basic": {
		"basic01",
		"basic02",
		"basic03",
	},
	"syntax": {
		"syntax01",
		"syntax02",
		"syntax03",
		"syntax04",
		"syntax05",
		"syntax06",
		"syntax07",
		"syntax08",
	},
	"address": {
		"address01",
		"address02",
		"address03",
	},
	"connectivity": {
		"connectivity01",
		"connectivity02",
		"connectivity03",
		"connectivity04",
		"connectivity05",
	},
	"consistency": {
		"consistency01",
		"consistency02",
		"consistency03",
		"consistency04",
		"consistency05",
		"consistency06",
	},
	"dnssec": dnssecTestcaseNames(),
	"delegation": {
		"delegation01",
		"delegation02",
		"delegation03",
		"delegation04",
		"delegation05",
		"delegation06",
		"delegation07",
	},
	"nameserver": {
		"nameserver01",
		"nameserver02",
		"nameserver03",
		"nameserver04",
		"nameserver05",
		"nameserver06",
		"nameserver07",
		"nameserver08",
		"nameserver09",
		"nameserver10",
		"nameserver11",
		"nameserver12",
		"nameserver13",
		"nameserver15",
		"nameserver16",
		"nameserver17",
		"nameserver18",
	},
	"zone": {
		"zone01",
		"zone02",
		"zone03",
		"zone04",
		"zone05",
		"zone06",
		"zone07",
		"zone08",
		"zone09",
		"zone10",
		"zone11",
		"zone12",
		"zone13",
		"zone14",
		"zone15",
	},
}

var moduleOrder = []string{
	"basic",
	"address",
	"connectivity",
	"consistency",
	"delegation",
	"dnssec",
	"nameserver",
	"syntax",
	"zone",
}

var moduleDisplayNames = map[string]string{
	"basic":        "Basic",
	"syntax":       "Syntax",
	"address":      "Address",
	"connectivity": "Connectivity",
	"consistency":  "Consistency",
	"dnssec":       "DNSSEC",
	"delegation":   "Delegation",
	"nameserver":   "Nameserver",
	"zone":         "Zone",
}

// AvailableTestcases returns all known test cases in module order.
func AvailableTestcases() []string {
	planned := []string{}
	for _, moduleName := range moduleOrder {
		display := moduleDisplayName(moduleName)
		for _, testcase := range moduleTestcases[moduleName] {
			planned = append(planned, display+":"+testcaseDisplayName(testcase))
		}
	}
	return planned
}

// PlannedTestcases returns the list of test cases expected to run for the request.
func PlannedTestcases(req RunRequest) ([]string, error) {
	module := strings.ToLower(strings.TrimSpace(req.Module))

	if module != "" && moduleTestcases[module] == nil {
		return nil, ErrNotImplemented
	}

	if len(req.Testcases) > 0 {
		selected := map[string]bool{}
		ordered := []string{}
		for _, raw := range req.Testcases {
			name := strings.ToLower(strings.TrimSpace(raw))
			if name == "" {
				continue
			}
			testModule := testcaseModule(name)
			if testModule == "" {
				return nil, ErrNotImplemented
			}
			if module != "" && module != testModule {
				return nil, ErrNotImplemented
			}
			if !selected[name] {
				selected[name] = true
				ordered = append(ordered, name)
			}
		}
		planned := []string{}
		for _, moduleName := range moduleOrder {
			for _, name := range moduleTestcases[moduleName] {
				if selected[name] {
					planned = append(planned, name)
				}
			}
		}
		if len(planned) == 0 {
			return ordered, nil
		}
		return planned, nil
	}

	if module != "" {
		return copyStrings(moduleTestcases[module]), nil
	}

	p, err := profile.Default()
	if err != nil {
		return nil, err
	}
	override, err := req.profileOverride()
	if err != nil {
		return nil, err
	}
	if override != nil {
		if err := p.Merge(override); err != nil {
			return nil, err
		}
	}

	enabled := map[string]bool{}
	for _, item := range p.TestCases {
		name, ok := item.(string)
		if !ok {
			continue
		}
		name = strings.ToLower(strings.TrimSpace(name))
		if name != "" {
			enabled[name] = true
		}
	}

	planned := []string{}
	for _, moduleName := range moduleOrder {
		for _, name := range moduleTestcases[moduleName] {
			if enabled[name] {
				planned = append(planned, name)
			}
		}
	}
	return planned, nil
}

func testcaseModule(testcase string) string {
	if _, ok := basicTests[testcase]; ok {
		return "basic"
	}
	if _, ok := syntaxTests[testcase]; ok {
		return "syntax"
	}
	if _, ok := addressTests[testcase]; ok {
		return "address"
	}
	if _, ok := connectivityTests[testcase]; ok {
		return "connectivity"
	}
	if _, ok := consistencyTests[testcase]; ok {
		return "consistency"
	}
	if _, ok := dnssecTests[testcase]; ok {
		return "dnssec"
	}
	if _, ok := delegationTests[testcase]; ok {
		return "delegation"
	}
	if _, ok := nameserverTests[testcase]; ok {
		return "nameserver"
	}
	if _, ok := zoneTests[testcase]; ok {
		return "zone"
	}
	return ""
}

func copyStrings(values []string) []string {
	out := make([]string, len(values))
	copy(out, values)
	return out
}

func moduleDisplayName(moduleName string) string {
	if display, ok := moduleDisplayNames[moduleName]; ok {
		return display
	}
	return titleCase(moduleName)
}

func titleCase(value string) string {
	if value == "" {
		return ""
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func testcaseDisplayName(testcase string) string {
	if strings.HasPrefix(testcase, "dnssec") {
		return "DNSSEC" + testcase[len("dnssec"):]
	}
	return titleCase(testcase)
}

func toAnySlice(values []string) []any {
	out := make([]any, len(values))
	for i, value := range values {
		out[i] = value
	}
	return out
}
