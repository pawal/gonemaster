package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerBatchTools(srv *mcp.Server, api *apiClient) {
	registerBatchGet(srv, api)
	registerBatchList(srv, api)
	registerCohortStats(srv, api)
	registerCohortTagValues(srv, api)
	registerFailuresByTag(srv, api)
	registerCohortReport(srv, api)
}

type batchListInput struct {
	Label string `json:"label,omitempty" jsonschema:"filter to batches whose tag contains this substring"`
	Limit int    `json:"limit,omitempty" jsonschema:"max batches to return (default 20, max 100)"`
}

type batchListItemOutput struct {
	BatchID     string `json:"batch_id"`
	Tag         string `json:"tag,omitempty" jsonschema:"the cohort tag this batch was built from"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status" jsonschema:"done or running"`
	Total       int    `json:"total" jsonschema:"domains in the batch"`
	Completed   int    `json:"completed" jsonschema:"domains with a completed run"`
	Completion  int    `json:"completion" jsonschema:"percent complete, 0-100"`
	CreatedAt   string `json:"created_at,omitempty"`
	FinishedAt  string `json:"finished_at,omitempty"`
}

type batchListOutput struct {
	Count   int                   `json:"count"`
	Total   int                   `json:"total" jsonschema:"total batches on the server, before limit"`
	Batches []batchListItemOutput `json:"batches" jsonschema:"recent batches, newest first"`
}

func registerBatchList(srv *mcp.Server, api *apiClient) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "batch_list",
		Description: "List recent batches (cohort runs), newest first, with status and completion. " +
			"Use to discover a batch_id (for example the most recent TLD run) to feed the other cohort " +
			"tools. The optional label filters by a substring of the batch tag.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in batchListInput) (*mcp.CallToolResult, batchListOutput, error) {
		q := url.Values{}
		if label := strings.TrimSpace(in.Label); label != "" {
			q.Set("label", label)
		}
		limit := in.Limit
		if limit <= 0 {
			limit = 20
		}
		q.Set("limit", strconv.Itoa(limit))
		v, err := api.listBatches(ctx, q)
		if err != nil {
			return nil, batchListOutput{}, toolError("list batches", err)
		}
		out := batchListOutput{Total: v.Total, Batches: []batchListItemOutput{}}
		for _, b := range v.Items {
			item := batchListItemOutput{
				BatchID: b.BatchID, Tag: b.Tag, Description: b.Description, Status: b.Status,
				Total: b.Total, Completed: b.Completed, Completion: b.Completion,
			}
			if !b.CreatedAt.IsZero() {
				item.CreatedAt = b.CreatedAt.UTC().Format(time.RFC3339)
			}
			if b.FinishedAt != nil && !b.FinishedAt.IsZero() {
				item.FinishedAt = b.FinishedAt.UTC().Format(time.RFC3339)
			}
			out.Batches = append(out.Batches, item)
		}
		out.Count = len(out.Batches)
		return nil, out, nil
	})
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

// Caps on the report tool's lists, so one call cannot page a whole cohort.
const (
	defaultReportRows = 20
	maxReportRows     = 200
)

type cohortReportInput struct {
	DatasetTag string `json:"dataset_tag,omitempty" jsonschema:"the cohort's dataset tag; the default public cohort when omitted"`
	From       string `json:"from,omitempty" jsonschema:"baseline snapshot slug; the snapshot before to when omitted"`
	To         string `json:"to,omitempty" jsonschema:"later snapshot slug; the newest snapshot when omitted"`
	Limit      int    `json:"limit,omitempty" jsonschema:"max tag rows per list and max mover rows (default 20, max 200)"`
	MinCluster int    `json:"min_cluster,omitempty" jsonschema:"domains a cluster needs (server default 3)"`
	MaxSpread  int    `json:"max_spread,omitempty" jsonschema:"score spread a cluster allows (server default 3)"`
}

