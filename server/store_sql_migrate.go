package server

import (
	"database/sql"
	"fmt"
)

type sqlMigration struct {
	version int
	stmts   []string
	stmtsFn func(sqlDialect) []string
}

// buildV1DDL is the consolidated fresh-schema migration. Every table
// the server uses is created here in its final shape; there are no
// follow-up ALTERs to chase. autoinc + bigint vary per dialect:
//
//   - SQLite:     autoinc="INTEGER PRIMARY KEY"           bigint="INTEGER"
//   - PostgreSQL: autoinc="BIGSERIAL PRIMARY KEY"         bigint="BIGINT"
//   - MariaDB:    autoinc="BIGINT AUTO_INCREMENT PRIMARY KEY"  bigint="BIGINT"
func buildV1DDL(d sqlDialect, autoinc, bigint string) []string {
	return []string{
		// ── domains ────────────────────────────────────────────────────────
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS domains (
			id              %s,
			name            VARCHAR(255) NOT NULL UNIQUE,
			latest_run_id   VARCHAR(255),
			latest_run_at   TEXT,
			latest_status   TEXT,
			latest_level    TEXT,
			latest_score    INTEGER,
			latest_grade    TEXT,
			created_at      TEXT NOT NULL,
			run_count       INTEGER NOT NULL DEFAULT 0
		)`, autoinc),
		`CREATE INDEX IF NOT EXISTS idx_domains_name         ON domains(name)`,
		`CREATE INDEX IF NOT EXISTS idx_domains_latest_level ON domains(latest_level)`,

		// ── tags ───────────────────────────────────────────────────────────
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS tags (
			name               VARCHAR(255) NOT NULL PRIMARY KEY,
			description        TEXT         NOT NULL DEFAULT '',
			created_at         TEXT         NOT NULL,
			default_profile_id %s
		)`, bigint),
		`CREATE INDEX IF NOT EXISTS idx_tags_default_profile_id ON tags(default_profile_id)`,

		// ── domain_tags ────────────────────────────────────────────────────
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS domain_tags (
			domain_id   %s NOT NULL,
			tag         VARCHAR(255) NOT NULL,
			PRIMARY KEY (domain_id, tag)
		)`, bigint),
		`CREATE INDEX IF NOT EXISTS idx_domain_tags_tag ON domain_tags(tag)`,

		// ── jobs ───────────────────────────────────────────────────────────
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS jobs (
			id           VARCHAR(255) NOT NULL PRIMARY KEY,
			domain_id    BIGINT       NOT NULL DEFAULT 0,
			domain       TEXT         NOT NULL DEFAULT '',
			batch_id     VARCHAR(255) NOT NULL DEFAULT '',
			status       VARCHAR(32)  NOT NULL DEFAULT 'queued',
			created_at   TEXT         NOT NULL,
			started_at   TEXT,
			progress     INTEGER      NOT NULL DEFAULT 0,
			error        TEXT         NOT NULL DEFAULT '',
			profile      TEXT         NOT NULL DEFAULT '',
			profile_id   %s,
			profile_name TEXT         NOT NULL DEFAULT '',
			priority     INTEGER      NOT NULL DEFAULT 0,
			config_json  TEXT,
			public_id    VARCHAR(16)  DEFAULT NULL
		)`, bigint),
		`CREATE INDEX IF NOT EXISTS idx_jobs_status         ON jobs(status)`,
		`CREATE INDEX IF NOT EXISTS idx_jobs_batch_id       ON jobs(batch_id)`,
		`CREATE INDEX IF NOT EXISTS idx_jobs_profile_id     ON jobs(profile_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_jobs_public_id ON jobs(public_id)`,

		// ── runs ───────────────────────────────────────────────────────────
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS runs (
			id                      VARCHAR(255) NOT NULL PRIMARY KEY,
			domain_id               BIGINT       NOT NULL DEFAULT 0,
			domain                  TEXT         NOT NULL DEFAULT '',
			batch_id                VARCHAR(255) NOT NULL DEFAULT '',
			status                  VARCHAR(32)  NOT NULL,
			created_at              TEXT         NOT NULL,
			started_at              TEXT,
			finished_at             TEXT,
			duration_ms             BIGINT,
			sev_notice              INTEGER      NOT NULL DEFAULT 0,
			sev_warning             INTEGER      NOT NULL DEFAULT 0,
			sev_error               INTEGER      NOT NULL DEFAULT 0,
			sev_critical            INTEGER      NOT NULL DEFAULT 0,
			worst_level             TEXT         NOT NULL DEFAULT '',
			entry_count             INTEGER      NOT NULL DEFAULT 0,
			profile                 TEXT         NOT NULL DEFAULT '',
			profile_id              %s,
			profile_name            TEXT         NOT NULL DEFAULT '',
			effective_profile       TEXT         NOT NULL DEFAULT '',
			priority                INTEGER      NOT NULL DEFAULT 0,
			score                   INTEGER,
			grade                   TEXT,
			nameserver_timings_json TEXT,
			config_json             TEXT,
			public_id               VARCHAR(16)  DEFAULT NULL
		)`, bigint),
		`CREATE INDEX IF NOT EXISTS idx_runs_domain_id   ON runs(domain_id)`,
		`CREATE INDEX IF NOT EXISTS idx_runs_domain      ON runs(domain)`,
		`CREATE INDEX IF NOT EXISTS idx_runs_batch_id    ON runs(batch_id)`,
		`CREATE INDEX IF NOT EXISTS idx_runs_finished_at ON runs(finished_at)`,
		`CREATE INDEX IF NOT EXISTS idx_runs_created_at  ON runs(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_runs_worst_level ON runs(worst_level)`,
		`CREATE INDEX IF NOT EXISTS idx_runs_profile_id  ON runs(profile_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_runs_public_id ON runs(public_id)`,

		// ── entries ────────────────────────────────────────────────────────
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

		// ── batches ────────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS batches (
			id              VARCHAR(255) NOT NULL PRIMARY KEY,
			tag             TEXT         NOT NULL DEFAULT '',
			created_at      TEXT         NOT NULL,
			domain_count    INTEGER      NOT NULL DEFAULT 0,
			description     TEXT         NOT NULL DEFAULT '',
			snapshot_intent INTEGER      NOT NULL DEFAULT 0
		)`,

		// ── profiles ───────────────────────────────────────────────────────
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

		// ── settings ───────────────────────────────────────────────────────
		settingsTableDDL(d),

		// ── analysis catalog ───────────────────────────────────────────────
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
			materialization_done       INTEGER      NOT NULL DEFAULT 0,
			materialization_total      INTEGER      NOT NULL DEFAULT 0,
			last_materialized_at       TEXT,
			last_materialization_error TEXT         NOT NULL DEFAULT '',
			default_snapshot_policy    VARCHAR(32)  NOT NULL DEFAULT 'auto_latest',
			default_snapshot_id        %s,
			created_at                 TEXT         NOT NULL,
			updated_at                 TEXT         NOT NULL
		)`, autoinc, bigint),
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_analysis_cohort_catalog_source ON analysis_cohort_catalog(source_type, source_tag)`,
		`CREATE INDEX IF NOT EXISTS idx_analysis_cohort_catalog_sort_order ON analysis_cohort_catalog(sort_order)`,
		`CREATE INDEX IF NOT EXISTS idx_analysis_cohort_catalog_analysis_enabled ON analysis_cohort_catalog(analysis_enabled)`,
		`CREATE INDEX IF NOT EXISTS idx_analysis_cohort_catalog_public_enabled ON analysis_cohort_catalog(public_enabled)`,

		// ── analysis entity registries ─────────────────────────────────────
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

		// ── per-run analysis facts ─────────────────────────────────────────
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

		// ── snapshots ──────────────────────────────────────────────────────
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

		// ── per-snapshot view tables ───────────────────────────────────────
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
			addresses_json  TEXT        NOT NULL DEFAULT '[]',
			asns_json       TEXT        NOT NULL DEFAULT '[]',
			domains_json    TEXT        NOT NULL DEFAULT '[]',
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
			domains_json    TEXT         NOT NULL DEFAULT '[]',
			PRIMARY KEY (snapshot_id, nameserver_id, address_id)
		)`, bigint, bigint, bigint),
		`CREATE INDEX IF NOT EXISTS idx_analysis_snapshot_endpoint_view_snapshot_dc ON analysis_snapshot_endpoint_view(snapshot_id, domain_count)`,
		`CREATE INDEX IF NOT EXISTS idx_analysis_snapshot_endpoint_view_snapshot_asn ON analysis_snapshot_endpoint_view(snapshot_id, asn)`,

		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS analysis_snapshot_asn_view (
			snapshot_id      %s           NOT NULL,
			asn              BIGINT       NOT NULL,
			label            VARCHAR(255) NOT NULL DEFAULT '',
			domain_count     INTEGER      NOT NULL DEFAULT 0,
			address_count    INTEGER      NOT NULL DEFAULT 0,
			nameserver_count INTEGER      NOT NULL DEFAULT 0,
			prefix_count     INTEGER      NOT NULL DEFAULT 0,
			ipv4_count       INTEGER      NOT NULL DEFAULT 0,
			ipv6_count       INTEGER      NOT NULL DEFAULT 0,
			domains_json     TEXT         NOT NULL DEFAULT '[]',
			nameservers_json TEXT         NOT NULL DEFAULT '[]',
			prefixes_json    TEXT         NOT NULL DEFAULT '[]',
			PRIMARY KEY (snapshot_id, asn)
		)`, bigint),
		`CREATE INDEX IF NOT EXISTS idx_analysis_snapshot_asn_view_snapshot_dc ON analysis_snapshot_asn_view(snapshot_id, domain_count)`,

		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS analysis_snapshot_tag_view (
			snapshot_id      %s           NOT NULL,
			tag              VARCHAR(128) NOT NULL,
			module           VARCHAR(64)  NOT NULL DEFAULT '',
			testcase         VARCHAR(64)  NOT NULL DEFAULT '',
			level            VARCHAR(16)  NOT NULL DEFAULT '',
			domain_count     INTEGER      NOT NULL DEFAULT 0,
			occurrence_count INTEGER      NOT NULL DEFAULT 0,
			domains_json     TEXT         NOT NULL DEFAULT '[]',
			PRIMARY KEY (snapshot_id, tag)
		)`, bigint),
		`CREATE INDEX IF NOT EXISTS idx_analysis_snapshot_tag_view_snapshot_oc ON analysis_snapshot_tag_view(snapshot_id, occurrence_count)`,

		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS analysis_snapshot_domain_view (
			snapshot_id      %s          NOT NULL,
			domain_id        %s          NOT NULL,
			domain_name      VARCHAR(255) NOT NULL DEFAULT '',
			score            INTEGER,
			grade            VARCHAR(2)  NOT NULL DEFAULT '',
			worst_level      VARCHAR(16) NOT NULL DEFAULT '',
			finished_at      TEXT,
			nameserver_count INTEGER     NOT NULL DEFAULT 0,
			endpoint_count   INTEGER     NOT NULL DEFAULT 0,
			asn_count        INTEGER     NOT NULL DEFAULT 0,
			prefix_count     INTEGER     NOT NULL DEFAULT 0,
			nameservers_json TEXT        NOT NULL DEFAULT '[]',
			addresses_json   TEXT        NOT NULL DEFAULT '[]',
			tags_json        TEXT        NOT NULL DEFAULT '[]',
			PRIMARY KEY (snapshot_id, domain_id)
		)`, bigint, bigint),
		`CREATE INDEX IF NOT EXISTS idx_analysis_snapshot_domain_view_snapshot_name ON analysis_snapshot_domain_view(snapshot_id, domain_name)`,
		`CREATE INDEX IF NOT EXISTS idx_analysis_snapshot_domain_view_snapshot_worst ON analysis_snapshot_domain_view(snapshot_id, worst_level, grade)`,

		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS analysis_snapshot_prefix_view (
			snapshot_id    %s           NOT NULL,
			prefix         VARCHAR(64)  NOT NULL,
			family         VARCHAR(8)   NOT NULL DEFAULT '',
			domain_count   INTEGER      NOT NULL DEFAULT 0,
			address_count  INTEGER      NOT NULL DEFAULT 0,
			asn            BIGINT,
			asn_label      VARCHAR(255) NOT NULL DEFAULT '',
			asns_json      TEXT         NOT NULL DEFAULT '[]',
			domains_json   TEXT         NOT NULL DEFAULT '[]',
			addresses_json TEXT         NOT NULL DEFAULT '[]',
			PRIMARY KEY (snapshot_id, prefix)
		)`, bigint),
		`CREATE INDEX IF NOT EXISTS idx_analysis_snapshot_prefix_view_snapshot_dc ON analysis_snapshot_prefix_view(snapshot_id, domain_count)`,

		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS analysis_snapshot_overview_view (
			snapshot_id             %s      NOT NULL PRIMARY KEY,
			domain_count            INTEGER NOT NULL DEFAULT 0,
			nameserver_count        INTEGER NOT NULL DEFAULT 0,
			endpoint_count          INTEGER NOT NULL DEFAULT 0,
			asn_count               INTEGER NOT NULL DEFAULT 0,
			prefix_count            INTEGER NOT NULL DEFAULT 0,
			top_tags_json           TEXT    NOT NULL DEFAULT '[]',
			top_nameservers_json    TEXT    NOT NULL DEFAULT '[]',
			top_asns_json           TEXT    NOT NULL DEFAULT '[]',
			fact_distributions_json TEXT    NOT NULL DEFAULT '{}'
		)`, bigint),
	}
}

