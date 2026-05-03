<script>
  import { onMount } from "svelte";
  import { t } from "./i18n.js";

  let { apiBase = "/api/v1", onprofileschanged } = $props();

  const defaultProfileKey = "__default__";

  let loading = $state(false);
  let saving = $state(false);
  let applyingFix = $state(false);
  let deletingProfileId = $state(null);
  let defaultProfile = $state(null);
  let compatibility = $state(null);
  let compatSummaries = $state([]);
  let markingAllReviewed = $state(false);
  let storedProfiles = $state([]);
  let filteredProfiles = $state([]);
  let usageCounts = $state({});
  let usageTags = $state({});
  let searchQuery = $state("");
  let activeFilter = $state("all");
  let noticeMessage = $state("");
  let noticeTone = $state("");
  let draft = $state(emptyDraft());
  let workspace = $state({ type: "empty" });

  function emptyDraft(config = {}) {
    return {
      name: "",
      description: "",
      public: false,
      configText: JSON.stringify(config || {}, null, 2)
    };
  }

  const apiFetch = async (path, options = {}) => {
    const url = path?.startsWith("/") ? `${apiBase}${path}` : `${apiBase}/${path}`;
    const headers = { ...(options.headers || {}) };
    if (options.body && !headers["Content-Type"]) {
      headers["Content-Type"] = "application/json";
    }
    const response = await fetch(url, { ...options, headers });
    const contentType = response.headers.get("content-type") || "";
    const payload = contentType.includes("application/json")
      ? await response.json()
      : await response.text();
    if (!response.ok) {
      const message = payload?.error?.message || payload?.message || response.statusText;
      throw new Error(message);
    }
    return payload;
  };

  const setNotice = (message, tone = "") => {
    noticeMessage = message;
    noticeTone = tone;
  };

  const clearNotice = () => {
    noticeMessage = "";
    noticeTone = "";
  };

  const buildProfileUsage = (tags = []) => {
    const counts = {};
    const refs = {};
    for (const tag of tags) {
      const id = Number(tag?.default_profile_id);
      if (!Number.isFinite(id) || id <= 0) continue;
      counts[id] = (counts[id] || 0) + 1;
      refs[id] = [...(refs[id] || []), tag.name].sort((left, right) => left.localeCompare(right));
    }
    return { counts, refs };
  };

  const makeCopyName = (name) => {
    const base = String(name || "").trim();
    return base ? `${base} copy` : "copy";
  };

  const formatTimestampLocal = (value) => {
    if (!value) return "unknown";
    const parsed = new Date(value);
    if (Number.isNaN(parsed.getTime())) return "unknown";
    return parsed.toLocaleString("sv-SE");
  };

  const usageCount = (profileId) => Number(usageCounts[profileId] || 0);
  const usageList = (profileId) => usageTags[profileId] || [];

  const rawDraftSignature = (candidate) => JSON.stringify({
    name: String(candidate?.name || ""),
    description: String(candidate?.description || ""),
    public: !!candidate?.public,
    configText: String(candidate?.configText || "")
  });

  const normalizeDraftPayload = (candidate, currentProfileId = null) => {
    const name = String(candidate?.name || "").trim();
    if (!name) {
      return { error: $t("profile_editor_name_required") };
    }
    const duplicate = storedProfiles.some((profile) =>
      profile.id !== currentProfileId && profile.name.toLowerCase() === name.toLowerCase()
    );
    if (duplicate) {
      return { error: $t("profile_editor_name_conflict") };
    }

    let parsedConfig;
    try {
      parsedConfig = JSON.parse(candidate?.configText || "{}");
    } catch (_) {
      return { error: $t("profile_editor_json_invalid") };
    }
    if (!parsedConfig || typeof parsedConfig !== "object" || Array.isArray(parsedConfig)) {
      return { error: $t("profile_editor_json_object_required") };
    }

    const payload = {
      name,
      description: String(candidate?.description || "").trim(),
      public: !!candidate?.public,
      config: parsedConfig
    };
    return {
      payload,
      signature: JSON.stringify({
        name: payload.name,
        description: payload.description,
        public: payload.public,
        config: JSON.stringify(payload.config)
      })
    };
  };

  const draftFromProfile = (profile, overrides = {}) => ({
    name: profile?.name || "",
    description: profile?.description || "",
    public: !!profile?.public,
    configText: JSON.stringify(profile?.config || {}, null, 2),
    ...overrides
  });

  const loadCompatibility = async (profileId) => {
    compatibility = null;
    try {
      compatibility = await apiFetch(`/profiles/${profileId}/compatibility`);
    } catch (_) {
      // Ignore - compatibility is best-effort; don't surface load errors
    }
  };

  const applyCompatFix = async (op, module = null) => {
    if (!editingProfileId || applyingFix) return;
    applyingFix = true;
    try {
      const body = module ? { op, module } : { op };
      await apiFetch(`/profiles/${editingProfileId}`, {
        method: "PATCH",
        body: JSON.stringify(body)
      });
      await loadProfiles({ selectKey: `profile:${editingProfileId}`, preserveNotice: true });
      onprofileschanged?.({ profiles: storedProfiles });
    } catch (error) {
      setNotice($t("profile_compat_fix_error", { error: error.message || "unknown error" }), "warn");
    } finally {
      applyingFix = false;
    }
  };

  const markAllReviewed = async () => {
    if (markingAllReviewed) return;
    markingAllReviewed = true;
    try {
      await apiFetch("/profiles/mark-all-reviewed", { method: "POST", body: "{}" });
      await loadProfiles({ preserveNotice: true });
      onprofileschanged?.({ profiles: storedProfiles });
    } catch (error) {
      setNotice($t("profile_compat_fix_error", { error: error.message || "unknown error" }), "warn");
    } finally {
      markingAllReviewed = false;
    }
  };

  const maybeDiscardChanges = () => {
    if (!isEditableWorkspace || !hasDirtyChanges) return true;
    return window.confirm($t("profile_editor_discard_confirm"));
  };

  const openDefaultProfile = ({ clear = true } = {}) => {
    workspace = { type: "default" };
    draft = emptyDraft(defaultProfile?.config || {});
    compatibility = null;
    if (clear) clearNotice();
  };

  const openStoredProfile = (profile, { clear = true } = {}) => {
    if (!profile) return;
    const nextDraft = draftFromProfile(profile);
    const normalized = normalizeDraftPayload(nextDraft, profile.id);
    workspace = {
      type: "edit",
      profileId: profile.id,
      seedDraft: { ...nextDraft },
      savedSignature: normalized.signature || "",
      savedRawSignature: rawDraftSignature(nextDraft)
    };
    draft = nextDraft;
    loadCompatibility(profile.id);
    if (clear) clearNotice();
  };

  const openNewDraft = ({ clear = true } = {}) => {
    const seed = emptyDraft(defaultProfile?.config || {});
    workspace = {
      type: "new",
      sourceName: defaultProfile?.name || "",
      seedDraft: { ...seed },
      savedSignature: normalizeDraftPayload(seed).signature || "",
      savedRawSignature: rawDraftSignature(seed)
    };
    draft = seed;
    compatibility = null;
    if (clear) clearNotice();
  };

  const duplicateProfile = (profile) => {
    if (!profile || !maybeDiscardChanges()) return;
    const seed = draftFromProfile(profile, { name: makeCopyName(profile.name) });
    workspace = {
      type: "duplicate",
      sourceName: profile.name,
      seedDraft: { ...seed },
      savedSignature: normalizeDraftPayload(seed).signature || "",
      savedRawSignature: rawDraftSignature(seed)
    };
    draft = seed;
    clearNotice();
  };

  const selectProfile = (profile) => {
    if (!profile || !maybeDiscardChanges()) return;
    if (profile.id === 0) {
      openDefaultProfile();
      return;
    }
    openStoredProfile(profile);
  };

  const selectProfileByKey = (profileKey) => {
    if (profileKey === defaultProfileKey) {
      openDefaultProfile({ clear: false });
      return;
    }
    const profileId = Number(String(profileKey || "").replace(/^profile:/, ""));
    if (!Number.isFinite(profileId) || profileId <= 0) {
      openDefaultProfile({ clear: false });
      return;
    }
    const match = storedProfiles.find((profile) => profile.id === profileId);
    if (match) {
      openStoredProfile(match, { clear: false });
      return;
    }
    openDefaultProfile({ clear: false });
  };

  const reconcileWorkspace = () => {
    if (workspace.type === "edit") {
      const current = storedProfiles.find((profile) => profile.id === workspace.profileId);
      if (!current) {
        openDefaultProfile({ clear: false });
      }
      return;
    }
    if (workspace.type === "default") {
      if (!defaultProfile) {
        workspace = { type: "empty" };
      }
      return;
    }
    if (workspace.type === "empty") {
      if (defaultProfile) {
        openDefaultProfile({ clear: false });
      }
    }
  };

  const loadProfiles = async (options = {}) => {
    const { selectKey = null, preserveNotice = false } = options;
    loading = true;
    try {
      const [defaultData, profileData, tagData, compatData] = await Promise.all([
        apiFetch("/profiles/default"),
        apiFetch("/profiles"),
        apiFetch("/tags?limit=500"),
        apiFetch("/profiles/compatibility").catch(() => [])
      ]);
      defaultProfile = defaultData && typeof defaultData === "object" && Number(defaultData.id) === 0
        ? defaultData
        : null;
      storedProfiles = Array.isArray(profileData) ? profileData : [];
      compatSummaries = Array.isArray(compatData) ? compatData : [];
      const usage = buildProfileUsage(Array.isArray(tagData) ? tagData : []);
      usageCounts = usage.counts;
      usageTags = usage.refs;
      if (selectKey !== null) {
        selectProfileByKey(selectKey);
      } else {
        reconcileWorkspace();
      }
      if (!preserveNotice) {
        clearNotice();
      }
    } catch (error) {
      setNotice($t("profile_load_error", { error: error.message || "unknown error" }), "warn");
    } finally {
      loading = false;
    }
  };

  const formatDraftJSON = () => {
    try {
      draft = {
        ...draft,
        configText: JSON.stringify(JSON.parse(draft.configText || "{}"), null, 2)
      };
      clearNotice();
    } catch (_) {
      setNotice($t("profile_editor_json_invalid"), "warn");
    }
  };

  const resetDraft = () => {
    if (!workspace.seedDraft) return;
    draft = { ...workspace.seedDraft };
    clearNotice();
  };

  const saveProfile = async () => {
    if (!isEditableWorkspace) return;
    const normalized = normalizeDraftPayload(draft, editingProfileId);
    if (normalized.error) {
      setNotice(normalized.error, "warn");
      return;
    }
    saving = true;
    try {
      const path = workspace.type === "edit" ? `/profiles/${workspace.profileId}` : "/profiles";
      const method = workspace.type === "edit" ? "PUT" : "POST";
      const saved = await apiFetch(path, {
        method,
        body: JSON.stringify(normalized.payload)
      });
      setNotice(
        workspace.type === "edit"
          ? $t("profile_saved")
          : $t("profile_created"),
        "ok"
      );
      await loadProfiles({ selectKey: `profile:${saved.id}`, preserveNotice: true });
      if (workspace.type === "edit") {
        await loadCompatibility(saved.id);
      }
      onprofileschanged?.({ profiles: storedProfiles });
    } catch (error) {
      setNotice($t("profile_save_error", { error: error.message || "unknown error" }), "warn");
    } finally {
      saving = false;
    }
  };

  const deleteProfile = async (profile) => {
    if (!profile || profile.id <= 0 || deletingProfileId !== null) return;
    if (!window.confirm($t("profile_delete_confirm", { name: profile.name }))) {
      return;
    }
    deletingProfileId = profile.id;
    try {
      await apiFetch(`/profiles/${profile.id}`, { method: "DELETE" });
      if (workspace.type === "edit" && workspace.profileId === profile.id) {
        workspace = { type: "empty" };
      }
      setNotice($t("profile_deleted"), "ok");
      await loadProfiles({ selectKey: defaultProfileKey, preserveNotice: true });
      onprofileschanged?.({ profiles: storedProfiles });
    } catch (error) {
      setNotice($t("profile_delete_error", { error: error.message || "unknown error" }), "warn");
    } finally {
      deletingProfileId = null;
    }
  };

  const profileMatchesFilter = (profile) => {
    const query = searchQuery.trim().toLowerCase();
    if (query) {
      const haystack = [profile.name, profile.description].filter(Boolean).join(" ").toLowerCase();
      if (!haystack.includes(query)) return false;
    }
    if (activeFilter === "public" && !profile.public) return false;
    if (activeFilter === "in_use" && usageCount(profile.id) === 0) return false;
    return true;
  };

  let libraryProfiles = $derived(defaultProfile ? [defaultProfile, ...storedProfiles] : storedProfiles);
  let incompatibleIds = $derived(new Set(compatSummaries.filter(s => !s.compatible).map(s => s.id)));
  let incompatibleCount = $derived(incompatibleIds.size);
  $effect(() => {
    filteredProfiles = libraryProfiles.filter(profileMatchesFilter);
  });
  let editingProfileId = $derived(workspace.type === "edit" ? workspace.profileId : null);
  let selectedStoredProfile = $derived(editingProfileId
    ? storedProfiles.find((profile) => profile.id === editingProfileId) || null
    : null);
  let selectedUsageTags = $derived(selectedStoredProfile ? usageList(selectedStoredProfile.id) : []);
  let selectedLibraryKey = $derived(workspace.type === "edit"
    ? `profile:${workspace.profileId}`
    : workspace.type === "default"
      ? defaultProfileKey
      : "");
  let isEditableWorkspace = $derived(workspace.type === "edit" || workspace.type === "new" || workspace.type === "duplicate");
  let draftValidation = $derived(isEditableWorkspace
    ? normalizeDraftPayload(draft, editingProfileId)
    : { payload: null, signature: "" });
  let hasDirtyChanges = $derived(isEditableWorkspace
    ? rawDraftSignature(draft) !== String(workspace.savedRawSignature || "")
    : false);
  let canSave = $derived(isEditableWorkspace && hasDirtyChanges && !draftValidation.error && !saving);

  onMount(() => {
    loadProfiles();
  });
