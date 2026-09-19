package server

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
)

// The JobStore contract, run against every implementation. Behaviour only one
// of them has stays in store_test.go and store_sql_test.go.

// ---- Public IDs ------------------------------------------------------------

func TestJobStoreCreateSetsPublicID(t *testing.T) {
	forEachStore(t, func(t *testing.T, s JobStore) {
		created, err := s.Create(Job{ID: "j1", Domain: "example.com", Status: JobQueued, CreatedAt: time.Now().UTC()})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if created.PublicID == "" {
			t.Fatal("expected PublicID to be set after Create")
		}
		got, ok := s.Get("j1")
		if !ok {
			t.Fatal("Get: not found")
		}
		if got.PublicID != created.PublicID {
			t.Fatalf("Get returned PublicID %q, want %q", got.PublicID, created.PublicID)
		}
	})
}

func TestJobStoreCreatePreservesExplicitPublicID(t *testing.T) {
	forEachStore(t, func(t *testing.T, s JobStore) {
		created, err := s.Create(Job{ID: "j1", PublicID: "myid1234", Domain: "example.com", Status: JobQueued, CreatedAt: time.Now().UTC()})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if created.PublicID != "myid1234" {
			t.Fatalf("PublicID: got %q, want %q", created.PublicID, "myid1234")
		}
		got, ok := s.GetByPublicID("myid1234")
		if !ok || got.ID != "j1" {
			t.Fatal("job not reachable by explicit public ID")
		}
	})
}

func TestJobStoreGetByPublicIDReturnsJob(t *testing.T) {
	forEachStore(t, func(t *testing.T, s JobStore) {
		created, err := s.Create(Job{ID: "j1", Domain: "example.com", Status: JobQueued, CreatedAt: time.Now().UTC()})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, ok := s.GetByPublicID(created.PublicID)
		if !ok {
			t.Fatal("expected job to be found by public ID")
		}
		if got.ID != "j1" {
			t.Fatalf("got ID %q, want %q", got.ID, "j1")
		}
	})
}

func TestJobStoreGetByPublicIDMissingReturnsFalse(t *testing.T) {
	forEachStore(t, func(t *testing.T, s JobStore) {
		if _, ok := s.GetByPublicID("notexist"); ok {
			t.Fatal("expected false for unknown public ID")
		}
	})
}

// ---- Domains ---------------------------------------------------------------

func TestJobStoreGetDomain(t *testing.T) {
	forEachStore(t, func(t *testing.T, s JobStore) {
		d, err := s.GetOrCreateDomain("example.com")
		if err != nil {
			t.Fatalf("GetOrCreateDomain: %v", err)
		}

		got, ok := s.GetDomain(d.ID)
		if !ok {
			t.Fatal("GetDomain: not found")
		}
		if got.ID != d.ID || got.Name != "example.com" {
			t.Fatalf("unexpected domain: %+v", got)
		}

		if _, ok := s.GetDomain(9999); ok {
			t.Fatal("expected false for missing id")
		}
	})
}

func TestJobStoreGetDomainByName(t *testing.T) {
	forEachStore(t, func(t *testing.T, s JobStore) {
		if _, err := s.GetOrCreateDomain("alpha.example"); err != nil {
			t.Fatalf("GetOrCreateDomain: %v", err)
		}

		got, ok := s.GetDomainByName("alpha.example")
		if !ok {
			t.Fatal("GetDomainByName: not found")
		}
		if got.Name != "alpha.example" {
			t.Fatalf("unexpected name: %q", got.Name)
		}

		if _, ok := s.GetDomainByName("notexist.example"); ok {
			t.Fatal("expected false for missing name")
		}
	})
}

func TestJobStoreUpdateDomainLatest(t *testing.T) {
	forEachStore(t, func(t *testing.T, s JobStore) {
		d, err := s.GetOrCreateDomain("example.com")
		if err != nil {
			t.Fatalf("GetOrCreateDomain: %v", err)
		}
		if d.RunCount != 0 {
			t.Fatalf("expected RunCount=0, got %d", d.RunCount)
		}

		finishedAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
		if err := s.UpdateDomainLatest(d.ID, "run-1", finishedAt, "succeeded", "ERROR"); err != nil {
			t.Fatalf("UpdateDomainLatest: %v", err)
		}

		got, ok := s.GetDomain(d.ID)
		if !ok {
			t.Fatal("GetDomain after update: not found")
		}
		if got.LatestRunID != "run-1" {
			t.Fatalf("LatestRunID: got %q, want %q", got.LatestRunID, "run-1")
		}
		if got.LatestStatus != "succeeded" {
			t.Fatalf("LatestStatus: got %q, want %q", got.LatestStatus, "succeeded")
		}
		if got.LatestLevel != "ERROR" {
			t.Fatalf("LatestLevel: got %q, want %q", got.LatestLevel, "ERROR")
		}
		if got.RunCount != 1 {
			t.Fatalf("RunCount: got %d, want 1", got.RunCount)
		}
		if !got.LatestRunAt.Equal(finishedAt) {
			t.Fatalf("LatestRunAt: got %v, want %v", got.LatestRunAt, finishedAt)
		}
	})
}

