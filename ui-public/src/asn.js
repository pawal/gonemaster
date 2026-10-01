const ASN_MAX = 4294967295;

// Unique AS numbers of one finding from its asn and asns args, ascending.
export function asnsOf(args) {
  const out = new Set();
  for (const v of [args?.asn, args?.asns].flat()) {
    if (Number.isInteger(v) && v >= 1 && v <= ASN_MAX) out.add(v);
  }
  return [...out].sort((a, b) => a - b);
}
