/* 与 Go 后端 JSON 输出一一对应的类型定义。字段名以服务端 json tag 为准。 */

export interface SetupStatus {
  required: boolean
  download_mode: DownloadMode
  /** 仅未初始化时返回，沿用部署配置中的路径。 */
  library_dir?: string
  download_dir?: string
  poll_minutes?: number
  proxy_enabled?: boolean
  keep_source_files?: boolean
  /** 运行平台（windows/linux/darwin…）：前端按平台切换 Windows 与容器文案。 */
  os?: string
}

export interface SetupResult {
  ok: boolean
  library_warning?: string
}

export interface SetupStorageResult {
  path: string
  mount_status: 'mapped' | 'unmapped' | 'unknown' | 'not_required'
  writable: boolean
  message: string
}

export interface StorageCheckInput {
  path: string
}

export interface Feed {
  completion_note: string
  episode_offset: number | null
  season_filter: number
  allow_unknown_season: boolean
  target_title: string
  /* 订阅保存位置：番剧根目录，归档时在其下按番剧名建立文件夹；空表示默认位置。 */
  storage_dir: string
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
  download_speed: number
  file_path: string
  anime_id: number
  season: number
  // season_auto 是标题分析解析出的季（3期 / S3 / 第三季），用户没在条目上指定季号时用它归档。
  season_auto: number
  manual: boolean
  cover: string
}

export interface AnimeSummary {
	/* Bangumi 明确标注的放送星期，0 表示未获取；手动 update_weekday 优先。 */
	broadcast_weekday?: number
	update_weekday: number
  id: number
  title: string
  search_name: string
  bangumi_id: number
  season: number
  storage_dir: string
  include_rules: string
  exclude_rules: string
  updated_at: string
  /* 仅写入接口返回：目录未映射进容器时的提示。 */
  storage_warning?: string
  cover: string
  meta_name: string
  meta_name_cn: string
  item_count: number
  saved_count: number
  episode_count: number
  rating: number
  summary: string
}

export interface ResourceItem {
  download_speed: number
  progress: number
  id: number
  anime_id: number
  title: string
  status: string
  episode: string
  season: number
  season_auto: number
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
	broadcast_weekday?: number
	update_weekday: number
  id: number
  title: string
  search_name: string
  bangumi_id: number
  season: number
  storage_dir: string
  include_rules: string
  exclude_rules: string
  updated_at: string
  /* 仅写入接口返回：目录未映射进容器时的提示。 */
  storage_warning?: string
  cover: string
  meta_name: string
  meta_name_cn: string
  meta_source: string
  item_count: number
  saved_count: number
  sources: string[]
  rating: number
  summary: string
  voices: VoiceActor[]
  seasons: SeasonGroup[]
}

export interface MetaRematchResult {
  ok: boolean
  queued: boolean
}

export interface VoiceActor { character: string; character_image: string; person: string; person_image: string }

export interface AnimeUpdateInput {
	update_weekday?: number
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
  max_concurrent_downloads: number
  counts: Record<string, number>
  auth_enabled: boolean
  proxy: string
  library_dir: string
  download_dir: string
  /* 运行中的版本号（构建注入，本地构建为 dev）与展示用标签 */
  version: string
  version_label: string
  /* 已有更新包写好、等待下次启动生效 */
  install_ready: boolean
  /* 运行平台（windows/linux），用于按平台切换文案。 */
  os?: string
}

/* 一键更新的实时进度：idle | downloading | installing | restarting | failed */
export type UpdateStatus = 'idle' | 'downloading' | 'installing' | 'restarting' | 'failed'

export interface UpdateInfo {
  /* 版本检查状态：unknown | ok | failed | disabled | error */
  state: string
  error: string
  checked_at: string
  source: string
  current: string
  latest: string
  release_url: string
  notes: string
  published_at: string
  update_available: boolean
  can_install: boolean
  install_hint: string
  pending: boolean
  status: UpdateStatus
  message: string
  /* 当前运行版本与构建信息 */
  version: string
  commit: string
  build_time: string
  platform: string
  enabled: boolean
  can_update: boolean
}

export interface ItemBatchResult {
  ok: boolean
  count: number
  skipped: number
  failed: { id: number; error: string }[]
}

export interface Settings {
  max_concurrent_downloads: number
  auto_cleanup_empty_anime: boolean
  keep_source_files: boolean
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
  /* 运行平台（windows/linux），用于按平台切换文案。 */
  os?: string
}

export interface SettingsInput {
  max_concurrent_downloads?: number
  auto_cleanup_empty_anime?: boolean
  keep_source_files?: boolean
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
  season?: number
  inferred_season?: number
  feed_url?: string
  query?: string
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
  base_keyword?: string
  requested_season?: number
  queries?: string[]
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
  episode_offset: number | null
  total: number
  count: number
  items: { title: string; link: string; published: string; description: string }[]
}

export type AvailabilityVerdict = 'available' | 'no_seeds' | 'unreachable' | 'unknown'

export interface AvailabilityItem {
  url: string
  kind: string
  info_hash?: string
  seeds: number
  leechers: number
  trackers_total: number
  trackers_alive: number
  verdict: AvailabilityVerdict
  error?: string
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

export type DownloadMode = 'native' | 'direct' | 'aria2' | 'qbittorrent' | 'transmission'

export const MODE_LABEL: Record<string, string> = {
  native: '内置 BT 下载器',
  direct: '本地直链',
  aria2: 'Aria2',
  qbittorrent: '外部 qBittorrent',
  transmission: 'Transmission',
}
export interface ItemPage {
  items: Item[]
  next_before: number
}
