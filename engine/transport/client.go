package transport

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/constants"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
)

const defaultTimeout = 5 * time.Second

// Client performs DNS exchanges with configurable behavior.
type Client struct {
	Timeout          time.Duration
	Retries          int
	Retrans          time.Duration
	UseTCP           bool
	Fallback         bool
	EDNSSize         uint16
	DNSSEC           bool
	EDNSDetails      *EDNSDetails
	RecursionDesired bool
	SourceIP         string
	SourcePort       int

	useTCPSpecified           bool
	timeoutSpecified          bool
	retriesSpecified          bool
	retransSpecified          bool
	fallbackSpecified         bool
	ednsSizeSpecified         bool
	recursionDesiredSpecified bool
}

// EDNSDetails captures explicit EDNS settings and overrides.
type EDNSDetails struct {
	Do      *bool
	Size    *uint16
	Version *uint8
	Z       *uint16
	Rcode   *uint8
	Data    []dns.EDNS0
}

// BuildQuery constructs a query message with common defaults.
func BuildQuery(name string, qtype uint16) *dns.Msg {
	return BuildQueryWithClass(name, qtype, dns.ClassINET)
}

// BuildQueryWithClass constructs a query message for the given class.
func BuildQueryWithClass(name string, qtype, qclass uint16) *dns.Msg {
	msg := new(dns.Msg)
	msg.Question = []dns.Question{{
		Name:   dns.Fqdn(name),
		Qtype:  qtype,
		Qclass: qclass,
	}}
	msg.RecursionDesired = false
	return msg
}

// SetUseTCP marks UseTCP as explicitly configured.
func (c *Client) SetUseTCP(value bool) {
	c.UseTCP = value
	c.useTCPSpecified = true
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

// SetRecursionDesired marks RecursionDesired as explicitly configured.
func (c *Client) SetRecursionDesired(value bool) {
	c.RecursionDesired = value
	c.recursionDesiredSpecified = true
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
	if !c.useTCPSpecified {
		c.UseTCP = defaults.UseVC
	}
	if !c.fallbackSpecified {
		c.Fallback = defaults.Fallback
	}
	if !c.recursionDesiredSpecified {
		c.RecursionDesired = defaults.Recurse
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

	c.ApplyProfileDefaults(profile.FromContext(ctx))
	prepared := c.prepareMessage(msg)
	attempts := 1
	if c.Retries > 0 {
		attempts = 1 + c.Retries
	}

	var lastErr error
	for i := 0; i < attempts; i++ {
		response, rtt, err := c.exchangeOnce(ctx, server, prepared, c.UseTCP)
		if err != nil {
			if !c.UseTCP && c.Fallback {
				response, rtt, err = c.exchangeOnce(ctx, server, prepared, true)
				if err == nil {
					pkt := packet.New(response)
					pkt.QueryTime = rtt
					pkt.Timestamp = time.Now()
					pkt.AnswerFrom = server
					return pkt, nil
				}
				lastErr = err
				continue
			}

			lastErr = err
			continue
		}

		if !c.UseTCP && response.Truncated && c.Fallback {
			response, rtt, err = c.exchangeOnce(ctx, server, prepared, true)
			if err != nil {
				lastErr = err
				continue
			}
		}

		pkt := packet.New(response)
		pkt.QueryTime = rtt
		pkt.Timestamp = time.Now()
		pkt.AnswerFrom = server
		return pkt, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("no response")
	}
	return packet.Packet{}, lastErr
}

func (c *Client) exchangeOnce(ctx context.Context, server string, msg *dns.Msg, useTCP bool) (*dns.Msg, time.Duration, error) {
	client := dns.Client{Net: "udp", Timeout: defaultTimeout}
	if c.Timeout > 0 {
		client.Timeout = c.Timeout
	}
	if useTCP {
		client.Net = "tcp"
	}

	dialer, err := c.buildDialer(client.Timeout, client.Net)
	if err != nil {
		return nil, 0, err
	}
	client.Dialer = dialer

	address := ensurePort(server)
	response, rtt, err := client.ExchangeContext(ctx, msg.Copy(), address)
	if err != nil {
		return nil, 0, err
	}
	return response, rtt, nil
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

	if c.EDNSSize > 0 || c.EDNSDetails != nil {
		prepared.SetEdns0(c.EDNSSize, c.DNSSEC)
		opt := prepared.IsEdns0()
		if c.EDNSDetails != nil {
			if opt == nil {
				return prepared
			}
			if c.EDNSDetails.Do != nil {
				opt.SetDo(*c.EDNSDetails.Do)
			}
			if c.EDNSDetails.Version != nil {
				opt.SetVersion(*c.EDNSDetails.Version)
			}
			if c.EDNSDetails.Z != nil {
				opt.SetZ(*c.EDNSDetails.Z)
			}
			if c.EDNSDetails.Rcode != nil {
				opt.SetExtendedRcode(uint16(*c.EDNSDetails.Rcode))
			}
			if len(c.EDNSDetails.Data) > 0 {
				opt.Option = append(opt.Option, c.EDNSDetails.Data...)
			}
		}
	}

	return prepared
}

func ensurePort(server string) string {
	host, port, err := net.SplitHostPort(server)
	if err == nil {
		return net.JoinHostPort(host, port)
	}
	return net.JoinHostPort(server, "53")
}
