<script setup>
// The Config tab: edits named settings inside a game's own configuration files.
//
// PortForge does not own these files — the game does, and writes far more to them
// than this page shows. So every edit is a change to one named setting, batched
// and written by the Go side, which splices the value in place and leaves every
// other byte as the game wrote it. Nothing here regenerates a file.
//
// Edits are held until Save, and they persist across files: the save bar counts
// changes from every file at once, so moving between files in the rail is not
// leaving the page.
import { ref, computed, watch, onUnmounted } from 'vue'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { GetGameConfig, SaveGameConfig, ChooseConfigPath } from '../../wailsjs/go/main/App'

const props = defineProps({
  game: { type: Object, required: true },
})

const cfg = ref(null)
const loading = ref(false)
const fileIndex = ref(0)
const query = ref('')

// Edits, keyed "<file path>|<pointer>". A value equal to what the file holds is
// removed rather than kept, so setting a toggle back is not a change to save.
const edits = ref({})
const saved = ref({})          // the values as last read from disk
const changedOnDisk = ref({})  // keys the game moved while this page was open
const failedFile = ref('')
const savedNote = ref('')
const leaveCb = ref(null)

const key = (file, pointer) => `${file}|${pointer}`

async function load() {
  loading.value = true
  try {
    const next = await GetGameConfig(props.game._itemTitle)
    const values = {}
    for (const f of next.files ?? []) {
      for (const sec of f.sections ?? []) {
        for (const fl of sec.fields ?? []) {
          if (fl.editable) values[key(f.path, fl.pointer)] = fl.value
        }
      }
    }
    // A value that moved since the last read was moved by the game, not by us.
    const moved = {}
    if (cfg.value) {
      for (const k of Object.keys(values)) {
        if (k in saved.value && String(saved.value[k]) !== String(values[k])) moved[k] = true
      }
    }
    changedOnDisk.value = moved
    saved.value = values
    cfg.value = next
    // An edit outlives a re-read. A setting the person changed here and the game
    // changed on disk keeps the person's value and shows as both dirty and moved;
    // theirs is what a Save writes.
    const live = {}
    for (const [k, v] of Object.entries(edits.value)) {
      if (k in values && String(v) !== String(values[k])) live[k] = v
    }
    edits.value = live
    if (fileIndex.value >= (next.files?.length ?? 0)) fileIndex.value = 0
  } catch (err) {
    console.error('config:', err)
  } finally {
    loading.value = false
  }
}

watch(() => props.game._itemTitle, () => {
  cfg.value = null
  edits.value = {}
  saved.value = {}
  changedOnDisk.value = {}
  failedFile.value = ''
  savedNote.value = ''
  fileIndex.value = 0
  query.value = ''
  load()
}, { immediate: true })

// A session starting locks the page; a session ending unlocks it and is also the
// moment the files are most likely to have moved, since these programs write their
// config on exit. Re-reading then is what makes "changed by the game" reachable at
// all: edits are kept, so a value the person changed is not overwritten by the
// re-read — it stays theirs and wins when saved.
const stopStarted = EventsOn('game:started', title => { if (title === props.game._itemTitle) load() })
const stopEnded = EventsOn('game:ended', title => { if (title === props.game._itemTitle) load() })
onUnmounted(() => { stopStarted?.(); stopEnded?.() })

const files = computed(() => cfg.value?.files ?? [])
const profile = computed(() => cfg.value?.profile || 'your profile')
const locked = computed(() => cfg.value?.state === 'running')
const neverLaunched = computed(() => cfg.value?.state === 'neverLaunched')

// ── values and validity ─────────────────────────────────────────────────────
const current = (file, fl) => {
  const k = key(file, fl.pointer)
  return k in edits.value ? edits.value[k] : saved.value[k]
}

function setValue(file, fl, value) {
  const k = key(file, fl.pointer)
  const next = { ...edits.value }
  if (String(value) === String(saved.value[k])) delete next[k]
  else next[k] = value
  edits.value = next
  savedNote.value = ''
}

