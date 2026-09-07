package extdata

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"syscall"
	"time"
)

// maxResponseBytes caps every response body read from a third party.
const maxResponseBytes = 2 << 20

// maxRedirects caps a redirect chain.
const maxRedirects = 5

var (
	errInsecureScheme = errors.New("source URL must be https")
	errMissingHost    = errors.New("source URL has no host")
	errNoRDAPService  = errors.New("no RDAP service for this domain")
	errBodyTooLarge   = fmt.Errorf("response exceeds %d bytes", maxResponseBytes)
)

// cgnatPrefix is RFC 6598 shared address space, which netip omits.
var cgnatPrefix = netip.MustParsePrefix("100.64.0.0/10")

// blockedAddrReason names why an address must not be contacted, or "" when
// it is globally routable.
func blockedAddrReason(addr netip.Addr) string {
	addr = addr.Unmap()
	switch {
	case addr.IsLoopback():
		return "loopback"
	case addr.IsUnspecified():
		return "unspecified"
	case addr.IsLinkLocalUnicast():
		return "link-local"
	case addr.IsLinkLocalMulticast():
		return "link-local multicast"
	case addr.IsInterfaceLocalMulticast():
		return "interface-local multicast"
	case addr.IsMulticast():
		return "multicast"
	case addr.IsPrivate():
		return "private"
	case cgnatPrefix.Contains(addr):
		return "CGNAT"
	case addr.Is4() && addr.As4() == [4]byte{255, 255, 255, 255}:
		return "broadcast"
	}
	return ""
}

// blockedHostReason rejects literal non-global hosts. A DNS name passes here
// and is checked again at dial time.
func blockedHostReason(host string) string {
	addr, err := netip.ParseAddr(strings.Trim(host, "[]"))
	if err != nil {
		return ""
	}
	return blockedAddrReason(addr)
}

// newHTTPClient builds the outbound client: HTTPS only, guarded dialer,
// guarded redirects, and a hard per-request timeout.
func newHTTPClient(cfg Config) *http.Client {
	transport := cfg.Transport
	if transport == nil {
		// Control runs after resolution, so a name pointing at a private
		// address is refused too.
		dialer := &net.Dialer{
			Timeout:   cfg.Timeout,
			KeepAlive: 30 * time.Second,
			Control: func(network, address string, _ syscall.RawConn) error {
				host, _, err := net.SplitHostPort(address)
				if err != nil {
					return err
				}
				if reason := blockedHostReason(host); reason != "" {
					return fmt.Errorf("refusing %s destination %s", reason, host)
				}
				return nil
			},
		}
		transport = &http.Transport{
			DialContext:         dialer.DialContext,
			Proxy:               http.ProxyFromEnvironment,
			MaxIdleConns:        10,
			IdleConnTimeout:     90 * time.Second,
			TLSHandshakeTimeout: cfg.Timeout,
		}
	}
	return &http.Client{
		Transport: transport,
		Timeout:   cfg.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("stopped after %d redirects", maxRedirects)
			}
			if cfg.allowInsecure {
				return nil
			}
			if req.URL.Scheme != "https" {
				return errInsecureScheme
			}
			if reason := blockedHostReason(req.URL.Hostname()); reason != "" {
				return fmt.Errorf("refusing redirect to %s address", reason)
			}
			return nil
		},
	}
}

// validateURL enforces the scheme and destination rules on a source URL.
func (p *Provider) validateURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	if u.Host == "" {
		return nil, errMissingHost
	}
	if p.cfg.allowInsecure {
		return u, nil
	}
	if u.Scheme != "https" {
		return nil, errInsecureScheme
	}
	if reason := blockedHostReason(u.Hostname()); reason != "" {
		return nil, fmt.Errorf("refusing %s destination", reason)
	}
	return u, nil
}

type fetchResult struct {
	body         []byte
	etag         string
	lastModified string
	notModified  bool
}

