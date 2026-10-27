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
  const subjects = []
  const modules = {
    vue,
    '@/api/client': { http: { get: (url, options) => new Promise((resolve, reject) => (url.startsWith('/api/bangumi/subjects/') ? subjects : requests).push({ url, options, resolve, reject })) } },
  }
  const context = { exports: {}, Error, AbortController, require: (id) => {
    assert.ok(id in modules, `unmocked store import: ${id}`)
    return modules[id]
  } }
  runInNewContext(compiled, context, { filename: 'dialogs.ts' })
  return { store: context.exports.useDialogs(), requests, subjects }
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

const cloud = { id: -101, bangumi_id: 101, title: '云端动画', meta_name_cn: '收藏动画', cover: '/cover.jpg', collectionType: 3, cloudOnly: true, item_count: 0, saved_count: 0 }

test('云端收藏打开只读详情，先查本地关联，不请求负数详情或创建番剧', async () => {
  const { store, requests } = dialogStore()
  store.openAnimeDetail(-101, cloud)
  assert.equal(store.state.animeDetail.data.title, '云端动画')
  assert.equal(store.state.animeDetail.collectionType, 3)
  assert.equal(store.state.animeDetail.loading, true)
  assert.equal(requests[0].url, '/api/animes')
  requests[0].resolve([])
  await flush()
  assert.equal(store.state.animeDetail.loading, false)
  assert.equal(store.state.animeDetail.data.id, -101)
  assert.equal(requests.length, 1)
})

test('云端收藏发现本地资源时切换真实详情并保留收藏状态', async () => {
  const { store, requests } = dialogStore()
  store.openAnimeDetail(-101, cloud)
  requests[0].resolve([{ id: 7, bangumi_id: 101 }])
  await flush()
  assert.equal(requests[1].url, '/api/animes/7')
  requests[1].resolve({ id: 7, title: '本地动画', saved_count: 2 })
  await flush()
  assert.equal(store.state.animeDetail.id, 7)
  assert.equal(store.state.animeDetail.data.saved_count, 2)
  assert.equal(store.state.animeDetail.collectionType, 3)
  assert.equal(store.state.animeDetail.loading, false)
  const refresh = store.reloadAnimeDetail()
  assert.equal(requests[2].url, '/api/animes/7')
  requests[2].resolve({ id: 7, title: '新资源' })
  await refresh
})

test('云端资源查询失败可重试，关闭或切换后旧查询不再请求或覆盖', async () => {
  const { store, requests } = dialogStore()
  store.openAnimeDetail(-101, cloud)
  requests[0].reject(new Error('读取失败'))
  await flush()
  assert.equal(store.state.animeDetail.error, '读取失败')
  assert.equal(store.state.animeDetail.data.id, -101)
  const refresh = store.reloadAnimeDetail()
  requests[1].resolve([])
  await refresh
  assert.equal(store.state.animeDetail.error, '')
  store.openAnimeDetail(-101, cloud)
  store.state.animeDetail.open = false
  requests[2].resolve([{ id: 7, bangumi_id: 101 }])
  await flush()
  assert.equal(requests.length, 3)
  store.openAnimeDetail(-101, cloud)
  store.openAnimeDetail(8)
  requests[3].resolve([{ id: 7, bangumi_id: 101 }])
  requests[4].resolve({ id: 8, title: '另一个番剧' })
  await flush()
  assert.equal(requests.length, 5)
  assert.equal(store.state.animeDetail.data.id, 8)
  assert.equal(store.state.animeDetail.collectionType, null)
})

test('Bangumi 详情按打开加载，资源轮询不重复拉取，资料失败不影响资源状态', async () => {
  const { store, requests, subjects } = dialogStore()
  store.openAnimeDetail(-101, cloud)
  assert.equal(subjects[0].url, '/api/bangumi/subjects/101')
  requests[0].resolve([])
  subjects[0].reject(new Error('资料暂不可用'))
  await flush()
  assert.equal(store.state.animeDetail.error, '')
  assert.equal(store.state.animeDetail.subjectError, '资料暂不可用')
  const refresh = store.reloadAnimeDetail()
  requests[1].resolve([])
  await refresh
  assert.equal(subjects.length, 1)
  store.reloadBangumiSubject()
  subjects[1].resolve({ id: 101, rating: 8.5, summary: '官方简介' })
  await flush()
  assert.equal(store.state.animeDetail.subject.rating, 8.5)
  assert.equal(store.state.animeDetail.subjectError, '')
  assert.equal(store.state.animeDetail.data.item_count, 0)
})

test('关闭、切换和重试取消旧 Bangumi 请求，迟到响应不覆盖新资料', async () => {
  const { store, requests, subjects } = dialogStore()
  store.openAnimeDetail(-101, cloud)
  store.state.animeDetail.open = false
  assert.equal(subjects[0].options.signal.aborted, true)
  subjects[0].resolve({ id: 101, summary: '迟到简介' })
  requests[0].resolve([])
  await flush()
  assert.equal(store.state.animeDetail.subject, null)
  store.openAnimeDetail(-101, cloud)
  store.reloadBangumiSubject()
  assert.equal(subjects[1].options.signal.aborted, true)
  subjects[2].resolve({ id: 101, rating: 8 })
  subjects[1].reject(new Error('旧错误'))
  await flush()
  assert.equal(store.state.animeDetail.subject.rating, 8)
  assert.equal(store.state.animeDetail.subjectError, '')
  store.openAnimeDetail(7, { id: 7, bangumi_id: 202 })
  assert.equal(store.state.animeDetail.subject, null)
  subjects[3].resolve({ id: 202, summary: '新番简介' })
  await flush()
  assert.equal(store.state.animeDetail.subject.id, 202)
})
