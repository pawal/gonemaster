import { render, screen, fireEvent } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import ThemeToggle from "./ThemeToggle.svelte";
import { themeStore } from "../lib/theme.svelte.js";
import { resetTheme, stubMatchMedia } from "../test/helpers.js";

describe("ThemeToggle", () => {
  beforeEach(() => {
    resetTheme();
  });

  afterEach(() => {
    resetTheme();
  });

  it("shows the sun icon when the OS is light and value is 'system'", () => {
    stubMatchMedia(false);
    render(ThemeToggle);
    expect(screen.getByRole("button").textContent.trim()).toBe("☀");
  });

  it("shows the moon icon when the OS is dark and value is 'system'", () => {
    stubMatchMedia(true);
    render(ThemeToggle);
    expect(screen.getByRole("button").textContent.trim()).toBe("☾");
  });

  it("clicking the toggle switches to dark when starting in light-OS system", async () => {
    stubMatchMedia(false);
    render(ThemeToggle);
    await fireEvent.click(screen.getByRole("button"));
    expect(themeStore.value).toBe("dark");
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
  });

  it("clicking the toggle persists the new value to localStorage", async () => {
    stubMatchMedia(false);
    render(ThemeToggle);
    await fireEvent.click(screen.getByRole("button"));
    expect(localStorage.getItem("gonemaster.ui.theme.v1")).toBe("dark");
  });
});
