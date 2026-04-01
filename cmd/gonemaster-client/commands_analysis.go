package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"flag"
)

// ── Types ────────────────────────────────────────────────────────────────────

type domain struct {
	ID           int64    `json:"id"`
	Name         string   `json:"name"`
	LatestRunID  string   `json:"latest_run_id,omitempty"`
	LatestRunAt  string   `json:"latest_run_at,omitempty"`
	LatestLevel  string   `json:"latest_level,omitempty"`
	LatestStatus string   `json:"latest_status,omitempty"`
	RunCount     int      `json:"run_count"`
	Tags         []string `json:"tags,omitempty"`
}

type domainList struct {
	Items []domain `json:"items"`
	Total int      `json:"total"`
}

type tag struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	DomainCount int    `json:"domain_count"`
}

type tagList struct {
	Items []tag `json:"items"`
	Total int   `json:"total"`
}

type tagSummary struct {
	Tag         string `json:"tag"`
	DomainCount int    `json:"domain_count"`
	OK          int    `json:"ok"`
	Notice      int    `json:"notice"`
	Warning     int    `json:"warning"`
	Error       int    `json:"error"`
	Critical    int    `json:"critical"`
}

type runRecord struct {
	ID          string `json:"id"`
	Domain      string `json:"domain"`
	BatchID     string `json:"batch_id,omitempty"`
	Status      string `json:"status"`
	DurationMs  int64  `json:"duration_ms,omitempty"`
	SevNotice   int    `json:"sev_notice"`
	SevWarning  int    `json:"sev_warning"`
	SevError    int    `json:"sev_error"`
	SevCritical int    `json:"sev_critical"`
	WorstLevel  string `json:"worst_level,omitempty"`
	EntryCount  int    `json:"entry_count"`
	FinishedAt  string `json:"finished_at,omitempty"`
}

type runList struct {
	Items []runRecord `json:"items"`
	Total int         `json:"total"`
}

type entry struct {
	ID       int64          `json:"id"`
	RunID    string         `json:"run_id"`
	DomainID int64          `json:"domain_id"`
	Domain   string         `json:"domain,omitempty"`
	Module   string         `json:"module"`
	Testcase string         `json:"testcase"`
	Tag      string         `json:"tag"`
	Level    string         `json:"level"`
	Args     map[string]any `json:"args,omitempty"`
}

type entryList struct {
	Items []entry `json:"items"`
	Total int     `json:"total"`
}

// ── HTTP helpers ─────────────────────────────────────────────────────────────

// doBytes performs a GET request and returns the raw response body.
// Used for CSV responses from the server.
func (c *apiClient) doBytes(ctx context.Context, path string) ([]byte, error) {
	full := strings.TrimRight(c.baseURL, "/") + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return nil, err
	}
	for name, values := range c.headers {
		for _, v := range values {
			req.Header.Add(name, v)
		}
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return data, nil
}

// ── Domain resolution helper ──────────────────────────────────────────────────

// resolveDomain looks up a domain by exact name via GET /domains?name=<name>.
func resolveDomain(ctx context.Context, client *apiClient, name string) (domain, error) {
	path := "/domains?name=" + url.QueryEscape(name) + "&limit=20"
	var list domainList
	if err := client.doJSON(ctx, http.MethodGet, path, nil, &list); err != nil {
		return domain{}, err
	}
	for _, d := range list.Items {
		if d.Name == name {
			return d, nil
		}
	}
	return domain{}, fmt.Errorf("domain not found: %s", name)
}

// ── domains ──────────────────────────────────────────────────────────────────

func runDomains(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "domains subcommand is required: list|get|runs|tag|untag")
		return 2
	}
	cmd := args[0]
	args = args[1:]
	switch cmd {
	case "list":
		return runDomainsList(ctx, client, opts, args, out, errOut)
	case "get":
		return runDomainsGet(ctx, client, opts, args, out, errOut)
	case "runs":
		return runDomainsRuns(ctx, client, opts, args, out, errOut)
	case "tag":
		return runDomainsTag(ctx, client, opts, args, out, errOut)
	case "untag":
		return runDomainsUntag(ctx, client, opts, args, out, errOut)
	default:
		fmt.Fprintf(errOut, "Unknown domains command %q\n", cmd)
		return 2
	}
}

