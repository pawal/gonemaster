package badkeys

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"

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
//  3. Embedded data (if compiled with badkeys_embed build tag)
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

	// Try embedded data.
	if share.BadkeysBlocklist != nil && share.BadkeysMetadata != nil {
		return loadFromEmbedded()
	}

	return nil, nil
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

func (e *BlocklistError) Error() string { return e.msg }