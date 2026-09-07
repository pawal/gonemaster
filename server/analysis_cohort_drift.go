package server

import (
	"sort"
	"strings"
	"time"
)

// driftDomainPage bounds one page of the source-tag read. A cohort larger
// than one page is read in several, so a big tag is not silently truncated
// into false "missing" entries.
const driftDomainPage = 2000

// analysisCohortSourceDrift compares the cohort's source tag against its
// reference list. Nil when the cohort names no list, the provider is off, or
// the list has not been fetched yet: an empty list would report every member
// of the tag as extra.
func (s *Server) analysisCohortSourceDrift(cohort AnalysisCohort) *AnalysisCohortSourceDrift {
	if cohort.ReferenceList == "" {
		return nil
	}
	lists := s.ReferenceLists()
	if lists == nil {
		return nil
	}
	names, version, ok := lists.ReferenceList(cohort.ReferenceList)
	if !ok {
		return nil
	}

	inList := make(map[string]struct{}, len(names))
	for _, name := range names {
		if n := normalizeDriftName(name); n != "" {
			inList[n] = struct{}{}
		}
	}
	inTag := map[string]struct{}{}
	for _, name := range s.tagDomainNames(cohort.SourceTag) {
		if n := normalizeDriftName(name); n != "" {
			inTag[n] = struct{}{}
		}
	}

	drift := &AnalysisCohortSourceDrift{
		CheckedAt:   time.Now().UTC(),
		ListVersion: version,
		Missing:     []string{},
		Extra:       []string{},
	}
	for name := range inList {
		if _, found := inTag[name]; !found {
			drift.Missing = append(drift.Missing, name)
		}
	}
	for name := range inTag {
		if _, found := inList[name]; !found {
			drift.Extra = append(drift.Extra, name)
		}
	}
	sort.Strings(drift.Missing)
	sort.Strings(drift.Extra)
	return drift
}

// tagDomainNames reads every domain name carrying tag.
func (s *Server) tagDomainNames(tag string) []string {
	var out []string
	for offset := 0; ; offset += driftDomainPage {
		list := s.store.ListDomainsByTag(tag, DomainFilter{Limit: driftDomainPage, Offset: offset})
		for _, d := range list.Items {
			out = append(out, d.Name)
		}
		if len(list.Items) < driftDomainPage {
			return out
		}
	}
}

// normalizeDriftName lowercases a name and drops one trailing root dot so
// both sides of the comparison use the same key.
func normalizeDriftName(name string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), ".")
}
