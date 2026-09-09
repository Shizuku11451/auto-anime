const pad = (n: number) => String(n).padStart(2, '0')

/** 时间统一显示为 yyyy-MM-dd HH:mm，无法解析时原样返回 */
export function formatTime(raw: string | null | undefined): string {
  if (!raw) return '暂无时间'
  const t = new Date(raw)
  if (Number.isNaN(t.getTime())) return raw
  return `${t.getFullYear()}-${pad(t.getMonth() + 1)}-${pad(t.getDate())} ${pad(t.getHours())}:${pad(t.getMinutes())}`
}

/** 相对时间：刚刚 / 3 分钟前 / 2 小时前 / 昨天 / 具体日期 */
export function relativeTime(raw: string | null | undefined): string {
  if (!raw) return '暂无'
  const t = new Date(raw)
  if (Number.isNaN(t.getTime())) return raw
  const diff = Date.now() - t.getTime()
  const min = Math.round(diff / 60000)
  if (min < 1) return '刚刚'
  if (min < 60) return `${min} 分钟前`
  const hr = Math.round(min / 60)
  if (hr < 24) return `${hr} 小时前`
  const day = Math.round(hr / 24)
  if (day === 1) return '昨天'
  if (day < 7) return `${day} 天前`
  return formatTime(raw)
}

export function seasonLabel(season: number): string {
  return `Season ${pad(Math.max(1, season || 1))}`
}

export function clamp(n: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, n))
}

/** 把逗号/中文逗号分隔的关键词切成数组用于展示 */
export function splitKeywords(raw: string | null | undefined): string[] {
  return String(raw ?? '')
    .split(/[,，]/)
    .map((s) => s.trim())
    .filter(Boolean)
}
