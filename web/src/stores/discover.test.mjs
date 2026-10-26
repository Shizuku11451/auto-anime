import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import ts from 'typescript'
import * as vue from 'vue'

function fixture() {
  const requests = []
  const modules = { vue, '@/api/client': {
    http: { get: (url, options) => new Promise((resolve, reject) => requests.push({ url, options, resolve, reject })) },
    errorMessage: e => e.message,
  } }
  const context = { exports: {}, require: id => modules[id], AbortController }
  const source = readFileSync(new URL('./discover.ts', import.meta.url), 'utf8')
  runInNewContext(ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText, context)
  const store = context.exports.useDiscover()
  store.state.sources = [{ id: 'a' }, { id: 'b' }]
  store.state.source = 'a'
  store.state.keyword = '夏日'
  return { store, requests }
}

test('换源取消旧搜索，迟到成功与失败均不能覆盖新结果或结束新加载', async () => {
  const { store, requests } = fixture()
  const old = store.search()
  store.state.source = 'b'
  const fresh = store.search()
  assert.equal(requests[0].options.signal.aborted, true)
  requests[0].resolve({ keyword: '旧结果' }); await old
  assert.equal(store.state.result, null)
  assert.equal(store.state.searching, true)
  requests[1].resolve({ keyword: '夏日', source: 'b' }); await fresh
  assert.equal(store.state.result.source, 'b')
  const fail = store.search()
  store.state.keyword = '冬日'
  const good = store.search()
  requests[3].resolve({ keyword: '冬日' }); await good
  requests[2].reject(new Error('过时错误')); await fail
  assert.equal(store.state.result.keyword, '冬日')
  assert.equal(store.state.error, '')
})

test('取消后立即释放加载状态，保留上次结果，迟到错误不显示为搜索失败', async () => {
  const { store, requests } = fixture()
  store.state.result = { keyword: '上次结果' }
  const search = store.search()
  store.cancelSearch()
  assert.equal(requests[0].options.signal.aborted, true)
  assert.equal(store.state.searching, false)
  assert.equal(store.state.cancelled, true)
  requests[0].reject(new Error('AbortError')); await search
  assert.equal(store.state.result.keyword, '上次结果')
  assert.equal(store.state.error, '')
  assert.equal(store.state.recent.length, 0)
})

test('重复提交不产生重复请求，重试准确复用失败查询而非未提交输入', async () => {
  const { store, requests } = fixture()
  const search = store.search()
  await store.search()
  assert.equal(requests.length, 1)
  requests[0].reject(new Error('连接超时')); await search
  store.state.keyword = '尚未搜索的输入'
  const retry = store.retry()
  assert.equal(store.state.keyword, '夏日')
  assert.equal(requests[1].url, requests[0].url)
  requests[1].resolve({ keyword: '夏日', items: [] }); await retry
  assert.equal(store.state.error, '')
  assert.deepEqual(Array.from(store.state.recent), ['夏日'])
})

test('最近搜索去重且最多保留六条，空关键词或失效来源不请求', async () => {
  const { store, requests } = fixture()
  for (const word of ['1', '2', '3', '4', '5', '6', '7', '3']) {
    store.state.keyword = word
    const pending = store.search()
    requests.at(-1).resolve({ keyword: word }); await pending
  }
  assert.deepEqual(Array.from(store.state.recent), ['3', '7', '6', '5', '4', '2'])
  store.state.keyword = '   '; await store.search()
  store.state.keyword = '输入'; store.state.source = 'removed'; await store.search()
  assert.equal(requests.length, 8)
})

test('原词模式独立请求，重试保留模式，旧扩展响应不覆盖新查询', async () => {
  const { store, requests } = fixture()
  const old = store.search()
  store.state.literal = true
  const literal = store.search()
  assert.ok(requests[1].url.endsWith('&literal=1'))
  requests[0].resolve({ keyword: '扩展' }); await old
  assert.equal(store.state.result, null)
  requests[1].reject(new Error('暂时失败')); await literal
  store.state.literal = false
  const retry = store.retry()
  assert.equal(store.state.literal, true)
  assert.equal(requests[2].url, requests[1].url)
  requests[2].resolve({ keyword: '原词' }); await retry
  assert.equal(store.state.result.keyword, '原词')
})

test('来源刷新隔离迟到响应，失败保留已有来源并退出加载状态', async () => {
  const { store, requests } = fixture()
  const old = store.loadSources(), fresh = store.loadSources()
  requests[1].resolve({ sources: [{ id: 'b' }] }); await fresh
  requests[0].resolve({ sources: [{ id: 'a' }] }); await old
  assert.equal(store.state.source, 'b')
  assert.equal(store.state.sources[0].id, 'b')
  const fail = store.loadSources()
  requests[2].reject(new Error('来源暂不可用')); await fail
  assert.equal(store.state.sources.length, 1)
  assert.equal(store.state.sourcesError, '来源暂不可用')
  assert.equal(store.state.sourcesLoading, false)
})

test('清空来源取消旧请求并清除旧结果；未配置时不搜索，配置后可继续使用', async () => {
  const { store, requests } = fixture()
  store.state.result = { keyword: '旧结果' }
  const old = store.search()
  const empty = store.loadSources()
  requests[1].resolve({ sources: [] }); await empty
  assert.equal(requests[0].options.signal.aborted, true)
  assert.equal(store.state.source, '')
  assert.equal(store.state.result, null)
  assert.equal(store.state.attempt, null)
  assert.equal(store.state.cancelled, false)
  await store.search()
  assert.equal(requests.length, 2)
  requests[0].resolve({ keyword: '迟到结果' }); await old
  assert.equal(store.state.result, null)
  const configured = store.loadSources()
  requests[2].resolve({ sources: [{ id: 'custom' }] }); await configured
  assert.equal(store.state.source, 'custom')
  const search = store.search()
  assert.ok(requests[3].url.includes('source=custom'))
  requests[3].resolve({ keyword: '夏日' }); await search
  assert.equal(store.state.result.keyword, '夏日')
})
