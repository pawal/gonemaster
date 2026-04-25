package server

import (
	"database/sql"
	"fmt"
)

type sqlMigration struct {
	version int
	stmts   []string
	stmtsFn func(sqlDialect) []string // dialect-specific statements (appended to stmts)
}

// buildV1DDL returns the migration 1 DDL statements for the given autoincrement
// and bigint type strings, which vary per SQL dialect.
//
//   - SQLite:     autoinc="INTEGER PRIMARY KEY"       bigint="INTEGER"
//   - PostgreSQL: autoinc="BIGSERIAL PRIMARY KEY"     bigint="BIGINT"
//   - MariaDB:    autoinc="BIGINT AUTO_INCREMENT PRIMARY KEY"  bigint="BIGINT"
func buildV1DDL(autoinc, bigint string) []string {
	return []string{
		// ── domains ────────────────────────────────────────────────────────────
		// Persistent domain registry. Created on first job submission.
		// latest_* columns are denormalized fast-path for analysis queries.
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS domains (
			id              %s,
			name            VARCHAR(255) NOT NULL UNIQUE,
			latest_run_id   VARCHAR(255),
			latest_run_at   TEXT,
			latest_status   TEXT,
			latest_level    TEXT,
			created_at      TEXT NOT NULL,
			run_count       INTEGER NOT NULL DEFAULT 0
		)`, autoinc),
		`CREATE INDEX IF NOT EXISTS idx_domains_name         ON domains(name)`,
		`CREATE INDEX IF NOT EXISTS idx_domains_latest_level ON domains(latest_level)`,

		// ── tags ───────────────────────────────────────────────────────────────
		// Named domain collections. Tags are created explicitly or auto-created
		// when first used in a job/batch submission.
		`CREATE TABLE IF NOT EXISTS tags (
			name        VARCHAR(255) NOT NULL PRIMARY KEY,
			description TEXT         NOT NULL DEFAULT '',
			created_at  TEXT         NOT NULL
		)`,

		// ── domain_tags ────────────────────────────────────────────────────────
		// Many-to-many: domains ↔ tags.
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS domain_tags (
			domain_id   %s NOT NULL,
			tag         VARCHAR(255) NOT NULL,
			PRIMARY KEY (domain_id, tag)
		)`, bigint),
		`CREATE INDEX IF NOT EXISTS idx_domain_tags_tag ON domain_tags(tag)`,

		// ── jobs ───────────────────────────────────────────────────────────────
		// Queue-only: in-flight jobs. Completed jobs graduate to runs+entries.
		// config_json stores tests, profile_overrides, undelegated ns/ds, min_level.
		`CREATE TABLE IF NOT EXISTS jobs (
			id          VARCHAR(255) NOT NULL PRIMARY KEY,
			domain_id   BIGINT       NOT NULL DEFAULT 0,
			domain      TEXT         NOT NULL DEFAULT '',
			batch_id    VARCHAR(255) NOT NULL DEFAULT '',
			status      VARCHAR(32)  NOT NULL DEFAULT 'queued',
			created_at  TEXT         NOT NULL,
			started_at  TEXT,
			progress    INTEGER      NOT NULL DEFAULT 0,
			error       TEXT         NOT NULL DEFAULT '',
			profile     TEXT         NOT NULL DEFAULT '',
			config_json TEXT,
			public_id   VARCHAR(16)  DEFAULT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_jobs_status         ON jobs(status)`,
		`CREATE INDEX IF NOT EXISTS idx_jobs_batch_id       ON jobs(batch_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_jobs_public_id ON jobs(public_id)`,

		// ── runs ───────────────────────────────────────────────────────────────
		// Completed executions. Immutable after creation. The primary analysis table.
		`CREATE TABLE IF NOT EXISTS runs (
			id           VARCHAR(255) NOT NULL PRIMARY KEY,
			domain_id    BIGINT       NOT NULL DEFAULT 0,
			domain       TEXT         NOT NULL DEFAULT '',
			batch_id     VARCHAR(255) NOT NULL DEFAULT '',
			status       VARCHAR(32)  NOT NULL,
			created_at   TEXT         NOT NULL,
			started_at   TEXT,
			finished_at  TEXT,
			duration_ms  BIGINT,
			sev_notice   INTEGER      NOT NULL DEFAULT 0,
			sev_warning  INTEGER      NOT NULL DEFAULT 0,
			sev_error    INTEGER      NOT NULL DEFAULT 0,
			sev_critical INTEGER      NOT NULL DEFAULT 0,
			worst_level  TEXT         NOT NULL DEFAULT '',
			entry_count  INTEGER      NOT NULL DEFAULT 0,
			profile      TEXT         NOT NULL DEFAULT '',
			config_json  TEXT,
			public_id    VARCHAR(16)  DEFAULT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_runs_domain_id   ON runs(domain_id)`,
		`CREATE INDEX IF NOT EXISTS idx_runs_domain      ON runs(domain)`,
		`CREATE INDEX IF NOT EXISTS idx_runs_batch_id    ON runs(batch_id)`,
		`CREATE INDEX IF NOT EXISTS idx_runs_finished_at ON runs(finished_at)`,
		`CREATE INDEX IF NOT EXISTS idx_runs_created_at  ON runs(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_runs_worst_level ON runs(worst_level)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_runs_public_id ON runs(public_id)`,

		// ── entries ────────────────────────────────────────────────────────────
		// Every engine log entry as its own row. SQL-queryable for analysis.
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS entries (
			id          %s,
			run_id      VARCHAR(255) NOT NULL,
			domain_id   BIGINT       NOT NULL DEFAULT 0,
			timestamp   REAL         NOT NULL,
			module      TEXT         NOT NULL,
			testcase    TEXT         NOT NULL DEFAULT '',
			tag         TEXT         NOT NULL,
			level       TEXT         NOT NULL,
			args_json   TEXT
		)`, autoinc),
		`CREATE INDEX IF NOT EXISTS idx_entries_run_id    ON entries(run_id)`,
		`CREATE INDEX IF NOT EXISTS idx_entries_domain_id ON entries(domain_id)`,
		`CREATE INDEX IF NOT EXISTS idx_entries_tag       ON entries(tag)`,
		`CREATE INDEX IF NOT EXISTS idx_entries_level     ON entries(level)`,
		`CREATE INDEX IF NOT EXISTS idx_entries_module    ON entries(module)`,
		`CREATE INDEX IF NOT EXISTS idx_entries_testcase  ON entries(testcase)`,

		// ── batches ────────────────────────────────────────────────────────────
		// Batch metadata. Created when a batch is submitted.
		`CREATE TABLE IF NOT EXISTS batches (
			id           VARCHAR(255) NOT NULL PRIMARY KEY,
			tag          TEXT         NOT NULL DEFAULT '',
			created_at   TEXT         NOT NULL,
			domain_count INTEGER      NOT NULL DEFAULT 0,
			description  TEXT         NOT NULL DEFAULT ''
		)`,
	}
}

// settingsTableDDL returns the CREATE TABLE statement for the settings table.
// "key" is a reserved word in MariaDB and must be quoted.
func settingsTableDDL(d sqlDialect) string {
	q := `"` // ANSI SQL quoting (SQLite, PostgreSQL)
	if _, ok := d.(mariadbDialect); ok {
		q = "`"
	}
	return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS settings (
		%skey%s   VARCHAR(255) NOT NULL PRIMARY KEY,
		value  TEXT         NOT NULL
	)`, q, q)
}

// buildV11DDL returns the migration 11 DDL statements for the cohort snapshot
// tables, plus the column additions on `batches` and `analysis_cohort_catalog`
// that Phase 1 of the cohort-snapshots plan requires.
func buildV11DDL(autoinc, bigint string) []string {
	return []string{
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS analysis_cohort_snapshots (
			id                         %s,
			cohort_id                  %s           NOT NULL,
			batch_id                   VARCHAR(255) NOT NULL,
			slug                       VARCHAR(64)  NOT NULL,
			label                      TEXT         NOT NULL DEFAULT '',
			description                TEXT         NOT NULL DEFAULT '',
			profile_id                 %s,
			profile_name               VARCHAR(255) NOT NULL DEFAULT '',
			captured_at                VARCHAR(64)  NOT NULL DEFAULT '',
			first_run_at               TEXT         NOT NULL DEFAULT '',
			last_run_at                TEXT         NOT NULL DEFAULT '',
			run_count                  INTEGER      NOT NULL DEFAULT 0,
			domain_count               INTEGER      NOT NULL DEFAULT 0,
			status                     VARCHAR(32)  NOT NULL DEFAULT 'pending',
			is_default                 INTEGER      NOT NULL DEFAULT 0,
			is_public                  INTEGER      NOT NULL DEFAULT 1,
			created_at                 TEXT         NOT NULL,
			updated_at                 TEXT         NOT NULL
		)`, autoinc, bigint, bigint),
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_analysis_cohort_snapshots_cohort_batch ON analysis_cohort_snapshots(cohort_id, batch_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_analysis_cohort_snapshots_cohort_slug  ON analysis_cohort_snapshots(cohort_id, slug)`,
		`CREATE INDEX IF NOT EXISTS idx_analysis_cohort_snapshots_lookup ON analysis_cohort_snapshots(cohort_id, status, captured_at)`,

		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS analysis_cohort_snapshot_aggregates (
			snapshot_id  %s           NOT NULL,
			category     VARCHAR(64)  NOT NULL,
			payload_json TEXT         NOT NULL,
			computed_at  TEXT         NOT NULL,
			PRIMARY KEY (snapshot_id, category)
		)`, bigint),
		`CREATE INDEX IF NOT EXISTS idx_analysis_cohort_snapshot_aggregates_snapshot_id ON analysis_cohort_snapshot_aggregates(snapshot_id)`,

		// Snapshot-intent gate on batches. Default 0 for batches created after
		// the upgrade; the follow-up UPDATE retrofits every pre-existing batch
		// as snapshot-intent so historical data remains part of the cohort
		// series.
		`ALTER TABLE batches ADD COLUMN snapshot_intent INTEGER NOT NULL DEFAULT 0`,
		`UPDATE batches SET snapshot_intent = 1`,

		// Per-cohort default-snapshot resolution.
		`ALTER TABLE analysis_cohort_catalog ADD COLUMN default_snapshot_policy VARCHAR(32) NOT NULL DEFAULT 'auto_latest'`,
		fmt.Sprintf(`ALTER TABLE analysis_cohort_catalog ADD COLUMN default_snapshot_id %s`, bigint),
	}
}