// A setting can be put back to the value the game itself uses. Offered per field
// rather than as a page-wide "restore defaults", so nothing is reset that the
// person did not point at.
const hasDefault = fl => fl.default !== undefined && fl.default !== null
const differsFromDefault = (file, fl) =>
  fl.editable && hasDefault(fl) && String(current(file, fl)) !== String(fl.default)

function resetToDefault(file, fl) {
  setValue(file, fl, fl.default)
}

// A number is checked on every keystroke so the engine is never handed a value
// it would refuse: a rejected save is a worse way to learn about a typo.
function invalidReason(file, fl) {
  if (fl.widget !== 'number' && fl.widget !== 'slider') return ''
  const raw = current(file, fl)
  if (raw === '' || raw === null || raw === undefined) return 'Enter a value.'
  const n = Number(raw)
  if (!Number.isFinite(n)) return 'Enter a number.'
  if (fl.kind === 'int' && !Number.isInteger(n)) return 'Enter a whole number.'
  const lo = fl.min, hi = fl.max
  if (lo !== undefined && hi !== undefined && (n < lo || n > hi)) {
    return fl.kind === 'int'
      ? `Enter a whole number from ${lo} to ${hi}.`
      : `Enter a number from ${lo} to ${hi}.`
  }
  if (lo !== undefined && n < lo) return `Enter a number of at least ${lo}.`
  if (hi !== undefined && n > hi) return `Enter a number of at most ${hi}.`
  return ''
}

const fieldIndex = computed(() => {
  const out = {}
  for (const f of files.value) {
    for (const sec of f.sections ?? []) {
      for (const fl of sec.fields ?? []) out[key(f.path, fl.pointer)] = { file: f, field: fl }
    }
  }
  return out
})

const dirtyKeys = computed(() => Object.keys(edits.value))
const dirtyCount = computed(() => dirtyKeys.value.length)
const isDirty = (file, fl) => key(file, fl.pointer) in edits.value

const anyInvalid = computed(() => dirtyKeys.value.some(k => {
  const at = fieldIndex.value[k]
  return at && invalidReason(at.file.path, at.field)
}))

const dirtyFileTitles = computed(() => {
  const titles = []
  for (const k of dirtyKeys.value) {
    const at = fieldIndex.value[k]
    if (at && !titles.includes(at.file.title)) titles.push(at.file.title)
  }
  return titles
})

const dirtyPerFile = computed(() => {
  const counts = {}
  for (const k of dirtyKeys.value) {
    const at = fieldIndex.value[k]
    if (at) counts[at.file.path] = (counts[at.file.path] ?? 0) + 1
  }
  return counts
})

// ── the rail and the filter ─────────────────────────────────────────────────
const filtering = computed(() => query.value.trim().length > 0)

// Filtering searches every file, because a person looking for "volume" does not
// know which file it is in. Results keep their file and section so the answer is
// still locatable afterwards.
const groups = computed(() => {
  if (!filtering.value) {
    const f = files.value[fileIndex.value]
    if (!f) return []
    return (f.sections ?? []).map(sec => ({
      heading: '', title: sec.title, help: sec.help, file: f, fields: sec.fields ?? [],
    }))
  }
  const q = query.value.trim().toLowerCase()
  const out = []
  for (const f of files.value) {
    for (const sec of f.sections ?? []) {
      const hits = (sec.fields ?? []).filter(fl =>
        fl.label.toLowerCase().includes(q) || (fl.help ?? '').toLowerCase().includes(q))
      if (hits.length) {
        out.push({ heading: `${f.title} · ${sec.title}`, title: '', help: '', file: f, fields: hits })
      }
    }
  }
  return out
})

const noResults = computed(() => filtering.value && groups.value.length === 0)

