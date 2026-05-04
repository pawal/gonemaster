// Punycode (RFC 3492) decoder for displaying IDN domain names in their
// Unicode form alongside the ACE/punycode form. Browsers ship no built-in
// punycode API, so we inline a small decoder rather than pull in a
// dependency for a single helper.

const BASE = 36;
const T_MIN = 1;
const T_MAX = 26;
const SKEW = 38;
const DAMP = 700;
const INITIAL_BIAS = 72;
const INITIAL_N = 128;

function decodeDigit(cp: number): number {
  if (cp >= 0x30 && cp <= 0x39) return cp - 0x30 + 26; // 0-9 -> 26-35
  if (cp >= 0x41 && cp <= 0x5a) return cp - 0x41; // A-Z -> 0-25
  if (cp >= 0x61 && cp <= 0x7a) return cp - 0x61; // a-z -> 0-25
  return -1;
}

function adapt(delta: number, numPoints: number, firstTime: boolean): number {
  delta = firstTime ? Math.floor(delta / DAMP) : delta >> 1;
  delta += Math.floor(delta / numPoints);
  let k = 0;
  while (delta > ((BASE - T_MIN) * T_MAX) >> 1) {
    delta = Math.floor(delta / (BASE - T_MIN));
    k += BASE;
  }
  return k + Math.floor(((BASE - T_MIN + 1) * delta) / (delta + SKEW));
}

// Decode a single punycode-encoded label (without the "xn--" prefix).
// Returns null on malformed input so callers can fall back to the raw label.
function decodeLabel(input: string): string | null {
  const output: number[] = [];
  const delim = input.lastIndexOf("-");
  const basicEnd = delim < 0 ? 0 : delim;

  for (let i = 0; i < basicEnd; i++) {
    const cp = input.charCodeAt(i);
    if (cp >= 0x80) return null; // basic codepoints must be ASCII
    output.push(cp);
  }

  let n = INITIAL_N;
  let bias = INITIAL_BIAS;
  let i = 0;
  let pos = basicEnd > 0 ? basicEnd + 1 : 0;

  while (pos < input.length) {
    const oldI = i;
    let w = 1;
    for (let k = BASE; ; k += BASE) {
      if (pos >= input.length) return null;
      const digit = decodeDigit(input.charCodeAt(pos++));
      if (digit < 0) return null;
      if (digit > Math.floor((0x7fffffff - i) / w)) return null;
      i += digit * w;
      const t = k <= bias ? T_MIN : k >= bias + T_MAX ? T_MAX : k - bias;
      if (digit < t) break;
      if (w > Math.floor(0x7fffffff / (BASE - t))) return null;
      w *= BASE - t;
    }

    const out = output.length + 1;
    bias = adapt(i - oldI, out, oldI === 0);
    if (Math.floor(i / out) > 0x7fffffff - n) return null;
    n += Math.floor(i / out);
    i = i % out;
    output.splice(i, 0, n);
    i++;
  }

  try {
    return String.fromCodePoint(...output);
  } catch {
    return null;
  }
}

// Convenience for `title=` tooltips on hostname-bearing UI elements:
// surfaces the Unicode form for IDN names while leaving ASCII names
// alone. Returns the original input when there's nothing to decode so
// callers can pass through without conditionals.
export function idnTooltip(name: string): string {
  return idnToUnicode(name);
}

// Convert a domain name to its Unicode form. Labels without the "xn--"
// ACE prefix are returned as-is; malformed punycode labels also pass
// through unchanged so we never hide the original input.
export function idnToUnicode(domain: string): string {
  if (!domain) return domain;
  return domain
    .split(".")
    .map((label) => {
      const lower = label.toLowerCase();
      if (!lower.startsWith("xn--")) return label;
      const decoded = decodeLabel(lower.slice(4));
      return decoded ?? label;
    })
    .join(".");
}
