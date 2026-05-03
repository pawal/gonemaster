package server

import (
	"encoding/json"
	"regexp"
	"testing"
)

var base62RE = regexp.MustCompile(`^[a-zA-Z0-9]{8}$`)

func TestGeneratePublicIDLength(t *testing.T) {
	id := GeneratePublicID()
	if len(id) != 8 {
		t.Fatalf("got length %d, want 8", len(id))
	}
}

func TestGeneratePublicIDBase62Chars(t *testing.T) {
	for range 100 {
		id := GeneratePublicID()
		if !base62RE.MatchString(id) {
			t.Fatalf("id %q contains non-base62 characters", id)
		}
	}
}

func TestGeneratePublicIDUnique(t *testing.T) {
	seen := make(map[string]struct{}, 1000)
	for range 1000 {
		id := GeneratePublicID()
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate public ID generated: %q", id)
		}
		seen[id] = struct{}{}
	}
}

func TestJobPublicIDFieldPresent(t *testing.T) {
	j := Job{PublicID: "x1y2z3w4"}
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

func TestJobPriorityValues(t *testing.T) {
	if PriorityNormal != 0 {
		t.Fatalf("PriorityNormal = %d, want 0", PriorityNormal)
	}
	if PriorityBatch != 1 {
		t.Fatalf("PriorityBatch = %d, want 1", PriorityBatch)
	}
	if PriorityNormal >= PriorityBatch {
		t.Fatal("PriorityNormal must be less than PriorityBatch so normal jobs are dequeued first")
	}
}

func TestJobPriorityIsInt(t *testing.T) {
	var p JobPriority = PriorityNormal
	if int(p) != 0 {
		t.Fatalf("int(PriorityNormal) = %d, want 0", int(p))
	}
}

func TestJobPriorityFieldPresent(t *testing.T) {
	j := Job{Priority: PriorityBatch}
	if j.Priority != PriorityBatch {
		t.Fatalf("got %d, want %d", j.Priority, PriorityBatch)
	}
}

func TestJobPrioritySerializesAsNumber(t *testing.T) {
	for _, tc := range []struct {
		priority JobPriority
		want     float64
	}{
		{PriorityNormal, 0},
		{PriorityBatch, 1},
	} {
		j := Job{ID: "a", Domain: "example.com", Status: JobQueued, Priority: tc.priority}
		b, err := json.Marshal(j)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		got, ok := m["priority"]
		if !ok {
			t.Fatal("priority field missing from JSON")
		}
		if got != tc.want {
			t.Fatalf("priority: got %v, want %v", got, tc.want)
		}
	}
}

func TestRunPriorityFieldPresent(t *testing.T) {
	r := Run{Priority: PriorityBatch}
	if r.Priority != PriorityBatch {
		t.Fatalf("got %d, want %d", r.Priority, PriorityBatch)
	}
}

func TestRunPrioritySerializesAsNumber(t *testing.T) {
	r := Run{ID: "abc", Domain: "example.com", Status: JobSucceeded, Priority: PriorityNormal}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["priority"] != float64(0) {
		t.Fatalf("priority: got %v, want 0", m["priority"])
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
