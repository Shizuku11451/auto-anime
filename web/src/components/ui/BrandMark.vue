<script setup lang="ts">
withDefaults(defineProps<{ size?: 'md' | 'lg'; subtitle?: string }>(), {
  size: 'md',
  subtitle: '',
})
</script>

<template>
  <div class="brand" :class="`s-${size}`">
    <img class="brand-logo" src="/logo.svg" width="1100" height="270" alt="Auto-Anime" />
    <div v-if="subtitle" class="brand-sub">{{ subtitle }}</div>
  </div>
</template>

<style scoped>
.brand {
  width: 208px;
  max-width: 100%;
  min-width: 0;
}
.brand-logo { display: block; width: 100%; height: auto; }
.s-lg { width: 280px; }
@media (max-width: 640px) { .brand:not(.s-lg) { width: 192px; flex: 0 1 192px; } }
.brand-sub {
  margin-top: 2px;
  color: var(--text-3);
  font-size: 10px;
  font-weight: 650;
  letter-spacing: 0;
  white-space: nowrap;
}
.s-lg .brand-sub {
  font-size: 11px;
}
</style>
