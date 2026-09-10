<script setup lang="ts">
/* 内置播放器：直接播放 <video> 原生支持的格式（硬件解码），
   后端按 Range 流式供片，拖动进度只拉取需要的字节段。 */
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { onClickOutside } from '@vueuse/core'
import {
  ChevronsLeft,
  ChevronsRight,
  Maximize,
  Minimize,
  Pause,
  PictureInPicture2,
  Play,
  RotateCcw,
  SkipBack,
  SkipForward,
  TriangleAlert,
  Volume2,
  VolumeX,
  X,
} from '@lucide/vue'
import { clamp } from '@/utils/format'
import { isTopDialog, popDialog, pushDialog } from '@/composables/dialogStack'
import type { PlayerSource } from '@/stores/dialogs'

const props = defineProps<{
  source: PlayerSource
  hasNext: boolean
  hasPrev: boolean
}>()

const emit = defineEmits<{ close: []; prev: []; next: [] }>()

const RATES = [0.5, 0.75, 1, 1.25, 1.5, 2, 3]
const PREFS_KEY = 'anime-player:prefs'
const AUTO_HIDE_MS = 2800

const container = ref<HTMLElement | null>(null)
const surfaceEl = ref<HTMLElement | null>(null)
const videoEl = ref<HTMLVideoElement | null>(null)
const rateWrap = ref<HTMLElement | null>(null)

const playing = ref(false)
const busy = ref(false)
const waiting = ref(false)
const ended = ref(false)
const error = ref('')
const duration = ref(0)
const current = ref(0)
const bufEnd = ref(0)
const volume = ref(1)
const muted = ref(false)
const rate = ref(1)
const fs = ref(false)
const controlsVisible = ref(true)
const scrubbing = ref(false)
const rateOpen = ref(false)
const resumeNote = ref('')
const ripple = ref<'l' | 'r' | ''>('')
const hoverPct = ref(-1)
const nextIn = ref(0)
const mkvDismissed = ref(false)

/* 浏览器对 MKV 的支持不一：提前探测，不能播时给出提示。 */
const mkvMaybeUnsupported = ref(false)

const rateLabel = computed(() => (rate.value === 1 ? '倍速' : `${rate.value}x`))
const showBigPlay = computed(() => !playing.value && !ended.value && !error.value && !busy.value)
const hideBars = computed(() => playing.value && !controlsVisible.value && !scrubbing.value && !rateOpen.value)

/* ---------- 弹窗栈：Esc/滚动锁与键盘输入只在最上层生效 ---------- */
const me = Symbol('player')
onMounted(() => {
  pushDialog(me)
  loadPrefs()
  window.addEventListener('keydown', onKey, true)
  document.addEventListener('fullscreenchange', onFsChange)
  window.addEventListener('pagehide', savePosition)
  const v = videoEl.value
  if (v && props.source.src.toLowerCase().includes('.mkv')) {
    mkvMaybeUnsupported.value = v.canPlayType('video/x-matroska; codecs="avc1.640028, mp4a.40.2"') === ''
  }
})
onBeforeUnmount(() => {
  savePosition()
  popDialog(me)
  window.removeEventListener('keydown', onKey, true)
  document.removeEventListener('fullscreenchange', onFsChange)
  window.removeEventListener('pagehide', savePosition)
  stopHideTimer()
  cancelNext()
  if (document.pictureInPictureElement) document.exitPictureInPicture().catch(() => {})
})
onClickOutside(rateWrap, () => (rateOpen.value = false))

/* ---------- 偏好与断点续播 ---------- */
function loadPrefs() {
  try {
    const raw = localStorage.getItem(PREFS_KEY)
    if (!raw) return
    const p = JSON.parse(raw) as { v?: number; m?: boolean; r?: number }
    if (typeof p.v === 'number' && p.v >= 0 && p.v <= 1) volume.value = p.v
    muted.value = !!p.m
    if (p.r && RATES.includes(p.r)) rate.value = p.r
  } catch {
    /* 忽略损坏的本地数据 */
  }
}
function savePrefs() {
  try {
    localStorage.setItem(PREFS_KEY, JSON.stringify({ v: volume.value, m: muted.value, r: rate.value }))
  } catch {
    /* 隐私模式等场景忽略 */
  }
}
function posKey() {
  return `anime-player:pos:${props.source.storageKey}`
}
function savePosition() {
  const d = duration.value
  const t = videoEl.value?.currentTime ?? 0
  try {
    if (!d || t < 10 || t > d - 15) localStorage.removeItem(posKey())
    else localStorage.setItem(posKey(), JSON.stringify({ t, d }))
  } catch {
    /* 忽略 */
  }
}

