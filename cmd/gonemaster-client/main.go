package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"codeberg.org/pawal/gonemaster/engine/normalization"
)

const (
	defaultServer = "http://localhost:8080/api/v1"
	defaultFormat = "pretty"
)

var doneStatuses = map[string]bool{
	"succeeded": true,
	"failed":    true,
	"canceled":  true,
	"expired":   true,
}

type globalOptions struct {
	server  string
	timeout time.Duration
	format  string
	output  string
	locale  string
	noColor bool
	headers headerList
}

type headerList []string

// String returns the header list in comma-separated form.
func (h *headerList) String() string {
	return strings.Join(*h, ", ")
}

// Set appends a header value to the list.
func (h *headerList) Set(value string) error {
	*h = append(*h, value)
	return nil
}

type stringList []string

// String returns the list in comma-separated form.
func (s *stringList) String() string {
	return strings.Join(*s, ", ")
}

// Set appends a value to the list.
func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

type apiClient struct {
	baseURL    string
	httpClient *http.Client
	headers    http.Header
	locale     string
}

var newHTTPClient = func(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}

type apiError struct {
	Error struct {
		Code    string         `json:"code"`
		Message string         `json:"message"`
		Details map[string]any `json:"details,omitempty"`
	} `json:"error"`
}

type job struct {
	ID         string    `json:"id"`
	BatchID    string    `json:"batch_id,omitempty"`
	Domain     string    `json:"domain"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	StartedAt  time.Time `json:"started_at,omitempty"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	Progress   int       `json:"progress"`
	ResultURL  string    `json:"result_url,omitempty"`
	Error      string    `json:"error,omitempty"`
}

type jobList struct {
	Items []job `json:"items"`
	Total int   `json:"total"`
}

type jobCreateRequest struct {
	Domain           string         `json:"domain"`
	Tests            []string       `json:"tests,omitempty"`
	ProfileOverrides map[string]any `json:"profile_overrides,omitempty"`
	MinLevel         string         `json:"min_level,omitempty"`
}

type jobBatchRequest struct {
	Domains          []string       `json:"domains"`
	Tests            []string       `json:"tests,omitempty"`
	ProfileOverrides map[string]any `json:"profile_overrides,omitempty"`
	MinLevel         string         `json:"min_level,omitempty"`
}

type jobBatchResponse struct {
	BatchID string   `json:"batch_id"`
	JobIDs  []string `json:"job_ids"`
}

type batchSummary struct {
	BatchID      string         `json:"batch_id"`
	Total        int            `json:"total"`
	StatusCounts map[string]int `json:"status_counts"`
	Items        []job          `json:"items"`
	Limit        int            `json:"limit,omitempty"`
	Offset       int            `json:"offset,omitempty"`
	NextCursor   string         `json:"next_cursor,omitempty"`
	PrevCursor   string         `json:"prev_cursor,omitempty"`
	Sort         string         `json:"sort,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	StartedAt    *time.Time     `json:"started_at,omitempty"`
	FinishedAt   *time.Time     `json:"finished_at,omitempty"`
}

type jobResult struct {
	JobID   string         `json:"job_id"`
	BatchID string         `json:"batch_id,omitempty"`
	Status  string         `json:"status"`
	Summary map[string]any `json:"summary,omitempty"`
	Raw     *jobResultRaw  `json:"raw,omitempty"`
}

type jobResultRaw struct {
	Locale  string           `json:"locale,omitempty"`
	Entries []jobResultEntry `json:"entries,omitempty"`
}

type jobResultEntry struct {
	Timestamp float64        `json:"timestamp"`
	Module    string         `json:"module"`
	Testcase  string         `json:"testcase"`
	Tag       string         `json:"tag"`
	Level     string         `json:"level"`
	Args      map[string]any `json:"args,omitempty"`
	Message   string         `json:"message,omitempty"`
	Raw       string         `json:"raw,omitempty"`
}

type queueReorderRequest struct {
	JobIDs []string `json:"job_ids"`
}

type queueRemoveRequest struct {
	JobIDs []string `json:"job_ids"`
}

type queueRemoveResponse struct {
	Removed []string `json:"removed"`
}

type summaryView struct {
	JobID  string         `json:"job_id"`
	Domain string         `json:"domain,omitempty"`
	Status string         `json:"status"`
	Total  int            `json:"total"`
	Levels map[string]int `json:"levels"`
	Error  string         `json:"error,omitempty"`
}

type moduleGroup struct {
	Module  string           `json:"module"`
	Counts  map[string]int   `json:"counts"`
	Entries []jobResultEntry `json:"entries,omitempty"`
}

type modulesView struct {
	JobID   string        `json:"job_id"`
	Domain  string        `json:"domain,omitempty"`
	Status  string        `json:"status"`
	Modules []moduleGroup `json:"modules"`
	Error   string        `json:"error,omitempty"`
}

type rawView struct {
	JobID   string           `json:"job_id"`
	Domain  string           `json:"domain,omitempty"`
	Status  string           `json:"status"`
	Entries []jobResultEntry `json:"entries"`
	Error   string           `json:"error,omitempty"`
}

type aggregateView struct {
	JobCount int              `json:"job_count"`
	Summary  *summaryView     `json:"summary,omitempty"`
	Modules  []moduleGroup    `json:"modules,omitempty"`
	Entries  []jobResultEntry `json:"entries,omitempty"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, out io.Writer, errOut io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := rejectSingleDashFlags(args); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}

	opts, rest, err := parseGlobalFlags(args, errOut)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if len(rest) == 0 {
		printUsage(errOut)
		return 2
	}

	client, err := newClient(opts)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}

	writer, closeOut, err := openOutput(opts.output, out)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	defer closeOut()

	group := rest[0]
	rest = rest[1:]
	switch group {
	case "jobs":
		return runJobs(ctx, client, opts, rest, writer, errOut)
	case "batches":
		return runBatches(ctx, client, opts, rest, writer, errOut)
	case "queue":
		return runQueue(ctx, client, opts, rest, writer, errOut)
	case "help", "-h", "--help":
		printUsage(writer)
		return 0
	default:
		fmt.Fprintf(errOut, "Unknown command %q\n\n", group)
		printUsage(errOut)
		return 2
	}
}

