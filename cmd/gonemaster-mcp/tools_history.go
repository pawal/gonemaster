package main

import (
	"context"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerHistoryTools(srv *mcp.Server, api *apiClient) {
	registerRunSearch(srv, api)
	registerRunDiff(srv, api)
}

type runSearchInput struct {
	Domain         string `json:"domain,omitempty" jsonschema:"domain name or substring to match"`
	Tag            string `json:"tag,omitempty" jsonschema:"only runs that emitted this message tag"`
	Status         string `json:"status,omitempty" jsonschema:"job status: succeeded, failed, canceled, expired"`
	Level          string `json:"level,omitempty" jsonschema:"worst severity level, e.g. WARNING, ERROR, CRITICAL"`
	Grade          string `json:"grade,omitempty" jsonschema:"letter grade, e.g. A, B, C"`
	FinishedAfter  string `json:"finished_after,omitempty" jsonschema:"RFC3339 lower bound on finish time"`
	FinishedBefore string `json:"finished_before,omitempty" jsonschema:"RFC3339 upper bound on finish time"`
	Limit          int    `json:"limit,omitempty" jsonschema:"max runs to return (default 20)"`
	Offset         int    `json:"offset,omitempty" jsonschema:"pagination offset"`
}

type runSearchOutput struct {
	Count int          `json:"count"`
	Total int          `json:"total" jsonschema:"total matches on the server, before limit/offset"`
	Runs  []runSummary `json:"runs"`
}

func registerRunSearch(srv *mcp.Server, api *apiClient) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "run_search",
		Description: "Search completed runs by domain, tag, status, severity, grade, or finish-time range. Newest first.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in runSearchInput) (*mcp.CallToolResult, runSearchOutput, error) {
		q := url.Values{}
		setIf := func(k, v string) {
			if v = strings.TrimSpace(v); v != "" {
				q.Set(k, v)
			}
		}
		setIf("domain", in.Domain)
		setIf("event_tag", in.Tag)
		setIf("status", in.Status)
		setIf("level", in.Level)
		setIf("grade", in.Grade)
		setIf("finished_after", in.FinishedAfter)
		setIf("finished_before", in.FinishedBefore)
		limit := in.Limit
		if limit <= 0 {
			limit = 20
		}
		q.Set("limit", strconv.Itoa(limit))
		if in.Offset > 0 {
			q.Set("offset", strconv.Itoa(in.Offset))
		}

		list, err := api.getRuns(ctx, q)
		if err != nil {
			return nil, runSearchOutput{}, toolError("search runs", err)
		}
		out := runSearchOutput{Total: list.Total, Runs: []runSummary{}}
		for _, r := range list.Items {
			out.Runs = append(out.Runs, toRunSummary(r))
		}
		out.Count = len(out.Runs)
		return nil, out, nil
	})
}

type runDiffInput struct {
	RunA string `json:"run_a" jsonschema:"baseline run id"`
	RunB string `json:"run_b" jsonschema:"comparison run id"`
	Lang string `json:"lang,omitempty" jsonschema:"language for rendered messages (default en)"`
}

type tagDelta struct {
	Tag       string `json:"tag"`
	Module    string `json:"module,omitempty"`
	Level     string `json:"level,omitempty" jsonschema:"severity for an added or removed tag"`
	FromLevel string `json:"from_level,omitempty" jsonschema:"prior severity for a changed tag"`
	ToLevel   string `json:"to_level,omitempty" jsonschema:"new severity for a changed tag"`
}

type runDiffOutput struct {
	RunA    string     `json:"run_a"`
	RunB    string     `json:"run_b"`
	GradeA  string     `json:"grade_a,omitempty"`
	GradeB  string     `json:"grade_b,omitempty"`
	Added   []tagDelta `json:"added" jsonschema:"tags present in run_b but not run_a"`
	Removed []tagDelta `json:"removed" jsonschema:"tags present in run_a but not run_b"`
	Changed []tagDelta `json:"changed" jsonschema:"tags in both runs whose severity changed"`
}

func registerRunDiff(srv *mcp.Server, api *apiClient) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "run_diff",
		Description: "Compare two runs at the tag level: which tags were added, removed, or changed severity.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in runDiffInput) (*mcp.CallToolResult, runDiffOutput, error) {
		a := strings.TrimSpace(in.RunA)
		b := strings.TrimSpace(in.RunB)
		if a == "" || b == "" {
			return nil, runDiffOutput{}, errors.New("run_a and run_b are required")
		}
		lang := strings.TrimSpace(in.Lang)
		if lang == "" {
			lang = "en"
		}
		resA, err := api.getResult(ctx, a, lang)
		if err != nil {
			return nil, runDiffOutput{}, toolError("get run_a", err)
		}
		resB, err := api.getResult(ctx, b, lang)
		if err != nil {
			return nil, runDiffOutput{}, toolError("get run_b", err)
		}

		ta, tb := worstLevelByTag(resA), worstLevelByTag(resB)
		out := runDiffOutput{RunA: a, RunB: b, Added: []tagDelta{}, Removed: []tagDelta{}, Changed: []tagDelta{}}
		if resA.Score != nil {
			out.GradeA = resA.Score.Grade
		}
		if resB.Score != nil {
			out.GradeB = resB.Score.Grade
		}
		for tag, ib := range tb {
			ia, ok := ta[tag]
			if !ok {
				out.Added = append(out.Added, tagDelta{Tag: tag, Module: ib.module, Level: ib.level})
			} else if ia.level != ib.level {
				out.Changed = append(out.Changed, tagDelta{Tag: tag, Module: ib.module, FromLevel: ia.level, ToLevel: ib.level})
			}
		}
		for tag, ia := range ta {
			if _, ok := tb[tag]; !ok {
				out.Removed = append(out.Removed, tagDelta{Tag: tag, Module: ia.module, Level: ia.level})
			}
		}
		sortDeltas(out.Added)
		sortDeltas(out.Removed)
		sortDeltas(out.Changed)
		return nil, out, nil
	})
}

var levelRank = map[string]int{"DEBUG": 1, "INFO": 2, "NOTICE": 3, "WARNING": 4, "ERROR": 5, "CRITICAL": 6}

type tagInfo struct {
	module string
	level  string
}

// worstLevelByTag collapses a run's entries to one entry per tag, keeping the
// highest-severity level seen for that tag.
func worstLevelByTag(res resultView) map[string]tagInfo {
	m := map[string]tagInfo{}
	if res.Raw == nil {
		return m
	}
	for _, e := range res.Raw.Entries {
		cur, ok := m[e.Tag]
		if !ok || levelRank[e.Level] > levelRank[cur.level] {
			m[e.Tag] = tagInfo{module: e.Module, level: e.Level}
		}
	}
	return m
}

func sortDeltas(d []tagDelta) {
	sort.Slice(d, func(i, j int) bool { return d[i].Tag < d[j].Tag })
}