/* ---------- 媒体事件 ---------- */
function onMeta() {
  const v = videoEl.value
  if (!v) return
  duration.value = v.duration || 0
  v.volume = volume.value
  v.muted = muted.value
  v.playbackRate = rate.value
  try {
    const raw = localStorage.getItem(posKey())
    if (raw) {
      const s = JSON.parse(raw) as { t?: number; d?: number }
      if (typeof s.t === 'number' && s.t > 10 && (!duration.value || s.t < duration.value - 20)) {
        v.currentTime = s.t
        resumeNote.value = `已从 ${fmtClock(s.t)} 继续播放`
        window.setTimeout(() => (resumeNote.value = ''), 2600)
      }
    }
  } catch {
    /* 忽略损坏的本地数据 */
  }
  // 打开播放器即意味着想播放；被浏览器自动播放策略拦截时停在暂停态等用户点击
  busy.value = true
  void v.play().catch(() => (busy.value = false))
}
let lastSave = 0
function onTime() {
  const v = videoEl.value
  if (!v) return
  current.value = v.currentTime
  updateBuffered()
  const now = Date.now()
  if (now - lastSave > 3000) {
    lastSave = now
    if (playing.value) savePosition()
  }
}
function updateBuffered() {
  const v = videoEl.value
  if (!v || !v.buffered.length) return
  for (let i = v.buffered.length - 1; i >= 0; i--) {
    if (v.currentTime >= v.buffered.start(i)) {
      bufEnd.value = v.buffered.end(i)
      return
    }
  }
}
function onPlay() {
  playing.value = true
  busy.value = false
  ended.value = false
  waiting.value = false
  cancelNext()
  showControls()
}
function onPause() {
  playing.value = false
  busy.value = false
  savePosition()
  showControls()
}
function onEnded() {
  playing.value = false
  ended.value = true
  try {
    localStorage.removeItem(posKey())
  } catch {
    /* 忽略 */
  }
  if (props.hasNext) startNextCountdown()
  showControls()
}
function onError() {
  busy.value = false
  playing.value = false
  waiting.value = false
  const code = videoEl.value?.error?.code
  error.value =
    code === 4
      ? '浏览器无法解码该视频格式'
      : code === 3
        ? '视频解码出错，文件可能不完整'
        : code === 2
          ? '网络错误，视频数据加载失败'
          : '视频无法播放'
}
function retry() {
  error.value = ''
  busy.value = true
  videoEl.value?.load()
  void videoEl.value?.play().catch(() => (busy.value = false))
}

