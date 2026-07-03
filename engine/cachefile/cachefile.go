// Package cachefile defines the on-disk schema for gonemaster's unified
// packet cache save/restore format and orchestrates import/export against
// the nameserver and recursor caches.
package cachefile

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"codeberg.org/pawal/gonemaster/engine/asnlookup"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/recursor"
)

const (
	// Format identifies the on-disk cache file format.
	Format = "gonemaster.packet-cache"
	// Version is the current schema version.
	Version = 2

	// KindNameserver tags per-nameserver cache entries.
	KindNameserver = "nameserver"
	// KindRecursor tags recursor cache entries.
	KindRecursor = "recursor"
	// KindASN tags ASN lookup cache entries.
	KindASN = "asn"
	// KindAXFR tags zone-transfer cache entries.
	KindAXFR = "axfr"
)

// File is the portable representation of the unified packet cache.
type File struct {
	Format   string  `json:"format"`
	Version  int     `json:"version"`
	Checksum string  `json:"checksum,omitempty"`
	Entries  []Entry `json:"entries"`
}

// Entry is a single cache record discriminated by Kind.
type Entry struct {
	Kind string `json:"kind"`

	// Nameserver-kind fields.
	Address    string `json:"address,omitempty"`
	Key        string `json:"key,omitempty"`
	AnswerFrom string `json:"answer_from,omitempty"`

	// Recursor-kind fields.
	Name        string          `json:"name,omitempty"`
	QType       string          `json:"qtype,omitempty"`
	QClass      string          `json:"qclass,omitempty"`
	Nameservers []NameserverRef `json:"nameservers,omitempty"`

	// ASN-kind fields.
	IP   string `json:"ip,omitempty"`
	ASNs []int  `json:"asns,omitempty"`
	// Prefix is shared: routed prefix for ASN-kind, unused for others.
	Prefix string `json:"prefix,omitempty"`
	Raw    string `json:"raw,omitempty"`
	Code   string `json:"code,omitempty"`

	// AXFR-kind fields (reuse Address / Name / QClass).
	RRs        []string `json:"rrs,omitempty"`
	NoTransfer bool     `json:"no_transfer,omitempty"`

	// Common fields.
	Message   string `json:"message,omitempty"`
	NoMessage bool   `json:"no_message,omitempty"`
}

// NameserverRef identifies a nameserver for recursor-kind entries.
type NameserverRef struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

// Option configures Import/Restore behaviour.
type Option func(*config)

type config struct {
	strict bool
	warnf  func(string, ...any)
}

func newConfig(opts []Option) *config {
	c := &config{}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *config) warn(format string, args ...any) {
	if c.warnf != nil {
		c.warnf(format, args...)
	}
}

// WithStrict turns every non-fatal warning into an error. Useful in CI or
// automated replay pipelines where silent acceptance of unknown fields or
// missing checksums is undesirable.
func WithStrict() Option {
	return func(c *config) { c.strict = true }
}

// WithWarnf installs a callback that receives non-fatal warnings (unknown
// fields, unknown kinds, missing checksum). In strict mode these become
// errors instead.
func WithWarnf(f func(string, ...any)) Option {
	return func(c *config) { c.warnf = f }
}

