<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref, watch } from 'vue'
import { useEventListener, useIntervalFn, useResizeObserver } from '@vueuse/core'
import { useRoute, useRouter, type LocationQuery } from 'vue-router'
import { Clapperboard, SearchX, Plus, ListChecks, Merge, Trash2, ServerOff, CalendarSearch, LayoutGrid, Rows3, CalendarDays } from '@lucide/vue'
import PageHeader from '@/components/ui/PageHeader.vue'
import SearchInput from '@/components/ui/SearchInput.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import SkeletonBlock from '@/components/ui/SkeletonBlock.vue'
import BaseButton from '@/components/ui/BaseButton.vue'
import AnimeCard from '@/components/library/AnimeCard.vue'
import { http, errorMessage } from '@/api/client'
import { BANGUMI_COLLECTION_LABELS, type LibraryAnime, type BangumiCollectionState, type BangumiCollectionType } from '@/api/types'
import { useAppStore } from '@/stores/app'
import { useDialogs } from '@/stores/dialogs'
import { localDate, weekdayOfDate, weekdays, effectiveWeekday } from '@/components/library/schedule'
import { hasOpenDialog } from '@/composables/dialogStack'

const store = useAppStore()
const dialogs = useDialogs()
const router = useRouter()
const route = useRoute()

const wishes = ref<BangumiCollectionState | null>(null)
const wishError = ref('')
let wishController: AbortController | undefined
async function loadWishes() {
  wishController?.abort()
  const controller = new AbortController()
  wishController = controller
  try {
    const state = await http.get<BangumiCollectionState>('/api/bangumi/wishes', { signal: controller.signal })
    if (controller.signal.aborted) return
    wishes.value = state
    wishError.value = ''
  } catch (e) {
    if (!controller.signal.aborted) wishError.value = errorMessage(e)
  }
}
onMounted(loadWishes)
useIntervalFn(loadWishes, 30000)
onBeforeUnmount(() => wishController?.abort())
const library = computed<LibraryAnime[]>(() => {
  const collections = new Map((wishes.value?.items ?? []).map(w => [w.id, w]))
  const local = store.animes.value.map(a => ({ ...a, cover: a.cover || collections.get(a.bangumi_id)?.cover || '', rating: collections.get(a.bangumi_id)?.rating || a.rating, collectionType: collections.get(a.bangumi_id)?.type }))
  const bound = new Set(local.map(a => a.bangumi_id).filter(Boolean))
  const pending = (wishes.value?.items ?? []).filter(w => !bound.has(w.id)).map(w => ({
    id: -w.id, bangumi_id: w.id, title: w.name || w.name_cn, meta_name: w.name, meta_name_cn: w.name_cn,
    cover: w.cover || '', cloudOnly: true, collectionType: w.type, search_name: '', season: 1, storage_dir: '', include_rules: '', exclude_rules: '',
    updated_at: '', update_weekday: 0, item_count: 0, saved_count: 0, episode_count: 0, rating: w.rating || 0, summary: '',
  }))
  return [...local, ...pending]
})

/* 筛选写入地址查询参数：刷新、从其他页面返回或分享链接时保持当前视图。 */
const SCOPES = ['all', 'empty', 'saved']
const SORTS = ['recent', 'title', 'resources', 'rating', 'weekday']
const COLLECTIONS = ['all', '1', '3', '2', '4', '5', 'none']
function pick(query: LocationQuery, key: string) {
  const value = query[key]
  return typeof value === 'string' ? value : ''
}
function readQuery(query: LocationQuery) {
  const day = pick(query, 'day')
  const date = pick(query, 'date')
  return {
    q: pick(query, 'q'),
    collection: COLLECTIONS.includes(pick(query, 'collection')) ? pick(query, 'collection') : 'all',
    scope: SCOPES.includes(pick(query, 'scope')) ? pick(query, 'scope') : 'all',
    sort: SORTS.includes(pick(query, 'sort')) ? pick(query, 'sort') : 'recent',
    day: /^[0-7]$/.test(day) ? Number(day) : -1,
    date: weekdayOfDate(date) ? date : '',
  }
}
function writeQuery(state: ReturnType<typeof readQuery>) {
  const next: Record<string, string> = {}
  if (state.q) next.q = state.q
  if (state.collection !== 'all') next.collection = state.collection
  if (state.scope !== 'all') next.scope = state.scope
  if (state.sort !== 'recent') next.sort = state.sort
  if (state.date) next.date = state.date
  else if (state.day !== -1) next.day = String(state.day)
  return next
}
const initial = readQuery(route.query)
const filter = ref(initial.q)
const selecting = ref(false)
const selected = ref<number[]>([])
const scope = ref(initial.scope)
const collection = ref(initial.collection)
const sort = ref(initial.sort)
const weekday = ref(initial.date ? -1 : initial.day)
const date = ref(initial.date)
const dateOpen = ref(!!initial.date)
watch([filter, collection, scope, sort, weekday, date], () => {
  const next = writeQuery({ q: filter.value.trim(), collection: collection.value, scope: scope.value, sort: sort.value, day: weekday.value, date: date.value })
  if (JSON.stringify(next) !== JSON.stringify(writeQuery(readQuery(route.query)))) void router.replace({ query: next })
})