func parseGlobalFlags(args []string, errOut io.Writer) (globalOptions, []string, error) {
	var opts globalOptions
	fs := flag.NewFlagSet("gonemaster-client", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.Usage = func() {
		printUsage(errOut)
	}
	fs.StringVar(&opts.server, "server", "", "Base server URL (default http://localhost:8080/api/v1)")
	fs.DurationVar(&opts.timeout, "timeout", 30*time.Second, "HTTP timeout (default 30s)")
	fs.StringVar(&opts.format, "format", defaultFormat, "Output format: pretty, json, jsonl")
	fs.StringVar(&opts.output, "output", "", "Write output to file instead of stdout")
	fs.StringVar(&opts.locale, "locale", "en", "Locale for translated messages")
	fs.BoolVar(&opts.noColor, "no-color", false, "Disable ANSI colors in pretty output")
	fs.Var(&opts.headers, "header", "Extra HTTP header (repeatable, NAME:VALUE)")
	if err := fs.Parse(args); err != nil {
		return opts, nil, err
	}
	if opts.server == "" {
		opts.server = defaultServer
	}
	return opts, fs.Args(), nil
}

func printUsage(out io.Writer) {
	fmt.Fprintln(out, "Usage: gonemaster-client [global options] <command> [command options]")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Global options:")
	fmt.Fprintln(out, "  --server URL     Base API URL (default http://localhost:8080/api/v1)")
	fmt.Fprintln(out, "  --timeout DURATION  HTTP timeout (default 30s)")
	fmt.Fprintln(out, "  --format FORMAT  Output format: pretty, json, jsonl")
	fmt.Fprintln(out, "  --output PATH    Write output to file instead of stdout")
	fmt.Fprintln(out, "  --locale LOCALE  Locale for translated messages (default en)")
	fmt.Fprintln(out, "  --no-color       Disable ANSI colors in pretty output")
	fmt.Fprintln(out, "  --header NAME:VALUE  Extra HTTP header (repeatable)")
	fmt.Fprintln(out, "  (Options use double hyphens; short single-dash flags are not supported.)")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Commands:")
	fmt.Fprintln(out, "  jobs create|batch|list|get|watch|cancel|results")
	fmt.Fprintln(out, "  batches get|watch|results|cancel|remove")
	fmt.Fprintln(out, "  queue pause|resume|reorder|remove")
}

func rejectSingleDashFlags(args []string) error {
	for _, arg := range args {
		if arg == "-" {
			continue
		}
		if strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") {
			return fmt.Errorf("use double-hyphen options (e.g. --server), not %q", arg)
		}
	}
	return nil
}

func newClient(opts globalOptions) (*apiClient, error) {
	base, err := normalizeBaseURL(opts.server)
	if err != nil {
		return nil, err
	}
	headers, err := parseHeaders(opts.headers)
	if err != nil {
		return nil, err
	}
	return &apiClient{
		baseURL:    base,
		httpClient: newHTTPClient(opts.timeout),
		headers:    headers,
		locale:     opts.locale,
	}, nil
}

func normalizeBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultServer, nil
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid server URL: %w", err)
	}
	path := strings.TrimRight(parsed.Path, "/")
	if path == "" {
		path = "/api/v1"
	} else if strings.HasPrefix(path, "/api/v1") {
		path = "/api/v1"
	} else {
		path = path + "/api/v1"
	}
	parsed.Path = path
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func parseHeaders(values []string) (http.Header, error) {
	headers := http.Header{}
	for _, value := range values {
		parts := strings.SplitN(value, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid header %q (expected NAME:VALUE)", value)
		}
		name := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if name == "" {
			return nil, fmt.Errorf("invalid header %q (missing name)", value)
		}
		headers.Add(name, val)
	}
	return headers, nil
}

func openOutput(path string, fallback io.Writer) (io.Writer, func(), error) {
	if path == "" {
		return fallback, func() {}, nil
	}
	file, err := os.Create(path)
	if err != nil {
		return nil, func() {}, err
	}
	return file, func() { _ = file.Close() }, nil
}

func parseWithReorderedFlags(fs *flag.FlagSet, args []string) error {
	return fs.Parse(reorderFlags(fs, args))
}

func reorderFlags(fs *flag.FlagSet, args []string) []string {
	flags := make([]string, 0, len(args))
	positionals := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "--") {
			positionals = append(positionals, arg)
			continue
		}

		name := strings.TrimPrefix(arg, "--")
		if name == "" {
			positionals = append(positionals, arg)
			continue
		}
		hasValue := false
		if eq := strings.Index(name, "="); eq >= 0 {
			name = name[:eq]
			hasValue = true
		}

		flags = append(flags, arg)
		if hasValue {
			continue
		}
		if flagExpectsValue(fs.Lookup(name)) {
			if i+1 < len(args) {
				flags = append(flags, args[i+1])
				i++
			}
		}
	}

	return append(flags, positionals...)
}

type boolFlag interface {
	IsBoolFlag() bool
}

func flagExpectsValue(f *flag.Flag) bool {
	if f == nil || f.Value == nil {
		return false
	}
	if bf, ok := f.Value.(boolFlag); ok && bf.IsBoolFlag() {
		return false
	}
	return true
}

func (c *apiClient) doJSON(ctx context.Context, method string, path string, body any, out any) error {
	full := strings.TrimRight(c.baseURL, "/") + path
	var payload io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, full, payload)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for name, values := range c.headers {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		var apiErr apiError
		if err := json.Unmarshal(raw, &apiErr); err == nil && apiErr.Error.Message != "" {
			return fmt.Errorf("%s (code=%s)", apiErr.Error.Message, apiErr.Error.Code)
		}
		return fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if out == nil {
		return nil
	}
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func runJobs(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "jobs subcommand is required")
		return 2
	}
	cmd := args[0]
	args = args[1:]
	switch cmd {
	case "create":
		return runJobsCreate(ctx, client, opts, args, out, errOut)
	case "batch":
		return runJobsBatch(ctx, client, opts, args, out, errOut)
	case "list":
		return runJobsList(ctx, client, opts, args, out, errOut)
	case "get":
		return runJobsGet(ctx, client, opts, args, out, errOut)
	case "watch":
		return runJobsWatch(ctx, client, opts, args, out, errOut)
	case "cancel":
		return runJobsCancel(ctx, client, opts, args, out, errOut)
	case "results":
		return runJobsResults(ctx, client, opts, args, out, errOut)
	default:
		fmt.Fprintf(errOut, "Unknown jobs command %q\n", cmd)
		return 2
	}
}

func runBatches(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "batches subcommand is required")
		return 2
	}
	cmd := args[0]
	args = args[1:]
	switch cmd {
	case "get":
		return runBatchesGet(ctx, client, opts, args, out, errOut)
	case "watch":
		return runBatchesWatch(ctx, client, opts, args, out, errOut)
	case "results":
		return runBatchesResults(ctx, client, opts, args, out, errOut)
	case "cancel":
		return runBatchesCancel(ctx, client, opts, args, out, errOut)
	case "remove":
		return runBatchesRemove(ctx, client, opts, args, out, errOut)
	default:
		fmt.Fprintf(errOut, "Unknown batches command %q\n", cmd)
		return 2
	}
}

func runQueue(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "queue subcommand is required")
		return 2
	}
	cmd := args[0]
	args = args[1:]
	switch cmd {
	case "pause":
		return runQueuePause(ctx, client, opts, out, errOut)
	case "resume":
		return runQueueResume(ctx, client, opts, out, errOut)
	case "reorder":
		return runQueueReorder(ctx, client, opts, args, out, errOut)
	case "remove":
		return runQueueRemove(ctx, client, opts, args, out, errOut)
	default:
		fmt.Fprintf(errOut, "Unknown queue command %q\n", cmd)
		return 2
	}
}

