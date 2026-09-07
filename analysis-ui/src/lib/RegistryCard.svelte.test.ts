import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/svelte";
import RegistryCard from "./RegistryCard.svelte";
import type { DomainRegistry } from "$lib/api";

const registry = (over: Partial<DomainRegistry> = {}): DomainRegistry => ({
  state: "fresh",
  fetched_at: "2026-09-07T09:00:00Z",
  source_url: "https://rdap.example.org/domain/example.se",
  registrar: "Example Registrar",
  registry_org: "Example Registry",
  status: ["active", "client transfer prohibited"],
  registered_at: "2001-03-04T00:00:00Z",
  expires_at: "2027-03-04T00:00:00Z",
  nameservers: ["ns1.example", "ns2.example"],
  delegation_signed: true,
  ...over
});

describe("RegistryCard", () => {
  it("renders nothing when the server sent no registry block", () => {
    const { container } = render(RegistryCard, {});
    expect(container.querySelector(".registry-card")).toBeNull();
  });

  it("renders the registration details", () => {
    render(RegistryCard, { registry: registry() });
    expect(screen.getByRole("heading", { name: "Registry" })).toBeInTheDocument();
    expect(screen.getByText("Example Registrar")).toBeInTheDocument();
    expect(screen.getByText("Example Registry")).toBeInTheDocument();
    expect(screen.getByText("active, client transfer prohibited")).toBeInTheDocument();
    expect(screen.getByText("2001-03-04")).toBeInTheDocument();
    expect(screen.getByText("2027-03-04")).toBeInTheDocument();
    expect(screen.getByText("ns1.example, ns2.example")).toBeInTheDocument();
  });

  it("spells out the delegation-signed flag in both directions", () => {
    const { unmount } = render(RegistryCard, { registry: registry() });
    expect(screen.getByText("Yes")).toBeInTheDocument();
    unmount();
    render(RegistryCard, { registry: registry({ delegation_signed: false }) });
    expect(screen.getByText("No")).toBeInTheDocument();
  });

  it("credits the source with a fetch time and an outbound link", () => {
    render(RegistryCard, { registry: registry() });
    const link = screen.getByRole("link", { name: "https://rdap.example.org/domain/example.se" });
    expect(link.getAttribute("target")).toBe("_blank");
    expect(link.getAttribute("rel")).toBe("noopener noreferrer");
    expect(screen.getByText(/Retrieved 2026-09-07/)).toBeInTheDocument();
  });

  it("tells the visitor the data is on its way while it is pending", () => {
    render(RegistryCard, { registry: { state: "pending" } });
    expect(screen.getByText(/being fetched/)).toBeInTheDocument();
    expect(screen.queryByRole("link")).toBeNull();
  });

  it("says so when no registry data can be had", () => {
    render(RegistryCard, { registry: { state: "unavailable" } });
    expect(screen.getByText(/No registry data is available/)).toBeInTheDocument();
  });

  it("handles a loaded record with no usable fields", () => {
    render(RegistryCard, { registry: { state: "fresh" } });
    expect(screen.getByText(/published no details/)).toBeInTheDocument();
  });

  it("still renders a stale record", () => {
    render(RegistryCard, { registry: registry({ state: "stale" }) });
    expect(screen.getByText("Example Registrar")).toBeInTheDocument();
  });
});