func TestJobStoreGetDomainTags(t *testing.T) {
	forEachStore(t, func(t *testing.T, s JobStore) {
		d, err := s.GetOrCreateDomain("example.com")
		if err != nil {
			t.Fatalf("GetOrCreateDomain: %v", err)
		}

		if tags := s.GetDomainTags(d.ID); len(tags) != 0 {
			t.Fatalf("expected no tags initially, got %v", tags)
		}

		_ = s.TagDomains("x", []int64{d.ID})
		_ = s.TagDomains("y", []int64{d.ID})

		if tags := s.GetDomainTags(d.ID); len(tags) != 2 {
			t.Fatalf("expected 2 tags, got %v", tags)
		}
	})
}

// ---- Tags ------------------------------------------------------------------

func TestJobStoreGetTag(t *testing.T) {
	forEachStore(t, func(t *testing.T, s JobStore) {
		if err := s.CreateTag("alpha", "first tag"); err != nil {
			t.Fatalf("CreateTag: %v", err)
		}
		d, _ := s.GetOrCreateDomain("example.com")
		_ = s.TagDomains("alpha", []int64{d.ID})

		got, ok := s.GetTag("alpha")
		if !ok {
			t.Fatal("GetTag: not found")
		}
		if got.Name != "alpha" || got.Description != "first tag" || got.DomainCount != 1 {
			t.Fatalf("unexpected tag: %+v", got)
		}

		if _, ok := s.GetTag("notexist"); ok {
			t.Fatal("expected false for missing tag")
		}
	})
}

func TestJobStoreUpdateTag(t *testing.T) {
	forEachStore(t, func(t *testing.T, s JobStore) {
		_ = s.CreateTag("beta", "old description")

		if err := s.UpdateTag("beta", "new description"); err != nil {
			t.Fatalf("UpdateTag: %v", err)
		}
		got, ok := s.GetTag("beta")
		if !ok {
			t.Fatal("GetTag after update: not found")
		}
		if got.Description != "new description" {
			t.Fatalf("expected updated description, got %q", got.Description)
		}

		if err := s.UpdateTag("notexist", "x"); err == nil {
			t.Fatal("expected error for missing tag")
		}
	})
}

func TestJobStoreDeleteTag(t *testing.T) {
	forEachStore(t, func(t *testing.T, s JobStore) {
		d, _ := s.GetOrCreateDomain("example.com")
		_ = s.CreateTag("gamma", "")
		_ = s.TagDomains("gamma", []int64{d.ID})

		if err := s.DeleteTag("gamma"); err != nil {
			t.Fatalf("DeleteTag: %v", err)
		}
		if _, ok := s.GetTag("gamma"); ok {
			t.Fatal("expected tag to be deleted")
		}
		// The domain association goes with it.
		for _, tag := range s.GetDomainTags(d.ID) {
			if tag == "gamma" {
				t.Fatal("expected domain_tag association to be removed")
			}
		}

		if err := s.DeleteTag("notexist"); err == nil {
			t.Fatal("expected error for missing tag")
		}
	})
}

func TestJobStoreUntagDomains(t *testing.T) {
	forEachStore(t, func(t *testing.T, s JobStore) {
		d1, _ := s.GetOrCreateDomain("a.example")
		d2, _ := s.GetOrCreateDomain("b.example")
		_ = s.TagDomains("delta", []int64{d1.ID, d2.ID})

		if err := s.UntagDomains("delta", []int64{d1.ID}); err != nil {
			t.Fatalf("UntagDomains: %v", err)
		}

		got, ok := s.GetTag("delta")
		if !ok {
			t.Fatal("GetTag: not found")
		}
		if got.DomainCount != 1 {
			t.Fatalf("expected 1 domain after untag, got %d", got.DomainCount)
		}
		for _, tag := range s.GetDomainTags(d1.ID) {
			if tag == "delta" {
				t.Fatal("expected d1 to be untagged")
			}
		}
		found := false
		for _, tag := range s.GetDomainTags(d2.ID) {
			if tag == "delta" {
				found = true
			}
		}
		if !found {
			t.Fatal("expected d2 to still be tagged")
		}
	})
}

func TestJobStoreListDomainsByTag(t *testing.T) {
	forEachStore(t, func(t *testing.T, s JobStore) {
		d1, _ := s.GetOrCreateDomain("a.example")
		d2, _ := s.GetOrCreateDomain("b.example")
		_, _ = s.GetOrCreateDomain("c.example") // untagged
		_ = s.TagDomains("group", []int64{d1.ID, d2.ID})

		result := s.ListDomainsByTag("group", DomainFilter{Limit: 10})
		if result.Total != 2 {
			t.Fatalf("expected 2 domains in tag, got %d", result.Total)
		}
	})
}

