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

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/cachefile"
	"codeberg.org/pawal/gonemaster/engine/logger"
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
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "--domain is required") {
		t.Fatalf("expected missing domain message, got %q", errOut.String())
	}
}

func TestRunHelpShowsGroupedFlags(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"-h"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	help := errOut.String()
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
	if out.Len() != 0 {
		t.Fatalf("expected no stdout output, got %q", out.String())
	}
}

func TestRunAcceptsPositionalDomain(t *testing.T) {
	var captured engine.RunRequest
	stubRunEngine(t, &captured)

	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"example.com"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr=%q)", code, errOut.String())
	}
	if captured.Domain != "example.com" {
		t.Fatalf("expected positional domain example.com, got %q", captured.Domain)
	}
}

func TestRunAcceptsPositionalDomainWithFlags(t *testing.T) {
	var captured engine.RunRequest
	stubRunEngine(t, &captured)

	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--min-level", "INFO", "--json", "example.com"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr=%q)", code, errOut.String())
	}
	if captured.Domain != "example.com" {
		t.Fatalf("expected positional domain example.com, got %q", captured.Domain)
	}
	if captured.MinLevel != "INFO" {
		t.Fatalf("expected min-level INFO, got %q", captured.MinLevel)
	}
}

func TestRunRejectsMultiplePositionalDomains(t *testing.T) {
	stubRunEngine(t, nil)

	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"example.com", "example.net"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "only one positional DOMAIN argument is allowed") {
		t.Fatalf("expected positional-argument validation error, got %q", errOut.String())
	}
}

func TestRunRejectsDomainProvidedTwice(t *testing.T) {
	stubRunEngine(t, nil)

	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--domain", "example.com", "example.net"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "domain provided twice; use either --domain DOMAIN or positional DOMAIN") {
		t.Fatalf("expected duplicate-domain validation error, got %q", errOut.String())
	}
}

func TestRunRejectsInvalidStopLevel(t *testing.T) {
	stubRunEngine(t, nil)

	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--domain", "example.com", "--stop-level", "BANANA"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "--stop-level must be one of") {
		t.Fatalf("expected stop-level validation error, got %q", errOut.String())
	}
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

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run([]string{
		"--domain", "example.com",
		"--json",
		"--min-level", "INFO",
		"--stop-level", "WARNING",
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr=%q)", code, errOut.String())
	}
	var payload []map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("expected JSON output, got %q (err=%v)", out.String(), err)
	}
	if len(payload) != 2 {
		t.Fatalf("expected 2 captured entries, got %d (%q)", len(payload), out.String())
	}
	if payload[0]["tag"] != "STOP_INFO" {
		t.Fatalf("unexpected first tag: %v", payload[0]["tag"])
	}
	if payload[1]["tag"] != "STOP_WARN" {
		t.Fatalf("unexpected second tag: %v", payload[1]["tag"])
	}
	if strings.TrimSpace(errOut.String()) != "" {
		t.Fatalf("expected no stderr output, got %q", errOut.String())
	}
}

func TestRunRejectsSaveAndVersion(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--version", "--save", "cache.json"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "--save/--restore cannot be combined with --version") {
		t.Fatalf("expected cache/version conflict message, got %q", errOut.String())
	}
}

func TestRunRejectsRestoreAndListTests(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--list-tests", "--restore", "cache.json"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "--save/--restore cannot be combined with --list-tests") {
		t.Fatalf("expected cache/list-tests conflict message, got %q", errOut.String())
	}
}

func TestRunRejectsSaveAndDumpProfile(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--save", "cache.json", "--dump-profile"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "--dump-profile cannot be combined with --save") {
		t.Fatalf("expected cache/dump-profile conflict message, got %q", errOut.String())
	}
}

func TestRunRejectsRestoreAndDumpProfile(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--restore", "cache.json", "--dump-profile"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "--dump-profile cannot be combined with --restore") {
		t.Fatalf("expected cache/dump-profile conflict message, got %q", errOut.String())
	}
}

func TestRunDumpProfileWithoutDomain(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--dump-profile"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("expected JSON output, got %q (err=%v)", out.String(), err)
	}
	if _, ok := payload["net"]; !ok {
		t.Fatalf("expected net in profile output, got %q", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", errOut.String())
	}
}

