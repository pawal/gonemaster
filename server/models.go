package server

import (
	"crypto/rand"
	"encoding/json"
	"math/big"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/scoring"
)

const publicIDAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
const publicIDLen = 8

// GeneratePublicID returns an 8-character base62 string using crypto/rand.
func GeneratePublicID() string {
	b := make([]byte, publicIDLen)
	alphabetLen := big.NewInt(int64(len(publicIDAlphabet)))
	for i := range b {
		n, err := rand.Int(rand.Reader, alphabetLen)
		if err != nil {
			panic("crypto/rand unavailable: " + err.Error())
		}
		b[i] = publicIDAlphabet[n.Int64()]
	}
	return string(b)
}

// JobPriority controls queue ordering. Normal jobs are always dequeued before
// batch jobs. Within each tier, FIFO order is preserved.
type JobPriority int

const (
	PriorityNormal JobPriority = 0 // interactive: single-domain submit
	PriorityBatch  JobPriority = 1 // background: batch / tag sweep
)

// JobStatus describes the current state of a job.
type JobStatus string

const (
	JobQueued    JobStatus = "queued"
	JobRunning   JobStatus = "running"
	JobSucceeded JobStatus = "succeeded"
	JobFailed    JobStatus = "failed"
	JobCanceled  JobStatus = "canceled"
	JobExpired   JobStatus = "expired"
	JobPaused    JobStatus = "paused"
)

// JobSort controls ordering in list responses.
type JobSort string

const (
	JobSortCreatedAtDesc  JobSort = "created_at_desc"
	JobSortCreatedAtAsc   JobSort = "created_at_asc"
	JobSortStartedAtDesc  JobSort = "started_at_desc"
	JobSortStartedAtAsc   JobSort = "started_at_asc"
	JobSortDomainAsc      JobSort = "domain_asc"
	JobSortDomainDesc     JobSort = "domain_desc"
	JobSortBatchIDAsc     JobSort = "batch_id_asc"
	JobSortBatchIDDesc    JobSort = "batch_id_desc"
	JobSortErrorDesc      JobSort = "error_desc"
	JobSortCriticalDesc   JobSort = "critical_desc"
	JobSortFinishedAtDesc JobSort = "finished_at_desc"
	JobSortFinishedAtAsc  JobSort = "finished_at_asc"
	JobSortWorstLevelDesc JobSort = "worst_level_desc"
	JobSortWorstLevelAsc  JobSort = "worst_level_asc"
	JobSortScoreDesc      JobSort = "score_desc"
	JobSortScoreAsc       JobSort = "score_asc"
	JobSortDurationDesc   JobSort = "duration_desc"
	JobSortDurationAsc    JobSort = "duration_asc"
	JobSortEntryCountDesc JobSort = "entry_count_desc"
	JobSortEntryCountAsc  JobSort = "entry_count_asc"
)

// JobSeverityFilter controls severity-based list filtering.
type JobSeverityFilter string

const (
	JobSeverityWarningsPlus JobSeverityFilter = "warnings_plus"
	JobSeverityErrorsOnly   JobSeverityFilter = "errors_only"
)

