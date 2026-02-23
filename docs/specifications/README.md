# Gonemaster Test Specifications

This directory contains the canonical testcase specifications for **gonemaster**.

The Zonemaster project may be used as an upstream reference during migration, but the documents in this tree define and describe gonemaster behavior.

## Goals
- Document each implemented gonemaster testcase with exact behavior.
- Treat emitted tags as part of the testcase contract.
- Keep documentation aligned with code through explicit workflow and validation.

## Documents

### Testcase Specifications
- [tests/](tests/README.md) — Canonical per-testcase specifications (algorithm, emitted tags, arguments, severity, upstream differences).
- [tags/](tags/README.md) — Per-module tag catalogs with severity levels and i18n coverage.

### Inventories
- [implemented-testcases.md](implemented-testcases.md) — Authoritative list of all implemented testcases.
- [possible-tags-by-testcase.md](possible-tags-by-testcase.md) — All possible tags per testcase, derived from code metadata.

### Gap and Divergence Tracking
- [known-intentional-gaps.md](known-intentional-gaps.md) — Upstream testcase gaps that are intentionally not implemented, with rationale.
- [known-behavior-divergences.md](known-behavior-divergences.md) — Behavior divergences identified during investigation, tagged for review.

### Migration
- [migration-tracker.md](migration-tracker.md) — Checklist tracking migration status per testcase.
- [upstream-testcase-matrix.md](upstream-testcase-matrix.md) — Maps upstream Zonemaster specs to gonemaster testcase IDs and migration status.

### Reference
- [upstream/](upstream/README.md) — Imported Zonemaster reference material (not normative for gonemaster).
- [templates/](templates/testcase-spec-template.md) — Reusable templates for testcase specs and tag tables.

## Source-Of-Truth Rules
- Gonemaster implementation is the runtime source of truth.
- `docs/specifications/tests/` is the documentation source of truth for expected behavior.
- `docs/specifications/upstream/` is reference-only and can differ from gonemaster.
- Any intentional divergence from upstream must be documented in the gonemaster testcase spec.
- Outside the `plans` directory tree, gonemaster docs must not reference files in `plans`.

## Update Workflow
1. Identify testcase behavior from current gonemaster code.
2. Compare with upstream reference when relevant.
3. Write or update canonical spec in `tests/` using the testcase template.
4. Update tag documentation in `tags/` using the tag table template.
5. Record progress in `migration-tracker.md`.
6. Add follow-up issues for unresolved ambiguities.

## Tooling

Refresh generated inventories:

```sh
make spec-export
```

Validate canonical testcase specs against implementation metadata:

```sh
make spec-validate
```

Optional validation with append-log scanner (metadata omission hints):

```sh
make spec-validate-scan
```

## Writing Principles
- Use exact language and explicit conditions.
- Prefer deterministic phrasing (`MUST`, `MUST NOT`, `MAY`) over vague wording.
- Describe possible emitted tags and argument keys explicitly.
- Keep examples minimal and implementation-aligned.

---

Copyright (c) Patrik Wallström  
Copyright (c) The Swedish Internet Foundation (https://internetstiftelsen.se/en/)  
Copyright (c) AFNIC (https://www.afnic.fr/en/)  
All rights reserved.  

Copyright belongs to external contributor where applicable.  

Creative Commons Attribution 4.0 International License applies. See https://creativecommons.org/licenses/by/4.0/ for the license conditions.
