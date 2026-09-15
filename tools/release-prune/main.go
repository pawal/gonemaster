// Command release-prune deletes attachments from superseded releases, keeping
// the newest few. Tags, release notes and history stay in place.
//
// Usage:
//
//	go run ./tools/release-prune [--repo owner/name] [--keep 3] [--dry-run=false]
//
// The API token is read from CODEBERG_RELEASE_TOKEN.
package main

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

const pageLimit = 50

type asset struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}

type release struct {
	ID      int64   `json:"id"`
	TagName string  `json:"tag_name"`
	Assets  []asset `json:"assets"`
}

// version is a vX.Y.Z tag. Tags of any other shape are never pruned.
type version struct{ major, minor, patch int }

func parseVersion(tag string) (version, bool) {
	rest, ok := strings.CutPrefix(tag, "v")
	if !ok {
		return version{}, false
	}
	parts := strings.Split(rest, ".")
	if len(parts) != 3 {
		return version{}, false
	}
	var v version
	for i, field := range []*int{&v.major, &v.minor, &v.patch} {
		n, err := strconv.Atoi(parts[i])
		if err != nil || n < 0 {
			return version{}, false
		}
		*field = n
	}
	return v, true
}

func compareVersion(a, b version) int {
	if c := cmp.Compare(a.major, b.major); c != 0 {
		return c
	}
	if c := cmp.Compare(a.minor, b.minor); c != 0 {
		return c
	}
	return cmp.Compare(a.patch, b.patch)
}

// prunable returns the releases that still carry attachments below the newest
// keep versions, newest first.
func prunable(rels []release, keep int) []release {
	type entry struct {
		rel release
		ver version
	}
	var versioned []entry
	for _, r := range rels {
		if v, ok := parseVersion(r.TagName); ok {
			versioned = append(versioned, entry{rel: r, ver: v})
		}
	}
	slices.SortFunc(versioned, func(a, b entry) int { return compareVersion(b.ver, a.ver) })

	var out []release
	for _, e := range versioned[min(keep, len(versioned)):] {
		if len(e.rel.Assets) > 0 {
			out = append(out, e.rel)
		}
	}
	return out
}

type client struct {
	base  string
	repo  string
	token string
	http  *http.Client
}

func (c *client) do(method, path string) ([]byte, error) {
	req, err := http.NewRequest(method, c.base+"/api/v1/repos/"+c.repo+path, nil)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "token "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, bytes.TrimSpace(body))
	}
	return body, nil
}

func (c *client) releases() ([]release, error) {
	var all []release
	for page := 1; ; page++ {
		body, err := c.do(http.MethodGet, fmt.Sprintf("/releases?limit=%d&page=%d", pageLimit, page))
		if err != nil {
			return nil, err
		}
		var batch []release
		if err := json.Unmarshal(body, &batch); err != nil {
			return nil, fmt.Errorf("decode releases page %d: %w", page, err)
		}
		all = append(all, batch...)
		if len(batch) < pageLimit {
			return all, nil
		}
	}
}

func (c *client) deleteAsset(releaseID, assetID int64) error {
	_, err := c.do(http.MethodDelete, fmt.Sprintf("/releases/%d/assets/%d", releaseID, assetID))
	return err
}

func mib(n int64) string {
	return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
}

func run(out io.Writer, c *client, keep int, dryRun bool) error {
	if c.repo == "" {
		return errors.New("no repository: pass --repo owner/name")
	}
	if keep < 1 {
		return errors.New("--keep must be at least 1")
	}
	if c.token == "" && !dryRun {
		return errors.New("CODEBERG_RELEASE_TOKEN is not set")
	}

	rels, err := c.releases()
	if err != nil {
		return err
	}
	targets := prunable(rels, keep)
	if len(targets) == 0 {
		fmt.Fprintf(out, "nothing to prune: %d releases, keeping %d\n", len(rels), keep)
		return nil
	}

	var count int
	var freed int64
	for _, r := range targets {
		for _, a := range r.Assets {
			if dryRun {
				fmt.Fprintf(out, "would delete %s %s (%s)\n", r.TagName, a.Name, mib(a.Size))
			} else {
				if err := c.deleteAsset(r.ID, a.ID); err != nil {
					return fmt.Errorf("delete %s %s: %w", r.TagName, a.Name, err)
				}
				fmt.Fprintf(out, "deleted %s %s (%s)\n", r.TagName, a.Name, mib(a.Size))
			}
			count++
			freed += a.Size
		}
	}
	fmt.Fprintf(out, "%d attachments on %d releases, %s\n", count, len(targets), mib(freed))
	return nil
}

func main() {
	repo := flag.String("repo", os.Getenv("CI_REPO"), "owner/name of the forge repository")
	baseURL := flag.String("base-url", "https://codeberg.org", "forge base URL")
	keep := flag.Int("keep", 3, "number of newest releases that keep their attachments")
	dryRun := flag.Bool("dry-run", true, "report what would be deleted without deleting it")
	flag.Parse()

	c := &client{
		base:  strings.TrimSuffix(*baseURL, "/"),
		repo:  *repo,
		token: os.Getenv("CODEBERG_RELEASE_TOKEN"),
		http:  &http.Client{Timeout: 60 * time.Second},
	}
	if err := run(os.Stdout, c, *keep, *dryRun); err != nil {
		fmt.Fprintf(os.Stderr, "release-prune: %v\n", err)
		os.Exit(1)
	}
}