func buildV7DDL(autoinc, bigint string) []string {
	return []string{
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS analysis_cohort_catalog (
			id                         %s,
			source_type                VARCHAR(32)  NOT NULL,
			source_tag                 VARCHAR(255) NOT NULL,
			label                      TEXT         NOT NULL DEFAULT '',
			description                TEXT         NOT NULL DEFAULT '',
			analysis_enabled           INTEGER      NOT NULL DEFAULT 0,
			public_enabled             INTEGER      NOT NULL DEFAULT 0,
			is_default                 INTEGER      NOT NULL DEFAULT 0,
			sort_order                 INTEGER      NOT NULL DEFAULT 0,
			materialization_status     VARCHAR(32)  NOT NULL DEFAULT 'pending',
			last_materialized_at       TEXT,
			last_materialization_error TEXT         NOT NULL DEFAULT '',
			created_at                 TEXT         NOT NULL,
			updated_at                 TEXT         NOT NULL
		)`, autoinc),
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_analysis_cohort_catalog_source ON analysis_cohort_catalog(source_type, source_tag)`,
		`CREATE INDEX IF NOT EXISTS idx_analysis_cohort_catalog_sort_order ON analysis_cohort_catalog(sort_order)`,
		`CREATE INDEX IF NOT EXISTS idx_analysis_cohort_catalog_analysis_enabled ON analysis_cohort_catalog(analysis_enabled)`,
		`CREATE INDEX IF NOT EXISTS idx_analysis_cohort_catalog_public_enabled ON analysis_cohort_catalog(public_enabled)`,

		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS analysis_nameservers (
			id            %s,
			name          VARCHAR(255) NOT NULL UNIQUE,
			first_seen_at TEXT         NOT NULL,
			last_seen_at  TEXT         NOT NULL
		)`, autoinc),
		`CREATE INDEX IF NOT EXISTS idx_analysis_nameservers_name ON analysis_nameservers(name)`,

		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS analysis_addresses (
			id            %s,
			address       VARCHAR(255) NOT NULL UNIQUE,
			family        VARCHAR(16)  NOT NULL,
			first_seen_at TEXT         NOT NULL,
			last_seen_at  TEXT         NOT NULL
		)`, autoinc),
		`CREATE INDEX IF NOT EXISTS idx_analysis_addresses_address ON analysis_addresses(address)`,
		`CREATE INDEX IF NOT EXISTS idx_analysis_addresses_family ON analysis_addresses(family)`,

		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS analysis_prefixes (
			id            %s,
			prefix        VARCHAR(255) NOT NULL UNIQUE,
			family        VARCHAR(16)  NOT NULL,
			first_seen_at TEXT         NOT NULL,
			last_seen_at  TEXT         NOT NULL
		)`, autoinc),
		`CREATE INDEX IF NOT EXISTS idx_analysis_prefixes_prefix ON analysis_prefixes(prefix)`,
		`CREATE INDEX IF NOT EXISTS idx_analysis_prefixes_family ON analysis_prefixes(family)`,

		`CREATE TABLE IF NOT EXISTS analysis_asns (
			asn           BIGINT       NOT NULL PRIMARY KEY,
			label         TEXT         NOT NULL DEFAULT '',
			first_seen_at TEXT         NOT NULL,
			last_seen_at  TEXT         NOT NULL
		)`,

		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS analysis_run_ns_endpoints (
			cohort_id      %s          NOT NULL,
			run_id         VARCHAR(255) NOT NULL,
			domain_id      %s          NOT NULL,
			nameserver_id  %s          NOT NULL,
			address_id     %s          NOT NULL,
			role           VARCHAR(32) NOT NULL DEFAULT '',
			source         VARCHAR(32) NOT NULL DEFAULT '',
			family         VARCHAR(16) NOT NULL DEFAULT '',
			avg_ms         REAL,
			min_ms         REAL,
			max_ms         REAL,
			query_count    INTEGER     NOT NULL DEFAULT 0,
			PRIMARY KEY (cohort_id, run_id, domain_id, nameserver_id, address_id, role, source)
		)`, bigint, bigint, bigint, bigint),
		`CREATE INDEX IF NOT EXISTS idx_analysis_run_ns_endpoints_cohort_id ON analysis_run_ns_endpoints(cohort_id)`,
		`CREATE INDEX IF NOT EXISTS idx_analysis_run_ns_endpoints_run_id ON analysis_run_ns_endpoints(run_id)`,
		`CREATE INDEX IF NOT EXISTS idx_analysis_run_ns_endpoints_domain_id ON analysis_run_ns_endpoints(domain_id)`,
		`CREATE INDEX IF NOT EXISTS idx_analysis_run_ns_endpoints_nameserver_id ON analysis_run_ns_endpoints(nameserver_id)`,
		`CREATE INDEX IF NOT EXISTS idx_analysis_run_ns_endpoints_address_id ON analysis_run_ns_endpoints(address_id)`,

		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS analysis_run_address_asns (
			cohort_id     %s          NOT NULL,
			run_id        VARCHAR(255) NOT NULL,
			domain_id     %s          NOT NULL,
			address_id    %s          NOT NULL,
			prefix_id     %s,
			asn           BIGINT,
			lookup_status VARCHAR(32) NOT NULL DEFAULT '',
			source        VARCHAR(32) NOT NULL DEFAULT '',
			PRIMARY KEY (cohort_id, run_id, domain_id, address_id)
		)`, bigint, bigint, bigint, bigint),
		`CREATE INDEX IF NOT EXISTS idx_analysis_run_address_asns_cohort_id ON analysis_run_address_asns(cohort_id)`,
		`CREATE INDEX IF NOT EXISTS idx_analysis_run_address_asns_run_id ON analysis_run_address_asns(run_id)`,
		`CREATE INDEX IF NOT EXISTS idx_analysis_run_address_asns_domain_id ON analysis_run_address_asns(domain_id)`,
		`CREATE INDEX IF NOT EXISTS idx_analysis_run_address_asns_address_id ON analysis_run_address_asns(address_id)`,
		`CREATE INDEX IF NOT EXISTS idx_analysis_run_address_asns_asn ON analysis_run_address_asns(asn)`,

		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS analysis_run_domain_asns (
			cohort_id  %s           NOT NULL,
			run_id     VARCHAR(255) NOT NULL,
			domain_id  %s           NOT NULL,
			asn        BIGINT       NOT NULL,
			family     VARCHAR(8)   NOT NULL DEFAULT '',
			source     VARCHAR(32)  NOT NULL DEFAULT '',
			PRIMARY KEY (cohort_id, run_id, domain_id, asn, family)
		)`, bigint, bigint),
		`CREATE INDEX IF NOT EXISTS idx_analysis_run_domain_asns_cohort_id ON analysis_run_domain_asns(cohort_id)`,
		`CREATE INDEX IF NOT EXISTS idx_analysis_run_domain_asns_run_id ON analysis_run_domain_asns(run_id)`,
		`CREATE INDEX IF NOT EXISTS idx_analysis_run_domain_asns_domain_id ON analysis_run_domain_asns(domain_id)`,
		`CREATE INDEX IF NOT EXISTS idx_analysis_run_domain_asns_asn ON analysis_run_domain_asns(asn)`,

		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS analysis_run_domain_summary (
			cohort_id         %s          NOT NULL,
			run_id            VARCHAR(255) NOT NULL,
			domain_id         %s          NOT NULL,
			score             INTEGER,
			grade             TEXT,
			nameserver_count  INTEGER     NOT NULL DEFAULT 0,
			endpoint_count    INTEGER     NOT NULL DEFAULT 0,
			asn_count         INTEGER     NOT NULL DEFAULT 0,
			prefix_count      INTEGER     NOT NULL DEFAULT 0,
			worst_level       VARCHAR(16) NOT NULL DEFAULT '',
			PRIMARY KEY (cohort_id, run_id, domain_id)
		)`, bigint, bigint),
		`CREATE INDEX IF NOT EXISTS idx_analysis_run_domain_summary_cohort_id ON analysis_run_domain_summary(cohort_id)`,
		`CREATE INDEX IF NOT EXISTS idx_analysis_run_domain_summary_run_id ON analysis_run_domain_summary(run_id)`,
		`CREATE INDEX IF NOT EXISTS idx_analysis_run_domain_summary_domain_id ON analysis_run_domain_summary(domain_id)`,
		`CREATE INDEX IF NOT EXISTS idx_analysis_run_domain_summary_worst_level ON analysis_run_domain_summary(worst_level)`,

		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS analysis_projection_state (
			cohort_id          %s           NOT NULL,
			run_id             VARCHAR(255) NOT NULL,
			projector_version  VARCHAR(64)  NOT NULL,
			status             VARCHAR(32)  NOT NULL,
			projected_at       TEXT,
			error              TEXT         NOT NULL DEFAULT '',
			PRIMARY KEY (cohort_id, run_id)
		)`, bigint),
		`CREATE INDEX IF NOT EXISTS idx_analysis_projection_state_status ON analysis_projection_state(status)`,
		`CREATE INDEX IF NOT EXISTS idx_analysis_projection_state_projected_at ON analysis_projection_state(projected_at)`,
	}
}