// Job represents a single test job (queue entry).
// Once completed, the job graduates to a Run + Entries.
// FinishedAt and SeverityTotals are populated from the associated Run when
// the job is terminal; they are not stored in the jobs table itself.
type Job struct {
	ID       string    `json:"id"`
	PublicID string    `json:"public_id,omitempty"`
	BatchID  string    `json:"batch_id,omitempty"`
	DomainID int64     `json:"-"`
	Domain   string    `json:"domain"`
	Status   JobStatus `json:"status"`
	// FinishedAt is populated from the Run after graduation; zero for in-flight jobs.
	FinishedAt       time.Time      `json:"finished_at,omitempty"`
	SeverityTotals   map[string]int `json:"severity_totals,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	StartedAt        time.Time      `json:"started_at,omitempty"`
	Priority         JobPriority    `json:"priority"`
	Progress         int            `json:"progress"`
	Error            string         `json:"error,omitempty"`
	Profile          string         `json:"-"`
	ProfileID        *int64         `json:"profile_id,omitempty"`
	ProfileName      string         `json:"profile_name,omitempty"`
	EffectiveProfile string         `json:"-"`
	// Score and Grade are copied from the Run after graduation.
	Score *int    `json:"score,omitempty"`
	Grade *string `json:"grade,omitempty"`
	// Config fields stored as config_json in the DB.
	Tests         []string                       `json:"-"`
	Overrides     map[string]any                 `json:"-"`
	UndelegatedNS []engine.UndelegatedNameserver `json:"-"`
	UndelegatedDS []engine.UndelegatedDSInfo     `json:"-"`
	MinLevel      string                         `json:"-"`
	IPv4Disabled  bool                           `json:"-"`
	IPv6Disabled  bool                           `json:"-"`
	// NameserverTimings carries per-run timing summaries into graduation.
	NameserverTimings []NameserverTiming `json:"-"`
}

// NameserverTiming holds timing stats for one tested authoritative nameserver.
type NameserverTiming struct {
	Nameserver string  `json:"nameserver"`
	Address    string  `json:"address"`
	AvgMS      float64 `json:"avg_ms"`
	MinMS      float64 `json:"min_ms"`
	MaxMS      float64 `json:"max_ms"`
	MedianMS   float64 `json:"median_ms"`
	StddevMS   float64 `json:"stddev_ms"`
	Count      int     `json:"count"`
	// Empty on rows written before this field existed; treat as "ok".
	Status string `json:"status,omitempty"`
}

// Status values for NameserverTiming.
const (
	NameserverTimingStatusOK          = "ok"
	NameserverTimingStatusUnreachable = "unreachable"
	NameserverTimingStatusUnresolved  = "unresolved"
)

// Domain is a persistent domain registry entry.
type Domain struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	LatestRunID  string    `json:"latest_run_id,omitempty"`
	LatestRunAt  time.Time `json:"latest_run_at,omitempty"`
	LatestStatus string    `json:"latest_status,omitempty"`
	LatestLevel  string    `json:"latest_level,omitempty"`
	LatestScore  *int      `json:"latest_score,omitempty"`
	LatestGrade  *string   `json:"latest_grade,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	RunCount     int       `json:"run_count"`
	Tags         []string  `json:"tags,omitempty"`
}

// Tag is a named domain collection.
type Tag struct {
	Name             string    `json:"name"`
	Description      string    `json:"description,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	DomainCount      int       `json:"domain_count"`
	DefaultProfileID *int64    `json:"default_profile_id,omitempty"`
}

// TagSummary holds per-severity and per-grade domain counts for a tag.
type TagSummary struct {
	Tag         string         `json:"tag"`
	DomainCount int            `json:"domain_count"`
	OK          int            `json:"ok"`
	Notice      int            `json:"notice"`
	Warning     int            `json:"warning"`
	Error       int            `json:"error"`
	Critical    int            `json:"critical"`
	Grades      map[string]int `json:"grades,omitempty"`
}

const (
	AnalysisMaterializationPending = "pending"
	AnalysisMaterializationReady   = "ready"
	AnalysisMaterializationFailed  = "failed"
)

// Snapshot lifecycle states for analysis_cohort_snapshots.status.
const (
	AnalysisSnapshotStatusPending             = "pending"
	AnalysisSnapshotStatusCaptured            = "captured"
	AnalysisSnapshotStatusRetired             = "retired"
	AnalysisSnapshotStatusFailedMixedProfiles = "failed_mixed_profiles"
)

// Per-cohort default snapshot resolution policies.
const (
	DefaultSnapshotPolicyAutoLatest = "auto_latest"
	DefaultSnapshotPolicyPinned     = "pinned"
)

// AnalysisCohort describes one admin-managed analysis cohort entry.
// V1 cohorts are tag-backed, analysis-enabled/public-enabled independently,
// and one public cohort may be marked as the default.
type AnalysisCohort struct {
	ID                       int64     `json:"id"`
	SourceType               string    `json:"source_type"`
	SourceTag                string    `json:"source_tag"`
	Label                    string    `json:"label"`
	Description              string    `json:"description,omitempty"`
	AnalysisEnabled          bool      `json:"analysis_enabled"`
	PublicEnabled            bool      `json:"public_enabled"`
	IsDefault                bool      `json:"is_default"`
	SortOrder                int       `json:"sort_order"`
	MaterializationStatus    string    `json:"materialization_status"`
	MaterializationDone      int       `json:"materialization_done,omitempty"`
	MaterializationTotal     int       `json:"materialization_total,omitempty"`
	LastMaterializedAt       time.Time `json:"last_materialized_at,omitempty"`
	LastMaterializationError string    `json:"last_materialization_error,omitempty"`
	DefaultSnapshotPolicy    string    `json:"default_snapshot_policy,omitempty"`
	DefaultSnapshotID        *int64    `json:"default_snapshot_id,omitempty"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

// MarshalJSON omits LastMaterializedAt when it is the zero time.
// Without this override encoding/json serializes a zero time.Time as
// "0001-01-01T00:00:00Z" regardless of the `omitempty` tag, which leaks into
// the admin UI as a bogus "1/1/1" timestamp.
func (c AnalysisCohort) MarshalJSON() ([]byte, error) {
	type alias AnalysisCohort
	aux := struct {
		*alias
		LastMaterializedAt *time.Time `json:"last_materialized_at,omitempty"`
	}{alias: (*alias)(&c)}
	if !c.LastMaterializedAt.IsZero() {
		t := c.LastMaterializedAt
		aux.LastMaterializedAt = &t
	}
	return json.Marshal(aux)
}

// AnalysisNameserver is one normalized nameserver hostname in the analysis layer.
type AnalysisNameserver struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	FirstSeenAt time.Time `json:"first_seen_at"`
	LastSeenAt  time.Time `json:"last_seen_at"`
}

