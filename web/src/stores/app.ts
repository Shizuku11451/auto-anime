/* 全局数据：订阅、条目、番剧库与运行状态，5 秒轮询，页面不可见时暂停。 */
import { computed, ref, watch } from 'vue'
import { useDocumentVisibility, useIntervalFn } from '@vueuse/core'
import { http, errorMessage } from '@/api/client'
import type { AnimeSummary, Feed, Item, RuntimeStatus } from '@/api/types'

const feeds = ref<Feed[]>([])
const items = ref<Item[]>([])
const animes = ref<AnimeSummary[]>([])
const status = ref<RuntimeStatus | null>(null)
const search = ref('')
const loaded = ref(false)
const loadError = ref('')
const runtimeError = ref(false)

const counts = computed<Record<string, number>>(() => status.value?.counts ?? {})
const authEnabled = computed(() => !!status.value?.auth_enabled)
const totalItems = computed(() => Object.values(counts.value).reduce((a, b) => a + b, 0))

let inflight: Promise<void> | null = null

async function load(): Promise<void> {
  if (inflight) return inflight
  inflight = (async () => {
    const query = search.value ? '?q=' + encodeURIComponent(search.value) : ''
    try {
      const [f, i, a] = await Promise.all([
        http.get<Feed[]>('/api/feeds'),
        http.get<Item[]>('/api/items' + query),
        http.get<AnimeSummary[]>('/api/animes'),
      ])
      feeds.value = f ?? []
      items.value = i ?? []
      animes.value = a ?? []
      loadError.value = ''
    } catch (e) {
      loadError.value = errorMessage(e)
    } finally {
      loaded.value = true
      inflight = null
    }
  })()
  return inflight
}

async function loadRuntime(): Promise<void> {
  try {
    status.value = await http.get<RuntimeStatus>('/api/status?_=' + Date.now())
    runtimeError.value = false
  } catch {
    runtimeError.value = true
  }
}

async function refresh(): Promise<void> {
  // An action's refresh must observe writes made after an earlier poll started.
  if (inflight) await inflight
  await Promise.all([load(), loadRuntime()])
}

let pollingStarted = false

function startPolling() {
  if (pollingStarted) return
  pollingStarted = true
  const visibility = useDocumentVisibility()
  const { pause, resume } = useIntervalFn(refresh, 5000, { immediate: true, immediateCallback: true })
  watch(visibility, (v) => {
    if (v === 'visible') {
      refresh()
      resume()
    } else {
      pause()
    }
  })
}

export function useAppStore() {
  return {
    feeds,
    items,
    animes,
    status,
    counts,
    totalItems,
    authEnabled,
    search,
    loaded,
    loadError,
    runtimeError,
    load,
    loadRuntime,
    refresh,
    startPolling,
  }
}
