package main

import (
	"context"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerWriteTools registers the mutating tools. It is only called when
// GONEMASTER_MCP_ALLOW_WRITE is set, so a default install is read + test only.
func registerWriteTools(srv *mcp.Server, api *apiClient) {
	registerBatchEnqueue(srv, api)
	registerBatchCancel(srv, api)
	registerCancelJob(srv, api)
}

type batchEnqueueInput struct {
	Domains []string `json:"domains,omitempty" jsonschema:"domains to enqueue; provide this or from_tag"`
	FromTag string   `json:"from_tag,omitempty" jsonschema:"enqueue every domain belonging to this existing tag"`
	Profile string   `json:"profile,omitempty" jsonschema:"test profile name; omit for the default"`
	Tags    []string `json:"tags,omitempty" jsonschema:"tags to apply to the submitted domains"`
}

type batchEnqueueOutput struct {
	BatchID  string `json:"batch_id"`
	JobCount int    `json:"job_count" jsonschema:"number of jobs enqueued; poll with batch_get"`
}

func registerBatchEnqueue(srv *mcp.Server, api *apiClient) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "batch_enqueue",
		Description: "Enqueue a batch of domain tests. Provide explicit domains or from_tag. Returns a batch id to poll.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in batchEnqueueInput) (*mcp.CallToolResult, batchEnqueueOutput, error) {
		domains := trimNonEmpty(in.Domains)
		fromTag := strings.TrimSpace(in.FromTag)
		if len(domains) == 0 && fromTag == "" {
			return nil, batchEnqueueOutput{}, errors.New("provide domains or from_tag")
		}
		resp, err := api.createBatch(ctx, batchCreateRequest{
			Domains: domains,
			FromTag: fromTag,
			Profile: strings.TrimSpace(in.Profile),
			Tags:    trimNonEmpty(in.Tags),
		})
		if err != nil {
			return nil, batchEnqueueOutput{}, toolError("enqueue batch", err)
		}
		return nil, batchEnqueueOutput{BatchID: resp.BatchID, JobCount: len(resp.JobIDs)}, nil
	})
}

type batchCancelInput struct {
	BatchID string `json:"batch_id" jsonschema:"the batch id to cancel and delete"`
}

type batchCancelOutput struct {
	BatchID  string `json:"batch_id"`
	Canceled bool   `json:"canceled"`
}

func registerBatchCancel(srv *mcp.Server, api *apiClient) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "batch_cancel",
		Description: "Cancel a batch's in-flight jobs and remove the batch and its derived data.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in batchCancelInput) (*mcp.CallToolResult, batchCancelOutput, error) {
		id := strings.TrimSpace(in.BatchID)
		if id == "" {
			return nil, batchCancelOutput{}, errors.New("batch_id is required")
		}
		if err := api.deleteBatch(ctx, id); err != nil {
			return nil, batchCancelOutput{}, toolError("cancel batch", err)
		}
		return nil, batchCancelOutput{BatchID: id, Canceled: true}, nil
	})
}

type cancelJobInput struct {
	JobID string `json:"job_id" jsonschema:"the job id to cancel"`
}

type cancelJobOutput struct {
	JobID  string `json:"job_id"`
	Status string `json:"status" jsonschema:"the job status after cancellation"`
}

func registerCancelJob(srv *mcp.Server, api *apiClient) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "cancel_job",
		Description: "Cancel a single queued or running job.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in cancelJobInput) (*mcp.CallToolResult, cancelJobOutput, error) {
		id := strings.TrimSpace(in.JobID)
		if id == "" {
			return nil, cancelJobOutput{}, errors.New("job_id is required")
		}
		job, err := api.cancelJob(ctx, id)
		if err != nil {
			return nil, cancelJobOutput{}, toolError("cancel job", err)
		}
		return nil, cancelJobOutput{JobID: job.ID, Status: job.Status}, nil
	})
}

func trimNonEmpty(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}
