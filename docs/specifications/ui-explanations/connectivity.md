# Public UI explanations: CONNECTIVITY

## Testcase connectivity01

Description:

Most DNS queries go over UDP. This check contacts each of your nameservers over UDP and asks for the zone's SOA and NS records, confirming that each server is reachable and answers authoritatively. A nameserver that drops UDP queries, answers them with the wrong data, or is not set as authoritative here is effectively useless for everyday lookups.

## Testcase connectivity02

Description:

Some DNS queries have to go over TCP - either because the answer is too big for UDP or because the client prefers TCP for privacy. Every nameserver must accept TCP connections on port 53 and answer the same way it answers UDP queries. This check runs the UDP tests again over TCP and reports any server that is blocked, unresponsive, or inconsistent on this transport.

## Testcase connectivity03

Description:

Autonomous Systems are the organisational units of the internet; each IP address is announced by one AS. Good practice is to spread the authoritative nameservers for a domain across more than one AS, so that a routing outage or a single network's problems cannot take them all offline at the same time. This check looks up the AS for each nameserver address and reports whether your servers are diverse enough.

## Testcase connectivity04

Description:

Nameservers should live on different IP network prefixes (such as `192.0.2.0/24`) so that a single network outage cannot take them all down together. This check groups your nameserver addresses by network prefix per IP family and reports when everything ends up in the same prefix or when most addresses cluster together, which signals a single point of network failure.

## Testcase connectivity05

Description:

A signed zone's DNSKEY answer is often larger than the UDP packet a resolver is prepared to receive, so the nameserver reports the answer as too big and the resolver asks again over TCP. This check measures how large your DNSKEY answer is, whether it arrives over UDP at the buffer size resolvers advertise today, and what each nameserver does when a client offers a buffer large enough for the whole answer. Receiving no answer at all is the case that hurts: the resolver waits out a timeout on every lookup, and some resolvers never recover with a retry.

## Tag CN01_MISSING_NS_RECORD_UDP

Header: UDP: NS answer has no NS record

Description:

A nameserver answered a UDP query for the zone's NS records but its reply did not include an NS record. The server is reachable but not actually serving the zone's delegation data over UDP, which is the transport resolvers use most of the time.

## Tag CN01_MISSING_SOA_RECORD_UDP

Header: UDP: SOA answer has no SOA record

Description:

A nameserver answered a UDP query for the zone's SOA record but left the SOA out of its reply. The server appears to be responding, but it is not loaded with your zone's data on the UDP transport everyday resolvers depend on.

## Tag CN01_NO_RESPONSE_NS_QUERY_UDP

Header: UDP: no response to NS query

Description:

A nameserver did not answer at all when asked for your zone's NS records over UDP. Resolvers that follow the delegation to this server will see timeouts and either retry over TCP (slower) or give up if TCP is also blocked.

## Tag CN01_NO_RESPONSE_SOA_QUERY_UDP

Header: UDP: no response to SOA query

Description:

A nameserver did not answer when asked for your zone's SOA record over UDP. That is the basic liveness question for an authoritative server; silence on the main DNS transport makes the server unusable from a resolver's point of view.

## Tag CN01_NO_RESPONSE_UDP

Header: UDP: nameserver unreachable

Description:

A nameserver did not respond to any UDP query the check sent. It is either offline, firewalled from port 53 UDP, or routed in a way that makes it unreachable from the internet. Resolvers that try to reach it will time out before eventually giving up.

## Tag CN01_NS_RECORD_NOT_AA_UDP

Header: UDP: NS answer not authoritative

Description:

A nameserver answered a UDP NS query without setting the "authoritative answer" flag. The server is reachable but is not really running your zone on UDP - probably a resolver or proxy forwarding the query somewhere else.

## Tag CN01_SOA_RECORD_NOT_AA_UDP

Header: UDP: SOA answer not authoritative

Description:

A nameserver answered a UDP SOA query without the "authoritative answer" flag set. The response cannot be trusted as coming directly from the zone's data, which is the whole point of using an authoritative server.

## Tag CN01_UNEXPECTED_RCODE_NS_QUERY_UDP

Header: UDP: unexpected code for NS query

Description:

