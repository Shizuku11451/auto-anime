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
  return [...new Set([keyword, item.title.includes(name) ? name : '', group, item.quality, releaseLanguage(item)]
    .map((s) => s.trim()).filter(Boolean))].join(',')
}

export function releaseEpisodes(items: DiscoverItem[]): number[] {
  return [...new Set(items.filter((item) => /^\d{1,3}$/.test(item.episode)).map((item) => Number(item.episode)))]
    .sort((a, b) => a - b)
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
    const key = JSON.stringify([title, item.fansub_group, item.quality, language].map((s) => s.toLowerCase()))
    const existing = groups.get(key)
    if (existing) existing.items.push(item)
    else groups.set(key, { key, title, language, item, items: [item] })
  }
  return [...groups.values()].map((group) => ({ ...group, episodes: releaseEpisodes(group.items) }))
}
