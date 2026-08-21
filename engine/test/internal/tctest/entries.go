package tctest

import (
	"strings"

	"codeberg.org/pawal/gonemaster/engine/logger"
)

// TB is the subset of testing.TB the assertion helpers use.
type TB interface {
	Helper()
	Fatalf(format string, args ...any)
}

// Has reports whether any entry carries the tag.
func Has(entries []*logger.Entry, tag string) bool {
	return First(entries, tag) != nil
}

// Count returns the number of entries carrying the tag.
func Count(entries []*logger.Entry, tag string) int {
	n := 0
	for _, entry := range entries {
		if entry != nil && entry.Tag == tag {
			n++
		}
	}
	return n
}

// First returns the first entry carrying the tag, or nil.
func First(entries []*logger.Entry, tag string) *logger.Entry {
	for _, entry := range entries {
		if entry != nil && entry.Tag == tag {
			return entry
		}
	}
	return nil
}

// All returns every entry carrying the tag, in emission order.
func All(entries []*logger.Entry, tag string) []*logger.Entry {
	var out []*logger.Entry
	for _, entry := range entries {
		if entry != nil && entry.Tag == tag {
			out = append(out, entry)
		}
	}
	return out
}

// Tags returns the tags of all entries, in emission order.
func Tags(entries []*logger.Entry) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry != nil {
			out = append(out, entry.Tag)
		}
	}
	return out
}

// TagsWithPrefix returns the tags starting with prefix, in emission order.
func TagsWithPrefix(entries []*logger.Entry, prefix string) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry != nil && strings.HasPrefix(entry.Tag, prefix) {
			out = append(out, entry.Tag)
		}
	}
	return out
}

// RequireTag fails the test unless the tag was emitted, and returns its first entry.
func RequireTag(t TB, entries []*logger.Entry, tag string) *logger.Entry {
	t.Helper()
	entry := First(entries, tag)
	if entry == nil {
		t.Fatalf("expected %s, got %v", tag, Tags(entries))
	}
	return entry
}

// RequireTags fails the test unless every tag was emitted.
func RequireTags(t TB, entries []*logger.Entry, tags ...string) {
	t.Helper()
	for _, tag := range tags {
		if !Has(entries, tag) {
			t.Fatalf("expected %s, got %v", tag, Tags(entries))
		}
	}
}

// RequireNoTag fails the test if any of the tags was emitted.
func RequireNoTag(t TB, entries []*logger.Entry, tags ...string) {
	t.Helper()
	for _, tag := range tags {
		if Has(entries, tag) {
			t.Fatalf("did not expect %s, got %v", tag, Tags(entries))
		}
	}
}

// RequireCount fails the test unless the tag was emitted exactly want times.
func RequireCount(t TB, entries []*logger.Entry, tag string, want int) {
	t.Helper()
	if got := Count(entries, tag); got != want {
		t.Fatalf("expected %d %s entries, got %d (%v)", want, tag, got, Tags(entries))
	}
}

// Normalize renders entries as "module:testcase:tag args" strings for comparison.
func Normalize(entries []*logger.Entry) []string {
	normalized := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		item := entry.Module + ":" + entry.Testcase + ":" + entry.Tag
		if args := entry.ArgString(); args != "" {
			item += " " + args
		}
		normalized = append(normalized, item)
	}
	return normalized
}

// NormalizeStable is Normalize without System/Unspecified DEBUG entries, whose
// emission order varies under parallel execution.
func NormalizeStable(entries []*logger.Entry) []string {
	kept := make([]*logger.Entry, 0, len(entries))
	for _, entry := range entries {
		if entry == nil || isSystemDebug(entry) {
			continue
		}
		kept = append(kept, entry)
	}
	return Normalize(kept)
}

func isSystemDebug(entry *logger.Entry) bool {
	return strings.EqualFold(entry.Module, "System") &&
		strings.EqualFold(entry.Testcase, "Unspecified") &&
		strings.HasPrefix(strings.ToUpper(entry.Level()), "DEBUG")
}
