import { reactive } from 'vue'
import { http, errorMessage } from '@/api/client'
import type { AggregateResult, DiscoverSubjectResult } from '@/api/types'

const state = reactive({
  keyword: '', attempted: '', searching: false, error: '',
  result: null as DiscoverSubjectResult | null,
  selected: 0, loadingResources: false, resourceError: '',
  resources: null as AggregateResult | null,
})
let searchVersion = 0
let resourceVersion = 0
let searchController: AbortController | undefined
let resourceController: AbortController | undefined

function cancelResources() {
  resourceVersion++
  resourceController?.abort()
  resourceController = undefined
  state.loadingResources = false
}

function cancel() {
  searchVersion++
  searchController?.abort()
  searchController = undefined
  state.searching = false
  cancelResources()
  state.selected = 0
  state.resources = null
  state.resourceError = ''
}

async function search() {
  const keyword = state.keyword.trim()
  if (!keyword || (state.searching && state.attempted === keyword)) return
  cancel()
  const version = ++searchVersion
  searchController = new AbortController()
  state.attempted = keyword
  state.searching = true
  state.error = ''
  state.result = null
  state.selected = 0
  state.resources = null
  state.resourceError = ''
  try {
    const result = await http.get<DiscoverSubjectResult>(`/api/discover/subjects?keyword=${encodeURIComponent(keyword)}`, { signal: searchController.signal })
    if (version === searchVersion) state.result = result
  } catch (e) {
    if (version === searchVersion) state.error = errorMessage(e)
  } finally {
    if (version === searchVersion) { state.searching = false; searchController = undefined }
  }
}

async function select(id: number) {
  if (!state.result?.items.some(item => item.id === id)) return
  cancelResources()
  state.selected = id
  state.resources = null
  state.resourceError = ''
  if (!state.result.resources_enabled) return
  const version = ++resourceVersion
  resourceController = new AbortController()
  state.loadingResources = true
  try {
    const result = await http.get<AggregateResult>(`/api/discover/resources?subject_id=${id}`, { signal: resourceController.signal })
    if (version === resourceVersion) state.resources = result
  } catch (e) {
    if (version === resourceVersion) state.resourceError = errorMessage(e)
  } finally {
    if (version === resourceVersion) { state.loadingResources = false; resourceController = undefined }
  }
}

function close() {
  cancelResources()
  state.selected = 0
  state.resources = null
  state.resourceError = ''
}

export function useSubjectSearch() { return { state, search, select, close, cancel } }