func TestJobStoreGetTagSummary(t *testing.T) {
	forEachStore(t, func(t *testing.T, s JobStore) {
		// The summary counts each tagged domain by its latest run level.
		for _, tc := range []struct {
			domain string
			id     string
			level  string
		}{
			{"a.example", "run-a", "ERROR"},
			{"b.example", "run-b", "WARNING"},
			{"c.example", "run-c", ""},
		} {
			now := time.Now().UTC()
			createAndGraduate(t, s, Job{
				ID:         tc.id,
				Domain:     tc.domain,
				Status:     JobSucceeded,
				CreatedAt:  now,
				FinishedAt: now,
			}, nil)
			d, _ := s.GetDomainByName(tc.domain)
			_ = s.UpdateDomainLatest(d.ID, tc.id, now, "succeeded", tc.level)
		}

		d1, _ := s.GetDomainByName("a.example")
		d2, _ := s.GetDomainByName("b.example")
		d3, _ := s.GetDomainByName("c.example")
		_ = s.TagDomains("summary-tag", []int64{d1.ID, d2.ID, d3.ID})

		summary, ok := s.GetTagSummary("summary-tag")
		if !ok {
			t.Fatal("GetTagSummary: not found")
		}
		if summary.DomainCount != 3 {
			t.Fatalf("DomainCount: got %d, want 3", summary.DomainCount)
		}
		if summary.Error != 1 {
			t.Fatalf("Error: got %d, want 1", summary.Error)
		}
		if summary.Warning != 1 {
			t.Fatalf("Warning: got %d, want 1", summary.Warning)
		}
		if summary.OK != 1 {
			t.Fatalf("OK: got %d, want 1", summary.OK)
		}

		if _, ok := s.GetTagSummary("notexist"); ok {
			t.Fatal("expected false for missing tag")
		}
	})
}

// ---- Runs and entries ------------------------------------------------------

func TestJobStoreGraduateMissingJobReturnsError(t *testing.T) {
	forEachStore(t, func(t *testing.T, s JobStore) {
		err := s.GraduateJob(Job{ID: "ghost", Domain: "example.com", Status: JobSucceeded}, nil)
		if err == nil {
			t.Fatal("expected error for missing job")
		}
	})
}

// ListRuns orders by started_at in both directions on every store.
func TestJobStoreListRunsOrdersByStartedAt(t *testing.T) {
	forEachStore(t, func(t *testing.T, s JobStore) {
		base := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
		// Reversed finishing order exposes a fallback to finished_at.
		for i, id := range []string{"r1", "r2", "r3"} {
			createAndGraduate(t, s, Job{
				ID:         id,
				Domain:     fmt.Sprintf("%s.example", id),
				Status:     JobSucceeded,
				CreatedAt:  base.Add(time.Duration(i) * time.Minute),
				StartedAt:  base.Add(time.Duration(i) * time.Minute),
				FinishedAt: base.Add(time.Duration(10-i) * time.Minute),
			}, nil)
		}

		for _, tc := range []struct {
			sort JobSort
			want []string
		}{
			{JobSortStartedAtAsc, []string{"r1", "r2", "r3"}},
			{JobSortStartedAtDesc, []string{"r3", "r2", "r1"}},
		} {
			list := s.ListRuns(RunFilter{Sort: tc.sort, Limit: 10})
			got := make([]string, len(list.Items))
			for i, run := range list.Items {
				got[i] = run.ID
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("%s: got %v, want %v", tc.sort, got, tc.want)
			}
		}
	})
}

func TestJobStoreListRunsByDomain(t *testing.T) {
	forEachStore(t, func(t *testing.T, s JobStore) {
		base := time.Now().UTC()

		for i, tc := range []struct{ id, domain string }{
			{"r1", "alpha.example"},
			{"r2", "alpha.example"},
			{"r3", "beta.example"},
		} {
			createAndGraduate(t, s, Job{
				ID:         tc.id,
				Domain:     tc.domain,
				Status:     JobSucceeded,
				CreatedAt:  base.Add(time.Duration(i) * time.Second),
				FinishedAt: base.Add(time.Duration(i)*time.Second + time.Minute),
			}, nil)
		}

		d, ok := s.GetDomainByName("alpha.example")
		if !ok {
			t.Fatal("GetDomainByName: not found")
		}

		list := s.ListRunsByDomain(d.ID, 10, 0)
		if list.Total != 2 {
			t.Fatalf("expected 2 runs for alpha.example, got %d", list.Total)
		}
		for _, r := range list.Items {
			if r.Domain != "alpha.example" {
				t.Fatalf("expected only alpha.example runs, got %q", r.Domain)
			}
		}

		page := s.ListRunsByDomain(d.ID, 1, 0)
		if page.Total != 2 || len(page.Items) != 1 {
			t.Fatalf("pagination: total=%d items=%d", page.Total, len(page.Items))
		}

		empty := s.ListRunsByDomain(9999, 10, 0)
		if empty.Total != 0 {
			t.Fatalf("expected 0 for unknown domain, got %d", empty.Total)
		}
	})
}

