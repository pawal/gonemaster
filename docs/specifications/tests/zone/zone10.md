# Zone10

Status: Final

## Purpose
- Validate SOA answer-shape correctness on nameservers: response presence, SOA presence, owner name correctness, and multiplicity. When a single correct SOA is present, also check for CNAME or DNAME at the zone apex.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Nameserver addresses from [`ZoneNameservers`](../../nameserver-resolution.md#zonenameservers).
  - SOA responses from each nameserver.
- Profile/config knobs that affect behavior:
  - `resolver.defaults.parallel`: parallel nameserver query fanout.
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Get nameservers from [`ZoneNameservers`](../../nameserver-resolution.md#zonenameservers).
3. For each nameserver (parallelized, input-order merged logs):
   - Skip disabled transports.
   - Query apex `SOA`.
   - If no response, emit `NO_RESPONSE`.
   - Else if no SOA in answer, emit `NO_SOA_IN_RESPONSE`.
   - Else if more than one SOA in answer, emit `MULTIPLE_SOA`.
   - Else (single SOA):
     - If single SOA owner name differs from expected zone FQDN, emit `WRONG_SOA`.
     - Query apex `CNAME`; if a CNAME record for the apex name is in the answer, emit `SOA_AND_CNAME`.
     - Query apex `DNAME`; if a DNAME record for the apex name is in the answer, emit `APEX_DNAME`.
4. After all nameservers, if no non-start tag has been emitted, emit `ONE_SOA`.
5. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `APEX_DNAME` | Single SOA present and nameserver returns a DNAME at the zone apex (legal per RFC 6672, informational). |
| `MULTIPLE_SOA` | SOA response contains more than one SOA RR in answer section. |
| `NO_RESPONSE` | Nameserver did not return a DNS response to SOA query. |
| `NO_SOA_IN_RESPONSE` | Nameserver returned response without SOA in answer section. |
| `ONE_SOA` | No non-start finding was emitted for any evaluated nameserver. |
| `SOA_AND_CNAME` | Single SOA present and nameserver returns a CNAME at the zone apex alongside the SOA (illegal per RFC 1034 s3.6.2). |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |
| `WRONG_SOA` | Single SOA answer owner name does not match tested zone apex FQDN. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `APEX_DNAME` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) returning DNAME at apex. |
| `APEX_DNAME` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `MULTIPLE_SOA` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) producing multiple SOA RRs. |
| `MULTIPLE_SOA` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `MULTIPLE_SOA` | `count` | `int` | Number of SOA RRs in answer section. |
| `NO_RESPONSE` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) with no response. |
| `NO_RESPONSE` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `NO_SOA_IN_RESPONSE` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) with SOA-missing answer. |
| `NO_SOA_IN_RESPONSE` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `ONE_SOA` | `-` | `-` | No arguments. |
| `SOA_AND_CNAME` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) returning apex CNAME alongside SOA. |
| `SOA_AND_CNAME` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Zone10`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Zone10`). |
| `WRONG_SOA` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) returning wrong SOA owner. |
| `WRONG_SOA` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `WRONG_SOA` | `owner` | `string` | SOA owner name found in response (lowercased). |
| `WRONG_SOA` | `query_name` | `string` | Expected zone apex FQDN (lowercased). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `APEX_DNAME` | `NOTICE` | Default from `share/profile.json` (`test_levels.ZONE`). DNAME at apex is legal per RFC 6672; informational only. |
| `MULTIPLE_SOA` | `ERROR` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `NO_RESPONSE` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `NO_SOA_IN_RESPONSE` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `ONE_SOA` | `INFO` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `SOA_AND_CNAME` | `ERROR` | Default from `share/profile.json` (`test_levels.ZONE`). CNAME coexisting with SOA is illegal per RFC 1034 s3.6.2. |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `WRONG_SOA` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |

### Per-NS SOA Probe and Apex CNAME/DNAME Check

{{% expand "Show diagram" %}}
```
ns list = ZoneNameservers

For each nameserver (parallel; fan-out = resolver.defaults.parallel):

   transport disabled        -> ipDisabledMessage (DEBUG), skip

   query SOA at z.Name
    +- no response           -> NO_RESPONSE (ns, address), continue
    +- no SOA in answer      -> NO_SOA_IN_RESPONSE (ns, address), continue
    +- count > 1             -> MULTIPLE_SOA (ns, address, count), continue
    +- count == 1
         owner != apex FQDN  -> WRONG_SOA (ns, address, owner, query_name)
         query CNAME at z.Name
          +- CNAME in answer  -> SOA_AND_CNAME (ns, address)
         query DNAME at z.Name
          +- DNAME in answer  -> APEX_DNAME (ns, address)

After all nameservers:
   no non-start entry emitted -> ONE_SOA

emit TEST_CASE_END
```
{{% /expand %}}

## Differences From Upstream
- Differences (Upstream vs Gonemaster):
  - Upstream: checks wrong-owner condition before multiplicity wording in procedure. Gonemaster: emits `MULTIPLE_SOA` first when SOA answer count is greater than one, and only checks `WRONG_SOA` in single-SOA branch.
  - Upstream: does not describe testcase boundary debug markers. Gonemaster: emits `TEST_CASE_START` and `TEST_CASE_END`.
  - Upstream: defines `ONE_SOA` as no message output for any server. Gonemaster: uses a generic non-start-entry gate (`hasNonStartEntry`) that can also be affected by shared helper emissions.
  - Upstream `zone10` never queries CNAME or DNAME at the apex. Gonemaster adds `SOA_AND_CNAME` and `APEX_DNAME` checks in the single-SOA branch.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- Query-shape checks do not require authoritative flag or specific RCODE in this testcase path.
- Shared helper transport-disabled debug tags can suppress `ONE_SOA` because they count as non-start entries.
