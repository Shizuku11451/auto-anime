<script setup lang="ts">
withDefaults(defineProps<{ size?: 'md' | 'lg'; subtitle?: string }>(), {
  size: 'md',
  subtitle: '',
})
</script>

<template>
  <div class="brand" :class="`s-${size}`">
    <img class="brand-logo" src="/favicon.svg" width="64" height="64" alt="Auto-Anime" />
    <div v-if="subtitle" class="brand-sub">{{ subtitle }}</div>
  </div>
</template>

<style scoped>
.brand {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 10px;
  min-width: 0;
}
.brand-logo { display: block; width: 40px; height: 40px; }
.s-lg .brand-logo { width: 52px; height: 52px; }
.brand-sub {
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