func TestJobStorePriorityPersistedOnJob(t *testing.T) {
	forEachStore(t, func(t *testing.T, s JobStore) {
		now := time.Now().UTC().Truncate(time.Microsecond)
		job := Job{ID: "pj1", Domain: "example.com", Status: JobQueued, CreatedAt: now, Priority: PriorityBatch}
		if _, err := s.Create(job); err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, ok := s.Get(job.ID)
		if !ok {
			t.Fatal("Get: not found")
		}
		if got.Priority != PriorityBatch {
			t.Fatalf("Priority: got %d, want %d", got.Priority, PriorityBatch)
		}
	})
}

func TestJobStorePriorityPersistedOnRun(t *testing.T) {
	forEachStore(t, func(t *testing.T, s JobStore) {
		now := time.Now().UTC().Truncate(time.Microsecond)
		job := Job{
			ID: "pj2", Domain: "example.com", Status: JobSucceeded,
			CreatedAt: now, FinishedAt: now, Priority: PriorityBatch,
		}
		createAndGraduate(t, s, job, nil)

		run, ok := s.GetRun(job.ID)
		if !ok {
			t.Fatal("GetRun: not found")
		}
		if run.Priority != PriorityBatch {
			t.Fatalf("run Priority: got %d, want %d", run.Priority, PriorityBatch)
		}
		// The job view reconstructed from the run keeps it too.
		reconstructed, ok := s.Get(job.ID)
		if !ok {
			t.Fatal("Get after graduation: not found")
		}
		if reconstructed.Priority != PriorityBatch {
			t.Fatalf("reconstructed Priority: got %d, want %d", reconstructed.Priority, PriorityBatch)
		}
	})
}

func TestJobStoreQueryEntries(t *testing.T) {
	forEachStore(t, func(t *testing.T, s JobStore) {
		base := time.Now().UTC()

		grad := func(id, domain, batchID string, entries []engine.LogEntry) {
			t.Helper()
			createAndGraduate(t, s, Job{
				ID:        id,
				Domain:    domain,
				BatchID:   batchID,
				Status:    JobSucceeded,
				CreatedAt: base,
			}, entries)
		}

		grad("run1", "alpha.example", "batch1", []engine.LogEntry{
			{Module: "DNSSEC", Testcase: "DNSSEC01", Tag: "DS01_ALGO_SHA1", Level: "WARNING"},
			{Module: "DNSSEC", Testcase: "DNSSEC02", Tag: "DS02_NO_DS", Level: "ERROR"},
		})
		grad("run2", "beta.example", "batch1", []engine.LogEntry{
			{Module: "DNSSEC", Testcase: "DNSSEC01", Tag: "DS01_ALGO_SHA1", Level: "NOTICE"},
		})
		grad("run3", "gamma.example", "", []engine.LogEntry{
			{Module: "BASIC", Testcase: "BASIC01", Tag: "BASIC01_NO_GLUE", Level: "ERROR"},
		})

		betaDomain, _ := s.GetDomainByName("beta.example")
		gammaDomain, _ := s.GetDomainByName("gamma.example")
		if err := s.CreateTag("tagged", ""); err != nil {
			t.Fatalf("CreateTag: %v", err)
		}
		if err := s.TagDomains("tagged", []int64{betaDomain.ID, gammaDomain.ID}); err != nil {
			t.Fatalf("TagDomains: %v", err)
		}

		for _, tc := range []struct {
			name   string
			filter EntryFilter
			want   int
		}{
			{name: "no filter returns all", filter: EntryFilter{}, want: 4},
			{name: "filter by run_id", filter: EntryFilter{RunID: "run1"}, want: 2},
			{name: "filter by module", filter: EntryFilter{Module: "DNSSEC"}, want: 3},
			{name: "filter by testcase", filter: EntryFilter{Testcase: "DNSSEC01"}, want: 2},
			{name: "filter by entry_tag", filter: EntryFilter{EntryTag: "DS02_NO_DS"}, want: 1},
			{name: "filter by level", filter: EntryFilter{Level: "ERROR"}, want: 2},
			// beta and gamma carry one entry each.
			{name: "filter by domain tag", filter: EntryFilter{Tag: "tagged"}, want: 2},
			{name: "filter by batch_id", filter: EntryFilter{BatchID: "batch1"}, want: 3},
		} {
			t.Run(tc.name, func(t *testing.T) {
				if list := s.QueryEntries(tc.filter); list.Total != tc.want {
					t.Fatalf("Total = %d, want %d", list.Total, tc.want)
				}
			})
		}

		t.Run("latest only", func(t *testing.T) {
			// A later alpha run wins latest-only, leaving it and gamma.
			grad("run1b", "alpha.example", "", []engine.LogEntry{
				{Module: "BASIC", Testcase: "BASIC01", Tag: "B", Level: "NOTICE"},
			})
			list := s.QueryEntries(EntryFilter{LatestOnly: true, Module: "BASIC"})
			if list.Total != 2 {
				t.Fatalf("expected 2 (latest-only BASIC entries), got %d", list.Total)
			}
		})

		t.Run("pagination", func(t *testing.T) {
			list := s.QueryEntries(EntryFilter{Limit: 2, Offset: 0})
			if len(list.Items) != 2 {
				t.Fatalf("expected 2 items on page 1, got %d", len(list.Items))
			}
			if list.NextCursor == "" {
				t.Fatal("expected NextCursor to be set")
			}
		})
	})
}

