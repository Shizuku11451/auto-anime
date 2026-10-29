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
const compiled = compile(source + '\nexport { weekdayList, weekdayIndicator, chooseDay, date, weekday, clearFilters, library, list, matching, filter, scope, collection, hasFilters, sort, selected, chosen, selectVisible, allSelected, openAnime, loadWishes, wishes, wishError, layout, dateOpen, showToday, collectionTabs, collectionCounts, collectionLabel, selecting, toggleMode, search, onKeydown };')
const schedule = { exports: {} }
runInNewContext(compile(readFileSync(new URL('../components/library/schedule.ts', import.meta.url), 'utf8')), schedule)
const types = { exports: {} }
runInNewContext(compile(readFileSync(new URL('../api/types.ts', import.meta.url), 'utf8')), types)

function fixture(t, { query = {}, storage = new Map(), dialogOpen = false } = {}) {
  let resize
  let unmount
  let keydown
  const requests = [], opened = [], routes = [], replaced = []
  const store = { animes: vue.ref([]), loaded: vue.ref(false) }
  const route = vue.reactive({ query })
  const localStorage = { getItem: key => storage.get(key) ?? null, setItem: (key, value) => storage.set(key, String(value)) }
  const modules = {
    vue: { ...vue, onMounted() {}, onBeforeUnmount(fn) { unmount = fn } },
    '@vueuse/core': { useResizeObserver(_target, callback) { resize = callback }, useIntervalFn() {}, useEventListener(_target, _event, fn) { keydown = fn } },
    'vue-router': {
      useRoute: () => route,
      useRouter: () => ({ push: next => routes.push(next), replace: next => { replaced.push(next.query); route.query = next.query; return Promise.resolve() } }),
    },
    '@/api/client': { http: { get: (url, options) => new Promise((resolve, reject) => requests.push({ url, options, resolve, reject })) }, errorMessage: e => e.message },
    '@/stores/app': { useAppStore: () => store },
    '@/stores/dialogs': { useDialogs: () => ({ openAnimeDetail: (id, entry) => opened.push({ id, entry }) }) },
    '@/components/library/schedule': schedule.exports,
    '@/composables/dialogStack': { hasOpenDialog: () => dialogOpen },
    '@/api/types': types.exports,
  }
  const context = { exports: {}, AbortController, window: {}, globalThis: { localStorage }, require: id => {
    assert.ok(id in modules, `unexpected import: ${id}`)
    return modules[id]
  } }
  const scope = vue.effectScope()
  scope.run(() => runInNewContext(compiled, context))
  t.after(() => scope.stop())
  return { h: context.exports, store, requests, opened, routes, replaced, route, storage, keydown: event => keydown(event), resize: () => resize(), unmount: () => unmount() }
}

function key(name, target = { tagName: 'BODY' }) {
  const event = { key: name, target, ctrlKey: false, metaKey: false, altKey: false, defaultPrevented: false, prevented: false }
  event.preventDefault = () => { event.prevented = true }
  return event
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
    { id: 101, name: 'duplicate', name_cn: '重复条目', type: 3, rating: 8.1, cover: 'https://example.test/local-cover.jpg' },
    { id: 102, name: 'Wish', name_cn: '想看动画', rating: 7.6, cover: 'https://example.test/cover.jpg', type: 1 },
    { id: 103, name: '无封面动画', name_cn: '', type: 5 },
  ] }
  assert.equal(h.library.value.length, 3)
  assert.equal(h.library.value[0].cover, 'https://example.test/local-cover.jpg')
  assert.equal(h.library.value[1].cover, 'https://example.test/cover.jpg')
  assert.equal(h.library.value[2].cover, '')
  assert.equal(h.library.value[0].rating, 8.1)
  assert.equal(h.library.value[1].rating, 7.6)
  assert.equal(h.library.value[2].rating, 0)
  h.openAnime(7)
  h.openAnime(-102)
  assert.deepEqual(opened.map(a => a.id), [7, -102])
  assert.equal(opened[0].entry.collectionType, 3)
  assert.equal(opened[1].entry.cloudOnly, true)
  assert.equal(opened[1].entry.meta_name_cn, '想看动画')
  assert.equal(routes.length, 0)
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
  assert.deepEqual(opened.map(a => a.id), [7])
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

