<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { useDocumentVisibility, useIntervalFn } from '@vueuse/core'
import { Inbox, Link, Pencil, Merge, Trash2, RefreshCw, Search } from '@lucide/vue'
import BaseDialog from '@/components/ui/BaseDialog.vue'
import BaseButton from '@/components/ui/BaseButton.vue'
import CoverImage from '@/components/ui/CoverImage.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import SkeletonBlock from '@/components/ui/SkeletonBlock.vue'
import ResourceRow from '@/components/library/ResourceRow.vue'
import { useDialogs } from '@/stores/dialogs'
import { useAppStore } from '@/stores/app'
import { useBusySet } from '@/composables/useBusy'
import { http, errorMessage } from '@/api/client'
import { toast } from '@/stores/toast'
import { confirmDialog } from '@/stores/confirm'
import { seasonLabel } from '@/utils/format'
import { BANGUMI_COLLECTION_LABELS, type MetaRematchResult, type ResourceItem } from '@/api/types'
import { isDownloading } from '@/utils/status'
import type { PlayerEntry } from '@/stores/dialogs'
import { isWindows } from '@/utils/platform'
/* Windows 本机没有 NAS 概念，删除记录只影响数据库记录。 */
const mediaKeptHint = computed(() =>
  isWindows(store.platformOS.value) ? '只删除这条资源记录，不会删除已保存的媒体文件。' : '只删除这条资源记录，不会删除 NAS 上已保存的文件。',
)
const dialogs = useDialogs()
const store = useAppStore()
const router = useRouter()
const s = dialogs.state.animeDetail
const visibility = useDocumentVisibility()
useIntervalFn(() => {
  if (s.open && !s.loading && visibility.value === 'visible') void dialogs.reloadAnimeDetail()
}, 5000)
const d = computed(() => s.data)
const subject = computed(() => s.subject?.id === d.value?.bangumi_id ? s.subject : null)
const rating = computed(() => subject.value?.rating ?? d.value?.rating ?? 0)
const summary = computed(() => subject.value?.summary || d.value?.summary || '')
const cover = computed(() => d.value?.cover || subject.value?.cover || '')
const cloudOnly = computed(() => (d.value?.id ?? 0) < 0)
const resourceState = computed(() => {
  const anime = d.value
  if (!anime) return ''
  if (cloudOnly.value) {
    if (s.loading) return '正在核对本地资源…'
    if (s.error) return '本地资源状态暂无法确认，请重试后再决定。'
    return '应用番剧库中未找到关联的本地番剧或资源。'
  }
  if (anime.saved_count > 0) return `本地已有 ${anime.saved_count} 条已保存资源，可在下方选择播放。`
  if (anime.item_count > 0) return '已有资源记录，尚无已保存媒体。可在下方查看下载状态并选择操作。'
  return '该番剧暂无资源记录，也没有已保存媒体。'
})
const downloadingCount = computed(() => d.value?.seasons.flatMap(se => [...se.episodes.flatMap(ep => ep.items), ...se.loose]).filter(it => isDownloading(it.status)).length ?? 0)
const { is: isBusy, run } = useBusySet()

const displayName = computed(() => subject.value?.name_cn || d.value?.meta_name_cn || d.value?.title || '')

const metaBits = computed(() => {
  const x = d.value
  if (!x) return []
  const bits: string[] = cloudOnly.value ? [] : [`识别名 ${x.title}`]
  if (x.meta_name && x.meta_name !== x.meta_name_cn) bits.push(x.meta_name)
  if (!cloudOnly.value && x.season > 0) bits.push(`默认第 ${x.season} 季`)
  if (x.bangumi_id) bits.push(`Bangumi #${x.bangumi_id}`)
  if (x.storage_dir) bits.push(`存储 ${x.storage_dir}`)
  return bits
})

const totalEpisodes = computed(() => d.value?.seasons.reduce((n, se) => n + se.episodes.length, 0) ?? 0)

