import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/svelte";
import PostureChip from "./PostureChip.svelte";

describe("PostureChip", () => {
  it("renders the label in the tone the server assigned", () => {
    const { container } = render(PostureChip, {
      posture: "nsec3",
      label: "NSEC3",
      tone: "ok"
    });
    const chip = container.querySelector(".posture-chip");
    expect(chip?.textContent?.trim()).toBe("NSEC3");
    expect(chip?.classList.contains("tone-ok")).toBe(true);
    expect(chip?.getAttribute("title")).toBe("Denial-of-existence posture: NSEC3");
  });

  it("falls back to the raw key when the server sent no label", () => {
    render(PostureChip, { posture: "mixed" });
    expect(screen.getByText("mixed")).toBeInTheDocument();
  });

  it("uses the neutral tone when the server sent no tone", () => {
    const { container } = render(PostureChip, { posture: "nsec" });
    expect(container.querySelector(".posture-chip")?.classList.contains("tone-neutral")).toBe(true);
  });

  it("renders a dash when the snapshot carries no posture", () => {
    const { container } = render(PostureChip, {});
    expect(container.querySelector(".posture-chip")).toBeNull();
    expect(container.querySelector(".posture-none")?.textContent).toBe("-");
  });
});
