<script>
  import { onMount, onDestroy } from "svelte";
  import { t } from "../i18n.js";
  import { getJob } from "../api.js";

  let { publicID, progress = $bindable(0), onjobdone } = $props();

  const TERMINAL = new Set(["succeeded", "failed", "canceled", "expired"]);
  const POLL_INTERVAL = 2000;

  let domain = $state("");
  let status = $state("queued");
  let errorKey = $state("");
  let timer;

  async function poll() {
    try {
      const res = await getJob(publicID);
      if (!res.ok) {
        clearInterval(timer);
        if (res.status === 404) {
          onjobdone?.({ publicID, status: "expired" });
        } else {
          errorKey = "pub.error_unknown";
        }
        return;
      }
      const job = await res.json();
      domain = job.domain;
      status = job.status;
      progress = job.progress;
      if (TERMINAL.has(status)) {
        clearInterval(timer);
        onjobdone?.({ publicID, status, domain, finishedAt: job.finished_at ?? null });
      }
    } catch (_) {
      clearInterval(timer);
      errorKey = "pub.error_network";
    }
  }

  onMount(() => {
    poll();
    timer = setInterval(poll, POLL_INTERVAL);
  });

  onDestroy(() => clearInterval(timer));
</script>

<div class="card stack" data-testid="progress-view">
  {#if errorKey}
    <p class="error-box" role="alert">{$t(errorKey)}</p>
  {:else}
    <p class="progress-status">{status === "queued" ? $t("pub.progress_queued") : $t("pub.progress_testing_label")}</p>
    {#if domain}
      <span class="progress-domain">{domain}</span>
    {/if}
    <div
      class="progress-bar-track"
      role="progressbar"
      aria-valuenow={progress}
      aria-valuemin="0"
      aria-valuemax="100"
    >
      <div class="progress-bar-fill" style="width: {progress}%"></div>
    </div>
  {/if}
</div>