/* ---------- 播放控制 ---------- */
function togglePlay() {
  const v = videoEl.value
  if (!v || error.value) return
  if (v.paused) {
    busy.value = true
    void v.play().catch(() => (busy.value = false))
  } else {
    v.pause()
  }
}
function replay() {
  const v = videoEl.value
  if (!v) return
  v.currentTime = 0
  ended.value = false
  busy.value = true
  void v.play().catch(() => (busy.value = false))
}
function skip(delta: number) {
  const v = videoEl.value
  if (!v || !duration.value) return
  v.currentTime = clamp(v.currentTime + delta, 0, duration.value)
  current.value = v.currentTime
  showControls()
}
function setVolume(e: Event) {
  const val = Number((e.target as HTMLInputElement).value)
  volume.value = clamp(val, 0, 1)
  muted.value = volume.value === 0
  applyVolume()
  savePrefs()
  showControls()
}
function changeVolume(delta: number) {
  volume.value = clamp(volume.value + delta, 0, 1)
  muted.value = volume.value === 0
  applyVolume()
  savePrefs()
  showControls()
}
function applyVolume() {
  const v = videoEl.value
  if (!v) return
  v.volume = volume.value
  v.muted = muted.value
}
function toggleMute() {
  muted.value = !muted.value
  if (!muted.value && volume.value === 0) volume.value = 0.5
  applyVolume()
  savePrefs()
  showControls()
}
function setRate(r: number) {
  rate.value = r
  if (videoEl.value) videoEl.value.playbackRate = r
  savePrefs()
  rateOpen.value = false
  showControls()
}
const pipSupported = typeof document !== 'undefined' && 'pictureInPictureEnabled' in document && document.pictureInPictureEnabled
async function togglePiP() {
  try {
    if (document.pictureInPictureElement) await document.exitPictureInPicture()
    else await videoEl.value?.requestPictureInPicture()
  } catch {
    /* 用户代理拒绝时忽略 */
  }
}
async function toggleFullscreen() {
  const el = container.value
  if (!el) return
  try {
    if (document.fullscreenElement) await document.exitFullscreen()
    else if (el.requestFullscreen) await el.requestFullscreen()
    else {
      const v = videoEl.value as (HTMLVideoElement & { webkitEnterFullscreen?: () => void }) | null
      v?.webkitEnterFullscreen?.()
    }
  } catch {
    /* 忽略不支持全屏的场景 */
  }
}
function onFsChange() {
  fs.value = document.fullscreenElement === container.value
  showControls()
}

/* ---------- 下一集连播 ---------- */
let nextTimer: number | null = null
function startNextCountdown() {
  cancelNext()
  nextIn.value = 5
  nextTimer = window.setInterval(() => {
    nextIn.value -= 1
    if (nextIn.value <= 0) {
      cancelNext()
      emit('next')
    }
  }, 1000)
}
function cancelNext() {
  if (nextTimer != null) {
    window.clearInterval(nextTimer)
    nextTimer = null
  }
  nextIn.value = 0
}

/* ---------- 控制栏显隐 ---------- */
let hideTimer: number | null = null
function stopHideTimer() {
  if (hideTimer != null) {
    window.clearTimeout(hideTimer)
    hideTimer = null
  }
}
function showControls() {
  controlsVisible.value = true
  stopHideTimer()
  if (playing.value && !scrubbing.value && !rateOpen.value) {
    hideTimer = window.setTimeout(() => (controlsVisible.value = false), AUTO_HIDE_MS)
  }
}
function scheduleHideSoon() {
  stopHideTimer()
  if (playing.value) hideTimer = window.setTimeout(() => (controlsVisible.value = false), 500)
}

/* ---------- 进度条拖动与悬停预览 ---------- */
const seekEl = ref<HTMLElement | null>(null)
function pctOf(t: number): string {
  if (!duration.value) return '0%'
  return `${clamp((t / duration.value) * 100, 0, 100)}%`
}
function eventTime(e: PointerEvent): number {
  const rect = seekEl.value?.getBoundingClientRect()
  if (!rect || !duration.value) return 0
  return clamp((e.clientX - rect.left) / rect.width, 0, 1) * duration.value
}
function onSeekDown(e: PointerEvent) {
  if (!duration.value) return
  scrubbing.value = true
  seekEl.value?.setPointerCapture(e.pointerId)
  seekTo(eventTime(e))
  showControls()
}
function onSeekMove(e: PointerEvent) {
  const rect = seekEl.value?.getBoundingClientRect()
  if (!rect) return
  hoverPct.value = clamp((e.clientX - rect.left) / rect.width, 0, 1)
  if (scrubbing.value) seekTo(eventTime(e))
}
function onSeekUp() {
  if (!scrubbing.value) return
  scrubbing.value = false
  savePosition()
  showControls()
}
function seekTo(t: number) {
  const v = videoEl.value
  if (!v || !duration.value) return
  v.currentTime = clamp(t, 0, duration.value)
  current.value = v.currentTime
}

