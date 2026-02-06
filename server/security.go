package server

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
)

var errCSRFMismatch = errors.New("origin must match request host")

func validateCSRFOrigin(r *http.Request) error {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		// Non-browser clients (for example gonemaster-client) generally omit Origin.
		return nil
	}
	if strings.EqualFold(origin, "null") {
		return errCSRFMismatch
	}

	originURL, err := url.Parse(origin)
	if err != nil || originURL.Host == "" || originURL.Scheme == "" {
		return errCSRFMismatch
	}
	if originURL.Scheme != "http" && originURL.Scheme != "https" {
		return errCSRFMismatch
	}

	reqScheme := "http"
	if r.TLS != nil {
		reqScheme = "https"
	}

	originHost, originPort, err := normalizedHostPort(originURL.Host, originURL.Scheme)
	if err != nil {
		return errCSRFMismatch
	}
	reqHost, reqPort, err := normalizedHostPort(r.Host, reqScheme)
	if err != nil {
		return errCSRFMismatch
	}

	if !strings.EqualFold(originHost, reqHost) || originPort != reqPort {
		return errCSRFMismatch
	}
	return nil
}

func enforceCSRF(w http.ResponseWriter, r *http.Request) bool {
	if err := validateCSRFOrigin(r); err != nil {
		writeError(w, http.StatusForbidden, "csrf_origin_mismatch", err.Error(), nil)
		return false
	}
	return true
}

func normalizedHostPort(hostport string, scheme string) (string, string, error) {
	hostport = strings.TrimSpace(hostport)
	if hostport == "" {
		return "", "", errCSRFMismatch
	}

	host := hostport
	port := ""

	parsedHost, parsedPort, err := net.SplitHostPort(hostport)
	if err == nil {
		host = parsedHost
		port = parsedPort
	} else if !strings.Contains(err.Error(), "missing port in address") {
		return "", "", errCSRFMismatch
	}

	host = strings.TrimPrefix(host, "[")
	host = strings.TrimSuffix(host, "]")
	host = strings.TrimSpace(host)
	if host == "" {
		return "", "", errCSRFMismatch
	}

	if ip := net.ParseIP(host); ip != nil {
		host = ip.String()
	}
	host = strings.ToLower(host)

	if port == "" {
		port = defaultPortForScheme(scheme)
	}
	return host, port, nil
}

func defaultPortForScheme(scheme string) string {
	switch strings.ToLower(strings.TrimSpace(scheme)) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}
