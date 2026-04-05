<script>
  import { onMount } from "svelte";
  import { t } from "./i18n.js";

  export let apiBase = "/api/v1";

  let loading = false;
  let deletingProfileId = null;
  let profiles = [];
  let filteredProfiles = [];
  let usageCounts = {};
  let usageTags = {};
  let searchQuery = "";
  let activeFilter = "all";
  let noticeMessage = "";
  let noticeTone = "";
  let workspace = { type: "empty" };

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

  const prettyJSON = (value) => JSON.stringify(value || {}, null, 2);

  const formatTimestampLocal = (value) => {
    if (!value) return "unknown";
    const parsed = new Date(value);
    if (Number.isNaN(parsed.getTime())) return "unknown";
    return parsed.toLocaleString();
  };

  const usageCount = (profileId) => Number(usageCounts[profileId] || 0);
  const usageList = (profileId) => usageTags[profileId] || [];

  const openFirstProfile = () => {
    if (profiles.length > 0) {
      workspace = { type: "profile", profileId: profiles[0].id };
    } else {
      workspace = { type: "empty" };
    }
  };

  const reconcileWorkspace = () => {
    if (workspace.type === "profile") {
      const exists = profiles.some((profile) => profile.id === workspace.profileId);
      if (!exists) {
        openFirstProfile();
      }
      return;
    }
    if (workspace.type === "empty") {
      openFirstProfile();
    }
  };

  const loadProfiles = async () => {
    loading = true;
    try {
      const [profileData, tagData] = await Promise.all([
        apiFetch("/profiles"),
        apiFetch("/tags?limit=500")
      ]);
      profiles = Array.isArray(profileData) ? profileData : [];
      const usage = buildProfileUsage(Array.isArray(tagData) ? tagData : []);
      usageCounts = usage.counts;
      usageTags = usage.refs;
      reconcileWorkspace();
      clearNotice();
    } catch (error) {
      setNotice($t("profile_load_error", { error: error.message || "unknown error" }), "warn");
    } finally {
      loading = false;
    }
  };

  const selectProfile = (profileId) => {
    workspace = { type: "profile", profileId };
    clearNotice();
  };

  const createDraftPreview = () => {
    workspace = {
      type: "new",
      draft: {
        name: "",
        description: "",
        public: false,
        config: {}
      }
    };
    clearNotice();
  };

  const duplicateProfile = (profile) => {
    if (!profile) return;
    workspace = {
      type: "duplicate",
      sourceProfileId: profile.id,
      draft: {
        name: makeCopyName(profile.name),
        description: profile.description || "",
        public: !!profile.public,
        config: profile.config || {}
      }
    };
    clearNotice();
  };

  const deleteProfile = async (profile) => {
    if (!profile || deletingProfileId !== null) return;
    if (!window.confirm($t("profile_delete_confirm", { name: profile.name }))) {
      return;
    }
    deletingProfileId = profile.id;
    try {
      await apiFetch(`/profiles/${profile.id}`, { method: "DELETE" });
      if (workspace.type === "profile" && workspace.profileId === profile.id) {
        workspace = { type: "empty" };
      }
      setNotice($t("profile_deleted"), "ok");
      await loadProfiles();
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

  $: {
    profiles;
    activeFilter;
    searchQuery;
    filteredProfiles = profiles.filter(profileMatchesFilter);
  }
  $: selectedProfile = workspace.type === "profile"
    ? profiles.find((profile) => profile.id === workspace.profileId) || null
    : null;
  $: selectedUsageTags = selectedProfile ? usageList(selectedProfile.id) : [];

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
        <button class="secondary" type="button" on:click={createDraftPreview}>
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

      {#if loading}
        <p class="small">{$t("loading")}</p>
      {:else if filteredProfiles.length === 0}
        <p class="small">{$t("profile_no_profiles")}</p>
      {:else}
        <div class="profile-list" role="list" aria-label={$t("profile_library_heading")}>
          {#each filteredProfiles as profile (profile.id)}
            <div class={`profile-row ${workspace.type === "profile" && workspace.profileId === profile.id ? "selected" : ""}`} role="listitem">
              <button class="profile-row-select" type="button" on:click={() => selectProfile(profile.id)}>
                <div class="profile-row-main">
                  <div class="profile-row-title">
                    <strong>{profile.name}</strong>
                    <div class="profile-row-badges">
                      {#if profile.public}
                        <span class="badge">{$t("profile_public_badge")}</span>
                      {/if}
                      {#if usageCount(profile.id) > 0}
                        <span class="badge usage-badge">{$t("profile_in_use_badge", { count: usageCount(profile.id) })}</span>
                      {/if}
                    </div>
                  </div>
                  <div class="profile-row-description">{profile.description || "—"}</div>
                  <div class="profile-row-meta">
                    {$t("profile_updated_prefix", { time: formatTimestampLocal(profile.updated_at) })}
                  </div>
                </div>
              </button>
              <div class="profile-row-actions">
                <button
                  class="ghost mini-button"
                  type="button"
                  on:click|stopPropagation={() => duplicateProfile(profile)}
                >
                  {$t("profile_duplicate_button")}
                </button>
                <button
                  class="ghost mini-button"
                  type="button"
                  disabled={deletingProfileId === profile.id}
                  on:click|stopPropagation={() => deleteProfile(profile)}
                >
                  {deletingProfileId === profile.id ? $t("submitting") : $t("profile_delete_button")}
                </button>
              </div>
            </div>
          {/each}
        </div>
      {/if}
    </aside>

    <section class="profile-workspace">
      {#if workspace.type === "profile" && selectedProfile}
        <div class="workspace-head">
          <div>
            <h3>{selectedProfile.name}</h3>
            <p>{selectedProfile.description || $t("profile_preview_no_description")}</p>
          </div>
          <div class="profile-row-badges">
            <span class="badge">{selectedProfile.public ? $t("profile_preview_visibility_public") : $t("profile_preview_visibility_private")}</span>
            {#if usageCount(selectedProfile.id) > 0}
              <span class="badge usage-badge">{$t("profile_in_use_badge", { count: usageCount(selectedProfile.id) })}</span>
            {/if}
          </div>
        </div>

        <div class="workspace-summary">
          <div class="summary-item">
            <span class="summary-label">{$t("profile_preview_name_label")}</span>
            <strong>{selectedProfile.name}</strong>
          </div>
          <div class="summary-item">
            <span class="summary-label">{$t("profile_preview_description_label")}</span>
            <strong>{selectedProfile.description || "—"}</strong>
          </div>
          <div class="summary-item">
            <span class="summary-label">{$t("profile_preview_updated_label")}</span>
            <strong>{formatTimestampLocal(selectedProfile.updated_at)}</strong>
          </div>
        </div>

        {#if selectedUsageTags.length > 0}
          <div class="stack">
            <div class="field-label">{$t("profile_used_by_heading")}</div>
            <div class="tag-ref-list">
              {#each selectedUsageTags as tagName}
                <span class="badge usage-badge">{tagName}</span>
              {/each}
            </div>
          </div>
        {/if}

        <div class="stack">
          <div class="field-label">{$t("profile_preview_config_label")}</div>
          <pre>{prettyJSON(selectedProfile.config)}</pre>
        </div>
      {:else if workspace.type === "new"}
        <div class="workspace-head">
          <div>
            <h3>{$t("profile_preview_new_title")}</h3>
            <p>{$t("profile_preview_mode_note")}</p>
          </div>
          <span class="badge">{$t("profile_preview_visibility_private")}</span>
        </div>

        <div class="workspace-summary">
          <div class="summary-item">
            <span class="summary-label">{$t("profile_preview_name_label")}</span>
            <strong>—</strong>
          </div>
          <div class="summary-item">
            <span class="summary-label">{$t("profile_preview_description_label")}</span>
            <strong>—</strong>
          </div>
          <div class="summary-item">
            <span class="summary-label">{$t("profile_preview_source_label")}</span>
            <strong>{$t("profile_preview_source_new")}</strong>
          </div>
        </div>

        <div class="stack">
          <div class="field-label">{$t("profile_preview_config_label")}</div>
          <pre>{prettyJSON(workspace.draft.config)}</pre>
        </div>
      {:else if workspace.type === "duplicate"}
        <div class="workspace-head">
          <div>
            <h3>{$t("profile_preview_duplicate_title")}</h3>
            <p>{$t("profile_preview_mode_note")}</p>
          </div>
          <span class="badge">{workspace.draft.public ? $t("profile_preview_visibility_public") : $t("profile_preview_visibility_private")}</span>
        </div>

        <div class="workspace-summary">
          <div class="summary-item">
            <span class="summary-label">{$t("profile_preview_name_label")}</span>
            <strong>{workspace.draft.name}</strong>
          </div>
          <div class="summary-item">
            <span class="summary-label">{$t("profile_preview_description_label")}</span>
            <strong>{workspace.draft.description || "—"}</strong>
          </div>
          <div class="summary-item">
            <span class="summary-label">{$t("profile_preview_source_label")}</span>
            <strong>{$t("profile_preview_source_duplicate", { name: profiles.find((profile) => profile.id === workspace.sourceProfileId)?.name || "" })}</strong>
          </div>
        </div>

        <div class="stack">
          <div class="field-label">{$t("profile_preview_config_label")}</div>
          <pre>{prettyJSON(workspace.draft.config)}</pre>
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
