# The engine as a C shared library

`libgonemaster_engine` exposes the engine to a host process through ten C
functions. Requests, events, metadata, messages and errors cross the boundary
as JSON strings, so any foreign function interface that handles `char *` and
`uint64_t` can load it. The library is a thin layer over the Go package
`engine/integration`, described in [dev.md](dev.md).

## Build

```sh
make build-gonemaster-lib
```

The target writes `bin/libgonemaster_engine.so` (`.dylib` on macOS) and the
header `bin/libgonemaster_engine.h`. It requires cgo and a C compiler, so it
is not part of the cross-compiled release matrix. The macOS library carries
the install name `@rpath/libgonemaster_engine.dylib`, so a host links it with
an rpath of its own or sets `DYLD_LIBRARY_PATH`. `make lib-smoke` compiles
[tools/lib-smoke/smoke.c](../tools/lib-smoke/smoke.c) against the library and
runs it; CI runs the same check.

## Functions

```c
char *gm_version_json(void);
char *gm_list_tests_json(void);
char *gm_check_profile_json(char *profile_json);
char *gm_translate_json(char *request_json);
char *gm_normalize_name_json(char *name);
uint64_t gm_run_start(char *request_json);
char *gm_run_next(uint64_t handle);
void gm_run_cancel(uint64_t handle);
void gm_run_free(uint64_t handle);
void gm_free_string(char *s);
```

- Every `char *` the library returns MUST be released with `gm_free_string`.
  A string passed in is read during the call and stays owned by the caller.
- `gm_run_start` always returns a handle. A request that fails validation
  produces a run whose only event is an `error`.
- `gm_run_next` blocks until the next event. After the final event, `done` or
  `error`, every further call returns that event again. For a freed or unknown
  handle it returns an `error` event with code `invalid_request`.
- `gm_run_cancel` stops the run. The stream then ends with an `error` event
  with code `canceled`, after any entries already queued.
- `gm_run_free` cancels a run that is still going and releases the handle. A
  handle that is never freed keeps its run in memory. A caller that stops
  pulling events MUST cancel or free the run, or the run blocks on its next
  event.
- A Go panic becomes an `error` event with code `panic`; the host process
  keeps running.
- The functions MAY be called from any thread.

## Request

`gm_run_start` takes one JSON object. Unknown fields are rejected.

| Field | Type | Meaning |
|---|---|---|
| `domain` | string | Required. An IDN is normalised to A-labels; an IP address is tested as its reverse name. |
| `module` | string | Limits the run to one module. |
| `testcases` | array of string | Limits the run to these testcases. |
| `exclude` | array of string | Testcases or modules removed from the selection. |
| `profile` | object | A profile applied over the defaults, in the format of `share/profile.json`. |
| `min_level` | string | Lowest level delivered, for example `NOTICE`. |
| `ipv4`, `ipv6` | boolean | Override `net.ipv4` and `net.ipv6`. |
| `resolver` | object | `timeout`, `retry`, `retrans`, `parallel`, `error_cache_ttl`, `positive_cache_ttl`, `negative_cache_ttl` (integers), `fallback`, `unordered` (booleans), `source4`, `source6` (addresses). Each overrides the profile property of the same name. |
| `undelegated_nameservers` | array of object | `name` and optional `ip` per nameserver. |
| `undelegated_ds` | array of object | `keytag`, `algorithm`, `digest_type` and hexadecimal `digest` per DS record. |
| `badkeys_path` | string | Overrides `badkeys.path`. |
| `include_timings` | boolean | Adds a `timings` event before `done`. |

## Events

Each `gm_run_next` call returns one JSON object whose `type` is one of:

- `entry`: `entry` carries `timestamp` (seconds since the run started),
  `module`, `testcase`, `tag`, `level` and `args`. Entries below `min_level`
  are not delivered. Entries the engine records outside its callback, the
  `System` entries such as `GLOBAL_VERSION`, arrive after the test entries.
- `timings`: `timings` is an array with `nameserver`, `address`, `avg_ms`,
  `min_ms`, `max_ms`, `median_ms`, `stddev_ms`, `count`, `status`,
  `timeout_count` and `refused_count` per address. Sent once, after the last
  entry, when `include_timings` is true.
- `done`: the run completed.
- `error`: `error` carries `code` and `message`; the run produced no further
  events.