// ---- Profiles and settings -------------------------------------------------

func TestJobStoreProfileCRUD(t *testing.T) {
	forEachStore(t, func(t *testing.T, s JobStore) {
		now := time.Now().UTC().Truncate(time.Microsecond)

		created, err := s.CreateProfile(StoredProfile{
			Name:        "default",
			Description: "Default profile",
			Config:      `{"test_cases":["ALL"]}`,
			Public:      true,
			CreatedAt:   now,
			UpdatedAt:   now,
		})
		if err != nil {
			t.Fatalf("CreateProfile: %v", err)
		}
		if created.ID == 0 {
			t.Fatal("expected non-zero ID after create")
		}
		if created.Name != "default" {
			t.Fatalf("Name: got %q", created.Name)
		}

		got, ok := s.GetProfile(created.ID)
		if !ok {
			t.Fatal("GetProfile: not found")
		}
		if got.Name != "default" {
			t.Fatalf("Name: got %q", got.Name)
		}
		if got.Description != "Default profile" {
			t.Fatalf("Description: got %q", got.Description)
		}
		if got.Config != `{"test_cases":["ALL"]}` {
			t.Fatalf("Config: got %q", got.Config)
		}
		if !got.Public {
			t.Fatal("expected Public=true")
		}
		if !got.CreatedAt.Equal(now) {
			t.Fatalf("CreatedAt: got %v, want %v", got.CreatedAt, now)
		}

		byName, ok := s.GetProfileByName("default")
		if !ok {
			t.Fatal("GetProfileByName: not found")
		}
		if byName.ID != created.ID {
			t.Fatalf("GetProfileByName ID: got %d, want %d", byName.ID, created.ID)
		}

		if _, ok := s.GetProfile(9999); ok {
			t.Fatal("expected ok=false for missing profile ID")
		}
		if _, ok := s.GetProfileByName("nonexistent"); ok {
			t.Fatal("expected ok=false for missing profile name")
		}

		got.Description = "Updated description"
		got.Public = false
		got.UpdatedAt = now.Add(time.Hour)
		if err := s.UpdateProfile(got); err != nil {
			t.Fatalf("UpdateProfile: %v", err)
		}
		updated, ok := s.GetProfile(got.ID)
		if !ok {
			t.Fatal("GetProfile after update: not found")
		}
		if updated.Description != "Updated description" {
			t.Fatalf("Description after update: got %q", updated.Description)
		}
		if updated.Public {
			t.Fatal("expected Public=false after update")
		}

		if _, err := s.CreateProfile(StoredProfile{Name: "default", Config: "{}", CreatedAt: now, UpdatedAt: now}); err == nil {
			t.Fatal("expected error on duplicate name create")
		}

		// Timestamps the caller leaves unset are filled in on create.
		second, err := s.CreateProfile(StoredProfile{Name: "strict", Config: "{}"})
		if err != nil {
			t.Fatalf("CreateProfile(strict): %v", err)
		}
		if second.CreatedAt.IsZero() || second.UpdatedAt.IsZero() {
			t.Fatalf("expected timestamps to be set, got %+v", second)
		}

		second.Name = "default"
		if err := s.UpdateProfile(second); err == nil {
			t.Fatal("expected error on duplicate name update")
		}

		profiles := s.ListProfiles()
		if len(profiles) != 2 {
			t.Fatalf("ListProfiles: got %d, want 2", len(profiles))
		}
		if profiles[0].Name != "default" || profiles[1].Name != "strict" {
			t.Fatalf("ListProfiles order: got [%q, %q]", profiles[0].Name, profiles[1].Name)
		}

		if err := s.DeleteProfile(created.ID); err != nil {
			t.Fatalf("DeleteProfile: %v", err)
		}
		if _, ok := s.GetProfile(created.ID); ok {
			t.Fatal("expected profile to be deleted")
		}
		if profiles = s.ListProfiles(); len(profiles) != 1 {
			t.Fatalf("ListProfiles after delete: got %d, want 1", len(profiles))
		}

		if err := s.DeleteProfile(9999); err == nil {
			t.Fatal("expected error on deleting non-existent profile")
		}
		if err := s.UpdateProfile(StoredProfile{ID: 9999, Name: "gone", Config: "{}"}); err == nil {
			t.Fatal("expected error on updating non-existent profile")
		}
	})
}

