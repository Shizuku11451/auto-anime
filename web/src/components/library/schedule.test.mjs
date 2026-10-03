import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import ts from 'typescript'

const source = readFileSync(new URL('./schedule.ts', import.meta.url), 'utf8')
const ctx = { exports: {}, Date }
runInNewContext(ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS } }).outputText, ctx)
const { localDate, weekdayOfDate } = ctx.exports
const { effectiveWeekday, scheduleLabel } = ctx.exports

test('自动放送星期参与筛选，手动设置优先且可恢复自动', () => {
  const anime = { update_weekday: 0, broadcast_weekday: 7 }
  assert.equal(effectiveWeekday(anime), 7)
  assert.equal(scheduleLabel(anime), '每周日放送 · Bangumi')
  anime.update_weekday = 1
  assert.equal(effectiveWeekday(anime), 1)
  assert.equal(scheduleLabel(anime), '每周一更新 · 手动')
  anime.update_weekday = 0
  assert.equal(effectiveWeekday(anime), 7)
  assert.equal(effectiveWeekday({}), 0)
  assert.equal(scheduleLabel({}), '更新日待补充')
})

test('本地日期和周日、周一、跨年、闰日的星期正确', () => {
  assert.equal(localDate(new Date(2026, 9, 3, 0, 1)), '2026-10-03')
  for (const [date, day] of [['2026-10-03', 6], ['2026-10-04', 7], ['2026-10-05', 1], ['2027-01-01', 5], ['2024-02-29', 4]]) {
    assert.equal(weekdayOfDate(date), day)
  }
})
test('清空或无效日期不会错误地对应某个星期', () => {
  for (const date of ['', '2026-02-29', '2026-13-01', '2026-04-31', '2026-00-01', 'invalid']) assert.equal(weekdayOfDate(date), null)
})
