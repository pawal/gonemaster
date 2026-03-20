<script>
  import { createEventDispatcher } from "svelte";
  import { t } from "../i18n.js";
  import { createJob } from "../api.js";
  import { validateDomain, emptyNsRow, emptyDsRow, buildJobOpts } from "./validate.js";

  const dispatch = createEventDispatcher();

  export let disabled = false;

  let domain = "";
  let submitting = false;
  let errorKey = "";
  let errorExtra = {};

  let ipMode = "default";
  let nsRows = [];
  let dsRows = [];

  async function handleSubmit() {
    if (disabled) return;
    const err = validateDomain(domain);
    if (err) { errorKey = err; errorExtra = {}; return; }
    errorKey = "";
    submitting = true;
    try {
      const opts = buildJobOpts(ipMode, nsRows, dsRows);
      const res = await createJob(domain.trim(), opts);
      if (res.status === 429) {
        const retryAfter = res.headers?.get("Retry-After") ?? "60";
        errorKey = "pub.error_rate_limited";
        errorExtra = { seconds: retryAfter };
        return;
      }
      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        if (body?.error?.code === "invalid_domain") {
          errorKey = "pub.error_domain_invalid";
        } else {
          errorKey = "pub.error_job_failed";
        }
        return;
      }
      const job = await res.json();
      dispatch("jobcreated", { publicID: job.public_id });
    } catch (_) {
      errorKey = "pub.error_network";
    } finally {
      submitting = false;
    }
  }

  function addNsRow() { nsRows = [...nsRows, emptyNsRow()]; }
  function removeNsRow(i) { nsRows = nsRows.filter((_, idx) => idx !== i); }
  function addDsRow() { dsRows = [...dsRows, emptyDsRow()]; }
  function removeDsRow(i) { dsRows = dsRows.filter((_, idx) => idx !== i); }
</script>

<div class="card stack" data-testid="test-form">
  <form on:submit|preventDefault={handleSubmit} novalidate>
    <div class="stack">
      <label for="domain-input">{$t("pub.domain_label")}</label>
      <input
        id="domain-input"
        type="text"
        bind:value={domain}
        placeholder={$t("pub.domain_placeholder")}
        autocomplete="off"
        autocapitalize="none"
        spellcheck="false"
        disabled={submitting || disabled}
      />

      {#if errorKey}
        <p class="error-box" role="alert" data-testid="form-error">
          {$t(errorKey, errorExtra)}
        </p>
      {/if}

      <details class="advanced-options">
        <summary>{$t("pub.options_summary")}</summary>

        <div class="stack" style="margin-top:10px">
          <label for="ip-mode">{$t("pub.ip_transport_label")}</label>
          <select id="ip-mode" bind:value={ipMode} disabled={submitting || disabled}>
            <option value="default">{$t("pub.ip_mode_default")}</option>
            <option value="disable_ipv4">{$t("pub.ip_mode_disable_ipv4")}</option>
            <option value="disable_ipv6">{$t("pub.ip_mode_disable_ipv6")}</option>
          </select>

          <details class="advanced-options">
            <summary>{$t("pub.ns_summary")}</summary>
            <div class="stack" style="margin-top:8px">
              {#each nsRows as row, i}
                <div class="undelegated-row" data-testid="ns-row">
                  <input
                    type="text"
                    bind:value={row.ns}
                    placeholder={$t("pub.ns_placeholder_ns")}
                    disabled={submitting || disabled}
                    aria-label="NS hostname"
                  />
                  <input
                    type="text"
                    bind:value={row.ip}
                    placeholder={$t("pub.ns_placeholder_ip")}
                    disabled={submitting || disabled}
                    aria-label="NS IP address"
                  />
                  <button
                    type="button"
                    class="ghost mini-button"
                    on:click={() => removeNsRow(i)}
                    disabled={submitting || disabled}
                  >{$t("pub.ns_remove")}</button>
                </div>
              {/each}
              <button
                type="button"
                class="ghost"
                on:click={addNsRow}
                disabled={submitting || disabled}
                data-testid="add-ns"
              >{$t("pub.ns_add")}</button>
            </div>
          </details>

          <details class="advanced-options">
            <summary>{$t("pub.ds_summary")}</summary>
            <div class="stack" style="margin-top:8px">
              {#each dsRows as row, i}
                <div class="undelegated-ds-row" data-testid="ds-row">
                  <input type="number" bind:value={row.keytag}  placeholder={$t("pub.ds_placeholder_keytag")}  disabled={submitting || disabled} aria-label="DS keytag" />
                  <input type="number" bind:value={row.algorithm} placeholder={$t("pub.ds_placeholder_algo")} disabled={submitting || disabled} aria-label="DS algorithm" />
                  <input type="number" bind:value={row.digtype} placeholder={$t("pub.ds_placeholder_digtype")} disabled={submitting || disabled} aria-label="DS digest type" />
                  <input type="text"   bind:value={row.digest}  placeholder={$t("pub.ds_placeholder_digest")} disabled={submitting || disabled} aria-label="DS digest" />
                  <button
                    type="button"
                    class="ghost mini-button"
                    on:click={() => removeDsRow(i)}
                    disabled={submitting || disabled}
                  >{$t("pub.ds_remove")}</button>
                </div>
              {/each}
              <button
                type="button"
                class="ghost"
                on:click={addDsRow}
                disabled={submitting || disabled}
                data-testid="add-ds"
              >{$t("pub.ds_add")}</button>
            </div>
          </details>
        </div>
      </details>

      <button type="submit" disabled={submitting || disabled}>
        {submitting ? $t("pub.testing_button") : $t("pub.test_button")}
      </button>
    </div>
  </form>
</div>
