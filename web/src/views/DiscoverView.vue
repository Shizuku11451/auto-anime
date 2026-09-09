<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { CloudOff, Compass, Copy, ExternalLink, Plus, Search, SearchX, TriangleAlert } from '@lucide/vue'
import PageHeader from '@/components/ui/PageHeader.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import BaseButton from '@/components/ui/BaseButton.vue'
import { useDiscover } from '@/stores/discover'
import { useDialogs } from '@/stores/dialogs'
import { toast } from '@/stores/toast'
import { formatTime } from '@/utils/format'
import type { DiscoverItem } from '@/api/types'
import { useAppStore } from '@/stores/app'
import { groupReleases, subscriptionTitle, subscriptionWords } from '@/utils/discover'

const { state, loadSources, search } = useDiscover()
const dialogs = useDialogs()
const store = useAppStore()
const releases = computed(() => groupReleases(state.result?.items ?? []))
const existingFeed = computed(() => store.feeds.value.find((f) => f.url === state.result?.feed_url))

onMounted(loadSources)

const currentSource = computed(() => state.sources.find((s) => s.id === state.source))
const noSources = computed(() => !state.sources.length)

function submit() {
  if (!state.keyword.trim()) return
  if (noSources.value) return
  void search()
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
      document.execCommand('copy')
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
  dialogs.openFeed({
    ...existingFeed.value,
    name: [subscriptionTitle(item), item.fansub_group, item.quality].filter(Boolean).join(' · '),
    url: r.feed_url,
    include_words: subscriptionWords(item, r.keyword),
    exclude_words: '',
    target_title: subscriptionTitle(item),
    auto_download: !!item.fansub_group && (existingFeed.value?.auto_download ?? true),
    enabled: existingFeed.value?.enabled ?? true,
    from_discover: true,
  })
}

function openPage(item: DiscoverItem) {
  window.open(item.link, '_blank', 'noopener')
}
</script>

<template>
  <div class="page">
    <PageHeader title="找番" description="搜索番剧，选择一个字幕组和版本，持续追更" />

    <form class="toolbar card" @submit.prevent="submit">
      <label class="source">
        <span class="src-label">搜索源</span>
        <select v-model="state.source" :disabled="noSources" aria-label="搜索源">
          <option v-if="noSources" value="">{{ state.sourcesError ? '搜索源加载失败' : '无可用搜索源' }}</option>
          <option v-for="s in state.sources" :key="s.id" :value="s.id" :title="s.hint || s.name">
            {{ s.name }}{{ s.custom ? '（自定义）' : '' }}
          </option>
        </select>
      </label>
      <label class="kw">
        <Search class="kw-ic" aria-hidden="true" />
        <input
          v-model="state.keyword"
          type="text"
          maxlength="100"
          placeholder="输入番剧名或关键词，例如：芙莉莲"
          autocomplete="off"
          enterkeyhint="search"
        />
      </label>
      <BaseButton type="submit" variant="primary" :icon="Search" :loading="state.searching" :disabled="noSources">
        搜索
      </BaseButton>
    </form>
    <p class="hint">
      同一版本的多集已折叠。选择后会自动限制字幕组、清晰度和已识别的字幕语言；保存前请预览匹配条目。
      <span v-if="currentSource?.hint" class="src-hint">当前源：{{ currentSource.hint }}</span>
    </p>

    <Transition name="slide-up">
      <div v-if="state.result && !state.searching" class="summary card">
        <div class="summary-main">
          <b>「{{ state.result.keyword }}」</b>
          <span class="muted">{{ state.result.source_name }} · {{ releases.length }} 个版本 · {{ state.result.count }} 条资源</span>
          <span class="rss mono truncate" :title="state.result.feed_url">{{ state.result.feed_url }}</span>
        </div>
        <div class="summary-actions">
          <BaseButton size="sm" :icon="Copy" @click="copyFeedUrl">复制 RSS 地址</BaseButton>
        </div>
      </div>
    </Transition>

    <EmptyState
      v-if="noSources && !state.result"
      :icon="TriangleAlert"
      title="没有可用搜索源"
      :hint="state.sourcesError || '请到设置页配置自定义 RSS 搜索模板'"
      tone="error"
    />
    <EmptyState v-else-if="state.searching" :icon="Compass" :title="`正在搜索「${state.keyword.trim()}」…`" loading />
    <EmptyState v-else-if="state.error" :icon="CloudOff" title="搜索失败" :hint="state.error" tone="error">
      <BaseButton @click="submit">重试</BaseButton>
    </EmptyState>
    <EmptyState
      v-else-if="!state.result"
      :icon="Compass"
      title="输入关键词开始搜索"
      hint="先搜索番剧，再选择喜欢的字幕组和清晰度；不必逐集添加"
    />
    <EmptyState
      v-else-if="!state.result.items.length"
      :icon="SearchX"
      :title="`没有找到与「${state.result.keyword}」匹配的资源`"
      hint="换个关键词，或到设置页换一个搜索源"
      compact
    />
    <TransitionGroup v-else tag="div" class="results" name="list" appear>
      <article v-for="(release, i) in releases" :key="release.key" class="result" :style="{ '--i': i }">
        <div class="r-info">
          <h3 class="r-title">
            <span>{{ release.title }}</span>
            <span class="ep">{{ release.items.length }} 条资源</span>
          </h3>
          <div class="meta-line">
            <span>{{ release.item.fansub_group || '未知字幕组' }}</span>
            <span v-if="release.item.quality">{{ release.item.quality }}</span>
            <span v-if="release.language">{{ release.language }}</span>
          </div>
          <p v-if="!release.item.fansub_group" class="r-original">未识别到字幕组，默认关闭自动下载，请先核对过滤规则。</p>
          <details>
            <summary class="r-original">查看集数与原始标题</summary>
            <p v-for="it in release.items" :key="it.torrent || it.link || it.title" class="r-original">
              {{ it.episode ? `第 ${it.episode} 集 · ` : '' }}{{ it.title }} · {{ formatTime(it.published) }}
              <a v-if="/^https?:/i.test(it.link || '')" :href="it.link" target="_blank" rel="noopener">资源页</a>
            </p>
          </details>
        </div>
        <div class="r-actions">
          <BaseButton v-if="/^https?:/i.test(release.item.link || '')" size="sm" :icon="ExternalLink" @click="openPage(release.item)">
            资源页
          </BaseButton>
          <BaseButton size="sm" variant="soft" :icon="Plus" @click="subscribe(release.item)">{{ existingFeed ? '改为此版本' : '订阅此版本' }}</BaseButton>
        </div>
      </article>
    </TransitionGroup>
  </div>
