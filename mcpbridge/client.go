package mcpbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"codeberg.org/pawal/gonemaster/internal/publicapi"
)

// DefaultServerURL is the admin API base when none is configured.
const DefaultServerURL = "http://localhost:8080/api/v1"

// Client is a thin bearer-authenticated HTTP client for the admin API.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
	tokenHint  string
	urlHint    string
}

// NewClient targets the admin API at baseURL; a nil hc gets a 30 s timeout.
func NewClient(baseURL, token string, hc *http.Client) (*Client, error) {
	base, err := normalizeBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{
		baseURL:    base,
		token:      token,
		httpClient: hc,
		tokenHint:  "set GONEMASTER_TOKEN to a valid admin token",
		urlHint:    "check GONEMASTER_URL",
	}, nil
}

// WithHints replaces the remedies named in auth and connectivity errors.
func (c *Client) WithHints(tokenHint, urlHint string) *Client {
	c.tokenHint, c.urlHint = tokenHint, urlHint
	return c
}

// BaseURL is the normalized admin API base.
func (c *Client) BaseURL() string { return c.baseURL }

// normalizeBaseURL ensures the URL ends in the /api/v1 admin prefix.
func normalizeBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DefaultServerURL, nil
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid server URL: %w", err)
	}
	path := strings.TrimRight(parsed.Path, "/")
	switch {
	case path == "":
		path = "/api/v1"
	case strings.HasPrefix(path, "/api/v1"):
		path = "/api/v1"
	default:
		path += "/api/v1"
	}
	parsed.Path = path
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

// doJSON sends a JSON request and decodes the response into out. Non-2xx returns
// an *httpError carrying the status code.
func (c *Client) doJSON(ctx context.Context, method, path string, body, out any) error {
	return c.doJSONURL(ctx, method, strings.TrimRight(c.baseURL, "/")+path, body, out)
}