// Export collects entries from the supplied caches into a File and stamps a
// checksum covering the entries.
func Export(ns *nameserver.CacheStore, rec *recursor.Recursor, asn *asnlookup.Cache) (File, error) {
	out := File{Format: Format, Version: Version, Entries: []Entry{}}

	if ns != nil {
		nsEntries, err := ns.ExportEntries()
		if err != nil {
			return File{}, err
		}
		for _, e := range nsEntries {
			entry := Entry{
				Kind:       KindNameserver,
				Address:    e.Address,
				Key:        e.Key,
				AnswerFrom: e.AnswerFrom,
				NoMessage:  e.NoMessage,
			}
			if !e.NoMessage {
				entry.Message = base64.StdEncoding.EncodeToString(e.Message)
			}
			out.Entries = append(out.Entries, entry)
		}

		axfrEntries, err := ns.ExportAXFREntries()
		if err != nil {
			return File{}, err
		}
		for _, e := range axfrEntries {
			entry := Entry{
				Kind:       KindAXFR,
				Address:    e.Address,
				Name:       e.Name,
				QClass:     e.QClass,
				NoTransfer: e.NoTransfer,
			}
			for _, rr := range e.RRs {
				entry.RRs = append(entry.RRs, base64.StdEncoding.EncodeToString(rr))
			}
			out.Entries = append(out.Entries, entry)
		}
	}

	if rec != nil {
		recEntries, err := rec.ExportCacheEntries()
		if err != nil {
			return File{}, err
		}
		for _, e := range recEntries {
			refs := make([]NameserverRef, 0, len(e.Nameservers))
			for _, r := range e.Nameservers {
				refs = append(refs, NameserverRef{Name: r.Name, Address: r.Address})
			}
			out.Entries = append(out.Entries, Entry{
				Kind:        KindRecursor,
				Name:        e.Name,
				QType:       e.QType,
				QClass:      e.QClass,
				Nameservers: refs,
				Message:     base64.StdEncoding.EncodeToString(e.Message),
			})
		}
	}

	if asn != nil {
		for _, e := range asn.ExportEntries() {
			out.Entries = append(out.Entries, Entry{
				Kind:   KindASN,
				IP:     e.IP,
				ASNs:   e.ASNs,
				Prefix: e.Prefix,
				Raw:    e.Raw,
				Code:   e.Code,
			})
		}
	}

	sum, err := checksumFor(out)
	if err != nil {
		return File{}, err
	}
	out.Checksum = sum
	return out, nil
}

// Import applies the supplied File to the caches. Either cache may be nil,
// in which case entries of that kind trigger an error.
func Import(file File, ns *nameserver.CacheStore, rec *recursor.Recursor, asn *asnlookup.Cache, opts ...Option) error {
	cfg := newConfig(opts)

	if err := validateHeaderAndChecksum(file, cfg); err != nil {
		return err
	}

	var nsEntries []nameserver.Entry
	var recEntries []recursor.CacheEntry
	var asnEntries []asnlookup.CacheEntry
	var axfrEntries []nameserver.AXFREntry

	for idx, entry := range file.Entries {
		switch entry.Kind {
		case KindNameserver:
			nsEntry := nameserver.Entry{
				Address:    entry.Address,
				Key:        entry.Key,
				AnswerFrom: entry.AnswerFrom,
				NoMessage:  entry.NoMessage,
			}
			if !entry.NoMessage {
				wire, err := base64.StdEncoding.DecodeString(entry.Message)
				if err != nil {
					return fmt.Errorf("entry %d: decode message: %w", idx, err)
				}
				nsEntry.Message = wire
			}
			nsEntries = append(nsEntries, nsEntry)
		case KindRecursor:
			wire, err := base64.StdEncoding.DecodeString(entry.Message)
			if err != nil {
				return fmt.Errorf("entry %d: decode message: %w", idx, err)
			}
			refs := make([]recursor.NameserverRef, 0, len(entry.Nameservers))
			for _, r := range entry.Nameservers {
				refs = append(refs, recursor.NameserverRef{Name: r.Name, Address: r.Address})
			}
			recEntries = append(recEntries, recursor.CacheEntry{
				Name:        entry.Name,
				QType:       entry.QType,
				QClass:      entry.QClass,
				Nameservers: refs,
				Message:     wire,
			})
		case KindASN:
			asnEntries = append(asnEntries, asnlookup.CacheEntry{
				IP:     entry.IP,
				ASNs:   entry.ASNs,
				Prefix: entry.Prefix,
				Raw:    entry.Raw,
				Code:   entry.Code,
			})
		case KindAXFR:
			axfrEntry := nameserver.AXFREntry{
				Address:    entry.Address,
				Name:       entry.Name,
				QClass:     entry.QClass,
				NoTransfer: entry.NoTransfer,
			}
			for _, s := range entry.RRs {
				wire, err := base64.StdEncoding.DecodeString(s)
				if err != nil {
					return fmt.Errorf("entry %d: decode rr: %w", idx, err)
				}
				axfrEntry.RRs = append(axfrEntry.RRs, wire)
			}
			axfrEntries = append(axfrEntries, axfrEntry)
		case "":
			if cfg.strict {
				return fmt.Errorf("entry %d: kind is required", idx)
			}
			cfg.warn("entry %d: kind is missing, skipping", idx)
			continue
		default:
			if cfg.strict {
				return fmt.Errorf("entry %d: unknown kind %q", idx, entry.Kind)
			}
			cfg.warn("entry %d: unknown kind %q, skipping", idx, entry.Kind)
			continue
		}
	}

	if len(nsEntries) > 0 {
		if ns == nil {
			return fmt.Errorf("nameserver cache entries present but nameserver cache is nil")
		}
		if err := ns.ImportEntries(nsEntries); err != nil {
			return err
		}
	}
	if len(recEntries) > 0 {
		if rec == nil {
			return fmt.Errorf("recursor cache entries present but recursor is nil")
		}
		if err := rec.ImportCacheEntries(recEntries); err != nil {
			return err
		}
	}
	if len(asnEntries) > 0 {
		if asn == nil {
			return fmt.Errorf("asn cache entries present but asn cache is nil")
		}
		if err := asn.ImportEntries(asnEntries); err != nil {
			return err
		}
	}
	if len(axfrEntries) > 0 {
		if ns == nil {
			return fmt.Errorf("axfr cache entries present but nameserver cache is nil")
		}
		if err := ns.ImportAXFREntries(axfrEntries); err != nil {
			return err
		}
	}

	return nil
}

