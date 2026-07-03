package nameserver

import (
	"fmt"
	"sort"
	"strings"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"
)

// AXFREntry is a portable representation of a cached zone-transfer result.
type AXFREntry struct {
	Address    string
	Name       string
	QClass     string
	RRs        [][]byte // wire bytes, one single-RR message per element
	NoTransfer bool
}

// axfrRecord is the in-memory form of a cached transfer.
type axfrRecord struct {
	address    string
	name       string // normalized fqdn, lower case
	qclass     string // upper case
	rrs        []dns.RR
	noTransfer bool
}

func axfrKey(address, name, qclass string) string {
	return address + "|" + normalizeAXFRName(name) + "|" + normalizeAXFRClass(qclass)
}

func normalizeAXFRName(name string) string {
	return strings.ToLower(dnsutil.Fqdn(strings.TrimSpace(name)))
}

func normalizeAXFRClass(qclass string) string {
	qc := strings.ToUpper(strings.TrimSpace(qclass))
	if qc == "" {
		qc = "IN"
	}
	return qc
}

func (c *CacheStore) axfrLookup(address, name, qclass string) (*axfrRecord, bool) {
	if c == nil {
		return nil, false
	}
	c.axfrMu.Lock()
	defer c.axfrMu.Unlock()
	rec, ok := c.axfrCache[axfrKey(address, name, qclass)]
	return rec, ok
}

func (c *CacheStore) axfrStore(address, name, qclass string, rrs []dns.RR, noTransfer bool) {
	if c == nil {
		return
	}
	nm := normalizeAXFRName(name)
	qc := normalizeAXFRClass(qclass)
	c.axfrMu.Lock()
	defer c.axfrMu.Unlock()
	if c.axfrCache == nil {
		c.axfrCache = map[string]*axfrRecord{}
	}
	c.axfrCache[address+"|"+nm+"|"+qc] = &axfrRecord{
		address:    address,
		name:       nm,
		qclass:     qc,
		rrs:        rrs,
		noTransfer: noTransfer,
	}
}

// ExportAXFREntries returns a deterministic snapshot of the AXFR cache.
func (c *CacheStore) ExportAXFREntries() ([]AXFREntry, error) {
	if c == nil {
		return nil, nil
	}
	c.axfrMu.Lock()
	keys := make([]string, 0, len(c.axfrCache))
	for k := range c.axfrCache {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	records := make([]*axfrRecord, 0, len(keys))
	for _, k := range keys {
		records = append(records, c.axfrCache[k])
	}
	c.axfrMu.Unlock()

	entries := make([]AXFREntry, 0, len(records))
	for _, rec := range records {
		entry := AXFREntry{Address: rec.address, Name: rec.name, QClass: rec.qclass, NoTransfer: rec.noTransfer}
		for _, rr := range rec.rrs {
			wire, err := packRR(rr)
			if err != nil {
				return nil, fmt.Errorf("pack axfr rr (%s, %s): %w", rec.address, rec.name, err)
			}
			entry.RRs = append(entry.RRs, wire)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// ImportAXFREntries merges the supplied AXFR entries into the store, validating
// that each entry carries either records or a no-transfer marker (not both).
func (c *CacheStore) ImportAXFREntries(entries []AXFREntry) error {
	if c == nil {
		return fmt.Errorf("cache store is nil")
	}
	for idx, entry := range entries {
		address, err := normalizeCacheAddress(entry.Address)
		if err != nil {
			return fmt.Errorf("axfr entry %d: %w", idx, err)
		}
		if strings.TrimSpace(entry.Name) == "" {
			return fmt.Errorf("axfr entry %d: name is required", idx)
		}
		if entry.NoTransfer && len(entry.RRs) > 0 {
			return fmt.Errorf("axfr entry %d: has both rrs and no_transfer", idx)
		}
		if !entry.NoTransfer && len(entry.RRs) == 0 {
			return fmt.Errorf("axfr entry %d: has neither rrs nor no_transfer", idx)
		}
		var rrs []dns.RR
		for _, wire := range entry.RRs {
			rr, err := unpackRR(wire)
			if err != nil {
				return fmt.Errorf("axfr entry %d: %w", idx, err)
			}
			rrs = append(rrs, rr)
		}
		c.axfrStore(address, entry.Name, entry.QClass, rrs, entry.NoTransfer)
	}
	return nil
}

// packRR encodes a single RR as the wire form of a one-answer DNS message.
// The fork has no standalone RR codec, so a minimal message is the portable unit.
func packRR(rr dns.RR) ([]byte, error) {
	m := new(dns.Msg)
	m.Answer = []dns.RR{rr}
	if err := m.Pack(); err != nil {
		return nil, err
	}
	return append([]byte(nil), m.Data...), nil
}

// unpackRR reverses packRR, requiring exactly one answer RR.
func unpackRR(wire []byte) (dns.RR, error) {
	m := new(dns.Msg)
	m.Data = append([]byte(nil), wire...)
	if err := m.Unpack(); err != nil {
		return nil, err
	}
	if len(m.Answer) != 1 {
		return nil, fmt.Errorf("expected 1 rr in axfr record, got %d", len(m.Answer))
	}
	return m.Answer[0], nil
}
