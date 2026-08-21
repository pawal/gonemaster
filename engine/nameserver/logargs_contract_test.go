package nameserver

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine/internal/dnstest"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/packet"
)

func TestContract_IPV4BlockedArgs(t *testing.T) {
	ctx, prof := testContext(t)
	log := logger.FromContext(ctx)
	prof.Net.IPv4 = false

	ns, err := NewWithContext(ctx, "ns.example", "192.0.2.81", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	if _, err := ns.QueryWithOptions(ctx, "example", "A", nil); err != nil {
		t.Fatalf("query with ipv4 disabled: %v", err)
	}

	entry := dnstest.RequireEntryByTag(t, log.Entries(), "IPV4_BLOCKED")
	requireNoArgSchema(t, entry)
	requireStringArg(t, entry, "ns", "ns.example")
	requireStringArg(t, entry, "address", "192.0.2.81")

	nsArg := entry.Args["ns"].(string)
	if strings.Contains(nsArg, "/") {
		t.Fatalf("expected canonical ns without endpoint separator, got %q", nsArg)
	}
}

func TestContract_ExternalQueryArgs(t *testing.T) {
	ctx, prof := testContext(t)
	// Exercise the real query path against a loopback stand-in address.
	prof.Net.AllowNonGlobalTargets = true
	log := logger.FromContext(ctx)

	ns, err := NewWithContext(ctx, "ns.example", "127.0.0.1", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	timeout := 10 * time.Millisecond
	opts := &QueryOptions{Timeout: &timeout}
	_, _ = ns.QueryWithOptions(ctx, "example.com", "SOA", opts)

	entry := dnstest.RequireEntryByTag(t, log.Entries(), "EXTERNAL_QUERY")
	requireNoArgSchema(t, entry)
	requireStringArg(t, entry, "ns", "ns.example")
	requireStringArg(t, entry, "address", "127.0.0.1")
	requireStringArg(t, entry, "query_name", "example.com")
	requireStringArg(t, entry, "query_type", "SOA")
	requireStringArg(t, entry, "query_class", "IN")
	if _, ok := entry.Args["ip"]; ok {
		t.Fatalf("legacy key ip should not be present: %#v", entry.Args)
	}
}

func TestContract_ErrorCacheSkipArgs(t *testing.T) {
	ctx, prof := testContext(t)
	log := logger.FromContext(ctx)
	prof.Resolver.Defaults.ErrorCacheTTL = 60

	ns, err := NewWithContext(ctx, "ns.example", "192.0.2.15", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		return packet.Packet{}, fmt.Errorf("network error")
	})

	// Pre-populate the error cache for the exact query we are about to
	// issue. The query cache is empty for this key, so shouldSkipQuery
	// reaches the error-cache check and emits ERROR_CACHE_SKIP.
	key, _, _, err := buildCacheKey("example", "A", "IN", nil)
	if err != nil {
		t.Fatalf("build cache key: %v", err)
	}
	ns.state.errorCache.set(key, time.Minute)

	if _, err := ns.QueryWithOptions(ctx, "example", "A", nil); err != nil {
		t.Fatalf("expected query to be suppressed by error cache, got %v", err)
	}

	entry := dnstest.RequireEntryByTag(t, log.Entries(), "ERROR_CACHE_SKIP")
	requireNoArgSchema(t, entry)
	requireStringArg(t, entry, "ns", "ns.example")
	requireStringArg(t, entry, "address", "192.0.2.15")
	requireStringArg(t, entry, "query_name", "example")
	requireStringArg(t, entry, "query_type", "A")
	requireStringArg(t, entry, "query_class", "IN")
	if _, ok := entry.Args["ttl_seconds"].(int); !ok {
		t.Fatalf("expected ttl_seconds int, got %#v", entry.Args["ttl_seconds"])
	}
}

func TestContract_FakeDSReturnedArgs(t *testing.T) {
	ctx, _ := testContext(t)
	log := logger.FromContext(ctx)

	ns, err := NewWithContext(ctx, "ns.example", "192.0.2.1", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	err = ns.AddFakeDS("example", []DSData{{
		KeyTag:     1234,
		Algorithm:  8,
		DigestType: 2,
		Digest:     "ABCD",
	}})
	if err != nil {
		t.Fatalf("add fake DS: %v", err)
	}

	if _, err := ns.QueryWithOptions(ctx, "example", "DS", nil); err != nil {
		t.Fatalf("query fake DS: %v", err)
	}

	entry := dnstest.RequireEntryByTag(t, log.Entries(), "FAKE_DS_RETURNED")
	requireNoArgSchema(t, entry)
	requireStringArg(t, entry, "ns", "ns.example")
	requireStringArg(t, entry, "address", "192.0.2.1")
	requireStringArg(t, entry, "query_name", "example")
	requireStringArg(t, entry, "query_type", "DS")
	requireStringArg(t, entry, "query_class", "IN")
}

func requireNoArgSchema(t *testing.T, entry *logger.Entry) {
	t.Helper()
	if _, ok := entry.Args["arg_schema"]; ok {
		t.Fatalf("did not expect arg_schema in args: %#v", entry.Args["arg_schema"])
	}
}

func requireStringArg(t *testing.T, entry *logger.Entry, key string, want string) {
	t.Helper()
	if got := dnstest.RequireStringArg(t, entry, key); got != want {
		t.Fatalf("arg %q = %q, want %q", key, got, want)
	}
}
