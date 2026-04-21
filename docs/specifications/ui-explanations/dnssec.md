# Public UI explanations: DNSSEC

## Testcase dnssec14

Description:

DNSSEC uses cryptographic keys to sign your zone so resolvers can verify that the answers they receive have not been tampered with. For RSA keys the size (in bits) decides how hard the key is to break. This check looks at every RSA DNSKEY you publish and compares its size against the allowed range and the current recommended size for that algorithm.

## Tag DNSKEY_TOO_SMALL_FOR_ALGO

Header: RSA DNSKEY is too small

Description:

One of your RSA signing keys is smaller than the minimum size allowed for its algorithm. A key this small is considered insecure - an attacker could plausibly forge DNSSEC signatures and redirect your domain to addresses they control. Roll the key to a larger size as soon as possible.

## Tag DNSKEY_SMALLER_THAN_REC

Header: RSA DNSKEY below recommended size

Description:

One of your RSA signing keys is within the allowed range but smaller than the currently recommended size for its algorithm. The key is not known to be insecure today, but recommended sizes go up over time as computing power grows. Plan to roll the key to a larger size before the size drops below the minimum.

## Tag DNSKEY_TOO_LARGE_FOR_ALGO

Header: RSA DNSKEY is too large

Description:

One of your RSA signing keys is larger than the maximum size allowed for its algorithm. Oversized keys make DNSSEC responses bigger than necessary, which can cause message truncation, TCP fallback, or resolvers refusing to validate your zone at all. Roll the key to a size within the allowed range.
