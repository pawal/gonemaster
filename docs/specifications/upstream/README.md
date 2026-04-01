# Upstream References

This directory is reserved for imported upstream specification material (primarily Zonemaster docs).

Content here is reference-only. It is used for traceability and comparison during migration.
It is not the normative specification for gonemaster behavior.

## Provenance

Imported upstream markdown files include a top-of-file metadata comment with:
- `Upstream-Source`
- `Upstream-Commit`
- `Upstream-Date`

## Refreshing Imported Content

Run from repo root:

```sh
tools/specifications/import-upstream-specs.sh
```

Optional explicit upstream repo path:

```sh
tools/specifications/import-upstream-specs.sh /path/to/zonemaster
```
