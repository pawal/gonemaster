import { render } from "@testing-library/svelte";
import { describe, expect, it } from "vitest";
import GradeChip from "./GradeChip.svelte";

describe("GradeChip", () => {
  it("renders the grade letter with its data-grade attribute and the bare score", () => {
    const { container } = render(GradeChip, { props: { grade: "A", score: 92 } });
    const letter = container.querySelector(".grade-chip-letter");
    expect(letter).not.toBeNull();
    expect(letter.getAttribute("data-grade")).toBe("A");
    expect(letter.textContent).toBe("A");
    expect(container.querySelector(".grade-chip-score").textContent).toBe("92");
  });

  it("appends /100 only when showDenominator is set", () => {
    const { container } = render(GradeChip, { props: { grade: "B", score: 80, showDenominator: true } });
    expect(container.querySelector(".grade-chip-score").textContent).toBe("80/100");
  });

  it("renders nothing when grade or score is missing", () => {
    const { container: a } = render(GradeChip, { props: { grade: null, score: 50 } });
    expect(a.querySelector(".grade-chip")).toBeNull();
    const { container: b } = render(GradeChip, { props: { grade: "A", score: null } });
    expect(b.querySelector(".grade-chip")).toBeNull();
  });

  it("treats a zero score as present", () => {
    const { container } = render(GradeChip, { props: { grade: "F", score: 0 } });
    expect(container.querySelector(".grade-chip")).not.toBeNull();
    expect(container.querySelector(".grade-chip-score").textContent).toBe("0");
  });
});
