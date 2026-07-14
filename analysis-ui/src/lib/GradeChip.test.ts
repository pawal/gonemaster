import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/svelte";
import GradeChip from "./GradeChip.svelte";

describe("GradeChip", () => {
  it("renders the grade letter and keys colour via data-grade", () => {
    const { container } = render(GradeChip, { grade: "A+" });
    const letter = container.querySelector(".grade-chip-letter");
    // The data-grade attribute is what the CSS palette selects on, so it
    // must carry the normalized grade string verbatim.
    expect(letter?.getAttribute("data-grade")).toBe("A+");
    expect(letter?.textContent).toBe("A+");
  });

  it("normalizes lowercase and whitespace before rendering", () => {
    const { container } = render(GradeChip, { grade: "  b  " });
    const letter = container.querySelector(".grade-chip-letter");
    expect(letter?.getAttribute("data-grade")).toBe("B");
    expect(letter?.textContent).toBe("B");
  });

  it("shows the score when supplied", () => {
    render(GradeChip, { grade: "C", score: 72 });
    expect(screen.getByText("72")).toBeInTheDocument();
  });

  it("appends the /100 denominator only when asked", () => {
    render(GradeChip, { grade: "C", score: 72, showDenominator: true });
    expect(screen.getByText("72/100")).toBeInTheDocument();
  });

  it("omits the score span when no score is given", () => {
    const { container } = render(GradeChip, { grade: "A" });
    expect(container.querySelector(".grade-chip-score")).toBeNull();
  });

  it("renders nothing when the grade is null or blank", () => {
    const nullChip = render(GradeChip, { grade: null });
    expect(nullChip.container.querySelector(".grade-chip")).toBeNull();

    const blankChip = render(GradeChip, { grade: "   " });
    expect(blankChip.container.querySelector(".grade-chip")).toBeNull();
  });

  it("treats score 0 as a real value, not absence", () => {
    render(GradeChip, { grade: "F", score: 0 });
    expect(screen.getByText("0")).toBeInTheDocument();
  });
});
