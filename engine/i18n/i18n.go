package i18n

import (
	"bufio"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"

	"codeberg.org/pawal/gonemaster/share"
)

var (
	loadOnce sync.Once

	// mu guards the catalogs against readers running concurrently with
	// RegisterCatalogs.
	mu        sync.RWMutex
	catalogs  map[string]map[string]string
	english   map[string]string
	localeIDs []string
)

// RegisterCatalogs merges the .po files matching pattern in fsys into the
// message catalogs. Locale names are taken from the file base names, and the
// entry keys follow the embedded convention: msgctxt, else a "MODULE:TAG"
// dot-comment. Registered entries take precedence over the embedded ones and
// over entries registered earlier.
//
// The files are parsed before anything is merged, so a read error leaves the
// catalogs unchanged. A pattern matching no file is not an error.
func RegisterCatalogs(fsys fs.FS, pattern string) error {
	if fsys == nil {
		return fmt.Errorf("i18n: nil filesystem")
	}
	loadCatalogs()

	paths, err := fs.Glob(fsys, pattern)
	if err != nil {
		return fmt.Errorf("i18n: glob %q: %w", pattern, err)
	}

	type parsed struct {
		locale string
		msgs   map[string]string
		ids    map[string]string
	}
	pending := make([]parsed, 0, len(paths))
	for _, poPath := range paths {
		data, err := fs.ReadFile(fsys, poPath)
		if err != nil {
			return fmt.Errorf("i18n: read %q: %w", poPath, err)
		}
		locale := strings.ToLower(strings.TrimSuffix(path.Base(poPath), ".po"))
		if locale == "" {
			continue
		}
		msgs, ids := parsePO(string(data))
		pending = append(pending, parsed{locale: locale, msgs: msgs, ids: ids})
	}

	mu.Lock()
	defer mu.Unlock()
	for _, item := range pending {
		if len(item.msgs) > 0 {
			if catalogs[item.locale] == nil {
				catalogs[item.locale] = map[string]string{}
				localeIDs = append(localeIDs, item.locale)
			}
			maps.Copy(catalogs[item.locale], item.msgs)
		}
		maps.Copy(english, item.ids)
	}
	sort.Strings(localeIDs)
	return nil
}

// AvailableLocales returns the list of embedded and registered locales, plus "en".
func AvailableLocales() []string {
	loadCatalogs()
	mu.RLock()
	locales := append([]string{}, localeIDs...)
	mu.RUnlock()
	locales = append(locales, "en")
	sort.Strings(locales)
	return locales
}

// DefaultLocale returns the best-effort locale name from the environment.
func DefaultLocale() string {
	if value := os.Getenv("LANGUAGE"); value != "" {
		if parts := strings.Split(value, ":"); len(parts) > 0 && parts[0] != "" {
			return parts[0]
		}
	}
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return "en"
}

// Translate returns a translated message for the module/tag/args tuple.
func Translate(locale string, module string, tag string, args map[string]any) string {
	msg, _ := TranslateWithStatus(locale, module, tag, args)
	return msg
}

// TranslateWithStatus returns a translated message and whether a translation was found.
func TranslateWithStatus(locale string, module string, tag string, args map[string]any) (string, bool) {
	loadCatalogs()

	key := strings.ToUpper(strings.TrimSpace(module)) + ":" + strings.ToUpper(strings.TrimSpace(tag))
	if key == ":" {
		return "", false
	}

	if locale == "" {
		locale = DefaultLocale()
	}

	mu.RLock()
	msg := ""
	for _, candidate := range localeCandidates(locale) {
		if found := lookupCatalog(candidate, key); found != "" {
			msg = found
			break
		}
	}
	if msg == "" {
		msg = english[key]
	}
	mu.RUnlock()

	if msg != "" {
		return interpolate(msg, args), true
	}

	return interpolate(key, args), false
}

// lookupCatalog reads the catalogs; callers must hold mu.
func lookupCatalog(locale string, key string) string {
	if catalogs == nil {
		return ""
	}
	msgs := catalogs[locale]
	if msgs == nil {
		return ""
	}
	return msgs[key]
}

func localeCandidates(locale string) []string {
	locale = normalizeLocale(locale)
	if locale == "" || locale == "c" {
		return nil
	}
	parts := strings.Split(locale, "_")
	if len(parts) > 1 {
		return []string{locale, parts[0]}
	}
	return []string{locale}
}

func normalizeLocale(locale string) string {
	locale = strings.TrimSpace(locale)
	if locale == "" {
		return ""
	}
	locale = strings.ReplaceAll(locale, "-", "_")
	locale = strings.ToLower(locale)
	if idx := strings.IndexAny(locale, ".@"); idx >= 0 {
		locale = locale[:idx]
	}
	return locale
}

func loadCatalogs() {
	loadOnce.Do(func() {
		mu.Lock()
		defer mu.Unlock()

		catalogs = map[string]map[string]string{}
		english = map[string]string{}
		localeIDs = []string{}

		paths, err := fs.Glob(share.POFiles, "lang/*.po")
		if err != nil {
			return
		}

		for _, poPath := range paths {
			data, err := share.POFiles.ReadFile(poPath)
			if err != nil {
				continue
			}
			locale := strings.ToLower(strings.TrimSuffix(path.Base(poPath), ".po"))
			if locale == "" {
				continue
			}
			msgs, ids := parsePO(string(data))
			if len(msgs) > 0 {
				catalogs[locale] = msgs
				localeIDs = append(localeIDs, locale)
			}
			for key, msgid := range ids {
				if _, ok := english[key]; !ok {
					english[key] = msgid
				}
			}
		}

		sort.Strings(localeIDs)
	})
}