// AnalysisAddress is one normalized IP address in the analysis layer.
type AnalysisAddress struct {
	ID          int64     `json:"id"`
	Address     string    `json:"address"`
	Family      string    `json:"family"`
	FirstSeenAt time.Time `json:"first_seen_at"`
	LastSeenAt  time.Time `json:"last_seen_at"`
}

// AnalysisPrefix is one normalized announced prefix in the analysis layer.
type AnalysisPrefix struct {
	ID          int64     `json:"id"`
	Prefix      string    `json:"prefix"`
	Family      string    `json:"family"`
	FirstSeenAt time.Time `json:"first_seen_at"`
	LastSeenAt  time.Time `json:"last_seen_at"`
}

// AnalysisASN is one normalized ASN in the analysis layer.
type AnalysisASN struct {
	ASN         int64     `json:"asn"`
	Label       string    `json:"label,omitempty"`
	FirstSeenAt time.Time `json:"first_seen_at"`
	LastSeenAt  time.Time `json:"last_seen_at"`
}

// AnalysisRunNameserverEndpoint is one materialized nameserver/address row for a run+cohort.
type AnalysisRunNameserverEndpoint struct {
	CohortID     int64   `json:"cohort_id"`
	RunID        string  `json:"run_id"`
	DomainID     int64   `json:"domain_id"`
	NameserverID int64   `json:"nameserver_id"`
	AddressID    int64   `json:"address_id"`
	Role         string  `json:"role"`
	Source       string  `json:"source"`
	Family       string  `json:"family"`
	AvgMS        float64 `json:"avg_ms,omitempty"`
	MinMS        float64 `json:"min_ms,omitempty"`
	MaxMS        float64 `json:"max_ms,omitempty"`
	QueryCount   int     `json:"query_count"`
}

// AnalysisRunDomainASN is an aggregate (domain, ASN) row derived from the
// per-family ASN sets gonemaster logs expose (e.g. IPV4_DIFFERENT_ASN). The
// engine does not pair ASNs with specific addresses, so this table carries
// the domain-level ASN footprint without an address column.
type AnalysisRunDomainASN struct {
	CohortID int64  `json:"cohort_id"`
	RunID    string `json:"run_id"`
	DomainID int64  `json:"domain_id"`
	ASN      int64  `json:"asn"`
	Family   string `json:"family,omitempty"`
	Source   string `json:"source,omitempty"`
}

