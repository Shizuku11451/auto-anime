<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useResizeObserver } from '@vueuse/core'
import { useRouter } from 'vue-router'
import { Clapperboard, SearchX, Plus, ListChecks, Merge, Trash2, ServerOff, SlidersHorizontal, LayoutGrid, Rows3, CalendarDays } from '@lucide/vue'
import PageHeader from '@/components/ui/PageHeader.vue'
import SearchInput from '@/components/ui/SearchInput.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import SkeletonBlock from '@/components/ui/SkeletonBlock.vue'
import BaseButton from '@/components/ui/BaseButton.vue'
import AnimeCard from '@/components/library/AnimeCard.vue'
import BangumiWishes from '@/components/discover/BangumiWishes.vue'
import { useAppStore } from '@/stores/app'
import { useDialogs } from '@/stores/dialogs'
import { localDate, weekdayOfDate, weekdays, effectiveWeekday } from '@/components/library/schedule'

const store = useAppStore()
const dialogs = useDialogs()
const router = useRouter()

const filter = ref('')
const selecting = ref(false)
const selected = ref<number[]>([])
const scope = ref('all')
const sort = ref('recent')
const weekday = ref(-1)
const date = ref('')
const moreFilters = ref(false)
const layout = ref<'compact' | 'poster'>('compact')
const hasFilters = computed(() => !!filter.value || scope.value !== 'all' || activeWeekday.value !== -1)
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
function showToday() { date.value = localDate(); weekday.value = -1 }
function clearFilters() { filter.value = ''; scope.value = 'all'; chooseDay(-1) }
watch([filter, scope, sort, weekday, date], () => { selected.value = [] })
const chosen = computed(() => store.animes.value.filter(a => selected.value.includes(a.id)))
watch(() => store.animes.value, all => { selected.value = selected.value.filter(id => all.some(a => a.id === id)) })
function toggle(id: number) { selected.value = selected.value.includes(id) ? selected.value.filter(x => x !== id) : [...selected.value, id] }
function toggleMode() { selecting.value = !selecting.value; selected.value = [] }
const matching = computed(() => {
  const q = filter.value.trim().toLowerCase()
  const all = store.animes.value.filter(a => (scope.value !== 'empty' || !a.item_count) && (scope.value !== 'saved' || a.saved_count > 0))
    .filter(a => !q || [a.title, a.meta_name_cn, a.meta_name, a.search_name].some(s => s && s.toLowerCase().includes(q)))
  if (sort.value === 'title') all.sort((a, b) => a.title.localeCompare(b.title, 'zh-CN'))
  if (sort.value === 'resources') all.sort((a, b) => b.item_count - a.item_count)
  return all
})
const dayCounts = computed(() => weekdays.map((_, day) => matching.value.filter(a => effectiveWeekday(a) === day).length))
watch([activeWeekday, dayCounts, store.loaded, weekdayList], updateWeekdayIndicator, { flush: 'post' })
const list = computed(() => matching.value.filter(a => activeWeekday.value === -1 || effectiveWeekday(a) === activeWeekday.value))
const allSelected = computed(() => list.value.length > 0 && list.value.every(a => selected.value.includes(a.id)))
function selectVisible() {
  selected.value = allSelected.value ? selected.value.filter(id => !list.value.some(a => a.id === id)) : [...new Set([...selected.value, ...list.value.map(a => a.id)])]
}
function searchWish(keyword: string) {
  void router.push({ path: '/discover', query: { keyword: keyword.slice(0, 100) } })
}
</script>

