package nameserver

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/constants"
	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/transport"
)

// Nameserver represents a DNS server endpoint.
type Nameserver struct {
	Name    dnsname.Name
	Address netip.Addr
	Client  *transport.Client
	state   *nsState
	cache   *CacheStore
}

const systemModuleName = "System"

// QueryOptions configures per-query settings that mirror Perl flags.
type QueryOptions struct {
	Class                string
	UseVC                *bool
	Recurse              *bool
	Fallback             *bool
	Retry                *int
	Retrans              *time.Duration
	Timeout              *time.Duration
	DNSSEC               *bool
	EDNSSize             *uint16
	EDNSDetails          *transport.EDNSDetails
	BlacklistingDisabled bool
}

// New creates a Nameserver from a name and IP address.
func New(name string, address string, client *transport.Client) (Nameserver, error) {
	return NewWithCache(defaultCache, name, address, client)
}

// NewWithContext creates a Nameserver using a cache store from ctx.
func NewWithContext(ctx context.Context, name string, address string, client *transport.Client) (Nameserver, error) {
	return NewWithCache(CacheFromContextOrDefault(ctx), name, address, client)
}

// NewWithCache creates a Nameserver using the supplied cache store.
func NewWithCache(cache *CacheStore, name string, address string, client *transport.Client) (Nameserver, error) {
	if cache == nil {
		cache = defaultCache
	}
	addr, err := netip.ParseAddr(address)
	if err != nil {
		return Nameserver{}, fmt.Errorf("invalid nameserver address %q: %w", address, err)
	}

	if client == nil {
		client = &transport.Client{}
	}

	nameKey := strings.ToLower(name)
	if name == "" {
		nameKey = "$$$noname"
	}
	nameObj := dnsname.New(nameKey)
	nameKey = strings.ToLower(nameObj.String())

	addrKey := addr.String()
	if cached := cache.cachedNameserver(nameKey, addrKey); cached != nil {
		return *cached, nil
	}

	state := &nsState{
		cache:           cache.cacheForAddress(addrKey),
		errorCache:      cache.errorCacheForAddress(addrKey),
		concurrencyCap:  cache.concurrencyCapForAddress(addrKey),
		fakeDelegations: map[string]delegation{},
		fakeDS:          map[string][]dns.RR{},
	}

	ns := &Nameserver{
		Name:    nameObj,
		Address: addr,
		Client:  client,
		state:   state,
		cache:   cache,
	}
	cache.storeNameserver(nameKey, addrKey, ns)
	return *ns, nil
}

// Query sends a DNS query using IN class.
func (ns Nameserver) Query(ctx context.Context, qname string, qtype string) (packet.Packet, error) {
	return ns.QueryWithOptions(ctx, qname, qtype, nil)
}

// QueryWithClass sends a DNS query using the given class.
func (ns Nameserver) QueryWithClass(ctx context.Context, qname string, qtype string, qclass string) (packet.Packet, error) {
	opts := &QueryOptions{Class: qclass}
	return ns.QueryWithOptions(ctx, qname, qtype, opts)
}