/* ---------- 画面区域点击/双击/双击快进 ---------- */
let tapTimer: number | null = null
let lastTap = { at: 0, zone: '' }
function onSurfaceTap(e: PointerEvent) {
  showControls()
  const rect = surfaceEl.value?.getBoundingClientRect()
  const x = rect ? (e.clientX - rect.left) / rect.width : 0.5
  const zone = e.pointerType === 'touch' ? (x < 0.35 ? 'l' : x > 0.65 ? 'r' : 'c') : 'c'
  const now = Date.now()
  if (tapTimer != null) {
    window.clearTimeout(tapTimer)
    tapTimer = null
  }
  const double = now - lastTap.at < 320 && lastTap.zone === zone
  lastTap = { at: now, zone }
  if (double && zone !== 'c') {
    ripple.value = zone === 'l' ? 'l' : 'r'
    skip(zone === 'l' ? -10 : 10)
    window.setTimeout(() => (ripple.value = ''), 450)
    return
  }
  if (double && zone === 'c' && e.pointerType !== 'touch') {
    void toggleFullscreen()
    return
  }
  tapTimer = window.setTimeout(() => {
    tapTimer = null
    if (e.pointerType === 'touch') {
      controlsVisible.value = !controlsVisible.value
      if (controlsVisible.value) showControls()
    } else {
      togglePlay()
    }
  }, 280)
}

/* ---------- 键盘快捷键 ---------- */
function onKey(e: KeyboardEvent) {
  if (!isTopDialog(me)) return
  const t = e.target as HTMLElement | null
  if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.tagName === 'SELECT' || t.isContentEditable)) return
  if (t && t.closest?.('button') && (e.key === ' ' || e.key === 'Enter')) return
  switch (e.key) {
    case ' ':
    case 'k':
    case 'K':
      e.preventDefault()
      togglePlay()
      break
    case 'ArrowLeft':
      e.preventDefault()
      skip(-5)
      break
    case 'ArrowRight':
      e.preventDefault()
      skip(5)
      break
    case 'j':
    case 'J':
      skip(-10)
      break
    case 'l':
    case 'L':
      skip(10)
      break
    case 'ArrowUp':
      e.preventDefault()
      changeVolume(0.1)
      break
    case 'ArrowDown':
      e.preventDefault()
      changeVolume(-0.1)
      break
    case 'm':
    case 'M':
      toggleMute()
      break
    case 'f':
    case 'F':
      void toggleFullscreen()
      break
    case 'p':
    case 'P':
      void togglePiP()
      break
    case 'Escape':
      // 全屏时浏览器自己退出全屏，不要顺带关闭播放器
      if (!document.fullscreenElement) emit('close')
      break
    default: {
      if (/^[0-9]$/.test(e.key) && duration.value > 0) seekTo((Number(e.key) / 10) * duration.value)
    }
  }
}

function fmtClock(s: number): string {
  if (!Number.isFinite(s) || s < 0) s = 0
  const t = Math.floor(s)
  const h = Math.floor(t / 3600)
  const m = Math.floor((t % 3600) / 60)
  const ss = String(t % 60).padStart(2, '0')
  return h > 0 ? `${h}:${String(m).padStart(2, '0')}:${ss}` : `${m}:${ss}`
}
</script>