func runJobsCreate(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	var (
		domain        string
		minLevel      string
		tests         stringList
		overridePairs stringList
		overrideFile  string
		wait          bool
		view          string
	)
	fs := flag.NewFlagSet("jobs create", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.StringVar(&domain, "domain", "", "Domain to test (required)")
	fs.StringVar(&minLevel, "min-level", "", "Minimum log level (optional)")
	fs.Var(&tests, "tests", "Test name (repeatable)")
	fs.Var(&overridePairs, "profile-override", "Profile override KEY=VALUE (repeatable)")
	fs.StringVar(&overrideFile, "profile-overrides-file", "", "Profile overrides JSON/YAML file")
	fs.BoolVar(&wait, "wait", false, "Wait for completion and display results")
	fs.StringVar(&view, "view", "", "Result view: summary, modules, raw, json (aliases: translated, full)")
	if err := parseWithReorderedFlags(fs, args); err != nil {
		return 2
	}
	if strings.TrimSpace(domain) == "" {
		fmt.Fprintln(errOut, "--domain is required")
		return 2
	}
	normalized, err := normalizeDomain(domain)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	overrides, err := parseOverrides(overridePairs, overrideFile)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	req := jobCreateRequest{
		Domain:           normalized,
		Tests:            tests,
		ProfileOverrides: overrides,
		MinLevel:         minLevel,
	}
	var created job
	if err := client.doJSON(ctx, http.MethodPost, "/jobs", req, &created); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if !wait || opts.format != "json" {
		if err := writeOutput(out, opts.format, created); err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 2
		}
	}
	if !wait {
		return 0
	}
	jobInfo, err := waitForJob(ctx, client, created.ID, 3*time.Second)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	finalView, err := resolveView(opts.format, view)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if err := renderJobResults(ctx, client, opts, finalView, "", false, true, "", []string{jobInfo.ID}, out, errOut); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	return 0
}

func runJobsBatch(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	var (
		domains       stringList
		files         stringList
		useStdin      bool
		tests         stringList
		minLevel      string
		overridePairs stringList
		overrideFile  string
		wait          bool
		view          string
		perJob        bool
	)
	fs := flag.NewFlagSet("jobs batch", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.Var(&domains, "domain", "Domain to test (repeatable)")
	fs.Var(&files, "file", "File with domains (repeatable)")
	fs.BoolVar(&useStdin, "stdin", false, "Read domains from stdin")
	fs.Var(&tests, "tests", "Test name (repeatable)")
	fs.StringVar(&minLevel, "min-level", "", "Minimum log level (optional)")
	fs.Var(&overridePairs, "profile-override", "Profile override KEY=VALUE (repeatable)")
	fs.StringVar(&overrideFile, "profile-overrides-file", "", "Profile overrides JSON/YAML file")
	fs.BoolVar(&wait, "wait", false, "Wait for completion and display results")
	fs.StringVar(&view, "view", "", "Result view: summary, modules, raw, json (aliases: translated, full)")
	fs.BoolVar(&perJob, "per-job", false, "Show per-job results when waiting")
	if err := parseWithReorderedFlags(fs, args); err != nil {
		return 2
	}
	entries, err := collectDomains(domains, files, useStdin, errOut)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if len(entries) == 0 {
		fmt.Fprintln(errOut, "no domains provided")
		return 2
	}
	overrides, err := parseOverrides(overridePairs, overrideFile)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	req := jobBatchRequest{
		Domains:          entries,
		Tests:            tests,
		ProfileOverrides: overrides,
		MinLevel:         minLevel,
	}
	var resp jobBatchResponse
	if err := client.doJSON(ctx, http.MethodPost, "/jobs/batch", req, &resp); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if !wait || opts.format != "json" {
		if err := writeOutput(out, opts.format, resp); err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 2
		}
	}
	if !wait {
		return 0
	}
	summary, err := waitForBatch(ctx, client, resp.BatchID, 4*time.Second)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	jobIDs := make([]string, 0, len(summary.Items))
	for _, item := range summary.Items {
		jobIDs = append(jobIDs, item.ID)
	}
	finalView, err := resolveView(opts.format, view)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if err := renderJobResults(ctx, client, opts, finalView, "", !perJob, perJob, "", jobIDs, out, errOut); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	return 0
}

func runJobsList(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	var (
		status       string
		batchID      string
		createdAfter string
		limit        int
		offset       int
	)
	fs := flag.NewFlagSet("jobs list", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.StringVar(&status, "status", "", "Filter by status")
	fs.StringVar(&batchID, "batch-id", "", "Filter by batch id")
	fs.StringVar(&createdAfter, "created-after", "", "Filter by created after timestamp (RFC3339)")
	fs.IntVar(&limit, "limit", 100, "Limit (1-500)")
	fs.IntVar(&offset, "offset", 0, "Offset")
	if err := parseWithReorderedFlags(fs, args); err != nil {
		return 2
	}
	if createdAfter != "" {
		if _, err := time.Parse(time.RFC3339, createdAfter); err != nil {
			fmt.Fprintln(errOut, "created-after must be RFC3339")
			return 2
		}
	}
	query := url.Values{}
	if status != "" {
		query.Set("status", status)
	}
	if batchID != "" {
		query.Set("batch_id", batchID)
	}
	if createdAfter != "" {
		query.Set("created_after", createdAfter)
	}
	if limit > 0 {
		query.Set("limit", fmt.Sprintf("%d", limit))
	}
	if offset > 0 {
		query.Set("offset", fmt.Sprintf("%d", offset))
	}
	path := "/jobs"
	if len(query) > 0 {
		path = path + "?" + query.Encode()
	}
	var list jobList
	if err := client.doJSON(ctx, http.MethodGet, path, nil, &list); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if opts.format == "jsonl" {
		for _, item := range list.Items {
			if err := writeJSONLine(out, item); err != nil {
				fmt.Fprintln(errOut, err.Error())
				return 2
			}
		}
		return 0
	}
	if err := writeOutput(out, opts.format, list); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	return 0
}

func runJobsGet(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "job id is required")
		return 2
	}
	jobID := strings.TrimSpace(args[0])
	var info job
	if err := client.doJSON(ctx, http.MethodGet, "/jobs/"+jobID, nil, &info); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if err := writeOutput(out, opts.format, info); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	return 0
}

func runJobsWatch(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	var poll time.Duration
	fs := flag.NewFlagSet("jobs watch", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.DurationVar(&poll, "poll", 3*time.Second, "Poll interval")
	if err := parseWithReorderedFlags(fs, args); err != nil {
		return 2
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(errOut, "job id is required")
		return 2
	}
	jobID := strings.TrimSpace(fs.Arg(0))
	info, err := waitForJob(ctx, client, jobID, poll)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if err := writeOutput(out, opts.format, info); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	return 0
}

func runJobsCancel(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "job id is required")
		return 2
	}
	jobID := strings.TrimSpace(args[0])
	var info job
	if err := client.doJSON(ctx, http.MethodPost, "/jobs/"+jobID+"/cancel", nil, &info); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if err := writeOutput(out, opts.format, info); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	return 0
}

func runJobsResults(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	return runResults(ctx, client, opts, args, out, errOut, false)
}

