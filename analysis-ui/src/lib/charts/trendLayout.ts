// Pure layout for the hand-rolled SVG trend charts: numeric series in, SVG
// geometry out. Kept DOM-free so the geometry can be unit tested without a
// browser, mirroring the dnssecChainLayout split used by the public UI.

export type ChartPoint = { x: number; y: number; value: number; index: number };

export type Tick = { value: number; y: number };

export type LineLayout = {
  width: number;
  height: number;
  points: ChartPoint[];
  // "M x y L x y ..." through every data point.
  linePath: string;
  // Closed path from the baseline for an optional area fill.
  areaPath: string;
  yMin: number;
  yMax: number;
  yTicks: Tick[];
  baselineY: number;
};

export type LineOptions = {
  width?: number;
  height?: number;
  padding?: { top?: number; right?: number; bottom?: number; left?: number };
  // Force a y-domain, e.g. [0, 100] for percentage series. When omitted the
  // domain runs from 0 to a "nice" rounded maximum of the data.
  yDomain?: [number, number];
  tickCount?: number;
};

const DEFAULTS = {
  width: 640,
  height: 200,
  padding: { top: 12, right: 12, bottom: 24, left: 40 },
  tickCount: 4
};

function round(n: number): number {
  return Math.round(n * 100) / 100;
}

// Classic Heckbert "nice numbers" for axis labelling: round a range to a
// 1/2/5 x 10^n value so ticks land on readable numbers.
function niceNum(range: number, round: boolean): number {
  if (range <= 0) return 1;
  const exp = Math.floor(Math.log10(range));
  const frac = range / Math.pow(10, exp);
  let nice: number;
  if (round) {
    if (frac < 1.5) nice = 1;
    else if (frac < 3) nice = 2;
    else if (frac < 7) nice = 5;
    else nice = 10;
  } else {
    if (frac <= 1) nice = 1;
    else if (frac <= 2) nice = 2;
    else if (frac <= 5) nice = 5;
    else nice = 10;
  }
  return nice * Math.pow(10, exp);
}

// Resolve the [min, max] domain for a set of values. Counts start at 0; an
// explicit domain (e.g. percentages) is passed through untouched.
export function resolveDomain(values: number[], override?: [number, number]): [number, number] {
  if (override) return override;
  const finite = values.filter((v) => Number.isFinite(v));
  const max = finite.length ? Math.max(...finite) : 0;
  if (max <= 0) return [0, 1];
  return [0, niceNum(max, false)];
}

function buildTicks(yMin: number, yMax: number, count: number, scaleY: (v: number) => number): Tick[] {
  if (yMax <= yMin) return [{ value: yMin, y: scaleY(yMin) }];
  const step = niceNum((yMax - yMin) / Math.max(1, count), true);
  const ticks: Tick[] = [];
  // Iterate on an integer counter to avoid floating-point drift accumulating
  // across additions.
  const start = Math.ceil(yMin / step);
  const end = Math.floor(yMax / step);
  for (let i = start; i <= end; i++) {
    const value = round(i * step);
    ticks.push({ value, y: scaleY(value) });
  }
  return ticks;
}

// Lay out a single numeric series as a line/area over an SVG viewBox.
export function layoutLine(values: number[], options: LineOptions = {}): LineLayout {
  const width = options.width ?? DEFAULTS.width;
  const height = options.height ?? DEFAULTS.height;
  const pad = {
    top: options.padding?.top ?? DEFAULTS.padding.top,
    right: options.padding?.right ?? DEFAULTS.padding.right,
    bottom: options.padding?.bottom ?? DEFAULTS.padding.bottom,
    left: options.padding?.left ?? DEFAULTS.padding.left
  };
  const tickCount = options.tickCount ?? DEFAULTS.tickCount;

  const plotW = Math.max(0, width - pad.left - pad.right);
  const plotH = Math.max(0, height - pad.top - pad.bottom);

  const [yMin, yMax] = resolveDomain(values, options.yDomain);
  const span = yMax - yMin || 1;

  const scaleY = (v: number) => round(pad.top + (1 - (v - yMin) / span) * plotH);
  const scaleX = (i: number) => {
    if (values.length <= 1) return round(pad.left + plotW / 2);
    return round(pad.left + (i / (values.length - 1)) * plotW);
  };

  const points: ChartPoint[] = values.map((value, index) => ({
    index,
    value,
    x: scaleX(index),
    y: scaleY(Number.isFinite(value) ? value : yMin)
  }));

  const baselineY = scaleY(yMin);

  let linePath = "";
  let areaPath = "";
  if (points.length === 1) {
    // A single sample has no segment; expose the point coordinate so the
    // renderer can still draw a dot.
    linePath = `M ${points[0].x} ${points[0].y}`;
  } else if (points.length > 1) {
    linePath = points.map((p, i) => `${i === 0 ? "M" : "L"} ${p.x} ${p.y}`).join(" ");
    const first = points[0];
    const last = points[points.length - 1];
    areaPath = `${linePath} L ${last.x} ${baselineY} L ${first.x} ${baselineY} Z`;
  }

  return {
    width,
    height,
    points,
    linePath,
    areaPath,
    yMin,
    yMax,
    yTicks: buildTicks(yMin, yMax, tickCount, scaleY),
    baselineY
  };
}

export type SparklineLayout = {
  width: number;
  height: number;
  points: ChartPoint[];
  linePath: string;
  areaPath: string;
  last: ChartPoint | null;
  // Sign of (last - first): 1 rising, -1 falling, 0 flat/insufficient data.
  direction: -1 | 0 | 1;
};

// Compact, axis-free line for stat tiles. Reuses layoutLine with zero padding.
export function layoutSparkline(values: number[], options: LineOptions = {}): SparklineLayout {
  const width = options.width ?? 96;
  const height = options.height ?? 24;
  const layout = layoutLine(values, {
    width,
    height,
    padding: { top: 2, right: 2, bottom: 2, left: 2 },
    tickCount: 0,
    yDomain: options.yDomain
  });
  const finite = values.filter((v) => Number.isFinite(v));
  let direction: -1 | 0 | 1 = 0;
  if (finite.length >= 2) {
    const delta = finite[finite.length - 1] - finite[0];
    direction = delta > 0 ? 1 : delta < 0 ? -1 : 0;
  }
  return {
    width,
    height,
    points: layout.points,
    linePath: layout.linePath,
    areaPath: layout.areaPath,
    last: layout.points.length ? layout.points[layout.points.length - 1] : null,
    direction
  };
}
