<script>
  import { onMount, untrack } from "svelte";
  import { t } from "../i18n.js";
  import {
    formatTimestampLocal,
    formatBatchTotalRuntime,
    formatBatchStatusCounts as formatBatchStatusCountsRaw,
    formatSnapshotSlugPreview,
  } from "../lib/format.js";
  import { progressPercent, hasActiveBatchJobs, normalizeStatus } from "../lib/jobUtils.js";
  import { normalizePageSize, normalizeCursor } from "../lib/persistence.js";
  import { moduleLevels, hasScore, chipGrade, chipScore } from "../lib/result.js";
  import GradeChip from "../components/GradeChip.svelte";
  import { href } from "../lib/router.svelte.js";

  let {
    apiFetch,
    setStatus = () => {},
    onWatchBatch = () => {},
    scoringEnabled = false,
    routeBatchId = null,
    onOpenBatch = () => {},
    onCloseBatch = () => {},
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

  let selectedBatchId = $state("");

  // ── List state ───────────────────────────────────────────────────────────
  let batchesList = $state([]);
  let batchesListTotal = $state(0);
  let batchesListOffset = $state(0);
  let batchesListLoading = $state(false);
  const batchesListLimit = 20;

  // ── Inspector state ──────────────────────────────────────────────────────
  let selectedBatch = $state(null);
  let batchLoading = $state(false);
  let autoRefreshBatch = $state(false);
  let queuePaused = $state(false);
  let queuePauseToggling = $state(false);

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

  const jobSeverityRows = (job) =>
    moduleLevels
      .map((level) => ({ level, count: Number(job?.severity_totals?.[level] || 0) }))
      .filter((entry) => entry.count > 0);

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
      autoRefreshBatch = true;
      onWatchBatch(response.batch_id);
      setStatus($t("batch_accepted", { id: response.batch_id }), "ok");
      onOpenBatch(response.batch_id);
    } catch (error) {
      setStatus($t("error_create_batch", { error: error.message }), "warn");
    } finally {
      batchSubmitting = false;
    }
  }

  async function loadBatchesList(options = {}) {
    const { reset = false, silent = false } = options;
    if (reset) batchesListOffset = 0;
    if (!silent) batchesListLoading = true;
    try {
      const params = new URLSearchParams({
        limit: String(batchesListLimit),
        offset: String(batchesListOffset),
      });
      const list = await apiFetch(`/batches?${params}`);
      batchesList = list?.items || [];
      batchesListTotal = Number.isFinite(Number(list?.total)) ? Number(list.total) : batchesList.length;
    } catch (error) {
      if (!silent) setStatus($t("error_load_batches", { error: error.message || $t("error_unknown") }), "warn");
    } finally {
      if (!silent) batchesListLoading = false;
    }
  }

  async function loadBatch(batchId = selectedBatchId, options = {}) {
    if (!batchId) return;
    const { resetCursor = false, silent = false } = options;
    if (resetCursor) batchCursor = 0;
    batchLoading = true;
    try {
      const params = batchQueryParams();
      const batch = await apiFetch(`/batches/${batchId}?${params.toString()}`);
      selectedBatch = batch;
      if (autoRefreshBatch && !hasActiveBatchJobs(batch)) {
        autoRefreshBatch = false;
      }
    } catch (error) {
      setStatus($t("error_load_batch", { error: error.message }), "warn");
      // Keep the currently shown batch on a transient auto-refresh failure;
      // only clear on an explicit (non-silent) load.
      if (!silent) selectedBatch = null;
    } finally {
      batchLoading = false;
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

  // Reload the list and the inspected batch when the modal deletes a batch.
  $effect(() => {
    const current = batchDeletedCounter;
    if (current === lastBatchDeletedCounter) return;
    const wasInitialized = lastBatchDeletedCounter !== undefined;
    lastBatchDeletedCounter = current;
    if (!wasInitialized) return;
    loadBatchesList({ silent: true });
    if (selectedBatchId) loadBatch(selectedBatchId, { silent: true });
  });

  // Drive the selected batch from the route: enter detail on a batch id,
  // return to the list otherwise.
  let lastRoutedBatchId = null;
  $effect(() => {
    const id = routeBatchId || "";
    untrack(() => {
      if (id === lastRoutedBatchId) return;
      lastRoutedBatchId = id;
      selectedBatchId = id;
      if (id) {
        loadBatch(id);
      } else {
        selectedBatch = null;
        autoRefreshBatch = false;
        loadBatchesList();
      }
    });
  });

  // Poll the inspected batch while auto-refresh is on.
  $effect(() => {
    if (!autoRefreshBatch || !selectedBatchId) return;
    const handle = setInterval(() => loadBatch(selectedBatchId, { silent: true }), 7000);
    return () => clearInterval(handle);
  });
  // Poll the list and queue status while viewing the list.
  $effect(() => {
    const handle = setInterval(() => {
      if (!selectedBatchId) loadBatchesList({ silent: true });
      fetchQueueStatus();
    }, 7000);
    return () => clearInterval(handle);
  });

  onMount(() => {
    fetchQueueStatus();
  });
</script>

<div class="grid panel-mt batches-panel-grid" id="panel-batches" role="tabpanel" aria-labelledby="tab-batches">
{#if selectedBatchId}
  <div class="card reveal delay-22 grid-span-full">
    <button class="secondary small" onclick={() => onCloseBatch()}>{$t("back_to_batches")}</button>
    <h2 class="mono">{selectedBatchId}</h2>
    <div class="row">
      <button onclick={() => loadBatch(selectedBatchId, { resetCursor: true })} disabled={batchLoading}>
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
              <div
                class="list-item clickable"
                onclick={() => onNavigateJob(item.id)}
                onkeydown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); onNavigateJob(item.id); } }}
                role="button"
                tabindex="0"
              >
                <div class="list-item-main">
                  <div class="job-headline">
                    <a
                      class="mono job-id-link"
                      href={href("single", { jobId: item.id })}
                      onclick={(e) => { e.preventDefault(); e.stopPropagation(); onNavigateJob(item.id); }}
                    >{item.id}</a>
                    <span class="small">{item.domain} - {item.status}</span>
                    {#if jobSeverityRows(item).length}
                      {#each jobSeverityRows(item) as entry (entry.level)}
                        <span class={`level-pill severity-${entry.level.toLowerCase()}`}>{entry.level} {entry.count}</span>
                      {/each}
                    {:else if item.severity_totals !== undefined}
                      <span class="level-pill severity-info">INFO</span>
                    {/if}
                    {#if scoringEnabled && hasScore(item)}
                      <GradeChip grade={chipGrade(item)} score={chipScore(item)} />
                    {/if}
                  </div>
                  {#if jobProfileName(item)}
                    <div class="small">{$t("job_profile_label")}: <span class="mono">{jobProfileName(item)}</span></div>
                  {/if}
                  <div class="progress compact list-progress" role="progressbar" aria-valuemin="0" aria-valuemax="100" aria-valuenow={progressPercent(item)}>
                    <div class="progress-bar" use:applyWidth={`${progressPercent(item)}%`}></div>
                    <span class="progress-value">{progressPercent(item)}%</span>
                  </div>
                </div>
              </div>
            {/each}
          {/if}
        </div>
      </div>
    {/if}
  </div>
{:else}
  <div class="card reveal delay-22 batch-form-card">
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

  <div class="card reveal delay-26 batch-list-card" data-testid="batches-list-card">
    <div class="row row-toolbar-end">
      <h2 class="m-zero">{$t("batches_list_heading")}</h2>
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
    {#if batchesListLoading && batchesList.length === 0}
      <p class="muted">{$t("loading")}</p>
    {:else if batchesList.length === 0}
      <p class="muted">{$t("no_batches")}</p>
    {:else}
      <table class="data-table">
        <thead>
          <tr>
            <th>{$t("batch_id_label")}</th>
            <th>{$t("batch_tag_label")}</th>
            <th>{$t("status_label")}</th>
            <th>{$t("col_created_at")}</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {#each batchesList as b (b.batch_id)}
            <tr
              class="row-clickable"
              onclick={(e) => { if (e.target.closest("[data-row-action]")) return; onOpenBatch(b.batch_id); }}
              onkeydown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); onOpenBatch(b.batch_id); } }}
              role="button"
              tabindex="0"
            >
              <td class="mono"><a href={href("batches", { batchId: b.batch_id })} onclick={(e) => { e.preventDefault(); e.stopPropagation(); onOpenBatch(b.batch_id); }}>{b.batch_id}</a></td>
              <td>{b.tag || "-"}</td>
              <td>{b.status}{#if b.completion != null} · {b.completion}%{/if}</td>
              <td>{b.created_at ? formatTimestampLocal(b.created_at) : "-"}</td>
              <td class="text-right" data-row-action>
                <button class="ghost small warn" type="button" data-row-action onclick={() => onOpenBatchDelete(b.batch_id)}>
                  {$t("batch_delete_button")}
                </button>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
      <div class="pagination">
        <button
          class="secondary small"
          disabled={batchesListOffset === 0}
          onclick={() => { batchesListOffset = Math.max(0, batchesListOffset - batchesListLimit); loadBatchesList(); }}
        >{$t("prev_page")}</button>
        <span class="muted small">{batchesListOffset + 1}-{Math.min(batchesListOffset + batchesListLimit, batchesListTotal)} / {batchesListTotal}</span>
        <button
          class="secondary small"
          disabled={batchesListOffset + batchesListLimit >= batchesListTotal}
          onclick={() => { batchesListOffset += batchesListLimit; loadBatchesList(); }}
        >{$t("next_page")}</button>
      </div>
    {/if}
  </div>
{/if}
</div>
