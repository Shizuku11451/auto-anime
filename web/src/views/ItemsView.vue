<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { watchDebounced } from '@vueuse/core'
import { Download, FilterX, Inbox, ListChecks, Pause, Play, SearchX, ServerOff, Trash2 } from '@lucide/vue'
import PageHeader from '@/components/ui/PageHeader.vue'
import SearchInput from '@/components/ui/SearchInput.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import SkeletonBlock from '@/components/ui/SkeletonBlock.vue'
import BaseButton from '@/components/ui/BaseButton.vue'
import AnimatedNumber from '@/components/ui/AnimatedNumber.vue'
import ItemRow from '@/components/items/ItemRow.vue'
import DownloadControls from '@/components/items/DownloadControls.vue'
import { useAppStore } from '@/stores/app'
import { useDialogs } from '@/stores/dialogs'
import { useBusySet } from '@/composables/useBusy'
import { http, errorMessage } from '@/api/client'
import { toast } from '@/stores/toast'
import { confirmDialog } from '@/stores/confirm'
import { CHIP_DEFS, chipCount, isPausable, isResumable, isRetryable } from '@/utils/status'
import type { Item, ItemBatchResult } from '@/api/types'
import type { PlayerEntry } from '@/stores/dialogs'

const store = useAppStore()
const dialogs = useDialogs()
const { is: isBusy, run } = useBusySet()

const filter = ref('全部')
const selecting = ref(false)
const selected = ref<number[]>([])
type BatchAction = 'download' | 'pause' | 'resume' | 'delete'
const batchRunning = ref<BatchAction | null>(null)
const batchIds = ref<number[]>([])
const batchFailures = ref<ItemBatchResult['failed']>([])
const selectedItems = computed(() => store.items.value.filter(x => selected.value.includes(x.id)))
const actionLabels = { download: '加入下载队列', pause: '暂停', resume: '加入继续队列', delete: '移除' }
function actionable(action: BatchAction) {
  return selectedItems.value.filter(x => !isBusy(x.id) && (action === 'delete' ||
    (action === 'download' ? isRetryable(x.status) : action === 'pause' ? isPausable(x.status) : isResumable(x.status))))
}
const downloadable = computed(() => actionable('download').length)
const pausable = computed(() => actionable('pause').length)
const resumable = computed(() => actionable('resume').length)
watch(store.items, (items) => {
  if (!batchRunning.value) selected.value = selected.value.filter(id => items.some(x => x.id === id))
})
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
const allSelected = computed(() => list.value.length > 0 && list.value.every(x => selected.value.includes(x.id)))
function toggle(id: number) { selected.value = selected.value.includes(id) ? selected.value.filter(x => x !== id) : [...selected.value, id] }
function selectVisible() { selected.value = allSelected.value ? selected.value.filter(id => !list.value.some(x => x.id === id)) : [...new Set([...selected.value, ...list.value.map(x => x.id)])] }
function toggleMode() { selecting.value = !selecting.value; selected.value = [] }
async function batch(action: BatchAction) {
  if (batchRunning.value) return
  const ids = actionable(action).map(x => x.id)
  if (!ids.length) return
  if (action === 'delete' && !await confirmDialog('批量移除记录', `确定移除选中的 ${ids.length} 条记录？不会删除已保存文件。`, { danger: true, okText: '移除' })) return
  batchRunning.value = action
  batchIds.value = ids
  batchFailures.value = []
  try {
    const result = await http.post<ItemBatchResult>('/api/items/manage', { action, ids })
    batchFailures.value = result.failed
    if (result.count) toast.success(`${result.count} 条任务已${actionLabels[action]}`)
    if (result.failed.length) toast.error(`${result.failed.length} 条任务操作失败，请查看具体原因`)
    else if (!result.count) toast.info('任务状态已更新，无需重复操作')
    // 保留未执行与失败的选中项；仅移除已成功的可操作条目。
    const failedIds = new Set(result.failed.map(x => x.id))
    selected.value = selected.value.filter(id => !ids.includes(id) || failedIds.has(id))
    selecting.value = selected.value.length > 0
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    await store.refresh()
    batchRunning.value = null
    batchIds.value = []
  }
}

async function control(item: Item, action: 'download' | 'pause' | 'resume') {
  if (batchIds.value.includes(item.id)) return
  await run(item.id, async () => {
    try {
      const result = await http.post<{ changed: boolean }>(`/api/items/${item.id}/${action}`)
      if (result.changed) toast.success(`已${actionLabels[action]}`)
      else toast.info('任务已处于该状态')
    } catch (e) {
      toast.error(errorMessage(e))
    }
    await store.refresh()
  })
}

