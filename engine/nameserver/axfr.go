package nameserver

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/miekg/dns"

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
	base.ApplyProfileDefaults(profile.FromContext(ctx))

	msg := new(dns.Msg)
	msg.SetAxfr(dns.Fqdn(domain))
	msg.Question[0].Qclass = qclass
	msg.RecursionDesired = false

	transfer := &dns.Transfer{
		DialTimeout:  base.Timeout,
		ReadTimeout:  base.Timeout,
		WriteTimeout: base.Timeout,
	}

	dialer, err := base.Dialer("tcp")
	if err != nil {
		return err
	}

	address := net.JoinHostPort(ns.Address.String(), "53")
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return err
	}
	transfer.Conn = &dns.Conn{Conn: conn}

	ch, err := transfer.In(msg, address)
	if err != nil {
		return err
	}

	stop := false
	for env := range ch {
		if env.Error != nil {
			if stop {
				return nil
			}
			return env.Error
		}
		for _, rr := range env.RR {
			if callback != nil && !stop {
				if !callback(rr) {
					stop = true
					transfer.Close()
				}
			}
		}
	}

	return nil
}
