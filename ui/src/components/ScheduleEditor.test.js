import { render, screen, fireEvent, waitFor, within } from "@testing-library/svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import ScheduleEditor from "./ScheduleEditor.svelte";
import {
  cohortFixture,
  installFetchRoutes,
  jsonResponse,
  noContentResponse,
  profileFixture,
  scheduleFixture,
} from "../test/helpers.js";

const PREVIEW = {
  summary: "Weekly, Monday and Thursday, 23:30 UTC",
  next: [
    "2026-10-05T23:30:00Z",
    "2026-10-08T23:30:00Z",
    "2026-10-12T23:30:00Z",
    "2026-10-15T23:30:00Z",
    "2026-10-19T23:30:00Z",
  ],
};

describe("ScheduleEditor", () => {
  let writes;

  beforeEach(() => {
    global.fetch = vi.fn();
    writes = [];
    installFetchRoutes({
      "/analysis/schedules/preview": PREVIEW,
      "/analysis/cohorts/1/schedule": (_url, options) => {
        writes.push({ method: options.method, body: options.body ? JSON.parse(options.body) : null });
        if (options.method === "DELETE") return noContentResponse();
        return jsonResponse(scheduleFixture(JSON.parse(options.body)));
      },
    });
  });

  const renderEditor = (props = {}) => {
    const handlers = { onsave: vi.fn(), ondelete: vi.fn(), onclose: vi.fn() };
    render(ScheduleEditor, {
      props: { open: true, cohort: cohortFixture(), schedule: null, profiles: [profileFixture()], ...handlers, ...props },
    });
    return handlers;
  };

  const dialog = () => screen.getByRole("dialog", { name: "Snapshot schedule for tld" });

  it("starts a new schedule as monthly on day 1 at 02:00 UTC", () => {
    renderEditor();

    expect(screen.getByLabelText("Monthly").checked).toBe(true);
    expect(screen.getByRole("button", { name: "1" }).getAttribute("aria-pressed")).toBe("true");
    expect(screen.getByRole("button", { name: "2" }).getAttribute("aria-pressed")).toBe("false");
    expect(screen.getByLabelText("Time").value).toBe("02:00");
    expect(screen.getByLabelText("Time zone").value).toBe("UTC");
    expect(screen.queryByRole("button", { name: "Remove schedule" })).toBeNull();
  });

  it.each([
    ["Weekly", [["Mon", "false"], ["Thu", "false"], ["Sun", "false"]]],
    ["Monthly", [["1", "true"], ["28", "false"], ["Last day of the month", "false"]]],
  ])("shows the day toggles of the %s kind", async (kind, toggles) => {
    renderEditor();
    await fireEvent.click(screen.getByLabelText(kind));

    for (const [label, pressed] of toggles) {
      expect(screen.getByRole("button", { name: label }).getAttribute("aria-pressed")).toBe(pressed);
    }
  });

  it("shows the interval and start date for the interval kind", async () => {
    renderEditor();
    await fireEvent.click(screen.getByLabelText("Every N days"));

    expect(screen.getByLabelText("Days between runs").value).toBe("1");
    expect(screen.getByLabelText("First run").type).toBe("date");
    expect(screen.queryByRole("button", { name: "1" })).toBeNull();
  });

  it("saves toggled weekdays in week order", async () => {
    const { onsave } = renderEditor();
    await fireEvent.click(screen.getByLabelText("Weekly"));
    await fireEvent.click(screen.getByRole("button", { name: "Thu" }));
    await fireEvent.click(screen.getByRole("button", { name: "Mon" }));
    expect(screen.getByRole("button", { name: "Mon" }).getAttribute("aria-pressed")).toBe("true");

    await fireEvent.click(screen.getByRole("button", { name: "Save schedule" }));

    await waitFor(() => expect(onsave).toHaveBeenCalledTimes(1));
    expect(writes).toHaveLength(1);
    expect(writes[0].method).toBe("PUT");
    expect(writes[0].body).toMatchObject({
      enabled: true,
      kind: "weekly",
      weekdays: ["mon", "thu"],
      time_of_day: "02:00",
      timezone: "UTC",
      profile_id: null,
      catch_up: true,
      promote_default: false,
    });
  });

  it("sends the chosen profile and flags", async () => {
    const { onsave } = renderEditor();
    await fireEvent.change(screen.getByLabelText("Profile"), { target: { value: "1" } });
    await fireEvent.click(screen.getByLabelText("Set the captured snapshot as cohort default"));
    await fireEvent.click(screen.getByLabelText("Run a missed occurrence after a restart"));
    await fireEvent.click(screen.getByRole("button", { name: "Save schedule" }));

    await waitFor(() => expect(onsave).toHaveBeenCalledTimes(1));
    expect(writes[0].body).toMatchObject({ profile_id: 1, promote_default: true, catch_up: false });
  });

  it("renders the rule summary and the next three occurrences", async () => {
    renderEditor({ schedule: scheduleFixture({ kind: "weekly", weekdays: ["mon", "thu"], days_of_month: [] }) });

    expect(await screen.findByText(PREVIEW.summary)).toBeInTheDocument();
    const times = [...dialog().querySelectorAll(".preview-list time")].map((el) => el.getAttribute("datetime"));
    expect(times).toEqual(["2026-10-05T23:30:00Z", "2026-10-08T23:30:00Z", "2026-10-12T23:30:00Z"]);
  });

  it("removes the schedule only after confirming", async () => {
    const { ondelete } = renderEditor({ schedule: scheduleFixture() });
    await fireEvent.click(within(dialog()).getByRole("button", { name: "Remove schedule" }));

    const confirm = screen.getByRole("dialog", { name: "Remove the snapshot schedule for tld?" });
    expect(writes).toHaveLength(0);
    await fireEvent.click(within(confirm).getByRole("button", { name: "Remove schedule" }));

    await waitFor(() => expect(ondelete).toHaveBeenCalledTimes(1));
    expect(writes).toEqual([{ method: "DELETE", body: null }]);
  });

  it("closes on Escape", async () => {
    const { onclose } = renderEditor();
    await fireEvent.keyDown(dialog(), { key: "Escape" });
    expect(onclose).toHaveBeenCalledTimes(1);
  });
});
