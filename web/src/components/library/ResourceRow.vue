<script setup lang="ts">
import { computed } from 'vue'
import { Download, PenLine, Play, RotateCw, Trash2 } from '@lucide/vue'
import StatusBadge from '@/components/ui/StatusBadge.vue'
import BaseButton from '@/components/ui/BaseButton.vue'
import { badgeInfo, isRetryable } from '@/utils/status'
import type { ResourceItem } from '@/api/types'
import { formatSpeed } from '@/utils/format'

const props = defineProps<{ item: ResourceItem; busy?: boolean }>()
const emit = defineEmits<{ download: []; correct: []; remove: []; play: [] }>()

const retryable = computed(() => isRetryable(props.item.status))
const isPending = computed(() => props.item.status === '待下载' || props.item.status === '等待下载')
const playable = computed(() => badgeInfo(props.item.status).tone === 'saved' && !!props.item.file_path)
</script>

<template>
  <div class="res-row">
    <StatusBadge :status="item.status" size="sm" class="res-badge" />
    <div class="res-info">
      <p class="res-title" :title="item.title">{{ item.title }}</p>
      <p class="res-meta">
        {{ item.fansub_group || '未知字幕组' }} · {{ item.quality || '未知清晰度' }}
        <template v-if="item.feed_name"> · 来源 {{ item.feed_name }}</template>
        <span v-if="item.manual" class="tag amber">已纠错</span>
      </p>
      <p v-if="item.file_path" class="path" :title="item.file_path">{{ item.file_path }}</p>
      <p v-if="badgeInfo(item.status).tone === 'downloading'" class="res-meta">{{ item.progress || 0 }}% · {{ formatSpeed(item.download_speed) }}</p>
    </div>
    <div class="res-actions">
      <BaseButton v-if="playable" size="sm" variant="primary" :icon="Play" @click="emit('play')">播放</BaseButton>
      <BaseButton
        v-if="retryable"
        size="sm"
        :variant="isPending ? 'soft' : 'default'"
        :icon="isPending ? Download : RotateCw"
        :loading="busy"
        @click="emit('download')"
      >
        {{ isPending ? '下载' : '重试' }}
      </BaseButton>
      <BaseButton size="sm" :icon="PenLine" @click="emit('correct')">纠错</BaseButton>
      <BaseButton size="sm" variant="danger" :icon="Trash2" @click="emit('remove')">移除</BaseButton>
    </div>
  </div>
</template>

<style scoped>
.res-row {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  gap: 8px 12px;
  align-items: start;
  padding: 10px 12px;
  border-radius: var(--r-md);
  transition: background var(--t-fast) var(--ease-out);
}
.res-row:hover {
  background: var(--surface-2);
}
.res-badge {
  margin-top: 2px;
}
.res-info {
  min-width: 0;
  display: grid;
  gap: 3px;
}
.res-title {
  font-size: 13px;
  font-weight: 550;
  line-height: 1.45;
  overflow-wrap: anywhere;
}
.res-meta {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px 6px;
  font-size: 12px;
  color: var(--text-3);
}
.res-actions {
  display: flex;
  gap: 5px;
  flex-wrap: wrap;
  justify-content: flex-end;
}
@media (max-width: 640px) {
  .res-row {
    grid-template-columns: auto minmax(0, 1fr);
  }
  .res-actions {
    grid-column: 1 / -1;
    justify-content: flex-start;
  }
}
</style>
