<script setup>
import { computed } from 'vue'

// A profile's picture, or its initial when it has none — the normal case, so
// the fallback is a plain letter on a plain panel rather than a generated
// colour or a silhouette. Square with the cover-art radius wherever it appears.
const props = defineProps({
  profile: { type: Object, default: null },
  size: { type: Number, default: 30 },
})

// Letter sizes from the design: 13px at 30, 15px at 36, 16px at 38.
const fontSize = computed(() => props.size >= 38 ? 16 : props.size >= 36 ? 15 : 13)
const initial = computed(() => (props.profile?.name ?? '?').trim().charAt(0).toUpperCase() || '?')
const picture = computed(() => props.profile?.picture || null)
</script>

<template>
  <span
    class="profile-picture"
    :style="{ width: size + 'px', height: size + 'px', fontSize: fontSize + 'px', backgroundImage: picture ? `url(${picture})` : 'none' }"
    aria-hidden="true"
  >{{ picture ? '' : initial }}</span>
</template>

<style lang="scss" scoped>
.profile-picture {
  display: grid;
  place-items: center;
  flex: 0 0 auto;
  border-radius: var(--r-cover);
  border: 1px solid var(--line);
  background: var(--panel2) center / cover no-repeat;
  font-weight: 600;
  color: var(--dim2);
  overflow: hidden;
}
</style>
