package nameserver

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"time"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/constants"
	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/logargs"
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
	log     *logger.Logger
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
	return newWithCache(nil, defaultCache, name, address, client)
}

// NewWithContext creates a Nameserver using a cache store from ctx.
func NewWithContext(ctx context.Context, name string, address string, client *transport.Client) (Nameserver, error) {
	return newWithCache(ctx, CacheFromContextOrDefault(ctx), name, address, client)
}

// NewWithCache creates a Nameserver using the supplied cache store.
func NewWithCache(cache *CacheStore, name string, address string, client *transport.Client) (Nameserver, error) {
	return newWithCache(nil, cache, name, address, client)
}

func newWithCache(ctx context.Context, cache *CacheStore, name string, address string, client *transport.Client) (Nameserver, error) {
	if cache == nil {
		cache = defaultCache
	}
	runLog := logger.FromContext(ctx)
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
	queryCache, cacheCreated := cache.cacheForAddressWithStatus(addrKey)
	if cacheCreated {
		logSystemWithLogger(runLog, "CACHE_CREATED", map[string]any{"address": addrKey})
	} else {
		logSystemWithLogger(runLog, "CACHE_FETCHED", map[string]any{"address": addrKey})
	}

	state := &nsState{
		cache:           queryCache,
		errorCache:      cache.errorCacheForAddress(addrKey),
		concurrencyCap:  cache.concurrencyCapForAddress(addrKey),
		fakeDelegations: map[string]delegation{},
		fakeDS:          map[string][]dns.RR{},
		blacklisted:     map[bool]bool{},
	}

	ns := &Nameserver{
		Name:    nameObj,
		Address: addr,
		Client:  client,
		state:   state,
		cache:   cache,
		log:     runLog,
	}
	cache.storeNameserver(nameKey, addrKey, ns)
	nsName := nameObj.String()
	logSystemWithLogger(runLog, "NS_CREATED", map[string]any{
		"ns":      nsName,
		"address": addrKey,
	})
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
	runLog := loggerFromContextOrFallback(ctx, ns.log)

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
		logSystemWithLogger(runLog, "IPV4_BLOCKED", logargs.NS(ns.NameString(), ns.AddressString()))
		return packet.Packet{}, nil
	}
	if ns.Address.Is6() && !prof.Net.IPv6 {
		logSystemWithLogger(runLog, "IPV6_BLOCKED", logargs.NS(ns.NameString(), ns.AddressString()))
		return packet.Packet{}, nil
	}
	queryArgs := map[string]any{
		"query_name":  qname,
		"query_type":  qtype,
		"query_class": qclass,
		"flags":       queryFlags(qclass, opts),
		"address":     ns.Address.String(),
	}
	logargs.SetNS(queryArgs, ns.NameString(), ns.AddressString())
	logSystemWithLogger(runLog, "QUERY", queryArgs)

	if resp, ok := ns.fakeDSResponse(qname, qtype, qclass, opts, runLog); ok {
		return resp, nil
	}
	if resp, ok := ns.fakeDelegationResponse(qname, qtype, qclass, opts, runLog); ok {
		return resp, nil
	}

	cacheKey, ednsSize, _, err := buildCacheKey(qname, qtype, qclass, opts)
	if err != nil {
		return packet.Packet{}, err
	}
	if ns.state != nil {
		if cached, ok := ns.state.cache.get(cacheKey); ok {
			if cached == nil {
				logCachedReturnWithLogger(runLog, packet.Packet{})
				return packet.Packet{}, nil
			}
			copyCached := *cached
			copyCached.Log = runLog
			logCachedReturnWithLogger(runLog, copyCached)
			return copyCached, nil
		}
	}

	if prof.NoNetwork {
		return packet.Packet{}, fmt.Errorf("external query for %s %s attempted to %s while running with no_network", qname, qtype, ns.String())
	}

	usevc := resolveUseVC(opts)
	if ttl := resolveReachabilityTTL(prof, opts); ttl > 0 {
		if skip, remaining := globalReachability.shouldSkip(ns.Address.String()); skip {
			skipArgs := map[string]any{
				"address":     ns.Address.String(),
				"protocol":    errorCacheProtocol(usevc),
				"ttl_seconds": int(remaining.Seconds()),
				"query_name":  qname,
				"query_type":  qtype,
				"query_class": qclass,
			}
			logargs.SetNS(skipArgs, ns.NameString(), ns.AddressString())
			logSystem(ctx, "REACHABILITY_CACHE_SKIP", skipArgs)
			return packet.Packet{}, nil
		}
	}
	if errorCacheTTL := resolveErrorCacheTTL(prof, opts); errorCacheTTL > 0 && ns.state != nil && ns.state.errorCache != nil {
		if skip, remaining := ns.state.errorCache.shouldSkip(errorCacheKey(usevc)); skip {
			skipArgs := map[string]any{
				"address":     ns.Address.String(),
				"protocol":    errorCacheProtocol(usevc),
				"ttl_seconds": int(remaining.Seconds()),
				"query_name":  qname,
				"query_type":  qtype,
				"query_class": qclass,
			}
			logargs.SetNS(skipArgs, ns.NameString(), ns.AddressString())
			logSystem(ctx, "ERROR_CACHE_SKIP", skipArgs)
			return packet.Packet{}, nil
		}
	}
	if constants.BlacklistingEnabled && ns.state != nil && ns.state.blacklisted[usevc] {
		blArgs := map[string]any{
			"query_name":  qname,
			"query_type":  qtype,
			"query_class": qclass,
		}
		logargs.SetNS(blArgs, ns.NameString(), ns.AddressString())
		logSystemWithLogger(runLog, "IS_BLACKLISTED", blArgs)
		return packet.Packet{}, nil
	}
	fastFailThreshold := resolveFastFailTimeoutCount(prof)
	if ns.state != nil && ns.state.fastFail.shouldSkip(usevc, fastFailThreshold) {
		skipArgs := map[string]any{
			"query_name":  qname,
			"query_type":  qtype,
			"query_class": qclass,
			"protocol":    errorCacheProtocol(usevc),
			"address":     ns.Address.String(),
		}
		logargs.SetNS(skipArgs, ns.NameString(), ns.AddressString())
		logSystemWithLogger(runLog, "FAST_FAIL_SKIP", skipArgs)
		return packet.Packet{}, nil
	}
	nameserverConcurrencyLimit := resolveNameserverConcurrencyLimit(prof)

	var inflight *inflightQuery
	if ns.state != nil && ns.state.cache != nil {
		if existing, wait := ns.state.cache.waitOrRegister(cacheKey); wait {
			if ctx == nil {
				<-existing.done
				if existing.resp == nil {
					logCachedReturnWithLogger(runLog, packet.Packet{})
					return packet.Packet{}, existing.err
				}
				copyResp := *existing.resp
				copyResp.Log = runLog
				logCachedReturnWithLogger(runLog, copyResp)
				return copyResp, existing.err
			}
			select {
			case <-existing.done:
				if existing.resp == nil {
					logCachedReturnWithLogger(runLog, packet.Packet{})
					return packet.Packet{}, existing.err
				}
				copyResp := *existing.resp
				copyResp.Log = runLog
				logCachedReturnWithLogger(runLog, copyResp)
				return copyResp, existing.err
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

	resp, err := ns.queryNetwork(ctx, qname, qtype, qclass, opts)
	if ns.state != nil {
		ns.state.fastFail.observeResult(usevc, isTimeoutPatternError(err), fastFailThreshold)
	}

	blacklistingDisabled := opts != nil && opts.BlacklistingDisabled
	if err != nil && (ctx == nil || ctx.Err() == nil) && qtype == "SOA" && ednsSize == 0 && !blacklistingDisabled {
		if ns.state != nil {
			ns.state.blacklisted[usevc] = true
			blArgs := map[string]any{
				"query_name":  qname,
				"query_type":  qtype,
				"query_class": qclass,
			}
			logargs.SetNS(blArgs, ns.NameString(), ns.AddressString())
			logSystemWithLogger(runLog, "BLACKLISTING", blArgs)
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

	// Log oversized packets before releasing inflight waiters — both paths share
	// the same *dns.Msg pointer, so Len() must not race with callers of the
	// released goroutines (e.g. KeyTag() writing the cached keytag field).
	if resp.Msg != nil && resp.Msg.Len() > 4096 {
		bigArgs := map[string]any{
			"size":    resp.Msg.Len(),
			"command": fmt.Sprintf("dig @%s %s %s", ns.Address.String(), qname, qtype),
		}
		logSystemWithLogger(runLog, "PACKET_BIG", bigArgs)
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
	logCachedReturnWithLogger(runLog, resp)
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

func (ns Nameserver) queryNetwork(ctx context.Context, qname string, qtype string, qclass string, opts *QueryOptions) (packet.Packet, error) {
	if ns.state != nil && ns.state.queryFunc != nil {
		resp, err := ns.state.queryFunc(ctx, qname, qtype, qclass, opts)
		resp.Log = loggerFromContextOrFallback(ctx, ns.log)
		return resp, err
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
	queryArgs := map[string]any{
		"query_name":  qname,
		"query_type":  qtype,
		"query_class": qclass,
		"address":     ns.Address.String(),
		"flags":       fmt.Sprintf(`{"class":%q}`, qclass),
	}
	logargs.SetNS(queryArgs, ns.NameString(), ns.AddressString())
	logSystem(ctx, "EXTERNAL_QUERY", queryArgs)

	resp, err := client.Exchange(ctx, server, msg)
	resp.Log = loggerFromContextOrFallback(ctx, ns.log)
	if ns.cache != nil && resp.QueryTime > 0 {
		ns.cache.RecordQueryTime(ns.NameString()+"/"+ns.AddressString(), resp.QueryTime)
	}

	args := map[string]any{
		"query_name":  qname,
		"query_type":  qtype,
		"query_class": qclass,
		"address":     ns.Address.String(),
		"flags":       fmt.Sprintf(`{"class":%q}`, qclass),
	}
	logargs.SetNS(args, ns.NameString(), ns.AddressString())
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
	logSystemWithLogger(loggerFromContextOrFallback(ctx, nil), tag, args)
}

func logSystemWithLogger(log *logger.Logger, tag string, args map[string]any) {
	if log == nil {
		return
	}
	if args == nil {
		args = map[string]any{}
	}
	_, _ = log.Add(tag, args, systemModuleName, "")
}

func logCachedReturnWithLogger(log *logger.Logger, resp packet.Packet) {
	if log == nil {
		return
	}
	args := map[string]any{"packet": "undef"}
	if resp.Msg != nil {
		args["packet"] = packetStringForLog(resp)
	}
	_, _ = log.Add("CACHED_RETURN", args, systemModuleName, "")
}

func queryFlags(qclass string, opts *QueryOptions) map[string]any {
	flags := map[string]any{
		"class":   qclass,
		"dnssec":  resolveDNSSEC(opts),
		"usevc":   resolveUseVC(opts),
		"recurse": resolveRecurse(opts),
	}
	if opts != nil {
		if opts.Fallback != nil {
			flags["fallback"] = *opts.Fallback
		}
		if opts.Retry != nil {
			flags["retry"] = *opts.Retry
		}
		if opts.Retrans != nil {
			flags["retrans"] = int(opts.Retrans.Seconds())
		}
		if opts.Timeout != nil {
			flags["timeout"] = int(opts.Timeout.Seconds())
		}
	}
	flags["edns_size"] = resolveEDNSSize(opts, flags["dnssec"].(bool))
	return flags
}

func loggerFromContextOrFallback(ctx context.Context, fallback *logger.Logger) *logger.Logger {
	if log := logger.FromContext(ctx); log != nil {
		return log
	}
	return fallback
}

func packetStringForLog(resp packet.Packet) string {
	if resp.Msg == nil {
		return "undef"
	}
	clone := resp.Msg.Copy()
	clone.ID = 0
	return clone.String()
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

	prof := profile.FromContext(ctx)
	base.ApplyProfileDefaults(prof)
	applyProfileSourceAddress(&base, ns.Address, prof)
	return &base, nil
}

func applyProfileSourceAddress(client *transport.Client, target netip.Addr, prof *profile.Profile) {
	if client == nil || client.SourceIP != "" {
		return
	}
	if !target.IsValid() {
		return
	}
	if prof == nil {
		prof = profile.Effective()
	}
	if target.Is4() && prof.Resolver.Source4 != "" {
		client.SourceIP = prof.Resolver.Source4
		return
	}
	if target.Is6() && prof.Resolver.Source6 != "" {
		client.SourceIP = prof.Resolver.Source6
	}
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
