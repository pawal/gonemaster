package transport

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/constants"
	"codeberg.org/pawal/gonemaster/engine/ednsopt"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/querytrace"
)

const defaultTimeout = 5 * time.Second

// Top Z bits the DNS library carries as named flags rather than in SetZ.
const (
	ednsZCompactAnswers = 0x4000
	ednsZDelegation     = 0x2000
)

// Client performs DNS exchanges; Exchange treats it as read-only and is safe to share.
type Client struct {
	// Timeout is the per-attempt timeout.
	Timeout time.Duration
	// Retries is the number of retries after the initial attempt.
	Retries int
	// Retrans is the retransmission interval between retries.
	Retrans time.Duration
	// UseTCP forces queries over TCP.
	UseTCP bool
	// Fallback retries truncated UDP responses over TCP when true.
	Fallback bool
	// EDNSSize sets the advertised EDNS UDP payload size.
	EDNSSize uint16
	// DNSSEC enables EDNS DO handling.
	DNSSEC bool
	// EDNSDetails provides explicit EDNS header and option overrides.
	EDNSDetails *EDNSDetails
	// RecursionDesired controls the RD bit.
	RecursionDesired bool
	// CheckingDisabled sets the CD bit, suppressing DNSSEC validation at a
	// validating resolver. It has no effect on authoritative answers.
	CheckingDisabled bool
	// SourceIP selects the local source IP address.
	SourceIP string
	// SourcePort selects the local source port.
	SourcePort int

	timeoutSpecified  bool
	retriesSpecified  bool
	retransSpecified  bool
	fallbackSpecified bool
	ednsSizeSpecified bool
}

// EDNSDetails captures explicit EDNS settings and overrides.
type EDNSDetails struct {
	// Do overrides the EDNS DO bit when non-nil.
	Do *bool
	// Size overrides the EDNS UDP payload size when non-nil.
	Size *uint16
	// Version overrides the EDNS version when non-nil.
	Version *uint8
	// Z carries the EDNS Z flags (low 15 bits); 0x4000 is CO and 0x2000 is DE.
	// When set, EDNS is encoded as an explicit OPT RR so Z reaches the wire.
	Z *uint16
	// Rcode overrides the EDNS extended response code when non-nil.
	Rcode *uint8
	// Data appends EDNS0 sub-options such as NSID or COOKIE.
	Data []dns.EDNS0
}

// BuildQuery constructs a query message with common defaults.
func BuildQuery(name string, qtype uint16) *dns.Msg {
	return BuildQueryWithClass(name, qtype, dns.ClassINET)
}

// BuildQueryWithClass constructs a query message for the given class.
func BuildQueryWithClass(name string, qtype, qclass uint16) *dns.Msg {
	msg := new(dns.Msg)
	newFn, ok := dns.TypeToRR[qtype]
	if !ok {
		return msg
	}
	rr := newFn()
	rr.Header().Name = dnsutil.Fqdn(name)
	rr.Header().Class = qclass
	msg.Question = []dns.RR{rr}
	msg.RecursionDesired = false
	return msg
}

// SetUseTCP sets whether queries use TCP.
func (c *Client) SetUseTCP(value bool) {
	c.UseTCP = value
}

// SetTimeout marks Timeout as explicitly configured.
func (c *Client) SetTimeout(value time.Duration) {
	c.Timeout = value
	c.timeoutSpecified = true
}

// SetRetries marks Retries as explicitly configured.
func (c *Client) SetRetries(value int) {
	c.Retries = value
	c.retriesSpecified = true
}

// SetRetrans marks Retrans as explicitly configured.
func (c *Client) SetRetrans(value time.Duration) {
	c.Retrans = value
	c.retransSpecified = true
}

// SetFallback marks Fallback as explicitly configured.
func (c *Client) SetFallback(value bool) {
	c.Fallback = value
	c.fallbackSpecified = true
}

// SetEDNSSize marks EDNSSize as explicitly configured.
func (c *Client) SetEDNSSize(value uint16) {
	c.EDNSSize = value
	c.ednsSizeSpecified = true
}

// SetRecursionDesired sets the RD bit on outbound queries.
func (c *Client) SetRecursionDesired(value bool) {
	c.RecursionDesired = value
}

// SetCheckingDisabled sets the CD bit on outbound queries.
func (c *Client) SetCheckingDisabled(value bool) {
	c.CheckingDisabled = value
}

