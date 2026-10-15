package dnsname

import "strings"

// Escape writes the octets 0x00 to 0x20 and 0x7F to 0xFF of s as \DDD (RFC 4343 section 2.1).
func Escape(s string) string {
	i := 0
	for i < len(s) && s[i] > 0x20 && s[i] < 0x7f {
		i++
	}
	if i == len(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	b.WriteString(s[:i])
	for ; i < len(s); i++ {
		if c := s[i]; c <= 0x20 || c >= 0x7f {
			writeDecimal(&b, c)
		} else {
			b.WriteByte(c)
		}
	}
	return b.String()
}

// writeDecimal writes c as \DDD.
func writeDecimal(b *strings.Builder, c byte) {
	b.Write([]byte{'\\', '0' + c/100, '0' + c/10%10, '0' + c%10})
}
