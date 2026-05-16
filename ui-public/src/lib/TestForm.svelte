<script>
  import { t } from "../i18n.js";
  import { createJob, lookupDomain } from "../api.js";
  import { validateDomain, emptyNsRow, emptyDsRow, buildJobOpts } from "./validate.js";

  let { disabled = false, focusSignal = 0, onjobcreated } = $props();

  let inputEl;
  $effect(() => {
    // Re-runs whenever focusSignal changes; 0 means "don't focus" (e.g. share-link load).
    if (focusSignal > 0) inputEl?.focus();
  });

  const DNSSEC_ALGORITHMS = [
    { group: "Recommended", options: [
      { value: 8,  label: "8 – RSA/SHA-256" },
      { value: 13, label: "13 – ECDSA P-256/SHA-256" },
      { value: 14, label: "14 – ECDSA P-384/SHA-384" },
      { value: 15, label: "15 – Ed25519" },
      { value: 16, label: "16 – Ed448" },
      { value: 17, label: "17 – SM2/SM3" },
      { value: 23, label: "23 – GOST R 34.10-2012" },
    ]},
    { group: "Deprecated", options: [
      { value: 10, label: "10 – RSA/SHA-512" },
      { value: 1,  label: "1 – RSA/MD5" },
      { value: 3,  label: "3 – DSA/SHA1" },
      { value: 5,  label: "5 – RSA/SHA-1" },
      { value: 6,  label: "6 – DSA-NSEC3-SHA1" },
      { value: 7,  label: "7 – RSASHA1-NSEC3-SHA1" },
      { value: 12, label: "12 – GOST R 34.10-2001" },
    ]},
  ];

  const DS_DIGEST_TYPES = [
    { group: "Recommended", options: [
      { value: 2, label: "2 – SHA-256" },
      { value: 4, label: "4 – SHA-384" },
      { value: 5, label: "5 – GOST R 34.11-2012" },
      { value: 6, label: "6 – SM3" },
    ]},
    { group: "Deprecated", options: [
      { value: 1, label: "1 – SHA-1" },
      { value: 3, label: "3 – GOST R 34.11-94" },
    ]},
  ];

  let domain = $state("");
  let submitting = $state(false);
  let errorKey = $state("");
  let errorExtra = $state({});

  let ipMode = $state("default");
  let nsRows = $state([]);
  let dsRows = $state([]);

  async function handleSubmit(e) {
    e.preventDefault();
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
      onjobcreated?.({ publicID: job.public_id });
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

  let optionsOpen = $state(false);
  let nsOpen = $state(false);
  let dsOpen = $state(false);

  function resetForm() {
    domain = "";
    nsRows = [];
    dsRows = [];
    ipMode = "default";
    errorKey = "";
    errorExtra = {};
    optionsOpen = false;
    nsOpen = false;
    dsOpen = false;
  }

  let fetching = $state(false);

  async function fetchFromParent() {
    const d = domain.trim();
    if (!d) return;
    fetching = true;
    try {
      const res = await lookupDomain(d);
      if (!res.ok) return;
      const data = await res.json();
      if (Array.isArray(data.nameservers) && data.nameservers.length > 0) {
        nsRows = data.nameservers.map((n) => ({ ns: n.ns ?? "", ip: n.ip ?? "" }));
      }
      if (Array.isArray(data.ds_records) && data.ds_records.length > 0) {
        dsRows = data.ds_records.map((d) => ({
          keytag: String(d.keytag ?? ""),
          algorithm: d.algorithm ?? "",
          digtype: d.digtype ?? "",
          digest: d.digest ?? "",
        }));
      }
    } catch (_) {
      // silently ignore lookup failures
    } finally {
      fetching = false;
    }
  }
</script>

<div class="card stack" data-testid="test-form">
  <form onsubmit={handleSubmit} novalidate>
    <div class="stack">
      <label for="domain-input">{$t("pub.domain_label")}</label>
      <input
        id="domain-input"
        type="text"
        bind:this={inputEl}
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

      <details class="advanced-options" bind:open={optionsOpen} inert={disabled || submitting ? '' : undefined}>
        <summary>{$t("pub.options_summary")}</summary>

        <div class="stack options-stack">
          <label for="ip-mode">{$t("pub.ip_transport_label")}</label>
          <select id="ip-mode" bind:value={ipMode} disabled={submitting || disabled}>
            <option value="default">{$t("pub.ip_mode_default")}</option>
            <option value="disable_ipv4">{$t("pub.ip_mode_disable_ipv4")}</option>
            <option value="disable_ipv6">{$t("pub.ip_mode_disable_ipv6")}</option>
          </select>

          <details class="advanced-options" bind:open={nsOpen}>
            <summary>{$t("pub.ns_summary")}</summary>
            <div class="stack options-substack">
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
                    onclick={() => removeNsRow(i)}
                    disabled={submitting || disabled}
                  >{$t("pub.ns_remove")}</button>
                </div>
              {/each}
              <div class="row">
                <button
                  type="button"
                  class="ghost"
                  onclick={addNsRow}
                  disabled={submitting || disabled}
                  data-testid="add-ns"
                >{$t("pub.ns_add")}</button>
                <button
                  type="button"
                  class="ghost"
                  onclick={fetchFromParent}
                  disabled={submitting || disabled || fetching || !domain.trim()}
                  data-testid="fetch-ns"
                >{fetching ? $t("pub.fetch_loading") : $t("pub.fetch_from_parent")}</button>
              </div>
            </div>
          </details>

          <details class="advanced-options" bind:open={dsOpen}>
            <summary>{$t("pub.ds_summary")}</summary>
            <div class="stack options-substack">
              {#each dsRows as row, i}
                <div class="undelegated-ds-row" data-testid="ds-row">
                  <input
                    type="text"
                    inputmode="numeric"
                    pattern="[0-9]*"
                    bind:value={row.keytag}
                    placeholder={$t("pub.ds_placeholder_keytag")}
                    disabled={submitting || disabled}
                    aria-label="DS keytag"
                  />
                  <select bind:value={row.algorithm} disabled={submitting || disabled} aria-label="DS algorithm">
                    <option value="" disabled>{$t("pub.ds_placeholder_algo")}</option>
                    {#each DNSSEC_ALGORITHMS as { group, options }}
                      <optgroup label={group}>
                        {#each options as { value, label }}
                          <option {value}>{label}</option>
                        {/each}
                      </optgroup>
                    {/each}
                  </select>
                  <select bind:value={row.digtype} disabled={submitting || disabled} aria-label="DS digest type">
                    <option value="" disabled>{$t("pub.ds_placeholder_digtype")}</option>
                    {#each DS_DIGEST_TYPES as { group, options }}
                      <optgroup label={group}>
                        {#each options as { value, label }}
                          <option {value}>{label}</option>
                        {/each}
                      </optgroup>
                    {/each}
                  </select>
                  <input type="text" bind:value={row.digest} placeholder={$t("pub.ds_placeholder_digest")} disabled={submitting || disabled} aria-label="DS digest" />
                  <button
                    type="button"
                    class="ghost mini-button"
                    onclick={() => removeDsRow(i)}
                    disabled={submitting || disabled}
                  >{$t("pub.ds_remove")}</button>
                </div>
              {/each}
              <div class="row">
                <button
                  type="button"
                  class="ghost"
                  onclick={addDsRow}
                  disabled={submitting || disabled}
                  data-testid="add-ds"
                >{$t("pub.ds_add")}</button>
                <button
                  type="button"
                  class="ghost"
                  onclick={fetchFromParent}
                  disabled={submitting || disabled || fetching || !domain.trim()}
                  data-testid="fetch-ds"
                >{fetching ? $t("pub.fetch_loading") : $t("pub.fetch_from_parent")}</button>
              </div>
            </div>
          </details>
        </div>
      </details>

      <button type="submit" disabled={submitting || disabled}>
        {submitting ? $t("pub.testing_button") : $t("pub.test_button")}
      </button>
      <button
        type="button"
        class="ghost"
        onclick={resetForm}
        disabled={submitting || disabled}
        data-testid="reset-form"
      >{$t("pub.reset_form")}</button>
    </div>
  </form>
</div>

<style>
  .options-stack { margin-top: 10px; }
  .options-substack { margin-top: 8px; }
</style>
