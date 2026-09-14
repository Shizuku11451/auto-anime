/* 条目状态的归类与展示信息，供徽章、筛选条与动作按钮共用。 */

export type BadgeTone = 'saved' | 'downloading' | 'failed' | 'waiting' | 'pending' | 'paused'

export interface BadgeInfo {
  tone: BadgeTone
  text: string
}

export function badgeInfo(status: string | null | undefined): BadgeInfo {
  const s = status || '待下载'
  if (s === '已保存') return { tone: 'saved', text: '已保存' }
  if (s === '下载中' || s === '继续中' || s.startsWith('已发送到')) return { tone: 'downloading', text: s }
  if (s === '下载失败') return { tone: 'failed', text: '下载失败' }
  if (s === '已暂停') return { tone: 'paused', text: '已暂停' }
  if (s === '等待下载器') return { tone: 'waiting', text: '等待下载器' }
  if (s === '等待下载') return { tone: 'pending', text: '排队中' }
  if (s === '等待继续') return { tone: 'pending', text: '排队中 · 继续' }
  if (s === '待下载') return { tone: 'pending', text: '待下载' }
  return { tone: 'pending', text: s }
}

export function isDownloading(status: string | null | undefined): boolean {
  return badgeInfo(status).tone === 'downloading'
}

/** 这些状态允许手动发起/重试下载 */
export function isRetryable(status: string | null | undefined): boolean {
  return ['待下载', '下载失败', '等待下载器'].includes(status || '待下载')
}

export function isPausable(status: string | null | undefined): boolean {
  return isDownloading(status) || ['待下载', '等待下载', '等待下载器', '等待继续'].includes(status || '待下载')
}

export function isResumable(status: string | null | undefined): boolean {
  return status === '已暂停'
}

export interface ChipDef {
  key: string
  match: (status: string) => boolean
}

export const CHIP_DEFS: ChipDef[] = [
  { key: '全部', match: () => true },
  { key: '待下载', match: (s) => s === '待下载' || s === '等待下载' || s === '等待继续' },
  { key: '下载中', match: isDownloading },
  { key: '已暂停', match: (s) => s === '已暂停' },
  { key: '已保存', match: (s) => s === '已保存' },
  { key: '下载失败', match: (s) => s === '下载失败' },
  { key: '等待下载器', match: (s) => s === '等待下载器' },
]

export function chipCount(def: ChipDef, counts: Record<string, number>): number {
  return Object.entries(counts)
    .filter(([status]) => def.match(status))
    .reduce((sum, [, n]) => sum + n, 0)
}
