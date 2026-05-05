<script>
  import { t } from "../i18n.js";

  let {
    apiFetch,
    setStatus = () => {},
    availableProfiles = [],
    profilesLoading = false,
    onJobCreated = () => {},
  } = $props();

  let singleDomain = $state("");
  let singleTags = $state("");
  let singleSubmitting = $state(false);
  let createdJobId = $state("");
  let singleIPMode = $state("default");
  let singleProfileId = $state("");
  let undelegatedNameservers = $state([]);
  let undelegatedDSInfo = $state([]);
  let undelegatedRowCounter = 0;

  const nextUndelegatedRowID = (prefix) => `${prefix}-${++undelegatedRowCounter}`;
  const emptyUndelegatedNameserverRow = () => ({ id: nextUndelegatedRowID("ns"), ns: "", ip: "" });
  const emptyUndelegatedDSRow = () => ({
    id: nextUndelegatedRowID("ds"),
    keytag: "",
    algorithm: "",
    digtype: "",
    digest: "",
  });
  const trimUndelegatedNameserverRow = (row = {}) => ({
    ns: String(row?.ns || "").trim(),
    ip: String(row?.ip || "").trim(),
  });
  const trimUndelegatedDSRow = (row = {}) => ({
    keytag: String(row?.keytag || "").trim(),
    algorithm: String(row?.algorithm || "").trim(),
    digtype: String(row?.digtype || "").trim(),
    digest: String(row?.digest || "").trim(),
  });
  const isIPv4Address = (value) => {
    const text = String(value || "").trim();
    const parts = text.split(".");
    if (parts.length !== 4) return false;
    return parts.every((part) => /^\d{1,3}$/.test(part) && Number(part) >= 0 && Number(part) <= 255);
  };
  const isIPv6Address = (value) => {
    const text = String(value || "").trim();
    if (!text.includes(":")) return false;
    try {
      const parsed = new URL(`http://[${text}]`).hostname;
      return parsed.startsWith("[") && parsed.endsWith("]");
    } catch (_) {
      return false;
    }
  };
  const isIPAddress = (value) => isIPv4Address(value) || isIPv6Address(value);
  const isUIntInRange = (value, min, max) => {
    if (!/^\d+$/.test(value)) return false;
    const numeric = Number(value);
    return Number.isFinite(numeric) && numeric >= min && numeric <= max;
  };
  const isHexDigest = (value) => /^[0-9a-fA-F]+$/.test(value);

  const normalizeDomainInput = (value) => {
    const trimmed = (value || "").trim();
    if (!trimmed) return "";
    try {
      const hasScheme = /^[a-z][a-z0-9+.-]*:\/\//i.test(trimmed);
      const url = new URL(hasScheme ? trimmed : `http://${trimmed}`);
      return url.hostname;
    } catch (_) {
      return trimmed;
    }
  };

  const normalizeOptionalProfileID = (value) => {
    const parsed = Number(value);
    if (!Number.isFinite(parsed) || parsed <= 0) return null;
    return parsed;
  };

  function addUndelegatedNameserverRow() {
    undelegatedNameservers = [...undelegatedNameservers, emptyUndelegatedNameserverRow()];
  }

  function removeUndelegatedNameserverRow(rowID) {
    undelegatedNameservers = undelegatedNameservers.filter((row) => row?.id !== rowID);
  }

  function addUndelegatedDSRow() {
    undelegatedDSInfo = [...undelegatedDSInfo, emptyUndelegatedDSRow()];
  }

  function removeUndelegatedDSRow(rowID) {
    undelegatedDSInfo = undelegatedDSInfo.filter((row) => row?.id !== rowID);
  }

  function buildUndelegatedPayload() {
    const nameservers = [];
    for (let i = 0; i < undelegatedNameservers.length; i += 1) {
      const row = trimUndelegatedNameserverRow(undelegatedNameservers[i]);
      if (!row.ns && !row.ip) continue;
      if (!row.ns) return { error: $t("error_ns_row_ns_required", { row: i + 1 }) };
      if (/\s/.test(row.ns)) return { error: $t("error_ns_row_ns_whitespace", { row: i + 1 }) };
      if (row.ip && !isIPAddress(row.ip)) {
        return { error: $t("error_ns_row_ip_invalid", { row: i + 1 }) };
      }
      const payloadRow = { ns: row.ns };
      if (row.ip) payloadRow.ip = row.ip;
      nameservers.push(payloadRow);
    }
    const dsInfo = [];
    for (let i = 0; i < undelegatedDSInfo.length; i += 1) {
      const row = trimUndelegatedDSRow(undelegatedDSInfo[i]);
      const hasAny = row.keytag || row.algorithm || row.digtype || row.digest;
      if (!hasAny) continue;
      const hasAll = row.keytag && row.algorithm && row.digtype && row.digest;
      if (!hasAll) return { error: $t("error_ds_row_all_required", { row: i + 1 }) };
      if (!isUIntInRange(row.keytag, 0, 65535)) {
        return { error: $t("error_ds_row_keytag_range", { row: i + 1, min: 0, max: 65535 }) };
      }
      if (!isUIntInRange(row.algorithm, 0, 255)) {
        return { error: $t("error_ds_row_algorithm_range", { row: i + 1, min: 0, max: 255 }) };
      }
      if (!isUIntInRange(row.digtype, 0, 255)) {
        return { error: $t("error_ds_row_digtype_range", { row: i + 1, min: 0, max: 255 }) };
      }
      if (!isHexDigest(row.digest)) {
        return { error: $t("error_ds_row_digest_hex", { row: i + 1 }) };
      }
      dsInfo.push({
        keytag: Number(row.keytag),
        algorithm: Number(row.algorithm),
        digtype: Number(row.digtype),
        digest: row.digest.toUpperCase(),
      });
    }
    return { nameservers, dsInfo };
  }

  async function submitSingle() {
    const normalizedDomain = normalizeDomainInput(singleDomain);
    if (!normalizedDomain) {
      setStatus($t("error_domain_required"), "warn");
      return;
    }
    const undelegatedPayload = buildUndelegatedPayload();
    if (undelegatedPayload.error) {
      setStatus(undelegatedPayload.error, "warn");
      return;
    }
    singleSubmitting = true;
    createdJobId = "";
    try {
      const parsedTags = singleTags.split(/[,\s]+/).map((s) => s.trim()).filter(Boolean);
      const selectedProfileID = normalizeOptionalProfileID(singleProfileId);
      const payload = {
        domain: normalizedDomain,
        ...(selectedProfileID && { profile_id: selectedProfileID }),
        ...(parsedTags.length > 0 && { tags: parsedTags }),
      };
      if (singleIPMode === "disable_ipv4") {
        payload.profile_overrides = { net: { ipv4: false, ipv6: true } };
      } else if (singleIPMode === "disable_ipv6") {
        payload.profile_overrides = { net: { ipv4: true, ipv6: false } };
      }
      if (undelegatedPayload.nameservers.length > 0) payload.nameservers = undelegatedPayload.nameservers;
      if (undelegatedPayload.dsInfo.length > 0) payload.ds_info = undelegatedPayload.dsInfo;

      const job = await apiFetch("/jobs", { method: "POST", body: JSON.stringify(payload) });
      createdJobId = job.id;
      onJobCreated(job.id);
    } catch (error) {
      setStatus($t("error_create_job", { error: error.message }), "warn");
    } finally {
      singleSubmitting = false;
    }
  }
