package main

import (
	"context"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerBatchTools(srv *mcp.Server, api *apiClient) {
	registerBatchGet(srv, api)
	registerCohortStats(srv, api)
	registerCohortTagValues(srv, api)
	registerFailuresByTag(srv, api)
}

type batchGetInput struct {
	BatchID string `json:"batch_id" jsonschema:"the batch id"`
}

type batchGetOutput struct {
	BatchID      string         `json:"batch_id"`
	Tag          string         `json:"tag,omitempty"`
	Total        int            `json:"total"`
	StatusCounts map[string]int `json:"status_counts" jsonschema:"job count per status"`
	Done         bool           `json:"done" jsonschema:"true when no jobs are queued or running"`
	CreatedAt    string         `json:"created_at,omitempty"`
	FinishedAt   string         `json:"finished_at,omitempty"`
}

func registerBatchGet(srv *mcp.Server, api *apiClient) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "batch_get",
		Description: "Get a batch's progress: total, per-status counts, completion, and timing. Use to poll a batch.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in batchGetInput) (*mcp.CallToolResult, batchGetOutput, error) {
		id := strings.TrimSpace(in.BatchID)
		if id == "" {
			return nil, batchGetOutput{}, errors.New("batch_id is required")
		}
		b, err := api.getBatch(ctx, id)
		if err != nil {
			return nil, batchGetOutput{}, toolError("get batch", err)
		}
		out := batchGetOutput{BatchID: b.BatchID, Tag: b.Tag, Total: b.Total, StatusCounts: b.StatusCounts}
		out.Done = b.StatusCounts["queued"] == 0 && b.StatusCounts["running"] == 0 && b.StatusCounts["paused"] == 0
		if !b.CreatedAt.IsZero() {
			out.CreatedAt = b.CreatedAt.UTC().Format(time.RFC3339)
		}
		if b.FinishedAt != nil && !b.FinishedAt.IsZero() {
			out.FinishedAt = b.FinishedAt.UTC().Format(time.RFC3339)
		}
		return nil, out, nil
	})
}

type cohortStatsInput struct {
	BatchID string `json:"batch_id" jsonschema:"the batch id"`
}

type cohortStatsOutput struct {
	BatchID     string         `json:"batch_id"`
	Total       int            `json:"total"`
	Grades      map[string]int `json:"grades" jsonschema:"run count per letter grade"`
	WorstLevels map[string]int `json:"worst_levels" jsonschema:"run count per worst severity level"`
}

func registerCohortStats(srv *mcp.Server, api *apiClient) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "cohort_stats",
		Description: "Grade distribution and worst-severity distribution across the completed runs in a batch.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in cohortStatsInput) (*mcp.CallToolResult, cohortStatsOutput, error) {
		id := strings.TrimSpace(in.BatchID)
		if id == "" {
			return nil, cohortStatsOutput{}, errors.New("batch_id is required")
		}
		b, err := api.getBatch(ctx, id)
		if err != nil {
			return nil, cohortStatsOutput{}, toolError("get batch", err)
		}
		q := url.Values{}
		q.Set("batch", id)
		q.Set("limit", "500")
		worst := map[string]int{}
		total := 0
		for offset := 0; ; offset += 500 {
			q.Set("offset", strconv.Itoa(offset))
			runs, err := api.getRuns(ctx, q)
			if err != nil {
				return nil, cohortStatsOutput{}, toolError("list batch runs", err)
			}
			for _, r := range runs.Items {
				total++
				if r.WorstLevel != "" {
					worst[r.WorstLevel]++
				}
			}
			if len(runs.Items) < 500 {
				break
			}
		}
		grades := b.Grades
		if grades == nil {
			grades = map[string]int{}
		}
		return nil, cohortStatsOutput{BatchID: id, Total: total, Grades: grades, WorstLevels: worst}, nil
	})
}