func TestJobStoreProfileReferencesPersist(t *testing.T) {
	forEachStore(t, func(t *testing.T, s JobStore) {
		profile, err := s.CreateProfile(StoredProfile{
			Name:   "strict",
			Config: `{"resolver.defaults.timeout":10}`,
		})
		if err != nil {
			t.Fatalf("CreateProfile: %v", err)
		}
		if err := s.CreateTag("ops", "operations"); err != nil {
			t.Fatalf("CreateTag: %v", err)
		}
		if err := s.SetTagDefaultProfile("ops", &profile.ID); err != nil {
			t.Fatalf("SetTagDefaultProfile: %v", err)
		}

		now := time.Now().UTC().Truncate(time.Microsecond)
		queued := Job{
			ID:               "job-profile-queued",
			Domain:           "queued.example",
			Status:           JobQueued,
			CreatedAt:        now,
			ProfileID:        &profile.ID,
			ProfileName:      profile.Name,
			EffectiveProfile: `{"resolver.defaults.timeout":10}`,
		}
		if _, err := s.Create(queued); err != nil {
			t.Fatalf("Create queued: %v", err)
		}
		gotQueued, ok := s.Get(queued.ID)
		if !ok {
			t.Fatal("Get queued: not found")
		}
		if gotQueued.ProfileID == nil || *gotQueued.ProfileID != profile.ID {
			t.Fatalf("queued ProfileID: got %v, want %d", gotQueued.ProfileID, profile.ID)
		}
		if gotQueued.ProfileName != profile.Name {
			t.Fatalf("queued ProfileName: got %q, want %q", gotQueued.ProfileName, profile.Name)
		}

		running := Job{
			ID:               "job-profile-run",
			Domain:           "run.example",
			Status:           JobSucceeded,
			CreatedAt:        now,
			StartedAt:        now,
			FinishedAt:       now.Add(2 * time.Second),
			ProfileID:        &profile.ID,
			ProfileName:      profile.Name,
			EffectiveProfile: `{"resolver.defaults.timeout":15}`,
		}
		createAndGraduate(t, s, running, nil)

		run, ok := s.GetRun(running.ID)
		if !ok {
			t.Fatal("GetRun: not found")
		}
		if run.ProfileID == nil || *run.ProfileID != profile.ID {
			t.Fatalf("run ProfileID: got %v, want %d", run.ProfileID, profile.ID)
		}
		if run.ProfileName != profile.Name {
			t.Fatalf("run ProfileName: got %q, want %q", run.ProfileName, profile.Name)
		}
		if run.EffectiveProfile != `{"resolver.defaults.timeout":15}` {
			t.Fatalf("run EffectiveProfile: got %q", run.EffectiveProfile)
		}

		// Deleting the profile clears the references but keeps the names.
		if err := s.DeleteProfile(profile.ID); err != nil {
			t.Fatalf("DeleteProfile: %v", err)
		}

		tag, ok := s.GetTag("ops")
		if !ok {
			t.Fatal("GetTag after delete: not found")
		}
		if tag.DefaultProfileID != nil {
			t.Fatalf("expected cleared tag DefaultProfileID, got %v", *tag.DefaultProfileID)
		}

		gotQueued, ok = s.Get(queued.ID)
		if !ok {
			t.Fatal("Get queued after delete: not found")
		}
		if gotQueued.ProfileID != nil {
			t.Fatalf("expected queued ProfileID cleared, got %v", *gotQueued.ProfileID)
		}
		if gotQueued.ProfileName != profile.Name {
			t.Fatalf("queued ProfileName after delete: got %q, want %q", gotQueued.ProfileName, profile.Name)
		}

		run, ok = s.GetRun(running.ID)
		if !ok {
			t.Fatal("GetRun after delete: not found")
		}
		if run.ProfileID != nil {
			t.Fatalf("expected run ProfileID cleared, got %v", *run.ProfileID)
		}
		if run.ProfileName != profile.Name {
			t.Fatalf("run ProfileName after delete: got %q, want %q", run.ProfileName, profile.Name)
		}
		if run.EffectiveProfile != `{"resolver.defaults.timeout":15}` {
			t.Fatalf("run EffectiveProfile after delete: got %q", run.EffectiveProfile)
		}
	})
}

func TestJobStoreSetTagDefaultProfile(t *testing.T) {
	forEachStore(t, func(t *testing.T, s JobStore) {
		if err := s.CreateTag("beta", "profiled tag"); err != nil {
			t.Fatalf("CreateTag: %v", err)
		}
		profile, err := s.CreateProfile(StoredProfile{Name: "default", Config: "{}"})
		if err != nil {
			t.Fatalf("CreateProfile: %v", err)
		}

		if err := s.SetTagDefaultProfile("beta", &profile.ID); err != nil {
			t.Fatalf("SetTagDefaultProfile: %v", err)
		}
		tag, ok := s.GetTag("beta")
		if !ok {
			t.Fatal("GetTag: not found")
		}
		if tag.DefaultProfileID == nil || *tag.DefaultProfileID != profile.ID {
			t.Fatalf("DefaultProfileID: got %v, want %d", tag.DefaultProfileID, profile.ID)
		}

		tags := s.ListTags(10, 0)
		if len(tags) != 1 {
			t.Fatalf("ListTags: got %d, want 1", len(tags))
		}
		if tags[0].DefaultProfileID == nil || *tags[0].DefaultProfileID != profile.ID {
			t.Fatalf("ListTags DefaultProfileID: got %v, want %d", tags[0].DefaultProfileID, profile.ID)
		}

		if err := s.SetTagDefaultProfile("beta", nil); err != nil {
			t.Fatalf("clear SetTagDefaultProfile: %v", err)
		}
		tag, _ = s.GetTag("beta")
		if tag.DefaultProfileID != nil {
			t.Fatalf("expected cleared DefaultProfileID, got %v", *tag.DefaultProfileID)
		}

		missingID := profile.ID + 1000
		if err := s.SetTagDefaultProfile("beta", &missingID); err == nil {
			t.Fatal("expected error for missing profile")
		}
		if err := s.SetTagDefaultProfile("missing-tag", &profile.ID); err == nil {
			t.Fatal("expected error for missing tag")
		}
	})
}