func runDomainsList(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	fs := flag.NewFlagSet("domains list", flag.ContinueOnError)
	fs.SetOutput(errOut)
	setSubcommandUsage(fs)
	var tag, level, name string
	var limit int
	fs.StringVar(&tag, "tag", "", "Filter by tag name")
	fs.StringVar(&level, "level", "", "Filter by latest level")
	fs.StringVar(&name, "name", "", "Filter by domain name substring")
	fs.IntVar(&limit, "limit", 100, "Max results")
	if err := parseWithReorderedFlags(fs, args); err != nil {
		return 2
	}
	q := url.Values{}
	if tag != "" {
		q.Set("tag", tag)
	}
	if level != "" {
		q.Set("level", level)
	}
	if name != "" {
		q.Set("name", name)
	}
	if limit != 100 {
		q.Set("limit", fmt.Sprintf("%d", limit))
	}
	path := "/domains"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var list domainList
	if err := client.doJSON(ctx, http.MethodGet, path, nil, &list); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if opts.format != "pretty" {
		if err := writeOutput(out, opts.format, list); err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 2
		}
		return 0
	}
	fmt.Fprintf(out, "Domains: %d\n", list.Total)
	for _, d := range list.Items {
		level := d.LatestLevel
		if level == "" {
			level = "-"
		}
		runAt := d.LatestRunAt
		if len(runAt) > 10 {
			runAt = runAt[:10] // date only
		}
		if runAt == "" {
			runAt = "-"
		}
		tags := ""
		if len(d.Tags) > 0 {
			tags = " [" + strings.Join(d.Tags, ", ") + "]"
		}
		fmt.Fprintf(out, "  %-40s  level=%-8s  last=%-10s  runs=%-4d%s\n",
			d.Name, level, runAt, d.RunCount, tags)
	}
	return 0
}

func runDomainsGet(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	fs := flag.NewFlagSet("domains get", flag.ContinueOnError)
	fs.SetOutput(errOut)
	setSubcommandUsage(fs)
	if err := parseWithReorderedFlags(fs, args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprintln(errOut, "domain name is required")
		return 2
	}
	name := strings.TrimSpace(rest[0])
	d, err := resolveDomain(ctx, client, name)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	// Fetch full detail with tags.
	var full domain
	if err := client.doJSON(ctx, http.MethodGet, fmt.Sprintf("/domains/%d", d.ID), nil, &full); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if opts.format != "pretty" {
		if err := writeOutput(out, opts.format, full); err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 2
		}
		return 0
	}
	fmt.Fprintf(out, "Domain: %s\n", full.Name)
	fmt.Fprintf(out, "  ID: %d\n", full.ID)
	fmt.Fprintf(out, "  Run count: %d\n", full.RunCount)
	if full.LatestLevel != "" {
		fmt.Fprintf(out, "  Latest level: %s\n", full.LatestLevel)
	}
	if full.LatestStatus != "" {
		fmt.Fprintf(out, "  Latest status: %s\n", full.LatestStatus)
	}
	if len(full.Tags) > 0 {
		fmt.Fprintf(out, "  Tags: %s\n", strings.Join(full.Tags, ", "))
	}
	return 0
}

func runDomainsRuns(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	fs := flag.NewFlagSet("domains runs", flag.ContinueOnError)
	fs.SetOutput(errOut)
	setSubcommandUsage(fs)
	var limit int
	fs.IntVar(&limit, "limit", 20, "Max results")
	if err := parseWithReorderedFlags(fs, args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprintln(errOut, "domain name is required")
		return 2
	}
	name := strings.TrimSpace(rest[0])
	d, err := resolveDomain(ctx, client, name)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	path := fmt.Sprintf("/domains/%d/runs?limit=%d", d.ID, limit)
	var list runList
	if err := client.doJSON(ctx, http.MethodGet, path, nil, &list); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if opts.format != "pretty" {
		if err := writeOutput(out, opts.format, list); err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 2
		}
		return 0
	}
	fmt.Fprintf(out, "Runs for %s: %d\n", name, list.Total)
	for _, r := range list.Items {
		dur := ""
		if r.DurationMs > 0 {
			dur = fmt.Sprintf("  %dms", r.DurationMs)
		}
		level := r.WorstLevel
		if level == "" {
			level = "-"
		}
		fmt.Fprintf(out, "  %s  %-10s  level=%-8s%s\n", r.ID, r.Status, level, dur)
	}
	return 0
}

