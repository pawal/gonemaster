<script>
  import { t } from "../i18n.js";
  import { hashFor } from "../router.js";

  export let publicID;
  export let domain = "";
  export let score = null;

  const GRADE_EMOJI = {
    "A+": "🏆",
    "A":  "✅",
    "B":  "🟡",
    "C":  "⚠️",
    "D":  "🔴",
    "F":  "❌",
  };

  let copied = false;
  let timer;

  async function share() {
    const url = window.location.origin + window.location.pathname + hashFor("result", publicID);
    let text = url;
    if (score && domain) {
      const emoji = GRADE_EMOJI[score.grade] ?? "🔵";
      text = `${emoji} ${domain} — DNS grade ${score.grade} (${score.score}/100)\n${url}`;
    }
    try {
      await navigator.clipboard.writeText(text);
    } catch (_) {
      const input = document.createElement("input");
      input.value = text;
      document.body.appendChild(input);
      input.select();
      document.execCommand("copy");
      document.body.removeChild(input);
    }
    copied = true;
    clearTimeout(timer);
    timer = setTimeout(() => { copied = false; }, 2000);
  }
</script>

<button
  type="button"
  class="ghost"
  on:click={share}
  data-testid="share-button"
>
  {copied ? $t("pub.result_share_copied") : $t("pub.result_share")}
</button>
