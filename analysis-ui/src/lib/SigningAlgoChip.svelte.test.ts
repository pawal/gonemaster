import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/svelte";
import SigningAlgoChip from "./SigningAlgoChip.svelte";

describe("SigningAlgoChip", () => {
  it("renders the mnemonic in the tone the server assigned", () => {
    const { container } = render(SigningAlgoChip, {
      algo: 5,
      label: "RSASHA1",
      tone: "error"
    });
    const chip = container.querySelector(".algo-chip");
    expect(chip?.textContent?.trim()).toBe("RSASHA1");
    expect(chip?.classList.contains("tone-error")).toBe(true);
    expect(chip?.getAttribute("title")).toBe("DNSKEY algorithm 5");
  });

  it("falls back to the algorithm number when the server sent no label", () => {
    render(SigningAlgoChip, { algo: 200 });
    expect(screen.getByText("ALGO 200")).toBeInTheDocument();
  });

  it("uses the neutral tone when the server sent no tone", () => {
    const { container } = render(SigningAlgoChip, { algo: 200 });
    expect(container.querySelector(".algo-chip")?.classList.contains("tone-neutral")).toBe(true);
  });

  it("renders a dash for an unsigned domain", () => {
    const { container } = render(SigningAlgoChip, {});
    expect(container.querySelector(".algo-chip")).toBeNull();
    expect(container.querySelector(".algo-none")?.textContent).toBe("-");
  });
});
