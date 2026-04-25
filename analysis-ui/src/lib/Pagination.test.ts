import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/svelte";
import Pagination from "./Pagination.svelte";

vi.mock("$app/navigation", () => ({ goto: vi.fn() }));
vi.mock("$app/state", () => ({
  page: { url: new URL("http://localhost/domains?dataset_tag=tld") }
}));

describe("Pagination", () => {
  it("disables Previous and enables Next on the first page", () => {
    render(Pagination, { total: 100, offset: 0, limit: 25, itemCount: 25 });
    expect(screen.getByRole("button", { name: /previous/i })).toBeDisabled();
    expect(screen.getByRole("button", { name: /next/i })).toBeEnabled();
  });

  it("enables Previous and disables Next on the last page", () => {
    render(Pagination, { total: 100, offset: 75, limit: 25, itemCount: 25 });
    expect(screen.getByRole("button", { name: /previous/i })).toBeEnabled();
    expect(screen.getByRole("button", { name: /next/i })).toBeDisabled();
  });

  it("disables both buttons when all items fit on one page", () => {
    render(Pagination, { total: 10, offset: 0, limit: 25, itemCount: 10 });
    expect(screen.getByRole("button", { name: /previous/i })).toBeDisabled();
    expect(screen.getByRole("button", { name: /next/i })).toBeDisabled();
  });

  it("shows the correct item range for a mid-list page", () => {
    render(Pagination, { total: 100, offset: 25, limit: 25, itemCount: 25 });
    expect(screen.getByText(/Showing 26–50 of 100/)).toBeInTheDocument();
  });

  it("shows 0–0 for empty results", () => {
    render(Pagination, { total: 0, offset: 0, limit: 25, itemCount: 0 });
    expect(screen.getByText(/Showing 0–0 of 0/)).toBeInTheDocument();
  });

  it("formats large totals with thousands separator", () => {
    render(Pagination, { total: 1500, offset: 0, limit: 25, itemCount: 25 });
    expect(screen.getByText(/of 1,500/)).toBeInTheDocument();
  });
});