func runBatchesGet(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "batch id is required")
		return 2
	}
	batchID := strings.TrimSpace(args[0])
	summary, err := fetchBatchSummary(ctx, client, batchID)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if err := writeOutput(out, opts.format, summary); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	return 0
}

func runBatchesWatch(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	var poll time.Duration
	fs := flag.NewFlagSet("batches watch", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.DurationVar(&poll, "poll", 4*time.Second, "Poll interval")
	if err := parseWithReorderedFlags(fs, args); err != nil {
		return 2
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(errOut, "batch id is required")
		return 2
	}
	batchID := strings.TrimSpace(fs.Arg(0))
	summary, err := waitForBatch(ctx, client, batchID, poll)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if err := writeOutput(out, opts.format, summary); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	return 0
}

func runBatchesResults(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	return runResults(ctx, client, opts, args, out, errOut, true)
}

func runBatchesCancel(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "batch id is required")
		return 2
	}
	batchID := strings.TrimSpace(args[0])
	summary, err := fetchBatchSummary(ctx, client, batchID)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	canceled := []string{}
	skipped := []string{}
	for _, item := range summary.Items {
		if item.Status != "queued" && item.Status != "running" && item.Status != "paused" {
			skipped = append(skipped, item.ID)
			continue
		}
		var jobInfo job
		if err := client.doJSON(ctx, http.MethodPost, "/jobs/"+item.ID+"/cancel", nil, &jobInfo); err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 2
		}
		canceled = append(canceled, item.ID)
	}
	if opts.format == "pretty" {
		if len(skipped) > 0 {
			fmt.Fprintf(out, "Canceled %d job(s) in batch %s (skipped %d).\n", len(canceled), batchID, len(skipped))
		} else {
			fmt.Fprintf(out, "Canceled %d job(s) in batch %s.\n", len(canceled), batchID)
		}
		return 0
	}
	payload := map[string]any{
		"batch_id": batchID,
		"canceled": canceled,
	}
	if len(skipped) > 0 {
		payload["skipped"] = skipped
	}
	if err := writeOutput(out, opts.format, payload); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	return 0
}

func runBatchesRemove(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	var cancelRunning bool
	fs := flag.NewFlagSet("batches remove", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.BoolVar(&cancelRunning, "cancel-running", false, "Cancel running jobs in the batch")
	if err := parseWithReorderedFlags(fs, args); err != nil {
		return 2
	}
	args = fs.Args()
	if len(args) == 0 {
		fmt.Fprintln(errOut, "batch id is required")
		return 2
	}
	batchID := strings.TrimSpace(args[0])
	summary, err := fetchBatchSummary(ctx, client, batchID)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	queued := []string{}
	canceled := []string{}
	skipped := []string{}
	for _, item := range summary.Items {
		if item.Status == "queued" {
			queued = append(queued, item.ID)
			continue
		}
		if cancelRunning && item.Status == "running" {
			var jobInfo job
			if err := client.doJSON(ctx, http.MethodPost, "/jobs/"+item.ID+"/cancel", nil, &jobInfo); err != nil {
				fmt.Fprintln(errOut, err.Error())
				return 2
			}
			canceled = append(canceled, item.ID)
			continue
		} else {
			skipped = append(skipped, item.ID)
		}
	}
	removed := []string{}
	if len(queued) > 0 {
		req := queueRemoveRequest{JobIDs: queued}
		var resp queueRemoveResponse
		if err := client.doJSON(ctx, http.MethodPost, "/queue/remove", req, &resp); err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 2
		}
		removed = resp.Removed
	}
	if opts.format == "pretty" {
		message := fmt.Sprintf("Removed %d queued job(s) in batch %s.", len(removed), batchID)
		if cancelRunning {
			message = fmt.Sprintf("%s Canceled %d running job(s).", message, len(canceled))
		}
		if len(skipped) > 0 {
			message = fmt.Sprintf("%s Skipped %d job(s).", message, len(skipped))
		}
		fmt.Fprintln(out, message)
		return 0
	}
	payload := map[string]any{
		"batch_id": batchID,
		"removed":  removed,
	}
	if cancelRunning {
		payload["canceled"] = canceled
	}
	if len(skipped) > 0 {
		payload["skipped"] = skipped
	}
	if err := writeOutput(out, opts.format, payload); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	return 0
}

func runQueuePause(ctx context.Context, client *apiClient, opts globalOptions, out io.Writer, errOut io.Writer) int {
	if err := client.doJSON(ctx, http.MethodPost, "/queue/pause", nil, nil); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if opts.format == "pretty" {
		fmt.Fprintln(out, "Queue paused.")
		return 0
	}
	if err := writeOutput(out, opts.format, map[string]string{"status": "paused"}); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	return 0
}

func runQueueResume(ctx context.Context, client *apiClient, opts globalOptions, out io.Writer, errOut io.Writer) int {
	if err := client.doJSON(ctx, http.MethodPost, "/queue/resume", nil, nil); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if opts.format == "pretty" {
		fmt.Fprintln(out, "Queue resumed.")
		return 0
	}
	if err := writeOutput(out, opts.format, map[string]string{"status": "resumed"}); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	return 0
}

