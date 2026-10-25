import type { DiscoverItem } from '../api/types'

// 仅清理发布标签和斜线分隔的别名，不按封面或相似字符串猜测番剧身份。
export function subscriptionTitle(item: DiscoverItem): string {
  return item.anime_title.replace(/\[([^\]]*)\]|【([^】]*)】/g, (tag, a: string, b: string) =>
    /^(?:S\d{1,3}|第?[\d一二三四五六七八九十]+[季期])$/i.test((a || b).trim()) ? tag : '')
    .split(/\s+[/／]\s+/)[0]!
    .replace(/\s+/g, ' ').trim() || item.anime_title || item.title
}

export function releaseLanguage(item: DiscoverItem): string {
  return item.title.match(/简繁|簡繁|简日|簡日|繁日|简中|簡中|繁中|简体|簡體|繁体|繁體|\bCHS\b|\bCHT\b|\bGB\b|\bBIG5\b/i)?.[0] ?? ''
}

export function subscriptionWords(item: DiscoverItem, keyword: string): string {
  // 保留原始括号，避免「字幕组」误匹配「另一字幕组」。
  const group = item.title.match(/^\s*([\[【（(][^\]】）)]+[\]】）)])/)?.[1] || item.fansub_group
  const name = subscriptionTitle(item)
  const title = item.title.toLowerCase()
  // 剥离季标记后番名可能不连续；拆成全部必需的片段，不删除搜索或版本条件。
  // 简繁等价比较由后端统一完成，前端保留用户看到的原始文字。
  const names = title.includes(name.toLowerCase()) ? [name] : name.split(/\s+/)
  return [...new Set([keyword, ...names, group, item.quality, releaseLanguage(item)]
    .flatMap((s) => s.split(','))
    .map((s) => s.trim()).filter(Boolean))].join(',')
}

export function releaseEpisodes(items: DiscoverItem[]): number[] {
  return [...new Set(items.filter((item) => /^\d{1,3}$/.test(item.episode)).map((item) => Number(item.episode)))]
    .sort((a, b) => a - b)
}

// Expanded intent is not a title substring; manual keyword rules stay intact.
export function discoveredSubscriptionWords(item: DiscoverItem, keyword: string, expanded: boolean): string {
  return subscriptionWords(item, expanded ? '' : keyword)
}

export function seasonLabel(season = 0): string {
  return season < 0 ? '季数冲突' : season === 0 ? '季数未标明' : `第 ${season} 季`
}

export function releaseSeason(item: DiscoverItem): number {
  return item.season || item.inferred_season || 0
}

export function releaseSeasonLabel(item: DiscoverItem): string {
  return !item.season && item.inferred_season ? `推测第 ${item.inferred_season} 季` : seasonLabel(item.season)
}

export function episodeSummary(episodes: number[]): string {
  const ranges: string[] = []
  const format = (episode: number) => String(episode).padStart(2, '0')
  for (let i = 0; i < episodes.length; i++) {
    const start = episodes[i]!
    let end = start
    while (episodes[i + 1] === end + 1) end = episodes[++i]!
    ranges.push(start === end ? format(start) : `${format(start)}–${format(end)}`)
  }
  return ranges.join('、')
}

export function groupReleases(items: DiscoverItem[]) {
  const groups = new Map<string, { key: string; title: string; language: string; item: DiscoverItem; items: DiscoverItem[] }>()
  for (const item of items) {
    const title = subscriptionTitle(item)
    const language = releaseLanguage(item)
    // Each card must be reproducible by one real RSS, not a query union.
    const key = JSON.stringify([...([title, item.fansub_group, item.quality, language].map((s) => s.toLowerCase())), item.season ?? 0, item.inferred_season ?? 0, item.feed_url ?? ''])
    const existing = groups.get(key)
    if (existing) existing.items.push(item)
    else groups.set(key, { key, title, language, item, items: [item] })
  }
  return [...groups.values()].map((group) => ({
    ...group,
    episodes: releaseEpisodes(group.items),
    latest: Math.max(0, ...group.items.map((item) => Date.parse(item.published) || 0)),
  }))
}

export type Release = ReturnType<typeof groupReleases>[number]
export type ReleaseField = 'group' | 'quality' | 'language'
export type ReleaseSort = 'source' | 'latest' | 'episodes'
export type ReleaseFilters = Record<ReleaseField, string>

export function releaseField(release: Release, field: ReleaseField): string {
  const value = field === 'group' ? release.item.fansub_group : field === 'quality' ? release.item.quality : release.language
  return value.trim().toLowerCase() || '未识别'
}

export function filterReleases(releases: Release[], filters: ReleaseFilters, sort: ReleaseSort): Release[] {
  const filtered = releases.filter((release) =>
    (Object.keys(filters) as ReleaseField[]).every((field) => !filters[field] || releaseField(release, field) === filters[field]),
  )
  if (sort === 'latest') filtered.sort((a, b) => b.latest - a.latest)
  if (sort === 'episodes') filtered.sort((a, b) => b.episodes.length - a.episodes.length || b.latest - a.latest)
  return filtered
}
