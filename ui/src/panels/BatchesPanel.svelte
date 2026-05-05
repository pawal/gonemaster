<script>
  import { onMount } from "svelte";
  import { t } from "../i18n.js";
  import {
    formatTimestampLocal,
    formatBatchTotalRuntime,
    formatBatchStatusCounts as formatBatchStatusCountsRaw,
    formatSnapshotSlugPreview,
  } from "../lib/format.js";
  import { progressPercent, hasActiveBatchJobs, normalizeStatus } from "../lib/jobUtils.js";
  import { normalizePageSize, normalizeCursor } from "../lib/persistence.js";

  let {
    apiFetch,
    setStatus = () => {},
    ensureNotificationPermission = () => Promise.resolve("denied"),
    selectedBatchId = $bindable(""),
    batchSort = $bindable("started_at_desc"),
    batchPageSize = $bindable(20),
    batchStatusFilter = $bindable(""),
    batchDomainFilter = $bindable(""),
    batchCursor = $bindable(0),
    availableTags = [],
    availableProfiles = [],
    profilesLoading = false,
    tagCohortByName = new Map(),
    batchSortOptions = [],
    batchStatuses = [],
    listPageSizes = [10, 20, 50, 100],
    batchDeletedCounter = 0,
    onNavigateJob = () => {},
    onOpenBatchDelete = () => {},
  } = $props();

  // ── Submit form state ────────────────────────────────────────────────────
  let batchDomains = $state("");
  let batchTags = $state("");
  let batchFromTagMode = $state(false);
  let batchFromTag = $state("");
  let batchSubmitting = $state(false);
  let createdBatchId = $state("");
  let batchProfileId = $state("");
  let batchSnapshotIntent = $state(false);
  let batchSnapshotIntentTouched = $state(false);

  // ── Inspector state ──────────────────────────────────────────────────────
  let selectedBatch = $state(null);
  let batchLoading = $state(false);
  let autoRefreshBatch = $state(false);
  let recentBatchOptions = $state([]);
  let recentBatchLoading = $state(false);
  let selectedRecentBatch = $state("");
  let activeBatches = $state([]);
  let activeBatchesLoading = $state(false);
  let queuePaused = $state(false);
  let queuePauseToggling = $state(false);

  let notifyOnBatchComplete = false;
  let lastBatchDeletedCounter;

  const formatBatchStatusCounts = (statusCounts) => formatBatchStatusCountsRaw(statusCounts, normalizeStatus);
  const normalizeBatchPageSize = (value) => normalizePageSize(value, listPageSizes, 20);

  const normalizeOptionalProfileID = (value) => {
    const parsed = Number(value);
    if (!Number.isFinite(parsed) || parsed <= 0) return null;
    return parsed;
  };
  const jobProfileName = (job, run = null) => {
    const direct = String(job?.profile_name || run?.profile_name || "").trim();
    if (direct) return direct;
    const id = normalizeOptionalProfileID(job?.profile_id || run?.profile_id);
    if (!id) return "";
    const match = availableProfiles.find((profile) => profile.id === id);
    return match?.name || `#${id}`;
  };

  const applyWidth = (node, value) => {
    node.style.width = value;
    return { update(v) { node.style.width = v; } };
  };

  // ── Snapshot checkbox derivations ────────────────────────────────────────
  const batchCohortForTag = $derived(batchFromTag ? tagCohortByName.get(batchFromTag) : null);
  const snapshotCheckboxVisible = $derived(!!(batchCohortForTag && batchCohortForTag.analysis_enabled));
  const batchSnapshotPartial = $derived(batchSnapshotIntent && !batchFromTagMode);
  const batchSnapshotSlugPreview = $derived(
    snapshotCheckboxVisible && batchSnapshotIntent ? formatSnapshotSlugPreview() : ""
  );

  $effect(() => {
    if (!snapshotCheckboxVisible && batchSnapshotIntent) {
      batchSnapshotIntent = false;
      batchSnapshotIntentTouched = false;
    }
  });
  $effect(() => {
    if (snapshotCheckboxVisible && !batchSnapshotIntentTouched) {
      batchSnapshotIntent = batchFromTagMode;
    }
  });

  function formatRecentBatchOption(option) {
    if (!option || !option.id) return "";
    const parts = [option.id];
    if (option.createdAt) {
      const parsed = new Date(option.createdAt);
      if (!Number.isNaN(parsed.getTime())) parts.push(parsed.toLocaleString("sv-SE"));
    }
    if (option.tag) parts.push(`[${option.tag}]`);
    return parts.join(" - ");
  }

  function syncSelectedRecentBatch() {
    const normalized = selectedBatchId.trim();
    if (!normalized) {
      selectedRecentBatch = "";
      return;
    }
    selectedRecentBatch = recentBatchOptions.some((option) => option.id === normalized) ? normalized : "";
  }

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

  const batchQueryParams = () => {
    const params = new URLSearchParams({
      limit: String(normalizeBatchPageSize(batchPageSize)),
      sort: batchSort,
    });
    const cursor = normalizeCursor(batchCursor);
    if (cursor > 0) params.set("cursor", String(cursor));
    if (batchStatusFilter) params.set("status", batchStatusFilter);
    const normalizedDomain = batchDomainFilter.trim();
    if (normalizedDomain) params.set("domain", normalizedDomain);
    return params;
  };

  async function submitBatch() {
    let payload;
    if (batchFromTagMode) {
      if (!batchFromTag) {
        setStatus($t("error_batch_from_tag_required"), "warn");
        return;
      }
      payload = { from_tag: batchFromTag };
    } else {
      const domains = batchDomains
        .split(/\n/)
        .map((entry) => normalizeDomainInput(entry))
        .filter(Boolean);
      if (!domains.length) {
        setStatus($t("error_batch_empty"), "warn");
        return;
      }
      payload = { domains };
    }
    const parsedTags = batchTags.split(/[,\s]+/).map((s) => s.trim()).filter(Boolean);
    const selectedProfileID = normalizeOptionalProfileID(batchProfileId);
    if (parsedTags.length > 0) payload.tags = parsedTags;
    if (selectedProfileID) payload.profile_id = selectedProfileID;
    if (batchSnapshotIntent) payload.snapshot_intent = true;
    batchSubmitting = true;
    createdBatchId = "";
    try {
      const response = await apiFetch("/jobs/batch", {
        method: "POST",
        body: JSON.stringify(payload),
      });
      createdBatchId = response.batch_id;
      selectedBatchId = response.batch_id;
      autoRefreshBatch = true;
      notifyOnBatchComplete = true;
      ensureNotificationPermission();
      setStatus($t("batch_accepted", { id: response.batch_id }), "ok");
      await loadRecentBatchOptions();
      await loadBatch(response.batch_id, { resetCursor: true });
    } catch (error) {
      setStatus($t("error_create_batch", { error: error.message }), "warn");
    } finally {
      batchSubmitting = false;
    }
  }

  async function loadRecentBatchOptions() {
    recentBatchLoading = true;
    try {
      const collected = [];
      const seen = new Set();
      let cursor = 0;
      let pages = 0;
      const maxItems = 20;
      const maxPages = 5;
      while (collected.length < maxItems && pages < maxPages) {
        const params = new URLSearchParams({ limit: "100", sort: "created_at_desc" });
        if (cursor > 0) params.set("cursor", String(cursor));
        const list = await apiFetch(`/jobs?${params.toString()}`);
        const items = list?.items || [];
        for (const item of items) {
          const batchID = String(item?.batch_id || "").trim();
          if (!batchID || seen.has(batchID)) continue;
          seen.add(batchID);
          collected.push({ id: batchID, createdAt: item?.created_at || "" });
          if (collected.length >= maxItems) break;
        }
        if (!list?.next_cursor) break;
        const nextCursor = normalizeCursor(list.next_cursor);
        if (nextCursor <= cursor) break;
        cursor = nextCursor;
        pages++;
      }
      recentBatchOptions = collected;
      syncSelectedRecentBatch();
    } catch (error) {
      setStatus($t("error_load_batches", { error: error.message }), "warn");
    } finally {
      recentBatchLoading = false;
    }
  }

  async function loadBatch(batchId = selectedBatchId, options = {}) {
    if (!batchId) return;
    const { resetCursor = false } = options;
    if (resetCursor) batchCursor = 0;
    batchLoading = true;
    try {
      const params = batchQueryParams();
      const batch = await apiFetch(`/batches/${batchId}?${params.toString()}`);
      selectedBatch = batch;
      if (batch?.tag) {
        const idx = recentBatchOptions.findIndex((o) => o.id === batchId);
        if (idx >= 0 && !recentBatchOptions[idx].tag) {
          recentBatchOptions = recentBatchOptions.map((o, i) => i === idx ? { ...o, tag: batch.tag } : o);
        }
      }
      if (notifyOnBatchComplete && !hasActiveBatchJobs(batch)) {
        notifyOnBatchComplete = false;
        sendBatchNotification(batch);
      }
      if (autoRefreshBatch && !hasActiveBatchJobs(batch)) {
        autoRefreshBatch = false;
      }
    } catch (error) {
      setStatus($t("error_load_batch", { error: error.message }), "warn");
      selectedBatch = null;
    } finally {
      batchLoading = false;
    }
  }

  async function loadActiveBatches() {
    activeBatchesLoading = true;
    try {
      const batchIds = [];
      const seen = new Set();
      let cursor = 0;
      let pages = 0;
      while (batchIds.length < 10 && pages < 3) {
        const params = new URLSearchParams({ limit: "100", sort: "created_at_desc" });
        if (cursor > 0) params.set("cursor", String(cursor));
        const list = await apiFetch(`/jobs?${params.toString()}`);
        const items = list?.items || [];
        for (const item of items) {
          const bid = String(item?.batch_id || "").trim();
          if (!bid || seen.has(bid)) continue;
          seen.add(bid);
          batchIds.push(bid);
          if (batchIds.length >= 10) break;
        }
        if (!list?.next_cursor) break;
        const next = normalizeCursor(list.next_cursor);
        if (next <= cursor) break;
        cursor = next;
        pages++;
      }
      const summaries = await Promise.all(
        batchIds.map(async (id) => {
          try {
            return await apiFetch(`/batches/${id}?limit=1&sort=started_at_desc`);
          } catch (_) {
            return null;
          }
        })
      );
      activeBatches = summaries.filter((b) => b && hasActiveBatchJobs(b));
    } catch (_) {
      // Silently ignore - active batches is supplementary.
    } finally {
      activeBatchesLoading = false;
    }
  }

  async function fetchQueueStatus() {
    try {
      const snapshot = await apiFetch("/metrics?window=1h&include=health");
      queuePaused = !!snapshot?.health?.queue_paused;
    } catch (_) {
      // Non-critical.
    }
  }

  async function toggleQueuePause() {
    if (queuePauseToggling) return;
    queuePauseToggling = true;
    try {
      const endpoint = queuePaused ? "/queue/resume" : "/queue/pause";
      await apiFetch(endpoint, { method: "POST" });
      queuePaused = !queuePaused;
      setStatus($t(queuePaused ? "queue_paused_status" : "queue_resumed_status"), "ok");
    } catch (error) {
      setStatus($t("error_queue_toggle", { error: error.message }), "warn");
    } finally {
      queuePauseToggling = false;
    }
  }

  async function applyBatchFilters() {
    batchCursor = 0;
    await loadBatch(selectedBatchId, { resetCursor: true });
  }

  async function clearBatchFilters() {
    batchSort = "started_at_desc";
    batchPageSize = 20;
    batchStatusFilter = "";
    batchDomainFilter = "";
    batchCursor = 0;
    await loadBatch(selectedBatchId, { resetCursor: true });
  }

  async function goToBatchCursor(cursor) {
    const parsed = Number(cursor);
    batchCursor = Number.isFinite(parsed) && parsed > 0 ? parsed : 0;
    await loadBatch(selectedBatchId);
  }

  async function sendBatchNotification(batch) {
    const permission = await ensureNotificationPermission();
    if (permission !== "granted") return;
    const title = $t("notify_batch_done_title");
    const body = $t("notify_batch_done_body", { id: batch.batch_id });
    try {
      new Notification(title, { body });
    } catch (err) {
      console.warn("[notify] Notification constructor failed:", err);
    }
  }

  // Reload active batches and the inspected batch when the modal deletes a batch.
  $effect(() => {
    const current = batchDeletedCounter;
    if (current === lastBatchDeletedCounter) return;
    const wasInitialized = lastBatchDeletedCounter !== undefined;
    lastBatchDeletedCounter = current;
    if (!wasInitialized) return;
    loadRecentBatchOptions();
    loadActiveBatches();
    if (selectedBatchId) loadBatch(selectedBatchId);
  });

  // Sync the recent-batches dropdown when selectedBatchId or the option list changes.
  $effect(() => {
    selectedBatchId;
    recentBatchOptions;
    syncSelectedRecentBatch();
  });

  // Polling: inspector and active-batches.
  $effect(() => {
    if (!autoRefreshBatch || !selectedBatchId) return;
    const handle = setInterval(() => loadBatch(), 7000);
    return () => clearInterval(handle);
  });
  $effect(() => {
    const handle = setInterval(() => {
      loadActiveBatches();
      fetchQueueStatus();
    }, 7000);
    return () => clearInterval(handle);
  });

  onMount(() => {
    loadRecentBatchOptions();
    loadActiveBatches();
    fetchQueueStatus();
    if (selectedBatchId) loadBatch(selectedBatchId);
  });