func runQueueReorder(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	var jobIDs stringList
	fs := flag.NewFlagSet("queue reorder", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.Var(&jobIDs, "job-id", "Job id (repeatable)")
	if err := parseWithReorderedFlags(fs, args); err != nil {
		return 2
	}
	jobIDs = append(jobIDs, fs.Args()...)
	jobIDs = normalizeIDs(jobIDs)
	if len(jobIDs) == 0 {
		fmt.Fprintln(errOut, "at least one job id is required")
		return 2
	}
	req := queueReorderRequest{JobIDs: jobIDs}
	if err := client.doJSON(ctx, http.MethodPost, "/queue/reorder", req, nil); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if opts.format == "pretty" {
		fmt.Fprintln(out, "Queue reordered.")
		return 0
	}
	if err := writeOutput(out, opts.format, map[string]any{"status": "reordered", "job_ids": jobIDs}); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	return 0
}

func runQueueRemove(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	var jobIDs stringList
	fs := flag.NewFlagSet("queue remove", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.Var(&jobIDs, "job-id", "Job id (repeatable)")
	if err := parseWithReorderedFlags(fs, args); err != nil {
		return 2
	}
	jobIDs = append(jobIDs, fs.Args()...)
	jobIDs = normalizeIDs(jobIDs)
	if len(jobIDs) == 0 {
		fmt.Fprintln(errOut, "at least one job id is required")
		return 2
	}
	req := queueRemoveRequest{JobIDs: jobIDs}
	var resp queueRemoveResponse
	if err := client.doJSON(ctx, http.MethodPost, "/queue/remove", req, &resp); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if opts.format == "pretty" {
		fmt.Fprintf(out, "Removed %d job(s).\n", len(resp.Removed))
		return 0
	}
	if err := writeOutput(out, opts.format, resp); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	return 0
}

func runResults(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer, batchMode bool) int {
	var (
		jobIDs       stringList
		batchID      string
		all          bool
		status       string
		createdAfter string
		view         string
		levels       string
		aggregate    bool
		perJob       bool
		splitDir     string
	)
	fs := flag.NewFlagSet("results", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.Var(&jobIDs, "job-id", "Job id (repeatable)")
	fs.StringVar(&batchID, "batch-id", "", "Batch id")
	fs.BoolVar(&all, "all", false, "Fetch results for all jobs")
	fs.StringVar(&status, "status", "", "Filter status when using --all")
	fs.StringVar(&createdAfter, "created-after", "", "Filter by created after timestamp (RFC3339)")
	fs.StringVar(&view, "view", "", "View: summary, modules, raw, json (aliases: translated, full)")
	fs.StringVar(&levels, "levels", "", "Comma-separated levels to include")
	fs.BoolVar(&aggregate, "aggregate", false, "Aggregate results across jobs")
	fs.BoolVar(&perJob, "per-job", false, "Show per-job results")
	fs.StringVar(&splitDir, "split-dir", "", "Write per-job results to files in this directory")
	if err := parseWithReorderedFlags(fs, args); err != nil {
		return 2
	}

	if batchMode {
		if fs.NArg() == 0 {
			fmt.Fprintln(errOut, "batch id is required")
			return 2
		}
		batchID = strings.TrimSpace(fs.Arg(0))
	}
	if createdAfter != "" {
		if _, err := time.Parse(time.RFC3339, createdAfter); err != nil {
			fmt.Fprintln(errOut, "created-after must be RFC3339")
			return 2
		}
	}
	resolved, err := resolveView(opts.format, view)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	view = resolved
	if !batchMode {
		jobIDs = append(jobIDs, fs.Args()...)
	}
	jobIDs = normalizeIDs(jobIDs)

	groupCount := 0
	if len(jobIDs) > 0 {
		groupCount++
	}
	if batchID != "" {
		groupCount++
	}
	if all {
		groupCount++
	}
	if groupCount == 0 {
		fmt.Fprintln(errOut, "job id, --batch-id, or --all is required")
		return 2
	}
	if groupCount > 1 {
		fmt.Fprintln(errOut, "use only one of job ids, --batch-id, or --all")
		return 2
	}

	var ids []string
	switch {
	case len(jobIDs) > 0:
		ids = jobIDs
	case batchID != "":
		summary, err := fetchBatchSummary(ctx, client, batchID)
		if err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 2
		}
		for _, item := range summary.Items {
			ids = append(ids, item.ID)
		}
	case all:
		found, err := listAllJobIDs(ctx, client, status, createdAfter)
		if err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 2
		}
		ids = found
	}

	if len(ids) == 0 {
		fmt.Fprintln(errOut, "no jobs found")
		return 2
	}

	if !aggregate && !perJob {
		perJob = len(ids) == 1
		if !perJob {
			perJob = true
		}
	}
	if err := renderJobResults(ctx, client, opts, view, levels, aggregate, perJob, splitDir, ids, out, errOut); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	return 0
}

func renderJobResults(ctx context.Context, client *apiClient, opts globalOptions, view string, levels string, aggregate bool, perJob bool, splitDir string, ids []string, out io.Writer, errOut io.Writer) error {
	levelSet, err := parseLevels(levels)
	if err != nil {
		return err
	}
	domains := map[string]string{}
	for _, jobID := range ids {
		info, err := fetchJobInfo(ctx, client, jobID)
		if err != nil {
			return err
		}
		domains[jobID] = info.Domain
	}
	results := make([]jobResult, 0, len(ids))
	for _, jobID := range ids {
		result, err := fetchJobResult(ctx, client, jobID)
		if err != nil {
			return err
		}
		results = append(results, result)
	}

	if splitDir != "" {
		if err := os.MkdirAll(splitDir, 0o755); err != nil {
			return err
		}
		for _, result := range results {
			ext := outputExt(opts.format)
			path := filepath.Join(splitDir, result.JobID+ext)
			file, err := os.Create(path)
			if err != nil {
				return err
			}
			if err := renderResultsToWriter(opts, view, levelSet, false, true, []jobResult{result}, domains, file); err != nil {
				_ = file.Close()
				return err
			}
			_ = file.Close()
			fmt.Fprintf(out, "Wrote %s\n", path)
		}
		if aggregate {
			return renderResultsToWriter(opts, view, levelSet, true, false, results, domains, out)
		}
		return nil
	}

	return renderResultsToWriter(opts, view, levelSet, aggregate, perJob, results, domains, out)
}

func fetchJobResult(ctx context.Context, client *apiClient, jobID string) (jobResult, error) {
	result, err := fetchJobResultOnce(ctx, client, jobID)
	if err == nil {
		return result, nil
	}
	if !isNotFoundError(err) {
		return jobResult{}, err
	}
	info, err := fetchJobInfo(ctx, client, jobID)
	if err != nil {
		return jobResult{}, err
	}
	if !doneStatuses[info.Status] {
		return jobResult{}, err
	}
	delay := 200 * time.Millisecond
	for i := 0; i < 4; i++ {
		select {
		case <-ctx.Done():
			return jobResult{}, ctx.Err()
		case <-time.After(delay):
		}
		result, err = fetchJobResultOnce(ctx, client, jobID)
		if err == nil {
			return result, nil
		}
		if !isNotFoundError(err) {
			return jobResult{}, err
		}
		delay *= 2
	}
	return jobResult{}, err
}

func fetchJobInfo(ctx context.Context, client *apiClient, jobID string) (job, error) {
	var info job
	if err := client.doJSON(ctx, http.MethodGet, "/jobs/"+jobID, nil, &info); err != nil {
		return job{}, err
	}
	return info, nil
}

func fetchJobResultOnce(ctx context.Context, client *apiClient, jobID string) (jobResult, error) {
	var result jobResult
	path := "/jobs/" + jobID + "/result"
	if client.locale != "" {
		path += "?locale=" + url.QueryEscape(client.locale)
	}
	if err := client.doJSON(ctx, http.MethodGet, path, nil, &result); err != nil {
		return jobResult{}, err
	}
	return result, nil
}

func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "code=not_found") || strings.Contains(msg, "not found")
}

