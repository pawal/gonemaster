package packet

import (
	"fmt"
	"strings"
	"time"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/logger"
)

// Packet wraps a DNS message and exposes helpers mirroring the Perl engine API.
type Packet struct {
	Msg        *dns.Msg
	AnswerFrom string
	QueryTime  time.Duration
	Timestamp  time.Time
	Log        *logger.Logger
}

// New wraps a dns.Msg into a Packet helper.
func New(msg *dns.Msg) Packet {
	return Packet{Msg: msg}
}

// ID returns the DNS message ID.
func (p Packet) ID() uint16 {
	if p.Msg == nil {
		return 0
	}
	return p.Msg.ID
}

// Opcode returns the DNS opcode string.
func (p Packet) Opcode() string {
	if p.Msg == nil {
		return ""
	}
	return dns.OpcodeToString[p.Msg.Opcode]
}

// Rcode returns the DNS rcode string.
func (p Packet) Rcode() string {
	if p.Msg == nil {
		return ""
	}
	return dns.RcodeToString[p.Msg.Rcode]
}

// String returns the textual representation of the message.
func (p Packet) String() string {
	if p.Msg == nil {
		return ""
	}
	return p.Msg.String()
}

// Data serializes the message to wire format.
func (p Packet) Data() []byte {
	if p.Msg == nil {
		return nil
	}
	if err := p.Msg.Pack(); err != nil {
		return nil
	}
	b := make([]byte, len(p.Msg.Data))
	copy(b, p.Msg.Data)
	return b
}

// AA reports whether the authoritative answer flag is set.
func (p Packet) AA() bool {
	if p.Msg == nil {
		return false
	}
	return p.Msg.Authoritative
}

// RA reports whether recursion available is set.
func (p Packet) RA() bool {
	if p.Msg == nil {
		return false
	}
	return p.Msg.RecursionAvailable
}

// TC reports whether the response is truncated.
func (p Packet) TC() bool {
	if p.Msg == nil {
		return false
	}
	return p.Msg.Truncated
}

// Question returns the DNS question section.
func (p Packet) Question() []dns.RR {
	if p.Msg == nil {
		return nil
	}
	return p.Msg.Question
}

// Answer returns the DNS answer section.
func (p Packet) Answer() []dns.RR {
	if p.Msg == nil {
		return nil
	}
	return p.Msg.Answer
}

// Authority returns the DNS authority section.
func (p Packet) Authority() []dns.RR {
	if p.Msg == nil {
		return nil
	}
	return p.Msg.Ns
}

// Additional returns the DNS additional section.
func (p Packet) Additional() []dns.RR {
	if p.Msg == nil {
		return nil
	}
	return p.Msg.Extra
}

// HasEdns reports whether an OPT record is present.
func (p Packet) HasEdns() bool {
	if p.Msg == nil {
		return false
	}
	return p.Msg.UDPSize > 0 || p.Msg.Security || len(p.Msg.Pseudo) > 0 || ednsOPT(p.Msg) != nil
}

// EdnsSize returns the EDNS UDP payload size.
func (p Packet) EdnsSize() uint16 {
	if p.Msg == nil {
		return 0
	}
	if p.Msg.UDPSize > 0 {
		return p.Msg.UDPSize
	}
	if opt := ednsOPT(p.Msg); opt != nil {
		return opt.UDPSize()
	}
	return 0
}

// EdnsRcode returns the EDNS extended rcode upper byte (bits 4–11 of the 12-bit rcode).
func (p Packet) EdnsRcode() int {
	if p.Msg == nil {
		return 0
	}
	if p.Msg.Rcode > 0xF {
		return int(p.Msg.Rcode >> 4)
	}
	if opt := ednsOPT(p.Msg); opt != nil {
		return int(opt.Rcode() >> 4)
	}
	return 0
}

