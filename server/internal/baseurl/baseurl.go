// Package baseurl resolves the deployment's canonical base URL, which is the
// site root: the public UI lives under "public/", the API under "pub/api/v1/".
package baseurl

import "net/http"

// Resolve returns the base URL, always ending in "/". An empty configured
// value means auto-detect from the request.
func Resolve(configured string, r *http.Request) string {
	if configured != "" {
		if configured[len(configured)-1] != '/' {
			configured += "/"
		}
		return configured
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto == "https" || proto == "http" {
		scheme = proto
	}
	host := r.Host
	if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
		host = fwdHost
	}
	// Go accepts quotes in Host, which would break out of an attribute.
	if !hostLooksValid(host) {
		return "/"
	}
	return scheme + "://" + host + "/"
}

func hostLooksValid(host string) bool {
	if host == "" {
		return false
	}
	for _, c := range host {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '-', c == '.', c == ':', c == '[', c == ']', c == '_':
		default:
			return false
		}
	}
	return true
}
