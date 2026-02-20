# Gonemaster Test Specifications

This directory contains the canonical testcase specifications for **gonemaster**.

The Zonemaster project may be used as an upstream reference during migration, but the documents in this tree define and describe gonemaster behavior.

## Goals
- Document each implemented gonemaster testcase with exact behavior.
- Treat emitted tags as part of the testcase contract.
- Keep documentation aligned with code through explicit workflow and validation.

## Directory Layout
- `upstream/`
  - Reference material copied from Zonemaster specifications.
  - Not normative for gonemaster.
- `tests/`
  - Canonical gonemaster testcase specifications.
  - Normative source for documented testcase behavior.
- `tags/`
  - Tag-level documentation and catalogs.
  - Covers possible tags per testcase/module and severity expectations.
- `templates/`
  - Reusable templates for testcase specs and tag tables.
- `migration-tracker.md`
  - Checklist-style tracker for migration status per testcase.
- `known-intentional-gaps.md`
  - Recorded implementation gaps against upstream testcase specs with rationale.
- `known-behavior-divergences.md`
  - Review-tagged behavior/spec divergences found in prior investigations.

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

## Writing Principles
- Use exact language and explicit conditions.
- Prefer deterministic phrasing (`MUST`, `MUST NOT`, `MAY`) over vague wording.
- Describe possible emitted tags and argument keys explicitly.
- Keep examples minimal and implementation-aligned.
