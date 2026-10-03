import { render, screen } from "@testing-library/svelte";
import { describe, expect, it } from "vitest";
import AsnNames from "./AsnNames.svelte";

const holders = new Map([
  [199973, { asn: 199973, name: "Migrationsverket", handle: "MIGR-AS", country: "SE", label: "MIGR-AS - Migrationsverket, SE" }],
  [64500, { asn: 64500, name: "Example Org", handle: "", country: "", label: "Example Org" }],
]);

describe("AsnNames", () => {
  it("renders the AS number, name and country with the raw label as title", () => {
    render(AsnNames, { props: { asns: [199973], names: holders } });
    const item = screen.getByText("AS199973: Migrationsverket (SE)");
    expect(item.getAttribute("title")).toBe("MIGR-AS - Migrationsverket, SE");
    expect(screen.getByTestId("asn-names").getAttribute("aria-label")).toBe("Registered AS holders");
  });

  it("omits the country when there is none", () => {
    render(AsnNames, { props: { asns: [64500], names: holders } });
    expect(screen.getByTestId("asn-names").textContent.trim()).toBe("AS64500: Example Org");
  });

  it("omits an AS without a name", () => {
    render(AsnNames, { props: { asns: [1299, 199973], names: holders } });
    expect(screen.getAllByRole("listitem").map((li) => li.textContent)).toEqual(["AS199973: Migrationsverket (SE)"]);
  });

  it.each([
    ["no AS has a name", [1299]],
    ["the list is empty", []],
  ])("renders nothing when %s", (_, asns) => {
    render(AsnNames, { props: { asns, names: holders } });
    expect(screen.queryByTestId("asn-names")).toBeNull();
  });
});
