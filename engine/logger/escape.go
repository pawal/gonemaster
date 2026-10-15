package logger

import (
	"maps"
	"slices"
	"strings"
	"unicode/utf8"
)

// escapeArgs applies escapeText to every string in args, copying only what changes.
func escapeArgs(args map[string]any) map[string]any {
	out, _ := escapeMap(args)
	return out
}

func escapeMap(m map[string]any) (map[string]any, bool) {
	var out map[string]any
	for k, v := range m {
		if ev, changed := escapeValue(v); changed {
			if out == nil {
				out = maps.Clone(m)
			}
			out[k] = ev
		}
	}
	if out == nil {
		return m, false
	}
	return out, true
}

func escapeValue(v any) (any, bool) {
	switch t := v.(type) {
	case string:
		return escapeText(t)
	case []string:
		return escapeSlice(t, escapeText)
	case []any:
		return escapeSlice(t, escapeValue)
	case map[string]any:
		return escapeMap(t)
	case []map[string]any:
		return escapeSlice(t, escapeMap)
	}
	return v, false
}

func escapeSlice[T any](s []T, esc func(T) (T, bool)) ([]T, bool) {
	var out []T
	for i, v := range s {
		if ev, changed := esc(v); changed {
			if out == nil {
				out = slices.Clone(s)
			}
			out[i] = ev
		}
	}
	if out == nil {
		return s, false
	}
	return out, true
}

// escapeText writes C0 controls, DEL, C1 controls and invalid UTF-8 as \DDD per octet.
func escapeText(s string) (string, bool) {
	i := 0
	for i < len(s) {
		r, size := utf8.DecodeRuneInString(s[i:])
		if unsafeRune(r, size) {
			break
		}
		i += size
	}
	if i == len(s) {
		return s, false
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	b.WriteString(s[:i])
	for i < len(s) {
		r, size := utf8.DecodeRuneInString(s[i:])
		if unsafeRune(r, size) {
			for _, c := range []byte(s[i : i+size]) {
				b.Write([]byte{'\\', '0' + c/100, '0' + c/10%10, '0' + c%10})
			}
		} else {
			b.WriteString(s[i : i+size])
		}
		i += size
	}
	return b.String(), true
}

func unsafeRune(r rune, size int) bool {
	return (r == utf8.RuneError && size == 1) || r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f)
}
