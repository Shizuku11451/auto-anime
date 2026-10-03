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
const compiled = compile(source + '\nexport { selected, selecting, batch, batchRunning, batchFailures, downloadable, pausable, resumable, control, search, applySearch, clearFilters, changePage, selectVisible, toggle, queryPending, refreshItems, refreshing, refreshMessage };')
const status = { exports: {} }
runInNewContext(compile(readFileSync(new URL('../utils/status.ts', import.meta.url), 'utf8')), status)

function itemsView(t) {
  const calls = [], messages = [], queries = []
  let refreshed = 0
  let confirmation = async () => true
  let debouncedSearch
  let response = async () => ({ ok: true, count: 1, skipped: 0, failed: [] })
  const store = {
    items: vue.ref(['待下载', '下载中', '已暂停', '已保存', '下载失败'].map((status, i) => ({ id: i + 1, status, title: `条目 ${i + 1}` }))),
    itemFilter: vue.ref('全部'),
    pageNumber: vue.ref(1), nextBefore: vue.ref(10), loaded: vue.ref(true), loadError: vue.ref(''),
    resetItemPage() { this.pageNumber.value = 1 },
    load: async () => { queries.push([store.itemFilter.value, store.search.value, store.pageNumber.value]) },
    nextPage: async () => { store.pageNumber.value++; await store.load() },
    previousPage: async () => { store.pageNumber.value--; await store.load() },
    counts: vue.ref({}), totalItems: vue.ref(5), search: vue.ref(''),
    refresh: async () => { refreshed++ },
  }
  const busy = vue.ref(new Set())
  const modules = {
    vue,
    '@vueuse/core': { watchDebounced(_source, callback) { debouncedSearch = callback } },
    '@/stores/app': { useAppStore: () => store },
    '@/stores/dialogs': { useDialogs: () => ({}) },
    '@/composables/useBusy': { useBusySet: () => ({
      is: id => busy.value.has(id),
      run: async (id, fn) => { if (busy.value.has(id)) return; busy.value.add(id); try { return await fn() } finally { busy.value.delete(id) } },
    }) },
    '@/api/client': { http: { post: async (...args) => { calls.push(args); return response() } }, errorMessage: e => e.message },
    '@/stores/confirm': { confirmDialog: (...args) => confirmation(...args) },
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
  return { component: context.exports, store, calls, messages, queries, refreshed: () => refreshed, respond: fn => { response = fn }, confirm: fn => { confirmation = fn }, debounce: () => debouncedSearch() }
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

test('切换状态和翻页清除旧选区，不会操作上一页或隐藏条目', async (t) => {
  const h = itemsView(t)
  h.component.selected.value = [1, 2, 3]
  h.store.itemFilter.value = '已暂停'
  // 即使筛选请求尚未开始，批量操作也只能取当前可见状态。
  assert.equal(h.component.pausable.value, 0)
  await vue.nextTick()
  assert.deepEqual(Array.from(h.component.selected.value), [])
  assert.deepEqual(h.queries[0], ['已暂停', '', 1])
  h.component.selected.value = [3]
  await h.component.changePage('next')
  assert.equal(h.store.pageNumber.value, 2)
  assert.deepEqual(Array.from(h.component.selected.value), [])
  await h.component.changePage('first')
  assert.equal(h.store.pageNumber.value, 1)
})

test('搜索输入立即清理选区，回车立即提交且随后的防抖不重复请求', async (t) => {
  const h = itemsView(t)
  h.component.selected.value = [1, 2]
  h.store.pageNumber.value = 3
  h.component.search.value = '  夏目友人帐  '
  assert.deepEqual(Array.from(h.component.selected.value), [])
  assert.equal(h.component.queryPending.value, true)
  h.component.selectVisible()
  assert.equal(h.component.selected.value.length, 0)
  h.component.applySearch()
  await vue.nextTick()
  assert.deepEqual(h.queries, [['全部', '夏目友人帐', 1]])
  h.debounce()
  await vue.nextTick()
  assert.equal(h.queries.length, 1)
})

test('清除筛选合并关键词和状态变化为一次查询，旧防抖不会恢复关键词', async (t) => {
  const h = itemsView(t)
  h.store.itemFilter.value = '下载失败'
  h.component.search.value = '夏目'
  h.component.applySearch()
  await vue.nextTick()
  h.component.clearFilters()
  await vue.nextTick()
  h.debounce()
  await vue.nextTick()
  assert.equal(h.queries.length, 2)
  assert.deepEqual(h.queries[1], ['全部', '', 1])
})

test('批量移除在确认期间锁定选区和重复操作，取消后保留选中', async (t) => {
  const h = itemsView(t)
  let finish
  let confirmations = 0
  h.confirm(() => { confirmations++; return new Promise(resolve => { finish = resolve }) })
  h.component.selected.value = [1, 3]
  const pending = h.component.batch('delete')
  h.component.toggle(1)
  h.component.selectVisible()
  await h.component.changePage('next')
  await h.component.batch('delete')
  assert.equal(confirmations, 1)
  assert.equal(h.store.pageNumber.value, 1)
  assert.deepEqual(Array.from(h.component.selected.value), [1, 3])
  finish(false)
  await pending
  assert.equal(h.calls.length, 0)
  assert.equal(h.refreshed(), 0)
  assert.equal(h.component.batchRunning.value, null)
  assert.deepEqual(Array.from(h.component.selected.value), [1, 3])
})

test('批量失败保留选中，刷新期间消失的条目不残留在选区', async (t) => {
  const h = itemsView(t)
  h.component.selected.value = [1, 2]
  h.respond(async () => { throw new Error('模拟网络中断') })
  await h.component.batch('pause')
  assert.deepEqual(Array.from(h.component.selected.value), [1, 2])
  h.respond(async () => ({ count: 1, skipped: 0, failed: [{ id: 2, error: '下载器不可达' }] }))
  h.store.refresh = async () => { h.store.items.value = [] }
  await h.component.batch('pause')
  assert.deepEqual(Array.from(h.component.selected.value), [])
  assert.equal(h.component.batchRunning.value, null)
})

test('手动刷新防止重复提交且失败不显示成功提示', async (t) => {
  const h = itemsView(t)
  let finish
  let refreshes = 0
  h.store.refresh = () => { refreshes++; return new Promise(resolve => { finish = resolve }) }
  const pending = h.component.refreshItems()
  await h.component.refreshItems()
  assert.equal(refreshes, 1)
  assert.equal(h.component.refreshing.value, true)
  h.store.loadError.value = '模拟网络中断'
  finish()
  await pending
  assert.equal(h.component.refreshing.value, false)
  assert.equal(h.component.refreshMessage.value, '')
})
