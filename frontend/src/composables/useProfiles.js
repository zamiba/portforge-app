import { ref, computed, readonly } from 'vue'
import {
  GetProfiles, CreateProfile, SetActiveProfile, RenameProfile, DeleteProfile,
  SetProfilePicture, RemoveProfilePicture,
} from '../../wailsjs/go/main/App'

// The active profile is app-wide state: the sidebar names it on every screen,
// Settings summarises it, and one modal — reachable from both — is where it
// changes. Module-level refs so those three share it without a prop chain.
const profiles = ref([])
const busy = ref(false)
const modalOpen = ref(false)

// The modal's footer line: the refusal while a game runs, or why a create,
// rename, delete or picture failed. A switch that succeeds needs no line —
// the dot, the pill and the sidebar all move at once, since the switch takes
// effect the moment it is made.
const error = ref(null)

const active = computed(() => profiles.value.find(p => p.active) ?? null)

async function load() {
  try {
    profiles.value = await GetProfiles()
    error.value = null
  } catch (e) {
    error.value = sentence(e)
  }
}

// Backend errors are lower-case fragments; the footer shows them on their own.
function sentence(e) {
  const s = String(e?.message ?? e)
  return s.charAt(0).toUpperCase() + s.slice(1) + (/[.!?]$/.test(s) ? '' : '.')
}

async function run(fn) {
  busy.value = true
  try {
    await fn()
    return true
  } catch (e) {
    error.value = sentence(e)
    return false
  } finally {
    busy.value = false
  }
}

// Reloads after a change that went through, and clears the line: whatever
// it said has been acted on.
async function refreshed() {
  await load()
  error.value = null
}

async function switchTo(slug) {
  const target = profiles.value.find(p => p.slug === slug)
  if (!target || target.active) return
  if (await run(() => SetActiveProfile(slug))) await refreshed()
}

async function create(name) {
  name = name.trim()
  if (!name) {
    error.value = 'Give the profile a name.'
    return false
  }
  if (!await run(() => CreateProfile(name))) return false
  await refreshed()
  return true
}

async function rename(slug, name) {
  name = name.trim()
  const current = profiles.value.find(p => p.slug === slug)
  if (!name || !current || current.name === name) return
  if (await run(() => RenameProfile(slug, name))) await refreshed()
}

async function remove(slug) {
  if (!await run(() => DeleteProfile(slug))) return false
  await refreshed()
  return true
}

// The backend opens the OS picker; a dismissed picker is not an error and
// changes nothing.
async function changePicture(slug) {
  if (await run(() => SetProfilePicture(slug))) await refreshed()
}

async function removePicture(slug) {
  if (await run(() => RemoveProfilePicture(slug))) await refreshed()
}

function open() {
  error.value = null
  modalOpen.value = true
  load()
}

function close() {
  modalOpen.value = false
  error.value = null
}

export function useProfiles() {
  return {
    profiles: readonly(profiles), active, busy: readonly(busy),
    modalOpen: readonly(modalOpen), error: readonly(error),
    load, switchTo, create, rename, remove, changePicture, removePicture, open, close,
  }
}
