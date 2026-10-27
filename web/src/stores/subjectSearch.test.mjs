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
  const source = readFileSync(new URL('./subjectSearch.ts', import.meta.url), 'utf8')
  runInNewContext(ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText, context)
  return { store: context.exports.useSubjectSearch(), requests }
}

test('无需 RSS 配置先搜条目，展开后才发送条目 ID 搜索资源', async () => {
  const { store, requests } = fixture()
  store.state.keyword = '测试动画'
  const search = store.search()
  await store.search()
  assert.equal(requests.length, 1)
  assert.ok(requests[0].url.startsWith('/api/discover/subjects?keyword='))
  requests[0].resolve({ items: [{ id: 1 }, { id: 2 }], resources_enabled: true }); await search
  assert.equal(requests.length, 1)
  const resources = store.select(1)
  assert.equal(requests[1].url, '/api/discover/resources?subject_id=1')
  requests[1].resolve({ subject_id: 1, items: [], warnings: [] }); await resources
  assert.equal(store.state.resources.subject_id, 1)
})

test('切换条目和关闭取消旧资源请求，迟到成功失败不能覆盖新状态', async () => {
  const { store, requests } = fixture()
  store.state.result = { items: [{ id: 1 }, { id: 2 }], resources_enabled: true }
  const old = store.select(1), fresh = store.select(2)
  assert.equal(requests[0].options.signal.aborted, true)
  requests[0].resolve({ subject_id: 1 }); await old
  assert.equal(store.state.resources, null)
  assert.equal(store.state.loadingResources, true)
  requests[1].resolve({ subject_id: 2 }); await fresh
  assert.equal(store.state.resources.subject_id, 2)
  const closing = store.select(1)
  store.close()
  requests[2].reject(new Error('过时失败')); await closing
  assert.equal(store.state.selected, 0)
  assert.equal(store.state.resourceError, '')
  assert.equal(store.state.loadingResources, false)
})

test('新搜索和卸载隔离迟到响应；未启用聚合服务时不请求资源', async () => {
  const { store, requests } = fixture()
  store.state.keyword = '旧搜索'
  const old = store.search()
  store.state.keyword = '新搜索'
  const fresh = store.search()
  requests[1].resolve({ keyword: '新搜索', items: [{ id: 1 }], resources_enabled: false }); await fresh
  requests[0].reject(new Error('过时错误')); await old
  assert.equal(store.state.error, '')
  assert.equal(store.state.result.keyword, '新搜索')
  await store.select(1)
  await store.select(999)
  assert.equal(requests.length, 2)
  const leaving = store.search()
  store.cancel()
  assert.equal(requests[2].options.signal.aborted, true)
  requests[2].resolve({ keyword: '迟到' }); await leaving
  assert.equal(store.state.result, null)
  assert.equal(store.state.searching, false)
})
