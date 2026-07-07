<script>
  import { t } from "../i18n.js";
  import { CAT_ORDER, CAT_LABELS, BONUS_HIDDEN } from "../lib/result.js";

  let { score = null } = $props();

  const GRADE_COLORS = { "A+": "var(--grade-aplus)", "A": "var(--grade-a)", "B": "var(--grade-b)", "C": "var(--grade-c)", "D": "var(--grade-d)", "F": "var(--grade-f)" };
  const gradeBarColor = (grade) => GRADE_COLORS[grade] ?? "var(--grade-a)";

  const applyBarStyle = (node, params) => {
    const apply = ({ pct, color, delay }) => {
      node.style.setProperty("--bar-pct", `${pct}%`);
      node.style.setProperty("--bar-color", color);
      node.style.animationDelay = `${delay}ms`;
    };
    apply(params);
    return { update(p) { apply(p); } };
  };

  const sortedCats = $derived(
    score ? CAT_ORDER.filter((c) => c in (score.categories ?? {})).map((c) => [c, score.categories[c]]) : []
  );
  const bonusCriteria = $derived(
    score?.bonus?.criteria
      ? Object.entries(score.bonus.criteria).filter(([k]) => !BONUS_HIDDEN.has(k))
      : []
  );
  const bonusMissing = $derived(bonusCriteria.filter((entry) => entry[1] === false).length);
</script>

{#if score}
  <div class="score-card">
    <div class="score-left">
      <div class="grade-badge" data-grade={score.grade}>
        <span class="grade-letter">{score.grade}</span>
      </div>
      <div class="score-meta">
        <div class="score-number">{score.score}<span class="score-denom">/100</span></div>
        <div class="score-label">{$t("pub.score_label")}</div>
      </div>
    </div>
    {#if sortedCats.length}
      <div class="score-cats">
        {#each sortedCats as [cat, res], i}
          <div class="score-cat-row" data-untested={res.tested === false ? "" : undefined}>
            <span class="score-cat-name">{CAT_LABELS[cat] ?? cat}</span>
            <div class="score-cat-bar-track">
              <div class="score-cat-bar" use:applyBarStyle={{ pct: res.tested === false ? 0 : res.score, color: gradeBarColor(score.grade), delay: i * 60 }}></div>
            </div>
            <span class="score-cat-num">{res.tested === false ? "-" : res.score}</span>
          </div>
        {/each}
      </div>
    {/if}
  </div>
  {#if bonusCriteria.length}
    <details class="score-bonus">
      <summary class="score-bonus-summary">
        <span class="score-bonus-chevron"></span>
        <span class="score-bonus-title">{$t("pub.score_aplus_criteria")}</span>
        <span class="score-bonus-status" data-met={score.bonus.eligible ? "yes" : "no"}>
          {score.bonus.eligible ? $t("pub.score_aplus_achieved") : $t("pub.score_aplus_missing", { n: bonusMissing })}
        </span>
      </summary>
      <div class="score-bonus-list">
        {#each bonusCriteria as [key, val]}
          <div class="score-bonus-item" data-met={val === null ? "na" : val ? "yes" : "no"}>
            <span class="score-bonus-icon">{val === null ? "-" : val ? "✓" : "✗"}</span>
            <span>{$t(`pub.score_bonus_${key}`)}</span>
          </div>
        {/each}
      </div>
    </details>
  {/if}
{/if}
