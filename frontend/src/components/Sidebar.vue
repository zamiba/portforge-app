<script setup>
import { useProfiles } from '../composables/useProfiles'
import ProfilePicture from './ProfilePicture.vue'

const props = defineProps({
  // Which destination is highlighted. The game page passes 'library', since it
  // is reached from there and the sidebar should not appear to leave it.
  active: { type: String, required: true },
})

defineEmits(['navigate'])

// The last thing in the sidebar: who is playing, and a way to change it from
// anywhere. It says nothing else — sync outcomes are the notice bar's, and a
// status dot here would be a permanent reminder of something that is fine
// almost all of the time.
const { active: activeProfile, open: openProfiles } = useProfiles()

// The design's third destination, Queue, is deliberately absent: there is no
// queue behind it. InstallVersion refuses a second concurrent job, and that is
// the accepted behaviour rather than a stopgap.
const items = [
  { id: 'library', label: 'Library' },
  { id: 'roms', label: 'ROMs' },
  { id: 'settings', label: 'Settings' },
]
</script>

<template>
  <nav class="sidebar" aria-label="Main">
    <div class="wordmark">PortForge</div>

    <ul class="nav">
      <li v-for="item in items" :key="item.id">
        <button
          class="nav-item"
          :class="{ active: props.active === item.id }"
          :aria-current="props.active === item.id ? 'page' : undefined"
          @click="$emit('navigate', item.id)"
        >{{ item.label }}</button>
      </li>
    </ul>

    <div class="sidebar-spacer" />

    <button v-if="activeProfile" class="profile-row" @click="openProfiles">
      <ProfilePicture :profile="activeProfile" :size="30" />
      <span class="profile-text">
        <span class="profile-eyebrow mono">PLAYING AS</span>
        <span class="profile-name">{{ activeProfile.name }}</span>
      </span>
      <span class="profile-chevron" aria-hidden="true" />
    </button>
  </nav>
</template>

<style lang="scss" scoped>
.sidebar {
  flex: 0 0 196px;
  display: flex;
  flex-direction: column;
  padding: 22px 14px 14px;
  background: var(--panel);
  border-right: 1px solid var(--line);
  z-index: 3;
}

/* The cartridge mark stays the window icon; the sidebar is text only. */
.wordmark {
  padding: 0 10px 22px;
  font-size: 17px;
  font-weight: 600;
  letter-spacing: -0.015em;
  color: var(--text);
}

.nav {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.nav-item {
  width: 100%;
  padding: 9px 10px;
  border-radius: var(--r-control);
  text-align: left;
  font-size: 13.5px;
  font-weight: 500;
  color: var(--dim);
  transition: var(--t-bg);

  &:hover {
    background: var(--panel2);
  }

  &.active {
    background: var(--panel2);
    color: var(--text);
    font-weight: 600;
  }
}

.sidebar-spacer {
  flex: 1;
}

/* ── Profile row ── */
/* The rule spans the sidebar's padding box, so the row's margins cancel the
   sidebar's own padding and put it back on the row's contents. */
.profile-row {
  display: flex;
  align-items: center;
  gap: 10px;
  width: auto;
  margin: 0 -14px -14px;
  padding: 13px 24px 23px;
  border-top: 1px solid var(--line);
  text-align: left;
  color: var(--text);

  &:hover {
    color: var(--accent);
    .profile-chevron { border-color: var(--accent); }
  }
}

.profile-text {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
}

.profile-eyebrow {
  font-size: 9.5px;
  letter-spacing: 0.1em;
  color: var(--dim2);
  margin-bottom: 3px;
}

.profile-name {
  font-size: 13.5px;
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.profile-chevron {
  flex: 0 0 auto;
  width: 7px;
  height: 7px;
  border-right: 1.5px solid var(--dim2);
  border-top: 1.5px solid var(--dim2);
  /* Points up: the modal it opens sits above the row, like an account menu. */
  transform: rotate(-45deg);
}
</style>
