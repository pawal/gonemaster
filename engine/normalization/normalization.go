package normalization

import (
	"regexp"
	"strings"

	"golang.org/x/net/idna"
)

var (
	validASCII = regexp.MustCompile(`^[A-Za-z0-9/_-]+$`)
	asciiOnly  = regexp.MustCompile(`^[\x00-\x7F]+$`)
)

var fullStopReplacer = strings.NewReplacer(
	"\uFF0E", ".", // FULLWIDTH FULL STOP
	"\u3002", ".", // IDEOGRAPHIC FULL STOP
	"\uFF61", ".", // HALFWIDTH IDEOGRAPHIC FULL STOP
)

var whitespaceRunes = map[rune]bool{
	0x0020: true,
	0x0009: true,
	0x00A0: true,
	0x2000: true,
	0x2001: true,
	0x2002: true,
	0x2003: true,
	0x2004: true,
	0x2005: true,
	0x2006: true,
	0x2007: true,
	0x2008: true,
	0x2009: true,
	0x200A: true,
	0x205F: true,
	0x3000: true,
	0x1680: true,
}

var ambiguousRunes = map[rune]string{
	0x0130: "LATIN CAPITAL LETTER I WITH DOT ABOVE",
}

// TrimSpace removes leading and trailing whitespace characters defined by the spec.
func TrimSpace(str string) string {
	return strings.TrimFunc(str, func(r rune) bool {
		return whitespaceRunes[r]
	})
}

// NormalizeLabel normalizes a single label and returns errors and the A-label.
func NormalizeLabel(label string) ([]Error, string) {
	if validASCII.MatchString(label) {
		alabel := strings.ToLower(label)
		if len(alabel) > 63 {
			err, _ := NewError("LABEL_TOO_LONG", map[string]string{"label": label})
			return []Error{err}, ""
		}
		return nil, alabel
	}

	if asciiOnly.MatchString(label) {
		err, _ := NewError("INVALID_ASCII", map[string]string{"label": label})
		return []Error{err}, ""
	}

	alabel, err := idna.Lookup.ToASCII(label)
	if err != nil {
		bad, _ := NewError("INVALID_U_LABEL", map[string]string{"label": label})
		return []Error{bad}, ""
	}

	if len(alabel) > 63 {
		bad, _ := NewError("LABEL_TOO_LONG", map[string]string{"label": label})
		return []Error{bad}, ""
	}

	return nil, alabel
}

// NormalizeName normalizes a domain name and returns errors and the normalized name.
func NormalizeName(name string) ([]Error, string) {
	if len(name) == 0 {
		err, _ := NewError("EMPTY_DOMAIN_NAME", nil)
		return []Error{err}, ""
	}

	for r, label := range ambiguousRunes {
		if strings.ContainsRune(name, r) {
			err, _ := NewError("AMBIGUOUS_DOWNCASING", map[string]string{"unicode_name": label})
			return []Error{err}, ""
		}
	}

	uname := fullStopReplacer.Replace(name)

	if uname == "." {
		return nil, uname
	}

	if strings.HasPrefix(uname, ".") {
		err, _ := NewError("INITIAL_DOT", nil)
		return []Error{err}, ""
	}

	if strings.Contains(uname, "..") {
		err, _ := NewError("REPEATED_DOTS", nil)
		return []Error{err}, ""
	}

	uname = strings.TrimSuffix(uname, ".")

	labels := strings.Split(uname, ".")
	normalized := make([]string, 0, len(labels))
	var errors []Error
	for _, label := range labels {
		labelErrors, alabel := NormalizeLabel(label)
		if len(labelErrors) > 0 {
			errors = append(errors, labelErrors...)
		}
		normalized = append(normalized, alabel)
	}

	if len(errors) > 0 {
		return errors, ""
	}

	finalName := strings.Join(normalized, ".")
	if len(finalName) > 253 {
		err, _ := NewError("DOMAIN_NAME_TOO_LONG", nil)
		return []Error{err}, ""
	}

	return nil, finalName
}