type cohortTagValuesInput struct {
	BatchID       string `json:"batch_id" jsonschema:"the batch id"`
	Tag           string `json:"tag" jsonschema:"the log message tag whose arg to roll up, e.g. N16_HAS_NSID or IPV4_ONE_ASN"`
	Arg           string `json:"arg" jsonschema:"the arg key within that tag to extract, e.g. nsid or asn"`
	MinCount      int    `json:"min_count,omitempty" jsonschema:"suppress values carried by fewer than this many domains (default 1)"`
	Limit         int    `json:"limit,omitempty" jsonschema:"max value rows to return (default 50, max 500)"`
	WeightByScore bool   `json:"weight_by_score,omitempty" jsonschema:"rank by mean domain score instead of occurrence count"`
}

type tagValueRollupOutput struct {
	Value         string   `json:"value" jsonschema:"the arg value"`
	Count         int      `json:"count" jsonschema:"distinct domains in the batch carrying this value"`
	AvgScore      *float64 `json:"avg_score,omitempty" jsonschema:"mean score of those domains, only when weight_by_score is set"`
	SampleDomains []string `json:"sample_domains" jsonschema:"up to 10 contributing domains"`
}

type cohortTagValuesOutput struct {
	BatchID       string                 `json:"batch_id"`
	Tag           string                 `json:"tag"`
	Arg           string                 `json:"arg"`
	MinCount      int                    `json:"min_count"`
	WeightByScore bool                   `json:"weight_by_score,omitempty"`
	Values        []tagValueRollupOutput `json:"values" jsonschema:"values ranked by count, or by avg_score when weight_by_score is set"`
}

func registerCohortTagValues(srv *mcp.Server, api *apiClient) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "cohort_tag_values",
		Description: "Roll up the values an argument takes across a completed batch: for the given (tag, arg), " +
			"how often each value appears and which domains carry it. Ranked by count, or by mean domain score " +
			"when weight_by_score is set. The (tag, arg) pair must come from the log-args inventory at " +
			"docs/specifications/log-args-inventory.json; use spec_list_testcases and spec_get_testcase to find " +
			"which tags a module emits. Examples: tag=N16_HAS_NSID arg=nsid (NSID strings in use), " +
			"tag=IPV4_ONE_ASN arg=asn (which AS serves each domain). List-valued args are unpacked: an entry whose " +
			"nameservers arg holds [\"a\",\"b\",\"c\"] contributes one count to each of a, b, and c.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in cohortTagValuesInput) (*mcp.CallToolResult, cohortTagValuesOutput, error) {
		id := strings.TrimSpace(in.BatchID)
		if id == "" {
			return nil, cohortTagValuesOutput{}, errors.New("batch_id is required")
		}
		tag := strings.TrimSpace(in.Tag)
		if tag == "" {
			return nil, cohortTagValuesOutput{}, errors.New("tag is required")
		}
		arg := strings.TrimSpace(in.Arg)
		if arg == "" {
			return nil, cohortTagValuesOutput{}, errors.New("arg is required")
		}
		q := url.Values{}
		q.Set("tag", tag)
		q.Set("arg", arg)
		if in.MinCount > 0 {
			q.Set("min_count", strconv.Itoa(in.MinCount))
		}
		if in.Limit > 0 {
			q.Set("limit", strconv.Itoa(in.Limit))
		}
		if in.WeightByScore {
			q.Set("weight_by_score", "true")
		}
		v, err := api.getBatchTagValues(ctx, id, q)
		if err != nil {
			return nil, cohortTagValuesOutput{}, toolError("get batch tag values", err)
		}
		out := cohortTagValuesOutput{BatchID: v.BatchID, Tag: v.Tag, Arg: v.Arg, MinCount: v.MinCount, WeightByScore: v.WeightByScore}
		out.Values = make([]tagValueRollupOutput, 0, len(v.Values))
		for _, row := range v.Values {
			out.Values = append(out.Values, tagValueRollupOutput{
				Value: row.Value, Count: row.Count, AvgScore: row.AvgScore, SampleDomains: row.SampleDomains,
			})
		}
		return nil, out, nil
	})
}

const maxFailureEntries = 5000

