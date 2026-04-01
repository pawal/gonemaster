package nameserver

import (
	"context"
	"fmt"
	"net"
	"strings"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/transport"
)

// AXFR performs a zone transfer and streams RRs to the callback.
func (ns Nameserver) AXFR(ctx context.Context, domain string, callback func(dns.RR) bool, class string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if class == "" {
		class = "IN"
	}
	class = strings.ToUpper(class)

	prof := profile.FromContext(ctx)
	if prof.NoNetwork {
		return fmt.Errorf("External AXFR query for %s attempted to %s while running with no_network", domain, ns.String())
	}
	if ns.Address.Is4() && !prof.Net.IPv4 {
		return nil
	}
	if ns.Address.Is6() && !prof.Net.IPv6 {
		return nil
	}

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