// sqlMigrations is the ordered list of schema migrations applied on startup.
var sqlMigrations = []sqlMigration{
	{
		version: 1,
		stmtsFn: func(d sqlDialect) []string {

			switch d.(type) {
			case postgresDialect:
				return buildV1DDL("BIGSERIAL PRIMARY KEY", "BIGINT")
			case mariadbDialect:
				return buildV1DDL("BIGINT AUTO_INCREMENT PRIMARY KEY", "BIGINT")
			default: // sqlite
				return buildV1DDL("INTEGER PRIMARY KEY", "INTEGER")
			}
		},
	},
	{
		version: 2,
		stmts: []string{
			`ALTER TABLE jobs ADD COLUMN priority INTEGER NOT NULL DEFAULT 0`,
			`ALTER TABLE runs ADD COLUMN priority INTEGER NOT NULL DEFAULT 0`,
		},
	},
	{
		version: 3,
		stmtsFn: func(d sqlDialect) []string {
			var autoinc, bigint string
			switch d.(type) {
			case postgresDialect:
				autoinc = "BIGSERIAL PRIMARY KEY"
				bigint = "BIGINT"
			case mariadbDialect:
				autoinc = "BIGINT AUTO_INCREMENT PRIMARY KEY"
				bigint = "BIGINT"
			default:
				autoinc = "INTEGER PRIMARY KEY"
				bigint = "INTEGER"
			}
			return []string{
				fmt.Sprintf(`CREATE TABLE IF NOT EXISTS profiles (
					id             %s,
					name           VARCHAR(255) NOT NULL UNIQUE,
					description    TEXT         NOT NULL DEFAULT '',
					config         TEXT         NOT NULL DEFAULT '{}',
					public         INTEGER      NOT NULL DEFAULT 0,
					schema_version TEXT         NOT NULL DEFAULT '',
					created_at     TEXT         NOT NULL,
					updated_at     TEXT         NOT NULL
				)`, autoinc),
				fmt.Sprintf(`ALTER TABLE tags ADD COLUMN default_profile_id %s REFERENCES profiles(id) ON DELETE SET NULL`, bigint),
				`CREATE INDEX IF NOT EXISTS idx_tags_default_profile_id ON tags(default_profile_id)`,
				fmt.Sprintf(`ALTER TABLE jobs ADD COLUMN profile_id %s REFERENCES profiles(id) ON DELETE SET NULL`, bigint),
				`ALTER TABLE jobs ADD COLUMN profile_name TEXT NOT NULL DEFAULT ''`,
				`CREATE INDEX IF NOT EXISTS idx_jobs_profile_id ON jobs(profile_id)`,
				fmt.Sprintf(`ALTER TABLE runs ADD COLUMN profile_id %s REFERENCES profiles(id) ON DELETE SET NULL`, bigint),
				`ALTER TABLE runs ADD COLUMN profile_name TEXT NOT NULL DEFAULT ''`,
				`ALTER TABLE runs ADD COLUMN effective_profile TEXT NOT NULL DEFAULT ''`,
				`CREATE INDEX IF NOT EXISTS idx_runs_profile_id ON runs(profile_id)`,
				settingsTableDDL(d),
			}
		},
	},
	{
		version: 4,
		stmts: []string{
			`ALTER TABLE runs ADD COLUMN score INTEGER DEFAULT NULL`,
			`ALTER TABLE runs ADD COLUMN grade TEXT DEFAULT NULL`,
		},
	},
	{
		version: 5,
		stmts: []string{
			`ALTER TABLE domains ADD COLUMN latest_score INTEGER DEFAULT NULL`,
			`ALTER TABLE domains ADD COLUMN latest_grade TEXT DEFAULT NULL`,
		},
	},
	{
		version: 6,
		stmts: []string{
			`ALTER TABLE runs ADD COLUMN nameserver_timings_json TEXT DEFAULT NULL`,
		},
	},
	{
		version: 7,
		stmtsFn: func(d sqlDialect) []string {
			switch d.(type) {
			case postgresDialect:
				return buildV7DDL("BIGSERIAL PRIMARY KEY", "BIGINT")
			case mariadbDialect:
				return buildV7DDL("BIGINT AUTO_INCREMENT PRIMARY KEY", "BIGINT")
			default:
				return buildV7DDL("INTEGER PRIMARY KEY", "INTEGER")
			}
		},
	},
	{
		// Progress counters for cohort materialization. Written by the
		// projector during rebuild so the admin UI can poll and render a
		// "Materializing N / M" progress indicator instead of a hang.
		version: 8,
		stmts: []string{
			`ALTER TABLE analysis_cohort_catalog ADD COLUMN materialization_done INTEGER NOT NULL DEFAULT 0`,
			`ALTER TABLE analysis_cohort_catalog ADD COLUMN materialization_total INTEGER NOT NULL DEFAULT 0`,
		},
	},
	{
		// Per-run tag aggregates so the /tags landing-page endpoint
		// does not have to scan the entries table once per domain on
		// every request. Populated by the projector from each run's
		// entries; served via the in-memory materialization cache.
		version: 9,
		stmtsFn: func(d sqlDialect) []string {
			var bigint string
			switch d.(type) {
			case postgresDialect, mariadbDialect:
				bigint = "BIGINT"
			default:
				bigint = "INTEGER"
			}
			return []string{
				fmt.Sprintf(`CREATE TABLE IF NOT EXISTS analysis_run_tag_summary (
					cohort_id        %s          NOT NULL,
					run_id           VARCHAR(255) NOT NULL,
					domain_id        %s          NOT NULL,
					tag              VARCHAR(128) NOT NULL,
					module           VARCHAR(64)  NOT NULL DEFAULT '',
					testcase         VARCHAR(64)  NOT NULL DEFAULT '',
					level            VARCHAR(16)  NOT NULL DEFAULT '',
					occurrence_count INTEGER      NOT NULL DEFAULT 0,
					PRIMARY KEY (cohort_id, run_id, domain_id, tag, testcase)
				)`, bigint, bigint),
				`CREATE INDEX IF NOT EXISTS idx_analysis_run_tag_summary_cohort_id ON analysis_run_tag_summary(cohort_id)`,
				`CREATE INDEX IF NOT EXISTS idx_analysis_run_tag_summary_run_id ON analysis_run_tag_summary(run_id)`,
				`CREATE INDEX IF NOT EXISTS idx_analysis_run_tag_summary_tag ON analysis_run_tag_summary(tag)`,
				`CREATE INDEX IF NOT EXISTS idx_analysis_run_tag_summary_level ON analysis_run_tag_summary(level)`,
			}
		},
	},
	{
		// Generic per-(cohort, run, domain) fact store for small
		// distribution-shaped statistics (DNSKEY algorithm, signed
		// posture, NSEC mode, ...). One row per (category, key).
		version: 10,
		stmtsFn: func(d sqlDialect) []string {
			var bigint string
			switch d.(type) {
			case postgresDialect, mariadbDialect:
				bigint = "BIGINT"
			default:
				bigint = "INTEGER"
			}
			return []string{
				fmt.Sprintf(`CREATE TABLE IF NOT EXISTS analysis_run_domain_facts (
					cohort_id %s           NOT NULL,
					run_id    VARCHAR(255) NOT NULL,
					domain_id %s           NOT NULL,
					category  VARCHAR(64)  NOT NULL,
					fact_key  VARCHAR(128) NOT NULL,
					value_num BIGINT       NULL,
					PRIMARY KEY (cohort_id, run_id, domain_id, category, fact_key)
				)`, bigint, bigint),
				`CREATE INDEX IF NOT EXISTS idx_analysis_run_domain_facts_cohort_id ON analysis_run_domain_facts(cohort_id)`,
				`CREATE INDEX IF NOT EXISTS idx_analysis_run_domain_facts_run_id ON analysis_run_domain_facts(run_id)`,
				`CREATE INDEX IF NOT EXISTS idx_analysis_run_domain_facts_category ON analysis_run_domain_facts(cohort_id, category)`,
				`CREATE INDEX IF NOT EXISTS idx_analysis_run_domain_facts_category_key ON analysis_run_domain_facts(cohort_id, category, fact_key)`,
			}
		},
	},
	{
		// Cohort snapshot tables (one row per snapshot-intent batch plus a
		// pre-computed aggregates table) and the batch-level snapshot_intent
		// flag — data model only; projector and read path wired later.
		version: 11,
		stmtsFn: func(d sqlDialect) []string {
			switch d.(type) {
			case postgresDialect:
				return buildV11DDL("BIGSERIAL PRIMARY KEY", "BIGINT")
			case mariadbDialect:
				return buildV11DDL("BIGINT AUTO_INCREMENT PRIMARY KEY", "BIGINT")
			default:
				return buildV11DDL("INTEGER PRIMARY KEY", "INTEGER")
			}
		},
	},
	{
		// Per-snapshot read views: one paginatable, indexed row set per
		// entity tab (nameservers, endpoints, ASNs). Written at snapshot
		// capture time so the public read path is one indexed SELECT
		// instead of a full-cohort fact-row scan with N+1 lookups.
		version: 12,
		stmtsFn: func(d sqlDialect) []string {
			var bigint string
			switch d.(type) {
			case postgresDialect, mariadbDialect:
				bigint = "BIGINT"
			default:
				bigint = "INTEGER"
			}
			return buildV12DDL(bigint)
		},
	},
}