// QueryWithOptions sends a DNS query using per-query options.
func (ns Nameserver) QueryWithOptions(ctx context.Context, qname string, qtype string, opts *QueryOptions) (packet.Packet, error) {
	if ns.Client == nil {
		return packet.Packet{}, fmt.Errorf("missing transport client")
	}

	if qtype == "" {
		qtype = "A"
	}
	qtype = strings.ToUpper(qtype)

	qclass := "IN"
	if opts != nil && opts.Class != "" {
		qclass = opts.Class
	}
	qclass = strings.ToUpper(qclass)

	prof := profile.FromContext(ctx)
	if ns.Address.Is4() && !prof.Net.IPv4 {
		return packet.Packet{}, nil
	}
	if ns.Address.Is6() && !prof.Net.IPv6 {
		return packet.Packet{}, nil
	}

	if resp, ok := ns.fakeDSResponse(qname, qtype, qclass, opts); ok {
		return resp, nil
	}
	if resp, ok := ns.fakeDelegationResponse(qname, qtype, qclass, opts); ok {
		return resp, nil
	}

	cacheKey, ednsSize, _, err := buildCacheKey(qname, qtype, qclass, opts)
	if err != nil {
		return packet.Packet{}, err
	}
	if ns.state != nil {
		if cached, ok := ns.state.cache.get(cacheKey); ok {
			if cached == nil {
				return packet.Packet{}, nil
			}
			return *cached, nil
		}
	}

	if prof.NoNetwork {
		return packet.Packet{}, fmt.Errorf("external query for %s %s attempted to %s while running with no_network", qname, qtype, ns.String())
	}

	usevc := resolveUseVC(opts)
	fastFailThreshold := resolveFastFailTimeoutCount(prof)
	nameserverConcurrencyLimit := resolveNameserverConcurrencyLimit(prof)
	pacingPolicy := resolveRateLimitPacingPolicyConfig(prof)
	if ns.state != nil {
		ns.state.rateLimitPacing.setPolicy(pacingPolicy)
	}
	if ttl := resolveReachabilityTTL(prof, opts); ttl > 0 {
		if skip, remaining := globalReachability.shouldSkip(ns.Address.String()); skip {
			logSystem(ctx, "REACHABILITY_CACHE_SKIP", map[string]any{
				"ip":          ns.Address.String(),
				"protocol":    errorCacheProtocol(usevc),
				"ttl_seconds": int(remaining.Seconds()),
				"query_name":  qname,
				"query_type":  qtype,
				"query_class": qclass,
			})
			return packet.Packet{}, nil
		}
	}
	if errorCacheTTL := resolveErrorCacheTTL(prof, opts); errorCacheTTL > 0 && ns.state != nil && ns.state.errorCache != nil {
		if skip, remaining := ns.state.errorCache.shouldSkip(errorCacheKey(usevc)); skip {
			logSystem(ctx, "ERROR_CACHE_SKIP", map[string]any{
				"ip":          ns.Address.String(),
				"protocol":    errorCacheProtocol(usevc),
				"ttl_seconds": int(remaining.Seconds()),
				"query_name":  qname,
				"query_type":  qtype,
				"query_class": qclass,
			})
			return packet.Packet{}, nil
		}
	}
	now := time.Now()
	if constants.BlacklistingEnabled && ns.state != nil && ns.state.blacklist.isBlocked(usevc, now) {
		return packet.Packet{}, nil
	}
	if ns.state != nil && ns.state.fastFail.shouldSkip(usevc, fastFailThreshold) {
		logSystem(ctx, "FAST_FAIL_SKIP", map[string]any{
			"ip":          ns.Address.String(),
			"protocol":    errorCacheProtocol(usevc),
			"query_name":  qname,
			"query_type":  qtype,
			"query_class": qclass,
		})
		return packet.Packet{}, nil
	}
	pacingDecision, err := ns.applyRateLimitPacing(ctx, usevc, prof, opts, pacingPolicy)
	if err != nil {
		return packet.Packet{}, err
	}
	switch pacingDecision.Action {
	case "skip":
		logSystem(ctx, "RATE_LIMIT_PACING_SKIP", rateLimitPacingLogArgs(ns, usevc, qname, qtype, qclass, pacingDecision, "delay_exceeds_budget"))
		return packet.Packet{}, nil
	case "delay":
		logSystem(ctx, "RATE_LIMIT_PACING_DELAY", rateLimitPacingLogArgs(ns, usevc, qname, qtype, qclass, pacingDecision, "paced_wait"))
	}

	var inflight *inflightQuery
	if ns.state != nil && ns.state.cache != nil {
		if existing, wait := ns.state.cache.waitOrRegister(cacheKey); wait {
			if ctx == nil {
				<-existing.done
				if existing.resp == nil {
					return packet.Packet{}, existing.err
				}
				return *existing.resp, existing.err
			}
			select {
			case <-existing.done:
				if existing.resp == nil {
					return packet.Packet{}, existing.err
				}
				return *existing.resp, existing.err
			case <-ctx.Done():
				return packet.Packet{}, ctx.Err()
			}
		} else {
			inflight = existing
		}
	}
	if ns.state != nil && ns.state.concurrencyCap != nil && nameserverConcurrencyLimit > 0 {
		if err := ns.state.concurrencyCap.acquire(ctx, nameserverConcurrencyLimit); err != nil {
			if inflight != nil {
				ns.state.cache.finish(cacheKey, nil, err)
			}
			return packet.Packet{}, err
		}
		defer ns.state.concurrencyCap.release()
	}

	queryOpts, trackAdaptive := ns.applyAdaptiveTimeoutOptions(prof, opts, usevc)
	resp, err := ns.queryNetwork(ctx, qname, qtype, qclass, queryOpts)
	if ns.state != nil && pacingPolicy.Enabled {
		signal := classifyRateLimitSignal(resp, err)
		observation := ns.state.rateLimitPacing.observeResultWithObservation(usevc, signal, time.Now())
		if observation.Detected {
			args := rateLimitPacingLogArgs(ns, usevc, qname, qtype, qclass, rateLimitPacingDecision{
				Action:    "observe",
				Remaining: 0,
				Budget:    0,
				Snapshot:  observation.Snapshot,
			}, observation.Reason)
			args["signal"] = rateLimitSignalString(observation.Signal)
			logSystem(ctx, "RATE_LIMIT_PACING_DETECTED", args)
		}
	}
	if ns.state != nil {
		ns.state.fastFail.observeResult(usevc, isTimeoutPatternError(err), fastFailThreshold)
	}
	if err == nil && ns.state != nil {
		ns.state.blacklist.observeSuccess(usevc)
	}
	if trackAdaptive && ns.state != nil {
		ns.state.adaptiveTimeout.observeResult(usevc, isTimeoutPatternError(err))
	}

	blacklistingDisabled := opts != nil && opts.BlacklistingDisabled
	if err != nil && (ctx == nil || ctx.Err() == nil) && qtype == "SOA" && ednsSize == 0 && !blacklistingDisabled {
		if ns.state != nil {
			baseTTL := resolveQueryTimeout(prof, opts)
			ns.state.blacklist.observeFailure(usevc, isTimeoutPatternError(err), baseTTL, now)
		}
	}
	if err != nil && (ctx == nil || ctx.Err() == nil) && ns.state != nil && ns.state.errorCache != nil {
		if errorCacheTTL := resolveErrorCacheTTL(prof, opts); errorCacheTTL > 0 {
			ns.state.errorCache.set(errorCacheKey(usevc), errorCacheTTL)
		}
	}
	if err != nil && (ctx == nil || ctx.Err() == nil) && isHardNetworkError(err) {
		if ttl := resolveReachabilityTTL(prof, opts); ttl > 0 {
			globalReachability.mark(ns.Address.String(), ttl)
		}
	}

	if ns.state != nil {
		var infResp *packet.Packet
		if resp.Msg != nil {
			copyResp := resp
			ns.state.cache.set(cacheKey, &copyResp)
			infResp = &copyResp
		} else if err == nil {
			ns.state.cache.set(cacheKey, nil)
		}
		if inflight != nil {
			ns.state.cache.finish(cacheKey, infResp, err)
		}
	}
	return resp, err
}

