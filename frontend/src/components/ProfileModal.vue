<script setup>
import { ref, computed, nextTick, onMounted, onUnmounted } from 'vue'
import { useProfiles } from '../composables/useProfiles'
import ProfilePicture from './ProfilePicture.vue'

// Every profile action lives here — switch, rename, picture, create, delete —
// reached from the sidebar row and from Settings. Neither of those says more
// than who is playing; the explanations (badges, slugs, the refusal while a
// game runs, the delete warning) belong in one place and this is it.
const {
  profiles, busy, error, switchTo, create, rename, remove, changePicture, removePicture, close,
} = useProfiles()

// ── Creating ────────────────────────────────────────────────────────────────
const creating = ref(false)
const newName = ref('')
const newInput = ref(null)

async function startCreate() {
  deleteMode.value = false
  creating.value = true
  newName.value = ''
  await nextTick()
  newInput.value?.focus()
}

function cancelCreate() {
  creating.value = false
  newName.value = ''
}

async function submitCreate() {
  if (await create(newName.value)) cancelCreate()
}

// ── Renaming ────────────────────────────────────────────────────────────────
// Enter or leaving the field saves; Escape cancels. The blur that follows an
// Escape must not save what was just abandoned, hence the flag. The picture
// links sit under the field; a plain click on them would blur the field
// first, unmounting the link before the click lands. They act on mousedown
// instead, save the name themselves, and leave the rename state — the OS
// picker takes the window's focus anyway.
const renamingSlug = ref(null)
const renameDraft = ref('')
const renameInput = ref(null)
let renameCancelled = false

async function startRename(p) {
  renamingSlug.value = p.slug
  renameDraft.value = p.name
  renameCancelled = false
  await nextTick()
  renameInput.value?.focus()
  renameInput.value?.select()
}

function cancelRename() {
  renameCancelled = true
  renamingSlug.value = null
}

async function finishRename() {
  const slug = renamingSlug.value
  if (!slug || renameCancelled) return
  renamingSlug.value = null
  await rename(slug, renameDraft.value)
}

async function pickPicture(p) {
  await finishRename()
  await changePicture(p.slug)
}

async function clearPicture(p) {
  await finishRename()
  await removePicture(p.slug)
}

// ── Deleting ────────────────────────────────────────────────────────────────
// Two steps, neither of them a click away from the list at rest: the footer
// button puts the list into delete mode, and a row's Delete opens a panel
// whose button stays inert until the toggle is flipped.
const deleteMode = ref(false)
const confirmSlug = ref(null)
const confirmToggle = ref(false)
const confirmTarget = computed(() => profiles.value.find(p => p.slug === confirmSlug.value) ?? null)

function toggleDeleteMode() {
  deleteMode.value = !deleteMode.value
  creating.value = false
  renamingSlug.value = null
}

function askDelete(p) {
  confirmSlug.value = p.slug
  confirmToggle.value = false
}

function cancelDelete() {
  confirmSlug.value = null
  confirmToggle.value = false
}

async function confirmDelete() {
  if (!confirmToggle.value || !confirmTarget.value) return
  if (await remove(confirmTarget.value.slug)) {
    cancelDelete()
    deleteMode.value = false
  } else {
    // The reason is in the footer, under the panel; show the list again.
    cancelDelete()
  }
}

function isDefault(p) {
  return p.slug === 'portforge' && p.createdBy === 'portforge'
}

// Escape closes the modal, unless something is open — then it is that thing's.
function onKey(e) {
  if (e.key !== 'Escape') return
  if (confirmSlug.value) return cancelDelete()
  if (creating.value) return cancelCreate()
  if (renamingSlug.value) return cancelRename()
  if (deleteMode.value) { deleteMode.value = false; return }
  close()
}

onMounted(() => document.addEventListener('keydown', onKey))
onUnmounted(() => document.removeEventListener('keydown', onKey))
</script>

