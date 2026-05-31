package main

import (
	"context"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// toolNames lists the tools a server registers for a given write gate.
func toolNames(t *testing.T, api *apiClient, allowWrite bool) map[string]bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	done := make(chan struct{})
	defer func() { cancel(); <-done }()

	clientT, serverT := mcp.NewInMemoryTransports()
	srv := newMCPServer(api, allowWrite)
	go func() { _ = srv.Run(ctx, serverT); close(done) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	session, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer session.Close()

	res, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	names := map[string]bool{}
	for _, tl := range res.Tools {
		names[tl.Name] = true
	}
	return names
}

var writeToolNames = []string{"batch_enqueue", "batch_cancel", "cancel_job"}

func TestWriteToolsHiddenByDefault(t *testing.T) {
	ts := newFakeServer(t, fakeOpts{})
	defer ts.Close()
	names := toolNames(t, clientFor(t, ts.URL, ""), false)

	if !names["test_domain"] || !names["ping"] {
		t.Errorf("read tools should always be present: %v", names)
	}
	for _, n := range writeToolNames {
		if names[n] {
			t.Errorf("write tool %q must not register without the write gate", n)
		}
	}
}

func TestWriteToolsRegisterWhenAllowed(t *testing.T) {
	ts := newFakeServer(t, fakeOpts{})
	defer ts.Close()
	names := toolNames(t, clientFor(t, ts.URL, ""), true)
	for _, n := range writeToolNames {
		if !names[n] {
			t.Errorf("write tool %q should register with the write gate on", n)
		}
	}
}

func TestBatchEnqueue(t *testing.T) {
	var captured batchCreateRequest
	ts := newFakeServer(t, fakeOpts{batchReq: &captured})
	defer ts.Close()

	var out batchEnqueueOutput
	res := callTool(t, clientFor(t, ts.URL, ""), "batch_enqueue", map[string]any{
		"domains": []any{"a.example", "b.example"},
		"profile": "thorough",
		"tags":    []any{"se"},
	}, &out)
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", errorText(res))
	}
	if out.BatchID != "batch_new" || out.JobCount != 2 {
		t.Errorf("output wrong: %+v", out)
	}
	if len(captured.Domains) != 2 || captured.Profile != "thorough" || len(captured.Tags) != 1 {
		t.Errorf("request not forwarded: %+v", captured)
	}
}

func TestBatchEnqueueRequiresDomainsOrTag(t *testing.T) {
	ts := newFakeServer(t, fakeOpts{})
	defer ts.Close()
	res := callTool(t, clientFor(t, ts.URL, ""), "batch_enqueue", map[string]any{}, nil)
	if !res.IsError {
		t.Fatalf("expected error with no domains or from_tag")
	}
}

func TestCancelJob(t *testing.T) {
	ts := newFakeServer(t, fakeOpts{})
	defer ts.Close()
	var out cancelJobOutput
	res := callTool(t, clientFor(t, ts.URL, ""), "cancel_job", map[string]any{"job_id": "job_7"}, &out)
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", errorText(res))
	}
	if out.JobID != "job_7" || out.Status != "canceled" {
		t.Errorf("cancel output wrong: %+v", out)
	}
}

func TestBatchCancel(t *testing.T) {
	ts := newFakeServer(t, fakeOpts{})
	defer ts.Close()
	var out batchCancelOutput
	res := callTool(t, clientFor(t, ts.URL, ""), "batch_cancel", map[string]any{"batch_id": "b1"}, &out)
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", errorText(res))
	}
	if !out.Canceled || out.BatchID != "b1" {
		t.Errorf("batch cancel output wrong: %+v", out)
	}
}
