<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { Search, X, ExternalLink, Copy, CloudOff, Compass, Plus } from '@lucide/vue'
import BaseButton from '@/components/ui/BaseButton.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import { useSubjectSearch } from '@/stores/subjectSearch'
import { toast } from '@/stores/toast'
import { formatTime } from '@/utils/format'
import { useDialogs } from '@/stores/dialogs'
import { useAppStore } from '@/stores/app'
import { aggregateSubscription } from '@/utils/aggregateSubscription'
import type { AggregateResource, DiscoverSubject } from '@/api/types'

const { state, search, select, close, cancel } = useSubjectSearch()
const route = useRoute()
const dialogs = useDialogs()
const store = useAppStore()
const webLink = (raw: string) => /^https?:\/\//i.test(raw)
const magnetLink = (raw: string) => /^magnet:\?xt=urn:btih:[a-f0-9]{40}$/i.test(raw)
const sizeLabel = (size: number) => size > 0 ? `${(size / 1024 ** 3).toFixed(2)} GB` : '大小未知'

function subscribe(subject: DiscoverSubject, item: AggregateResource) {
  const feed = aggregateSubscription(subject, item, store.feeds.value)
  if (feed) dialogs.openFeed(feed)
}

async function copyMagnet(magnet: string) {
  if (!magnetLink(magnet)) return
  try { await navigator.clipboard.writeText(magnet); toast.success('磁力链接已复制') }
  catch { toast.error('无法复制，请在磁力链接上使用浏览器的复制链接功能') }
}

onMounted(() => {
  if (typeof route.query.keyword === 'string' && route.query.keyword.trim()) {
    state.keyword = route.query.keyword.slice(0, 100)
    void search()
  }
})
onUnmounted(cancel)
</script>

<template>
  <section class="search-panel card" aria-label="搜索 Bangumi 番剧">
    <form @submit.prevent="search">
      <label class="keyword"><Search aria-hidden="true" /><input v-model="state.keyword" maxlength="100" type="search"
        aria-label="番剧名称" placeholder="输入番名" enterkeyhint="search" /></label>
      <BaseButton type="submit" variant="primary" :icon="Search" :disabled="!state.keyword.trim() || (state.searching && state.attempted === state.keyword.trim())">搜索番剧</BaseButton>
    </form>
  </section>
  <EmptyState v-if="state.searching" :icon="Compass" title="正在搜索 Bangumi" loading>
    <BaseButton :icon="X" @click="cancel">取消搜索</BaseButton>
  </EmptyState>
  <EmptyState v-else-if="state.error" :icon="CloudOff" title="番剧搜索失败" :hint="state.error" tone="error">
    <BaseButton @click="search">重试</BaseButton>
  </EmptyState>
  <EmptyState v-else-if="!state.result" :icon="Compass" title="先找到你想看的那部番" />
  <template v-else>
    <p class="result-note" role="status">「{{ state.result.keyword }}」找到 {{ state.result.items.length }} 个 Bangumi 候选条目，最多展示 8 个。</p>
    <p v-if="!state.result.resources_enabled" class="notice" role="status">此实例尚未配置在线聚合服务，仍可浏览番剧条目；查看资源需由管理员配置服务。</p>
    <EmptyState v-if="!state.result.items.length" :icon="Search" title="没有找到番剧条目" hint="试试原名或其他别名。" />
    <div class="subjects">
      <article v-for="subject in state.result.items" :key="subject.id" class="subject card">
        <div class="subject-heading">
          <img v-if="webLink(subject.cover)" :src="subject.cover" :alt="`${subject.name_cn || subject.name}封面`" loading="lazy" referrerpolicy="no-referrer" />
          <div class="subject-info">
            <h2>{{ subject.name_cn || subject.name }}</h2>
            <p v-if="subject.name_cn && subject.name" class="original-name">{{ subject.name }}</p>
            <p class="metadata"><span v-if="subject.date">{{ subject.date }}</span><span v-if="subject.rating > 0">Bangumi {{ subject.rating.toFixed(1) }}</span><a :href="`https://bgm.tv/subject/${subject.id}`" target="_blank" rel="noopener noreferrer">条目 #{{ subject.id }} <ExternalLink aria-hidden="true" /></a></p>
            <p v-if="subject.summary" class="summary">{{ subject.summary }}</p>
            <BaseButton size="sm" :aria-expanded="state.selected === subject.id" :aria-controls="`resources-${subject.id}`" @click="state.selected === subject.id ? close() : select(subject.id)">{{ state.selected === subject.id ? '收起资源' : '展开查看资源' }}</BaseButton>
          </div>
        </div>
        <section v-if="state.selected === subject.id" :id="`resources-${subject.id}`" class="resources" aria-label="候选种子资源" :aria-busy="state.loadingResources">
          <EmptyState v-if="state.loadingResources" :icon="Search" title="正在聚合搜索资源" hint="由在线服务查询已配置站点，部分来源失败时仍会展示其他结果。" compact loading>
            <BaseButton @click="close">取消并收起</BaseButton>
          </EmptyState>
          <EmptyState v-else-if="state.resourceError" :icon="CloudOff" title="资源搜索失败" :hint="state.resourceError" compact tone="error"><BaseButton @click="select(subject.id)">重试资源搜索</BaseButton></EmptyState>
          <template v-else-if="state.resources">
            <p class="notice">候选资源，请核对番名、季数与字幕版本。</p>
            <p v-for="warning in state.resources.warnings" :key="warning" class="notice" role="status">{{ warning }}</p>
            <p v-if="!state.resources.items.length" role="status">没有找到候选资源。可能是来源未收录、命名不同或搜索范围受限。</p>
            <div v-for="(item, index) in state.resources.items" :key="index" class="resource">
              <h3>{{ item.title }}</h3>
              <p class="metadata"><span>{{ item.sources.join(' / ') }}</span><span v-if="item.fansub_group">{{ item.fansub_group }}</span><span v-if="item.quality">{{ item.quality }}</span><span>{{ sizeLabel(item.size) }}</span><span v-if="item.published">{{ formatTime(item.published) }}</span><span v-if="item.season > 0">标题标记第 {{ item.season }} 季</span><span v-else>季数未标明</span></p>
              <div class="resource-actions"><BaseButton v-if="magnetLink(item.magnet) && item.anime_title.trim() && item.season >= 0" size="sm" variant="primary" :icon="Plus" @click="subscribe(subject, item)">订阅此版本</BaseButton><BaseButton v-if="magnetLink(item.magnet)" size="sm" :icon="Copy" @click="copyMagnet(item.magnet)">复制磁力</BaseButton><a v-if="magnetLink(item.magnet)" :href="item.magnet">磁力链接</a><a v-if="webLink(item.details_url)" :href="item.details_url" target="_blank" rel="noopener noreferrer">来源页 <ExternalLink aria-hidden="true" /></a></div>
            </div>
          </template>
          <p v-else class="notice">请由实例管理员配置在线聚合服务。</p>
        </section>
      </article>
    </div>
  </template>
