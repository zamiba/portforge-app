<script setup>
import { ref, computed, inject, onMounted } from 'vue'
import { catalogActivityLabel, catalogActivityPercent, catalogActivityRunning } from '../lib/catalog'
import {
  GetSettings, ValidateMediaItemsPath, SetAutoRefreshCatalog,
  GetCatalogInfo, CheckMediaItemsUpdate, SyncMediaItems,
  GetLibraryStorage, RefreshLibraryIndex,
  GetStorageUnits, AddStorageUnit, RemoveStorageUnit, ReorderStorageUnits, OpenStorageUnit,
  GetProfiles, CreateProfile, SetActiveProfile, RenameProfile,
} from '../../wailsjs/go/main/App'

const emit = defineEmits(['saved'])

const props = defineProps({
  /** If true, renders the full-screen first-run setup layout instead of the settings page */
  setup: { type: Boolean, default: false },
})

const saving = ref(false)
const error = ref(null)

const catalog = ref({ sha: '', syncedAt: '', portCount: 0, devMode: false })
const storage = ref({ available: false, totalBytes: 0, freeBytes: 0, libraryBytes: 0 })

// ── Storage units ───────────────────────────────────────────────────────────
// The ordered list of folders finished output is written to, shared with every
// other program in the MediaItem suite. Order is priority: the topmost folder
// with room receives new output.
const units = ref([])
const unitsError = ref(null)
const unitsBusy = ref(false)

async function loadUnits() {
  try {
    units.value = await GetStorageUnits()
    unitsError.value = null
  } catch (e) {
    units.value = []
    unitsError.value = String(e)
  }
}

async function addUnit() {
  unitsBusy.value = true
  try {
    const added = await AddStorageUnit()
    if (added) await loadUnits()
    unitsError.value = null
  } catch (e) {
    unitsError.value = String(e)
  } finally {
    unitsBusy.value = false
  }
}

// Removing forgets the location; nothing stored there is touched. Worth saying
// out loud, because "remove" next to a folder full of games reads as "delete".
async function removeUnit(u) {
  unitsBusy.value = true
  try {
    await RemoveStorageUnit(u.id)
    await loadUnits()
    unitsError.value = null
  } catch (e) {
    unitsError.value = String(e)
  } finally {
    unitsBusy.value = false
  }
}

// Reorder sends every id, so a stale list can't drop or invent a unit.
async function moveUnit(index, delta) {
  const next = index + delta
  if (next < 0 || next >= units.value.length) return
  const ids = units.value.map(u => u.id)
  ;[ids[index], ids[next]] = [ids[next], ids[index]]
  unitsBusy.value = true
  try {
    await ReorderStorageUnits(ids)
    await loadUnits()
    unitsError.value = null
  } catch (e) {
    unitsError.value = String(e)
  } finally {
    unitsBusy.value = false
  }
}

async function openUnit(u) {
  try {
    await OpenStorageUnit(u.id)
    unitsError.value = null
  } catch (e) {
    unitsError.value = String(e)
  }
}

function unitSpace(u) {
  if (u.unreachable) return 'Not connected'
  if (!u.totalBytes) return 'Size unavailable'
  return `${formatBytes(u.freeBytes)} free of ${formatBytes(u.totalBytes)}`
}

const updateAvailable = ref(null)   // null until checked
const checkingUpdate = ref(false)

// ── Catalog activity ────────────────────────────────────────────────────────
// What the catalog is doing comes from App, which hears about every sync and
// rebuild — including the startup refresh and a job started before this page
// was opened — so the buttons here are disabled and the bar shown for all of
// them, not only the one clicked here. `requested` covers the moment between a
// click and the backend's first report.
const catalogActivity = inject('catalogActivity', ref(null))
const requested = ref(null) // 'sync' | 'index' | null
const activeKind = computed(() =>
  catalogActivityRunning(catalogActivity.value) ? catalogActivity.value.kind : requested.value
)
const downloading = computed(() => activeKind.value === 'sync')
const refreshing = computed(() => activeKind.value === 'index')
const downloadPercent = computed(() => catalogActivityPercent(catalogActivity.value))