/* 展示方式是个人偏好，保存在本机浏览器。 */
const LAYOUT_KEY = 'auto-anime:library-layout'
function storedLayout(): 'compact' | 'poster' {
  try { return globalThis.localStorage?.getItem(LAYOUT_KEY) === 'poster' ? 'poster' : 'compact' } catch { return 'compact' }
}
const layout = ref<'compact' | 'poster'>(storedLayout())
watch(layout, value => {
  try { globalThis.localStorage?.setItem(LAYOUT_KEY, value) } catch { /* 无法写入时只在本次生效 */ }
})
const hasFilters = computed(() => !!filter.value || scope.value !== 'all' || collection.value !== 'all' || activeWeekday.value !== -1)
const resultTitle = computed(() => activeWeekday.value === -1 ? '全部番剧' : activeWeekday.value === 0 ? '更新日待补充' : `${weekdays[activeWeekday.value]}追番`)
const activeWeekday = computed(() => date.value ? weekdayOfDate(date.value) ?? -1 : weekday.value)
const weekdayList = ref<HTMLElement | null>(null)
const weekdayIndicator = ref<{ transform: string; width: string; height: string }>()
function updateWeekdayIndicator() {
  const button = weekdayList.value?.querySelector<HTMLElement>('button[aria-pressed="true"]')
  if (!button) return
  weekdayIndicator.value = { transform: `translateX(${button.offsetLeft}px)`, width: `${button.offsetWidth}px`, height: `${button.offsetHeight}px` }
}
useResizeObserver(weekdayList, updateWeekdayIndicator)
function chooseDay(day: number) { date.value = ''; weekday.value = day }
function showToday() { date.value = localDate(); weekday.value = -1; dateOpen.value = true }
function clearFilters() { filter.value = ''; scope.value = 'all'; collection.value = 'all'; chooseDay(-1) }
watch([filter, scope, collection, sort, weekday, date], () => { selected.value = [] })
const chosen = computed(() => store.animes.value.filter(a => selected.value.includes(a.id)))
function toggle(id: number) { selected.value = selected.value.includes(id) ? selected.value.filter(x => x !== id) : [...selected.value, id] }
function toggleMode() { selecting.value = !selecting.value; selected.value = [] }
const searched = computed(() => {
  const q = filter.value.trim().toLowerCase()
  return library.value.filter(a => (scope.value !== 'empty' || !a.item_count) && (scope.value !== 'saved' || a.saved_count > 0))
    .filter(a => !q || [a.title, a.meta_name_cn, a.meta_name, a.search_name].some(s => s && s.toLowerCase().includes(q)))
})
function inCollection(a: LibraryAnime, key: string) {
  return key === 'all' || (key === 'none' ? !a.collectionType : String(a.collectionType) === key)
}
function collectionLabel(key: string) {
  return key === 'all' ? '全部' : key === 'none' ? '未收藏' : BANGUMI_COLLECTION_LABELS[Number(key) as BangumiCollectionType]
}
/* 计数跟随搜索、资源范围和星期，不受当前收藏标签影响，便于横向比较。 */
const collectionCounts = computed<Record<string, number>>(() => {
  const pool = searched.value.filter(a => activeWeekday.value === -1 || effectiveWeekday(a) === activeWeekday.value)
  return Object.fromEntries(COLLECTIONS.map(key => [key, pool.filter(a => inCollection(a, key)).length]))
})
/* 没有 Bangumi 收藏时不显示收藏标签；空分类隐藏，但当前选中的保留以便切回。 */
const collectionTabs = computed(() => library.value.some(a => a.collectionType)
  ? COLLECTIONS.filter(key => key === 'all' || key === collection.value || collectionCounts.value[key])
  : COLLECTIONS.filter(key => key === collection.value && key !== 'all'))
