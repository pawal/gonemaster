# Zone10 (zone10)

Status: Final

## Purpose
- Validate SOA answer-shape correctness on nameservers: response presence, SOA presence, owner name correctness, and multiplicity.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Nameserver addresses from `methods.Method4and5`.
  - SOA responses from each nameserver.
- Profile/config knobs that affect behavior:
  - `resolver.defaults.parallel`: parallel nameserver query fanout.
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Get nameservers from `Method4and5`.
3. For each nameserver (parallelized, input-order merged logs):
   - Skip disabled transports.
   - Query apex `SOA`.
   - If no response, emit `NO_RESPONSE`.
   - Else if no SOA in answer, emit `NO_SOA_IN_RESPONSE`.
   - Else if more than one SOA in answer, emit `MULTIPLE_SOA`.
   - Else if single SOA owner name differs from expected zone FQDN, emit `WRONG_SOA`.
4. After all nameservers, if no non-start tag has been emitted, emit `ONE_SOA`.
5. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `MULTIPLE_SOA` | SOA response contains more than one SOA RR in answer section. |
| `NO_RESPONSE` | Nameserver did not return a DNS response to SOA query. |
| `NO_SOA_IN_RESPONSE` | Nameserver returned response without SOA in answer section. |
| `ONE_SOA` | No non-start finding was emitted for any evaluated nameserver. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |
| `WRONG_SOA` | Single SOA answer owner name does not match tested zone apex FQDN. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `MULTIPLE_SOA` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) producing multiple SOA RRs. |
| `MULTIPLE_SOA` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `MULTIPLE_SOA` | `count` | `int` | Number of SOA RRs in answer section. |
| `NO_RESPONSE` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) with no response. |
| `NO_RESPONSE` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `NO_SOA_IN_RESPONSE` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) with SOA-missing answer. |
| `NO_SOA_IN_RESPONSE` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `ONE_SOA` | `-` | `-` | No arguments. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Zone10`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Zone10`). |
| `WRONG_SOA` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) returning wrong SOA owner. |
| `WRONG_SOA` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `WRONG_SOA` | `owner` | `string` | SOA owner name found in response (lowercased). |
| `WRONG_SOA` | `name` | `string` | Expected zone apex FQDN (lowercased). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `MULTIPLE_SOA` | `ERROR` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `NO_RESPONSE` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `NO_SOA_IN_RESPONSE` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `ONE_SOA` | `INFO` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `WRONG_SOA` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |

## Differences From Upstream
- Upstream reference: [`zone10.md`](../../upstream/tests/Zone-TP/zone10.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: checks wrong-owner condition before multiplicity wording in procedure. Gonemaster: emits `MULTIPLE_SOA` first when SOA answer count is greater than one, and only checks `WRONG_SOA` in single-SOA branch.
  - Upstream: does not describe testcase boundary debug markers. Gonemaster: emits `TEST_CASE_START` and `TEST_CASE_END`.
  - Upstream: defines `ONE_SOA` as no message output for any server. Gonemaster: uses a generic non-start-entry gate (`hasNonStartEntry`) that can also be affected by shared helper emissions.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- Query-shape checks do not require authoritative flag or specific RCODE in this testcase path.
- Shared helper transport-disabled debug tags can suppress `ONE_SOA` because they count as non-start entries.
