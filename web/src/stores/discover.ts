/* 找番视图的状态放在模块级，切走再切回时保留搜索结果。 */
import { reactive } from 'vue'
import { http, errorMessage } from '@/api/client'
import type { DiscoverResult, DiscoverSource } from '@/api/types'

interface DiscoverState {
  sources: DiscoverSource[]
  sourcesError: string
  source: string
  keyword: string
  searching: boolean
  result: DiscoverResult | null
  error: string
}

const state = reactive<DiscoverState>({
  sources: [],
  sourcesError: '',
  source: '',
  keyword: '',
  searching: false,
  result: null,
  error: '',
})

/** 每次进入找番视图都刷新搜索源：设置页改了自定义模板后立即生效，并尽量保持之前选中的源 */
async function loadSources() {
  try {
    const data = await http.get<{ sources: DiscoverSource[] }>('/api/discover/sources')
    state.sources = data.sources ?? []
    state.sourcesError = ''
  } catch (e) {
    state.sources = []
    state.sourcesError = errorMessage(e)
    return
  }
  if (!state.sources.some((s) => s.id === state.source)) {
    state.source = state.sources[0]?.id ?? ''
  }
}

async function search() {
  const keyword = state.keyword.trim()
  if (!keyword || !state.source) return
  state.searching = true
  state.error = ''
  try {
    state.result = await http.get<DiscoverResult>(
      `/api/discover/search?source=${encodeURIComponent(state.source)}&keyword=${encodeURIComponent(keyword)}`,
    )
  } catch (e) {
    state.error = errorMessage(e)
  } finally {
    state.searching = false
  }
}

export function useDiscover() {
  return { state, loadSources, search }
}
