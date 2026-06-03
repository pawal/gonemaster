# Analysis

Gonemaster can run large domain sets, store historical results, and publish
read-only cohort views for public analysis.

## Concepts

- A tag is a named collection of domains.
- A batch runs tests for many domains, often selected from a tag.
- A cohort is a curated public dataset backed by one source tag.
- A snapshot is an immutable view of a cohort captured from one
  snapshot-intent batch.

## Workflow

1. Create a tag and add domains.
2. Run a tagged batch.
3. Create or enable a cohort for the tag.
4. Capture a snapshot.
5. Share the public analysis URL.

## Guides

- Tags: [tags.md](tags.md)
- Cohorts: [cohorts.md](cohorts.md)
- Snapshots: [snapshots.md](snapshots.md)
- Querying: [querying.md](querying.md)
- Public UI: [public-ui.md](public-ui.md)
- Agent-driven analysis (MCP): [../mcp/analysis-examples.md](../mcp/analysis-examples.md)
