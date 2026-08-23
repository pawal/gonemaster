import { render, screen } from "@testing-library/svelte";
import { describe, expect, it } from "vitest";
import GradeChip from "../components/GradeChip.svelte";

// Guards the setup.js import the suite relies on instead of manual cleanup().
describe("auto-cleanup", () => {
  it("renders a component", () => {
    render(GradeChip, { props: { grade: "A", score: 92 } });
    expect(screen.getByText("A")).toBeInTheDocument();
  });

  it("leaves an empty body for the next test", () => {
    expect(document.body.innerHTML).toBe("");
  });
});