// ── Profiles ────────────────────────────────────────────────────────────────
// Who the saves belong to. Shared with the other MediaItem programs, like the
// storage list; PortForge only picks which one it writes to. Nobody has to
// make one — a "portforge" profile stands in until they do.
const profiles = ref([])
const profilesError = ref(null)
const profilesBusy = ref(false)
const newProfileName = ref('')
const creatingProfile = ref(false)
// Slug of the profile whose name is being edited, and the draft.
const renamingSlug = ref(null)
const renameDraft = ref('')

const activeProfile = computed(() => profiles.value.find(p => p.active) ?? null)

async function loadProfiles() {
  try {
    profiles.value = await GetProfiles()
    profilesError.value = null
  } catch (e) {
    profilesError.value = String(e)
  }
}

async function switchProfile(slug) {
  if (slug === activeProfile.value?.slug) return
  profilesBusy.value = true
  try {
    await SetActiveProfile(slug)
    await loadProfiles()
  } catch (e) {
    profilesError.value = String(e)
  } finally {
    profilesBusy.value = false
  }
}

async function createProfile() {
  const name = newProfileName.value.trim()
  if (!name) return
  profilesBusy.value = true
  try {
    await CreateProfile(name)
    newProfileName.value = ''
    creatingProfile.value = false
    await loadProfiles()
  } catch (e) {
    profilesError.value = String(e)
  } finally {
    profilesBusy.value = false
  }
}

function startRename(p) {
  renamingSlug.value = p.slug
  renameDraft.value = p.name
}

async function finishRename() {
  const slug = renamingSlug.value
  const name = renameDraft.value.trim()
  renamingSlug.value = null
  if (!slug || !name) return
  try {
    await RenameProfile(slug, name)
    await loadProfiles()
  } catch (e) {
    profilesError.value = String(e)
  }
}

// The setup screen asks once, in plain terms: a profile with a name of the
// user's choosing, or one PortForge names after itself. Blank means the latter.
const setupProfileName = ref('')

// ── Auto-refresh ────────────────────────────────────────────────────────────
// Whether startup checks GitHub for a newer catalog. Written the moment it is
// toggled; there is no separate save on the settings page.
const autoRefresh = ref(true)

async function loadAutoRefresh() {
  try {
    autoRefresh.value = (await GetSettings()).autoRefreshCatalog
  } catch { /* keep the default */ }
}

async function setAutoRefresh(on) {
  const before = autoRefresh.value
  autoRefresh.value = on
  try {
    await SetAutoRefreshCatalog(on)
  } catch (e) {
    autoRefresh.value = before
    error.value = String(e)
  }
}

onMounted(async () => {
  const loads = [loadCatalog(), loadStorage(), loadUnits(), loadAutoRefresh()]
  if (!props.setup) loads.push(loadProfiles())
  await Promise.all(loads)
})

async function loadCatalog() {
  catalog.value = await GetCatalogInfo().catch(() => catalog.value)
}

// Walks the library to size it, so it is only called on mount and after a sync.
async function loadStorage() {
  storage.value = await GetLibraryStorage().catch(() => storage.value)
}

// ── Formatting ──────────────────────────────────────────────────────────────
const UNITS = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']

function formatBytes(n) {
  if (!n || n < 0) return '0 B'
  let v = n, i = 0
  while (v >= 1024 && i < UNITS.length - 1) { v /= 1024; i++ }
  return `${i === 0 || v >= 100 ? Math.round(v) : v.toFixed(1)} ${UNITS[i]}`
}

function formatDate(iso) {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  return d.toLocaleString(undefined, {
    day: 'numeric', month: 'short', year: 'numeric',
    hour: '2-digit', minute: '2-digit',
  })
}

// ── Catalog ─────────────────────────────────────────────────────────────────
const portCountLabel = computed(() => {
  const n = catalog.value.portCount
  return `${n} port${n === 1 ? '' : 's'} available`
})

const catalogLine = computed(() => {
  const { devMode, sha, syncedAt } = catalog.value
  if (devMode) return `Project-local catalog (dev mode) · ${portCountLabel.value}`
  if (!sha) return 'Not synced yet'
  if (sha === 'unknown') return `Present, but not synced by PortForge · ${portCountLabel.value}`
  const when = syncedAt ? formatDate(syncedAt) : ''
  return when
    ? `Last synced ${when} · ${portCountLabel.value}`
    : portCountLabel.value
})

