import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import ts from 'typescript'
import * as vue from 'vue'

const source = readFileSync(new URL('./dialogs.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
}).outputText
const flush = () => new Promise((resolve) => setImmediate(resolve))

function dialogStore() {
  const requests = []
  const modules = {
    vue,
    '@/api/client': { http: { get: (url) => new Promise((resolve, reject) => requests.push({ url, resolve, reject })) } },
  }
  const context = { exports: {}, require: (id) => {
    assert.ok(id in modules, `unmocked store import: ${id}`)
    return modules[id]
  } }
  runInNewContext(compiled, context, { filename: 'dialogs.ts' })
  return { store: context.exports.useDialogs(), requests }
}

test('重新识别后的详情刷新不被较早轮询的结果覆盖', async () => {
  const { store, requests } = dialogStore()
  store.openAnimeDetail(1)
  const refresh = store.reloadAnimeDetail()
  requests[1].resolve({ id: 1, title: '新匹配', cover: '/api/covers/new.jpg' })
  await refresh
  requests[0].resolve({ id: 1, title: '旧匹配', cover: '/api/covers/old.jpg' })
  await flush()
  assert.equal(store.state.animeDetail.data.title, '新匹配')
  assert.equal(store.state.animeDetail.loading, false)
})

test('旧请求失败不会结束新刷新或写入错误提示', async () => {
  const { store, requests } = dialogStore()
  store.openAnimeDetail(1)
  const refresh = store.reloadAnimeDetail()
  requests[0].reject(new Error('旧轮询失败'))
  await flush()
  assert.equal(store.state.animeDetail.loading, true)
  assert.equal(store.state.animeDetail.error, '')
  requests[1].resolve({ id: 1, title: '最新数据' })
  await refresh
  assert.equal(store.state.animeDetail.data.title, '最新数据')
})

test('关闭后迟到的详情响应不会写入，重新打开同一番剧使用新请求', async () => {
  const { store, requests } = dialogStore()
  store.openAnimeDetail(1)
  store.state.animeDetail.open = false
  requests[0].resolve({ id: 1, title: '关闭前的数据' })
  await flush()
  assert.equal(store.state.animeDetail.data, null)
  store.openAnimeDetail(1)
  requests[1].resolve({ id: 1, title: '重新打开的数据' })
  await flush()
  assert.equal(store.state.animeDetail.data.title, '重新打开的数据')
})
