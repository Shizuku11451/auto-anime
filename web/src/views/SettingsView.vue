<script setup lang="ts">
import { onMounted } from 'vue'
import { useEventListener } from '@vueuse/core'
import { onBeforeRouteLeave, onBeforeRouteUpdate, useRoute, useRouter } from 'vue-router'
import { FolderTree, Globe, HardDrive, Rocket, Star, UserRound } from '@lucide/vue'
import PageHeader from '@/components/ui/PageHeader.vue'
import BaseButton from '@/components/ui/BaseButton.vue'
import BangumiWishes from '@/components/discover/BangumiWishes.vue'
import DownloadSettings from '@/components/settings/DownloadSettings.vue'
import LibrarySettings from '@/components/settings/LibrarySettings.vue'
import NetworkSettings from '@/components/settings/NetworkSettings.vue'
import AccountSettings from '@/components/settings/AccountSettings.vue'
import UpdateSettings from '@/components/settings/UpdateSettings.vue'
import { SECTIONS, isSectionId, provideSettings, type SectionId } from '@/composables/useSettingsForm'
import { useUpdateStore } from '@/stores/update'

const ICONS: Record<SectionId, unknown> = { download: HardDrive, library: FolderTree, network: Globe, account: UserRound, bangumi: Star, update: Rocket }
const PANELS = { download: DownloadSettings, library: LibrarySettings, network: NetworkSettings, account: AccountSettings, bangumi: BangumiWishes, update: UpdateSettings }

const route = useRoute()
const router = useRouter()
const update = useUpdateStore()
const settings = provideSettings()
const { active, loading, loadErr, saving, changedSections, dirty, load, selectSection, canLeave, onKeydown } = settings

/* 当前分类写在 ?tab= 中，刷新与外部链接可以直达；切换前由 selectSection 处理未保存更改。 */
function tabOf(query: unknown): SectionId {
  return isSectionId(query) ? query : 'download'
}
active.value = tabOf(route.query.tab)

async function open(id: SectionId) {
  if (id === active.value) return
  if (!await selectSection(id)) return
  void router.replace({ query: { ...route.query, tab: id } })
}
onBeforeRouteUpdate(async to => {
  const id = tabOf(to.query.tab)
  if (id === active.value) return true
  return selectSection(id)
})
onBeforeRouteLeave(canLeave)
useEventListener(window, 'beforeunload', (event: BeforeUnloadEvent) => {
  if (dirty.value) { event.preventDefault(); event.returnValue = '' }
})
useEventListener(window, 'keydown', onKeydown)

onMounted(() => {
  void load()
  void update.checkNow()
})

defineExpose({ settings, open })
</script>

<template>
  <div class="page settings">
    <PageHeader title="设置" />

    <div v-if="loadErr" class="load-error" role="alert">
      <span>读取设置失败：{{ loadErr }}</span>
      <BaseButton :loading="loading" :disabled="saving" @click="load">重新加载</BaseButton>
    </div>
    <div class="settings-layout">
      <nav class="settings-nav" aria-label="设置分类">
        <button
          v-for="tab in SECTIONS"
          :key="tab.id"
          type="button"
          :aria-current="active === tab.id ? 'page' : undefined"
          :class="{ active: active === tab.id }"
          :disabled="saving && active !== tab.id"
          @click="open(tab.id)"
        >
          <component :is="ICONS[tab.id]" aria-hidden="true" />
          <span>{{ tab.title }}</span>
          <span v-if="changedSections.includes(tab.id)" class="change-dot" role="img" aria-label="未保存" />
        </button>
      </nav>

      <div class="settings-content">
        <component :is="PANELS[active]" :key="active" />
      </div>
    </div>
  </div>
</template>

<style scoped>
.settings {
  max-width: 1120px;
}
.settings-layout {
  display: grid;
  grid-template-columns: 180px minmax(0, 1fr);
  gap: 28px;
  align-items: start;
}
.settings-content {
  min-width: 0;
}
.load-error {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 12px;
  margin-bottom: 18px;
  padding: 12px 16px;
  border: 1px solid var(--red-border);
  border-radius: var(--r-md);
  background: var(--red-soft);
  color: var(--red);
  overflow-wrap: anywhere;
}
.settings-nav { position: sticky; top: 24px; display: grid; gap: 4px; }
.settings-nav button { display: flex; align-items: center; gap: 10px; min-height: 44px; padding: 11px 12px; text-align: left; white-space: nowrap; background: transparent; border: 1px solid transparent; border-radius: var(--r-sm); color: var(--text-2); cursor: pointer; }
.settings-nav button:hover:not(:disabled) { background: var(--surface-2); }
.settings-nav button:disabled { opacity: 0.5; cursor: not-allowed; }
.settings-nav button.active { background: var(--accent-soft); border-color: var(--accent-border); color: var(--accent); font-weight: 600; }
.settings-nav svg { width: 16px; height: 16px; flex: none; }
.change-dot { width: 6px; height: 6px; margin-left: auto; border-radius: 50%; background: var(--accent); flex: none; }
@media (max-width: 1100px) {
  .settings-layout { grid-template-columns: minmax(0, 1fr); gap: 16px; }
  .settings-nav { position: static; display: flex; overflow-x: auto; padding-bottom: 6px; }
  .settings-nav button { flex: none; }
  .change-dot { margin-left: 0; }
}
</style>