func runDomainsTag(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	fs := flag.NewFlagSet("domains tag", flag.ContinueOnError)
	fs.SetOutput(errOut)
	setSubcommandUsage(fs)
	if err := parseWithReorderedFlags(fs, args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) < 2 {
		fmt.Fprintln(errOut, "usage: domains tag <domain> <tag...>")
		return 2
	}
	domainName := rest[0]
	tags := rest[1:]
	body := map[string][]string{"domains": {domainName}}
	for _, t := range tags {
		if err := client.doJSON(ctx, http.MethodPost, "/tags/"+url.PathEscape(t)+"/domains", body, nil); err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 2
		}
	}
	if opts.format == "pretty" {
		fmt.Fprintf(out, "Tagged %s with: %s\n", domainName, strings.Join(tags, ", "))
	}
	return 0
}

func runDomainsUntag(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	fs := flag.NewFlagSet("domains untag", flag.ContinueOnError)
	fs.SetOutput(errOut)
	setSubcommandUsage(fs)
	if err := parseWithReorderedFlags(fs, args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) < 2 {
		fmt.Fprintln(errOut, "usage: domains untag <domain> <tag...>")
		return 2
	}
	domainName := rest[0]
	tags := rest[1:]
	body := map[string][]string{"domains": {domainName}}
	for _, t := range tags {
		if err := client.doJSON(ctx, http.MethodDelete, "/tags/"+url.PathEscape(t)+"/domains", body, nil); err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 2
		}
	}
	if opts.format == "pretty" {
		fmt.Fprintf(out, "Removed %s from tags: %s\n", domainName, strings.Join(tags, ", "))
	}
	return 0
}

// ── tags ──────────────────────────────────────────────────────────────────────

func runTags(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "tags subcommand is required: list|create|delete|domains|summary|add-domains")
		return 2
	}
	cmd := args[0]
	args = args[1:]
	switch cmd {
	case "list":
		return runTagsList(ctx, client, opts, args, out, errOut)
	case "create":
		return runTagsCreate(ctx, client, opts, args, out, errOut)
	case "delete":
		return runTagsDelete(ctx, client, opts, args, out, errOut)
	case "domains":
		return runTagsDomains(ctx, client, opts, args, out, errOut)
	case "summary":
		return runTagsSummary(ctx, client, opts, args, out, errOut)
	case "add-domains":
		return runTagsAddDomains(ctx, client, opts, args, out, errOut)
	default:
		fmt.Fprintf(errOut, "Unknown tags command %q\n", cmd)
		return 2
	}
}

func runTagsList(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	fs := flag.NewFlagSet("tags list", flag.ContinueOnError)
	fs.SetOutput(errOut)
	setSubcommandUsage(fs)
	if err := parseWithReorderedFlags(fs, args); err != nil {
		return 2
	}
	var list []tag
	if err := client.doJSON(ctx, http.MethodGet, "/tags", nil, &list); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if opts.format != "pretty" {
		if err := writeOutput(out, opts.format, list); err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 2
		}
		return 0
	}
	fmt.Fprintf(out, "Tags: %d\n", len(list))
	for _, t := range list {
		desc := ""
		if t.Description != "" {
			desc = "  — " + t.Description
		}
		fmt.Fprintf(out, "  %-30s  domains=%-6d%s\n", t.Name, t.DomainCount, desc)
	}
	return 0
}

