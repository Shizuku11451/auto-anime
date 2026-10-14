/* 全局数据：订阅、条目、番剧库与运行状态，5 秒轮询，页面不可见时暂停。 */
import { computed, ref, watch } from 'vue'
import { useDocumentVisibility, useIntervalFn } from '@vueuse/core'
import { http, errorMessage } from '@/api/client'
import type { AnimeSummary, Feed, Item, ItemPage, RuntimeStatus } from '@/api/types'

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
/* 运行平台（/api/status）：Windows 桌面版与容器/Linux 的文案与路径示例不同。 */
const platformOS = computed(() => status.value?.os || '')
const totalItems = computed(() => Object.values(counts.value).reduce((a, b) => a + b, 0))

const itemFilter = ref('全部')
const pageBefore = ref(0)
const nextBefore = ref(0)
const pageHistory = ref<number[]>([])
const pageNumber = computed(() => pageHistory.value.length + 1)
let loadVersion = 0
let runtimeVersion = 0
let controller: AbortController | null = null
let lastQuery = ''
let loading = false

async function load(): Promise<void> {
  const version = ++loadVersion
  loading = true
  controller?.abort()
  controller = new AbortController()
  const options = { signal: controller.signal }
  const query = new URLSearchParams({ page: '1', group: itemFilter.value })
  if (search.value) query.set('q', search.value)
  if (pageBefore.value) query.set('before', String(pageBefore.value))
  const key = query.toString()
  if (key !== lastQuery) {
    items.value = []
    loaded.value = false
    nextBefore.value = 0
    lastQuery = key
  }
  try {
    const [f, page, a] = await Promise.all([
      http.get<Feed[]>('/api/feeds', options),
      http.get<ItemPage>('/api/items?' + key, options),
      http.get<AnimeSummary[]>('/api/animes', options),
    ])
    if (version !== loadVersion) return
    feeds.value = f ?? []
    items.value = page.items ?? []
    nextBefore.value = page.next_before
    animes.value = a ?? []
    loadError.value = ''
  } catch (e) {
    if (version === loadVersion) loadError.value = errorMessage(e)
  } finally {
    if (version === loadVersion) { loaded.value = true; loading = false }
  }
}

function resetItemPage() {
  pageBefore.value = 0
  pageHistory.value = []
}
async function nextPage() {
  if (!nextBefore.value) return
  pageHistory.value = [...pageHistory.value, pageBefore.value]
  pageBefore.value = nextBefore.value
  await load()
}
async function previousPage() {
  if (!pageHistory.value.length) return
  pageBefore.value = pageHistory.value.at(-1)!
  pageHistory.value = pageHistory.value.slice(0, -1)
  await load()
}

async function loadRuntime(): Promise<void> {
  const version = ++runtimeVersion
  try {
    const result = await http.get<RuntimeStatus>('/api/status?_=' + Date.now())
    if (version !== runtimeVersion) return
    status.value = result
    runtimeError.value = false
  } catch {
    if (version === runtimeVersion) runtimeError.value = true
  }
}

async function refresh(): Promise<void> {
  await Promise.all([load(), loadRuntime()])
}

let pollingStarted = false

function startPolling() {
  if (pollingStarted) return
  pollingStarted = true
  const visibility = useDocumentVisibility()
  const poll = () => {
    if (!loading) void load()
    void loadRuntime()
  }
  const { pause, resume } = useIntervalFn(poll, 5000, { immediate: true, immediateCallback: true })
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
    itemFilter, pageNumber, nextBefore, nextPage, previousPage, resetItemPage,
    feeds,
    items,
    animes,
    status,
    counts,
    totalItems,
    platformOS,
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