</template>

<style scoped>
.toolbar {
  display: grid;
  grid-template-columns: 220px minmax(0, 1fr) auto;
  gap: 10px;
  padding: 12px;
  border-radius: var(--r-lg);
  animation: rise-in var(--t-slow) var(--ease-out) both;
  animation-delay: 60ms;
}
.source {
  position: relative;
  display: block;
}
.src-label {
  position: absolute;
  left: 12px;
  top: 50%;
  translate: 0 -50%;
  font-size: 12px;
  color: var(--text-3);
  pointer-events: none;
}
.source select {
  padding-left: 60px;
}
.kw {
  position: relative;
  display: block;
}
.kw-ic {
  position: absolute;
  left: 12px;
  top: 50%;
  translate: 0 -50%;
  width: 15px;
  height: 15px;
  color: var(--text-3);
  pointer-events: none;
}
.kw input {
  padding-left: 36px;
}
.hint {
  margin: 12px 4px 18px;
  color: var(--text-3);
  font-size: 12.5px;
  line-height: 1.6;
}
.src-hint {
  display: block;
  margin-top: 2px;
}

.summary {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px 16px;
  flex-wrap: wrap;
  padding: 12px 16px;
  margin-bottom: 14px;
  border-radius: var(--r-md);
  background: var(--accent-soft);
  border-color: var(--accent-border);
  box-shadow: none;
}
.summary-main {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px 12px;
  min-width: 0;
  flex: 1;
}
.summary-main b {
  font-weight: 700;
}
.rss {
  max-width: 100%;
  color: var(--text-3);
  font-size: 11.5px;
}
.summary-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.results {
  position: relative;
  display: grid;
  gap: 10px;
}
.result {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 14px;
  padding: 14px 16px;
  background: var(--surface);
  border: 1px solid var(--line);
  border-radius: var(--r-lg);
  box-shadow: var(--shadow-sm);
  transition:
    border-color var(--t-med) var(--ease-out),
    box-shadow var(--t-med) var(--ease-out);
}
.result:hover {
  border-color: var(--line-strong);
  box-shadow: var(--shadow-md);
}
.r-info {
  min-width: 0;
  display: grid;
  gap: 5px;
}
.r-title {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px 8px;
  font-size: 14.5px;
  font-weight: 650;
  line-height: 1.35;
}
.ep {
  display: inline-flex;
  align-items: center;
  height: 22px;
  padding: 0 8px;
  border-radius: 999px;
  font-size: 11.5px;
  font-weight: 700;
  color: var(--accent);
  background: var(--accent-soft);
  white-space: nowrap;
}
.r-original {
  color: var(--text-3);
  font-size: 12px;
  overflow-wrap: anywhere;
}
.r-actions {
  display: flex;
  gap: 6px;
  flex: none;
}

@media (max-width: 760px) {
  .toolbar {
    grid-template-columns: 1fr;
  }
  .result {
    flex-direction: column;
  }
  .r-actions {
    width: 100%;
  }
  .r-actions :deep(.btn) {
    flex: 1;
  }
}
</style>