func renderResultsToWriter(opts globalOptions, view string, levelSet map[string]bool, aggregate bool, perJob bool, results []jobResult, domains map[string]string, out io.Writer) error {
	switch view {
	case "json":
		if opts.format == "jsonl" {
			for _, result := range results {
				if err := writeJSONLine(out, result); err != nil {
					return err
				}
			}
			return nil
		}
		if len(results) == 1 {
			return writeJSON(out, results[0])
		}
		return writeJSON(out, results)
	case "summary":
		views := buildSummaryViews(results, levelSet, domains)
		if opts.format == "json" {
			if aggregate {
				payload := map[string]any{
					"jobs":      views,
					"aggregate": buildAggregateSummary(results, levelSet),
				}
				return writeJSON(out, payload)
			}
			if len(views) == 1 {
				return writeJSON(out, views[0])
			}
			return writeJSON(out, views)
		}
		if opts.format == "jsonl" {
			for _, view := range views {
				if err := writeJSONLine(out, view); err != nil {
					return err
				}
			}
			return nil
		}
		if perJob {
			for _, view := range views {
				printSummaryPretty(out, view)
			}
		}
		if aggregate {
			printAggregateSummaryPretty(out, buildAggregateSummary(results, levelSet))
		}
		return nil
	case "modules":
		views := buildModulesViews(results, levelSet, domains)
		if opts.format == "json" {
			if aggregate {
				payload := map[string]any{
					"jobs":      views,
					"aggregate": buildAggregateModules(results, levelSet),
				}
				return writeJSON(out, payload)
			}
			if len(views) == 1 {
				return writeJSON(out, views[0])
			}
			return writeJSON(out, views)
		}
		if opts.format == "jsonl" {
			for _, view := range views {
				if err := writeJSONLine(out, view); err != nil {
					return err
				}
			}
			return nil
		}
		if perJob {
			for _, view := range views {
				printModulesPretty(out, view)
			}
		}
		if aggregate {
			printAggregateModulesPretty(out, buildAggregateModules(results, levelSet))
		}
		return nil
	case "raw":
		views := buildRawViews(results, levelSet, domains)
		if opts.format == "json" {
			if aggregate {
				payload := map[string]any{
					"jobs":      views,
					"aggregate": buildAggregateRaw(results, levelSet),
				}
				return writeJSON(out, payload)
			}
			if len(views) == 1 {
				return writeJSON(out, views[0])
			}
			return writeJSON(out, views)
		}
		if opts.format == "jsonl" {
			for _, view := range views {
				if err := writeJSONLine(out, view); err != nil {
					return err
				}
			}
			return nil
		}
		if perJob {
			for _, view := range views {
				printRawPretty(out, view)
			}
		}
		if aggregate {
			printAggregateRawPretty(out, buildAggregateRaw(results, levelSet))
		}
		return nil
	}
	return nil
}

func writeOutput(out io.Writer, format string, payload any) error {
	switch format {
	case "json":
		return writeJSON(out, payload)
	case "jsonl":
		return writeJSONLine(out, payload)
	default:
		return writePretty(out, payload)
	}
}

func writeJSON(out io.Writer, payload any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

func writeJSONLine(out io.Writer, payload any) error {
	enc := json.NewEncoder(out)
	return enc.Encode(payload)
}

func writePretty(out io.Writer, payload any) error {
	switch value := payload.(type) {
	case job:
		fmt.Fprintf(out, "Job %s\n", value.ID)
		if value.Domain != "" {
			fmt.Fprintf(out, "  Domain: %s\n", value.Domain)
		}
		fmt.Fprintf(out, "  Status: %s\n", value.Status)
		fmt.Fprintf(out, "  Progress: %d%%\n", value.Progress)
		if value.BatchID != "" {
			fmt.Fprintf(out, "  Batch: %s\n", value.BatchID)
		}
		return nil
	case jobBatchResponse:
		fmt.Fprintf(out, "Batch %s accepted (%d jobs)\n", value.BatchID, len(value.JobIDs))
		return nil
	case jobList:
		fmt.Fprintf(out, "Jobs: %d\n", value.Total)
		for _, item := range value.Items {
			line := fmt.Sprintf("- %s", item.ID)
			if item.Domain != "" {
				line = fmt.Sprintf("%s (%s)", line, item.Domain)
			}
			line = fmt.Sprintf("%s %s %d%%", line, item.Status, item.Progress)
			fmt.Fprintln(out, line)
		}
		return nil
	case batchSummary:
		fmt.Fprintf(out, "Batch %s\n", value.BatchID)
		fmt.Fprintf(out, "  Total: %d\n", value.Total)
		for status, count := range value.StatusCounts {
			fmt.Fprintf(out, "  %s: %d\n", status, count)
		}
		return nil
	default:
		return writeJSON(out, payload)
	}
}

func normalizeDomain(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("domain is required")
	}
	if errs, normalized := normalization.NormalizeName(trimmed); len(errs) > 0 {
		return "", fmt.Errorf("invalid domain: %s", errs[0].Message())
	} else {
		return normalized, nil
	}
}

func collectDomains(domains []string, files []string, useStdin bool, errOut io.Writer) ([]string, error) {
	entries := make([]string, 0)
	for _, domain := range domains {
		entries = append(entries, domain)
	}
	for _, file := range files {
		lines, err := readDomainsFromFile(file)
		if err != nil {
			return nil, err
		}
		entries = append(entries, lines...)
	}
	if useStdin {
		lines, err := readDomainsFromReader(os.Stdin)
		if err != nil {
			return nil, err
		}
		entries = append(entries, lines...)
	}
	seen := map[string]bool{}
	normalized := make([]string, 0, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if strings.HasPrefix(entry, "#") {
			continue
		}
		domain, err := normalizeDomain(entry)
		if err != nil {
			fmt.Fprintln(errOut, err.Error())
			return nil, err
		}
		if seen[domain] {
			continue
		}
		seen[domain] = true
		normalized = append(normalized, domain)
	}
	return normalized, nil
}

func readDomainsFromFile(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return readDomainsFromReader(file)
}

func readDomainsFromReader(r io.Reader) ([]string, error) {
	lines := []string{}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

func parseOverrides(pairs []string, file string) (map[string]any, error) {
	overrides := map[string]any{}
	if file != "" {
		content, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		if len(bytes.TrimSpace(content)) > 0 {
			if err := json.Unmarshal(content, &overrides); err != nil {
				return nil, fmt.Errorf("profile overrides file must be JSON-compatible: %w", err)
			}
		}
	}
	for _, pair := range pairs {
		key, value, ok := strings.Cut(pair, "=")
		if !ok {
			return nil, fmt.Errorf("invalid profile-override %q (expected KEY=VALUE)", pair)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("invalid profile-override %q (missing key)", pair)
		}
		val := parseOverrideValue(strings.TrimSpace(value))
		setNestedValue(overrides, strings.Split(key, "."), val)
	}
	return overrides, nil
}

func parseOverrideValue(value string) any {
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "{") || strings.HasPrefix(value, "[") || strings.HasPrefix(value, "\"") || value == "true" || value == "false" || value == "null" || isNumeric(value) {
		var parsed any
		if err := json.Unmarshal([]byte(value), &parsed); err == nil {
			return parsed
		}
	}
	return value
}

func isNumeric(value string) bool {
	for i, r := range value {
		if (r < '0' || r > '9') && !(i == 0 && (r == '-' || r == '+')) && r != '.' {
			return false
		}
	}
	return true
}

func setNestedValue(target map[string]any, path []string, value any) {
	if len(path) == 0 {
		return
	}
	current := target
	for i := 0; i < len(path)-1; i++ {
		key := path[i]
		next, ok := current[key]
		if !ok {
			child := map[string]any{}
			current[key] = child
			current = child
			continue
		}
		child, ok := next.(map[string]any)
		if !ok {
			child = map[string]any{}
			current[key] = child
		}
		current = child
	}
	current[path[len(path)-1]] = value
}

func normalizeIDs(ids []string) []string {
	normalized := make([]string, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		normalized = append(normalized, id)
	}
	return normalized
}

