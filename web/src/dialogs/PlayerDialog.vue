<script setup lang="ts">
/* 播放器宿主：全屏黑底覆盖层，负责播放列表切换；
   播放控制与键盘交互都在 VideoPlayer 内部。 */
import { computed } from 'vue'
import VideoPlayer from '@/components/player/VideoPlayer.vue'
import { useDialogs } from '@/stores/dialogs'
import type { PlayerEntry, PlayerSource } from '@/stores/dialogs'

const dialogs = useDialogs()
const p = dialogs.state.player

const entry = computed<PlayerEntry | null>(() => p.items[p.index] ?? null)
const nextEntry = computed<PlayerEntry | null>(() => p.items[p.index + 1] ?? null)

const source = computed<PlayerSource | null>(() => {
  const e = entry.value
  if (!e) return null
  const epNum = String(e.episode).padStart(2, '0')
  const season = Math.max(1, e.season || 1)
  const label = e.episode ? `第 ${e.episode} 集` : ''
  const badge = e.episode ? `S${String(season).padStart(2, '0')}E${epNum}` : ''
  const sub = [e.fansub_group, e.quality].filter(Boolean).join(' · ')
  return {
    src: `/api/items/${e.id}/stream`,
    storageKey: String(e.id),
    series: e.title,
    label,
    badge,
    sub,
    nextLabel: nextEntry.value?.title === e.title ? epTitle(nextEntry.value) : nextEntry.value?.title || '',
  }
})

function epTitle(e: PlayerEntry): string {
  return e.episode ? `${e.title} 第 ${e.episode} 集` : e.title
}

function close() {
  dialogs.closePlayer()
}
function prev() {
  if (p.index > 0) p.index -= 1
}
function next() {
  if (p.index < p.items.length - 1) p.index += 1
}
</script>

<template>
  <Teleport to="body">
    <!-- 显式 duration：leave 用定时器收尾，页面失焦导致 transitionend 丢失时也能卸载 -->
    <Transition name="player-fade" :duration="240">
      <div v-if="p.open && source" class="player-overlay" role="dialog" aria-modal="true" aria-label="视频播放器">
        <VideoPlayer
          :key="entry!.id"
          :source="source"
          :has-prev="p.index > 0"
          :has-next="p.index < p.items.length - 1"
          @close="close"
          @prev="prev"
          @next="next"
        />
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.player-overlay {
  position: fixed;
  inset: 0;
  z-index: 140;
  background: #000;
}

.player-fade-enter-active,
.player-fade-leave-active {
  transition: opacity 220ms var(--ease-out);
}
.player-fade-enter-from,
.player-fade-leave-to {
  opacity: 0;
}

@media (prefers-reduced-motion: reduce) {
  .player-fade-enter-active,
  .player-fade-leave-active {
    transition: none;
  }
}
</style>
