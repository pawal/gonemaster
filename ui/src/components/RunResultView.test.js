import { render } from "@testing-library/svelte";
import { describe, expect, it } from "vitest";
import RunResultView from "./RunResultView.svelte";
import { setCatalog } from "../i18n.js";

setCatalog({ "pub.score_label": "DNS Quality Score" });

const resultWithScore = () => ({
  job_id: "run_1",
  summary: { levels: {} },
  raw: { entries: [] },
  score: { grade: "A", score: 95, categories: {}, bonus: null },
});

describe("RunResultView", () => {
  it("renders nothing without a result", () => {
    const { container } = render(RunResultView, { props: { result: null } });
    expect(container.querySelector(".score-card")).toBeNull();
    expect(container.querySelector(".module-list")).toBeNull();
  });

  it("shows the score card when scoring is enabled and a score is present", () => {
    const { container } = render(RunResultView, {
      props: { result: resultWithScore(), scoringEnabled: true },
    });
    expect(container.querySelector(".score-card")).not.toBeNull();
  });

  it("hides the score card when scoring is disabled", () => {
    const { container } = render(RunResultView, {
      props: { result: resultWithScore(), scoringEnabled: false },
    });
    expect(container.querySelector(".score-card")).toBeNull();
  });
});
