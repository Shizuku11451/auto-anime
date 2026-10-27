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
const source = parse(readFileSync(new URL('./LibraryView.vue', import.meta.url), 'utf8')).descriptor.scriptSetup.content
const compiled = compile(source + '\nexport { weekdayList, weekdayIndicator, chooseDay, date, clearFilters, library, list, matching, filter, scope, collection, hasFilters, sort, selected, chosen, selectVisible, allSelected, openAnime, loadWishes, wishes, wishError };')
const schedule = { exports: {} }
runInNewContext(compile(readFileSync(new URL('../components/library/schedule.ts', import.meta.url), 'utf8')), schedule)
const types = { exports: {} }
runInNewContext(compile(readFileSync(new URL('../api/types.ts', import.meta.url), 'utf8')), types)

function fixture(t) {
  let resize
  let unmount
  const requests = [], opened = [], routes = []
  const store = { animes: vue.ref([]), loaded: vue.ref(false) }
  const modules = {
    vue: { ...vue, onMounted() {}, onBeforeUnmount(fn) { unmount = fn } },
    '@vueuse/core': { useResizeObserver(_target, callback) { resize = callback }, useIntervalFn() {} },
    'vue-router': { useRouter: () => ({ push: route => routes.push(route) }) },
    '@/api/client': { http: { get: (url, options) => new Promise((resolve, reject) => requests.push({ url, options, resolve, reject })) }, errorMessage: e => e.message },
    '@/stores/app': { useAppStore: () => store },
    '@/stores/dialogs': { useDialogs: () => ({ openAnimeDetail: id => opened.push(id) }) },
    '@/components/library/schedule': schedule.exports,
    '@/api/types': types.exports,
  }
  const context = { exports: {}, AbortController, require: id => {
    assert.ok(id in modules, `unexpected import: ${id}`)
    return modules[id]
  } }
  const scope = vue.effectScope()
  scope.run(() => runInNewContext(compiled, context))
  t.after(() => scope.stop())
  return { h: context.exports, store, requests, opened, routes, resize: () => resize(), unmount: () => unmount() }
}

test('星期滑块跟随按钮、日期和清除筛选，尺寸变化与数据刷新后重新对齐', async t => {
  const { h, store, resize } = fixture(t)
  let button = { offsetLeft: 3, offsetWidth: 90, offsetHeight: 41 }
  h.weekdayList.value = { querySelector: () => button }
  await vue.nextTick()
  const check = () => {
    assert.equal(h.weekdayIndicator.value.transform, `translateX(${button.offsetLeft}px)`)
    assert.equal(h.weekdayIndicator.value.width, `${button.offsetWidth}px`)
    assert.equal(h.weekdayIndicator.value.height, `${button.offsetHeight}px`)
  }
  check()
  button = { offsetLeft: 280, offsetWidth: 105, offsetHeight: 41 }
  h.chooseDay(3)
  await vue.nextTick()
  check()
  button.offsetLeft = 480
  h.date.value = '2026-10-25'
  await vue.nextTick()
  check()
  button.offsetWidth = 72
  resize()
  check()
  button.offsetWidth = 80
  store.loaded.value = true
  await vue.nextTick()
  check()
  button.offsetLeft = 3
  h.clearFilters()
  await vue.nextTick()
  check()
})

test('想看直接进入番剧库，按 Bangumi ID 去重并参与搜索、排序与筛选', t => {
  const { h, store, opened, routes } = fixture(t)
  store.animes.value = [{ id: 7, bangumi_id: 101, title: '本地动画', item_count: 2, saved_count: 1, update_weekday: 3 }]
  h.wishes.value = { items: [
    { id: 101, name: 'duplicate', name_cn: '重复条目', type: 3, cover: 'https://example.test/local-cover.jpg' },
    { id: 102, name: 'Wish', name_cn: '想看动画', cover: 'https://example.test/cover.jpg', type: 1 },
    { id: 103, name: '无封面动画', name_cn: '', type: 5 },
  ] }
  assert.equal(h.library.value.length, 3)
  assert.equal(h.library.value[0].cover, 'https://example.test/local-cover.jpg')
  assert.equal(h.library.value[1].cover, 'https://example.test/cover.jpg')
  assert.equal(h.library.value[2].cover, '')
  h.openAnime(7)
  h.openAnime(-102)
  assert.deepEqual(opened, [7])
  assert.equal(routes[0].path, '/discover')
  assert.equal(routes[0].query.keyword, '想看动画')
  h.filter.value = 'wish'
  assert.equal(h.list.value[0].id, -102)
  h.filter.value = ''
  h.sort.value = 'resources'
  assert.equal(h.list.value[0].id, 7)
  h.scope.value = 'empty'
  assert.equal(h.list.value.length, 2)
  h.chooseDay(0)
  assert.equal(h.list.value.length, 2)
  h.scope.value = 'saved'
  h.chooseDay(-1)
  assert.equal(h.list.value.length, 1)
})