func errorCacheKey(usevc bool) string {
	if usevc {
		return "tcp"
	}
	return "udp"
}

func errorCacheProtocol(usevc bool) string {
	if usevc {
		return "tcp"
	}
	return "udp"
}

func resolveErrorCacheTTL(prof *profile.Profile, opts *QueryOptions) time.Duration {
	if prof == nil {
		prof = profile.Effective()
	}
	if prof == nil {
		return 0
	}

	return resolveTTLWithBudget(prof.Resolver.Defaults.ErrorCacheTTL, prof, opts)
}

func resolveReachabilityTTL(prof *profile.Profile, opts *QueryOptions) time.Duration {
	if prof == nil {
		prof = profile.Effective()
	}
	if prof == nil {
		return 0
	}
	baseSeconds := prof.Resolver.Defaults.NegativeCacheTTL
	if baseSeconds <= 0 {
		baseSeconds = prof.Resolver.Defaults.ErrorCacheTTL
	}
	return resolveTTLWithBudget(baseSeconds, prof, opts)
}

func resolveTTLWithBudget(baseSeconds int, prof *profile.Profile, opts *QueryOptions) time.Duration {
	if baseSeconds <= 0 {
		return 0
	}
	baseTTL := time.Duration(baseSeconds) * time.Second
	timeout := time.Duration(prof.Resolver.Defaults.Timeout) * time.Second
	retries := prof.Resolver.Defaults.Retry
	retrans := time.Duration(prof.Resolver.Defaults.Retrans) * time.Second

	if opts != nil {
		if opts.Timeout != nil {
			timeout = *opts.Timeout
		}
		if opts.Retry != nil {
			retries = *opts.Retry
		}
		if opts.Retrans != nil {
			retrans = *opts.Retrans
		}
	}

	if retries < 0 {
		retries = 0
	}

	perAttempt := timeout
	if perAttempt <= 0 && retrans > 0 {
		perAttempt = retrans
	}

	if perAttempt <= 0 {
		return baseTTL
	}

	attempts := retries + 1
	if attempts < 1 {
		attempts = 1
	}
	budget := perAttempt * time.Duration(attempts)
	if budget <= 0 {
		return baseTTL
	}
	if budget < baseTTL {
		return budget
	}
	return baseTTL
}