// doJSONURL is doJSON against an absolute URL, for the public API, which
// sits beside the admin base rather than under it.
func (c *Client) doJSONURL(ctx context.Context, method, full string, body, out any) error {
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
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
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
		return &httpError{status: resp.StatusCode, body: strings.TrimSpace(string(raw))}
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// httpError carries a non-2xx status from the admin API.
type httpError struct {
	status int
	body   string
}

func (e *httpError) Error() string {
	if e.body == "" {
		return fmt.Sprintf("http %d", e.status)
	}
	return fmt.Sprintf("http %d: %s", e.status, e.body)
}

// whoamiResponse mirrors GET /api/v1/whoami.
type whoamiResponse struct {
	Mode          string `json:"mode"`
	Authenticated bool   `json:"authenticated"`
}

func (c *Client) whoami(ctx context.Context) (whoamiResponse, error) {
	var out whoamiResponse
	err := c.doJSON(ctx, http.MethodGet, "/whoami", nil, &out)
	return out, err
}

// createJobRequest is the POST /api/v1/jobs payload (minimal subset).
type createJobRequest struct {
	Domain    string `json:"domain"`
	ProfileID *int64 `json:"profile_id,omitempty"`
}

// jobView decodes a job from POST /jobs and GET /jobs/{id}.
type jobView struct {
	ID         string    `json:"id"`
	Domain     string    `json:"domain"`
	BatchID    string    `json:"batch_id"`
	PublicID   string    `json:"public_id"`
	Status     string    `json:"status"`
	Progress   int       `json:"progress"`
	Error      string    `json:"error"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
}

// runView decodes a completed run from GET /runs and GET /runs/{id}.
type runView struct {
	ID         string    `json:"id"`
	Domain     string    `json:"domain"`
	BatchID    string    `json:"batch_id"`
	PublicID   string    `json:"public_id"`
	Status     string    `json:"status"`
	DurationMs int64     `json:"duration_ms"`
	WorstLevel string    `json:"worst_level"`
	Score      *int      `json:"score"`
	Grade      *string   `json:"grade"`
	FinishedAt time.Time `json:"finished_at"`
	Error      string    `json:"error"`
}

type runListView struct {
	Items []runView `json:"items"`
	Total int       `json:"total"`
}

// resultView decodes GET /jobs/{id}/result (and the identical /runs/{id}/result).
type resultView struct {
	JobID             string         `json:"job_id"`
	Status            string         `json:"status"`
	Raw               *resultRawView `json:"raw,omitempty"`
	Score             *resultScore   `json:"score,omitempty"`
	NameserverTimings []nsTimingView `json:"nameserver_timings,omitempty"`
}

// nsTimingView decodes one entry of nameserver_timings.
type nsTimingView struct {
	Nameserver string  `json:"nameserver"`
	Address    string  `json:"address"`
	AvgMS      float64 `json:"avg_ms"`
	MinMS      float64 `json:"min_ms"`
	MaxMS      float64 `json:"max_ms"`
	MedianMS   float64 `json:"median_ms"`
	Count      int     `json:"count"`
	Status     string  `json:"status"`
}

type resultRawView struct {
	Entries []entryView `json:"entries"`
}

type resultScore struct {
	Score int    `json:"score"`
	Grade string `json:"grade"`
}

type entryView struct {
	Module   string `json:"module"`
	Testcase string `json:"testcase"`
	Tag      string `json:"tag"`
	Level    string `json:"level"`
	Message  string `json:"message"`
}

func (c *Client) createJob(ctx context.Context, req createJobRequest) (jobView, error) {
	var out jobView
	err := c.doJSON(ctx, http.MethodPost, "/jobs", req, &out)
	return out, err
}

func (c *Client) getJob(ctx context.Context, id string) (jobView, error) {
	var out jobView
	err := c.doJSON(ctx, http.MethodGet, "/jobs/"+url.PathEscape(id), nil, &out)
	return out, err
}

func (c *Client) getRun(ctx context.Context, id string) (runView, error) {
	var out runView
	err := c.doJSON(ctx, http.MethodGet, "/runs/"+url.PathEscape(id), nil, &out)
	return out, err
}

func (c *Client) getResult(ctx context.Context, id, locale string) (resultView, error) {
	var out resultView
	path := "/jobs/" + url.PathEscape(id) + "/result"
	if locale != "" {
		path += "?locale=" + url.QueryEscape(locale)
	}
	err := c.doJSON(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// batchCreateRequest is the POST /api/v1/jobs/batch payload (minimal subset).
type batchCreateRequest struct {
	Domains []string `json:"domains,omitempty"`
	FromTag string   `json:"from_tag,omitempty"`
	Profile string   `json:"profile,omitempty"`
	Tags    []string `json:"tags,omitempty"`
}

type batchCreateResponse struct {
	BatchID string   `json:"batch_id"`
	JobIDs  []string `json:"job_ids"`
}

func (c *Client) createBatch(ctx context.Context, req batchCreateRequest) (batchCreateResponse, error) {
	var out batchCreateResponse
	err := c.doJSON(ctx, http.MethodPost, "/jobs/batch", req, &out)
	return out, err
}

func (c *Client) cancelJob(ctx context.Context, id string) (jobView, error) {
	var out jobView
	err := c.doJSON(ctx, http.MethodPost, "/jobs/"+url.PathEscape(id)+"/cancel", nil, &out)
	return out, err
}

func (c *Client) deleteBatch(ctx context.Context, id string) error {
	return c.doJSON(ctx, http.MethodDelete, "/batches/"+url.PathEscape(id), nil, nil)
}

// batchSummaryView decodes GET /batches/{id}.
type batchSummaryView struct {
	BatchID      string         `json:"batch_id"`
	Tag          string         `json:"tag"`
	Total        int            `json:"total"`
	StatusCounts map[string]int `json:"status_counts"`
	Grades       map[string]int `json:"grades"`
	WorstLevels  map[string]int `json:"worst_levels"`
	CreatedAt    time.Time      `json:"created_at"`
	FinishedAt   *time.Time     `json:"finished_at"`
}

// entryRecord decodes one item from GET /entries.
type entryRecord struct {
	Domain string `json:"domain"`
	Module string `json:"module"`
	Tag    string `json:"tag"`
	Level  string `json:"level"`
}

type entryListView struct {
	Items []entryRecord `json:"items"`
	Total int           `json:"total"`
}

func (c *Client) getBatch(ctx context.Context, id string) (batchSummaryView, error) {
	var out batchSummaryView
	err := c.doJSON(ctx, http.MethodGet, "/batches/"+url.PathEscape(id), nil, &out)
	return out, err
}

// batchListItemView decodes one row of GET /batches.
type batchListItemView struct {
	BatchID     string     `json:"batch_id"`
	Tag         string     `json:"tag,omitempty"`
	Description string     `json:"description,omitempty"`
	Status      string     `json:"status"`
	Total       int        `json:"total"`
	Completed   int        `json:"completed"`
	Completion  int        `json:"completion"`
	CreatedAt   time.Time  `json:"created_at"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
}

type batchListView struct {
	Items []batchListItemView `json:"items"`
	Total int                 `json:"total"`
}

func (c *Client) listBatches(ctx context.Context, q url.Values) (batchListView, error) {
	var out batchListView
	path := "/batches"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	err := c.doJSON(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

func (c *Client) listEntries(ctx context.Context, q url.Values) (entryListView, error) {
	var out entryListView
	path := "/entries"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	err := c.doJSON(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// tagValueRollupView decodes one row of GET /batches/{id}/tag-values.
type tagValueRollupView struct {
	Value         string   `json:"value"`
	Count         int      `json:"count"`
	AvgScore      *float64 `json:"avg_score,omitempty"`
	SampleDomains []string `json:"sample_domains"`
}

type batchTagValuesView struct {
	BatchID       string               `json:"batch_id"`
	Tag           string               `json:"tag"`
	Arg           string               `json:"arg"`
	MinCount      int                  `json:"min_count"`
	WeightByScore bool                 `json:"weight_by_score,omitempty"`
	Values        []tagValueRollupView `json:"values"`
}

func (c *Client) getBatchTagValues(ctx context.Context, id string, q url.Values) (batchTagValuesView, error) {
	var out batchTagValuesView
	path := "/batches/" + url.PathEscape(id) + "/tag-values"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	err := c.doJSON(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// specTestcaseView decodes one item from GET /spec/testcases.
type specTestcaseView struct {
	ID          string `json:"id"`
	Module      string `json:"module"`
	Description string `json:"description"`
	Excluded    bool   `json:"excluded"`
}

type specTestcaseListView struct {
	Items []specTestcaseView `json:"items"`
	Total int                `json:"total"`
}

type specTagView struct {
	Tag     string `json:"tag"`
	Message string `json:"message"`
}

// specTestcaseDetailView decodes GET /spec/testcases/{id} (flat shape).
type specTestcaseDetailView struct {
	ID          string        `json:"id"`
	Module      string        `json:"module"`
	Description string        `json:"description"`
	Excluded    bool          `json:"excluded"`
	Locale      string        `json:"locale"`
	Tags        []specTagView `json:"tags"`
}

func (c *Client) listSpecTestcases(ctx context.Context, category string) (specTestcaseListView, error) {
	var out specTestcaseListView
	path := "/spec/testcases"
	if category != "" {
		path += "?category=" + url.QueryEscape(category)
	}
	err := c.doJSON(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

func (c *Client) getSpecTestcase(ctx context.Context, id, locale string) (specTestcaseDetailView, error) {
	var out specTestcaseDetailView
	path := "/spec/testcases/" + url.PathEscape(id)
	if locale != "" {
		path += "?locale=" + url.QueryEscape(locale)
	}
	err := c.doJSON(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

func (c *Client) getRuns(ctx context.Context, q url.Values) (runListView, error) {
	var out runListView
	path := "/runs"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	err := c.doJSON(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

func (c *Client) listRuns(ctx context.Context, domain string, limit int) (runListView, error) {
	q := url.Values{}
	if domain != "" {
		q.Set("domain", domain)
		q.Set("domain_exact", "1")
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	return c.getRuns(ctx, q)
}

// profileView decodes one item of GET /profiles.
type profileView struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Public      bool   `json:"public"`
}

func (c *Client) listProfiles(ctx context.Context) ([]profileView, error) {
	var out []profileView
	err := c.doJSON(ctx, http.MethodGet, "/profiles", nil, &out)
	return out, err
}

// domainTagView decodes one item of GET /tags, a user domain tag.
type domainTagView struct {
	Name             string `json:"name"`
	Description      string `json:"description"`
	DomainCount      int    `json:"domain_count"`
	DefaultProfileID *int64 `json:"default_profile_id"`
}

func (c *Client) listDomainTags(ctx context.Context, limit int) ([]domainTagView, error) {
	var out []domainTagView
	path := "/tags"
	if limit > 0 {
		path += "?limit=" + strconv.Itoa(limit)
	}
	err := c.doJSON(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// cohortScheduleView decodes one item of GET /analysis/schedules.
type cohortScheduleView struct {
	SourceTag   string `json:"source_tag"`
	Label       string `json:"label"`
	Enabled     bool   `json:"enabled"`
	Summary     string `json:"summary"`
	NextRunAt   string `json:"next_run_at"`
	LastRunAt   string `json:"last_run_at"`
	LastOutcome string `json:"last_outcome"`
	LastBatchID string `json:"last_batch_id"`
}

func (c *Client) listCohortSchedules(ctx context.Context) ([]cohortScheduleView, error) {
	var out []cohortScheduleView
	err := c.doJSON(ctx, http.MethodGet, "/analysis/schedules", nil, &out)
	return out, err
}

// getPublic decodes a GET against the public API into out.
func (c *Client) getPublic(ctx context.Context, path string, out any) error {
	return c.doJSONURL(ctx, http.MethodGet, publicapi.Base(c.baseURL)+path, nil, out)
}

// analysisCohortView is one cohort of GET /analysis/catalog.
type analysisCohortView struct {
	DatasetTag string `json:"dataset_tag"`
	Label      string `json:"label"`
	IsDefault  bool   `json:"is_default"`
}

type analysisCatalogView struct {
	DefaultTag string               `json:"default_tag"`
	Cohorts    []analysisCohortView `json:"cohorts"`
}

// analysisSnapshotView is one row of a cohort's snapshot list.
type analysisSnapshotView struct {
	Slug        string    `json:"slug"`
	Label       string    `json:"label"`
	CapturedAt  time.Time `json:"captured_at"`
	DomainCount int       `json:"domain_count"`
}

type analysisSnapshotListView struct {
	DatasetTag string                 `json:"dataset_tag"`
	Snapshots  []analysisSnapshotView `json:"snapshots"`
}

// The report views below decode only the fields the tool reports.
type reportVocabularyView struct {
	FromAvailable bool `json:"from_available"`
	ToAvailable   bool `json:"to_available"`
	FromTagCount  int  `json:"from_tag_count"`
	ToTagCount    int  `json:"to_tag_count"`
	Added         []struct {
		Tag string `json:"tag"`
	} `json:"added"`
	Removed []struct {
		Tag string `json:"tag"`
	} `json:"removed"`
	LevelChanged []struct {
		Tag string `json:"tag"`
	} `json:"level_changed"`
}

type reportSideView struct {
	Slug          string    `json:"slug"`
	CapturedAt    time.Time `json:"captured_at"`
	EngineVersion string    `json:"engine_version"`
	ProfileName   string    `json:"profile_name"`
	DomainCount   int       `json:"domain_count"`
}

type reportHeaderView struct {
	From                 reportSideView       `json:"from"`
	To                   reportSideView       `json:"to"`
	Vocabulary           reportVocabularyView `json:"vocabulary"`
	ScoringConfigChanged string               `json:"scoring_config_changed"`
	TagFloor             string               `json:"tag_floor"`
}

type reportTotalsView struct {
	FromDomainCount  int            `json:"from_domain_count"`
	ToDomainCount    int            `json:"to_domain_count"`
	BothDomainCount  int            `json:"both_domain_count"`
	Added            int            `json:"added"`
	Removed          int            `json:"removed"`
	IdenticalScore   int            `json:"identical_score"`
	Improved         int            `json:"improved"`
	Regressed        int            `json:"regressed"`
	FromMeanScore    *float64       `json:"from_mean_score"`
	ToMeanScore      *float64       `json:"to_mean_score"`
	FromGrades       map[string]int `json:"from_grades"`
	ToGrades         map[string]int `json:"to_grades"`
	DomainCategories map[string]int `json:"domain_categories"`
}

type reportTagEntryView struct {
	Tag            string `json:"tag"`
	Module         string `json:"module"`
	FromLevel      string `json:"from_level"`
	ToLevel        string `json:"to_level"`
	DomainDelta    int    `json:"domain_delta"`
	Classification string `json:"classification"`
}

type reportTagsView struct {
	Appeared     []reportTagEntryView `json:"appeared"`
	Cleared      []reportTagEntryView `json:"cleared"`
	LevelChanged []reportTagEntryView `json:"level_changed"`
}

type reportDomainView struct {
	Domain           string `json:"domain"`
	FromScore        *int   `json:"from_score"`
	ToScore          *int   `json:"to_score"`
	ScoreDelta       *int   `json:"score_delta"`
	FromGrade        string `json:"from_grade"`
	ToGrade          string `json:"to_grade"`
	Category         string `json:"category"`
	ExplainedDelta   int    `json:"explained_delta"`
	UnexplainedDelta int    `json:"unexplained_delta"`
	Appeared         []struct {
		Tag            string `json:"tag"`
		Classification string `json:"classification"`
	} `json:"appeared"`
	Cleared []struct {
		Tag            string `json:"tag"`
		Classification string `json:"classification"`
	} `json:"cleared"`
}

type reportClusterView struct {
	Dimensions []struct {
		Dimension    string `json:"dimension"`
		Value        string `json:"value"`
		Label        string `json:"label"`
		TotalDomains int    `json:"total_domains"`
	} `json:"dimensions"`
	Domains   []string `json:"domains"`
	Size      int      `json:"size"`
	MinDelta  int      `json:"min_delta"`
	MaxDelta  int      `json:"max_delta"`
	Direction string   `json:"direction"`
}

type reportTransitionView struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Category string `json:"category,omitempty"`
	Count    int    `json:"count"`
}

type reportTransitionsView struct {
	Grade []reportTransitionView `json:"grade" jsonschema:"grade moves; equal from and to count the domains that held"`
	Level []reportTransitionView `json:"level" jsonschema:"worst-level moves: OK, NOTICE, WARNING, ERROR, CRITICAL"`
}

// analysisReportView decodes the cohort report endpoint.
type analysisReportView struct {
	DatasetTag  string                `json:"dataset_tag"`
	FromSlug    string                `json:"from_slug"`
	ToSlug      string                `json:"to_slug"`
	Header      reportHeaderView      `json:"header"`
	Totals      reportTotalsView      `json:"totals"`
	Tags        reportTagsView        `json:"tags"`
	Domains     []reportDomainView    `json:"domains"`
	Clusters    []reportClusterView   `json:"clusters"`
	Transitions reportTransitionsView `json:"transitions"`
}

func (c *Client) getAnalysisCatalog(ctx context.Context) (analysisCatalogView, error) {
	var out analysisCatalogView
	err := c.getPublic(ctx, "/analysis/catalog", &out)
	return out, err
}

func (c *Client) listAnalysisSnapshots(ctx context.Context, datasetTag string) (analysisSnapshotListView, error) {
	var out analysisSnapshotListView
	err := c.getPublic(ctx, "/analysis/cohorts/"+url.PathEscape(datasetTag)+"/snapshots", &out)
	return out, err
}

func (c *Client) getAnalysisReport(ctx context.Context, datasetTag string, q url.Values) (analysisReportView, error) {
	var out analysisReportView
	path := "/analysis/cohorts/" + url.PathEscape(datasetTag) + "/report"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	err := c.getPublic(ctx, path, &out)
	return out, err
}
