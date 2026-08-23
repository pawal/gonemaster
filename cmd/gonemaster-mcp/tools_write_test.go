package main

import (
	"context"
	"testing"

	"codeberg.org/pawal/gonemaster/cmd/internal/mcptest"
	"codeberg.org/pawal/gonemaster/internal/apitest"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// toolNames lists the tools a server registers for a given write gate.
func toolNames(t *testing.T, api *apiClient, allowWrite bool) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	mcptest.Session(t, newMCPServer(api, allowWrite), func(ctx context.Context, session *mcp.ClientSession) {
		res, err := session.ListTools(ctx, nil)
		if err != nil {
			t.Fatalf("list tools: %v", err)
		}
		for _, tl := range res.Tools {
			names[tl.Name] = true
		}
	})
	return names
}

var writeToolNames = []string{"batch_enqueue", "batch_cancel", "cancel_job"}

func TestWriteToolsHiddenByDefault(t *testing.T) {
	api := fakeAPI(t, apitest.Opts{})
	names := toolNames(t, api, false)

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
	api := fakeAPI(t, apitest.Opts{})
	names := toolNames(t, api, true)
	for _, n := range writeToolNames {
		if !names[n] {
			t.Errorf("write tool %q should register with the write gate on", n)
		}
	}
}

func TestBatchEnqueue(t *testing.T) {
	var captured apitest.BatchCreateRequest
	api := fakeAPI(t, apitest.Opts{BatchReq: &captured})

	var out batchEnqueueOutput
	res := callTool(t, api, "batch_enqueue", map[string]any{
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
	api := fakeAPI(t, apitest.Opts{})
	res := callTool(t, api, "batch_enqueue", map[string]any{}, nil)
	if !res.IsError {
		t.Fatalf("expected error with no domains or from_tag")
	}
}

func TestCancelJob(t *testing.T) {
	api := fakeAPI(t, apitest.Opts{})
	var out cancelJobOutput
	res := callTool(t, api, "cancel_job", map[string]any{"job_id": "job_7"}, &out)
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", errorText(res))
	}
	if out.JobID != "job_7" || out.Status != "canceled" {
		t.Errorf("cancel output wrong: %+v", out)
	}
}

func TestBatchCancel(t *testing.T) {
	api := fakeAPI(t, apitest.Opts{})
	var out batchCancelOutput
	res := callTool(t, api, "batch_cancel", map[string]any{"batch_id": "b1"}, &out)
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", errorText(res))
	}
	if !out.Canceled || out.BatchID != "b1" {
		t.Errorf("batch cancel output wrong: %+v", out)
	}
}
