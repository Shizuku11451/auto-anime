/* 连播列表：最近条目与番剧详情共用。 */
import type { AnimeDetail, ResourceItem } from '@/api/types'
import type { PlayerEntry } from '@/stores/dialogs'

type Playable = Pick<ResourceItem, 'id' | 'season' | 'episode' | 'fansub_group' | 'quality' | 'file_path'>

/** 条目自身的季号优先，否则用所在分组的季 */
export function playerEntry(title: string, r: Playable, season = 1): PlayerEntry {
  return {
    id: r.id,
    title,
    season: r.season > 0 ? r.season : season,
    episode: r.episode,
    fansub_group: r.fansub_group,
    quality: r.quality,
    file_path: r.file_path,
  }
}

/** 番剧详情内所有已保存资源按季/集排序 */
export function playerQueue(d: AnimeDetail): PlayerEntry[] {
  const title = d.meta_name_cn || d.title
  const entries: PlayerEntry[] = []
  for (const se of [...d.seasons].sort((a, b) => a.season - b.season)) {
    const rows = [...se.episodes.flatMap((ep) => ep.items), ...se.loose]
    rows.sort((a, b) => epNo(a) - epNo(b))
    for (const r of rows) {
      if (r.status === '已保存' && r.file_path) entries.push(playerEntry(title, r, se.season))
    }
  }
  return entries
}

function epNo(x: { episode: string }): number {
  const n = Number.parseFloat(x.episode)
  return Number.isFinite(n) ? n : Number.MAX_SAFE_INTEGER
}