type cohortReportProvenance struct {
	FromEngineVersion     string `json:"from_engine_version,omitempty"`
	ToEngineVersion       string `json:"to_engine_version,omitempty"`
	VocabularyKnown       bool   `json:"vocabulary_known" jsonschema:"false means no change can be attributed to the engine"`
	TagsAddedToEngine     int    `json:"tags_added_to_engine"`
	TagsRemovedFromEngine int    `json:"tags_removed_from_engine"`
	TagsReclassified      int    `json:"tags_reclassified"`
	ScoringConfigChanged  string `json:"scoring_config_changed" jsonschema:"true, false or unknown"`
	TagFloor              string `json:"tag_floor,omitempty" jsonschema:"findings below this level are absent from the per-domain lists"`
	FromProfile           string `json:"from_profile,omitempty"`
	ToProfile             string `json:"to_profile,omitempty"`
}

type cohortReportTotals struct {
	FromDomainCount  int            `json:"from_domain_count"`
	ToDomainCount    int            `json:"to_domain_count"`
	BothDomainCount  int            `json:"both_domain_count"`
	Added            int            `json:"added"`
	Removed          int            `json:"removed"`
	IdenticalScore   int            `json:"identical_score"`
	Improved         int            `json:"improved"`
	Regressed        int            `json:"regressed"`
	FromMeanScore    *float64       `json:"from_mean_score,omitempty"`
	ToMeanScore      *float64       `json:"to_mean_score,omitempty"`
	FromGrades       map[string]int `json:"from_grades,omitempty"`
	ToGrades         map[string]int `json:"to_grades,omitempty"`
	DomainCategories map[string]int `json:"domain_categories,omitempty" jsonschema:"movers per cause: real, measurement, mixed, unknown"`
}

type cohortReportTag struct {
	Tag            string `json:"tag"`
	Module         string `json:"module,omitempty"`
	FromLevel      string `json:"from_level,omitempty"`
	ToLevel        string `json:"to_level,omitempty"`
	DomainDelta    int    `json:"domain_delta"`
	Classification string `json:"classification" jsonschema:"new_in_engine, removed_from_engine, level_reclassified, cohort_change or unknown"`
}

type cohortReportMover struct {
	Domain           string   `json:"domain"`
	FromScore        *int     `json:"from_score,omitempty"`
	ToScore          *int     `json:"to_score,omitempty"`
	ScoreDelta       *int     `json:"score_delta,omitempty"`
	FromGrade        string   `json:"from_grade,omitempty"`
	ToGrade          string   `json:"to_grade,omitempty"`
	Category         string   `json:"category" jsonschema:"real, measurement, mixed or unknown"`
	ExplainedDelta   int      `json:"explained_delta" jsonschema:"score movement the listed findings account for"`
	UnexplainedDelta int      `json:"unexplained_delta" jsonschema:"non-zero names a cause the report cannot see"`
	Appeared         []string `json:"appeared,omitempty" jsonschema:"tags that appeared on this domain"`
	Cleared          []string `json:"cleared,omitempty" jsonschema:"tags that cleared on this domain"`
}

type cohortReportCluster struct {
	Dimensions []string `json:"dimensions" jsonschema:"the shared values, as dimension=value (n of total)"`
	Domains    []string `json:"domains"`
	Size       int      `json:"size"`
	MinDelta   int      `json:"min_delta"`
	MaxDelta   int      `json:"max_delta"`
	Direction  string   `json:"direction" jsonschema:"improved or regressed"`
}

type cohortReportOutput struct {
	DatasetTag       string                 `json:"dataset_tag"`
	FromSlug         string                 `json:"from_slug"`
	ToSlug           string                 `json:"to_slug"`
	Provenance       cohortReportProvenance `json:"provenance"`
	Totals           cohortReportTotals     `json:"totals"`
	TagsAppeared     []cohortReportTag      `json:"tags_appeared"`
	TagsCleared      []cohortReportTag      `json:"tags_cleared"`
	TagsLevelChanged []cohortReportTag      `json:"tags_level_changed"`
	Movers           []cohortReportMover    `json:"movers" jsonschema:"biggest score movements first, in either direction"`
	Clusters         []cohortReportCluster  `json:"clusters" jsonschema:"movers that moved together and share a nameserver, ASN, prefix or software version"`
	Truncated        bool                   `json:"truncated" jsonschema:"true when a list was cut to limit"`
}

