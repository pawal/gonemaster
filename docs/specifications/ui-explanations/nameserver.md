# Public UI explanations: NAMESERVER

## Testcase nameserver01

Description:

Authoritative nameservers should only answer questions about the zones they serve, not act as open recursive resolvers that will look up anything for anyone. This check asks each nameserver a recursive question to see whether it obliges. A nameserver that does can be abused in denial-of-service reflection attacks and usually signals a misconfiguration.

## Testcase nameserver02

Description:

EDNS(0) is an extension to DNS that allows larger answers and signals support for features like DNSSEC. Every modern authoritative nameserver must handle it correctly. This check sends EDNS queries to each nameserver and looks for servers that silently drop the query, return errors, or mishandle the extension header.

## Testcase nameserver03

Description:

AXFR is the DNS zone-transfer operation used by secondary servers to copy a zone from the primary. It is not meant to be available to arbitrary clients, because the full contents of a zone can reveal information an operator may prefer to keep private. This check asks each nameserver whether it will hand out an AXFR and reports the answer.

## Testcase nameserver04

Description:

When a nameserver replies, the response should come from the same IP address that the query was sent to. Some broken or multi-homed setups reply from a different address, which most resolvers treat as a spoofed packet and discard. Clients that see this behaviour will appear to get no answer from the server at all.

## Testcase nameserver05

Description:

This check tests whether each nameserver handles AAAA (IPv6 address) queries correctly. It first confirms the server answers A queries normally, then looks at how the same server handles an AAAA query to the same name. Servers that time out, return errors, or send malformed answers for AAAA will cause IPv6-preferring clients to fail in ways that IPv4 clients never see.

## Testcase nameserver06

Description:

Every nameserver name listed for your domain must be resolvable to at least one IP address, either through glue records or through the normal DNS. This check goes through each of those names and tries to find an address for it. A name that resolves to nothing is an unreachable server, which reduces the effective number of working nameservers for your domain.

## Testcase nameserver07

Description:

An authoritative nameserver that is asked about a domain it does not serve should answer REFUSED - not hand back a pointer to the root servers as if it were a resolver. This check probes for that "upward referral" behaviour, which is an old misconfiguration that can be abused to amplify denial-of-service attacks.

## Testcase nameserver08

Description:

When a nameserver echoes back the question in its response, it should preserve the exact mixed case of the queried name (this is used by some resolvers as an extra check against forged answers). This check sends a mixed-case query and verifies the server keeps the case intact in its reply.

## Testcase nameserver09

Description:

The DNS is case-insensitive, so `EXAMPLE.com` and `example.COM` must be treated as the same name. This check sends two differently cased versions of the same query to each nameserver and compares the answers. Different responses mean the server is treating case as significant, which breaks resolvers that rely on mixed-case queries for security.

## Testcase nameserver10

Description:

EDNS version 1 does not exist yet, so a nameserver that receives such a query must respond with a specific "bad version" error. This check sends an EDNS v1 query and confirms the server follows the standard instead of dropping the query or returning an unrelated error, both of which confuse modern resolvers.

## Testcase nameserver11

Description:

The DNS standard requires nameservers to answer normally when a query carries an EDNS option the server does not recognise, and to simply ignore the unknown option. This check sends a query with an unknown option code and verifies the server does not crash, time out, or return an error.

## Testcase nameserver12

Description:

EDNS has a block of reserved flag bits (the "Z flags") that must be ignored by servers that do not recognise them. This check sets a Z flag in the query and confirms the nameserver still answers normally, as required by the standard.

## Testcase nameserver13

Description:

When a nameserver has to truncate a response because it does not fit over UDP, the truncated reply must still carry an OPT record confirming EDNS support. This check forces a truncated response and verifies the OPT record is still present, because resolvers will fall back to TCP only when they can trust the truncation signal.

## Testcase nameserver15

Description:

A nameserver can be asked for its software name and version through a special CHAOS-class TXT query. Some operators prefer to hide this information to avoid advertising potentially exploitable software versions. This check asks each nameserver the question and reports what, if anything, it reveals.

## Testcase nameserver16

Description:

NSID is an EDNS option that lets a nameserver identify which specific instance answered a query - useful when several servers share the same name behind an anycast address. This check requests the NSID and reports which servers provide one and what it says.

## Testcase nameserver17

Description:

DNS Cookies (RFC 7873) are a lightweight anti-spoofing and anti-amplification feature for authoritative servers. This check sends a Client Cookie, inspects the Server Cookie that comes back, and confirms the server accepts the cookie it just issued.

## Testcase nameserver18

Description:

Extended DNS Errors (RFC 8914) let a server attach a short, machine-readable reason to its answer. This check reads any such reason returned by your authoritative servers and reports what it says, flagging the few cases that point to a real problem: a server refusing or disowning the zone, a filtering device in the path, or a host behaving like a recursive resolver.

