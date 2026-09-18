package analysis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"codeberg.org/pawal/gonemaster/engine/profile"
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

// scoringConfigSetting is the settings key holding a stored scoring
// configuration override.
const scoringConfigSetting = "scoring_config"

// scoringConfigDefault marks a capture taken with no stored override.
const scoringConfigDefault = "default"

// runVocabulary returns the run's test_levels table as canonical JSON, or ""
// when the run carries no readable effective profile. Read from the run, not
// from the running build, so capture and backfill agree.
func runVocabulary(run serverpkg.Run) string {
	if strings.TrimSpace(run.EffectiveProfile) == "" {
		return ""
	}
	p, err := profile.FromJSON(run.EffectiveProfile)
	if err != nil || len(p.TestLevels) == 0 {
		return ""
	}
	raw, err := json.Marshal(p.TestLevels)
	if err != nil {
		return ""
	}
	return string(raw)
}

// scoringConfigIdentity hashes the stored scoring configuration so a score
// move that came from a penalty change can be told from one that came from
// the findings. It covers the database setting only: a configuration passed
// by CLI flag or config file reads as the default.
func (c *Controller) scoringConfigIdentity() string {
	raw, ok := c.store.GetSetting(scoringConfigSetting)
	if !ok || strings.TrimSpace(raw) == "" {
		return scoringConfigDefault
	}
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// vocabularyProbeRuns caps how many runs the backfill reads before giving
// up on a batch. One run suffices unless its effective profile is missing.
const vocabularyProbeRuns = 5

// batchVocabulary returns the first readable vocabulary among a batch's
// runs. Mixed-profile batches fail capture, so any run is representative.
func (c *Controller) batchVocabulary(batchID string) string {
	runs := c.store.ListRuns(serverpkg.RunFilter{BatchID: batchID, Limit: vocabularyProbeRuns})
	for _, run := range runs.Items {
		if v := runVocabulary(run); v != "" {
			return v
		}
	}
	return ""
}

// BackfillSnapshotVocabularies stamps the tag vocabulary onto snapshots
// captured before it was recorded. Idempotent: already-stamped snapshots are
// skipped, and a snapshot whose runs are gone stays unknown rather than
// guessed at from the running build. The scoring configuration hash is not
// backfilled: what was in force at capture is not recoverable afterwards.
func (c *Controller) BackfillSnapshotVocabularies(ctx context.Context) error {
	for _, cohort := range c.store.ListAnalysisCohorts() {
		if err := ctx.Err(); err != nil {
			return err
		}
		for _, snap := range c.store.ListAnalysisCohortSnapshots(cohort.ID) {
			if err := ctx.Err(); err != nil {
				return err
			}
			if snap.Vocabulary != "" {
				continue
			}
			vocabulary := c.batchVocabulary(snap.BatchID)
			if vocabulary == "" {
				continue
			}
			snap.Vocabulary = vocabulary
			if _, err := c.store.UpsertAnalysisCohortSnapshot(snap); err != nil {
				return fmt.Errorf("backfill vocabulary for snapshot %d: %w", snap.ID, err)
			}
		}
	}
	return nil
}