// resolveReportPair fills the dataset tag from the catalog and a missing
// slug from the snapshot list, newest first.
func resolveReportPair(ctx context.Context, api *apiClient, in cohortReportInput) (string, string, string, error) {
	datasetTag := strings.TrimSpace(in.DatasetTag)
	if datasetTag == "" {
		catalog, err := api.getAnalysisCatalog(ctx)
		if err != nil {
			return "", "", "", toolError("get analysis catalog", err)
		}
		datasetTag = catalog.DefaultTag
		if datasetTag == "" && len(catalog.Cohorts) == 1 {
			datasetTag = catalog.Cohorts[0].DatasetTag
		}
		if datasetTag == "" {
			names := make([]string, 0, len(catalog.Cohorts))
			for _, c := range catalog.Cohorts {
				names = append(names, c.DatasetTag)
			}
			if len(names) == 0 {
				return "", "", "", errors.New("no public analysis cohort is available")
			}
			return "", "", "", errors.New("dataset_tag is required; available: " + strings.Join(names, ", "))
		}
	}
	from, to := strings.TrimSpace(in.From), strings.TrimSpace(in.To)
	if from != "" && to != "" {
		return datasetTag, from, to, nil
	}
	list, err := api.listAnalysisSnapshots(ctx, datasetTag)
	if err != nil {
		return "", "", "", toolError("list snapshots", err)
	}
	if len(list.Snapshots) < 2 {
		return "", "", "", errors.New("cohort " + datasetTag + " has fewer than two snapshots to compare")
	}
	if to == "" {
		to = list.Snapshots[0].Slug
	}
	if from != "" {
		return datasetTag, from, to, nil
	}
	for i, snap := range list.Snapshots {
		if snap.Slug == to && i+1 < len(list.Snapshots) {
			return datasetTag, list.Snapshots[i+1].Slug, to, nil
		}
	}
	return "", "", "", errors.New("no snapshot precedes " + to + " in cohort " + datasetTag)
}

