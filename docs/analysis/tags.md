# Tags

A tag is a named collection of domains. Domains can belong to many tags, and a
tag can contain many domains.

Tags are used for:

- selecting domains for a batch with `from_tag`
- labeling domains and runs with `tags`
- filtering domains, runs, and entries
- backing public analysis cohorts
- choosing a default stored profile for tagged work

## Membership

Adding domains to a tag creates missing domain records and links existing
records without duplication. Deleting a tag removes the memberships but keeps
domains, runs, entries, batches, and snapshots unless a separate deletion action
removes those records.

## Batch Use

`from_tag` selects input domains. `tags` applies tags to the created jobs,
domains, and resulting runs.

For a normal rerun of a tag:

```sh
gonemaster-client jobs batch --from-tag tld --tag tld --wait
```

## Default Profiles

A tag may have a default stored profile. Jobs and batches that use that tag can
inherit the profile unless they explicitly choose another profile.

If multiple tags imply different default profiles, the server rejects the
ambiguous request and asks the caller to choose a profile explicitly.
