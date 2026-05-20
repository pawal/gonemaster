package badkeys

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"strings"

	"codeberg.org/pawal/gonemaster/share"
)

const blockSize = 16 // 15-byte BKHASH120 + 1-byte source ID

// Blocklist holds the loaded blocklist data and metadata.
type Blocklist struct {
	Data     []byte            // raw blocklist.dat content (sorted 16-byte blocks)
	Sources  map[int]string    // source ID → blocklist name
	Entries  int               // number of 16-byte blocks
}

// LoadBlocklist finds and loads the blocklist using the search path:
//  1. profilePath (from badkeys.path profile key / --badkeys-path CLI flag)
//  2. XDG user data directory (~/.local/share/gonemaster/badkeys/)
//  3. XDG system data directories (XDG_DATA_DIRS, default
//     /usr/local/share:/usr/share), looking under <dir>/gonemaster/badkeys/
//  4. Embedded data (if compiled with badkeys_embed build tag)
//
// Returns nil, nil if no blocklist is available anywhere.
func LoadBlocklist(profilePath string) (*Blocklist, error) {
	// Try explicit path first.
	if profilePath != "" {
		return loadFromDir(profilePath)
	}

	// Try default user data directory.
	dataDir := DefaultDataDir()
	if bl, err := loadFromDir(dataDir); err == nil && bl != nil {
		return bl, nil
	}

	// Try system data directories per XDG_DATA_DIRS.
	for _, sysDir := range systemDataDirs() {
		candidate := filepath.Join(sysDir, "gonemaster", "badkeys")
		if bl, err := loadFromDir(candidate); err == nil && bl != nil {
			return bl, nil
		}
	}

	// Try embedded data.
	if share.BadkeysBlocklist != nil && share.BadkeysMetadata != nil {
		return loadFromEmbedded()
	}

	return nil, nil
}

// systemDataDirs returns the directories from XDG_DATA_DIRS, falling back to
// the spec default (/usr/local/share, /usr/share) when unset or empty.
func systemDataDirs() []string {
	raw := os.Getenv("XDG_DATA_DIRS")
	if raw == "" {
		return []string{"/usr/local/share", "/usr/share"}
	}
	parts := strings.Split(raw, ":")
	out := make([]string, 0, len(parts))
	for _, d := range parts {
		if d != "" {
			out = append(out, d)
		}
	}
	if len(out) == 0 {
		return []string{"/usr/local/share", "/usr/share"}
	}
	return out
}

func loadFromDir(dir string) (*Blocklist, error) {
	blPath := filepath.Join(dir, "blocklist.dat")
	metaPath := filepath.Join(dir, "badkeysdata.json")

	data, err := os.ReadFile(blPath)
	if err != nil {
		return nil, err
	}
	if len(data)%blockSize != 0 {
		return nil, &BlocklistError{msg: "blocklist.dat size is not a multiple of 16 bytes"}
	}

	sources, err := loadSources(metaPath)
	if err != nil {
		// Blocklist without metadata is still usable, just no source names.
		sources = map[int]string{}
	}

	return &Blocklist{
		Data:    data,
		Sources: sources,
		Entries: len(data) / blockSize,
	}, nil
}

func loadFromEmbedded() (*Blocklist, error) {
	// Embedded blocklist is gzip-compressed.
	gr, err := gzip.NewReader(bytes.NewReader(share.BadkeysBlocklist))
	if err != nil {
		return nil, err
	}
	defer gr.Close()

	data, err := io.ReadAll(gr)
	if err != nil {
		return nil, err
	}
	if len(data)%blockSize != 0 {
		return nil, &BlocklistError{msg: "embedded blocklist size is not a multiple of 16 bytes"}
	}

	sources, err := parseSources(share.BadkeysMetadata)
	if err != nil {
		sources = map[int]string{}
	}

	return &Blocklist{
		Data:    data,
		Sources: sources,
		Entries: len(data) / blockSize,
	}, nil
}

func loadSources(metaPath string) (map[int]string, error) {
	raw, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, err
	}
	return parseSources(raw)
}

// badkeysDataFull is the full badkeysdata.json structure for source parsing.
type badkeysDataFull struct {
	Blocklists []blocklistEntry `json:"blocklists"`
}

type blocklistEntry struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func parseSources(raw []byte) (map[int]string, error) {
	var data badkeysDataFull
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	sources := make(map[int]string, len(data.Blocklists))
	for _, bl := range data.Blocklists {
		sources[bl.ID] = bl.Name
	}
	return sources, nil
}

// BlocklistError is returned for blocklist format issues.
type BlocklistError struct {
	msg string
}

// Error returns the human-readable blocklist format error.
func (e *BlocklistError) Error() string { return e.msg }

// BKHASH120 computes the badkeys truncated hash for a key's numeric value.
// The value is encoded as big-endian bytes without leading zeros, SHA-256
// hashed, and truncated to 15 bytes (120 bits).
func BKHASH120(val *big.Int) [15]byte {
	b := val.Bytes() // big-endian, no leading zeros
	h := sha256.Sum256(b)
	var trunc [15]byte
	copy(trunc[:], h[:15])
	return trunc
}

// CheckResult holds the result of a blocklist lookup.
type CheckResult struct {
	// SourceID is the badkeys source identifier encoded in the blocklist.
	SourceID int
	// SourceName is the resolved human-readable source name.
	SourceName string
}

// Check looks up a key's numeric value in the blocklist.
// Returns nil if the key is not found.
func (bl *Blocklist) Check(val *big.Int) *CheckResult {
	if bl == nil || bl.Entries == 0 {
		return nil
	}

	hash := BKHASH120(val)

	lo, hi := 0, bl.Entries-1
	for lo <= hi {
		mid := lo + (hi-lo)/2
		offset := mid * blockSize
		entry := bl.Data[offset : offset+15]

		cmp := bytes.Compare(hash[:], entry)
		if cmp == 0 {
			sourceID := int(bl.Data[offset+15])
			name := fmt.Sprintf("id%d", sourceID)
			if n, ok := bl.Sources[sourceID]; ok {
				name = n
			}
			return &CheckResult{
				SourceID:   sourceID,
				SourceName: name,
			}
		}
		if cmp > 0 {
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}

	return nil
}