function close() {
  s.open = false
}

async function findResources() {
  const anime = d.value
  if (!anime || (cloudOnly.value && (s.loading || s.error))) return
  try {
    const failure = await router.push({ path: '/discover', query: { keyword: (anime.meta_name_cn || anime.title).slice(0, 100) } })
    if (!failure && s.data === anime) close()
  } catch (e) { toast.error(errorMessage(e)) }
}

async function rematchMetadata() {
  const anime = d.value
  if (!anime || cloudOnly.value) return
  if (anime.bangumi_id || anime.meta_source === 'manual') {
    toast.error('该番剧已手动绑定元数据，请先在「元数据绑定」中解除绑定')
    return
  }
  await run('rematch', async () => {
    const ok = await confirmDialog('重新识别元数据', `将清除「${anime.title}」当前的封面和元数据匹配，并在后台重新获取。已设置的元数据搜索词会继续使用。`, {
      okText: '重新识别',
    })
    if (!ok || !s.open || s.id !== anime.id) return
    try {
      await http.post<MetaRematchResult>(`/api/animes/${anime.id}/rematch`)
      toast.success('已加入重新识别队列，结果将自动刷新')
      await Promise.all([dialogs.reloadAnimeDetail(), store.refresh()])
    } catch (e) {
      toast.error(errorMessage(e))
    }
  })
}

async function download(item: ResourceItem) {
  await run(item.id, async () => {
    try {
      await http.post(`/api/items/${item.id}/download`)
      toast.success('已加入下载队列')
    } catch (e) {
      toast.error(errorMessage(e))
    }
    await Promise.all([dialogs.reloadAnimeDetail(), store.refresh()])
  })
}

