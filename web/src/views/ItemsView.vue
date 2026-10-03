<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { watchDebounced } from '@vueuse/core'
import { RouterLink } from 'vue-router'
import { ChevronLeft, ChevronRight, Download, FilterX, Inbox, ListChecks, Pause, Play, RefreshCw, SearchX, ServerOff, Trash2, X } from '@lucide/vue'
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

const filter = store.itemFilter
const selecting = ref(false)
const selected = ref<number[]>([])
const listHeading = ref<HTMLElement | null>(null)
const refreshing = ref(false)
const refreshMessage = ref('')
type BatchAction = 'download' | 'pause' | 'resume' | 'delete'
const batchRunning = ref<BatchAction | null>(null)
const batchIds = ref<number[]>([])
const batchFailures = ref<ItemBatchResult['failed']>([])
const selectedItems = computed(() => list.value.filter(x => selected.value.includes(x.id)))
const actionLabels = { download: '加入下载队列', pause: '暂停', resume: '加入继续队列', delete: '移除' }
function actionable(action: BatchAction) {
  return selectedItems.value.filter(x => !isBusy(x.id) && (action === 'delete' ||
    (action === 'download' ? isRetryable(x.status) : action === 'pause' ? isPausable(x.status) : isResumable(x.status))))
}
const downloadable = computed(() => actionable('download').length)
const pausable = computed(() => actionable('pause').length)
const resumable = computed(() => actionable('resume').length)
const search = ref(store.search.value)
const queryPending = computed(() => search.value.trim() !== store.search.value || !store.loaded.value)
const navigationDisabled = computed(() => queryPending.value || refreshing.value || !!batchRunning.value)
const hasFilters = computed(() => filter.value !== '全部' || !!search.value.trim())
function clearSelection() { selected.value = []; batchFailures.value = [] }
watch(search, () => { if (!batchRunning.value) clearSelection() }, { flush: 'sync' })
watch([filter, store.search], () => {
  clearSelection()
  refreshMessage.value = ''
  store.resetItemPage()
  void store.load()
})
watch(store.pageNumber, clearSelection, { flush: 'sync' })
function applySearch() {
  if (batchRunning.value) return
  store.search.value = search.value.trim()
}
watchDebounced(search, applySearch, { debounce: 350 })
function clearFilters() {
  if (batchRunning.value) return
  search.value = ''
  store.search.value = ''
  filter.value = '全部'
}
async function refreshItems() {
  if (refreshing.value || batchRunning.value) return
  refreshing.value = true
  refreshMessage.value = ''
  try {
    await store.refresh()
    if (!store.loadError.value) refreshMessage.value = '列表已更新'
  } finally { refreshing.value = false }
}
async function changePage(direction: 'next' | 'previous' | 'first') {
  if (navigationDisabled.value) return
  if (direction === 'next' && !store.nextBefore.value) return
  if (direction !== 'next' && store.pageNumber.value <= 1) return
  clearSelection()
  if (direction === 'first') { store.resetItemPage(); await store.load() }
  else if (direction === 'next') await store.nextPage()
  else await store.previousPage()
  await nextTick()
  listHeading.value?.focus({ preventScroll: true })
  listHeading.value?.scrollIntoView({ block: 'start' })
}

const chips = computed(() =>
  CHIP_DEFS.map((d) => ({ key: d.key, n: d.key === '全部' ? store.totalItems.value : chipCount(d, store.counts.value) })),
)

