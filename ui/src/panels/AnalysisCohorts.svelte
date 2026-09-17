<script>
  import { onMount, onDestroy } from "svelte";
  import { t } from "../i18n.js";
  import { apiCall } from "../lib/api.js";
  import { formatTimestampLocal } from "../lib/format.js";
  import { href, navigate } from "../lib/router.svelte.js";
  import {
    compareText,
    compareNumber,
    compareTimestamp,
    nextTableSort,
    tableSortAria,
    tableSortIndicator,
    sortItems,
  } from "../lib/sort.js";
  import InlineNotice from "../components/InlineNotice.svelte";
  import ConfirmDialog from "../components/ConfirmDialog.svelte";

  let { apiBase = "/api/v1", onDeleteBatch = null, refreshSignal = 0 } = $props();

  let loading = $state(false);
  let loadError = $state("");
  let cohorts = $state([]);
  let noticeMessage = $state("");
  let noticeTone = $state("");
  let confirmState = $state(null);

  const confirmProps = $derived.by(() => {
    const s = confirmState;
    if (!s) return null;
    if (s.kind === "delete-cohort") {
      return {
        title: $t("analysis_cohorts_delete_confirm", { tag: s.cohort.source_tag }),
        confirmLabel: $t("analysis_cohorts_delete"),
      };
    }
    if (s.kind === "retire-snapshot") {
      return {
        title: $t("analysis_snapshots_retire_confirm", { slug: s.snap.slug }),
        confirmLabel: $t("analysis_snapshots_retire"),
      };
    }
    if (s.kind === "rebuild-all-snapshots") {
      return {
        title: $t("analysis_snapshots_rebuild_all_confirm", { count: s.count }),
        confirmLabel: $t("analysis_snapshots_rebuild_all"),
      };
    }
    return {
      title: $t("analysis_snapshots_purge_confirm", { slug: s.snap.slug }),
      confirmLabel: $t("analysis_snapshots_purge"),
      confirmPhrase: s.snap.slug,
      phrasePrompt: $t("batch_delete_typed_confirm_label", { id: s.snap.slug }),
    };
  });

  const runConfirm = async () => {
    const s = confirmState;
    confirmState = null;
    if (!s) return;
    if (s.kind === "delete-cohort") await performDeleteCohort(s.cohort);
    else if (s.kind === "retire-snapshot") await performRetireSnapshot(s.cohort, s.snap);
    else if (s.kind === "purge-snapshot") await performPurgeSnapshot(s.cohort, s.snap);
    else if (s.kind === "rebuild-all-snapshots") await performRebuildAllSnapshots(s.cohort);
  };
  let busyCohortId = $state(null);
  let creating = $state(false);
  let draft = $state(emptyDraft());
  let existingTagNames = $state(new Set());
  let editingCohortId = $state(null);
  let editingSourceTag = $state("");
  let saving = $state(false);
  let backendStatus = $state({ backend_supported: true, unsupported_message: "" });

  // Past tag batches that the admin can promote at cohort-create time.
  let candidateBatches = $state([]);
  let candidateBatchesLoading = $state(false);
  let promoteBatchIds = $state(new Set());
  let candidateLookupTag = $state("");

  // Source-tag drift against a cohort's reference list, keyed by cohort id.
  // Only cohorts with a reference list are fetched, and only on load.
  let driftByCohortId = $state({});

  // Snapshots are loaded lazily per-cohort (expand toggled by admin).
  let snapshotsByCohortId = $state({});
  let snapshotsLoadingIds = $state(new Set());
  let expandedSnapshotCohortId = $state(null);
  let busySnapshotKey = $state("");
  // Snapshot table sort; defaults to newest-captured-first (API order).
  let snapshotSort = $state({ key: "captured", direction: "desc" });
  let editingLabelKey = $state("");
  let editingLabelValue = $state("");
  let submittingSnapshotCohortId = $state(null);
  // Which cohort's "Run new snapshot" split-button menu is open (id or null).
  let openRunMenuCohortId = $state(null);
  // Poll handle kept outside state so we can clear it without reactively
  // re-triggering. Rebuilds run server-side in a goroutine and report
  // materialization_done / materialization_total on the cohort row; while
  // any cohort is pending we poll a few times per second so the bar
  // animates and small/fast rebuilds don't slip past unobserved.
  let pollTimer = null;
  const POLL_INTERVAL_MS = 250;
  // A rebuild started here reports no work total until its first progress
  // write, so watch the cohorts we kicked off until they leave pending. The
  // tick budget bounds the wait in case that report never lands.
  const POLL_GRACE_TICKS = 120;
  let pollGraceIds = $state(new Set());
  let pollGraceTicks = $state(0);

  function emptyDraft() {
    return {
      source_tag: "",
      label: "",
      description: "",
      analysis_enabled: true,
      public_enabled: false,
      is_default: false,
      sort_order: 0,
      reference_list: "",
    };
  }

  const apiFetch = async (path, options) => apiCall(apiBase, path, options);

  const setNotice = (message, tone = "") => {
    noticeMessage = message;
    noticeTone = tone;
  };

  const clearNotice = () => {
    noticeMessage = "";
    noticeTone = "";
  };

  async function loadCohorts({ preserveNotice = false } = {}) {
    loading = true;
    loadError = "";
    try {
      const result = await apiFetch("/analysis/cohorts");
      cohorts = Array.isArray(result) ? result : [];
      if (!preserveNotice) clearNotice();
      loadDrift(cohorts);
    } catch (error) {
      loadError = error.message || $t("analysis_cohorts_load_error_generic");
    } finally {
      loading = false;
    }
  }

  async function loadBackendStatus() {
    try {
      const status = await apiFetch("/analysis/status");
      backendStatus = {
        backend_supported: status?.backend_supported !== false,
        unsupported_message: status?.unsupported_message || "",
      };
    } catch (_) {
      // Soft hint; don't block the cohort panel on this lookup.
    }
  }

  // Drift is derived server-side per cohort, so only rows that name a
  // reference list are asked for it.
  async function loadDrift(rows) {
    const next = {};
    await Promise.all(
      rows
        .filter((c) => c.reference_list)
        .map(async (c) => {
          try {
            const detail = await apiFetch(`/analysis/cohorts/${c.id}`);
            if (detail?.source_drift) next[c.id] = detail.source_drift;
          } catch (_) {
            // Drift is a soft hint; a failed read just leaves it unshown.
          }
        })
    );
    driftByCohortId = next;
  }

  const REFERENCE_LISTS = ["", "iana_tlds"];

  const referenceListLabel = (value) =>
    value === "iana_tlds"
      ? $t("analysis_cohorts_reference_list_iana_tlds")
      : $t("analysis_cohorts_reference_list_none");

  const driftCount = (drift, key) => (Array.isArray(drift?.[key]) ? drift[key].length : 0);

  const driftNames = (drift, key) => (Array.isArray(drift?.[key]) ? drift[key].join(", ") : "");

  async function loadExistingTags() {
    try {
      const tags = await apiFetch("/tags?limit=500");
      if (Array.isArray(tags)) {
        existingTagNames = new Set(tags.map((t) => String(t?.name || "")).filter(Boolean));
      }
    } catch (_) {
      // Tag lookup is a soft hint; ignore failures.
    }
  }

  const normalizedDraftTag = $derived(String(draft.source_tag || "").trim());
  const draftTagExists = $derived(
    normalizedDraftTag !== "" && existingTagNames.has(normalizedDraftTag)
  );

  async function loadCandidateBatches(tag) {
    if (!tag || editingCohortId) {
      candidateBatches = [];
      promoteBatchIds = new Set();
      candidateLookupTag = "";
      return;
    }
    candidateBatchesLoading = true;
    try {
      const data = await apiFetch(`/tags/${encodeURIComponent(tag)}/batches?limit=10`);
      const items = data?.items ?? [];
      candidateBatches = items;
      const next = new Set();
      for (const b of items) {
        if (!b.snapshot_intent) next.add(b.id);
      }
      promoteBatchIds = next;
      candidateLookupTag = tag;
    } catch (_) {
      candidateBatches = [];
      promoteBatchIds = new Set();
    } finally {
      candidateBatchesLoading = false;
    }
  }

  $effect(() => {
    if (editingCohortId) return;
    const tag = normalizedDraftTag;
    if (tag === candidateLookupTag) return;
    if (!tag) {
      candidateBatches = [];
      promoteBatchIds = new Set();
      candidateLookupTag = "";
      return;
    }
    if (!draftTagExists) {
      candidateBatches = [];
      promoteBatchIds = new Set();
      candidateLookupTag = tag;
      return;
    }
    loadCandidateBatches(tag);
  });

  function togglePromoteBatch(id) {
    const next = new Set(promoteBatchIds);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    promoteBatchIds = next;
  }

  function startEdit(cohort) {
    editingCohortId = cohort.id;
    editingSourceTag = cohort.source_tag;
    draft = {
      source_tag: cohort.source_tag,
      label: cohort.label || "",
      description: cohort.description || "",
      analysis_enabled: !!cohort.analysis_enabled,
      public_enabled: !!cohort.public_enabled,
      is_default: !!cohort.is_default,
      sort_order: cohort.sort_order || 0,
      reference_list: cohort.reference_list || "",
    };
    clearNotice();
    if (typeof document !== "undefined") {
      const target = document.getElementById("cohort-edit-form");
      if (target && typeof target.scrollIntoView === "function") {
        target.scrollIntoView({ behavior: "smooth", block: "start" });
      }
    }
  }

  function cancelEdit() {
    editingCohortId = null;
    editingSourceTag = "";
    draft = emptyDraft();
    clearNotice();
  }

  async function saveEdit() {
    if (editingCohortId == null) return;
    saving = true;
    try {
      await apiFetch(`/analysis/cohorts/${editingCohortId}`, {
        method: "PATCH",
        body: JSON.stringify({
          label: String(draft.label || "").trim(),
          description: String(draft.description || "").trim(),
          sort_order: Number(draft.sort_order) || 0,
          reference_list: String(draft.reference_list || ""),
        }),
      });
      await loadCohorts({ preserveNotice: true });
      const tag = editingSourceTag;
      editingCohortId = null;
      editingSourceTag = "";
      draft = emptyDraft();
      setNotice($t("analysis_cohorts_edit_saved", { tag }), "ok");
    } catch (error) {
      setNotice($t("analysis_cohorts_edit_error", { error: error.message || "" }), "warn");
    } finally {
      saving = false;
    }
  }

  const deleteCohort = (cohort) => { confirmState = { kind: "delete-cohort", cohort }; };

  async function performDeleteCohort(cohort) {
    busyCohortId = cohort.id;
    try {
      await apiFetch(`/analysis/cohorts/${cohort.id}`, { method: "DELETE" });
      if (editingCohortId === cohort.id) {
        editingCohortId = null;
        editingSourceTag = "";
        draft = emptyDraft();
      }
      await Promise.all([loadCohorts({ preserveNotice: true }), loadExistingTags()]);
      setNotice($t("analysis_cohorts_deleted", { tag: cohort.source_tag }), "ok");
    } catch (error) {
      setNotice($t("analysis_cohorts_delete_error", { error: error.message || "" }), "warn");
    } finally {
      busyCohortId = null;
    }
  }

  async function createCohort() {
    const tag = String(draft.source_tag || "").trim();
    if (!tag) {
      setNotice($t("analysis_cohorts_validation_source_tag"), "warn");
      return;
    }
    if (draft.public_enabled && !draft.analysis_enabled) {
      setNotice($t("analysis_cohorts_validation_public_requires_analysis"), "warn");
      return;
    }
    if (draft.is_default && (!draft.public_enabled || !draft.analysis_enabled)) {
      setNotice($t("analysis_cohorts_validation_default_requires_public"), "warn");
      return;
    }
    creating = true;
    try {
      const promoteIds = Array.from(promoteBatchIds);
      let promotedCount = 0;
      for (const id of promoteIds) {
        try {
          await apiFetch(`/batches/${encodeURIComponent(id)}`, {
            method: "PATCH",
            body: JSON.stringify({ snapshot_intent: true }),
          });
          promotedCount++;
        } catch (_) {
          // best-effort: cohort create still proceeds
        }
      }
      const created = await apiFetch("/analysis/cohorts", {
        method: "POST",
        body: JSON.stringify({
          source_type: "tag",
          source_tag: tag,
          label: String(draft.label || "").trim(),
          description: String(draft.description || "").trim(),
          analysis_enabled: !!draft.analysis_enabled,
          public_enabled: !!draft.public_enabled,
          is_default: !!draft.is_default,
          sort_order: Number(draft.sort_order) || 0,
          reference_list: String(draft.reference_list || ""),
        }),
      });
      // The server rebuilds a new cohort right away when analysis is on.
      if (draft.analysis_enabled) watchRebuild(created?.id);
      draft = emptyDraft();
      candidateBatches = [];
      promoteBatchIds = new Set();
      candidateLookupTag = "";
      await Promise.all([loadCohorts({ preserveNotice: true }), loadExistingTags()]);
      if (promotedCount > 0) {
        setNotice($t("analysis_cohorts_created_with_promotion", { tag, count: promotedCount }), "ok");
      } else {
        setNotice($t("analysis_cohorts_created", { tag }), "ok");
      }
    } catch (error) {
      setNotice($t("analysis_cohorts_create_error", { error: error.message || "" }), "warn");
    } finally {
      creating = false;
    }
  }

  async function patchCohort(cohort, patch, successKey) {
    busyCohortId = cohort.id;
    try {
      await apiFetch(`/analysis/cohorts/${cohort.id}`, {
        method: "PATCH",
        body: JSON.stringify(patch),
      });
      await loadCohorts({ preserveNotice: true });
      if (successKey) {
        setNotice($t(successKey, { tag: cohort.source_tag }), "ok");
      }
    } catch (error) {
      setNotice($t("analysis_cohorts_update_error", { error: error.message || "" }), "warn");
    } finally {
      busyCohortId = null;
    }
  }

  const toggleAnalysis = (cohort) => {
    const next = !cohort.analysis_enabled;
    const patch = { analysis_enabled: next };
    if (!next) {
      patch.public_enabled = false;
      patch.is_default = false;
    }
    return patchCohort(cohort, patch,
      next ? "analysis_cohorts_enabled" : "analysis_cohorts_disabled");
  };

  const togglePublic = (cohort) => {
    const next = !cohort.public_enabled;
    if (next && !cohort.analysis_enabled) {
      setNotice($t("analysis_cohorts_validation_public_requires_analysis"), "warn");
      return;
    }
    const patch = { public_enabled: next };
    if (!next) patch.is_default = false;
    return patchCohort(cohort, patch,
      next ? "analysis_cohorts_public_on" : "analysis_cohorts_public_off");
  };

  const setDefault = (cohort) => {
    if (!cohort.analysis_enabled || !cohort.public_enabled) {
      setNotice($t("analysis_cohorts_validation_default_requires_public"), "warn");
      return;
    }
    return patchCohort(cohort, { is_default: true }, "analysis_cohorts_default_set");
  };

  async function triggerAction(cohort, action, successKey) {
    busyCohortId = cohort.id;
    try {
      await apiFetch(`/analysis/cohorts/${cohort.id}/${action}`, { method: "POST", body: "{}" });
      await loadCohorts({ preserveNotice: true });
      setNotice($t(successKey, { tag: cohort.source_tag }), "ok");
    } catch (error) {
      setNotice($t("analysis_cohorts_action_error", { action, error: error.message || "" }), "warn");
    } finally {
      busyCohortId = null;
    }
  }

  const rebuildCohort = (cohort) => {
    watchRebuild(cohort.id);
    return triggerAction(cohort, "rebuild", "analysis_cohorts_rebuild_ok");
  };
  const clearCohort = (cohort) => triggerAction(cohort, "clear", "analysis_cohorts_clear_ok");

  const snapshotKey = (cohortId, slug) => `${cohortId}/${slug}`;

  // External batch deletions invalidate the cached snapshot list for every
  // cohort. Re-fetch the snapshot list for cohorts we have already loaded so
  // the UI reflects the new state without requiring the admin to collapse
  // and re-expand each row.
  let lastRefreshSignal = 0;
  $effect(() => {
    if (refreshSignal === lastRefreshSignal) return;
    lastRefreshSignal = refreshSignal;
    if (!cohorts || cohorts.length === 0) return;
    const loadedIds = Object.keys(snapshotsByCohortId).map((id) => Number(id));
    for (const id of loadedIds) {
      const cohort = cohorts.find((c) => c.id === id);
      if (cohort) loadSnapshots(cohort, { refresh: true });
    }
  });

  async function loadSnapshots(cohort, { refresh = false } = {}) {
    if (!cohort) return;
    if (!refresh && snapshotsByCohortId[cohort.id]) return;
    if (!refresh) {
      snapshotsLoadingIds.add(cohort.id);
      snapshotsLoadingIds = new Set(snapshotsLoadingIds);
    }
    try {
      const result = await apiFetch(`/analysis/cohorts/${cohort.id}/snapshots`);
      snapshotsByCohortId = { ...snapshotsByCohortId, [cohort.id]: Array.isArray(result) ? result : [] };
    } catch (_) {
      snapshotsByCohortId = { ...snapshotsByCohortId, [cohort.id]: [] };
    } finally {
      if (!refresh) {
        snapshotsLoadingIds.delete(cohort.id);
        snapshotsLoadingIds = new Set(snapshotsLoadingIds);
      }
    }
  }

  function toggleSnapshots(cohort) {
    if (expandedSnapshotCohortId === cohort.id) {
      expandedSnapshotCohortId = null;
      return;
    }
    expandedSnapshotCohortId = cohort.id;
    loadSnapshots(cohort);
  }

  async function patchSnapshot(cohort, snap, patch, successKey) {
    const key = snapshotKey(cohort.id, snap.slug);
    busySnapshotKey = key;
    try {
      await apiFetch(`/analysis/cohorts/${cohort.id}/snapshots/${encodeURIComponent(snap.slug)}`, {
        method: "POST",
        body: JSON.stringify(patch),
      });
      await loadSnapshots(cohort, { refresh: true });
      if (successKey) setNotice($t(successKey, { slug: snap.slug }), "ok");
    } catch (error) {
      setNotice($t("analysis_snapshots_action_error", { error: error.message || "" }), "warn");
    } finally {
      busySnapshotKey = "";
    }
  }

  const setSnapshotDefault = (cohort, snap) =>
    patchSnapshot(cohort, snap, { is_default: true }, "analysis_snapshots_default_set");

  const retireSnapshot = (cohort, snap) => { confirmState = { kind: "retire-snapshot", cohort, snap }; };

  const performRetireSnapshot = (cohort, snap) =>
    patchSnapshot(cohort, snap, { status: "retired", is_public: false }, "analysis_snapshots_retired");

  const restoreSnapshot = (cohort, snap) =>
    patchSnapshot(cohort, snap, { status: "captured", is_public: true }, "analysis_snapshots_restored");

  async function rebuildSnapshotAggregates(cohort, snap) {
    const key = snapshotKey(cohort.id, snap.slug);
    busySnapshotKey = key;
    try {
      // The rebuild runs server-side in the background; the POST returns 202
      // with the snapshot pending. Reload once so the progress bar appears,
      // then the poll loop tracks it to completion.
      await apiFetch(
        `/analysis/cohorts/${cohort.id}/snapshots/${encodeURIComponent(snap.slug)}/rematerialize`,
        { method: "POST" }
      );
      await loadSnapshots(cohort, { refresh: true });
      setNotice($t("analysis_snapshots_rebuild_started", { slug: snap.slug }), "ok");
    } catch (error) {
      setNotice($t("analysis_snapshots_action_error", { error: error.message || "" }), "warn");
    } finally {
      busySnapshotKey = "";
    }
  }

  const rebuildAllSnapshots = (cohort, count) => {
    confirmState = { kind: "rebuild-all-snapshots", cohort, count };
  };

  // The server walks the snapshots one at a time; the poll loop tracks them.
  async function performRebuildAllSnapshots(cohort) {
    busyCohortId = cohort.id;
    try {
      const result = await apiFetch(`/analysis/cohorts/${cohort.id}/snapshots/rematerialize`, {
        method: "POST"
      });
      await loadSnapshots(cohort, { refresh: true });
      setNotice($t("analysis_snapshots_rebuild_all_started", { count: result?.queued ?? 0 }), "ok");
    } catch (error) {
      setNotice($t("analysis_snapshots_action_error", { error: error.message || "" }), "warn");
    } finally {
      busyCohortId = null;
    }
  }

  const purgeSnapshot = (cohort, snap) => { confirmState = { kind: "purge-snapshot", cohort, snap }; };

  async function performPurgeSnapshot(cohort, snap) {
    const key = snapshotKey(cohort.id, snap.slug);
    busySnapshotKey = key;
    try {
      await apiFetch(
        `/analysis/cohorts/${cohort.id}/snapshots/${encodeURIComponent(snap.slug)}?purge=true`,
        { method: "DELETE" }
      );
      await loadSnapshots(cohort, { refresh: true });
      setNotice($t("analysis_snapshots_purged", { slug: snap.slug }), "ok");
    } catch (error) {
      setNotice($t("analysis_snapshots_action_error", { error: error.message || "" }), "warn");
    } finally {
      busySnapshotKey = "";
    }
  }

  function startLabelEdit(cohort, snap) {
    editingLabelKey = snapshotKey(cohort.id, snap.slug);
    editingLabelValue = snap.label || "";
  }

  function cancelLabelEdit() {
    editingLabelKey = "";
    editingLabelValue = "";
  }

  async function saveLabelEdit(cohort, snap) {
    await patchSnapshot(cohort, snap, { label: editingLabelValue.trim() }, "analysis_snapshots_label_saved");
    cancelLabelEdit();
  }

  // "Run new snapshot" → POST /jobs/batch with from_tag + snapshot_intent.
  // With promoteDefault, the captured snapshot is pinned as cohort default.
  async function runSnapshot(cohort, { promoteDefault = false } = {}) {
    openRunMenuCohortId = null;
    submittingSnapshotCohortId = cohort.id;
    try {
      const payload = { from_tag: cohort.source_tag, snapshot_intent: true };
      if (promoteDefault) payload.promote_snapshot_default = true;
      const response = await apiFetch("/jobs/batch", {
        method: "POST",
        body: JSON.stringify(payload),
      });
      const noticeKey = promoteDefault
        ? "analysis_cohorts_snapshot_submitted_default"
        : "analysis_cohorts_snapshot_submitted";
      setNotice(
        $t(noticeKey, { id: response.batch_id, tag: cohort.source_tag }),
        "ok"
      );
      expandedSnapshotCohortId = cohort.id;
      await loadSnapshots(cohort, { refresh: true });
    } catch (error) {
      setNotice(
        $t("analysis_cohorts_snapshot_submit_error", { error: error.message || "" }),
        "warn"
      );
    } finally {
      submittingSnapshotCohortId = null;
    }
  }

  function toggleRunMenu(cohort) {
    openRunMenuCohortId = openRunMenuCohortId === cohort.id ? null : cohort.id;
  }

  // Place the run-options menu and close it on Escape or any click/focus
  // outside its split button. The table wrapper scrolls horizontally, which
  // makes it clip vertically too, so the menu is positioned against the
  // section around the wrapper: an absolutely positioned box whose containing
  // block sits outside a scroll box is neither clipped nor scrolled by it.
  // Coordinates come from measured rects because the CSP forbids inline
  // styles. Listeners are scoped to the mounted menu and removed on destroy;
  // the menu only renders while open.
  function runMenuPopup(node) {
    const scope = node.closest(".run-split") || node;
    const origin = node.closest(".cohort-section");
    const gap = 4;
    const place = () => {
      if (!origin) return;
      const base = origin.getBoundingClientRect();
      const anchor = scope.getBoundingClientRect();
      const menu = node.getBoundingClientRect();
      const viewport = window.innerHeight || document.documentElement.clientHeight || 0;
      const fitsBelow = anchor.bottom + gap + menu.height <= viewport;
      const fitsAbove = anchor.top - gap - menu.height >= 0;
      const top = !fitsBelow && fitsAbove
        ? anchor.top - gap - menu.height
        : anchor.bottom + gap;
      node.style.top = `${top - base.top}px`;
      node.style.left = `${anchor.right - menu.width - base.left}px`;
    };
    const onKey = (e) => { if (e.key === "Escape") openRunMenuCohortId = null; };
    const onOutside = (e) => { if (!scope.contains(e.target)) openRunMenuCohortId = null; };
    place();
    window.addEventListener("keydown", onKey);
    window.addEventListener("click", onOutside, true);
    window.addEventListener("focusin", onOutside, true);
    window.addEventListener("scroll", place, true);
    window.addEventListener("resize", place);
    return {
      destroy() {
        window.removeEventListener("keydown", onKey);
        window.removeEventListener("click", onOutside, true);
        window.removeEventListener("focusin", onOutside, true);
        window.removeEventListener("scroll", place, true);
        window.removeEventListener("resize", place);
      },
    };
  }

  function snapshotStatusTone(status) {
    switch (status) {
      case "captured": return "ok";
      case "pending": return "neutral";
      case "retired": return "neutral";
      case "failed_mixed_profiles": return "warn";
      default: return "neutral";
    }
  }

  function statusBadgeTone(status) {
    switch (status) {
      case "ready":
        return "ok";
      case "failed":
        return "warn";
      case "pending":
      default:
        return "neutral";
    }
  }

  function isReadyWithoutMaterialization(cohort) {
    return cohort?.materialization_status === "ready" && !cohort?.last_materialized_at;
  }

  function materializationBadgeTone(cohort) {
    if (isReadyWithoutMaterialization(cohort)) return "neutral";
    return statusBadgeTone(cohort?.materialization_status);
  }

  function materializationStatusLabel(cohort) {
    if (isReadyWithoutMaterialization(cohort)) {
      return $t("analysis_cohorts_never_materialized");
    }
    return cohort?.materialization_status || "pending";
  }

  function formatTimestamp(value) {
    if (!value) return "";
    const parsed = new Date(value);
    if (Number.isNaN(parsed.getTime()) || parsed.getUTCFullYear() <= 1) return "";
    return formatTimestampLocal(value);
  }

  function formatSnapshotCapturedAt(snap) {
    if (snap?.status === "pending") return $t("analysis_snapshots_not_captured");
    return formatTimestamp(snap?.captured_at) || $t("analysis_snapshots_not_captured");
  }

  function snapshotDisplayName(snap) {
    const label = String(snap?.label || "").trim();
    if (label) return label;
    const slug = String(snap?.slug || "");
    const match = slug.match(/^(\d{4}-\d{2}-\d{2})-/);
    return match?.[1] || $t("analysis_snapshots_unlabeled");
  }

  function shortID(value) {
    const id = String(value || "");
    if (id.length <= 28) return id;
    return `${id.slice(0, 14)}...${id.slice(-8)}`;
  }

  // "pending" also marks a cohort that was never built or was cleared, which
  // is a resting state, so a published work total is what tells a rebuild in
  // flight from an idle row. Ready + failed rows are stable too.
  const isRebuilding = (row) =>
    row?.materialization_status === "pending" && Number(row?.materialization_total) > 0;

  const anyPending = $derived(cohorts.some(isRebuilding));

  function progressPercent(cohort) {
    const total = Number(cohort?.materialization_total) || 0;
    const done = Number(cohort?.materialization_done) || 0;
    if (total <= 0) return 0;
    if (done >= total) return 100;
    return Math.floor((done / total) * 100);
  }

  // Same math for a snapshot's rematerialize progress.
  const snapshotProgressPercent = (snap) => progressPercent(snap);

  // Any loaded snapshot mid-rebuild -> keep polling that cohort's list.
  const anySnapshotMaterializing = $derived(
    Object.values(snapshotsByCohortId).some((list) =>
      Array.isArray(list) && list.some(isRebuilding),
    ),
  );

  const SNAPSHOT_COMPARATORS = {
    snapshot: (a, b) => compareText(snapshotDisplayName(a), snapshotDisplayName(b)),
    captured: (a, b) => compareTimestamp(a.captured_at, b.captured_at),
    profile: (a, b) => compareText(a.profile_name, b.profile_name),
    counts: (a, b) => compareNumber(a.domain_count, b.domain_count),
    status: (a, b) => compareText(a.status, b.status),
  };
  const snapshotTieBreaker = (a, b) => compareNumber(a.id, b.id);
  const sortSnapshots = (list) => sortItems(list, snapshotSort, SNAPSHOT_COMPARATORS, snapshotTieBreaker);
  const setSnapshotSort = (key, defaultDir = "asc") => {
    snapshotSort = nextTableSort(snapshotSort, key, defaultDir);
  };

  // Watch a cohort whose rebuild this client just started.
  function watchRebuild(cohortId) {
    if (cohortId == null || !backendStatus.backend_supported) return;
    pollGraceIds = new Set(pollGraceIds).add(cohortId);
    pollGraceTicks = POLL_GRACE_TICKS;
  }

  // Drop the cohorts whose rebuild has landed; other rows resting in pending
  // are none of this window's business.
  function releaseFinishedRebuilds() {
    if (pollGraceIds.size === 0) return;
    const running = new Set();
    for (const id of pollGraceIds) {
      const row = cohorts.find((c) => c.id === id);
      if (row?.materialization_status === "pending") running.add(id);
    }
    if (running.size !== pollGraceIds.size) pollGraceIds = running;
    if (running.size === 0) pollGraceTicks = 0;
  }

  async function pollCohortsQuietly() {
    if (pollGraceTicks > 0) {
      pollGraceTicks -= 1;
      if (pollGraceTicks === 0) pollGraceIds = new Set();
    }
    try {
      const result = await apiFetch("/analysis/cohorts");
      if (Array.isArray(result)) cohorts = result;
    } catch (_) {
      // Silent: a transient fetch error shouldn't overwrite the visible
      // notice area while a rebuild is running.
    }
    releaseFinishedRebuilds();
    // Refresh the expanded cohort's snapshot list while either the cohort is
    // rebuilding or one of its snapshots is being rematerialized, so the
    // per-snapshot progress bar and counts advance without a manual reload.
    if (expandedSnapshotCohortId != null) {
      const cohort = cohorts.find((c) => c.id === expandedSnapshotCohortId);
      const snaps = snapshotsByCohortId[expandedSnapshotCohortId] ?? [];
      if (isRebuilding(cohort) || snaps.some(isRebuilding)) {
        loadSnapshots(cohort, { refresh: true });
      }
    }
  }

  $effect(() => {
    // Poll while a cohort rebuild or a snapshot rematerialize is in flight;
    // stop once nothing is running so a quiet page makes no requests at all.
    // A backend that cannot materialize never leaves a cohort pending, so
    // there is nothing to watch there at all.
    const active = backendStatus.backend_supported
      && (anyPending || anySnapshotMaterializing || pollGraceIds.size > 0);
    if (active && pollTimer == null) {
      pollTimer = setInterval(pollCohortsQuietly, POLL_INTERVAL_MS);
    } else if (!active && pollTimer != null) {
      clearInterval(pollTimer);
      pollTimer = null;
    }
  });

  onMount(() => {
    loadCohorts();
    loadExistingTags();
    loadBackendStatus();
  });

  onDestroy(() => {
    if (pollTimer != null) {
      clearInterval(pollTimer);
      pollTimer = null;
    }
  });

  // Apply a dynamic width via DOM API instead of inline style="..." or style:width,
  // which the admin-UI CSP (style-src 'self') blocks.
  const applyWidth = (node, value) => {
    node.style.width = value;
    return { update(v) { node.style.width = v; } };
  };