</template>

<style scoped>
.search-panel { padding: 20px; margin-bottom: 20px; border-radius: var(--r-lg); }
form { display: flex; gap: 10px; }
.keyword { position: relative; flex: 1; min-width: 0; }
.keyword svg { position: absolute; left: 12px; top: 12px; width: 17px; height: 17px; color: var(--text-3); }
.keyword input { padding-left: 38px; }
.result-note { font-size: 12px; color: var(--text-3); margin-top: 12px; line-height: 1.7; }
.result-note { margin-bottom: 16px; }
.subjects { display: grid; gap: 16px; }
.subject { padding: 20px; border-radius: var(--r-lg); min-width: 0; }
.subject-heading { display: flex; gap: 20px; }
.subject-heading img { width: 96px; height: 136px; object-fit: cover; border-radius: var(--r-sm); background: var(--surface-2); }
.subject-info { flex: 1; min-width: 0; }
h2 { font-size: 18px; overflow-wrap: anywhere; }
.original-name, .metadata { color: var(--text-3); font-size: 12px; margin: 6px 0 10px; overflow-wrap: anywhere; }
.metadata { display: flex; gap: 6px 12px; flex-wrap: wrap; }
.summary { display: -webkit-box; -webkit-line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; margin-bottom: 12px; font-size: 13px; line-height: 1.7; color: var(--text-2); white-space: pre-line; }
a { display: inline-flex; align-items: center; gap: 4px; font-size: 12px; overflow-wrap: anywhere; }
a svg { width: 12px; height: 12px; }
.resources { border-top: 1px solid var(--line); margin-top: 20px; padding-top: 16px; display: grid; gap: 12px; }
.notice { color: var(--amber); font-size: 12px; line-height: 1.7; margin-bottom: 10px; }
.resource { padding: 14px; background: var(--surface-2); border-radius: var(--r-sm); min-width: 0; }
h3 { font-size: 13px; line-height: 1.7; overflow-wrap: anywhere; }
.resource-actions { display: flex; gap: 12px; align-items: center; flex-wrap: wrap; }
@media (max-width: 560px) { .search-panel, .subject { padding: 16px; } .subject-heading { gap: 12px; } .subject-heading img { width: 64px; height: 92px; } form { flex-wrap: wrap; } form > .btn { width: 100%; } .keyword { flex-basis: 100%; } }
</style>