// AnalysisRunAddressASN is one materialized address/prefix/ASN row for a run+cohort.
type AnalysisRunAddressASN struct {
	CohortID     int64  `json:"cohort_id"`
	RunID        string `json:"run_id"`
	DomainID     int64  `json:"domain_id"`
	AddressID    int64  `json:"address_id"`
	PrefixID     *int64 `json:"prefix_id,omitempty"`
	ASN          *int64 `json:"asn,omitempty"`
	LookupStatus string `json:"lookup_status,omitempty"`
	Source       string `json:"source,omitempty"`
}

// AnalysisRunDomainSummary is one materialized summary row for a run+cohort.
type AnalysisRunDomainSummary struct {
	CohortID        int64   `json:"cohort_id"`
	RunID           string  `json:"run_id"`
	DomainID        int64   `json:"domain_id"`
	Score           *int    `json:"score,omitempty"`
	Grade           *string `json:"grade,omitempty"`
	NameserverCount int     `json:"nameserver_count"`
	EndpointCount   int     `json:"endpoint_count"`
	ASNCount        int     `json:"asn_count"`
	PrefixCount     int     `json:"prefix_count"`
	WorstLevel      string  `json:"worst_level,omitempty"`
}

// AnalysisRunTagSummary is one materialized (tag, testcase) aggregate for a
// run+cohort. Counts entries emitted with that tag so the landing page's
// "top findings" panel can be served from one cached SELECT instead of
// issuing a per-domain QueryEntries scan on every request.
type AnalysisRunTagSummary struct {
	CohortID        int64  `json:"cohort_id"`
	RunID           string `json:"run_id"`
	DomainID        int64  `json:"domain_id"`
	Tag             string `json:"tag"`
	Module          string `json:"module,omitempty"`
	Testcase        string `json:"testcase,omitempty"`
	Level           string `json:"level,omitempty"`
	OccurrenceCount int    `json:"occurrence_count"`
}

// AnalysisRunDomainFact is one (category, key) fact observed for a domain in
// one run+cohort. Categories are small, distribution-shaped statistics such
// as DNSKEY algorithm or signed/unsigned posture; the registry in
// analysis_fact_categories.go defines their display metadata.
type AnalysisRunDomainFact struct {
	CohortID int64  `json:"cohort_id"`
	RunID    string `json:"run_id"`
	DomainID int64  `json:"domain_id"`
	Category string `json:"category"`
	Key      string `json:"key"`
	ValueNum *int64 `json:"value_num,omitempty"`
}

// AnalysisProjectionState tracks projection status for one run+cohort pair.
type AnalysisProjectionState struct {
	CohortID         int64     `json:"cohort_id"`
	RunID            string    `json:"run_id"`
	ProjectorVersion string    `json:"projector_version"`
	Status           string    `json:"status"`
	ProjectedAt      time.Time `json:"projected_at,omitempty"`
	Error            string    `json:"error,omitempty"`
}

// Run is a completed execution, graduated from a Job.
type Run struct {
	ID               string      `json:"id"`
	DomainID         int64       `json:"domain_id"`
	Domain           string      `json:"domain"`
	BatchID          string      `json:"batch_id,omitempty"`
	Status           JobStatus   `json:"status"`
	CreatedAt        time.Time   `json:"created_at"`
	StartedAt        time.Time   `json:"started_at,omitempty"`
	FinishedAt       time.Time   `json:"finished_at,omitempty"`
	DurationMs       int64       `json:"duration_ms,omitempty"`
	SevNotice        int         `json:"sev_notice"`
	SevWarning       int         `json:"sev_warning"`
	SevError         int         `json:"sev_error"`
	SevCritical      int         `json:"sev_critical"`
	WorstLevel       string      `json:"worst_level,omitempty"`
	EntryCount       int         `json:"entry_count"`
	Priority         JobPriority `json:"priority"`
	Profile          string      `json:"profile,omitempty"`
	ProfileID        *int64      `json:"profile_id,omitempty"`
	ProfileName      string      `json:"profile_name,omitempty"`
	EffectiveProfile string      `json:"effective_profile,omitempty"`
	PublicID         string      `json:"public_id,omitempty"`
	// SeverityTotals mirrors the sev_* columns as a map for API compat.
	SeverityTotals map[string]int `json:"severity_totals,omitempty"`
	// Score and Grade are computed by the scoring engine at graduation and
	// cached in the runs table. Nil when scoring has not yet been computed.
	Score             *int               `json:"score,omitempty"`
	Grade             *string            `json:"grade,omitempty"`
	NameserverTimings []NameserverTiming `json:"nameserver_timings,omitempty"`
}

