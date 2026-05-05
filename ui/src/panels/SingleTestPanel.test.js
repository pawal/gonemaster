import { render, screen, fireEvent, waitFor, cleanup } from "@testing-library/svelte";
import { afterEach, describe, expect, it, vi } from "vitest";
import SingleTestPanel from "./SingleTestPanel.svelte";

describe("SingleTestPanel", () => {
  afterEach(() => cleanup());

  it("renders the form with a Run Single Job button", () => {
    render(SingleTestPanel, { props: { apiFetch: vi.fn() } });
    expect(screen.getByPlaceholderText("example.com")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Run Single Job/i })).toBeInTheDocument();
  });

  it("warns and does not submit when the domain is empty", async () => {
    const apiFetch = vi.fn();
    const setStatus = vi.fn();
    render(SingleTestPanel, { props: { apiFetch, setStatus } });
    await fireEvent.click(screen.getByRole("button", { name: /Run Single Job/i }));
    expect(setStatus).toHaveBeenCalledWith(expect.any(String), "warn");
    expect(apiFetch).not.toHaveBeenCalled();
  });

  it("submits a job and invokes onJobCreated with the new id", async () => {
    const apiFetch = vi.fn().mockResolvedValue({ id: "job_new" });
    const onJobCreated = vi.fn();
    render(SingleTestPanel, { props: { apiFetch, onJobCreated } });
    await fireEvent.input(screen.getByPlaceholderText("example.com"), {
      target: { value: "example.com" },
    });
    await fireEvent.click(screen.getByRole("button", { name: /Run Single Job/i }));
    await waitFor(() => expect(apiFetch).toHaveBeenCalledWith("/jobs", expect.objectContaining({ method: "POST" })));
    expect(onJobCreated).toHaveBeenCalledWith("job_new");
  });

  it("includes parsed tags and selected profile in the request payload", async () => {
    const apiFetch = vi.fn().mockResolvedValue({ id: "job_new" });
    render(SingleTestPanel, {
      props: {
        apiFetch,
        availableProfiles: [{ id: 7, name: "strict" }],
      },
    });
    await fireEvent.input(screen.getByPlaceholderText("example.com"), { target: { value: "example.com" } });
    await fireEvent.input(screen.getByLabelText(/Tags/i), { target: { value: "tag-a, tag-b" } });
    const profileSelect = screen.getByLabelText(/Stored profile/i);
    await fireEvent.change(profileSelect, { target: { value: "7" } });
    await fireEvent.click(screen.getByRole("button", { name: /Run Single Job/i }));
    await waitFor(() => expect(apiFetch).toHaveBeenCalled());
    const body = JSON.parse(apiFetch.mock.calls[0][1].body);
    expect(body.domain).toBe("example.com");
    expect(body.tags).toEqual(["tag-a", "tag-b"]);
    expect(body.profile_id).toBe(7);
  });

  it("adds an undelegated nameserver row when the button is clicked", async () => {
    render(SingleTestPanel, { props: { apiFetch: vi.fn() } });
    await fireEvent.click(screen.getByRole("button", { name: /Add nameserver/i }));
    expect(screen.getByPlaceholderText("ns1.example.com")).toBeInTheDocument();
  });
});
