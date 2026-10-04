package mcpbridge

import (
	"context"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerDiscoveryTools(srv *mcp.Server, api *Client) {
	registerProfileList(srv, api)
	registerDomainTagList(srv, api)
	registerCohortList(srv, api)
	registerCohortScheduleList(srv, api)
}

type profileListInput struct{}

type profileOut struct {
	ID          int64  `json:"id" jsonschema:"pass as test_domain's profile_id"`
	Name        string `json:"name" jsonschema:"pass as batch_enqueue's profile"`
	Description string `json:"description,omitempty"`
	Public      bool   `json:"public" jsonschema:"true when the public UI offers this profile"`
}

type profileListOutput struct {
	Count    int          `json:"count"`
	Profiles []profileOut `json:"profiles"`
}

func registerProfileList(srv *mcp.Server, api *Client) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "profile_list",
		Description: "List the stored test profiles: the id feeds test_domain's profile_id, the name feeds batch_enqueue's profile.",
		Annotations: readOnly("List profiles"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ profileListInput) (*mcp.CallToolResult, profileListOutput, error) {
		items, err := api.listProfiles(ctx)
		if err != nil {
			return nil, profileListOutput{}, api.toolError("list profiles", err)
		}
		out := profileListOutput{Profiles: []profileOut{}}
		for _, p := range items {
			out.Profiles = append(out.Profiles, profileOut{ID: p.ID, Name: p.Name, Description: p.Description, Public: p.Public})
		}
		out.Count = len(out.Profiles)
		return nil, out, nil
	})
}

type domainTagListInput struct {
	Limit int `json:"limit,omitempty" jsonschema:"max tags to return (default 100, max 500)"`
}

type domainTagOut struct {
	Name             string `json:"name" jsonschema:"pass as batch_enqueue's from_tag"`
	Description      string `json:"description,omitempty"`
	DomainCount      int    `json:"domain_count"`
	DefaultProfileID *int64 `json:"default_profile_id,omitempty"`
}

type domainTagListOutput struct {
	Count int            `json:"count"`
	Tags  []domainTagOut `json:"tags"`
}

func registerDomainTagList(srv *mcp.Server, api *Client) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "domain_tag_list",
		Description: "List user domain tags: named groups of domains that batches are built from and that batch_enqueue's " +
			"from_tag accepts. These are not finding tags (message tags such as SOATIME); spec_get_testcase lists those.",
		Annotations: readOnly("List domain tags"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in domainTagListInput) (*mcp.CallToolResult, domainTagListOutput, error) {
		items, err := api.listDomainTags(ctx, clampLimit(in.Limit, 100, maxRunsLimit))
		if err != nil {
			return nil, domainTagListOutput{}, api.toolError("list domain tags", err)
		}
		out := domainTagListOutput{Tags: []domainTagOut{}}
		for _, t := range items {
			out.Tags = append(out.Tags, domainTagOut{Name: t.Name, Description: t.Description, DomainCount: t.DomainCount, DefaultProfileID: t.DefaultProfileID})
		}
		out.Count = len(out.Tags)
		return nil, out, nil
	})
}

type cohortListInput struct {
	DatasetTag string `json:"dataset_tag,omitempty" jsonschema:"also list this cohort's snapshots, newest first"`
}

type cohortOut struct {
	DatasetTag string `json:"dataset_tag" jsonschema:"pass as cohort_report's dataset_tag"`
	Label      string `json:"label,omitempty"`
	IsDefault  bool   `json:"is_default"`
}

type snapshotOut struct {
	Slug        string `json:"slug" jsonschema:"pass as cohort_report's from or to"`
	Label       string `json:"label,omitempty"`
	CapturedAt  string `json:"captured_at,omitempty"`
	DomainCount int    `json:"domain_count"`
}

