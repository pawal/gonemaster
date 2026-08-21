package dnstest

import (
	"testing"

	"codeberg.org/pawal/gonemaster/engine/logger"
)

// HasTag reports whether any entry carries the tag.
func HasTag(entries []*logger.Entry, tag string) bool {
	return EntryByTag(entries, tag) != nil
}

// EntryByTag returns the first entry carrying the tag, or nil.
func EntryByTag(entries []*logger.Entry, tag string) *logger.Entry {
	for _, entry := range entries {
		if entry != nil && entry.Tag == tag {
			return entry
		}
	}
	return nil
}

// Tags returns the tags of the entries, in order.
func Tags(entries []*logger.Entry) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry != nil {
			out = append(out, entry.Tag)
		}
	}
	return out
}

// Signature renders each entry as module/testcase/tag/level, for comparing two
// runs entry by entry.
func Signature(entries []*logger.Entry) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry != nil {
			out = append(out, entry.Module+"/"+entry.Testcase+"/"+entry.Tag+"/"+entry.Level())
		}
	}
	return out
}

// RequireEntryByTag returns the first entry carrying the tag or fails.
func RequireEntryByTag(t testing.TB, entries []*logger.Entry, tag string) *logger.Entry {
	t.Helper()
	entry := EntryByTag(entries, tag)
	if entry == nil {
		t.Fatalf("expected a %s entry, got tags %v", tag, Tags(entries))
	}
	return entry
}

// RequireStringArg returns the entry's arg as a string or fails.
func RequireStringArg(t testing.TB, entry *logger.Entry, key string) string {
	t.Helper()
	value, ok := entry.Args[key].(string)
	if !ok {
		t.Fatalf("expected string arg %s on %s, got %#v", key, entry.Tag, entry.Args[key])
	}
	return value
}

// AssertOnlyTag fails unless want fired and none of mustNot did.
func AssertOnlyTag(t testing.TB, entries []*logger.Entry, want string, mustNot []string) {
	t.Helper()
	if !HasTag(entries, want) {
		t.Fatalf("expected %s to fire, got tags %v", want, Tags(entries))
	}
	for _, tag := range mustNot {
		if HasTag(entries, tag) {
			t.Fatalf("expected %s not to fire alongside %s, got tags %v", tag, want, Tags(entries))
		}
	}
}
