import { describe, expect, it } from "vitest";
import { ADDED_KEY, REMOVED_KEY, layoutFlow, type FlowLink } from "./flowLayout";

const CATEGORIES = ["real", "mixed", "measurement", "unknown"];
const tight = { width: 200, height: 100, nodeWidth: 10, labelWidth: 20, gap: 2, padY: 0 };

function node(layout: ReturnType<typeof layoutFlow>, side: "from" | "to", key: string) {
  const found = layout.nodes.find((n) => n.side === side && n.key === key);
  if (!found) throw new Error(`no ${side} node ${key}`);
  return found;
}

describe("layoutFlow", () => {
  const links: FlowLink[] = [
    { from: "A", to: "A", category: "", count: 6 },
    { from: "A", to: "B", category: "real", count: 2 },
    { from: "B", to: "B", category: "", count: 2 }
  ];

  it("sizes nodes by the domains through them", () => {
    const layout = layoutFlow(["A", "B"], links, CATEGORIES, tight);
    expect(layout.nodes.map((n) => [n.side, n.key, n.y, n.height, n.count])).toEqual([
      ["from", "A", 0, 78.4, 8],
      ["from", "B", 80.4, 19.6, 2],
      ["to", "A", 0, 58.8, 6],
      ["to", "B", 60.8, 39.2, 4]
    ]);
    expect(layout.height).toBe(100);
  });

  it("stacks slots in the other end's order", () => {
    const layout = layoutFlow(["A", "B"], links, CATEGORIES, tight);
    const moved = layout.ribbons.find((r) => r.from === "A" && r.to === "B");
    expect(moved?.path).toBe("M 30 58.8 C 100 58.8 100 60.8 170 60.8 L 170 80.4 C 100 80.4 100 78.4 30 78.4 Z");
    expect(moved?.held).toBe(false);
    expect(layout.ribbons.filter((r) => r.held).map((r) => r.from)).toEqual(["A", "B"]);
  });

  it("orders slots of one pair by category", () => {
    const layout = layoutFlow(
      ["A", "B"],
      [
        { from: "A", to: "B", category: "measurement", count: 1 },
        { from: "A", to: "B", category: "real", count: 1 }
      ],
      CATEGORIES,
      tight
    );
    const top = (category: string) =>
      Number(layout.ribbons.find((r) => r.category === category)?.path.split(" ")[2]);
    expect(top("real")).toBe(0);
    expect(top("measurement")).toBe(50);
  });

  it("keeps one domain visible and pushes crowded labels apart", () => {
    const layout = layoutFlow(
      ["A", "F"],
      [
        { from: "A", to: "A", category: "", count: 998 },
        { from: "A", to: "F", category: "real", count: 1 },
        { from: "F", to: "F", category: "", count: 1 }
      ],
      CATEGORIES
    );
    expect(node(layout, "from", "F").height).toBe(2);
    expect(node(layout, "to", "F").height).toBe(2);
    expect(node(layout, "to", "F").labelY - node(layout, "to", "A").labelY).toBeGreaterThanOrEqual(13);
  });

  it("adds end nodes for domains that appeared or left", () => {
    const layout = layoutFlow(
      ["A"],
      [
        { from: "A", to: "A", category: "", count: 5 },
        { from: ADDED_KEY, to: "A", category: "", count: 3 },
        { from: "A", to: REMOVED_KEY, category: "", count: 1 }
      ],
      CATEGORIES,
      tight
    );
    expect(layout.nodes.map((n) => `${n.side}:${n.key}:${n.count}`)).toEqual([
      "from:A:6",
      "from:new:3",
      "to:A:8",
      "to:removed:1"
    ]);
  });

  it("drops links whose ends are not in the order and counts their domains", () => {
    const layout = layoutFlow(
      ["A"],
      [
        { from: "A", to: "A", category: "", count: 5 },
        { from: "", to: "A", category: "unknown", count: 2 }
      ],
      CATEGORIES,
      tight
    );
    expect(layout.dropped).toBe(2);
    expect(layout.ribbons).toHaveLength(1);
  });

  it("returns nothing to draw for no links", () => {
    const layout = layoutFlow(["A", "B"], [], CATEGORIES, tight);
    expect(layout.nodes).toEqual([]);
    expect(layout.ribbons).toEqual([]);
    expect(layout.height).toBe(0);
  });
});