type cohortListOutput struct {
	DefaultTag string        `json:"default_tag,omitempty"`
	Cohorts    []cohortOut   `json:"cohorts"`
	DatasetTag string        `json:"dataset_tag,omitempty" jsonschema:"the cohort whose snapshots follow"`
	Snapshots  []snapshotOut `json:"snapshots,omitempty"`
}

func registerCohortList(srv *mcp.Server, api *Client) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "cohort_list",
		Description: "List the public analysis cohorts and, when dataset_tag is given, that cohort's snapshots newest first. Slugs feed cohort_report's from and to.",
		Annotations: readOnly("List cohorts and snapshots"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in cohortListInput) (*mcp.CallToolResult, cohortListOutput, error) {
		catalog, err := api.getAnalysisCatalog(ctx)
		if err != nil {
			return nil, cohortListOutput{}, api.toolError("get analysis catalog", err)
		}
		out := cohortListOutput{DefaultTag: catalog.DefaultTag, Cohorts: []cohortOut{}}
		for _, c := range catalog.Cohorts {
			out.Cohorts = append(out.Cohorts, cohortOut{DatasetTag: c.DatasetTag, Label: c.Label, IsDefault: c.IsDefault})
		}
		tag := strings.TrimSpace(in.DatasetTag)
		if tag == "" {
			return nil, out, nil
		}
		list, err := api.listAnalysisSnapshots(ctx, tag)
		if err != nil {
			return nil, cohortListOutput{}, api.toolError("list snapshots", err)
		}
		out.DatasetTag = tag
		out.Snapshots = []snapshotOut{}
		for _, s := range list.Snapshots {
			snap := snapshotOut{Slug: s.Slug, Label: s.Label, DomainCount: s.DomainCount}
			if !s.CapturedAt.IsZero() {
				snap.CapturedAt = s.CapturedAt.UTC().Format(time.RFC3339)
			}
			out.Snapshots = append(out.Snapshots, snap)
		}
		return nil, out, nil
	})
}

type cohortScheduleListInput struct{}

type cohortScheduleOut struct {
	DatasetTag  string `json:"dataset_tag" jsonschema:"the cohort, as cohort_report's dataset_tag"`
	Label       string `json:"label,omitempty"`
	Enabled     bool   `json:"enabled"`
	Summary     string `json:"summary" jsonschema:"the recurrence in English"`
	NextRunAt   string `json:"next_run_at,omitempty"`
	LastRunAt   string `json:"last_run_at,omitempty"`
	LastOutcome string `json:"last_outcome,omitempty" jsonschema:"submitted, skipped_missed, skipped_disabled, skipped_active, skipped_empty or error"`
	LastBatchID string `json:"last_batch_id,omitempty" jsonschema:"the last scheduled batch; pass to batch_get"`
}

type cohortScheduleListOutput struct {
	Count     int                 `json:"count"`
	Schedules []cohortScheduleOut `json:"schedules"`
}

func registerCohortScheduleList(srv *mcp.Server, api *Client) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "cohort_schedule_list",
		Description: "List the cohort snapshot schedules: the rule, the next run, and the outcome and batch of the last firing.",
		Annotations: readOnly("List cohort schedules"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ cohortScheduleListInput) (*mcp.CallToolResult, cohortScheduleListOutput, error) {
		items, err := api.listCohortSchedules(ctx)
		if err != nil {
			return nil, cohortScheduleListOutput{}, api.toolError("list cohort schedules", err)
		}
		out := cohortScheduleListOutput{Schedules: []cohortScheduleOut{}}
		for _, s := range items {
			out.Schedules = append(out.Schedules, cohortScheduleOut{
				DatasetTag: s.SourceTag, Label: s.Label, Enabled: s.Enabled, Summary: s.Summary,
				NextRunAt: s.NextRunAt, LastRunAt: s.LastRunAt, LastOutcome: s.LastOutcome, LastBatchID: s.LastBatchID,
			})
		}
		out.Count = len(out.Schedules)
		return nil, out, nil
	})
}
