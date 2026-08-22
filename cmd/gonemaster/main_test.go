package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/cmd/internal/clitest"
	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/cachefile"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/profile"
)

func stubRunEngine(t *testing.T, captured *engine.RunRequest) {
	t.Helper()
	previous := runEngine
	runEngine = func(req engine.RunRequest) ([]engine.LogEntry, error) {
		if captured != nil {
			*captured = req
		}
		return nil, nil
	}
	t.Cleanup(func() {
		runEngine = previous
	})
}

func samplePacketCacheFile(t *testing.T) cachefile.File {
	t.Helper()

	msg := new(dns.Msg)
	dnsutil.SetQuestion(msg, "example.com.", dns.TypeA)
	msg.Response = true
	if err := msg.Pack(); err != nil {
		t.Fatalf("pack sample dns msg: %v", err)
	}
	wire := msg.Data

	return cachefile.File{
		Format:  cachefile.Format,
		Version: cachefile.Version,
		Entries: []cachefile.Entry{
			{
				Kind:       cachefile.KindNameserver,
				Address:    "192.0.2.53",
				Key:        "fixture.key",
				Message:    base64.StdEncoding.EncodeToString(wire),
				AnswerFrom: "192.0.2.53:53",
			},
		},
	}
}

func TestRunRequiresDomain(t *testing.T) {
	res := clitest.Run(t, run)
	res.RequireCode(t, 2)
	res.RequireErrContains(t, "--domain is required")
}

func TestRunHelpShowsGroupedFlags(t *testing.T) {
	res := clitest.Run(t, run, "-h")
	res.RequireCode(t, 2)
	help := res.Err
	expected := []string{
		"Usage: gonemaster [flags] [DOMAIN]",
		"Flags:",
		"Target:",
		"Output:",
		"Cache:",
		"Resolver/Profile Overrides:",
		"Undelegated:",
		"Utility:",
		"DOMAIN",
		"--domain DOMAIN",
		"--stop-level LEVEL",
		"--count",
		"--save PATH",
		"--save-compress",
		"--save-max-entries N",
		"--sourceaddr4 IPADDR",
		"--sourceaddr6 IPADDR",
	}
	for _, fragment := range expected {
		if !strings.Contains(help, fragment) {
			t.Fatalf("expected %q in help output, got %q", fragment, help)
		}
	}
	if len(res.Out) != 0 {
		t.Fatalf("expected no stdout output, got %q", res.Out)
	}
}

func TestRunAcceptsPositionalDomain(t *testing.T) {
	var captured engine.RunRequest
	stubRunEngine(t, &captured)

	res := clitest.Run(t, run, "example.com")
	res.RequireCode(t, 0)
	if captured.Domain != "example.com" {
		t.Fatalf("expected positional domain example.com, got %q", captured.Domain)
	}
}

func TestRunAcceptsPositionalDomainWithFlags(t *testing.T) {
	var captured engine.RunRequest
	stubRunEngine(t, &captured)

	res := clitest.Run(t, run, "--min-level", "INFO", "--json", "example.com")
	res.RequireCode(t, 0)
	if captured.Domain != "example.com" {
		t.Fatalf("expected positional domain example.com, got %q", captured.Domain)
	}
	if captured.MinLevel != "INFO" {
		t.Fatalf("expected min-level INFO, got %q", captured.MinLevel)
	}
}

func TestRunRejectsMultiplePositionalDomains(t *testing.T) {
	stubRunEngine(t, nil)

	res := clitest.Run(t, run, "example.com", "example.net")
	res.RequireCode(t, 2)
	res.RequireErrContains(t, "only one positional DOMAIN argument is allowed")
}

func TestRunRejectsDomainProvidedTwice(t *testing.T) {
	stubRunEngine(t, nil)

	res := clitest.Run(t, run, "--domain", "example.com", "example.net")
	res.RequireCode(t, 2)
	res.RequireErrContains(t, "domain provided twice; use either --domain DOMAIN or positional DOMAIN")
}

func TestRunRejectsInvalidStopLevel(t *testing.T) {
	stubRunEngine(t, nil)

	res := clitest.Run(t, run, "--domain", "example.com", "--stop-level", "BANANA")
	res.RequireCode(t, 2)
	res.RequireErrContains(t, "--stop-level must be one of")
}

func TestRunStopLevelTreatsContextCanceledAsSuccessForJSON(t *testing.T) {
	previous := runEngine
	runEngine = func(req engine.RunRequest) ([]engine.LogEntry, error) {
		if req.LogCallback == nil {
			t.Fatalf("expected log callback")
		}
		if req.Context == nil {
			t.Fatalf("expected context when stop-level is configured")
		}

		p := profile.New()
		if err := p.Set("test_levels", map[string]map[string]string{
			"SYSTEM": {
				"STOP_INFO": "INFO",
				"STOP_WARN": "WARNING",
			},
		}); err != nil {
			t.Fatalf("set test_levels: %v", err)
		}
		log := logger.New()
		log.SetProfile(p)
		infoEntry, err := log.Add("STOP_INFO", map[string]any{"phase": "before"}, "System", "Unspecified")
		if err != nil {
			t.Fatalf("add stop info: %v", err)
		}
		if cbErr := req.LogCallback(infoEntry); cbErr != nil {
			t.Fatalf("callback stop info: %v", cbErr)
		}
		warnEntry, err := log.Add("STOP_WARN", map[string]any{"phase": "stop"}, "System", "Unspecified")
		if err != nil {
			t.Fatalf("add stop warn: %v", err)
		}
		if cbErr := req.LogCallback(warnEntry); cbErr != nil {
			t.Fatalf("callback stop warn: %v", cbErr)
		}
		return nil, context.Canceled
	}
	t.Cleanup(func() {
		runEngine = previous
	})

	res := clitest.Run(t, run, "--domain", "example.com", "--json", "--min-level", "INFO", "--stop-level", "WARNING")
	res.RequireCode(t, 0)
	var payload []map[string]any
	if err := json.Unmarshal([]byte(res.Out), &payload); err != nil {
		t.Fatalf("expected JSON output, got %q (err=%v)", res.Out, err)
	}
	if len(payload) != 2 {
		t.Fatalf("expected 2 captured entries, got %d (%q)", len(payload), res.Out)
	}
	if payload[0]["tag"] != "STOP_INFO" {
		t.Fatalf("unexpected first tag: %v", payload[0]["tag"])
	}
	if payload[1]["tag"] != "STOP_WARN" {
		t.Fatalf("unexpected second tag: %v", payload[1]["tag"])
	}
	if strings.TrimSpace(res.Err) != "" {
		t.Fatalf("expected no stderr output, got %q", res.Err)
	}
}