// rankTags puts the largest movement first, so a cut list keeps the rows
// worth reading.
func rankTags(entries []reportTagEntryView, limit int) ([]cohortReportTag, bool) {
	sorted := append([]reportTagEntryView(nil), entries...)
	sort.Slice(sorted, func(i, j int) bool {
		di, dj := abs(sorted[i].DomainDelta), abs(sorted[j].DomainDelta)
		if di != dj {
			return di > dj
		}
		return sorted[i].Tag < sorted[j].Tag
	})
	truncated := len(sorted) > limit
	if truncated {
		sorted = sorted[:limit]
	}
	out := make([]cohortReportTag, 0, len(sorted))
	for _, e := range sorted {
		out = append(out, cohortReportTag{
			Tag: e.Tag, Module: e.Module, FromLevel: e.FromLevel, ToLevel: e.ToLevel,
			DomainDelta: e.DomainDelta, Classification: e.Classification,
		})
	}
	return out, truncated
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func deltaOrZero(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

// rankMovers orders by the size of the score movement in either direction.
func rankMovers(domains []reportDomainView, limit int) ([]cohortReportMover, bool) {
	sorted := append([]reportDomainView(nil), domains...)
	sort.Slice(sorted, func(i, j int) bool {
		di, dj := abs(deltaOrZero(sorted[i].ScoreDelta)), abs(deltaOrZero(sorted[j].ScoreDelta))
		if di != dj {
			return di > dj
		}
		return sorted[i].Domain < sorted[j].Domain
	})
	truncated := len(sorted) > limit
	if truncated {
		sorted = sorted[:limit]
	}
	out := make([]cohortReportMover, 0, len(sorted))
	for _, d := range sorted {
		mover := cohortReportMover{
			Domain: d.Domain, FromScore: d.FromScore, ToScore: d.ToScore, ScoreDelta: d.ScoreDelta,
			FromGrade: d.FromGrade, ToGrade: d.ToGrade, Category: d.Category,
			ExplainedDelta: d.ExplainedDelta, UnexplainedDelta: d.UnexplainedDelta,
		}
		for _, t := range d.Appeared {
			mover.Appeared = append(mover.Appeared, t.Tag+" ("+t.Classification+")")
		}
		for _, t := range d.Cleared {
			mover.Cleared = append(mover.Cleared, t.Tag+" ("+t.Classification+")")
		}
		out = append(out, mover)
	}
	return out, truncated
}

func registerCohortReport(srv *mcp.Server, api *apiClient) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "cohort_report",
		Description: "Compare two snapshots of an analysis cohort and classify every change as engine-driven " +
			"or real. Returns the provenance of both sides (engine version, tag vocabulary delta, scoring " +
			"configuration state, tag floor), what moved cohort-wide, each moving domain with its cause, and " +
			"clusters of domains that moved together behind a shared nameserver, ASN, prefix or software " +
			"version. Answers \"did the cohort get worse, or did the engine start looking harder?\": a tag " +
			"classified new_in_engine appeared because the engine gained it, a cohort_change tag appeared " +
			"because the domains changed. With no arguments it compares the two newest snapshots of the " +
			"default cohort. Snapshots are keyed by batch, so this is also the batch-to-batch comparison.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in cohortReportInput) (*mcp.CallToolResult, cohortReportOutput, error) {
		datasetTag, from, to, err := resolveReportPair(ctx, api, in)
		if err != nil {
			return nil, cohortReportOutput{}, err
		}
		limit := in.Limit
		if limit <= 0 {
			limit = defaultReportRows
		}
		if limit > maxReportRows {
			limit = maxReportRows
		}
		q := url.Values{}
		q.Set("from", from)
		q.Set("to", to)
		if in.MinCluster > 0 {
			q.Set("min_cluster", strconv.Itoa(in.MinCluster))
		}
		if in.MaxSpread > 0 {
			q.Set("max_spread", strconv.Itoa(in.MaxSpread))
		}
		report, err := api.getAnalysisReport(ctx, datasetTag, q)
		if err != nil {
			return nil, cohortReportOutput{}, toolError("get cohort report", err)
		}

		vocab := report.Header.Vocabulary
		out := cohortReportOutput{
			DatasetTag: report.DatasetTag, FromSlug: report.FromSlug, ToSlug: report.ToSlug,
			Provenance: cohortReportProvenance{
				FromEngineVersion:     report.Header.From.EngineVersion,
				ToEngineVersion:       report.Header.To.EngineVersion,
				VocabularyKnown:       vocab.FromAvailable && vocab.ToAvailable,
				TagsAddedToEngine:     len(vocab.Added),
				TagsRemovedFromEngine: len(vocab.Removed),
				TagsReclassified:      len(vocab.LevelChanged),
				ScoringConfigChanged:  report.Header.ScoringConfigChanged,
				TagFloor:              report.Header.TagFloor,
				FromProfile:           report.Header.From.ProfileName,
				ToProfile:             report.Header.To.ProfileName,
			},
			Totals: cohortReportTotals{
				FromDomainCount: report.Totals.FromDomainCount, ToDomainCount: report.Totals.ToDomainCount,
				BothDomainCount: report.Totals.BothDomainCount, Added: report.Totals.Added,
				Removed: report.Totals.Removed, IdenticalScore: report.Totals.IdenticalScore,
				Improved: report.Totals.Improved, Regressed: report.Totals.Regressed,
				FromMeanScore: report.Totals.FromMeanScore, ToMeanScore: report.Totals.ToMeanScore,
				FromGrades: report.Totals.FromGrades, ToGrades: report.Totals.ToGrades,
				DomainCategories: report.Totals.DomainCategories,
			},
		}
		var cut bool
		out.TagsAppeared, cut = rankTags(report.Tags.Appeared, limit)
		out.Truncated = out.Truncated || cut
		out.TagsCleared, cut = rankTags(report.Tags.Cleared, limit)
		out.Truncated = out.Truncated || cut
		out.TagsLevelChanged, cut = rankTags(report.Tags.LevelChanged, limit)
		out.Truncated = out.Truncated || cut
		out.Movers, cut = rankMovers(report.Domains, limit)
		out.Truncated = out.Truncated || cut

		out.Clusters = make([]cohortReportCluster, 0, len(report.Clusters))
		for _, c := range report.Clusters {
			cluster := cohortReportCluster{
				Domains: c.Domains, Size: c.Size, MinDelta: c.MinDelta, MaxDelta: c.MaxDelta, Direction: c.Direction,
			}
			for _, d := range c.Dimensions {
				value := d.Label
				if value == "" {
					value = d.Value
				}
				cluster.Dimensions = append(cluster.Dimensions,
					fmt.Sprintf("%s=%s (%d of %d)", d.Dimension, value, c.Size, d.TotalDomains))
			}
			out.Clusters = append(out.Clusters, cluster)
		}
		return nil, out, nil
	})
}