// ── banners ────────────────────────────────────────────────────────────────
const banner = computed(() => {
  const name = props.game.title || props.game._itemTitle
  if (locked.value) {
    return {
      tone: 'warn',
      title: `${name} is running`,
      body: 'The game rewrites these files while it runs, so editing is paused. PortForge reads them again when you quit.',
    }
  }
  if (neverLaunched.value) {
    return {
      tone: 'warn',
      title: `${name} hasn't written its settings yet`,
      body: 'The game creates these files the first time it runs. Launch it once and quit, and every setting below becomes editable.',
    }
  }
  if (failedFile.value) {
    return {
      tone: 'bad',
      title: `Couldn't save to ${failedFile.value}`,
      body: `The write didn't go through, so nothing in the file changed and the game will use its old values. Your ${dirtyCount.value} change${dirtyCount.value === 1 ? '' : 's'} ${dirtyCount.value === 1 ? 'is' : 'are'} still here.`,
      action: 'Try again',
      run: save,
    }
  }
  const moved = Object.keys(changedOnDisk.value).length
  if (moved) {
    return {
      tone: 'accent',
      title: 'Settings changed outside PortForge',
      body: `${name} was run since this page opened, so PortForge read its files again. ${moved} value${moved === 1 ? '' : 's'} changed and ${moved === 1 ? 'is' : 'are'} marked below.`,
      action: 'Dismiss',
      run: () => { changedOnDisk.value = {} },
    }
  }
  return null
})

// ── saving ─────────────────────────────────────────────────────────────────
const saving = ref(false)

async function save() {
  if (!dirtyCount.value || anyInvalid.value || locked.value || saving.value) return false
  const changes = dirtyKeys.value.map(k => {
    const { file, field } = fieldIndex.value[k]
    let value = edits.value[k]
    if (field.kind === 'int') value = Math.trunc(Number(value))
    else if (field.kind === 'float') value = Number(value)
    return { file: file.path, pointer: field.pointer, value }
  })
  saving.value = true
  try {
    await SaveGameConfig(props.game._itemTitle, changes)
    edits.value = {}
    failedFile.value = ''
    await load()
    savedNote.value = `Saved to ${profile.value}'s profile`
    return true
  } catch (err) {
    // The message names the file it could not write; the edits stay so nothing
    // the person typed is lost to a failed disk.
    const text = String(err)
    const at = text.indexOf(':')
    failedFile.value = at > 0 ? text.slice(0, at) : 'the file'
    console.error('config save:', err)
    return false
  } finally {
    saving.value = false
  }
}

function discard() {
  edits.value = {}
  failedFile.value = ''
}

// requestLeave lets the game page route a tab change or a navigation through the
// unsaved-changes question. Switching files inside this tab is not leaving, so it
// never calls this.
function requestLeave(go) {
  if (!dirtyCount.value) {
    go()
    return
  }
  leaveCb.value = go
}

function leaveKeepEditing() { leaveCb.value = null }

function leaveDiscard() {
  const go = leaveCb.value
  leaveCb.value = null
  discard()
  go?.()
}

async function leaveSave() {
  const go = leaveCb.value
  if (await save()) {
    leaveCb.value = null
    go?.()
  } else {
    leaveCb.value = null
  }
}

defineExpose({ requestLeave, dirtyCount })

async function choosePath(file, fl) {
  try {
    const picked = await ChooseConfigPath(false, String(current(file, fl) ?? ''))
    if (picked) setValue(file, fl, picked)
  } catch (err) {
    console.error('config path:', err)
  }
}

const unavailableCopy = reason => reason === 'kindChanged'
  ? 'A newer release of the game stores this differently, so PortForge leaves it alone.'
  : 'Not in the file yet. The game writes this setting the first time it uses it.'
</script>