// SaveOption configures Save behaviour.
type SaveOption func(*saveConfig)

type saveConfig struct {
	compress   bool
	maxEntries int
}

func newSaveConfig(opts []SaveOption) *saveConfig {
	c := &saveConfig{}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// WithCompression gzip-compresses the on-disk file. The output is a single
// gzip stream containing the same JSON document as the uncompressed form.
func WithCompression() SaveOption {
	return func(c *saveConfig) { c.compress = true }
}

// WithMaxEntries makes Save fail before writing if the exported file would
// contain more than n entries. A value of 0 (the default) disables the
// guardrail.
func WithMaxEntries(n int) SaveOption {
	return func(c *saveConfig) { c.maxEntries = n }
}

// gzipMagic identifies a gzip stream by its first two bytes (RFC 1952 §2.3.1).
var gzipMagic = []byte{0x1f, 0x8b}

// hasGzipSuffix reports whether path has a case-insensitive .gz suffix.
func hasGzipSuffix(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".gz")
}

// Save writes the caches to path as JSON. With WithCompression() (or a path
// ending in .gz) the JSON is wrapped in a single gzip stream.
func Save(path string, ns *nameserver.CacheStore, rec *recursor.Recursor, asn *asnlookup.Cache, opts ...SaveOption) error {
	target := strings.TrimSpace(path)
	if target == "" {
		return fmt.Errorf("packet cache save path is required")
	}
	cfg := newSaveConfig(opts)
	payload, err := Export(ns, rec, asn)
	if err != nil {
		return err
	}
	if cfg.maxEntries > 0 && len(payload.Entries) > cfg.maxEntries {
		return fmt.Errorf("packet cache has %d entries, exceeds max-entries=%d", len(payload.Entries), cfg.maxEntries)
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	if cfg.compress || hasGzipSuffix(target) {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		if _, werr := gz.Write(data); werr != nil {
			return werr
		}
		if cerr := gz.Close(); cerr != nil {
			return cerr
		}
		return os.WriteFile(target, buf.Bytes(), 0o644)
	}
	return os.WriteFile(target, data, 0o644)
}

// Restore reads path and imports its entries into the caches. A gzip stream
// is decompressed transparently (detected by magic bytes, regardless of file
// name).
func Restore(path string, ns *nameserver.CacheStore, rec *recursor.Recursor, asn *asnlookup.Cache, opts ...Option) error {
	cfg := newConfig(opts)

	source := strings.TrimSpace(path)
	if source == "" {
		return fmt.Errorf("packet cache restore path is required")
	}
	raw, err := os.ReadFile(source)
	if err != nil {
		return err
	}

	data, err := maybeDecompress(raw)
	if err != nil {
		return err
	}

	if err := reportUnknownFields(data, cfg); err != nil {
		return err
	}

	var payload File
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	return Import(payload, ns, rec, asn, opts...)
}

// maybeDecompress returns the gzip-decompressed payload if data starts with
// the gzip magic bytes; otherwise it returns data unchanged.
func maybeDecompress(data []byte) ([]byte, error) {
	if len(data) < 2 || !bytes.HasPrefix(data, gzipMagic) {
		return data, nil
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("gzip header: %w", err)
	}
	defer gz.Close()
	out, err := io.ReadAll(gz)
	if err != nil {
		return nil, fmt.Errorf("gzip body: %w", err)
	}
	return out, nil
}

// validateHeaderAndChecksum checks format, version, and checksum. A missing
// checksum warns in lenient mode and errors in strict mode.
func validateHeaderAndChecksum(file File, cfg *config) error {
	if strings.TrimSpace(file.Format) == "" {
		return fmt.Errorf("packet cache format is required")
	}
	if file.Format != Format {
		return fmt.Errorf("unsupported packet cache format %q", file.Format)
	}
	if file.Version != Version {
		return fmt.Errorf("unsupported packet cache version %d", file.Version)
	}
	if strings.TrimSpace(file.Checksum) == "" {
		if cfg.strict {
			return fmt.Errorf("packet cache checksum is missing")
		}
		cfg.warn("packet cache checksum is missing")
		return nil
	}
	want := strings.ToLower(strings.TrimSpace(file.Checksum))
	got, err := checksumFor(file)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("packet cache checksum mismatch: want %s, got %s", want, got)
	}
	return nil
}