func TestRunRejectsSaveAndVersion(t *testing.T) {
	res := clitest.Run(t, run, "--version", "--save", "cache.json")
	res.RequireCode(t, 2)
	res.RequireErrContains(t, "--save/--restore cannot be combined with --version")
}

func TestRunRejectsRestoreAndListTests(t *testing.T) {
	res := clitest.Run(t, run, "--list-tests", "--restore", "cache.json")
	res.RequireCode(t, 2)
	res.RequireErrContains(t, "--save/--restore cannot be combined with --list-tests")
}

func TestRunRejectsSaveAndDumpProfile(t *testing.T) {
	res := clitest.Run(t, run, "--save", "cache.json", "--dump-profile")
	res.RequireCode(t, 2)
	res.RequireErrContains(t, "--dump-profile cannot be combined with --save")
}

func TestRunRejectsRestoreAndDumpProfile(t *testing.T) {
	res := clitest.Run(t, run, "--restore", "cache.json", "--dump-profile")
	res.RequireCode(t, 2)
	res.RequireErrContains(t, "--dump-profile cannot be combined with --restore")
}

func TestRunDumpProfileWithoutDomain(t *testing.T) {
	res := clitest.Run(t, run, "--dump-profile")
	res.RequireCode(t, 0)
	var payload map[string]any
	if err := json.Unmarshal([]byte(res.Out), &payload); err != nil {
		t.Fatalf("expected JSON output, got %q (err=%v)", res.Out, err)
	}
	if _, ok := payload["net"]; !ok {
		t.Fatalf("expected net in profile output, got %q", res.Out)
	}
	if len(res.Err) != 0 {
		t.Fatalf("expected no stderr output, got %q", res.Err)
	}
}

func TestRunWritesJSONAndError(t *testing.T) {
	res := clitest.Run(t, run, "--domain", ".", "--testcase", "basic01", "--min-level", "INFO", "--json")
	res.RequireCode(t, 0)
	res.RequireOutContains(t, "B01_ROOT_HAS_NO_PARENT")
	if len(res.Err) != 0 {
		t.Fatalf("expected no stderr output, got %q", res.Err)
	}
}

func TestRunRestorePacketCacheLoadsRequestCache(t *testing.T) {
	dir := t.TempDir()
	restorePath := filepath.Join(dir, "restore-cache.json")
	fixture := samplePacketCacheFile(t)
	data, err := json.Marshal(fixture)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	if err := os.WriteFile(restorePath, data, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	previous := runEngine
	runEngine = func(req engine.RunRequest) ([]engine.LogEntry, error) {
		if req.NameserverCache == nil {
			t.Fatalf("expected nameserver cache in request")
		}
		exported, exportErr := req.NameserverCache.ExportEntries()
		if exportErr != nil {
			t.Fatalf("export request cache: %v", exportErr)
		}
		if len(exported) != 1 {
			t.Fatalf("expected 1 restored cache entry, got %d", len(exported))
		}
		if exported[0].Address != "192.0.2.53" || exported[0].Key != "fixture.key" {
			t.Fatalf("unexpected restored entry: %+v", exported[0])
		}
		return nil, nil
	}
	t.Cleanup(func() {
		runEngine = previous
	})

	res := clitest.Run(t, run, "--domain", "example.com", "--json", "--restore", restorePath)
	res.RequireCode(t, 0)
	if len(res.Err) != 0 {
		t.Fatalf("expected no stderr output, got %q", res.Err)
	}
}

func TestRunSavePacketCacheWritesFile(t *testing.T) {
	dir := t.TempDir()
	savePath := filepath.Join(dir, "saved-cache.json")
	fixture := samplePacketCacheFile(t)

	previous := runEngine
	runEngine = func(req engine.RunRequest) ([]engine.LogEntry, error) {
		if req.NameserverCache == nil {
			t.Fatalf("expected nameserver cache in request")
		}
		if importErr := cachefile.Import(fixture, req.NameserverCache, req.Recursor, req.ASNCache); importErr != nil {
			t.Fatalf("import fixture into run cache: %v", importErr)
		}
		return nil, nil
	}
	t.Cleanup(func() {
		runEngine = previous
	})

	res := clitest.Run(t, run, "--domain", "example.com", "--json", "--save", savePath)
	res.RequireCode(t, 0)

	payloadBytes, err := os.ReadFile(savePath)
	if err != nil {
		t.Fatalf("read saved cache file: %v", err)
	}
	var payload cachefile.File
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		t.Fatalf("unmarshal saved cache: %v", err)
	}
	if payload.Format != cachefile.Format || payload.Version != cachefile.Version {
		t.Fatalf("unexpected saved cache header: %+v", payload)
	}
	if len(payload.Entries) != 1 {
		t.Fatalf("expected 1 saved entry, got %d", len(payload.Entries))
	}
	if payload.Entries[0].Kind != cachefile.KindNameserver || payload.Entries[0].Key != "fixture.key" {
		t.Fatalf("unexpected saved entry: %+v", payload.Entries[0])
	}
}

