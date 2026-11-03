<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { Info, Sparkles } from '@lucide/vue'
import RouteLink from '@/components/ui/RouteLink.vue'
import ThemeToggle from '@/components/ui/ThemeToggle.vue'
import NavIcon from './NavIcon.vue'
import ServiceTools from './ServiceTools.vue'
import { useAppStore } from '@/stores/app'
import { NAV_ITEMS } from '@/nav'
import { useDialogs } from '@/stores/dialogs'
import { useUpdateStore } from '@/stores/update'
import { isDevVersion } from '@/utils/update'

const route = useRoute()
const store = useAppStore()
const activeIndex = computed(() => Math.max(0, NAV_ITEMS.findIndex((n) => n.name === route.name)))

const update = useUpdateStore()
const { openUpdate } = useDialogs()
/** dev 之外显示真实版本号，方便确认更新是否生效 */
const versionText = computed(() => {
  const label = update.currentLabel.value
  if (!label || isDevVersion(label.split(' ')[0])) return 'dev · 本地服务'
  return `v${label.replace(/^v/, '')}`
})
const versionTitle = computed(() =>
  update.updateAvailable.value ? `${versionText.value} · 发现新版本 ${update.info.value?.latest || ''}`.trim() : versionText.value,
)
const connectionText = computed(() =>
  store.runtimeError.value ? '服务连接异常' : store.status.value ? '服务已连接' : '正在连接服务',
)

function showRelease() {
  update.dismiss()
  openUpdate()
}
</script>

<template>
  <aside class="sidebar">
    <nav class="nav" aria-label="主导航" :style="{ '--active': activeIndex }">
      <span class="nav-indicator" aria-hidden="true" />
      <RouteLink
        v-for="(item, i) in NAV_ITEMS"
        :key="item.name"
        :to="item.to"
        :active="i === activeIndex"
        :title="item.label"
        class="nav-link"
        :class="{ active: i === activeIndex }"
      >
        <NavIcon :icon="item.icon" :active="i === activeIndex" class="nav-ic" />
        <span class="nav-label">{{ item.label }}</span>
      </RouteLink>
    </nav>

    <div class="spacer" />

    <div class="side-foot">
      <div class="connection" role="status" :title="connectionText">
        <i :class="{ off: store.runtimeError.value, pending: !store.status.value }" aria-hidden="true" />
        <span class="hidden-label">{{ connectionText }}</span>
      </div>
      <ServiceTools />
      <button
        type="button"
        class="foot-ver"
        :class="{ hot: update.updateAvailable.value, unread: update.notify.value }"
        :title="versionTitle"
        :aria-label="versionTitle"
        @click="showRelease"
      >
        <component :is="update.updateAvailable.value ? Sparkles : Info" aria-hidden="true" />
      </button>
      <ThemeToggle />
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
  padding: 24px 10px 18px;
  background: var(--surface);
  border-right: 1px solid var(--line);
  overflow: hidden auto;
}

.nav {
  --row: 56px;
  --gap: 6px;
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
  transform: translateY(calc(var(--active) * (var(--row) + var(--gap))));
  transition: transform var(--t-med) var(--ease-out);
  pointer-events: none;
}
.nav-link {
  position: relative;
  z-index: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 4px;
  height: var(--row);
  border-radius: var(--r-md);
  color: var(--text-2);
  font-size: 11px;
  font-weight: 650;
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
}
.nav-ic {
  color: var(--text-3);
  transition:
    color var(--t-med) var(--ease-out),
    transform var(--t-med) var(--ease-spring);
}
.nav-link:hover .nav-ic {
  color: var(--text-2);
}
.nav-link:hover:not(.active) .nav-ic {
  transform: translateY(-2px);
}
.nav-link.active .nav-ic {
  color: var(--accent);
}
.nav-label {
  max-width: 100%;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
  line-height: 1.2;
  animation: fade-in var(--t-med) var(--ease-out) both;
}
/* 未选中的项只显示图标，名称保留给读屏 */
.nav-link:not(.active) .nav-label,
.hidden-label {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip-path: inset(50%);
  white-space: nowrap;
}

.spacer {
  flex: 1;
  min-height: 16px;
}
.side-foot {
  border-top: 1px solid var(--line);
  padding-top: 14px;
  margin-top: 12px;
  display: grid;
  justify-items: center;
  gap: 10px;
}
.connection { position: relative; display: grid; place-items: center; width: 36px; height: 16px; }
.connection i { width: 7px; height: 7px; border-radius: 50%; background: var(--green); }
.connection i.pending { background: var(--text-3); }
.connection i.off { background: var(--red); }
.foot-ver {
  position: relative;
  width: 36px;
  height: 36px;
  display: grid;
  place-items: center;
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--text-3);
  cursor: pointer;
  border-radius: var(--r-sm);
  transition:
    color var(--t-fast) var(--ease-out),
    background var(--t-fast) var(--ease-out);
}
.foot-ver svg {
  width: 18px;
  height: 18px;
}
.foot-ver:hover {
  color: var(--text);
  background: var(--surface-2);
}
.foot-ver.hot {
  color: var(--accent);
}
.foot-ver.unread::after {
  content: "";
  position: absolute;
  top: 7px;
  right: 7px;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--accent);
  box-shadow: 0 0 0 2px var(--surface);
}
</style>