const syncLabel = computed(() => {
  if (!downloading.value) return 'Refresh catalog'
  return catalogActivityLabel(catalogActivity.value) || 'Downloading…'
})

// A sync that fails is reported through the same channel as its progress, so a
// failure the user did not start — the background refresh — is shown here too.
const shownError = computed(() =>
  error.value ?? (catalogActivity.value?.phase === 'failed' ? catalogActivity.value.error : null)
)

// App reloads the library when the backend reports the sync done; this only
// refreshes what the page itself shows.
async function syncCatalog() {
  requested.value = 'sync'
  error.value = null
  try {
    await SyncMediaItems()
    updateAvailable.value = false
    await Promise.all([loadCatalog(), loadStorage(), loadUnits()])
  } catch (e) {
    error.value = String(e)
  } finally {
    requested.value = null
  }
}

async function checkUpdate() {
  checkingUpdate.value = true
  error.value = null
  try {
    updateAvailable.value = await CheckMediaItemsUpdate()
  } catch (e) {
    error.value = String(e)
  } finally {
    checkingUpdate.value = false
  }
}

async function rebuildIndex() {
  requested.value = 'index'
  error.value = null
  try {
    await RefreshLibraryIndex()
  } catch (e) {
    error.value = String(e)
  } finally {
    requested.value = null
  }
}

// The setup screen needs a catalog before it can hand over to the library,
// and a fresh install has none. Syncing is part of getting started rather than
// a separate visit to Settings — which the setup screen cannot reach anyway.
const catalogReady = computed(() => catalog.value.devMode || !!catalog.value.sha)

const startLabel = computed(() => {
  if (downloading.value) return syncLabel.value
  if (saving.value) return 'Starting…'
  return catalogReady.value ? 'Get Started' : 'Download the catalog and get started'
})