func TestRunSaveCompressWritesGzip(t *testing.T) {
	dir := t.TempDir()
	savePath := filepath.Join(dir, "saved-cache.json")
	fixture := samplePacketCacheFile(t)

	previous := runEngine
	runEngine = func(req engine.RunRequest) ([]engine.LogEntry, error) {
		if importErr := cachefile.Import(fixture, req.NameserverCache, req.Recursor, req.ASNCache); importErr != nil {
			t.Fatalf("import fixture: %v", importErr)
		}
		return nil, nil
	}
	t.Cleanup(func() {
		runEngine = previous
	})

	res := clitest.Run(t, run, "--domain", "example.com", "--json", "--save", savePath, "--save-compress")
	res.RequireCode(t, 0)

	data, err := os.ReadFile(savePath)
	if err != nil {
		t.Fatalf("read saved cache file: %v", err)
	}
	if len(data) < 2 || data[0] != 0x1f || data[1] != 0x8b {
		t.Fatalf("expected gzip magic in saved file with --save-compress, got first bytes %x", data[:min(len(data), 8)])
	}

	// And restoring through the CLI must work on the same file.
	restorePrev := runEngine
	runEngine = func(req engine.RunRequest) ([]engine.LogEntry, error) {
		if req.NameserverCache == nil {
			t.Fatalf("expected nameserver cache in request")
		}
		entries, exportErr := req.NameserverCache.ExportEntries()
		if exportErr != nil {
			t.Fatalf("export cache: %v", exportErr)
		}
		if len(entries) != 1 || entries[0].Key != "fixture.key" {
			t.Fatalf("expected 1 restored entry with key fixture.key, got %+v", entries)
		}
		return nil, nil
	}
	t.Cleanup(func() { runEngine = restorePrev })

	clitest.Run(t, run, "--domain", "example.com", "--json", "--restore", savePath).RequireCode(t, 0)
}

func TestRunSaveCompressRejectsWithoutSave(t *testing.T) {
	stubRunEngine(t, nil)

	res := clitest.Run(t, run, "--domain", "example.com", "--save-compress")
	res.RequireCode(t, 2)
	res.RequireErrContains(t, "--save-compress requires --save")
}

func TestRunSaveMaxEntriesUnderLimitSucceeds(t *testing.T) {
	dir := t.TempDir()
	savePath := filepath.Join(dir, "saved-cache.json")
	fixture := samplePacketCacheFile(t)

	previous := runEngine
	runEngine = func(req engine.RunRequest) ([]engine.LogEntry, error) {
		if importErr := cachefile.Import(fixture, req.NameserverCache, req.Recursor, req.ASNCache); importErr != nil {
			t.Fatalf("import fixture: %v", importErr)
		}
		return nil, nil
	}
	t.Cleanup(func() { runEngine = previous })

	res := clitest.Run(t, run, "--domain", "example.com", "--json", "--save", savePath, "--save-max-entries", "10")
	res.RequireCode(t, 0)
	if _, err := os.Stat(savePath); err != nil {
		t.Fatalf("expected save file to exist when under limit: %v", err)
	}
}

func TestRunSaveMaxEntriesZeroMeansUnlimited(t *testing.T) {
	dir := t.TempDir()
	savePath := filepath.Join(dir, "saved-cache.json")
	fixture := samplePacketCacheFile(t)

	original := runEngine
	runEngine = func(req engine.RunRequest) ([]engine.LogEntry, error) {
		if importErr := cachefile.Import(fixture, req.NameserverCache, req.Recursor, req.ASNCache); importErr != nil {
			t.Fatalf("import fixture: %v", importErr)
		}
		return nil, nil
	}
	t.Cleanup(func() { runEngine = original })

	res := clitest.Run(t, run, "--domain", "example.com", "--json", "--save", savePath, "--save-max-entries", "0")
	res.RequireCode(t, 0)
}

func TestRunSaveMaxEntriesOverLimitFails(t *testing.T) {
	dir := t.TempDir()
	savePath := filepath.Join(dir, "saved-cache.json")
	fixture := samplePacketCacheFile(t)

	original := runEngine
	runEngine = func(req engine.RunRequest) ([]engine.LogEntry, error) {
		twoEntryFixture := fixture
		extra := fixture.Entries[0]
		extra.Key = "fixture.key.2"
		twoEntryFixture.Entries = append(twoEntryFixture.Entries, extra)
		if importErr := cachefile.Import(twoEntryFixture, req.NameserverCache, req.Recursor, req.ASNCache); importErr != nil {
			t.Fatalf("import fixture: %v", importErr)
		}
		return nil, nil
	}
	t.Cleanup(func() { runEngine = original })

	res := clitest.Run(t, run, "--domain", "example.com", "--json", "--save", savePath, "--save-max-entries", "1")
	if res.Code == 0 {
		t.Fatalf("expected non-zero exit when entry count exceeds --save-max-entries")
	}
	res.RequireErrContains(t, "max-entries=1")
	if _, err := os.Stat(savePath); err == nil {
		t.Fatalf("expected save file NOT to be written when over limit")
	}
}

