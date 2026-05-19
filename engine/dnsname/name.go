package dnsname

import "strings"

// Name represents a DNS name split into labels.
type Name struct {
	labels []string
	cached *string
}

// FromString parses a domain string into a Name, preserving a single trailing dot when present.
func FromString(domain string) (Name, error) {
	if domain == "" {
		empty := ""
		return Name{labels: []string{}, cached: &empty}, nil
	}

	labels := splitDomain(domain)
	cache := domain
	if strings.HasSuffix(cache, ".") && len(cache) > 1 {
		cache = strings.TrimSuffix(cache, ".")
	}

	cached := cache
	return Name{labels: labels, cached: &cached}, nil
}

// New builds a Name from a domain string by splitting it into labels.
func New(domain string) Name {
	if domain == "" {
		return Name{labels: []string{}}
	}

	return Name{labels: splitDomain(domain)}
}

// NewFromLabels constructs a Name from labels.
func NewFromLabels(labels []string) Name {
	copyLabels := make([]string, len(labels))
	copy(copyLabels, labels)
	return Name{labels: copyLabels}
}

// Labels returns a copy of the labels slice.
func (n Name) Labels() []string {
	copyLabels := make([]string, len(n.labels))
	copy(copyLabels, n.labels)
	return copyLabels
}

// String returns the name without a trailing dot, or a single dot for root.
func (n Name) String() string {
	if n.cached != nil {
		return *n.cached
	}

	joined := strings.Join(n.labels, ".")
	if joined == "" {
		joined = "."
	}
	return joined
}

// FQDN returns the name with a trailing dot.
func (n Name) FQDN() string {
	if len(n.labels) == 0 {
		return "."
	}

	return strings.Join(n.labels, ".") + "."
}

// Compare compares names case-insensitively.
func (n Name) Compare(other Name) int {
	left := strings.ToUpper(n.String())
	otherName := strings.ToUpper(other.String())
	return strings.Compare(left, otherName)
}

// CompareString compares against a raw string, ignoring a single trailing dot.
func (n Name) CompareString(other string) int {
	left := strings.ToUpper(n.String())
	right := strings.ToUpper(trimTrailingDot(other))
	return strings.Compare(left, right)
}

// NextHigher returns the parent name, or false for the root.
func (n Name) NextHigher() (Name, bool) {
	if len(n.labels) == 0 {
		return Name{}, false
	}

	labels := make([]string, len(n.labels)-1)
	copy(labels, n.labels[1:])
	return Name{labels: labels}, true
}

// Common returns the number of shared labels from the right.
func (n Name) Common(other Name) int {
	me := reverseCopy(n.labels)
	them := reverseCopy(other.labels)

	count := 0
	for len(me) > 0 && len(them) > 0 {
		m := strings.ToUpper(me[0])
		t := strings.ToUpper(them[0])
		if m != t {
			break
		}
		count++
		me = me[1:]
		them = them[1:]
	}

	return count
}

// IsInBailiwick reports whether other is within this name's bailiwick.
func (n Name) IsInBailiwick(other Name) bool {
	return len(n.labels) == n.Common(other)
}

// Prepend returns a new name with label added to the front.
func (n Name) Prepend(label string) Name {
	labels := make([]string, 0, len(n.labels)+1)
	labels = append(labels, label)
	labels = append(labels, n.labels...)
	return Name{labels: labels}
}

func splitDomain(domain string) []string {
	parts := strings.Split(domain, ".")
	for len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}

func trimTrailingDot(value string) string {
	if strings.HasSuffix(value, ".") && len(value) > 1 {
		return strings.TrimSuffix(value, ".")
	}
	return value
}

func reverseCopy(input []string) []string {
	out := make([]string, len(input))
	for i := range input {
		out[i] = input[len(input)-1-i]
	}
	return out
}

// StringLower returns String() lowercased.
func (n Name) StringLower() string {
	return strings.ToLower(n.String())
}

// StringUpper returns String() uppercased.
func (n Name) StringUpper() string {
	return strings.ToUpper(n.String())
}
