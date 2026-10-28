import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { parse } from '@vue/compiler-sfc'

const read = path => parse(readFileSync(new URL(path, import.meta.url), 'utf8')).descriptor

test('每个可编辑分类各自是表单并通过提交保存，Bangumi 操作不嵌入设置表单', () => {
  const section = read('../components/settings/SettingsSection.vue').template.content
  assert.match(section, /<form v-if="editable" novalidate @submit\.prevent="s\.save\(id\)">/)
  assert.match(section, /<BaseButton type="submit"/)
  const view = read('./SettingsView.vue')
  assert.match(view.scriptSetup.content, /bangumi: BangumiWishes/)
  assert.doesNotMatch(view.template.content, /<form/)
})

test('下载模式使用原生单选组', () => {
  assert.match(read('../components/settings/DownloadSettings.vue').template.content, /type="radio" name="download-mode"/)
})

test('设置页分类由 tab 查询参数驱动并在切换前处理未保存更改', () => {
  const script = read('./SettingsView.vue').scriptSetup.content
  assert.match(script, /onBeforeRouteUpdate/)
  assert.match(script, /onBeforeRouteLeave\(canLeave\)/)
  assert.match(script, /selectSection/)
})