func TestRunSaveMaxEntriesRejectsWithoutSave(t *testing.T) {
	stubRunEngine(t, nil)

	res := clitest.Run(t, run, "--domain", "example.com", "--save-max-entries", "5")
	res.RequireCode(t, 2)
	res.RequireErrContains(t, "--save-max-entries requires --save")
}

func TestRunSaveMaxEntriesRejectsNegative(t *testing.T) {
	stubRunEngine(t, nil)

	clitest.Run(t, run, "--domain", "example.com", "--save", "out.json", "--save-max-entries", "-1").
		RequireCode(t, 2).
		RequireErrContains(t, "--save-max-entries must be >= 0")
}

// writeSavedCache writes a real cache file (with checksum) holding 3
// nameserver entries across 2 addresses. Returns the path.
func writeSavedCache(t *testing.T, dir, name string, compress bool) string {
	t.Helper()
	msg := new(dns.Msg)
	dnsutil.SetQuestion(msg, "example.com.", dns.TypeA)
	msg.Response = true
	if err := msg.Pack(); err != nil {
		t.Fatalf("pack: %v", err)
	}
	ns := nameserver.NewCacheStore()
	if err := ns.ImportEntries([]nameserver.Entry{
		{Address: "192.0.2.1", Key: "k1", Message: msg.Data, AnswerFrom: "192.0.2.1:53"},
		{Address: "192.0.2.1", Key: "k2", NoMessage: true},
		{Address: "192.0.2.2", Key: "k3", Message: msg.Data},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	path := filepath.Join(dir, name)
	var opts []cachefile.SaveOption
	if compress {
		opts = append(opts, cachefile.WithCompression())
	}
	if err := cachefile.Save(path, ns, nil, nil, opts...); err != nil {
		t.Fatalf("save: %v", err)
	}
	return path
}

func TestRunCacheStatsPrintsReport(t *testing.T) {
	stubRunEngine(t, nil)
	path := writeSavedCache(t, t.TempDir(), "cache.json", false)

	res := clitest.Run(t, run, "--cache-stats", path)
	res.RequireCode(t, 0)
	got := res.Out
	for _, want := range []string{"entries:  3 total", "nameserver  3", "by address (nameserver):", "192.0.2.1", "192.0.2.2", "plain"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in output, got %q", want, got)
		}
	}
}

func TestRunCacheStatsGzipReportsCompression(t *testing.T) {
	stubRunEngine(t, nil)
	path := writeSavedCache(t, t.TempDir(), "cache.json.gz", true)

	res := clitest.Run(t, run, "--cache-stats", path)
	res.RequireCode(t, 0)
	res.RequireOutContains(t, "gzip")
}

func TestRunCacheStatsStrictRejectsUnknownField(t *testing.T) {
	stubRunEngine(t, nil)
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.json")
	blob := []byte(`{"format":"gonemaster.packet-cache","version":2,"mystery":1,"entries":[]}`)
	if err := os.WriteFile(path, blob, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	res := clitest.Run(t, run, "--cache-stats", path, "--cache-strict")
	res.RequireCode(t, 2)
	res.RequireErrContains(t, "unknown field")
}

func TestRunRestoreStrictRejectsUnknownField(t *testing.T) {
	stubRunEngine(t, nil)
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.json")
	blob := []byte(`{"format":"gonemaster.packet-cache","version":2,"mystery":1,"entries":[]}`)
	if err := os.WriteFile(path, blob, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	res := clitest.Run(t, run, "--domain", "example.com", "--json", "--restore", path, "--cache-strict")
	res.RequireCode(t, 2)
	res.RequireErrContains(t, "unknown field")
}

func TestRunCacheStrictRequiresRestoreOrStats(t *testing.T) {
	stubRunEngine(t, nil)
	res := clitest.Run(t, run, "--domain", "example.com", "--cache-strict")
	res.RequireCode(t, 2)
	res.RequireErrContains(t, "--cache-strict requires --restore or --cache-stats")
}

func TestRunCacheStatsRejectsWithSave(t *testing.T) {
	stubRunEngine(t, nil)
	res := clitest.Run(t, run, "--cache-stats", "a.json", "--save", "b.json")
	res.RequireCode(t, 2)
	res.RequireErrContains(t, "--cache-stats cannot be combined with --save/--restore")
}

func TestRunRestorePrintsCacheSummary(t *testing.T) {
	stubRunEngine(t, nil)
	path := writeSavedCache(t, t.TempDir(), "cache.json", false)

	res := clitest.Run(t, run, "--domain", "example.com", "--restore", path)
	res.RequireCode(t, 0)
	res.RequireOutContains(t, "packet cache: 0 hits, 0 misses")
}

func TestRunWritesOutputFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out.json")

	clitest.Run(t, run, "--domain", ".", "--testcase", "basic01", "--min-level", "INFO", "--json", "--output", target).
		RequireCode(t, 0)
	clitest.FileContains(t, target, "B01_ROOT_HAS_NO_PARENT")
}

func TestRunRawStreamsOutput(t *testing.T) {
	res := clitest.Run(t, run, "--domain", ".", "--testcase", "basic01", "--min-level", "INFO", "--raw")
	res.RequireCode(t, 0)
	res.RequireOutContains(t, "B01_ROOT_HAS_NO_PARENT")
	if strings.Contains(res.Out, "\"timestamp\"") {
		t.Fatalf("expected raw output, got %q", res.Out)
	}
	if len(res.Err) != 0 {
		t.Fatalf("expected no stderr output, got %q", res.Err)
	}
}

func TestRunJSONStreamOutputs(t *testing.T) {
	res := clitest.Run(t, run, "--domain", ".", "--testcase", "basic01", "--min-level", "INFO", "--json-stream")
	res.RequireCode(t, 0)
	lines := strings.Split(res.Out, "\n")
	var first string
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		first = line
		break
	}
	if first == "" {
		t.Fatalf("expected json-stream output, got %q", res.Out)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(first), &payload); err != nil {
		t.Fatalf("expected JSON line, got %q (err=%v)", first, err)
	}
	if tag, ok := payload["Tag"].(string); !ok || !strings.Contains(tag, "B01_") {
		t.Fatalf("expected Tag in JSON output, got %v", payload["Tag"])
	}
	if len(res.Err) != 0 {
		t.Fatalf("expected no stderr output, got %q", res.Err)
	}
}

func TestRunRejectsRawAndJSON(t *testing.T) {
	res := clitest.Run(t, run, "--domain", ".", "--json", "--raw")
	res.RequireCode(t, 2)
	res.RequireErrContains(t, "--json cannot be combined with --raw")
	if len(res.Out) != 0 {
		t.Fatalf("expected no stdout output, got %q", res.Out)
	}
}

func TestRunRejectsJSONStreamAndRaw(t *testing.T) {
	res := clitest.Run(t, run, "--domain", ".", "--json-stream", "--raw")
	res.RequireCode(t, 2)
	res.RequireErrContains(t, "--json-stream cannot be combined with --raw")
	if len(res.Out) != 0 {
		t.Fatalf("expected no stdout output, got %q", res.Out)
	}
}

func TestRunRejectsJSONStreamAndJSON(t *testing.T) {
	res := clitest.Run(t, run, "--domain", ".", "--json-stream", "--json")
	res.RequireCode(t, 2)
	res.RequireErrContains(t, "--json-stream cannot be combined with --json")
	if len(res.Out) != 0 {
		t.Fatalf("expected no stdout output, got %q", res.Out)
	}
}

func TestRunRejectsCountAndRaw(t *testing.T) {
	res := clitest.Run(t, run, "--domain", ".", "--count", "--raw")
	res.RequireCode(t, 2)
	res.RequireErrContains(t, "--count cannot be combined with --raw")
	if len(res.Out) != 0 {
		t.Fatalf("expected no stdout output, got %q", res.Out)
	}
}

func TestRunRejectsCountAndJSON(t *testing.T) {
	res := clitest.Run(t, run, "--domain", ".", "--count", "--json")
	res.RequireCode(t, 2)
	res.RequireErrContains(t, "--count cannot be combined with --json")
	if len(res.Out) != 0 {
		t.Fatalf("expected no stdout output, got %q", res.Out)
	}
}

func TestRunRejectsCountAndJSONStream(t *testing.T) {
	res := clitest.Run(t, run, "--domain", ".", "--count", "--json-stream")
	res.RequireCode(t, 2)
	res.RequireErrContains(t, "--count cannot be combined with --json-stream")
	if len(res.Out) != 0 {
		t.Fatalf("expected no stdout output, got %q", res.Out)
	}
}

func TestRunRejectsCountAndDumpProfile(t *testing.T) {
	res := clitest.Run(t, run, "--count", "--dump-profile")
	res.RequireCode(t, 2)
	res.RequireErrContains(t, "--dump-profile cannot be combined with --count")
	if len(res.Out) != 0 {
		t.Fatalf("expected no stdout output, got %q", res.Out)
	}
}

func TestRunParsesUndelegatedNameserverFlags(t *testing.T) {
	var captured engine.RunRequest
	stubRunEngine(t, &captured)

	res := clitest.Run(t, run, "--domain", "example.com", "--json", "--ns", "NS1.Example.com/192.0.2.1", "--ns", "ns2.example.net")
	res.RequireCode(t, 0)
	if len(captured.UndelegatedNameservers) != 2 {
		t.Fatalf("expected 2 nameserver rows, got %d", len(captured.UndelegatedNameservers))
	}
	first := captured.UndelegatedNameservers[0]
	second := captured.UndelegatedNameservers[1]
	if first.Name != "ns1.example.com" || first.IP != "192.0.2.1" {
		t.Fatalf("unexpected first nameserver: %+v", first)
	}
	if second.Name != "ns2.example.net" || second.IP != "" {
		t.Fatalf("unexpected second nameserver: %+v", second)
	}
}

func TestRunParsesUndelegatedNameserverSameNameMultipleIPs(t *testing.T) {
	var captured engine.RunRequest
	stubRunEngine(t, &captured)

	res := clitest.Run(t, run, "--domain", "example.com", "--json", "--ns", "NS1.Example.com/192.0.2.1", "--ns", "ns1.example.com/2001:db8::1")
	res.RequireCode(t, 0)
	if len(captured.UndelegatedNameservers) != 2 {
		t.Fatalf("expected 2 nameserver rows, got %d", len(captured.UndelegatedNameservers))
	}
	first := captured.UndelegatedNameservers[0]
	second := captured.UndelegatedNameservers[1]
	if first.Name != "ns1.example.com" || first.IP != "192.0.2.1" {
		t.Fatalf("unexpected first nameserver: %+v", first)
	}
	if second.Name != "ns1.example.com" || second.IP != "2001:db8::1" {
		t.Fatalf("unexpected second nameserver: %+v", second)
	}
}

func TestRunParsesUndelegatedDSFlags(t *testing.T) {
	var captured engine.RunRequest
	stubRunEngine(t, &captured)

	res := clitest.Run(t, run, "--domain", "example.com", "--json", "--ds", "12345,13,2,"+strings.Repeat("a", 64), "--ds", "23456,8,2,"+strings.Repeat("B", 64))
	res.RequireCode(t, 0)
	if len(captured.UndelegatedDSInfo) != 2 {
		t.Fatalf("expected 2 DS rows, got %d", len(captured.UndelegatedDSInfo))
	}
	first := captured.UndelegatedDSInfo[0]
	second := captured.UndelegatedDSInfo[1]
	if first.KeyTag != 12345 || first.Algorithm != 13 || first.DigestType != 2 || first.Digest != strings.Repeat("A", 64) {
		t.Fatalf("unexpected first DS row: %+v", first)
	}
	if second.KeyTag != 23456 || second.Algorithm != 8 || second.DigestType != 2 || second.Digest != strings.Repeat("B", 64) {
		t.Fatalf("unexpected second DS row: %+v", second)
	}
}

func TestRunRejectsMalformedUndelegatedNameserverFlag(t *testing.T) {
	stubRunEngine(t, nil)

	res := clitest.Run(t, run, "--domain", "example.com", "--json", "--ns", "bad!name.example/192.0.2.1")
	res.RequireCode(t, 2)
	res.RequireErrContains(t, "undelegated nameserver")
}

func TestRunRejectsMalformedUndelegatedDSFlag(t *testing.T) {
	stubRunEngine(t, nil)

	res := clitest.Run(t, run, "--domain", "example.com", "--json", "--ds", "12345,13,2,NOT-HEX")
	res.RequireCode(t, 2)
	res.RequireErrContains(t, "undelegated DS")
}

func TestRunCarriesUndelegatedInputsInRunRequest(t *testing.T) {
	var captured engine.RunRequest
	stubRunEngine(t, &captured)

	res := clitest.Run(t, run, "--domain", "example.com", "--json", "--ns", "ns1.example.com/192.0.2.10", "--ds", "12345,13,2,"+strings.Repeat("a", 64))
	res.RequireCode(t, 0)
	if len(captured.UndelegatedNameservers) != 1 {
		t.Fatalf("expected one undelegated nameserver row, got %d", len(captured.UndelegatedNameservers))
	}
	if len(captured.UndelegatedDSInfo) != 1 {
		t.Fatalf("expected one undelegated DS row, got %d", len(captured.UndelegatedDSInfo))
	}
	if captured.UndelegatedNameservers[0].Name != "ns1.example.com" || captured.UndelegatedNameservers[0].IP != "192.0.2.10" {
		t.Fatalf("unexpected undelegated nameserver in request: %+v", captured.UndelegatedNameservers[0])
	}
	if captured.UndelegatedDSInfo[0].KeyTag != 12345 {
		t.Fatalf("unexpected undelegated DS in request: %+v", captured.UndelegatedDSInfo[0])
	}
}

func TestRunParsesSourceAddrOverrides(t *testing.T) {
	var captured engine.RunRequest
	stubRunEngine(t, &captured)

	res := clitest.Run(t, run, "--domain", "example.com", "--json", "--sourceaddr4", "192.0.2.44", "--sourceaddr6", "2001:db8::44")
	res.RequireCode(t, 0)
	if captured.SourceAddr4 == nil || *captured.SourceAddr4 != "192.0.2.44" {
		t.Fatalf("unexpected SourceAddr4 override: %#v", captured.SourceAddr4)
	}
	if captured.SourceAddr6 == nil || *captured.SourceAddr6 != "2001:db8::44" {
		t.Fatalf("unexpected SourceAddr6 override: %#v", captured.SourceAddr6)
	}
}

func TestRunRejectsInvalidSourceAddr4(t *testing.T) {
	stubRunEngine(t, nil)

	res := clitest.Run(t, run, "--domain", "example.com", "--json", "--sourceaddr4", "not-an-ip")
	res.RequireCode(t, 2)
	res.RequireErrContains(t, "--sourceaddr4 must be a valid IPv4 address")
}

func TestRunRejectsInvalidSourceAddr6(t *testing.T) {
	stubRunEngine(t, nil)

	res := clitest.Run(t, run, "--domain", "example.com", "--json", "--sourceaddr6", "192.0.2.10")
	res.RequireCode(t, 2)
	res.RequireErrContains(t, "--sourceaddr6 must be a valid IPv6 address")
}

func TestRunOutputsTranslatedByDefault(t *testing.T) {
	res := clitest.Run(t, run, "--domain", ".", "--testcase", "basic01", "--min-level", "INFO", "--locale", "en")
	res.RequireCode(t, 0)
	if !strings.HasPrefix(res.Out, "Seconds Level    Message") {
		t.Fatalf("expected translated header, got %q", res.Out)
	}
	if strings.Contains(res.Out, "\"timestamp\"") {
		t.Fatalf("expected translated output, got %q", res.Out)
	}
	res.RequireOutContains(t, "test of the root zone")
	if len(res.Err) != 0 {
		t.Fatalf("expected no stderr output, got %q", res.Err)
	}
}

func TestRunOutputsLocalizedHeaderByLocale(t *testing.T) {
	stubRunEngine(t, nil)

	res := clitest.Run(t, run, "--domain", "example.com", "--min-level", "CRITICAL", "--locale", "sv")
	res.RequireCode(t, 0)

	firstLine := strings.SplitN(res.Out, "\n", 2)[0]
	if !strings.Contains(firstLine, "Sekunder") || !strings.Contains(firstLine, "Nivå") || !strings.Contains(firstLine, "Meddelande") {
		t.Fatalf("expected localized header for sv locale, got %q", firstLine)
	}
	if strings.Contains(firstLine, "Seconds") {
		t.Fatalf("expected non-English header for sv locale, got %q", firstLine)
	}
	if len(res.Err) != 0 {
		t.Fatalf("expected no stderr output, got %q", res.Err)
	}
}

func TestWriteHumanHeaderAlignsDisplayWidthForJapanese(t *testing.T) {
	var out bytes.Buffer
	if err := writeHumanHeader(&out, "ja"); err != nil {
		t.Fatalf("write header: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 header lines, got %d (%q)", len(lines), out.String())
	}

	headerWidth := terminalCellWidth(lines[0])
	dividerWidth := terminalCellWidth(lines[1])
	if headerWidth != dividerWidth {
		t.Fatalf("expected header/divider display widths to match, got header=%d divider=%d (%q)", headerWidth, dividerWidth, out.String())
	}
	if !strings.Contains(lines[0], "秒") || !strings.Contains(lines[0], "レベル") || !strings.Contains(lines[0], "メッセージ") {
		t.Fatalf("expected Japanese labels in header, got %q", lines[0])
	}
}

func TestRunOutputsLooksOKWhenNoEntriesAtLevel(t *testing.T) {
	res := clitest.Run(t, run, "--domain", ".", "--testcase", "basic01", "--min-level", "CRITICAL", "--locale", "en")
	res.RequireCode(t, 0)
	if !strings.HasPrefix(res.Out, "Seconds Level    Message") {
		t.Fatalf("expected translated header, got %q", res.Out)
	}
	res.RequireOutContains(t, "Looks OK.")
	if len(res.Err) != 0 {
		t.Fatalf("expected no stderr output, got %q", res.Err)
	}
}

func TestRunCountPrintsSummariesFromAllLevels(t *testing.T) {
	res := clitest.Run(t, run, "--domain", ".", "--testcase", "basic01", "--count", "--locale", "en", "--no-progress")
	res.RequireCode(t, 0)
	if !strings.HasPrefix(res.Out, "Seconds Level    Message") {
		t.Fatalf("expected translated header, got %q", res.Out)
	}
	res.RequireOutContains(t, "Looks OK.")
	res.RequireOutContains(t, "Number of log entries")
	res.RequireOutContains(t, "Message tag")
	res.RequireOutContains(t, "INFO")
	res.RequireOutContains(t, "B01_ROOT_HAS_NO_PARENT")
	if len(res.Err) != 0 {
		t.Fatalf("expected no stderr output, got %q", res.Err)
	}
}

func TestRunJSONNoEntriesRemainsJSONArray(t *testing.T) {
	res := clitest.Run(t, run, "--domain", ".", "--testcase", "basic01", "--min-level", "CRITICAL", "--json")
	res.RequireCode(t, 0)
	if strings.Contains(res.Out, "Looks OK.") {
		t.Fatalf("expected JSON output without Looks OK marker, got %q", res.Out)
	}
	if strings.TrimSpace(res.Out) != "[]" {
		t.Fatalf("expected empty JSON array, got %q", res.Out)
	}
	if len(res.Err) != 0 {
		t.Fatalf("expected no stderr output, got %q", res.Err)
	}
}

func TestRunRawNoEntriesRemainsEmpty(t *testing.T) {
	res := clitest.Run(t, run, "--domain", ".", "--testcase", "basic01", "--min-level", "CRITICAL", "--raw")
	res.RequireCode(t, 0)
	if strings.TrimSpace(res.Out) != "" {
		t.Fatalf("expected empty raw stream, got %q", res.Out)
	}
	if len(res.Err) != 0 {
		t.Fatalf("expected no stderr output, got %q", res.Err)
	}
}

func TestRunJSONStreamNoEntriesRemainsEmpty(t *testing.T) {
	res := clitest.Run(t, run, "--domain", ".", "--testcase", "basic01", "--min-level", "CRITICAL", "--json-stream")
	res.RequireCode(t, 0)
	if strings.TrimSpace(res.Out) != "" {
		t.Fatalf("expected empty json-stream output, got %q", res.Out)
	}
	if len(res.Err) != 0 {
		t.Fatalf("expected no stderr output, got %q", res.Err)
	}
}

func TestRunListTests(t *testing.T) {
	res := clitest.Run(t, run, "--list-tests")
	res.RequireCode(t, 0)
	expected := []string{
		"Basic:Basic01",
		"Syntax:Syntax01",
		"Address:Address01",
		"Connectivity:Connectivity01",
		"Consistency:Consistency01",
		"DNSSEC:DNSSEC01",
		"Delegation:Delegation01",
		"Nameserver:Nameserver01",
		"Zone:Zone01",
	}
	res.RequireOutContains(t, expected...)
	if len(res.Err) != 0 {
		t.Fatalf("expected no stderr output, got %q", res.Err)
	}
}

func TestRunVersion(t *testing.T) {
	res := clitest.Run(t, run, "--version")
	res.RequireCode(t, 0)
	res.RequireOutContains(t, "Gonemaster version")
	if !strings.Contains(res.Out, engine.VersionString()) {
		t.Fatalf("expected version output, got %q", res.Out)
	}
	res.RequireOutContains(t, "Miekg DNS version")
	if len(res.Err) != 0 {
		t.Fatalf("expected no stderr output, got %q", res.Err)
	}
}

func TestRunNSTimesCreatesCache(t *testing.T) {
	var captured engine.RunRequest
	stubRunEngine(t, &captured)

	res := clitest.Run(t, run, "--domain", "example.com", "--nstimes")
	res.RequireCode(t, 0)
	if captured.NameserverCache == nil {
		t.Fatal("expected NameserverCache to be created when --nstimes is used")
	}
}

func TestRunNSTimesOutputsTable(t *testing.T) {
	previous := runEngine
	runEngine = func(req engine.RunRequest) ([]engine.LogEntry, error) {
		// Simulate query timing data in the cache the CLI created.
		if req.NameserverCache != nil {
			req.NameserverCache.RecordQueryTime("ns1.example.com/192.0.2.1", 10*time.Millisecond)
			req.NameserverCache.RecordQueryTime("ns1.example.com/192.0.2.1", 20*time.Millisecond)
		}
		return nil, nil
	}
	t.Cleanup(func() { runEngine = previous })

	res := clitest.Run(t, run, "--domain", "example.com", "--nstimes")
	res.RequireCode(t, 0)
	output := res.Out
	if !strings.Contains(output, "Name servers") {
		t.Fatalf("expected nstimes header in output, got %q", output)
	}
	if !strings.Contains(output, "ns1.example.com/192.0.2.1") {
		t.Fatalf("expected nameserver entry in output, got %q", output)
	}
	if !strings.Contains(output, "Grand total") {
		t.Fatalf("expected grand total in output, got %q", output)
	}
}

func TestRunAllowNonGlobalFlag(t *testing.T) {
	// --allow-non-global sets the RunRequest override; absent, it stays nil so
	// the profile/server default governs.
	var captured engine.RunRequest
	stubRunEngine(t, &captured)
	res := clitest.Run(t, run, "--allow-non-global", "example.com")
	res.RequireCode(t, 0)
	if captured.AllowNonGlobalTargets == nil || !*captured.AllowNonGlobalTargets {
		t.Fatalf("expected AllowNonGlobalTargets override true, got %#v", captured.AllowNonGlobalTargets)
	}

	captured = engine.RunRequest{}
	clitest.Run(t, run, "example.com").RequireCode(t, 0)
	if captured.AllowNonGlobalTargets != nil {
		t.Fatalf("expected AllowNonGlobalTargets unset by default, got %#v", captured.AllowNonGlobalTargets)
	}
}

// The CLI tells the engine how much to capture. Anything the run itself will
// return has to stay capturable, so the capture level tracks the level the CLI
// asked the engine for rather than the user's display level.
func TestRunPassesCaptureMinLevelMatchingTheEngineLevel(t *testing.T) {
	var captured engine.RunRequest
	previous := runEngine
	runEngine = func(req engine.RunRequest) ([]engine.LogEntry, error) {
		captured = req
		return nil, nil
	}
	t.Cleanup(func() {
		runEngine = previous
	})

	res := clitest.Run(t, run, "--domain", "example.com", "--json")
	res.RequireCode(t, 0)

	if captured.MinLevel == "" {
		t.Fatal("expected the run to request a min level")
	}
	if captured.CaptureMinLevel != captured.MinLevel {
		t.Fatalf("capture level %q, want the engine min level %q", captured.CaptureMinLevel, captured.MinLevel)
	}
}

// --count tallies every entry it is handed, including levels far below the
// display floor, so that run must capture everything.
func TestRunCountCapturesEverything(t *testing.T) {
	var captured engine.RunRequest
	previous := runEngine
	runEngine = func(req engine.RunRequest) ([]engine.LogEntry, error) {
		captured = req
		return nil, nil
	}
	t.Cleanup(func() {
		runEngine = previous
	})

	res := clitest.Run(t, run, "--domain", "example.com", "--count")
	res.RequireCode(t, 0)

	if captured.CaptureMinLevel != "" {
		t.Fatalf("capture level %q, want empty so --count sees every entry", captured.CaptureMinLevel)
	}
}

// The packet cache is written from the nameserver cache, not from log entries,
// so capturing fewer entries must not change what --save produces.
func TestRunSavePacketCacheUnaffectedByCaptureLevel(t *testing.T) {
	dir := t.TempDir()
	savePath := filepath.Join(dir, "saved-cache.json")
	fixture := samplePacketCacheFile(t)

	var captured engine.RunRequest
	previous := runEngine
	runEngine = func(req engine.RunRequest) ([]engine.LogEntry, error) {
		captured = req
		if req.NameserverCache == nil {
			t.Fatalf("expected nameserver cache in request")
		}
		if importErr := cachefile.Import(fixture, req.NameserverCache, req.Recursor, req.ASNCache); importErr != nil {
			t.Fatalf("import fixture into run cache: %v", importErr)
		}
		return nil, nil
	}
	t.Cleanup(func() {
		runEngine = previous
	})

	res := clitest.Run(t, run, "--domain", "example.com", "--save", savePath)
	res.RequireCode(t, 0)

	if captured.CaptureMinLevel == "" {
		t.Fatal("expected the save run to be gated, otherwise this test proves nothing")
	}

	payloadBytes, err := os.ReadFile(savePath)
	if err != nil {
		t.Fatalf("read saved cache file: %v", err)
	}
	var payload cachefile.File
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		t.Fatalf("unmarshal saved cache: %v", err)
	}
	if len(payload.Entries) != 1 || payload.Entries[0].Key != "fixture.key" {
		t.Fatalf("unexpected saved cache contents: %+v", payload.Entries)
	}
}
