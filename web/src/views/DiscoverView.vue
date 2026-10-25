<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { CloudOff, Compass, Copy, History, Search, SearchX, SlidersHorizontal, TriangleAlert, X } from '@lucide/vue'
import PageHeader from '@/components/ui/PageHeader.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import BaseButton from '@/components/ui/BaseButton.vue'
import ReleaseCard from '@/components/discover/ReleaseCard.vue'
import { useDiscover } from '@/stores/discover'
import { useDialogs } from '@/stores/dialogs'
import { toast } from '@/stores/toast'
import type { DiscoverItem } from '@/api/types'
import { useAppStore } from '@/stores/app'
import { discoveredSubscriptionWords, filterReleases, groupReleases, releaseField, releaseSeason, releaseSeasonLabel, seasonLabel, subscriptionTitle, type ReleaseField, type ReleaseFilters, type ReleaseSort } from '@/utils/discover'

const { state, loadSources, search, cancelSearch, retry } = useDiscover()
const dialogs = useDialogs()
const store = useAppStore()
const releases = computed(() => groupReleases(state.result?.items ?? []))
const existingFeed = computed(() => store.feeds.value.find((f) => f.url === state.result?.feed_url))
const filters = reactive<ReleaseFilters>({ group: '', quality: '', language: '' })
const sort = ref<ReleaseSort>('source')
const season = ref('')
const seasons = computed(() => [...new Set(releases.value.map(r => releaseSeason(r.item)))].sort((a, b) => a - b))
const filterFields: { key: ReleaseField; label: string }[] = [
  { key: 'group', label: '字幕组' }, { key: 'quality', label: '清晰度' }, { key: 'language', label: '字幕语言' },
]
const filtered = computed(() => filterReleases(releases.value, filters, sort.value).filter(r => !season.value || String(releaseSeason(r.item)) === season.value))
const hasFilters = computed(() => !!season.value || Object.values(filters).some(Boolean))
const activeSourceName = computed(() => state.sources.find((s) => s.id === state.attempt?.source)?.name || '所选来源')
const showResults = computed(() => !!state.result && !state.searching && !state.error)
const sameSearch = computed(() => state.searching && state.attempt?.keyword === state.keyword.trim() && state.attempt.source === state.source && state.attempt.literal === state.literal)

function filterOptions(field: ReleaseField) {
  const values = new Map<string, number>()
  for (const release of releases.value) {
    const value = releaseField(release, field)
    values.set(value, (values.get(value) || 0) + 1)
  }
  return [...values].map(([value, count]) => ({ value, count }))
}

function resetFilters() {
  Object.assign(filters, { group: '', quality: '', language: '' })
  season.value = ''
}

watch(() => state.result, () => {
  resetFilters()
  if (state.result?.requested_season) season.value = String(state.result.requested_season)
}, { immediate: true })

onMounted(loadSources)
onUnmounted(cancelSearch)

const currentSource = computed(() => state.sources.find((s) => s.id === state.source))
const noSources = computed(() => !state.sources.length)

function submit() {
  if (!state.keyword.trim()) return
  if (noSources.value) return
  void search()
}

function switchSource(source: string) {
  state.source = source
  cancelSearch()
  submit()
}

function searchRecent(keyword: string) {
  state.keyword = keyword
  submit()
}

function clearKeyword() {
  cancelSearch()
  state.keyword = ''
  document.getElementById('discover-keyword')?.focus()
}

async function copyFeedUrl() {
  const text = state.result?.feed_url
  if (!text) return
  try {
    await navigator.clipboard.writeText(text)
    toast.success('RSS 地址已复制')
  } catch {
    const ta = document.createElement('textarea')
    ta.value = text
    ta.style.position = 'fixed'
    ta.style.opacity = '0'
    document.body.appendChild(ta)
    ta.select()
    try {
      if (!document.execCommand('copy')) throw new Error('copy failed')
      toast.success('RSS 地址已复制')
    } catch {
      toast.error('复制失败，请手动复制')
    }
    ta.remove()
  }
}

