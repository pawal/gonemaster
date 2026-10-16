package server

import (
	"net/http"
	"net/netip"
	"strings"
)

// Column widths of jobs.client_ip and jobs.client_user_agent.
const (
	maxClientIPLen        = 64
	maxClientUserAgentLen = 256
)

// ClientVia values: a browser on the server's own pages, or anything else.
const (
	ClientViaUI  = "ui"
	ClientViaAPI = "api"
)

// submitter is the client recorded on a job.
type submitter struct {
	IP        string
	UserAgent string
	Via       string
}

// submitterOf returns the client of r as stored on a job.
func (s *Server) submitterOf(r *http.Request) submitter {
	via := ClientViaAPI
	if r.Header.Get("Origin") != "" && validateCSRFOrigin(r, s.trustedProxies) == nil {
		via = ClientViaUI
	}
	return submitter{
		IP:        clientField(clientIP(r, s.trustedProxies), maxClientIPLen),
		UserAgent: clientField(r.UserAgent(), maxClientUserAgentLen),
		Via:       via,
	}
}

// stamp records c on job.
func (c submitter) stamp(job *Job) {
	job.ClientIP, job.ClientUserAgent, job.ClientVia = c.IP, c.UserAgent, c.Via
}

// clientField maps runes outside printable ASCII to '?' and cuts to n bytes.
func clientField(v string, n int) string {
	v = strings.Map(func(r rune) rune {
		if r < 0x20 || r > 0x7e {
			return '?'
		}
		return r
	}, v)
	if len(v) > n {
		v = v[:n]
	}
	return v
}

// clientIPFilter puts an address filter in the stored form.
func clientIPFilter(v string) string {
	v = strings.TrimSpace(v)
	if a, err := netip.ParseAddr(v); err == nil {
		return a.Unmap().String()
	}
	return v
}
