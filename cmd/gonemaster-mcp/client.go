package main

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
)

// apiClient is a thin bearer-authenticated HTTP client for the admin API.
type apiClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func newAPIClient(cfg config) (*apiClient, error) {
	base, err := normalizeBaseURL(cfg.serverURL)
	if err != nil {
		return nil, err
	}
	return &apiClient{
		baseURL:    base,
		token:      cfg.token,
		httpClient: &http.Client{Timeout: cfg.timeout},
	}, nil
}

// normalizeBaseURL ensures the URL ends in the /api/v1 admin prefix.
func normalizeBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultServerURL, nil
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
func (c *apiClient) doJSON(ctx context.Context, method, path string, body, out any) error {
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

func (c *apiClient) whoami(ctx context.Context) (whoamiResponse, error) {
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
	JobID  string         `json:"job_id"`
	Status string         `json:"status"`
	Raw    *resultRawView `json:"raw,omitempty"`
	Score  *resultScore   `json:"score,omitempty"`
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

func (c *apiClient) createJob(ctx context.Context, req createJobRequest) (jobView, error) {
	var out jobView
	err := c.doJSON(ctx, http.MethodPost, "/jobs", req, &out)
	return out, err
}

func (c *apiClient) getJob(ctx context.Context, id string) (jobView, error) {
	var out jobView
	err := c.doJSON(ctx, http.MethodGet, "/jobs/"+url.PathEscape(id), nil, &out)
	return out, err
}

func (c *apiClient) getRun(ctx context.Context, id string) (runView, error) {
	var out runView
	err := c.doJSON(ctx, http.MethodGet, "/runs/"+url.PathEscape(id), nil, &out)
	return out, err
}

func (c *apiClient) getResult(ctx context.Context, id, locale string) (resultView, error) {
	var out resultView
	path := "/jobs/" + url.PathEscape(id) + "/result"
	if locale != "" {
		path += "?locale=" + url.QueryEscape(locale)
	}
	err := c.doJSON(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// specTestcaseView decodes one item from GET /spec/testcases.
type specTestcaseView struct {
	ID          string `json:"id"`
	Module      string `json:"module"`
	Description string `json:"description"`
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
	Locale      string        `json:"locale"`
	Tags        []specTagView `json:"tags"`
}

func (c *apiClient) listSpecTestcases(ctx context.Context, category string) (specTestcaseListView, error) {
	var out specTestcaseListView
	path := "/spec/testcases"
	if category != "" {
		path += "?category=" + url.QueryEscape(category)
	}
	err := c.doJSON(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

func (c *apiClient) getSpecTestcase(ctx context.Context, id, locale string) (specTestcaseDetailView, error) {
	var out specTestcaseDetailView
	path := "/spec/testcases/" + url.PathEscape(id)
	if locale != "" {
		path += "?locale=" + url.QueryEscape(locale)
	}
	err := c.doJSON(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

func (c *apiClient) listRuns(ctx context.Context, domain string, limit int) (runListView, error) {
	var out runListView
	q := url.Values{}
	if domain != "" {
		q.Set("domain", domain)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	path := "/runs"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	err := c.doJSON(ctx, http.MethodGet, path, nil, &out)
	return out, err
}