// ApplyProfileDefaults sets unset fields using the effective profile defaults.
func (c *Client) ApplyProfileDefaults(p *profile.Profile) {
	if p == nil {
		p = profile.Effective()
	}

	defaults := p.Resolver.Defaults

	if !c.timeoutSpecified && c.Timeout == 0 {
		c.Timeout = time.Duration(defaults.Timeout) * time.Second
	}
	if !c.retriesSpecified && c.Retries == 0 {
		c.Retries = defaults.Retry
	}
	if !c.retransSpecified && c.Retrans == 0 {
		c.Retrans = time.Duration(defaults.Retrans) * time.Second
	}
	if !c.fallbackSpecified {
		c.Fallback = defaults.Fallback
	}
	if !c.ednsSizeSpecified && c.EDNSSize == 0 {
		if c.DNSSEC {
			c.EDNSSize = constants.EDNSUDPPayloadDNSSECDefault
		}
	}
}

// Exchange sends a DNS query to the given server and returns a Packet.
func (c *Client) Exchange(ctx context.Context, server string, msg *dns.Msg) (packet.Packet, error) {
	if msg == nil {
		return packet.Packet{}, fmt.Errorf("nil DNS message")
	}

	if err := acquireQuerySlot(ctx); err != nil {
		return packet.Packet{}, err
	}
	defer releaseQuerySlot(ctx)

	// Defaults land on a copy; the caller's client is never written to.
	eff := *c
	eff.ApplyProfileDefaults(profile.FromContext(ctx))
	prepared := eff.prepareMessage(msg)
	trace := querytrace.FromContext(ctx)
	qname, qtype := questionNameType(prepared)
	attempts := 1
	if eff.Retries > 0 {
		attempts = 1 + eff.Retries
	}

	var lastErr error
	for i := 0; i < attempts; i++ {
		if ctx != nil {
			if cerr := ctx.Err(); cerr != nil {
				lastErr = cerr
				break
			}
		}

		response, rtt, err := eff.tracedExchangeOnce(ctx, trace, server, prepared, qname, qtype, eff.UseTCP, false, i+1)
		if err != nil {
			if ctx != nil {
				if cerr := ctx.Err(); cerr != nil {
					lastErr = cerr
					break
				}
			}
			lastErr = err
			continue
		}

		usedTCP := eff.UseTCP
		if !eff.UseTCP && response.Truncated && eff.Fallback {
			usedTCP = true
			response, rtt, err = eff.tracedExchangeOnce(ctx, trace, server, prepared, qname, qtype, true, true, i+1)
			if err != nil {
				if ctx != nil {
					if cerr := ctx.Err(); cerr != nil {
						lastErr = cerr
						break
					}
				}
				lastErr = err
				continue
			}
		}

		pkt := packet.New(response)
		pkt.QueryTime = rtt
		pkt.Timestamp = time.Now()
		pkt.AnswerFrom = server
		pkt.Protocol = protocolName(usedTCP)
		return pkt, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("no response")
	}
	return packet.Packet{}, lastErr
}

// tracedExchangeOnce wraps exchangeOnce, reporting one AttemptEvent per attempt
// (including timeouts) when tracing is enabled.
func (c *Client) tracedExchangeOnce(ctx context.Context, trace querytrace.QueryTrace, server string, msg *dns.Msg, qname, qtype string, useTCP, fromUDPFallback bool, attempt int) (*dns.Msg, time.Duration, error) {
	if trace == nil {
		return c.exchangeOnce(ctx, server, msg, useTCP, fromUDPFallback)
	}

	start := time.Now()
	response, rtt, err := c.exchangeOnce(ctx, server, msg, useTCP, fromUDPFallback)

	trace.AttemptDone(querytrace.AttemptEvent{
		NSAddr:   server,
		QName:    qname,
		QType:    qtype,
		Protocol: protocolName(useTCP),
		Attempt:  attempt,
		Elapsed:  time.Since(start),
		Outcome:  classifyOutcome(err),
		Err:      errString(err),
	})
	return response, rtt, err
}

// protocolName names the transport of a completed exchange.
func protocolName(useTCP bool) string {
	if useTCP {
		return "tcp"
	}
	return "udp"
}

// questionNameType extracts the query name and type string from a message's
// first question, mirroring the access pattern in engine/packet.
func questionNameType(msg *dns.Msg) (string, string) {
	if msg == nil || len(msg.Question) == 0 {
		return "", ""
	}
	q := msg.Question[0]
	return q.Header().Name, dns.TypeToString[dns.RRToType(q)]
}