test('批量全选只管理本地番剧，解除连接后移除想看但保留本地番剧', t => {
  const { h, store } = fixture(t)
  store.animes.value = [{ id: 7, bangumi_id: 101, title: '本地动画', item_count: 0, saved_count: 0 }]
  h.wishes.value = { items: [{ id: 102, name: '想看动画', name_cn: '', type: 1 }] }
  h.selectVisible()
  assert.deepEqual([...h.selected.value], [7])
  assert.equal(h.chosen.value.length, 1)
  assert.equal(h.allSelected.value, true)
  h.selectVisible()
  assert.equal(h.selected.value.length, 0)
  h.filter.value = '想看'
  h.selectVisible()
  assert.equal(h.selected.value.length, 0)
  assert.equal(h.allSelected.value, false)
  h.wishes.value = { items: [] }
  assert.equal(h.library.value.length, 1)
  assert.equal(h.library.value[0].id, 7)
})

test('五种收藏状态可筛选，本地条目合并状态并保留资源和管理能力', async t => {
  const { h, store, opened } = fixture(t)
  store.animes.value = [
    { id: 7, bangumi_id: 101, title: '本地动画', cover: '/local.jpg', item_count: 2, saved_count: 1 },
    { id: 8, bangumi_id: 0, title: '未关联动画', item_count: 1, saved_count: 1 },
  ]
  h.wishes.value = { items: [1, 2, 3, 4, 5].map(type => ({ id: 100 + type, name: `动画${type}`, name_cn: '', cover: '/remote.jpg', type })) }
  assert.equal(h.library.value.length, 6)
  assert.equal(h.library.value[0].collectionType, 1)
  assert.equal(h.library.value[0].item_count, 2)
  assert.equal(h.library.value[0].saved_count, 1)
  assert.equal(h.library.value[0].cover, '/local.jpg')
  assert.equal(h.library.value[0].cloudOnly, undefined)
  for (const type of [1, 2, 3, 4, 5]) {
    h.collection.value = String(type)
    assert.equal(h.list.value.length, 1)
    assert.equal(h.list.value[0].collectionType, type)
  }
  h.collection.value = 'none'
  assert.equal(h.list.value.length, 1)
  assert.equal(h.list.value[0].id, 8)
  assert.equal(h.hasFilters.value, true)
  h.clearFilters()
  assert.equal(h.collection.value, 'all')
  assert.equal(h.hasFilters.value, false)
  h.selectVisible()
  assert.deepEqual([...h.selected.value], [7, 8])
  h.openAnime(7)
  assert.deepEqual(opened, [7])
  h.collection.value = '1'
  await vue.nextTick()
  assert.equal(h.selected.value.length, 0)
  h.selectVisible()
  assert.deepEqual([...h.selected.value], [7])
  h.wishes.value = { items: [{ id: 101, name: '动画', name_cn: '', type: 4 }] }
  assert.equal(h.library.value[0].collectionType, 4)
  assert.equal(h.list.value.length, 0)
  await vue.nextTick()
  assert.equal(h.selected.value.length, 0)
  h.wishes.value = { items: [] }
  assert.equal(h.library.value.length, 2)
  assert.equal(h.library.value[0].collectionType, undefined)
  assert.equal(h.library.value[0].saved_count, 1)
})

test('想看请求忽略旧响应、旧错误和离开页面后的响应，失败保留上次结果', async t => {
  const { h, requests, unmount } = fixture(t)
  const old = h.loadWishes(), fresh = h.loadWishes()
  assert.equal(requests[0].options.signal.aborted, true)
  requests[1].resolve({ items: [{ id: 1, name: '新结果', type: 3 }] }); await fresh
  requests[0].resolve({ items: [{ id: 2, name: '旧结果', type: 1 }] }); await old
  assert.equal(h.library.value[0].id, -1)
  const fail = h.loadWishes()
  requests[2].reject(new Error('读取失败')); await fail
  assert.equal(h.library.value[0].id, -1)
  assert.equal(h.wishError.value, '读取失败')
  const late = h.loadWishes(), latest = h.loadWishes()
  requests[4].resolve({ items: [] }); await latest
  requests[3].reject(new Error('旧错误')); await late
  assert.equal(h.wishError.value, '')
  const leaving = h.loadWishes()
  unmount()
  requests[5].resolve({ items: [{ id: 3, name: '迟到结果', type: 1 }] }); await leaving
  assert.equal(h.library.value.length, 0)
})
