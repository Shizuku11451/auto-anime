/* 新版本提醒与一键更新：启动后检查一次，之后定时复查；更新过程中轮询进度。 */
import { computed, ref } from 'vue'
import { useIntervalFn } from '@vueuse/core'
import { http, errorMessage } from '@/api/client'
import type { UpdateInfo } from '@/api/types'
import { isWindowsPlatform } from '@/utils/platform'

const CHECK_INTERVAL_MS = 30 * 60 * 1000
const POLL_INTERVAL_MS = 800
/** 更新重启后允许的等待时间，超时则提示用户手动刷新 */
const RESTART_TIMEOUT_MS = 120 * 1000
/** 检测到新版本后稍等片刻再刷新，确保后端把更新包写入数据目录 */
const RELOAD_DELAY_MS = 2000

const SKIPPED_VERSION_KEY = 'auto-anime:skipped-version'

const info = ref<UpdateInfo | null>(null)
const checking = ref(false)
const checkError = ref('')
const installing = ref(false)
const restarting = ref(false)
const skipped = ref(readSkipped())
const dismissed = ref(false)

function readSkipped(): string {
  try {
    return localStorage.getItem(SKIPPED_VERSION_KEY) || ''
  } catch {
    return ''
  }
}

function writeSkipped(version: string) {
  try {
    if (version) localStorage.setItem(SKIPPED_VERSION_KEY, version)
    else localStorage.removeItem(SKIPPED_VERSION_KEY)
  } catch {
    /* 隐私模式下忽略 */
  }
}

const updateAvailable = computed(() => !!info.value?.update_available && info.value.latest !== skipped.value)
const pending = computed(() => !!info.value?.pending)
/** 客户端应显示「有新版本」提示 */
const notify = computed(() => updateAvailable.value && !dismissed.value)
const status = computed(() => info.value?.status || 'idle')
const progressText = computed(() => info.value?.message || '')
const installHint = computed(() => info.value?.install_hint || '')
const canInstall = computed(() => !!info.value?.can_update && !installing.value)
const platform = computed(() => info.value?.platform || '')
const currentVersion = computed(() => info.value?.version || '')
const currentLabel = computed(() => {
  const v = info.value
  if (!v || !v.version) return ''
  return v.commit ? `${v.version} (${v.commit.slice(0, 7)})` : v.version
})

async function fetchVersion(): Promise<UpdateInfo | null> {
  try {
    const latest = await http.get<UpdateInfo>('/api/update')
    info.value = latest
    return latest
  } catch {
    return null
  }
}

/** 主动检查：把失败原因显示给用户，供设置页按钮使用 */
async function checkNow(): Promise<void> {
  checkError.value = ''
  checking.value = true
  try {
    info.value = await http.get<UpdateInfo>('/api/update')
  } catch (e) {
    checkError.value = errorMessage(e)
  } finally {
    checking.value = false
  }
}

let baseline = ''
let restartDeadline = 0
let poller: { pause: () => void; resume: () => void } | null = null

function ensurePoller() {
  if (poller) return poller
  poller = useIntervalFn(onTick, POLL_INTERVAL_MS, { immediate: false })
  poller.pause()
  return poller
}

function stopPolling() {
  poller?.pause()
  installing.value = false
  restarting.value = false
}

/** 更新过程中的轮询：服务重启时请求会失败，因此失败也要当成进度信号。 */
async function onTick() {
  const latest = await fetchVersion()
  if (!installing.value) {
    poller?.pause()
    return
  }
  if (latest && latest.status === 'failed') {
    restarting.value = false
    stopPolling()
    return
  }
  if (latest && latest.status === 'restarting') restarting.value = true
  if (latest && baseline && latest.version && latest.version !== baseline) {
    // 已经跑在新版本上：等后端落盘后刷新页面
    await new Promise((resolve) => setTimeout(resolve, RELOAD_DELAY_MS))
    stopPolling()
    location.reload()
    return
  }
  // 请求持续失败说明服务正在重启，继续等待
  if (!latest) restarting.value = true
  if (Date.now() > restartDeadline) {
    checkError.value = isWindowsPlatform(info.value?.platform)
      ? '更新超时：请刷新页面确认版本，或查看程序日志'
      : '更新超时：请刷新页面确认版本，或查看容器日志'
    stopPolling()
  }
}

function beginPolling() {
  baseline = info.value?.version || ''
  restartDeadline = Date.now() + RESTART_TIMEOUT_MS
  ensurePoller().resume()
}

/** 一键更新：下载 → 校验替换 → 重启，全程由状态接口反馈 */
async function install(): Promise<void> {
  if (installing.value) return
  installing.value = true
  restarting.value = false
  checkError.value = ''
  baseline = info.value?.version || ''
  try {
    await http.post('/api/update', {})
  } catch (e) {
    checkError.value = errorMessage(e)
    installing.value = false
    await fetchVersion()
    return
  }
  beginPolling()
}

/** 忽略这个版本，直到出现更高的版本 */
function skipVersion() {
  const latest = info.value?.latest
  if (!latest) return
  skipped.value = latest
  writeSkipped(latest)
}

function restoreSkipped() {
  skipped.value = ''
  writeSkipped('')
}

function dismiss() {
  dismissed.value = true
}

let started = false

/** 应用启动后延迟检查一次，之后定时复查 */
function startUpdateWatch() {
  if (started) return
  started = true
  window.setTimeout(() => void fetchVersion(), 4000)
  useIntervalFn(() => void fetchVersion(), CHECK_INTERVAL_MS)
}

export function useUpdateStore() {
  return {
    info,
    checking,
    checkError,
    installing,
    restarting,
    skipped,
    updateAvailable,
    notify,
    pending,
    status,
    progressText,
    canInstall,
    installHint,
    platform,
    currentVersion,
    currentLabel,
    startUpdateWatch,
    checkNow,
    install,
    skipVersion,
    restoreSkipped,
    dismiss,
  }
}