<template>
  <div
    ref="container"
    class="vplayer"
    :class="{ 'bars-hidden': hideBars, fullscreen: fs }"
    tabindex="-1"
    @pointermove="showControls"
    @pointerdown="showControls"
    @pointerleave="scheduleHideSoon"
  >
    <div ref="surfaceEl" class="surface" @pointerup="onSurfaceTap">
      <video
        ref="videoEl"
        class="video"
        :src="source.src"
        preload="metadata"
        playsinline
        webkit-playsinline
        :aria-label="source.series"
        @loadedmetadata="onMeta"
        @timeupdate="onTime"
        @progress="updateBuffered"
        @play="onPlay"
        @pause="onPause"
        @ended="onEnded"
        @waiting="waiting = true"
        @playing="waiting = false"
        @canplay="busy = false; waiting = false"
        @error="onError"
      />
    </div>

    <!-- 中央状态 -->
    <Transition name="pf">
      <div v-if="(busy || waiting) && !error && !ended" class="center no-click">
        <span class="spinner" role="status" aria-label="加载中" />
      </div>
    </Transition>
    <button v-if="showBigPlay" class="center big-play" aria-label="播放" @pointerup.stop @click.stop="togglePlay">
      <Play :size="34" :stroke-width="2.2" />
    </button>
    <button
      v-if="ended && !hasNext && !error"
      class="center big-play"
      aria-label="重新播放"
      @pointerup.stop
      @click.stop="replay"
    >
      <RotateCcw :size="30" :stroke-width="2.2" />
    </button>

    <!-- 错误面板 -->
    <div v-if="error" class="center error-panel" role="alert">
      <TriangleAlert :size="34" />
      <p class="error-title">{{ error }}</p>
      <p class="error-hint">MP4（H.264）与 WebM 兼容性最好；MKV 建议使用 Edge/Chrome。若登录已过期，请刷新页面重新登录。</p>
      <div class="error-actions">
        <button class="pbtn solid" @click="retry">重试</button>
        <button class="pbtn solid" @click="emit('close')">关闭</button>
      </div>
    </div>

    <!-- 连播下一集 -->
    <Transition name="pf">
      <div v-if="ended && hasNext && !error" class="next-card">
        <p class="next-title">即将播放{{ source.nextLabel ? `：${source.nextLabel}` : '下一集' }}</p>
        <p class="next-count">{{ nextIn }} 秒</p>
        <div class="next-actions">
          <button class="pbtn solid accent" @click="emit('next')">立即播放</button>
          <button class="pbtn solid" @click="cancelNext">取消</button>
        </div>
      </div>
    </Transition>

    <!-- 顶部信息栏 -->
    <header class="bar top">
      <div class="titles">
        <h2 class="series">{{ source.series }}</h2>
        <p class="ep-line">
          <span v-if="source.label">{{ source.label }}</span>
          <span v-if="source.badge" class="badge">{{ source.badge }}</span>
          <span v-if="source.sub" class="sub">{{ source.sub }}</span>
        </p>
      </div>
      <button class="pbtn" aria-label="关闭播放器" @click="emit('close')"><X :size="22" /></button>
    </header>

    <!-- MKV 兼容提示 -->
    <div v-if="mkvMaybeUnsupported && !mkvDismissed" class="mkv-hint" role="status">
      <TriangleAlert :size="15" />
      <span>当前浏览器可能不支持 MKV，建议使用 Edge/Chrome 播放</span>
      <button class="hint-close" aria-label="关闭提示" @click="mkvDismissed = true"><X :size="14" /></button>
    </div>

    <!-- 恢复播放提示与快进涟漪 -->
    <Transition name="pf">
      <div v-if="resumeNote" class="note">{{ resumeNote }}</div>
    </Transition>
    <Transition name="pf">
      <div v-if="ripple" class="ripple" :class="ripple">
        <ChevronsLeft v-if="ripple === 'l'" :size="30" />
        <ChevronsRight v-else :size="30" />
        <span>{{ ripple === 'l' ? '快退 10 秒' : '快进 10 秒' }}</span>
      </div>
    </Transition>

    <!-- 底部控制栏 -->
    <footer class="bar bottom">
      <div class="seek-row">
        <span class="time now">{{ fmtClock(current) }}</span>
        <div
          ref="seekEl"
          class="seek"
          :class="{ scrubbing }"
          @pointerdown="onSeekDown"
          @pointermove="onSeekMove"
          @pointerup="onSeekUp"
          @pointercancel="onSeekUp"
          @pointerleave="hoverPct = -1"
        >
          <div class="seek-track">
            <div class="seek-buf" :style="{ width: pctOf(bufEnd) }" />
            <div class="seek-fill" :style="{ width: pctOf(current) }" />
          </div>
          <div class="seek-thumb" :style="{ left: pctOf(current) }" />
          <div v-if="hoverPct >= 0 && duration" class="seek-tip" :style="{ left: `${hoverPct * 100}%` }">
            {{ fmtClock(hoverPct * duration) }}
          </div>
        </div>
        <span class="time dur">{{ fmtClock(duration) }}</span>
      </div>
      <div class="ctrl-row">
        <button class="pbtn" :aria-label="playing ? '暂停' : '播放'" @click="togglePlay">
          <Pause v-if="playing" :size="22" />
          <Play v-else :size="22" />
        </button>
        <button class="pbtn" :disabled="!hasPrev" aria-label="上一集" @click="emit('prev')"><SkipBack :size="19" /></button>
        <button class="pbtn" :disabled="!hasNext" aria-label="下一集" @click="emit('next')"><SkipForward :size="19" /></button>
        <div class="vol">
          <button class="pbtn" :aria-label="muted ? '取消静音' : '静音'" @click="toggleMute">
            <VolumeX v-if="muted || volume === 0" :size="20" />
            <Volume2 v-else :size="20" />
          </button>
          <input
            class="vol-range"
            type="range"
            min="0"
            max="1"
            step="0.05"
            :value="muted ? 0 : volume"
            aria-label="音量"
            @input="setVolume"
          />
        </div>
        <span class="flex-spacer" />
        <div ref="rateWrap" class="rate-wrap">
          <button class="pbtn rate-btn" :aria-expanded="rateOpen" aria-label="播放速度" @click="rateOpen = !rateOpen">
            {{ rateLabel }}
          </button>
          <div v-if="rateOpen" class="rate-menu" role="menu">
            <button
              v-for="r in RATES"
              :key="r"
              role="menuitem"
              :class="{ on: rate === r }"
              @click="setRate(r)"
            >
              {{ r === 1 ? '正常速度' : `${r}x` }}
            </button>
          </div>
        </div>
        <button v-if="pipSupported" class="pbtn" aria-label="画中画" @click="togglePiP"><PictureInPicture2 :size="20" /></button>
        <button class="pbtn" :aria-label="fs ? '退出全屏' : '全屏'" @click="toggleFullscreen">
          <Minimize v-if="fs" :size="20" />
          <Maximize v-else :size="20" />
        </button>
      </div>
    </footer>
  </div>