func TestRunWritesJSONAndError(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--domain", ".", "--testcase", "basic01", "--min-level", "INFO", "--json"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(out.String(), "B01_ROOT_HAS_NO_PARENT") {
		t.Fatalf("expected basic01 output, got %q", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", errOut.String())
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

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run([]string{"--domain", "example.com", "--json", "--restore", restorePath}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr=%q)", code, errOut.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", errOut.String())
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

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run([]string{"--domain", "example.com", "--json", "--save", savePath}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr=%q)", code, errOut.String())
	}

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

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run([]string{"--domain", "example.com", "--json", "--save", savePath, "--save-compress"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr=%q)", code, errOut.String())
	}

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

	out.Reset()
	errOut.Reset()
	code = run([]string{"--domain", "example.com", "--json", "--restore", savePath}, &out, &errOut)
	if code != 0 {
		t.Fatalf("restore: expected exit code 0, got %d (stderr=%q)", code, errOut.String())
	}
}

func TestRunSaveCompressRejectsWithoutSave(t *testing.T) {
	stubRunEngine(t, nil)

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run([]string{"--domain", "example.com", "--save-compress"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "--save-compress requires --save") {
		t.Fatalf("expected save-compress validation error, got %q", errOut.String())
	}
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

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run([]string{"--domain", "example.com", "--json", "--save", savePath, "--save-max-entries", "10"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr=%q)", code, errOut.String())
	}
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

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run([]string{"--domain", "example.com", "--json", "--save", savePath, "--save-max-entries", "0"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("--save-max-entries 0 should mean unlimited, got exit %d (stderr=%q)", code, errOut.String())
	}
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

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run([]string{"--domain", "example.com", "--json", "--save", savePath, "--save-max-entries", "1"}, &out, &errOut)
	if code == 0 {
		t.Fatalf("expected non-zero exit when entry count exceeds --save-max-entries")
	}
	if !strings.Contains(errOut.String(), "max-entries=1") {
		t.Fatalf("expected max-entries error message, got %q", errOut.String())
	}
	if _, err := os.Stat(savePath); err == nil {
		t.Fatalf("expected save file NOT to be written when over limit")
	}
}

func TestRunSaveMaxEntriesRejectsWithoutSave(t *testing.T) {
	stubRunEngine(t, nil)

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run([]string{"--domain", "example.com", "--save-max-entries", "5"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "--save-max-entries requires --save") {
		t.Fatalf("expected save-max-entries validation error, got %q", errOut.String())
	}
}

func TestRunSaveMaxEntriesRejectsNegative(t *testing.T) {
	stubRunEngine(t, nil)

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run([]string{"--domain", "example.com", "--save", "out.json", "--save-max-entries", "-1"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "--save-max-entries must be >= 0") {
		t.Fatalf("expected negative-rejection error, got %q", errOut.String())
	}
}

func TestRunWritesOutputFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out.json")

	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--domain", ".", "--testcase", "basic01", "--min-level", "INFO", "--json", "--output", target}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !strings.Contains(string(data), "B01_ROOT_HAS_NO_PARENT") {
		t.Fatalf("unexpected file output: %q", string(data))
	}
}

func TestRunRawStreamsOutput(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--domain", ".", "--testcase", "basic01", "--min-level", "INFO", "--raw"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(out.String(), "B01_ROOT_HAS_NO_PARENT") {
		t.Fatalf("expected raw output, got %q", out.String())
	}
	if strings.Contains(out.String(), "\"timestamp\"") {
		t.Fatalf("expected raw output, got %q", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", errOut.String())
	}
}

func TestRunJSONStreamOutputs(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--domain", ".", "--testcase", "basic01", "--min-level", "INFO", "--json-stream"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	lines := strings.Split(out.String(), "\n")
	var first string
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		first = line
		break
	}
	if first == "" {
		t.Fatalf("expected json-stream output, got %q", out.String())
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(first), &payload); err != nil {
		t.Fatalf("expected JSON line, got %q (err=%v)", first, err)
	}
	if tag, ok := payload["Tag"].(string); !ok || !strings.Contains(tag, "B01_") {
		t.Fatalf("expected Tag in JSON output, got %v", payload["Tag"])
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", errOut.String())
	}
}

func TestRunRejectsRawAndJSON(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--domain", ".", "--json", "--raw"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "--json cannot be combined with --raw") {
		t.Fatalf("expected error message, got %q", errOut.String())
	}
	if out.Len() != 0 {
		t.Fatalf("expected no stdout output, got %q", out.String())
	}
}

func TestRunRejectsJSONStreamAndRaw(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--domain", ".", "--json-stream", "--raw"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "--json-stream cannot be combined with --raw") {
		t.Fatalf("expected error message, got %q", errOut.String())
	}
	if out.Len() != 0 {
		t.Fatalf("expected no stdout output, got %q", out.String())
	}
}

func TestRunRejectsJSONStreamAndJSON(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--domain", ".", "--json-stream", "--json"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "--json-stream cannot be combined with --json") {
		t.Fatalf("expected error message, got %q", errOut.String())
	}
	if out.Len() != 0 {
		t.Fatalf("expected no stdout output, got %q", out.String())
	}
}