<template>
  <section v-if="files.length" class="cfg">
    <header class="cfg-head">
      <h2 class="cfg-title">Game settings for {{ profile }}</h2>
      <p class="cfg-sub">
        Read from {{ profile }}'s profile. PortForge edits only the settings listed here and
        leaves the rest of each file exactly as the game wrote it.
      </p>
      <p v-if="savedNote && !dirtyCount" class="cfg-saved"><span class="cfg-dot-saved" />{{ savedNote }}</p>
    </header>

    <div v-if="banner" class="alert cfg-banner" :class="`cfg-banner-${banner.tone}`">
      <span class="cfg-banner-dot" :class="`cfg-dot-${banner.tone}`" aria-hidden="true" />
      <div class="alert-body">
        <p class="alert-title">{{ banner.title }}</p>
        <p class="alert-text">{{ banner.body }}</p>
      </div>
      <button v-if="banner.action" class="chip cfg-banner-action" @click="banner.run">{{ banner.action }}</button>
    </div>

    <div class="cfg-grid">
      <!-- The rail stays even for a single file, so the filename is always on
           screen: knowing which file a setting lives in is part of trusting the
           page. -->
      <aside class="cfg-rail">
        <input v-model="query" class="cfg-filter" type="text" placeholder="Filter settings" />
        <button
          v-for="(f, i) in files"
          :key="f.path"
          class="cfg-rail-row"
          :class="{ active: !filtering && i === fileIndex }"
          @click="fileIndex = i; query = ''"
        >
          <span class="cfg-rail-title">{{ f.title }}</span>
          <span class="cfg-rail-path mono">{{ f.path.split('/').pop() }}</span>
          <span v-if="dirtyPerFile[f.path]" class="cfg-rail-badge">{{ dirtyPerFile[f.path] }}</span>
        </button>
      </aside>

      <div class="cfg-content">
        <p v-if="noResults" class="cfg-empty">Nothing matches “{{ query }}”.</p>

        <section v-for="(g, gi) in groups" :key="gi" class="cfg-section">
          <h3 v-if="g.heading" class="eyebrow cfg-group">{{ g.heading }}</h3>
          <h3 v-else class="eyebrow">{{ g.title }}</h3>
          <p v-if="g.help" class="cfg-section-help">{{ g.help }}</p>

          <div class="card cfg-card">
            <div
              v-for="fl in g.fields"
              :key="fl.pointer"
              class="cfg-row"
              :class="{
                'cfg-row-lead': fl.widget === 'checkbox' && fl.editable,
                'cfg-row-moved': changedOnDisk[key(g.file.path, fl.pointer)],
                'cfg-row-off': locked,
              }"
            >
              <!-- A checkbox leads its row; every other control is right-aligned. -->
              <label
                v-if="fl.widget === 'checkbox' && fl.editable"
                class="cfg-check"
                :class="{ on: current(g.file.path, fl) === true }"
              >
                <input
                  type="checkbox"
                  :checked="current(g.file.path, fl) === true"
                  :disabled="locked"
                  @change="setValue(g.file.path, fl, $event.target.checked)"
                />
                <span class="cfg-check-box" aria-hidden="true" />
              </label>

              <div class="cfg-row-text">
                <span class="cfg-label">
                  <span v-if="isDirty(g.file.path, fl)" class="cfg-dot-dirty" aria-hidden="true" />
                  {{ fl.label }}
                  <span v-if="fl.readOnly" class="cfg-pill">Read-only</span>
                  <!-- The game has not written this one, so what is shown is the
                       value the game itself would use, not one it recorded. -->
                  <span v-else-if="fl.unset && !isDirty(g.file.path, fl)" class="cfg-pill" :title="`${game.title || game._itemTitle} hasn't written this setting yet. This is the value it uses until something changes it.`">Default</span>
                  <span v-if="changedOnDisk[key(g.file.path, fl.pointer)]" class="cfg-pill">Changed by the game</span>
                </span>
                <span v-if="fl.help" class="cfg-help">{{ fl.help }}</span>
                <span v-if="!fl.editable && !fl.readOnly && !neverLaunched" class="cfg-reason">
                  {{ unavailableCopy(fl.reason) }}
                </span>
                <span v-if="invalidReason(g.file.path, fl)" class="cfg-invalid">
                  {{ invalidReason(g.file.path, fl) }}
                </span>
              </div>

              <button
                v-if="!locked && differsFromDefault(g.file.path, fl)"
                class="cfg-reset"
                :title="`Put this back to ${String(fl.default)}, the value ${game.title || game._itemTitle} uses`"
                @click="resetToDefault(g.file.path, fl)"
              >Reset</button>

              <!-- Read-only: the value, no control. -->
              <span v-if="fl.readOnly" class="cfg-ro mono">{{ fl.display || '—' }}</span>

              <!-- Unavailable, or the game has never written the file. -->
              <span v-else-if="!fl.editable" class="cfg-none">{{ neverLaunched ? '—' : "Can't edit" }}</span>

              <template v-else>
                <span
                  v-if="fl.widget === 'toggle'"
                  class="cfg-switch"
                  :class="{ on: current(g.file.path, fl) === (fl.kind === 'bool' ? true : fl.on) }"
                  role="switch"
                  :aria-checked="current(g.file.path, fl) === (fl.kind === 'bool' ? true : fl.on)"
                  @click="locked || setValue(g.file.path, fl,
                    fl.kind === 'bool'
                      ? !(current(g.file.path, fl) === true)
                      : (current(g.file.path, fl) === fl.on ? fl.off : fl.on))"
                />

                <select
                  v-else-if="fl.widget === 'select'"
                  class="cfg-select"
                  :disabled="locked"
                  :value="String(current(g.file.path, fl))"
                  @change="setValue(g.file.path, fl,
                    fl.options.find(o => String(o.value) === $event.target.value)?.value)"
                >
                  <option v-for="o in fl.options" :key="String(o.value)" :value="String(o.value)">{{ o.label }}</option>
                </select>

                <span v-else-if="fl.widget === 'radio'" class="cfg-radios">
                  <label v-for="o in fl.options" :key="String(o.value)" class="cfg-radio">
                    <input
                      type="radio"
                      :checked="String(current(g.file.path, fl)) === String(o.value)"
                      :disabled="locked"
                      @change="setValue(g.file.path, fl, o.value)"
                    />
                    <span class="cfg-radio-ring" aria-hidden="true" />
                    <span class="cfg-radio-label">{{ o.label }}</span>
                  </label>
                </span>

                <input
                  v-else-if="fl.widget === 'text'"
                  class="cfg-text mono"
                  type="text"
                  :disabled="locked"
                  :value="current(g.file.path, fl) ?? ''"
                  @input="setValue(g.file.path, fl, $event.target.value)"
                />

                <span v-else-if="fl.widget === 'number'" class="cfg-number">
                  <span v-if="fl.min !== undefined && fl.max !== undefined" class="cfg-range">
                    {{ fl.min }}–{{ fl.max }}
                  </span>
                  <input
                    class="cfg-number-input mono"
                    :class="{ bad: invalidReason(g.file.path, fl) }"
                    type="text"
                    inputmode="numeric"
                    :disabled="locked"
                    :value="current(g.file.path, fl) ?? ''"
                    @input="setValue(g.file.path, fl, $event.target.value)"
                  />
                  <span v-if="fl.unit" class="cfg-unit">{{ fl.unit }}</span>
                </span>

                <span v-else-if="fl.widget === 'slider'" class="cfg-slider">
                  <input
                    type="range"
                    :min="fl.min"
                    :max="fl.max"
                    :step="fl.step ?? 1"
                    :disabled="locked"
                    :value="Number(current(g.file.path, fl)) || 0"
                    @input="setValue(g.file.path, fl, Number($event.target.value))"
                  />
                  <span class="cfg-readout mono">{{ current(g.file.path, fl) }}{{ fl.unit ?? '' }}</span>
                </span>

                <span v-else-if="fl.widget === 'path'" class="cfg-path">
                  <span class="cfg-path-box mono" :title="current(g.file.path, fl) || ''">
                    {{ current(g.file.path, fl) || '—' }}
                  </span>
                  <button class="chip" :disabled="locked" @click="choosePath(g.file.path, fl)">Choose…</button>
                </span>
              </template>
            </div>
          </div>
        </section>

        <!-- The save bar sits in the content column so the rail stays reachable. -->
        <div v-if="dirtyCount && !locked" class="card cfg-savebar">
          <div class="cfg-savebar-text">
            <p class="cfg-savebar-count">{{ dirtyCount }} unsaved change{{ dirtyCount === 1 ? '' : 's' }}</p>
            <p class="cfg-savebar-sub" :class="{ bad: anyInvalid }">
              {{ anyInvalid
                ? 'Fix the value marked in red to save.'
                : `In ${dirtyFileTitles.join(', ')} · applies to ${profile} only` }}
            </p>
          </div>
          <button class="chip" @click="discard">Discard</button>
          <button class="btn-accent" :disabled="anyInvalid || saving" @click="save">Save</button>
        </div>
      </div>
    </div>

    <!-- Leaving with unsaved edits. Switching files does not come through here. -->
    <div v-if="leaveCb" class="cfg-scrim" @click.self="leaveKeepEditing">
      <div class="card cfg-modal">
        <h3 class="cfg-modal-title">Save your changes?</h3>
        <p class="cfg-modal-body">
          You have {{ dirtyCount }} unsaved change{{ dirtyCount === 1 ? '' : 's' }} to
          {{ game.title || game._itemTitle }}'s settings for {{ profile }}. Nothing has been
          written to the game's files yet.
        </p>
        <div class="cfg-modal-actions">
          <button class="chip" @click="leaveKeepEditing">Keep editing</button>
          <button class="chip" @click="leaveDiscard">Discard</button>
          <button class="btn-accent" :disabled="anyInvalid" @click="leaveSave">Save and leave</button>
        </div>
      </div>
    </div>
  </section>
