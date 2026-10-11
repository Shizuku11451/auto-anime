<script setup lang="ts">
import { computed, ref } from 'vue'
import { ArrowRight, ExternalLink } from '@lucide/vue'
import BaseButton from '@/components/ui/BaseButton.vue'
import { formatTime } from '@/utils/format'
import { episodeSummary, seasonLabel, type Release } from '@/utils/discover'
import type { DiscoverItem } from '@/api/types'

const props = defineProps<{ release: Release; editing: boolean }>()
defineEmits<{ subscribe: [item: DiscoverItem] }>()
const expanded = ref(false)
const visibleCount = ref(20)
const items = computed(() => props.release.items.slice(0, visibleCount.value))
const isWebLink = (link: string) => /^https?:\/\//i.test(link)
</script>

<template>
  <article class="release-card">
    <div class="release-heading">
      <div class="release-info">
        <h3>{{ release.title }}</h3>
        <div class="version-tags">
          <span>{{ seasonLabel(release.item.season) }}</span>
          <span class="group-tag">{{ release.item.fansub_group || '字幕组未识别' }}</span>
          <span v-if="release.item.quality">{{ release.item.quality }}</span>
          <span v-if="release.language">{{ release.language }}</span>
        </div>
      </div>
      <BaseButton variant="soft" size="sm" :icon="ArrowRight" :disabled="(release.item.season ?? 0) < 0" @click="$emit('subscribe', release.item)">
        {{ editing ? '预览并调整' : '预览并订阅' }}
      </BaseButton>
    </div>
    <div class="release-stats">
      <span v-if="release.episodes.length"><strong>{{ release.episodes.length }}</strong> 集已发现</span>
      <span>{{ release.items.length }} 条资源</span>
      <span v-if="release.latest">最近发布 {{ formatTime(new Date(release.latest).toISOString()) }}</span>
    </div>
    <p v-if="release.episodes.length" class="episodes">集数：{{ episodeSummary(release.episodes) }}</p>
    <p v-else class="notice">尚未识别出单集编号，请展开核对原始标题。</p>
    <p v-if="!release.item.fansub_group" class="notice">字幕组未识别，默认关闭自动下载；订阅前请核对过滤规则。</p>
    <p v-if="!release.item.season" class="notice">季数未标明，默认关闭自动下载，请人工核对。</p>
    <p v-if="(release.item.season ?? 0) < 0" class="notice">标题中的季数互相冲突，不能自动订阅。</p>
    <p v-if="release.item.query" class="notice">实际 RSS 查询：{{ release.item.query }}；本卡片仅包含该 RSS 的资源。</p>
    <details class="resource-details" @toggle="expanded = ($event.target as HTMLDetailsElement).open">
      <summary>查看集数与原始标题 <span>{{ release.items.length }} 条</span></summary>
      <div v-if="expanded" class="resource-list">
        <div v-for="(item, index) in items" :key="index" class="resource-item">
          <p>{{ item.title }}</p>
          <div class="resource-meta">
            <span>{{ /^\d{1,3}$/.test(item.episode) ? `第 ${item.episode} 集` : '合集或集数未识别' }}</span>
            <span>{{ formatTime(item.published) }}</span>
            <a v-if="isWebLink(item.link)" :href="item.link" target="_blank" rel="noopener noreferrer">
              资源页 <ExternalLink aria-hidden="true" />
            </a>
          </div>
        </div>
        <BaseButton v-if="visibleCount < release.items.length" size="sm" @click="visibleCount += 20">
          再显示 {{ Math.min(20, release.items.length - visibleCount) }} 条（剩余 {{ release.items.length - visibleCount }} 条）
        </BaseButton>
      </div>
    </details>
  </article>
</template>

<style scoped>
.release-card { min-width: 0; padding: 20px; background: var(--surface); border: 1px solid var(--line); border-radius: var(--r-lg); }
.release-heading { display: flex; justify-content: space-between; align-items: flex-start; gap: 16px; }
.release-heading > .btn { flex: none; }
.release-info { min-width: 0; }
h3 { font-size: 16px; font-weight: 650; line-height: 1.5; overflow-wrap: anywhere; }
.version-tags { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 10px; }
.version-tags span { padding: 3px 9px; border-radius: var(--r-xs); background: var(--surface-2); color: var(--text-2); font-size: 12px; overflow-wrap: anywhere; }
.version-tags .group-tag { color: var(--accent); background: var(--accent-soft); }
.release-stats { display: flex; flex-wrap: wrap; gap: 6px 16px; margin-top: 16px; font-size: 12px; color: var(--text-3); }
.release-stats strong { color: var(--text); font-size: 15px; }
.episodes, .notice { margin-top: 6px; font-size: 12px; line-height: 1.7; overflow-wrap: anywhere; }
.episodes { color: var(--text-2); }
.notice { color: var(--amber); }
.resource-details { margin-top: 14px; border-top: 1px solid var(--line); }
summary { padding-top: 12px; font-size: 12px; color: var(--text-2); cursor: pointer; }
summary span { margin-left: 6px; color: var(--text-3); }
.resource-list { display: grid; gap: 10px; max-height: 340px; overflow: auto; margin-top: 12px; padding-right: 4px; }
.resource-item { padding: 10px 12px; border-radius: var(--r-sm); background: var(--surface-2); }
.resource-item p { font-size: 12px; line-height: 1.7; overflow-wrap: anywhere; }
.resource-meta { display: flex; flex-wrap: wrap; gap: 6px 12px; margin-top: 4px; color: var(--text-3); font-size: 11.5px; }
.resource-meta a { display: inline-flex; align-items: center; gap: 4px; }
.resource-meta svg { width: 12px; height: 12px; }
@media (max-width: 560px) {
  .release-card { padding: 16px; }
  .release-heading { flex-direction: column; gap: 12px; }
  .release-heading > .btn { width: 100%; height: 38px; }
}
</style>
