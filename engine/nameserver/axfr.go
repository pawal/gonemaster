package nameserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/transport"
)

// errCachedAXFRFailure marks a restored no-transfer entry as a failure.
var errCachedAXFRFailure = errors.New("cached AXFR failure")

// AXFR performs a zone transfer and streams RRs to the callback. A per-run cache,
// when present, is consulted first and the streamed result recorded on a miss.
func (ns Nameserver) AXFR(ctx context.Context, domain string, callback func(dns.RR) bool, class string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if class == "" {
		class = "IN"
	}
	class = strings.ToUpper(class)

	store := CacheFromContext(ctx)
	address := ns.Address.String()
	if store != nil {
		if rec, ok := store.axfrLookup(address, domain, class); ok {
			return replayAXFR(rec, callback)
		}
	}

	prof := profile.FromContext(ctx)
	if prof.NoNetwork {
		return fmt.Errorf("external AXFR query for %s attempted to %s while running with no_network", domain, ns.String())
	}
	if ns.Address.Is4() && !prof.Net.IPv4 {
		return nil
	}
	if ns.Address.Is6() && !prof.Net.IPv6 {
		return nil
	}

	var collected []dns.RR
	recording := func(rr dns.RR) bool {
		collected = append(collected, rr)
		if callback == nil {
			return true
		}
		return callback(rr)
	}

	err := ns.transferIn(ctx, domain, class, recording, prof)

	if store != nil {
		switch {
		case err != nil:
			store.axfrStore(address, domain, class, nil, true)
		case len(collected) > 0:
			store.axfrStore(address, domain, class, collected, false)
		}
	}
	return err
}

// replayAXFR delivers a cached transfer, or a failure for a no-transfer entry.
func replayAXFR(rec *axfrRecord, callback func(dns.RR) bool) error {
	if rec == nil || rec.noTransfer {
		return errCachedAXFRFailure
	}
	for _, rr := range rec.rrs {
		if callback != nil && !callback(rr) {
			return nil
		}
	}
	return nil
}

// transferIn runs the live transfer (or the test hook), without caching.
func (ns Nameserver) transferIn(ctx context.Context, domain string, class string, callback func(dns.RR) bool, prof *profile.Profile) error {
	if ns.state != nil && ns.state.axfrFunc != nil {
		return ns.state.axfrFunc(ctx, domain, callback, class)
	}

	qclass, ok := dns.StringToClass[class]
	if !ok {
		return fmt.Errorf("unknown query class %q", class)
	}

	base := transport.Client{}
	if ns.Client != nil {
		base = *ns.Client
	}
	base.SetUseTCP(true)
	base.SetRecursionDesired(false)
	base.ApplyProfileDefaults(prof)
	applyProfileSourceAddress(&base, ns.Address, prof)

	msg := new(dns.Msg)
	dnsutil.SetQuestion(msg, dnsutil.Fqdn(domain), dns.TypeAXFR)
	msg.Question[0].Header().Class = qclass
	msg.RecursionDesired = false

	client := dns.NewClient()
	if base.Timeout > 0 {
		client.Transport.ReadTimeout = base.Timeout
		client.Transport.WriteTimeout = base.Timeout
	}
	dialer, err := base.Dialer("tcp")
	if err != nil {
		return err
	}
	client.Transport.Dialer = dialer

	address := net.JoinHostPort(ns.Address.String(), "53")

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ch, err := client.TransferIn(ctx, msg, "tcp", address)
	if err != nil {
		return err
	}

	for env := range ch {
		if env.Error != nil {
			return env.Error
		}
		for _, rr := range env.Answer {
			if callback != nil && !callback(rr) {
				cancel()
				return nil
			}
		}
	}

	return nil
}
