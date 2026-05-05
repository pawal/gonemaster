<script>
  import { t } from "../i18n.js";
  import ProfileSettings from "../ProfileSettings.svelte";
  import ServerSettings from "../ServerSettings.svelte";
  import ScoringSettings from "../ScoringSettings.svelte";

  let {
    settingsSubTab = "system",
    settingsSubTabs = [],
    onSetSubTab = () => {},
    onProfilesChanged = () => {},
  } = $props();
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
        onclick={() => onSetSubTab(subTab.id)}
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
