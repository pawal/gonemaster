package nameserver

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

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

	entry := requireEntryByTag(t, log.Entries(), "IPV4_BLOCKED")
	requireNoArgSchema(t, entry)
	requireStringArg(t, entry, "ns", "ns.example")
	requireStringArg(t, entry, "address", "192.0.2.81")

	nsArg := entry.Args["ns"].(string)
	if strings.Contains(nsArg, "/") {
		t.Fatalf("expected canonical ns without endpoint separator, got %q", nsArg)
	}
}

func TestContract_ExternalQueryArgs(t *testing.T) {
	ns, err := New("ns.example", "127.0.0.1", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	log := logger.New()
	ctx := logger.WithContext(context.Background(), log)

	timeout := 10 * time.Millisecond
	opts := &QueryOptions{Timeout: &timeout}
	_, _ = ns.QueryWithOptions(ctx, "example.com", "SOA", opts)

	entry := requireEntryByTag(t, log.Entries(), "EXTERNAL_QUERY")
	requireNoArgSchema(t, entry)
	requireStringArg(t, entry, "ns", "ns.example")
	requireStringArg(t, entry, "address", "127.0.0.1")
	requireStringArg(t, entry, "query_name", "example.com")
	requireStringArg(t, entry, "query_type", "SOA")
	requireStringArg(t, entry, "query_class", "IN")
	requireStringArg(t, entry, "ip", "127.0.0.1")
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

	if _, err := ns.QueryWithOptions(ctx, "example1", "A", nil); err == nil {
		t.Fatalf("expected first query error")
	}
	if _, err := ns.QueryWithOptions(ctx, "example2", "A", nil); err != nil {
		t.Fatalf("expected second query to be skipped by error cache, got %v", err)
	}

	entry := requireEntryByTag(t, log.Entries(), "ERROR_CACHE_SKIP")
	requireNoArgSchema(t, entry)
	requireStringArg(t, entry, "ns", "ns.example")
	requireStringArg(t, entry, "address", "192.0.2.15")
	requireStringArg(t, entry, "query_name", "example2")
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

	entry := requireEntryByTag(t, log.Entries(), "FAKE_DS_RETURNED")
	requireNoArgSchema(t, entry)
	requireStringArg(t, entry, "ns", "ns.example")
	requireStringArg(t, entry, "address", "192.0.2.1")
	requireStringArg(t, entry, "query_name", "example")
	requireStringArg(t, entry, "query_type", "DS")
	requireStringArg(t, entry, "query_class", "IN")
}

func requireEntryByTag(t *testing.T, entries []*logger.Entry, tag string) *logger.Entry {
	t.Helper()
	for _, entry := range entries {
		if entry != nil && entry.Tag == tag {
			return entry
		}
	}
	t.Fatalf("missing log entry with tag %s", tag)
	return nil
}

func requireNoArgSchema(t *testing.T, entry *logger.Entry) {
	t.Helper()
	if _, ok := entry.Args["arg_schema"]; ok {
		t.Fatalf("did not expect arg_schema in args: %#v", entry.Args["arg_schema"])
	}
}

func requireStringArg(t *testing.T, entry *logger.Entry, key string, want string) {
	t.Helper()
	value, ok := entry.Args[key]
	if !ok {
		t.Fatalf("missing arg key %q in %#v", key, entry.Args)
	}
	got, ok := value.(string)
	if !ok {
		t.Fatalf("arg %q is not string: %#v", key, value)
	}
	if got != want {
		t.Fatalf("arg %q mismatch: got %q want %q", key, got, want)
	}
}