// Load parses path (gzip auto-detected) and validates it without applying the
// entries to any cache. Honors WithStrict / WithWarnf.
func Load(path string, opts ...Option) (File, error) {
	cfg := newConfig(opts)

	source := strings.TrimSpace(path)
	if source == "" {
		return File{}, fmt.Errorf("packet cache path is required")
	}
	raw, err := os.ReadFile(source)
	if err != nil {
		return File{}, err
	}
	data, err := maybeDecompress(raw)
	if err != nil {
		return File{}, err
	}
	if err := reportUnknownFields(data, cfg); err != nil {
		return File{}, err
	}
	var payload File
	if err := json.Unmarshal(data, &payload); err != nil {
		return File{}, err
	}
	if err := validateHeaderAndChecksum(payload, cfg); err != nil {
		return File{}, err
	}
	return payload, nil
}

// Stats summarizes the entries in a parsed cache file.
type Stats struct {
	Total     int
	ByKind    map[string]int // entries per kind
	ByAddress map[string]int // nameserver entries per address
}

// Stats counts entries by kind and, for nameserver entries, by address.
func (f File) Stats() Stats {
	s := Stats{ByKind: map[string]int{}, ByAddress: map[string]int{}}
	for _, e := range f.Entries {
		s.Total++
		s.ByKind[e.Kind]++
		if e.Kind == KindNameserver {
			s.ByAddress[e.Address]++
		}
	}
	return s
}

// checksumFor computes the SHA-256 checksum of the file with Checksum blanked.
// Output is lowercase hex.
func checksumFor(file File) (string, error) {
	file.Checksum = ""
	data, err := json.Marshal(file)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

var knownFileFields = map[string]bool{
	"format":   true,
	"version":  true,
	"checksum": true,
	"entries":  true,
}

var knownEntryFields = map[string]bool{
	"kind":        true,
	"address":     true,
	"key":         true,
	"answer_from": true,
	"name":        true,
	"qtype":       true,
	"qclass":      true,
	"nameservers": true,
	"message":     true,
	"no_message":  true,
	"ip":          true,
	"asns":        true,
	"prefix":      true,
	"raw":         true,
	"code":        true,
	"rrs":         true,
	"no_transfer": true,
}

func reportUnknownFields(data []byte, cfg *config) error {
	var fileMap map[string]json.RawMessage
	if err := json.Unmarshal(data, &fileMap); err != nil {
		// Not a JSON object at the top level; let the struct decoder
		// produce the canonical error.
		return nil
	}
	for key := range fileMap {
		if knownFileFields[key] {
			continue
		}
		if cfg.strict {
			return fmt.Errorf("unknown field %q", key)
		}
		cfg.warn("unknown field %q", key)
	}
	entriesRaw, ok := fileMap["entries"]
	if !ok {
		return nil
	}
	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(entriesRaw, &entries); err != nil {
		return nil
	}
	for idx, entry := range entries {
		for key := range entry {
			if knownEntryFields[key] {
				continue
			}
			if cfg.strict {
				return fmt.Errorf("entry %d: unknown field %q", idx, key)
			}
			cfg.warn("entry %d: unknown field %q", idx, key)
		}
	}
	return nil
}