// save finishes first-run setup. There is no path to persist any more — the
// storage location was already written to the shared list when it was added —
// so this fetches the catalog if there is none yet and confirms it is usable
// before leaving the setup screen.
async function save() {
  if (!units.value.length) return
  saving.value = true
  error.value = null
  try {
    if (!catalogReady.value) {
      await syncCatalog()
      if (error.value) return
    }
    const warning = await ValidateMediaItemsPath()
    if (warning) {
      error.value = warning
      return
    }
    // A name means a profile of their own from the start; otherwise the
    // default appears by itself the first time something needs it.
    if (setupProfileName.value.trim()) {
      await CreateProfile(setupProfileName.value.trim())
    }
    emit('saved')
  } catch (e) {
    error.value = String(e)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <!-- ── First run ───────────────────────────────────────────────────────── -->
  <div v-if="props.setup" class="setup-screen">
    <div class="setup-content">
      <h1 class="setup-title">Welcome to PortForge</h1>
      <p class="setup-subtitle">
        Choose where PortForge should keep what it builds and imports. This folder is
        shared with the other MediaItem programs on this machine, and you can add more
        later. PortForge then downloads the port catalog, which is small and takes a
        moment.
      </p>

      <div class="setup-units">
        <div v-for="u in units" :key="u.id" class="setup-unit">
          <span class="setup-unit-name">{{ u.name }}</span>
          <span class="setup-unit-path mono selectable">{{ u.path }}</span>
        </div>
        <button class="btn-outline" :disabled="unitsBusy" @click="addUnit">
          {{ units.length ? 'Add another folder…' : 'Choose a folder…' }}
        </button>
      </div>

      <label class="toggle-row">
        <input type="checkbox" :checked="autoRefresh" @change="setAutoRefresh($event.target.checked)" />
        <span>Check for a newer catalog whenever PortForge starts</span>
      </label>

      <div class="setup-profile">
        <label class="setup-profile-label" for="setup-profile-name">Profile</label>
        <p class="setup-profile-hint">
          Saves are kept under a profile, so more than one person can play on this
          machine and a profile can be moved to another. Name yours, or leave this
          blank and PortForge keeps everything under a profile of its own that you can
          rename or replace later.
        </p>
        <input
          id="setup-profile-name"
          v-model="setupProfileName"
          class="text-input"
          type="text"
          placeholder="Your name (optional)"
          :disabled="saving"
        />
      </div>

      <p v-if="unitsError" class="error-text">{{ unitsError }}</p>
      <p v-if="shownError" class="error-text">{{ shownError }}</p>

      <button class="btn-primary" :disabled="!units.length || saving || downloading" @click="save">
        {{ startLabel }}
      </button>
      <div v-if="downloading" class="card-progress setup-progress">
        <div class="card-progress-fill" :style="{ width: downloadPercent + '%' }" />
      </div>
    </div>
  </div>

  <!-- ── Settings ────────────────────────────────────────────────────────── -->
  <div v-else class="settings">
    <h1 class="page-title settings-title">Settings</h1>

    <section class="card catalog-card">
      <div class="card-text">
        <span class="card-name">Port catalog</span>
        <span class="card-meta">{{ catalogLine }}</span>
      </div>

      <div class="card-actions">
        <span v-if="catalog.sha && catalog.sha !== 'unknown'" class="sha mono">{{ catalog.sha }}</span>
        <span v-if="updateAvailable === true" class="badge-update">Update available</span>
        <span v-else-if="updateAvailable === false && !checkingUpdate" class="badge-quiet">Up to date</span>

        <button
          v-if="!catalog.devMode && catalog.sha && catalog.sha !== 'unknown'"
          class="btn-outline"
          :disabled="checkingUpdate || downloading"
          @click="checkUpdate"
        >{{ checkingUpdate ? 'Checking…' : 'Check for updates' }}</button>

        <button
          class="btn-primary"
          :disabled="downloading || catalog.devMode"
          :title="catalog.devMode ? 'Syncing is disabled in dev mode' : undefined"
          @click="syncCatalog"
        >{{ syncLabel }}</button>
      </div>

      <div v-if="downloading" class="card-progress">
        <div class="card-progress-fill" :style="{ width: downloadPercent + '%' }" />
      </div>

      <label class="toggle-row card-toggle" :class="{ disabled: catalog.devMode }">
        <input type="checkbox" :checked="autoRefresh" :disabled="catalog.devMode" @change="setAutoRefresh($event.target.checked)" />
        <span>Check for a newer catalog whenever PortForge starts, and fetch it in the background</span>
      </label>
    </section>

    <header class="section-head">
      <span class="section-name">Profile</span>
      <span class="spacer" />
      <button v-if="!creatingProfile" class="btn-outline" :disabled="profilesBusy" @click="creatingProfile = true">New profile…</button>
    </header>
    <p class="section-desc">
      Saves and settings go to the active profile. Profiles are folders shared with the
      other MediaItem programs on this machine, so what they record about a person —
      achievements, what they have watched — sits beside the saves, and a whole profile
      can be copied or synced to another device however you like. Switching takes effect
      on the next launch.
    </p>

    <section class="card profile-list">
      <form v-if="creatingProfile" class="profile-new" @submit.prevent="createProfile">
        <input
          v-model="newProfileName"
          class="text-input"
          type="text"
          placeholder="Profile name"
          autofocus
          :disabled="profilesBusy"
        />
        <button class="btn-primary btn-small" type="submit" :disabled="profilesBusy || !newProfileName.trim()">Create and use</button>
        <button class="btn-outline btn-small" type="button" :disabled="profilesBusy" @click="creatingProfile = false; newProfileName = ''">Cancel</button>
      </form>

      <div v-for="p in profiles" :key="p.slug" class="profile" :class="{ active: p.active }">
        <button
          class="profile-pick"
          :title="p.active ? 'The active profile' : 'Use this profile'"
          :disabled="profilesBusy || p.active"
          @click="switchProfile(p.slug)"
        >
          <span class="profile-dot" />
        </button>
        <div class="profile-body">
          <form v-if="renamingSlug === p.slug" class="profile-rename" @submit.prevent="finishRename">
            <input
              v-model="renameDraft"
              class="text-input"
              type="text"
              autofocus
              @keydown.esc="renamingSlug = null"
              @blur="finishRename"
            />
          </form>
          <div v-else class="profile-name">
            {{ p.name }}
            <span v-if="p.active" class="unit-badge">active</span>
            <span v-if="p.createdBy && p.slug === 'portforge'" class="unit-badge" title="Made by PortForge for saves that have no profile of their own">default</span>
          </div>
          <div class="profile-slug mono selectable">{{ p.slug }}</div>
        </div>
        <div class="unit-actions">
          <button class="btn-outline btn-small" :disabled="profilesBusy" @click="startRename(p)">Rename</button>
        </div>
      </div>

      <p v-if="profilesError" class="error-text">{{ profilesError }}</p>
    </section>

    <header class="section-head">
      <span class="section-name">Storage locations</span>
      <span class="spacer" />
      <button class="btn-outline" :disabled="unitsBusy" @click="addUnit">Add folder…</button>
    </header>
    <p class="section-desc">
      Shared with the other MediaItem programs on this machine, so a folder added here
      is a folder they all know about. Order is priority — new output goes to the
      topmost folder with room. Removing one forgets the location; nothing stored
      there is touched.
    </p>

    <section class="card unit-list">
      <div v-if="!units.length" class="unit-empty">
        <p>No storage locations yet.</p>
        <p class="unit-empty-hint">
          Choose where finished builds and imported files should be kept. Nothing is
          added for you — the folders here are only ever ones you pick.
        </p>
      </div>
      <div v-for="(u, i) in units" :key="u.id" class="unit" :class="{ offline: u.unreachable }">
        <div class="unit-rank mono">{{ i + 1 }}</div>
        <div class="unit-body">
          <div class="unit-name">
            {{ u.name }}
            <span v-if="u.unreachable" class="unit-badge">not connected</span>
          </div>
          <div class="unit-path mono selectable">{{ u.path }}</div>
        </div>
        <div class="unit-space">{{ unitSpace(u) }}</div>
        <div class="unit-actions">
          <button
            class="btn-icon"
            title="Move up"
            :disabled="i === 0 || unitsBusy"
            @click="moveUnit(i, -1)"
          >&uarr;</button>
          <button
            class="btn-icon"
            title="Move down"
            :disabled="i === units.length - 1 || unitsBusy"
            @click="moveUnit(i, 1)"
          >&darr;</button>
          <button
            class="btn-outline btn-small"
            :disabled="u.unreachable"
            :title="u.unreachable ? 'This folder isn\'t connected right now' : 'Show this folder in your file manager'"
            @click="openUnit(u)"
          >Open folder</button>
          <button class="btn-outline btn-small btn-danger" :disabled="unitsBusy" @click="removeUnit(u)">
            Remove
          </button>
        </div>
      </div>

      <div v-if="units.length && storage.available" class="unit-usage">
        <span class="legend-item">
          <i class="swatch swatch-library" />PortForge · {{ formatBytes(storage.libraryBytes) }}
        </span>
        <span class="unit-usage-in">in {{ units[0].name }}</span>
      </div>
    </section>

    <p v-if="unitsError" class="error-text">{{ unitsError }}</p>

    <header class="section-head">
      <span class="section-name">Maintenance</span>
    </header>
    <p class="section-desc">
      Rebuilds the search index from the catalog already on disk. Needed only after
      editing catalog files by hand — a sync does it for you.
    </p>

    <section class="card maintenance-card">
      <div class="card-text">
        <span class="card-name">Library index</span>
        <span class="card-meta">Rescans installed ports and matched ROMs.</span>
      </div>
      <div class="card-actions">
        <button class="btn-outline" :disabled="refreshing" @click="rebuildIndex">
          {{ refreshing ? 'Rebuilding…' : 'Rebuild index' }}
        </button>
      </div>
    </section>

    <p v-if="shownError" class="error-text">{{ shownError }}</p>
  </div>
</template>

<style lang="scss" scoped>
/* ── Page ─────────────────────────────────────────────────────────────────── */
.settings {
  max-width: 780px;
  padding: 18px var(--pad-page) 44px;
}

.settings-title {
  margin-bottom: 26px;
}

.section-head {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 26px;
}

.section-name {
  font-size: 14px;
  font-weight: 500;
  color: var(--text);
}

.spacer {
  flex: 1;
}

.section-desc {
  max-width: 60ch;
  margin: 6px 0 12px;
  font-size: 12.5px;
  color: var(--dim2);
}

/* ── Cards ────────────────────────────────────────────────────────────────── */
.card {
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: var(--r-panel);
  padding: 18px 20px;
}

.catalog-card,
.maintenance-card {
  display: flex;
  align-items: center;
  gap: 20px;
  flex-wrap: wrap;
}

.card-text {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.card-name {
  font-size: 14px;
  font-weight: 500;
  color: var(--text);
}

.card-meta {
  font-size: 12.5px;
  color: var(--dim2);
}

.card-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

/* Spans the card once it wraps below the buttons. */
.card-progress {
  flex-basis: 100%;
  height: 4px;
  border-radius: 4px;
  background: var(--panel2);
  overflow: hidden;
}

.card-progress-fill {
  height: 100%;
  background: var(--accent);
  transition: width 200ms ease;
}

.toggle-row {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 12.5px;
  color: var(--text);
  cursor: pointer;

  input { accent-color: var(--accent); }
  &.disabled { opacity: 0.5; cursor: default; }
}

/* Spans the card below the buttons, like the progress bar. */
.card-toggle {
  flex-basis: 100%;
  margin-top: 4px;
}

/* ── Profiles ── */
.setup-profile {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-top: 6px;
}

.setup-profile-label {
  font-size: 12px;
  font-weight: 500;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--dim);
}

.setup-profile-hint {
  margin: 0;
  font-size: 12.5px;
  line-height: 1.5;
  color: var(--dim);
  text-wrap: pretty;
}

.text-input {
  padding: 7px 11px;
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: var(--r-control);
  color: var(--text);
  font-size: 13.5px;
  outline: none;
  min-width: 0;

  &:focus { border-color: var(--line2); }
  &::placeholder { color: var(--dim2); }
}

.profile-list {
  display: flex;
  flex-direction: column;
  padding: 6px 20px;
}

.profile-new {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 0 12px;
  border-bottom: 1px solid var(--line);

  .text-input { flex: 1; }
}

.profile {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 12px 0;
  border-bottom: 1px solid var(--line);

  &:last-of-type { border-bottom: none; }
}

.profile-pick {
  width: 22px;
  height: 22px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  flex-shrink: 0;

  &:not(:disabled):hover .profile-dot { border-color: var(--accent); }
  &:disabled { cursor: default; }
}

.profile-dot {
  width: 12px;
  height: 12px;
  border-radius: 50%;
  border: 2px solid var(--line2);
  transition: border-color 120ms ease, background 120ms ease;

  .active & {
    border-color: var(--accent);
    background: var(--accent);
  }
}

.profile-body {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.profile-name {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  font-weight: 500;
  color: var(--text);
}

.profile-slug {
  font-size: 11.5px;
  color: var(--dim2);
}

.profile-rename .text-input {
  width: 260px;
  padding: 4px 8px;
}

.setup-progress {
  margin-top: 14px;
}

/* ── Library folder ───────────────────────────────────────────────────────── */
/* ── Storage locations ── */
.unit-list { padding: 0; overflow: hidden; }

.unit-empty {
  padding: 16px;
  font-size: 12px;
  color: var(--text);

  p { margin: 0; }
}

.unit-empty-hint {
  margin-top: 4px !important;
  color: var(--dim);
  line-height: 1.5;
}

.unit {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 12px 16px;
  border-bottom: 1px solid var(--line);

  &:last-child { border-bottom: 0; }
  &.offline { opacity: 0.62; }
}

.unit-rank {
  width: 20px;
  font-size: 11px;
  color: var(--dim);
  text-align: right;
}

.unit-body { flex: 1; min-width: 0; }

.unit-name {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: var(--text);
}

.unit-badge {
  font-size: 10px;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--dim);
  border: 1px solid var(--line);
  border-radius: 3px;
  padding: 1px 5px;
}

.unit-path {
  font-size: 11px;
  color: var(--dim);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.unit-space {
  font-size: 11.5px;
  color: var(--dim);
  white-space: nowrap;
}

.unit-usage {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 16px;
  border-top: 1px solid var(--line);
  font-size: 11.5px;
  color: var(--dim);
}

.unit-usage-in { color: var(--dim); }

.setup-units {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: 10px;
  margin: 18px 0;
}

.setup-unit {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 10px 14px;
  border: 1px solid var(--line);
  border-radius: var(--r-panel);
  background: var(--panel);
  text-align: left;
}

.setup-unit-name { font-size: 13px; color: var(--text); }
.setup-unit-path { font-size: 11px; color: var(--dim); word-break: break-all; }

.unit-actions {
  display: flex;
  align-items: center;
  gap: 6px;
}

.btn-icon {
  width: 24px;
  height: 24px;
  border: 1px solid var(--line);
  border-radius: 4px;
  font-size: 12px;
  line-height: 1;
  color: var(--dim);

  &:hover:not(:disabled) { background: var(--panel2); color: var(--text); }
  &:disabled { opacity: 0.35; }
}

.btn-small { padding: 4px 10px; font-size: 11.5px; }

/* btn-danger is defined in GameDetail's scoped block, so it does not reach here.
   Same treatment, stated once more rather than hoisted — the two screens are the
   only users, and a shared button system is a bigger change than this warrants. */
.btn-outline.btn-danger {
  color: var(--bad);
  &:hover:not(:disabled) { border-color: var(--bad); }
}

.folder-card {
  padding: 16px 18px;
}

.folder-head {
  display: flex;
  align-items: center;
  gap: 12px;
}

.folder-path {
  font-size: 12.5px;
  color: var(--text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.storage {
  display: flex;
  align-items: center;
  gap: 14px;
  margin-top: 14px;
}

/* Two stacked segments over a --panel2 track; the remainder is free space. */
.storage-bar {
  flex: 1;
  display: flex;
  height: 6px;
  border-radius: 6px;
  background: var(--panel2);
  overflow: hidden;
}

.seg {
  height: 100%;
}

.seg-other {
  background: var(--dim2);
}

.seg-library {
  background: var(--accent);
}

.storage-free {
  font-size: 12.5px;
  color: var(--dim);
  white-space: nowrap;
}

.legend {
  display: flex;
  flex-wrap: wrap;
  gap: 16px;
  margin-top: 9px;
  font-size: 11.5px;
  color: var(--dim2);
}

.legend-item {
  display: flex;
  align-items: center;
  gap: 6px;
}

.swatch {
  width: 8px;
  height: 8px;
  border-radius: 2px;
  flex-shrink: 0;
}

.swatch-library { background: var(--accent); }
.swatch-other { background: var(--dim2); }

.storage-none {
  margin: 12px 0 0;
  font-size: 12.5px;
  color: var(--dim2);
}

/* ── Badges ───────────────────────────────────────────────────────────────── */
.sha {
  padding: 2px 8px;
  border-radius: var(--r-tile);
  border: 1px solid var(--line);
  background: var(--panel2);
  font-size: 11.5px;
  color: var(--dim);
}

.badge-update {
  padding: 3px 9px;
  border-radius: var(--r-pill);
  background: var(--accent-soft);
  color: var(--accent-hi);
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.03em;
}

.badge-quiet {
  font-size: 12.5px;
  color: var(--dim2);
}

/* ── Buttons ──────────────────────────────────────────────────────────────── */
.btn-primary {
  padding: 9px 16px;
  border-radius: var(--r-control);
  background: var(--accent);
  color: var(--on-accent);
  font-size: 13px;
  font-weight: 600;

  &:hover:not(:disabled) { background: var(--accent-hi); }
  &:disabled { opacity: 0.5; cursor: default; }
}

.btn-outline {
  padding: 8px 14px;
  border-radius: var(--r-control);
  border: 1px solid var(--line2);
  font-size: 13px;
  font-weight: 500;
  color: var(--text);

  &:hover:not(:disabled) { border-color: var(--dim2); }
  &:disabled { opacity: 0.5; cursor: default; }
}

.error-text {
  margin-top: 16px;
  font-size: 13px;
  color: var(--bad);
}

/* ── First run ────────────────────────────────────────────────────────────── */
.setup-screen {
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--pad-page);
}

.setup-content {
  width: 100%;
  max-width: 520px;
}

.setup-title {
  margin: 0;
  font-size: 26px;
  font-weight: 600;
  letter-spacing: -0.02em;
}

.setup-subtitle {
  margin: 10px 0 26px;
  font-size: 14px;
  line-height: 1.65;
  color: var(--dim);
}

.path-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 10px 8px 14px;
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: var(--r-control);

  &:focus-within { border-color: var(--line2); }
}

.path-input {
  flex: 1;
  min-width: 0;
  border: none;
  outline: none;
  background: none;
  font-size: 12.5px;
  color: var(--text);

  &::placeholder { color: var(--dim2); font-family: var(--font-ui); }
}

.setup-content .btn-primary {
  margin-top: 22px;
  padding: 11px 26px;
  font-size: 14px;
}
</style>
