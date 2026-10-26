/* 找番视图的状态放在模块级，切走再切回时保留搜索结果。 */
import { reactive } from 'vue'
import { http, errorMessage } from '@/api/client'
import type { DiscoverResult, DiscoverSource } from '@/api/types'

interface DiscoverState {
  sources: DiscoverSource[]
  sourcesLoading: boolean
  sourcesError: string
  source: string
  keyword: string
  literal: boolean
  searching: boolean
  result: DiscoverResult | null
  error: string
  attempt: { source: string; keyword: string; literal: boolean } | null
  cancelled: boolean
  recent: string[]
}

const state = reactive<DiscoverState>({
  sources: [],
  sourcesLoading: true,
  sourcesError: '',
  source: '',
  keyword: '',
  literal: false,
  searching: false,
  result: null,
  error: '',
  attempt: null,
  cancelled: false,
  recent: [],
})

let sourcesRequest = 0
let searchRequest = 0
let controller: AbortController | undefined

/** 每次进入找番视图都刷新搜索源：设置页改了自定义模板后立即生效，并尽量保持之前选中的源 */
async function loadSources() {
  const request = ++sourcesRequest
  state.sourcesLoading = true
  try {
    const data = await http.get<{ sources: DiscoverSource[] }>('/api/discover/sources')
    if (request !== sourcesRequest) return
    state.sources = data.sources ?? []
    state.sourcesError = ''
    if (!state.sources.some((s) => s.id === state.source)) {
      cancelSearch()
      state.result = null
      state.error = ''
      state.attempt = null
      state.cancelled = false
      state.source = state.sources[0]?.id ?? ''
    }
  } catch (e) {
    if (request !== sourcesRequest) return
    state.sourcesError = errorMessage(e)
  } finally {
    if (request === sourcesRequest) state.sourcesLoading = false
  }
}

function cancelSearch() {
  searchRequest++
  controller?.abort()
  controller = undefined
  if (state.searching) state.cancelled = true
  state.searching = false
}

async function search() {
  const keyword = state.keyword.trim()
  if (!keyword || !state.sources.some((s) => s.id === state.source)) return
  const source = state.source
  const literal = state.literal
  if (state.searching && state.attempt?.source === source && state.attempt.keyword === keyword && state.attempt.literal === literal) return
  cancelSearch()
  const request = ++searchRequest
  controller = new AbortController()
  state.keyword = keyword
  state.attempt = { source, keyword, literal }
  state.searching = true
  state.cancelled = false
  state.error = ''
  try {
    const result = await http.get<DiscoverResult>(
      `/api/discover/search?source=${encodeURIComponent(source)}&keyword=${encodeURIComponent(keyword)}${literal ? '&literal=1' : ''}`,
      { signal: controller.signal },
    )
    if (request !== searchRequest) return
    state.result = result
    state.recent = [keyword, ...state.recent.filter((word) => word !== keyword)].slice(0, 6)
  } catch (e) {
    if (request === searchRequest) state.error = errorMessage(e)
  } finally {
    if (request === searchRequest) {
      state.searching = false
      controller = undefined
    }
  }
}

function retry() {
  if (!state.attempt) return
  state.source = state.attempt.source
  state.keyword = state.attempt.keyword
  state.literal = state.attempt.literal
  return search()
}

export function useDiscover() {
  return { state, loadSources, search, cancelSearch, retry }
}