const matching = computed(() => {
  const all = searched.value.filter(a => inCollection(a, collection.value))
  const name = (a: LibraryAnime) => a.meta_name_cn || a.title
  if (sort.value === 'title') all.sort((a, b) => name(a).localeCompare(name(b), 'zh-CN'))
  if (sort.value === 'resources') all.sort((a, b) => b.item_count - a.item_count)
  if (sort.value === 'rating') all.sort((a, b) => (b.rating || 0) - (a.rating || 0))
  if (sort.value === 'weekday') all.sort((a, b) => (effectiveWeekday(a) || 8) - (effectiveWeekday(b) || 8))
  return all
})
const dayCounts = computed(() => weekdays.map((_, day) => matching.value.filter(a => effectiveWeekday(a) === day).length))
watch([activeWeekday, dayCounts, store.loaded, weekdayList], updateWeekdayIndicator, { flush: 'post' })
const list = computed(() => matching.value.filter(a => activeWeekday.value === -1 || effectiveWeekday(a) === activeWeekday.value))
watch(list, all => { selected.value = selected.value.filter(id => all.some(a => a.id === id && !a.cloudOnly)) })
const selectable = computed(() => list.value.filter(a => !a.cloudOnly))
const allSelected = computed(() => selectable.value.length > 0 && selectable.value.every(a => selected.value.includes(a.id)))
function selectVisible() {
  selected.value = allSelected.value ? selected.value.filter(id => !selectable.value.some(a => a.id === id)) : [...new Set([...selected.value, ...selectable.value.map(a => a.id)])]
}
function openAnime(id: number) {
  const anime = library.value.find(a => a.id === id)
  if (!anime) return
  dialogs.openAnimeDetail(id, anime)
}

/* 「/」聚焦搜索，Esc 退出批量管理；弹窗打开或正在输入时不拦截。 */
const search = ref<{ focus: () => void } | null>(null)
function typing(target: EventTarget | null) {
  const el = target as HTMLElement | null
  return !!el && (el.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(el.tagName))
}
function onKeydown(event: KeyboardEvent) {
  if (event.defaultPrevented || event.ctrlKey || event.metaKey || event.altKey || hasOpenDialog() || typing(event.target)) return
  if (event.key === '/') {
    event.preventDefault()
    search.value?.focus()
  } else if (event.key === 'Escape' && selecting.value) {
    toggleMode()
  }
}
useEventListener(window, 'keydown', onKeydown)
</script>

