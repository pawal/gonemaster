import { render, screen, cleanup } from "@testing-library/svelte";
import { afterEach, describe, expect, it } from "vitest";
import InlineNotice from "./InlineNotice.svelte";

describe("InlineNotice", () => {
  afterEach(() => cleanup());

  it("renders nothing when there is no message", () => {
    const { container } = render(InlineNotice, { props: { message: "" } });
    expect(container.querySelector(".inline-notice")).toBeNull();
  });

  it("renders an ok notice with a polite live region", () => {
    const { container } = render(InlineNotice, { props: { message: "Saved", tone: "ok" } });
    const el = container.querySelector(".inline-notice");
    expect(el).toHaveClass("inline-notice-ok");
    expect(el).toHaveAttribute("aria-live", "polite");
    expect(screen.getByText("Saved")).toBeInTheDocument();
  });

  it("defaults to the warn tone for any non-ok tone", () => {
    const { container } = render(InlineNotice, { props: { message: "Oops", tone: "warn" } });
    expect(container.querySelector(".inline-notice")).toHaveClass("inline-notice-warn");
    const { container: c2 } = render(InlineNotice, { props: { message: "Oops" } });
    expect(c2.querySelector(".inline-notice")).toHaveClass("inline-notice-warn");
  });
});
