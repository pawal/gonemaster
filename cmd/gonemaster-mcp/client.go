package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
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