func runTagsCreate(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	fs := flag.NewFlagSet("tags create", flag.ContinueOnError)
	fs.SetOutput(errOut)
	setSubcommandUsage(fs)
	var description string
	fs.StringVar(&description, "description", "", "Tag description")
	if err := parseWithReorderedFlags(fs, args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprintln(errOut, "tag name is required")
		return 2
	}
	name := strings.TrimSpace(rest[0])
	body := map[string]string{"name": name, "description": description}
	var t tag
	if err := client.doJSON(ctx, http.MethodPost, "/tags", body, &t); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if opts.format != "pretty" {
		if err := writeOutput(out, opts.format, t); err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 2
		}
		return 0
	}
	fmt.Fprintf(out, "Created tag %q\n", t.Name)
	return 0
}

func runTagsDelete(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	fs := flag.NewFlagSet("tags delete", flag.ContinueOnError)
	fs.SetOutput(errOut)
	setSubcommandUsage(fs)
	if err := parseWithReorderedFlags(fs, args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprintln(errOut, "tag name is required")
		return 2
	}
	name := rest[0]
	if err := client.doJSON(ctx, http.MethodDelete, "/tags/"+url.PathEscape(name), nil, nil); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if opts.format == "pretty" {
		fmt.Fprintf(out, "Deleted tag %q\n", name)
	}
	return 0
}

func runTagsDomains(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	fs := flag.NewFlagSet("tags domains", flag.ContinueOnError)
	fs.SetOutput(errOut)
	setSubcommandUsage(fs)
	var limit int
	fs.IntVar(&limit, "limit", 100, "Max results")
	if err := parseWithReorderedFlags(fs, args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprintln(errOut, "tag name is required")
		return 2
	}
	name := rest[0]
	path := fmt.Sprintf("/tags/%s/domains?limit=%d", url.PathEscape(name), limit)
	var list domainList
	if err := client.doJSON(ctx, http.MethodGet, path, nil, &list); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if opts.format != "pretty" {
		if err := writeOutput(out, opts.format, list); err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 2
		}
		return 0
	}
	fmt.Fprintf(out, "Domains in tag %q: %d\n", name, list.Total)
	for _, d := range list.Items {
		level := d.LatestLevel
		if level == "" {
			level = "-"
		}
		fmt.Fprintf(out, "  %-40s  level=%-8s  runs=%d\n", d.Name, level, d.RunCount)
	}
	return 0
}

func runTagsSummary(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	fs := flag.NewFlagSet("tags summary", flag.ContinueOnError)
	fs.SetOutput(errOut)
	setSubcommandUsage(fs)
	if err := parseWithReorderedFlags(fs, args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprintln(errOut, "tag name is required")
		return 2
	}
	name := rest[0]
	var s tagSummary
	if err := client.doJSON(ctx, http.MethodGet, "/tags/"+url.PathEscape(name)+"/summary", nil, &s); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if opts.format != "pretty" {
		if err := writeOutput(out, opts.format, s); err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 2
		}
		return 0
	}
	fmt.Fprintf(out, "Tag summary: %s\n", s.Tag)
	fmt.Fprintf(out, "  Domains: %d\n", s.DomainCount)
	fmt.Fprintf(out, "  OK: %d  Notice: %d  Warning: %d  Error: %d  Critical: %d\n",
		s.OK, s.Notice, s.Warning, s.Error, s.Critical)
	return 0
}

func runTagsAddDomains(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	var files stringList
	var useStdin bool
	fs := flag.NewFlagSet("tags add-domains", flag.ContinueOnError)
	fs.SetOutput(errOut)
	setSubcommandUsage(fs)
	fs.Var(&files, "file", "File with domain names (repeatable)")
	fs.BoolVar(&useStdin, "stdin", false, "Read domains from stdin")
	if err := parseWithReorderedFlags(fs, args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprintln(errOut, "tag name is required")
		return 2
	}
	name := rest[0]
	domainArgs := rest[1:]
	domains, err := collectDomains(domainArgs, files, useStdin, errOut)
	if err != nil {
		return 2
	}
	if len(domains) == 0 {
		fmt.Fprintln(errOut, "at least one domain is required")
		return 2
	}
	body := map[string][]string{"domains": domains}
	if err := client.doJSON(ctx, http.MethodPost, "/tags/"+url.PathEscape(name)+"/domains", body, nil); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if opts.format == "pretty" {
		fmt.Fprintf(out, "Added %d domain(s) to tag %q\n", len(domains), name)
	}
	return 0
}