func resolveQueryTimeout(prof *profile.Profile, opts *QueryOptions) time.Duration {
	if prof == nil {
		prof = profile.Effective()
	}
	if prof == nil {
		return 0
	}
	timeout := time.Duration(prof.Resolver.Defaults.Timeout) * time.Second
	if opts != nil && opts.Timeout != nil {
		timeout = *opts.Timeout
	}
	return timeout
}

func resolveFastFailTimeoutCount(prof *profile.Profile) int {
	if prof == nil {
		prof = profile.Effective()
	}
	if prof == nil {
		return 0
	}
	if prof.Resolver.Defaults.FastFailTimeoutCount < 0 {
		return 0
	}
	return prof.Resolver.Defaults.FastFailTimeoutCount
}

func resolveNameserverConcurrencyLimit(prof *profile.Profile) int {
	if prof == nil {
		prof = profile.Effective()
	}
	if prof == nil {
		return 0
	}
	if prof.Resolver.Defaults.NameserverConcurrency < 0 {
		return 0
	}
	return prof.Resolver.Defaults.NameserverConcurrency
}

func (ns Nameserver) applyRateLimitPacing(ctx context.Context, usevc bool, prof *profile.Profile, opts *QueryOptions, policy rateLimitPacingPolicyConfig) (rateLimitPacingDecision, error) {
	if ns.state == nil || !policy.Enabled {
		return rateLimitPacingDecision{}, nil
	}
	shouldPace, remaining := ns.state.rateLimitPacing.shouldPace(usevc, time.Now())
	if !shouldPace || remaining <= 0 {
		return rateLimitPacingDecision{}, nil
	}

	budget := resolveQueryTimeout(prof, opts)
	if budget > 0 && remaining > budget {
		// Skip this nameserver so callers can fall back to alternatives instead of stalling.
		ns.state.rateLimitPacing.recordPacingSkip(usevc)
		return rateLimitPacingDecision{
			Action:    "skip",
			Remaining: remaining,
			Budget:    budget,
			Snapshot:  ns.state.rateLimitPacing.snapshot(usevc),
		}, nil
	}
	ns.state.rateLimitPacing.recordPacingDelay(usevc)
	decision := rateLimitPacingDecision{
		Action:    "delay",
		Remaining: remaining,
		Budget:    budget,
		Snapshot:  ns.state.rateLimitPacing.snapshot(usevc),
	}

	timer := time.NewTimer(remaining)
	defer timer.Stop()
	if ctx == nil {
		<-timer.C
		return decision, nil
	}
	select {
	case <-timer.C:
		return decision, nil
	case <-ctx.Done():
		return decision, ctx.Err()
	}
}

