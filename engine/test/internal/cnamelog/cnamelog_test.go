package cnamelog

import (
	"context"
	"errors"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/recursor"
)

func TestLogIgnoresNilError(t *testing.T) {
	ctx := logger.WithContext(context.Background(), logger.New())
	var results []*logger.Entry
	Log(ctx, &results, "Basic", "Basic01", nil)
	if len(results) != 0 {
		t.Fatalf("expected no entries appended, got %d", len(results))
	}
}

func TestLogIgnoresNonCNAMEError(t *testing.T) {
	ctx := logger.WithContext(context.Background(), logger.New())
	var results []*logger.Entry
	Log(ctx, &results, "Basic", "Basic01", errors.New("unrelated"))
	if len(results) != 0 {
		t.Fatalf("expected no entries appended, got %d", len(results))
	}
}

func TestLogEmitsTooManyTag(t *testing.T) {
	ctx := logger.WithContext(context.Background(), logger.New())
	var results []*logger.Entry
	err := &recursor.CNAMEError{Reason: recursor.CNAMETooMany, Name: "ns1.example."}
	Log(ctx, &results, "Basic", "Basic01", err)
	if len(results) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(results))
	}
	if results[0].Tag != "CNAME_TOO_MANY_RECORDS" {
		t.Fatalf("expected CNAME_TOO_MANY_RECORDS, got %q", results[0].Tag)
	}
	if results[0].Args["query_name"] != "ns1.example." {
		t.Fatalf("expected query_name=ns1.example., got %#v", results[0].Args["query_name"])
	}
}

func TestLogEmitsChainTooLongTag(t *testing.T) {
	ctx := logger.WithContext(context.Background(), logger.New())
	var results []*logger.Entry
	err := &recursor.CNAMEError{Reason: recursor.CNAMEChainTooLong, Name: "ns2.example."}
	Log(ctx, &results, "Basic", "Basic01", err)
	if len(results) != 1 || results[0].Tag != "CNAME_CHAIN_TOO_LONG" {
		t.Fatalf("expected CNAME_CHAIN_TOO_LONG, got %#v", results)
	}
	if results[0].Args["query_name"] != "ns2.example." {
		t.Fatalf("expected query_name=ns2.example., got %#v", results[0].Args["query_name"])
	}
}

func TestLogEmitsUnresolvedTagWithBothArgs(t *testing.T) {
	ctx := logger.WithContext(context.Background(), logger.New())
	var results []*logger.Entry
	err := &recursor.CNAMEError{Reason: recursor.CNAMEUnresolved, Name: "ns3.example.", Target: "alias.example.", Detail: "loop"}
	Log(ctx, &results, "Basic", "Basic01", err)
	if len(results) != 1 || results[0].Tag != "CNAME_TARGET_UNRESOLVED" {
		t.Fatalf("expected CNAME_TARGET_UNRESOLVED, got %#v", results)
	}
	args := results[0].Args
	if args["query_name"] != "ns3.example." {
		t.Fatalf("expected query_name=ns3.example., got %#v", args["query_name"])
	}
	if args["cname_target"] != "alias.example." {
		t.Fatalf("expected cname_target=alias.example., got %#v", args["cname_target"])
	}
}