async function remove(item: Item) {
  if (isBusy(item.id) || batchIds.value.includes(item.id)) return
  const ok = await confirmDialog('移除记录', '只删除这条记录，不会删除 NAS 上已保存的文件。', {
    danger: true,
    okText: '移除',
  })
  if (!ok) return
  await run(item.id, async () => {
    try {
      await http.delete(`/api/items/${item.id}`)
      toast.success('已移除记录')
    } catch (e) {
      toast.error(errorMessage(e))
    }
    await store.refresh()
  })
}

/** 播放：同番已保存的资源按季/集排序成连播列表 */
function play(item: Item) {
  const series = item.anime_title || item.title
  const saved = store.items.value.filter(
    (x) => x.status === '已保存' && x.file_path && (x.anime_title || x.title) === series,
  )
  const toEntry = (x: Item): PlayerEntry => ({
    id: x.id,
    title: series,
    season: x.season > 0 ? x.season : 1,
    episode: x.episode,
    fansub_group: x.fansub_group,
    quality: x.quality,
  })
  saved.sort((a, b) => (a.season - b.season) || (epNo(a) - epNo(b)))
  const entries = saved.map(toEntry)
  dialogs.openPlayer(entries.length ? entries : [toEntry(item)], item.id)
}

function epNo(x: Item): number {
  const n = Number.parseFloat(x.episode)
  return Number.isFinite(n) ? n : Number.MAX_SAFE_INTEGER
}
</script>

<template>
  <div class="page">
    <PageHeader title="最近条目" description="最新 100 条，每 5 秒自动刷新">
      <SearchInput v-model="search" placeholder="搜索番剧、原始标题、字幕组" width="300px" />
      <BaseButton :icon="ListChecks" :variant="selecting ? 'soft' : 'default'" :disabled="!store.items.value.length || !!batchRunning" @click="toggleMode">{{ selecting ? '完成管理' : '批量管理' }}</BaseButton>
    </PageHeader>

    <DownloadControls />

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
    <div v-if="selecting" class="batch-toolbar">
      <label><input type="checkbox" :checked="allSelected" :disabled="!!batchRunning" :indeterminate="!allSelected && list.some(x => selected.includes(x.id))" @change="selectVisible" /> 全选当前结果</label>
      <span>已选 {{ selected.length }} 条</span>
      <BaseButton size="sm" :icon="Download" :loading="batchRunning === 'download'" :disabled="!downloadable || !!batchRunning" @click="batch('download')">下载 {{ downloadable }}</BaseButton>
      <BaseButton size="sm" :icon="Pause" :loading="batchRunning === 'pause'" :disabled="!pausable || !!batchRunning" @click="batch('pause')">暂停 {{ pausable }}</BaseButton>
      <BaseButton size="sm" :icon="Play" :loading="batchRunning === 'resume'" :disabled="!resumable || !!batchRunning" @click="batch('resume')">继续 {{ resumable }}</BaseButton>
      <BaseButton size="sm" variant="danger" :icon="Trash2" :loading="batchRunning === 'delete'" :disabled="!selected.length || !!batchRunning" @click="batch('delete')">批量移除</BaseButton>
    </div>
    <div v-if="batchFailures.length" class="batch-errors" role="alert">
      <p v-for="failure in batchFailures" :key="failure.id">条目 #{{ failure.id }}：{{ failure.error }}</p>
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
        :busy="isBusy(item.id) || batchIds.includes(item.id)"
        :selecting="selecting"
        :selected="selected.includes(item.id)"
        :style="{ '--i': i }"
        @download="control(item, 'download')"
        @pause="control(item, 'pause')"
        @resume="control(item, 'resume')"
        @storage="dialogs.openStorage(item.anime_title || item.title)"
        @correct="dialogs.openCorrect(item)"
        @remove="remove(item)"
        @play="play(item)"
        @toggle="toggle(item.id)"
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
.batch-toolbar { display: flex; align-items: center; flex-wrap: wrap; gap: 12px; margin-bottom: 16px; padding: 10px 12px; border: 1px solid var(--line); border-radius: var(--r-md); color: var(--text-2); font-size: 13px; }
.batch-errors { display: grid; gap: 5px; padding: 12px; margin-bottom: 16px; background: var(--red-soft); border: 1px solid var(--red-border); border-radius: var(--r-md); color: var(--red); font-size: 12px; line-height: 1.6; overflow-wrap: anywhere; }
.select-box { display: grid; place-items: center; padding-top: 8px; }
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