func waitForJob(ctx context.Context, client *apiClient, jobID string, poll time.Duration) (job, error) {
	for {
		var info job
		if err := client.doJSON(ctx, http.MethodGet, "/jobs/"+jobID, nil, &info); err != nil {
			return job{}, err
		}
		if doneStatuses[info.Status] {
			return info, nil
		}
		select {
		case <-ctx.Done():
			return job{}, ctx.Err()
		case <-time.After(poll):
		}
	}
}

func waitForBatch(ctx context.Context, client *apiClient, batchID string, poll time.Duration) (batchSummary, error) {
	for {
		summary, err := fetchBatchSummary(ctx, client, batchID)
		if err != nil {
			return batchSummary{}, err
		}
		if batchFinished(summary) {
			return summary, nil
		}
		select {
		case <-ctx.Done():
			return batchSummary{}, ctx.Err()
		case <-time.After(poll):
		}
	}
}

func batchFinished(summary batchSummary) bool {
	for _, item := range summary.Items {
		if !doneStatuses[item.Status] {
			return false
		}
	}
	return true
}

func fetchBatchSummary(ctx context.Context, client *apiClient, batchID string) (batchSummary, error) {
	var summary batchSummary
	if err := client.doJSON(ctx, http.MethodGet, "/batches/"+batchID, nil, &summary); err != nil {
		return batchSummary{}, err
	}
	if summary.Total <= len(summary.Items) {
		return summary, nil
	}

	const pageLimit = 500
	items := make([]job, 0, summary.Total)
	items = append(items, summary.Items...)
	offset := len(summary.Items)

	for offset < summary.Total {
		query := url.Values{}
		query.Set("limit", fmt.Sprintf("%d", pageLimit))
		query.Set("offset", fmt.Sprintf("%d", offset))

		var page batchSummary
		path := "/batches/" + batchID + "?" + query.Encode()
		if err := client.doJSON(ctx, http.MethodGet, path, nil, &page); err != nil {
			return batchSummary{}, err
		}
		if len(page.Items) == 0 {
			break
		}
		items = append(items, page.Items...)
		offset += len(page.Items)
	}

	summary.Items = items
	return summary, nil
}

func listAllJobIDs(ctx context.Context, client *apiClient, status string, createdAfter string) ([]string, error) {
	limit := 500
	offset := 0
	var ids []string
	for {
		query := url.Values{}
		query.Set("limit", fmt.Sprintf("%d", limit))
		query.Set("offset", fmt.Sprintf("%d", offset))
		if status != "" {
			query.Set("status", status)
		}
		if createdAfter != "" {
			query.Set("created_after", createdAfter)
		}
		var list jobList
		path := "/jobs?" + query.Encode()
		if err := client.doJSON(ctx, http.MethodGet, path, nil, &list); err != nil {
			return nil, err
		}
		for _, item := range list.Items {
			ids = append(ids, item.ID)
		}
		offset += limit
		if len(ids) >= list.Total || len(list.Items) == 0 {
			break
		}
	}
	return ids, nil
}

func resolveView(format string, view string) (string, error) {
	view = strings.ToLower(strings.TrimSpace(view))
	switch view {
	case "translated":
		view = "modules"
	case "full":
		view = "json"
	}
	if view == "" {
		if format == "json" || format == "jsonl" {
			view = "json"
		} else {
			view = "summary"
		}
	}
	switch view {
	case "summary", "modules", "raw", "json":
		return view, nil
	default:
		return "", fmt.Errorf("view must be summary, modules, raw, or json")
	}
}

func parseLevels(value string) (map[string]bool, error) {
	if strings.TrimSpace(value) == "" {
		return map[string]bool{
			"NOTICE":   true,
			"WARNING":  true,
			"ERROR":    true,
			"CRITICAL": true,
		}, nil
	}
	levels := map[string]bool{}
	parts := strings.Split(value, ",")
	for _, part := range parts {
		level := strings.ToUpper(strings.TrimSpace(part))
		if level == "" {
			continue
		}
		levels[level] = true
	}
	if len(levels) == 0 {
		return nil, fmt.Errorf("levels must not be empty")
	}
	return levels, nil
}

func buildSummaryViews(results []jobResult, levels map[string]bool, domains map[string]string) []summaryView {
	views := make([]summaryView, 0, len(results))
	for _, result := range results {
		views = append(views, buildSummaryView(result, levels, domains[result.JobID]))
	}
	return views
}

func buildSummaryView(result jobResult, levels map[string]bool, domain string) summaryView {
	total, counts, errMsg := extractSummaryCounts(result, levels)
	return summaryView{
		JobID:  result.JobID,
		Domain: domain,
		Status: result.Status,
		Total:  total,
		Levels: counts,
		Error:  errMsg,
	}
}

func buildModulesViews(results []jobResult, levels map[string]bool, domains map[string]string) []modulesView {
	views := make([]modulesView, 0, len(results))
	for _, result := range results {
		views = append(views, buildModulesView(result, levels, domains[result.JobID]))
	}
	return views
}

func buildModulesView(result jobResult, levels map[string]bool, domain string) modulesView {
	return modulesView{
		JobID:   result.JobID,
		Domain:  domain,
		Status:  result.Status,
		Modules: groupEntriesByModule(result.Raw, levels),
		Error:   extractError(result),
	}
}

func buildRawViews(results []jobResult, levels map[string]bool, domains map[string]string) []rawView {
	views := make([]rawView, 0, len(results))
	for _, result := range results {
		views = append(views, buildRawView(result, levels, domains[result.JobID]))
	}
	return views
}

func buildRawView(result jobResult, levels map[string]bool, domain string) rawView {
	return rawView{
		JobID:   result.JobID,
		Domain:  domain,
		Status:  result.Status,
		Entries: filterEntries(result.Raw, levels),
		Error:   extractError(result),
	}
}

func buildAggregateSummary(results []jobResult, levels map[string]bool) aggregateView {
	counts := map[string]int{}
	total := 0
	for _, result := range results {
		t, c, _ := extractSummaryCounts(result, levels)
		total += t
		for level, count := range c {
			counts[level] += count
		}
	}
	summary := &summaryView{
		JobID:  "aggregate",
		Status: "aggregate",
		Total:  total,
		Levels: counts,
	}
	return aggregateView{
		JobCount: len(results),
		Summary:  summary,
	}
}

func buildAggregateModules(results []jobResult, levels map[string]bool) aggregateView {
	entries := []jobResultEntry{}
	for _, result := range results {
		entries = append(entries, filterEntries(result.Raw, levels)...)
	}
	return aggregateView{
		JobCount: len(results),
		Modules:  groupEntriesByModule(&jobResultRaw{Entries: entries}, levels),
	}
}

func buildAggregateRaw(results []jobResult, levels map[string]bool) aggregateView {
	entries := []jobResultEntry{}
	for _, result := range results {
		entries = append(entries, filterEntries(result.Raw, levels)...)
	}
	return aggregateView{
		JobCount: len(results),
		Entries:  entries,
	}
}

