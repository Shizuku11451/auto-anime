/* 全局弹窗状态：各视图只负责"打开"，弹窗组件统一挂在 App 根部读取这里的状态。 */
import { reactive } from 'vue'
import { http } from '@/api/client'
import type { AnimeDetail, AnimeSummary, Feed, Item, ResourceItem } from '@/api/types'

export type FeedPrefill = Partial<Feed> & { url?: string; name?: string; from_discover?: boolean; requires_season_confirmation?: boolean }

/** 播放器的一个条目：只带展示与流地址所需字段 */
export interface PlayerEntry {
  id: number
  /** 系列展示名（番剧译名/识别名） */
  title: string
  season: number
  episode: string
  fansub_group: string
  quality: string
}

/** 传给播放器组件的展示信息 */
export interface PlayerSource {
  src: string
  /** 断点续播的存储键（按条目区分） */
  storageKey: string
  series: string
  label: string
  badge: string
  sub: string
  /** 连播提示里显示的下一集名 */
  nextLabel: string
}

interface DialogState {
  feed: { open: boolean; feed: FeedPrefill | null }
  storage: { open: boolean; title: string }
  correct: { open: boolean; item: Item | ResourceItem | null }
  animeDetail: { open: boolean; id: number | null; data: AnimeDetail | null; loading: boolean; error: string }
  animeEdit: { open: boolean; anime: AnimeDetail | AnimeSummary | null }
  bind: { open: boolean; anime: AnimeDetail | AnimeSummary | null }
  animeManage: { open: boolean; action: 'delete' | 'merge'; animes: (AnimeDetail | AnimeSummary)[] }
  player: { open: boolean; items: PlayerEntry[]; index: number }
  /** 新版本详情与一键更新 */
  update: { open: boolean }
}

const state = reactive<DialogState>({
  feed: { open: false, feed: null },
  storage: { open: false, title: '' },
  correct: { open: false, item: null },
  animeDetail: { open: false, id: null, data: null, loading: false, error: '' },
  animeEdit: { open: false, anime: null },
  bind: { open: false, anime: null },
  animeManage: { open: false, action: 'delete', animes: [] },
  player: { open: false, items: [], index: 0 },
  update: { open: false },
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

let detailRequestVersion = 0

async function loadAnimeDetail(id: number) {
  const version = ++detailRequestVersion
  const current = () => version === detailRequestVersion && state.animeDetail.open && state.animeDetail.id === id
  state.animeDetail.loading = true
  state.animeDetail.error = ''
  try {
    const d = await http.get<AnimeDetail>('/api/animes/' + id)
    if (current()) state.animeDetail.data = d
  } catch (e) {
    if (current()) state.animeDetail.error = e instanceof Error ? e.message : String(e)
  } finally {
    if (current()) state.animeDetail.loading = false
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

/** 打开播放器：items 为播放列表（可只传一个），startId 指定从哪条开始播 */
function openPlayer(items: PlayerEntry[], startId?: number) {
  if (!items.length) return
  const start = startId != null ? items.findIndex((x) => x.id === startId) : 0
  state.player.items = items
  state.player.index = start >= 0 ? start : 0
  state.player.open = true
}

function closePlayer() {
  state.player.open = false
}

/** 打开发布详情与一键更新弹窗 */
function openUpdate() {
  state.update.open = true
}

function closeUpdate() {
  state.update.open = false
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
    openPlayer,
    closePlayer,
    openUpdate,
    closeUpdate,
  }
}