func TestRunRejectsCountAndRaw(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--domain", ".", "--count", "--raw"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "--count cannot be combined with --raw") {
		t.Fatalf("expected error message, got %q", errOut.String())
	}
	if out.Len() != 0 {
		t.Fatalf("expected no stdout output, got %q", out.String())
	}
}

func TestRunRejectsCountAndJSON(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--domain", ".", "--count", "--json"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "--count cannot be combined with --json") {
		t.Fatalf("expected error message, got %q", errOut.String())
	}
	if out.Len() != 0 {
		t.Fatalf("expected no stdout output, got %q", out.String())
	}
}

func TestRunRejectsCountAndJSONStream(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--domain", ".", "--count", "--json-stream"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "--count cannot be combined with --json-stream") {
		t.Fatalf("expected error message, got %q", errOut.String())
	}
	if out.Len() != 0 {
		t.Fatalf("expected no stdout output, got %q", out.String())
	}
}

func TestRunRejectsCountAndDumpProfile(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--count", "--dump-profile"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "--dump-profile cannot be combined with --count") {
		t.Fatalf("expected error message, got %q", errOut.String())
	}
	if out.Len() != 0 {
		t.Fatalf("expected no stdout output, got %q", out.String())
	}
}

func TestRunParsesUndelegatedNameserverFlags(t *testing.T) {
	var captured engine.RunRequest
	stubRunEngine(t, &captured)

	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{
		"--domain", "example.com",
		"--json",
		"--ns", "NS1.Example.com/192.0.2.1",
		"--ns", "ns2.example.net",
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr=%q)", code, errOut.String())
	}
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

	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{
		"--domain", "example.com",
		"--json",
		"--ns", "NS1.Example.com/192.0.2.1",
		"--ns", "ns1.example.com/2001:db8::1",
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr=%q)", code, errOut.String())
	}
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

	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{
		"--domain", "example.com",
		"--json",
		"--ds", "12345,13,2," + strings.Repeat("a", 64),
		"--ds", "23456,8,2," + strings.Repeat("B", 64),
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr=%q)", code, errOut.String())
	}
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

	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{
		"--domain", "example.com",
		"--json",
		"--ns", "bad!name.example/192.0.2.1",
	}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "undelegated nameserver") {
		t.Fatalf("expected undelegated nameserver parse error, got %q", errOut.String())
	}
}

func TestRunRejectsMalformedUndelegatedDSFlag(t *testing.T) {
	stubRunEngine(t, nil)

	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{
		"--domain", "example.com",
		"--json",
		"--ds", "12345,13,2,NOT-HEX",
	}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "undelegated DS") {
		t.Fatalf("expected undelegated DS parse error, got %q", errOut.String())
	}
}

func TestRunCarriesUndelegatedInputsInRunRequest(t *testing.T) {
	var captured engine.RunRequest
	stubRunEngine(t, &captured)

	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{
		"--domain", "example.com",
		"--json",
		"--ns", "ns1.example.com/192.0.2.10",
		"--ds", "12345,13,2," + strings.Repeat("a", 64),
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr=%q)", code, errOut.String())
	}
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

	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{
		"--domain", "example.com",
		"--json",
		"--sourceaddr4", "192.0.2.44",
		"--sourceaddr6", "2001:db8::44",
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr=%q)", code, errOut.String())
	}
	if captured.SourceAddr4 == nil || *captured.SourceAddr4 != "192.0.2.44" {
		t.Fatalf("unexpected SourceAddr4 override: %#v", captured.SourceAddr4)
	}
	if captured.SourceAddr6 == nil || *captured.SourceAddr6 != "2001:db8::44" {
		t.Fatalf("unexpected SourceAddr6 override: %#v", captured.SourceAddr6)
	}
}

func TestRunRejectsInvalidSourceAddr4(t *testing.T) {
	stubRunEngine(t, nil)

	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{
		"--domain", "example.com",
		"--json",
		"--sourceaddr4", "not-an-ip",
	}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "--sourceaddr4 must be a valid IPv4 address") {
		t.Fatalf("expected sourceaddr4 parse error, got %q", errOut.String())
	}
}

func TestRunRejectsInvalidSourceAddr6(t *testing.T) {
	stubRunEngine(t, nil)

	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{
		"--domain", "example.com",
		"--json",
		"--sourceaddr6", "192.0.2.10",
	}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "--sourceaddr6 must be a valid IPv6 address") {
		t.Fatalf("expected sourceaddr6 parse error, got %q", errOut.String())
	}
}