// settingsTableDDL returns the CREATE TABLE statement for settings.
// "key" is reserved in MariaDB and must be backtick-quoted.
func settingsTableDDL(d sqlDialect) string {
	q := `"`
	if _, ok := d.(mariadbDialect); ok {
		q = "`"
	}
	return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS settings (
		%skey%s   VARCHAR(255) NOT NULL PRIMARY KEY,
		value  TEXT         NOT NULL
	)`, q, q)
}

// sqlMigrations is the ordered list of schema migrations applied on
// startup. The schema is created in one shot at v1; this slice exists
// so future structural changes can be added as new versioned entries.
var sqlMigrations = []sqlMigration{
	{
		version: 1,
		stmtsFn: func(d sqlDialect) []string {
			switch d.(type) {
			case postgresDialect:
				return buildV1DDL(d, "BIGSERIAL PRIMARY KEY", "BIGINT")
			case mariadbDialect:
				return buildV1DDL(d, "BIGINT AUTO_INCREMENT PRIMARY KEY", "BIGINT")
			default:
				return buildV1DDL(d, "INTEGER PRIMARY KEY", "INTEGER")
			}
		},
	},
	{
		// Carry job.error onto the graduated run.
		version: 2,
		stmts: []string{
			`ALTER TABLE runs ADD COLUMN error TEXT NOT NULL DEFAULT ''`,
		},
	},
	{
		// Per-cohort floor for the snapshot tag view (empty = use server
		// default), and the effective floor stamped on each snapshot at
		// capture time so the public API can tell the UI which tag pills
		// are clickable.
		version: 3,
		stmts: []string{
			`ALTER TABLE analysis_cohort_catalog ADD COLUMN tag_view_min_level VARCHAR(16) NOT NULL DEFAULT ''`,
			`ALTER TABLE analysis_cohort_snapshots ADD COLUMN tag_view_min_level VARCHAR(16) NOT NULL DEFAULT ''`,
		},
	},
	{
		// Per-run DNSSEC chain summary blob, kept out of the shared runCols
		// select and served only through the lazy chain endpoint.
		version: 4,
		stmts: []string{
			`CREATE TABLE IF NOT EXISTS run_dnssec_chain (
				run_id     VARCHAR(255) NOT NULL PRIMARY KEY,
				chain_json TEXT         NOT NULL
			)`,
		},
	},
	{
		// Aggregated latency (median + p95 ms, sample count) per snapshot
		// entity. Nullable: pre-migration snapshots read as no data.
		version: 5,
		stmts: []string{
			`ALTER TABLE analysis_snapshot_nameserver_view ADD COLUMN latency_p50_ms REAL`,
			`ALTER TABLE analysis_snapshot_nameserver_view ADD COLUMN latency_p95_ms REAL`,
			`ALTER TABLE analysis_snapshot_nameserver_view ADD COLUMN latency_samples INTEGER`,
			`ALTER TABLE analysis_snapshot_endpoint_view ADD COLUMN latency_p50_ms REAL`,
			`ALTER TABLE analysis_snapshot_endpoint_view ADD COLUMN latency_p95_ms REAL`,
			`ALTER TABLE analysis_snapshot_endpoint_view ADD COLUMN latency_samples INTEGER`,
			`ALTER TABLE analysis_snapshot_asn_view ADD COLUMN latency_p50_ms REAL`,
			`ALTER TABLE analysis_snapshot_asn_view ADD COLUMN latency_p95_ms REAL`,
			`ALTER TABLE analysis_snapshot_asn_view ADD COLUMN latency_samples INTEGER`,
		},
	},
	{
		// Per-snapshot rematerialize progress so the admin UI can poll and
		// render a bar. Separate from the capture-lifecycle `status` column.
		version: 6,
		stmts: []string{
			`ALTER TABLE analysis_cohort_snapshots ADD COLUMN materialization_status VARCHAR(32) NOT NULL DEFAULT ''`,
			`ALTER TABLE analysis_cohort_snapshots ADD COLUMN materialization_done INTEGER NOT NULL DEFAULT 0`,
			`ALTER TABLE analysis_cohort_snapshots ADD COLUMN materialization_total INTEGER NOT NULL DEFAULT 0`,
			`ALTER TABLE analysis_cohort_snapshots ADD COLUMN last_materialization_error TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE analysis_cohort_snapshots ADD COLUMN last_materialized_at VARCHAR(64) NOT NULL DEFAULT ''`,
		},
	},
	{
		// Retire the bailiwick tag identifiers. entries.tag is the only
		// stored copy, so rewriting it keeps tag history continuous.
		version: 7,
		stmts: []string{
			`UPDATE entries SET tag = 'IN_DOMAIN_ADDR_MISMATCH' WHERE tag = 'IN_BAILIWICK_ADDR_MISMATCH'`,
			`UPDATE entries SET tag = 'NOT_IN_DOMAIN_ADDR_MISMATCH' WHERE tag = 'OUT_OF_BAILIWICK_ADDR_MISMATCH'`,
			`UPDATE entries SET tag = 'IN_DOMAIN_GLUE_MISSING' WHERE tag = 'IN_BAILIWICK_GLUE_MISSING'`,
		},
	},
	{
		// Snapshot provenance: which engine produced a snapshot's runs, and
		// whether the batch spanned an upgrade. Without it a trend line
		// cannot separate engine change from cohort change.
		version: 8,
		stmts: []string{
			`ALTER TABLE analysis_cohort_snapshots ADD COLUMN engine_version VARCHAR(64) NOT NULL DEFAULT ''`,
			`ALTER TABLE analysis_cohort_snapshots ADD COLUMN mixed_engine_version INTEGER NOT NULL DEFAULT 0`,
		},
	},
	{
		// Migration 7 missed the analysis layer's own copies of a tag, which
		// left a renamed tag page empty on snapshots captured before it.
		version: 9,
		stmts:   renameAnalysisTagStmts(),
	},
	{
		// Per-domain address-family and signing-algorithm columns on the
		// snapshot domain view. The algorithm columns are nullable: an
		// unsigned domain has no weakest algorithm and no keys.
		version: 10,
		stmts: []string{
			`ALTER TABLE analysis_snapshot_domain_view ADD COLUMN ipv4_ns_count INTEGER NOT NULL DEFAULT 0`,
			`ALTER TABLE analysis_snapshot_domain_view ADD COLUMN ipv6_ns_count INTEGER NOT NULL DEFAULT 0`,
			`ALTER TABLE analysis_snapshot_domain_view ADD COLUMN dnskey_algo_weakest INTEGER`,
			`ALTER TABLE analysis_snapshot_domain_view ADD COLUMN dnskey_count INTEGER`,
		},
	},
}

// retiredAnalysisTags is frozen at migration 9; a later rename needs its
// own migration.
var retiredAnalysisTags = [][2]string{
	{"IN_BAILIWICK_ADDR_MISMATCH", "IN_DOMAIN_ADDR_MISMATCH"},
	{"OUT_OF_BAILIWICK_ADDR_MISMATCH", "NOT_IN_DOMAIN_ADDR_MISMATCH"},
	{"IN_BAILIWICK_GLUE_MISSING", "IN_DOMAIN_GLUE_MISSING"},
}

// renameAnalysisTagStmts rewrites a retired tag everywhere the analysis
// layer stores one. A snapshot spanning the rename carries both names, so
// the retired row is dropped rather than colliding on (snapshot_id, tag).
func renameAnalysisTagStmts() []string {
	stmts := make([]string, 0, len(retiredAnalysisTags)*5)
	for _, pair := range retiredAnalysisTags {
		old, current := pair[0], pair[1]
		stmts = append(stmts,
			// The derived table lets MariaDB read the table it deletes from,
			// and DISTINCT keeps it from being merged back into the DELETE.
			fmt.Sprintf(`DELETE FROM analysis_snapshot_tag_view
				WHERE tag = '%s'
				  AND snapshot_id IN (
					SELECT snapshot_id FROM (
						SELECT DISTINCT snapshot_id FROM analysis_snapshot_tag_view WHERE tag = '%s'
					) already_renamed
				)`, old, current),
			fmt.Sprintf(`UPDATE analysis_snapshot_tag_view SET tag = '%s' WHERE tag = '%s'`, current, old),
			fmt.Sprintf(`UPDATE analysis_run_tag_summary SET tag = '%s' WHERE tag = '%s'`, current, old),
			// LIKE reads "_" as a wildcard; REPLACE still matches exactly.
			fmt.Sprintf(`UPDATE analysis_snapshot_domain_view SET tags_json = REPLACE(tags_json, '%s', '%s') WHERE tags_json LIKE '%%%s%%'`, old, current, old),
			fmt.Sprintf(`UPDATE analysis_snapshot_overview_view SET top_tags_json = REPLACE(top_tags_json, '%s', '%s') WHERE top_tags_json LIKE '%%%s%%'`, old, current, old),
		)
	}
	return stmts
}

// runMigrations creates the schema_migrations tracking table and applies
// any pending migrations. Each migration runs inside its own transaction
// so a partial failure leaves the database in the last fully-applied state.
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