// ── runs ──────────────────────────────────────────────────────────────────────

func runRuns(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "runs subcommand is required: list|get|results")
		return 2
	}
	cmd := args[0]
	args = args[1:]
	switch cmd {
	case "list":
		return runRunsList(ctx, client, opts, args, out, errOut)
	case "get":
		return runRunsGet(ctx, client, opts, args, out, errOut)
	case "results":
		return runRunsResults(ctx, client, opts, args, out, errOut)
	default:
		fmt.Fprintf(errOut, "Unknown runs command %q\n", cmd)
		return 2
	}
}

func runRunsList(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	fs := flag.NewFlagSet("runs list", flag.ContinueOnError)
	fs.SetOutput(errOut)
	setSubcommandUsage(fs)
	var tagName, domainName, batchID, level string
	var limit int
	fs.StringVar(&tagName, "tag", "", "Filter by tag")
	fs.StringVar(&domainName, "domain", "", "Filter by domain substring")
	fs.StringVar(&batchID, "batch", "", "Filter by batch ID")
	fs.StringVar(&level, "level", "", "Filter by worst level")
	fs.IntVar(&limit, "limit", 100, "Max results")
	if err := parseWithReorderedFlags(fs, args); err != nil {
		return 2
	}
	q := url.Values{}
	if tagName != "" {
		q.Set("tag", tagName)
	}
	if domainName != "" {
		q.Set("domain", domainName)
	}
	if batchID != "" {
		q.Set("batch", batchID)
	}
	if level != "" {
		q.Set("level", level)
	}
	if limit != 100 {
		q.Set("limit", fmt.Sprintf("%d", limit))
	}
	path := "/runs"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var list runList
	if err := client.doJSON(ctx, http.MethodGet, path, nil, &list); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if opts.format != "pretty" {
		if err := writeOutput(out, opts.format, list); err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 2
		}
		return 0
	}
	fmt.Fprintf(out, "Runs: %d\n", list.Total)
	for _, r := range list.Items {
		level := r.WorstLevel
		if level == "" {
			level = "-"
		}
		finAt := r.FinishedAt
		if len(finAt) > 10 {
			finAt = finAt[:10] // date only
		}
		if finAt == "" {
			finAt = "-"
		}
		dur := "-"
		if r.DurationMs > 0 {
			dur = fmt.Sprintf("%dms", r.DurationMs)
		}
		fmt.Fprintf(out, "  %-36s  %-30s  %-10s  level=%-8s  %-10s  %s\n",
			r.ID, r.Domain, r.Status, level, finAt, dur)
	}
	return 0
}

func runRunsGet(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	fs := flag.NewFlagSet("runs get", flag.ContinueOnError)
	fs.SetOutput(errOut)
	setSubcommandUsage(fs)
	if err := parseWithReorderedFlags(fs, args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprintln(errOut, "run ID is required")
		return 2
	}
	runID := strings.TrimSpace(rest[0])
	var r runRecord
	if err := client.doJSON(ctx, http.MethodGet, "/runs/"+url.PathEscape(runID), nil, &r); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if opts.format != "pretty" {
		if err := writeOutput(out, opts.format, r); err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 2
		}
		return 0
	}
	fmt.Fprintf(out, "Run: %s\n", r.ID)
	fmt.Fprintf(out, "  Domain: %s\n", r.Domain)
	fmt.Fprintf(out, "  Status: %s\n", r.Status)
	if r.WorstLevel != "" {
		fmt.Fprintf(out, "  Worst level: %s\n", r.WorstLevel)
	}
	fmt.Fprintf(out, "  Entries: %d\n", r.EntryCount)
	fmt.Fprintf(out, "  Severities: notice=%d warning=%d error=%d critical=%d\n",
		r.SevNotice, r.SevWarning, r.SevError, r.SevCritical)
	return 0
}

