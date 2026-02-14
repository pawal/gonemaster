package server

import (
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/profile"
)

func TestApplyProfileOverridesWithBase(t *testing.T) {
	base := `{"net":{"ipv4":false}}`
	baseFile, err := os.CreateTemp("", "gm-profile-*.json")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	if _, err := baseFile.WriteString(base); err != nil {
		_ = baseFile.Close()
		_ = os.Remove(baseFile.Name())
		t.Fatalf("write base: %v", err)
	}
	if err := baseFile.Close(); err != nil {
		_ = os.Remove(baseFile.Name())
		t.Fatalf("close base: %v", err)
	}
	defer os.Remove(baseFile.Name())

	overrides := map[string]any{
		"resolver": map[string]any{
			"defaults": map[string]any{
				"timeout": 3,
			},
		},
	}

	req := engine.RunRequest{Domain: "example.com"}
	cleanup, err := applyProfileOverrides(&req, overrides, baseFile.Name())
	if err != nil {
		t.Fatalf("apply overrides: %v", err)
	}
	if cleanup == nil {
		t.Fatalf("expected cleanup function")
	}
	defer cleanup()

	if req.Profile == "" {
		t.Fatalf("expected profile path set")
	}
	payload, err := os.ReadFile(req.Profile)
	if err != nil {
		t.Fatalf("read profile: %v", err)
	}
	merged, err := profile.FromYAML(string(payload))
	if err != nil {
		t.Fatalf("parse merged: %v", err)
	}
	ipv4, err := merged.Get("net.ipv4")
	if err != nil {
		t.Fatalf("get net.ipv4: %v", err)
	}
	if value, ok := ipv4.(bool); !ok || value != false {
		t.Fatalf("expected net.ipv4 false, got %v", ipv4)
	}
	timeout, err := merged.Get("resolver.defaults.timeout")
	if err != nil {
		t.Fatalf("get resolver.defaults.timeout: %v", err)
	}
	if value, ok := timeout.(int); !ok || value != 3 {
		t.Fatalf("expected timeout 3, got %v", timeout)
	}
}

func TestApplyProfileOverridesBaseOnly(t *testing.T) {
	base := `{"net":{"ipv6":false}}`
	baseFile, err := os.CreateTemp("", "gm-profile-*.json")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	if _, err := baseFile.WriteString(base); err != nil {
		_ = baseFile.Close()
		_ = os.Remove(baseFile.Name())
		t.Fatalf("write base: %v", err)
	}
	if err := baseFile.Close(); err != nil {
		_ = os.Remove(baseFile.Name())
		t.Fatalf("close base: %v", err)
	}
	defer os.Remove(baseFile.Name())

	req := engine.RunRequest{Domain: "example.com"}
	cleanup, err := applyProfileOverrides(&req, nil, baseFile.Name())
	if err != nil {
		t.Fatalf("apply overrides: %v", err)
	}
	if cleanup != nil {
		t.Fatalf("expected nil cleanup for base-only profile")
	}
	if req.Profile != baseFile.Name() {
		t.Fatalf("expected profile path to be base file")
	}
}

func TestApplyProfileOverridesWithCacheReuse(t *testing.T) {
	base := `{"net":{"ipv4":false}}`
	baseFile, err := os.CreateTemp("", "gm-profile-*.json")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	if _, err := baseFile.WriteString(base); err != nil {
		_ = baseFile.Close()
		_ = os.Remove(baseFile.Name())
		t.Fatalf("write base: %v", err)
	}
	if err := baseFile.Close(); err != nil {
		_ = os.Remove(baseFile.Name())
		t.Fatalf("close base: %v", err)
	}
	defer os.Remove(baseFile.Name())

	overrides := map[string]any{
		"resolver": map[string]any{
			"defaults": map[string]any{
				"timeout": 3,
			},
		},
	}

	cache := newProfileOverrideCache(8, time.Hour)
	defer cache.Close()

	req1 := engine.RunRequest{Domain: "example.com"}
	cleanup1, err := applyProfileOverridesWithCache(&req1, overrides, baseFile.Name(), cache)
	if err != nil {
		t.Fatalf("first apply overrides: %v", err)
	}
	if cleanup1 != nil {
		t.Fatalf("expected nil cleanup for cached profile on first write")
	}
	if req1.Profile == "" {
		t.Fatalf("expected first profile path set")
	}

	req2 := engine.RunRequest{Domain: "example.net"}
	cleanup2, err := applyProfileOverridesWithCache(&req2, overrides, baseFile.Name(), cache)
	if err != nil {
		t.Fatalf("second apply overrides: %v", err)
	}
	if cleanup2 != nil {
		t.Fatalf("expected nil cleanup on cache hit")
	}
	if req2.Profile != req1.Profile {
		t.Fatalf("expected cache reuse path=%q, got %q", req1.Profile, req2.Profile)
	}
}