// Entry is a single engine log entry stored as a row for SQL analysis.
type Entry struct {
	ID        int64          `json:"id"`
	RunID     string         `json:"run_id"`
	DomainID  int64          `json:"domain_id"`
	Domain    string         `json:"domain,omitempty"`
	Timestamp float64        `json:"timestamp"`
	Module    string         `json:"module"`
	Testcase  string         `json:"testcase"`
	Tag       string         `json:"tag"`
	Level     string         `json:"level"`
	Args      map[string]any `json:"args,omitempty"`
}

// Batch is metadata for a batch submission.
type Batch struct {
	ID             string    `json:"id"`
	Tag            string    `json:"tag,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	DomainCount    int       `json:"domain_count"`
	Description    string    `json:"description,omitempty"`
	SnapshotIntent bool      `json:"snapshot_intent,omitempty"`
}

// AnalysisCohortSnapshot is one point-in-time materialization of a cohort,
// backed by exactly one snapshot-intent batch. Snapshots are identified by
// (cohort_id, batch_id); the slug is a separate human-readable handle that
// is unique within a cohort.
type AnalysisCohortSnapshot struct {
	ID          int64     `json:"id"`
	CohortID    int64     `json:"cohort_id"`
	BatchID     string    `json:"batch_id"`
	Slug        string    `json:"slug"`
	Label       string    `json:"label,omitempty"`
	Description string    `json:"description,omitempty"`
	ProfileID   *int64    `json:"profile_id,omitempty"`
	ProfileName string    `json:"profile_name,omitempty"`
	CapturedAt  time.Time `json:"captured_at,omitempty"`
	FirstRunAt  time.Time `json:"first_run_at,omitempty"`
	LastRunAt   time.Time `json:"last_run_at,omitempty"`
	RunCount    int       `json:"run_count"`
	DomainCount int       `json:"domain_count"`
	Status      string    `json:"status"`
	IsDefault   bool      `json:"is_default"`
	IsPublic    bool      `json:"is_public"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// AnalysisCohortSnapshotAggregate is one pre-computed aggregate row
// attached to a captured snapshot. The payload shape is category-specific.
type AnalysisCohortSnapshotAggregate struct {
	SnapshotID  int64     `json:"snapshot_id"`
	Category    string    `json:"category"`
	PayloadJSON string    `json:"payload_json"`
	ComputedAt  time.Time `json:"computed_at"`
}

// AnalysisSnapshotNameserverView is one pre-computed nameserver row for a
// captured snapshot, ready to serve the Nameservers tab without scanning facts.
type AnalysisSnapshotNameserverView struct {
	SnapshotID     int64  `json:"snapshot_id"`
	NameserverID   int64  `json:"nameserver_id"`
	NameserverName string `json:"nameserver_name"`
	DomainCount    int    `json:"domain_count"`
	EndpointCount  int    `json:"endpoint_count"`
	IPv4Count      int    `json:"ipv4_count"`
	IPv6Count      int    `json:"ipv6_count"`
	ASNCount       int    `json:"asn_count"`
	Operator       string `json:"operator,omitempty"`
	OperatorASN    *int64 `json:"operator_asn,omitempty"`
	QueryCount     int    `json:"query_count,omitempty"`
}

// AnalysisSnapshotEndpointView is one pre-computed (nameserver, address) row
// for a captured snapshot.
type AnalysisSnapshotEndpointView struct {
	SnapshotID     int64  `json:"snapshot_id"`
	NameserverID   int64  `json:"nameserver_id"`
	AddressID      int64  `json:"address_id"`
	NameserverName string `json:"nameserver_name"`
	Address        string `json:"address"`
	Family         string `json:"family"`
	DomainCount    int    `json:"domain_count"`
	ASN            *int64 `json:"asn,omitempty"`
	ASNLabel       string `json:"asn_label,omitempty"`
	Prefix         string `json:"prefix,omitempty"`
}

// AnalysisSnapshotASNView is one pre-computed ASN row for a captured snapshot.
type AnalysisSnapshotASNView struct {
	SnapshotID      int64  `json:"snapshot_id"`
	ASN             int64  `json:"asn"`
	Label           string `json:"label,omitempty"`
	DomainCount     int    `json:"domain_count"`
	AddressCount    int    `json:"address_count"`
	NameserverCount int    `json:"nameserver_count"`
	PrefixCount     int    `json:"prefix_count"`
	IPv4Count       int    `json:"ipv4_count"`
	IPv6Count       int    `json:"ipv6_count"`
}

// StoredProfile is a named, server-stored test configuration.
type StoredProfile struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	Config        string    `json:"config"` // JSON, same schema as profile_overrides
	Public        bool      `json:"public"` // visible in public UI dropdown
	SchemaVersion string    `json:"schema_version"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Profile is the API representation of a stored profile.
type Profile struct {
	ID            int64          `json:"id"`
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	Config        map[string]any `json:"config"`
	Public        bool           `json:"public"`
	SchemaVersion string         `json:"schema_version"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

// ── Filter types ──────────────────────────────────────────────────────────────

// JobFilter controls listing behavior.
type JobFilter struct {
	Status        JobStatus
	BatchID       string
	Domain        string
	Severity      JobSeverityFilter
	CreatedAfter  time.Time
	CreatedBefore time.Time
	Limit         int
	Offset        int
	Sort          JobSort
}

// DomainSort controls ordering for domain list queries.
type DomainSort string

const (
	DomainSortNameAsc      DomainSort = "name_asc"
	DomainSortNameDesc     DomainSort = "name_desc"
	DomainSortLevelDesc    DomainSort = "latest_level_desc"
	DomainSortLevelAsc     DomainSort = "latest_level_asc"
	DomainSortScoreDesc    DomainSort = "latest_score_desc"
	DomainSortScoreAsc     DomainSort = "latest_score_asc"
	DomainSortLastRunDesc  DomainSort = "latest_run_at_desc"
	DomainSortLastRunAsc   DomainSort = "latest_run_at_asc"
	DomainSortRunCountDesc DomainSort = "run_count_desc"
	DomainSortRunCountAsc  DomainSort = "run_count_asc"
)

// DomainFilter filters domain list queries.
type DomainFilter struct {
	Tag         string
	Name        string
	LatestLevel string
	MinLevel    string // minimum severity threshold (inclusive); "WARNING" matches WARNING/ERROR/CRITICAL
	Limit       int
	Offset      int
	Sort        DomainSort
}

// RunFilter filters run list queries.
type RunFilter struct {
	DomainID       int64
	Domain         string
	BatchID        string
	Tag            string
	Status         JobStatus
	WorstLevel     string
	Grade          string
	FinishedAfter  time.Time
	FinishedBefore time.Time
	Limit          int
	Offset         int
	Sort           JobSort
}

// EntryFilter filters cross-run entry queries.
type EntryFilter struct {
	RunID      string // exact run ID
	DomainID   int64  // exact domain ID
	Tag        string // domain tag filter (join via domain_tags)
	Module     string // exact module name
	Testcase   string // exact testcase name
	EntryTag   string // log event tag (entries.tag column)
	Level      string // exact level
	LatestOnly bool   // restrict to entries from each domain's latest run
	BatchID    string // restrict to runs from a batch
	Limit      int
	Offset     int
}

// ── List result types ─────────────────────────────────────────────────────────

// JobList represents paginated job results.
type JobList struct {
	Items      []Job  `json:"items"`
	Total      int    `json:"total"`
	Limit      int    `json:"limit,omitempty"`
	Offset     int    `json:"offset,omitempty"`
	NextCursor string `json:"next_cursor,omitempty"`
	PrevCursor string `json:"prev_cursor,omitempty"`
	Sort       string `json:"sort,omitempty"`
}

// DomainList represents paginated domain results.
type DomainList struct {
	Items      []Domain `json:"items"`
	Total      int      `json:"total"`
	Limit      int      `json:"limit,omitempty"`
	Offset     int      `json:"offset,omitempty"`
	NextCursor string   `json:"next_cursor,omitempty"`
	PrevCursor string   `json:"prev_cursor,omitempty"`
}

// RunList represents paginated run results.
type RunList struct {
	Items      []Run  `json:"items"`
	Total      int    `json:"total"`
	Limit      int    `json:"limit,omitempty"`
	Offset     int    `json:"offset,omitempty"`
	NextCursor string `json:"next_cursor,omitempty"`
	PrevCursor string `json:"prev_cursor,omitempty"`
}

// EntryList represents paginated entry results.
type EntryList struct {
	Items      []Entry `json:"items"`
	Total      int     `json:"total"`
	Limit      int     `json:"limit,omitempty"`
	Offset     int     `json:"offset,omitempty"`
	NextCursor string  `json:"next_cursor,omitempty"`
	PrevCursor string  `json:"prev_cursor,omitempty"`
}

// BatchList represents paginated batch results.
type BatchList struct {
	Items  []Batch `json:"items"`
	Total  int     `json:"total"`
	Limit  int     `json:"limit,omitempty"`
	Offset int     `json:"offset,omitempty"`
}

// BatchDeletePreviewSnapshot names one cohort snapshot that will vanish
// if the batch is deleted.
type BatchDeletePreviewSnapshot struct {
	CohortID      int64  `json:"cohort_id"`
	CohortLabel   string `json:"cohort_label,omitempty"`
	SnapshotSlug  string `json:"snapshot_slug"`
	SnapshotLabel string `json:"snapshot_label,omitempty"`
	IsDefault     bool   `json:"is_default,omitempty"`
}

// BatchDeletePreview is the impact summary rendered in the admin
// confirmation modal before a batch deletion is confirmed.
type BatchDeletePreview struct {
	BatchID        string                       `json:"batch_id"`
	Tag            string                       `json:"tag,omitempty"`
	CreatedAt      time.Time                    `json:"created_at,omitempty"`
	SnapshotIntent bool                         `json:"snapshot_intent,omitempty"`
	Exists         bool                         `json:"exists"`
	QueuedJobs     int                          `json:"queued_jobs"`
	RunningJobs    int                          `json:"running_jobs"`
	CompletedRuns  int                          `json:"completed_runs"`
	Entries        int                          `json:"entries"`
	FactRows       int                          `json:"fact_rows"`
	Snapshots      []BatchDeletePreviewSnapshot `json:"snapshots,omitempty"`
}

// ── Result types (unchanged shape for API compat) ────────────────────────────

// JobResult holds the assembled output for a job/run.
type JobResult struct {
	JobID                string             `json:"job_id"`
	BatchID              string             `json:"batch_id,omitempty"`
	Status               JobStatus          `json:"status"`
	Summary              map[string]any     `json:"summary,omitempty"`
	NameserverTimings    []NameserverTiming `json:"nameserver_timings,omitempty"`
	Raw                  *JobResultRaw      `json:"raw,omitempty"`
	TestcaseDescriptions map[string]string  `json:"testcase_descriptions,omitempty"`
	// Score holds the full scoring result. Populated by GetResult; nil when
	// the run has no entries or scoring is not available.
	Score *scoring.Result `json:"score,omitempty"`
}

// JobResultRaw contains the raw log entries for a job.
type JobResultRaw struct {
	Locale  string           `json:"locale,omitempty"`
	Entries []JobResultEntry `json:"entries,omitempty"`
}

// JobResultEntry is a single log entry for API consumers.
type JobResultEntry struct {
	Timestamp float64        `json:"timestamp"`
	Module    string         `json:"module"`
	Testcase  string         `json:"testcase"`
	Tag       string         `json:"tag"`
	Level     string         `json:"level"`
	Args      map[string]any `json:"args,omitempty"`
	Message   string         `json:"message,omitempty"`
	Raw       string         `json:"raw,omitempty"`
}

// ── Request/response types ────────────────────────────────────────────────────

// JobCreateRequest is the payload for a single job.
type JobCreateRequest struct {
	Domain           string                       `json:"domain"`
	Tests            []string                     `json:"tests,omitempty"`
	ProfileID        *int64                       `json:"profile_id,omitempty"`
	ProfileOverrides map[string]any               `json:"profile_overrides,omitempty"`
	Nameservers      []UndelegatedNameserverInput `json:"nameservers,omitempty"`
	DSInfo           []UndelegatedDSInput         `json:"ds_info,omitempty"`
	MinLevel         string                       `json:"min_level,omitempty"`
	Tags             []string                     `json:"tags,omitempty"`
	Profile          string                       `json:"profile,omitempty"`
	IPv4Disabled     bool                         `json:"ipv4_disabled,omitempty"`
	IPv6Disabled     bool                         `json:"ipv6_disabled,omitempty"`
}

// JobBatchRequest is the payload for a batch submission.
type JobBatchRequest struct {
	Domains          []string                      `json:"domains,omitempty"`
	FromTag          string                        `json:"from_tag,omitempty"`
	Tests            []string                      `json:"tests,omitempty"`
	ProfileID        *int64                        `json:"profile_id,omitempty"`
	ProfileOverrides map[string]any                `json:"profile_overrides,omitempty"`
	Nameservers      *[]UndelegatedNameserverInput `json:"nameservers,omitempty"`
	DSInfo           *[]UndelegatedDSInput         `json:"ds_info,omitempty"`
	MinLevel         string                        `json:"min_level,omitempty"`
	Tags             []string                      `json:"tags,omitempty"`
	Profile          string                        `json:"profile,omitempty"`
	Description      string                        `json:"description,omitempty"`
	// SnapshotIntent flags this batch as intended to become a cohort
	// snapshot. Defaults to false so ad-hoc retests and partial-cohort
	// repairs never enter the cohort series by accident; the admin UI's
	// "Capture as cohort snapshot" checkbox is what sets it to true.
	SnapshotIntent bool `json:"snapshot_intent,omitempty"`
}

// UndelegatedNameserverInput represents one undelegated nameserver row.
type UndelegatedNameserverInput struct {
	NS string `json:"ns"`
	IP string `json:"ip,omitempty"`
}

// UndelegatedDSInput represents one undelegated DS row.
type UndelegatedDSInput struct {
	KeyTag    int    `json:"keytag"`
	Algorithm int    `json:"algorithm"`
	DigType   int    `json:"digtype"`
	Digest    string `json:"digest"`
}

// JobBatchResponse describes the batch submission result.
type JobBatchResponse struct {
	BatchID string   `json:"batch_id"`
	JobIDs  []string `json:"job_ids"`
}

// BatchSummary aggregates jobs for a batch.
type BatchSummary struct {
	BatchID      string         `json:"batch_id"`
	Tag          string         `json:"tag,omitempty"`
	Total        int            `json:"total"`
	StatusCounts map[string]int `json:"status_counts"`
	Grades       map[string]int `json:"grades,omitempty"`
	Items        []Job          `json:"items"`
	Limit        int            `json:"limit,omitempty"`
	Offset       int            `json:"offset,omitempty"`
	NextCursor   string         `json:"next_cursor,omitempty"`
	PrevCursor   string         `json:"prev_cursor,omitempty"`
	Sort         string         `json:"sort,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	StartedAt    *time.Time     `json:"started_at,omitempty"`
	FinishedAt   *time.Time     `json:"finished_at,omitempty"`
}

// QueueReorderRequest reorders queued jobs.
type QueueReorderRequest struct {
	JobIDs []string `json:"job_ids"`
}

// QueueRemoveRequest removes queued jobs by id.
type QueueRemoveRequest struct {
	JobIDs []string `json:"job_ids"`
}

// QueueRemoveResponse reports removed jobs.
type QueueRemoveResponse struct {
	Removed []string `json:"removed"`
}

// ErrorResponse is the standard error response body.
type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody contains error details.
type ErrorBody struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}
