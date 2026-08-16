package analysis

import (
	"context"
	"fmt"
	"sort"
	"strings"

	serverpkg "codeberg.org/pawal/gonemaster/server"
)

// Provenance is read from the run's own GLOBAL_VERSION entry, not the
// running binary, so capture and backfill agree.
const (
	globalVersionTag = "GLOBAL_VERSION"
	globalVersionArg = "version"
)

// runEngineVersion returns the version recorded in a run's entries, or ""
// when unknown. Never falls back to the running build.
func runEngineVersion(entries []serverpkg.Entry) string {
	for _, entry := range entries {
		if entry.Tag != globalVersionTag {
			continue
		}
		if v := stringArg(entry.Args, globalVersionArg); v != "" {
			return v
		}
	}
	return ""
}

// mixedEngineVersions reports whether two observed versions disagree. An
// unknown side is a gap, not a conflict.
func mixedEngineVersions(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	return a != b
}

// batchEngineVersions returns the distinct engine versions observed across a
// batch's runs, sorted for determinism. Empty when no run recorded one.
func (c *Controller) batchEngineVersions(batchID string) []string {
	list := c.store.QueryEntries(serverpkg.EntryFilter{
		BatchID:  batchID,
		EntryTag: globalVersionTag,
	})
	seen := map[string]struct{}{}
	for _, entry := range list.Items {
		if v := stringArg(entry.Args, globalVersionArg); v != "" {
			seen[v] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for v := range seen {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// BackfillSnapshotEngineVersions stamps provenance onto snapshots captured
// before it was recorded. Idempotent: already-stamped snapshots are skipped,
// and a snapshot whose runs carry no version is left unknown rather than
// guessed at.
func (c *Controller) BackfillSnapshotEngineVersions(ctx context.Context) error {
	for _, cohort := range c.store.ListAnalysisCohorts() {
		if err := ctx.Err(); err != nil {
			return err
		}
		for _, snap := range c.store.ListAnalysisCohortSnapshots(cohort.ID) {
			if err := ctx.Err(); err != nil {
				return err
			}
			if snap.EngineVersion != "" {
				continue
			}
			versions := c.batchEngineVersions(snap.BatchID)
			if len(versions) == 0 {
				continue
			}
			// The sample is the lowest by string order, chosen only so the
			// value is stable across reruns; the mixed flag is the signal.
			snap.EngineVersion = versions[0]
			if len(versions) > 1 {
				snap.MixedEngineVersion = true
			}
			if _, err := c.store.UpsertAnalysisCohortSnapshot(snap); err != nil {
				return fmt.Errorf("backfill engine version for snapshot %d: %w", snap.ID, err)
			}
		}
	}
	return nil
}
