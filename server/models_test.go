package server

import (
	"encoding/json"
	"testing"
)

func TestJobPublicIDFieldPresent(t *testing.T) {
	j := Job{ID: "abc", PublicID: "x1y2z3w4", Domain: "example.com", Status: JobQueued}
	if j.PublicID != "x1y2z3w4" {
		t.Fatalf("got %q, want %q", j.PublicID, "x1y2z3w4")
	}
}

func TestJobPublicIDSerializesWhenSet(t *testing.T) {
	j := Job{ID: "abc", PublicID: "x1y2z3w4", Domain: "example.com", Status: JobQueued}
	b, err := json.Marshal(j)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["public_id"] != "x1y2z3w4" {
		t.Fatalf("public_id not serialized correctly: %v", m["public_id"])
	}
}

func TestJobPublicIDOmittedWhenEmpty(t *testing.T) {
	j := Job{ID: "abc", Domain: "example.com", Status: JobQueued}
	b, err := json.Marshal(j)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["public_id"]; ok {
		t.Fatal("public_id should be omitted when empty")
	}
}

func TestJobPublicIDRoundTrip(t *testing.T) {
	j := Job{ID: "abc", PublicID: "x1y2z3w4", Domain: "example.com", Status: JobQueued}
	b, err := json.Marshal(j)
	if err != nil {
		t.Fatal(err)
	}
	var j2 Job
	if err := json.Unmarshal(b, &j2); err != nil {
		t.Fatal(err)
	}
	if j2.PublicID != j.PublicID {
		t.Fatalf("round-trip: got %q, want %q", j2.PublicID, j.PublicID)
	}
}