type failuresByTagInput struct {
	BatchID     string `json:"batch_id" jsonschema:"the batch id"`
	SeverityMin string `json:"severity_min,omitempty" jsonschema:"lowest level to include: NOTICE, WARNING (default), ERROR, CRITICAL"`
	Limit       int    `json:"limit,omitempty" jsonschema:"max ranked tags to return (default 20)"`
}

type tagFailure struct {
	Tag            string   `json:"tag"`
	WorstLevel     string   `json:"worst_level,omitempty"`
	Count          int      `json:"count"`
	ExampleDomains []string `json:"example_domains,omitempty"`
}

type failuresByTagOutput struct {
	BatchID     string       `json:"batch_id"`
	SeverityMin string       `json:"severity_min"`
	Scanned     int          `json:"scanned" jsonschema:"number of entries examined"`
	Capped      bool         `json:"capped" jsonschema:"true when the scan hit its entry cap and results are partial"`
	Tags        []tagFailure `json:"tags" jsonschema:"tags ranked by occurrence, highest first"`
}

func registerFailuresByTag(srv *mcp.Server, api *apiClient) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "failures_by_tag",
		Description: "Rank the message tags driving failures in a batch, at or above a severity, with example domains.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in failuresByTagInput) (*mcp.CallToolResult, failuresByTagOutput, error) {
		id := strings.TrimSpace(in.BatchID)
		if id == "" {
			return nil, failuresByTagOutput{}, errors.New("batch_id is required")
		}
		minLevel := strings.ToUpper(strings.TrimSpace(in.SeverityMin))
		if minLevel == "" {
			minLevel = "WARNING"
		}
		minRank, ok := levelRank[minLevel]
		if !ok {
			return nil, failuresByTagOutput{}, errors.New("severity_min must be one of DEBUG, INFO, NOTICE, WARNING, ERROR, CRITICAL")
		}
		topN := in.Limit
		if topN <= 0 {
			topN = 20
		}

		counts := map[string]int{}
		worst := map[string]string{}
		examples := map[string][]string{}
		seenDomain := map[string]map[string]bool{}
		scanned := 0
		capped := false

		// The entries level filter is exact, so query each qualifying level. WARNING+
		// is a small fraction of all entries, so this stays cheap.
		for level, rank := range levelRank {
			if rank < minRank {
				continue
			}
			q := url.Values{}
			q.Set("batch", id)
			q.Set("level", level)
			q.Set("limit", "500")
			for offset := 0; ; offset += 500 {
				if scanned >= maxFailureEntries {
					capped = true
					break
				}
				q.Set("offset", strconv.Itoa(offset))
				list, err := api.listEntries(ctx, q)
				if err != nil {
					return nil, failuresByTagOutput{}, toolError("list entries", err)
				}
				for _, e := range list.Items {
					scanned++
					counts[e.Tag]++
					if levelRank[e.Level] > levelRank[worst[e.Tag]] {
						worst[e.Tag] = e.Level
					}
					if e.Domain != "" {
						if seenDomain[e.Tag] == nil {
							seenDomain[e.Tag] = map[string]bool{}
						}
						if !seenDomain[e.Tag][e.Domain] && len(examples[e.Tag]) < 3 {
							seenDomain[e.Tag][e.Domain] = true
							examples[e.Tag] = append(examples[e.Tag], e.Domain)
						}
					}
				}
				if len(list.Items) < 500 {
					break
				}
			}
			if capped {
				break
			}
		}

		ranked := make([]tagFailure, 0, len(counts))
		for tag, n := range counts {
			ranked = append(ranked, tagFailure{Tag: tag, WorstLevel: worst[tag], Count: n, ExampleDomains: examples[tag]})
		}
		sort.Slice(ranked, func(i, j int) bool {
			if ranked[i].Count != ranked[j].Count {
				return ranked[i].Count > ranked[j].Count
			}
			return ranked[i].Tag < ranked[j].Tag
		})
		if len(ranked) > topN {
			ranked = ranked[:topN]
		}
		return nil, failuresByTagOutput{BatchID: id, SeverityMin: minLevel, Scanned: scanned, Capped: capped, Tags: ranked}, nil
	})
}
