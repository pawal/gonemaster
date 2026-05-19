# Delegation03

Status: Final

## Purpose
- Check whether a synthesized maximal referral response can fit within the 512-byte non-EDNS UDP DNS payload limit.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Parent zone object from `z.Parent(ctx)`.
  - Delegation NS names from `z.GlueNames(ctx)`.
  - Delegation addressed NS from `GlueNameservers`.
- Profile/config knobs that affect behavior:
  - No direct profile knob in this testcase.
  - Size limit is fixed by `constants.UDPPayloadLimit` (512).

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Build a maximally long qname under the tested zone apex.
3. Build a DNS message with question `<max-length-name> IN NS`.
4. Add NS records for all names from `z.GlueNames` into the authority section.
5. Get parent zone from `z.Parent(ctx)`; if parent is missing, return an error.
6. Read delegation addressed NS from `GlueNameservers`.
7. If IPv4 delegation NS list is non-empty and all names are in-bailiwick of parent:
   - Add one `A` glue record to additional section, using the first IPv4 nameserver.
8. If IPv6 delegation NS list is non-empty and all names are in-bailiwick of parent:
   - Add one `AAAA` glue record to additional section, using the first IPv6 nameserver.
9. Pack wire format with compression enabled and measure packet size.
10. Emit one of:
    - `REFERRAL_SIZE_TOO_LARGE` when packed size `> 512`.
    - `REFERRAL_SIZE_OK` otherwise.
11. Emit `TEST_CASE_END`.

### Synthetic Referral Build and Size Check (steps 2-11)

{{% expand "Show diagram" %}}
```
build qname = maxLengthNameFor(z.Name)   (FQDN padded to constants.FQDNMaxLength)
build msg with question <qname> IN NS

authority section:
   for each name in z.GlueNames -> add NS RR (owner = z.Name)

parent = z.Parent(ctx)
parent absent -> return error (testcase aborts)

delNS = GlueNameservers

additional section, A:
   nssV4 = filterByIPVersion(delNS, IPv4)
   nssV4 non-empty AND every NS name in nssV4 is in-bailiwick(parent.Name)
     -> add one A RR using nssV4[0]

additional section, AAAA:
   nssV6 = filterByIPVersion(delNS, IPv6)
   nssV6 non-empty AND every NS name in nssV6 is in-bailiwick(parent.Name)
     -> add one AAAA RR using nssV6[0]

pack msg with compression; size = len(packed)
   size >  constants.UDPPayloadLimit (512) -> REFERRAL_SIZE_TOO_LARGE (size)
   size <= 512                             -> REFERRAL_SIZE_OK        (size)

emit TEST_CASE_END
```
{{% /expand %}}

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `REFERRAL_SIZE_OK` | Synthesized packed referral size is within UDP 512-byte limit. |
| `REFERRAL_SIZE_TOO_LARGE` | Synthesized packed referral size exceeds UDP 512-byte limit. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `REFERRAL_SIZE_OK` | `size` | `int` | Packed referral size in bytes. |
| `REFERRAL_SIZE_TOO_LARGE` | `size` | `int` | Packed referral size in bytes. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Delegation03`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Delegation03`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `REFERRAL_SIZE_OK` | `INFO` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `REFERRAL_SIZE_TOO_LARGE` | `WARNING` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.DELEGATION`). |

## Differences From Upstream
- Upstream reference: [`delegation03.md`](../../upstream/tests/Delegation-TP/delegation03.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: describes adding one A/AAAA record with any address when conditions are met. Gonemaster: adds one record using the first available NS address from `GlueNameservers` for each family.
  - Upstream: does not describe testcase boundary debug markers. Gonemaster: emits `TEST_CASE_START` and `TEST_CASE_END`.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- This testcase does not send network queries; it evaluates synthesized packet size only.
- When a family has any out-of-bailiwick NS name relative to parent, no additional-section record is added for that family.
- A missing parent zone object is treated as a hard error instead of a logged result tag.
