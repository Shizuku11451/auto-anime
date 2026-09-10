/* 与 Go 后端 JSON 输出一一对应的类型定义。字段名以服务端 json tag 为准。 */

export interface Feed {
  target_title: string
  id: number
  name: string
  url: string
  include_words: string
  exclude_words: string
  auto_download: boolean
  enabled: boolean
  last_sync: string
  last_error: string
}

export interface FeedInput {
  target_title?: string
  name: string
  url: string
  include_words: string
  exclude_words: string
  auto_download: boolean
  enabled?: boolean
}

export interface Item {
  id: number
  title: string
  link: string
  published: string
  status: string
  anime_title: string
  episode: string
  quality: string
  fansub_group: string
  feed_name: string
  downloader: string
  task_error: string
  progress: number
  file_path: string
  anime_id: number
  season: number
  manual: boolean
  cover: string
}

export interface AnimeSummary {
  id: number
  title: string
  search_name: string
  bangumi_id: number
  season: number
  storage_dir: string
  include_rules: string
  exclude_rules: string
  updated_at: string
  cover: string
  meta_name: string
  meta_name_cn: string
  item_count: number
  saved_count: number
  episode_count: number
}

export interface ResourceItem {
  id: number
  anime_id: number
  title: string
  status: string
  episode: string
  season: number
  quality: string
  fansub_group: string
  feed_name: string
  file_path: string
  cover: string
  link: string
  manual: boolean
  episode_no: number
}

export interface EpisodeGroup {
  episode: string
  items: ResourceItem[]
}

export interface SeasonGroup {
  season: number
  episodes: EpisodeGroup[]
  loose: ResourceItem[]
}

export interface AnimeDetail {
  id: number
  title: string
  search_name: string
  bangumi_id: number
  season: number
  storage_dir: string
  include_rules: string
  exclude_rules: string
  updated_at: string
  cover: string
  meta_name: string
  meta_name_cn: string
  item_count: number
  saved_count: number
  sources: string[]
  seasons: SeasonGroup[]
}

export interface AnimeUpdateInput {
  title: string
  search_name: string
  season: number
  storage_dir: string
  include_rules: string
  exclude_rules: string
}

export interface StorageInfo {
  anime_title: string
  storage_dir: string
  season: number
  default_path: string
  effective_path: string
}

export interface RuntimeStatus {
  poll_minutes: number
  download_mode: string
  counts: Record<string, number>
  auth_enabled: boolean
  proxy: string
  library_dir: string
  download_dir: string
}

export interface Settings {
  auto_cleanup_empty_anime: boolean
  web_user: string
  web_password_set: boolean
  download_mode: string
  aria2_url: string
  aria2_token_set: boolean
  aria2_dir: string
  aria2_local_dir: string
  qbit_url: string
  qbit_user: string
  qbit_password_set: boolean
  qbit_save_path: string
  qbit_local_dir: string
  transmission_url: string
  transmission_user: string
  transmission_password_set: boolean
  transmission_download_dir: string
  transmission_local_dir: string
  proxy_url: string
  proxy_credentials_set: boolean
  poll_minutes: number
  discover_search_template: string
  library_dir: string
  download_dir: string
  setup_done: boolean
}

export interface SettingsInput {
  auto_cleanup_empty_anime?: boolean
  web_user?: string
  web_password?: string
  download_mode?: string
  aria2_url?: string
  aria2_token?: string
  aria2_dir?: string
  aria2_local_dir?: string
  qbit_url?: string
  qbit_user?: string
  qbit_password?: string
  qbit_save_path?: string
  qbit_local_dir?: string
  transmission_url?: string
  transmission_user?: string
  transmission_password?: string
  transmission_download_dir?: string
  transmission_local_dir?: string
  proxy_url?: string
  proxy_clear_credentials?: boolean
  poll_minutes?: number
  discover_search_template?: string
  library_dir?: string
}

export interface DiscoverSource {
  id: string
  name: string
  hint: string
  custom: boolean
}

export interface DiscoverItem {
  title: string
  anime_title: string
  fansub_group: string
  episode: string
  quality: string
  link: string
  torrent: string
  published: string
}

export interface FeedReadInfo {
  pages: number
  history: boolean
  limited: boolean
  warning: string
}

export interface DiscoverResult extends FeedReadInfo {
  source: string
  source_name: string
  keyword: string
  feed_url: string
  count: number
  items: DiscoverItem[]
}

export interface MetaCandidate {
  id: number
  name: string
  name_cn: string
}

export interface PreviewResult extends FeedReadInfo {
  total: number
  count: number
  items: { title: string; link: string; published: string; description: string }[]
}

export interface CorrectInput {
  anime_id?: number
  anime_title?: string
  season: number
  episode: string
  rearchive: boolean
}

export interface CorrectResult {
  ok: boolean
  moved: boolean
  cleanup_warning: string
}

export type DownloadMode = 'direct' | 'aria2' | 'qbittorrent' | 'transmission'

export const MODE_LABEL: Record<string, string> = {
  direct: '本地直链',
  aria2: 'Aria2',
  qbittorrent: 'qBittorrent',
  transmission: 'Transmission',
}