</template>

<style scoped>
.vplayer {
  position: relative;
  width: 100%;
  height: 100%;
  overflow: hidden;
  background: #000;
  color: #fff;
  outline: none;
  user-select: none;
}
.vplayer.fullscreen {
  border-radius: 0;
}
.vplayer.bars-hidden {
  cursor: none;
}
.surface {
  position: absolute;
  inset: 0;
}
.video {
  width: 100%;
  height: 100%;
  object-fit: contain;
  background: #000;
}

.center {
  position: absolute;
  inset: 0;
  display: grid;
  place-items: center;
}
.center.no-click {
  pointer-events: none;
}
.spinner {
  width: 46px;
  height: 46px;
  border-radius: 50%;
  border: 3px solid rgba(255, 255, 255, 0.22);
  border-top-color: #fff;
  animation: vspin 0.8s linear infinite;
}
@keyframes vspin {
  to {
    transform: rotate(360deg);
  }
}
.big-play {
  width: 92px;
  height: 92px;
  place-self: center;
  display: grid;
  place-items: center;
  border: none;
  border-radius: 50%;
  background: rgba(20, 20, 28, 0.62);
  color: #fff;
  backdrop-filter: blur(6px);
  cursor: pointer;
  transition:
    transform var(--t-fast) var(--ease-out),
    background var(--t-fast) var(--ease-out);
}
.big-play:hover {
  transform: scale(1.06);
  background: rgba(20, 20, 28, 0.8);
}
.big-play:active {
  transform: scale(0.96);
}

.error-panel {
  background: rgba(0, 0, 0, 0.72);
  backdrop-filter: blur(8px);
  place-items: center;
  align-content: center;
  gap: 10px;
  text-align: center;
  padding: 24px;
  color: #ffb4b4;
}
.error-title {
  font-size: 17px;
  font-weight: 700;
  color: #fff;
}
.error-hint {
  max-width: 460px;
  font-size: 13px;
  line-height: 1.7;
  color: rgba(255, 255, 255, 0.75);
}
.error-actions {
  display: flex;
  gap: 10px;
  margin-top: 6px;
}

.next-card {
  position: absolute;
  right: 22px;
  bottom: 108px;
  z-index: 6;
  display: grid;
  gap: 4px;
  justify-items: start;
  padding: 14px 16px;
  border-radius: 14px;
  background: rgba(18, 18, 26, 0.88);
  border: 1px solid rgba(255, 255, 255, 0.14);
  backdrop-filter: blur(10px);
  box-shadow: 0 12px 32px rgba(0, 0, 0, 0.5);
}
.next-title {
  font-size: 13.5px;
  font-weight: 650;
  max-width: 320px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.next-count {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.65);
}
.next-actions {
  display: flex;
  gap: 8px;
  margin-top: 8px;
}