<template>
  <div class="page library-page">
    <PageHeader
      title="番剧库"
      :count="store.loaded.value ? library.length : undefined"
    >
      <BaseButton :icon="ListChecks" :variant="selecting ? 'soft' : 'default'" :disabled="!store.animes.value.length" @click="toggleMode">{{ selecting ? '完成管理' : '批量管理' }}</BaseButton>
      <BaseButton :icon="Plus" variant="primary" @click="dialogs.openAnimeEdit()">新建番剧</BaseButton>
    </PageHeader>

    <p v-if="wishError || wishes?.error" class="wish-error" role="alert">Bangumi 收藏同步异常：{{ wishError || wishes?.error }}；已有番剧不受影响。<button type="button" class="reset-button" @click="loadWishes">重新读取</button><RouterLink to="/settings?tab=bangumi">前往设置</RouterLink></p>

    <section class="schedule-panel" aria-label="每周追番安排">
      <div class="schedule-heading"><CalendarDays aria-hidden="true" />每周追番</div>
      <div ref="weekdayList" class="weekday-list" role="group" aria-label="按更新日筛选">
        <span v-show="weekdayIndicator" class="weekday-indicator" :style="weekdayIndicator" aria-hidden="true" />
        <button type="button" :aria-pressed="activeWeekday === -1" @click="chooseDay(-1)">全部<span>{{ store.loaded.value ? matching.length : '—' }}</span></button>
        <button v-for="day in [1, 2, 3, 4, 5, 6, 7, 0]" :key="day" type="button" :aria-pressed="activeWeekday === day" @click="chooseDay(day)">{{ weekdays[day] }}<span>{{ store.loaded.value ? dayCounts[day] : '—' }}</span></button>
      </div>
      <button type="button" class="today-button" @click="showToday">今天</button>
    </section>

    <div class="library-toolbar">
      <div class="results-heading">
        <h2>{{ resultTitle }}</h2><span role="status">{{ store.loaded.value ? `${list.length} 部` : '加载中…' }}</span><span v-if="date" class="date-context">{{ date }}</span>
        <button v-if="hasFilters" type="button" class="reset-button" @click="clearFilters">清除筛选</button>
      </div>
      <SearchInput ref="search" v-model="filter" placeholder="搜索番剧名称" width="240px" />
      <select v-model="scope" class="scope-select" aria-label="资源范围"><option value="all">全部资源</option><option value="empty">无资源</option><option value="saved">有已保存</option></select>
      <BaseButton size="sm" :icon="CalendarSearch" :variant="dateOpen || date ? 'soft' : 'ghost'" :aria-expanded="dateOpen" aria-controls="library-filters" @click="dateOpen = !dateOpen">按日期</BaseButton>
      <select v-model="sort" class="sort-select" aria-label="排序"><option value="recent">最近收录</option><option value="title">名称</option><option value="resources">资源数量</option><option value="rating">评分</option><option value="weekday">更新日</option></select>
      <div class="view-switch" role="group" aria-label="番剧展示方式">
        <button type="button" :aria-pressed="layout === 'compact'" aria-label="信息卡片" title="信息卡片" @click="layout = 'compact'"><Rows3 aria-hidden="true" /></button>
        <button type="button" :aria-pressed="layout === 'poster'" aria-label="海报墙" title="海报墙" @click="layout = 'poster'"><LayoutGrid aria-hidden="true" /></button>
      </div>
    </div>
    <div v-if="collectionTabs.length > 1" class="collection-tabs" role="group" aria-label="按收藏状态筛选">
      <button v-for="key in collectionTabs" :key="key" type="button" :aria-pressed="collection === key" @click="collection = key">
        {{ collectionLabel(key) }}<span>{{ store.loaded.value ? collectionCounts[key] : '—' }}</span>
      </button>
    </div>
    <div v-show="dateOpen" id="library-filters" class="extra-filters">
      <label for="library-date">选择日期 <input id="library-date" v-model="date" type="date" @change="weekday = -1" /></label>
      <p>按每周放送星期筛选，可在「编辑番剧」中调整；不代表实际到片或仍在连载。</p>
    </div>
    <div v-if="selecting" class="batch-toolbar">
      <label class="select-all"><input type="checkbox" :checked="allSelected" :indeterminate="!allSelected && list.some(a => selected.includes(a.id))" @change="selectVisible" />全选当前结果</label>
      <span>已选 {{ chosen.length }} 部</span><span class="batch-hint">Esc 退出</span>
      <BaseButton size="sm" :icon="Merge" :disabled="!chosen.length || chosen.length > 500 || store.animes.value.length < 2" @click="dialogs.openAnimeManage('merge', chosen)">合并</BaseButton>
      <BaseButton size="sm" variant="danger" :icon="Trash2" :disabled="!chosen.length || chosen.length > 500" @click="dialogs.openAnimeManage('delete', chosen)">删除</BaseButton>
    </div>

    <div v-if="!store.loaded.value" class="grid" :class="layout" aria-busy="true">
      <div v-for="i in 8" :key="i" class="sk-card" :style="{ '--i': i }">
        <SkeletonBlock height="auto" :width="layout === 'compact' ? '100px' : '100%'" :radius="layout === 'compact' ? 'var(--r-sm)' : '0'" class="sk-poster" />
        <div class="sk-body">
          <SkeletonBlock height="14px" width="80%" />
          <SkeletonBlock height="11px" width="55%" />
        </div>
      </div>
    </div>

    <EmptyState v-else-if="store.loadError.value && !library.length" :icon="ServerOff" title="番剧库加载失败" hint="请检查服务连接后重新加载" tone="error" compact>
      <BaseButton @click="store.refresh()">重新加载</BaseButton>
    </EmptyState>
    <EmptyState
      v-else-if="!library.length"
      :icon="Clapperboard"
      title="番剧库还是空的"
      hint="添加 RSS 订阅并同步后，识别出的番剧会自动出现在这里"
      compact
    >
      <BaseButton variant="primary" @click="router.push('/feeds')">去添加订阅</BaseButton>
      <BaseButton @click="router.push('/discover')">去搜索</BaseButton>
    </EmptyState>

    <EmptyState v-else-if="!list.length" :icon="SearchX" title="没有匹配的番剧" hint="试试其他关键词，或清除筛选" compact>
      <BaseButton @click="clearFilters">清除筛选</BaseButton>
    </EmptyState>

    <TransitionGroup v-else tag="div" class="grid" :class="layout" name="list" appear>
      <AnimeCard
        v-for="(a, i) in list"
        :key="a.id"
        :anime="a"
        :cloud-only="a.cloudOnly"
        :collection-type="a.collectionType"
        :compact="layout === 'compact'"
        :selecting="selecting"
        :selected="selected.includes(a.id)"
        :style="{ '--i': i }"
        @open="openAnime"
        @toggle="toggle"
      />
    </TransitionGroup>
  </div>