func rateLimitPacingLogArgs(ns Nameserver, usevc bool, qname string, qtype string, qclass string, decision rateLimitPacingDecision, reason string) map[string]any {
	args := map[string]any{
		"ip":                   ns.Address.String(),
		"protocol":             errorCacheProtocol(usevc),
		"query_name":           qname,
		"query_type":           qtype,
		"query_class":          qclass,
		"reason":               reason,
		"delay_ms":             decision.Remaining.Milliseconds(),
		"budget_ms":            decision.Budget.Milliseconds(),
		"backoff_ms":           decision.Snapshot.BackoffDelay.Milliseconds(),
		"adaptive_ms":          decision.Snapshot.AdaptiveDelay.Milliseconds(),
		"estimated_qps":        decision.Snapshot.EstimatedQPS,
		"detection_timeout":    decision.Snapshot.DetectionTimeoutBurst,
		"detection_servfail":   decision.Snapshot.DetectionServfail,
		"detection_conn_error": decision.Snapshot.DetectionConnError,
		"pacing_delays":        decision.Snapshot.PacingDelayCount,
		"pacing_skips":         decision.Snapshot.PacingSkipCount,
	}
	return args
}

func cloneQueryOptions(opts *QueryOptions) *QueryOptions {
	if opts == nil {
		return &QueryOptions{}
	}
	copyOpts := *opts
	return &copyOpts
}

func (ns Nameserver) applyAdaptiveTimeoutOptions(prof *profile.Profile, opts *QueryOptions, usevc bool) (*QueryOptions, bool) {
	if ns.state == nil || prof == nil || !prof.Resolver.Defaults.AdaptiveTimeout {
		return opts, false
	}
	if opts != nil && opts.Timeout != nil {
		// Keep explicit timeout overrides untouched.
		return opts, false
	}

	baseTimeout := resolveQueryTimeout(prof, opts)
	if baseTimeout <= 0 {
		return opts, false
	}
	reduced := ns.state.adaptiveTimeout.timeoutFor(baseTimeout, usevc)
	if reduced <= 0 || reduced >= baseTimeout {
		return opts, true
	}

	queryOpts := cloneQueryOptions(opts)
	queryOpts.Timeout = &reduced
	return queryOpts, true
}