</script>

<div class="card reveal delay-18">
  <h2>{$t("single_job_heading")}</h2>
  <div class="stack">
    <label for="single-domain">{$t("single_domain_label")}</label>
    <input
      id="single-domain"
      type="text"
      placeholder="example.com"
      bind:value={singleDomain}
      onkeydown={(event) => {
        if (event.key === "Enter") {
          event.preventDefault();
          submitSingle();
        }
      }}
    />
  </div>
  <div class="stack">
    <label for="single-tags">{$t("single_tags_label")}</label>
    <input
      id="single-tags"
      type="text"
      placeholder={$t("single_tags_placeholder")}
      bind:value={singleTags}
    />
    <div class="small">{$t("single_tags_hint")}</div>
  </div>
  <div class="stack">
    <label for="single-profile">{$t("stored_profile_label")}</label>
    <select id="single-profile" bind:value={singleProfileId} disabled={profilesLoading && availableProfiles.length === 0}>
      <option value="">{$t("stored_profile_auto_option")}</option>
      {#each availableProfiles as profile}
        <option value={profile.id}>{profile.name}</option>
      {/each}
    </select>
    <div class="small">{$t("stored_profile_hint")}</div>
  </div>
  <details class="advanced-options">
    <summary>{$t("advanced_profile_summary")}</summary>
    <div class="stack advanced-stack">
      <label for="single-ip-mode">{$t("ip_transport_label")}</label>
      <select id="single-ip-mode" bind:value={singleIPMode}>
        <option value="default">{$t("ip_mode_default")}</option>
        <option value="disable_ipv4">{$t("ip_mode_disable_ipv4")}</option>
        <option value="disable_ipv6">{$t("ip_mode_disable_ipv6")}</option>
      </select>
      <div class="small">{$t("ip_mode_hint")}</div>
    </div>
  </details>
  <details class="advanced-options">
    <summary>{$t("undelegated_summary")}</summary>
    <div class="stack advanced-stack">
      <div class="field-label">{$t("ns_field_label")}</div>
      {#if undelegatedNameservers.length === 0}
        <div class="small">{$t("ns_none")}</div>
      {:else}
        <div class="undelegated-list">
          {#each undelegatedNameservers as row, index (row.id)}
            <div class="undelegated-row">
              <input
                type="text"
                aria-label={$t("ns_aria_label", { n: index + 1 })}
                placeholder="ns1.example.com"
                bind:value={row.ns}
              />
              <input
                type="text"
                aria-label={$t("ns_ip_aria_label", { n: index + 1 })}
                placeholder="192.0.2.10 or 2001:db8::10"
                bind:value={row.ip}
              />
              <button class="ghost mini-button" type="button" onclick={() => removeUndelegatedNameserverRow(row.id)}>
                {$t("remove")}
              </button>
            </div>
          {/each}
        </div>
      {/if}
      <button class="ghost" type="button" onclick={addUndelegatedNameserverRow}>
        {$t("add_nameserver")}
      </button>

      <div class="field-label">{$t("ds_field_label")}</div>
      {#if undelegatedDSInfo.length === 0}
        <div class="small">{$t("ds_none")}</div>
      {:else}
        <div class="undelegated-list">
          {#each undelegatedDSInfo as row, index (row.id)}
            <div class="undelegated-ds-row">
              <input type="text" aria-label={$t("ds_keytag_aria_label", { n: index + 1 })} placeholder="12345" bind:value={row.keytag} />
              <input type="text" aria-label={$t("ds_algorithm_aria_label", { n: index + 1 })} placeholder="13" bind:value={row.algorithm} />
              <input type="text" aria-label={$t("ds_digtype_aria_label", { n: index + 1 })} placeholder="2" bind:value={row.digtype} />
              <input
                type="text"
                aria-label={$t("ds_digest_aria_label", { n: index + 1 })}
                placeholder="ABCD..."
                bind:value={row.digest}
              />
              <button class="ghost mini-button" type="button" onclick={() => removeUndelegatedDSRow(row.id)}>
                {$t("remove")}
              </button>
            </div>
          {/each}
        </div>
      {/if}
      <button class="ghost" type="button" onclick={addUndelegatedDSRow}>
        {$t("add_ds_record")}
      </button>
      <div class="small">{$t("undelegated_validation_hint")}</div>
    </div>
  </details>
  <button onclick={submitSingle} disabled={singleSubmitting}>
    {singleSubmitting ? $t("submitting") : $t("run_single_job")}
  </button>
  {#if createdJobId}
    <div class="small">{$t("created_job_prefix")} <span class="mono">{createdJobId}</span></div>
  {/if}
</div>