<template>
  <div class="page library-page">
    <PageHeader
      title="番剧库"
      description="每周的期待，都在这里"
      :count="store.loaded.value ? store.animes.value.length : undefined"
    >
      <BaseButton :icon="ListChecks" :variant="selecting ? 'soft' : 'default'" :disabled="!store.animes.value.length" @click="toggleMode">{{ selecting ? '完成管理' : '批量管理' }}</BaseButton>
      <BaseButton :icon="Plus" variant="primary" @click="dialogs.openAnimeEdit()">新建番剧</BaseButton>
    </PageHeader>

    <BangumiWishes @search="searchWish" />

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
      <SearchInput v-model="filter" placeholder="搜索番剧名称" width="240px" />
      <BaseButton size="sm" :icon="SlidersHorizontal" :variant="moreFilters || scope !== 'all' || date ? 'soft' : 'ghost'" :aria-expanded="moreFilters" aria-controls="library-filters" @click="moreFilters = !moreFilters">筛选与日期</BaseButton>
      <select v-model="sort" class="sort-select" aria-label="排序"><option value="recent">最近收录</option><option value="title">名称排序</option><option value="resources">资源数量</option></select>
      <div class="view-switch" role="group" aria-label="番剧展示方式">
        <button type="button" :aria-pressed="layout === 'compact'" aria-label="信息卡片" title="信息卡片" @click="layout = 'compact'"><Rows3 aria-hidden="true" /></button>
        <button type="button" :aria-pressed="layout === 'poster'" aria-label="海报墙" title="海报墙" @click="layout = 'poster'"><LayoutGrid aria-hidden="true" /></button>
      </div>
    </div>
    <div v-show="moreFilters" id="library-filters" class="extra-filters">
      <div class="filter-fields">
        <label>资源范围 <select v-model="scope"><option value="all">全部番剧</option><option value="empty">无资源</option><option value="saved">有已保存资源</option></select></label>
        <label for="library-date">选择日期 <input id="library-date" v-model="date" type="date" @change="weekday = -1" /></label>
      </div>
      <p>自动采用 Bangumi 放送星期，可在「编辑番剧」中手动调整；日期仅对应每周安排，不代表实际到片或仍在连载。</p>
    </div>
    <div v-if="selecting" class="batch-toolbar">
      <label class="select-all"><input type="checkbox" :checked="allSelected" :indeterminate="!allSelected && list.some(a => selected.includes(a.id))" @change="selectVisible" />全选当前结果</label>
      <span>已选 {{ chosen.length }} 部</span>
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

    <EmptyState v-else-if="store.loadError.value && !store.animes.value.length" :icon="ServerOff" title="番剧库加载失败" hint="请检查服务连接后重新加载" tone="error" compact>
      <BaseButton @click="store.refresh()">重新加载</BaseButton>
    </EmptyState>
    <EmptyState
      v-else-if="!store.animes.value.length"
      :icon="Clapperboard"
      title="番剧库还是空的"
      hint="添加 RSS 订阅并同步后，识别出的番剧会自动出现在这里"
      compact
    >
      <BaseButton variant="primary" @click="router.push('/feeds')">去添加订阅</BaseButton>
      <BaseButton @click="router.push('/discover')">先去找番</BaseButton>
    </EmptyState>

    <EmptyState v-else-if="!list.length" :icon="SearchX" title="没有匹配的番剧" hint="试试其他关键词，或清除筛选" compact>
      <BaseButton @click="clearFilters">清除筛选</BaseButton>
    </EmptyState>

    <TransitionGroup v-else tag="div" class="grid" :class="layout" name="list" appear>
      <AnimeCard
        v-for="(a, i) in list"
        :key="a.id"
        :anime="a"
        :compact="layout === 'compact'"
        :selecting="selecting"
        :selected="selected.includes(a.id)"
        :style="{ '--i': i }"
        @open="dialogs.openAnimeDetail"
        @toggle="toggle"
      />
    </TransitionGroup>
  </div>
</template>

<style scoped>
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
.view-switch { display: flex; padding: 3px; border: 1px solid var(--line); background: var(--surface); border-radius: var(--r-sm); }
.view-switch button { display: grid; place-items: center; width: 34px; height: 30px; border: 0; background: transparent; color: var(--text-3); border-radius: var(--r-xs); cursor: pointer; }
.view-switch button[aria-pressed="true"] { color: var(--accent); background: var(--accent-soft); }
.view-switch svg { width: 16px; height: 16px; }
.extra-filters { padding: 14px 16px; margin-bottom: 18px; border-radius: var(--r-md); background: var(--surface-2); }
.filter-fields { display: flex; flex-wrap: wrap; gap: 12px 24px; }
.filter-fields label, .select-all { display: inline-flex; align-items: center; gap: 8px; font-size: 12px; }
.filter-fields input, .filter-fields select { width: 160px; min-width: 0; }
.extra-filters p { margin: 10px 0 0; font-size: 12px; line-height: 1.7; color: var(--text-3); }
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
