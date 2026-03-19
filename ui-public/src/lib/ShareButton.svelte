<script>
  import { t } from "../i18n.js";
  import { hashFor } from "../router.js";

  export let publicID;

  let copied = false;
  let timer;

  async function share() {
    const url = window.location.origin + window.location.pathname + hashFor("result", publicID);
    try {
      await navigator.clipboard.writeText(url);
    } catch (_) {
      // Fallback: create a temporary input element for environments without clipboard API
      const input = document.createElement("input");
      input.value = url;
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
