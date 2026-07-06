<script>
  import { onMount, untrack } from "svelte";
  import { t } from "../i18n.js";
  import {
    sortItems,
    nextTableSort,
    tableSortIndicator,
    tableSortAria,
    compareNumber,
    compareText,
    compareTimestamp,
    compareSeverity,
  } from "../lib/sort.js";
  import GradeChip from "../components/GradeChip.svelte";

  let {
    apiFetch,
    setStatus = () => {},
    clearStatus = () => {},
    routeTagName = null,
    onOpenTag = () => {},
    onCloseTag = () => {},
    availableProfiles = [],
    profilesLoading = false,
    tagCohortByName = new Map(),
    scoringEnabled = false,
    onNavigateDomainDetail = () => {},
    onSetTab = () => {},
    onOpenBatchDelete = () => {},
    onOpenBatchFromTagRow = () => {},
    onTagProfileChanged = () => {},
    onTagsListChanged = () => {},
  } = $props();

  let selectedTag = $state(null);

  let tagsList = $state([]);
  let tagsListLoading = $state(false);
  let tagsListSortState = $state({ key: "", direction: "asc" });

  let tagSummary = $state(null);
  let tagSummaryLoading = $state(false);

  let tagDomains = $state([]);
  let tagDomainsTotal = $state(0);
  let tagDomainsOffset = $state(0);
  const tagDomainsLimit = 50;
  let tagDomainsLoading = $state(false);
  let tagDomainsSortState = $state({ key: "", direction: "asc" });
  let tagDomainLevelFilter = $state("");

  let tagBatches = $state([]);
  let tagBatchesTotal = $state(0);
  let tagBatchesOffset = $state(0);
  const tagBatchesLimit = 20;
  let tagBatchesLoading = $state(false);

  let tagCreateName = $state("");
  let tagCreateDescription = $state("");
  let tagCreating = $state(false);

  let tagAddDomainsInput = $state("");
  let tagAddingDomains = $state(false);
  let tagRemoveDomainsInput = $state("");
  let tagRemovingDomains = $state(false);

  let tagRunAllSubmitting = $state(false);
  let tagDeleteConfirm = $state(false);
  let tagDeleting = $state(false);
  let tagPurgeConfirm = $state(false);
  let tagPurging = $state(false);

  let tagProfileDraftId = $state("");
  let tagProfileUpdating = $state(false);
  let tagProfileClearing = $state(false);

  let lastSelectedTagName = null;

  const domainLevel = (d) => d?.latest_level || (d?.latest_run_at ? "INFO" : "");

  const domainSortParam = (state) => {
    const map = { name: "name", latest_level: "latest_level", latest_score: "latest_score", latest_run_at: "latest_run_at", run_count: "run_count" };
    const col = map[state?.key];
    return col ? `${col}_${state.direction}` : "";
  };

  const normalizeOptionalProfileID = (value) => {
    const parsed = Number(value);
    if (!Number.isFinite(parsed) || parsed <= 0) return null;
    return parsed;
  };
  const profileNameByID = (profileID) => {
    const normalized = normalizeOptionalProfileID(profileID);
    if (!normalized) return "";
    const match = availableProfiles.find((profile) => profile.id === normalized);
    return match?.name || `#${normalized}`;
  };

  const tagProfileCurrentID = $derived(normalizeOptionalProfileID(selectedTag?.default_profile_id));
  const tagProfileSelectedID = $derived(normalizeOptionalProfileID(tagProfileDraftId));
  const tagProfileDirty = $derived(tagProfileCurrentID !== tagProfileSelectedID);

  const sortedTagDomains = $derived(
    sortItems(tagDomains, tagDomainsSortState, {
      name: (left, right) => compareText(left?.name, right?.name),
      latest_level: (left, right) => compareSeverity(domainLevel(left), domainLevel(right)),
      latest_score: (left, right) => compareNumber(left?.latest_score, right?.latest_score),
      latest_run_at: (left, right) => compareTimestamp(left?.latest_run_at, right?.latest_run_at),
    }, (left, right) => compareText(left?.name, right?.name))
  );

  const sortedTagsList = $derived(
    sortItems(tagsList, tagsListSortState, {
      name: (left, right) => compareText(left?.name, right?.name),
      description: (left, right) => compareText(left?.description, right?.description),
      domain_count: (left, right) => compareNumber(left?.domain_count, right?.domain_count),
    }, (left, right) => compareText(left?.name, right?.name))
  );

  async function loadTagsList() {
    tagsListLoading = true;
    try {
      const data = await apiFetch("/tags");
      tagsList = Array.isArray(data) ? data : [];
    } catch (error) {
      setStatus($t("tags_load_error", { error: error.message || "unknown error" }), "warn");
    } finally {
      tagsListLoading = false;
    }
  }

  async function loadTagSummary() {
    if (!selectedTag) return;
    tagSummaryLoading = true;
    try {
      tagSummary = await apiFetch(`/tags/${encodeURIComponent(selectedTag.name)}/summary`);
    } catch (_) {
      tagSummary = null;
    } finally {
      tagSummaryLoading = false;
    }
  }

  async function loadTagDomains(options = {}) {
    if (!selectedTag) return;
    const { reset = false } = options;
    if (reset) tagDomainsOffset = 0;
    tagDomainsLoading = true;
    try {
      const params = new URLSearchParams({ limit: String(tagDomainsLimit), offset: String(tagDomainsOffset) });
      if (tagDomainLevelFilter) params.set("min_level", tagDomainLevelFilter);
      const sortVal = domainSortParam(tagDomainsSortState);
      if (sortVal) params.set("sort", sortVal);
      const data = await apiFetch(`/tags/${encodeURIComponent(selectedTag.name)}/domains?${params}`);
      tagDomains = data?.items ?? [];
      tagDomainsTotal = data?.total ?? 0;
    } catch (error) {
      setStatus($t("tag_domains_load_error", { error: error.message || "unknown error" }), "warn");
    } finally {
      tagDomainsLoading = false;
    }
  }

  async function loadTagBatches(options = {}) {
    if (!selectedTag) return;
    const { reset = false } = options;
    if (reset) tagBatchesOffset = 0;
    tagBatchesLoading = true;
    try {
      const params = new URLSearchParams({ limit: String(tagBatchesLimit), offset: String(tagBatchesOffset) });
      const data = await apiFetch(`/tags/${encodeURIComponent(selectedTag.name)}/batches?${params}`);
      tagBatches = data?.items ?? [];
      tagBatchesTotal = data?.total ?? 0;
    } catch (error) {
      setStatus($t("tag_domains_load_error", { error: error.message || "unknown error" }), "warn");
    } finally {
      tagBatchesLoading = false;
    }
  }

  async function toggleBatchSnapshotIntent(batch) {
    const next = !batch.snapshot_intent;
    try {
      await apiFetch(`/batches/${encodeURIComponent(batch.id)}`, {
        method: "PATCH",
        body: JSON.stringify({ snapshot_intent: next }),
      });
      setStatus($t(next
        ? "batch_snapshot_intent_enabled"
        : "batch_snapshot_intent_disabled", { id: batch.id }), "ok");
      await loadTagBatches();
    } catch (error) {
      setStatus($t("batch_snapshot_intent_error", { error: error.message || "unknown error" }), "warn");
    }
  }

  const sortTagDomains = (key, defaultDir = "asc") => {
    tagDomainsSortState = nextTableSort(tagDomainsSortState, key, defaultDir);
    loadTagDomains({ reset: true });
  };

  function syncTagProfileLocal(tagName, profileID) {
    const nextValue = normalizeOptionalProfileID(profileID);
    if (selectedTag?.name === tagName) {
      selectedTag = { ...selectedTag, default_profile_id: nextValue };
    }
    tagsList = tagsList.map((tag) =>
      tag.name === tagName ? { ...tag, default_profile_id: nextValue } : tag
    );
    tagProfileDraftId = nextValue ? String(nextValue) : "";
    onTagProfileChanged(tagName, nextValue);
  }

  async function createTag() {
    const name = tagCreateName.trim();
    if (!name) return;
    tagCreating = true;
    try {
      await apiFetch("/tags", {
        method: "POST",
        body: JSON.stringify({ name, description: tagCreateDescription.trim() }),
      });
      tagCreateName = "";
      tagCreateDescription = "";
      setStatus($t("tag_created"), "ok");
      await loadTagsList();
      onTagsListChanged();
    } catch (error) {
      setStatus($t("tag_create_error", { error: error.message || "unknown error" }), "warn");
    } finally {
      tagCreating = false;
    }
  }

  async function deleteTag() {
    if (!selectedTag) return;
    tagDeleting = true;
    try {
      await apiFetch(`/tags/${encodeURIComponent(selectedTag.name)}`, { method: "DELETE" });
      setStatus($t("tag_deleted"), "ok");
      selectedTag = null;
      tagProfileDraftId = "";
      tagDeleteConfirm = false;
      onCloseTag();
      await loadTagsList();
      onTagsListChanged();
    } catch (error) {
      setStatus($t("tag_delete_error", { error: error.message || "unknown error" }), "warn");
    } finally {
      tagDeleting = false;
    }
  }

  async function runAllFromTag() {
    if (!selectedTag) return;
    tagRunAllSubmitting = true;
    try {
      const response = await apiFetch("/jobs/batch", {
        method: "POST",
        body: JSON.stringify({ from_tag: selectedTag.name }),
      });
      const id = response.batch_id || "";
      setStatus($t("batch_accepted", { id }), "ok");
      onOpenBatchFromTagRow(id);
    } catch (error) {
      setStatus($t("tag_run_all_error", { error: error.message || "unknown error" }), "warn");
    } finally {
      tagRunAllSubmitting = false;
    }
  }

  async function purgeTagRuns() {
    if (!selectedTag) return;
    tagPurging = true;
    try {
      const response = await apiFetch(`/tags/${encodeURIComponent(selectedTag.name)}/purge`, {
        method: "POST",
      });
      tagPurgeConfirm = false;
      setStatus($t("tag_purge_success", { count: response.purged_runs ?? 0 }), "ok");
      await loadTagSummary();
      await loadTagDomains();
    } catch (error) {
      setStatus($t("tag_purge_error", { error: error.message || "unknown error" }), "warn");
    } finally {
      tagPurging = false;
    }
  }

  async function saveTagProfile() {
    if (!selectedTag || !tagProfileSelectedID || !tagProfileDirty) return;
    tagProfileUpdating = true;
    try {
      await apiFetch(`/tags/${encodeURIComponent(selectedTag.name)}/profile`, {
        method: "PUT",
        body: JSON.stringify({ profile_id: tagProfileSelectedID }),
      });
      const tagName = selectedTag.name;
      const profileID = tagProfileSelectedID;
      syncTagProfileLocal(tagName, profileID);
      setStatus($t("tag_profile_saved", { name: profileNameByID(profileID) }), "ok");
      await loadTagsList();
    } catch (error) {
      setStatus($t("tag_profile_save_error", { error: error.message || "unknown error" }), "warn");
    } finally {
      tagProfileUpdating = false;
    }
  }

  async function clearTagProfile() {
    if (!selectedTag || !tagProfileCurrentID) return;
    tagProfileClearing = true;
    try {
      await apiFetch(`/tags/${encodeURIComponent(selectedTag.name)}/profile`, {
        method: "DELETE",
      });
      syncTagProfileLocal(selectedTag.name, null);
      setStatus($t("tag_profile_cleared"), "ok");
      await loadTagsList();
    } catch (error) {
      setStatus($t("tag_profile_clear_error", { error: error.message || "unknown error" }), "warn");
    } finally {
      tagProfileClearing = false;
    }
  }

  async function addTagDomains() {
    if (!selectedTag || !tagAddDomainsInput.trim()) return;
    const domains = tagAddDomainsInput.split(/[\n,]+/).map((s) => s.trim()).filter(Boolean);
    if (domains.length === 0) return;
    tagAddingDomains = true;
    try {
      await apiFetch(`/tags/${encodeURIComponent(selectedTag.name)}/domains`, {
        method: "POST",
        body: JSON.stringify({ domains }),
      });
      tagAddDomainsInput = "";
      setStatus($t("tag_domains_added"), "ok");
      await loadTagDomains({ reset: true });
      await loadTagSummary();
    } catch (error) {
      setStatus($t("tag_domains_add_error", { error: error.message || "unknown error" }), "warn");
    } finally {
      tagAddingDomains = false;
    }
  }

  async function removeTagDomains() {
    if (!selectedTag || !tagRemoveDomainsInput.trim()) return;
    const domains = tagRemoveDomainsInput.split(/[\n,]+/).map((s) => s.trim()).filter(Boolean);
    if (domains.length === 0) return;
    tagRemovingDomains = true;
    try {
      await apiFetch(`/tags/${encodeURIComponent(selectedTag.name)}/domains`, {
        method: "DELETE",
        body: JSON.stringify({ domains }),
      });
      tagRemoveDomainsInput = "";
      setStatus($t("tag_domains_removed"), "ok");
      await loadTagDomains({ reset: true });
      await loadTagSummary();
    } catch (error) {
      setStatus($t("tag_domains_remove_error", { error: error.message || "unknown error" }), "warn");
    } finally {
      tagRemovingDomains = false;
    }
  }

  function navigateToTagDetail(tag) {
    clearStatus();
    selectedTag = tag;
    onOpenTag(tag.name);
  }

  // Resolve selectedTag from the routed tag name (row click, deep link, or
  // browser back/forward). The list dep re-resolves a deep link once loaded.
  $effect(() => {
    const name = routeTagName;
    const list = tagsList;
    untrack(() => {
      if (name === (selectedTag?.name ?? null)) return;
      if (!name) {
        selectedTag = null;
        return;
      }
      const match = (list || []).find((tg) => tg.name === name);
      if (match) selectedTag = match;
    });
  });

  // React to selectedTag changes to load or reset the tag detail.
  $effect(() => {
    const name = selectedTag?.name ?? null;
    if (name === lastSelectedTagName) return;
    lastSelectedTagName = name;
    tagDomainsOffset = 0;
    tagBatchesOffset = 0;
    tagDomainLevelFilter = "";
    tagDeleteConfirm = false;
    tagPurgeConfirm = false;
    tagProfileDraftId = selectedTag?.default_profile_id ? String(selectedTag.default_profile_id) : "";
    if (name == null) {
      tagSummary = null;
      tagDomains = [];
      tagBatches = [];
    } else {
      loadTagSummary();
      loadTagDomains();
      loadTagBatches({ reset: true });
    }
  });

  onMount(() => {
    loadTagsList();
  });