</template>

<style lang="scss" scoped>
/* card, chip and the alert family are defined per component in this codebase
   rather than globally — GameDetail and Settings each carry their own — and a
   parent's scoped styles do not reach a child's template. So they are repeated
   here with the same values; only .eyebrow and .mono are global (tokens.css). */
.card {
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: var(--r-panel);
  padding: 17px 18px;
}

.chip {
  padding: 6px 13px;
  border-radius: var(--r-pill);
  border: 1px solid var(--line);
  background: var(--panel2);
  color: var(--dim);
  font: inherit;
  font-size: 12.5px;
  font-weight: 500;
  cursor: pointer;

  &:hover { color: var(--text); }
  &:disabled { color: var(--dim2); cursor: not-allowed; }
}

.alert {
  display: flex;
  gap: 12px;
  padding: 14px 16px;
  background: var(--panel);
  border: 1px solid var(--line2);
  border-radius: 11px;
}

.alert-body { min-width: 0; flex: 1; }

.alert-title {
  margin: 0 0 4px;
  font-size: 13.5px;
  font-weight: 500;
  color: var(--text);
}

.alert-text {
  margin: 0;
  font-size: 12.5px;
  line-height: 1.5;
  color: var(--dim);
}

.cfg { display: flex; flex-direction: column; gap: 14px; }

