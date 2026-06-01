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
	registerCohortOperators(srv, api)
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

type cohortOperatorsInput struct {
	BatchID  string `json:"batch_id" jsonschema:"the batch id"`
	GroupBy  string `json:"group_by" jsonschema:"grouping dimension: ns_parent or asn"`
	MinCount int    `json:"min_count,omitempty" jsonschema:"suppress operators serving fewer than this many domains (default 10)"`
	Limit    int    `json:"limit,omitempty" jsonschema:"max operator rows to return (default 20, max 100)"`
}

type operatorRollupOutput struct {
	Key           string   `json:"key" jsonschema:"the NS parent domain or ASN number"`
	DomainCount   int      `json:"domain_count" jsonschema:"distinct domains in the batch served by this operator"`
	AvgScore      float64  `json:"avg_score" jsonschema:"mean score of those domains"`
	SampleDomains []string `json:"sample_domains" jsonschema:"up to 10 domains, highest score first"`
}

type cohortOperatorsOutput struct {
	BatchID   string                 `json:"batch_id"`
	GroupBy   string                 `json:"group_by"`
	MinCount  int                    `json:"min_count"`
	Operators []operatorRollupOutput `json:"operators" jsonschema:"operators ranked by avg_score, highest first"`
}

func registerCohortOperators(srv *mcp.Server, api *apiClient) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "cohort_operators",
		Description: "Top operators in a completed batch, grouped by ns_parent or asn, ranked by mean score. " +
			"Use group_by=asn for cross-TLD operator rollups: with ns_parent, per-TLD nameserver names " +
			"like dns1.nic.<tld> do not collapse across TLDs. ASN keys are numbers only, with no org-name enrichment.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in cohortOperatorsInput) (*mcp.CallToolResult, cohortOperatorsOutput, error) {
		id := strings.TrimSpace(in.BatchID)
		if id == "" {
			return nil, cohortOperatorsOutput{}, errors.New("batch_id is required")
		}
		groupBy := strings.TrimSpace(in.GroupBy)
		if groupBy != "ns_parent" && groupBy != "asn" {
			return nil, cohortOperatorsOutput{}, errors.New("group_by must be ns_parent or asn")
		}
		q := url.Values{}
		q.Set("group_by", groupBy)
		if in.MinCount > 0 {
			q.Set("min_count", strconv.Itoa(in.MinCount))
		}
		if in.Limit > 0 {
			q.Set("limit", strconv.Itoa(in.Limit))
		}
		v, err := api.getBatchOperators(ctx, id, q)
		if err != nil {
			return nil, cohortOperatorsOutput{}, toolError("get batch operators", err)
		}
		out := cohortOperatorsOutput{BatchID: v.BatchID, GroupBy: v.GroupBy, MinCount: v.MinCount}
		out.Operators = make([]operatorRollupOutput, 0, len(v.Operators))
		for _, op := range v.Operators {
			out.Operators = append(out.Operators, operatorRollupOutput{
				Key: op.Key, DomainCount: op.DomainCount, AvgScore: op.AvgScore, SampleDomains: op.SampleDomains,
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
