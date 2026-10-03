<script setup lang="ts">
import { computed, ref } from 'vue'
import { ChevronDown, Download, FolderOpen, Pause, PenLine, Play, RotateCw, Trash2, Tv } from '@lucide/vue'
import CoverImage from '@/components/ui/CoverImage.vue'
import StatusBadge from '@/components/ui/StatusBadge.vue'
import ProgressBar from '@/components/ui/ProgressBar.vue'
import BaseButton from '@/components/ui/BaseButton.vue'
import { badgeInfo, isPausable, isResumable, isRetryable } from '@/utils/status'
import { clamp, formatTime, formatSpeed } from '@/utils/format'
import type { Item } from '@/api/types'
import { MODE_LABEL } from '@/api/types'

const props = defineProps<{ item: Item; busy?: boolean; selecting?: boolean; selected?: boolean; selectionDisabled?: boolean }>()
const emit = defineEmits<{ download: []; pause: []; resume: []; storage: []; correct: []; remove: []; play: []; toggle: [] }>()

const badge = computed(() => badgeInfo(props.item.status))
const downloading = computed(() => badge.value.tone === 'downloading')
const expanded = ref(false)
const progress = computed(() => Number(clamp(props.item.progress || 0, 0, 100).toFixed(1)))
const retryable = computed(() => isRetryable(props.item.status))
const pausable = computed(() => isPausable(props.item.status))
const resumable = computed(() => isResumable(props.item.status))
const isPending = computed(() => props.item.status === '待下载' || props.item.status === '等待下载')
const playable = computed(() => badge.value.tone === 'saved' && !!props.item.file_path)
const showProgress = computed(() => downloading.value || ((resumable.value || props.item.status === '等待继续') && progress.value > 0))
const progressHint = computed(() => {
  if (resumable.value) return props.item.downloader === 'direct' ? '继续时将从头下载' : '进度已保留'
  if (props.item.status === '等待继续') return '等待空闲名额后继续'
  if (props.item.status === '继续中') return '正在确认继续状态'
  return props.item.download_speed > 0 ? formatSpeed(props.item.download_speed) : '等待传输'
})
</script>

<template>
  <article class="item-row" :class="{ selecting, selected: selecting && selected, failed: badge.tone === 'failed' }" :aria-busy="busy || undefined">
    <label v-if="selecting" class="select-box"><input type="checkbox" :checked="selected" :disabled="busy || selectionDisabled" :aria-label="`选择 ${item.title}`" @change="emit('toggle')" /></label>
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
        <span class="feed-name">{{ item.feed_name || '未知订阅' }}</span>
      </div>
      <p v-if="item.task_error" class="error-line">{{ item.task_error }}</p>
      <div v-if="showProgress" class="transfer" :class="{ paused: resumable }">
        <div class="transfer-label"><span>{{ progressHint }}</span><span>{{ progress }}%</span></div>
        <ProgressBar :value="progress" :aria-label="`${item.anime_title || item.title} 下载进度`" />
      </div>
    </div>

    <div class="side">
      <StatusBadge :status="item.status" />
      <div class="actions">
        <BaseButton v-if="playable" size="sm" variant="primary" :icon="Play" :disabled="busy" @click="emit('play')">播放</BaseButton>
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
        <BaseButton v-if="pausable" size="sm" :icon="Pause" :loading="busy" @click="emit('pause')">暂停</BaseButton>
        <BaseButton v-if="resumable" size="sm" variant="soft" :icon="Play" :loading="busy" @click="emit('resume')">继续</BaseButton>
        <BaseButton size="sm" variant="ghost" :icon="ChevronDown" class="details-toggle" :class="{ expanded }" :aria-expanded="expanded" :aria-controls="`item-details-${item.id}`" @click="expanded = !expanded">{{ expanded ? '收起' : '详情' }}</BaseButton>
      </div>
    </div>

    <div v-if="expanded" :id="`item-details-${item.id}`" class="item-details">
      <dl>
        <div><dt>原始标题</dt><dd>{{ item.title }}</dd></div>
        <div><dt>发布时间</dt><dd>{{ formatTime(item.published) }}</dd></div>
        <div v-if="item.downloader"><dt>下载方式</dt><dd>{{ MODE_LABEL[item.downloader] || item.downloader }}</dd></div>
        <div v-if="badge.tone === 'saved' && item.file_path"><dt>归档位置</dt><dd class="path">{{ item.file_path }}</dd></div>
      </dl>
      <div class="detail-actions">
        <BaseButton v-if="item.anime_title" size="sm" :icon="FolderOpen" :disabled="busy" @click="emit('storage')">存储位置</BaseButton>
        <BaseButton size="sm" :icon="PenLine" :disabled="busy" @click="emit('correct')">纠错</BaseButton>
        <BaseButton size="sm" variant="danger" :icon="Trash2" :disabled="busy" @click="emit('remove')">移除记录</BaseButton>
        <span>移除记录不会删除已保存文件</span>
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
  box-shadow: var(--shadow-sm);
}
.item-row.failed { border-left: 3px solid var(--red); }
.item-row.selected { border-color: var(--accent-border); background: var(--accent-soft); }
.select-box { display: flex; align-items: flex-start; justify-content: center; padding: 8px 0; cursor: pointer; }
.feed-name { overflow-wrap: anywhere; }
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
.error-line {
  color: var(--red);
  font-size: 12.5px;
  line-height: 1.5;
  overflow-wrap: anywhere;
}
.transfer {
  display: grid;
  gap: 6px;
  margin-top: 4px;
  max-width: 460px;
}
.transfer-label { display: flex; justify-content: space-between; gap: 12px; color: var(--text-2); font-size: 12px; font-variant-numeric: tabular-nums; }
.transfer.paused :deep(.bar) { background: var(--gray); }
.transfer.paused :deep(.glow) { animation: none; display: none; }
.item-row.selecting {
  grid-template-columns: 22px 54px minmax(0, 1fr) auto;
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
.details-toggle :deep(svg) { transition: transform var(--t-fast) var(--ease-out); }
.details-toggle.expanded :deep(svg) { transform: rotate(180deg); }
.item-details { grid-column: 1 / -1; min-width: 0; border-top: 1px solid var(--line); padding-top: 12px; }
.item-details dl { display: grid; gap: 8px; font-size: 12px; line-height: 1.6; }
.item-details dl > div { display: grid; grid-template-columns: 64px minmax(0, 1fr); gap: 12px; }
.item-details dt { color: var(--text-3); }
.item-details dd { margin: 0; color: var(--text-2); overflow-wrap: anywhere; user-select: text; }
.detail-actions { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; margin-top: 12px; }
.detail-actions > span { color: var(--text-3); font-size: 12px; margin-left: auto; }

@media (max-width: 1100px) {
  .item-row {
    grid-template-columns: 46px minmax(0, 1fr);
    padding: 12px 13px;
    gap: 10px 12px;
  }
  .item-row.selecting {
    grid-template-columns: 22px 46px minmax(0, 1fr);
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
@media (max-width: 640px) {
  .side { gap: 8px; }
  .actions { margin-left: auto; gap: 4px; }
  .item-details dl > div { grid-template-columns: 56px minmax(0, 1fr); gap: 8px; }
  .detail-actions > span { flex-basis: 100%; margin-left: 0; }
}
</style>
