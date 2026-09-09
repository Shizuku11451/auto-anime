<script setup lang="ts">
import { computed } from 'vue'
import { Download, FolderOpen, PenLine, RotateCw, Trash2, Tv } from '@lucide/vue'
import CoverImage from '@/components/ui/CoverImage.vue'
import StatusBadge from '@/components/ui/StatusBadge.vue'
import ProgressBar from '@/components/ui/ProgressBar.vue'
import BaseButton from '@/components/ui/BaseButton.vue'
import { badgeInfo, isRetryable } from '@/utils/status'
import { clamp, formatTime } from '@/utils/format'
import type { Item } from '@/api/types'

const props = defineProps<{ item: Item; busy?: boolean }>()
const emit = defineEmits<{ download: []; storage: []; correct: []; remove: [] }>()

const badge = computed(() => badgeInfo(props.item.status))
const downloading = computed(() => badge.value.tone === 'downloading')
const progress = computed(() => clamp(props.item.progress || 0, 0, 100))
const retryable = computed(() => isRetryable(props.item.status))
const isPending = computed(() => props.item.status === '待下载' || props.item.status === '等待下载')
</script>

<template>
  <article class="item-row">
    <CoverImage :src="item.cover" :icon="Tv" class="thumb" radius="var(--r-sm)" />

    <div class="info">
      <h3 class="title">
        <span class="name">{{ item.anime_title || item.title }}</span>
        <span v-if="item.episode" class="ep">第 {{ item.episode }} 集</span>
        <span v-if="item.season > 0" class="tag">S{{ String(item.season).padStart(2, '0') }}</span>
        <span v-if="item.manual" class="tag amber">已纠错</span>
      </h3>
      <div class="meta-line">
        <span>{{ item.fansub_group || '未知字幕组' }}</span>
        <span>{{ item.quality || '未识别清晰度' }}</span>
        <span>{{ item.feed_name || '未知订阅' }}</span>
        <span>{{ formatTime(item.published) }}</span>
      </div>
      <p class="original" :title="item.title">{{ item.title }}</p>
      <p v-if="badge.tone === 'saved' && item.file_path" class="path" title="归档位置">{{ item.file_path }}</p>
      <p v-if="item.task_error" class="error-line">{{ item.task_error }}</p>
      <ProgressBar v-if="downloading" :value="progress" class="progress" />
    </div>

    <div class="side">
      <StatusBadge :status="item.status" :suffix="downloading && progress > 0 ? ` ${progress}%` : ''" />
      <div class="actions">
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
        <BaseButton v-if="item.anime_title" size="sm" :icon="FolderOpen" @click="emit('storage')">位置</BaseButton>
        <BaseButton size="sm" :icon="PenLine" @click="emit('correct')">纠错</BaseButton>
        <BaseButton size="sm" variant="danger" :icon="Trash2" @click="emit('remove')">移除</BaseButton>
      </div>
    </div>
  </article>
</template>

<style scoped>
.item-row {
  display: grid;
  grid-template-columns: 54px minmax(0, 1fr) auto;
  gap: 14px 16px;
  align-items: start;
  padding: 14px 16px;
  background: var(--surface);
  border: 1px solid var(--line);
  border-radius: var(--r-lg);
  box-shadow: var(--shadow-sm);
  transition:
    border-color var(--t-med) var(--ease-out),
    box-shadow var(--t-med) var(--ease-out),
    transform var(--t-med) var(--ease-out);
}
.item-row:hover {
  border-color: var(--line-strong);
  box-shadow: var(--shadow-md);
}
.thumb {
  width: 54px;
}
.info {
  min-width: 0;
  display: grid;
  gap: 5px;
}
.title {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px 8px;
  font-size: 14.5px;
  font-weight: 650;
  line-height: 1.35;
}
.name {
  min-width: 0;
  overflow-wrap: anywhere;
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
.original {
  color: var(--text-3);
  font-size: 12px;
  line-height: 1.5;
  overflow-wrap: anywhere;
}
.error-line {
  color: var(--red);
  font-size: 12.5px;
  line-height: 1.5;
  overflow-wrap: anywhere;
}
.progress {
  margin-top: 4px;
  max-width: 460px;
}
.side {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 10px;
}
.actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 6px;
}

@media (max-width: 720px) {
  .item-row {
    grid-template-columns: 46px minmax(0, 1fr);
    padding: 12px 13px;
    gap: 10px 12px;
  }
  .thumb {
    width: 46px;
  }
  .side {
    grid-column: 1 / -1;
    flex-direction: row;
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
  }
  .actions {
    justify-content: flex-start;
  }
}
</style>
