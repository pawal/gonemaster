package baseurl

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolve(t *testing.T) {
	tests := []struct {
		name       string
		configured string
		host       string
		fwdProto   string
		fwdHost    string
		tls        bool
		want       string
	}{
		{
			name:       "configured root",
			configured: "https://example.com/",
			host:       "ignored.example.com",
			want:       "https://example.com/",
		},
		{
			name:       "configured subpath",
			configured: "https://example.com/public/",
			host:       "ignored.example.com",
			want:       "https://example.com/public/",
		},
		{
			name:       "configured without trailing slash gets one",
			configured: "https://example.com",
			host:       "ignored.example.com",
			want:       "https://example.com/",
		},
		{
			name: "auto-detect http",
			host: "myhost.example.com",
			want: "http://myhost.example.com/",
		},
		{
			name: "auto-detect tls",
			host: "myhost.example.com",
			tls:  true,
			want: "https://myhost.example.com/",
		},
		{
			name:     "X-Forwarded-Proto https",
			host:     "myhost.example.com",
			fwdProto: "https",
			want:     "https://myhost.example.com/",
		},
		{
			name:    "X-Forwarded-Host overrides Host",
			host:    "internal:8080",
			fwdHost: "public.example.com",
			want:    "http://public.example.com/",
		},
		{
			name: "IPv6 literal with port",
			host: "[2001:db8::1]:8080",
			want: "http://[2001:db8::1]:8080/",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Host = tt.host
			if tt.fwdProto != "" {
				req.Header.Set("X-Forwarded-Proto", tt.fwdProto)
			}
			if tt.fwdHost != "" {
				req.Header.Set("X-Forwarded-Host", tt.fwdHost)
			}
			if tt.tls {
				req.TLS = &tls.ConnectionState{}
			}
			if got := Resolve(tt.configured, req); got != tt.want {
				t.Fatalf("Resolve() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Go accepts a Host header carrying quotes or angle brackets, and the result is
// substituted into HTML attributes and XML, so a non-host value is refused.
func TestResolveRefusesHostileHost(t *testing.T) {
	for _, host := range []string{
		`evil"onload="alert(1)`,
		"a<script>b",
		"host with spaces",
		"host\\backslash",
		"",
	} {
		t.Run(host, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Host = host
			if got := Resolve("", req); got != "/" {
				t.Fatalf("Resolve() = %q, want %q", got, "/")
			}
		})
	}
}

// A configured value is set by the operator, so it is used as given.
func TestResolveTrustsConfiguredValue(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = `evil"onload="x`
	if got := Resolve("https://example.com/", req); got != "https://example.com/" {
		t.Fatalf("Resolve() = %q, want the configured URL", got)
	}
}
