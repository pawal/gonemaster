# Public UI explanations: SYNTAX

## Testcase syntax01

Description:

DNS hostnames are only allowed to contain letters A-Z, digits 0-9, and the hyphen character. This check looks at every label in your domain name and flags any character that falls outside that set. A disallowed character makes the name unusable with strict resolvers and most registration systems.

## Testcase syntax02

Description:

Each label in a domain name (the parts between the dots) must not start or end with a hyphen. A leading or trailing hyphen is a syntax error that resolvers and registries are allowed to reject, so a domain that has one may work in some places but be refused in others.

## Testcase syntax03

Description:

A domain label should not have two hyphens in the third and fourth positions (like `ab--cd`). That pattern is reserved for internationalised domain names, which always start with `xn--`. Any other label matching the pattern risks being misinterpreted by software that tries to decode it as an IDN.

## Testcase syntax04

Description:

Your nameserver hostnames must follow the same character and shape rules as any other DNS name. This check goes through every nameserver name listed in the parent zone's delegation and in your zone itself and runs the same syntax checks on each, because a single bad nameserver name can put part of the domain's service out of reach.

## Testcase syntax05

Description:

The SOA record for your zone carries a contact email address in a special DNS form where the `@` sign is replaced by a dot (so `hostmaster@example.com` becomes `hostmaster.example.com`). This check looks at that field and flags the common mistake of leaving an actual `@` sign in place, which breaks anyone trying to reach you through the address.

## Testcase syntax06

Description:

The SOA contact address should be a working mailbox. This check takes the address from your SOA record, verifies that it looks like a valid email address, and tries to resolve its mail domain to make sure there is somewhere to deliver mail. Problems here mean the operational contact your zone publishes is unreachable for registry and security notifications.

## Testcase syntax07

Description:

The SOA record also names the zone's primary (or "master") nameserver in the MNAME field. That name must be a well-formed DNS hostname. This check runs the same syntax checks on MNAME as on any other nameserver name, because tools that use MNAME (such as secondary servers pulling zone updates) will fail if it is not a valid hostname.

## Testcase syntax08

Description:

Every hostname listed as a mail exchanger (MX) for your zone must itself be a valid DNS hostname. This check fetches your MX records and runs syntax checks on each target name. A bad MX target will not accept mail from well-behaved mail servers, which shows up as silent delivery failures.

## Tag NON_ALLOWED_CHARS

Header: Disallowed characters in domain name

Description:

The domain name contains at least one character outside the permitted set of letters, digits, and the hyphen. Registrars and resolvers that follow the DNS host-name rules will refuse to look it up, so the domain will not work for many users even if some systems tolerate it.

## Tag INITIAL_HYPHEN

Header: Label begins with a hyphen

Description:

One of the labels in your domain name starts with a hyphen, which is not allowed under the DNS host-name rules. Software that enforces the rules will treat the name as invalid, so the domain will fail to resolve or register in several places where it otherwise seems to work.

## Tag TERMINAL_HYPHEN

Header: Label ends with a hyphen

Description:

One of the labels in your domain name ends with a hyphen, which is not allowed under the DNS host-name rules. Strict resolvers and registration systems will reject the name, which leads to inconsistent behaviour: looking up the domain may succeed with some tools and fail with others.

## Tag DISCOURAGED_DOUBLE_DASH

Header: Ambiguous "--" in label

Description:

A label in your domain has two hyphens in the third and fourth positions (like `ab--cd`). That pattern is reserved as the prefix for internationalised domain names (always `xn--`). Using it outside that context can cause software that tries to decode IDNs to misinterpret your name.

## Tag NAMESERVER_NON_ALLOWED_CHARS

Header: Disallowed characters in nameserver name

Description:

One of the hostnames listed as a nameserver for your domain contains characters that are not allowed in DNS names. Strict resolvers will refuse to use that server, reducing the effective redundancy of your delegation and possibly putting the domain out of reach for some clients.

## Tag NAMESERVER_DISCOURAGED_DOUBLE_DASH

Header: Ambiguous "--" in nameserver name

