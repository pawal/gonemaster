import { render, screen } from "@testing-library/svelte";
import { describe, expect, it } from "vitest";
import GlossaryTerm from "../lib/GlossaryTerm.svelte";

// Guards the setup.js import the suite relies on instead of manual cleanup().
describe("auto-cleanup", () => {
  it("renders a component", () => {
    render(GlossaryTerm, { props: { term: "delegation", slug: "delegation" } });
    expect(screen.getByTestId("glossary-term")).toBeInTheDocument();
  });

  it("leaves an empty body for the next test", () => {
    expect(document.body.innerHTML).toBe("");
  });
});
