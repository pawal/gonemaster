import { render, screen, cleanup } from "@testing-library/svelte";
import { afterEach, describe, expect, it } from "vitest";
import ScoreCard from "./ScoreCard.svelte";
import { setCatalog } from "../i18n.js";

setCatalog({
  "pub.score_label": "DNS Quality Score",
  "pub.score_aplus_criteria": "A+ criteria",
  "pub.score_aplus_achieved": "Achieved",
  "pub.score_aplus_missing": "{n} unmet",
  "pub.score_bonus_dnssec_enabled": "DNSSEC enabled",
});

const sampleScore = () => ({
  grade: "A",
  score: 92,
  categories: {
    dnssec: { score: 100, tested: true },
    nameserver_health: { score: 88, tested: true },
    connectivity: { score: 0, tested: false },
    zone_consistency: { score: 90, tested: true },
  },
  bonus: {
    eligible: false,
    criteria: { dnssec_enabled: true, no_warnings_or_errors: false },
  },
});

describe("ScoreCard", () => {
  afterEach(() => cleanup());

  it("renders nothing when no score is given", () => {
    const { container } = render(ScoreCard, { props: { score: null } });
    expect(container.querySelector(".score-card")).toBeNull();
  });

  it("renders the grade badge, score out of 100, and the localized label", () => {
    const { container } = render(ScoreCard, { props: { score: sampleScore() } });
    const badge = container.querySelector(".grade-badge");
    expect(badge.getAttribute("data-grade")).toBe("A");
    expect(container.querySelector(".score-number").textContent).toContain("92");
    expect(screen.getByText("DNS Quality Score")).toBeInTheDocument();
  });

  it("renders the categories in CAT_ORDER and dashes an untested category", () => {
    const { container } = render(ScoreCard, { props: { score: sampleScore() } });
    const names = Array.from(container.querySelectorAll(".score-cat-name")).map((n) => n.textContent);
    expect(names).toEqual(["DNSSEC", "Nameserver", "Connectivity", "Zone"]);
    const nums = Array.from(container.querySelectorAll(".score-cat-num")).map((n) => n.textContent);
    // Connectivity is untested -> "-"
    expect(nums[2]).toBe("-");
  });

  it("hides the no_warnings_or_errors bonus row but shows the rest with met icons", () => {
    const { container } = render(ScoreCard, { props: { score: sampleScore() } });
    const items = container.querySelectorAll(".score-bonus-item");
    // Only dnssec_enabled shown (no_warnings_or_errors is filtered out).
    expect(items.length).toBe(1);
    expect(items[0].getAttribute("data-met")).toBe("yes");
    expect(items[0].querySelector(".score-bonus-icon").textContent).toBe("✓");
  });

  it("shows the unmet count in the summary when not eligible", () => {
    const { container } = render(ScoreCard, { props: { score: sampleScore() } });
    const status = container.querySelector(".score-bonus-status");
    expect(status.getAttribute("data-met")).toBe("no");
  });
});
