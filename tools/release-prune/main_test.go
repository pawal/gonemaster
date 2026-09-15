package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseVersion(t *testing.T) {
	cases := []struct {
		tag  string
		want bool
	}{
		{"v1.7.9", true},
		{"v10.0.12", true},
		{"1.7.9", false},
		{"v1.7", false},
		{"v1.7.9-rc1", false},
		{"vnext", false},
		{"v1.7.x", false},
	}
	for _, c := range cases {
		if _, ok := parseVersion(c.tag); ok != c.want {
			t.Errorf("parseVersion(%q) = %v, want %v", c.tag, ok, c.want)
		}
	}
}

func TestParseVersionOrdersByNumber(t *testing.T) {
	older, _ := parseVersion("v1.7.9")
	newer, _ := parseVersion("v1.10.0")
	if compareVersion(older, newer) >= 0 {
		t.Fatalf("v1.7.9 must sort below v1.10.0")
	}
}

// One attachment, so a release counts as carrying some.
func rel(id int64, tag string) release {
	return release{ID: id, TagName: tag, Assets: []asset{{ID: id * 10, Name: tag + ".tar.gz", Size: 1 << 20}}}
}

func tags(rels []release) []string {
	out := make([]string, len(rels))
	for i, r := range rels {
		out[i] = r.TagName
	}
	return out
}

func TestPrunableKeepsNewest(t *testing.T) {
	rels := []release{rel(1, "v1.7.7"), rel(2, "v1.8.0"), rel(3, "v1.7.9"), rel(4, "v1.7.6"), rel(5, "v1.7.8")}

	got := tags(prunable(rels, 3))
	want := []string{"v1.7.7", "v1.7.6"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("prunable = %v, want %v", got, want)
	}
}

func TestPrunableSkipsUnversionedAndEmpty(t *testing.T) {
	rels := []release{
		rel(1, "v1.8.0"), rel(2, "v1.7.9"), rel(3, "v1.7.8"),
		rel(4, "nightly"),
		{ID: 5, TagName: "v1.7.7"},
		rel(6, "v1.7.6"),
	}

	got := tags(prunable(rels, 3))
	want := []string{"v1.7.6"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("prunable = %v, want %v", got, want)
	}
}

func TestPrunableFewerReleasesThanKeep(t *testing.T) {
	if got := prunable([]release{rel(1, "v1.8.0")}, 3); len(got) != 0 {
		t.Fatalf("prunable = %v, want none", tags(got))
	}
}

// forge serves one page of releases and records the assets deleted.
func forge(t *testing.T, rels []release) (*httptest.Server, *[]string) {
	t.Helper()
	var deleted []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/repos/owner/name/releases":
			if r.URL.Query().Get("page") != "1" {
				fmt.Fprint(w, "[]")
				return
			}
			if err := json.NewEncoder(w).Encode(rels); err != nil {
				t.Errorf("encode releases: %v", err)
			}
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/assets/"):
			deleted = append(deleted, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &deleted
}

func TestRunDeletesSupersededAttachments(t *testing.T) {
	rels := []release{rel(1, "v1.8.0"), rel(2, "v1.7.9"), rel(3, "v1.7.8"), rel(4, "v1.7.7")}
	srv, deleted := forge(t, rels)
	c := &client{base: srv.URL, repo: "owner/name", token: "t", http: srv.Client()}

	var out bytes.Buffer
	if err := run(&out, c, 3, false); err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(*deleted) != 1 || (*deleted)[0] != "/api/v1/repos/owner/name/releases/4/assets/40" {
		t.Fatalf("deleted = %v", *deleted)
	}
	if !strings.Contains(out.String(), "deleted v1.7.7 v1.7.7.tar.gz") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestRunDryRunDeletesNothing(t *testing.T) {
	rels := []release{rel(1, "v1.8.0"), rel(2, "v1.7.9"), rel(3, "v1.7.8"), rel(4, "v1.7.7")}
	srv, deleted := forge(t, rels)
	c := &client{base: srv.URL, repo: "owner/name", http: srv.Client()}

	var out bytes.Buffer
	if err := run(&out, c, 3, true); err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(*deleted) != 0 {
		t.Fatalf("deleted = %v, want none", *deleted)
	}
	if !strings.Contains(out.String(), "would delete v1.7.7") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestRunRejectsBadArguments(t *testing.T) {
	srv, _ := forge(t, nil)

	cases := []struct {
		name   string
		c      *client
		keep   int
		dryRun bool
	}{
		{"no repo", &client{base: srv.URL, token: "t", http: srv.Client()}, 3, false},
		{"keep zero", &client{base: srv.URL, repo: "owner/name", token: "t", http: srv.Client()}, 0, false},
		{"no token", &client{base: srv.URL, repo: "owner/name", http: srv.Client()}, 3, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := run(&bytes.Buffer{}, c.c, c.keep, c.dryRun); err == nil {
				t.Fatal("want an error")
			}
		})
	}
}

func TestRunReportsForgeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"message":"token required"}`)
	}))
	t.Cleanup(srv.Close)
	c := &client{base: srv.URL, repo: "owner/name", token: "t", http: srv.Client()}

	err := run(&bytes.Buffer{}, c, 3, false)
	if err == nil || !strings.Contains(err.Error(), "token required") {
		t.Fatalf("err = %v, want the forge message", err)
	}
}
