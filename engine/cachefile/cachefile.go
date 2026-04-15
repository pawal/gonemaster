// Package cachefile defines the on-disk schema for gonemaster's unified
// packet cache save/restore format and orchestrates import/export against
// the nameserver and recursor caches.
package cachefile

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

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
)

// File is the portable representation of the unified packet cache.
type File struct {
	Format  string  `json:"format"`
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
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

	// Common fields.
	Message   string `json:"message,omitempty"`
	NoMessage bool   `json:"no_message,omitempty"`
}

// NameserverRef identifies a nameserver for recursor-kind entries.
type NameserverRef struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

// Export collects entries from the supplied caches into a File.
// Either cache may be nil.
func Export(ns *nameserver.CacheStore, rec *recursor.Recursor) (File, error) {
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

	return out, nil
}

// Import applies the supplied File to the caches. Either cache may be nil,
// in which case entries of that kind are skipped.
func Import(file File, ns *nameserver.CacheStore, rec *recursor.Recursor) error {
	if strings.TrimSpace(file.Format) == "" {
		return fmt.Errorf("packet cache format is required")
	}
	if file.Format != Format {
		return fmt.Errorf("unsupported packet cache format %q", file.Format)
	}
	if file.Version != Version {
		return fmt.Errorf("unsupported packet cache version %d", file.Version)
	}

	var nsEntries []nameserver.Entry
	var recEntries []recursor.CacheEntry

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
		case "":
			return fmt.Errorf("entry %d: kind is required", idx)
		default:
			return fmt.Errorf("entry %d: unknown kind %q", idx, entry.Kind)
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

	return nil
}

// Save writes the caches to path as JSON.
func Save(path string, ns *nameserver.CacheStore, rec *recursor.Recursor) error {
	target := strings.TrimSpace(path)
	if target == "" {
		return fmt.Errorf("packet cache save path is required")
	}
	payload, err := Export(ns, rec)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(target, data, 0o644)
}

// Restore reads path and imports its entries into the caches.
func Restore(path string, ns *nameserver.CacheStore, rec *recursor.Recursor) error {
	source := strings.TrimSpace(path)
	if source == "" {
		return fmt.Errorf("packet cache restore path is required")
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	var payload File
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	return Import(payload, ns, rec)
}
