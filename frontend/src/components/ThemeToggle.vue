<script setup>
import { computed } from 'vue'
import { useTheme } from '../composables/useTheme'

// Sits in each page's top bar, right-aligned. It used to be the last thing in
// the sidebar; the profile row took that place.
const { theme, toggleTheme } = useTheme()

// Names the mode the toggle switches *to*, not the one in force.
const label = computed(() => theme.value === 'dark' ? 'Light mode' : 'Dark mode')
</script>

<template>
  <button class="theme-toggle" @click="toggleTheme">{{ label }}</button>
</template>

<style lang="scss" scoped>
.theme-toggle {
  flex-shrink: 0;
  padding: 7px 13px;
  border-radius: var(--r-control);
  border: 1px solid var(--line);
  background: var(--panel);
  font-size: 12.5px;
  font-weight: 500;
  color: var(--dim);
  white-space: nowrap;

  &:hover {
    border-color: var(--dim2);
    color: var(--text);
  }
}
</style>
