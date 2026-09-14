import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import { parse } from '@vue/compiler-sfc'
import ts from 'typescript'
import * as vue from 'vue'

const compile = source => ts.transpileModule(source, {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
}).outputText
const source = parse(readFileSync(new URL('./ItemsView.vue', import.meta.url), 'utf8')).descriptor.scriptSetup.content
const compiled = compile(source + '\nexport { selected, selecting, batch, batchRunning, batchFailures, downloadable, pausable, resumable, control };')
const status = { exports: {} }
runInNewContext(compile(readFileSync(new URL('../utils/status.ts', import.meta.url), 'utf8')), status)

function itemsView(t) {
  const calls = [], messages = []
  let refreshed = 0
  let response = async () => ({ ok: true, count: 1, skipped: 0, failed: [] })
  const store = {
    items: vue.ref(['待下载', '下载中', '已暂停', '已保存', '下载失败'].map((status, i) => ({ id: i + 1, status, title: `条目 ${i + 1}` }))),
    counts: vue.ref({}), totalItems: vue.ref(5), search: vue.ref(''),
    refresh: async () => { refreshed++ },
  }
  const busy = vue.ref(new Set())
  const modules = {
    vue,
    '@vueuse/core': { watchDebounced() {} },
    '@/stores/app': { useAppStore: () => store },
    '@/stores/dialogs': { useDialogs: () => ({}) },
    '@/composables/useBusy': { useBusySet: () => ({
      is: id => busy.value.has(id),
      run: async (id, fn) => { if (busy.value.has(id)) return; busy.value.add(id); try { return await fn() } finally { busy.value.delete(id) } },
    }) },
    '@/api/client': { http: { post: async (...args) => { calls.push(args); return response() } }, errorMessage: e => e.message },
    '@/stores/confirm': { confirmDialog: async () => true },
    '@/stores/toast': { toast: Object.fromEntries(['success', 'info', 'error'].map(type => [type, message => messages.push({ type, message })])) },
    '@/utils/status': status.exports,
  }
  const context = { exports: {}, require: id => {
    assert.ok(id in modules, `unexpected setup import: ${id}`)
    return modules[id]
  } }
  const scope = vue.effectScope()
  scope.run(() => runInNewContext(compiled, context))
  t.after(() => scope.stop())
  return { component: context.exports, store, calls, messages, refreshed: () => refreshed, respond: fn => { response = fn } }
}

test('批量控制仅提交符合操作状态的选中任务，并保留失败条目供重试', async (t) => {
  const h = itemsView(t)
  h.component.selected.value = [1, 2, 3, 4, 5]
  h.component.selecting.value = true
  assert.equal(h.component.downloadable.value, 2)
  assert.equal(h.component.pausable.value, 2)
  assert.equal(h.component.resumable.value, 1)
  h.respond(async () => ({ ok: false, count: 1, skipped: 0, failed: [{ id: 2, error: '下载器不可达' }] }))
  await h.component.batch('pause')
  assert.deepEqual(JSON.parse(JSON.stringify(h.calls[0])), ['/api/items/manage', { action: 'pause', ids: [1, 2] }])
  // 失败的 2 与未执行可操作的 3、4、5 保持选中，已成功的 1 被移出选区。
  assert.deepEqual(Array.from(h.component.selected.value), [2, 3, 4, 5])
  assert.equal(h.component.batchFailures.value[0].error, '下载器不可达')
  assert.equal(h.component.selecting.value, true)
  assert.ok(h.messages.some(x => x.type === 'success' && x.message.includes('1 条')))
  assert.ok(h.messages.some(x => x.type === 'error' && x.message.includes('1 条')))
  assert.equal(h.refreshed(), 1)
})

test('批量操作期间锁定重复请求，无可操作的条目时不提交', async (t) => {
  const h = itemsView(t)
  h.component.selected.value = [3, 4]
  await h.component.batch('pause')
  assert.equal(h.calls.length, 0)
  let finish
  h.respond(() => new Promise(resolve => { finish = resolve }))
  const pending = h.component.batch('resume')
  await h.component.batch('resume')
  assert.equal(h.calls.length, 1)
  assert.equal(h.component.batchRunning.value, 'resume')
  finish({ ok: true, count: 1, skipped: 0, failed: [] })
  await pending
  assert.deepEqual(Array.from(h.component.selected.value), [4])
  assert.equal(h.component.batchRunning.value, null)
})

test('单条控制失败反馈真实原因并刷新，重复暂停不误报新操作', async (t) => {
  const h = itemsView(t)
  h.respond(async () => { throw new Error('暂停失败：连接超时') })
  await h.component.control(h.store.items.value[1], 'pause')
  assert.equal(h.calls[0][0], '/api/items/2/pause')
  assert.equal(h.messages.at(-1).message, '暂停失败：连接超时')
  assert.equal(h.refreshed(), 1)
  h.respond(async () => ({ changed: false }))
  await h.component.control(h.store.items.value[2], 'pause')
  assert.equal(h.messages.at(-1).type, 'info')
})

test('已暂停与等待继续的筛选和按钮语义一致', () => {
  assert.equal(status.exports.isRetryable('已暂停'), false)
  assert.equal(status.exports.isPausable('已暂停'), false)
  assert.equal(status.exports.isResumable('已暂停'), true)
  assert.equal(status.exports.isPausable('等待继续'), true)
  assert.equal(status.exports.isDownloading('等待继续'), false)
  assert.equal(status.exports.isDownloading('继续中'), true)
  assert.equal(status.exports.isPausable('继续中'), true)
  assert.equal(status.exports.badgeInfo('等待继续').text, '排队中 · 继续')
  const paused = status.exports.CHIP_DEFS.find(x => x.key === '已暂停')
  assert.equal(status.exports.chipCount(paused, { '已暂停': 3, '等待继续': 1, '下载中': 2 }), 3)
})