<template>
  <div class="scrim" @click.self="close">
    <div class="modal" role="dialog" aria-modal="true" aria-labelledby="profiles-title">
      <header class="modal-head">
        <div class="modal-title-row">
          <h2 id="profiles-title" class="modal-title">Profiles</h2>
          <span class="spacer" />
          <button class="link-close" @click="close">Close</button>
        </div>
        <p class="modal-desc">
          Saves go to the active profile. Profiles are shared with the other MediaItem
          programs on this machine.
        </p>
      </header>

      <div class="rows">
        <form v-if="creating" class="row row-new" @submit.prevent="submitCreate">
          <input
            ref="newInput"
            v-model="newName"
            class="field"
            type="text"
            placeholder="Profile name"
            :disabled="busy"
            @keydown.esc.stop="cancelCreate"
          />
          <button class="btn-primary" type="submit" :disabled="busy">Create and use</button>
          <button class="btn-outline" type="button" :disabled="busy" @click="cancelCreate">Cancel</button>
        </form>

        <div v-for="p in profiles" :key="p.slug" class="row" :class="{ active: p.active }">
          <button
            class="radio"
            :class="{ on: p.active }"
            :aria-label="p.active ? 'The active profile' : `Use ${p.name}`"
            :disabled="busy"
            @click="switchTo(p.slug)"
          />
          <ProfilePicture :profile="p" :size="36" />

          <div class="row-body">
            <template v-if="renamingSlug !== p.slug">
              <div class="name-line">
                <button class="name" :disabled="busy" @click="switchTo(p.slug)">{{ p.name }}</button>
                <span v-if="p.active" class="pill pill-active">ACTIVE</span>
                <span v-if="isDefault(p)" class="pill pill-default" title="Made by PortForge for saves that have no profile of their own">DEFAULT</span>
              </div>
              <div class="slug mono selectable">{{ p.slug }}</div>
            </template>
            <form v-else class="rename" @submit.prevent="finishRename">
              <input
                ref="renameInput"
                v-model="renameDraft"
                class="field field-rename"
                type="text"
                @keydown.esc.stop="cancelRename"
                @blur="finishRename"
              />
              <div class="picture-links">
                <button class="link-accent" type="button" :disabled="busy" @mousedown.prevent="pickPicture(p)">Change picture…</button>
                <button v-if="p.picture" class="link-dim" type="button" :disabled="busy" @mousedown.prevent="clearPicture(p)">Remove picture</button>
              </div>
            </form>
          </div>

          <template v-if="deleteMode">
            <span v-if="p.active" class="in-use">In use</span>
            <button v-else class="btn-delete" :disabled="busy" @click="askDelete(p)">Delete</button>
          </template>
          <button
            v-else-if="renamingSlug !== p.slug"
            class="btn-rename"
            :disabled="busy"
            @click="startRename(p)"
          >Rename</button>
        </div>
      </div>

      <footer class="modal-foot">
        <p v-if="error" class="foot-error">{{ error }}</p>
        <div class="foot-row">
          <button v-if="!creating" class="btn-outline" :disabled="busy" @click="startCreate">New profile…</button>
          <span class="spacer" />
          <button class="btn-outline btn-delete-mode" :disabled="busy" @click="toggleDeleteMode">
            {{ deleteMode ? 'Done' : 'Delete a profile…' }}
          </button>
        </div>
      </footer>

      <div v-if="confirmTarget" class="confirm">
        <h3 class="confirm-title">Delete {{ confirmTarget.name }}?</h3>
        <p class="confirm-text">
          This deletes the folder <span class="mono confirm-slug">{{ confirmTarget.slug }}</span>
          from this device, for every MediaItem program that uses it — game saves,
          achievements and watch history go with it. It cannot be undone from PortForge.
        </p>
        <p class="confirm-aside">Copies of this profile on your other devices are not touched.</p>
        <button class="confirm-toggle" type="button" @click="confirmToggle = !confirmToggle">
          <span class="switch" :class="{ on: confirmToggle }" role="switch" :aria-checked="confirmToggle" />
          <span>I understand these saves will be gone from this device.</span>
        </button>
        <span class="spacer" />
        <div class="confirm-row">
          <button class="btn-outline" :disabled="busy" @click="cancelDelete">Cancel</button>
          <span class="spacer" />
          <button
            class="btn-destroy"
            :class="{ armed: confirmToggle }"
            :disabled="busy"
            :aria-disabled="!confirmToggle"
            @click="confirmDelete"
          >Delete {{ confirmTarget.name }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<style lang="scss" scoped>
.scrim {
  position: fixed;
  inset: 0;
  z-index: 40;
  display: grid;
  place-items: center;
  padding: 40px;
  background: rgba(10, 10, 10, 0.42);
}

.modal {
  position: relative;
  display: flex;
  flex-direction: column;
  width: 440px;
  max-width: 100%;
  max-height: 100%;
  min-height: 330px;
  background: var(--panel);
  border: 1px solid var(--line2);
  border-radius: 15px;
  box-shadow: var(--shadow);
  overflow: hidden;
}

.spacer { flex: 1; }

/* ── Header ── */
.modal-head {
  padding: 20px 22px 14px;
  border-bottom: 1px solid var(--line);
  flex-shrink: 0;
}

.modal-title-row {
  display: flex;
  align-items: center;
  gap: 12px;
}

.modal-title {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
  letter-spacing: -0.01em;
  color: var(--text);
}

.link-close {
  font-size: 12.5px;
  color: var(--dim2);
  &:hover { color: var(--text); }
}

.modal-desc {
  margin: 7px 0 0;
  font-size: 12.5px;
  line-height: 1.55;
  color: var(--dim2);
  text-wrap: pretty;
}

/* ── Rows ── */
.rows {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
  padding: 8px 10px;
}

.row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 11px 12px;
  border-radius: 11px;

  &.active { background: var(--panel2); }
}

.row-new {
  gap: 9px;
  padding: 10px 12px;
  margin-bottom: 4px;
  background: var(--panel2);
}

.radio {
  width: 15px;
  height: 15px;
  flex: 0 0 auto;
  border-radius: 50%;
  border: 1.5px solid var(--line2);
  display: grid;
  place-items: center;

  &::after {
    content: "";
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: transparent;
  }
  &.on { border-color: var(--accent); }
  &.on::after { background: var(--accent); }
  &:not(.on):not(:disabled):hover { border-color: var(--accent); }
}