test('筛选从地址恢复，修改后写回地址，非法参数回退默认值', async t => {
  const { h, replaced, route } = fixture(t, { query: { q: '测试', collection: '3', scope: 'saved', sort: 'rating', day: '5' } })
  assert.equal(h.filter.value, '测试')
  assert.equal(h.collection.value, '3')
  assert.equal(h.scope.value, 'saved')
  assert.equal(h.sort.value, 'rating')
  assert.equal(h.weekday.value, 5)
  assert.equal(h.hasFilters.value, true)
  h.collection.value = 'all'
  h.filter.value = '  新关键词  '
  await vue.nextTick()
  assert.deepEqual({ ...route.query }, { q: '新关键词', scope: 'saved', sort: 'rating', day: '5' })
  h.clearFilters()
  h.sort.value = 'recent'
  await vue.nextTick()
  assert.deepEqual({ ...route.query }, {})
  const count = replaced.length
  h.filter.value = ''
  await vue.nextTick()
  assert.equal(replaced.length, count)

  const bad = fixture(t, { query: { collection: '9', scope: 'x', sort: 'drop', day: '9', date: '2026-02-30' } }).h
  assert.equal(bad.collection.value, 'all')
  assert.equal(bad.scope.value, 'all')
  assert.equal(bad.sort.value, 'recent')
  assert.equal(bad.weekday.value, -1)
  assert.equal(bad.date.value, '')
  assert.equal(bad.hasFilters.value, false)
})

test('页面复用时地址从外部改变，筛选跟随地址且不来回写入', async t => {
  const { h, route, replaced } = fixture(t, { query: { collection: '1', sort: 'rating' } })
  assert.equal(h.collection.value, '1')
  route.query = {}
  await vue.nextTick()
  assert.equal(h.collection.value, 'all')
  assert.equal(h.sort.value, 'recent')
  assert.equal(h.hasFilters.value, false)
  route.query = { q: '甲', date: '2026-10-28', day: '3' }
  await vue.nextTick()
  assert.equal(h.filter.value, '甲')
  assert.equal(h.date.value, '2026-10-28')
  assert.equal(h.weekday.value, -1)
  assert.equal(h.dateOpen.value, true)
  await vue.nextTick()
  assert.equal(replaced.length, 0)
})

test('日期参数优先于星期并展开日期面板，选今天时写入日期', async t => {
  const { h, route } = fixture(t, { query: { day: '2', date: '2026-10-28' } })
  assert.equal(h.date.value, '2026-10-28')
  assert.equal(h.weekday.value, -1)
  assert.equal(h.dateOpen.value, true)
  h.chooseDay(4)
  await vue.nextTick()
  assert.deepEqual({ ...route.query }, { day: '4' })
  const other = fixture(t).h
  assert.equal(other.dateOpen.value, false)
  other.showToday()
  assert.equal(other.dateOpen.value, true)
  assert.match(other.date.value, /^\d{4}-\d{2}-\d{2}$/)
})

test('默认海报墙，展示方式记在本机，读取异常值回退海报墙', async t => {
  const storage = new Map()
  const first = fixture(t, { storage }).h
  assert.equal(first.layout.value, 'poster')
  first.layout.value = 'compact'
  await vue.nextTick()
  assert.equal(storage.get('auto-anime:library-layout'), 'compact')
  assert.equal(fixture(t, { storage }).h.layout.value, 'compact')
  storage.set('auto-anime:library-layout', 'other')
  assert.equal(fixture(t, { storage }).h.layout.value, 'poster')
})

