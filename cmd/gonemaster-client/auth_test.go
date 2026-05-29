package main

import "testing"

func TestNewClientSetsAuthHeader(t *testing.T) {
	c, err := newClient(globalOptions{server: "http://localhost:8080/api/v1", token: "gm_abc"})
	if err != nil {
		t.Fatal(err)
	}
	if got := c.headers.Get("Authorization"); got != "Bearer gm_abc" {
		t.Fatalf("want Bearer gm_abc, got %q", got)
	}
}

func TestNewClientNoTokenNoAuthHeader(t *testing.T) {
	c, err := newClient(globalOptions{server: "http://localhost:8080/api/v1"})
	if err != nil {
		t.Fatal(err)
	}
	if got := c.headers.Get("Authorization"); got != "" {
		t.Fatalf("expected no auth header, got %q", got)
	}
}