A nameserver returned a response code to a UDP NS query that should not occur for a zone it is authoritative for. The server is reachable but behaves inconsistently, which points to misconfiguration on the UDP path.

## Tag CN01_UNEXPECTED_RCODE_SOA_QUERY_UDP

Header: UDP: unexpected code for SOA query

Description:

A nameserver returned an unusual response code to a UDP SOA query at the zone apex. An authoritative server should always answer its own zone's SOA with NOERROR, so anything else is a sign that the server is not correctly set up for this zone.

## Tag CN01_WRONG_NS_RECORD_UDP

Header: UDP: NS record has wrong owner

Description:

A nameserver answered the UDP NS query but the NS record it returned belongs to a different owner name than the zone that was asked about. Either the server is misconfigured or the query was being forwarded somewhere that does not actually serve the zone.

## Tag CN01_WRONG_SOA_RECORD_UDP

Header: UDP: SOA record has wrong owner

Description:

A nameserver answered a UDP SOA query but the SOA it returned is for a different zone. The server is not really serving your zone on UDP, even though it responds to queries about it.

## Tag CN02_MISSING_NS_RECORD_TCP

Header: TCP: NS answer has no NS record

Description:

A nameserver answered a TCP query for the zone's NS records but its reply did not include any NS records. TCP is required to be available on every authoritative server; an empty answer there points to a server that is not correctly set up for TCP.

## Tag CN02_MISSING_SOA_RECORD_TCP

Header: TCP: SOA answer has no SOA record

Description:

A nameserver responded to a TCP SOA query but the response did not carry the SOA. The server answers on TCP but is not loaded with your zone's data on that transport, which breaks large-answer retries and TCP-preferring clients.

## Tag CN02_NO_RESPONSE_NS_QUERY_TCP

Header: TCP: no response to NS query

Description:

A nameserver did not answer a TCP query for your zone's NS records. This almost always indicates a firewall that blocks DNS-over-TCP, which is a configuration error - TCP is required for any answer that does not fit in one UDP packet.

## Tag CN02_NO_RESPONSE_SOA_QUERY_TCP

Header: TCP: no response to SOA query

Description:

A nameserver did not answer a TCP query for your zone's SOA record. Resolvers that need TCP - because an answer was truncated or because they prefer it for privacy - will see timeouts and fail over to another server, lowering the effective redundancy of your delegation.

## Tag CN02_NO_RESPONSE_TCP

Header: TCP: nameserver unreachable

Description:

A nameserver did not answer any TCP query the check sent. It is likely blocked by a firewall that allows UDP but not TCP on port 53. The DNS protocol requires TCP to be available, and modern resolvers fall back to it regularly.

## Tag CN02_NS_RECORD_NOT_AA_TCP

Header: TCP: NS answer not authoritative

Description:

A nameserver answered a TCP NS query without setting the "authoritative answer" flag. The response cannot be relied on as coming directly from your zone, which breaks resolvers that validate the AA flag before accepting data.

## Tag CN02_SOA_RECORD_NOT_AA_TCP

Header: TCP: SOA answer not authoritative

Description:

A nameserver answered a TCP SOA query without the "authoritative answer" flag. Resolvers that insist on authoritative answers on the TCP path (many do) will discard the response and retry elsewhere.

## Tag CN02_UNEXPECTED_RCODE_NS_QUERY_TCP

Header: TCP: unexpected code for NS query

Description:

A nameserver returned an unusual response code to a TCP NS query. The server is reachable over TCP but is mishandling the question, which points to either broken DNS software or a load balancer that is not forwarding TCP correctly.

## Tag CN02_UNEXPECTED_RCODE_SOA_QUERY_TCP

Header: TCP: unexpected code for SOA query

Description:

A nameserver returned an unusual response code to a TCP SOA query at the zone apex. Authoritative servers must always answer their own SOA with NOERROR; anything else is a misconfiguration on the TCP path.

## Tag CN02_WRONG_NS_RECORD_TCP

Header: TCP: NS record has wrong owner

Description:

A nameserver returned an NS record on TCP but the record does not belong to the queried zone. The server is answering on TCP but with data from a different zone, most often a sign of a misconfigured view or forwarder.