test('收藏标签带计数，只显示有内容的状态，无收藏时隐藏', t => {
  const { h, store } = fixture(t)
  store.loaded.value = true
  store.animes.value = [
    { id: 7, bangumi_id: 101, title: '在看本地', item_count: 1, saved_count: 1, update_weekday: 2 },
    { id: 8, bangumi_id: 0, title: '未关联', item_count: 0, saved_count: 0, update_weekday: 3 },
  ]
  assert.deepEqual([...h.collectionTabs.value], [])
  h.wishes.value = { items: [
    { id: 101, name: '在看本地', name_cn: '', type: 3 },
    { id: 102, name: '想看甲', name_cn: '', type: 1 },
    { id: 103, name: '想看乙', name_cn: '', type: 1 },
  ] }
  assert.deepEqual([...h.collectionTabs.value], ['all', '1', '3', 'none'])
  assert.equal(h.collectionCounts.value.all, 4)
  assert.equal(h.collectionCounts.value['1'], 2)
  assert.equal(h.collectionCounts.value['3'], 1)
  assert.equal(h.collectionCounts.value.none, 1)
  assert.equal(h.collectionLabel('1'), '想看')
  assert.equal(h.collectionLabel('none'), '未收藏')
  // 计数不受当前收藏标签影响，但跟随搜索与星期。
  h.collection.value = '1'
  assert.equal(h.collectionCounts.value.all, 4)
  assert.equal(h.list.value.length, 2)
  h.chooseDay(2)
  assert.equal(h.collectionCounts.value.all, 1)
  assert.equal(h.collectionCounts.value['3'], 1)
  // 当前选中的标签即使为空也保留，方便切回。
  assert.deepEqual([...h.collectionTabs.value], ['all', '1', '3'])
  assert.equal(h.list.value.length, 0)
})

test('评分与更新日排序，名称排序优先中文名', t => {
  const { h, store } = fixture(t)
  store.animes.value = [
    { id: 1, bangumi_id: 0, title: 'b-title', meta_name_cn: '乙', item_count: 0, saved_count: 0, rating: 6.5, update_weekday: 0 },
    { id: 2, bangumi_id: 0, title: 'a-title', meta_name_cn: '丙', item_count: 0, saved_count: 0, rating: 8.2, update_weekday: 5 },
    { id: 3, bangumi_id: 0, title: 'c-title', meta_name_cn: '', item_count: 0, saved_count: 0, rating: 0, update_weekday: 1 },
  ]
  h.sort.value = 'rating'
  assert.deepEqual([...h.list.value.map(a => a.id)], [2, 1, 3])
  h.sort.value = 'weekday'
  assert.deepEqual([...h.list.value.map(a => a.id)], [3, 2, 1])
  h.sort.value = 'title'
  assert.equal(h.list.value.at(-1).id, 3)
})

test('「/」聚焦搜索，Esc 退出批量管理；输入中或弹窗打开时不拦截', t => {
  const { h, store, keydown } = fixture(t)
  store.animes.value = [{ id: 7, bangumi_id: 0, title: '本地', item_count: 0, saved_count: 0 }]
  let focused = 0
  h.search.value = { focus: () => { focused++ } }
  const slash = key('/')
  keydown(slash)
  assert.equal(focused, 1)
  assert.equal(slash.prevented, true)
  const typing = key('/', { tagName: 'INPUT' })
  keydown(typing)
  assert.equal(focused, 1)
  assert.equal(typing.prevented, false)
  h.toggleMode()
  h.selectVisible()
  assert.equal(h.selecting.value, true)
  keydown(key('Escape', { tagName: 'INPUT' }))
  assert.equal(h.selecting.value, true)
  keydown(key('Escape'))
  assert.equal(h.selecting.value, false)
  assert.equal(h.selected.value.length, 0)

  const blocked = fixture(t, { dialogOpen: true })
  blocked.h.search.value = { focus: () => { focused++ } }
  blocked.h.toggleMode()
  blocked.keydown(key('/'))
  blocked.keydown(key('Escape'))
  assert.equal(focused, 1)
  assert.equal(blocked.h.selecting.value, true)
})