.cfg-head { display: flex; flex-direction: column; gap: 4px; }
.cfg-title { margin: 0; font-size: 15px; font-weight: 600; letter-spacing: -0.01em; color: var(--text); }
.cfg-sub { margin: 0; max-width: 72ch; font-size: 12.5px; line-height: 1.5; color: var(--dim2); }
.cfg-saved { display: flex; align-items: center; gap: 7px; margin: 2px 0 0; font-size: 12.5px; color: var(--dim); }
.cfg-dot-saved { width: 7px; height: 7px; border-radius: 50%; background: var(--dim); }

/* ── banner ── */
.cfg-banner { align-items: flex-start; gap: 11px; }
.cfg-banner-warn { border-color: var(--warn); }
.cfg-banner-bad { border-color: var(--bad); }
.cfg-banner-accent { border-color: var(--accent); }
.cfg-banner-dot { flex: 0 0 auto; width: 8px; height: 8px; margin-top: 5px; border-radius: 50%; }
.cfg-dot-warn { background: var(--warn); }
.cfg-dot-bad { background: var(--bad); }
.cfg-dot-accent { background: var(--accent); }
.cfg-banner-action { flex: 0 0 auto; align-self: center; }

/* ── layout ── */
.cfg-grid { display: grid; grid-template-columns: 172px minmax(0, 1fr); gap: 22px; align-items: start; }

.cfg-rail { position: sticky; top: 74px; display: flex; flex-direction: column; gap: 4px; }

.cfg-filter {
  width: 100%;
  margin-bottom: 6px;
  padding: 7px 10px;
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 9px;
  font: inherit;
  font-size: 12.5px;
  color: var(--text);

  &::placeholder { color: var(--dim2); }
  &:focus { outline: none; border-color: var(--accent); }
}

