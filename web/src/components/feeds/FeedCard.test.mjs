import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import { parse, compileScript, compileTemplate } from '@vue/compiler-sfc'
import { renderToString } from '@vue/server-renderer'
import ts from 'typescript'
import * as vue from 'vue'

const descriptor = parse(readFileSync(new URL('./FeedCard.vue', import.meta.url), 'utf8')).descriptor
const script = compileScript(descriptor, { id: 'feed-test' })
const template = compileTemplate({ source: descriptor.template.content, filename: 'FeedCard.vue', id: 'feed-test', compilerOptions: { bindingMetadata: script.bindings } })
const stub = { render: () => null }
const context = { exports: {}, require: id => ({
  vue,
  '@lucide/vue': new Proxy({}, { get: () => stub }),
  '@/components/ui/BaseButton.vue': stub,
  '@/utils/format': { formatTime: () => '', relativeTime: () => '', splitKeywords: () => [] },
})[id] }
runInNewContext(ts.transpileModule(script.content + '\n' + template.code, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS } }).outputText, context)

test('订阅卡片区分集齐停订和手动暂停，显示服务端原因', async () => {
  const html = async feed => renderToString(vue.createSSRApp({ render: () => vue.h({ ...context.exports.default, render: context.exports.render }, { feed }) }))
  const feed = { name: '测试订阅', url: 'https://example.test/rss', enabled: false, completion_note: 'Bangumi 正篇 12 集已全部归档，已自动停订（第 2 季）' }
  const completed = await html(feed)
  assert.match(completed, /已集齐/)
  assert.match(completed, /12 集已全部归档/)
  assert.doesNotMatch(completed, /已暂停，不参与定时同步/)
  const paused = await html({ ...feed, completion_note: '' })
  assert.match(paused, /已暂停，不参与定时同步/)
  assert.doesNotMatch(paused, /已集齐/)
})