test('自动识别的 Bangumi 条目也与收藏合并，手动绑定优先，同一条目只合并一次', t => {
  const { h, store } = fixture(t)
  store.animes.value = [
    // 资料自动识别为 910001，未手动绑定：应与收藏合并为一张卡片。
    { id: 1, bangumi_id: 0, meta_subject_id: 910001, title: '自动识别', item_count: 9, saved_count: 9 },
    // 手动绑定到 700，资料曾识别为别的条目：以手动绑定为准。
    { id: 2, bangumi_id: 700, meta_subject_id: 0, title: '手动绑定', item_count: 1, saved_count: 0 },
    // 自动识别也指向 700：已被手动绑定占用，不再合并。
    { id: 3, bangumi_id: 0, meta_subject_id: 700, title: '重复识别', item_count: 1, saved_count: 0 },
  ]
  h.wishes.value = { items: [
    { id: 910001, name: 'Example Animation', name_cn: '虚构测试番剧', type: 3, rating: 6.6, cover: '/remote.jpg' },
    { id: 700, name: 'Manual', name_cn: '', type: 1 },
    { id: 800, name: 'Cloud', name_cn: '', type: 2 },
  ] }
  const byId = Object.fromEntries(h.library.value.map(a => [a.id, a]))
  assert.equal(h.library.value.length, 4)
  assert.equal(byId[1].collectionType, 3)
  assert.equal(byId[1].cover, '/remote.jpg')
  assert.equal(byId[1].rating, 6.6)
  assert.equal(byId[2].collectionType, 1)
  assert.equal(byId[3].collectionType, undefined)
  assert.equal(byId[-910001], undefined)
  assert.equal(byId[-700], undefined)
  assert.equal(byId[-800].cloudOnly, true)
})

test('未下载的收藏使用 Bangumi 简介，本地简介优先', t => {
  const { h, store } = fixture(t)
  store.animes.value = [
    { id: 1, bangumi_id: 10, title: '本地有简介', summary: '本地简介', item_count: 0, saved_count: 0 },
    { id: 2, bangumi_id: 20, title: '本地无简介', summary: '', item_count: 0, saved_count: 0 },
  ]
  h.wishes.value = { items: [
    { id: 10, name: 'a', name_cn: '', type: 1, summary: '收藏简介甲' },
    { id: 20, name: 'b', name_cn: '', type: 1, summary: '收藏简介乙' },
    { id: 30, name: 'c', name_cn: '', type: 1, summary: '云端简介' },
    { id: 40, name: 'd', name_cn: '', type: 1 },
  ] }
  const byId = Object.fromEntries(h.library.value.map(a => [a.id, a]))
  assert.equal(byId[1].summary, '本地简介')
  assert.equal(byId[2].summary, '收藏简介乙')
  assert.equal(byId[-30].summary, '云端简介')
  assert.equal(byId[-40].summary, '')
})

test('云端收藏的放送星期参与筛选，旧缓存仍归入待补充', t => {
  const { h, store } = fixture(t)
  store.animes.value = [{ id: 1, bangumi_id: 10, title: '手动周二', update_weekday: 2, broadcast_weekday: 4, item_count: 0, saved_count: 0 }]
  h.wishes.value = { items: [
    { id: 10, name: 'local', type: 3, broadcast_weekday: 4 },
    { id: 20, name: 'airing', type: 3, broadcast_weekday: 4 },
    { id: 30, name: 'legacy', type: 1 },
  ] }
  h.chooseDay(4)
  assert.deepEqual([...h.list.value.map(a => a.id)], [-20])
  h.chooseDay(2)
  assert.deepEqual([...h.list.value.map(a => a.id)], [1])
  h.chooseDay(0)
  assert.deepEqual([...h.list.value.map(a => a.id)], [-30])
})

test('收藏的播放结束日期让云端条目和缺资料的本地番剧归入已完结', t => {
  const { h, store } = fixture(t)
  store.animes.value = [
    { id: 1, bangumi_id: 10, title: '本地缺资料', update_weekday: 0, item_count: 1, saved_count: 0 },
    { id: 2, bangumi_id: 20, title: '本地手动', update_weekday: 3, item_count: 1, saved_count: 0 },
  ]
  h.wishes.value = { items: [
    { id: 10, name: 'local', type: 2, broadcast_end: '2005-03-31' },
    { id: 20, name: 'manual', type: 2, broadcast_end: '2005-03-31' },
    { id: 30, name: 'cloud', type: 1, broadcast_weekday: 4, broadcast_end: '2018-09-23' },
    { id: 40, name: 'airing', type: 3, broadcast_weekday: 2, broadcast_end: '2999-01-01' },
  ] }
  h.chooseDay(8)
  assert.deepEqual([...h.list.value.map(a => a.id)].sort(), [-30, 1])
  h.chooseDay(2)
  assert.deepEqual([...h.list.value.map(a => a.id)], [-40])
  h.chooseDay(3)
  assert.deepEqual([...h.list.value.map(a => a.id)], [2])
})