</script>

<div class="card reveal delay-34 panel-mt" id="panel-tags" role="tabpanel" aria-labelledby="tab-tags">
  {#if selectedTag}
    <button class="secondary small" onclick={() => { selectedTag = null; tagProfileDraftId = ""; onCloseTag(); }}>{$t("back_to_tags")}</button>
    <h2 class="mt-half">{$t("batch_tag_label")}: {selectedTag.name}</h2>
    {#if tagCohortByName.has(selectedTag.name)}
      <p class="small heading-tight">
        {$t("tag_cohort_source_prefix")}
        <button type="button" class="link-button" onclick={() => onSetTab("cohorts")}>
          {tagCohortByName.get(selectedTag.name).label || tagCohortByName.get(selectedTag.name).source_tag}
        </button>
      </p>
    {/if}

    {#if tagSummaryLoading}
      <p class="muted">{$t("loading")}</p>
    {:else if tagSummary}
      <div class="toolbar-row gap-1 mb-one">
        <span class="level-pill severity-info">{$t("sev_ok")} {tagSummary.ok}</span>
        <span class="level-pill severity-notice">{$t("sev_notice")} {tagSummary.notice}</span>
        <span class="level-pill severity-warning">{$t("sev_warning")} {tagSummary.warning}</span>
        <span class="level-pill severity-error">{$t("sev_error")} {tagSummary.error}</span>
        <span class="level-pill severity-critical">{$t("sev_critical")} {tagSummary.critical}</span>
      </div>
    {/if}

    <div class="toolbar-row mb-one">
      <button class="secondary" onclick={runAllFromTag} disabled={tagRunAllSubmitting}>
        {tagRunAllSubmitting ? $t("submitting") : $t("tag_run_all_button")}
      </button>
      {#if tagPurgeConfirm}
        <button class="warn" onclick={purgeTagRuns} disabled={tagPurging}>{tagPurging ? $t("submitting") : $t("tag_purge_confirm_button")}</button>
        <button class="ghost" onclick={() => { tagPurgeConfirm = false; }}>{$t("tag_purge_cancel_button")}</button>
      {:else}
        <button class="ghost" onclick={() => { tagPurgeConfirm = true; }}>{$t("tag_purge_button")}</button>
      {/if}
      {#if tagDeleteConfirm}
        <button class="warn" onclick={deleteTag} disabled={tagDeleting}>{tagDeleting ? $t("submitting") : $t("tag_delete_confirm_button")}</button>
        <button class="ghost" onclick={() => { tagDeleteConfirm = false; }}>{$t("tag_delete_cancel_button")}</button>
      {:else}
        <button class="ghost" onclick={() => { tagDeleteConfirm = true; }}>{$t("tag_delete_button")}</button>
      {/if}
    </div>

    <h3>{$t("earlier_batches_heading")}</h3>
    {#if tagBatchesLoading}
      <p class="muted">{$t("loading")}</p>
    {:else if tagBatches.length === 0}
      <p class="muted">{$t("earlier_batches_empty")}</p>
    {:else}
      <table class="data-table">
        <thead><tr>
          <th>{$t("batch_id_label")}</th>
          <th>{$t("col_created_at")}</th>
          <th>{$t("col_domain_count")}</th>
          <th></th>
        </tr></thead>
        <tbody>
          {#each tagBatches as b (b.id)}
            <tr
              class="row-clickable"
              onclick={(e) => { if (e.target.closest("[data-row-action]")) return; onOpenBatchFromTagRow(b.id); }}
              onkeydown={(e) => { if (e.key === "Enter" || e.key === " ") onOpenBatchFromTagRow(b.id); }}
              role="button"
              tabindex="0"
            >
              <td class="mono">
                {b.id}
                {#if b.snapshot_intent}<span class="pill snapshot-intent ml-quarter">{$t("batch_snapshot_intent_pill")}</span>{/if}
              </td>
              <td>{b.created_at ? b.created_at.slice(0, 19).replace("T", " ") : "-"}</td>
              <td>{b.domain_count ?? "-"}</td>
              <td class="text-right" data-row-action>
                <button
                  class="ghost small"
                  type="button"
                  data-row-action
                  title={$t("batch_snapshot_intent_toggle_title")}
                  onclick={() => toggleBatchSnapshotIntent(b)}
                >{b.snapshot_intent
                  ? $t("batch_snapshot_intent_disable")
                  : $t("batch_snapshot_intent_enable")}</button>
                <button
                  class="ghost small warn"
                  type="button"
                  data-row-action
                  onclick={() => onOpenBatchDelete(b.id)}
                >{$t("batch_delete_button")}</button>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
      <div class="pagination">
        <button class="secondary small" disabled={tagBatchesOffset === 0}
          onclick={() => { tagBatchesOffset = Math.max(0, tagBatchesOffset - tagBatchesLimit); loadTagBatches(); }}
        >{$t("prev_page")}</button>
        <span class="muted small">
          {$t("earlier_batches_pagination_label", {
            from: tagBatchesOffset + 1,
            to: Math.min(tagBatchesOffset + tagBatchesLimit, tagBatchesTotal),
            total: tagBatchesTotal,
          })}
        </span>
        <button class="secondary small" disabled={tagBatchesOffset + tagBatchesLimit >= tagBatchesTotal}
          onclick={() => { tagBatchesOffset += tagBatchesLimit; loadTagBatches(); }}
        >{$t("next_page")}</button>
      </div>
    {/if}

    <h3>{$t("tag_default_profile_heading")}</h3>
    <div class="stack config-form">
      <label for="tag-default-profile">{$t("tag_default_profile_label")}</label>
      <select id="tag-default-profile" bind:value={tagProfileDraftId} disabled={profilesLoading && availableProfiles.length === 0}>
        <option value="">{$t("tag_default_profile_none_option")}</option>
        {#each availableProfiles as profile}
          <option value={profile.id}>{profile.name}</option>
        {/each}
      </select>
      <div class="small">{$t("tag_default_profile_hint")}</div>
      <div class="small">
        {#if tagProfileCurrentID}
          {$t("tag_default_profile_current", { name: profileNameByID(tagProfileCurrentID) })}
        {:else}
          {$t("tag_default_profile_current_none")}
        {/if}
      </div>
      <div class="row">
        <button
          class="secondary small"
          onclick={saveTagProfile}
          disabled={!tagProfileDirty || !tagProfileSelectedID || tagProfileUpdating || tagProfileClearing}
        >
          {tagProfileUpdating ? $t("submitting") : $t("tag_default_profile_save_button")}
        </button>
        <button
          class="ghost small"
          onclick={clearTagProfile}
          disabled={!tagProfileCurrentID || tagProfileUpdating || tagProfileClearing}
        >
          {tagProfileClearing ? $t("submitting") : $t("tag_default_profile_clear_button")}
        </button>
      </div>
    </div>

    <h3>{$t("tag_domains_heading")}</h3>
    <div class="toolbar-row mb-half">
      <select
        bind:value={tagDomainLevelFilter}
        onchange={() => loadTagDomains({ reset: true })}
        aria-label={$t("level_filter_label")}
        class="fb-180-shrink"
      >
        <option value="">{$t("level_filter_all")}</option>
        <option value="WARNING">{$t("level_filter_warning_plus")}</option>
        <option value="ERROR">{$t("level_filter_error_plus")}</option>
      </select>
    </div>
    {#if tagDomainsLoading}
      <p class="muted">{$t("loading")}</p>
    {:else if tagDomains.length === 0}
      <p class="muted">{$t("no_domains")}</p>
    {:else}
      <table class="data-table">
        <thead><tr>
          <th class="sortable-column" aria-sort={tableSortAria(tagDomainsSortState, "name")}><button class="table-sort-button" type="button" onclick={() => { sortTagDomains("name"); }}><span>{$t("col_domain_name")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(tagDomainsSortState, "name")}</span></button></th>
          <th class="sortable-column" aria-sort={tableSortAria(tagDomainsSortState, "latest_level")}><button class="table-sort-button" type="button" onclick={() => { sortTagDomains("latest_level", "desc"); }}><span>{$t("col_latest_level")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(tagDomainsSortState, "latest_level")}</span></button></th>
          {#if scoringEnabled}<th class="sortable-column" aria-sort={tableSortAria(tagDomainsSortState, "latest_score")}><button class="table-sort-button" type="button" onclick={() => { sortTagDomains("latest_score", "desc"); }}><span>{$t("col_score")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(tagDomainsSortState, "latest_score")}</span></button></th>{/if}
          <th class="sortable-column" aria-sort={tableSortAria(tagDomainsSortState, "latest_run_at")}><button class="table-sort-button" type="button" onclick={() => { sortTagDomains("latest_run_at", "desc"); }}><span>{$t("col_latest_run_at")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(tagDomainsSortState, "latest_run_at")}</span></button></th>
        </tr></thead>
        <tbody>
          {#each sortedTagDomains as d}
            <tr
              class="row-clickable"
              onclick={() => onNavigateDomainDetail(d)}
              role="button"
              tabindex="0"
              onkeydown={(e) => { if (e.key === "Enter" || e.key === " ") onNavigateDomainDetail(d); }}
            >
              <td class="mono">{d.name}</td>
              <td>{#if domainLevel(d)}<span class="badge level-{domainLevel(d).toLowerCase()}">{domainLevel(d)}</span>{:else}-{/if}</td>
              {#if scoringEnabled}<td>{#if d.latest_grade != null && d.latest_score != null}<GradeChip grade={d.latest_grade} score={d.latest_score} />{:else}-{/if}</td>{/if}
              <td>{d.latest_run_at ? d.latest_run_at.slice(0, 10) : "-"}</td>
            </tr>
          {/each}
        </tbody>
      </table>
      <div class="pagination">
        <button class="secondary small" disabled={tagDomainsOffset === 0}
          onclick={() => { tagDomainsOffset = Math.max(0, tagDomainsOffset - tagDomainsLimit); loadTagDomains(); }}
        >{$t("prev_page")}</button>
        <span class="muted small">{tagDomainsOffset + 1}–{Math.min(tagDomainsOffset + tagDomainsLimit, tagDomainsTotal)} / {tagDomainsTotal}</span>
        <button class="secondary small" disabled={tagDomainsOffset + tagDomainsLimit >= tagDomainsTotal}
          onclick={() => { tagDomainsOffset += tagDomainsLimit; loadTagDomains(); }}
        >{$t("next_page")}</button>
      </div>
    {/if}

    <h3 class="mt-1-25">{$t("tag_add_domains_heading")}</h3>
    <textarea
      bind:value={tagAddDomainsInput}
      placeholder={$t("tag_domains_placeholder")}
      rows="3"
      class="input-fluid"
    ></textarea>
    <button class="secondary" onclick={addTagDomains} disabled={tagAddingDomains}>
      {tagAddingDomains ? $t("submitting") : $t("tag_add_domains_button")}
    </button>

    <h3 class="mt-1-25">{$t("tag_remove_domains_heading")}</h3>
    <textarea
      bind:value={tagRemoveDomainsInput}
      placeholder={$t("tag_domains_placeholder")}
      rows="3"
      class="input-fluid"
    ></textarea>
    <button class="ghost" onclick={removeTagDomains} disabled={tagRemovingDomains}>
      {tagRemovingDomains ? $t("submitting") : $t("tag_remove_domains_button")}
    </button>

  {:else}
    <h2>{$t("tags_tab_heading")}</h2>

    <div class="toolbar-row mb-one align-end">
      <div class="stack fb-160-grow">
        <label for="tag-create-name">{$t("tag_name_label")}</label>
        <input id="tag-create-name" type="text" bind:value={tagCreateName} placeholder="my-tag" />
      </div>
      <div class="stack fb-240-grow-2">
        <label for="tag-create-desc">{$t("tag_description_label")}</label>
        <input id="tag-create-desc" type="text" bind:value={tagCreateDescription} placeholder={$t("tag_description_placeholder")} onkeydown={(e) => { if (e.key === 'Enter') createTag(); }} />
      </div>
      <button class="secondary" onclick={createTag} disabled={tagCreating || !tagCreateName.trim()}>
        {tagCreating ? $t("submitting") : $t("tag_create_button")}
      </button>
    </div>

    {#if tagsListLoading}
      <p class="muted">{$t("loading")}</p>
    {:else if tagsList.length === 0}
      <p class="muted">{$t("no_tags")}</p>
    {:else}
      <table class="data-table">
        <thead><tr>
          <th class="sortable-column" aria-sort={tableSortAria(tagsListSortState, "name")}><button class="table-sort-button" type="button" onclick={() => { tagsListSortState = nextTableSort(tagsListSortState, "name"); }}><span>{$t("tag_name_label")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(tagsListSortState, "name")}</span></button></th>
          <th>{$t("tag_cohort_col_header")}</th>
          <th class="sortable-column" aria-sort={tableSortAria(tagsListSortState, "description")}><button class="table-sort-button" type="button" onclick={() => { tagsListSortState = nextTableSort(tagsListSortState, "description"); }}><span>{$t("tag_description_label")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(tagsListSortState, "description")}</span></button></th>
          <th class="sortable-column" aria-sort={tableSortAria(tagsListSortState, "domain_count")}><button class="table-sort-button" type="button" onclick={() => { tagsListSortState = nextTableSort(tagsListSortState, "domain_count", "desc"); }}><span>{$t("col_domain_count")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(tagsListSortState, "domain_count")}</span></button></th>
          <th>{$t("analysis_cohorts_col_actions")}</th>
        </tr></thead>
        <tbody>
          {#each sortedTagsList as tag}
            <tr
              class="row-clickable"
              onclick={() => navigateToTagDetail(tag)}
              role="button"
              tabindex="0"
              onkeydown={(e) => { if (e.key === "Enter" || e.key === " ") navigateToTagDetail(tag); }}
            >
              <td class="mono">{tag.name}</td>
              <td>
                {#if tagCohortByName.has(tag.name)}
                  <button
                    type="button"
                    class="tag-cohort-chip"
                    title={$t("tag_cohort_link_title", { cohort: tagCohortByName.get(tag.name).label || tagCohortByName.get(tag.name).source_tag })}
                    onclick={(e) => { e.stopPropagation(); onSetTab("cohorts"); }}
                  >
                    {tagCohortByName.get(tag.name).label || tagCohortByName.get(tag.name).source_tag}
                  </button>
                {:else}
                  -
                {/if}
              </td>
              <td>{tag.description || "-"}</td>
              <td>{tag.domain_count ?? 0}</td>
              <td class="text-right">
                <button
                  class="secondary small"
                  type="button"
                  onclick={(e) => { e.stopPropagation(); navigateToTagDetail(tag); }}
                >
                  {$t("earlier_batches_heading")}
                </button>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}
  {/if}
</div>
