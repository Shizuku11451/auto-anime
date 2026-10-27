import type { AggregateResource, DiscoverSubject, Feed } from '@/api/types'
import type { FeedPrefill } from '@/stores/dialogs'

export function aggregateSubscription(subject: DiscoverSubject, item: AggregateResource, feeds: Feed[]): FeedPrefill | null {
  if (!Number.isSafeInteger(subject.id) || subject.id <= 0 || subject.id > 2147483647 ||
    !Number.isInteger(item.season) || item.season < 0 || item.season > 999 ||
    !item.anime_title.trim() || !/^magnet:\?xt=urn:btih:[a-f0-9]{40}$/i.test(item.magnet)) return null
  const language = item.title.match(/简繁|簡繁|简日|簡日|繁日|简中|簡中|繁中|简体|簡體|繁体|繁體|\bCHS\b|\bCHT\b|\bGB\b|\bBIG5\b/i)?.[0] ?? ''
  const rules = new URLSearchParams({ title: item.anime_title, group: item.fansub_group, quality: item.quality, season: String(item.season), language })
  rules.sort()
  const url = `aggregate://subject/${subject.id}?${rules}`
  const existing = feeds.find(feed => {
    if (feed.url.split('?')[0] !== `aggregate://subject/${subject.id}`) return false
    const saved = new URLSearchParams(feed.url.split('?')[1])
    saved.sort()
    return saved.toString() === rules.toString()
  })
  return {
    ...(existing ?? {
      name: [subject.name_cn || subject.name, item.fansub_group, item.quality, language].filter(Boolean).join(' · '),
      url,
      target_title: subject.name_cn || subject.name,
      auto_download: true,
      season_filter: item.season > 0 ? item.season : -1,
      episode_offset: 0,
    }),
    from_discover: true,
    requires_season_confirmation: true,
  }
}