async function remove(item: ResourceItem) {
  const ok = await confirmDialog('移除记录', mediaKeptHint.value, {
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
  await Promise.all([dialogs.reloadAnimeDetail(), store.refresh()])
}

/** 播放：当前详情内所有已保存资源按季/集排序成连播列表 */
function play(item: ResourceItem) {
  const d = s.data
  if (!d) return
  const name = d.meta_name_cn || d.title
  const entries: PlayerEntry[] = []
  for (const se of [...d.seasons].sort((a, b) => a.season - b.season)) {
    const rows = [...se.episodes.flatMap((ep) => ep.items), ...se.loose]
    rows.sort((a, b) => epNo(a) - epNo(b))
    for (const r of rows) {
      if (r.status !== '已保存' || !r.file_path) continue
      entries.push({
        id: r.id,
        title: name,
        season: r.season > 0 ? r.season : se.season,
        episode: r.episode,
        fansub_group: r.fansub_group,
        quality: r.quality,
      })
    }
  }
  dialogs.openPlayer(entries.length ? entries : [toEntry(name, item)], item.id)
}

function toEntry(name: string, r: ResourceItem): PlayerEntry {
  return {
    id: r.id,
    title: name,
    season: r.season > 0 ? r.season : 1,
    episode: r.episode,
    fansub_group: r.fansub_group,
    quality: r.quality,
  }
}

function epNo(x: { episode: string }): number {
  const n = Number.parseFloat(x.episode)
  return Number.isFinite(n) ? n : Number.MAX_SAFE_INTEGER
}
</script>

<template>
  <BaseDialog :open="s.open" :title="displayName || '番剧详情'" size="xl" @close="close">
    <div v-if="!d && s.loading" class="hero" aria-busy="true">
      <SkeletonBlock width="112px" height="168px" radius="var(--r-md)" />
      <div class="hero-lines">
        <SkeletonBlock height="14px" width="60%" />
        <SkeletonBlock height="12px" width="85%" />
        <SkeletonBlock height="12px" width="40%" />
      </div>
    </div>

    <EmptyState v-else-if="!d && s.error" :icon="Inbox" title="加载失败" :hint="s.error" tone="error" compact>
      <BaseButton @click="dialogs.reloadAnimeDetail()">重试</BaseButton>
    </EmptyState>

    <template v-else-if="d">
      <section class="banner" :class="{ 'has-cover': !!cover }">
        <template v-if="cover">
          <img class="banner-bg blurred" :src="cover" alt="" aria-hidden="true" />
          <img class="banner-bg sharp" :src="cover" alt="" aria-hidden="true" />
        </template>
        <div class="banner-shade" aria-hidden="true" />
        <div class="banner-content">
          <CoverImage :src="cover" :alt="displayName" class="hero-cover" radius="var(--r-md)" />
          <div class="banner-text">
            <p class="banner-title">{{ displayName }}</p>
            <p v-if="d.meta_name && d.meta_name !== displayName" class="banner-sub">{{ d.meta_name }}</p>
            <div class="banner-facts">
              <span v-if="rating > 0" class="banner-rating" :aria-label="`Bangumi 评分 ${rating.toFixed(1)} 分`"><b>{{ rating.toFixed(1) }}</b> / 10<template v-if="subject?.rating_total"> · {{ subject.rating_total }} 人评分</template></span>
              <span v-if="s.collectionType" class="tag accent" title="只读收藏状态，不代表本地下载或播放进度">{{ BANGUMI_COLLECTION_LABELS[s.collectionType] }}</span>
              <span v-if="subject?.date">{{ subject.date }}</span>
              <span v-if="subject && subject.eps > 0">正篇 {{ subject.eps }} 集</span>
              <span v-if="subject && subject.rank > 0">排名 #{{ subject.rank }}</span>
            </div>
          </div>
        </div>
      </section>
      <section class="hero">
        <div class="hero-info">
          <div v-if="!cloudOnly" class="hero-stats">
            <div class="stat"><b>{{ d.item_count }}</b><span>资源</span></div>
            <div class="stat"><b>{{ d.saved_count }}</b><span>已保存</span></div>
            <div class="stat"><b>{{ totalEpisodes }}</b><span>集</span></div>
            <div class="stat"><b>{{ d.seasons.length }}</b><span>季</span></div>
          </div>
          <p v-if="summary" class="summary">{{ summary }}</p>
          <p v-else-if="subject" class="hero-meta">Bangumi 暂无简介</p>
          <p v-if="s.subjectLoading" class="hero-meta" role="status">正在加载 Bangumi 详情…</p>
          <p v-if="s.subjectError" class="resource-error" role="alert">{{ s.subjectError }} <BaseButton size="sm" @click="dialogs.reloadBangumiSubject()">重试资料</BaseButton></p>
          <div v-if="metaBits.length || subject" class="hero-meta">
            <template v-for="(b, i) in metaBits" :key="i">
              <span class="dot" v-if="i > 0" aria-hidden="true">·</span>
              <span>{{ b }}</span>
            </template>
            <span v-if="subject && !subject.rating">暂无评分</span>
            <a v-if="subject" :href="`https://bgm.tv/subject/${subject.id}`" target="_blank" rel="noopener noreferrer">Bangumi 条目</a>
          </div>
          <p v-if="!cloudOnly" class="hero-rules">
            <template v-if="d.include_rules">
              多来源规则：包含 <code>{{ d.include_rules }}</code>
              <template v-if="d.exclude_rules"> · 排除 <code>{{ d.exclude_rules }}</code></template>
            </template>
            <template v-else>未设置多来源规则，仅按识别标题归并</template>
          </p>
          <p v-if="d.sources?.length" class="hero-sources">来源订阅：{{ d.sources.join('、') }}</p>
          <div v-if="!cloudOnly" class="hero-actions">
            <BaseButton size="sm" :icon="Pencil" @click="dialogs.openAnimeEdit(d)">编辑番剧</BaseButton>
            <BaseButton size="sm" :icon="Link" :variant="d.bangumi_id ? 'default' : 'soft'" @click="dialogs.openBind(d)">
              {{ d.bangumi_id ? '更换元数据' : '元数据绑定' }}
            </BaseButton>
            <BaseButton size="sm" :icon="RefreshCw" :loading="isBusy('rematch')" @click="rematchMetadata">重新识别元数据</BaseButton>
            <BaseButton size="sm" :icon="Merge" :disabled="store.animes.value.length < 2" @click="dialogs.openAnimeManage('merge', [d])">合并番剧</BaseButton>
            <BaseButton size="sm" :icon="Trash2" variant="danger" @click="dialogs.openAnimeManage('delete', [d])">删除番剧</BaseButton>
          </div>
        </div>
      </section>

      <section class="resource-status" aria-label="本地资源状态" aria-live="polite" :aria-busy="cloudOnly && s.loading">
        <h3>本地资源</h3>
        <p>{{ resourceState }}</p>
        <p v-if="downloadingCount">{{ downloadingCount }} 条资源正在下载或等待下载器完成，尚不代表已保存。</p>
        <p v-if="s.error" class="resource-error" role="alert">资源状态刷新失败：{{ s.error }} <BaseButton size="sm" :loading="s.loading" @click="dialogs.reloadAnimeDetail()">重试</BaseButton></p>
        <p class="resource-hint">仅检查应用内关联的资源记录，不扫描磁盘上的其他文件。打开详情不会自动订阅或下载。</p>
        <BaseButton :icon="Search" variant="primary" :disabled="cloudOnly && (s.loading || !!s.error)" @click="findResources">找资源并选择下载</BaseButton>
        <p class="resource-hint">进入搜索后核对季数、字幕组和版本，再自行选择订阅或下载。</p>
      </section>

      <section v-if="d.voices?.length" class="voices">
        <h3>声优信息</h3>
        <div class="voice-grid">
          <div v-for="v in d.voices" :key="v.person + v.character" class="voice-card">
            <img v-if="v.person_image" :src="v.person_image" :alt="v.person" loading="lazy" />
            <div><b>{{ v.person }}</b><span>{{ v.character }}</span></div>
          </div>
        </div>
      </section>

      <EmptyState v-if="!cloudOnly && !d.seasons.length" :icon="Inbox" title="该番剧下暂无资源" compact />

      <div v-else-if="d.seasons.length" class="seasons" :class="{ refreshing: s.loading }">
        <section v-for="se in d.seasons" :key="se.season" class="season">
          <h3 class="season-title">
            <span>{{ seasonLabel(se.season) }}</span>
            <span class="season-count">{{ se.episodes.length }} 集{{ se.loose.length ? ` · ${se.loose.length} 个未识别` : '' }}</span>
          </h3>
          <div v-for="ep in se.episodes" :key="ep.episode" class="ep-block">
            <div class="ep-title">
              <span class="ep-num">第 {{ ep.episode }} 集</span>
              <span v-if="ep.items.length > 1" class="tag accent">{{ ep.items.length }} 个版本</span>
            </div>
            <ResourceRow
              v-for="it in ep.items"
              :key="it.id"
              :item="it"
              :busy="isBusy(it.id)"
              @download="download(it)"
              @correct="dialogs.openCorrect(it)"
              @remove="remove(it)"
              @play="play(it)"
            />
          </div>
          <div v-if="se.loose.length" class="ep-block">
            <div class="ep-title"><span class="ep-num loose">未识别集数</span></div>
            <ResourceRow
              v-for="it in se.loose"
              :key="it.id"
              :item="it"
              :busy="isBusy(it.id)"
              @download="download(it)"
              @correct="dialogs.openCorrect(it)"
              @remove="remove(it)"
              @play="play(it)"
            />
          </div>
        </section>
      </div>
    </template>

    <template #footer>
      <BaseButton @click="close">关闭</BaseButton>
    </template>
  </BaseDialog>
</template>

<style scoped>
.collection-status { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 10px; color: var(--text-3); font-size: 12px; }
.resource-status { display: grid; justify-items: start; gap: 10px; padding: 16px; margin-bottom: 18px; border: 1px solid var(--line); border-radius: var(--r-md); background: var(--surface-2); }
.resource-status h3 { font-size: 14px; }
.resource-status p { font-size: 13px; line-height: 1.7; color: var(--text-2); }
.resource-status .resource-hint { font-size: 12px; color: var(--text-3); }
.resource-error { color: var(--red); font-size: 13px; }
/* 头图分三层，过渡连续无分界：底层是同一封面的模糊版；上层清晰封面用遮罩从中间向上下渐隐，
   露出底下的模糊版；最上层再把上下边缘平滑淡入弹窗背景色。没有封面时退回主题色渐变。 */
.banner {
  position: relative;
  overflow: hidden;
  margin: -20px -24px 18px;
  min-height: 320px;
  display: flex;
  align-items: flex-end;
  background: linear-gradient(135deg, var(--accent-soft), var(--surface-2));
}
.banner-bg {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  max-width: none;
  object-fit: cover;
  object-position: center 60%;
}
/* 模糊版与清晰版必须完全对齐（不能缩放），否则交界处会出现错位重影；
   边缘的模糊羽化被顶部与底部的背景淡入盖住。 */
.banner-bg.blurred {
  filter: blur(20px) saturate(1.1);
}
.banner-bg.sharp {
  -webkit-mask-image: linear-gradient(to bottom, transparent 0%, rgba(0, 0, 0, 0.6) 8%, #000 18%, #000 48%, rgba(0, 0, 0, 0.6) 64%, transparent 84%);
  mask-image: linear-gradient(to bottom, transparent 0%, rgba(0, 0, 0, 0.6) 8%, #000 18%, #000 48%, rgba(0, 0, 0, 0.6) 64%, transparent 84%);
}
/* 多个停靠点让淡入曲线接近缓动，比两点线性渐变更不容易看出一条带子。 */
.banner-shade {
  position: absolute;
  inset: 0;
  background:
    linear-gradient(to bottom,
      color-mix(in srgb, var(--surface) 70%, transparent) 0%,
      color-mix(in srgb, var(--surface) 30%, transparent) 10%,
      transparent 22%),
    linear-gradient(to top,
      var(--surface) 0%,
      color-mix(in srgb, var(--surface) 92%, transparent) 12%,
      color-mix(in srgb, var(--surface) 74%, transparent) 26%,
      color-mix(in srgb, var(--surface) 46%, transparent) 40%,
      color-mix(in srgb, var(--surface) 20%, transparent) 54%,
      transparent 70%);
}
.banner:not(.has-cover) .banner-shade {
  background: linear-gradient(to bottom, transparent 40%, var(--surface) 100%);
}
.banner-content {
  position: relative;
  display: flex;
  align-items: flex-end;
  gap: 20px;
  width: 100%;
  padding: 36px 24px 18px;
}
.banner-text {
  flex: 1;
  min-width: 0;
  display: grid;
  gap: 6px;
  padding-bottom: 4px;
}
.banner-title {
  margin: 0;
  font-size: 24px;
  font-weight: 750;
  line-height: 1.3;
  letter-spacing: -0.01em;
  overflow-wrap: anywhere;
}
.banner-sub {
  margin: 0;
  color: var(--text-2);
  font-size: 13px;
  overflow-wrap: anywhere;
}
.has-cover .banner-title { color: var(--text); }
.has-cover .banner-title,
.has-cover .banner-sub,
.has-cover .banner-facts {
  text-shadow: 0 0 12px var(--surface), 0 0 4px var(--surface);
}
.banner-facts {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px 12px;
  margin-top: 4px;
  color: var(--text-2);
  font-size: 13px;
}
.banner-rating { color: var(--amber); }
.banner-rating b { font-size: 20px; font-weight: 750; }
.hero {
  display: flex;
  gap: 20px;
  padding: 0 0 20px;
  border-bottom: 1px solid var(--line);
  margin-bottom: 18px;
}
.hero-cover {
  width: 132px;
  flex: none;
  box-shadow: 0 12px 32px rgba(0, 0, 0, 0.35);
}
.hero-lines {
  flex: 1;
  display: grid;
  gap: 12px;
  align-content: start;
  padding-top: 4px;
}
.hero-info {
  flex: 1;
  min-width: 0;
  display: grid;
  gap: 10px;
  align-content: start;
}
.rating { color: var(--amber); font-size: 14px; }
.rating b { font-size: 22px; }
.summary { color: var(--text-2); font-size: 13px; line-height: 1.65; max-height: 110px; overflow: auto; white-space: pre-line; }
.voices { border-bottom: 1px solid var(--line); padding-bottom: 18px; margin-bottom: 18px; }
.voices h3 { margin: 0 0 10px; font-size: 14px; }
.voice-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(180px, 1fr)); gap: 8px; }
.voice-card { display: flex; align-items: center; gap: 9px; padding: 7px; border: 1px solid var(--line); border-radius: var(--r-md); }
.voice-card img { width: 38px; height: 38px; border-radius: 50%; object-fit: cover; background: var(--surface-2); }
.voice-card div { min-width: 0; display: grid; gap: 2px; }
.voice-card span { color: var(--text-3); font-size: 12px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.hero-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 8px;
  color: var(--text-2);
  font-size: 13px;
}
.hero-meta .dot {
  color: var(--text-3);
}
.hero-stats {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
.stat {
  display: flex;
  align-items: baseline;
  gap: 5px;
  padding: 6px 12px;
  border-radius: var(--r-sm);
  background: var(--surface-2);
  border: 1px solid var(--line);
}
.stat b {
  font-size: 16px;
  font-weight: 750;
  font-variant-numeric: tabular-nums;
  letter-spacing: -0.02em;
}
.stat span {
  font-size: 12px;
  color: var(--text-3);
}
.hero-rules,
.hero-sources {
  color: var(--text-2);
  font-size: 13px;
  line-height: 1.6;
  overflow-wrap: anywhere;
}
.hero-rules code {
  font-family: var(--mono);
  font-size: 12px;
  padding: 1px 6px;
  border-radius: 5px;
  background: var(--surface-3);
}
.hero-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  margin-top: 2px;
}

.seasons {
  display: grid;
  gap: 20px;
  padding-bottom: 8px;
  transition: opacity var(--t-med) var(--ease-out);
}
.seasons.refreshing {
  opacity: 0.7;
}
.season-title {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: var(--accent);
  margin-bottom: 10px;
  padding-bottom: 8px;
  border-bottom: 1px dashed var(--line-strong);
}
.season-count {
  color: var(--text-3);
  letter-spacing: 0;
  font-weight: 550;
  text-transform: none;
}
.ep-block {
  border: 1px solid var(--line);
  border-radius: var(--r-md);
  background: var(--surface);
  padding: 6px;
  margin-bottom: 10px;
  animation: rise-in var(--t-slow) var(--ease-out) both;
}
.ep-title {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 12px 8px;
}
.ep-num {
  font-weight: 700;
  font-size: 13.5px;
}
.ep-num.loose {
  color: var(--text-2);
  font-weight: 600;
}

@media (max-width: 640px) {
  .banner { margin: -18px -18px 16px; min-height: 200px; }
  .banner-content { gap: 14px; padding: 28px 18px 14px; }
  .banner-title { font-size: 19px; }
  .hero-cover { width: 92px; }
}
@media (prefers-reduced-transparency: reduce) {
  .banner-bg.blurred { display: none; }
  .has-cover .banner-shade { background: linear-gradient(to top, var(--surface) 0%, color-mix(in srgb, var(--surface) 85%, transparent) 100%); }
}
</style>