// get performs one conditional GET under the size cap.
func (p *Provider) get(ctx context.Context, raw, etag, lastMod string, wantJSON bool) (fetchResult, error) {
	u, err := p.validateURL(raw)
	if err != nil {
		return fetchResult{}, err
	}
	reqCtx, cancel := context.WithTimeout(ctx, p.cfg.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fetchResult{}, err
	}
	req.Header.Set("User-Agent", p.cfg.UserAgent)
	if wantJSON {
		req.Header.Set("Accept", "application/rdap+json, application/json")
	} else {
		req.Header.Set("Accept", "text/plain")
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if lastMod != "" {
		req.Header.Set("If-Modified-Since", lastMod)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return fetchResult{}, err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
	}()
	if resp.StatusCode == http.StatusNotModified {
		return fetchResult{notModified: true}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return fetchResult{}, fmt.Errorf("http %d", resp.StatusCode)
	}
	if wantJSON {
		if err := requireJSON(resp.Header.Get("Content-Type")); err != nil {
			return fetchResult{}, err
		}
	}
	body, err := readCapped(resp.Body)
	if err != nil {
		return fetchResult{}, err
	}
	return fetchResult{
		body:         body,
		etag:         resp.Header.Get("ETag"),
		lastModified: resp.Header.Get("Last-Modified"),
	}, nil
}

// requireJSON rejects a record response that is not JSON.
func requireJSON(contentType string) error {
	if strings.TrimSpace(contentType) == "" {
		return errors.New("missing content-type")
	}
	media, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return fmt.Errorf("content-type: %w", err)
	}
	if !strings.Contains(media, "json") {
		return fmt.Errorf("unexpected content-type %q", media)
	}
	return nil
}

// readCapped reads at most maxResponseBytes and errors past that.
func readCapped(r io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, maxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxResponseBytes {
		return nil, errBodyTooLarge
	}
	return body, nil
}

// fetchKey performs one queued fetch. A refused token drops the job; the
// next lookup re-enqueues it.
func (p *Provider) fetchKey(ctx context.Context, key string) {
	defer p.clearInflight(key)
	kind, ok := kindOf(key)
	if !ok {
		return
	}
	if !p.bucket.take() {
		return
	}
	switch kind {
	case kindDataset:
		p.fetchDataset(ctx, key)
	case kindRecord:
		p.fetchRecord(ctx, key)
	}
}

func (p *Provider) clearInflight(key string) {
	p.mu.Lock()
	delete(p.inflight, key)
	p.mu.Unlock()
}

func (p *Provider) fetchDataset(ctx context.Context, key string) {
	d, ok := datasetByName(strings.TrimPrefix(key, datasetPrefix))
	if !ok {
		return
	}
	source := d.url(p.cfg.Sources)
	etag, lastMod := p.conditionsFor(key)
	res, err := p.get(ctx, source, etag, lastMod, d.wantJSON)
	if err != nil {
		p.fail(key, kindDataset, err)
		return
	}
	if res.notModified {
		p.touch(key, p.now())
		return
	}
	value, version, err := d.parse(res.body)
	if err != nil {
		p.fail(key, kindDataset, err)
		return
	}
	p.store(key, kindDataset, Item{
		Value:     value,
		FetchedAt: p.now(),
		SourceURL: source,
		Version:   version,
	}, res.etag, res.lastModified)
}

func (p *Provider) fetchRecord(ctx context.Context, key string) {
	source, st := p.recordURL(key)
	switch st {
	case urlPending:
		// The bootstrap is still loading; no failure to record.
		return
	case urlUnavailable:
		p.fail(key, kindRecord, errNoRDAPService)
		return
	}
	res, err := p.get(ctx, source, "", "", true)
	if err != nil {
		p.fail(key, kindRecord, err)
		return
	}
	value, err := parseRDAPDomain(res.body, source)
	if err != nil {
		p.fail(key, kindRecord, err)
		return
	}
	p.store(key, kindRecord, Item{Value: value, FetchedAt: p.now(), SourceURL: source}, "", "")
}

// tokenBucket paces outbound requests so a crawler walking detail pages
// cannot turn the server into a registry scanner.
type tokenBucket struct {
	mu        sync.Mutex
	capacity  float64
	tokens    float64
	perSecond float64
	last      time.Time
	now       func() time.Time
}

func newTokenBucket(capacity int, per time.Duration, now func() time.Time) *tokenBucket {
	if capacity < 1 {
		capacity = 1
	}
	if per <= 0 {
		per = time.Minute
	}
	if now == nil {
		now = time.Now
	}
	return &tokenBucket{
		capacity:  float64(capacity),
		tokens:    float64(capacity),
		perSecond: float64(capacity) / per.Seconds(),
		last:      now(),
		now:       now,
	}
}

// take consumes one token, returning false when the bucket is empty.
func (b *tokenBucket) take() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	if elapsed := now.Sub(b.last).Seconds(); elapsed > 0 {
		b.tokens = min(b.capacity, b.tokens+elapsed*b.perSecond)
		b.last = now
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}