func runRunsResults(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	var view string
	fs := flag.NewFlagSet("runs results", flag.ContinueOnError)
	fs.SetOutput(errOut)
	setSubcommandUsage(fs)
	fs.StringVar(&view, "view", "", "View: summary, modules, raw, json")
	if err := parseWithReorderedFlags(fs, args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprintln(errOut, "run ID is required")
		return 2
	}
	runID := strings.TrimSpace(rest[0])

	// Fetch run metadata (for domain name).
	var r runRecord
	if err := client.doJSON(ctx, http.MethodGet, "/runs/"+url.PathEscape(runID), nil, &r); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}

	// Fetch result from the runs endpoint.
	var result jobResult
	path := "/runs/" + url.PathEscape(runID) + "/result"
	if v := strings.TrimSpace(opts.locale); v != "" {
		path += "?locale=" + url.QueryEscape(v)
	}
	if err := client.doJSON(ctx, http.MethodGet, path, nil, &result); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}

	finalView, err := resolveView(opts.format, view)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	domains := map[string]string{result.JobID: r.Domain}
	if err := renderResultsToWriter(opts, finalView, nil, false, true, []jobResult{result}, domains, out); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	return 0
}

// ── entries ───────────────────────────────────────────────────────────────────

func runEntries(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "entries subcommand is required: query")
		return 2
	}
	cmd := args[0]
	args = args[1:]
	switch cmd {
	case "query":
		return runEntriesQuery(ctx, client, opts, args, out, errOut)
	default:
		fmt.Fprintf(errOut, "Unknown entries command %q\n", cmd)
		return 2
	}
}

func runEntriesQuery(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	var tagName, module, testcase, entryTag, level string
	var latest bool
	var limit int
	fs := flag.NewFlagSet("entries query", flag.ContinueOnError)
	fs.SetOutput(errOut)
	setSubcommandUsage(fs)
	fs.StringVar(&tagName, "tag", "", "Filter by domain tag")
	fs.StringVar(&module, "module", "", "Filter by module name")
	fs.StringVar(&testcase, "testcase", "", "Filter by testcase name")
	fs.StringVar(&entryTag, "entry-tag", "", "Filter by entry tag field")
	fs.StringVar(&level, "level", "", "Filter by level")
	fs.BoolVar(&latest, "latest", false, "Only entries from each domain's latest run")
	fs.IntVar(&limit, "limit", 100, "Max results")
	if err := parseWithReorderedFlags(fs, args); err != nil {
		return 2
	}
	q := url.Values{}
	if tagName != "" {
		q.Set("tag", tagName)
	}
	if module != "" {
		q.Set("module", module)
	}
	if testcase != "" {
		q.Set("testcase", testcase)
	}
	if entryTag != "" {
		q.Set("entry_tag", entryTag)
	}
	if level != "" {
		q.Set("level", level)
	}
	if latest {
		q.Set("latest", "true")
	}
	if limit != 100 {
		q.Set("limit", fmt.Sprintf("%d", limit))
	}

	// CSV: pass format=csv to the server and stream the raw response.
	if opts.format == "csv" {
		q.Set("format", "csv")
		path := "/entries?" + q.Encode()
		data, err := client.doBytes(ctx, path)
		if err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 2
		}
		_, _ = out.Write(data)
		return 0
	}

	path := "/entries"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var list entryList
	if err := client.doJSON(ctx, http.MethodGet, path, nil, &list); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if opts.format != "pretty" {
		if err := writeOutput(out, opts.format, list); err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 2
		}
		return 0
	}
	fmt.Fprintf(out, "Entries: %d\n", list.Total)
	for _, e := range list.Items {
		domainCol := e.Domain
		if domainCol == "" && e.DomainID != 0 {
			domainCol = fmt.Sprintf("id:%d", e.DomainID)
		}
		fmt.Fprintf(out, "  %-30s  %-10s  %-20s  %-30s  %s\n",
			domainCol, e.Level, e.Module, e.Testcase, e.Tag)
	}
	return 0
}
