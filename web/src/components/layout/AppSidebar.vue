<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import BrandMark from '@/components/ui/BrandMark.vue'
import ThemeToggle from '@/components/ui/ThemeToggle.vue'
import RuntimeCard from './RuntimeCard.vue'
import SideActions from './SideActions.vue'
import UpdateNotice from './UpdateNotice.vue'
import { NAV_ITEMS } from '@/nav'
import { useDialogs } from '@/stores/dialogs'
import { useUpdateStore } from '@/stores/update'
import { isDevVersion } from '@/utils/update'

const route = useRoute()
const activeIndex = computed(() => Math.max(0, NAV_ITEMS.findIndex((n) => n.name === route.name)))

const update = useUpdateStore()
const { openUpdate } = useDialogs()
/** dev 之外显示真实版本号，方便确认更新是否生效 */
const versionText = computed(() => {
  const label = update.currentLabel.value
  if (!label || isDevVersion(label.split(' ')[0])) return 'dev · 本地服务'
  return `v${label.replace(/^v/, '')}`
})

function showRelease() {
  update.dismiss()
  openUpdate()
}
</script>

<template>
  <aside class="sidebar">
    <BrandMark class="sidebar-brand" />

    <nav class="nav" aria-label="主导航" :style="{ '--active': activeIndex }">
      <span class="nav-indicator" aria-hidden="true" />
      <RouterLink
        v-for="(item, i) in NAV_ITEMS"
        :key="item.name"
        :to="item.to"
        class="nav-link"
        :class="{ active: i === activeIndex }"
      >
        <component :is="item.icon" class="nav-ic" aria-hidden="true" />
        <span>{{ item.label }}</span>
      </RouterLink>
    </nav>

    <div class="spacer" />

    <UpdateNotice />

    <RuntimeCard />

    <div class="side-foot">
      <SideActions layout="grid" />
      <div class="foot-row">
        <button type="button" class="foot-ver" :class="{ hot: update.updateAvailable.value }" @click="showRelease">
          {{ versionText }}
        </button>
        <ThemeToggle />
      </div>
    </div>
  </aside>
</template>

<style scoped>
.sidebar {
  position: sticky;
  top: 0;
  width: var(--sidebar-w);
  height: 100dvh;
  flex: none;
  display: flex;
  flex-direction: column;
  padding: 22px 16px 16px;
  background: var(--surface-glass);
  border-right: 1px solid var(--line);
  backdrop-filter: blur(18px) saturate(1.2);
  -webkit-backdrop-filter: blur(18px) saturate(1.2);
  overflow: hidden auto;
}
.sidebar-brand {
  padding: 2px 8px 22px;
}

.nav {
  --row: 42px;
  --gap: 4px;
  position: relative;
  display: flex;
  flex-direction: column;
  gap: var(--gap);
}
.nav-indicator {
  position: absolute;
  left: 0;
  right: 0;
  top: 0;
  height: var(--row);
  border-radius: var(--r-md);
  background: var(--accent-soft);
  box-shadow: inset 0 0 0 1px var(--accent-border);
  transform: translateY(calc(var(--active) * (var(--row) + var(--gap))));
  transition: transform 460ms var(--ease-spring);
  pointer-events: none;
}
.nav-link {
  position: relative;
  z-index: 1;
  display: flex;
  align-items: center;
  gap: 11px;
  height: var(--row);
  padding: 0 13px;
  border-radius: var(--r-md);
  color: var(--text-2);
  font-weight: 550;
  font-size: 14px;
  text-decoration: none;
  transition:
    color var(--t-med) var(--ease-out),
    background var(--t-fast) var(--ease-out);
}
.nav-link:hover {
  color: var(--text);
  text-decoration: none;
}
.nav-link:hover:not(.active) {
  background: var(--surface-2);
}
.nav-link.active {
  color: var(--accent);
  font-weight: 650;
}
.nav-ic {
  width: 18px;
  height: 18px;
  color: var(--text-3);
  transition:
    color var(--t-med) var(--ease-out),
    transform var(--t-med) var(--ease-spring);
}
.nav-link:hover .nav-ic {
  color: var(--text-2);
  transform: translateX(1px);
}
.nav-link.active .nav-ic {
  color: var(--accent);
}

.spacer {
  flex: 1;
  min-height: 16px;
}
.side-foot {
  margin-top: 12px;
  display: grid;
  gap: 10px;
}
.foot-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 2px 0 8px;
}
.foot-ver {
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--text-3);
  font-size: 11px;
  font-weight: 550;
  letter-spacing: 0.04em;
  cursor: pointer;
  border-radius: 6px;
  transition: color var(--t-fast) var(--ease-out);
}
.foot-ver:hover {
  color: var(--text);
}
.foot-ver.hot {
  color: var(--accent);
  font-weight: 650;
}
</style>
