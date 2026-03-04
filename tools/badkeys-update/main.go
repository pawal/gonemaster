// Command badkeys-update downloads the badkeys blocklist data files.
//
// It fetches badkeysdata.json from the upstream update server, then downloads
// and decompresses blocklist.dat.xz, verifying the SHA-256 hash of the
// uncompressed data. Files are stored in the specified output directory.
//
// Usage:
//
//	go run ./tools/badkeys-update [--output DIR]
//
// The default output directory is share/badkeys/.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/ulikunitz/xz"
)

const (
	updateURL = "https://update.badkeys.info/v0/badkeysdata.json"
	bkFormat  = 0
)

// badkeysData is the subset of badkeysdata.json we need.
type badkeysData struct {
	BKFormat        int    `json:"bkformat"`
	Deprecated      any    `json:"deprecated,omitempty"`
	BlocklistURL    string `json:"blocklist_url"`
	BlocklistSHA256 string `json:"blocklist_sha256"`
}

func main() {
	outputDir := flag.String("output", "share/badkeys", "Output directory for blocklist files")
	flag.Parse()

	if err := run(*outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "badkeys-update: %v\n", err)
		os.Exit(1)
	}
}

func run(outputDir string) error {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	// Step 1: Download badkeysdata.json.
	fmt.Println("Downloading badkeysdata.json...")
	jsonBytes, err := httpGet(updateURL)
	if err != nil {
		return fmt.Errorf("download badkeysdata.json: %w", err)
	}

	var data badkeysData
	if err := json.Unmarshal(jsonBytes, &data); err != nil {
		return fmt.Errorf("parse badkeysdata.json: %w", err)
	}

	if data.BKFormat != bkFormat {
		return fmt.Errorf("unsupported bkformat %d (expected %d)", data.BKFormat, bkFormat)
	}

	if data.Deprecated != nil {
		fmt.Fprintf(os.Stderr, "WARNING: blocklist format is deprecated, consider updating badkeys-update tool\n")
	}

	if data.BlocklistURL == "" {
		return fmt.Errorf("blocklist_url not present in badkeysdata.json")
	}

	// Check if existing blocklist already matches.
	blPath := filepath.Join(outputDir, "blocklist.dat")
	if existingHash, err := fileSHA256(blPath); err == nil {
		if existingHash == data.BlocklistSHA256 {
			fmt.Println("Blocklist is already up to date.")
			return writeAtomic(filepath.Join(outputDir, "badkeysdata.json"), jsonBytes)
		}
	}

	// Step 2: Download blocklist.dat.xz.
	fmt.Printf("Downloading blocklist from %s...\n", data.BlocklistURL)
	xzBytes, err := httpGet(data.BlocklistURL)
	if err != nil {
		return fmt.Errorf("download blocklist.dat.xz: %w", err)
	}

	// Step 3: Decompress xz.
	fmt.Println("Decompressing blocklist...")
	plainBytes, err := xzDecompress(xzBytes)
	if err != nil {
		return fmt.Errorf("decompress blocklist: %w", err)
	}

	// Step 4: Verify SHA-256 hash.
	h := sha256.Sum256(plainBytes)
	gotHash := hex.EncodeToString(h[:])
	if gotHash != data.BlocklistSHA256 {
		return fmt.Errorf("SHA-256 mismatch: got %s, expected %s", gotHash, data.BlocklistSHA256)
	}
	fmt.Println("SHA-256 verified.")

	// Step 5: Validate blocklist structure (must be multiple of 16 bytes).
	if len(plainBytes)%16 != 0 {
		return fmt.Errorf("blocklist.dat size %d is not a multiple of 16 bytes", len(plainBytes))
	}
	entries := len(plainBytes) / 16
	fmt.Printf("Blocklist contains %d entries (%d MB uncompressed).\n", entries, len(plainBytes)/(1024*1024))

	// Step 6: Write files atomically.
	if err := writeAtomic(blPath, plainBytes); err != nil {
		return fmt.Errorf("write blocklist.dat: %w", err)
	}
	if err := writeAtomic(filepath.Join(outputDir, "badkeysdata.json"), jsonBytes); err != nil {
		return fmt.Errorf("write badkeysdata.json: %w", err)
	}

	fmt.Println("Done.")
	return nil
}

func httpGet(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}

	return io.ReadAll(resp.Body)
}

func xzDecompress(data []byte) ([]byte, error) {
	r, err := xz.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("xz reader: %w", err)
	}
	return io.ReadAll(r)
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