</script>

<section class="settings-section">
  <div class="section-head">
    <div>
      <h2>{$t("settings_profiles_heading")}</h2>
      <p>{$t("settings_profiles_subtitle")}</p>
    </div>
  </div>

  {#if noticeMessage}
    <div class={`inline-notice inline-notice-${noticeTone === "ok" ? "ok" : "warn"}`} role="status" aria-live="polite">
      {noticeMessage}
    </div>
  {/if}

  <div class="profile-layout">
    <aside class="profile-library">
      <div class="library-head">
        <div>
          <h3>{$t("profile_library_heading")}</h3>
          <p>{$t("profile_library_subtitle")}</p>
        </div>
        <button class="secondary" type="button" onclick={() => maybeDiscardChanges() && openNewDraft()}>
          {$t("profile_new_button")}
        </button>
      </div>

      <div class="library-controls">
        <div class="stack">
          <label for="profile-search">{$t("profile_search_label")}</label>
          <input
            id="profile-search"
            type="search"
            bind:value={searchQuery}
            placeholder={$t("profile_search_placeholder")}
          />
        </div>
        <div class="stack">
          <label for="profile-filter">{$t("profile_filter_label")}</label>
          <select id="profile-filter" bind:value={activeFilter}>
            <option value="all">{$t("profile_filter_all")}</option>
            <option value="public">{$t("profile_filter_public")}</option>
            <option value="in_use">{$t("profile_filter_in_use")}</option>
          </select>
        </div>
      </div>

      {#if incompatibleCount > 0}
        <div class="library-compat-warning" role="status">
          <span class="small">{$t("profile_library_compat_warning", { count: incompatibleCount })}</span>
          <button
            class="ghost mini-button"
            type="button"
            disabled={markingAllReviewed}
            onclick={markAllReviewed}
          >{markingAllReviewed ? $t("submitting") : $t("profile_library_mark_all_reviewed")}</button>
        </div>
      {/if}

      {#if loading}
        <p class="small">{$t("loading")}</p>
      {:else if filteredProfiles.length === 0}
        <p class="small">{$t("profile_no_profiles")}</p>
      {:else}
        <div class="profile-list" role="list" aria-label={$t("profile_library_heading")}>
          {#each filteredProfiles as profile (profile.id === 0 ? defaultProfileKey : profile.id)}
            <div class={`profile-row ${selectedLibraryKey === (profile.id === 0 ? defaultProfileKey : `profile:${profile.id}`) ? "selected" : ""}`} role="listitem">
              <button class="profile-row-select" type="button" onclick={() => selectProfile(profile)}>
                <div class="profile-row-main">
                  <div class="profile-row-title">
                    <strong>{profile.name}</strong>
                    <div class="profile-row-badges">
                      {#if profile.id === 0}
                        <span class="badge default-badge">{$t("profile_default_badge")}</span>
                      {/if}
                      {#if profile.public}
                        <span class="badge">{$t("profile_public_badge")}</span>
                      {/if}
                      {#if usageCount(profile.id) > 0}
                        <span class="badge usage-badge">{$t("profile_in_use_badge", { count: usageCount(profile.id) })}</span>
                      {/if}
                      {#if incompatibleIds.has(profile.id)}
                        <span class="badge warn-badge">{$t("profile_compat_needs_review")}</span>
                      {/if}
                    </div>
                  </div>
                  <div class="profile-row-description">{profile.description || $t("profile_preview_no_description")}</div>
                  {#if profile.id > 0}
                    <div class="profile-row-meta">
                      {$t("profile_updated_prefix", { time: formatTimestampLocal(profile.updated_at) })}
                    </div>
                  {/if}
                </div>
              </button>
              {#if profile.id > 0}
                <div class="profile-row-actions">
                  <button
                    class="ghost mini-button"
                    type="button"
                    onclick={(e) => { e.stopPropagation(); duplicateProfile(profile); }}
                  >
                    {$t("profile_duplicate_button")}
                  </button>
                  <button
                    class="ghost mini-button"
                    type="button"
                    disabled={deletingProfileId === profile.id}
                    onclick={(e) => { e.stopPropagation(); deleteProfile(profile); }}
                  >
                    {deletingProfileId === profile.id ? $t("submitting") : $t("profile_delete_button")}
                  </button>
                </div>
              {/if}
            </div>
          {/each}
        </div>
      {/if}
    </aside>

    <section class="profile-workspace">
      {#if workspace.type === "default" && defaultProfile}
        <div class="workspace-head">
          <div>
            <h3>{defaultProfile.name}</h3>
            <p>{defaultProfile.description}</p>
          </div>
          <div class="profile-row-badges">
            <span class="badge default-badge">{$t("profile_default_badge")}</span>
          </div>
        </div>

        <div class="workspace-summary">
          <div class="summary-item">
            <span class="summary-label">{$t("profile_preview_name_label")}</span>
            <strong>{defaultProfile.name}</strong>
          </div>
          <div class="summary-item">
            <span class="summary-label">{$t("profile_preview_description_label")}</span>
            <strong>{defaultProfile.description}</strong>
          </div>
          <div class="summary-item">
            <span class="summary-label">{$t("profile_preview_source_label")}</span>
            <strong>{$t("profile_default_source")}</strong>
          </div>
        </div>

        <div class="row">
          <button class="secondary" type="button" onclick={openNewDraft}>
            {$t("profile_new_from_default_button")}
          </button>
        </div>

        <div class="stack">
          <div class="field-label">{$t("profile_preview_config_label")}</div>
          <textarea
            class="profile-config-textarea"
            rows="18"
            readonly
            value={JSON.stringify(defaultProfile.config || {}, null, 2)}
          ></textarea>
        </div>
      {:else if isEditableWorkspace}
        <div class="workspace-head">
          <div>
            <h3>
              {#if workspace.type === "edit"}
                {$t("profile_editor_edit_title")}
              {:else if workspace.type === "duplicate"}
                {$t("profile_preview_duplicate_title")}
              {:else}
                {$t("profile_preview_new_title")}
              {/if}
            </h3>
            <p>
              {#if workspace.type === "edit"}
                {$t("profile_editor_edit_subtitle")}
              {:else if workspace.type === "duplicate"}
                {$t("profile_preview_source_duplicate", { name: workspace.sourceName || "" })}
              {:else}
                {$t("profile_editor_seeded_from_default")}
              {/if}
            </p>
          </div>
          <div class="profile-row-badges">
            <span class="badge">{draft.public ? $t("profile_preview_visibility_public") : $t("profile_preview_visibility_private")}</span>
            {#if workspace.type === "edit" && usageCount(workspace.profileId) > 0}
              <span class="badge usage-badge">{$t("profile_in_use_badge", { count: usageCount(workspace.profileId) })}</span>
            {/if}
          </div>
        </div>

        {#if workspace.type === "edit" && selectedUsageTags.length > 0}
          <div class="stack">
            <div class="field-label">{$t("profile_used_by_heading")}</div>
            <div class="tag-ref-list">
              {#each selectedUsageTags as tagName}
                <span class="badge usage-badge">{tagName}</span>
              {/each}
            </div>
          </div>
        {/if}

        {#if workspace.type === "edit" && compatibility && !compatibility.compatible}
          <div class="compat-banner" role="alert" aria-label={$t("profile_compat_banner_label")}>
            <div class="compat-banner-summary">
              <strong>{$t("profile_compat_issues_heading", { count: compatibility.issues.length })}</strong>
            </div>
            <ul class="compat-issue-list">
              {#each compatibility.issues as issue}
                <li class="compat-issue">
                  <span class="compat-issue-detail">{issue.detail}</span>
                  <span class="compat-issue-suggestion small">{issue.suggestion}</span>
                </li>
              {/each}
            </ul>
            <div class="row compat-actions">
              {#if compatibility.issues.some(i => i.type === "missing_test_case")}
                <button class="ghost" type="button" disabled={applyingFix || hasDirtyChanges}
                  onclick={() => applyCompatFix("add_missing_test_cases")}>
                  {$t("profile_compat_add_test_cases")}
                </button>
              {/if}
              {#if compatibility.issues.some(i => i.type === "missing_test_levels")}
                <button class="ghost" type="button" disabled={applyingFix || hasDirtyChanges}
                  onclick={() => applyCompatFix("add_missing_test_levels")}>
                  {$t("profile_compat_add_test_levels")}
                </button>
              {/if}
              {#if compatibility.issues.some(i => i.type === "missing_test_case")}
                <button class="ghost" type="button" disabled={applyingFix || hasDirtyChanges}
                  onclick={() => applyCompatFix("reset_test_cases")}>
                  {$t("profile_compat_reset_test_cases")}
                </button>
              {/if}
              {#each compatibility.issues.filter(i => i.type === "missing_test_levels") as issue}
                <button class="ghost" type="button" disabled={applyingFix || hasDirtyChanges}
                  onclick={() => applyCompatFix("reset_test_levels", issue.module)}>
                  {$t("profile_compat_reset_test_levels", { module: issue.module })}
                </button>
              {/each}
              <button class="ghost" type="button" disabled={applyingFix || hasDirtyChanges}
                onclick={() => applyCompatFix("mark_reviewed")}>
                {$t("profile_compat_mark_reviewed")}
              </button>
            </div>
          </div>
        {/if}

        <div class="stack">
          <label for="profile-editor-name">{$t("profile_preview_name_label")}</label>
          <input id="profile-editor-name" type="text" bind:value={draft.name} />
        </div>

        <div class="stack">
          <label for="profile-editor-description">{$t("profile_preview_description_label")}</label>
          <input id="profile-editor-description" type="text" bind:value={draft.description} />
        </div>

        <label class="checkbox-row" for="profile-editor-public">
          <span>{$t("profile_editor_public_label")}</span>
          <input id="profile-editor-public" type="checkbox" bind:checked={draft.public} />
        </label>
        <div class="small">{$t("profile_editor_public_hint")}</div>

        <div class="stack">
          <label for="profile-editor-config">{$t("profile_preview_config_label")}</label>
          <textarea
            id="profile-editor-config"
            class="profile-config-textarea"
            rows="18"
            bind:value={draft.configText}
          ></textarea>
        </div>

        {#if draftValidation.error}
          <div class="inline-notice inline-notice-warn" role="status">
            {draftValidation.error}
          </div>
        {/if}

        <div class="row editor-actions">
          <button type="button" onclick={saveProfile} disabled={!canSave}>
            {saving ? $t("submitting") : $t("profile_editor_save_button")}
          </button>
          <button class="ghost" type="button" onclick={formatDraftJSON}>
            {$t("profile_editor_format_button")}
          </button>
          <button class="ghost" type="button" onclick={resetDraft} disabled={!hasDirtyChanges}>
            {$t(workspace.type === "edit" ? "profile_editor_reset_button" : "profile_editor_cancel_button")}
          </button>
          {#if workspace.type === "edit" && selectedStoredProfile}
            <button
              class="ghost"
              type="button"
              disabled={deletingProfileId === selectedStoredProfile.id}
              onclick={() => deleteProfile(selectedStoredProfile)}
            >
              {deletingProfileId === selectedStoredProfile.id ? $t("submitting") : $t("profile_delete_button")}
            </button>
          {/if}
        </div>
      {:else}
        <div class="workspace-empty">
          <h3>{$t("profile_preview_empty_title")}</h3>
          <p>{$t("profile_preview_empty_body")}</p>
        </div>
      {/if}
    </section>
  </div>
</section>

<style>
  .settings-section {
    display: flex;
    flex-direction: column;
    gap: 18px;
  }

  .section-head h2,
  .library-head h3,
  .workspace-head h3,
  .workspace-empty h3 {
    margin: 0;
  }

  .section-head p,
  .library-head p,
  .workspace-head p,
  .workspace-empty p {
    margin: 4px 0 0;
    color: var(--muted);
  }

  .inline-notice {
    border: 1px solid transparent;
    border-radius: 10px;
    padding: 10px 12px;
    font-size: 0.85rem;
  }

  .inline-notice-ok {
    background: #e7f8ee;
    border-color: #a7f3d0;
    color: #065f46;
  }

  .inline-notice-warn {
    background: #fef3c7;
    border-color: #fcd34d;
    color: #92400e;
  }

  .profile-layout {
    display: grid;
    grid-template-columns: minmax(300px, 0.95fr) minmax(0, 1.35fr);
    gap: 16px;
    align-items: start;
  }

  .profile-library,
  .profile-workspace {
    border: 1px solid var(--border);
    border-radius: 14px;
    background: var(--surface);
    padding: 16px;
    display: flex;
    flex-direction: column;
    gap: 14px;
    min-width: 0;
  }

  .library-head,
  .workspace-head {
    display: flex;
    justify-content: space-between;
    gap: 12px;
    align-items: flex-start;
  }

  .library-controls {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 140px;
    gap: 10px;
  }

  .profile-list {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .profile-row {
    border: 1px solid var(--border);
    border-radius: 12px;
    background: var(--card);
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: 10px;
    align-items: start;
    overflow: hidden;
  }

  .profile-row-select {
    width: 100%;
    text-align: left;
    padding: 12px;
    background: transparent;
    color: inherit;
    box-shadow: none;
    border-radius: 0;
    border: none;
    cursor: pointer;
  }

  .profile-row-select:hover:not(:disabled) {
    transform: none;
    box-shadow: none;
  }

  .profile-row.selected {
    border-color: var(--accent-2);
    box-shadow: 0 0 0 2px rgba(14, 116, 144, 0.16);
  }

  .profile-row:hover {
    box-shadow: 0 8px 16px rgba(11, 45, 77, 0.12);
  }

  .profile-row-main {
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .profile-row-title {
    display: flex;
    flex-wrap: wrap;
    justify-content: space-between;
    gap: 10px;
    align-items: flex-start;
  }

  .profile-row-description,
  .profile-row-meta {
    color: var(--muted);
    font-size: 0.82rem;
  }

  .profile-row-badges {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    align-items: center;
  }

  .profile-row-actions {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
    justify-content: flex-end;
    padding: 12px 12px 12px 0;
  }

  .usage-badge {
    background: rgba(14, 116, 144, 0.12);
    color: var(--accent-2);
  }

  .default-badge {
    background: rgba(18, 95, 54, 0.12);
    color: #166534;
  }

  .warn-badge {
    background: rgba(202, 138, 4, 0.15);
    color: #92400e;
  }

  .library-compat-warning {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    padding: 6px 8px;
    border-radius: 6px;
    background: rgba(202, 138, 4, 0.1);
    border: 1px solid rgba(202, 138, 4, 0.3);
    margin-bottom: 4px;
  }

  .workspace-summary {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
    gap: 10px;
  }

  .tag-ref-list {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
  }

  .checkbox-row {
    display: flex;
    align-items: center;
    justify-content: flex-start;
    gap: 10px;
    font-weight: 600;
    width: fit-content;
    cursor: pointer;
  }

  .checkbox-row input {
    width: auto;
    margin: 0;
    flex: 0 0 auto;
  }

  .profile-config-textarea {
    min-height: 320px;
    font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  }

  .compat-banner {
    border: 1px solid #fcd34d;
    border-radius: 10px;
    background: #fef9ee;
    padding: 12px 14px;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .compat-issue-list {
    margin: 0;
    padding: 0 0 0 1.2em;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .compat-issue {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .compat-issue-detail {
    font-size: 0.85rem;
  }

  .compat-issue-suggestion {
    color: var(--muted);
  }

  .compat-actions {
    flex-wrap: wrap;
    gap: 6px;
  }

  .editor-actions {
    flex-wrap: wrap;
  }

  .workspace-empty {
    min-height: 260px;
    display: flex;
    flex-direction: column;
    justify-content: center;
    align-items: flex-start;
    gap: 8px;
  }

  @media (max-width: 900px) {
    .profile-layout {
      grid-template-columns: 1fr;
    }
  }

  @media (max-width: 640px) {
    .library-head,
    .workspace-head,
    .profile-row {
      grid-template-columns: 1fr;
    }

    .library-controls {
      grid-template-columns: 1fr;
    }

    .profile-row-actions {
      justify-content: flex-start;
    }
  }
</style>
