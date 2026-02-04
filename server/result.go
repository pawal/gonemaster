package server

import (
	"strings"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/i18n"
	"codeberg.org/pawal/gonemaster/engine/logger"
)

func buildResultEntries(entries []engine.LogEntry) []JobResultEntry {
	if len(entries) == 0 {
		return nil
	}
	out := make([]JobResultEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, JobResultEntry{
			Timestamp: entry.Timestamp,
			Module:    entry.Module,
			Testcase:  entry.Testcase,
			Tag:       entry.Tag,
			Level:     entry.Level,
			Args:      entry.Args,
		})
	}
	return out
}

func localizeResultEntries(entries []JobResultEntry, locale string) []JobResultEntry {
	if len(entries) == 0 {
		return nil
	}
	locale = strings.TrimSpace(locale)
	if locale == "" {
		locale = "en"
	}

	out := make([]JobResultEntry, len(entries))
	for i, entry := range entries {
		out[i] = entry
		raw := rawEntryString(entry)
		message, found := i18n.TranslateWithStatus(locale, entry.Module, entry.Tag, entry.Args)
		if !found {
			message = raw
		}
		out[i].Message = message
		out[i].Raw = raw
	}
	return out
}

func rawEntryString(entry JobResultEntry) string {
	tmp, err := logger.NewEntry(entry.Tag, entry.Args, entry.Testcase, entry.Module)
	if err == nil {
		return tmp.String()
	}
	raw := entry.Module
	if entry.Testcase != "" {
		if raw == "" {
			raw = entry.Testcase
		} else {
			raw += ":" + entry.Testcase
		}
	}
	if entry.Tag != "" {
		if raw == "" {
			raw = entry.Tag
		} else {
			raw += ":" + entry.Tag
		}
	}
	return raw
}