func extractSummaryCounts(result jobResult, levels map[string]bool) (int, map[string]int, string) {
	if result.Raw != nil && len(result.Raw.Entries) > 0 {
		counts := map[string]int{}
		total := 0
		for _, entry := range result.Raw.Entries {
			level := strings.ToUpper(entry.Level)
			if !levels[level] {
				continue
			}
			counts[level]++
			total++
		}
		return total, counts, extractError(result)
	}
	counts := map[string]int{}
	total := 0
	if result.Summary != nil {
		if rawLevels, ok := result.Summary["levels"]; ok {
			if levelMap, ok := rawLevels.(map[string]any); ok {
				for key, value := range levelMap {
					level := strings.ToUpper(key)
					if !levels[level] {
						continue
					}
					if num, ok := value.(float64); ok {
						counts[level] = int(num)
						total += int(num)
					}
				}
			}
		}
		if rawTotal, ok := result.Summary["total"]; ok {
			if num, ok := rawTotal.(float64); ok {
				total = int(num)
			}
		}
	}
	return total, counts, extractError(result)
}

func extractError(result jobResult) string {
	if result.Summary == nil {
		return ""
	}
	if rawErr, ok := result.Summary["error"]; ok {
		if message, ok := rawErr.(string); ok {
			return message
		}
	}
	return ""
}

func groupEntriesByModule(raw *jobResultRaw, levels map[string]bool) []moduleGroup {
	entries := filterEntries(raw, levels)
	if len(entries) == 0 {
		return nil
	}
	order := []string{}
	groups := map[string]*moduleGroup{}
	for _, entry := range entries {
		module := entry.Module
		if module == "" {
			module = "Unspecified"
		}
		key := strings.ToUpper(module)
		group, ok := groups[key]
		if !ok {
			group = &moduleGroup{
				Module: module,
				Counts: map[string]int{},
			}
			groups[key] = group
			order = append(order, key)
		}
		level := strings.ToUpper(entry.Level)
		group.Counts[level]++
		group.Entries = append(group.Entries, entry)
	}
	result := make([]moduleGroup, 0, len(order))
	for _, key := range order {
		result = append(result, *groups[key])
	}
	return result
}

func filterEntries(raw *jobResultRaw, levels map[string]bool) []jobResultEntry {
	if raw == nil {
		return nil
	}
	filtered := make([]jobResultEntry, 0, len(raw.Entries))
	for _, entry := range raw.Entries {
		level := strings.ToUpper(entry.Level)
		if !levels[level] {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

func printSummaryPretty(out io.Writer, view summaryView) {
	fmt.Fprintf(out, "Job %s (%s)\n", view.JobID, view.Status)
	if view.Domain != "" {
		fmt.Fprintf(out, "  Domain: %s\n", view.Domain)
	}
	if view.Error != "" {
		fmt.Fprintf(out, "  Error: %s\n", view.Error)
	}
	if len(view.Levels) == 0 {
		fmt.Fprintln(out, "  No matching results.")
		return
	}
	for _, level := range orderedLevels() {
		if count, ok := view.Levels[level]; ok && count > 0 {
			fmt.Fprintf(out, "  %s: %d\n", level, count)
		}
	}
}

func printAggregateSummaryPretty(out io.Writer, agg aggregateView) {
	if agg.Summary == nil {
		return
	}
	fmt.Fprintf(out, "Aggregate (%d jobs)\n", agg.JobCount)
	if len(agg.Summary.Levels) == 0 {
		fmt.Fprintln(out, "  No matching results.")
		return
	}
	for _, level := range orderedLevels() {
		if count, ok := agg.Summary.Levels[level]; ok && count > 0 {
			fmt.Fprintf(out, "  %s: %d\n", level, count)
		}
	}
}

func printModulesPretty(out io.Writer, view modulesView) {
	fmt.Fprintf(out, "Job %s (%s)\n", view.JobID, view.Status)
	if view.Domain != "" {
		fmt.Fprintf(out, "  Domain: %s\n", view.Domain)
	}
	if view.Error != "" {
		fmt.Fprintf(out, "  Error: %s\n", view.Error)
	}
	if len(view.Modules) == 0 {
		fmt.Fprintln(out, "  No matching results.")
		return
	}
	for _, module := range view.Modules {
		fmt.Fprintf(out, "%s\n", module.Module)
		printLevelCounts(out, module.Counts, "  ")
		for _, entry := range module.Entries {
			fmt.Fprintf(out, "  [%s] %s\n", strings.ToUpper(entry.Level), entryText(entry))
		}
	}
}

func printAggregateModulesPretty(out io.Writer, agg aggregateView) {
	fmt.Fprintf(out, "Aggregate (%d jobs)\n", agg.JobCount)
	if len(agg.Modules) == 0 {
		fmt.Fprintln(out, "  No matching results.")
		return
	}
	for _, module := range agg.Modules {
		fmt.Fprintf(out, "%s\n", module.Module)
		printLevelCounts(out, module.Counts, "  ")
		for _, entry := range module.Entries {
			fmt.Fprintf(out, "  [%s] %s\n", strings.ToUpper(entry.Level), entryText(entry))
		}
	}
}

func printRawPretty(out io.Writer, view rawView) {
	fmt.Fprintf(out, "Job %s (%s)\n", view.JobID, view.Status)
	if view.Domain != "" {
		fmt.Fprintf(out, "  Domain: %s\n", view.Domain)
	}
	if view.Error != "" {
		fmt.Fprintf(out, "  Error: %s\n", view.Error)
	}
	if len(view.Entries) == 0 {
		fmt.Fprintln(out, "  No matching results.")
		return
	}
	for _, entry := range view.Entries {
		fmt.Fprintf(out, "[%s] %s\n", strings.ToUpper(entry.Level), entryText(entry))
	}
}

func printAggregateRawPretty(out io.Writer, agg aggregateView) {
	fmt.Fprintf(out, "Aggregate (%d jobs)\n", agg.JobCount)
	if len(agg.Entries) == 0 {
		fmt.Fprintln(out, "  No matching results.")
		return
	}
	for _, entry := range agg.Entries {
		fmt.Fprintf(out, "[%s] %s\n", strings.ToUpper(entry.Level), entryText(entry))
	}
}

func printLevelCounts(out io.Writer, counts map[string]int, prefix string) {
	for _, level := range orderedLevels() {
		if count, ok := counts[level]; ok && count > 0 {
			fmt.Fprintf(out, "%s%s: %d\n", prefix, level, count)
		}
	}
}

func orderedLevels() []string {
	return []string{"CRITICAL", "ERROR", "WARNING", "NOTICE", "INFO", "DEBUG", "DEBUG2", "DEBUG3"}
}

func entryText(entry jobResultEntry) string {
	if entry.Message != "" {
		return entry.Message
	}
	if entry.Raw != "" {
		return entry.Raw
	}
	parts := []string{}
	if entry.Module != "" {
		parts = append(parts, entry.Module)
	}
	if entry.Testcase != "" {
		parts = append(parts, entry.Testcase)
	}
	if entry.Tag != "" {
		parts = append(parts, entry.Tag)
	}
	return strings.Join(parts, ":")
}

func outputExt(format string) string {
	switch format {
	case "json":
		return ".json"
	case "jsonl":
		return ".jsonl"
	default:
		return ".txt"
	}
}