func TestApplyProfileOverridesWithCacheInvalidatesOnBaseChange(t *testing.T) {
	base := `{"net":{"ipv4":false}}`
	baseFile, err := os.CreateTemp("", "gm-profile-*.json")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	if _, err := baseFile.WriteString(base); err != nil {
		_ = baseFile.Close()
		_ = os.Remove(baseFile.Name())
		t.Fatalf("write base: %v", err)
	}
	if err := baseFile.Close(); err != nil {
		_ = os.Remove(baseFile.Name())
		t.Fatalf("close base: %v", err)
	}
	defer os.Remove(baseFile.Name())

	overrides := map[string]any{
		"resolver": map[string]any{
			"defaults": map[string]any{
				"timeout": 3,
			},
		},
	}

	cache := newProfileOverrideCache(8, time.Hour)
	defer cache.Close()

	req1 := engine.RunRequest{Domain: "example.com"}
	cleanup1, err := applyProfileOverridesWithCache(&req1, overrides, baseFile.Name(), cache)
	if err != nil {
		t.Fatalf("first apply overrides: %v", err)
	}
	if cleanup1 != nil {
		t.Fatalf("expected nil cleanup for cached profile on first write")
	}
	if req1.Profile == "" {
		t.Fatalf("expected first profile path set")
	}

	time.Sleep(2 * time.Millisecond)
	if err := os.WriteFile(baseFile.Name(), []byte(`{"net":{"ipv4":true}}`), 0o600); err != nil {
		t.Fatalf("rewrite base profile: %v", err)
	}

	req2 := engine.RunRequest{Domain: "example.net"}
	cleanup2, err := applyProfileOverridesWithCache(&req2, overrides, baseFile.Name(), cache)
	if err != nil {
		t.Fatalf("second apply overrides: %v", err)
	}
	if cleanup2 != nil {
		t.Fatalf("expected nil cleanup on cached profile")
	}
	if req1.Profile == req2.Profile {
		t.Fatalf("expected new cached path after base profile change")
	}

	payload, err := os.ReadFile(req2.Profile)
	if err != nil {
		t.Fatalf("read profile: %v", err)
	}
	merged, err := profile.FromYAML(string(payload))
	if err != nil {
		t.Fatalf("parse merged: %v", err)
	}
	ipv4, err := merged.Get("net.ipv4")
	if err != nil {
		t.Fatalf("get net.ipv4: %v", err)
	}
	if value, ok := ipv4.(bool); !ok || value != true {
		t.Fatalf("expected net.ipv4 true, got %v", ipv4)
	}
}

func TestProfileOverrideCacheCloseRemovesCachedFile(t *testing.T) {
	cache := newProfileOverrideCache(8, time.Hour)
	overrides := map[string]any{
		"resolver": map[string]any{
			"defaults": map[string]any{
				"timeout": 3,
			},
		},
	}

	req := engine.RunRequest{Domain: "example.com"}
	cleanup, err := applyProfileOverridesWithCache(&req, overrides, "", cache)
	if err != nil {
		t.Fatalf("apply overrides: %v", err)
	}
	if cleanup != nil {
		t.Fatalf("expected nil cleanup for cached profile")
	}
	if req.Profile == "" {
		t.Fatalf("expected cached profile path")
	}
	if _, err := os.Stat(req.Profile); err != nil {
		t.Fatalf("expected profile file to exist before close: %v", err)
	}

	cache.Close()

	if _, err := os.Stat(req.Profile); !os.IsNotExist(err) {
		t.Fatalf("expected profile file removed on cache close, got err=%v", err)
	}
}

func TestApplyProfileOverridesWithCacheConcurrentSingleMaterialization(t *testing.T) {
	baseFile, err := os.CreateTemp("", "gm-profile-*.json")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	if _, err := baseFile.WriteString(`{"net":{"ipv4":false}}`); err != nil {
		_ = baseFile.Close()
		_ = os.Remove(baseFile.Name())
		t.Fatalf("write base: %v", err)
	}
	if err := baseFile.Close(); err != nil {
		_ = os.Remove(baseFile.Name())
		t.Fatalf("close base: %v", err)
	}
	defer os.Remove(baseFile.Name())

	overrides := map[string]any{
		"resolver": map[string]any{
			"defaults": map[string]any{
				"timeout": 3,
			},
		},
	}
	cache := newProfileOverrideCache(8, time.Hour)
	defer cache.Close()

	originalBuildFn := buildMergedProfileFileFunc
	var builds atomic.Int32
	buildMergedProfileFileFunc = func(payload []byte, baseProfile string) (string, error) {
		builds.Add(1)
		time.Sleep(15 * time.Millisecond)
		return originalBuildFn(payload, baseProfile)
	}
	defer func() {
		buildMergedProfileFileFunc = originalBuildFn
	}()

	const workers = 24
	paths := make([]string, workers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func(i int) {
			defer wg.Done()
			<-start
			req := engine.RunRequest{Domain: "example.com"}
			cleanup, err := applyProfileOverridesWithCache(&req, overrides, baseFile.Name(), cache)
			if err != nil {
				t.Errorf("apply[%d]: %v", i, err)
				return
			}
			if cleanup != nil {
				cleanup()
				t.Errorf("apply[%d]: expected nil cleanup on cached profile path", i)
				return
			}
			paths[i] = req.Profile
		}(i)
	}

	close(start)
	wg.Wait()
	if t.Failed() {
		return
	}

	first := paths[0]
	if first == "" {
		t.Fatalf("expected first profile path")
	}
	for i, path := range paths {
		if path != first {
			t.Fatalf("expected all goroutines to reuse cached profile path; index %d=%q first=%q", i, path, first)
		}
	}
	if got := builds.Load(); got != 1 {
		t.Fatalf("expected exactly one profile materialization, got %d", got)
	}
}
