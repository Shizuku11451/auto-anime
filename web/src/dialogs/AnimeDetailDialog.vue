<script setup lang="ts">
import { computed } from 'vue'
import { useDocumentVisibility, useIntervalFn } from '@vueuse/core'
import { Inbox, Link, Pencil, Merge, Trash2 } from '@lucide/vue'
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
import type { ResourceItem } from '@/api/types'
import type { PlayerEntry } from '@/stores/dialogs'

const dialogs = useDialogs()
const store = useAppStore()
const s = dialogs.state.animeDetail
const visibility = useDocumentVisibility()
useIntervalFn(() => {
  if (s.open && !s.loading && visibility.value === 'visible') void dialogs.reloadAnimeDetail()
}, 5000)
const d = computed(() => s.data)
const { is: isBusy, run } = useBusySet()

const displayName = computed(() => d.value?.meta_name_cn || d.value?.title || '')

const metaBits = computed(() => {
  const x = d.value
  if (!x) return []
  const bits: string[] = [`识别名 ${x.title}`]
  if (x.meta_name && x.meta_name !== x.meta_name_cn) bits.push(x.meta_name)
  if (x.season > 0) bits.push(`默认第 ${x.season} 季`)
  if (x.bangumi_id) bits.push(`Bangumi #${x.bangumi_id}`)
  if (x.storage_dir) bits.push(`存储 ${x.storage_dir}`)
  return bits
})

const totalEpisodes = computed(() => d.value?.seasons.reduce((n, se) => n + se.episodes.length, 0) ?? 0)

function close() {
  s.open = false
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
  const ok = await confirmDialog('移除记录', '只删除这条资源记录，不会删除 NAS 上已保存的文件。', {
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
      <BaseButton @click="s.id && dialogs.openAnimeDetail(s.id)">重试</BaseButton>
    </EmptyState>

    <template v-else-if="d">
      <section class="hero">
        <CoverImage :src="d.cover" :alt="displayName" class="hero-cover" radius="var(--r-md)" />
        <div class="hero-info">
          <div class="hero-meta">
            <template v-for="(b, i) in metaBits" :key="i">
              <span class="dot" v-if="i > 0" aria-hidden="true">·</span>
              <span>{{ b }}</span>
            </template>
          </div>
          <div class="hero-stats">
            <div class="stat"><b>{{ d.item_count }}</b><span>资源</span></div>
            <div class="stat"><b>{{ d.saved_count }}</b><span>已保存</span></div>
            <div class="stat"><b>{{ totalEpisodes }}</b><span>集</span></div>
            <div class="stat"><b>{{ d.seasons.length }}</b><span>季</span></div>
          </div>
          <div v-if="d.rating" class="rating" :aria-label="`评分 ${d.rating.toFixed(1)} 分`">评分 <b>{{ d.rating.toFixed(1) }}</b> / 10</div>
          <p v-if="d.summary" class="summary">{{ d.summary }}</p>
          <p class="hero-rules">
            <template v-if="d.include_rules">
              多来源规则：包含 <code>{{ d.include_rules }}</code>
              <template v-if="d.exclude_rules"> · 排除 <code>{{ d.exclude_rules }}</code></template>
            </template>
            <template v-else>未设置多来源规则，仅按识别标题归并</template>
          </p>
          <p v-if="d.sources?.length" class="hero-sources">来源订阅：{{ d.sources.join('、') }}</p>
          <div class="hero-actions">
            <BaseButton size="sm" :icon="Pencil" @click="dialogs.openAnimeEdit(d)">编辑番剧</BaseButton>
            <BaseButton size="sm" :icon="Link" :variant="d.bangumi_id ? 'default' : 'soft'" @click="dialogs.openBind(d)">
              {{ d.bangumi_id ? '更换元数据' : '元数据绑定' }}
            </BaseButton>
            <BaseButton size="sm" :icon="Merge" :disabled="store.animes.value.length < 2" @click="dialogs.openAnimeManage('merge', [d])">合并番剧</BaseButton>
            <BaseButton size="sm" :icon="Trash2" variant="danger" @click="dialogs.openAnimeManage('delete', [d])">删除番剧</BaseButton>
          </div>
        </div>
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

      <EmptyState v-if="!d.seasons.length" :icon="Inbox" title="该番剧下暂无资源" compact />

      <div v-else class="seasons" :class="{ refreshing: s.loading }">
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
.hero {
  display: flex;
  gap: 20px;
  padding: 6px 0 20px;
  border-bottom: 1px solid var(--line);
  margin-bottom: 18px;
}
.hero-cover {
  width: 112px;
  box-shadow: var(--shadow-md);
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
  .hero {
    gap: 14px;
  }
  .hero-cover {
    width: 86px;
  }
}
</style>
