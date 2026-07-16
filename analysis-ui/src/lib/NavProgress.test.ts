import { describe, expect, it } from "vitest";
import { render } from "@testing-library/svelte";
import NavProgress from "./NavProgress.svelte";

describe("NavProgress", () => {
  it("carries the active class while a navigation is pending", () => {
    const { container } = render(NavProgress, { active: true });
    const bar = container.querySelector(".nav-progress");
    expect(bar?.classList.contains("active")).toBe(true);
  });

  it("drops the active class when idle so the bar is hidden", () => {
    const { container } = render(NavProgress, { active: false });
    const bar = container.querySelector(".nav-progress");
    expect(bar).not.toBeNull();
    expect(bar?.classList.contains("active")).toBe(false);
  });

  it("hides the bar from assistive tech", () => {
    const { container } = render(NavProgress, { active: true });
    expect(container.querySelector(".nav-progress")?.getAttribute("aria-hidden")).toBe("true");
  });
});
