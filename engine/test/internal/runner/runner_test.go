package runner

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"

	"codeberg.org/pawal/gonemaster/engine/logger"
)

func TestRunMergesInOrder(t *testing.T) {
	// The bubble replaces a wall-clock sleep: Wait returns once the other two
	// tasks have logged and task 0 is parked on release.
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()

		release := make(chan struct{})
		tasks := []Task{
			func(_ context.Context, log *logger.Logger) error {
				<-release
				_, _ = log.Add("TASK0", nil, "System", "Case")
				return nil
			},
			func(_ context.Context, log *logger.Logger) error {
				_, _ = log.Add("TASK1", nil, "System", "Case")
				return nil
			},
			func(_ context.Context, log *logger.Logger) error {
				_, _ = log.Add("TASK2", nil, "System", "Case")
				return nil
			},
		}

		resultsCh := make(chan []*logger.Entry, 1)
		go func() {
			entries, _ := Run(ctx, tasks, Options{Parallel: 2, CancelOnError: false})
			resultsCh <- entries
		}()

		synctest.Wait()
		close(release)

		entries := <-resultsCh
		if len(entries) != 3 {
			t.Fatalf("expected 3 entries, got %d", len(entries))
		}
		if entries[0].Tag != "TASK0" || entries[1].Tag != "TASK1" || entries[2].Tag != "TASK2" {
			t.Fatalf("unexpected entry order: %s, %s, %s", entries[0].Tag, entries[1].Tag, entries[2].Tag)
		}
	})
}

func TestRunCancelOnErrorDropsLaterLogs(t *testing.T) {
	ctx := context.Background()

	tasks := []Task{
		func(_ context.Context, log *logger.Logger) error {
			_, _ = log.Add("OK0", nil, "System", "Case")
			return nil
		},
		func(_ context.Context, log *logger.Logger) error {
			_, _ = log.Add("ERR1", nil, "System", "Case")
			return errors.New("boom")
		},
		func(_ context.Context, log *logger.Logger) error {
			_, _ = log.Add("OK2", nil, "System", "Case")
			return nil
		},
	}

	entries, err := Run(ctx, tasks, Options{Parallel: 2, CancelOnError: true})
	if err == nil {
		t.Fatalf("expected error")
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Tag != "OK0" || entries[1].Tag != "ERR1" {
		t.Fatalf("unexpected entry order: %s, %s", entries[0].Tag, entries[1].Tag)
	}
}
