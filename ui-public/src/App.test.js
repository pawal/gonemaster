import { render, screen, cleanup } from "@testing-library/svelte";
import { afterEach, describe, expect, it } from "vitest";
import App from "./App.svelte";

describe("App scaffold", () => {
  afterEach(() => {
    cleanup();
  });

  it("mounts without throwing", () => {
    expect(() => render(App)).not.toThrow();
  });

  it("renders a root element", () => {
    render(App);
    expect(document.querySelector("main")).not.toBeNull();
  });
});
