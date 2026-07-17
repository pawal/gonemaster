import { render, screen, fireEvent, cleanup } from "@testing-library/svelte";
import { afterEach, describe, expect, it } from "vitest";
import GlossaryTerm from "./GlossaryTerm.svelte";

// These render against the real bundled en.json, so the definitions come
// from the synced pub.glossary.* keys.
describe("GlossaryTerm", () => {
  afterEach(() => cleanup());

  it("renders the term text and carries the definition in aria-label", () => {
    render(GlossaryTerm, { props: { term: "delegation", slug: "delegation" } });
    const trigger = screen.getByTestId("glossary-term");
    expect(trigger.tagName).toBe("BUTTON");
    expect(trigger.textContent).toBe("delegation");
    expect(trigger.getAttribute("aria-label")).toMatch(
      /^delegation: A delegation is the pointer in the parent zone/,
    );
  });

  it("does not render the tooltip until hovered or focused", () => {
    render(GlossaryTerm, { props: { term: "delegation", slug: "delegation" } });
    expect(screen.queryByTestId("glossary-tip")).toBeNull();
  });

  it("shows the tooltip on hover and hides on leave", async () => {
    render(GlossaryTerm, { props: { term: "delegation", slug: "delegation" } });
    const trigger = screen.getByTestId("glossary-term");
    await fireEvent.mouseEnter(trigger);
    const tip = screen.getByTestId("glossary-tip");
    expect(tip.getAttribute("role")).toBe("tooltip");
    expect(tip.textContent).toMatch(/pointer in the parent zone/i);
    await fireEvent.mouseLeave(trigger);
    expect(screen.queryByTestId("glossary-tip")).toBeNull();
  });

  it("shows the tooltip on keyboard focus", async () => {
    render(GlossaryTerm, { props: { term: "glue", slug: "glue" } });
    await fireEvent.focus(screen.getByTestId("glossary-term"));
    expect(screen.getByTestId("glossary-tip").textContent).toMatch(
      /nameserver's IP address/i,
    );
  });
});
