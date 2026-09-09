<script setup lang="ts">
import { computed } from 'vue'
import { CircleCheck, Film, Tv, Pencil, Merge, Trash2 } from '@lucide/vue'
import BaseButton from '@/components/ui/BaseButton.vue'
import CoverImage from '@/components/ui/CoverImage.vue'
import type { AnimeSummary } from '@/api/types'

const props = defineProps<{ anime: AnimeSummary; selecting?: boolean; selected?: boolean; canMerge?: boolean }>()
const emit = defineEmits<{ open: [id: number]; toggle: [id: number]; edit: []; merge: []; remove: [] }>()

const displayName = computed(() => props.anime.meta_name_cn || props.anime.title)
const showSub = computed(() => displayName.value !== props.anime.title)
const ratio = computed(() =>
  props.anime.item_count ? Math.round((props.anime.saved_count / props.anime.item_count) * 100) : 0,
)
</script>

<template>
  <article class="anime-card" :class="{ selected }">
    <button class="card-open" type="button" :aria-label="`${selecting ? '选择' : '打开'} ${displayName}`" :aria-pressed="selecting ? !!selected : undefined" @click="selecting ? emit('toggle', anime.id) : emit('open', anime.id)">
    <div class="poster-wrap">
      <CoverImage :src="anime.cover" :alt="displayName" radius="0" class="poster" />
      <div class="poster-shade" aria-hidden="true" />
      <div class="poster-tags">
        <span v-if="anime.bangumi_id" class="tag accent">已绑定</span>
        <span v-else-if="anime.include_rules" class="tag">多来源</span>
        <span v-if="anime.season > 1" class="tag">第 {{ anime.season }} 季</span>
      </div>
    </div>
    <div class="body">
      <h3 class="title" :title="displayName">{{ displayName }}</h3>
      <p v-if="showSub" class="sub truncate" :title="anime.title">{{ anime.title }}</p>
      <div class="stats">
        <span><Film aria-hidden="true" />{{ anime.item_count }} 资源</span>
        <span><CircleCheck aria-hidden="true" />{{ anime.saved_count }} 已保存</span>
        <span v-if="anime.episode_count"><Tv aria-hidden="true" />{{ anime.episode_count }} 集</span>
      </div>
      <div class="ratio" :title="`已保存 ${ratio}%`" aria-hidden="true">
        <i :style="{ width: ratio + '%' }" />
      </div>
    </div>
    </button>
    <div class="card-actions">
      <label v-if="selecting" class="select-label"><input type="checkbox" :checked="selected" :aria-label="`选择 ${displayName}`" @change="emit('toggle', anime.id)" />{{ selected ? '已选择' : '选择' }}</label>
      <template v-else>
        <BaseButton :icon="Pencil" icon-only size="sm" variant="ghost" title="编辑番剧" aria-label="编辑番剧" @click="emit('edit')" />
        <BaseButton :icon="Merge" icon-only size="sm" variant="ghost" :disabled="!canMerge" title="合并番剧" aria-label="合并番剧" @click="emit('merge')" />
        <BaseButton :icon="Trash2" icon-only size="sm" variant="danger" title="删除番剧" aria-label="删除番剧" @click="emit('remove')" />
      </template>
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
.anime-card:hover,
.anime-card:focus-visible {
  transform: translateY(-4px);
  border-color: var(--accent-border);
  box-shadow: var(--shadow-md), 0 0 0 1px var(--accent-border);
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
    transform 900ms var(--ease-out);
}
.anime-card:hover .poster :deep(img.loaded) {
  transform: scale(1.06);
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
}
.poster-tags .tag {
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
  background: rgba(20, 22, 30, 0.6);
  color: #f1f3f8;
  border-color: rgba(255, 255, 255, 0.14);
}
.poster-tags .tag.accent {
  background: rgba(139, 124, 255, 0.55);
  border-color: rgba(255, 255, 255, 0.22);
  color: #fff;
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