.bar {
  position: absolute;
  left: 0;
  right: 0;
  z-index: 5;
  display: flex;
  padding: 14px 18px;
  transition:
    opacity 240ms var(--ease-out),
    transform 240ms var(--ease-out);
}
.bar.top {
  top: 0;
  align-items: flex-start;
  justify-content: space-between;
  gap: 14px;
  background: linear-gradient(to bottom, rgba(0, 0, 0, 0.78), rgba(0, 0, 0, 0.35) 60%, transparent);
}
.bar.bottom {
  bottom: 0;
  flex-direction: column;
  gap: 4px;
  padding-bottom: calc(12px + env(safe-area-inset-bottom));
  background: linear-gradient(to top, rgba(0, 0, 0, 0.82), rgba(0, 0, 0, 0.4) 55%, transparent);
}
.bars-hidden .bar {
  opacity: 0;
  pointer-events: none;
}
.bars-hidden .bar.top {
  transform: translateY(-8px);
}
.bars-hidden .bar.bottom {
  transform: translateY(8px);
}

.titles {
  min-width: 0;
  display: grid;
  gap: 3px;
}
.series {
  font-size: 15.5px;
  font-weight: 700;
  line-height: 1.35;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 70vw;
}
.ep-line {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px 8px;
  font-size: 12.5px;
  color: rgba(255, 255, 255, 0.78);
}
.badge {
  padding: 1px 8px;
  border-radius: 999px;
  background: var(--accent);
  color: var(--accent-contrast, #fff);
  font-size: 11px;
  font-weight: 700;
}
.sub {
  color: rgba(255, 255, 255, 0.6);
}

.mkv-hint {
  position: absolute;
  top: 74px;
  right: 18px;
  z-index: 5;
  display: flex;
  align-items: center;
  gap: 8px;
  max-width: min(420px, calc(100vw - 36px));
  padding: 8px 12px;
  border-radius: 10px;
  background: rgba(18, 18, 26, 0.9);
  border: 1px solid rgba(255, 190, 90, 0.35);
  color: #ffd9a0;
  font-size: 12.5px;
}
.hint-close {
  display: grid;
  place-items: center;
  width: 22px;
  height: 22px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: inherit;
  cursor: pointer;
}
.hint-close:hover {
  background: rgba(255, 255, 255, 0.12);
}

.note {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  z-index: 5;
  padding: 8px 16px;
  border-radius: 999px;
  background: rgba(0, 0, 0, 0.66);
  font-size: 13px;
  font-weight: 600;
  pointer-events: none;
}
.ripple {
  position: absolute;
  top: 50%;
  transform: translateY(-50%);
  z-index: 5;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 10px 14px;
  border-radius: 999px;
  background: rgba(0, 0, 0, 0.6);
  font-size: 13px;
  font-weight: 650;
  pointer-events: none;
}
.ripple.l {
  left: 8%;
}
.ripple.r {
  right: 8%;
}

.seek-row {
  display: flex;
  align-items: center;
  gap: 12px;
}
.time {
  font-size: 12.5px;
  font-variant-numeric: tabular-nums;
  color: rgba(255, 255, 255, 0.92);
  min-width: 44px;
}
.time.dur {
  color: rgba(255, 255, 255, 0.6);
  text-align: right;
}
.seek {
  position: relative;
  flex: 1;
  height: 22px;
  display: flex;
  align-items: center;
  cursor: pointer;
  touch-action: none;
}
.seek-track {
  position: relative;
  width: 100%;
  height: 4px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.22);
  overflow: hidden;
  transition: height 140ms var(--ease-out);
}
.seek:hover .seek-track,
.seek.scrubbing .seek-track {
  height: 6px;
}
.seek-buf {
  position: absolute;
  inset: 0 auto 0 0;
  background: rgba(255, 255, 255, 0.34);
}
.seek-fill {
  position: absolute;
  inset: 0 auto 0 0;
  background: var(--accent, #8b7cf8);
}
.seek-thumb {
  position: absolute;
  top: 50%;
  width: 13px;
  height: 13px;
  border-radius: 50%;
  background: #fff;
  box-shadow: 0 1px 6px rgba(0, 0, 0, 0.55);
  transform: translate(-50%, -50%) scale(0);
  transition: transform 140ms var(--ease-out);
  pointer-events: none;
}
.seek:hover .seek-thumb,
.seek.scrubbing .seek-thumb {
  transform: translate(-50%, -50%) scale(1);
}
.seek-tip {
  position: absolute;
  bottom: 24px;
  transform: translateX(-50%);
  padding: 3px 8px;
  border-radius: 7px;
  background: rgba(12, 12, 18, 0.92);
  font-size: 11.5px;
  font-variant-numeric: tabular-nums;
  pointer-events: none;
  white-space: nowrap;
}

.ctrl-row {
  display: flex;
  align-items: center;
  gap: 4px;
  min-height: 44px;
}
.pbtn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  min-width: 40px;
  height: 40px;
  padding: 0 9px;
  border: none;
  border-radius: 10px;
  background: transparent;
  color: rgba(255, 255, 255, 0.9);
  font-size: 13px;
  font-weight: 650;
  cursor: pointer;
  transition:
    background var(--t-fast) var(--ease-out),
    color var(--t-fast) var(--ease-out),
    transform var(--t-fast) var(--ease-out);
}
.pbtn:hover {
  background: rgba(255, 255, 255, 0.14);
  color: #fff;
}
.pbtn:active {
  transform: scale(0.93);
}
.pbtn:disabled {
  opacity: 0.35;
  cursor: default;
}
.pbtn:disabled:hover {
  background: transparent;
}
.pbtn.solid {
  background: rgba(255, 255, 255, 0.14);
  border: 1px solid rgba(255, 255, 255, 0.2);
}
.pbtn.solid:hover {
  background: rgba(255, 255, 255, 0.24);
}
.pbtn.solid.accent {
  background: var(--accent, #8b7cf8);
  border-color: transparent;
  color: var(--accent-contrast, #fff);
}
.pbtn.solid.accent:hover {
  filter: brightness(1.1);
}
.pbtn:focus-visible {
  outline: 2px solid rgba(255, 255, 255, 0.75);
  outline-offset: 2px;
}

.vol {
  display: flex;
  align-items: center;
}
.vol-range {
  width: 74px;
  accent-color: var(--accent, #8b7cf8);
  cursor: pointer;
}
.flex-spacer {
  flex: 1;
}

.rate-wrap {
  position: relative;
}
.rate-menu {
  position: absolute;
  right: 0;
  bottom: calc(100% + 10px);
  display: grid;
  padding: 6px;
  border-radius: 12px;
  background: rgba(18, 18, 26, 0.94);
  border: 1px solid rgba(255, 255, 255, 0.14);
  box-shadow: 0 12px 32px rgba(0, 0, 0, 0.5);
  backdrop-filter: blur(10px);
}
.rate-menu button {
  display: block;
  width: 108px;
  padding: 8px 12px;
  border: none;
  border-radius: 8px;
  background: transparent;
  color: rgba(255, 255, 255, 0.85);
  font-size: 13px;
  text-align: left;
  cursor: pointer;
}
.rate-menu button:hover {
  background: rgba(255, 255, 255, 0.12);
  color: #fff;
}
.rate-menu button.on {
  color: var(--accent, #8b7cf8);
  font-weight: 700;
}

.pf-enter-active,
.pf-leave-active {
  transition: opacity 200ms var(--ease-out);
}
.pf-enter-from,
.pf-leave-to {
  opacity: 0;
}

@media (max-width: 640px) {
  .bar {
    padding-left: 10px;
    padding-right: 10px;
  }
  .vol-range {
    display: none;
  }
  .next-card {
    right: 12px;
    left: 12px;
    bottom: 118px;
    justify-items: stretch;
  }
  .next-title {
    max-width: none;
    white-space: normal;
  }
  .series {
    max-width: calc(100vw - 120px);
  }
  .pbtn {
    min-width: 42px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .bar,
  .big-play,
  .seek-track,
  .seek-thumb,
  .pbtn {
    transition: none;
  }
  .spinner {
    animation-duration: 1.6s;
  }
}
</style>
