<script setup lang="ts">
import { computed } from 'vue'
import { CircleAlert, Pause, Pencil, Play, RefreshCw, Trash2, Zap } from '@lucide/vue'
import BaseButton from '@/components/ui/BaseButton.vue'
import { formatTime, relativeTime, splitKeywords } from '@/utils/format'
import type { Feed } from '@/api/types'

const props = defineProps<{ feed: Feed; syncing?: boolean }>()
const emit = defineEmits<{ sync: []; edit: []; toggle: []; remove: [] }>()

const paused = computed(() => props.feed.enabled === false)
const includes = computed(() => splitKeywords(props.feed.include_words))
const excludes = computed(() => splitKeywords(props.feed.exclude_words))
</script>

<template>
  <article class="feed-card" :class="{ paused }">
    <header class="head">
      <h3 class="name" :title="feed.name">{{ feed.name }}</h3>
      <span v-if="paused && feed.completion_note" class="tag green">已集齐</span>
      <span v-else-if="paused" class="tag amber">已暂停</span>
      <span v-else-if="feed.last_error" class="tag red">同步失败</span>
      <span v-else class="tag green">正常</span>
    </header>

    <p class="url mono truncate" :title="feed.url.startsWith('aggregate:') ? undefined : feed.url">{{ feed.url.startsWith('aggregate:') ? '在线聚合订阅' : feed.url }}</p>
    <p v-if="feed.target_title" class="state" :title="'归入番剧：' + feed.target_title">
      <span>归入番剧：{{ feed.target_title }}</span>
    </p>

    <div class="rules">
      <span class="tag" :class="feed.auto_download ? 'accent' : ''">
        <Zap aria-hidden="true" />{{ feed.auto_download ? '自动下载' : '手动下载' }}
      </span>
      <span v-for="k in includes" :key="'i' + k" class="tag" :title="'包含关键词：' + k"><span class="truncate">含 {{ k }}</span></span>
      <span v-for="k in excludes" :key="'e' + k" class="tag" :title="'排除关键词：' + k"><span class="truncate">排 {{ k }}</span></span>
    </div>

    <p v-if="paused && feed.completion_note" class="state completed" :title="feed.completion_note"><span>{{ feed.completion_note }}</span></p>
    <p v-else-if="paused" class="state">已暂停，不参与定时同步</p>
    <p v-else-if="feed.last_error" class="state err" :title="feed.last_error">
      <CircleAlert aria-hidden="true" />
      <span>{{ feed.last_error }}</span>
    </p>
    <p v-else class="state" :title="formatTime(feed.last_sync)">
      <RefreshCw aria-hidden="true" />
      <span>最近同步 {{ relativeTime(feed.last_sync) }}</span>
    </p>

    <footer class="actions">
      <BaseButton size="sm" :icon="RefreshCw" :loading="syncing" @click="emit('sync')">立即同步</BaseButton>
      <BaseButton size="sm" variant="ghost" :icon="Pencil" @click="emit('edit')">编辑</BaseButton>
      <BaseButton size="sm" variant="ghost" :icon="paused ? Play : Pause" @click="emit('toggle')">{{ paused ? '启用' : '暂停' }}</BaseButton>
      <BaseButton size="sm" variant="danger" :icon="Trash2" @click="emit('remove')">删除</BaseButton>
    </footer>
  </article>
</template>

<style scoped>
.feed-card {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  min-width: 0;
  gap: 10px;
  padding: 16px 18px 14px;
  background: var(--surface);
  border: 1px solid var(--line);
  border-radius: var(--r-lg);
  box-shadow: var(--shadow-sm);
  transition:
    transform var(--t-med) var(--ease-out),
    box-shadow var(--t-med) var(--ease-out),
    border-color var(--t-med) var(--ease-out),
    opacity var(--t-med) var(--ease-out);
}
.feed-card:hover {
  border-color: var(--line-strong);
  box-shadow: var(--shadow-sm);
}
.feed-card.paused {
  opacity: 0.78;
}
.feed-card.paused:hover {
  opacity: 1;
}
.head {
  display: flex;
  align-items: center;
  gap: 10px;
}
.head > .tag {
  flex: none;
}
.name {
  flex: 1;
  min-width: 0;
  font-size: 15px;
  font-weight: 680;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.url {
  color: var(--text-3);
  font-size: 11.5px;
}
.rules {
  display: flex;
  flex-wrap: wrap;
  gap: 5px;
}
.rules > .tag {
  min-width: 0;
  max-width: 100%;
}
.state {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12.5px;
  color: var(--text-2);
  min-width: 0;
}
.state svg {
  width: 13px;
  height: 13px;
  flex: none;
  color: var(--text-3);
}
.state span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.state.err {
  color: var(--red);
}
.state.completed span {
  white-space: normal;
  overflow-wrap: anywhere;
}
.state.err svg {
  color: var(--red);
}
.actions {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 2px;
  padding-top: 12px;
  border-top: 1px solid var(--line);
}
</style>
