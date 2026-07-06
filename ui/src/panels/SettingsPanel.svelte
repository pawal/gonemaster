<script>
  import { t } from "../i18n.js";
  import ProfileSettings from "./ProfileSettings.svelte";
  import ServerSettings from "./ServerSettings.svelte";
  import ScoringSettings from "./ScoringSettings.svelte";

  let {
    settingsSubTab = "system",
    settingsSubTabs = [],
    onSetSubTab = () => {},
    onProfilesChanged = () => {},
  } = $props();

  // Roving arrow-key navigation for the ARIA tablist.
  const handleTabKeydown = (e) => {
    const ids = settingsSubTabs.map((s) => s.id);
    const idx = ids.indexOf(settingsSubTab);
    if (idx < 0) return;
    let next = idx;
    if (e.key === "ArrowRight" || e.key === "ArrowDown") next = (idx + 1) % ids.length;
    else if (e.key === "ArrowLeft" || e.key === "ArrowUp") next = (idx - 1 + ids.length) % ids.length;
    else if (e.key === "Home") next = 0;
    else if (e.key === "End") next = ids.length - 1;
    else return;
    e.preventDefault();
    const nextId = ids[next];
    onSetSubTab(nextId);
    document.getElementById(`settings-subtab-${nextId}`)?.focus();
  };
</script>

<div class="grid panel-settings" id="panel-settings" role="tabpanel" aria-labelledby="tab-settings">
  <div class="settings-subtabs" role="tablist" aria-label={$t("settings_subtabs_aria")}>
    {#each settingsSubTabs as subTab}
      <button
        class={`settings-subtab ${settingsSubTab === subTab.id ? "active" : ""}`}
        type="button"
        role="tab"
        id={`settings-subtab-${subTab.id}`}
        aria-selected={settingsSubTab === subTab.id}
        aria-controls={`settings-subpanel-${subTab.id}`}
        tabindex={settingsSubTab === subTab.id ? 0 : -1}
        onclick={() => onSetSubTab(subTab.id)}
        onkeydown={handleTabKeydown}
      >
        {$t(subTab.labelKey)}
      </button>
    {/each}
  </div>
  {#if settingsSubTab === "system"}
    <div class="card reveal delay-34 grid-span-full" id="settings-subpanel-system" role="tabpanel" aria-labelledby="settings-subtab-system">
      <ServerSettings />
    </div>
  {:else if settingsSubTab === "profiles"}
    <div class="card reveal delay-34 grid-span-full" id="settings-subpanel-profiles" role="tabpanel" aria-labelledby="settings-subtab-profiles">
      <ProfileSettings onprofileschanged={onProfilesChanged} />
    </div>
  {:else if settingsSubTab === "scoring"}
    <div class="card reveal delay-34 grid-span-full" id="settings-subpanel-scoring" role="tabpanel" aria-labelledby="settings-subtab-scoring">
      <ScoringSettings />
    </div>
  {/if}
</div>