</template>

<style scoped>
.wish-error { margin: 0 0 16px; color: var(--red); font-size: 13px; }
.library-page :deep(.page-head) { margin-bottom: 22px; padding-bottom: 0; border: 0; }
.schedule-panel { display: flex; align-items: center; gap: 12px; padding: 10px 12px; margin-bottom: 14px; border: 1px solid var(--line); border-radius: var(--r-lg); background: var(--surface); }
.schedule-heading { display: flex; align-items: center; gap: 6px; flex: none; font-size: 12px; color: var(--text-2); }
.schedule-heading svg { width: 14px; height: 14px; }
.today-button, .reset-button { border: 0; background: transparent; color: var(--accent); font: inherit; font-size: 12px; cursor: pointer; padding: 6px; border-radius: var(--r-xs); }
.today-button { flex: none; white-space: nowrap; min-height: 36px; }
.weekday-list { position: relative; display: flex; flex: 1; min-width: 0; gap: 4px; overflow-x: auto; scrollbar-width: thin; padding: 3px; }
.weekday-indicator { position: absolute; top: 3px; left: 0; border: 1px solid var(--accent-border); border-radius: var(--r-sm); background: var(--accent-soft); pointer-events: none; transition: transform var(--t-med) var(--ease-out), width var(--t-med) var(--ease-out); }
.weekday-list button { position: relative; flex: 1 0 auto; display: flex; align-items: center; justify-content: center; gap: 8px; padding: 9px 10px; border: 1px solid transparent; border-radius: var(--r-sm); background: transparent; color: var(--text-2); font: inherit; font-size: 13px; cursor: pointer; white-space: nowrap; transition: color var(--t-med) var(--ease-out); }
.weekday-list button span { font-size: 11px; opacity: .7; font-variant-numeric: tabular-nums; }
.weekday-list button:hover:not([aria-pressed="true"]) { background: var(--surface-2); }
.weekday-list button[aria-pressed="true"] { color: var(--accent); font-weight: 650; }
.weekday-list button:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
.library-toolbar, .batch-toolbar { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; margin-bottom: 16px; font-size: 13px; }
.library-toolbar :deep(.search) { flex: 1; max-width: 280px; min-width: 160px; }
.sort-select { margin-left: auto; width: 116px; font-size: 12px; }
.scope-select { width: 108px; font-size: 12px; }
.collection-tabs { display: flex; gap: 6px; margin: -4px 0 16px; padding: 2px; overflow-x: auto; scrollbar-width: thin; }
.collection-tabs button { flex: none; display: inline-flex; align-items: center; gap: 6px; min-height: 32px; padding: 5px 12px; border: 1px solid var(--line); border-radius: 999px; background: var(--surface); color: var(--text-2); font: inherit; font-size: 12.5px; cursor: pointer; transition: border-color var(--t-fast), background var(--t-fast), color var(--t-fast); }
.collection-tabs button span { font-size: 11px; opacity: .7; font-variant-numeric: tabular-nums; }
.collection-tabs button:hover:not([aria-pressed="true"]) { border-color: var(--line-strong); }
.collection-tabs button[aria-pressed="true"] { border-color: var(--accent-border); background: var(--accent-soft); color: var(--accent); font-weight: 600; }
.collection-tabs button:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
.batch-hint { color: var(--text-3); font-size: 12px; }
.view-switch { display: flex; padding: 3px; border: 1px solid var(--line); background: var(--surface); border-radius: var(--r-sm); }
.view-switch button { display: grid; place-items: center; width: 34px; height: 30px; border: 0; background: transparent; color: var(--text-3); border-radius: var(--r-xs); cursor: pointer; }
.view-switch button[aria-pressed="true"] { color: var(--accent); background: var(--accent-soft); }
.view-switch svg { width: 16px; height: 16px; }
.extra-filters { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 16px; padding: 12px 16px; margin-bottom: 16px; border-radius: var(--r-md); background: var(--surface-2); }
.extra-filters label, .select-all { display: inline-flex; align-items: center; gap: 8px; font-size: 12px; }
.extra-filters input { width: 160px; min-width: 0; }
.extra-filters p { margin: 0; font-size: 12px; line-height: 1.7; color: var(--text-3); }
.results-heading { display: flex; flex-wrap: wrap; align-items: baseline; gap: 6px 8px; }
.results-heading h2 { margin: 0; font-size: 15px; font-weight: 650; }
.results-heading span { color: var(--text-3); font-size: 12px; }
.batch-toolbar { padding: 10px 12px; border: 1px solid var(--accent-border); border-radius: var(--r-sm); background: var(--surface); position: sticky; top: 12px; z-index: 5; }
.grid {
  position: relative;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 154px), 1fr));
  gap: 18px;
}
.grid.compact { grid-template-columns: repeat(auto-fill, minmax(min(100%, 310px), 1fr)); gap: 14px; }
.compact .sk-card { display: flex; padding: 12px; gap: 14px; }
.compact .sk-poster { width: 100px; flex: none; border-radius: var(--r-sm); }
.compact .sk-body { flex: 1; padding: 20px 0; align-content: center; }
@media (max-width: 640px) {
  .library-page :deep(.page-head) { gap: 12px; margin-bottom: 16px; }
  .library-page :deep(.page-desc) { margin-top: 5px; }
  .schedule-panel { flex-wrap: wrap; gap: 4px 10px; padding: 8px; margin-bottom: 12px; }
  .weekday-list { order: 1; flex-basis: 100%; }
  .today-button { margin-left: auto; }
  .weekday-list button { padding: 9px 10px; }
  .library-toolbar { gap: 8px; margin-bottom: 10px; }
  .results-heading { flex-basis: 100%; }
  .library-toolbar :deep(.search) { max-width: none; flex-basis: 100%; }
  .batch-toolbar { top: 76px; }
  .batch-hint { display: none; }
  .collection-tabs { margin: 0 0 12px; }
  .date-context { flex-basis: 100%; }
  .grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 12px;
  }
  .grid.compact { grid-template-columns: minmax(0, 1fr); gap: 10px; }
}
.sk-card {
  border: 1px solid var(--line);
  border-radius: var(--r-lg);
  overflow: hidden;
  background: var(--surface);
  animation: fade-in var(--t-slow) var(--ease-out) both;
  animation-delay: calc(var(--i) * 40ms);
}
.sk-poster {
  aspect-ratio: 2 / 3;
}
.sk-body {
  display: grid;
  gap: 8px;
  padding: 12px 13px 14px;
}
</style>