## Tag CN02_WRONG_SOA_RECORD_TCP

Header: TCP: SOA record has wrong owner

Description:

A nameserver returned an SOA record on TCP but it belongs to a different zone than the one queried. The server is reachable on TCP but does not actually serve your zone there.

## Tag IPV4_ONE_ASN

Header: All IPv4 nameservers in one AS

Description:

Every nameserver for your domain that has an IPv4 address is announced by the same Autonomous System (AS). If that single network has a routing outage, IPv4 lookups for your domain will fail at all nameservers at once. Spreading across more than one AS is the standard way to mitigate this.

## Tag IPV6_ONE_ASN

Header: All IPv6 nameservers in one AS

Description:

Every nameserver for your domain that has an IPv6 address is announced by the same Autonomous System (AS). A single routing event at that AS will take IPv6 lookups for your domain offline across every nameserver simultaneously. Using at least one nameserver on a second AS removes this concentrated risk.

## Tag CN04_IPV4_SINGLE_PREFIX

Header: All IPv4 nameservers in one network prefix

Description:

Every IPv4 address across your nameservers falls in a single network prefix, meaning they all sit behind one network. A prefix-level outage (for example a router failure or a BGP issue) affects all of them at the same time, defeating the purpose of running more than one nameserver.

## Tag CN04_IPV6_SINGLE_PREFIX

Header: All IPv6 nameservers in one network prefix

Description:

Every IPv6 address across your nameservers falls in a single network prefix. A single network-level failure in that prefix takes all of them offline together, so the redundancy your domain appears to have on paper is not real in practice.

## Tag CN05_ANSWER_FITS_UDP

Header: DNSKEY answer fits in a UDP reply

Description:

The DNSKEY answer for your zone arrived complete over UDP, within the buffer size resolvers advertise by default. Lookups need no second round trip over TCP, which is the fastest and most widely supported outcome.

## Tag CN05_ANSWER_NEEDS_TCP

Header: DNSKEY answer needs a TCP retry

Description:

Your DNSKEY answer does not fit the UDP buffer resolvers advertise by default, so the nameserver reports it as too big and the resolver asks again over TCP. This is correct behaviour for a large signed answer. It costs one extra round trip per lookup and works only where TCP to port 53 is reachable, so publishing fewer or smaller keys is worth considering.

## Tag CN05_LARGE_ANSWER_DELIVERED_UDP

Header: Full answer delivered over UDP

Description:

When a client offered a buffer large enough for the whole DNSKEY answer, the nameserver delivered it over UDP in a single exchange. Resolvers configured with a large buffer avoid the TCP retry entirely.

## Tag CN05_SERVER_CAPS_UDP_ANSWER

Header: Nameserver truncates whatever buffer is offered

Description:

Even with a buffer large enough for the entire DNSKEY answer on offer, the nameserver reported the answer as too big instead of sending it. Nothing is broken: clients receive that reply and retry over TCP, and capping the size of UDP responses is a common deliberate configuration.

## Tag CN05_LARGE_ANSWER_NO_UDP_ANSWER

Header: No answer when a large buffer is offered

Description:

A client that offers a buffer large enough for your whole DNSKEY answer receives nothing at all, although the same nameservers answer smaller queries. The large reply is being dropped in front of the nameserver, typically by a firewall or middlebox that discards oversized or fragmented UDP packets. Resolvers that advertise a large buffer, as many older and embedded ones still do, spend a full timeout on every DNSKEY lookup and some fail outright. Two fixes work: let large UDP responses and IP fragments through the network in front of the nameserver, or configure the nameserver to truncate at a smaller size so clients receive a "too big" reply and retry over TCP.

## Tag CN05_UDP_LOSS_SIZE_DEPENDENT

Header: Large UDP answers are lost, small ones arrive

Description:

Your nameservers answer small queries over UDP, and TCP delivers the DNSKEY answer, but nothing arrives over UDP at the buffer size resolvers advertise by default. The loss depends on the size of the reply, which points at the network in front of the nameserver rather than at the nameserver itself. Validating resolvers wait out a timeout before they fall back, making DNSSEC lookups for your zone slow and unreliable.