func TestJobStoreSettingsCRUD(t *testing.T) {
	forEachStore(t, func(t *testing.T, s JobStore) {
		if _, ok := s.GetSetting("worker_count"); ok {
			t.Fatal("expected ok=false for missing setting")
		}

		if err := s.SetSetting("worker_count", "8"); err != nil {
			t.Fatalf("SetSetting: %v", err)
		}
		v, ok := s.GetSetting("worker_count")
		if !ok {
			t.Fatal("expected setting to exist")
		}
		if v != "8" {
			t.Fatalf("got %q, want %q", v, "8")
		}

		// Setting the same key again upserts.
		if err := s.SetSetting("worker_count", "12"); err != nil {
			t.Fatalf("SetSetting overwrite: %v", err)
		}
		v, _ = s.GetSetting("worker_count")
		if v != "12" {
			t.Fatalf("got %q after overwrite, want %q", v, "12")
		}

		if err := s.SetSetting("min_level", "WARNING"); err != nil {
			t.Fatalf("SetSetting min_level: %v", err)
		}

		all := s.ListSettings()
		if len(all) != 2 {
			t.Fatalf("ListSettings: got %d, want 2", len(all))
		}
		if all["worker_count"] != "12" || all["min_level"] != "WARNING" {
			t.Fatalf("ListSettings: unexpected values: %v", all)
		}

		if err := s.DeleteSetting("worker_count"); err != nil {
			t.Fatalf("DeleteSetting: %v", err)
		}
		if _, ok := s.GetSetting("worker_count"); ok {
			t.Fatal("expected setting deleted")
		}
		if err := s.DeleteSetting("nonexistent"); err == nil {
			t.Fatal("expected error on deleting missing setting")
		}
		if all = s.ListSettings(); len(all) != 1 {
			t.Fatalf("ListSettings after delete: got %d, want 1", len(all))
		}
	})
}

// ---- Purge -----------------------------------------------------------------

func TestJobStorePurgeDeletesTerminalRuns(t *testing.T) {
	forEachStore(t, func(t *testing.T, s JobStore) {
		cutoff := time.Now().UTC()
		old := cutoff.Add(-24 * time.Hour)

		ids := []string{"s1", "f1", "c1", "e1"}
		for i, status := range []JobStatus{JobSucceeded, JobFailed, JobCanceled, JobExpired} {
			createAndGraduate(t, s, Job{
				ID: ids[i], Domain: "example.com", Status: status, CreatedAt: old, FinishedAt: old,
			}, nil)
		}

		n, err := s.PurgeOlderThan(cutoff)
		if err != nil {
			t.Fatalf("purge: %v", err)
		}
		if n != 4 {
			t.Fatalf("expected 4 purged, got %d", n)
		}
		for _, id := range ids {
			if _, ok := s.GetRun(id); ok {
				t.Fatalf("expected run %s to be deleted after purge", id)
			}
		}
	})
}

func TestJobStorePurgePreservesActiveJobs(t *testing.T) {
	forEachStore(t, func(t *testing.T, s JobStore) {
		cutoff := time.Now().UTC()
		old := cutoff.Add(-24 * time.Hour)

		for i, status := range []JobStatus{JobQueued, JobRunning, JobPaused} {
			id := fmt.Sprintf("j%d", i)
			if _, err := s.Create(Job{ID: id, Domain: "example.com", Status: status, CreatedAt: old}); err != nil {
				t.Fatalf("create %s: %v", id, err)
			}
		}

		n, err := s.PurgeOlderThan(cutoff)
		if err != nil {
			t.Fatalf("purge: %v", err)
		}
		if n != 0 {
			t.Fatalf("expected 0 purged, got %d", n)
		}
		if list := s.List(JobFilter{Limit: 100}); list.Total != 3 {
			t.Fatalf("expected 3 jobs preserved, got %d", list.Total)
		}
	})
}

func TestJobStorePurgePreservesNewRuns(t *testing.T) {
	forEachStore(t, func(t *testing.T, s JobStore) {
		cutoff := time.Now().UTC()
		recent := cutoff.Add(time.Hour)

		createAndGraduate(t, s, Job{
			ID: "s1", Domain: "example.com", Status: JobSucceeded, CreatedAt: recent, FinishedAt: recent,
		}, nil)

		n, err := s.PurgeOlderThan(cutoff)
		if err != nil {
			t.Fatalf("purge: %v", err)
		}
		if n != 0 {
			t.Fatalf("expected 0 purged, got %d", n)
		}
	})
}

