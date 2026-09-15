/* 更新相关：版本比较与更新说明的轻量渲染（纯函数，便于测试）。 */

/** 版本号主体：v1.2.3-rc1+build → [1,2,3]；无法解析返回 null */
export function versionParts(value: string): number[] | null {
  let core = (value || '').trim().replace(/^[vV]/, '')
  const cut = core.search(/[-+]/)
  if (cut >= 0) core = core.slice(0, cut)
  if (!core) return null
  const parts: number[] = []
  for (const field of core.split('.')) {
    if (!/^\d+$/.test(field)) return null
    parts.push(Number(field))
  }
  return parts
}

/** 语义化比较：a<b 返回 -1，相等 0，a>b 返回 1；无法解析视为相等 */
export function compareVersions(a: string, b: string): number {
  const left = versionParts(a)
  const right = versionParts(b)
  if (!left || !right) return 0
  const length = Math.max(left.length, right.length)
  for (let i = 0; i < length; i++) {
    const x = left[i] ?? 0
    const y = right[i] ?? 0
    if (x !== y) return x < y ? -1 : 1
  }
  return 0
}

/** 本地构建（dev）不支持自动更新 */
export function isDevVersion(version: string): boolean {
  return !versionParts(version)
}

export interface NoteBlock {
  /** heading=小标题，bullet=列表项，text=正文 */
  kind: 'heading' | 'bullet' | 'text'
  text: string
}

/**
 * 把更新说明整理成可渲染的块。后端已把 HTML 转成纯文本，这里只做安全的
 * 分行/列表识别，不解析任何标记，因此不需要 v-html。
 */
export function noteBlocks(notes: string): NoteBlock[] {
  const lines = (notes || '').replace(/\r\n/g, '\n').split('\n')
  const blocks: NoteBlock[] = []
  for (const raw of lines) {
    const line = raw.trimEnd()
    const trimmed = line.trim()
    if (!trimmed) continue
    const heading = /^#{1,6}\s+/.exec(trimmed)
    if (heading) {
      blocks.push({ kind: 'heading', text: trimmed.slice(heading[0].length).trim() })
      continue
    }
    const bullet = /^(?:[-*•]|\d+[.、)])\s*/.exec(trimmed)
    if (bullet) {
      blocks.push({ kind: 'bullet', text: trimmed.slice(bullet[0].length).trim() })
      continue
    }
    blocks.push({ kind: 'text', text: trimmed })
  }
  return blocks
}

/** 版本标签：dev 或 v1.2.0（abc1234） */
export function versionLabel(version: string, commit = ''): string {
  const value = (version || '').trim() || 'dev'
  const short = (commit || '').trim().slice(0, 7)
  return short ? `${value} (${short})` : value
}

/** 把版本写入 localStorage 的键名 */
export const SKIPPED_VERSION_KEY = 'auto-anime:skipped-version'