## Tag IS_A_RECURSOR

Header: Nameserver acts as open recursor

Description:

A nameserver listed for your domain answered a recursive query that had nothing to do with your zone, which means it is operating as an open resolver. Open recursors can be abused for denial-of-service amplification attacks, and the server's operator may be held responsible for traffic generated through it.

## Tag BREAKS_ON_EDNS

Header: Nameserver broken on EDNS queries

Description:

A nameserver stopped answering or returned nonsense when the check added the EDNS(0) extension to a standard query. EDNS is required by every modern resolver, DNSSEC included, so a server that cannot handle it will have its responses discarded or will trigger repeated retries from the resolvers trying to reach you.

## Tag EDNS_RESPONSE_WITHOUT_EDNS

Header: EDNS reply without EDNS header

Description:

A nameserver answered an EDNS query but did not include the required EDNS/OPT record in its reply. Resolvers use the OPT record to confirm that the server understood the EDNS extension; without it they may treat the response as unreliable and fall back to plain DNS, disabling DNSSEC and larger response sizes along the way.

## Tag EDNS_VERSION_ERROR

Header: Wrong EDNS version handling

Description:

A nameserver did not report an EDNS version mismatch the way the standard requires when sent an unsupported EDNS version. That indicates a broken EDNS implementation, which can cause modern resolvers to fail over your domain when they test for EDNS support before issuing real queries.

## Tag NO_EDNS_SUPPORT

Header: No EDNS support on nameserver

Description:

A nameserver does not appear to support EDNS(0) at all. Every modern resolver expects EDNS; without it your domain cannot use DNSSEC, is limited to short UDP responses that often have to retry over TCP, and will increasingly be treated as a second-class citizen by upgrading client software.

## Tag NS_ERROR

Header: Nameserver error during EDNS probing

Description:

The check's EDNS probes against a nameserver produced an error response or an unexpected rcode that is not one of the known EDNS success or failure signals. The server is likely running non-standard or broken DNS software, which shows up as unpredictable behaviour for resolvers that contact it.

## Tag DIFFERENT_SOURCE_IP

Header: Reply came from a different IP

Description:

A nameserver sent its answer from a different IP address than the one the query was sent to. Most resolvers treat such replies as spoofed and discard them, so from the caller's perspective the server appears to be unreachable even though it is responding.

## Tag A_UNEXPECTED_RCODE

Header: Unexpected A-query response code

Description:

A nameserver returned an unusual response code for a straightforward A-record query that it should have answered with NOERROR or NXDOMAIN. The server is reachable but misbehaving, which points to a misconfigured authoritative path or a non-DNS service replying on the same endpoint.

## Tag AAAA_BAD_RDATA

Header: Malformed AAAA response

Description:

A nameserver returned an AAAA answer whose data is not a valid IPv6 address. Resolvers that validate the answer will reject it, so IPv6 clients will fail to reach names served through this server even though IPv4 clients may appear to work fine.

## Tag AAAA_QUERY_DROPPED

Header: AAAA query dropped by nameserver

Description:

A nameserver did not answer an AAAA (IPv6 address) query at all, even though it answers A (IPv4 address) queries normally. This usually means outdated software or a broken firewall rule, and the result is that IPv6-preferring clients will experience delays or outright failures.

## Tag AAAA_UNEXPECTED_RCODE

Header: Unexpected AAAA-query response code

Description:

A nameserver returned an unusual response code for an AAAA query instead of the expected NOERROR or NXDOMAIN. This is a frequent symptom of server software that does not correctly handle IPv6 records, which causes inconsistent behaviour for clients on IPv6.

## Tag CAN_NOT_BE_RESOLVED

Header: Nameserver name does not resolve

Description:

A name listed as a nameserver for your domain did not resolve to any IP address through the normal DNS. Resolvers that follow that name will have nowhere to send their query, which reduces the effective redundancy of your delegation.

## Tag NO_RESOLUTION

Header: No nameserver could be resolved

Description:

None of the nameserver names listed for your domain could be resolved to any IP address. Resolvers have no machine they can actually contact for your zone, which makes the domain unreachable even though the delegation is in place.

## Tag UPWARD_REFERRAL

Header: Nameserver sends upward referral

Description:

An authoritative nameserver for your domain answered a query for an unrelated name by handing back a pointer toward the root servers, as if it were a recursive resolver. This is a long-standing misconfiguration that can be abused to amplify denial-of-service attacks and should be disabled in the nameserver software.

## Tag QNAME_CASE_INSENSITIVE

Header: Query name case not preserved

Description:

A nameserver changed the case of the queried name in the echoed question section of its reply. Some resolvers use mixed-case queries as an extra anti-spoofing check, and a server that normalises case defeats that defence, making responses harder to authenticate.

## Tag CASE_QUERIES_RESULTS_DIFFER

Header: Case-sensitive answer from nameserver

Description:

A nameserver returned different answers for two queries that differ only in letter case (DNS is case-insensitive, so the answers must be identical). The server is handling case incorrectly, which means two clients that happen to query your name with different case can get completely different data.

## Tag CASE_QUERY_DIFFERENT_ANSWER

Header: Different answer for mixed-case query

Description:

A nameserver returned different record data when the query name was written in different case even though DNS requires case-insensitive matching. Resolvers that randomise case for anti-spoofing will see conflicting answers and may treat the zone as unreliable.

## Tag CASE_QUERY_DIFFERENT_RC

Header: Different response code for mixed-case query

Description:

A nameserver returned one response code for a lowercase query and a different one for the same query in mixed case. The server is treating the two as different names, which is a protocol violation and causes inconsistent results across resolvers.

## Tag CASE_QUERY_NO_ANSWER

Header: No answer for mixed-case query

Description:

A nameserver answered a lowercase query normally but gave no answer at all when the same query was sent in mixed case. This is a bug in the server's case handling; resolvers that randomise case for security will see intermittent lookup failures.

## Tag N10_EDNS_RESPONSE_ERROR

Header: EDNS v1 handled incorrectly

Description:

A nameserver did not follow the standard way of rejecting an unsupported EDNS version. Modern resolvers probe EDNS behaviour as part of establishing how to talk to an authoritative server, and a server that mishandles this path can end up blacklisted or treated as broken.

## Tag N10_NO_RESPONSE_EDNS1_QUERY

Header: No answer to EDNS v1 query

Description:

A nameserver dropped an EDNS version 1 probe instead of answering it with the required "bad version" error. Resolvers that probe for EDNS capabilities see this as a broken server and may fall back to slower, less-featureful paths when talking to your domain.

## Tag N10_UNEXPECTED_RCODE

Header: Unexpected reply to EDNS v1 query

Description:

A nameserver returned a response code to an EDNS version 1 query that is not the one the standard requires. This indicates a broken EDNS implementation and will affect how modern resolvers decide what capabilities to use when contacting the server.

## Tag N11_NO_EDNS

Header: EDNS support disappears with unknown option

Description:

A nameserver that normally supports EDNS stopped doing so when the query carried an unknown EDNS option. The standard says unknown options must simply be ignored; the observed behaviour instead disables EDNS entirely, which will cause problems for resolvers that depend on it.

## Tag N11_NO_RESPONSE

Header: No answer with unknown EDNS option

Description:

A nameserver dropped a query that carried an unknown EDNS option instead of ignoring the option and answering normally. Future DNS extensions will use new option codes, and a server that cannot handle unknown options will stop working correctly as resolvers roll out those extensions.

## Tag N11_RETURNS_UNKNOWN_OPTION_CODE

Header: Unknown EDNS option echoed back

Description:

A nameserver echoed the unknown EDNS option back in its response. The standard says unknown options should be ignored and not copied into the reply; a server that echoes them can interact badly with resolvers that reuse the option code for a known purpose.

## Tag N11_UNEXPECTED_ANSWER_SECTION

Header: Unexpected answer with unknown EDNS option

Description:

A nameserver returned an answer section that did not match the query when the query carried an unknown EDNS option. The presence of the option should not change which data the server returns, so this indicates a bug that could affect real queries as EDNS extensions evolve.

## Tag N11_UNEXPECTED_RCODE

Header: Unexpected response code with unknown EDNS option

Description:

A nameserver responded with an unexpected rcode when the query carried an unknown EDNS option, even though the standard requires it to ignore the option and answer normally. This is a DNS standards violation that can confuse modern resolvers.

## Tag N11_UNSET_AA

Header: Authoritative flag missing with unknown EDNS option

Description:

A nameserver forgot to set the "authoritative answer" flag when the query included an unknown EDNS option, though its answer should otherwise have been authoritative. Resolvers rely on this flag to know the data can be trusted; without it, they may discard the response.

## Tag MISSING_OPT_IN_TRUNCATED

Header: Truncated reply missing OPT record

Description:

A nameserver sent a truncated response but left out the EDNS OPT record the standard requires. Without it, the resolver cannot confirm that the server still supports EDNS and may not know whether to retry over TCP, which causes timeouts and stalled lookups.

## Tag N15_SOFTWARE_VERSION

Header: Software version disclosed

Description:

A nameserver answered the CHAOS-class version query and returned its software name and version. This is informational and carries no penalty; it only records what the server reveals. Operators who prefer not to advertise a potentially targetable version string can configure the server to withhold it.