func (ns Nameserver) queryNetwork(ctx context.Context, qname string, qtype string, qclass string, opts *QueryOptions) (packet.Packet, error) {
	if ns.state != nil && ns.state.queryFunc != nil {
		return ns.state.queryFunc(ctx, qname, qtype, qclass, opts)
	}

	client, err := ns.clientForOptions(ctx, opts)
	if err != nil {
		return packet.Packet{}, err
	}

	qtypeID, ok := dns.StringToType[qtype]
	if !ok {
		return packet.Packet{}, fmt.Errorf("unknown query type %q", qtype)
	}

	qclassID, ok := dns.StringToClass[qclass]
	if !ok {
		return packet.Packet{}, fmt.Errorf("unknown query class %q", qclass)
	}

	msg := transport.BuildQueryWithClass(qname, qtypeID, qclassID)
	server := ns.Address.String()

	// Emit EXTERNAL_QUERY log entry
	logSystem(ctx, "EXTERNAL_QUERY", map[string]any{
		"name":  qname,
		"type":  qtype,
		"ip":    ns.Address.String(),
		"flags": fmt.Sprintf(`{"class":%q}`, qclass),
	})

	resp, err := client.Exchange(ctx, server, msg)

	args := map[string]any{
		"name":  qname,
		"type":  qtype,
		"ip":    ns.Address.String(),
		"flags": fmt.Sprintf(`{"class":%q}`, qclass),
	}
	if resp.Msg != nil {
		args["rcode"] = dns.RcodeToString[resp.Msg.Rcode]
		args["answers"] = len(resp.Msg.Answer)
		args["authority"] = len(resp.Msg.Ns)
		args["additional"] = len(resp.Msg.Extra)
		args["aa"] = resp.Msg.Authoritative
		args["tc"] = resp.Msg.Truncated
		args["rd"] = resp.Msg.RecursionDesired
		args["ra"] = resp.Msg.RecursionAvailable
		args["ad"] = resp.Msg.AuthenticatedData
		args["cd"] = resp.Msg.CheckingDisabled
	}
	if err != nil {
		args["exception"] = err.Error()
	}
	if resp.Msg == nil && err == nil {
		logSystem(ctx, "EMPTY_RETURN", args)
	} else {
		logSystem(ctx, "EXTERNAL_RESPONSE", args)
	}

	return resp, err
}

func logSystem(ctx context.Context, tag string, args map[string]any) {
	log := logger.FromContext(ctx)
	if log == nil {
		return
	}
	_, _ = log.Add(tag, args, systemModuleName, "")
}

func (ns Nameserver) clientForOptions(ctx context.Context, opts *QueryOptions) (*transport.Client, error) {
	base := transport.Client{}
	if ns.Client != nil {
		base = *ns.Client
	}

	base.SetUseTCP(resolveUseVC(opts))
	base.SetRecursionDesired(resolveRecurse(opts))

	if opts != nil {
		if opts.Fallback != nil {
			base.SetFallback(*opts.Fallback)
		}
		if opts.Retry != nil {
			base.SetRetries(*opts.Retry)
		}
		if opts.Retrans != nil {
			base.SetRetrans(*opts.Retrans)
		}
		if opts.Timeout != nil {
			base.SetTimeout(*opts.Timeout)
		}
		if opts.DNSSEC != nil {
			base.DNSSEC = *opts.DNSSEC
		}
	}

	dnssec := base.DNSSEC
	if opts != nil && opts.EDNSDetails != nil && opts.EDNSDetails.Do != nil {
		dnssec = *opts.EDNSDetails.Do
		base.DNSSEC = dnssec
	}

	if opts != nil && opts.EDNSDetails != nil {
		ednsSize := resolveEDNSSize(opts, dnssec)
		base.SetEDNSSize(ednsSize)
		base.EDNSDetails = opts.EDNSDetails
	} else if opts != nil && opts.EDNSSize != nil {
		base.SetEDNSSize(*opts.EDNSSize)
	} else if dnssec {
		base.SetEDNSSize(constants.EDNSUDPPayloadDNSSECDefault)
	}

	base.ApplyProfileDefaults(profile.FromContext(ctx))
	return &base, nil
}

func resolveEDNSSize(opts *QueryOptions, dnssec bool) uint16 {
	if opts != nil && opts.EDNSDetails != nil {
		if opts.EDNSDetails.Size != nil {
			return *opts.EDNSDetails.Size
		}
		if opts.EDNSSize != nil {
			return *opts.EDNSSize
		}
		if dnssec {
			return constants.EDNSUDPPayloadDNSSECDefault
		}
		return constants.EDNSUDPPayloadDefault
	}
	if opts != nil && opts.EDNSSize != nil {
		return *opts.EDNSSize
	}
	if dnssec {
		return constants.EDNSUDPPayloadDNSSECDefault
	}
	return 0
}
