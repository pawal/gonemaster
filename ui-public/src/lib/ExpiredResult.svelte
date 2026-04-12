<script>
  import { t } from "../i18n.js";

  let { onnewtest } = $props();
</script>

<div class="card stack" data-testid="expired-view">
  <h2>{$t("pub.expired_heading")}</h2>
  <p>{$t("pub.expired_body")}</p>
  <button type="button" onclick={onnewtest} data-testid="new-test-button">
    {$t("pub.expired_new_test")}
  </button>
</div>