.row-body {
  flex: 1;
  min-width: 0;
}

.name-line {
  display: flex;
  align-items: center;
  gap: 7px;
  min-width: 0;
}

.name {
  font-size: 14px;
  font-weight: 500;
  color: var(--text);
  text-align: left;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}

.pill {
  flex: 0 0 auto;
  padding: 2px 7px;
  border-radius: var(--r-pill);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.04em;
}

.pill-active {
  background: var(--accent-soft);
  color: var(--accent-hi);
}

.pill-default {
  background: var(--panel3);
  color: var(--dim);
}

.slug {
  margin-top: 3px;
  font-size: 11.5px;
  color: var(--dim2);
}

.field {
  flex: 1;
  min-width: 0;
  padding: 8px 10px;
  border-radius: 8px;
  border: 1px solid var(--line2);
  background: var(--panel);
  color: var(--text);
  font-size: 13.5px;
  outline: none;

  &::placeholder { color: var(--dim2); }
}

.rename { display: block; }

.field-rename {
  width: 100%;
  padding: 7px 9px;
  border-color: var(--accent);
}

.picture-links {
  display: flex;
  gap: 12px;
  margin-top: 6px;
}

.link-accent,
.link-dim {
  font-size: 11.5px;
  font-weight: 500;
  &:disabled { opacity: 0.5; cursor: default; }
}

.link-accent {
  color: var(--accent);
  &:hover:not(:disabled) { color: var(--accent-hi); }
}

.link-dim {
  color: var(--dim2);
  &:hover:not(:disabled) { color: var(--text); }
}

.btn-rename,
.btn-delete {
  flex: 0 0 auto;
  padding: 6px 12px;
  border-radius: 8px;
  border: 1px solid var(--line2);
  font-size: 12.5px;
  font-weight: 500;
  color: var(--text);

  &:hover:not(:disabled) { border-color: var(--dim2); }
  &:disabled { opacity: 0.5; cursor: default; }
}

.btn-delete {
  border-color: var(--bad);
  color: var(--bad);

  &:hover:not(:disabled) {
    border-color: var(--bad);
    background: var(--bad);
    color: #fff;
  }
}

.in-use {
  flex: 0 0 auto;
  font-size: 12px;
  color: var(--dim2);
}

/* ── Footer ── */
.modal-foot {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 12px 16px 16px;
  border-top: 1px solid var(--line);
  flex-shrink: 0;
}

.foot-error {
  margin: 0;
  font-size: 12.5px;
  color: var(--bad);
  text-wrap: pretty;
}

.foot-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.btn-primary {
  padding: 8px 13px;
  border-radius: 8px;
  background: var(--accent);
  color: var(--on-accent);
  font-size: 12.5px;
  font-weight: 600;
  white-space: nowrap;

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
  white-space: nowrap;

  &:hover:not(:disabled) { border-color: var(--dim2); }
  &:disabled { opacity: 0.5; cursor: default; }
}

.btn-delete-mode {
  color: var(--dim);
  &:hover:not(:disabled) { border-color: var(--bad); color: var(--bad); }
}

.row-new .btn-outline {
  padding: 8px 12px;
  border-radius: 8px;
  font-size: 12.5px;
}

/* ── Delete confirmation ── */
/* Covers the modal body, header and footer included: a warning this serious
   should not be read next to the list it is about. */
.confirm {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 22px;
  background: var(--panel);
  border-radius: 15px;
}

.confirm-title {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
  letter-spacing: -0.01em;
  color: var(--text);
}

.confirm-text {
  margin: 0;
  font-size: 13px;
  line-height: 1.6;
  color: var(--text);
  text-wrap: pretty;
}

.confirm-slug {
  font-size: 12px;
  color: var(--dim);
}

.confirm-aside {
  margin: 0;
  font-size: 12.5px;
  line-height: 1.55;
  color: var(--dim);
  text-wrap: pretty;
}

.confirm-toggle {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 14px;
  border-radius: 11px;
  background: var(--panel2);
  font-size: 12.5px;
  color: var(--text);
  text-align: left;
  text-wrap: pretty;
}

.switch {
  position: relative;
  flex: 0 0 auto;
  width: 38px;
  height: 22px;
  border-radius: var(--r-pill);
  background: var(--panel3);
  transition: var(--t-bg);

  &::after {
    content: "";
    position: absolute;
    top: 3px;
    left: 3px;
    width: 16px;
    height: 16px;
    border-radius: 50%;
    background: #fff;
    box-shadow: 0 1px 2px rgba(0, 0, 0, 0.3);
    transition: left 120ms ease;
  }
  &.on { background: var(--bad); }
  &.on::after { left: 19px; }
}

.confirm-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.btn-destroy {
  padding: 9px 16px;
  border-radius: var(--r-control);
  background: var(--panel3);
  color: var(--dim2);
  font-size: 13px;
  font-weight: 600;
  cursor: not-allowed;

  &.armed {
    background: var(--bad);
    color: #fff;
    cursor: pointer;
  }
  &:disabled { opacity: 0.5; }
}
</style>