func TestRunOutputsTranslatedByDefault(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--domain", ".", "--testcase", "basic01", "--min-level", "INFO", "--locale", "en"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.HasPrefix(out.String(), "Seconds Level    Message") {
		t.Fatalf("expected translated header, got %q", out.String())
	}
	if strings.Contains(out.String(), "\"timestamp\"") {
		t.Fatalf("expected translated output, got %q", out.String())
	}
	if !strings.Contains(out.String(), "test of the root zone") {
		t.Fatalf("expected translated output, got %q", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", errOut.String())
	}
}

func TestRunOutputsLocalizedHeaderByLocale(t *testing.T) {
	stubRunEngine(t, nil)

	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--domain", "example.com", "--min-level", "CRITICAL", "--locale", "sv"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}

	firstLine := strings.SplitN(out.String(), "\n", 2)[0]
	if !strings.Contains(firstLine, "Sekunder") || !strings.Contains(firstLine, "Nivå") || !strings.Contains(firstLine, "Meddelande") {
		t.Fatalf("expected localized header for sv locale, got %q", firstLine)
	}
	if strings.Contains(firstLine, "Seconds") {
		t.Fatalf("expected non-English header for sv locale, got %q", firstLine)
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", errOut.String())
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
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--domain", ".", "--testcase", "basic01", "--min-level", "CRITICAL", "--locale", "en"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.HasPrefix(out.String(), "Seconds Level    Message") {
		t.Fatalf("expected translated header, got %q", out.String())
	}
	if !strings.Contains(out.String(), "Looks OK.") {
		t.Fatalf("expected Looks OK when no entries match level, got %q", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", errOut.String())
	}
}

func TestRunCountPrintsSummariesFromAllLevels(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--domain", ".", "--testcase", "basic01", "--count", "--locale", "en", "--no-progress"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.HasPrefix(out.String(), "Seconds Level    Message") {
		t.Fatalf("expected translated header, got %q", out.String())
	}
	if !strings.Contains(out.String(), "Looks OK.") {
		t.Fatalf("expected Looks OK marker when min-level hides entries, got %q", out.String())
	}
	if !strings.Contains(out.String(), "Number of log entries") {
		t.Fatalf("expected level count summary, got %q", out.String())
	}
	if !strings.Contains(out.String(), "Message tag") {
		t.Fatalf("expected tag count summary, got %q", out.String())
	}
	if !strings.Contains(out.String(), "INFO") {
		t.Fatalf("expected INFO counts from entries below default min-level, got %q", out.String())
	}
	if !strings.Contains(out.String(), "B01_ROOT_HAS_NO_PARENT") {
		t.Fatalf("expected B01 tag count in summary, got %q", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", errOut.String())
	}
}

func TestRunJSONNoEntriesRemainsJSONArray(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--domain", ".", "--testcase", "basic01", "--min-level", "CRITICAL", "--json"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if strings.Contains(out.String(), "Looks OK.") {
		t.Fatalf("expected JSON output without Looks OK marker, got %q", out.String())
	}
	if strings.TrimSpace(out.String()) != "[]" {
		t.Fatalf("expected empty JSON array, got %q", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", errOut.String())
	}
}

func TestRunRawNoEntriesRemainsEmpty(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--domain", ".", "--testcase", "basic01", "--min-level", "CRITICAL", "--raw"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if strings.TrimSpace(out.String()) != "" {
		t.Fatalf("expected empty raw stream, got %q", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", errOut.String())
	}
}

func TestRunJSONStreamNoEntriesRemainsEmpty(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--domain", ".", "--testcase", "basic01", "--min-level", "CRITICAL", "--json-stream"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if strings.TrimSpace(out.String()) != "" {
		t.Fatalf("expected empty json-stream output, got %q", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", errOut.String())
	}
}

func TestRunListTests(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--list-tests"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
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
	for _, want := range expected {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("expected %q in list-tests output, got %q", want, out.String())
		}
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", errOut.String())
	}
}

func TestRunVersion(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--version"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(out.String(), "Gonemaster version") {
		t.Fatalf("expected version output, got %q", out.String())
	}
	if !strings.Contains(out.String(), engine.VersionString()) {
		t.Fatalf("expected version output, got %q", out.String())
	}
	if !strings.Contains(out.String(), "Miekg DNS version") {
		t.Fatalf("expected dependency version output, got %q", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", errOut.String())
	}
}

func TestRunNSTimesCreatesCache(t *testing.T) {
	var captured engine.RunRequest
	stubRunEngine(t, &captured)

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run([]string{"--domain", "example.com", "--nstimes"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", code, errOut.String())
	}
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

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run([]string{"--domain", "example.com", "--nstimes"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", code, errOut.String())
	}
	output := out.String()
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