func TestJobStorePurgeDeletesEntries(t *testing.T) {
	forEachStore(t, func(t *testing.T, s JobStore) {
		cutoff := time.Now().UTC()
		old := cutoff.Add(-24 * time.Hour)

		createAndGraduate(t, s, Job{
			ID: "s1", Domain: "example.com", Status: JobSucceeded, CreatedAt: old, FinishedAt: old,
		}, []engine.LogEntry{{Module: "DNS", Tag: "TAG", Level: "NOTICE", Timestamp: 1.0}})

		if _, err := s.PurgeOlderThan(cutoff); err != nil {
			t.Fatalf("purge: %v", err)
		}
		if _, ok := s.GetResult("s1"); ok {
			t.Fatal("expected result to be deleted after purge")
		}
	})
}

func TestJobStorePurgeRemovesPublicIDIndex(t *testing.T) {
	forEachStore(t, func(t *testing.T, s JobStore) {
		cutoff := time.Now().UTC()
		old := cutoff.Add(-48 * time.Hour)

		created := createAndGraduate(t, s, Job{
			ID: "j1", Domain: "example.com", Status: JobSucceeded, CreatedAt: old, FinishedAt: old,
		}, nil)

		if _, err := s.PurgeOlderThan(cutoff); err != nil {
			t.Fatalf("purge: %v", err)
		}
		if _, ok := s.GetByPublicID(created.PublicID); ok {
			t.Fatal("expected public ID lookup to fail after purge")
		}
	})
}

func TestJobStorePurgeReturnsZeroWhenNothingMatches(t *testing.T) {
	forEachStore(t, func(t *testing.T, s JobStore) {
		n, err := s.PurgeOlderThan(time.Now().UTC())
		if err != nil {
			t.Fatalf("purge: %v", err)
		}
		if n != 0 {
			t.Fatalf("expected 0 on empty store, got %d", n)
		}
	})
}

func TestJobStorePurgeByTagDeletesTaggedRunsAndEntries(t *testing.T) {
	forEachStore(t, func(t *testing.T, s JobStore) {
		now := time.Now().UTC()

		// Two tagged domains plus one untagged domain, all graduated.
		for _, name := range []string{"se", "dk", "other.example"} {
			createAndGraduate(t, s, Job{
				ID: name, Domain: name, Status: JobSucceeded, CreatedAt: now, FinishedAt: now,
			}, []engine.LogEntry{{Module: "DNSSEC", Tag: "OK", Level: "INFO", Timestamp: 1.0}})
		}
		if err := s.CreateTag("tld", ""); err != nil {
			t.Fatalf("CreateTag: %v", err)
		}
		for _, name := range []string{"se", "dk"} {
			d, ok := s.GetDomainByName(name)
			if !ok {
				t.Fatalf("domain %s not found", name)
			}
			if err := s.TagDomains("tld", []int64{d.ID}); err != nil {
				t.Fatalf("TagDomains %s: %v", name, err)
			}
		}

		n, err := s.PurgeByTag("tld")
		if err != nil {
			t.Fatalf("purge by tag: %v", err)
		}
		if n != 2 {
			t.Fatalf("expected 2 purged, got %d", n)
		}
		for _, id := range []string{"se", "dk"} {
			if _, ok := s.GetRun(id); ok {
				t.Fatalf("expected run %s to be deleted", id)
			}
			if _, ok := s.GetResult(id); ok {
				t.Fatalf("expected entries for %s to be deleted", id)
			}
		}
		if _, ok := s.GetRun("other.example"); !ok {
			t.Fatal("expected untagged run to survive purge by tag")
		}
	})
}

func TestJobStorePurgeByTagPreservesActiveJobs(t *testing.T) {
	forEachStore(t, func(t *testing.T, s JobStore) {
		d, err := s.GetOrCreateDomain("se")
		if err != nil {
			t.Fatalf("GetOrCreateDomain: %v", err)
		}
		if _, err := s.Create(Job{ID: "q1", Domain: "se", Status: JobQueued, CreatedAt: time.Now().UTC()}); err != nil {
			t.Fatalf("create: %v", err)
		}
		if err := s.CreateTag("tld", ""); err != nil {
			t.Fatalf("CreateTag: %v", err)
		}
		if err := s.TagDomains("tld", []int64{d.ID}); err != nil {
			t.Fatalf("TagDomains: %v", err)
		}

		n, err := s.PurgeByTag("tld")
		if err != nil {
			t.Fatalf("purge by tag: %v", err)
		}
		if n != 0 {
			t.Fatalf("expected 0 purged, got %d", n)
		}
		if list := s.List(JobFilter{Limit: 100}); list.Total != 1 {
			t.Fatalf("expected queued job preserved, got %d", list.Total)
		}
	})
}

func TestJobStorePurgeByTagReturnsZeroForUnknownTag(t *testing.T) {
	forEachStore(t, func(t *testing.T, s JobStore) {
		n, err := s.PurgeByTag("nope")
		if err != nil {
			t.Fatalf("purge by tag: %v", err)
		}
		if n != 0 {
			t.Fatalf("expected 0 for unknown tag, got %d", n)
		}
	})
}