.cfg-rail-row {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 1px;
  padding: 8px 10px;
  background: transparent;
  border: 0;
  border-radius: 8px;
  text-align: left;
  cursor: pointer;
  transition: var(--t-bg);

  &:hover { background: var(--panel); }
  &.active { background: var(--panel2); }
}
.cfg-rail-title { font-size: 12.5px; font-weight: 500; color: var(--text); }
.cfg-rail-path { font-size: 11px; color: var(--dim2); }
.cfg-rail-badge {
  position: absolute;
  top: 8px;
  right: 9px;
  min-width: 17px;
  padding: 1px 5px;
  border-radius: var(--r-pill);
  background: var(--accent);
  font-size: 10.5px;
  font-weight: 600;
  color: #fff;
  text-align: center;
}

.cfg-content { display: flex; flex-direction: column; gap: 18px; min-width: 0; }
.cfg-empty { margin: 0; font-size: 12.5px; color: var(--dim2); }

.cfg-section { display: flex; flex-direction: column; gap: 8px; }
.cfg-group { color: var(--dim2); }
.cfg-section-help { margin: -4px 0 0; font-size: 12.5px; color: var(--dim2); }

.cfg-card { padding: 0; overflow: hidden; }

/* ── rows ── */
.cfg-row {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 13px 17px;
  transition: var(--t-bg);

  & + & { border-top: 1px solid var(--line); }
}
.cfg-row-moved { background: var(--accent-soft); }
.cfg-row-off { opacity: 0.5; pointer-events: none; }

.cfg-row-text { display: flex; flex-direction: column; gap: 2px; min-width: 0; flex: 1; }
.cfg-label { display: flex; align-items: center; gap: 7px; font-size: 13.5px; font-weight: 500; color: var(--text); }
.cfg-help { font-size: 12.5px; line-height: 1.45; color: var(--dim2); }
.cfg-reason { font-size: 12.5px; line-height: 1.45; color: var(--warn); }
.cfg-invalid { font-size: 12.5px; line-height: 1.45; color: var(--bad); }

.cfg-dot-dirty { flex: 0 0 auto; width: 7px; height: 7px; border-radius: 50%; background: var(--accent); }

.cfg-pill {
  padding: 1px 6px;
  border: 1px solid var(--line);
  border-radius: 5px;
  font-family: var(--font-mono);
  font-size: 10.5px;
  font-weight: 400;
  color: var(--dim2);
}

.cfg-reset {
  flex: 0 0 auto;
  padding: 0;
  background: none;
  border: 0;
  font: inherit;
  font-size: 12px;
  color: var(--dim2);
  cursor: pointer;

  &:hover { color: var(--accent-hi); }
}

.cfg-ro { flex: 0 0 auto; font-size: 12px; color: var(--dim); }
.cfg-none { flex: 0 0 auto; font-size: 12.5px; color: var(--dim2); }

/* ── controls ──
   The switch repeats ProfileModal's geometry rather than sharing it: that one is
   scoped and uses --bad for a destructive confirmation, and this one is --accent. */
.cfg-switch {
  position: relative;
  flex: 0 0 auto;
  width: 38px;
  height: 22px;
  border-radius: var(--r-pill);
  background: var(--panel3);
  cursor: pointer;
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
  &.on { background: var(--accent); }
  &.on::after { left: 19px; }
}

.cfg-check { flex: 0 0 auto; display: inline-flex; cursor: pointer; }
.cfg-check input { position: absolute; opacity: 0; width: 0; height: 0; }
.cfg-check-box {
  width: 18px;
  height: 18px;
  border: 1px solid var(--line2);
  border-radius: 5px;
  background: var(--panel2);
  position: relative;
  transition: var(--t-bg);
}
.cfg-check.on .cfg-check-box {
  background: var(--accent);
  border-color: var(--accent);

  &::after {
    content: "";
    position: absolute;
    top: 4px;
    left: 5px;
    width: 6px;
    height: 3px;
    border-left: 1.8px solid #fff;
    border-bottom: 1.8px solid #fff;
    transform: rotate(-45deg);
  }
}

.cfg-select {
  flex: 0 0 auto;
  min-width: 150px;
  padding: 6px 9px;
  background: var(--panel2);
  border: 1px solid var(--line2);
  border-radius: 8px;
  font: inherit;
  font-size: 12.5px;
  color: var(--text);
}

