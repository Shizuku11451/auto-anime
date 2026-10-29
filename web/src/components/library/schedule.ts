export const weekdays = ['待补充', '周一', '周二', '周三', '周四', '周五', '周六', '周日']
/* 番剧库分类：0 待补充，1–7 星期，8 已完结。手动更新日只用 0–7。 */
export const FINISHED = 8
export const dayLabels = [...weekdays, '已完结']

type Schedule = { update_weekday?: number; broadcast_weekday?: number; broadcast_end?: string }
/* 播放结束日期早于今天视为已完结；手动指定更新日优先。 */
export function isFinished(anime: Schedule, today = localDate()): boolean {
  return !anime.update_weekday && /^\d{4}-\d{2}-\d{2}$/.test(anime.broadcast_end || '') && anime.broadcast_end! < today
}
export function effectiveWeekday(anime: Schedule, today = localDate()): number {
  if (isFinished(anime, today)) return FINISHED
  const day = anime.update_weekday || anime.broadcast_weekday || 0
  return Number.isInteger(day) && day >= 1 && day <= 7 ? day : 0
}
export function scheduleLabel(anime: Schedule, today = localDate()): string {
  const day = effectiveWeekday(anime, today)
  if (day === FINISHED) return `已完结 · ${anime.broadcast_end}`
  return day ? (anime.update_weekday ? `每${weekdays[day]}更新 · 手动` : `每${weekdays[day]}放送 · Bangumi`) : '更新日待补充'
}

// 用本地日历日期，不经 UTC 转换，避免午夜前后偏移一天。
export function localDate(date = new Date()): string {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`
}

export function weekdayOfDate(value: string): number | null {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return null
  const [year, month, day] = value.split('-').map(Number) as [number, number, number]
  const date = new Date(0)
  date.setFullYear(year, month - 1, day)
  date.setHours(12, 0, 0, 0)
  if (localDate(date) !== value) return null
  return date.getDay() || 7
}
