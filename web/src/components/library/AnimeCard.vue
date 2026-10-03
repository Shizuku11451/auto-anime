<script setup lang="ts">
import { computed, ref } from 'vue'
import { CircleCheck, Film, Star, ChevronDown, CalendarDays } from '@lucide/vue'
import CoverImage from '@/components/ui/CoverImage.vue'
import type { AnimeSummary } from '@/api/types'
import { scheduleLabel } from '@/components/library/schedule'

const props = defineProps<{ anime: AnimeSummary; selecting?: boolean; selected?: boolean; compact?: boolean }>()
const emit = defineEmits<{ open: [id: number]; toggle: [id: number] }>()

const displayName = computed(() => props.anime.meta_name_cn || props.anime.title)
const showSub = computed(() => displayName.value !== props.anime.title)
const expanded = ref(false)
const ratio = computed(() =>
  props.anime.item_count ? Math.round((props.anime.saved_count / props.anime.item_count) * 100) : 0,
)
</script>

<template>
  <article class="anime-card" :class="{ selected, compact }" :style="{ '--cover-tone': ['var(--accent)', 'var(--blue)', 'var(--amber)'][anime.id % 3] }">
    <button class="card-open" type="button" :aria-label="`${selecting ? '选择' : '打开'} ${displayName}`" :aria-pressed="selecting ? !!selected : undefined" @click="selecting ? emit('toggle', anime.id) : emit('open', anime.id)">
    <div class="poster-wrap">
      <CoverImage :src="anime.cover" :alt="displayName" radius="0" class="poster">
        <div v-if="!anime.cover" class="poster-fallback" aria-hidden="true"><span>{{ displayName.slice(0, 2) }}</span><small>暂无封面</small></div>
      </CoverImage>
      <div v-if="anime.cover" class="poster-shade" aria-hidden="true" />
      <span v-if="anime.rating > 0" class="rating-badge" :title="`Bangumi 评分 ${anime.rating.toFixed(1)}`">
        <Star aria-hidden="true" />{{ anime.rating.toFixed(1) }}
      </span>
      <div class="poster-tags">
        <span v-if="anime.include_rules" class="tag">多来源</span>
        <span v-if="anime.season > 1" class="tag">第 {{ anime.season }} 季</span>
      </div>
      <div v-if="anime.summary" class="poster-veil" aria-hidden="true">
        <p class="veil-text">{{ anime.summary }}</p>
      </div>
    </div>
    <div class="body">
      <h3 class="title" :title="displayName">{{ displayName }}</h3>
      <p v-if="showSub" class="sub truncate" :title="anime.title">{{ anime.title }}</p>
      <p class="schedule"><CalendarDays aria-hidden="true" />{{ scheduleLabel(anime) }}</p>
      <div class="stats">
        <span><Film aria-hidden="true" />{{ anime.item_count }} 资源</span>
        <span><CircleCheck aria-hidden="true" />{{ anime.saved_count }} 已保存</span>
      </div>
      <div class="ratio" :title="`已保存 ${ratio}%`" aria-hidden="true">
        <i :style="{ width: ratio + '%' }" />
      </div>
    </div>
    </button>
    <button v-if="anime.summary" type="button" class="summary-toggle" :aria-expanded="expanded" @click="expanded = !expanded">
      <span>简介</span>
      <ChevronDown aria-hidden="true" class="chev" :class="{ open: expanded }" />
    </button>
    <div v-if="anime.summary" v-show="expanded" class="summary-panel">{{ anime.summary }}</div>
    <div v-if="selecting" class="card-actions">
      <label class="select-label"><input type="checkbox" :checked="selected" :aria-label="`选择 ${displayName}`" @change="emit('toggle', anime.id)" />{{ selected ? '已选择' : '选择' }}</label>
    </div>
  </article>
</template>