function subscribe(item: DiscoverItem) {
  const r = state.result
  if (!r) return
  if ((item.season ?? 0) < 0) return
  const url = item.feed_url || r.feed_url
  const existing = store.feeds.value.find(f => f.url === url)
  dialogs.openFeed({
    ...existing,
    name: [subscriptionTitle(item), releaseSeason(item) ? releaseSeasonLabel(item) : '', item.fansub_group, item.quality].filter(Boolean).join(' · '),
    url,
    include_words: discoveredSubscriptionWords(item, r.keyword, !!r.requested_season),
    season_filter: item.season || -1,
    allow_unknown_season: false,
    requires_season_confirmation: !item.season,
    exclude_words: '',
    target_title: subscriptionTitle(item),
    auto_download: !!item.fansub_group && !!item.season && (existing?.auto_download ?? true),
    enabled: existing?.enabled ?? true,
    from_discover: true,
  })
}

</script>

<template>
  <div class="page">
    <PageHeader title="找番" description="搜番名、选版本，喜欢的字幕组持续追更" />

    <section class="search-panel card" aria-label="搜索番剧">
      <form class="toolbar" @submit.prevent="submit">
        <label class="kw">
          <Search class="kw-ic" aria-hidden="true" />
          <input id="discover-keyword" v-model="state.keyword" aria-label="番剧名或关键词" type="text"
            maxlength="100" placeholder="输入番剧名，例如：芙莉莲" autocomplete="off" enterkeyhint="search" />
          <button v-if="state.keyword" class="clear-keyword" type="button" aria-label="清空关键词" @click="clearKeyword">
            <X aria-hidden="true" />
          </button>
        </label>
        <BaseButton type="submit" variant="primary" :icon="Search" :disabled="noSources || !state.keyword.trim() || sameSearch">
          {{ state.searching ? '搜索新关键词' : '搜索' }}
        </BaseButton>
      </form>
      <div class="source-row" role="group" aria-label="搜索源">
        <span class="row-label">搜索源</span>
        <span v-if="state.sourcesLoading && noSources" class="muted" role="status">正在加载搜索源…</span>
        <button v-for="source in state.sources" :key="source.id" type="button" class="source-button"
          :class="{ active: state.source === source.id }" :aria-pressed="state.source === source.id"
          :title="source.hint" @click="switchSource(source.id)">
          {{ source.name }}
        </button>
      </div>
      <p v-if="currentSource?.hint" class="source-hint">{{ currentSource.hint }}</p>
      <label class="source-hint"><input v-model="state.literal" type="checkbox" /> 按原词搜索（不识别季数、不扩展查询）</label>
      <div v-if="state.sourcesError && !noSources" class="source-error" role="status">
        <span>搜索源刷新失败：{{ state.sourcesError }}</span>
        <BaseButton size="sm" :loading="state.sourcesLoading" @click="loadSources">重新加载</BaseButton>
      </div>
      <div v-if="state.recent.length" class="recent-row" role="group" aria-label="最近搜索">
        <span class="row-label"><History aria-hidden="true" />最近搜索</span>
        <button v-for="keyword in state.recent" :key="keyword" class="recent-button" type="button" @click="searchRecent(keyword)">{{ keyword }}</button>
        <button class="clear-recent" type="button" @click="state.recent = []">清除记录</button>
      </div>
    </section>

    <p v-if="state.cancelled && !state.searching" class="cancel-notice" role="status">
      搜索已取消。{{ state.result ? '下方保留上一次的搜索结果。' : '可以修改关键词或切换搜索源重新搜索。' }}
    </p>

    <EmptyState v-if="state.sourcesLoading && noSources" :icon="Compass" title="正在加载搜索源" hint="加载完成后即可搜索番剧" loading />
    <EmptyState v-else-if="noSources" :icon="TriangleAlert" title="没有可用搜索源"
      :hint="state.sourcesError || '请到设置页配置自定义 RSS 搜索模板'" tone="error">
      <BaseButton :loading="state.sourcesLoading" @click="loadSources">重新加载搜索源</BaseButton>
      <RouterLink class="settings-link" to="/settings">前往设置</RouterLink>
    </EmptyState>
    <EmptyState v-else-if="state.searching" :icon="Compass" :title="`正在${activeSourceName}搜索「${state.attempt?.keyword}」`"
      hint="正在读取资源与可用的历史页。你可以取消，或直接切换上方搜索源。" loading>
      <BaseButton :icon="X" @click="cancelSearch">取消搜索</BaseButton>
    </EmptyState>
    <EmptyState v-else-if="state.error" :icon="CloudOff" :title="`${activeSourceName}搜索失败`" :hint="state.error" tone="error">
      <BaseButton @click="retry">重试本次搜索</BaseButton>
      <BaseButton v-for="source in state.sources.filter(s => s.id !== state.attempt?.source)" :key="source.id" size="sm"
        @click="switchSource(source.id)">改用{{ source.name }}</BaseButton>
    </EmptyState>
    <EmptyState v-else-if="!state.result" :icon="Compass" title="想追哪部番？从名字开始"
      hint="搜索中文名、日文名或别名，再按字幕组和清晰度选择版本；同一版本的多集会自动合并展示。" />

    <template v-if="showResults && state.result">
      <section class="result-summary" aria-label="搜索结果概况">
        <div>
          <h2>「{{ state.result.keyword }}」的搜索结果</h2>
          <p>{{ state.result.source_name }}<span>·</span>{{ releases.length }} 个版本<span>·</span>{{ state.result.count }} 条资源<span>·</span>已读取 {{ state.result.pages }} 页</p>
        </div>
        <BaseButton size="sm" :icon="Copy" @click="copyFeedUrl">{{ state.result.requested_season ? '复制原词 RSS' : '复制 RSS 地址' }}</BaseButton>
      </section>
      <p class="read-scope" :class="{ limited: state.result.limited }" role="status">
        {{ state.result.warning || (state.result.history ? '集数来自此来源已发现的资源，不代表全季已发布。' : '此来源仅提供当前 RSS 列表；查找早期集数可换源，或加上字幕组缩小搜索范围。') }}
      </p>
      <p v-if="state.result.requested_season" class="read-scope" role="status">
        已识别：{{ seasonLabel(state.result.requested_season) }}；同源查询：{{ state.result.queries?.join('、') }}。季数筛选包含明确标记与推测结果；推测仍需人工确认，无佐证的资源可切换「季数未标明」查看，不自动认作目标季。订阅使用各版本的实际 RSS，原词 RSS 不包含合并结果。
      </p>
      <p v-if="existingFeed" class="subscription-notice">
        此 RSS 已订阅为「{{ existingFeed.name }}」。选择版本后可预览并调整原订阅，已有条目与文件保留。
      </p>

      <EmptyState v-if="!releases.length" :icon="SearchX" title="暂时没有找到匹配资源"
        hint="试试更短的番名、日文名或别名，也可以保持关键词换个来源。" compact>
        <BaseButton v-for="source in state.sources.filter(s => s.id !== state.result?.source)" :key="source.id" size="sm"
          @click="switchSource(source.id)">改用{{ source.name }}</BaseButton>
      </EmptyState>
      <template v-else>
        <section class="filter-panel card" aria-label="筛选版本">
          <div class="filter-heading"><span><SlidersHorizontal aria-hidden="true" />筛选版本</span><span class="muted">仅筛选本次结果</span></div>
          <div class="filters">
            <label><span>季数</span><select v-model="season" aria-label="季数">
              <option value="">全部季数</option>
              <option v-if="state.result.requested_season && !seasons.includes(state.result.requested_season)" :value="String(state.result.requested_season)">{{ seasonLabel(state.result.requested_season) }}（暂无明确资源）</option>
              <option v-for="value in seasons" :key="value" :value="String(value)">{{ seasonLabel(value) }}</option>
            </select></label>
            <label v-for="field in filterFields" :key="field.key">
              <span>{{ field.label }}</span>
              <select v-model="filters[field.key]" :aria-label="field.label">
                <option value="">全部{{ field.label }}</option>
                <option v-for="option in filterOptions(field.key)" :key="option.value" :value="option.value">{{ option.value }}（{{ option.count }}）</option>
              </select>
            </label>
            <label><span>排序</span><select v-model="sort" aria-label="版本排序">
              <option value="source">来源顺序</option><option value="latest">最近发布优先</option><option value="episodes">已发现集数最多</option>
            </select></label>
          </div>
          <div class="filter-footer">
            <span role="status" aria-live="polite">显示 {{ filtered.length }} / {{ releases.length }} 个版本</span>
            <BaseButton v-if="hasFilters" size="sm" variant="ghost" @click="resetFilters">清除筛选</BaseButton>
          </div>
        </section>
        <EmptyState v-if="!filtered.length" :icon="SearchX" title="没有符合筛选条件的版本" hint="可以清除筛选，查看此来源的全部搜索结果。" compact>
          <BaseButton @click="resetFilters">清除筛选</BaseButton>
        </EmptyState>
        <div v-else class="results">
          <ReleaseCard v-for="release in filtered" :key="release.key" :release="release" :editing="store.feeds.value.some(f => f.url === (release.item.feed_url || state.result?.feed_url))" @subscribe="subscribe" />
        </div>
        <p class="subscription-hint">选择版本后将自动预览字幕组、清晰度与字幕语言的匹配结果，核对后再保存订阅。</p>
      </template>
    </template>
  </div>
