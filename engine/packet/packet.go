package packet

import (
	"strings"
	"time"

	"github.com/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
)

// Packet wraps a DNS message and exposes helpers mirroring the Perl engine API.
type Packet struct {
	Msg        *dns.Msg
	AnswerFrom string
	QueryTime  time.Duration
	Timestamp  time.Time
}

func New(msg *dns.Msg) Packet {
	return Packet{Msg: msg}
}

func (p Packet) ID() uint16 {
	if p.Msg == nil {
		return 0
	}
	return p.Msg.Id
}

func (p Packet) Opcode() string {
	if p.Msg == nil {
		return ""
	}
	return dns.OpcodeToString[p.Msg.Opcode]
}

func (p Packet) Rcode() string {
	if p.Msg == nil {
		return ""
	}
	return dns.RcodeToString[p.Msg.Rcode]
}

func (p Packet) String() string {
	if p.Msg == nil {
		return ""
	}
	return p.Msg.String()
}

func (p Packet) Data() []byte {
	if p.Msg == nil {
		return nil
	}
	wire, err := p.Msg.Pack()
	if err != nil {
		return nil
	}
	return wire
}

func (p Packet) AA() bool {
	if p.Msg == nil {
		return false
	}
	return p.Msg.Authoritative
}

func (p Packet) RA() bool {
	if p.Msg == nil {
		return false
	}
	return p.Msg.RecursionAvailable
}

func (p Packet) TC() bool {
	if p.Msg == nil {
		return false
	}
	return p.Msg.Truncated
}

func (p Packet) Question() []dns.Question {
	if p.Msg == nil {
		return nil
	}
	return p.Msg.Question
}

func (p Packet) Answer() []dns.RR {
	if p.Msg == nil {
		return nil
	}
	return p.Msg.Answer
}

func (p Packet) Authority() []dns.RR {
	if p.Msg == nil {
		return nil
	}
	return p.Msg.Ns
}

func (p Packet) Additional() []dns.RR {
	if p.Msg == nil {
		return nil
	}
	return p.Msg.Extra
}

func (p Packet) HasEdns() bool {
	if p.Msg == nil {
		return false
	}
	return p.Msg.IsEdns0() != nil
}

func (p Packet) EdnsSize() uint16 {
	if opt := p.Msg.IsEdns0(); opt != nil {
		return opt.UDPSize()
	}
	return 0
}

func (p Packet) EdnsRcode() int {
	if opt := p.Msg.IsEdns0(); opt != nil {
		return opt.ExtendedRcode() >> 4
	}
	return 0
}

func (p Packet) EdnsVersion() uint8 {
	if opt := p.Msg.IsEdns0(); opt != nil {
		return opt.Version()
	}
	return 0
}

func (p Packet) EdnsZ() uint16 {
	if opt := p.Msg.IsEdns0(); opt != nil {
		return opt.Z()
	}
	return 0
}

func (p Packet) EdnsData() []dns.EDNS0 {
	if opt := p.Msg.IsEdns0(); opt != nil {
		return opt.Option
	}
	return nil
}

func (p Packet) DO() bool {
	if opt := p.Msg.IsEdns0(); opt != nil {
		return opt.Do()
	}
	return false
}

// Type approximates LDNS packet classification.
func (p Packet) Type() string {
	if p.Msg == nil {
		return ""
	}
	if p.Msg.Rcode == dns.RcodeNameError {
		return "nxdomain"
	}
	if p.Msg.Rcode != dns.RcodeSuccess {
		return "error"
	}
	if len(p.Msg.Answer) == 0 {
		if hasType(p.Msg.Ns, dns.TypeSOA) {
			return "nodata"
		}
		if hasType(p.Msg.Ns, dns.TypeNS) {
			return "referral"
		}
	}
	return "answer"
}

// NoSuchRecord reports a nodata response.
func (p Packet) NoSuchRecord() bool {
	return p.Type() == "nodata"
}

// NoSuchName reports an NXDOMAIN response.
func (p Packet) NoSuchName() bool {
	return p.Type() == "nxdomain"
}

// IsRedirect reports a referral response.
func (p Packet) IsRedirect() bool {
	return p.Type() == "referral"
}

// AnswerFromString returns the source address or "<unknown>" when missing.
func (p Packet) AnswerFromString() string {
	if p.AnswerFrom == "" {
		return "<unknown>"
	}
	return p.AnswerFrom
}

// UniquePush adds an RR to a section if it is not already present.
func (p *Packet) UniquePush(section string, rr dns.RR) bool {
	if p == nil || p.Msg == nil || rr == nil {
		return false
	}

	section = strings.ToLower(section)
	var target *[]dns.RR
	switch section {
	case "answer":
		target = &p.Msg.Answer
	case "authority":
		target = &p.Msg.Ns
	case "additional":
		target = &p.Msg.Extra
	default:
		return false
	}

	for _, existing := range *target {
		if strings.EqualFold(existing.String(), rr.String()) {
			return false
		}
	}

	*target = append(*target, rr)
	return true
}

// GetRecords returns RRs of the given type in the requested sections.
func (p Packet) GetRecords(rrtype string, sections ...string) []dns.RR {
	if p.Msg == nil {
		return nil
	}

	sections = normalizeSections(sections)
	wantType := strings.ToUpper(rrtype)

	var out []dns.RR
	for _, section := range sections {
		switch section {
		case "answer":
			out = append(out, filterRRs(p.Msg.Answer, wantType)...)
		case "authority":
			out = append(out, filterRRs(p.Msg.Ns, wantType)...)
		case "additional":
			out = append(out, filterRRs(p.Msg.Extra, wantType)...)
		}
	}

	return out
}

// GetRecordsForName returns RRs of the given type for a specific name.
func (p Packet) GetRecordsForName(rrtype string, name dnsname.Name, sections ...string) []dns.RR {
	nameStr := name.String()
	var out []dns.RR
	for _, rr := range p.GetRecords(rrtype, sections...) {
		rrName := dnsname.New(rr.Header().Name)
		if strings.EqualFold(rrName.String(), nameStr) {
			out = append(out, rr)
		}
	}
	return out
}

// HasRRsOfTypeForName checks for any RR of the given type and name.
func (p Packet) HasRRsOfTypeForName(rrtype string, name dnsname.Name, sections ...string) bool {
	return len(p.GetRecordsForName(rrtype, name, sections...)) > 0
}

func normalizeSections(sections []string) []string {
	if len(sections) == 0 {
		return []string{"answer", "authority", "additional"}
	}
	out := make([]string, 0, len(sections))
	for _, section := range sections {
		out = append(out, strings.ToLower(section))
	}
	return out
}

func filterRRs(rrs []dns.RR, wantType string) []dns.RR {
	var out []dns.RR
	for _, rr := range rrs {
		if strings.EqualFold(dns.TypeToString[rr.Header().Rrtype], wantType) {
			out = append(out, rr)
		}
	}
	return out
}

func hasType(rrs []dns.RR, want uint16) bool {
	for _, rr := range rrs {
		if rr.Header().Rrtype == want {
			return true
		}
	}
	return false
}