</script>

<div class="grid panel-mt" id="panel-batches" role="tabpanel" aria-labelledby="tab-batches">
  <div class="card reveal delay-22">
    <h2>{$t("batch_jobs_heading")}</h2>
    <div class="toolbar-row no-wrap mb-half">
      <button
        class={batchFromTagMode ? "ghost" : "secondary small"}
        type="button"
        onclick={() => { batchFromTagMode = false; }}
      >{$t("batch_domains_mode_label")}</button>
      <button
        class={batchFromTagMode ? "secondary small" : "ghost"}
        type="button"
        onclick={() => { batchFromTagMode = true; }}
      >{$t("batch_from_tag_mode_label")}</button>
    </div>
    {#if batchFromTagMode}
      <div class="stack">
        <label for="batch-from-tag">{$t("batch_from_tag_label")}</label>
        <select id="batch-from-tag" bind:value={batchFromTag}>
          <option value="">{$t("tag_filter_all")}</option>
          {#each availableTags as tag}
            <option value={tag.name}>{tag.name}{tag.domain_count ? ` (${tag.domain_count})` : ""}</option>
          {/each}
        </select>
      </div>
    {:else}
      <div class="stack">
        <label for="batch-domains">{$t("domains_label")}</label>
        <textarea
          id="batch-domains"
          placeholder={`example.com
example.org`}
          bind:value={batchDomains}
        ></textarea>
      </div>
    {/if}
    <div class="stack">
      <label for="batch-tags">{$t("batch_tags_label")}</label>
      <input
        id="batch-tags"
        type="text"
        placeholder={$t("batch_tags_placeholder")}
        bind:value={batchTags}
      />
      <div class="small">{$t("batch_tags_hint")}</div>
    </div>
    <div class="stack">
      <label for="batch-profile">{$t("stored_profile_label")}</label>
      <select id="batch-profile" bind:value={batchProfileId} disabled={profilesLoading && availableProfiles.length === 0}>
        <option value="">{$t("stored_profile_auto_option")}</option>
        {#each availableProfiles as profile}
          <option value={profile.id}>{profile.name}</option>
        {/each}
      </select>
      <div class="small">{$t("stored_profile_hint")}</div>
    </div>
    {#if snapshotCheckboxVisible}
      <div class="stack batch-snapshot-field">
        <label class="batch-snapshot-check">
          <input
            type="checkbox"
            bind:checked={batchSnapshotIntent}
            onchange={() => { batchSnapshotIntentTouched = true; }}
          />
          <span>{$t("batch_snapshot_label")}</span>
        </label>
        <div class="small">{$t("batch_snapshot_hint")}</div>
        {#if batchSnapshotIntent}
          <div class="small mono">
            {$t("batch_snapshot_slug_preview", { slug: batchSnapshotSlugPreview })}
          </div>
        {/if}
        {#if batchSnapshotPartial}
          <div class="notice notice-warn" role="status" aria-live="polite">
            {$t("batch_snapshot_partial_warning")}
          </div>
        {/if}
      </div>
    {/if}
    <button class="secondary" onclick={submitBatch} disabled={batchSubmitting}>
      {batchSubmitting ? $t("submitting") : $t("run_batch")}
    </button>
    {#if createdBatchId}
      <div class="small">{$t("created_batch_prefix")} <span class="mono">{createdBatchId}</span></div>
    {/if}
  </div>

  <div class="card reveal delay-26" data-testid="active-batches-card">
    <div class="row row-toolbar-end">
      <h2 class="m-zero">{$t("active_batches_heading")}</h2>
      <button
        class={queuePaused ? "secondary small" : "ghost small"}
        type="button"
        onclick={toggleQueuePause}
        disabled={queuePauseToggling}
        aria-busy={queuePauseToggling}
        title={$t("queue_pause_tooltip")}
      >
        {queuePauseToggling ? $t("loading") : queuePaused ? $t("queue_resume_button") : $t("queue_pause_button")}
      </button>
    </div>
    {#if queuePaused}
      <div class="status-banner warn" role="status" aria-live="polite">{$t("queue_paused_banner")}</div>
    {/if}
    {#if activeBatchesLoading && activeBatches.length === 0}
      <div class="small">{$t("loading")}</div>
    {:else if activeBatches.length === 0}
      <div class="small">{$t("no_active_batches")}</div>
    {:else}
      <div class="list">
        {#each activeBatches as batch (batch.batch_id)}
          <div class="list-item clickable" class:disabled={batchLoading} onclick={() => {
            if (batchLoading) return;
            selectedBatchId = batch.batch_id;
            loadBatch(batch.batch_id, { resetCursor: true });
          }} onkeydown={(e) => {
            if (batchLoading) return;
            if (e.key === "Enter" || e.key === " ") {
              e.preventDefault();
              selectedBatchId = batch.batch_id;
              loadBatch(batch.batch_id, { resetCursor: true });
            }
          }} role="button" tabindex="0" aria-disabled={batchLoading}>
            <div class="list-item-main">
              <div class="mono">
                {batch.batch_id}{#if batch.tag} <span class="small">({batch.tag})</span>{/if}
              </div>
              <div class="small">
                {$t("total_label")}: {batch.total} · {formatBatchStatusCounts(batch.status_counts)} · {formatBatchTotalRuntime(batch)}
              </div>
            </div>
          </div>
        {/each}
      </div>
    {/if}
  </div>

  <div class="card reveal delay-30">
    <h2>{$t("batch_inspector_heading")}</h2>
    <div class="stack">
      <label for="batch-recent">{$t("recent_batches_label")}</label>
      <select
        id="batch-recent"
        bind:value={selectedRecentBatch}
        disabled={recentBatchLoading}
        onchange={async () => {
          const nextBatchID = selectedRecentBatch.trim();
          if (!nextBatchID) return;
          selectedBatchId = nextBatchID;
          await loadBatch(nextBatchID, { resetCursor: true });
        }}
      >
        <option value="">{recentBatchLoading ? $t("loading_batches") : $t("select_recent_batch")}</option>
        {#each recentBatchOptions as option}
          <option value={option.id}>{formatRecentBatchOption(option)}</option>
        {/each}
      </select>
      <div class="small">{$t("latest_batches_hint")}</div>
      <label for="batch-id">{$t("batch_id_label")}</label>
      <input
        id="batch-id"
        type="text"
        placeholder="batch_123"
        bind:value={selectedBatchId}
        onchange={() => loadBatch(selectedBatchId, { resetCursor: true })}
      />
    </div>
    <div class="row">
      <button
        onclick={async () => {
          await loadRecentBatchOptions();
          await loadBatch();
        }}
        disabled={batchLoading}
      >
        {batchLoading ? $t("loading") : $t("refresh")}
      </button>
      <button class="ghost" type="button" onclick={() => (autoRefreshBatch = !autoRefreshBatch)}>
        {autoRefreshBatch ? $t("auto_refresh_on") : $t("auto_refresh_off")}
      </button>
      <button
        class="warn"
        type="button"
        onclick={() => onOpenBatchDelete(selectedBatchId)}
        disabled={!selectedBatchId || batchLoading}
      >
        {$t("batch_delete_button")}
      </button>
    </div>
    <div class="batch-controls">
      <div class="sort-control">
        <label for="batch-sort">{$t("sort_label")}</label>
        <select id="batch-sort" bind:value={batchSort} onchange={applyBatchFilters}>
          {#each batchSortOptions as option}
            <option value={option.id}>{$t(option.labelKey)}</option>
          {/each}
        </select>
      </div>
      <div class="sort-control">
        <label for="batch-page-size">{$t("page_size_label")}</label>
        <select id="batch-page-size" bind:value={batchPageSize} onchange={applyBatchFilters}>
          {#each listPageSizes as pageSize}
            <option value={pageSize}>{pageSize}</option>
          {/each}
        </select>
      </div>
      <div class="sort-control">
        <label for="batch-status">{$t("status_label")}</label>
        <select id="batch-status" bind:value={batchStatusFilter} onchange={applyBatchFilters}>
          {#each batchStatuses as status}
            <option value={status}>{status || $t("batch_status_all")}</option>
          {/each}
        </select>
      </div>
      <div class="sort-control grow">
        <label for="batch-domain-filter">{$t("domain_contains_label")}</label>
        <input
          id="batch-domain-filter"
          type="text"
          placeholder="example"
          bind:value={batchDomainFilter}
          onkeydown={(event) => {
            if (event.key === "Enter") {
              event.preventDefault();
              applyBatchFilters();
            }
          }}
        />
      </div>
      <div class="row">
        <button class="ghost" type="button" onclick={applyBatchFilters} disabled={batchLoading}>{$t("apply_filters")}</button>
        <button class="ghost" type="button" onclick={clearBatchFilters} disabled={batchLoading}>{$t("clear")}</button>
      </div>
    </div>
    {#if selectedBatch}
      <div class="kv">
        {#if selectedBatch.tag}
          <span>{$t("batch_tag_label")}</span>
          <strong class="mono">{selectedBatch.tag}</strong>
        {/if}
        <span>{$t("total_label")}</span>
        <strong>{selectedBatch.total}</strong>
        <span>{$t("created_label")}</span>
        <strong>{formatTimestampLocal(selectedBatch.created_at)}</strong>
        <span>{$t("total_runtime_label")}</span>
        <strong>{formatBatchTotalRuntime(selectedBatch)}</strong>
        <span>{$t("status_counts_label")}</span>
        <strong>{formatBatchStatusCounts(selectedBatch.status_counts)}</strong>
      </div>
      <div class="stack">
        <div class="field-label">{$t("jobs_label")}</div>
        <div class="row batch-pagination">
          <button
            class="ghost"
            type="button"
            onclick={() => goToBatchCursor(selectedBatch.prev_cursor)}
            disabled={!selectedBatch.prev_cursor || batchLoading}
          >
            {$t("previous")}
          </button>
          <button
            class="ghost"
            type="button"
            onclick={() => goToBatchCursor(selectedBatch.next_cursor)}
            disabled={!selectedBatch.next_cursor || batchLoading}
          >
            {$t("next")}
          </button>
          <span class="small">
            {$t("showing_jobs", { shown: selectedBatch.items.length, total: selectedBatch.total, offset: selectedBatch.offset || 0 })}
          </span>
        </div>
        <div class="list">
          {#if selectedBatch.items.length === 0}
            <div class="small">{$t("no_batch_jobs")}</div>
          {:else}
            {#each selectedBatch.items as item (item.id)}
              <div class="list-item">
                <div class="list-item-main">
                  <div class="mono">{item.id}</div>
                  <div class="small">{item.domain} - {item.status}</div>
                  {#if jobProfileName(item)}
                    <div class="small">{$t("job_profile_label")}: <span class="mono">{jobProfileName(item)}</span></div>
                  {/if}
                  <div class="progress compact list-progress" role="progressbar" aria-valuemin="0" aria-valuemax="100" aria-valuenow={progressPercent(item)}>
                    <div class="progress-bar" use:applyWidth={`${progressPercent(item)}%`}></div>
                    <span class="progress-value">{progressPercent(item)}%</span>
                  </div>
                </div>
                <button class="ghost" type="button" onclick={() => onNavigateJob(item.id)}>{$t("inspect")}</button>
              </div>
            {/each}
          {/if}
        </div>
      </div>
    {/if}
  </div>
</div>
