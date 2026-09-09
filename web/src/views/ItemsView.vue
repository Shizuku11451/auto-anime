<script setup lang="ts">
import { computed, ref } from 'vue'
import { watchDebounced } from '@vueuse/core'
import { FilterX, Inbox, SearchX, ServerOff } from '@lucide/vue'
import PageHeader from '@/components/ui/PageHeader.vue'
import SearchInput from '@/components/ui/SearchInput.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import SkeletonBlock from '@/components/ui/SkeletonBlock.vue'
import BaseButton from '@/components/ui/BaseButton.vue'
import AnimatedNumber from '@/components/ui/AnimatedNumber.vue'
import ItemRow from '@/components/items/ItemRow.vue'
import { useAppStore } from '@/stores/app'
import { useDialogs } from '@/stores/dialogs'
import { useBusySet } from '@/composables/useBusy'
import { http, errorMessage } from '@/api/client'
import { toast } from '@/stores/toast'
import { confirmDialog } from '@/stores/confirm'
import { CHIP_DEFS, chipCount } from '@/utils/status'
import type { Item } from '@/api/types'

const store = useAppStore()
const dialogs = useDialogs()
const { is: isBusy, run } = useBusySet()

const filter = ref('全部')
const search = ref(store.search.value)
watchDebounced(
  search,
  (v) => {
    const next = v.trim()
    if (next === store.search.value) return
    store.search.value = next
    void store.load()
  },
  { debounce: 350 },
)

const chips = computed(() =>
  CHIP_DEFS.map((d) => ({ key: d.key, n: d.key === '全部' ? store.totalItems.value : chipCount(d, store.counts.value) })),
)
const activeChip = computed(() => Math.max(0, CHIP_DEFS.findIndex((d) => d.key === filter.value)))

const list = computed(() => {
  const def = CHIP_DEFS.find((d) => d.key === filter.value) ?? CHIP_DEFS[0]!
  return store.items.value.filter((x) => def.match(x.status || '待下载'))
})

async function download(item: Item) {
  await run(item.id, async () => {
    try {
      await http.post(`/api/items/${item.id}/download`)
      toast.success('已加入下载队列')
    } catch (e) {
      toast.error(errorMessage(e))
    }
    await store.refresh()
  })
}

async function remove(item: Item) {
  const ok = await confirmDialog('移除记录', '只删除这条记录，不会删除 NAS 上已保存的文件。', {
    danger: true,
    okText: '移除',
  })
  if (!ok) return
  try {
    await http.delete(`/api/items/${item.id}`)
    toast.success('已移除记录')
  } catch (e) {
    toast.error(errorMessage(e))
  }
  await store.refresh()
}
</script>

<template>
  <div class="page">
    <PageHeader title="最近条目" description="最新 100 条，每 5 秒自动刷新">
      <SearchInput v-model="search" placeholder="搜索番剧、原始标题、字幕组" width="300px" />
    </PageHeader>

    <div class="chips" role="tablist" aria-label="状态筛选" :style="{ '--active': activeChip }">
      <button
        v-for="c in chips"
        :key="c.key"
        type="button"
        role="tab"
        class="chip"
        :class="{ active: filter === c.key }"
        :aria-selected="filter === c.key"
        @click="filter = c.key"
      >
        {{ c.key }}
        <span class="n"><AnimatedNumber :value="c.n" /></span>
      </button>
    </div>

    <div v-if="!store.loaded.value" class="list" aria-busy="true">
      <div v-for="i in 5" :key="i" class="sk-row" :style="{ '--i': i }">
        <SkeletonBlock width="54px" height="81px" radius="9px" />
        <div class="sk-lines">
          <SkeletonBlock height="14px" width="52%" />
          <SkeletonBlock height="11px" width="70%" />
          <SkeletonBlock height="11px" width="85%" />
        </div>
      </div>
    </div>

    <EmptyState
      v-else-if="store.loadError.value && !store.items.value.length"
      :icon="ServerOff"
      title="数据加载失败"
      :hint="store.loadError.value"
      tone="error"
    >
      <BaseButton variant="primary" @click="store.refresh()">重试</BaseButton>
    </EmptyState>

    <EmptyState
      v-else-if="!store.items.value.length && store.search.value"
      :icon="SearchX"
      title="没有匹配的条目"
      hint="换个关键词，或清除搜索"
      compact
    >
      <BaseButton @click="search = ''">清除搜索</BaseButton>
    </EmptyState>

    <EmptyState
      v-else-if="!store.items.value.length"
      :icon="Inbox"
      title="暂无条目"
      hint="添加 RSS 订阅并同步后，新番会出现在这里"
    />

    <EmptyState
      v-else-if="!list.length"
      :icon="FilterX"
      title="当前筛选下没有条目"
      hint="换个状态筛选，或清除搜索关键词"
      compact
    >
      <BaseButton @click="filter = '全部'">查看全部</BaseButton>
    </EmptyState>

    <TransitionGroup v-else tag="div" class="list" name="list" appear>
      <ItemRow
        v-for="(item, i) in list"
        :key="item.id"
        :item="item"
        :busy="isBusy(item.id)"
        :style="{ '--i': i }"
        @download="download(item)"
        @storage="dialogs.openStorage(item.anime_title || item.title)"
        @correct="dialogs.openCorrect(item)"
        @remove="remove(item)"
      />
    </TransitionGroup>
  </div>
</template>

<style scoped>
.chips {
  display: flex;
  gap: 6px;
  margin-bottom: 18px;
  overflow-x: auto;
  padding-bottom: 2px;
  scrollbar-width: none;
}
.chips::-webkit-scrollbar {
  display: none;
}
.chip {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  height: 32px;
  padding: 0 6px 0 13px;
  border: 1px solid var(--line);
  border-radius: 999px;
  background: var(--surface);
  color: var(--text-2);
  font-size: 13px;
  font-weight: 550;
  white-space: nowrap;
  cursor: pointer;
  transition:
    background var(--t-fast) var(--ease-out),
    color var(--t-fast) var(--ease-out),
    border-color var(--t-fast) var(--ease-out),
    transform var(--t-fast) var(--ease-out);
}
.chip:hover {
  border-color: var(--line-strong);
  color: var(--text);
}
.chip:active {
  transform: scale(0.96);
}
.chip.active {
  background: var(--accent-soft);
  border-color: var(--accent-border);
  color: var(--accent);
  font-weight: 650;
}
.chip .n {
  display: inline-grid;
  place-items: center;
  min-width: 22px;
  height: 20px;
  padding: 0 6px;
  border-radius: 999px;
  background: var(--surface-3);
  color: var(--text-2);
  font-size: 11.5px;
  font-weight: 700;
  transition:
    background var(--t-fast) var(--ease-out),
    color var(--t-fast) var(--ease-out);
}
.chip.active .n {
  background: var(--accent);
  color: var(--accent-contrast);
}

.list {
  position: relative;
  display: grid;
  gap: 10px;
}
.sk-row {
  display: grid;
  grid-template-columns: 54px 1fr;
  gap: 16px;
  padding: 14px 16px;
  border: 1px solid var(--line);
  border-radius: var(--r-lg);
  background: var(--surface);
  animation: fade-in var(--t-slow) var(--ease-out) both;
  animation-delay: calc(var(--i) * 50ms);
}
.sk-lines {
  display: grid;
  gap: 10px;
  align-content: start;
  padding-top: 4px;
}
</style>