// classifyOutcome maps an exchange error to a trace Outcome.
func classifyOutcome(err error) querytrace.Outcome {
	if err == nil {
		return querytrace.OutcomeOK
	}
	if isTimeoutErr(err) {
		return querytrace.OutcomeTimeout
	}
	return querytrace.OutcomeError
}

func isTimeoutErr(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "timeout")
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (c *Client) exchangeOnce(ctx context.Context, server string, msg *dns.Msg, useTCP bool, fromUDPFallback bool) (*dns.Msg, time.Duration, error) {
	timeout := c.effectiveAttemptTimeout(ctx, useTCP, fromUDPFallback)
	network := "udp"
	if useTCP {
		network = "tcp"
	}

	dialer, err := c.buildDialer(timeout, network)
	if err != nil {
		return nil, 0, err
	}

	address := ensurePort(server)
	conn, err := dialer.DialContext(ctx, network, address)
	if err != nil {
		return nil, 0, err
	}
	defer conn.Close()

	if timeout > 0 {
		_ = conn.SetDeadline(time.Now().Add(timeout))
	}

	client := dns.NewClient()
	client.Transport.ReadTimeout = timeout
	return c.exchangeWithConnCancelable(ctx, client, msg, conn, useTCP)
}

func (c *Client) exchangeWithConnCancelable(ctx context.Context, client *dns.Client, msg *dns.Msg, conn net.Conn, useTCP bool) (*dns.Msg, time.Duration, error) {
	if client == nil || conn == nil {
		return nil, 0, fmt.Errorf("missing dns client or connection")
	}

	wireMsg, err := prepareWireMessage(msg, useTCP)
	if err != nil {
		return nil, 0, err
	}

	if ctx == nil || ctx.Done() == nil {
		return client.ExchangeWithConn(ctx, wireMsg, conn)
	}

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			// Force socket interruption when cancellation happens without an
			// earlier deadline applied to the underlying connection.
			_ = conn.SetDeadline(time.Now())
		case <-done:
		}
	}()

	resp, rtt, err := client.ExchangeWithConn(ctx, wireMsg, conn)
	close(done)
	if cerr := ctx.Err(); cerr != nil {
		return nil, 0, cerr
	}
	return resp, rtt, err
}

func prepareWireMessage(msg *dns.Msg, useTCP bool) (*dns.Msg, error) {
	if msg == nil {
		return nil, fmt.Errorf("nil DNS message")
	}

	wireMsg := msg.Copy()
	if len(wireMsg.Data) == 0 {
		if err := wireMsg.Pack(); err != nil {
			return nil, err
		}
	}

	if !useTCP && wireMsg.UDPSize < constants.EDNSUDPPayloadCommonLimit {
		// Keep the outgoing query bytes untouched, but allow reception of UDP
		// responses that exceed 512 bytes from authoritative servers.
		wireMsg.UDPSize = constants.EDNSUDPPayloadCommonLimit
	}

	return wireMsg, nil
}

func (c *Client) effectiveAttemptTimeout(ctx context.Context, useTCP bool, fromUDPFallback bool) time.Duration {
	attempt := c.Timeout
	if attempt <= 0 {
		if c.Retrans > 0 {
			attempt = c.Retrans
		} else {
			attempt = defaultTimeout
		}
	} else if c.Retrans > 0 && c.Retrans < attempt && (!useTCP || fromUDPFallback) {
		// UDP and fallback TCP attempts should use retrans pacing so a blocked
		// path does not stall progression for the full timeout budget.
		attempt = c.Retrans
	}
	if ctx != nil {
		if deadline, ok := ctx.Deadline(); ok {
			remaining := time.Until(deadline)
			if remaining > 0 && remaining < attempt {
				attempt = remaining
			}
		}
	}
	if attempt <= 0 {
		return defaultTimeout
	}
	return attempt
}

func (c *Client) buildDialer(timeout time.Duration, network string) (*net.Dialer, error) {
	dialer := &net.Dialer{Timeout: timeout}
	if c.SourceIP == "" {
		return dialer, nil
	}

	ip := net.ParseIP(c.SourceIP)
	if ip == nil {
		return nil, fmt.Errorf("invalid source IP: %s", c.SourceIP)
	}

	if network == "udp" {
		dialer.LocalAddr = &net.UDPAddr{IP: ip, Port: c.SourcePort}
		return dialer, nil
	}

	dialer.LocalAddr = &net.TCPAddr{IP: ip, Port: c.SourcePort}
	return dialer, nil
}

