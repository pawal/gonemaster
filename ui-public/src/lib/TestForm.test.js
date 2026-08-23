import { render, screen, fireEvent, waitFor } from "@testing-library/svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import TestForm from "./TestForm.svelte";
import { errorResponse, jsonResponse } from "../test/helpers.js";

const okResponse = (data, status = 201) => jsonResponse(data, status);

const errResponse = (status, body = {}) => errorResponse(status, body, { "Retry-After": "30" });

describe("TestForm", () => {
  beforeEach(() => {
    global.fetch = vi.fn();
  });

  // Renders the form, types a domain and submits it.
  const submitDomain = async (domain) => {
    render(TestForm);
    await fireEvent.input(screen.getByLabelText("Domain"), { target: { value: domain } });
    await fireEvent.click(screen.getByRole("button", { name: "Test" }));
  };

  // Rendering

  it("renders domain input", () => {
    render(TestForm);
    expect(screen.getByLabelText("Domain")).toBeTruthy();
  });

  it("renders the Test button", () => {
    render(TestForm);
    expect(screen.getByRole("button", { name: "Test" })).toBeTruthy();
  });

  it("renders collapsible Options section", () => {
    render(TestForm);
    expect(screen.getByText("Options")).toBeTruthy();
  });

  it("does not show error initially", () => {
    render(TestForm);
    expect(screen.queryByRole("alert")).toBeNull();
  });

  // Validation

  it("shows required error when submitting with empty domain", async () => {
    render(TestForm);
    await fireEvent.click(screen.getByRole("button", { name: "Test" }));
    expect(screen.getByRole("alert")).toBeTruthy();
    expect(fetch).not.toHaveBeenCalled();
  });

  it("clears error when domain is valid and submission starts", async () => {
    global.fetch.mockResolvedValue(okResponse({ public_id: "abc12345" }));
    render(TestForm);
    // Trigger validation error first
    await fireEvent.click(screen.getByRole("button", { name: "Test" }));
    expect(screen.getByRole("alert")).toBeTruthy();
    // Now type a domain and submit
    await fireEvent.input(screen.getByLabelText("Domain"), {
      target: { value: "example.com" },
    });
    await fireEvent.click(screen.getByRole("button", { name: "Test" }));
    await waitFor(() => expect(screen.queryByRole("alert")).toBeNull());
  });

  // Successful submission

  it("calls createJob with the trimmed domain", async () => {
    global.fetch.mockResolvedValue(okResponse({ public_id: "abc12345" }));
    await submitDomain("  example.com  ");
    await waitFor(() => expect(fetch).toHaveBeenCalled());
    const body = JSON.parse(fetch.mock.calls[0][1].body);
    expect(body.domain).toBe("example.com");
  });

  it("dispatches jobcreated event with publicID on success", async () => {
    global.fetch.mockResolvedValue(okResponse({ public_id: "abc12345" }));
    const handler = vi.fn();
    render(TestForm, { props: { onjobcreated: handler } });
    await fireEvent.input(screen.getByLabelText("Domain"), {
      target: { value: "example.com" },
    });
    await fireEvent.click(screen.getByRole("button", { name: "Test" }));
    await waitFor(() => expect(handler).toHaveBeenCalledOnce());
    expect(handler.mock.calls[0][0].publicID).toBe("abc12345");
  });

  // Error responses

  it("shows rate limit error on 429", async () => {
    global.fetch.mockResolvedValue(errResponse(429));
    await submitDomain("example.com");
    await waitFor(() => expect(screen.getByRole("alert")).toBeTruthy());
    expect(screen.getByRole("alert").textContent).toMatch(/30/);
  });

  it("shows domain invalid error on 400 with invalid_domain code", async () => {
    global.fetch.mockResolvedValue(
      errResponse(400, { error: { code: "invalid_domain", message: "bad" } })
    );
    await submitDomain("notvalid");
    await waitFor(() => expect(screen.getByRole("alert")).toBeTruthy());
  });

  it("shows generic error on other non-ok response", async () => {
    global.fetch.mockResolvedValue(errResponse(500, {}));
    await submitDomain("example.com");
    await waitFor(() => expect(screen.getByRole("alert")).toBeTruthy());
  });

  it("shows network error when fetch throws", async () => {
    global.fetch.mockRejectedValue(new Error("network down"));
    await submitDomain("example.com");
    await waitFor(() => expect(screen.getByRole("alert")).toBeTruthy());
  });

  // NS rows

  it("adds an NS row when Add nameserver is clicked", async () => {
    render(TestForm);
    expect(screen.queryAllByTestId("ns-row")).toHaveLength(0);
    await fireEvent.click(screen.getByTestId("add-ns"));
    expect(screen.getAllByTestId("ns-row")).toHaveLength(1);
  });

  it("removes an NS row when Remove is clicked", async () => {
    render(TestForm);
    await fireEvent.click(screen.getByTestId("add-ns"));
    await fireEvent.click(screen.getByTestId("add-ns"));
    expect(screen.getAllByTestId("ns-row")).toHaveLength(2);
    await fireEvent.click(screen.getAllByRole("button", { name: "Remove" })[0]);
    expect(screen.getAllByTestId("ns-row")).toHaveLength(1);
  });

  // DS rows

  it("adds a DS row when Add DS record is clicked", async () => {
    render(TestForm);
    expect(screen.queryAllByTestId("ds-row")).toHaveLength(0);
    await fireEvent.click(screen.getByTestId("add-ds"));
    expect(screen.getAllByTestId("ds-row")).toHaveLength(1);
  });

  it("removes a DS row when Remove is clicked", async () => {
    render(TestForm);
    await fireEvent.click(screen.getByTestId("add-ds"));
    await fireEvent.click(screen.getByTestId("add-ds"));
    expect(screen.getAllByTestId("ds-row")).toHaveLength(2);
    const removes = screen.getAllByRole("button", { name: "Remove" });
    await fireEvent.click(removes[removes.length - 1]);
    expect(screen.getAllByTestId("ds-row")).toHaveLength(1);
  });

  // Fetch from parent

  it("fetch from parent populates NS and DS rows", async () => {
    global.fetch.mockResolvedValue(jsonResponse({
      nameservers: [
        { ns: "ns1.example.com", ip: "192.0.2.1" },
        { ns: "ns2.example.com", ip: "198.51.100.1" },
      ],
      ds_records: [
        { keytag: 12345, algorithm: 13, digtype: 2, digest: "abcdef" },
      ],
    }));
    render(TestForm);
    await fireEvent.input(screen.getByLabelText("Domain"), {
      target: { value: "example.com" },
    });
    await fireEvent.click(screen.getByTestId("fetch-ns"));
    await waitFor(() =>
      expect(screen.getAllByTestId("ns-row")).toHaveLength(2)
    );
    expect(screen.getAllByTestId("ds-row")).toHaveLength(1);
  });

  it("fetch from parent button is disabled when domain is empty", () => {
    render(TestForm);
    expect(screen.getByTestId("fetch-ns").disabled).toBe(true);
  });

  // Reset form

  it("reset clears domain, NS rows, DS rows, and errors", async () => {
    render(TestForm);
    // Type a domain
    await fireEvent.input(screen.getByLabelText("Domain"), {
      target: { value: "example.com" },
    });
    // Add NS and DS rows
    await fireEvent.click(screen.getByTestId("add-ns"));
    await fireEvent.click(screen.getByTestId("add-ds"));
    expect(screen.getAllByTestId("ns-row")).toHaveLength(1);
    expect(screen.getAllByTestId("ds-row")).toHaveLength(1);
    // Click reset
    await fireEvent.click(screen.getByTestId("reset-form"));
    // Domain cleared
    expect(screen.getByLabelText("Domain").value).toBe("");
    // NS and DS rows removed
    expect(screen.queryAllByTestId("ns-row")).toHaveLength(0);
    expect(screen.queryAllByTestId("ds-row")).toHaveLength(0);
  });

  // Button state during submission

  it("shows Testing… while submitting", async () => {
    let resolve;
    global.fetch.mockReturnValue(new Promise((r) => { resolve = r; }));
    render(TestForm);
    await fireEvent.input(screen.getByLabelText("Domain"), {
      target: { value: "example.com" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Test" }));
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: "Testing…" })).toBeTruthy()
    );
    resolve(okResponse({ public_id: "x" }));
  });

  // Prefill from results callout

  it("prefills, flashes, and submits the test in one go when prefillSignal is bumped", async () => {
    global.fetch.mockResolvedValue(okResponse({ public_id: "abc12345" }));
    const onjobcreated = vi.fn();
    const { rerender } = render(TestForm, { props: { prefillDomain: "eosc.ch", prefillSignal: 0, onjobcreated } });
    const input = screen.getByLabelText("Domain");
    expect(input.value).toBe("");
    await rerender({ prefillDomain: "eosc.ch", prefillSignal: 1, onjobcreated });
    await waitFor(() => expect(input.value).toBe("eosc.ch"));
    expect(input.classList.contains("domain-prefill-flash")).toBe(true);
    // One press: the test starts automatically, no separate Test click.
    await waitFor(() => expect(onjobcreated).toHaveBeenCalledWith({ publicID: "abc12345" }));
    expect(global.fetch).toHaveBeenCalled();
  });

  // Regression: the prefill effect must depend only on prefillSignal. Earlier
  // it tracked submit()'s reads (domain, disabled, ...), so the running->done
  // `disabled` flip re-fired it and resubmitted in an endless loop.
  it("submits once per prefillSignal bump and does not resubmit when disabled toggles", async () => {
    global.fetch.mockResolvedValue(okResponse({ public_id: "abc12345" }));
    const onjobcreated = vi.fn();
    const { rerender } = render(TestForm, {
      props: { prefillDomain: "eosc.ch", prefillSignal: 1, disabled: false, onjobcreated },
    });
    await waitFor(() => expect(onjobcreated).toHaveBeenCalledTimes(1));
    expect(global.fetch).toHaveBeenCalledTimes(1);

    // Mimic the phase running->done transition that flips `disabled`.
    await rerender({ prefillDomain: "eosc.ch", prefillSignal: 1, disabled: true, onjobcreated });
    await rerender({ prefillDomain: "eosc.ch", prefillSignal: 1, disabled: false, onjobcreated });
    // Let any stray effect run; there must be no second submission.
    await Promise.resolve();

    expect(global.fetch).toHaveBeenCalledTimes(1);
    expect(onjobcreated).toHaveBeenCalledTimes(1);
  });
});
