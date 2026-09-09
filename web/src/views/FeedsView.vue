<script setup lang="ts">
import { useRouter } from 'vue-router'
import { Plus, SatelliteDish } from '@lucide/vue'
import PageHeader from '@/components/ui/PageHeader.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import SkeletonBlock from '@/components/ui/SkeletonBlock.vue'
import BaseButton from '@/components/ui/BaseButton.vue'
import FeedCard from '@/components/feeds/FeedCard.vue'
import { useAppStore } from '@/stores/app'
import { useDialogs } from '@/stores/dialogs'
import { useBusySet } from '@/composables/useBusy'
import { http, errorMessage } from '@/api/client'
import { toast } from '@/stores/toast'
import { confirmDialog } from '@/stores/confirm'
import type { Feed } from '@/api/types'

const store = useAppStore()
const dialogs = useDialogs()
const router = useRouter()
const { is: isSyncing, run } = useBusySet()

async function sync(feed: Feed) {
  await run(feed.id, async () => {
    try {
      const r = await http.post<{ added: number }>('/api/sync/' + feed.id)
      toast.success(`「${feed.name}」同步完成，新增 ${r.added} 条`)
    } catch (e) {
      toast.error(errorMessage(e))
    }
    await store.refresh()
  })
}

async function toggle(feed: Feed) {
  const enable = feed.enabled === false
  try {
    await http.put('/api/feeds/' + feed.id, {
      name: feed.name,
      include_words: feed.include_words,
      exclude_words: feed.exclude_words,
      auto_download: feed.auto_download,
      enabled: enable,
    })
    toast.success(enable ? '订阅已启用' : '订阅已暂停')
  } catch (e) {
    toast.error(errorMessage(e))
  }
  await store.refresh()
}

async function remove(feed: Feed) {
  const ok = await confirmDialog(
    '删除订阅',
    `删除订阅“${feed.name}”及其全部条目记录？NAS 上已保存的文件不会被删除。`,
    { danger: true, okText: '删除' },
  )
  if (!ok) return
  try {
    await http.delete('/api/feeds/' + feed.id)
    toast.success('订阅已删除')
  } catch (e) {
    toast.error(errorMessage(e))
  }
  await store.refresh()
}
</script>

<template>
  <div class="page">
    <PageHeader
      title="订阅管理"
      description="按关键词过滤 RSS 条目，自动下载新番"
      :count="store.loaded.value ? store.feeds.value.length : undefined"
    >
      <BaseButton variant="primary" :icon="Plus" @click="dialogs.openFeed(null)">添加订阅</BaseButton>
    </PageHeader>

    <div v-if="!store.loaded.value" class="grid" aria-busy="true">
      <div v-for="i in 4" :key="i" class="sk-card" :style="{ '--i': i }">
        <SkeletonBlock height="15px" width="45%" />
        <SkeletonBlock height="11px" width="90%" />
        <SkeletonBlock height="11px" width="60%" />
      </div>
    </div>

    <EmptyState
      v-else-if="!store.feeds.value.length"
      :icon="SatelliteDish"
      title="还没有订阅"
      hint="粘贴一个 RSS 地址开始追番，或先在「找番」里搜索资源站"
    >
      <BaseButton variant="primary" :icon="Plus" @click="dialogs.openFeed(null)">添加订阅</BaseButton>
      <BaseButton @click="router.push('/discover')">去找番</BaseButton>
    </EmptyState>

    <TransitionGroup v-else tag="div" class="grid" name="list" appear>
      <FeedCard
        v-for="(f, i) in store.feeds.value"
        :key="f.id"
        :feed="f"
        :syncing="isSyncing(f.id)"
        :style="{ '--i': i }"
        @sync="sync(f)"
        @edit="dialogs.openFeed(f)"
        @toggle="toggle(f)"
        @remove="remove(f)"
      />
    </TransitionGroup>
  </div>
</template>

<style scoped>
.grid {
  position: relative;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(320px, 100%), 1fr));
  gap: 14px;
}
@media (max-width: 640px) {
  .grid {
    grid-template-columns: minmax(0, 1fr);
  }
}
.sk-card {
  display: grid;
  gap: 12px;
  padding: 18px;
  border: 1px solid var(--line);
  border-radius: var(--r-lg);
  background: var(--surface);
  animation: fade-in var(--t-slow) var(--ease-out) both;
  animation-delay: calc(var(--i) * 50ms);
}
</style>
