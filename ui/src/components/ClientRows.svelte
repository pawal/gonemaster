<script>
  import { t } from "../i18n.js";

  let { item = null, onFilterClient = () => {} } = $props();
</script>

{#if item?.client_ip}
  <span>{$t("job_client_label")}</span>
  <strong class="client">
    <button class="ghost btn-text-mono" type="button" title={$t("job_client_filter_title")} onclick={() => onFilterClient(item.client_ip)}>{item.client_ip}</button>
    {#if item.client_via}
      <span class="badge">{item.client_via === "ui" ? $t("job_client_via_ui") : $t("job_client_via_api")}</span>
    {/if}
  </strong>
{/if}
{#if item?.client_user_agent}
  <span>{$t("job_user_agent_label")}</span>
  <strong class="user-agent">{item.client_user_agent}</strong>
{/if}

<style>
  .client {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
  }

  .user-agent {
    overflow-wrap: anywhere;
  }
</style>