// Dialer returns a dialer configured with the client's source settings.
func (c *Client) Dialer(network string) (*net.Dialer, error) {
	timeout := c.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	return c.buildDialer(timeout, network)
}

func (c *Client) prepareMessage(msg *dns.Msg) *dns.Msg {
	prepared := msg.Copy()
	if c.RecursionDesired {
		prepared.RecursionDesired = true
	}
	if c.CheckingDisabled {
		prepared.CheckingDisabled = true
	}

	if c.EDNSSize > 0 || c.EDNSDetails != nil {
		prepared.UDPSize = c.EDNSSize
		prepared.Security = c.DNSSEC

		var z *uint16
		if c.EDNSDetails != nil {
			if c.EDNSDetails.Do != nil {
				prepared.Security = *c.EDNSDetails.Do
			}
			if c.EDNSDetails.Size != nil {
				prepared.UDPSize = *c.EDNSDetails.Size
			}
			if c.EDNSDetails.Version != nil {
				prepared.Version = *c.EDNSDetails.Version
			}
			if c.EDNSDetails.Z != nil {
				z = c.EDNSDetails.Z
			}
			if c.EDNSDetails.Rcode != nil {
				// Extended rcode: lower 4 bits stay in header Rcode; upper 8 bits go in OPT.
				// v2 encodes this transparently from m.Rcode (uint16).
				prepared.Rcode = uint16(*c.EDNSDetails.Rcode)
			}
			if len(c.EDNSDetails.Data) > 0 {
				for _, opt := range c.EDNSDetails.Data {
					prepared.Pseudo = append(prepared.Pseudo, opt)
				}
			}
		}

		// The dns v2 auto-OPT path ignores Version and omits OPT when UDPSize is 512.
		// Force explicit OPT for EDNSDetails and 512-byte EDNS queries.
		if c.EDNSDetails != nil || prepared.UDPSize <= dns.MinMsgSize {
			applyExplicitEDNS(prepared, z)
		}
	}

	return prepared
}

func applyExplicitEDNS(msg *dns.Msg, z *uint16) {
	if msg == nil {
		return
	}

	// If pseudo contains non-EDNS records (e.g. TSIG), keep default packing path
	// to avoid changing section ordering semantics.
	opt := &dns.OPT{Hdr: dns.Header{Name: "."}}
	for _, rr := range msg.Pseudo {
		edns, ok := rr.(dns.EDNS0)
		if !ok {
			return
		}
		opt.Options = append(opt.Options, edns)
	}

	compactAnswers, delegation := msg.CompactAnswers, msg.Delegation
	if z != nil {
		compactAnswers = compactAnswers || *z&ednsZCompactAnswers != 0
		delegation = delegation || *z&ednsZDelegation != 0
	}

	udpSize := max(msg.UDPSize, dns.MinMsgSize)
	ednsopt.SetUDPSize(opt, udpSize)
	ednsopt.SetVersion(opt, msg.Version)
	ednsopt.SetSecurity(opt, msg.Security)
	ednsopt.SetCompactAnswers(opt, compactAnswers)
	ednsopt.SetDelegation(opt, delegation)
	ednsopt.SetRcode(opt, msg.Rcode)
	if z != nil {
		ednsopt.SetZ(opt, *z)
	}

	extra := make([]dns.RR, 0, len(msg.Extra)+1)
	for _, rr := range msg.Extra {
		if _, isOPT := rr.(*dns.OPT); isOPT {
			continue
		}
		extra = append(extra, rr)
	}
	extra = append(extra, opt)
	msg.Extra = extra

	// Prevent Msg.Pack from auto-synthesizing a second OPT RR. The explicit OPT
	// above now carries EDNS settings/options, with the base rcode kept in header.
	msg.Pseudo = nil
	msg.UDPSize = 0
	msg.Security = false
	msg.CompactAnswers = false
	msg.Delegation = false
	msg.Version = 0
	msg.Z = 0
	msg.Rcode &= 0xF
}

func ensurePort(server string) string {
	host, port, err := net.SplitHostPort(server)
	if err == nil {
		return net.JoinHostPort(host, port)
	}
	return net.JoinHostPort(server, "53")
}