<style scoped>
.anime-card {
  min-width: 0;
  display: flex;
  flex-direction: column;
  background: var(--surface);
  border: 1px solid var(--line);
  border-radius: var(--r-lg);
  overflow: hidden;
  cursor: pointer;
  box-shadow: var(--shadow-sm);
  transition:
    transform var(--t-med) var(--ease-out),
    box-shadow var(--t-med) var(--ease-out),
    border-color var(--t-med) var(--ease-out);
}
.card-open { display: flex; flex-direction: column; justify-content: flex-start; width: 100%; min-width: 0; padding: 0; border: 0; background: transparent; color: inherit; text-align: left; font: inherit; cursor: pointer; flex: 1; }
.card-open > * { width: 100%; }
.card-actions { display: flex; align-items: center; justify-content: flex-end; gap: 6px; padding: 8px 10px; border-top: 1px solid var(--line); min-height: 47px; }
.select-label { display: flex; align-items: center; gap: 8px; width: 100%; font-size: 13px; cursor: pointer; }
.anime-card.selected { border-color: var(--accent); }
.anime-card:focus-within { border-color: var(--accent-border); }
.poster-fallback { position: absolute; inset: 0; display: flex; flex-direction: column; justify-content: center; align-items: center; gap: 10px; color: var(--cover-tone); background: linear-gradient(150deg, color-mix(in srgb, var(--cover-tone) 18%, var(--surface)), var(--surface-2)); }
.poster-fallback::before { content: ''; position: absolute; width: 120%; aspect-ratio: 1; border: 1px solid color-mix(in srgb, var(--cover-tone) 14%, transparent); border-radius: 50%; left: 25%; top: 45%; }
.poster-fallback > span { font-size: 28px; font-weight: 650; letter-spacing: .08em; }
.poster-fallback small { font-size: 10px; opacity: .7; }
.compact { padding: 12px; position: relative; }
.compact .card-open { flex-direction: row; gap: 14px; align-items: center; }
.compact .poster-wrap { width: 100px; flex: none; border-radius: var(--r-sm); overflow: hidden; }
.compact .body { flex: 1; min-width: 0; width: auto; padding: 2px 0; gap: 8px; }
.compact .title { font-size: 15px; line-height: 1.5; }
.compact .rating-badge { top: 5px; right: 5px; padding: 3px 5px; font-size: 10px; }
.compact .poster-tags { left: 5px; right: 5px; bottom: 5px; }
.compact .poster-tags .tag { font-size: 9px; }
.compact .summary-toggle { margin-top: 8px; padding: 4px 0 0; border: 0; justify-content: flex-end; gap: 6px; }
.compact .summary-panel { padding: 10px 0 0; margin-top: 8px; }
.compact .card-actions { padding: 8px 0 0; margin-top: 8px; min-height: 32px; }
.compact .ratio { margin-top: 2px; }
.anime-card:hover,
.anime-card:focus-visible {
  transform: translateY(-2px);
  border-color: var(--accent-border);
  box-shadow: var(--shadow-sm);
}
.anime-card:active {
  transform: translateY(-1px) scale(0.995);
}
.poster-wrap {
  position: relative;
}
.poster {
  width: 100%;
}
.poster :deep(img) {
  transition:
    opacity var(--t-slow) var(--ease-out),
    transform 900ms var(--ease-out),
    filter var(--t-med) var(--ease-out);
}
/* 简介轻量淡入，保留封面轮廓；键盘聚焦时同样生效。 */
.poster-wrap:hover .poster :deep(img.loaded),
.card-open:focus-visible .poster :deep(img.loaded) {
  transform: scale(1.025);
}
.poster-shade {
  position: absolute;
  inset: auto 0 0;
  height: 46%;
  background: linear-gradient(to top, rgba(8, 10, 16, 0.72), transparent);
  pointer-events: none;
  opacity: 0.9;
}
.poster-tags {
  position: absolute;
  left: 10px;
  right: 10px;
  bottom: 10px;
  display: flex;
  flex-wrap: wrap;
  gap: 5px;
  transition: opacity var(--t-med) var(--ease-out);
}
.poster-wrap:hover .poster-tags,
.card-open:focus-visible .poster-tags {
  opacity: 0;
}
.poster-veil {
  position: absolute;
  inset: 0;
  z-index: 3;
  display: flex;
  padding: 12px 11px;
  background: rgba(8, 10, 16, 0.42);
  opacity: 0;
  pointer-events: none;
  transition: opacity var(--t-med) var(--ease-out);
}
.poster-wrap:hover .poster-veil,
.card-open:focus-visible .poster-veil {
  opacity: 1;
  pointer-events: auto;
}
.veil-text {
  flex: 1;
  min-height: 0;
  margin: 0;
  overflow: auto;
  scrollbar-width: thin;
  scrollbar-color: rgba(255, 255, 255, 0.35) transparent;
  color: #f1f3f8;
  font-size: 12px;
  line-height: 1.65;
  white-space: pre-line;
  overflow-wrap: anywhere;
  text-shadow: 0 1px 3px rgba(0, 0, 0, 0.55);
}
.poster-tags .tag {
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
  background: rgba(20, 22, 30, 0.6);
  color: #f1f3f8;
  border-color: rgba(255, 255, 255, 0.14);
}
.poster-tags .tag.accent {
  background: rgba(22, 116, 81, 0.85);
  border-color: rgba(255, 255, 255, 0.22);
  color: #fff;
}
.rating-badge {
  position: absolute;
  top: 10px;
  right: 10px;
  z-index: 4;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 3px 8px;
  border-radius: 999px;
  border: 1px solid rgba(255, 255, 255, 0.14);
  background: rgba(20, 22, 30, 0.6);
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
  color: #f1f3f8;
  font-size: 12px;
  font-weight: 650;
  line-height: 1.2;
}
.rating-badge svg {
  width: 12px;
  height: 12px;
  color: var(--amber);
  fill: var(--amber);
}
.summary-toggle {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  padding: 8px 13px;
  border: 0;
  border-top: 1px solid var(--line);
  background: transparent;
  color: var(--text-2);
  font-size: 12px;
  font-family: inherit;
  text-align: left;
  cursor: pointer;
  transition: background var(--t-fast) var(--ease-out), color var(--t-fast) var(--ease-out);
}
.summary-toggle:hover { color: var(--text); background: var(--surface-2); }
.summary-toggle:focus-visible { outline: 2px solid var(--accent-border); outline-offset: -2px; }
.summary-toggle .chev { width: 14px; height: 14px; flex: none; transition: transform var(--t-med) var(--ease-out); }
.summary-toggle .chev.open { transform: rotate(180deg); }
.summary-panel {
  padding: 10px 13px 12px;
  border-top: 1px dashed var(--line);
  color: var(--text-2);
  font-size: 12px;
  line-height: 1.65;
  white-space: pre-line;
  max-height: 180px;
  overflow: auto;
}
.body {
  padding: 12px 13px 13px;
  display: grid;
  gap: 6px;
}
.title {
  overflow-wrap: anywhere;
  font-size: 14px;
  font-weight: 650;
  line-height: 1.35;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.sub {
  font-size: 12px;
  color: var(--text-3);
}
.stats {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 10px;
  margin-top: 2px;
  color: var(--text-2);
  font-size: 12px;
}
.schedule { display: flex; align-items: center; gap: 5px; font-size: 12px; color: var(--text-2); }
.schedule svg { width: 13px; height: 13px; color: var(--accent); }
.schedule svg { flex: none; }
.stats span {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
.stats svg {
  width: 12px;
  height: 12px;
  color: var(--text-3);
}
.ratio {
  height: 3px;
  border-radius: 999px;
  background: var(--surface-3);
  overflow: hidden;
  margin-top: 4px;
}
.ratio i {
  display: block;
  height: 100%;
  border-radius: inherit;
  background: linear-gradient(90deg, var(--accent), var(--green));
  transition: width 700ms var(--ease-out);
}
</style>
