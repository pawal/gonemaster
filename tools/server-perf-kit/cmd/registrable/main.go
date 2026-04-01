package main

import (
	"bufio"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"
)

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// rank<TAB>domain format passthrough for ranked inputs.
		if rank, domain, ok := splitRankedLine(line); ok {
			if reg, ok := toRegistrable(domain); ok {
				fmt.Printf("%s\t%s\n", rank, reg)
			}
			continue
		}

		if reg, ok := toRegistrable(line); ok {
			fmt.Println(reg)
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func splitRankedLine(line string) (rank string, domain string, ok bool) {
	parts := strings.SplitN(line, "\t", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	r := strings.TrimSpace(parts[0])
	d := strings.TrimSpace(parts[1])
	if r == "" || d == "" {
		return "", "", false
	}
	if _, err := strconv.Atoi(r); err != nil {
		return "", "", false
	}
	return r, d, true
}

func toRegistrable(raw string) (string, bool) {
	host, ok := extractHost(raw)
	if !ok {
		return "", false
	}
	host = strings.ToLower(strings.Trim(host, "."))
	if host == "" {
		return "", false
	}
	if net.ParseIP(host) != nil {
		return "", false
	}

	ascii, err := idna.Lookup.ToASCII(host)
	if err != nil || ascii == "" {
		return "", false
	}
	ascii = strings.ToLower(strings.Trim(ascii, "."))
	if ascii == "" {
		return "", false
	}

	reg, err := publicsuffix.EffectiveTLDPlusOne(ascii)
	if err != nil || reg == "" {
		return "", false
	}
	return strings.ToLower(reg), true
}

func extractHost(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	s = strings.Trim(s, "\"'")
	if s == "" {
		return "", false
	}

	// If already URL-like, parse as URL.
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err == nil && u.Host != "" {
			return stripPort(u.Host), true
		}
	}

	// Strip obvious path/query fragments for host-like inputs.
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}

	// If host:port, remove port.
	s = stripPort(s)
	s = strings.TrimSpace(strings.Trim(s, "."))
	if s == "" {
		return "", false
	}
	return s, true
}

func stripPort(hostport string) string {
	h := strings.TrimSpace(hostport)
	h = strings.TrimPrefix(h, "[")
	h = strings.TrimSuffix(h, "]")
	if h == "" {
		return ""
	}
	// IPv6 literal with colons (without brackets) should remain unchanged.
	if strings.Count(h, ":") > 1 {
		return h
	}
	host, port, err := net.SplitHostPort(hostport)
	if err == nil {
		_ = port
		host = strings.TrimPrefix(host, "[")
		host = strings.TrimSuffix(host, "]")
		return host
	}
	return h
}

