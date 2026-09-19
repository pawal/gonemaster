package badkeys

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ulikunitz/xz"
)

// updateFixture serves badkeysdata.json and the compressed blocklist it
// points at, so an update runs its whole flow without reaching upstream.
type updateFixture struct {
	srv       *httptest.Server
	blocklist []byte
	meta      Metadata
	// metaJSON overrides the served metadata when non-empty.
	metaJSON []byte
	// metaStatus overrides the metadata response status.
	metaStatus int
	metaHits   int
}

func newUpdateFixture(t *testing.T, entries int) *updateFixture {
	t.Helper()

	f := &updateFixture{blocklist: bytes.Repeat([]byte("0123456789abcdef"), entries)}
	mux := http.NewServeMux()
	mux.HandleFunc("/badkeysdata.json", func(w http.ResponseWriter, _ *http.Request) {
		f.metaHits++
		if f.metaStatus != 0 {
			w.WriteHeader(f.metaStatus)
			return
		}
		if len(f.metaJSON) > 0 {
			_, _ = w.Write(f.metaJSON)
			return
		}
		body, err := json.Marshal(f.meta)
		if err != nil {
			t.Errorf("marshal metadata: %v", err)
			return
		}
		_, _ = w.Write(body)
	})
	mux.HandleFunc("/blocklist.xz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(xzCompress(t, f.blocklist))
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)

	sum := sha256.Sum256(f.blocklist)
	f.meta = Metadata{
		BKFormat:        bkFormat,
		BlocklistURL:    f.srv.URL + "/blocklist.xz",
		BlocklistSHA256: hex.EncodeToString(sum[:]),
	}
	return f
}

func (f *updateFixture) updater() Updater {
	return Updater{URL: f.srv.URL + "/badkeysdata.json", Client: f.srv.Client()}
}

func xzCompress(t *testing.T, plain []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := xz.NewWriter(&buf)
	if err != nil {
		t.Fatalf("xz writer: %v", err)
	}
	if _, err := w.Write(plain); err != nil {
		t.Fatalf("xz write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("xz close: %v", err)
	}
	return buf.Bytes()
}

func TestUpdateWritesVerifiedBlocklist(t *testing.T) {
	f := newUpdateFixture(t, 3)
	dir := filepath.Join(t.TempDir(), "badkeys")

	var out bytes.Buffer
	if err := f.updater().Update(dir, &out); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "blocklist.dat"))
	if err != nil {
		t.Fatalf("read blocklist: %v", err)
	}
	if !bytes.Equal(got, f.blocklist) {
		t.Errorf("blocklist.dat = %d bytes, want %d", len(got), len(f.blocklist))
	}

	var wrote Metadata
	raw, err := os.ReadFile(filepath.Join(dir, "badkeysdata.json"))
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	if err := json.Unmarshal(raw, &wrote); err != nil {
		t.Fatalf("parse written metadata: %v", err)
	}
	if wrote.BlocklistSHA256 != f.meta.BlocklistSHA256 {
		t.Errorf("stored sha = %q, want %q", wrote.BlocklistSHA256, f.meta.BlocklistSHA256)
	}

	if !strings.Contains(out.String(), "SHA-256 verified.") {
		t.Errorf("expected the run to report verification, got %q", out.String())
	}
	if !strings.Contains(out.String(), "Blocklist contains 3 entries") {
		t.Errorf("expected the entry count, got %q", out.String())
	}
}

// A second update over a matching blocklist refreshes metadata only.
func TestUpdateSkipsDownloadWhenBlocklistMatches(t *testing.T) {
	f := newUpdateFixture(t, 2)
	dir := filepath.Join(t.TempDir(), "badkeys")

	if err := f.updater().Update(dir, &bytes.Buffer{}); err != nil {
		t.Fatalf("first update: %v", err)
	}

	var out bytes.Buffer
	if err := f.updater().Update(dir, &out); err != nil {
		t.Fatalf("second update: %v", err)
	}

	if !strings.Contains(out.String(), "already up to date") {
		t.Errorf("expected the second run to skip the download, got %q", out.String())
	}
	if f.metaHits != 2 {
		t.Errorf("metadata fetched %d times, want 2", f.metaHits)
	}
}

func TestUpdateRejectsShaMismatch(t *testing.T) {
	f := newUpdateFixture(t, 2)
	f.meta.BlocklistSHA256 = strings.Repeat("00", sha256.Size)

	err := f.updater().Update(filepath.Join(t.TempDir(), "badkeys"), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "SHA-256 mismatch") {
		t.Fatalf("expected a SHA-256 mismatch, got %v", err)
	}
}

// A blocklist that is not a whole number of 16 byte entries is rejected.
func TestUpdateRejectsMisalignedBlocklist(t *testing.T) {
	f := newUpdateFixture(t, 2)
	f.blocklist = append(f.blocklist, 'x')
	sum := sha256.Sum256(f.blocklist)
	f.meta.BlocklistSHA256 = hex.EncodeToString(sum[:])

	err := f.updater().Update(filepath.Join(t.TempDir(), "badkeys"), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "not a multiple of 16 bytes") {
		t.Fatalf("expected a size rejection, got %v", err)
	}
}

func TestUpdateRejectsUnsupportedFormat(t *testing.T) {
	f := newUpdateFixture(t, 1)
	f.meta.BKFormat = bkFormat + 1

	err := f.updater().Update(filepath.Join(t.TempDir(), "badkeys"), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "unsupported bkformat") {
		t.Fatalf("expected an unsupported format error, got %v", err)
	}
}

func TestUpdateRejectsMissingBlocklistURL(t *testing.T) {
	f := newUpdateFixture(t, 1)
	f.meta.BlocklistURL = ""

	err := f.updater().Update(filepath.Join(t.TempDir(), "badkeys"), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "blocklist_url not present") {
		t.Fatalf("expected a missing url error, got %v", err)
	}
}

func TestUpdateReportsHTTPError(t *testing.T) {
	f := newUpdateFixture(t, 1)
	f.metaStatus = http.StatusServiceUnavailable

	err := f.updater().Update(filepath.Join(t.TempDir(), "badkeys"), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("HTTP %d", http.StatusServiceUnavailable)) {
		t.Fatalf("expected the status in the error, got %v", err)
	}
}

func TestUpdateRejectsUnparsableMetadata(t *testing.T) {
	f := newUpdateFixture(t, 1)
	f.metaJSON = []byte("{not json")

	err := f.updater().Update(filepath.Join(t.TempDir(), "badkeys"), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "parse badkeysdata.json") {
		t.Fatalf("expected a parse error, got %v", err)
	}
}

// Upstream marks the format deprecated with a warning, not an error.
func TestUpdateWarnsOnDeprecatedFormat(t *testing.T) {
	f := newUpdateFixture(t, 1)
	f.meta.Deprecated = "use v1"

	var out bytes.Buffer
	if err := f.updater().Update(filepath.Join(t.TempDir(), "badkeys"), &out); err != nil {
		t.Fatalf("update: %v", err)
	}
	if !strings.Contains(out.String(), "deprecated") {
		t.Errorf("expected a deprecation warning, got %q", out.String())
	}
}

// The zero value targets the upstream endpoint without querying it.
func TestUpdaterZeroValueUsesUpstream(t *testing.T) {
	if got := (Updater{}).metadataURL(); got != UpdateURL {
		t.Errorf("metadataURL = %q, want %q", got, UpdateURL)
	}
	if got := (Updater{}).client(); got != http.DefaultClient {
		t.Errorf("expected the default HTTP client")
	}
}