</template>

<style scoped>
.search-panel { padding: 20px; border-radius: var(--r-lg); margin-bottom: 24px; }
.toolbar { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 10px; }
.kw { position: relative; display: block; min-width: 0; }
.kw input { padding-left: 40px; padding-right: 40px; }
.kw-ic { position: absolute; left: 13px; top: 50%; translate: 0 -50%; width: 17px; height: 17px; color: var(--text-3); pointer-events: none; }
.clear-keyword { position: absolute; right: 4px; top: 4px; width: 32px; height: 32px; display: grid; place-items: center; border: 0; border-radius: var(--r-xs); background: transparent; color: var(--text-3); cursor: pointer; }
.clear-keyword:hover { background: var(--surface-3); color: var(--text); }
.clear-keyword svg { width: 16px; height: 16px; }
.source-row, .recent-row { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; }
.source-row { margin-top: 16px; }
.row-label { display: inline-flex; align-items: center; gap: 6px; margin-right: 4px; font-size: 12px; color: var(--text-3); flex: none; }
.row-label svg { width: 13px; height: 13px; }
.source-button { padding: 7px 12px; border: 1px solid var(--line); background: var(--surface); border-radius: var(--r-sm); font-size: 12px; color: var(--text-2); cursor: pointer; overflow-wrap: anywhere; max-width: 100%; }
.source-button:hover { border-color: var(--accent-border); color: var(--accent); }
.source-button.active { color: var(--accent); background: var(--accent-soft); border-color: var(--accent-border); font-weight: 600; }
.source-hint { margin-top: 10px; font-size: 12px; color: var(--text-3); line-height: 1.6; overflow-wrap: anywhere; }
.source-error { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; color: var(--amber); font-size: 12px; margin-top: 10px; overflow-wrap: anywhere; }
.recent-row { margin-top: 16px; padding-top: 14px; border-top: 1px solid var(--line); }
.recent-button, .clear-recent { border: 0; background: transparent; color: var(--text-2); font-size: 12px; cursor: pointer; padding: 6px 8px; border-radius: var(--r-xs); }
.recent-button { max-width: 180px; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; background: var(--surface-2); }
.recent-button:hover { color: var(--accent); }
.clear-recent { color: var(--text-3); }
.cancel-notice { margin-bottom: 16px; font-size: 13px; color: var(--text-2); }
.result-summary { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.result-summary > div { min-width: 0; }
.result-summary h2 { font-size: 17px; font-weight: 650; line-height: 1.5; overflow-wrap: anywhere; }
.result-summary p { display: flex; align-items: center; flex-wrap: wrap; gap: 4px 9px; font-size: 12px; color: var(--text-3); margin-top: 6px; }
.read-scope, .subscription-notice, .subscription-hint { font-size: 12px; line-height: 1.7; overflow-wrap: anywhere; color: var(--text-3); }
.read-scope { margin: 12px 0 18px; }
.read-scope.limited { color: var(--amber); }
.subscription-notice { margin-bottom: 16px; padding: 10px 14px; border: 1px solid var(--accent-border); border-radius: var(--r-sm); color: var(--accent); background: var(--accent-soft); }
.filter-panel { margin-bottom: 16px; padding: 16px; border-radius: var(--r-md); }
.filter-heading { display: flex; align-items: center; justify-content: space-between; gap: 8px; font-size: 12px; margin-bottom: 12px; }
.filter-heading > span:first-child { display: flex; align-items: center; gap: 6px; color: var(--text-2); }
.filter-heading svg { width: 14px; height: 14px; }
.filters { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; }
.filters label { min-width: 0; }
.filters label > span { display: block; font-size: 12px; color: var(--text-3); margin-bottom: 6px; }
.filters select { text-overflow: ellipsis; }
.filter-footer { display: flex; align-items: center; justify-content: space-between; min-height: 32px; margin-top: 8px; font-size: 12px; color: var(--text-3); }
.results { display: grid; gap: 12px; }
.subscription-hint { margin-top: 16px; text-align: center; }
.settings-link { font-size: 13px; align-self: center; }
@media (max-width: 760px) {
  .search-panel { padding: 16px; }
  .filters { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}
@media (max-width: 380px) {
  .toolbar { grid-template-columns: minmax(0, 1fr); }
  .filter-panel { padding: 12px; }
}
</style>
