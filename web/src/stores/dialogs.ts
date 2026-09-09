/* 全局弹窗状态：各视图只负责"打开"，弹窗组件统一挂在 App 根部读取这里的状态。 */
import { reactive } from 'vue'
import { http } from '@/api/client'
import type { AnimeDetail, AnimeSummary, Feed, Item, ResourceItem } from '@/api/types'

export type FeedPrefill = Partial<Feed> & { url?: string; name?: string; from_discover?: boolean }

interface DialogState {
  feed: { open: boolean; feed: FeedPrefill | null }
  storage: { open: boolean; title: string }
  correct: { open: boolean; item: Item | ResourceItem | null }
  animeDetail: { open: boolean; id: number | null; data: AnimeDetail | null; loading: boolean; error: string }
  animeEdit: { open: boolean; anime: AnimeDetail | AnimeSummary | null }
  bind: { open: boolean; anime: AnimeDetail | AnimeSummary | null }
  animeManage: { open: boolean; action: 'delete' | 'merge'; animes: (AnimeDetail | AnimeSummary)[] }
}

const state = reactive<DialogState>({
  feed: { open: false, feed: null },
  storage: { open: false, title: '' },
  correct: { open: false, item: null },
  animeDetail: { open: false, id: null, data: null, loading: false, error: '' },
  animeEdit: { open: false, anime: null },
  bind: { open: false, anime: null },
  animeManage: { open: false, action: 'delete', animes: [] },
})

function openFeed(feed: FeedPrefill | null = null) {
  state.feed.feed = feed
  state.feed.open = true
}

function openStorage(title: string) {
  state.storage.title = title
  state.storage.open = true
}

function openCorrect(item: Item | ResourceItem) {
  state.correct.item = item
  state.correct.open = true
}

async function loadAnimeDetail(id: number) {
  state.animeDetail.loading = true
  state.animeDetail.error = ''
  try {
    const d = await http.get<AnimeDetail>('/api/animes/' + id)
    if (state.animeDetail.id === id) state.animeDetail.data = d
  } catch (e) {
    if (state.animeDetail.id === id) state.animeDetail.error = e instanceof Error ? e.message : String(e)
  } finally {
    if (state.animeDetail.id === id) state.animeDetail.loading = false
  }
}

function openAnimeDetail(id: number) {
  const same = state.animeDetail.id === id
  state.animeDetail.id = id
  if (!same) state.animeDetail.data = null
  state.animeDetail.open = true
  void loadAnimeDetail(id)
}

/** 详情打开时刷新（编辑/绑定/纠错后调用），未打开则忽略 */
async function reloadAnimeDetail() {
  if (state.animeDetail.open && state.animeDetail.id) await loadAnimeDetail(state.animeDetail.id)
}

function openAnimeEdit(anime: AnimeDetail | AnimeSummary | null = null) {
  state.animeEdit.anime = anime
  state.animeEdit.open = true
}

function openAnimeManage(action: 'delete' | 'merge', animes: (AnimeDetail | AnimeSummary)[]) {
  state.animeManage.action = action
  state.animeManage.animes = animes
  state.animeManage.open = true
}

function openBind(anime: AnimeDetail | AnimeSummary) {
  state.bind.anime = anime
  state.bind.open = true
}

export function useDialogs() {
  return {
    state,
    openFeed,
    openStorage,
    openCorrect,
    openAnimeDetail,
    reloadAnimeDetail,
    openAnimeEdit,
    openBind,
    openAnimeManage,
  }
}