// buildV12DDL returns migration 12: per-snapshot entity-view tables that
// shape a captured snapshot's nameservers/endpoints/ASNs into rows the
// public read path can paginate at SQL.
func buildV12DDL(bigint string) []string {
	return []string{
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS analysis_snapshot_nameserver_view (
			snapshot_id     %s          NOT NULL,
			nameserver_id   %s          NOT NULL,
			nameserver_name VARCHAR(255) NOT NULL DEFAULT '',
			domain_count    INTEGER     NOT NULL DEFAULT 0,
			endpoint_count  INTEGER     NOT NULL DEFAULT 0,
			ipv4_count      INTEGER     NOT NULL DEFAULT 0,
			ipv6_count      INTEGER     NOT NULL DEFAULT 0,
			asn_count       INTEGER     NOT NULL DEFAULT 0,
			operator        VARCHAR(255) NOT NULL DEFAULT '',
			operator_asn    BIGINT,
			query_count     INTEGER     NOT NULL DEFAULT 0,
			PRIMARY KEY (snapshot_id, nameserver_id)
		)`, bigint, bigint),
		`CREATE INDEX IF NOT EXISTS idx_analysis_snapshot_nameserver_view_snapshot_dc ON analysis_snapshot_nameserver_view(snapshot_id, domain_count)`,
		`CREATE INDEX IF NOT EXISTS idx_analysis_snapshot_nameserver_view_snapshot_name ON analysis_snapshot_nameserver_view(snapshot_id, nameserver_name)`,

		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS analysis_snapshot_endpoint_view (
			snapshot_id     %s          NOT NULL,
			nameserver_id   %s          NOT NULL,
			address_id      %s          NOT NULL,
			nameserver_name VARCHAR(255) NOT NULL DEFAULT '',
			address         VARCHAR(64)  NOT NULL DEFAULT '',
			family          VARCHAR(8)   NOT NULL DEFAULT '',
			domain_count    INTEGER      NOT NULL DEFAULT 0,
			asn             BIGINT,
			asn_label       VARCHAR(255) NOT NULL DEFAULT '',
			prefix          VARCHAR(64)  NOT NULL DEFAULT '',
			PRIMARY KEY (snapshot_id, nameserver_id, address_id)
		)`, bigint, bigint, bigint),
		`CREATE INDEX IF NOT EXISTS idx_analysis_snapshot_endpoint_view_snapshot_dc ON analysis_snapshot_endpoint_view(snapshot_id, domain_count)`,
		`CREATE INDEX IF NOT EXISTS idx_analysis_snapshot_endpoint_view_snapshot_asn ON analysis_snapshot_endpoint_view(snapshot_id, asn)`,

		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS analysis_snapshot_asn_view (
			snapshot_id      %s          NOT NULL,
			asn              BIGINT       NOT NULL,
			label            VARCHAR(255) NOT NULL DEFAULT '',
			domain_count     INTEGER      NOT NULL DEFAULT 0,
			address_count    INTEGER      NOT NULL DEFAULT 0,
			nameserver_count INTEGER      NOT NULL DEFAULT 0,
			prefix_count     INTEGER      NOT NULL DEFAULT 0,
			ipv4_count       INTEGER      NOT NULL DEFAULT 0,
			ipv6_count       INTEGER      NOT NULL DEFAULT 0,
			PRIMARY KEY (snapshot_id, asn)
		)`, bigint),
		`CREATE INDEX IF NOT EXISTS idx_analysis_snapshot_asn_view_snapshot_dc ON analysis_snapshot_asn_view(snapshot_id, domain_count)`,
	}
}

// runMigrations creates the schema_migrations tracking table and applies any
// pending migrations. Each migration runs inside its own transaction so a
// partial failure leaves the database in the last fully-applied state.
func runMigrations(db *sql.DB, d sqlDialect) error {
	if _, err := db.Exec(
		`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY)`,
	); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	ph := d.Placeholder(1)
	for _, m := range sqlMigrations {
		var count int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM schema_migrations WHERE version = `+ph, m.version,
		).Scan(&count); err != nil {
			return fmt.Errorf("check migration %d: %w", m.version, err)
		}
		if count > 0 {
			continue
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", m.version, err)
		}

		allStmts := m.stmts
		if m.stmtsFn != nil {
			allStmts = append(allStmts, m.stmtsFn(d)...)
		}
		for _, stmt := range allStmts {
			if _, err := tx.Exec(stmt); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("migration %d: %w", m.version, err)
			}
		}
		if _, err := tx.Exec(
			`INSERT INTO schema_migrations(version) VALUES (`+ph+`)`, m.version,
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %d: %w", m.version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", m.version, err)
		}
	}
	return nil
}
