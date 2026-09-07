import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/svelte";
import ExternalLinks from "./ExternalLinks.svelte";
import type { ExternalLink } from "$lib/externalLinks";

const link = (over: Partial<ExternalLink> = {}): ExternalLink => ({
  label: "RIPEstat",
  href: "https://stat.ripe.net/AS64500",
  title: "Routing and registry data at RIPEstat",
  ...over
});

describe("ExternalLinks", () => {
  it("renders one link per entry inside a labelled nav", () => {
    render(ExternalLinks, {
      links: [link(), link({ label: "IANA root zone", href: "https://www.iana.org/x" })]
    });
    const nav = screen.getByRole("navigation", { name: "External references" });
    expect(nav).toBeInTheDocument();
    expect(screen.getByText("Elsewhere")).toBeInTheDocument();
    expect(screen.getAllByRole("link")).toHaveLength(2);
  });

  it("opens every link in a new tab without leaking the referrer", () => {
    render(ExternalLinks, { links: [link()] });
    const anchor = screen.getByRole("link", { name: "RIPEstat" });
    expect(anchor.getAttribute("href")).toBe("https://stat.ripe.net/AS64500");
    expect(anchor.getAttribute("target")).toBe("_blank");
    expect(anchor.getAttribute("rel")).toBe("noopener noreferrer");
    expect(anchor.getAttribute("title")).toBe("Routing and registry data at RIPEstat");
  });

  it("renders nothing at all for an empty link list", () => {
    const { container } = render(ExternalLinks, { links: [] });
    expect(container.querySelector(".external-links")).toBeNull();
    expect(screen.queryByRole("navigation")).toBeNull();
  });

  it("accepts a caller-supplied heading", () => {
    render(ExternalLinks, { links: [link()], heading: "References" });
    expect(screen.getByText("References")).toBeInTheDocument();
  });
});