// EdnsVersion returns the EDNS version.
func (p Packet) EdnsVersion() uint8 {
	if p.Msg == nil {
		return 0
	}
	if p.Msg.Version != 0 {
		return p.Msg.Version
	}
	if opt := ednsOPT(p.Msg); opt != nil {
		return opt.Version()
	}
	return 0
}

// EdnsZ returns the raw EDNS Z bits.
func (p Packet) EdnsZ() uint16 {
	if p.Msg == nil {
		return 0
	}
	if opt := ednsOPT(p.Msg); opt != nil {
		return opt.Z()
	}
	return 0
}

// EdnsData returns the EDNS pseudo-section option records (e.g. NSID, COOKIE).
func (p Packet) EdnsData() []dns.RR {
	if p.Msg == nil {
		return nil
	}
	if len(p.Msg.Pseudo) > 0 {
		return p.Msg.Pseudo
	}
	if opt := ednsOPT(p.Msg); opt != nil && len(opt.Options) > 0 {
		out := make([]dns.RR, 0, len(opt.Options))
		for _, edns := range opt.Options {
			out = append(out, edns)
		}
		return out
	}
	return nil
}

// DO reports whether the EDNS DO bit is set.
func (p Packet) DO() bool {
	if p.Msg == nil {
		return false
	}
	if p.Msg.Security {
		return true
	}
	if opt := ednsOPT(p.Msg); opt != nil {
		return opt.Security()
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
	if p.Type() != "nodata" {
		return false
	}
	args := map[string]any{}
	if name, qtype, ok := packetQuestionInfo(p.Msg); ok {
		args["name"] = name
		args["type"] = qtype
	}
	p.logSystem("NO_SUCH_RECORD", args)
	return true
}

// NoSuchName reports an NXDOMAIN response.
func (p Packet) NoSuchName() bool {
	if p.Type() != "nxdomain" {
		return false
	}
	args := map[string]any{}
	if name, qtype, ok := packetQuestionInfo(p.Msg); ok {
		args["name"] = name
		args["type"] = qtype
	}
	p.logSystem("NO_SUCH_NAME", args)
	return true
}

// IsRedirect reports a referral response.
func (p Packet) IsRedirect() bool {
	if p.Type() != "referral" {
		return false
	}
	args := map[string]any{}
	if name, qtype, ok := packetQuestionInfo(p.Msg); ok {
		args["name"] = name
		args["type"] = qtype
	}
	if p.Msg != nil && len(p.Msg.Ns) > 0 {
		args["to"] = dnsname.New(p.Msg.Ns[0].Header().Name).String()
	}
	p.logSystem("IS_REDIRECT", args)
	return true
}

// AnswerFromString returns the source address or "<unknown>" when missing.
func (p Packet) AnswerFromString() string {
	if p.AnswerFrom == "" {
		return "<unknown>"
	}
	return p.AnswerFrom
}

func (p Packet) logSystem(tag string, args map[string]any) {
	if p.Log == nil {
		return
	}
	_, _ = p.Log.Add(tag, args, "System", "")
}

func ednsOPT(msg *dns.Msg) *dns.OPT {
	if msg == nil {
		return nil
	}
	for _, rr := range msg.Extra {
		opt, ok := rr.(*dns.OPT)
		if ok {
			return opt
		}
	}
	return nil
}

func packetQuestionInfo(msg *dns.Msg) (string, string, bool) {
	if msg == nil || len(msg.Question) == 0 {
		return "", "", false
	}
	q := msg.Question[0]
	name := dnsname.New(q.Header().Name).String()
	qtype := dns.TypeToString[dns.RRToType(q)]
	if qtype == "" {
		qtype = fmt.Sprint(dns.RRToType(q))
	}
	return name, qtype, true
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
		if strings.EqualFold(dns.TypeToString[dns.RRToType(rr)], wantType) {
			out = append(out, rr)
		}
	}
	return out
}

func hasType(rrs []dns.RR, want uint16) bool {
	for _, rr := range rrs {
		if dns.RRToType(rr) == want {
			return true
		}
	}
	return false
}