func parsePO(data string) (map[string]string, map[string]string) {
	translations := map[string]string{}
	msgids := map[string]string{}

	var pendingKeys []string
	var msgid strings.Builder
	var msgstr strings.Builder
	inMsgid := false
	inMsgstr := false

	flush := func() {
		if len(pendingKeys) > 0 && msgid.Len() > 0 {
			idText := msgid.String()
			if msgstr.Len() > 0 {
				for _, key := range pendingKeys {
					msgids[key] = idText
					translations[key] = msgstr.String()
				}
			} else {
				for _, key := range pendingKeys {
					msgids[key] = idText
				}
			}
		}
		pendingKeys = nil
		msgid.Reset()
		msgstr.Reset()
		inMsgid = false
		inMsgstr = false
	}

	scanner := bufio.NewScanner(strings.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			if msgid.Len() > 0 || msgstr.Len() > 0 {
				flush()
			} else {
				pendingKeys = nil
			}
			continue
		}
		if after, ok := strings.CutPrefix(line, "#."); ok {
			tag := strings.TrimSpace(after)
			if tag != "" {
				if idx := strings.IndexAny(tag, " \t"); idx >= 0 {
					tag = tag[:idx]
				}
				if parts := strings.SplitN(tag, ":", 2); len(parts) == 2 {
					module := strings.ToUpper(strings.TrimSpace(parts[0]))
					msgTag := strings.ToUpper(strings.TrimSpace(parts[1]))
					if module != "" && msgTag != "" {
						pendingKeys = append(pendingKeys, module+":"+msgTag)
					}
				}
			}
			continue
		}
		if strings.HasPrefix(line, "msgctxt ") {
			// msgctxt is the canonical key for this entry; it takes priority
			// over any #. comment keys accumulated above.
			if value, ok := parseQuoted(line); ok {
				if parts := strings.SplitN(value, ":", 2); len(parts) == 2 {
					module := strings.ToUpper(strings.TrimSpace(parts[0]))
					msgTag := strings.ToUpper(strings.TrimSpace(parts[1]))
					if module != "" && msgTag != "" {
						pendingKeys = []string{module + ":" + msgTag}
					}
				}
			}
			continue
		}
		if strings.HasPrefix(line, "msgid ") {
			if msgid.Len() > 0 || msgstr.Len() > 0 {
				flush()
			}
			inMsgid = true
			inMsgstr = false
			if value, ok := parseQuoted(line); ok {
				msgid.WriteString(value)
			}
			continue
		}
		if strings.HasPrefix(line, "msgstr ") {
			inMsgid = false
			inMsgstr = true
			if value, ok := parseQuoted(line); ok {
				msgstr.WriteString(value)
			}
			continue
		}
		if strings.HasPrefix(line, "\"") {
			if inMsgid {
				if value, ok := parseQuoted(line); ok {
					msgid.WriteString(value)
				}
			} else if inMsgstr {
				if value, ok := parseQuoted(line); ok {
					msgstr.WriteString(value)
				}
			}
		}
	}
	flush()

	return translations, msgids
}

func parseQuoted(line string) (string, bool) {
	idx := strings.Index(line, "\"")
	if idx < 0 {
		return "", false
	}
	value, err := strconv.Unquote(line[idx:])
	if err != nil {
		return "", false
	}
	return value, true
}

func interpolate(template string, args map[string]any) string {
	if template == "" || args == nil || !strings.Contains(template, "{") {
		return template
	}
	var out strings.Builder
	out.Grow(len(template))

	for i := 0; i < len(template); {
		if template[i] != '{' {
			out.WriteByte(template[i])
			i++
			continue
		}
		end := strings.IndexByte(template[i+1:], '}')
		if end < 0 {
			out.WriteByte(template[i])
			i++
			continue
		}
		key := template[i+1 : i+1+end]
		if value, ok := args[key]; ok {
			out.WriteString(formatValue(value))
			i += end + 2
			continue
		}
		out.WriteByte(template[i])
		i++
	}

	return out.String()
}

func formatValue(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case []string:
		return strings.Join(v, ",")
	case []int:
		parts := make([]string, len(v))
		for i, item := range v {
			parts[i] = fmt.Sprint(item)
		}
		return strings.Join(parts, ",")
	case []map[string]any:
		return formatStructuredMaps(v)
	case []any:
		parts := make([]string, len(v))
		for i, item := range v {
			if text, ok := formatStructuredItem(item); ok {
				parts[i] = text
				continue
			}
			parts[i] = formatValue(item)
		}
		return strings.Join(parts, ",")
	case map[string]any:
		if text, ok := formatStructuredMap(v); ok {
			return text
		}
		return fmt.Sprint(value)
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return fmt.Sprint(value)
	}
}

func formatStructuredMaps(items []map[string]any) string {
	if len(items) == 0 {
		return ""
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := formatStructuredMap(item); ok {
			parts = append(parts, text)
			continue
		}
		parts = append(parts, fmt.Sprint(item))
	}
	return strings.Join(parts, ";")
}

func formatStructuredItem(item any) (string, bool) {
	m, ok := item.(map[string]any)
	if !ok {
		return "", false
	}
	return formatStructuredMap(m)
}

func formatStructuredMap(item map[string]any) (string, bool) {
	if len(item) == 0 {
		return "", false
	}
	ns, hasNS := item["ns"].(string)
	address, hasAddress := item["address"].(string)
	if !hasNS && !hasAddress {
		return "", false
	}
	ns = strings.TrimSpace(ns)
	address = strings.TrimSpace(address)
	if ns == "" && address == "" {
		return "", true
	}
	if ns == "" {
		return address, true
	}
	if address == "" {
		return ns, true
	}
	return ns + "/" + address, true
}
