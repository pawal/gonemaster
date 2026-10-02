package server

import (
	"maps"
	"regexp"
	"strings"
	"sync"

	"codeberg.org/pawal/gonemaster/engine/i18n"
	"codeberg.org/pawal/gonemaster/engine/logger"
)

var (
	knownLocalesOnce sync.Once
	knownLocales     map[string]struct{}
)

// resolveResultLocale returns requested if it is a known engine locale, else "en".
func resolveResultLocale(requested string) string {
	knownLocalesOnce.Do(func() {
		codes := i18n.AvailableLocales()
		knownLocales = make(map[string]struct{}, len(codes))
		for _, c := range codes {
			knownLocales[strings.ToLower(c)] = struct{}{}
		}
	})
	key := strings.ToLower(strings.TrimSpace(requested))
	if key == "" {
		return "en"
	}
	if _, ok := knownLocales[key]; ok {
		return key
	}
	return "en"
}

// localEndpointInError matches the local endpoint of a net.OpError string,
// "udp 10.0.0.5:5300->" in "read udp 10.0.0.5:5300->192.0.2.1:53: i/o timeout".
var localEndpointInError = regexp.MustCompile(`\b((?:udp|tcp)[46]?) \S+?->`)

// redactLocalEndpoints drops the server's own address and port from the
// exception arg of entries, copying any args it changes.
func redactLocalEndpoints(entries []JobResultEntry) []JobResultEntry {
	out := make([]JobResultEntry, len(entries))
	for i, e := range entries {
		out[i] = e
		exc, ok := e.Args["exception"].(string)
		if !ok || !localEndpointInError.MatchString(exc) {
			continue
		}
		out[i].Args = maps.Clone(e.Args)
		out[i].Args["exception"] = localEndpointInError.ReplaceAllString(exc, "$1 ")
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
