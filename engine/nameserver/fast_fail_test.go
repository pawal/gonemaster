package nameserver

import "testing"

func TestFastFailTrackerThreshold(t *testing.T) {
	tracker := &fastFailTracker{}
	threshold := 3

	tracker.observeResult(false, true, threshold)
	tracker.observeResult(false, true, threshold)
	if tracker.shouldSkip(false, threshold) {
		t.Fatalf("expected no fast-fail skip before threshold")
	}

	tracker.observeResult(false, true, threshold)
	if !tracker.shouldSkip(false, threshold) {
		t.Fatalf("expected fast-fail skip after threshold")
	}
}

func TestFastFailTrackerDisabledWhenThresholdZero(t *testing.T) {
	tracker := &fastFailTracker{}
	tracker.observeResult(false, true, 0)
	if tracker.shouldSkip(false, 0) {
		t.Fatalf("expected fast-fail disabled when threshold is zero")
	}
}

func TestFastFailTrackerResetsOnSuccess(t *testing.T) {
	tracker := &fastFailTracker{}
	threshold := 2

	tracker.observeResult(false, true, threshold)
	tracker.observeResult(false, true, threshold)
	if !tracker.shouldSkip(false, threshold) {
		t.Fatalf("expected fast-fail skip after threshold")
	}

	tracker.observeResult(false, false, threshold)
	if tracker.shouldSkip(false, threshold) {
		t.Fatalf("expected success to clear fast-fail skip state")
	}
}

func TestFastFailTrackerProtocolIsolation(t *testing.T) {
	tracker := &fastFailTracker{}
	threshold := 2

	tracker.observeResult(false, true, threshold)
	tracker.observeResult(false, true, threshold)
	if !tracker.shouldSkip(false, threshold) {
		t.Fatalf("expected UDP skip after threshold")
	}
	if tracker.shouldSkip(true, threshold) {
		t.Fatalf("expected TCP skip state unaffected by UDP failures")
	}
}