Description:

A label in one of your nameserver hostnames has two hyphens in the third and fourth positions but does not begin with the `xn--` prefix reserved for internationalised domains. Tools that try to decode the name as an IDN may misinterpret it, so the nameserver can misbehave in hard-to-diagnose ways.

## Tag NAMESERVER_NUMERIC_TLD

Header: All-numeric top-level label in nameserver name

Description:

The last (rightmost) label of one of your nameserver hostnames is made up entirely of digits, which is not a valid top-level domain. Resolvers and system libraries that try to distinguish IP addresses from hostnames may treat the name as an address instead of a name, and the server will not be reachable.

## Tag RNAME_MISUSED_AT_SIGN

Header: "@" in SOA contact address

Description:

Your SOA record's contact address still contains an `@` sign. In DNS the contact mailbox is written with the `@` replaced by a dot (`hostmaster.example.com`, not `hostmaster@example.com`). Leaving the `@` in place means any tool that reads the field gets a malformed address and cannot deliver notifications or alerts.

## Tag RNAME_MAIL_DOMAIN_LOCALHOST

Header: SOA contact domain points to localhost

Description:

The mail domain of your SOA contact address resolves to a loopback address (`127.0.0.1` or `::1`), which is only reachable on the machine itself. Nobody on the public internet can deliver mail there, so the published contact address is effectively unreachable.

## Tag RNAME_MAIL_ILLEGAL_CNAME

Header: SOA contact domain is a CNAME

Description:

The mail domain in your SOA contact address is defined as a CNAME, but mail-delivery rules forbid the target of an MX (or the mailbox domain itself) from being a CNAME. Strict mail servers refuse to deliver to such an address, so notifications sent to the published contact can silently fail.

## Tag RNAME_RFC822_INVALID

Header: SOA contact is not a valid mailbox

Description:

After converting your SOA contact from DNS form to email form, the result is not a syntactically valid mailbox address. Automated mailers that try to reach the domain operator will reject the address, so any registry, security, or abuse notifications aimed at the contact will never arrive.

## Tag MNAME_NON_ALLOWED_CHARS

Header: Disallowed characters in SOA master nameserver

Description:

The MNAME field of your SOA record (the name of your primary nameserver) contains characters that are not allowed in DNS hostnames. Secondary nameservers and tools that rely on MNAME to find the primary server will refuse to use it, which breaks zone transfers and related operations.

## Tag MNAME_DISCOURAGED_DOUBLE_DASH

Header: Ambiguous "--" in SOA master nameserver

Description:

The MNAME in your SOA record has two hyphens in the third and fourth positions of a label without the `xn--` prefix reserved for internationalised domains. Tools that decode the name as an IDN can misinterpret it, leading to subtle failures in software that consumes the SOA record.

## Tag MNAME_NUMERIC_TLD

Header: All-numeric top-level label in SOA master nameserver

Description:

The last label of the MNAME in your SOA record is all digits, which is not a valid top-level domain. Resolvers that guard against IP-address-looking hostnames may reject the name, so operations that target the primary nameserver by MNAME can fail.

## Tag MX_NON_ALLOWED_CHARS

Header: Disallowed characters in MX target

Description:

One of the hostnames your zone lists as a mail exchanger contains characters that are not allowed in DNS names. Sending mail servers that follow the rules will refuse to use the MX target, and mail directed at your domain may be deferred or bounced.

## Tag MX_DISCOURAGED_DOUBLE_DASH

Header: Ambiguous "--" in MX target

Description:

A label in one of your MX target hostnames has two hyphens in the third and fourth positions but does not use the `xn--` IDN prefix. Mail software that tries to decode the name as an internationalised domain may misinterpret it, which can cause inconsistent mail routing behaviour.

## Tag MX_NUMERIC_TLD

Header: All-numeric top-level label in MX target

Description:

The last label of one of your MX target hostnames is entirely numeric, which is not a valid top-level domain. Mail servers that treat such a name as an IP address instead of a hostname will either fail to connect or skip the target, leading to delivery issues for mail aimed at your domain.
