<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { NAV_ITEMS } from '@/nav'

const route = useRoute()
const activeIndex = computed(() => Math.max(0, NAV_ITEMS.findIndex((n) => n.name === route.name)))
</script>

<template>
  <nav class="tabbar" aria-label="主导航" :style="{ '--active': activeIndex, '--n': NAV_ITEMS.length }">
    <span class="tab-indicator" aria-hidden="true" />
    <RouterLink
      v-for="(item, i) in NAV_ITEMS"
      :key="item.name"
      :to="item.to"
      class="tab"
      :class="{ active: i === activeIndex }"
    >
      <component :is="item.icon" class="tab-ic" aria-hidden="true" />
      <span class="tab-label">{{ item.label }}</span>
    </RouterLink>
  </nav>
</template>

<style scoped>
.tabbar {
  position: fixed;
  left: 0;
  right: 0;
  bottom: 0;
  z-index: 60;
  display: grid;
  grid-template-columns: repeat(var(--n), 1fr);
  padding: 6px 8px calc(6px + env(safe-area-inset-bottom));
  background: var(--surface-glass);
  border-top: 1px solid var(--line);
  backdrop-filter: blur(18px) saturate(1.2);
  -webkit-backdrop-filter: blur(18px) saturate(1.2);
}
.tab-indicator {
  position: absolute;
  top: 6px;
  left: 8px;
  width: calc((100% - 16px) / var(--n));
  height: calc(100% - 12px - env(safe-area-inset-bottom));
  border-radius: var(--r-md);
  background: var(--accent-soft);
  transform: translateX(calc(var(--active) * 100%));
  transition: transform var(--t-med) var(--ease-out);
  pointer-events: none;
}
.tab {
  position: relative;
  z-index: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 3px;
  height: 54px;
  border-radius: var(--r-md);
  color: var(--text-3);
  font-size: 11px;
  font-weight: 600;
  text-decoration: none;
  transition: color var(--t-med) var(--ease-out);
}
.tab:hover {
  text-decoration: none;
}
.tab.active {
  color: var(--accent);
}
.tab-ic {
  width: 20px;
  height: 20px;
  transition: transform var(--t-med) var(--ease-spring);
}
.tab.active .tab-ic {
  transform: translateY(-1px) scale(1.06);
}
</style>
