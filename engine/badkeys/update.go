// Package badkeys implements DNSKEY vulnerability checks and blocklist management.
package badkeys

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"github.com/ulikunitz/xz"
)

const (
	// UpdateURL is the upstream badkeys metadata endpoint.
	UpdateURL = "https://update.badkeys.info/v0/badkeysdata.json"
	bkFormat  = 0
)

// Metadata is the subset of badkeysdata.json needed for blocklist management.
type Metadata struct {
	// BKFormat is the badkeys blocklist format version.
	BKFormat int `json:"bkformat"`
	// Deprecated carries upstream deprecation metadata when present.
	Deprecated any `json:"deprecated,omitempty"`
	// BlocklistURL is the download URL for the compressed blocklist.
	BlocklistURL string `json:"blocklist_url"`
	// BlocklistSHA256 is the expected SHA-256 of the uncompressed blocklist.
	BlocklistSHA256 string `json:"blocklist_sha256"`
}

// Updater downloads the badkeys blocklist. The zero value fetches from
// UpdateURL over the default HTTP client.
type Updater struct {
	// URL overrides the metadata endpoint.
	URL string
	// Client overrides the HTTP client.
	Client *http.Client
}

// Update downloads the badkeys blocklist from the upstream endpoint.
func Update(outputDir string, w io.Writer) error {
	return Updater{}.Update(outputDir, w)
}

func (u Updater) metadataURL() string {
	if u.URL != "" {
		return u.URL
	}
	return UpdateURL
}

func (u Updater) client() *http.Client {
	if u.Client != nil {
		return u.Client
	}
	return http.DefaultClient
}

// Update downloads the badkeys blocklist to outputDir.
// It returns nil on success. Progress messages are written to w.
func (u Updater) Update(outputDir string, w io.Writer) error {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	fmt.Fprintln(w, "Downloading badkeysdata.json...")
	jsonBytes, err := u.httpGet(u.metadataURL())
	if err != nil {
		return fmt.Errorf("download badkeysdata.json: %w", err)
	}

	var data Metadata
	if err := json.Unmarshal(jsonBytes, &data); err != nil {
		return fmt.Errorf("parse badkeysdata.json: %w", err)
	}

	if data.BKFormat != bkFormat {
		return fmt.Errorf("unsupported bkformat %d (expected %d)", data.BKFormat, bkFormat)
	}

	if data.Deprecated != nil {
		fmt.Fprintln(w, "WARNING: blocklist format is deprecated, consider updating")
	}

	if data.BlocklistURL == "" {
		return fmt.Errorf("blocklist_url not present in badkeysdata.json")
	}

	// Check if existing blocklist already matches.
	blPath := filepath.Join(outputDir, "blocklist.dat")
	if existingHash, err := fileSHA256(blPath); err == nil {
		if existingHash == data.BlocklistSHA256 {
			fmt.Fprintln(w, "Blocklist is already up to date.")
			return writeAtomic(filepath.Join(outputDir, "badkeysdata.json"), jsonBytes)
		}
	}

	fmt.Fprintf(w, "Downloading blocklist from %s...\n", data.BlocklistURL)
	xzBytes, err := u.httpGet(data.BlocklistURL)
	if err != nil {
		return fmt.Errorf("download blocklist: %w", err)
	}

	fmt.Fprintln(w, "Decompressing blocklist...")
	r, err := xz.NewReader(bytes.NewReader(xzBytes))
	if err != nil {
		return fmt.Errorf("xz reader: %w", err)
	}
	plainBytes, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("xz decompress: %w", err)
	}

	h := sha256.Sum256(plainBytes)
	gotHash := hex.EncodeToString(h[:])
	if gotHash != data.BlocklistSHA256 {
		return fmt.Errorf("SHA-256 mismatch: got %s, expected %s", gotHash, data.BlocklistSHA256)
	}
	fmt.Fprintln(w, "SHA-256 verified.")

	if len(plainBytes)%16 != 0 {
		return fmt.Errorf("blocklist.dat size %d is not a multiple of 16 bytes", len(plainBytes))
	}
	entries := len(plainBytes) / 16
	fmt.Fprintf(w, "Blocklist contains %d entries (%d MB uncompressed).\n", entries, len(plainBytes)/(1024*1024))

	if err := writeAtomic(blPath, plainBytes); err != nil {
		return fmt.Errorf("write blocklist.dat: %w", err)
	}
	if err := writeAtomic(filepath.Join(outputDir, "badkeysdata.json"), jsonBytes); err != nil {
		return fmt.Errorf("write badkeysdata.json: %w", err)
	}

	fmt.Fprintln(w, "Done.")
	return nil
}

// DefaultDataDir returns the default user data directory for badkeys files.
// On Linux/macOS: ~/.local/share/gonemaster/badkeys
// On other platforms: ~/.gonemaster/badkeys
func DefaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
			return filepath.Join(xdg, "gonemaster", "badkeys")
		}
		return filepath.Join(home, ".local", "share", "gonemaster", "badkeys")
	}
	return filepath.Join(home, ".gonemaster", "badkeys")
}

func (u Updater) httpGet(url string) ([]byte, error) {
	resp, err := u.client().Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}
	return io.ReadAll(resp.Body)
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func writeAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