## Tag N15_NO_VERSION_REVEALED

Header: Software version withheld

Description:

A nameserver returned no software version in response to the CHAOS-class version query. Withholding the version is a common hardening choice and is reported here for information only, with no penalty.

## Tag N15_ERROR_ON_VERSION_QUERY

Header: No answer to version query

Description:

A nameserver did not answer the CHAOS-class version query, either staying silent or replying with SERVFAIL. This usually means the version is deliberately withheld and is reported for information only, with no penalty.

## Tag N15_WRONG_CLASS

Header: Wrong class in version reply

Description:

A nameserver replied to a CHAOS-class version query using the wrong DNS class in its answer. The software behind the server is likely old or unusual, which is a minor operational issue on its own but can point at broader standards-compliance problems.

## Tag N16_NO_RESPONSE

Header: No response to NSID request

Description:

A nameserver did not answer the NSID probe used to identify which specific server instance is responding. This is only a minor issue in most setups, but makes it harder for operators to troubleshoot problems with anycast or load-balanced nameservers.

## Tag N16_UNEXPECTED_RCODE

Header: Unexpected response code for NSID request

Description:

A nameserver returned an unusual response code when asked for its NSID. The server is reachable but is mishandling a straightforward EDNS option request, which usually indicates older or non-standard software.

## Tag N17_COOKIE_SUPPORTED

Header: DNS Cookie supported

Description:

A nameserver returned a valid DNS Cookie, a modern anti-spoofing and anti-amplification feature (RFC 7873). This is good posture and helps the server resist off-path forgery and abuse.

## Tag N17_COOKIE_ENFORCED

Header: DNS Cookie enforced

Description:

A nameserver requires DNS Cookies: it answered a probe without a Server Cookie with BADCOOKIE and a fresh, well-formed Server Cookie, then accepted that cookie on retry (RFC 7873). This is the strongest cookie posture and gives the best protection against off-path forgery and amplification abuse.

## Tag N17_NO_COOKIE

Header: No DNS Cookie

Description:

A nameserver did not return a DNS Cookie. This is common and not an error; the feature is simply not enabled, or a middlebox stripped it.

## Tag N17_COOKIE_ROUNDTRIP_OK

Header: DNS Cookie round-trip works

Description:

A nameserver did not reject the Server Cookie it had issued when the check sent it back on a follow-up query. This shows the server handles its own cookies consistently.

## Tag N17_COOKIE_CLIENT_ONLY

Header: Incomplete DNS Cookie

Description:

A nameserver reflected the Client Cookie but did not generate its own Server Cookie half. That is a non-conformant DNS Cookie implementation and provides none of the anti-spoofing benefit.

## Tag N17_COOKIE_MALFORMED

Header: Malformed DNS Cookie

Description:

A nameserver returned a DNS Cookie of an invalid size. A cookie must be 8 bytes, or 16 to 40 bytes; anything else can break strict clients that validate it.

## Tag N17_COOKIE_SELF_REJECT

Header: DNS Cookie rejected by issuer

Description:

A nameserver refused with BADCOOKIE a Server Cookie it had just issued, even after the check retried with the fresh cookie. That degrades legitimate clients and points to inconsistent cookie secrets.

## Tag N17_NO_RESPONSE

Header: No response to DNS Cookie probe

Description:

A nameserver did not answer the DNS Cookie probe at all. The cookie capability cannot be assessed for that server.

## Tag N18_NO_EXTENDED_ERROR

Header: No extended error

Description:

A nameserver answered cleanly without attaching an Extended DNS Error (RFC 8914). This is the normal, healthy case.

## Tag N18_EXTENDED_ERROR_REPORTED

Header: Server sent an extended error

Description:

A nameserver attached an informational Extended DNS Error (RFC 8914) to its answer. This is usually not a zone fault, but it is worth seeing what the server reported.

## Tag N18_SERVER_ERROR_REPORTED

Header: Server reported a problem

Description:

A nameserver reported, via an Extended DNS Error (RFC 8914), that it is prohibited, not authoritative, or does not support the query. For a server listed for your domain this is a real configuration problem.

## Tag N18_RESOLVER_BEHAVIOR_REPORTED

Header: Resolver-style error from this server

Description:

A host listed as authoritative returned an Extended DNS Error (RFC 8914) of a kind normally produced by a recursive resolver. This may mean a resolver is answering on an authoritative address, or simply an unusual server configuration.

## Tag N18_FILTERED_RESPONSE

Header: Filtering detected

Description:

An Extended DNS Error (RFC 8914) indicates that a blocking or filtering policy device sits between us and the nameserver, so its answers cannot be fully trusted.

## Tag N18_NO_RESPONSE

Header: No response

Description:

A nameserver did not answer the Extended DNS Error probe at all.