.cfg-radios { flex: 0 0 auto; display: flex; align-items: center; gap: 14px; }
.cfg-radio { display: inline-flex; align-items: center; gap: 6px; cursor: pointer; }
.cfg-radio input { position: absolute; opacity: 0; width: 0; height: 0; }
.cfg-radio-ring {
  width: 16px;
  height: 16px;
  border: 1px solid var(--line2);
  border-radius: 50%;
  background: var(--panel2);
  position: relative;
}
.cfg-radio input:checked + .cfg-radio-ring {
  border-color: var(--accent);

  &::after {
    content: "";
    position: absolute;
    inset: 3px;
    border-radius: 50%;
    background: var(--accent);
  }
}
.cfg-radio-label { font-size: 12.5px; color: var(--text); }

.cfg-text {
  flex: 0 0 auto;
  width: 160px;
  padding: 6px 9px;
  background: var(--panel2);
  border: 1px solid var(--line2);
  border-radius: 8px;
  font-size: 12px;
  color: var(--text);

  &:focus { outline: none; border-color: var(--accent); }
}

.cfg-number { flex: 0 0 auto; display: flex; align-items: center; gap: 8px; }
.cfg-range { font-size: 12px; color: var(--dim2); }
.cfg-unit { font-size: 12px; color: var(--dim2); }
.cfg-number-input {
  width: 72px;
  padding: 6px 9px;
  background: var(--panel2);
  border: 1px solid var(--line2);
  border-radius: 8px;
  font-size: 12px;
  color: var(--text);
  text-align: right;

  &:focus { outline: none; border-color: var(--accent); }
  &.bad { border-color: var(--bad); }
}

.cfg-slider { flex: 0 0 auto; display: flex; align-items: center; gap: 10px; }
.cfg-slider input[type="range"] { width: 170px; accent-color: var(--accent); }
.cfg-readout { width: 46px; font-size: 12px; color: var(--dim); text-align: right; }

.cfg-path { flex: 0 0 auto; display: flex; align-items: center; gap: 8px; }
.cfg-path-box {
  width: 230px;
  padding: 6px 9px;
  background: var(--panel2);
  border: 1px solid var(--line2);
  border-radius: 8px;
  font-size: 12px;
  color: var(--text);
  /* Truncated from the left, so the folder name — the part that identifies the
     location — stays visible rather than the drive root. */
  direction: rtl;
  text-align: left;
  unicode-bidi: plaintext;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* ── save bar ── */
.cfg-savebar {
  position: sticky;
  bottom: 18px;
  z-index: 5;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 16px;
  /* Opaque on purpose: it floats over the rows it is about, and --panel is the
     card colour rather than a transparent tint. */
  background: var(--panel);
  border: 1px solid var(--line2);
  box-shadow: var(--shadow);
}
.cfg-savebar-text { flex: 1; min-width: 0; }
.cfg-savebar-count { margin: 0; font-size: 13.5px; font-weight: 500; color: var(--text); }
.cfg-savebar-sub { margin: 2px 0 0; font-size: 12.5px; color: var(--dim); }
.cfg-savebar-sub.bad { color: var(--bad); }

.btn-accent {
  padding: 7px 15px;
  background: var(--accent);
  border: 0;
  border-radius: 9px;
  font: inherit;
  font-size: 12.5px;
  font-weight: 500;
  color: #fff;
  cursor: pointer;

  &:disabled { background: var(--panel3); color: var(--dim2); cursor: not-allowed; }
}

/* ── leave confirmation ── */
.cfg-scrim {
  position: fixed;
  inset: 0;
  z-index: 40;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: rgba(12, 12, 14, 0.5);
}
.cfg-modal { width: min(440px, 100%); padding: 20px; box-shadow: var(--shadow); }
.cfg-modal-title { margin: 0 0 8px; font-size: 15px; font-weight: 600; color: var(--text); }
.cfg-modal-body { margin: 0 0 16px; font-size: 12.5px; line-height: 1.55; color: var(--dim); }
.cfg-modal-actions { display: flex; justify-content: flex-end; gap: 8px; }
</style>
