# Tags

A tag is a named collection of domains. Domains can belong to many tags, and a
tag can contain many domains.

Tags are the shared grouping concept for:

- selecting domains for a batch with `from_tag`
- labeling domains and resulting runs with `tags`
- filtering domains, runs, and entries
- backing public analysis cohorts
- choosing a default stored profile for tagged work

## Membership

Adding domains to a tag creates missing domain records and links existing
records without duplication.

```sh
gonemaster-client tags create tld --description "Top-level domains"
gonemaster-client tags add-domains tld --file tlds.txt
```

Other input forms:

```sh
cat tlds.txt | gonemaster-client tags add-domains tld --stdin
gonemaster-client tags add-domains municipalities-se stockholm.se malmo.se goteborg.se
```

Deleting a tag removes the memberships. It does not delete domains, runs,
entries, batches, or cohort snapshots.

## Batch Use

`from_tag` selects input domains:

```sh
gonemaster-client jobs batch --from-tag tld --wait
```

`--tag` applies tags to the submitted domains and resulting runs:

```sh
gonemaster-client jobs batch --file domains.txt --tag tld --wait
```

For a normal rerun of a tag, use both:

```sh
gonemaster-client jobs batch --from-tag tld --tag tld --wait
```

That command reads domains from the current `tld` membership and writes new run
records linked back to `tld`.

## Queries

```sh
gonemaster-client tags list
gonemaster-client tags domains tld
gonemaster-client tags summary tld
gonemaster-client domains list --tag tld --level ERROR
gonemaster-client entries query --tag tld --module DNSSEC --latest
```

Admin API equivalents:

```bash
curl -s http://localhost:8080/api/v1/tags
curl -s http://localhost:8080/api/v1/tags/tld/domains
curl -s http://localhost:8080/api/v1/tags/tld/summary
curl -s "http://localhost:8080/api/v1/domains?tag=tld&level=ERROR"
curl -s "http://localhost:8080/api/v1/entries?tag=tld&module=DNSSEC&latest=true"
```

## Default Profiles

A tag may have a default stored profile. Jobs and batches that use that tag can
inherit the profile unless they explicitly choose another profile.

If multiple tags imply different default profiles, the server rejects the
ambiguous request and asks the caller to choose a profile explicitly.

API entry points:

```bash
curl -s -X PUT http://localhost:8080/api/v1/tags/{name}/profile \
  -H "Content-Type: application/json" \
  -d '{"profile": "default"}'
curl -s -X DELETE http://localhost:8080/api/v1/tags/{name}/profile
```

## Cohorts

A cohort is backed by one source tag. Changing tag membership affects future
cohort rebuilds and future snapshot-intent batches. Captured snapshots remain
immutable.

See [cohorts.md](cohorts.md) and [snapshots.md](snapshots.md).

## Batch History

The admin UI tag detail view can show earlier batches for a tag. This is the
operator path for answering "what did we run for this tag recently?" and for
opening batch deletion previews.

API entry point:

```bash
curl -s http://localhost:8080/api/v1/tags/{name}/batches
```
