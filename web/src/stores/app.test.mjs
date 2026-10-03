import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import ts from 'typescript'
import * as vue from 'vue'

function fixture() {
  const requests = []
  const modules = { vue, '@vueuse/core': {}, '@/api/client': {
    http: { get: (url, options) => new Promise((resolve, reject) => requests.push({ url, options, resolve, reject })) }, errorMessage: e => e.message,
  } }
  const context = { exports: {}, require: id => modules[id], AbortController, URLSearchParams }
  const source = readFileSync(new URL('./app.ts', import.meta.url), 'utf8')
  runInNewContext(ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText, context)
  return { store: context.exports.useAppStore(), requests }
}

test('较早的运行状态响应和错误不能覆盖新结果', async () => {
  const { store, requests } = fixture()
  const old = store.loadRuntime(), fresh = store.loadRuntime()
  requests[1].resolve({ download_mode: 'fresh' }); await fresh
  requests[0].resolve({ download_mode: 'old' }); await old
  assert.equal(store.status.value.download_mode, 'fresh')
  const fail = store.loadRuntime(), good = store.loadRuntime()
  requests[3].resolve({ download_mode: 'latest' }); await good
  requests[2].reject(new Error('old error')); await fail
  assert.equal(store.runtimeError.value, false)
})

test('改变筛选取消旧列表请求，并从第一页请求完整状态集合', async () => {
  const { store, requests } = fixture()
  const old = store.load()
  store.itemFilter.value = '已暂停'; store.resetItemPage()
  const fresh = store.load()
  assert.equal(requests[0].options.signal.aborted, true)
  assert.equal(new URL(requests[4].url, 'http://localhost').searchParams.get('group'), '已暂停')
  requests[3].resolve([]); requests[4].resolve({ items: [{ id: 102 }], next_before: 3 }); requests[5].resolve([]); await fresh
  requests[0].resolve([]); requests[1].resolve({ items: [{ id: 999 }], next_before: 0 }); requests[2].resolve([]); await old
  assert.equal(store.items.value[0].id, 102)
  const next = store.nextPage()
  assert.equal(new URL(requests[7].url, 'http://localhost').searchParams.get('before'), '3')
  requests[6].resolve([]); requests[7].resolve({ items: [{ id: 2 }], next_before: 0 }); requests[8].resolve([]); await next
  assert.equal(store.pageNumber.value, 2)
  const back = store.previousPage()
  assert.equal(new URL(requests[10].url, 'http://localhost').searchParams.has('before'), false)
  requests[9].resolve([]); requests[10].resolve({ items: [], next_before: 0 }); requests[11].resolve([]); await back
  assert.equal(store.pageNumber.value, 1)
})