const list = computed(() => {
  const def = CHIP_DEFS.find((d) => d.key === filter.value) ?? CHIP_DEFS[0]!
  return store.items.value.filter((x) => def.match(x.status || '待下载'))
})
const allSelected = computed(() => list.value.length > 0 && list.value.every(x => selected.value.includes(x.id)))
watch(list, (items) => {
  if (!batchRunning.value) selected.value = selected.value.filter(id => items.some(x => x.id === id))
})
function toggle(id: number) {
  if (batchRunning.value || queryPending.value || isBusy(id)) return
  selected.value = selected.value.includes(id) ? selected.value.filter(x => x !== id) : [...selected.value, id]
}
function selectVisible() {
  if (batchRunning.value || queryPending.value) return
  selected.value = allSelected.value ? [] : list.value.filter(x => !isBusy(x.id)).map(x => x.id)
}
function toggleMode() {
  if (batchRunning.value) return
  selecting.value = !selecting.value
  clearSelection()
}
async function batch(action: BatchAction) {
  if (batchRunning.value || queryPending.value) return
  const ids = actionable(action).map(x => x.id)
  if (!ids.length) return
  batchRunning.value = action
  batchIds.value = ids
  let submitted = false
  try {
    if (action === 'delete' && !await confirmDialog('批量移除记录', `确定移除选中的 ${ids.length} 条记录？不会删除已保存文件。`, { danger: true, okText: '移除' })) return
    batchFailures.value = []
    submitted = true
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
    if (submitted) await store.refresh()
    batchRunning.value = null
    batchIds.value = []
    selected.value = selected.value.filter(id => list.value.some(x => x.id === id))
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
  const ok = await confirmDialog('移除记录', `确定移除「${item.anime_title || item.title}」${item.episode ? `第 ${item.episode} 集` : ''}？不会删除已保存的文件。`, {
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
    <PageHeader title="最近条目" description="查看下载进度，管理每一集的保存状态">
      <BaseButton :icon="RefreshCw" :loading="refreshing" :disabled="queryPending || !!batchRunning" @click="refreshItems">刷新</BaseButton>
      <BaseButton :icon="ListChecks" :variant="selecting ? 'soft' : 'default'" :disabled="(!list.length && !selecting) || !!batchRunning" :aria-pressed="selecting" @click="toggleMode">{{ selecting ? '完成管理' : '批量管理' }}</BaseButton>
    </PageHeader>

    <DownloadControls />

    <section class="item-filters" aria-label="搜索与筛选">
      <div class="search-line">
        <fieldset :disabled="!!batchRunning" class="search-field">
          <SearchInput v-model="search" placeholder="搜索番剧、原始标题、字幕组" width="100%" @enter="applySearch" />
        </fieldset>
        <BaseButton v-if="hasFilters" size="sm" variant="ghost" :icon="FilterX" :disabled="!!batchRunning" @click="clearFilters">清除筛选</BaseButton>
        <span class="filter-note">状态数量为全库统计</span>
      </div>
      <div class="chips" role="group" aria-label="状态筛选">
      <button
        v-for="c in chips"
        :key="c.key"
        type="button"
        class="chip"
        :class="{ active: filter === c.key }"
        :aria-pressed="filter === c.key"
        :disabled="!!batchRunning"
        @click="filter = c.key"
      >
        {{ c.key }}
        <span class="n"><AnimatedNumber v-if="store.status.value" :value="c.n" /><span v-else>—</span></span>
      </button>
      </div>
    </section>

    <div ref="listHeading" class="list-heading" tabindex="-1" aria-label="条目列表">
      <div class="result-summary" role="status">
        <span v-if="queryPending">正在加载条目…</span>
        <span v-else-if="store.loadError.value && !store.items.value.length">条目加载失败</span>
        <span v-else>第 {{ store.pageNumber.value }} 页 · 本页 <b>{{ list.length }}</b> 条</span>
        <span class="update-note">{{ store.loadError.value ? '更新失败，可点击刷新重试' : refreshMessage || '每 5 秒自动更新' }}</span>
      </div>
      <nav v-if="store.pageNumber.value > 1 || store.nextBefore.value" class="page-shortcuts" aria-label="顶部翻页">
        <BaseButton v-if="store.pageNumber.value > 1" size="sm" variant="ghost" :disabled="navigationDisabled" @click="changePage('first')">回到首页</BaseButton>
        <BaseButton size="sm" icon-only :icon="ChevronLeft" aria-label="上一页" :disabled="store.pageNumber.value <= 1 || navigationDisabled" @click="changePage('previous')" />
        <BaseButton size="sm" icon-only :icon="ChevronRight" aria-label="下一页" :disabled="!store.nextBefore.value || navigationDisabled" @click="changePage('next')" />
      </nav>
    </div>

    <div v-if="selecting" class="batch-toolbar" role="region" aria-label="批量操作">
      <div class="batch-selection">
        <label><input type="checkbox" :checked="allSelected" :disabled="queryPending || !list.length || !!batchRunning" :indeterminate="!allSelected && selected.length > 0" @change="selectVisible" /> 全选当前页</label>
        <span role="status">已选 <b>{{ selected.length }}</b> 条</span>
        <BaseButton size="sm" variant="ghost" aria-label="取消选择" :disabled="!selected.length || !!batchRunning" @click="selected = []">清空</BaseButton>
        <BaseButton size="sm" variant="ghost" icon-only :icon="X" aria-label="退出批量管理" :disabled="!!batchRunning" @click="toggleMode" />
      </div>
      <div class="batch-actions">
        <BaseButton size="sm" :icon="Download" :loading="batchRunning === 'download'" :disabled="!downloadable || queryPending || !!batchRunning" @click="batch('download')">下载 / 重试 {{ downloadable }}</BaseButton>
        <BaseButton size="sm" :icon="Pause" :loading="batchRunning === 'pause'" :disabled="!pausable || queryPending || !!batchRunning" @click="batch('pause')">暂停 {{ pausable }}</BaseButton>
        <BaseButton size="sm" :icon="Play" :loading="batchRunning === 'resume'" :disabled="!resumable || queryPending || !!batchRunning" @click="batch('resume')">继续 {{ resumable }}</BaseButton>
        <BaseButton size="sm" variant="danger" :icon="Trash2" :loading="batchRunning === 'delete'" :disabled="!selectedItems.length || queryPending || !!batchRunning" @click="batch('delete')">移除记录</BaseButton>
      </div>
      <p class="batch-hint">{{ batchRunning ? '正在处理选中的条目，请稍候…' : '仅操作本页选中且符合状态的条目；移除记录保留已保存文件。' }}</p>
    </div>
    <div v-if="batchFailures.length" class="batch-errors" role="alert">
      <strong>{{ batchFailures.length }} 条操作失败，请检查原因后重试</strong>
      <p v-for="failure in batchFailures" :key="failure.id">{{ list.find(x => x.id === failure.id)?.title || `条目 #${failure.id}` }}：{{ failure.error }}</p>
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
      <BaseButton variant="primary" :loading="refreshing" @click="refreshItems">重新加载</BaseButton>
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
      v-else-if="!store.items.value.length && filter === '全部' && store.pageNumber.value === 1"
      :icon="Inbox"
      title="暂无条目"
      hint="添加 RSS 订阅并同步后，新番会出现在这里"
    >
      <RouterLink class="empty-link" to="/feeds">前往订阅管理</RouterLink>
    </EmptyState>

    <EmptyState
      v-else-if="!list.length && store.pageNumber.value > 1"
      :icon="Inbox"
      title="这一页已没有条目"
      hint="条目可能已被移除或状态发生变化，可返回首页查看"
      compact
    >
      <BaseButton @click="changePage('first')">回到首页</BaseButton>
    </EmptyState>

    <EmptyState
      v-else-if="!list.length"
      :icon="FilterX"
      title="当前筛选下没有条目"
      hint="换个状态筛选，或清除搜索关键词"
      compact
    >
      <BaseButton @click="clearFilters">查看全部</BaseButton>
    </EmptyState>

    <TransitionGroup v-else tag="div" class="list" name="list" appear>
      <ItemRow
        v-for="(item, i) in list"
        :key="item.id"
        :item="item"
        :busy="isBusy(item.id) || batchIds.includes(item.id)"
        :selecting="selecting"
        :selected="selected.includes(item.id)"
        :selection-disabled="!!batchRunning || queryPending"
        :style="{ '--i': Math.min(i, 8) }"
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
    <nav v-if="list.length || store.pageNumber.value > 1" class="pagination" aria-label="条目分页">
      <BaseButton :icon="ChevronLeft" :disabled="store.pageNumber.value <= 1 || navigationDisabled" @click="changePage('previous')">上一页</BaseButton>
      <span>第 {{ store.pageNumber.value }} 页<span class="page-size"> · 每页最多 100 条</span></span>
      <BaseButton :icon="ChevronRight" :disabled="!store.nextBefore.value || navigationDisabled" @click="changePage('next')">下一页</BaseButton>
    </nav>
  </div>
</template>

<style scoped>
.item-filters { padding: 16px; margin-bottom: 16px; border: 1px solid var(--line); border-radius: var(--r-lg); background: var(--surface); }
.search-line { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 12px; margin-bottom: 14px; }
.search-field { flex: 1; min-width: min(100%, 260px); max-width: 460px; padding: 0; margin: 0; border: 0; }
.filter-note { margin-left: auto; color: var(--text-3); font-size: 12px; }
.chips {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
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
.chip:disabled { cursor: not-allowed; opacity: 0.6; }
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
.list-heading { display: flex; align-items: center; justify-content: space-between; gap: 10px; flex-wrap: wrap; margin: 16px 0 12px; scroll-margin-top: 20px; }
.result-summary { display: flex; flex-wrap: wrap; gap: 6px 14px; color: var(--text-2); font-size: 13px; }
.update-note { color: var(--text-3); font-size: 12px; }
.page-shortcuts { display: flex; gap: 6px; align-items: center; margin-left: auto; }
.batch-toolbar { position: sticky; top: 12px; z-index: 20; display: flex; align-items: center; flex-wrap: wrap; gap: 8px 16px; margin-bottom: 14px; padding: 10px 12px; border: 1px solid var(--accent-border); border-radius: var(--r-md); background: var(--surface); box-shadow: var(--shadow-sm); color: var(--text-2); font-size: 13px; }
.batch-selection, .batch-actions { display: flex; align-items: center; flex-wrap: wrap; gap: 6px 10px; }
.batch-selection label { display: inline-flex; align-items: center; gap: 8px; cursor: pointer; min-height: 32px; }
.batch-actions { margin-left: auto; }
.batch-hint { flex-basis: 100%; color: var(--text-3); font-size: 12px; line-height: 1.5; }
.batch-errors { display: grid; gap: 5px; padding: 12px; margin-bottom: 16px; background: var(--red-soft); border: 1px solid var(--red-border); border-radius: var(--r-md); color: var(--red); font-size: 12px; line-height: 1.6; overflow-wrap: anywhere; }
.empty-link { color: var(--accent); font-size: 13px; padding: 8px 12px; }
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
.pagination { display: flex; flex-wrap: wrap; align-items: center; justify-content: center; gap: 12px; margin: 20px 0; }
.pagination span { font-size: 13px; }
@media (max-width: 900px) {
  .batch-toolbar { top: calc(76px + env(safe-area-inset-top)); }
  .list-heading { scroll-margin-top: calc(90px + env(safe-area-inset-top)); }
}
@media (max-width: 640px) {
  .item-filters { padding: 12px; }
  .search-field { flex-basis: 100%; max-width: none; }
  .filter-note { font-size: 11px; }
  .chip { height: 34px; padding-left: 10px; font-size: 12px; }
  .batch-selection { width: 100%; justify-content: space-between; gap: 4px; }
  .batch-actions { margin-left: 0; width: 100%; display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 6px; }
  .batch-hint { display: none; }
  .pagination { gap: 10px; }
  .page-size { display: none; }
}
</style>