```json
{"type":"entry","entry":{"timestamp":0.123,"module":"Basic","testcase":"Basic02","tag":"B02_AUTH_RESPONSE_SOA","level":"INFO","args":{"ns":"ns1.example.com/192.0.2.1"}}}
{"type":"timings","timings":[{"nameserver":"ns1.example.com","address":"192.0.2.1","median_ms":12.4,"count":9,"status":"ok"}]}
{"type":"done"}
{"type":"error","error":{"code":"invalid_profile","message":"unknown property \"net\""}}
```

## Error codes

| Code | Meaning |
|---|---|
| `invalid_request` | The request JSON does not decode, carries an unknown field, an unknown `min_level`, invalid undelegated input, or an unknown handle. |
| `invalid_domain` | The domain fails normalisation. |
| `unknown_module` | `module` names no module. |
| `unknown_testcase` | A `testcases` or `exclude` entry names no testcase, or the selection is empty after exclusion. |
| `invalid_profile` | The profile does not parse or sets an unknown property or an invalid value. |
| `canceled` | The run was canceled. |
| `panic` | The engine panicked; the message carries the panic value. |
| `run_failed` | Any other failure. |

## Metadata

`gm_list_tests_json` returns the inventory of the build:

```json
{"engine_version":"v1.7.25","schema":1,"levels":["DEBUG3","DEBUG2","DEBUG","INFO","NOTICE","WARNING","ERROR","CRITICAL"],
 "languages":["cs","da","de","en","es","fi","fr","ja","nb","nl","sl","sv"],
 "profile_properties":["asn_db.sources","asn_db.style","net.ipv4","..."],
 "modules":[{"name":"basic","display":"Basic","testcases":[{"id":"basic01","display":"Basic01","tags":["B01_CHILD_FOUND", "..."]}]}]}
```

Modules are in run order. `display` values match the `module` and `testcase`
fields of entries; `name` and `id` match the values accepted in a request.
`levels` is ordered from lowest to highest. `languages` lists the locales
`gm_translate_json` has a catalog for. `profile_properties` lists the dotted
paths a profile may set; a path names one property, so a `test_levels`
document replaces the whole map.

## Profile check

`gm_check_profile_json` returns `{"errors":[],"effective":{...}}` when the
profile applies, or one object per problem in `errors` with `code`, `path`
and `message`. Codes are `invalid_json`, `unknown_property`, `invalid_value`
and `unknown_testcase`. Parsing stops at the first property that fails. An
`unknown_testcase` entry names a `test_cases` value this build does not
implement; the engine ignores such values, so a run with that profile still
starts and `effective` is present.

`effective` is the profile a run with this document would use: the defaults
with the document applied, in the format of `share/profile.json`. The
document `{}` yields the defaults.

## Translation

`gm_translate_json` renders messages from the embedded catalogs, the same
ones the `gonemaster` command prints. The request names a language and the
entries; the reply carries one message per entry, in order:

```json
{"language":"sv","entries":[{"module":"System","tag":"GLOBAL_VERSION","args":{"version":"v1.7.25"}}]}
{"messages":[{"message":"Använder version v1.7.25 av Gonemasters testmotor.","found":true}]}
```

`language` is a locale such as `sv` or `sv_SE.UTF-8`; an empty string and a
locale without a catalog render English. `found` is false when no catalog has
the tag; `message` is then `MODULE:TAG` with the args appended. Unknown
request fields are an `error` event with code `invalid_request`.

## Name normalisation

`gm_normalize_name_json` applies the engine's domain name normalisation,
IDN conversion included, and returns the name or the problems:

```json
{"name":"xn--rksmrgs-5wao1o.se","errors":[]}
{"errors":[{"tag":"INVALID_ASCII","params":{"label":"!%~&"},"message":"Domain name has an ASCII label (\"!%~&\") with a character not permitted."}]}
```

A run normalises its `domain` the same way; the call exists for a host that
validates input before it starts a run.

## Version

`gm_version_json` returns the build and what the contract supports:

```json
{"engine_version":"v1.7.25","commit":"...","dns_library":"v0.6.118","schema":1,
 "capabilities":{"cancel":true,"effective_profile":true,"nameserver_timings":true,"normalization":true,"profile_json":true,"streaming":true,"translation":true,"undelegated_ds":true,"undelegated_ns":true}}
```

`schema` is the version of this contract. A host SHOULD refuse a library
whose `schema` it does not know.

## Example

```c
uint64_t h = gm_run_start("{\"domain\":\"example.com\",\"min_level\":\"NOTICE\"}");
for (;;) {
	char *ev = gm_run_next(h);
	int last = strstr(ev, "\"type\":\"done\"") || strstr(ev, "\"type\":\"error\"");
	puts(ev);
	gm_free_string(ev);
	if (last) break;
}
gm_run_free(h);
```