</script>

<h2>{$t("analysis_cohorts_heading")}</h2>
<div class="small subtitle">{$t("analysis_cohorts_subtitle")}</div>

{#if !backendStatus.backend_supported}
  <div class="inline-notice inline-notice-warn" role="alert">
    <strong>{$t("analysis_cohorts_backend_warning_title")}</strong>
    <div class="small notice-detail">
      {backendStatus.unsupported_message || $t("analysis_cohorts_backend_warning_body")}
    </div>
  </div>
{/if}

<InlineNotice message={noticeMessage} tone={noticeTone} />

{#if loading}
  <p class="small">{$t("analysis_cohorts_loading")}</p>
{:else if loadError}
  <p class="error">{$t("analysis_cohorts_load_error", { error: loadError })}</p>
{:else}
  <section class="cohort-section">
    <h3>{$t("analysis_cohorts_existing_heading")}</h3>
    {#if cohorts.length === 0}
      <p class="small">{$t("analysis_cohorts_empty")}</p>
    {:else}
      <div class="cohort-table-wrap">
        <table class="data-table cohort-table">
          <thead>
            <tr>
              <th scope="col">{$t("analysis_cohorts_col_cohort")}</th>
              <th scope="col" class="col-center">{$t("analysis_cohorts_col_analysis")}</th>
              <th scope="col" class="col-center">{$t("analysis_cohorts_col_public")}</th>
              <th scope="col" class="col-center">{$t("analysis_cohorts_col_default")}</th>
              <th scope="col">{$t("analysis_cohorts_col_materialization")}</th>
              <th scope="col" class="col-right">{$t("analysis_cohorts_col_actions")}</th>
            </tr>
          </thead>
          <tbody>
            {#each cohorts as cohort (cohort.id)}
              {@const busy = busyCohortId === cohort.id}
              {@const tone = materializationBadgeTone(cohort)}
              {@const runDisabled = busy || !cohort.analysis_enabled || submittingSnapshotCohortId === cohort.id}
              {@const drift = driftByCohortId[cohort.id]}
              <tr class:row-editing={editingCohortId === cohort.id}>
                <th scope="row" class="cohort-cell">
                  <span class="cohort-tag">{cohort.source_tag}</span>
                  {#if cohort.label && cohort.label !== cohort.source_tag}
                    <span class="cohort-label">{cohort.label}</span>
                  {/if}
                  {#if drift}
                    {@const missing = driftCount(drift, "missing")}
                    {@const extra = driftCount(drift, "extra")}
                    {#if missing === 0 && extra === 0}
                      <span class="cohort-drift muted">{$t("analysis_cohorts_drift_in_sync")}</span>
                    {:else}
                      <details class="cohort-drift">
                        <summary>{$t("analysis_cohorts_drift")}: +{missing} / -{extra}</summary>
                        <div class="drift-detail">
                          <div class="small muted">
                            {$t("analysis_cohorts_drift_checked", {
                              time: formatTimestamp(drift.checked_at),
                              version: drift.list_version || "?",
                            })}
                          </div>
                          {#if missing > 0}
                            <div class="drift-group">
                              <span class="field-label">{$t("analysis_cohorts_drift_missing")}</span>
                              <span class="mono drift-names">{driftNames(drift, "missing")}</span>
                            </div>
                          {/if}
                          {#if extra > 0}
                            <div class="drift-group">
                              <span class="field-label">{$t("analysis_cohorts_drift_extra")}</span>
                              <span class="mono drift-names">{driftNames(drift, "extra")}</span>
                            </div>
                          {/if}
                        </div>
                      </details>
                    {/if}
                  {/if}
                </th>
                <td class="col-center">
                  <button
                    type="button"
                    class="toggle-pill"
                    class:toggle-on={cohort.analysis_enabled}
                    class:toggle-off={!cohort.analysis_enabled}
                    disabled={busy}
                    aria-pressed={cohort.analysis_enabled}
                    onclick={() => toggleAnalysis(cohort)}
                  >
                    {cohort.analysis_enabled ? $t("analysis_cohorts_toggle_on") : $t("analysis_cohorts_toggle_off")}
                  </button>
                </td>
                <td class="col-center">
                  <button
                    type="button"
                    class="toggle-pill"
                    class:toggle-on={cohort.public_enabled}
                    class:toggle-off={!cohort.public_enabled}
                    disabled={busy || !cohort.analysis_enabled}
                    aria-pressed={cohort.public_enabled}
                    onclick={() => togglePublic(cohort)}
                  >
                    {cohort.public_enabled ? $t("analysis_cohorts_toggle_on") : $t("analysis_cohorts_toggle_off")}
                  </button>
                </td>
                <td class="col-center">
                  {#if cohort.is_default}
                    <span class="badge badge-default" title={$t("analysis_cohorts_default_cohort_tooltip")}>{$t("analysis_cohorts_default_active")}</span>
                  {:else}
                    <button
                      type="button"
                      class="link-action"
                      disabled={busy || !cohort.analysis_enabled || !cohort.public_enabled}
                      onclick={() => setDefault(cohort)}
                    >
                      {$t("analysis_cohorts_default_set_action")}
                    </button>
                  {/if}
                </td>
                <td class="materialization-cell">
                  <div class="materialization-main">
                    <span class={`badge badge-status badge-status-${tone}`}>
                      {materializationStatusLabel(cohort)}
                    </span>
                    {#if cohort.materialization_status === "pending" && cohort.materialization_total > 0}
                      <span class="materialization-when">
                        {cohort.materialization_done || 0} / {cohort.materialization_total} ({progressPercent(cohort)}%)
                      </span>
                    {:else if cohort.last_materialized_at}
                      <time class="materialization-when" datetime={cohort.last_materialized_at}>
                        {formatTimestamp(cohort.last_materialized_at)}
                      </time>
                    {:else if cohort.materialization_status !== "ready"}
                      <span class="materialization-when muted">{$t("analysis_cohorts_never_materialized")}</span>
                    {/if}
                  </div>
                  {#if cohort.materialization_status === "pending" && cohort.materialization_total > 0}
                    <div class="materialization-progress" role="progressbar"
                         aria-valuenow={progressPercent(cohort)}
                         aria-valuemin="0" aria-valuemax="100">
                      <div class="materialization-progress-fill" use:applyWidth={`${progressPercent(cohort)}%`}></div>
                    </div>
                  {/if}
                  {#if cohort.last_materialization_error}
                    <span class="materialization-error" title={cohort.last_materialization_error}>
                      {cohort.last_materialization_error}
                    </span>
                  {/if}
                </td>
                <td class="col-right">
                  <div class="row-actions">
                    <div class="run-split">
                      <button type="button" class="row-action row-action-primary run-split-main"
                              disabled={runDisabled}
                              title={$t("analysis_cohorts_run_snapshot_title")}
                              onclick={() => runSnapshot(cohort)}>
                        {$t("analysis_cohorts_run_snapshot")}
                      </button>
                      <button type="button" class="row-action row-action-primary run-split-caret"
                              disabled={runDisabled}
                              aria-haspopup="menu"
                              aria-expanded={openRunMenuCohortId === cohort.id}
                              aria-label={$t("analysis_cohorts_run_snapshot_menu_toggle")}
                              onclick={() => toggleRunMenu(cohort)}>
                        <span aria-hidden="true">&#9662;</span>
                      </button>
                      {#if openRunMenuCohortId === cohort.id}
                        <div class="run-menu" role="menu" use:runMenuPopup>
                          <button type="button" class="run-menu-item" role="menuitem"
                                  onclick={() => runSnapshot(cohort, { promoteDefault: true })}>
                            {$t("analysis_cohorts_run_snapshot_default")}
                          </button>
                        </div>
                      {/if}
                    </div>
                    <button type="button" class="row-action" disabled={busy} onclick={() => toggleSnapshots(cohort)}>
                      {$t("analysis_snapshots_heading")}
                    </button>
                    <button type="button" class="row-action" disabled={busy || editingCohortId === cohort.id} onclick={() => startEdit(cohort)}>
                      {$t("analysis_cohorts_edit")}
                    </button>
                    <button type="button" class="row-action" disabled={busy || !cohort.analysis_enabled} onclick={() => rebuildCohort(cohort)}>
                      {$t("analysis_cohorts_rebuild")}
                    </button>
                    <button type="button" class="row-action" disabled={busy} onclick={() => clearCohort(cohort)}>
                      {$t("analysis_cohorts_clear")}
                    </button>
                    <button type="button" class="row-action row-action-danger" disabled={busy} onclick={() => deleteCohort(cohort)}>
                      {$t("analysis_cohorts_delete")}
                    </button>
                  </div>
                </td>
              </tr>
              {#if expandedSnapshotCohortId === cohort.id}
                {@const snaps = snapshotsByCohortId[cohort.id] ?? []}
                {@const sortedSnaps = sortSnapshots(snaps)}
                {@const mixed = snaps.some((s) => s.status === "failed_mixed_profiles")}
                <tr class="snapshot-subrow">
                  <td colspan="6">
                    {#if snapshotsLoadingIds.has(cohort.id) && snaps.length === 0}
                      <p class="small">{$t("analysis_cohorts_loading")}</p>
                    {:else if snaps.length === 0}
                      <p class="small">{$t("analysis_snapshots_empty")}</p>
                    {:else}
                      {#if mixed}
                        <div class="inline-notice inline-notice-warn" role="alert">
                          {$t("analysis_snapshots_mixed_profile_banner")}
                        </div>
                      {/if}
                      {@const rebuildable = snaps.filter((s) => s.source_runs_available !== false)}
                      <div class="snapshot-toolbar">
                        <button type="button" class="row-action"
                                title={$t("analysis_snapshots_rebuild_all_title")}
                                disabled={busyCohortId === cohort.id || rebuildable.length === 0
                                  || snaps.some((s) => s.materialization_status === "pending")}
                                onclick={() => rebuildAllSnapshots(cohort, rebuildable.length)}>
                          {$t("analysis_snapshots_rebuild_all")}
                        </button>
                      </div>
                      <table class="data-table snapshot-table">
                        <thead>
                          <tr>
                            <th class="sortable-column" aria-sort={tableSortAria(snapshotSort, "snapshot")}><button class="table-sort-button" type="button" onclick={() => setSnapshotSort("snapshot")}><span>{$t("analysis_snapshots_col_snapshot")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(snapshotSort, "snapshot")}</span></button></th>
                            <th class="sortable-column" aria-sort={tableSortAria(snapshotSort, "captured")}><button class="table-sort-button" type="button" onclick={() => setSnapshotSort("captured", "desc")}><span>{$t("analysis_snapshots_col_captured_at")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(snapshotSort, "captured")}</span></button></th>
                            <th class="sortable-column" aria-sort={tableSortAria(snapshotSort, "profile")}><button class="table-sort-button" type="button" onclick={() => setSnapshotSort("profile")}><span>{$t("analysis_snapshots_col_profile")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(snapshotSort, "profile")}</span></button></th>
                            <th class="sortable-column" aria-sort={tableSortAria(snapshotSort, "counts")}><button class="table-sort-button" type="button" onclick={() => setSnapshotSort("counts", "desc")}><span>{$t("analysis_snapshots_col_counts")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(snapshotSort, "counts")}</span></button></th>
                            <th>{$t("analysis_snapshots_col_source_batch")}</th>
                            <th class="sortable-column" aria-sort={tableSortAria(snapshotSort, "status")}><button class="table-sort-button" type="button" onclick={() => setSnapshotSort("status")}><span>{$t("analysis_snapshots_col_status")}</span><span class="sort-indicator" aria-hidden="true">{tableSortIndicator(snapshotSort, "status")}</span></button></th>
                            <th class="col-right">{$t("analysis_snapshots_col_actions")}</th>
                          </tr>
                        </thead>
                        <tbody>
                          {#each sortedSnaps as snap (snap.id)}
                            {@const snapKey = snapshotKey(cohort.id, snap.slug)}
                            {@const snapBusy = busySnapshotKey === snapKey}
                            {@const snapRebuilding = snap.materialization_status === "pending"}
                            {@const editing = editingLabelKey === snapKey}
                            <tr>
                              <td>
                                {#if editing}
                                  <input type="text" bind:value={editingLabelValue} />
                                {:else}
                                  <div class="snapshot-name">{snapshotDisplayName(snap)}</div>
                                  <div class="snapshot-id mono" title={snap.slug}>{snap.slug}</div>
                                {/if}
                              </td>
                              <td>{formatSnapshotCapturedAt(snap)}</td>
                              <td>{snap.profile_name || ""}</td>
                              <td>{snap.run_count} / {snap.domain_count}</td>
                              <td>
                                {#if snap.batch_id}
                                  <a
                                    class="mono source-batch-id"
                                    href={href("batches", { batchId: snap.batch_id })}
                                    title={snap.batch_id}
                                    onclick={(e) => { e.preventDefault(); navigate("batches", { batchId: snap.batch_id }); }}
                                  >{shortID(snap.batch_id)}</a>
                                {:else}
                                  <span class="muted">-</span>
                                {/if}
                              </td>
                              <td>
                                <span class={`badge badge-status badge-status-${snapshotStatusTone(snap.status)}`}>
                                  {snap.status}
                                </span>
                                {#if snap.is_default}
                                  <span class="badge badge-default" title={$t("analysis_cohorts_default_snapshot_tooltip")}>{$t("analysis_cohorts_default_active")}</span>
                                {/if}
                                {#if snapRebuilding}
                                  <div class="materialization-progress" role="progressbar"
                                       aria-valuenow={snapshotProgressPercent(snap)}
                                       aria-valuemin="0" aria-valuemax="100"
                                       aria-label={$t("analysis_snapshots_rebuilding")}>
                                    <div class="materialization-progress-fill" use:applyWidth={`${snapshotProgressPercent(snap)}%`}></div>
                                  </div>
                                  <span class="snapshot-rebuilding-label">
                                    {$t("analysis_snapshots_rebuilding")} {snap.materialization_done ?? 0}/{snap.materialization_total ?? 0}
                                  </span>
                                {:else if snap.materialization_status === "failed"}
                                  <span class="materialization-error" title={snap.last_materialization_error}>
                                    {$t("analysis_snapshots_rebuild_failed")}
                                  </span>
                                {/if}
                              </td>
                              <td class="col-right">
                                <div class="row-actions">
                                  {#if editing}
                                    <button type="button" class="row-action" disabled={snapBusy} onclick={() => saveLabelEdit(cohort, snap)}>
                                      {$t("analysis_snapshots_save_label")}
                                    </button>
                                    <button type="button" class="row-action" disabled={snapBusy} onclick={cancelLabelEdit}>
                                      {$t("analysis_cohorts_cancel")}
                                    </button>
                                  {:else}
                                    <button type="button" class="row-action" disabled={snapBusy} onclick={() => startLabelEdit(cohort, snap)}>
                                      {$t("analysis_snapshots_edit_label")}
                                    </button>
                                    {#if snap.status === "captured" && !snap.is_default}
                                      <button type="button" class="row-action" disabled={snapBusy} onclick={() => setSnapshotDefault(cohort, snap)}>
                                        {$t("analysis_snapshots_set_default")}
                                      </button>
                                    {/if}
                                    {#if snap.status === "captured"}
                                      <button
                                        type="button"
                                        class="row-action"
                                        title={snap.source_runs_available === false
                                          ? $t("analysis_snapshots_rebuild_unavailable_title")
                                          : $t("analysis_snapshots_rebuild_title")}
                                        disabled={snapBusy || snapRebuilding || snap.source_runs_available === false}
                                        onclick={() => rebuildSnapshotAggregates(cohort, snap)}
                                      >
                                        {$t("analysis_snapshots_rebuild")}
                                      </button>
                                    {/if}
                                    {#if snap.status === "retired"}
                                      <button type="button" class="row-action" disabled={snapBusy} onclick={() => restoreSnapshot(cohort, snap)}>
                                        {$t("analysis_snapshots_restore")}
                                      </button>
                                    {:else if snap.status !== "pending"}
                                      <button type="button" class="row-action" disabled={snapBusy} onclick={() => retireSnapshot(cohort, snap)}>
                                        {$t("analysis_snapshots_retire")}
                                      </button>
                                    {/if}
                                    <button
                                      type="button"
                                      class="row-action row-action-danger"
                                      title={$t("analysis_snapshots_purge_title")}
                                      disabled={snapBusy}
                                      onclick={() => purgeSnapshot(cohort, snap)}
                                    >
                                      {$t("analysis_snapshots_purge")}
                                    </button>
                                    {#if onDeleteBatch && snap.batch_id}
                                      <button
                                        type="button"
                                        class="row-action row-action-danger"
                                        title={$t("analysis_snapshots_delete_source_batch_title", { id: snap.batch_id })}
                                        disabled={snapBusy}
                                        onclick={() => onDeleteBatch(snap.batch_id)}
                                      >
                                        {$t("analysis_snapshots_delete_source_batch")}
                                      </button>
                                    {/if}
                                  {/if}
                                </div>
                              </td>
                            </tr>
                          {/each}
                        </tbody>
                      </table>
                    {/if}
                  </td>
                </tr>
              {/if}
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </section>

  <section class="cohort-section cohort-create" id="cohort-edit-form">
    <h3>
      {editingCohortId
        ? $t("analysis_cohorts_edit_heading", { tag: editingSourceTag })
        : $t("analysis_cohorts_create_heading")}
    </h3>
    <p class="small cohort-create-hint">
      {editingCohortId
        ? $t("analysis_cohorts_edit_hint")
        : $t("analysis_cohorts_create_hint")}
    </p>
    <div class="cohort-form-grid">
      <label class="field">
        <span class="field-label">{$t("analysis_cohorts_field_source_tag")}</span>
        <input
          type="text"
          bind:value={draft.source_tag}
          placeholder="tld"
          disabled={editingCohortId != null}
        />
        {#if !editingCohortId && normalizedDraftTag}
          <span class={`tag-match-indicator ${draftTagExists ? "tag-match-existing" : "tag-match-new"}`}>
            {draftTagExists
              ? $t("analysis_cohorts_tag_match_existing")
              : $t("analysis_cohorts_tag_match_new")}
          </span>
        {/if}
      </label>
      <label class="field">
        <span class="field-label">{$t("analysis_cohorts_field_label")}</span>
        <input type="text" bind:value={draft.label} placeholder="TLD" />
      </label>
      <label class="field">
        <span class="field-label">{$t("analysis_cohorts_field_sort_order")}</span>
        <input type="number" bind:value={draft.sort_order} min="0" />
      </label>
      <label class="field">
        <span class="field-label">{$t("analysis_cohorts_field_reference_list")}</span>
        <select bind:value={draft.reference_list}>
          {#each REFERENCE_LISTS as value (value)}
            <option {value}>{referenceListLabel(value)}</option>
          {/each}
        </select>
      </label>
      <label class="field field-full">
        <span class="field-label">{$t("analysis_cohorts_field_description")}</span>
        <input type="text" bind:value={draft.description} />
      </label>
      {#if !editingCohortId}
        <fieldset class="cohort-flags field-full">
          <legend class="field-label">{$t("analysis_cohorts_col_status")}</legend>
          <label class="check-field">
            <input type="checkbox" bind:checked={draft.analysis_enabled} />
            <span>{$t("analysis_cohorts_field_analysis_enabled")}</span>
          </label>
          <label class="check-field">
            <input type="checkbox" bind:checked={draft.public_enabled} />
            <span>{$t("analysis_cohorts_field_public_enabled")}</span>
          </label>
          <label class="check-field">
            <input type="checkbox" bind:checked={draft.is_default} />
            <span>{$t("analysis_cohorts_field_is_default")}</span>
          </label>
        </fieldset>
        {#if candidateBatches.length > 0}
          <fieldset class="cohort-promote field-full">
            <legend class="field-label">{$t("analysis_cohorts_promote_legend")}</legend>
            <p class="small cohort-create-hint">{$t("analysis_cohorts_promote_hint")}</p>
            {#each candidateBatches as b (b.id)}
              <label class="check-field">
                <input
                  type="checkbox"
                  checked={promoteBatchIds.has(b.id)}
                  onchange={() => togglePromoteBatch(b.id)}
                />
                <span class="mono">{b.id}</span>
                <span class="muted small">
                  {b.created_at ? formatTimestampLocal(b.created_at) : ""}
                  {b.snapshot_intent ? `(${$t("batch_snapshot_intent_pill")})` : ""}
                </span>
              </label>
            {/each}
          </fieldset>
        {/if}
      {/if}
    </div>
    <div class="cohort-form-actions">
      {#if editingCohortId}
        <button type="button" class="ghost" disabled={saving} onclick={cancelEdit}>
          {$t("analysis_cohorts_cancel")}
        </button>
        <button type="button" disabled={saving} onclick={saveEdit}>
          {saving ? $t("analysis_cohorts_saving") : $t("analysis_cohorts_save")}
        </button>
      {:else}
        <button type="button" disabled={creating} onclick={createCohort}>
          {creating ? $t("analysis_cohorts_creating") : $t("analysis_cohorts_create_button")}
        </button>
      {/if}
    </div>
  </section>
{/if}

<ConfirmDialog
  open={confirmState !== null}
  title={confirmProps?.title ?? ""}
  confirmLabel={confirmProps?.confirmLabel ?? ""}
  confirmPhrase={confirmProps?.confirmPhrase ?? ""}
  phrasePrompt={confirmProps?.phrasePrompt ?? ""}
  onConfirm={runConfirm}
  onCancel={() => (confirmState = null)}
/>

<style>
  .subtitle { margin-bottom: 12px; }
  .notice-detail { margin-top: 4px; }

  /* Positioning origin for the run-options menu, kept outside the wrapper's
     overflow so the menu can overlay the page instead of scrolling it. */
  .cohort-section {
    position: relative;
  }

  .cohort-section + .cohort-section {
    margin-top: 28px;
  }

  .cohort-section h3 {
    margin: 0 0 10px;
  }

  .cohort-create-hint {
    margin: 0 0 14px;
    color: var(--ink-2);
  }

  .cohort-table-wrap {
    overflow-x: auto;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--surface);
  }

  .cohort-table {
    font-size: var(--text-sm);
  }

  .cohort-table td,
  .cohort-table th[scope="row"] {
    font-family: var(--sans, inherit);
    vertical-align: middle;
  }

  .cohort-table tbody tr:last-child th[scope="row"] {
    border-bottom: none;
  }

  .cohort-tag {
    display: block;
    font-family: var(--mono);
    font-weight: 600;
    color: var(--ink);
  }

  .cohort-label {
    display: block;
    margin-top: 2px;
    font-size: var(--text-xs);
    color: var(--ink-2);
  }

  .cohort-drift {
    display: block;
    margin-top: 4px;
    font-size: var(--text-xs);
    font-family: var(--sans, inherit);
    font-weight: 400;
  }

  .cohort-drift summary {
    cursor: pointer;
    color: var(--ink-2);
  }

  .drift-detail {
    margin-top: 4px;
    max-width: 320px;
  }

  .drift-group {
    margin-top: 4px;
  }

  .drift-names {
    display: block;
    max-height: 8em;
    overflow-y: auto;
    overflow-wrap: anywhere;
    color: var(--ink-2);
  }

  .row-editing {
    background: rgba(3, 105, 161, 0.06);
  }

  .col-center { text-align: center; }
  .col-right  { text-align: right; }

  .toggle-pill {
    padding: 4px 10px;
    border-radius: 6px;
    border: 1px solid var(--border);
    background: var(--surface-2);
    color: var(--ink-2);
    font-size: var(--text-xs);
    font-weight: 500;
    cursor: pointer;
    box-shadow: none;
    transition: background 0.12s ease, color 0.12s ease, border-color 0.12s ease;
  }

  .toggle-pill:hover:not(:disabled) {
    transform: none;
    box-shadow: none;
    background: var(--surface);
    color: var(--ink);
  }

  .toggle-pill.toggle-on {
    background: #d6f1d0;
    color: #1e5b1e;
    border-color: #a6d6a0;
  }

  .toggle-pill.toggle-off {
    background: var(--surface-2);
    color: var(--ink-2);
    border-color: var(--border);
  }

  .toggle-pill:disabled {
    opacity: 0.55;
    cursor: not-allowed;
  }

  .badge-default {
    background: rgba(3, 105, 161, 0.12);
    color: var(--accent-2);
    border-radius: 6px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    font-size: var(--text-xs);
  }

  .badge-status {
    border-radius: 6px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    font-size: var(--text-xs);
  }

  .badge-status-ok {
    background: rgba(22, 163, 74, 0.16);
    color: #16a34a;
  }

  .badge-status-warn {
    background: rgba(220, 38, 38, 0.16);
    color: #dc2626;
  }

  .badge-status-neutral {
    background: var(--surface-2);
    color: var(--on-surface-2);
  }

  .materialization-main {
    display: inline-flex;
    align-items: center;
    gap: 8px;
  }

  .materialization-when {
    font-size: var(--text-xs);
    color: var(--ink-2);
  }

  .materialization-error {
    display: inline-block;
    max-width: 28ch;
    color: #991b1b;
    font-family: var(--mono);
    font-size: var(--text-xs);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .materialization-progress {
    margin-top: 4px;
    width: 140px;
    height: 4px;
    background: var(--surface-2);
    border-radius: 2px;
    overflow: hidden;
  }

  .materialization-progress-fill {
    height: 100%;
    background: var(--accent-1, #4c9bff);
    transition: width 0.2s ease;
  }

  .snapshot-rebuilding-label {
    display: block;
    margin-top: 2px;
    font-size: var(--text-xs);
    color: var(--ink-2);
  }

  .link-action {
    background: transparent;
    border: none;
    padding: 2px 4px;
    font-size: var(--text-xs);
    font-weight: 500;
    color: var(--accent-2);
    cursor: pointer;
    box-shadow: none;
  }

  .link-action:hover:not(:disabled) {
    text-decoration: underline;
    transform: none;
    box-shadow: none;
    background: transparent;
  }

  .link-action:disabled {
    color: var(--ink-2);
    opacity: 0.6;
    cursor: not-allowed;
  }

  .row-actions {
    display: inline-flex;
    flex-wrap: nowrap;
    gap: 4px;
    justify-content: flex-end;
  }

  .row-action {
    padding: 4px 10px;
    font-size: var(--text-xs);
    font-weight: 500;
    color: var(--ink-2);
    background: transparent;
    border: 1px solid var(--border);
    border-radius: 6px;
    box-shadow: none;
    cursor: pointer;
  }

  .row-action:hover:not(:disabled) {
    background: var(--surface-2);
    color: var(--ink);
    transform: none;
    box-shadow: none;
  }

  .row-action:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .row-action-danger:not(:disabled) {
    color: #991b1b;
    border-color: rgba(153, 27, 27, 0.35);
  }

  .row-action-danger:not(:disabled):hover {
    background: rgba(153, 27, 27, 0.08);
  }

  .row-action-primary:not(:disabled) {
    color: var(--accent-2);
    border-color: color-mix(in srgb, var(--accent-2) 45%, var(--border));
    font-weight: 600;
  }

  .row-action-primary:not(:disabled):hover {
    background: color-mix(in srgb, var(--accent-2) 8%, var(--surface-2));
  }

  /* Not positioned: the run menu must resolve against .cohort-section, which
     lies outside the scrolling table wrapper. */
  .run-split {
    display: inline-flex;
  }

  .run-split-main {
    border-top-right-radius: 0;
    border-bottom-right-radius: 0;
  }

  .run-split-caret {
    margin-left: -1px;
    padding-left: 7px;
    padding-right: 7px;
    border-top-left-radius: 0;
    border-bottom-left-radius: 0;
    font-size: 0.7em;
    line-height: 1;
  }

  .run-menu {
    position: absolute;
    top: 0;
    left: 0;
    z-index: 20;
    min-width: max-content;
    padding: 4px;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 6px;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.12);
  }

  .run-menu-item {
    display: block;
    width: 100%;
    padding: 6px 10px;
    text-align: left;
    white-space: nowrap;
    font-size: var(--text-xs);
    font-weight: 500;
    color: var(--ink);
    background: transparent;
    border: none;
    border-radius: 4px;
    box-shadow: none;
    cursor: pointer;
  }

  .run-menu-item:hover:not(:disabled) {
    background: var(--surface-2);
    transform: none;
    box-shadow: none;
  }

  .snapshot-subrow > td {
    padding: 12px 14px;
    background: var(--surface-2);
  }

  .snapshot-toolbar {
    display: flex;
    justify-content: flex-end;
    margin-bottom: 8px;
  }

  .snapshot-table {
    width: 100%;
    font-size: var(--text-sm);
  }

  .snapshot-table th,
  .snapshot-table td {
    padding: 6px 8px;
  }

  .snapshot-name {
    font-weight: 650;
  }

  .snapshot-id,
  .source-batch-id {
    color: var(--ink-2);
    font-size: var(--text-xs);
  }

  .mono { font-family: var(--mono); }

  .muted {
    color: var(--ink-2);
  }

  .cohort-form-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
    gap: 14px 16px;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: var(--text-sm);
  }

  .field-full {
    grid-column: 1 / -1;
  }

  .field-label {
    color: var(--ink-2);
    font-size: var(--text-xs);
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }

  .tag-match-indicator {
    display: inline-block;
    margin-top: 4px;
    font-size: var(--text-xs);
    font-weight: 600;
    letter-spacing: 0.02em;
  }

  .tag-match-existing {
    color: #1e5b1e;
  }

  .tag-match-new {
    color: var(--ink-2);
  }

  .field input[type="text"],
  .field input[type="number"] {
    padding: 6px 10px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--surface);
    color: var(--ink);
    font: inherit;
  }

  .cohort-flags {
    display: flex;
    flex-wrap: wrap;
    gap: 12px 22px;
    padding: 12px 14px;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--surface);
    margin: 0;
  }

  .cohort-flags legend {
    padding: 0 6px;
  }

  .check-field {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    font-size: var(--text-sm);
    color: var(--ink);
  }

  .cohort-form-actions {
    margin-top: 16px;
    display: flex;
    justify-content: flex-end;
  }
</style>
