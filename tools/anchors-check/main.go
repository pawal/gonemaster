// Command anchors-check holds the root trust anchors in engine/dnssecutil to
// the digests IANA publishes as current, after verifying the detached
// signature over root-anchors.xml against the checked-in ICANN certificate.
//
// Usage:
//
//	go run ./tools/anchors-check [--xml URL|PATH] [--p7s URL|PATH] [--bundle PATH] [--openssl BIN]
//
// It needs the network and openssl, and is never part of make test.
package main

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"codeberg.org/pawal/gonemaster/engine/dnssecutil"
)

const (
	xmlURL = "https://data.iana.org/root-anchors/root-anchors.xml"
	p7sURL = "https://data.iana.org/root-anchors/root-anchors.p7s"
	// bundlePath is data.iana.org/root-anchors/icannbundle.pem, checked in.
	bundlePath   = "tools/anchors-check/icannbundle.pem"
	fetchTimeout = 30 * time.Second
)

type options struct {
	xml     string
	p7s     string
	bundle  string
	openssl string
}

// anchor is one root DS, digest in upper case.
type anchor struct {
	keyTag     uint16
	algorithm  uint8
	digestType uint8
	digest     string
}

func main() {
	opts := options{}
	flag.StringVar(&opts.xml, "xml", xmlURL, "trust anchor document, URL or path")
	flag.StringVar(&opts.p7s, "p7s", p7sURL, "detached signature over it, URL or path")
	flag.StringVar(&opts.bundle, "bundle", bundlePath, "ICANN certificate the signature is verified against")
	flag.StringVar(&opts.openssl, "openssl", "openssl", "openssl binary that verifies the signature")
	flag.Parse()

	if err := run(opts, time.Now().UTC()); err != nil {
		fmt.Fprintln(os.Stderr, "anchors-check:", err)
		os.Exit(1)
	}
}

func run(opts options, now time.Time) error {
	document, err := read(opts.xml)
	if err != nil {
		return err
	}
	signature, err := read(opts.p7s)
	if err != nil {
		return err
	}
	if err := verify(opts, document, signature); err != nil {
		return err
	}
	current, err := published(document, now)
	if err != nil {
		return err
	}
	built := builtIn()
	if problems := compare(built, current); len(problems) > 0 {
		return errors.New("engine/dnssecutil differs from IANA:\n  " + strings.Join(problems, "\n  "))
	}
	fmt.Printf("anchors-check: the %d anchors in engine/dnssecutil match the %d digests IANA publishes as current\n",
		len(built), len(current))
	return nil
}

// read takes a URL or a path, so the check runs against a stored copy.
func read(source string) ([]byte, error) {
	if !strings.HasPrefix(source, "http://") && !strings.HasPrefix(source, "https://") {
		return os.ReadFile(source)
	}
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching %s: %s", source, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}

// verify checks the detached CMS signature with openssl; the standard library has no CMS.
func verify(opts options, document, signature []byte) error {
	file, err := os.CreateTemp("", "root-anchors-*.xml")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(document); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	cmd := exec.Command(opts.openssl, "smime", "-verify", "-CAfile", opts.bundle,
		"-inform", "DER", "-content", file.Name(), "-out", os.DevNull)
	cmd.Stdin = bytes.NewReader(signature)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("verifying the signature over %s: %v: %s", opts.xml, err, out)
	}
	return nil
}

// trustAnchorDocument is root-anchors.xml; only the digests are read.
type trustAnchorDocument struct {
	Zone       string `xml:"Zone"`
	KeyDigests []struct {
		ValidFrom  string `xml:"validFrom,attr"`
		ValidUntil string `xml:"validUntil,attr"`
		KeyTag     uint16 `xml:"KeyTag"`
		Algorithm  uint8  `xml:"Algorithm"`
		DigestType uint8  `xml:"DigestType"`
		Digest     string `xml:"Digest"`
	} `xml:"KeyDigest"`
}

// published returns the root digests valid at now.
func published(document []byte, now time.Time) ([]anchor, error) {
	var doc trustAnchorDocument
	if err := xml.Unmarshal(document, &doc); err != nil {
		return nil, fmt.Errorf("reading the trust anchor document: %w", err)
	}
	if doc.Zone != "." {
		return nil, fmt.Errorf("the trust anchor document is for zone %q, not the root", doc.Zone)
	}
	var out []anchor
	for _, digest := range doc.KeyDigests {
		from, err := when(digest.ValidFrom)
		if err != nil {
			return nil, fmt.Errorf("key digest %d: %w", digest.KeyTag, err)
		}
		until, err := when(digest.ValidUntil)
		if err != nil {
			return nil, fmt.Errorf("key digest %d: %w", digest.KeyTag, err)
		}
		if now.Before(from) || (!until.IsZero() && !now.Before(until)) {
			continue
		}
		out = append(out, anchor{digest.KeyTag, digest.Algorithm, digest.DigestType, strings.ToUpper(digest.Digest)})
	}
	if len(out) == 0 {
		return nil, errors.New("the trust anchor document carries no current digest")
	}
	return out, nil
}

func when(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, value)
}

// builtIn returns the anchors engine/dnssecutil carries.
func builtIn() []anchor {
	var out []anchor
	for _, ds := range dnssecutil.RootTrustAnchors() {
		out = append(out, anchor{ds.KeyTag, ds.Algorithm, ds.DigestType, strings.ToUpper(ds.Digest)})
	}
	return out
}

// compare reports what each set carries and the other does not.
func compare(built, current []anchor) []string {
	var problems []string
	for _, a := range current {
		if !slices.Contains(built, a) {
			problems = append(problems, fmt.Sprintf("IANA publishes key %d, which engine/dnssecutil does not carry", a.keyTag))
		}
	}
	for _, a := range built {
		if !slices.Contains(current, a) {
			problems = append(problems, fmt.Sprintf("engine/dnssecutil carries key %d, which IANA does not publish as current", a.keyTag))
		}
	}
	return problems
}
