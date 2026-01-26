package util

import "github.com/pawal/gonemaster/engine/hints"

// ParseHints parses a root hints zone file into a map of names to IP addresses.
func ParseHints(text string) (map[string][]string, error) {
	return hints.ParseHints(text)
}
