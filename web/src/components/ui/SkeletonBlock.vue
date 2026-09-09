<script setup lang="ts">
withDefaults(defineProps<{ width?: string; height?: string; radius?: string; circle?: boolean }>(), {
  width: '100%',
  height: '14px',
  radius: '8px',
  circle: false,
})
</script>

<template>
  <span
    class="skeleton"
    :style="{ width, height, borderRadius: circle ? '50%' : radius }"
    aria-hidden="true"
  />
</template>

<style scoped>
.skeleton {
  display: block;
  background: linear-gradient(100deg, var(--surface-3) 30%, var(--surface-2) 50%, var(--surface-3) 70%);
  background-size: 200% 100%;
  animation: shimmer 1.6s linear infinite;
}
</style>
