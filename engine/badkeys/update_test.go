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

// updateFixture serves badkeysdata.json and the compressed blocklist it points at.
type updateFixture struct {
	srv       *httptest.Server
	blocklist []byte
	meta      Metadata
	// metaJSON overrides the served metadata when non-empty.
	metaJSON []byte
	// metaStatus overrides the metadata response status.
	metaStatus int
	metaHits   int
	// blocklistBody overrides the served blocklist bytes when non-nil.
	blocklistBody []byte
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
		if f.blocklistBody != nil {
			_, _ = w.Write(f.blocklistBody)
			return
		}
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

func TestUpdateErrors(t *testing.T) {
	tests := []struct {
		name    string
		entries int
		mutate  func(t *testing.T, f *updateFixture, dir string)
		wantErr string
	}{
		{
			name:    "sha mismatch",
			entries: 2,
			mutate: func(_ *testing.T, f *updateFixture, _ string) {
				f.meta.BlocklistSHA256 = strings.Repeat("00", sha256.Size)
			},
			wantErr: "SHA-256 mismatch",
		},
		{
			// A blocklist that is not a whole number of 16 byte entries is rejected.
			name:    "misaligned blocklist",
			entries: 2,
			mutate: func(_ *testing.T, f *updateFixture, _ string) {
				f.blocklist = append(f.blocklist, 'x')
				sum := sha256.Sum256(f.blocklist)
				f.meta.BlocklistSHA256 = hex.EncodeToString(sum[:])
			},
			wantErr: "not a multiple of 16 bytes",
		},
		{
			name:    "unsupported format",
			entries: 1,
			mutate: func(_ *testing.T, f *updateFixture, _ string) {
				f.meta.BKFormat = bkFormat + 1
			},
			wantErr: "unsupported bkformat",
		},
		{
			name:    "missing blocklist url",
			entries: 1,
			mutate: func(_ *testing.T, f *updateFixture, _ string) {
				f.meta.BlocklistURL = ""
			},
			wantErr: "blocklist_url not present",
		},
		{
			name:    "http error",
			entries: 1,
			mutate: func(_ *testing.T, f *updateFixture, _ string) {
				f.metaStatus = http.StatusServiceUnavailable
			},
			wantErr: fmt.Sprintf("HTTP %d", http.StatusServiceUnavailable),
		},
		{
			name:    "unparsable metadata",
			entries: 1,
			mutate: func(_ *testing.T, f *updateFixture, _ string) {
				f.metaJSON = []byte("{not json")
			},
			wantErr: "parse badkeysdata.json",
		},
		{
			name:    "blocklist download failure",
			entries: 1,
			mutate: func(_ *testing.T, f *updateFixture, _ string) {
				f.meta.BlocklistURL = f.srv.URL + "/absent"
			},
			wantErr: "download blocklist",
		},
		{
			name:    "non xz blocklist",
			entries: 1,
			mutate: func(_ *testing.T, f *updateFixture, _ string) {
				f.blocklistBody = []byte("this is not compressed")
			},
			wantErr: "xz reader",
		},
		{
			name:    "truncated blocklist",
			entries: 64,
			mutate: func(t *testing.T, f *updateFixture, _ string) {
				full := xzCompress(t, f.blocklist)
				f.blocklistBody = full[:len(full)-16]
			},
			wantErr: "xz decompress",
		},
		{
			// A directory on the .tmp staging path blocks the blocklist write.
			name:    "blocklist write failure",
			entries: 1,
			mutate: func(t *testing.T, _ *updateFixture, dir string) {
				if err := os.MkdirAll(filepath.Join(dir, "blocklist.dat.tmp"), 0o755); err != nil {
					t.Fatalf("mkdir blocker: %v", err)
				}
			},
			wantErr: "write blocklist.dat",
		},
		{
			name:    "metadata write failure",
			entries: 1,
			mutate: func(t *testing.T, _ *updateFixture, dir string) {
				if err := os.MkdirAll(filepath.Join(dir, "badkeysdata.json.tmp"), 0o755); err != nil {
					t.Fatalf("mkdir blocker: %v", err)
				}
			},
			wantErr: "write badkeysdata.json",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newUpdateFixture(t, tc.entries)
			dir := filepath.Join(t.TempDir(), "badkeys")
			tc.mutate(t, f, dir)

			err := f.updater().Update(dir, &bytes.Buffer{})
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected %q in the error, got %v", tc.wantErr, err)
			}
		})
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

// An unusable output path fails before any fetch.
func TestUpdateRejectsUnusableOutputDir(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, nil, 0o644); err != nil {
		t.Fatalf("write blocker: %v", err)
	}

	err := Update(filepath.Join(blocked, "badkeys"), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "create output dir") {
		t.Fatalf("expected an output dir error, got %v", err)
	}
}

func TestUpdateReportsUnreachableEndpoint(t *testing.T) {
	u := Updater{URL: "http://127.0.0.1:1/badkeysdata.json"}

	err := u.Update(filepath.Join(t.TempDir(), "badkeys"), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "download badkeysdata.json") {
		t.Fatalf("expected a download error, got %v", err)
	}
}

// An unreadable blocklist.dat counts as absent, so the update redownloads.
func TestUpdateTreatsUnreadableBlocklistAsAbsent(t *testing.T) {
	f := newUpdateFixture(t, 1)
	dir := filepath.Join(t.TempDir(), "badkeys")
	if err := os.MkdirAll(filepath.Join(dir, "blocklist.dat"), 0o755); err != nil {
		t.Fatalf("mkdir blocker: %v", err)
	}

	var out bytes.Buffer
	err := f.updater().Update(dir, &out)
	if err == nil || !strings.Contains(err.Error(), "write blocklist.dat") {
		t.Fatalf("expected the write to fail, got %v", err)
	}
	if strings.Contains(out.String(), "already up to date") {
		t.Error("an unreadable blocklist must not count as up to date")
	}
}

func TestDefaultDataDir(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"xdg data home", map[string]string{"XDG_DATA_HOME": "/xdg"}, filepath.Join("/xdg", "gonemaster", "badkeys")},
		{"home without xdg", map[string]string{"XDG_DATA_HOME": "", "HOME": "/home/tester"}, filepath.Join("/home/tester", ".local", "share", "gonemaster", "badkeys")},
		// With no home to resolve the path stays relative rather than empty.
		{"no home", map[string]string{"XDG_DATA_HOME": "", "HOME": ""}, filepath.Join(".", ".local", "share", "gonemaster", "badkeys")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for key, value := range tc.env {
				t.Setenv(key, value)
			}
			if got := DefaultDataDir(); got != tc.want {
				t.Errorf("DefaultDataDir() = %q, want %q", got, tc.want)
			}
		})
	}
}

// Off Linux and macOS the data sits under the home dot directory and XDG_DATA_HOME is ignored.
func TestDataDirOutsideUnixIgnoresXDG(t *testing.T) {
	want := filepath.Join("/home/tester", ".gonemaster", "badkeys")
	if got := dataDir("windows", "/home/tester", "/xdg"); got != want {
		t.Errorf("dataDir(windows) = %q, want %q", got, want)
	}
}
